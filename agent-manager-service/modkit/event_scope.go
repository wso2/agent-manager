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

package modkit

import "context"

// EventScope is the event a route emits after a successful response. The core
// puts it on the request's context; a module adds to it with EventEnvironment
// and EventData, and raises more events with EmitAlso.
type EventScope interface {
	EventEnvironment(name string)
	EventData(key string, value any)
	EventAlso(eventType string, data map[string]any)
}

type eventScopeKey struct{}

// WithEventScope returns ctx carrying s, for the core to set on requests.
func WithEventScope(ctx context.Context, s EventScope) context.Context {
	return context.WithValue(ctx, eventScopeKey{}, s)
}

// EventEnvironment records the environment the current request acted on, for
// the event its route emits, when the path does not name it. Outside an
// event-emitting route it does nothing.
func EventEnvironment(ctx context.Context, name string) {
	if s, _ := ctx.Value(eventScopeKey{}).(EventScope); s != nil {
		s.EventEnvironment(name)
	}
}

// EventData adds a field to the data of the event the current request's route
// emits, over the field of the same name in the response. Outside an
// event-emitting route it does nothing.
func EventData(ctx context.Context, key string, value any) {
	if s, _ := ctx.Value(eventScopeKey{}).(EventScope); s != nil {
		s.EventData(key, value)
	}
}

// EmitAlso raises one more webhook event after the current request's 2xx response, next to the
// event its route emits: one of the module's event types that has a scope, with data as its
// data. Outside an event-emitting route it does nothing.
func EmitAlso(ctx context.Context, eventType string, data map[string]any) {
	if s, _ := ctx.Value(eventScopeKey{}).(EventScope); s != nil {
		s.EventAlso(eventType, data)
	}
}
