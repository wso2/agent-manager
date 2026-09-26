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

// Package growthanalytics reports opt-in feature-usage telemetry
// (MOESIF_ENABLED, off by default) from agent-manager-service to Moesif, per the Product Feature
// Usage Tracking taxonomy (see samples/products/agent-manager/taxonomy.yaml
// in the Feature Usage Tracking initiative docs). It instruments the
// "Agent Development", "Security & Access", "Discovery", "Deployment Ops"
// and "Observability" feature categories — every route registered with
// Track, all of which are mutating (POST/PUT/PATCH/DELETE); read paths are
// deliberately not tracked.
//
// Events are posted directly to Moesif's Events API
// (POST /v1/events) through moesif-collector-api, an authenticated
// OpenChoreo reverse-proxy component that validates a platform-idp JWT and
// injects the real Moesif Application ID server-side — this package never
// handles that credential (see clients/moesifcollector). Earlier this went
// through the Moesif SDK directly; that's no longer available from this
// deployment, hence the proxy.
//
// Authentication to the proxy is delegated, not configured: each event is
// sent using the bearer JWT already on the request being tracked (the same
// token the caller authenticated to this service with), not a static
// credential read from config. The proxy only checks that a token's issuer
// is platform-idp — no scope or audience check — so the caller's own token
// is already sufficient. This means there is no shared secret to provision,
// rotate, or leak for this feature, and every event is naturally
// attributable to the real caller.
//
// Tracking is always fire-and-forget: it must never change a request's
// outcome or add observable latency to it. The wrapped handler always runs
// first, on the caller's own goroutine, completely outside any recover() —
// a panic there is the router's concern, not this package's, and must
// propagate exactly as it would for an untracked route. Only the
// metadata-building and event-send that happens *after* the handler
// returns — which can no longer affect the response already written — runs
// under a recover() and is dispatched to its own goroutine, so a bad token,
// an unreachable proxy, or any bug in this package surfaces as a log line,
// never a hung or broken request.
package growthanalytics

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/moesifcollector"
	"github.com/wso2/agent-manager/agent-manager-service/clients/requests"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
)

type ctxKey int

const overridesKey ctxKey = iota

// eventSendTimeout bounds how long the detached goroutine that posts an
// event to the collector proxy may run, so a hung/unreachable proxy can
// never accumulate unbounded background goroutines.
const eventSendTimeout = 10 * time.Second

// dimensionOverrides carries dimension values a handler only learns after
// looking at the request body (e.g. create-agent's creation_method, which
// depends on the payload's provisioning type) from the handler back to the
// point where the event is built. Track's static `dimensions` argument
// can't express these — it's fixed at route-registration time — so a
// handler reports them at request time instead, via SetDimension.
type dimensionOverrides struct {
	mu   sync.Mutex
	vals map[string]interface{}
}

func (d *dimensionOverrides) set(key string, value interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.vals == nil {
		d.vals = make(map[string]interface{})
	}
	d.vals[key] = value
}

func (d *dimensionOverrides) snapshot() map[string]interface{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]interface{}, len(d.vals))
	for k, v := range d.vals {
		out[k] = v
	}
	return out
}

// SetDimension lets a handler wrapped by Track report a dimension value that
// can only be known once the handler actually runs and inspects the request
// — e.g. amp.agent-development.create-agent's creation_method, which depends
// on the request body's provisioning type, not just which route fired.
// Overrides win over any static dimension of the same name passed to Track.
//
// It's a no-op if called outside a Track-wrapped request (e.g. a unit test
// that calls the controller directly), so handlers can call it
// unconditionally without checking whether tracking is enabled.
func SetDimension(ctx context.Context, key string, value interface{}) {
	if o, ok := ctx.Value(overridesKey).(*dimensionOverrides); ok {
		o.set(key, value)
	}
}

// DynamicOutcome is a sentinel value: pass it under the "outcome" key in
// Track's dimensions to have the actual outcome ("success"/"failure")
// computed from the wrapped handler's real response status instead of a
// fixed value supplied up front.
var DynamicOutcome = struct{ dynamicOutcome bool }{true}

