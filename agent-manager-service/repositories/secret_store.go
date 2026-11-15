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

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/vault"
)

// SecretStore keeps secret values in the secrets table. Owning rows hold a
// secret's id; the value goes through the vault, which either encrypts it into
// the row or stores it elsewhere and leaves a reference. Callers hand over and
// get back plaintext.
type SecretStore struct {
	vaults *vault.Vaults
}

// NewSecretStore creates a SecretStore over the deployment's vaults.
func NewSecretStore(vaults *vault.Vaults) *SecretStore {
	return &SecretStore{vaults: vaults}
}

// Write stores value as the secret id, creating the row when absent. An
// existing row keeps its organization and owner.
func (s *SecretStore) Write(ctx context.Context, tx *gorm.DB, orgID, ownerKind string, ownerID, id uuid.UUID, value []byte) error {
	var rows []models.Secret
	if err := tx.WithContext(ctx).Where("id = ?", id).Find(&rows).Error; err != nil {
		return err
	}
	writer := s.vaults.Writer()
	now := dbx.Now()
	if len(rows) == 0 {
		sealed, err := writer.Seal(ctx, vault.Ref{OrgID: orgID, SecretID: id, OwnerKind: ownerKind}, value, nil)
		if err != nil {
			return fmt.Errorf("failed to seal secret: %w", err)
		}
		row := models.Secret{
			ID: id, OrgID: orgID, OwnerKind: ownerKind, OwnerID: ownerID, Provider: writer.Provider(),
			Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		setSealed(&row, sealed)
		return tx.WithContext(ctx).Create(&row).Error
	}

	row := rows[0]
	if row.OrgID != orgID {
		return fmt.Errorf("secret %s belongs to another organization", id)
	}
	ref := secretRef(&row)
	var current *vault.Sealed
	if row.Provider == writer.Provider() {
		held := sealedOf(&row)
		current = &held
	}
	sealed, err := writer.Seal(ctx, ref, value, current)
	if err != nil {
		return fmt.Errorf("failed to seal secret: %w", err)
	}
	if err := tx.WithContext(ctx).Model(&models.Secret{}).Where("id = ?", id).Updates(map[string]any{
		"provider": writer.Provider(), "ciphertext": sealed.Ciphertext, "key_id": nilIfEmpty(sealed.KeyID),
		"external_ref": nilIfEmpty(sealed.ExternalRef), "version": gorm.Expr("version + 1"), "updated_at": now,
	}).Error; err != nil {
		return err
	}
	// A value that moved to another vault leaves nothing behind in the old one.
	// This runs after the row is rewritten so a failure never loses the value.
	if current == nil {
		return s.destroy(ctx, &row)
	}
	return nil
}

// Read returns the secret's value. found is false when there is no such row.
func (s *SecretStore) Read(ctx context.Context, tx *gorm.DB, id uuid.UUID) (value []byte, found bool, err error) {
	return s.read(ctx, tx, id, false)
}

// read is Read. With asStored, a row written before the vault existed is
// returned as it is rather than opened: its owner stored the form it uses.
func (s *SecretStore) read(ctx context.Context, tx *gorm.DB, id uuid.UUID, asStored bool) ([]byte, bool, error) {
	var rows []models.Secret
	if err := tx.WithContext(ctx).Where("id = ?", id).Find(&rows).Error; err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	row := rows[0]
	if asStored && row.KeyID == nil && row.ExternalRef == nil {
		return row.Ciphertext, true, nil
	}
	v, err := s.vaults.For(row.Provider)
	if err != nil {
		return nil, false, fmt.Errorf("secret %s: %w", id, err)
	}
	value, err := v.Open(ctx, secretRef(&row), sealedOf(&row))
	if err != nil {
		return nil, false, fmt.Errorf("failed to open secret %s: %w", id, err)
	}
	return value, true, nil
}

// Delete removes the secret and anything its vault keeps for it. A missing row is a no-op.
func (s *SecretStore) Delete(ctx context.Context, tx *gorm.DB, id uuid.UUID) error {
	var rows []models.Secret
	if err := tx.WithContext(ctx).Where("id = ?", id).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	if err := tx.WithContext(ctx).Where("id = ?", id).Delete(&models.Secret{}).Error; err != nil {
		return err
	}
	// After the row, so a failure here rolls the delete back with the value intact.
	return s.destroy(ctx, &rows[0])
}

func (s *SecretStore) destroy(ctx context.Context, row *models.Secret) error {
	v, err := s.vaults.For(row.Provider)
	if err != nil {
		return fmt.Errorf("secret %s: %w", row.ID, err)
	}
	if err := v.Destroy(ctx, secretRef(row), sealedOf(row)); err != nil {
		return fmt.Errorf("failed to remove secret %s from its vault: %w", row.ID, err)
	}
	return nil
}

// put stores value for the owner and returns the secret's id: it rewrites the
// secret current names, or creates one. An empty value returns nil; the caller
// deletes current once nothing refers to it.
func (s *SecretStore) put(tx *gorm.DB, orgID, ownerKind string, ownerID uuid.UUID, current *uuid.UUID, value string) (*uuid.UUID, error) {
	if value == "" {
		//nolint:nilnil // no value means no secret: the caller clears its reference
		return nil, nil
	}
	id := newID()
	if current != nil {
		id = *current
	}
	if err := s.Write(txContext(tx), tx, orgID, ownerKind, ownerID, id, []byte(value)); err != nil {
		return nil, err
	}
	return &id, nil
}

// get reads a secret's value; a nil id or a missing row is empty.
func (s *SecretStore) get(tx *gorm.DB, id *uuid.UUID) (string, error) {
	if id == nil {
		return "", nil
	}
	value, _, err := s.read(txContext(tx), tx, *id, false)
	return string(value), err
}

// getAsStored is get for an owner that hands the store a form it encrypted
// itself, so its rows from before the vault already hold what it reads back.
func (s *SecretStore) getAsStored(tx *gorm.DB, id *uuid.UUID) (string, error) {
	if id == nil {
		return "", nil
	}
	value, _, err := s.read(txContext(tx), tx, *id, true)
	return string(value), err
}

// delete removes a secret; nil is a no-op.
func (s *SecretStore) delete(tx *gorm.DB, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	return s.Delete(txContext(tx), tx, *id)
}

// errSecretHeldExternally is returned by deleteRowSecret for a value an external vault holds.
var errSecretHeldExternally = errors.New("secret is held by an external vault and must be deleted through the secret store")

// deleteRowSecret removes a secret whose value lives in its row, for delete
// paths that have no SecretStore. It refuses a secret an external vault holds
// rather than orphan the value there.
func deleteRowSecret(tx *gorm.DB, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	res := tx.Where("id = ? AND external_ref IS NULL", *id).Delete(&models.Secret{})
	if res.Error != nil || res.RowsAffected > 0 {
		return res.Error
	}
	var remaining int64
	if err := tx.Model(&models.Secret{}).Where("id = ?", *id).Count(&remaining).Error; err != nil {
		return err
	}
	if remaining > 0 {
		return fmt.Errorf("secret %s: %w", *id, errSecretHeldExternally)
	}
	return nil
}

func txContext(tx *gorm.DB) context.Context {
	if tx.Statement != nil && tx.Statement.Context != nil {
		return tx.Statement.Context
	}
	return context.Background()
}

func secretRef(row *models.Secret) vault.Ref {
	return vault.Ref{OrgID: row.OrgID, SecretID: row.ID, OwnerKind: row.OwnerKind}
}

func sealedOf(row *models.Secret) vault.Sealed {
	sealed := vault.Sealed{Ciphertext: row.Ciphertext}
	if row.KeyID != nil {
		sealed.KeyID = *row.KeyID
	}
	if row.ExternalRef != nil {
		sealed.ExternalRef = *row.ExternalRef
	}
	return sealed
}

func setSealed(row *models.Secret, sealed vault.Sealed) {
	row.Ciphertext = sealed.Ciphertext
	row.KeyID = nilIfEmpty(sealed.KeyID)
	row.ExternalRef = nilIfEmpty(sealed.ExternalRef)
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
