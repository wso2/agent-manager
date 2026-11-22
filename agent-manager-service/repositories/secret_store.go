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
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/vault"
)

// SecretStore persists secret references and, for local or promoting vaults,
// ciphertext. Reference-only vaults store the value before the SQL reference
// is published. Replaced external values are destroyed only after commit.
type SecretStore struct {
	vaults *vault.Vaults
}

// ErrNoSecretStore is a credential to save or resolve through a repository built without a SecretStore.
var ErrNoSecretStore = errors.New("repository has no secret store for upstream credentials")

// NewSecretStore creates a SecretStore over the deployment's vaults.
func NewSecretStore(vaults *vault.Vaults) *SecretStore {
	return &SecretStore{vaults: vaults}
}

// externalValue is a value an external vault keeps: the provider that holds it and its reference there.
type externalValue struct {
	provider string
	ref      string
}

// Write stores value as the secret id, creating the row when absent. An existing row keeps its organization and
// owner. The value is readable in tx at once; a promotion to an external vault follows the commit.
func (s *SecretStore) Write(ctx context.Context, tx *gorm.DB, orgID, ownerKind string, ownerID, id uuid.UUID, value []byte) error {
	if _, active := tx.Statement.ConnPool.(gorm.TxCommitter); !active {
		var err error
		ctx, err = s.PrepareWrite(ctx, tx, orgID, ownerKind, id, value)
		if err != nil {
			return err
		}
	}
	return tx.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.write(ctx, tx, orgID, ownerKind, ownerID, id, value)
	})
}

// writeAttempts bounds how often write retries a row that was deleted between losing the insert and locking it.
const writeAttempts = 3

func (s *SecretStore) write(ctx context.Context, tx *gorm.DB, orgID, ownerKind string, ownerID, id uuid.UUID, value []byte) error {
	orgID = canonicalOrgID(orgID)
	db := tx.WithContext(ctx)
	provider, sealed, err := s.sealWrite(ctx, db, orgID, ownerKind, id, value)
	if err != nil {
		return err
	}
	for range writeAttempts {
		// Insert the row, or find it already there: an insert that treats a duplicate key as "exists" keeps two first writes of one secret from both
		// creating it.
		created, err := s.insert(db, orgID, ownerKind, ownerID, id, provider, sealed)
		if err != nil || created {
			return err
		}
		// The row exists. Lock it before reading it, so concurrent writes of one secret hand its pending external
		// value on in turn.
		err = dbx.LockRow(db, models.Secret{}.TableName(), "version", "id = ?", id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue // deleted since the insert lost; insert again
		}
		if err != nil {
			return fmt.Errorf("failed to lock secret %s: %w", id, err)
		}
		var row models.Secret
		if err := db.Where("id = ?", id).Take(&row).Error; err != nil {
			return fmt.Errorf("failed to read secret %s: %w", id, err)
		}
		return s.update(db, &row, orgID, provider, sealed)
	}
	return fmt.Errorf("failed to write secret %s: it was deleted by %d concurrent writes in turn", id, writeAttempts)
}

// canonicalOrgID spells an organization id the way the database returns it. A value is bound to its row's org id,
// and reads and updates use the id read back from the row: on PostgreSQL that is the lowercase hyphenated form,
// whatever spelling the write was given.
func canonicalOrgID(orgID string) string {
	if id, err := uuid.Parse(orgID); err == nil {
		return id.String()
	}
	return orgID
}

const stagedSecretTTL = 2 * time.Minute

type stagedSecretKey struct {
	org, kind string
	id        uuid.UUID
}
type stagedSecret struct {
	jobID          uuid.UUID
	provider, path string
}

// PrepareWrite stages reference-only credentials before the owning SQL
// transaction starts. The independent cleanup job survives a rollback or a
// process crash. Its cancellation and publication use the same transaction.
// StagesWrites reports whether new values go to an external vault, which must be written before the owning
// transaction (see PrepareWrite).
func (s *SecretStore) StagesWrites() bool {
	_, external := s.vaults.Writer().(vault.ReferenceVault)
	return external
}