// statusHolder carries the wrapped handler's real response status from the
// request goroutine to the event-building step, since "outcome" is the only
// dimension that depends on the response.
type statusHolder struct{ code int }

type statusRecordingWriter struct {
	http.ResponseWriter
	holder      *statusHolder
	wroteHeader bool
}

func (s *statusRecordingWriter) WriteHeader(code int) {
	if !s.wroteHeader {
		s.holder.code = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the underlying writer to http.ResponseController, so Flush,
// Hijack and the rest keep working through this wrapper. No route Track wraps
// streams today, but middleware.responseRecorder — the only other
// ResponseWriter wrapper in this service — implements this for the same
// reason, and a tracked route that starts streaming should not silently break
// because tracking was added to it.
func (s *statusRecordingWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// eventSender abstracts posting a built event to the collector proxy, so
// tests can substitute a fake and never make a network call.
type eventSender interface {
	SendEvent(ctx context.Context, evt moesifcollector.Event) error
}

// sharedHTTPClient is reused across every Track-wrapped request so
// connections to the collector proxy get pooled rather than dialed fresh
// per event.
var sharedHTTPClient = requests.NewRetryableHTTPClient(&http.Client{Timeout: eventSendTimeout})

// newSender builds the eventSender used to deliver an event, authenticating
// with the given caller JWT (forwarded from the request being tracked, not
// a config credential — see the package doc comment). A package var so
// tests can substitute a fake in place of the real client.
var newSender = func(ga config.GrowthAnalyticsConfig, token string) eventSender {
	return moesifcollector.NewClient(sharedHTTPClient, ga.MoesifCollectorBaseURL, token, ga.MoesifCollectorHostHeader)
}

// Track wraps handler so calls to it are reported to Moesif as the given
// feature-taxonomy event name (e.g. "amp.agent-development.create-agent"),
// enriched with the given dimensions — the taxonomy's per-feature dimension
// values. Most dimensions are fixed for a given route (e.g. update_target)
// and can be passed as plain values; pass DynamicOutcome under the
// "outcome" key to have it computed from the handler's real response status
// instead. Pass nil when a feature has no dimensions.
//
// Track is a no-op — returns handler unchanged — unless telemetry export is
// both configured (GrowthAnalytics.MoesifCollectorBaseURL is set) and
// switched on (GrowthAnalytics.Enabled). The two are separate because the
// collector URL always resolves once deployed, so its presence cannot
// signal intent: MOESIF_ENABLED is what turns reporting off in an
// environment without deleting the rest of the configuration.
// IsOnPremDeployment is not consulted: both switches are off by default, so
// any deployment that reports has opted in, and deployment_model labels its
// events as on-prem or SaaS. Every route Track wraps requires
// authentication (see the package doc comment on the token this uses), so a
// missing caller JWT at send time is treated as a bug, not a normal case —
// it's logged and the event is dropped rather than sent unauthenticated.
//
// Request and response bodies are never forwarded to Moesif: several of the
// routes this wraps return generated secrets (API keys, tokens, identity
// secrets) in the response body.
func Track(featureCode string, dimensions map[string]interface{}, handler http.HandlerFunc) http.HandlerFunc {
	ga := config.GetConfig().GrowthAnalytics
	if !ga.Enabled || ga.MoesifCollectorBaseURL == "" {
		return handler
	}

	return func(w http.ResponseWriter, r *http.Request) {
		overrides := &dimensionOverrides{}
		ctx := context.WithValue(r.Context(), overridesKey, overrides)
		holder := &statusHolder{code: http.StatusOK}
		sw := &statusRecordingWriter{ResponseWriter: w, holder: holder}

		reqSnapshot := snapshotRequest(r)
		userID := identifyUser(ctx)
		companyID := middleware.OUIDFromRequest(r)
		token := jwtassertion.GetJWTFromContext(ctx)

		// The business handler runs completely outside any recover() here —
		// a panic in it must propagate exactly as it would for an untracked
		// route, not get silently absorbed by this package.
		handler(sw, r.WithContext(ctx))

		reportEvent(r.Context(), ga, token, featureCode, dimensions, overrides, holder, reqSnapshot, userID, companyID)
	}
}

// reportEvent resolves the event's final metadata and fires off the send to
// the collector proxy on a detached goroutine. The response has already
// been fully written by the time this runs, so nothing here can affect the
// request's outcome — a recover() guards the synchronous part (building the
// event) and the goroutine guards itself the same way.
func reportEvent(
	requestCtx context.Context,
	ga config.GrowthAnalyticsConfig,
	token string,
	featureCode string,
	dimensions map[string]interface{},
	overrides *dimensionOverrides,
	holder *statusHolder,
	reqSnapshot moesifcollector.EventRequest,
	userID, companyID string,
) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("growthanalytics: recovered panic reporting event, request already served", "panic", rec, "feature", featureCode)
		}
	}()

	if token == "" {
		// Every route Track wraps requires authentication, so this means the
		// caller's JWT didn't make it onto the request context — a bug
		// upstream, not a normal unauthenticated request. Drop the event
		// rather than send it to the proxy with no Authorization header.
		slog.Error("growthanalytics: no caller JWT on request context, dropping event", "feature", featureCode)
		return
	}

	resolved := resolveDimensions(dimensions, holder)
	for k, v := range overrides.snapshot() {
		if resolved == nil {
			resolved = make(map[string]interface{})
		}
		resolved[k] = v
	}
	metadata := buildMetadata(featureCode, resolved, config.GetConfig().PackageVersion, ga.DeploymentModel, ga.Environment, SourceAPI)

	evt := moesifcollector.Event{
		Request: reqSnapshot,
		Response: moesifcollector.EventResponse{
			Time:   time.Now().UTC().Format(time.RFC3339),
			Status: holder.code,
		},
		UserID:    userID,
		CompanyID: companyID,
		Metadata:  metadata,
	}

	// Logged here, before the async hand-off, since this is the last point
	// this package controls before the send is fired off in the
	// background — the only place that can say "we tried to send this"
	// rather than silently maybe-doing nothing.
	logger.GetLogger(requestCtx).Debug("growthanalytics: sending feature-usage event to Moesif collector",
		"feature", featureCode,
		"org_id", companyID,
		"metadata", metadata,
	)

	// Same drop-when-full backstop as console sends (see
	// maxInFlightConsoleSends): a slow collector must cost a fixed number of
	// goroutines, not one per tracked request. Captured once so the goroutine
	// releases the semaphore it acquired even if tests swap the variable.
	slots := eventSendSlots
	select {
	case slots <- struct{}{}:
	default:
		slog.Warn("growthanalytics: event send pool is full, dropping event",
			"feature", featureCode, "maxInFlight", maxInFlightEventSends)
		return
	}

	sender := newSender(ga, token)
	go func() {
		defer func() { <-slots }()
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("growthanalytics: recovered panic sending event", "panic", rec, "feature", featureCode)
			}
		}()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), eventSendTimeout)
		defer cancel()
		if err := sender.SendEvent(ctx, evt); err != nil {
			slog.Error("growthanalytics: failed to send event to Moesif collector", "error", err, "feature", featureCode)
		}
	}()
}

