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
	"github.com/google/uuid"
)

// RestAPI represents a REST API entity in the platform
type RestAPI struct {
	UUID            uuid.UUID     `gorm:"column:uuid;primaryKey" json:"id"`
	Description     string        `gorm:"column:description" json:"description,omitempty"`
	CreatedBy       string        `gorm:"column:created_by" json:"createdBy,omitempty"`
	ProjectUUID     uuid.UUID     `gorm:"column:project_uuid" json:"projectId"`
	LifecycleStatus string        `gorm:"column:lifecycle_status" json:"lifeCycleStatus,omitempty"`
	Transport       string        `gorm:"column:transport" json:"transport,omitempty"` // JSON array as TEXT
	Configuration   RestAPIConfig `gorm:"column:configuration;type:jsonb;serializer:json" json:"configuration"`

	// Relations - populated via joins
	Artifact *Artifact `gorm:"foreignKey:UUID;references:UUID" json:"artifact,omitempty"`
}

// TableName returns the table name for the RestAPI model
func (RestAPI) TableName() string {
	return "rest_apis"
}

// RestAPIConfig represents the REST API configuration
type RestAPIConfig struct {
	Name       string         `json:"name,omitempty"`
	Version    string         `json:"version,omitempty"`
	Context    *string        `json:"context,omitempty"`
	Vhost      *string        `json:"vhost,omitempty"`
	Upstream   UpstreamConfig `json:"upstream,omitempty"`
	Policies   []Policy       `json:"policies,omitempty"`
	Operations []Operation    `json:"operations,omitempty"`
}

// Operation represents an API operation
type Operation struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Request     *OperationRequest `json:"request,omitempty"`
}

// Channel represents an API channel
type Channel struct {
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Request     *ChannelRequest `json:"request,omitempty"`
}

// OperationRequest represents operation request details
type OperationRequest struct {
	Method   string   `json:"method,omitempty"`
	Path     string   `json:"path,omitempty"`
	Policies []Policy `json:"policies,omitempty"`
}

// ChannelRequest represents channel request details
type ChannelRequest struct {
	Method   string   `json:"method,omitempty"`
	Name     string   `json:"name,omitempty"`
	Policies []Policy `json:"policies,omitempty"`
}

// Policy represents a request or response policy
type Policy struct {
	ExecutionCondition *string                 `json:"executionCondition,omitempty"`
	Name               string                  `json:"name"`
	Params             *map[string]interface{} `json:"params,omitempty"`
	Version            string                  `json:"version"`
}

// APIMetadata contains minimal API information for handle-to-UUID resolution
type APIMetadata struct {
	UUID             uuid.UUID `json:"id" db:"uuid"`
	Handle           string    `json:"handle" db:"handle"`
	Name             string    `json:"name" db:"name"`
	Version          string    `json:"version" db:"version"`
	Kind             string    `json:"kind" db:"kind"`
	OrganizationUUID uuid.UUID `json:"organizationId" db:"organization_uuid"`
}
