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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// maxConsoleActionBatchBytes caps a single telemetry flush. The console sends
// at most 50 small actions per flush, so this is roughly an order of magnitude
// of headroom over the largest legitimate batch — enough that a client never
// hits it, small enough that the endpoint cannot be used to push bulk data
// through to the collector.
const maxConsoleActionBatchBytes = 64 << 10

// consoleActionsRefillPerSecond / consoleActionsBurst bound how often one user
// may report. Unlike the size caps below, this bounds the *rate* of outbound
// work rather than one request's size.
//
// The key is the user, not the tab, so every console tab a person has open
// draws on the same bucket — and each tab flushes on its own 5s timer and
// again every time it is hidden. Someone switching between several busy tabs
// can briefly send several flushes a second. The burst is sized to absorb that
// without a 429; the sustained refill is what actually caps a scripted caller.
const (
	consoleActionsRefillPerSecond = 1
	consoleActionsBurst           = 30
)

// consoleActionsLimiterTTL is how long an idle user's bucket is kept. The
// limiter is keyed by user, so without eviction the map grows once per user
// forever — a slower version of the problem the limiter exists to prevent.
const consoleActionsLimiterTTL = 30 * time.Minute

// userRateLimiter is a per-user token bucket. Mirrors tokenBucketLimiter in
// thunder_ask_routes.go (stdlib only, no external rate-limiting dependency),
// but keyed, because the limit that matters here is per caller: one busy
// console must not exhaust a budget shared with every other user.
type userRateLimiter struct {
	mu              sync.Mutex
	buckets         map[string]*userBucket
	refillPerSecond float64
	burst           float64
	lastSweep       time.Time
}

type userBucket struct {
	tokens float64
	last   time.Time
	// throttled is set when a request is refused and cleared on the next one
	// allowed, so the handler can report the start of a throttling episode once
	// instead of on every refused request.
	throttled bool
}

func newUserRateLimiter(refillPerSecond, burst float64) *userRateLimiter {
	return &userRateLimiter{
		buckets:         make(map[string]*userBucket),
		refillPerSecond: refillPerSecond,
		burst:           burst,
		lastSweep:       time.Now(),
	}
}

// Allow reports whether this user may send now, consuming a token if so.
func (l *userRateLimiter) Allow(userID string) bool {
	allowed, _ := l.AllowWithTransition(userID)
	return allowed
}

// AllowWithTransition is Allow, plus whether this refusal is the first since
// the user was last allowed through — the moment worth logging.
func (l *userRateLimiter) AllowWithTransition(userID string) (allowed, newlyThrottled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.sweepLocked(now)

	b, ok := l.buckets[userID]
	if !ok {
		b = &userBucket{tokens: l.burst, last: now}
		l.buckets[userID] = b
	}
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = min(l.burst, b.tokens+elapsed*l.refillPerSecond)
		b.last = now
	}
	if b.tokens < 1 {
		newlyThrottled = !b.throttled
		b.throttled = true
		return false, newlyThrottled
	}
	b.tokens--
	b.throttled = false
	return true, false
}

// sweepLocked drops buckets no one has touched for consoleActionsLimiterTTL.
// Amortized into Allow rather than run on a ticker so the limiter owns no
// goroutine of its own.
func (l *userRateLimiter) sweepLocked(now time.Time) {
	if now.Sub(l.lastSweep) < consoleActionsLimiterTTL {
		return
	}
	l.lastSweep = now
	for id, b := range l.buckets {
		if now.Sub(b.last) > consoleActionsLimiterTTL {
			delete(l.buckets, id)
		}
	}
}

// consoleActionsLimiter bounds per-user reporting. Package-level: the limit is
// meaningless if it is rebuilt per request.
var consoleActionsLimiter = newUserRateLimiter(consoleActionsRefillPerSecond, consoleActionsBurst)

