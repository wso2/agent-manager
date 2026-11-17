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
	"context"
	"encoding/json"
	"strings"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// ModuleEventSink records module events in the event outbox as local events,
// which the outbox drainer hands to the in-process subscribers.
type ModuleEventSink struct{ db *gorm.DB }

// NewModuleEventSink returns the outbox writer for module events.
func NewModuleEventSink(db *gorm.DB) *ModuleEventSink { return &ModuleEventSink{db: db} }

// Put records e, inside tx when tx is not nil so it is delivered only if tx commits.
func (s *ModuleEventSink) Put(ctx context.Context, tx *gorm.DB, e modkit.Event) error {
	body, err := json.Marshal(events.Event{
		SpecVersion: events.SpecVersion, ID: e.ID, Source: e.Source, Type: events.TypePrefix + e.Type,
		Subject: e.Subject, Time: e.Time.UTC(), DataContentType: events.ContentTypeJSON, Data: e.Data,
		OrgID: e.OrgID, Scope: events.ScopeOrg, Local: true,
	})
	if err != nil {
		return err
	}
	db := tx
	if db == nil {
		db = s.db
	}
	row := eventOutboxRow{ID: outboxID(e.ID), OrgID: e.OrgID, Body: body, AvailableAt: dbx.Now()}
	_, err = dbx.InsertIfAbsent(db.WithContext(ctx), &row)
	return err
}

// ModuleEvent is the module view of a local event read back from the outbox.
func ModuleEvent(e events.Event) modkit.Event {
	var data json.RawMessage
	switch d := e.Data.(type) {
	case nil:
	case json.RawMessage:
		data = d
	default:
		data, _ = json.Marshal(d)
	}
	return modkit.Event{
		ID: e.ID, Type: strings.TrimPrefix(e.Type, events.TypePrefix), Source: e.Source,
		OrgID: e.OrgID, Subject: e.Subject, Time: e.Time, Data: data,
	}
}
