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

// EnvThunderURL records an environment's externally-reachable env-Thunder
// origin, keyed by (OUID, EnvName). ThunderURL is always set and is the
// authoritative value every reader uses. ThunderHandle is set only for the
// handle path (AMS computes ThunderURL from it); nil for a row that supplies
// ThunderURL directly. ThunderHandle is *string, not string, so an omitted
// handle writes SQL NULL rather than "" — uq_env_thunder_urls_handle relies
// on Postgres never treating multiple NULLs as conflicting to let any number
// of handle-less rows coexist.
type EnvThunderURL struct {
	ID            uuid.UUID `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID          string    `gorm:"column:ou_id;not null;uniqueIndex:uq_env_thunder_urls_ou_env"`
	EnvName       string    `gorm:"column:env_name;not null;uniqueIndex:uq_env_thunder_urls_ou_env"`
	ThunderHandle *string   `gorm:"column:thunder_handle;uniqueIndex:uq_env_thunder_urls_handle"`
	ThunderURL    string    `gorm:"column:thunder_url;not null;uniqueIndex:uq_env_thunder_urls_url"`
	CreatedAt     time.Time `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null;default:NOW()"`
}

// TableName returns the table name for the EnvThunderURL model.
func (EnvThunderURL) TableName() string { return "env_thunder_urls" }

// ThunderURLRecord is the public-facing result of resolving an environment's
// env-Thunder registration. URL is always the authoritative origin; Handle is
// set only for an on-prem row. A zero value means not provisioned.
type ThunderURLRecord struct {
	Handle string
	URL    string
}