func (s *SecretStore) PrepareWrite(ctx context.Context, db *gorm.DB, orgID, kind string, id uuid.UUID, value []byte) (context.Context, error) {
	writer, external := s.vaults.Writer().(vault.ReferenceVault)
	if !external {
		return ctx, nil
	}
	if _, active := db.Statement.ConnPool.(gorm.TxCommitter); active {
		return ctx, errors.New("external secret must be staged before the transaction")
	}
	key := stagedSecretKey{org: canonicalOrgID(orgID), kind: kind, id: id}
	if _, exists := ctx.Value(key).(stagedSecret); exists {
		return ctx, nil
	}
	job := newSecretJob(key.org, id, kind, models.SecretJobAbandon, nil)
	ref := vault.Ref{OperationID: job.ID, OrgID: key.org, SecretID: id, OwnerKind: kind}
	path, err := writer.Reserve(ref)
	if err != nil {
		return ctx, err
	}
	provider := writer.Provider()
	job.Provider, job.ExternalRef = &provider, &path
	job.AvailableAt = job.AvailableAt.Add(stagedSecretTTL)
	if err := db.WithContext(ctx).Create(&job).Error; err != nil {
		return ctx, fmt.Errorf("reserve external secret cleanup: %w", err)
	}
	sealed, err := writer.Seal(ctx, ref, value, nil)
	if err != nil {
		return ctx, fmt.Errorf("stage external secret: %w", err)
	}
	if sealed.ExternalRef != path || len(sealed.Ciphertext) != 0 || sealed.KeyID != "" {
		return ctx, fmt.Errorf("vault %q must return only the reserved reference", provider)
	}
	return context.WithValue(ctx, key, stagedSecret{jobID: job.ID, provider: provider, path: path}), nil
}

func (s *SecretStore) sealWrite(ctx context.Context, db *gorm.DB, orgID, ownerKind string, id uuid.UUID, value []byte) (string, vault.Sealed, error) {
	if _, external := s.vaults.Writer().(vault.ReferenceVault); external {
		staged, ok := ctx.Value(stagedSecretKey{org: orgID, kind: ownerKind, id: id}).(stagedSecret)
		if !ok {
			return "", vault.Sealed{}, errors.New("external secret was not staged before the transaction")
		}
		res := db.Where("id = ? AND action = ? AND claimed_by IS NULL AND available_at > ?", staged.jobID, models.SecretJobAbandon, dbx.Now()).Delete(&models.SecretJob{})
		if res.Error != nil {
			return "", vault.Sealed{}, fmt.Errorf("publish external secret: %w", res.Error)
		}
		if res.RowsAffected != 1 {
			return "", vault.Sealed{}, errors.New("external secret staging expired")
		}
		return staged.provider, vault.Sealed{Ciphertext: nil, KeyID: "", ExternalRef: staged.path}, nil
	}
	local, err := s.vaults.For(vault.ProviderLocal)
	if err != nil {
		return "", vault.Sealed{}, err
	}
	sealed, err := local.Seal(ctx, vault.Ref{OperationID: uuid.Nil, OrgID: orgID, SecretID: id, OwnerKind: ownerKind}, value, nil)
	if err != nil {
		return "", vault.Sealed{}, fmt.Errorf("failed to seal secret: %w", err)
	}
	return local.Provider(), sealed, nil
}

