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
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// moduleSecretNamespace derives a module secret's row id from its organization, owner kind and owner, so two first
// writes of one secret meet on the primary key instead of creating two rows.
var moduleSecretNamespace = uuid.MustParse("0d7e3a52-8f0b-5c41-a3c9-6d6f64736563")

// ModuleSecretStore keeps module secrets in the secrets table, one row per organization, owner kind and owner id. The
// module host decides the owner kind and id, so a module only reaches its own. Values go through the SecretStore, so
// they are sealed by the configured vault and promoted to an external one after the change needing them commits, the
// same as core secrets.
type ModuleSecretStore struct {
	db      *gorm.DB
	secrets *SecretStore
}

// NewModuleSecretStore returns the secret store for modules.
func NewModuleSecretStore(db *gorm.DB, secrets *SecretStore) *ModuleSecretStore {
	return &ModuleSecretStore{db: db, secrets: secrets}
}

func (s *ModuleSecretStore) conn(ctx context.Context, tx *gorm.DB) *gorm.DB {
	if tx == nil {
		tx = s.db
	}
	return tx.WithContext(ctx)
}

func ownerCond(orgID, kind string, owner uuid.UUID) (string, []any) {
	return "org_id = ? AND owner_kind = ? AND owner_id = ?", []any{orgID, kind, owner}
}

// ownerRows lists the ids of the owner's secret rows. There is one, except for rows left by concurrent first writes
// before ids were derived from the owner.
func (s *ModuleSecretStore) ownerRows(db *gorm.DB, orgID, kind string, owner uuid.UUID) ([]uuid.UUID, error) {
	where, args := ownerCond(orgID, kind, owner)
	var ids []uuid.UUID
	if err := db.Model(&models.Secret{}).Where(where, args...).Order("created_at, id").Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("failed to find %s secret %s: %w", kind, owner, err)
	}
	return ids, nil
}

// Put stores value, rewriting the owner's secret or creating it.
func (s *ModuleSecretStore) Put(ctx context.Context, tx *gorm.DB, orgID, kind string, owner uuid.UUID, value string) error {
	org, err := uuid.Parse(orgID)
	if err != nil {
		return err
	}
	return s.conn(ctx, tx).Transaction(func(db *gorm.DB) error {
		ids, err := s.ownerRows(db, org.String(), kind, owner)
		if err != nil {
			return err
		}
		id := uuid.NewSHA1(moduleSecretNamespace, []byte(org.String()+"/"+kind+"/"+owner.String()))
		if len(ids) > 0 {
			id = ids[0]
		}
		return s.secrets.write(ctx, db, org.String(), kind, owner, id, []byte(value))
	})
}

// Get reads the owner's secret; ok is false when there is none.
func (s *ModuleSecretStore) Get(ctx context.Context, tx *gorm.DB, orgID, kind string, owner uuid.UUID) (string, bool, error) {
	db := s.conn(ctx, tx)
	ids, err := s.ownerRows(db, orgID, kind, owner)
	if err != nil || len(ids) == 0 {
		return "", false, err
	}
	value, found, err := s.secrets.read(ctx, db, ids[0], false)
	if err != nil || !found {
		return "", false, err
	}
	return string(value), true, nil
}

// Delete removes the owner's secret, if any. An external value it had is destroyed after tx commits.
func (s *ModuleSecretStore) Delete(ctx context.Context, tx *gorm.DB, orgID, kind string, owner uuid.UUID) error {
	return s.conn(ctx, tx).Transaction(func(db *gorm.DB) error {
		ids, err := s.ownerRows(db, orgID, kind, owner)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := ReleaseSecret(db, &id); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteKind removes every secret of an owner kind, in every organization: a module purge. External values are
// destroyed after tx commits.
func (s *ModuleSecretStore) DeleteKind(ctx context.Context, tx *gorm.DB, kind string) error {
	return s.conn(ctx, tx).Transaction(func(db *gorm.DB) error {
		var ids []uuid.UUID
		if err := db.Model(&models.Secret{}).Where("owner_kind = ?", kind).Pluck("id", &ids).Error; err != nil {
			return fmt.Errorf("failed to list %s secrets: %w", kind, err)
		}
		for _, id := range ids {
			if err := ReleaseSecret(db, &id); err != nil {
				return err
			}
		}
		return nil
	})
}
