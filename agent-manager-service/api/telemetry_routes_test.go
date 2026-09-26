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

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
)

// consoleActionsRequest builds an authenticated request the way the middleware
// stack would leave it by the time the handler runs.
// useFreshLimiter gives the calling test its own rate limiter for its duration.
//
// The handler goes through the package-level consoleActionsLimiter, and every
// test here reports as the same subject. Sharing it means the tests only pass
// while their combined requests stay under the burst — add one more test, or
// run with -count=2, and they start failing with 429 for reasons unrelated to
// what they check. Per test rather than per request: the rate-limit test calls
// the handler repeatedly and needs the bucket to drain across those calls.
func useFreshLimiter(t *testing.T) {
	t.Helper()
	original := consoleActionsLimiter
	consoleActionsLimiter = newUserRateLimiter(consoleActionsRefillPerSecond, consoleActionsBurst)
	t.Cleanup(func() { consoleActionsLimiter = original })
}

func consoleActionsRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/telemetry/console-actions", strings.NewReader(body))
	ctx := jwtassertion.ContextWithTokenClaims(context.Background(),
		&jwtassertion.TokenClaims{Sub: "user-1", OuId: "ou-1"})
	ctx = jwtassertion.ContextWithJWT(ctx, "caller-jwt")
	return r.WithContext(ctx)
}

func decodeBatchResponse(t *testing.T, body []byte) (accepted, dropped float64) {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, body)
	}
	// WriteSuccessResponse may wrap the payload; accept either shape.
	payload := resp
	if inner, ok := resp["data"].(map[string]interface{}); ok {
		payload = inner
	}
	accepted, _ = payload["accepted"].(float64)
	dropped, _ = payload["dropped"].(float64)
	return accepted, dropped
}

// TestConsoleActionsRejectsMalformedBody: a body that is not a batch is the
// one client-side failure worth reporting, since retrying will not fix it.
func TestConsoleActionsRejectsMalformedBody(t *testing.T) {
	useFreshLimiter(t)

	for _, body := range []string{"not json", `{"actions":[]}`, `{}`} {
		w := httptest.NewRecorder()
		handleConsoleActions(w, consoleActionsRequest(t, body))
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %q → status %d, want 400", body, w.Code)
		}
	}
}

// TestConsoleActionsRejectsOversizedBatch bounds the fan-out: one request must
// not be able to push an unbounded payload through to the collector.
func TestConsoleActionsRejectsOversizedBatch(t *testing.T) {
	useFreshLimiter(t)

	actions := make([]map[string]string, maxConsoleActionsPerBatch+1)
	for i := range actions {
		actions[i] = map[string]string{"action": "amp.console.navigation.page-view"}
	}
	body, err := json.Marshal(map[string]interface{}{"actions": actions})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	w := httptest.NewRecorder()
	handleConsoleActions(w, consoleActionsRequest(t, string(body)))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", w.Code)
	}
}

func TestConsoleActionsRejectsOversizedBody(t *testing.T) {
	useFreshLimiter(t)

	padding := strings.Repeat("x", maxConsoleActionBatchBytes+1)
	body := `{"actions":[{"action":"amp.console.navigation.page-view","page":"` + padding + `"}]}`

	w := httptest.NewRecorder()
	r := consoleActionsRequest(t, body)
	handleConsoleActions(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", w.Code)
	}
}

// TestConsoleActionsAcceptsBatchWithUnknownActions is the compatibility
// contract: a console newer than the service must degrade quietly. The
// unrecognized actions are dropped and counted, never 4xx'd, because the
// client cannot act on the failure and must not retry.
//
// Telemetry export is off in the unit tier (no collector URL configured), so
// every action reports as dropped — which is itself the behavior that matters
// here: the response shape and status do not depend on whether this deployment
// reports anything.
func TestConsoleActionsAcceptsBatchWithUnknownActions(t *testing.T) {
	useFreshLimiter(t)

	body := `{"actions":[
		{"action":"amp.console.navigation.page-view"},
		{"action":"amp.console.invented-by-a-newer-console"}
	]}`

	w := httptest.NewRecorder()
	handleConsoleActions(w, consoleActionsRequest(t, body))

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	accepted, dropped := decodeBatchResponse(t, w.Body.Bytes())
	if accepted+dropped != 2 {
		t.Errorf("accepted+dropped = %v, want 2 (body %s)", accepted+dropped, w.Body.String())
	}
}