// insert creates the secret's row unless one exists, and reports whether it did.
func (s *SecretStore) insert(db *gorm.DB, orgID, ownerKind string, ownerID, id uuid.UUID, provider string, sealed vault.Sealed) (bool, error) {
	now := dbx.Now()
	row := models.Secret{
		ID: id, OrgID: orgID, OwnerKind: ownerKind, OwnerID: ownerID, Provider: provider,
		Ciphertext: sealed.Ciphertext, KeyID: nilIfEmpty(sealed.KeyID), ExternalRef: nilIfEmpty(sealed.ExternalRef), Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	inserted, err := dbx.InsertIfAbsent(db, &row)
	if err != nil {
		return false, fmt.Errorf("failed to create secret %s: %w", id, err)
	}
	if !inserted {
		return false, nil
	}
	return true, s.afterWrite(db, &row, nil)
}

// update replaces the value of row, which the caller has locked.
func (s *SecretStore) update(db *gorm.DB, row *models.Secret, orgID, provider string, sealed vault.Sealed) error {
	if row.OrgID != orgID {
		return fmt.Errorf("secret %s belongs to another organization", row.ID)
	}
	// The external value this write replaces: the row's own, or the one a promotion still pending would replace.
	replaced, err := supersedePromotions(db, row.ID)
	if err != nil {
		return err
	}
	if row.ExternalRef != nil {
		replaced = rowExternalValue(row)
	}
	res := db.Model(&models.Secret{}).Where("id = ?", row.ID).Updates(map[string]any{
		"provider": provider, "ciphertext": sealed.Ciphertext, "key_id": nilIfEmpty(sealed.KeyID),
		"external_ref": nilIfEmpty(sealed.ExternalRef), "version": gorm.Expr("version + 1"), "updated_at": dbx.Now(),
	})
	if res.Error != nil {
		return fmt.Errorf("failed to update secret %s: %w", row.ID, res.Error)
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("failed to update secret %s: the locked row is gone", row.ID)
	}
	row.Provider = provider
	row.Version++
	return s.afterWrite(db, row, replaced)
}

// afterWrite queues promotion when needed, or releases the previous external reference after commit.
func (s *SecretStore) afterWrite(db *gorm.DB, row *models.Secret, replaced *externalValue) error {
	writer := s.vaults.Writer()
	if row.Provider == vault.ProviderLocal && writer.Provider() != vault.ProviderLocal {
		external, ok := writer.(vault.ExternalVault)
		if !ok {
			return fmt.Errorf("vault %q does not support promotion", writer.Provider())
		}
		version := row.Version
		job := newSecretJob(row.OrgID, row.ID, row.OwnerKind, models.SecretJobPromote, replaced)
		job.Version = &version
		ref := secretRef(row)
		ref.OperationID = job.ID
		target, err := external.PromotionReference(ref)
		if err != nil {
			return fmt.Errorf("failed to reserve promotion reference for secret %s: %w", row.ID, err)
		}
		provider := writer.Provider()
		if target == "" || (replaced != nil && replaced.provider == provider && replaced.ref == target) {
			return fmt.Errorf("vault %q must reserve a new promotion reference", provider)
		}
		job.StagedProvider = &provider
		job.StagedRef = &target
		if err := db.Create(&job).Error; err != nil {
			return fmt.Errorf("failed to queue promotion of secret %s: %w", row.ID, err)
		}
		return nil
	}
	if replaced == nil {
		return nil
	}
	return queueDestroy(db, row.OrgID, row.ID, row.OwnerKind, *replaced)
}

// Read returns the secret's value. found is false when there is no such row.
func (s *SecretStore) Read(ctx context.Context, tx *gorm.DB, id uuid.UUID) (value []byte, found bool, err error) {
	return s.read(ctx, tx, id, false)
}

// read is Read. With asStored, a row written before the vault existed is returned as it is rather than opened: its
// owner stored the form it uses.
func (s *SecretStore) read(ctx context.Context, tx *gorm.DB, id uuid.UUID, asStored bool) ([]byte, bool, error) {
	row, found, err := loadSecretRow(ctx, tx, id)
	if err != nil || !found {
		return nil, false, err
	}
	if asStored && row.KeyID == nil && row.ExternalRef == nil {
		return row.Ciphertext, true, nil
	}
	for attempt := 1; ; attempt++ {
		value, openErr := s.open(ctx, row)
		if openErr == nil {
			return value, true, nil
		}
		if row.ExternalRef == nil || attempt >= readAttempts {
			return nil, false, openErr
		}
		// A rotation may have published a new external value and destroyed the one this read captured. Reload, and
		// open again only when the stored value changed; otherwise the vault's error stands.
		current, found, err := loadSecretRow(ctx, tx, id)
		if err != nil {
			return nil, false, err
		}
		if !found {
			return nil, false, nil
		}
		if !storedValueChanged(row, current) {
			return nil, false, openErr
		}
		row = current
	}
}

// readAttempts bounds how often read reopens a secret whose external value was replaced while it was being read.
const readAttempts = 3

func loadSecretRow(ctx context.Context, tx *gorm.DB, id uuid.UUID) (*models.Secret, bool, error) {
	var rows []models.Secret
	if err := tx.WithContext(ctx).Where("id = ?", id).Find(&rows).Error; err != nil {
		return nil, false, fmt.Errorf("failed to read secret %s: %w", id, err)
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	return &rows[0], true, nil
}

// storedValueChanged reports whether b holds a different stored value than a.
func storedValueChanged(a, b *models.Secret) bool {
	return a.Version != b.Version || a.Provider != b.Provider ||
		!equalStringPtr(a.ExternalRef, b.ExternalRef) || !bytes.Equal(a.Ciphertext, b.Ciphertext)
}

func equalStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (s *SecretStore) open(ctx context.Context, row *models.Secret) ([]byte, error) {
	v, err := s.vaults.For(row.Provider)
	if err != nil {
		return nil, fmt.Errorf("secret %s: %w", row.ID, err)
	}
	value, err := v.Open(ctx, secretRef(row), sealedOf(row))
	if err != nil {
		if row.Provider == vault.ProviderLocal {
			// The local vault does no I/O: a value it cannot open now will never open with this key ring.
			return nil, fmt.Errorf("failed to open secret %s: %w: %w", row.ID, ErrSecretUnreadable, err)
		}
		return nil, fmt.Errorf("failed to open secret %s: %w", row.ID, err)
	}
	return value, nil
}

// ErrSecretUnreadable is a stored value the configured keys cannot open. Retrying does not help.
var ErrSecretUnreadable = errors.New("secret cannot be opened with the configured keys")

// UpstreamCredential snapshots a deployment credential without accessing its vault.
type UpstreamCredential struct {
	store *SecretStore
	row   models.Secret
}

func (s *SecretStore) SnapshotUpstreamCredential(ctx context.Context, tx *gorm.DB, id *uuid.UUID) (*UpstreamCredential, error) {
	if id == nil {
		return nil, nil //nolint:nilnil // no referenced credential
	}
	if s == nil {
		return nil, ErrNoSecretStore
	}
	snapshot := &UpstreamCredential{store: s, row: models.Secret{}}
	err := tx.WithContext(ctx).Where("id = ?", *id).Take(&snapshot.row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("upstream credential %s is missing", *id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read upstream credential %s: %w", *id, err)
	}
	return snapshot, nil
}

// SnapshotSecret is SnapshotUpstreamCredential for an owner whose missing secret reads as none: nil, nil.
func (s *SecretStore) SnapshotSecret(ctx context.Context, tx *gorm.DB, id *uuid.UUID) (*UpstreamCredential, error) {
	if id == nil {
		return nil, nil //nolint:nilnil // no referenced secret
	}
	if s == nil {
		return nil, ErrNoSecretStore
	}
	var rows []models.Secret
	if err := tx.WithContext(ctx).Where("id = ?", *id).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to read secret %s: %w", *id, err)
	}
	if len(rows) == 0 {
		return nil, nil //nolint:nilnil // a missing row is no secret
	}
	return &UpstreamCredential{store: s, row: rows[0]}, nil
}

func (c *UpstreamCredential) External() bool {
	return c != nil && c.row.Provider != vault.ProviderLocal
}

func (c *UpstreamCredential) SameVersion(other *UpstreamCredential) bool {
	return c != nil && other != nil && c.row.ID == other.row.ID && c.row.Version == other.row.Version
}

func (c *UpstreamCredential) Open(ctx context.Context) (string, error) {
	value, err := c.store.open(ctx, &c.row)
	return string(value), err
}

// OpenAsStored is Open for an owner that hands the store a form it encrypted itself: a row written before the vault
// existed is returned as it is. It opens the snapshot's own row, so a locally sealed snapshot never reaches an
// external vault even if the row has been promoted since.
func (c *UpstreamCredential) OpenAsStored(ctx context.Context) (string, error) {
	if c.row.KeyID == nil && c.row.ExternalRef == nil {
		return string(c.row.Ciphertext), nil
	}
	return c.Open(ctx)
}

// LockVersion keeps the rendered credential current until the deployment commits.
func (c *UpstreamCredential) LockVersion(ctx context.Context, tx *gorm.DB) error {
	return dbx.LockRow(tx.WithContext(ctx), models.Secret{}.TableName(), "version", "id = ? AND version = ?", c.row.ID, c.row.Version)
}

// Delete removes the secret. An external value it had is destroyed after tx commits. A missing row is a no-op.
func (s *SecretStore) Delete(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
	return tx.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return ReleaseSecret(tx, &id)
	})
}

