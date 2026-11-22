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

// Secret is a credential another row refers to by id, so the owning row never holds the value. Provider names
// the vault that protects it: Ciphertext and KeyID are set when that vault encrypts into the row, ExternalRef
// when it keeps the value elsewhere.
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

// Secret job actions.
const (
	// SecretJobPromote copies a value sealed into the row to the external vault that seals new values.
	SecretJobPromote = "promote"
	// SecretJobDestroy removes a value from an external vault once no row refers to it.
	SecretJobDestroy = "destroy"
)

// SecretJob is external-vault work that runs only after the transaction that queued it commits. A promote job
// names the secret's Version it promotes, and in Provider and ExternalRef the external value that version replaces,
// which is destroyed once the promoted value is in the row. StagedProvider and StagedRef reserve the target when queued,
// so any worker can resume the same promotion or clean up an interrupted write. A destroy job names the value in Provider and
// ExternalRef.
type SecretJob struct {
	ID             uuid.UUID  `gorm:"column:id;primaryKey"`
	OrgID          string     `gorm:"column:org_id"`
	SecretID       uuid.UUID  `gorm:"column:secret_id"`
	OwnerKind      string     `gorm:"column:owner_kind"`
	Action         string     `gorm:"column:action"`
	Provider       *string    `gorm:"column:provider"`
	ExternalRef    *string    `gorm:"column:external_ref"`
	StagedProvider *string    `gorm:"column:staged_provider"`
	StagedRef      *string    `gorm:"column:staged_ref"`
	Version        *int       `gorm:"column:version"`
	Attempts       int        `gorm:"column:attempts"`
	LastError      *string    `gorm:"column:last_error"`
	AvailableAt    time.Time  `gorm:"column:available_at"`
	ClaimedBy      *string    `gorm:"column:claimed_by"`
	ClaimedUntil   *time.Time `gorm:"column:claimed_until"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
}

func (SecretJob) TableName() string { return "secret_jobs" }
