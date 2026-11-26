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

package events

import (
	"context"
	"regexp"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// annotations let a handler add what the route middleware cannot see: the
// environment when it arrives in the request body, or extra details.
type annotations struct {
	environment string
	project     string
	data        map[string]any
	also        []Extra
}

// Extra is one more event a handler raised next to its route's event.
type Extra struct {
	Type string
	Data map[string]any
}

type annotationsKey struct{}

// WithAnnotations returns a context a handler can annotate with SetEnvironment
// and AddData, or a module with modkit.EventEnvironment and modkit.EventData,
// and the holder the middleware reads back.
func WithAnnotations(ctx context.Context) (context.Context, *annotations) {
	a := &annotations{}
	ctx = modkit.WithEventScope(ctx, a)
	return context.WithValue(ctx, annotationsKey{}, a), a
}

func annotationsFrom(ctx context.Context) *annotations {
	a, _ := ctx.Value(annotationsKey{}).(*annotations)
	return a
}

// SetEnvironment records the environment the request acted on. A no-op
// outside an event-emitting route.
func SetEnvironment(ctx context.Context, env string) {
	if a := annotationsFrom(ctx); a != nil {
		a.EventEnvironment(env)
	}
}

// SetProject records the project the request acted on, when its path and query do not name it. A
// no-op outside an event-emitting route.
func SetProject(ctx context.Context, project string) {
	if a := annotationsFrom(ctx); a != nil {
		a.project = project
	}
}

// AddData adds a field to the event's data. A no-op outside an
// event-emitting route.
func AddData(ctx context.Context, key string, value any) {
	if a := annotationsFrom(ctx); a != nil {
		a.EventData(key, value)
	}
}

// EventEnvironment implements modkit.EventScope.
func (a *annotations) EventEnvironment(name string) {
	a.environment = name
}

// EventData implements modkit.EventScope.
func (a *annotations) EventData(key string, value any) {
	if a.data == nil {
		a.data = map[string]any{}
	}
	a.data[key] = value
}

// EventAlso implements modkit.EventScope.
func (a *annotations) EventAlso(eventType string, data map[string]any) {
	a.also = append(a.also, Extra{Type: eventType, Data: data})
}

// Also returns the extra events, in the order they were raised.
func (a *annotations) Also() []Extra {
	if a == nil {
		return nil
	}
	return a.also
}

// Environment returns the annotated environment.
func (a *annotations) Environment() string {
	if a == nil {
		return ""
	}
	return a.environment
}

// Project returns the annotated project.
func (a *annotations) Project() string {
	if a == nil {
		return ""
	}
	return a.project
}

// Data returns the annotated data.
func (a *annotations) Data() map[string]any {
	if a == nil {
		return nil
	}
	return a.data
}

// sensitiveKey matches field names that hold credentials. Response bodies are
// resources, not secrets, but a few carry one (a client secret, an API key
// shown once); routes known to return one omit data entirely, and this is the
// backstop for the rest.
var sensitiveKey = regexp.MustCompile(`(?i)(secret|password|passwd|token|apikey|api_key|credential|private_?key|authorization|^key$|signing)`)

// Redact removes credential-looking fields from decoded JSON, recursively.
func Redact(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if sensitiveKey.MatchString(strings.ReplaceAll(k, "-", "_")) {
				continue
			}
			out[k] = Redact(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = Redact(val)
		}
		return out
	default:
		return v
	}
}
