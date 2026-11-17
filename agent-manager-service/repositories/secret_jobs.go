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

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/vault"
)

const (
	// secretJobLease is how long a claimed job is held; one attempt is a vault call and two short transactions.
	secretJobLease = 2 * time.Minute
	// secretJobBatch is how many jobs one pass claims.
	secretJobBatch = 50
	// secretJobMaxBackoff caps the wait between attempts of a failing job.
	secretJobMaxBackoff = time.Hour
	// secretJobAlarmAttempts is how many failed attempts make a job log at Error rather than Warn.
	secretJobAlarmAttempts = 5
)

// errPromotionSuperseded is a promotion whose secret was rewritten or deleted while its value was being written.
var errPromotionSuperseded = errors.New("secret changed while being promoted")

// SecretJobRunner does the external-vault work SecretStore queues in secret_jobs once the transaction that queued it
// has committed. Jobs are claimed with a lease, so any number of replicas can run passes, and a failed job is retried
// with backoff until it succeeds. Every step is safe to repeat.
type SecretJobRunner struct {
	db     *gorm.DB
	vaults *vault.Vaults
	logger *slog.Logger
}

// NewSecretJobRunner runs secret jobs against db with the deployment's vaults.
func NewSecretJobRunner(db *gorm.DB, vaults *vault.Vaults, logger *slog.Logger) *SecretJobRunner {
	return &SecretJobRunner{db: db, vaults: vaults, logger: logger}
}

// RunOnce claims the jobs due at now and runs them, returning how many it ran and how many failed. err is a failure to
// claim; a failed job is recorded on it and retried later.
func (r *SecretJobRunner) RunOnce(ctx context.Context, now time.Time) (ran, failed int, err error) {
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
		ran++
		if err := r.run(ctx, &job); err != nil {
			failed++
			r.retryLater(ctx, &job, now, err)
		}
	}
	return ran, failed, nil
}

func (r *SecretJobRunner) run(ctx context.Context, job *models.SecretJob) error {
	switch job.Action {
	case models.SecretJobDestroy:
		return r.destroy(ctx, job)
	case models.SecretJobPromote:
		return r.promote(ctx, job)
	default:
		return fmt.Errorf("unknown secret job action %q", job.Action)
	}
}