// Put stores value for the owner and returns the secret's id: it rewrites the secret current names, or creates one.
// An empty value returns nil; the caller deletes current once nothing refers to it.
func (s *SecretStore) Put(tx *gorm.DB, orgID, ownerKind string, ownerID uuid.UUID, current *uuid.UUID, value string) (*uuid.UUID, error) {
	if value == "" {
		return nil, nil //nolint:nilnil // no value means no secret
	}
	id := NewID()
	if current != nil {
		id = *current
	}
	if err := s.Write(txContext(tx), tx, orgID, ownerKind, ownerID, id, []byte(value)); err != nil {
		return nil, err
	}
	return &id, nil
}

// Get reads a secret's value; a nil id or a missing row is empty.
func (s *SecretStore) Get(tx *gorm.DB, id *uuid.UUID) (string, error) {
	if id == nil {
		return "", nil
	}
	value, _, err := s.read(txContext(tx), tx, *id, false)
	return string(value), err
}

// GetAsStored is Get for an owner that hands the store a form it encrypted itself, so its rows from before the
// vault already hold what it reads back.
func (s *SecretStore) GetAsStored(tx *gorm.DB, id *uuid.UUID) (string, error) {
	if id == nil {
		return "", nil
	}
	value, _, err := s.read(txContext(tx), tx, *id, true)
	return string(value), err
}