// maxInFlightEventSends caps concurrent feature-usage event sends. Events come
// from API mutations and so outnumber console batches, hence a larger pool than
// maxInFlightConsoleSends; a full pool drops the event rather than queueing it.
const maxInFlightEventSends = 64

// eventSendSlots is a counting semaphore; a token is held for one send.
var eventSendSlots = make(chan struct{}, maxInFlightEventSends)

// identifyUser resolves the event's user_id from the request's validated
// JWT, mirroring the taxonomy's need to attribute usage to the acting user.
func identifyUser(ctx context.Context) string {
	claims := jwtassertion.GetTokenClaims(ctx)
	if claims == nil {
		return ""
	}
	return claims.Sub
}

// snapshotRequest captures the request-side fields reported to Moesif.
// Deliberately excludes headers and body — see the package doc comment.
func snapshotRequest(r *http.Request) moesifcollector.EventRequest {
	return moesifcollector.EventRequest{
		Time:      time.Now().UTC().Format(time.RFC3339),
		URI:       requestURI(r),
		Verb:      r.Method,
		IPAddress: ClientIP(r),
	}
}

// requestURI reconstructs the request's absolute URL on a best-effort basis:
// this service normally sits behind a gateway/proxy, so the scheme is taken
// from X-Forwarded-Proto when present rather than assumed from r.TLS.
func requestURI(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "http" || proto == "https" {
		scheme = proto
	}
	host := r.Host
	if host == "" {
		host = "unknown"
	}
	// sanitizeURI drops the query string and fragment: search terms and
	// filter values must not reach Moesif, the same rule the console path
	// applies.
	return sanitizeURI(scheme + "://" + host + r.URL.RequestURI())
}

