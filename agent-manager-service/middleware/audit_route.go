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
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// WithAudit returns a middleware that records one audit event per request for
// the given route.
//
// It is installed as the outermost per-route wrapper, so it observes the 400
// from path-parameter validation and the 403 from the permission check as well
// as whatever the handler itself returns. It sits inside the JWT middleware,
// which is applied at the mux level, so token claims are already on the context.
//
// The event it writes describes the request envelope: who called what, and what
// came back. It does not describe the domain effect — that is the semantic
// tier's job, and when a semantic event was emitted for a successful request
// this middleware stands down so the trail carries one record, not two.
func WithAudit(recorder audit.Recorder, meta audit.RouteMeta) func(http.HandlerFunc) http.HandlerFunc {
	if !meta.Audited {
		return func(next http.HandlerFunc) http.HandlerFunc { return next }
	}

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := newResponseRecorder(w)

			rbacEnabled := false
			if cfg := config.GetConfig(); cfg != nil {
				rbacEnabled = cfg.RBACEnabled
			}

			ctx := audit.WithRecorder(r.Context(), recorder)
			ctx = audit.WithSource(ctx, audit.Source{
				Surface:      meta.Surface,
				IP:           utils.ClientIP(r),
				UserAgent:    r.UserAgent(),
				Method:       meta.Method,
				Pattern:      meta.Path,
				RBACEnforced: rbacEnabled,
				// Carried on the source so semantic emits inherit it. They know
				// what they changed but not which permission gated the route,
				// and "rbacEnforced: false" without it says a check was skipped
				// without saying which — on exactly the credential and privilege
				// operations where that is worth knowing.
				RequiredPermission: audit.ScopesOf(meta.Perms),
			})
			ctx, scope := audit.NewRequestScope(ctx)

			defer func() {
				// RecovererOnPanic is the outermost middleware in the chain, so a
				// panic unwinds through here first. Record the 500 the recoverer
				// is about to write, then re-panic so its behaviour is unchanged.
				// Without this the request would be recorded as a success.
				panicked := recover()
				if panicked != nil {
					rec.setStatus(http.StatusInternalServerError)
				}

				emitEnvelope(ctx, recorder, meta, rec, scope, start)

				if panicked != nil {
					panic(panicked)
				}
			}()

			next(rec, r.WithContext(ctx))
		}
	}
}

// WithAuditRecorder installs an audit recorder and a surface description on
// every request passing through it.
//
// Used for surfaces that do not register through RouteRegistrar — the internal
// gateway server and MCP — so that emit sites there reach a real recorder
// instead of the "not installed" fallback. It records nothing by itself; the
// events come from the handlers.
func WithAuditRecorder(recorder audit.Recorder, surface audit.Surface) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := audit.WithRecorder(r.Context(), recorder)
			ctx = audit.WithSource(ctx, audit.Source{
				Surface:   surface,
				IP:        utils.ClientIP(r),
				UserAgent: r.UserAgent(),
				Method:    r.Method,
				// Surfaces without a registrar have no route pattern, so the
				// path is left to the handler to supply via a semantic emit.
			})
			ctx, _ = audit.NewRequestScope(ctx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// emitEnvelope writes the envelope event unless a semantic event already
// described this request.
func emitEnvelope(
	ctx context.Context,
	recorder audit.Recorder,
	meta audit.RouteMeta,
	rec *responseRecorder,
	scope *audit.RequestScope,
	start time.Time,
) {
	if scope.Suppressed() {
		return
	}
	// Routes a machine polls on a timer record once per caller per window.
	// Without this the gateway bulk-sync endpoints alone would produce millions
	// of near-identical records and bury everything else.
	if meta.Coalesce > 0 && !routeCoalescer.allow(meta.Pattern+"|"+sourceKeyOf(ctx, scope), meta.Coalesce) {
		return
	}
	// A semantic emit already recorded what happened. Keep the envelope for
	// failures: a request rejected before it reached the service emits nothing
	// semantic, and that rejection is exactly what must not go unrecorded.
	if scope.SemanticEmitted() && rec.Status() < http.StatusBadRequest {
		return
	}

	e := audit.BuildEvent(
		ctx, meta.Action,
		audit.Status(rec.Status()),
		audit.RequiredPermissions(meta.Perms...),
		audit.Detail("envelope", true),
	)
	e.DurationMs = time.Since(start).Milliseconds()
	recorder.Record(ctx, e)
}

// routeCoalescer bounds how often a polled route is recorded per caller.
var routeCoalescer = newCoalescer()

// sourceKeyOf identifies the caller for coalescing.
//
// The scope actor comes first, because it is the only one of the three that is
// available on the surface where coalescing actually runs. The polled routes
// are on the internal gateway server, which carries no token: Source is built
// before the handler, so its ActorID is empty there, and the handler is what
// authenticates the gateway. Without the scope this fell through to the source
// IP, and two gateways behind one egress address suppressed each other.
//
// The IP still stands in when nothing identified the caller — a request that
// never authenticated has nothing better, and suppressing a flood of those by
// address is the right behaviour anyway.
func sourceKeyOf(ctx context.Context, scope *audit.RequestScope) string {
	if _, actor, _ := scope.Actor(); actor != "" {
		return actor
	}
	src, _ := audit.SourceFromContext(ctx)
	if src.ActorID != "" {
		return src.ActorID
	}
	return src.IP
}

// coalescer suppresses repeats of the same key within a window.
type coalescer struct {
	mu        sync.Mutex
	seen      map[string]time.Time
	lastSweep time.Time
}

func newCoalescer() *coalescer {
	return &coalescer{seen: map[string]time.Time{}, lastSweep: time.Now()}
}

// allow reports whether this key should be recorded now, and starts a new
// window when it is.
func (c *coalescer) allow(key string, window time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	// Bound map growth: sweep expired entries at most once per window.
	if now.Sub(c.lastSweep) >= window {
		c.lastSweep = now
		for k, at := range c.seen {
			if now.Sub(at) >= window {
				delete(c.seen, k)
			}
		}
	}

	if at, ok := c.seen[key]; ok && now.Sub(at) < window {
		return false
	}
	c.seen[key] = now
	return true
}
