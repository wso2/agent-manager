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

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// ModuleSecretStore keeps module secrets in the secrets table, one row per
// organization, owner kind and owner id, encrypted with the core's key. The
// module host decides the owner kind and id, so a module only reaches its own.
type ModuleSecretStore struct {
	db  *gorm.DB
	key []byte
}

// NewModuleSecretStore returns the secret store for modules.
func NewModuleSecretStore(db *gorm.DB, key []byte) *ModuleSecretStore {
	return &ModuleSecretStore{db: db, key: key}
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

// Put stores value, rewriting the owner's secret or creating it.
func (s *ModuleSecretStore) Put(ctx context.Context, tx *gorm.DB, orgID, kind string, owner uuid.UUID, value string) error {
	ciphertext, err := utils.EncryptBytes([]byte(value), s.key)
	if err != nil {
		return err
	}
	db := s.conn(ctx, tx)
	where, args := ownerCond(orgID, kind, owner)
	now := dbx.Now()
	res := db.Model(&models.Secret{}).Where(where, args...).
		Updates(map[string]any{"ciphertext": ciphertext, "version": gorm.Expr("version + 1"), "updated_at": now})
	if res.Error != nil || res.RowsAffected > 0 {
		return res.Error
	}
	org, err := uuid.Parse(orgID)
	if err != nil {
		return err
	}
	return db.Create(&models.Secret{
		ID: NewID(), OrgID: org.String(), OwnerKind: kind, OwnerID: owner, Provider: models.SecretProviderLocal,
		Ciphertext: ciphertext, Version: 1, CreatedAt: now, UpdatedAt: now,
	}).Error
}

// Get reads the owner's secret; ok is false when there is none.
func (s *ModuleSecretStore) Get(ctx context.Context, tx *gorm.DB, orgID, kind string, owner uuid.UUID) (string, bool, error) {
	where, args := ownerCond(orgID, kind, owner)
	var rows []models.Secret
	if err := s.conn(ctx, tx).Where(where, args...).Limit(1).Find(&rows).Error; err != nil {
		return "", false, err
	}
	if len(rows) == 0 {
		return "", false, nil
	}
	plain, err := utils.DecryptBytes(rows[0].Ciphertext, s.key)
	if err != nil {
		return "", false, err
	}
	return string(plain), true, nil
}

// Delete removes the owner's secret, if any.
func (s *ModuleSecretStore) Delete(ctx context.Context, tx *gorm.DB, orgID, kind string, owner uuid.UUID) error {
	where, args := ownerCond(orgID, kind, owner)
	return s.conn(ctx, tx).Where(where, args...).Delete(&models.Secret{}).Error
}