// ClientIP extracts the caller's address, preferring the first
// X-Forwarded-For entry over the immediate TCP peer. The forwarded value is
// caller-controlled, so it is used only when it parses as an IP; anything else
// falls back to the peer rather than reaching Moesif verbatim.
func ClientIP(r *http.Request) string {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peer = host
	}
	fwd := r.Header.Get("X-Forwarded-For")
	if fwd == "" {
		return peer
	}
	if idx := strings.IndexByte(fwd, ','); idx >= 0 {
		fwd = fwd[:idx]
	}
	if ip := net.ParseIP(strings.TrimSpace(fwd)); ip != nil {
		return ip.String()
	}
	return peer
}

// resolveDimensions substitutes DynamicOutcome (if present) with the real
// outcome derived from the handler's captured response status, leaving
// every other dimension untouched. The input map is never mutated.
func resolveDimensions(dimensions map[string]interface{}, holder *statusHolder) map[string]interface{} {
	if dimensions == nil {
		return nil
	}
	resolved := make(map[string]interface{}, len(dimensions))
	for k, v := range dimensions {
		if k == "outcome" && v == DynamicOutcome {
			code := http.StatusOK
			if holder != nil {
				code = holder.code
			}
			resolved[k] = outcomeFromStatus(code)
			continue
		}
		resolved[k] = v
	}
	return resolved
}

// buildMetadata assembles the event metadata: the feature code every event
// must carry, product/deployment context, and the route's resolved
// taxonomy dimensions. Extracted as a pure function so the metadata shape
// is unit-testable without touching the network.
//
// deploymentModel comes from config rather than being compiled in, so a
// deployment that is not the cloud one cannot silently label its events
// "saas" — see GrowthAnalyticsConfig.DeploymentModel.
//
// environment is what makes dev/stage/prod usage separable in Moesif: every
// environment reports into a single Moesif application, so without this
// field the only way to tell them apart is parsing the host out of each
// event's request URI. An empty value omits the field entirely rather than
// reporting environment:"" — a blank bucket would be indistinguishable from
// a real environment named "".
//
// source says which stream the record came from — SourceAPI for the endpoint
// events this package's Track produces, SourceConsole for the UI actions
// reported through console.go. Both streams share "platform": "Agent Manager"
// so they aggregate into one product view; source is what lets a query keep
// them apart without knowing that one arrives as a Moesif Event and the other
// as a Moesif Action.
func buildMetadata(featureCode string, dimensions map[string]interface{}, productVersion, deploymentModel, environment, source string) map[string]interface{} {
	meta := map[string]interface{}{
		"platform":         "Agent Manager",
		"source":           source,
		"growth_action":    featureCode,
		"product_version":  productVersion,
		"deployment_model": deploymentModel,
	}
	if environment != "" {
		meta["environment"] = environment
	}
	for k, v := range dimensions {
		meta[k] = v
	}
	return meta
}

// outcomeFromStatus classifies an HTTP status code into the "success"/
// "failure" outcome dimension the taxonomy declares for features whose
// tracked action can fail (e.g. amp.agent-development.build-agent).
func outcomeFromStatus(status int) string {
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return "success"
	}
	return "failure"
}
