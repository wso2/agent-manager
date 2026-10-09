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
	"net/http"
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

// Response fields that name the environment, in the order they are tried.
var envResponseFields = []string{"environment", "environmentName", "targetEnvironment"}

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

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx, notes := events.WithAnnotations(r.Context())
			rec := &bodyRecorder{responseRecorder: newResponseRecorder(w), capture: !omitData}
			next(rec, r.WithContext(ctx))

			status := rec.Status()
			if status < 200 || status > 299 {
				return
			}

			// Actor and org resolve the same way the audit trail does.
			ae := audit.BuildEvent(r.Context(), audit.Action(eventType))
			if ae.OUID == "" {
				return
			}
			e := events.New("evt_"+uuid.NewString(), eventType, def.Scope, ae.OUID,
				r.PathValue(utils.PathParamProjName), r.PathValue(utils.PathParamAgentName), time.Now())
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

			e.Environment = notes.Environment()
			for _, p := range envPathParams {
				if e.Environment == "" {
					e.Environment = r.PathValue(p)
				}
			}
			if e.Environment == "" {
				e.Environment = r.URL.Query().Get("environment")
			}
			for _, f := range envResponseFields {
				if e.Environment == "" {
					if s, ok := data[f].(string); ok {
						e.Environment = s
					}
				}
			}

			publisher.Publish(r.Context(), e)
		}
	}
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
