// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package repositories

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/vault"
)

const (
	// secretJobLease is how long a claimed job is held; one attempt is a vault call and two short transactions.
	secretJobLease          = 2 * time.Minute
	secretJobAttemptTimeout = time.Minute
	// secretJobBatch is how many jobs one pass claims.
	secretJobBatch = 50
	// secretJobMaxBackoff caps the wait between attempts of a failing job.
	secretJobMaxBackoff = time.Hour
	// secretJobAlarmAttempts is how many failed attempts make a job log at Error rather than Warn.
	secretJobAlarmAttempts = 5
)

// errPromotionSuperseded is a promotion whose secret was rewritten or deleted while its value was being written.
var errPromotionSuperseded = errors.New("secret changed while being promoted")

var errSecretJobClaimLost = errors.New("secret job claim expired or was taken over")

type secretJobAttempt struct {
	*models.SecretJob
	now func() time.Time
}

func (a *secretJobAttempt) claimed(db *gorm.DB) *gorm.DB {
	return db.Where("id = ? AND claimed_by = ? AND claimed_until >= ?", a.ID, a.ClaimedBy, a.now())
}

func requireSecretJobChange(res *gorm.DB) error {
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errSecretJobClaimLost
	}
	return nil
}

// SecretJobRunner does the external-vault work SecretStore queues in secret_jobs once the transaction that queued it
// has committed. Jobs are claimed with a lease, so any number of replicas can run passes, and a failed job is retried
// with backoff until it succeeds. Every step is safe to repeat.
type SecretJobRunner struct {
	db             *gorm.DB
	vaults         *vault.Vaults
	logger         *slog.Logger
	attemptTimeout time.Duration
}

// NewSecretJobRunner runs secret jobs against db with the deployment's vaults.
func NewSecretJobRunner(db *gorm.DB, vaults *vault.Vaults, logger *slog.Logger) *SecretJobRunner {
	return &SecretJobRunner{db: db, vaults: vaults, logger: logger, attemptTimeout: secretJobAttemptTimeout}
}

// RunOnce claims the jobs due at now and runs them, returning how many it ran and how many failed. err is a failure to
// claim; a failed job is recorded on it and retried later.
func (r *SecretJobRunner) RunOnce(ctx context.Context, now time.Time) (ran, failed int, err error) {
	started := time.Now()
	clock := func() time.Time { return now.Add(time.Since(started)).UTC() }
	claimed, err := dbx.ClaimRows(ctx, r.db, dbx.Claim{
		Table: models.SecretJob{}.TableName(),
		Key:   []string{"id"},
		Due: func(q *gorm.DB) *gorm.DB {
			return q.Where("available_at <= ?", now).Order("available_at, id")
		},
		Limit: secretJobBatch,
		Now:   now,
		Lease: secretJobLease,
	})
	if err != nil {
		return 0, 0, err
	}
	for _, key := range claimed.Keys {
		var job models.SecretJob
		if err := r.db.WithContext(ctx).Where("id = ? AND claimed_by = ?", key[0], claimed.Token).Take(&job).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return ran, failed, fmt.Errorf("failed to read claimed secret job: %w", err)
		}
		attempt := &secretJobAttempt{SecretJob: &job, now: clock}
		if job.AvailableAt.After(now) {
			// Another runner ran and rescheduled the job between this pass selecting and claiming it. Hand the
			// claim back so the job waits out its backoff.
			if err := r.release(ctx, attempt); err != nil && !errors.Is(err, errSecretJobClaimLost) {
				return ran, failed, err
			}
			continue
		}
		if err := r.renew(ctx, attempt); err != nil {
			if errors.Is(err, errSecretJobClaimLost) {
				continue
			}
			return ran, failed, err
		}
		ran++
		if err := r.runWithLease(ctx, attempt); err != nil && !errors.Is(err, errSecretJobClaimLost) {
			failed++
			r.retryLater(ctx, attempt, err)
		}
	}
	return ran, failed, nil
}

// release gives up the claim without counting an attempt.
func (r *SecretJobRunner) release(ctx context.Context, job *secretJobAttempt) error {
	return requireSecretJobChange(job.claimed(r.db.WithContext(ctx).Model(&models.SecretJob{})).
		Updates(map[string]any{"claimed_by": nil, "claimed_until": nil}))
}

func (r *SecretJobRunner) renew(ctx context.Context, job *secretJobAttempt) error {
	return requireSecretJobChange(job.claimed(r.db.WithContext(ctx).Model(&models.SecretJob{})).
		Update("claimed_until", job.now().Add(secretJobLease)))
}

func (r *SecretJobRunner) runWithLease(ctx context.Context, job *secretJobAttempt) error {
	deadlineCtx, stopDeadline := context.WithTimeout(ctx, r.attemptTimeout)
	defer stopDeadline()
	workCtx, cancel := context.WithCancelCause(deadlineCtx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(secretJobLease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-workCtx.Done():
				return
			case <-ticker.C:
				if err := r.renew(workCtx, job); err != nil {
					cancel(err)
					return
				}
			}
		}
	}()
	err := r.run(workCtx, job)
	cause := context.Cause(workCtx)
	cancel(nil)
	<-done
	if err != nil && cause != nil {
		return cause
	}
	return err
}

