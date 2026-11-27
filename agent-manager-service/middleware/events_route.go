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

package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// maxEventBodyBytes caps the response body copied into an event. A larger
// response is still served in full; the event just carries no data.
const maxEventBodyBytes = 256 << 10

// Path parameters that carry the environment an agent-scope route acts on.
var envPathParams = []string{"envName", "envID"}

// WithEvents publishes the route's catalog event after a successful response.
//
// It wraps the handler directly, inside org resolution and authorization, so
// the resolved org and the caller are on the request context it sees. Routes
// not in the catalog are returned unwrapped.
func WithEvents(publisher events.Publisher, meta audit.RouteMeta) func(http.HandlerFunc) http.HandlerFunc {
	eventType, omitData, ok := events.RouteEventType(meta.Pattern)
	if publisher == nil || !ok {
		return func(next http.HandlerFunc) http.HandlerFunc { return next }
	}
	def, found := events.Lookup(eventType)
	if !found {
		panic("events: route " + meta.Pattern + " emits " + eventType + ", which is not in the catalog")
	}
	return withRouteEvent(publisher, meta, eventType, def.Scope, omitData)
}

// withRouteEvent emits eventType, of scope, after a 2xx response; an empty
// eventType emits nothing. Module routes name their event and its scope
// themselves.
func withRouteEvent(publisher events.Publisher, meta audit.RouteMeta, eventType string, scope events.Scope, omitData bool) func(http.HandlerFunc) http.HandlerFunc {
	if publisher == nil || eventType == "" {
		return func(next http.HandlerFunc) http.HandlerFunc { return next }
	}

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx, notes := events.WithAnnotations(r.Context())
			rec := &bodyRecorder{responseRecorder: newResponseRecorder(w), capture: !omitData}
			next(rec, r.WithContext(ctx))

			status := rec.Status()
			if status < 200 || status > 299 {
				return
			}
			// The response is complete. Send it now, so the client does not
			// wait on the outbox insert below.
			_ = http.NewResponseController(w).Flush()

			// Actor and org resolve the same way the audit trail does.
			ae := audit.BuildEvent(r.Context(), audit.Action(eventType))
			if ae.OUID == "" {
				return
			}
			project := requestProject(r)
			if project == "" {
				project = notes.Project()
			}
			e := events.New("evt_"+uuid.NewString(), eventType, scope, ae.OUID,
				project, r.PathValue(utils.PathParamAgentName), time.Now())
			e.OrgHandle = ae.OrgHandle
			if ae.ActorID != "" {
				e.ActorType, e.ActorID = string(ae.ActorType), ae.ActorID
			}
			// The subject is the resource acted on: the last path parameter,
			// e.g. the monitor name on a monitor route.
			if n := len(meta.Params); n > 0 && meta.Params[n-1] != "orgName" {
				e.Subject = r.PathValue(meta.Params[n-1])
			}

			var data map[string]any
			if !omitData && !rec.overflow && rec.body.Len() > 0 {
				var decoded any
				if json.Unmarshal(rec.body.Bytes(), &decoded) == nil {
					if m, ok := events.Redact(decoded).(map[string]any); ok {
						data = m
					}
				}
			}
			for k, v := range notes.Data() {
				if data == nil {
					data = map[string]any{}
				}
				data[k] = v
			}
			if data != nil {
				e.Data = data
			}
			nameCreated(&e, meta, data)

			e.Environment = notes.Environment()
			for _, p := range envPathParams {
				if e.Environment == "" {
					e.Environment = r.PathValue(p)
				}
			}

			if err := publisher.Publish(r.Context(), e); err != nil {
				slog.ErrorContext(r.Context(), "Failed to persist route event", "eventId", e.ID, "orgId", e.OrgID, "error", err)
			}
			publishExtras(r, publisher, e, notes.Also())
		}
	}
}

// publishExtras publishes the events a handler raised next to its route's event, about the same
// resource and caller. Only the route's own module's types are sent, and none narrower in scope
// than the route's event, whose project and agent it takes; any other is logged and skipped.
func publishExtras(r *http.Request, publisher events.Publisher, route events.Event, extras []events.Extra) {
	module, _, _ := strings.Cut(route.Name(), ".")
	for _, x := range extras {
		def, ok := events.LookupModule(x.Type)
		if !ok || !strings.HasPrefix(x.Type, module+".") {
			slog.ErrorContext(r.Context(), "Skipped an event that is not one of the route module's types", "eventType", x.Type, "route", route.Name())
			continue
		}
		if def.Scope.NarrowerThan(route.Scope) {
			slog.ErrorContext(r.Context(), "Skipped an event narrower in scope than its route's", "eventType", x.Type, "scope", def.Scope, "route", route.Name())
			continue
		}
		e := events.New("evt_"+uuid.NewString(), x.Type, def.Scope, route.OrgID, route.Project, route.Agent, route.Time)
		e.OrgHandle, e.ActorType, e.ActorID = route.OrgHandle, route.ActorType, route.ActorID
		e.Subject, e.Environment = route.Subject, route.Environment
		if data, ok := events.Redact(plainJSON(x.Data)).(map[string]any); ok && len(data) > 0 {
			e.Data = data
		}
		if err := publisher.Publish(r.Context(), e); err != nil {
			slog.ErrorContext(r.Context(), "Failed to persist route event", "eventId", e.ID, "orgId", e.OrgID, "error", err)
		}
	}
}

// plainJSON round-trips data through JSON so Redact also reaches into typed maps and structs; data
// that does not encode is dropped.
func plainJSON(data map[string]any) any {
	if len(data) == 0 {
		return nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// nameCreated points a create event at the resource it created. A create
// route ends in its collection (POST .../agents), so the created name is only
// in the response, and the last path parameter names the parent instead. A
// created project or agent is also set on the event, which moves its source
// down to that resource.
func nameCreated(e *events.Event, meta audit.RouteMeta, data map[string]any) {
	segments := strings.Split(meta.Path, "/")
	collection := segments[len(segments)-1]
	if meta.Method != http.MethodPost || strings.HasPrefix(collection, "{") {
		return
	}
	name := createdName(data)
	if name == "" {
		return
	}
	e.Subject = name
	switch collection {
	case "projects":
		e.Project = name
	case "agents":
		e.Agent = name
	default:
		return
	}
	e.Source = events.SourceOf(e.OrgID, e.Project, e.Agent)
}

// createdName is the handle a create response names: metadata.name on a document, else name.
func createdName(data map[string]any) string {
	if md, ok := data["metadata"].(map[string]any); ok {
		if name, _ := md["name"].(string); name != "" {
			return name
		}
	}
	name, _ := data["name"].(string)
	return name
}

// bodyRecorder keeps a bounded copy of the response body.
type bodyRecorder struct {
	*responseRecorder
	capture  bool
	body     bytes.Buffer
	overflow bool
}

func (b *bodyRecorder) Write(p []byte) (int, error) {
	if b.capture && !b.overflow {
		if b.body.Len()+len(p) > maxEventBodyBytes {
			b.overflow = true
			b.body.Reset()
		} else {
			b.body.Write(p)
		}
	}
	return b.responseRecorder.Write(p)
}
