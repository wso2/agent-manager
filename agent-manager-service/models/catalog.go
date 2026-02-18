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

// CatalogEntry represents a resource in the catalog
// This model maps to the artifacts table with in_catalog filter
type CatalogEntry struct {
	UUID             uuid.UUID `gorm:"column:uuid;primaryKey" json:"uuid"`
	Handle           string    `gorm:"column:handle;not null" json:"handle"`
	Name             string    `gorm:"column:name;not null" json:"name"`
	Version          string    `gorm:"column:version;not null" json:"version"`
	Kind             string    `gorm:"column:kind;not null" json:"kind"`
	InCatalog        bool      `gorm:"column:in_catalog" json:"inCatalog"`
	OrganizationUUID uuid.UUID `gorm:"column:organization_uuid;not null" json:"-"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"createdAt"`
}

// TableName returns the table name for catalog queries
func (CatalogEntry) TableName() string {
	return "artifacts"
}

// Catalog resource kind constants
const (
	CatalogKindLLMProvider = "llmProvider"
	CatalogKindAgent       = "agent"
	CatalogKindMCP         = "mcp"
)
