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

package models

import (
	"time"

	"github.com/google/uuid"
)

// SecretProviderLocal secrets hold their value encrypted in the row.
const SecretProviderLocal = "local"

// Secret is a credential another row refers to by id, so the owning row never holds the value.
type Secret struct {
	ID          uuid.UUID `gorm:"column:id;primaryKey"`
	OrgID       string    `gorm:"column:org_id"`
	OwnerKind   string    `gorm:"column:owner_kind"`
	OwnerID     uuid.UUID `gorm:"column:owner_id"`
	Provider    string    `gorm:"column:provider"`
	Ciphertext  []byte    `gorm:"column:ciphertext"`
	KeyID       *string   `gorm:"column:key_id"`
	ExternalRef *string   `gorm:"column:external_ref"`
	Version     int       `gorm:"column:version"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (Secret) TableName() string { return "secrets" }