func (r *SecretJobRunner) run(ctx context.Context, job *secretJobAttempt) error {
	switch job.Action {
	case models.SecretJobDestroy:
		return r.destroy(ctx, job)
	case models.SecretJobPromote:
		return r.promote(ctx, job)
	case models.SecretJobAbandon:
		if job.Provider == nil || job.ExternalRef == nil {
			return errors.New("abandon job is missing its vault reference")
		}
		backend, err := r.vaults.For(*job.Provider)
		if err != nil {
			return err
		}
		external, ok := backend.(vault.ReferenceVault)
		if !ok {
			return errors.New("vault cannot abandon staged references")
		}
		if err := external.Abandon(ctx, jobRef(job.SecretJob), *job.ExternalRef); err != nil {
			return err
		}
		return r.finishQueueing(ctx, job)
	default:
		return fmt.Errorf("unknown secret job action %q", job.Action)
	}
}

// destroy removes the job's external value unless a committed row, or a promotion still pending, refers to it: then
// that row owns it and the job is dropped.
func (r *SecretJobRunner) destroy(ctx context.Context, job *secretJobAttempt) error {
	if job.Provider == nil || job.ExternalRef == nil {
		return r.finishQueueing(ctx, job)
	}
	value := externalValue{provider: *job.Provider, ref: *job.ExternalRef}
	inUse, err := r.inUse(ctx, value)
	if err != nil {
		return err
	}
	if !inUse {
		if err := r.renew(ctx, job); err != nil {
			return err
		}
		if err := r.destroyValue(ctx, jobRef(job.SecretJob), value); err != nil {
			return err
		}
	}
	return r.finishQueueing(ctx, job)
}

// inUse reports whether a committed row, or a promotion still pending, refers to value.
func (r *SecretJobRunner) inUse(ctx context.Context, value externalValue) (bool, error) {
	db := r.db.WithContext(ctx)
	var rows int64
	if err := db.Model(&models.Secret{}).Where("provider = ? AND external_ref = ?", value.provider, value.ref).
		Count(&rows).Error; err != nil {
		return false, fmt.Errorf("failed to check whether a secret refers to an external value: %w", err)
	}
	if rows > 0 {
		return true, nil
	}
	var promotions int64
	if err := db.Model(&models.SecretJob{}).
		Where("action = ? AND version IS NOT NULL AND provider = ? AND external_ref = ?", models.SecretJobPromote, value.provider, value.ref).
		Count(&promotions).Error; err != nil {
		return false, fmt.Errorf("failed to check whether a promotion refers to an external value: %w", err)
	}
	return promotions > 0, nil
}

func (r *SecretJobRunner) destroyValue(ctx context.Context, ref vault.Ref, value externalValue) error {
	v, err := r.vaults.For(value.provider)
	if err != nil {
		return err
	}
	if err := v.Destroy(ctx, ref, vault.Sealed{ExternalRef: value.ref}); err != nil {
		return fmt.Errorf("failed to remove a value from vault %s: %w", value.provider, err)
	}
	return nil
}