// maxConsoleActionsPerBatch mirrors the OpenAPI maxItems. Enforced here too:
// the generated types do not validate, and this bound is what keeps one
// request from fanning out into an unbounded collector payload.
const maxConsoleActionsPerBatch = 50

// registerTelemetryRoutes registers the console's analytics ingest endpoint.
//
// Registered with plain HandleFuncWithValidation — deliberately, and listed in
// api.unguardedRouteAllowlist. There is no meaningful permission to demand:
// the endpoint lets an authenticated user report their own UI activity, under
// their own identity, to a telemetry sink. Gating it on an RBAC permission
// would mean granting that permission to every role that can open the console
// (i.e. all of them), which states a rule without enforcing anything, and
// would silently blind analytics for any role that missed the grant.
//
// It takes no controller: the handler owns no state beyond config, has no
// service or repository layer beneath it, and persists nothing. Routing it
// through the wire graph would add a controller, a provider and a mock for a
// function that validates a list and hands it to a client.
func registerTelemetryRoutes(rr *middleware.RouteRegistrar) {
	rr.HandleFuncWithValidation("POST /telemetry/console-actions", handleConsoleActions)
}

// handleConsoleActions accepts a batch of console UI actions and forwards the
// recognized ones to the analytics collector.
//
// It answers 202 for every well-formed batch, including one where the
// allowlist dropped every action. The console must never retry telemetry and
// must never show the user an error for it, so the only failures worth
// reporting are the ones that mean the client is malformed — a body that is
// not a batch, or one too large to be a real flush.
func handleConsoleActions(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())
	ga := config.GetConfig().GrowthAnalytics

	// Checked per request rather than at registration time (as Track does),
	// because the route must keep answering 202 with reporting switched off:
	// a 404 would make every console in the fleet log a failed telemetry call
	// on every flush.
	enabled := growthanalytics.ConsoleEnabled(ga)

	// Identity and the rate limit are resolved before the body is touched.
	// Claims come from the JWT, not the payload, so nothing here needs the
	// request read first — and rejecting a limited caller only after a 64 KB
	// read and a JSON parse would leave exactly the unbounded work the limit
	// exists to prevent.
	claims := jwtassertion.GetTokenClaims(r.Context())
	if claims == nil {
		// The route is behind the auth middleware, so this is an upstream bug
		// rather than an unauthenticated caller.
		log.Error("telemetry: no token claims on console-actions request, dropping batch")
		utils.WriteSuccessResponse(w, http.StatusAccepted, spec.ConsoleActionBatchResponse{
			Accepted: 0,
			Dropped:  0,
		})
		return
	}

	// 429 rather than a silent 202: a client hitting this is malfunctioning or
	// hostile, and telemetry is the one thing that must never be retried into a
	// limit — the console drops its buffer on any failure, which is the
	// behaviour we want here.
	allowed, newlyThrottled := consoleActionsLimiter.AllowWithTransition(claims.Sub)
	if !allowed {
		// The console drops its buffer on any failure, so a 429 is data that is
		// gone for good. Logged so that loss is visible — but once per throttling
		// episode, not per refused request, or a caller looping into the limit
		// would turn it into a log flood.
		if newlyThrottled {
			log.Warn("telemetry: console action reporting rate-limited; batches are being dropped",
				"userId", claims.Sub,
				"refillPerSecond", consoleActionsRefillPerSecond,
				"burst", consoleActionsBurst)
		}
		utils.WriteErrorResponse(w, http.StatusTooManyRequests,
			"console action reporting rate limit exceeded")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxConsoleActionBatchBytes))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			utils.WriteErrorResponse(w, http.StatusRequestEntityTooLarge,
				"console action batch exceeds the accepted size")
			return
		}
		utils.WriteErrorResponse(w, http.StatusBadRequest, "failed to read console action batch")
		return
	}

	var req spec.ConsoleActionBatchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "malformed console action batch")
		return
	}
	if len(req.Actions) == 0 {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "console action batch contains no actions")
		return
	}
	if len(req.Actions) > maxConsoleActionsPerBatch {
		utils.WriteErrorResponse(w, http.StatusRequestEntityTooLarge,
			"console action batch contains too many actions")
		return
	}

	// Checked after the rate limit, not before: the limit must hold whether or
	// not this deployment forwards anything.
	if !enabled {
		// Accepted and discarded: the client's contract is unchanged whether
		// or not this deployment reports anything.
		utils.WriteSuccessResponse(w, http.StatusAccepted, spec.ConsoleActionBatchResponse{
			Accepted: 0,
			Dropped:  int32(len(req.Actions)),
		})
		return
	}

	inputs := make([]growthanalytics.ConsoleActionInput, 0, len(req.Actions))
	for _, a := range req.Actions {
		in := growthanalytics.ConsoleActionInput{
			Action:     a.Action,
			Dimensions: a.Dimensions,
		}
		if a.OccurredAt != nil {
			in.OccurredAt = *a.OccurredAt
		}
		if a.Page != nil {
			in.Page = *a.Page
		}
		if a.SessionId != nil {
			in.SessionID = *a.SessionId
		}
		inputs = append(inputs, in)
	}

	// Identity is taken from the token, never from the payload: the browser
	// reports what happened, the server decides who it happened to. OuId
	// rather than middleware.OUIDFromRequest because this route is not
	// org-scoped — see registerTelemetryRoutes — so RequireOrgMatch, which is
	// what populates the resolved-org context, never runs for it.
	actions, dropped := growthanalytics.BuildConsoleActions(inputs, growthanalytics.ConsoleContext{
		UserID:    claims.Sub,
		CompanyID: claims.OuId,
		IPAddress: consoleClientIP(r),
		UserAgent: truncateUserAgent(r.UserAgent()),
		URI:       r.Referer(),
	}, ga)

	if dropped > 0 {
		// Worth a log line rather than silence: the usual cause is a console
		// build emitting an action this service does not know yet, which is
		// invisible in Moesif precisely because the action never arrives.
		log.Warn("telemetry: dropped unrecognized console actions",
			"dropped", dropped, "accepted", len(actions))
	}

	growthanalytics.ReportConsoleActions(r.Context(), ga, jwtassertion.GetJWTFromContext(r.Context()), actions)

	utils.WriteSuccessResponse(w, http.StatusAccepted, spec.ConsoleActionBatchResponse{
		Accepted: int32(len(actions)),
		Dropped:  int32(dropped),
	})
}