// destroy removes the job's external value unless a committed row, or a promotion still pending, refers to it: then
// that row owns it and the job is dropped.
func (r *SecretJobRunner) destroy(ctx context.Context, job *models.SecretJob) error {
	if job.Provider == nil || job.ExternalRef == nil {
		return r.finish(ctx, job)
	}
	value := externalValue{provider: *job.Provider, ref: *job.ExternalRef}
	inUse, err := r.inUse(ctx, value)
	if err != nil {
		return err
	}
	if !inUse {
		if err := r.destroyValue(ctx, jobRef(job), value); err != nil {
			return err
		}
	}
	return r.finish(ctx, job)
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

// promote copies the version the job names from the row to the vault that seals new values, then points the row at
// it and queues the removal of the external value it replaces. A version the row no longer holds is not promoted.
func (r *SecretJobRunner) promote(ctx context.Context, job *models.SecretJob) error {
	// A value an earlier attempt wrote but did not record is first reclaimed, or found recorded.
	if job.StagedRef != nil && job.StagedProvider != nil {
		staged := externalValue{provider: *job.StagedProvider, ref: *job.StagedRef}
		inUse, err := r.inUse(ctx, staged)
		if err != nil {
			return err
		}
		if inUse {
			return r.finish(ctx, job)
		}
		if err := r.destroyValue(ctx, jobRef(job), staged); err != nil {
			return err
		}
		if err := r.db.WithContext(ctx).Model(&models.SecretJob{}).Where("id = ?", job.ID).
			Updates(map[string]any{"staged_provider": nil, "staged_ref": nil}).Error; err != nil {
			return fmt.Errorf("failed to clear a reclaimed value from secret job %s: %w", job.ID, err)
		}
		job.StagedProvider, job.StagedRef = nil, nil
	}
	// Superseded: a later write or delete took over what it replaces and queued its own work.
	if job.Version == nil {
		return r.finish(ctx, job)
	}

	var rows []models.Secret
	if err := r.db.WithContext(ctx).Where("id = ?", job.SecretID).Find(&rows).Error; err != nil {
		return err
	}
	replaced := jobExternalValue(job)
	if len(rows) == 0 || rows[0].Version != *job.Version || rows[0].Provider != vault.ProviderLocal {
		// Not superseded, yet its version is gone. Hand what it replaces to a destroy job, which keeps a value in use.
		return r.finishQueueing(ctx, job, replaced)
	}
	row := rows[0]
	writer := r.vaults.Writer()
	if writer.Provider() == vault.ProviderLocal {
		// The deployment moved back to the local vault: the row already has the value.
		return r.finishQueueing(ctx, job, replaced)
	}

	local, err := r.vaults.For(vault.ProviderLocal)
	if err != nil {
		return err
	}
	plaintext, err := local.Open(ctx, secretRef(&row), sealedOf(&row))
	if err != nil {
		return fmt.Errorf("failed to open secret %s for promotion: %w", row.ID, err)
	}
	var current *vault.Sealed
	if replaced != nil && replaced.provider == writer.Provider() {
		current = &vault.Sealed{ExternalRef: replaced.ref}
	}
	sealed, err := writer.Seal(ctx, secretRef(&row), plaintext, current)
	if err != nil {
		return fmt.Errorf("failed to write secret %s to vault %s: %w", row.ID, writer.Provider(), err)
	}
	var staged *externalValue
	if sealed.ExternalRef != "" {
		staged = &externalValue{provider: writer.Provider(), ref: sealed.ExternalRef}
		// Recorded before the row, so a failed or interrupted attempt is reclaimed by the next.
		if err := r.db.WithContext(ctx).Model(&models.SecretJob{}).Where("id = ?", job.ID).
			Updates(map[string]any{"staged_provider": staged.provider, "staged_ref": staged.ref}).Error; err != nil {
			r.reclaim(ctx, job, *staged)
			return fmt.Errorf("failed to record a value written for secret %s: %w", row.ID, err)
		}
	}

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.Secret{}).
			Where("id = ? AND version = ? AND provider = ?", row.ID, row.Version, vault.ProviderLocal).
			Updates(map[string]any{
				"provider": writer.Provider(), "ciphertext": sealed.Ciphertext, "key_id": nilIfEmpty(sealed.KeyID),
				"external_ref": nilIfEmpty(sealed.ExternalRef), "updated_at": dbx.Now(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errPromotionSuperseded
		}
		// A vault that wrote over the replaced value in place leaves nothing else to remove.
		if replaced != nil && (staged == nil || *replaced != *staged) {
			if err := queueDestroy(tx, job.OrgID, job.SecretID, job.OwnerKind, *replaced); err != nil {
				return err
			}
		}
		return tx.Where("id = ?", job.ID).Delete(&models.SecretJob{}).Error
	})
	if errors.Is(err, errPromotionSuperseded) {
		if staged != nil {
			r.reclaim(ctx, job, *staged)
		}
		return r.finish(ctx, job)
	}
	if err != nil {
		return fmt.Errorf("failed to record secret %s in vault %s: %w", row.ID, writer.Provider(), err)
	}
	return nil
}

