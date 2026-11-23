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

// Package notifyv1 is the notifications/v1 capability: the core sends a module's event
// to notification channels the module names, such as a budget's warning channels.
package notifyv1

import (
	"context"

	"gorm.io/gorm"
)

// ID is the capability id. The core provides it; a module lists it in Requires.
const ID = "notifications/v1"

// Notification is one event for named channels.
type Notification struct {
	// OrgID is the organization whose channels are named.
	OrgID string
	// OrgName is the organization's handle, set on the event for channels that show it.
	OrgName string
	// Channels are the organization's notification channels, by name. A name with no
	// channel, or whose channel may not be used for Project, is skipped.
	Channels []string
	// Type is the event type, "<module>.<event>".
	Type string
	// ID identifies the event. The same ID reaches a channel once, so a module that
	// derives it from what happened can send again without duplicates.
	ID string
	// Project and Agent are what the event is about; Project also limits it to channels
	// available to that project. Both are optional.
	Project string
	Agent   string
	// Subject names the resource the event is about, such as a budget.
	Subject string
	// Data is the event's data; a "message" string is the summary channels show.
	Data map[string]any
}

// Unusable is a channel name that cannot be used.
type Unusable struct {
	Name string
	// Missing is true when the organization has no channel of that name, false when the
	// channel exists but may not be used for the project.
	Missing bool
}

// Notifier sends events to named channels.
type Notifier interface {
	// Notify queues n for its channels inside tx, so it is sent only if tx commits, with
	// the core's delivery retries and channel kinds. A nil tx uses the core's database.
	Notify(ctx context.Context, tx *gorm.DB, n Notification) error
	// Unusable returns the names among names that cannot be used for project ("" for the
	// whole organization), in their order.
	Unusable(ctx context.Context, orgID, project string, names []string) ([]Unusable, error)
}