// truncateUserAgent bounds the user agent without splitting a rune: a plain
// byte slice can cut a multi-byte character in half, and JSON encoding then
// rewrites the broken tail to U+FFFD in the value sent to the collector.
func truncateUserAgent(ua string) string {
	if len(ua) <= maxUserAgentLength {
		return ua
	}
	n := maxUserAgentLength
	for n > 0 && !utf8.RuneStart(ua[n]) {
		n--
	}
	return ua[:n]
}

// maxUserAgentLength bounds the user agent copied onto every action.
//
// Headers are not covered by the request body cap: Go allows up to
// MaxHeaderBytes (1 MB by default), and this value is duplicated onto each of
// the batch's actions, so one accepted request could otherwise expand into a
// payload two orders of magnitude larger than the request that produced it.
const maxUserAgentLength = 256

// consoleClientIP resolves the reporting browser's address, preferring the
// proxy-supplied forwarded header over the immediate peer.
//
// The forwarded value is parsed rather than trusted. A client can set
// X-Forwarded-For itself and a fronting proxy only appends to it, so the
// left-most entry is caller-controlled: unvalidated, it would let any
// authenticated user stamp an arbitrary string — of arbitrary length — onto
// every action as its source address. Anything that is not an IP falls back to
// the immediate peer, which the caller cannot forge.
func consoleClientIP(r *http.Request) string {
	return growthanalytics.ClientIP(r)
}
