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

// Package eventdestinations defines how events leave the platform. An event
// destination has a kind, and each kind has a Driver that validates the
// destination's settings and delivers events to it. Matching, queuing,
// retries and the delivery log are the same for every kind and live outside
// the drivers.
package eventdestinations

import (
	"context"
	"encoding/json"
	"sort"
)

// Message is one event to deliver.
type Message struct {
	// ID is the event ID. Every attempt for the same event carries it, so the
	// destination can drop duplicates.
	ID string
	// Type is the event's CloudEvents type.
	Type string
	// Body is the structured-mode CloudEvent JSON.
	Body []byte
}

// Destination is an event destination resolved for sending.
type Destination struct {
	// Config is the destination's normalized settings.
	Config json.RawMessage
	// Secret is the destination's decrypted secret; empty for kinds without
	// one.
	Secret string
}

// Result is the outcome of one delivery attempt.
type Result struct {
	// StatusCode is the destination's HTTP status, or 0 if it gave none.
	StatusCode int
	// Err is nil when the destination accepted the message.
	Err error
	// Retryable says whether a later attempt could succeed.
	Retryable bool
}

// Driver delivers events to the destinations of one kind.
type Driver interface {
	// Kind is the destination kind this driver handles, e.g. "webhook".
	Kind() string
	// NormalizeConfig validates the kind's settings and returns them in
	// canonical form. Invalid settings wrap utils.ErrInvalidInput.
	NormalizeConfig(ctx context.Context, raw json.RawMessage) (json.RawMessage, error)
	// RedactConfig masks any part of the settings that may carry a
	// credential, for callers who may see a destination but not change it.
	RedactConfig(cfg json.RawMessage) json.RawMessage
	// Origin describes where the destination sends, safe to log and audit
	// (scheme and host for an HTTP destination).
	Origin(cfg json.RawMessage) string
	// NewSecret generates a secret for a new destination or a rotation. ok is
	// false for kinds that have none.
	NewSecret() (secret string, ok bool, err error)
	// Deliver sends one message.
	Deliver(ctx context.Context, dest Destination, msg Message) Result
}

// CredentialDriver is a Driver whose settings include a credential the user
// supplies, such as a service URL with a token in it. The credential is
// stored encrypted, like a generated secret, and never returned: callers see
// only the settings SplitCredential leaves in place.
type CredentialDriver interface {
	Driver
	// SplitCredential takes the credential out of settings NormalizeConfig
	// accepted, returning the settings to store in plain and the credential.
	// An empty credential keeps the destination's current one.
	SplitCredential(cfg json.RawMessage) (json.RawMessage, string)
}

// Service is one destination service a kind can deliver to, such as Slack
// for a notification destination.
type Service struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Category string `json:"category"`
	// URLFormat shows the shape of the service URL the user enters.
	URLFormat string `json:"urlFormat"`
	// DocsURL explains how to get the values the URL needs.
	DocsURL string `json:"docsUrl"`
}

// ServiceCatalog is implemented by drivers that deliver to several services,
// so the console can offer them without knowing each one.
type ServiceCatalog interface {
	Services() []Service
}

// Registry holds the driver of each supported kind.
type Registry struct {
	drivers map[string]Driver
}

// NewRegistry creates a registry of the given drivers.
func NewRegistry(drivers ...Driver) *Registry {
	r := &Registry{drivers: make(map[string]Driver, len(drivers))}
	for _, d := range drivers {
		r.drivers[d.Kind()] = d
	}
	return r
}

// Get returns the driver of a kind.
func (r *Registry) Get(kind string) (Driver, bool) {
	d, ok := r.drivers[kind]
	return d, ok
}

// All returns every driver, sorted by kind.
func (r *Registry) All() []Driver {
	out := make([]Driver, 0, len(r.drivers))
	for _, k := range r.Kinds() {
		out = append(out, r.drivers[k])
	}
	return out
}

// Kinds lists the supported kinds, sorted.
func (r *Registry) Kinds() []string {
	kinds := make([]string, 0, len(r.drivers))
	for k := range r.drivers {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	return kinds
}
