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

// Webhook endpoint scopes.
const (
	WebhookScopeOrg     = "org"
	WebhookScopeProject = "project"
	WebhookScopeAgent   = "agent"
)

// Webhook delivery statuses.
const (
	WebhookDeliveryPending   = "pending"
	WebhookDeliveryDelivered = "delivered"
	WebhookDeliveryFailed    = "failed"
)

// WebhookEndpoint receives the events it subscribes to. Its scope decides
// which events can reach it: org endpoints get org events, project endpoints
// get their project's events, agent endpoints get their agent's events in the
// environments they select.
type WebhookEndpoint struct {
	ID              uuid.UUID `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	OUID            string    `gorm:"column:ou_id;not null" json:"ouId"`
	Scope           string    `gorm:"column:scope;not null" json:"scope"`
	ProjectName     string    `gorm:"column:project_name;not null" json:"projectName"`
	AgentName       string    `gorm:"column:agent_name;not null" json:"agentName"`
	Environments    []string  `gorm:"column:environments;type:jsonb;serializer:json;not null" json:"environments"`
	Name            string    `gorm:"column:name;not null" json:"name"`
	Description     string    `gorm:"column:description;not null" json:"description"`
	URL             string    `gorm:"column:url;not null" json:"url"`
	EventTypes      []string  `gorm:"column:event_types;type:jsonb;serializer:json;not null" json:"eventTypes"`
	Enabled         bool      `gorm:"column:enabled;not null" json:"enabled"`
	SecretEncrypted []byte    `gorm:"column:secret_encrypted;not null" json:"-"`
	CreatedBy       string    `gorm:"column:created_by;not null" json:"createdBy"`
	CreatedAt       time.Time `gorm:"column:created_at;not null;default:NOW()" json:"createdAt"`
	UpdatedAt       time.Time `gorm:"column:updated_at;not null;default:NOW()" json:"updatedAt"`
}

func (WebhookEndpoint) TableName() string { return "webhook_endpoints" }

// Subscribes reports whether the endpoint wants an event type.
func (e *WebhookEndpoint) Subscribes(eventType string) bool {
	for _, t := range e.EventTypes {
		if t == eventType {
			return true
		}
	}
	return false
}

// WebhookDelivery is the outcome of sending one event to one endpoint.
type WebhookDelivery struct {
	ID           uuid.UUID  `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	EndpointID   uuid.UUID  `gorm:"column:endpoint_id;type:uuid;not null" json:"endpointId"`
	OUID         string     `gorm:"column:ou_id;not null" json:"ouId"`
	EventID      string     `gorm:"column:event_id;not null" json:"eventId"`
	EventType    string     `gorm:"column:event_type;not null" json:"eventType"`
	Status       string     `gorm:"column:status;not null" json:"status"`
	Attempts     int        `gorm:"column:attempts;not null" json:"attempts"`
	ResponseCode *int       `gorm:"column:response_code" json:"responseCode,omitempty"`
	LastError    string     `gorm:"column:last_error;not null" json:"lastError,omitempty"`
	DeliveredAt  *time.Time `gorm:"column:delivered_at" json:"deliveredAt,omitempty"`
	CreatedAt    time.Time  `gorm:"column:created_at;not null;default:NOW()" json:"createdAt"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;not null;default:NOW()" json:"updatedAt"`
}

func (WebhookDelivery) TableName() string { return "webhook_deliveries" }