// promote copies the named version to a recoverable destination and publishes it
// only while this attempt still owns the job.
func (r *SecretJobRunner) promote(ctx context.Context, job *secretJobAttempt) error {
	staged := jobStagedValue(job.SecretJob)
	if job.Version == nil {
		// The superseding transaction took over the previous external value.
		return r.finishQueueing(ctx, job, staged)
	}

	var row models.Secret
	err := r.db.WithContext(ctx).Where("id = ?", job.SecretID).Take(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	replaced := jobExternalValue(job.SecretJob)
	if err != nil || row.Version != *job.Version || row.Provider != vault.ProviderLocal {
		return r.finishQueueing(ctx, job, staged, replaced)
	}

	if staged == nil {
		return fmt.Errorf("secret promotion %s has no recorded vault target; save the credential again to queue a new promotion", job.ID)
	}
	writer, err := r.vaults.For(staged.provider)
	if err != nil {
		return err
	}
	external, ok := writer.(vault.ExternalVault)
	if !ok {
		return fmt.Errorf("secret vault %q does not support recoverable writes", writer.Provider())
	}

	local, err := r.vaults.For(vault.ProviderLocal)
	if err != nil {
		return err
	}
	plaintext, err := local.Open(ctx, secretRef(&row), sealedOf(&row))
	if err != nil {
		return fmt.Errorf("failed to open secret %s for promotion: %w", row.ID, err)
	}
	ref := jobRef(job.SecretJob)
	ref.OperationID = job.ID

	var current *vault.Sealed
	if replaced != nil && replaced.provider == writer.Provider() {
		current = &vault.Sealed{ExternalRef: replaced.ref}
	}
	if err := r.renew(ctx, job); err != nil {
		return err
	}
	sealed, err := external.Seal(ctx, ref, plaintext, current)
	if err != nil {
		return fmt.Errorf("failed to write secret %s to vault %s: %w", row.ID, writer.Provider(), err)
	}
	if sealed.ExternalRef != staged.ref {
		return fmt.Errorf("vault %s wrote to a different reference than it reserved", writer.Provider())
	}
	if err := r.publish(ctx, job, &row, sealed, staged.provider, replaced); err != nil {
		if errors.Is(err, errPromotionSuperseded) {
			return r.finishQueueing(ctx, job, staged, replaced)
		}
		return fmt.Errorf("failed to record secret %s in vault %s: %w", row.ID, writer.Provider(), err)
	}
	return nil
}

// publish locks the secret first, matching writes and deletes, then consumes the
// still-valid claim in the same transaction that publishes the external reference.
func (r *SecretJobRunner) publish(ctx context.Context, job *secretJobAttempt, row *models.Secret, sealed vault.Sealed, provider string, replaced *externalValue) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := dbx.LockRow(tx, models.Secret{}.TableName(), "version", "id = ? AND version = ? AND provider = ?", row.ID, row.Version, vault.ProviderLocal); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errPromotionSuperseded
			}
			return err
		}
		if err := requireSecretJobChange(job.claimed(tx).Where("version = ?", row.Version).Delete(&models.SecretJob{})); err != nil {
			return err
		}
		if err := tx.Model(&models.Secret{}).Where("id = ?", row.ID).Updates(map[string]any{
			"provider": provider, "ciphertext": sealed.Ciphertext, "key_id": nilIfEmpty(sealed.KeyID),
			"external_ref": sealed.ExternalRef, "updated_at": dbx.Now(),
		}).Error; err != nil {
			return err
		}
		if replaced != nil {
			return queueDestroy(tx, job.OrgID, job.SecretID, job.OwnerKind, *replaced)
		}
		return nil
	})
}

// finishQueueing consumes the current claim and hands cleanup to durable jobs
// together. A worker that lost its claim cannot destroy any staged value.
func (r *SecretJobRunner) finishQueueing(ctx context.Context, job *secretJobAttempt, values ...*externalValue) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireSecretJobChange(job.claimed(tx).Delete(&models.SecretJob{})); err != nil {
			return err
		}
		for _, value := range values {
			if value != nil {
				if err := queueDestroy(tx, job.OrgID, job.SecretID, job.OwnerKind, *value); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// retryLater releases the job's claim and makes it due again after a growing backoff.
func (r *SecretJobRunner) retryLater(ctx context.Context, job *secretJobAttempt, cause error) {
	attempts := job.Attempts + 1
	backoff := secretJobBackoff(attempts)
	err := requireSecretJobChange(job.claimed(r.db.WithContext(ctx).Model(&models.SecretJob{})).Updates(map[string]any{
		"attempts": attempts, "last_error": cause.Error(), "available_at": job.now().Add(backoff),
		"claimed_by": nil, "claimed_until": nil,
	}))
	if errors.Is(err, errSecretJobClaimLost) {
		return
	}
	if err != nil {
		r.logger.Error("Failed to schedule a secret vault job's retry; its claim expires instead", "jobID", job.ID, "error", err)
		return
	}
	level := slog.LevelWarn
	if attempts >= secretJobAlarmAttempts {
		level = slog.LevelError
	}
	r.logger.Log(ctx, level, "Secret vault job failed; it will be retried",
		"jobID", job.ID, "action", job.Action, "secretID", job.SecretID, "orgID", job.OrgID,
		"attempts", attempts, "retryIn", backoff, "error", cause)
}

func secretJobBackoff(attempts int) time.Duration {
	backoff := 15 * time.Second
	for i := 1; i < attempts && backoff < secretJobMaxBackoff; i++ {
		backoff *= 2
	}
	return min(backoff, secretJobMaxBackoff)
}

func jobRef(job *models.SecretJob) vault.Ref {
	return vault.Ref{OrgID: job.OrgID, SecretID: job.SecretID, OwnerKind: job.OwnerKind}
}

func jobExternalValue(job *models.SecretJob) *externalValue {
	if job.Provider == nil || job.ExternalRef == nil {
		return nil
	}
	return &externalValue{provider: *job.Provider, ref: *job.ExternalRef}
}

func jobStagedValue(job *models.SecretJob) *externalValue {
	if job.StagedProvider == nil || job.StagedRef == nil {
		return nil
	}
	return &externalValue{provider: *job.StagedProvider, ref: *job.StagedRef}
}

// PendingSecretJobs counts the secret jobs not yet done, for tests and diagnostics.
func PendingSecretJobs(ctx context.Context, db *gorm.DB, secretID uuid.UUID) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Model(&models.SecretJob{}).Where("secret_id = ?", secretID).Count(&n).Error
	return n, err
}
