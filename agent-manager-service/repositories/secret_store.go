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
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// Owning rows keep a secret's id; the value, already encrypted by the caller, lives in the secrets table.

// putSecret stores value for the owner and returns the secret's id: it rewrites the secret current names, or creates
// one. An empty value returns nil; the caller deletes current once nothing refers to it.
func putSecret(tx *gorm.DB, orgID, ownerKind string, ownerID uuid.UUID, current *uuid.UUID, value string) (*uuid.UUID, error) {
	if value == "" {
		return nil, nil
	}
	now := dbx.Now()
	if current != nil {
		res := tx.Model(&models.Secret{}).Where("id = ?", *current).
			Updates(map[string]any{"ciphertext": []byte(value), "version": gorm.Expr("version + 1"), "updated_at": now})
		if res.Error != nil {
			return nil, res.Error
		}
		if res.RowsAffected > 0 {
			return current, nil
		}
	}
	secret := models.Secret{
		ID: newID(), OrgID: orgID, OwnerKind: ownerKind, OwnerID: ownerID, Provider: models.SecretProviderLocal,
		Ciphertext: []byte(value), Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&secret).Error; err != nil {
		return nil, err
	}
	return &secret.ID, nil
}

// getSecret reads a secret's value; a nil id or a missing row is empty.
func getSecret(tx *gorm.DB, id *uuid.UUID) (string, error) {
	if id == nil {
		return "", nil
	}
	var secrets []models.Secret
	if err := tx.Where("id = ?", *id).Find(&secrets).Error; err != nil {
		return "", err
	}
	if len(secrets) == 0 {
		return "", nil
	}
	return string(secrets[0].Ciphertext), nil
}

// deleteSecret removes a secret; nil is a no-op.
func deleteSecret(tx *gorm.DB, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	return tx.Where("id = ?", *id).Delete(&models.Secret{}).Error
}
