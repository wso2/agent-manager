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

// Package events publishes platform events (a project created, an agent
// deployed, a monitor run finished) to NATS JetStream. A dispatcher subscribes
// to them and delivers each one to the webhook endpoints configured for its
// org, project or agent.
//
// Events report what happened. They carry no judgement: there are no
// thresholds or detections here, receivers decide what matters to them.
// Every event is a CloudEvents 1.0 event.
package events

import (
	"net/url"
	"strings"
	"time"
)

// Scope is the level an event belongs to, and the level of the endpoints
// that can subscribe to it.
type Scope string

const (
	ScopeOrg     Scope = "org"
	ScopeProject Scope = "project"
	ScopeAgent   Scope = "agent"
)

// Valid reports whether s is a known scope.
func (s Scope) Valid() bool {
	return s == ScopeOrg || s == ScopeProject || s == ScopeAgent
}

// CloudEvents attributes this service sets.
const (
	// SpecVersion is the CloudEvents specification version.
	SpecVersion = "1.0"
	// TypePrefix namespaces event types, as CloudEvents recommends
	// reverse-DNS names. Catalog and subscriptions use the short name after it.
	TypePrefix = "com.wso2.agentmanager."
	// ContentTypeJSON is the datacontenttype of every event.
	ContentTypeJSON = "application/json"
	// MediaType is the Content-Type of an event sent in structured mode.
	MediaType = "application/cloudevents+json"
	// sourceRoot prefixes every source URI-reference.
	sourceRoot = "/agent-manager"
)

// Event is a CloudEvents 1.0 event (https://cloudevents.io), in structured
// JSON form: it is what is published to the bus and POSTed to webhooks.
//
// The context attributes follow the specification. Extension attributes
// (lowercase, alphanumeric) carry where the event belongs, so a receiver can
// route without reading data.
type Event struct {
	SpecVersion     string    `json:"specversion"`
	ID              string    `json:"id"`
	Source          string    `json:"source"`
	Type            string    `json:"type"`
	Subject         string    `json:"subject,omitempty"`
	Time            time.Time `json:"time"`
	DataContentType string    `json:"datacontenttype"`
	Data            any       `json:"data,omitempty"`

	// Extension attributes.
	OrgID       string `json:"orgid"`
	OrgHandle   string `json:"orghandle,omitempty"`
	Scope       Scope  `json:"scope"`
	Project     string `json:"project,omitempty"`
	Agent       string `json:"agent,omitempty"`
	Environment string `json:"environment,omitempty"`
	ActorType   string `json:"actortype,omitempty"`
	ActorID     string `json:"actorid,omitempty"`
}

// New returns an event of the catalog type name (e.g. "agent.deployed") with
// the CloudEvents attributes set. source is derived from the org, project and
// agent.
func New(id, name string, scope Scope, orgID, project, agent string, at time.Time) Event {
	return Event{
		SpecVersion:     SpecVersion,
		ID:              id,
		Source:          SourceOf(orgID, project, agent),
		Type:            TypePrefix + name,
		Time:            at.UTC(),
		DataContentType: ContentTypeJSON,
		OrgID:           orgID,
		Scope:           scope,
		Project:         project,
		Agent:           agent,
	}
}

// Name is the catalog type name, without the reverse-DNS prefix.
func (e Event) Name() string {
	return strings.TrimPrefix(e.Type, TypePrefix)
}

// SourceOf is the source URI-reference of events about an org, project or
// agent: /agent-manager/orgs/<orgId>[/projects/<project>[/agents/<agent>]].
func SourceOf(orgID, project, agent string) string {
	var b strings.Builder
	b.WriteString(sourceRoot + "/orgs/" + url.PathEscape(orgID))
	if project != "" {
		b.WriteString("/projects/" + url.PathEscape(project))
		if agent != "" {
			b.WriteString("/agents/" + url.PathEscape(agent))
		}
	}
	return b.String()
}