// AdoptSecret records ownerID as the owner of a secret a replacement row took over.
func AdoptSecret(tx *gorm.DB, id, ownerID uuid.UUID) error {
	if err := tx.Model(&models.Secret{}).Where("id = ?", id).
		Updates(map[string]any{"owner_id": ownerID, "updated_at": dbx.Now()}).Error; err != nil {
		return fmt.Errorf("failed to hand secret %s to its new owner: %w", id, err)
	}
	return nil
}

// ReleaseSecret deletes a secret's row in tx and queues the removal of any external value it has, which runs after
// tx commits. It needs no vault, so a delete path without a SecretStore can call it. nil and a missing row are no-ops.
// Delete the owning row, or clear its reference, first: owners refer to secrets by foreign key.
func ReleaseSecret(tx *gorm.DB, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	locked, err := dbx.LockRows(tx, models.Secret{}.TableName(), "version", "id = ?", *id)
	if err != nil {
		return fmt.Errorf("failed to lock secret %s for release: %w", *id, err)
	}
	if locked == 0 {
		// Nothing to release. Reading again could see a row another transaction created after the lock matched
		// nothing, and delete it without holding its lock.
		return nil
	}
	var rows []models.Secret
	if err := tx.Where("id = ?", *id).Find(&rows).Error; err != nil {
		return fmt.Errorf("failed to read secret %s for release: %w", *id, err)
	}
	if len(rows) == 0 {
		return nil
	}
	row := rows[0]
	replaced, err := supersedePromotions(tx, row.ID)
	if err != nil {
		return err
	}
	if err := tx.Where("id = ?", row.ID).Delete(&models.Secret{}).Error; err != nil {
		return fmt.Errorf("failed to delete secret %s: %w", row.ID, err)
	}
	for _, value := range []*externalValue{replaced, rowExternalValue(&row)} {
		if value == nil {
			continue
		}
		if err := queueDestroy(tx, row.OrgID, row.ID, row.OwnerKind, *value); err != nil {
			return err
		}
	}
	return nil
}

