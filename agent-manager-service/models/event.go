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
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"
)

// Event subscription scopes: where a subscription sits.
const (
	EventScopeOrg     = "org"
	EventScopeProject = "project"
	EventScopeAgent   = "agent"
)

// Destination kinds. A kind decides how events are delivered; each has a
// driver in services/eventdestinations.
const (
	DestinationKindWebhook = "webhook"
	// DestinationKindNotify delivers a text notification to a chat, email,
	// paging or push service through the Shoutrrr library.
	DestinationKindNotify = "notify"
)

// Destination availability: which subscriptions below the org may use a
// destination. Org subscriptions may use any destination.
const (
	DestinationAvailableAll      = "all"
	DestinationAvailableOrg      = "org"
	DestinationAvailableProjects = "projects"
)

// Event delivery statuses.
const (
	EventDeliveryPending   = "pending"
	EventDeliveryDelivered = "delivered"
	EventDeliveryFailed    = "failed"
	// EventDeliverySkipped marks a delivery dropped before it was sent
	// because no subscription that queued it still wanted it.
	EventDeliverySkipped = "skipped"
)

// EventDestination is where events leave the platform: a webhook endpoint
// or a notification service. Org admins register destinations; subscriptions
// at org, project and agent level pick one to send their events to.
type EventDestination struct {
	ID          uuid.UUID `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	OUID        string    `gorm:"column:ou_id;not null" json:"ouId"`
	Name        string    `gorm:"column:name;not null" json:"name"`
	Description string    `gorm:"column:description;not null" json:"description"`
	Kind        string    `gorm:"column:kind;not null" json:"kind"`
	// Config holds the kind's non-secret settings, e.g. {"url": "..."}.
	Config json.RawMessage `gorm:"column:config;type:jsonb;not null" json:"config"`
	// SecretEncrypted holds the kind's secret, if it has one: a webhook's
	// signing secret or a notification service URL. Never returned by the
	// API, except a generated secret right after it is generated.
	SecretEncrypted []byte `gorm:"column:secret_encrypted" json:"-"`
	// Availability says which subscriptions below the org may use the
	// destination: those of every project, none, or those of Projects.
	Availability string    `gorm:"column:availability;not null" json:"availability"`
	Projects     []string  `gorm:"column:projects;type:jsonb;serializer:json;not null" json:"projects"`
	CreatedBy    string    `gorm:"column:created_by;not null" json:"createdBy"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;default:NOW()" json:"createdAt"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null;default:NOW()" json:"updatedAt"`
}

func (EventDestination) TableName() string { return "event_destinations" }

// UsableBy reports whether a subscription at scope in project may send to
// the destination. Org subscriptions may use any destination.
func (d *EventDestination) UsableBy(scope, project string) bool {
	if scope == EventScopeOrg {
		return true
	}
	switch d.Availability {
	case DestinationAvailableAll:
		return true
	case DestinationAvailableProjects:
		return slices.Contains(d.Projects, project)
	default:
		return false
	}
}

// EventSubscription sends the events it selects to a destination. Its scope
// decides which events can reach it: an org subscription gets the events of
// the whole org, a project subscription those of its project and its agents,
// an agent subscription those of its agent.
type EventSubscription struct {
	ID            uuid.UUID `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	OUID          string    `gorm:"column:ou_id;not null" json:"ouId"`
	Scope         string    `gorm:"column:scope;not null" json:"scope"`
	ProjectName   string    `gorm:"column:project_name;not null" json:"projectName"`
	AgentName     string    `gorm:"column:agent_name;not null" json:"agentName"`
	Name          string    `gorm:"column:name;not null" json:"name"`
	Description   string    `gorm:"column:description;not null" json:"description"`
	DestinationID uuid.UUID `gorm:"column:destination_id;type:uuid;not null" json:"destinationId"`
	// EventTypes are catalog event types, or just "*" for every type the
	// scope can receive.
	EventTypes []string `gorm:"column:event_types;type:jsonb;serializer:json;not null" json:"eventTypes"`
	// Environments limits the subscription to events in these environments;
	// empty means every environment. Events tied to no environment, such as a
	// build, are sent whatever it holds.
	Environments []string  `gorm:"column:environments;type:jsonb;serializer:json;not null" json:"environments"`
	Enabled      bool      `gorm:"column:enabled;not null" json:"enabled"`
	CreatedBy    string    `gorm:"column:created_by;not null" json:"createdBy"`
	CreatedAt    time.Time `gorm:"column:created_at;not null;default:NOW()" json:"createdAt"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null;default:NOW()" json:"updatedAt"`
}

func (EventSubscription) TableName() string { return "event_subscriptions" }

// EventDelivery is the delivery of one event to one destination. Several
// subscriptions may match the same event and send it to the same
// destination; it is delivered once, and SubscriptionIDs lists them all.
type EventDelivery struct {
	ID              uuid.UUID `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	OUID            string    `gorm:"column:ou_id;not null" json:"ouId"`
	DestinationID   uuid.UUID `gorm:"column:destination_id;type:uuid;not null" json:"destinationId"`
	SubscriptionIDs []string  `gorm:"column:subscription_ids;type:jsonb;serializer:json;not null" json:"subscriptionIds"`
	EventID         string    `gorm:"column:event_id;not null" json:"eventId"`
	EventType       string    `gorm:"column:event_type;not null" json:"eventType"`
	// EventScope, ProjectName, AgentName and Environment say where the event
	// happened, so each level's delivery log shows only its own events.
	EventScope   string     `gorm:"column:event_scope;not null" json:"eventScope"`
	ProjectName  string     `gorm:"column:project_name;not null" json:"projectName"`
	AgentName    string     `gorm:"column:agent_name;not null" json:"agentName"`
	Environment  string     `gorm:"column:environment;not null" json:"environment"`
	Status       string     `gorm:"column:status;not null" json:"status"`
	Attempts     int        `gorm:"column:attempts;not null" json:"attempts"`
	ResponseCode *int       `gorm:"column:response_code" json:"responseCode,omitempty"`
	LastError    string     `gorm:"column:last_error;not null" json:"lastError,omitempty"`
	DeliveredAt  *time.Time `gorm:"column:delivered_at" json:"deliveredAt,omitempty"`
	CreatedAt    time.Time  `gorm:"column:created_at;not null;default:NOW()" json:"createdAt"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;not null;default:NOW()" json:"updatedAt"`
}

func (EventDelivery) TableName() string { return "event_deliveries" }