// TestConsoleActionsWithReportingDisabledStillAccepts: with export off the
// endpoint must keep answering 202, or every console in the fleet logs a
// failed telemetry call on every flush.
func TestConsoleActionsWithReportingDisabledStillAccepts(t *testing.T) {
	useFreshLimiter(t)

	body := `{"actions":[{"action":"amp.console.navigation.page-view"}]}`

	w := httptest.NewRecorder()
	handleConsoleActions(w, consoleActionsRequest(t, body))

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", w.Code)
	}
}

// TestConsoleActionsWithoutClaimsDoesNotPanic: the route sits behind the auth
// middleware, so absent claims mean an upstream bug — it must degrade to a
// dropped batch, not a 500.
func TestConsoleActionsWithoutClaimsDoesNotPanic(t *testing.T) {
	useFreshLimiter(t)

	r := httptest.NewRequest(http.MethodPost, "/telemetry/console-actions",
		bytes.NewBufferString(`{"actions":[{"action":"amp.console.navigation.page-view"}]}`))

	w := httptest.NewRecorder()
	handleConsoleActions(w, r)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", w.Code)
	}
}

// TestConsoleActionsRateLimitsPerUser: the endpoint is designed to be called
// continuously, so an authenticated caller must not be able to drive unbounded
// outbound work by looping batches.
func TestConsoleActionsRateLimitsPerUser(t *testing.T) {
	useFreshLimiter(t)

	body := `{"actions":[{"action":"amp.console.navigation.page-view"}]}`

	// The burst is spendable...
	for i := 0; i < consoleActionsBurst; i++ {
		w := httptest.NewRecorder()
		handleConsoleActions(w, consoleActionsRequest(t, body))
		if w.Code != http.StatusAccepted {
			t.Fatalf("request %d within burst → %d, want 202", i, w.Code)
		}
	}

	// ...and then exhausted.
	w := httptest.NewRecorder()
	handleConsoleActions(w, consoleActionsRequest(t, body))
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("request past burst → %d, want 429", w.Code)
	}
}

// TestConsoleActionsRateLimitIsPerUserNotGlobal: one busy console must not
// spend the budget of every other user.
func TestConsoleActionsRateLimitIsPerUserNotGlobal(t *testing.T) {
	useFreshLimiter(t)

	for i := 0; i <= consoleActionsBurst; i++ {
		consoleActionsLimiter.Allow("noisy-user")
	}

	if !consoleActionsLimiter.Allow("quiet-user") {
		t.Error("a second user was refused after the first exhausted its own budget")
	}
}

// TestUserRateLimiterEvictsIdleBuckets: the limiter is keyed by user, so
// without eviction it leaks one bucket per user seen — a slower version of the
// exhaustion it exists to prevent.
func TestUserRateLimiterEvictsIdleBuckets(t *testing.T) {
	l := newUserRateLimiter(consoleActionsRefillPerSecond, consoleActionsBurst)
	l.Allow("old-user")

	// Age the bucket and the sweep clock past the TTL.
	l.mu.Lock()
	l.buckets["old-user"].last = time.Now().Add(-2 * consoleActionsLimiterTTL)
	l.lastSweep = time.Now().Add(-2 * consoleActionsLimiterTTL)
	l.mu.Unlock()

	l.Allow("new-user")

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.buckets["old-user"]; ok {
		t.Error("idle bucket was not evicted")
	}
	if _, ok := l.buckets["new-user"]; !ok {
		t.Error("active bucket was evicted")
	}
}

