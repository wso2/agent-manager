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

// Artifact represents an API Platform artifact (API, LLM Provider, LLM Proxy)
type Artifact struct {
	UUID             uuid.UUID `gorm:"column:uuid;primaryKey" json:"uuid"`
	Handle           string    `gorm:"column:handle" json:"handle"`
	Name             string    `gorm:"column:name" json:"name"`
	Version          string    `gorm:"column:version" json:"version"`
	Kind             string    `gorm:"column:kind" json:"kind"`
	OrganizationName string    `gorm:"column:organization_name" json:"organization_name"`
	InCatalog        bool      `gorm:"column:in_catalog;default:false" json:"inCatalog"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updated_at"`
}

// TableName returns the table name for the Artifact model
func (Artifact) TableName() string {
	return "artifacts"
}

// Artifact Kind constants
const (
	KindRestAPI     = "RestAPI"
	KindWebSubAPI   = "WebSubAPI"
	KindLLMProvider = "LlmProvider"
	KindLLMProxy    = "LlmProxy"
)