// supersedePromotions takes over the secret's pending promotions, returning the external value the latest of them
// would have replaced. The jobs stay queued without it, so the worker still reclaims a value one of them may already
// have written to the vault.
func supersedePromotions(tx *gorm.DB, id uuid.UUID) (*externalValue, error) {
	var pending []models.SecretJob
	if err := tx.Where("secret_id = ? AND action = ? AND version IS NOT NULL", id, models.SecretJobPromote).
		Order("created_at DESC").Find(&pending).Error; err != nil {
		return nil, fmt.Errorf("failed to read pending promotions of secret %s: %w", id, err)
	}
	if len(pending) == 0 {
		return nil, nil //nolint:nilnil // no pending promotion replaces anything
	}
	if err := tx.Model(&models.SecretJob{}).Where("secret_id = ? AND action = ?", id, models.SecretJobPromote).
		Updates(map[string]any{"provider": nil, "external_ref": nil, "version": nil}).Error; err != nil {
		return nil, fmt.Errorf("failed to supersede pending promotions of secret %s: %w", id, err)
	}
	// Only the newest can carry one: each write hands the replaced value on to the job it queues.
	for _, job := range pending {
		if job.Provider != nil && job.ExternalRef != nil {
			return &externalValue{provider: *job.Provider, ref: *job.ExternalRef}, nil
		}
	}
	return nil, nil //nolint:nilnil // no pending promotion replaces an external value
}

func queueDestroy(tx *gorm.DB, orgID string, secretID uuid.UUID, ownerKind string, value externalValue) error {
	job := newSecretJob(orgID, secretID, ownerKind, models.SecretJobDestroy, &value)
	if err := tx.Create(&job).Error; err != nil {
		return fmt.Errorf("failed to queue removal of a secret's external value: %w", err)
	}
	return nil
}

func newSecretJob(orgID string, secretID uuid.UUID, ownerKind, action string, value *externalValue) models.SecretJob {
	now := dbx.Now()
	job := models.SecretJob{
		ID: NewID(), OrgID: orgID, SecretID: secretID, OwnerKind: ownerKind, Action: action,
		Provider: nil, ExternalRef: nil, StagedProvider: nil, StagedRef: nil, Version: nil,
		Attempts: 0, LastError: nil, AvailableAt: now, ClaimedBy: nil, ClaimedUntil: nil, CreatedAt: now,
	}
	if value != nil {
		provider, ref := value.provider, value.ref
		job.Provider, job.ExternalRef = &provider, &ref
	}
	return job
}

func rowExternalValue(row *models.Secret) *externalValue {
	if row.ExternalRef == nil {
		return nil
	}
	return &externalValue{provider: row.Provider, ref: *row.ExternalRef}
}

func txContext(tx *gorm.DB) context.Context {
	if tx.Statement != nil && tx.Statement.Context != nil {
		return tx.Statement.Context
	}
	return context.Background()
}

func secretRef(row *models.Secret) vault.Ref {
	return vault.Ref{OperationID: uuid.Nil, OrgID: row.OrgID, SecretID: row.ID, OwnerKind: row.OwnerKind}
}

func sealedOf(row *models.Secret) vault.Sealed {
	sealed := vault.Sealed{Ciphertext: row.Ciphertext, KeyID: "", ExternalRef: ""}
	if row.KeyID != nil {
		sealed.KeyID = *row.KeyID
	}
	if row.ExternalRef != nil {
		sealed.ExternalRef = *row.ExternalRef
	}
	return sealed
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