// TestConsoleClientIPRejectsForgedForwardedValues: X-Forwarded-For is
// caller-controlled (a fronting proxy only appends to it), so an unvalidated
// left-most entry would let any authenticated user stamp an arbitrary string
// of arbitrary length onto every action as its source address.
func TestConsoleClientIPRejectsForgedForwardedValues(t *testing.T) {
	tests := []struct {
		name, forwarded, remoteAddr, want string
	}{
		{"first hop of a chain is used", "203.0.113.9, 10.0.0.1", "10.0.0.1:1234", "203.0.113.9"},
		{"single value is trimmed", "  203.0.113.9 ", "10.0.0.1:1234", "203.0.113.9"},
		{"no header falls back to the peer", "", "10.0.0.1:1234", "10.0.0.1"},
		{"non-IP falls back to the peer", "not-an-ip", "10.0.0.1:1234", "10.0.0.1"},
		{"oversized junk falls back to the peer", strings.Repeat("x", 4096), "10.0.0.1:1234", "10.0.0.1"},
		{"IPv6 is accepted", "2001:db8::1", "10.0.0.1:1234", "2001:db8::1"},
		{"unsplittable peer is returned as-is", "", "not-a-host-port", "not-a-host-port"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/telemetry/console-actions", nil)
			r.RemoteAddr = tt.remoteAddr
			if tt.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tt.forwarded)
			}
			if got := consoleClientIP(r); got != tt.want {
				t.Errorf("consoleClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestTruncateUserAgentBoundsAmplification: the body is capped, but headers are
// not, and the user agent is copied onto every action in the batch — so an
// unbounded one turns a 64 KB request into a far larger collector payload.
func TestTruncateUserAgentBoundsAmplification(t *testing.T) {
	if got := truncateUserAgent(strings.Repeat("u", maxUserAgentLength*10)); len(got) != maxUserAgentLength {
		t.Errorf("length = %d, want %d", len(got), maxUserAgentLength)
	}
	if got := truncateUserAgent("Mozilla/5.0"); got != "Mozilla/5.0" {
		t.Errorf("a normal user agent was altered: %q", got)
	}

	// A 3-byte rune straddling the limit must be dropped whole, not cut.
	ua := strings.Repeat("a", maxUserAgentLength-1) + "€€"
	got := truncateUserAgent(ua)
	if !utf8.ValidString(got) {
		t.Errorf("truncation produced invalid UTF-8: %q", got[len(got)-4:])
	}
	if len(got) > maxUserAgentLength {
		t.Errorf("length = %d, want <= %d", len(got), maxUserAgentLength)
	}
}

// TestUserRateLimiterReportsThrottlingOncePerEpisode: a 429 is data the console
// drops for good, so the first refusal is logged — but only the first, or a
// caller looping into the limit would turn the warning into a log flood.
func TestUserRateLimiterReportsThrottlingOncePerEpisode(t *testing.T) {
	l := newUserRateLimiter(consoleActionsRefillPerSecond, consoleActionsBurst)
	for i := 0; i < consoleActionsBurst; i++ {
		if ok, _ := l.AllowWithTransition("user-1"); !ok {
			t.Fatalf("request %d within burst was refused", i)
		}
	}

	if ok, first := l.AllowWithTransition("user-1"); ok || !first {
		t.Fatalf("first refusal: allowed=%v newlyThrottled=%v, want false/true", ok, first)
	}
	for i := 0; i < 5; i++ {
		if ok, first := l.AllowWithTransition("user-1"); ok || first {
			t.Fatalf("repeat refusal %d: allowed=%v newlyThrottled=%v, want false/false", i, ok, first)
		}
	}

	// Once allowed again, the next refusal is a new episode and is reported.
	l.mu.Lock()
	l.buckets["user-1"].tokens = 1
	l.mu.Unlock()
	if ok, _ := l.AllowWithTransition("user-1"); !ok {
		t.Fatal("refilled request was refused")
	}
	if _, first := l.AllowWithTransition("user-1"); !first {
		t.Error("a refusal after recovery was not reported as a new episode")
	}
}
