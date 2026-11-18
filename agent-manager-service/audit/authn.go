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

package audit

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// authnFailureWindow is the per-source bucket over which failures are counted.
const authnFailureWindow = time.Minute

// maxAuthnFailuresPerWindow bounds how many failure events one source produces
// per window. Beyond it, failures are counted and folded into the next emitted
// event rather than each producing a record.
//
// Without this, an expired token in an open browser tab — or a deliberate flood
// — turns an authentication problem into an audit-volume problem. The count is
// preserved, so the signal survives even when the individual events do not.
const maxAuthnFailuresPerWindow = 10

// authnLimiter throttles authentication-failure events per source IP.
type authnLimiter struct {
	mu      sync.Mutex
	buckets map[string]*authnBucket
	// lastSweep bounds map growth: a scan of expired buckets runs at most once
	// per window, so a spray of distinct source IPs cannot grow the map forever.
	lastSweep time.Time
}

type authnBucket struct {
	windowStart time.Time
	emitted     int
	suppressed  int
}

func newAuthnLimiter() *authnLimiter {
	return &authnLimiter{buckets: map[string]*authnBucket{}, lastSweep: time.Now()}
}

// allow reports whether an event should be emitted for this source, and returns
// how many were suppressed since the last emission.
func (l *authnLimiter) allow(source string, now time.Time) (emit bool, suppressed int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.sweepLocked(now)

	b, ok := l.buckets[source]
	if !ok || now.Sub(b.windowStart) >= authnFailureWindow {
		l.buckets[source] = &authnBucket{windowStart: now, emitted: 1}
		if ok {
			// Carry the tail of the previous window into the first event of
			// this one so nothing is lost silently at a window boundary.
			return true, b.suppressed
		}
		return true, 0
	}

	if b.emitted < maxAuthnFailuresPerWindow {
		b.emitted++
		s := b.suppressed
		b.suppressed = 0
		return true, s
	}

	b.suppressed++
	return false, 0
}

// sweepLocked drops buckets whose window has passed. Caller must hold the lock.
func (l *authnLimiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < authnFailureWindow {
		return
	}
	l.lastSweep = now
	for k, b := range l.buckets {
		// Keep buckets that still hold an unreported suppression count.
		if now.Sub(b.windowStart) >= 2*authnFailureWindow && b.suppressed == 0 {
			delete(l.buckets, k)
		}
	}
}

// AuthFailureRecorder returns a handler for authentication failures, suitable
// for jwtassertion.SetAuthFailureHook.
//
// Authentication failures happen before route matching, so no recorder is on
// the request context — this closes over one instead. The event carries the
// classified reason, the source IP and the path, and never any part of the
// token that was rejected.
func AuthFailureRecorder(recorder Recorder) func(r *http.Request, reason string) {
	limiter := newAuthnLimiter()

	return func(r *http.Request, reason string) {
		if recorder == nil || r == nil {
			return
		}
		ip := utils.ClientIP(r)
		emit, suppressed := limiter.allow(ip, time.Now())
		if !emit {
			return
		}

		ctx := r.Context()
		e := Event{
			Action:      ActionAuthnFailure,
			ActionClass: ClassAuthn,
			Severity:    SeverityWarning,
			Outcome:     OutcomeDeny,
			StatusCode:  http.StatusUnauthorized,
			ActorType:   ActorAnonymous,
			AuthMethod:  "jwt-bearer",
			// The surface is known but the route is not: rejection happens
			// before route matching, so the raw path is the only locator there
			// is. It is sanitised on the way into the record.
			Surface:       SurfaceAPI,
			SourceIP:      ip,
			UserAgent:     r.UserAgent(),
			RequestMethod: r.Method,
			RequestPath:   r.URL.Path,
			CorrelationID: logger.GetCorrelationId(ctx),
			Details: map[string]any{
				"reason":     reason,
				"authHeader": r.Header.Get("Authorization") != "",
			},
		}
		if suppressed > 0 {
			e.Details["suppressedCount"] = suppressed
		}

		recorder.Record(context.WithoutCancel(ctx), e)
	}
}