// reclaim removes a value the job wrote that no row will refer to. When the vault refuses, a destroy job takes it
// over, so it is retried like any other.
func (r *SecretJobRunner) reclaim(ctx context.Context, job *models.SecretJob, staged externalValue) {
	err := r.destroyValue(ctx, jobRef(job), staged)
	if err == nil {
		if clearErr := r.db.WithContext(ctx).Model(&models.SecretJob{}).Where("id = ?", job.ID).
			Updates(map[string]any{"staged_provider": nil, "staged_ref": nil}).Error; clearErr != nil {
			r.logger.Warn("Failed to clear a reclaimed secret value from its job; the next attempt finds it gone",
				"jobID", job.ID, "secretID", job.SecretID, "error", clearErr)
		}
		job.StagedProvider, job.StagedRef = nil, nil
		return
	}
	r.logger.Warn("Failed to reclaim a secret value no row refers to; queued its removal",
		"jobID", job.ID, "secretID", job.SecretID, "provider", staged.provider, "error", err)
	if queueErr := queueDestroy(r.db.WithContext(ctx), job.OrgID, job.SecretID, job.OwnerKind, staged); queueErr != nil {
		r.logger.Error("Failed to queue removal of a secret value no row refers to; the promotion job keeps it",
			"jobID", job.ID, "secretID", job.SecretID, "provider", staged.provider, "error", queueErr)
		return
	}
	if clearErr := r.db.WithContext(ctx).Model(&models.SecretJob{}).Where("id = ?", job.ID).
		Updates(map[string]any{"staged_provider": nil, "staged_ref": nil}).Error; clearErr == nil {
		job.StagedProvider, job.StagedRef = nil, nil
	}
}

// finish deletes a job that is done. A value it still records as staged is left to the next attempt.
func (r *SecretJobRunner) finish(ctx context.Context, job *models.SecretJob) error {
	if job.StagedRef != nil {
		return fmt.Errorf("secret job %s still holds a value it wrote to vault %s", job.ID, deref(job.StagedProvider))
	}
	if err := r.db.WithContext(ctx).Where("id = ?", job.ID).Delete(&models.SecretJob{}).Error; err != nil {
		return fmt.Errorf("failed to delete finished secret job %s: %w", job.ID, err)
	}
	return nil
}

// finishQueueing deletes the job and queues the removal of value in the same transaction.
func (r *SecretJobRunner) finishQueueing(ctx context.Context, job *models.SecretJob, value *externalValue) error {
	if value == nil {
		return r.finish(ctx, job)
	}
	if job.StagedRef != nil {
		return fmt.Errorf("secret job %s still holds a value it wrote to vault %s", job.ID, deref(job.StagedProvider))
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", job.ID).Delete(&models.SecretJob{}).Error; err != nil {
			return err
		}
		return queueDestroy(tx, job.OrgID, job.SecretID, job.OwnerKind, *value)
	})
}

// retryLater releases the job's claim and makes it due again after a backoff that grows with its attempts.
func (r *SecretJobRunner) retryLater(ctx context.Context, job *models.SecretJob, now time.Time, cause error) {
	attempts := job.Attempts + 1
	backoff := secretJobBackoff(attempts)
	message := cause.Error()
	level := slog.LevelWarn
	if attempts >= secretJobAlarmAttempts {
		level = slog.LevelError
	}
	r.logger.Log(ctx, level, "Secret vault job failed; it will be retried",
		"jobID", job.ID, "action", job.Action, "secretID", job.SecretID, "orgID", job.OrgID,
		"attempts", attempts, "retryIn", backoff, "error", cause)
	if err := r.db.WithContext(ctx).Model(&models.SecretJob{}).Where("id = ?", job.ID).Updates(map[string]any{
		"attempts": attempts, "last_error": message, "available_at": now.Add(backoff),
		"claimed_by": nil, "claimed_until": nil,
	}).Error; err != nil {
		r.logger.Error("Failed to schedule a secret vault job's retry; its claim expires instead",
			"jobID", job.ID, "error", err)
	}
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

// PendingSecretJobs counts the secret jobs not yet done, for tests and diagnostics.
func PendingSecretJobs(ctx context.Context, db *gorm.DB, secretID uuid.UUID) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Model(&models.SecretJob{}).Where("secret_id = ?", secretID).Count(&n).Error
	return n, err
}
