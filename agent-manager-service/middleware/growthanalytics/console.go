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

// This file covers the console half of growth analytics: what a user did in
// the UI, as opposed to which API endpoint they called (track.go).
//
// The two halves stay deliberately separate all the way into Moesif. Endpoint
// usage is reported as Moesif *Events*, console usage as Moesif *Actions* —
// different object types, so console volume can never skew the metrics
// computed over the API-call stream. They rejoin on user_id/company_id, and
// every record from either half carries "platform": "Agent Manager" plus a
// "source" of "api" or "console", so a query can span both or isolate one
// without knowing which ingestion shape it came from.
//
// The taxonomy below is intentionally disjoint from the endpoint feature
// codes: an action here exists precisely because the API cannot see it. Page
// views, a create dialog opened and then abandoned, a cancelled delete
// confirmation, a copied connection snippet, an empty state someone stared at
// — none of those produce a request, so none of them can be a Track code.
// Nothing here duplicates a successful mutation that track.go already reports;
// what it adds is the denominator those successes are a fraction of.
//
// Unlike Track, the console cannot be trusted to name its own telemetry: the
// payload arrives from a browser. Every action name and every dimension key is
// checked against consoleActions below, and anything unrecognized is dropped
// rather than forwarded. That is a data-quality control first (a typo'd action
// silently becoming a new Moesif metric is worse than it being dropped loudly)
// and a blast-radius control second.
package growthanalytics

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wso2/agent-manager/agent-manager-service/clients/moesifcollector"
	"github.com/wso2/agent-manager/agent-manager-service/config"
)

// Source values for the "source" metadata field every record carries.
const (
	// SourceAPI marks the endpoint feature-usage events Track produces.
	SourceAPI = "api"
	// SourceConsole marks the UI actions reported through this file.
	SourceConsole = "console"
)

// maxDimensionValueLength bounds a single dimension's string value. The
// allowlist already fixes which keys may appear, but not how long their
// values are, and these values come from a browser. Anything longer is a bug
// or an attempt to smuggle a payload through a dimension; truncating keeps a
// usable value rather than dropping the whole action over it.
const maxDimensionValueLength = 256

// maxURILength bounds a reported page/route. Matches the OpenAPI maxLength for
// ConsoleAction.page; enforced here too, since the generated types do not
// validate and this value reaches Moesif as an event URI.
const maxURILength = 512

// maxSessionIDLength bounds the client-generated session token, matching the
// OpenAPI maxLength for the same reason.
const maxSessionIDLength = 64

// consoleActionSendTimeout bounds the detached goroutine that forwards a
// batch, for the same reason eventSendTimeout bounds a single event: an
// unreachable collector must not accumulate background goroutines.
const consoleActionSendTimeout = 10 * time.Second

// maxInFlightConsoleSends caps how many batches may be in flight at once.
//
// The per-user rate limit on the ingest route bounds any single caller, but not
// the fleet: enough consoles reporting at once — or one slow collector holding
// every send open for the full timeout — would otherwise grow goroutines and
// outbound connections without limit. This is the backstop that makes the
// worst case a fixed cost rather than a function of traffic.
//
// Telemetry is the right thing to shed under pressure, so a full pool drops the
// batch instead of queueing it: a queue deep enough to matter is just a slower
// way to run out of memory, and a stale action is worth less than the headroom
// it costs.
const maxInFlightConsoleSends = 32

// consoleSendSlots is a counting semaphore; a token is held for the duration of
// one send.
var consoleSendSlots = make(chan struct{}, maxInFlightConsoleSends)

// consoleActions is the closed set of console actions this service will
// forward, mapped to the dimension keys each one may carry.
//
// Adding an action here is the whole registration step — there is no separate
// list to keep in sync — but it is a deliberate one: an action that is not
// here does not reach Moesif, so a console release that starts emitting a new
// action needs a matching service release. That ordering is intentional. The
// alternative, forwarding whatever the browser sends, means Moesif's metric
// list becomes whatever any console build ever typo'd.
//
// Dimension keys are per-action rather than global so a key cannot quietly
// acquire a second meaning on a different action.
var consoleActions = map[string]map[string]bool{
	// Navigation and discovery. None of this reaches the API today: the
	// console reads through cached queries, so which pages a user actually
	// works in is invisible server-side.
	"amp.console.navigation.page-view":       dims("route", "entity_scope", "referrer_route"),
	"amp.console.navigation.tab-switch":      dims("page", "tab"),
	"amp.console.navigation.search":          dims("scope", "result_count", "query_length", "zero_results"),
	"amp.console.navigation.filter-applied":  dims("page", "filter"),
	"amp.console.navigation.context-switch":  dims("switch_type"),
	"amp.console.navigation.docs-link-click": dims("doc_target", "from_page"),

	// Intent. These are the counterpart to the successful mutations track.go
	// reports: opening a create dialog, giving up on the form, cancelling a
	// destructive confirmation. Pairing them with the endpoint events is what
	// turns "N agents were created" into a conversion rate.
	"amp.console.intent.dialog-opened":          dims("entity", "trigger"),
	"amp.console.intent.form-abandoned":         dims("entity", "furthest_step", "seconds_on_form"),
	"amp.console.intent.validation-error":       dims("entity", "field"),
	"amp.console.intent.confirmation-cancelled": dims("entity", "destructive_action"),

	// Onboarding. Wizard progress and abandonment are only observable in the
	// UI — a wizard that is quit on step 2 issues no request at all.
	"amp.console.onboarding.first-session":      dims("referrer"),
	"amp.console.onboarding.wizard-step-viewed": dims("wizard", "step", "step_index"),
	"amp.console.onboarding.wizard-abandoned":   dims("wizard", "last_step"),

	// Interactive surfaces. Mostly read paths, which track.go deliberately
	// does not instrument, so usage of the observability and test tooling is
	// currently unmeasurable.
	"amp.console.playground.test-invoke":              dims("invoke_mode"),
	"amp.console.observability.trace-opened":          dims("source_page", "has_error"),
	"amp.console.observability.log-query":             dims("time_range", "filter_count"),
	"amp.console.observability.metrics-range-changed": dims("time_range"),
	"amp.console.observability.eval-result-viewed":    dims("evaluator_type"),

	// Self-serve utility: strong activation signals that leave no server-side
	// trace. snippet_type names *which* snippet was copied — never its value;
	// several of these snippets contain credentials.
	"amp.console.utility.copy-snippet":       dims("snippet_type", "language"),
	"amp.console.utility.download-artifact":  dims("artifact_type"),
	"amp.console.utility.secret-revealed":    dims("secret_type", "surface"),
	"amp.console.utility.cli-connect-viewed": dims("context"),

	// Friction. The reason to instrument the console at all: an error the user
	// saw, an empty state they landed on, a control RBAC hid from them.
	"amp.console.friction.error-shown":       dims("operation", "status", "error_code"),
	"amp.console.friction.empty-state":       dims("resource", "state"),
	"amp.console.friction.permission-denied": dims("feature", "required_permission"),
	"amp.console.friction.session-expired":   dims("page"),
	"amp.console.friction.client-error":      dims("component", "error_name"),

	// Session shape.
	"amp.console.session.start": dims("referrer", "viewport", "browser"),
	"amp.console.session.end":   dims("duration_seconds", "page_count", "action_count"),
}

func dims(keys ...string) map[string]bool {
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		out[k] = true
	}
	return out
}

// ConsoleActionsAllowed reports whether a console action name is registered.
// Exported for the console-taxonomy test and for callers that want to reject
// early; BuildConsoleActions applies it regardless.
func ConsoleActionsAllowed(action string) bool {
	_, ok := consoleActions[action]
	return ok
}

// ConsoleActionInput is one action as submitted by the console, already
// decoded from the request body. Kept independent of the generated spec types
// so this package stays usable from a test without constructing spec structs.
type ConsoleActionInput struct {
	Action     string
	OccurredAt time.Time
	Page       string
	SessionID  string
	Dimensions map[string]interface{}
}

// ConsoleContext is the server-resolved identity and request context stamped
// onto every action in a batch. None of it is taken from the request body:
// the browser says what happened, the server says who it happened to.
type ConsoleContext struct {
	UserID    string
	CompanyID string
	IPAddress string
	UserAgent string
	// URI is the console origin the batch was reported from, used as each
	// action's request URI when the action itself names no page.
	URI string
}

// ConsoleEnabled reports whether console actions should be forwarded: the
// collector must be configured, telemetry must be on, and the console stream
// must not have been switched off on its own. See
// GrowthAnalyticsConfig.ConsoleEnabled for why the last is separate.
func ConsoleEnabled(ga config.GrowthAnalyticsConfig) bool {
	return ga.Enabled && ga.ConsoleEnabled && ga.MoesifCollectorBaseURL != ""
}

// BuildConsoleActions converts a submitted batch into collector actions,
// dropping anything the taxonomy does not recognize. It returns the actions to
// forward and how many inputs were discarded.
//
// Dropping rather than erroring is what lets a console deploy that runs ahead
// of a service deploy degrade quietly: the unknown actions vanish, the known
// ones still land. The count is returned so that "quietly" still means
// "visibly in the response and the logs".
func BuildConsoleActions(
	inputs []ConsoleActionInput,
	base ConsoleContext,
	ga config.GrowthAnalyticsConfig,
) (actions []moesifcollector.Action, dropped int) {
	now := time.Now().UTC()
	productVersion := config.GetConfig().PackageVersion

	for _, in := range inputs {
		allowedDims, ok := consoleActions[in.Action]
		if !ok {
			dropped++
			continue
		}

		occurred := in.OccurredAt
		// A browser clock can be wrong in both directions, but a zero value
		// means the field was simply absent. Only that case falls back to
		// receipt time; a skewed-but-present clock is preserved, since
		// rewriting it would silently destroy the ordering within a flush.
		if occurred.IsZero() {
			occurred = now
		}

		// Both URI sources are caller-supplied: Page comes from the request
		// body, and base.URI from the Referer header. The console strips query
		// strings before sending, but nothing stops another client from
		// posting a URI with a query, a fragment, or an unbounded length — so
		// the trimming is redone here rather than trusted.
		page := sanitizeURI(in.Page)
		uri := sanitizeURI(base.URI)
		if page != "" {
			uri = page
		}

		metadata := buildMetadata(
			in.Action,
			sanitizeDimensions(in.Dimensions, allowedDims),
			productVersion,
			ga.DeploymentModel,
			ga.Environment,
			SourceConsole,
		)
		if page != "" {
			// Deliberately not "page": three allowlisted actions (tab-switch,
			// filter-applied, session-expired) declare a dimension of that
			// name, and writing the route here would silently overwrite the
			// value the action actually reported.
			metadata["route_path"] = page
		}

		actions = append(actions, moesifcollector.Action{
			ActionName: in.Action,
			Request: moesifcollector.ActionRequest{
				Time:            occurred.UTC().Format(time.RFC3339),
				URI:             uri,
				IPAddress:       base.IPAddress,
				UserAgentString: base.UserAgent,
			},
			UserID:       base.UserID,
			CompanyID:    base.CompanyID,
			SessionToken: truncateTo(in.SessionID, maxSessionIDLength),
			Metadata:     metadata,
		})
	}

	return actions, dropped
}

// sanitizeDimensions keeps only the keys this action declares and only scalar
// values. A nested object or array in a dimension is always either a bug or an
// attempt to ship a payload (a request body, a form's contents) into Moesif
// under a dimension name, so it is dropped rather than flattened.
func sanitizeDimensions(in map[string]interface{}, allowed map[string]bool) map[string]interface{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		if !allowed[k] {
			continue
		}
		switch val := v.(type) {
		case string:
			out[k] = truncate(val)
		case bool, float64, float32, int, int32, int64:
			out[k] = val
		default:
			// Unsupported shape (object, array, null) — skip the key.
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func truncate(s string) string {
	return truncateTo(s, maxDimensionValueLength)
}

// truncateTo bounds a string to max BYTES without splitting a rune.
//
// A plain s[:max] can cut a multi-byte character in half, and the resulting
// invalid UTF-8 is silently rewritten to U+FFFD on the way into JSON — a
// corrupted trailing character in whatever reaches the collector. Backing up
// to the last rune boundary costs nothing and keeps the value well-formed.
func truncateTo(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}

// sanitizeURI strips anything after the path from a caller-supplied URI and
// bounds its length.
//
// A query string or fragment is where identifiers and free-form input hide —
// a search term, a selected trace, a filter value — so neither belongs in
// telemetry even when a well-behaved client already removes them.
func sanitizeURI(raw string) string {
	if raw == "" {
		return ""
	}
	if idx := strings.IndexAny(raw, "?#"); idx >= 0 {
		raw = raw[:idx]
	}
	return truncateTo(raw, maxURILength)
}

// newConsoleSender builds the sender used to forward a batch. A package var so
// tests can substitute a fake, mirroring newSender in track.go.
var newConsoleSender = func(ga config.GrowthAnalyticsConfig, token string) consoleActionSender {
	return moesifcollector.NewClient(sharedHTTPClient, ga.MoesifCollectorBaseURL, token, ga.MoesifCollectorHostHeader)
}

type consoleActionSender interface {
	SendActions(ctx context.Context, actions []moesifcollector.Action) error
}

// ReportConsoleActions forwards a built batch to the collector on a detached
// goroutine and returns immediately.
//
// Same discipline as reportEvent: telemetry must never delay or fail the
// request that carried it. The caller has already answered 202 by the time the
// send completes, so every failure here is a log line and nothing more. token
// is the caller's own JWT, forwarded exactly as track.go does — no shared
// credential exists for this path either.
func ReportConsoleActions(
	requestCtx context.Context,
	ga config.GrowthAnalyticsConfig,
	token string,
	actions []moesifcollector.Action,
) {
	if len(actions) == 0 {
		return
	}
	if token == "" {
		slog.Error("growthanalytics: no caller JWT on console telemetry request, dropping batch",
			"actions", len(actions))
		return
	}

	// Claimed before the goroutine starts, not inside it: acquiring after the
	// spawn would let unbounded goroutines pile up waiting for a slot, which is
	// the thing being bounded.
	// Captured once: the goroutine must release the same semaphore it
	// acquired, even if the package variable is swapped in the meantime (as
	// tests do to isolate their slot counts).
	slots := consoleSendSlots
	select {
	case slots <- struct{}{}:
	default:
		slog.Warn("growthanalytics: console action send pool is full, dropping batch",
			"actions", len(actions), "maxInFlight", maxInFlightConsoleSends)
		return
	}

	sender := newConsoleSender(ga, token)
	// context.WithoutCancel: the request context is cancelled the moment the
	// 202 is written, which is before this send would otherwise finish.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), consoleActionSendTimeout)

	go func() {
		defer func() { <-slots }()
		defer cancel()
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("growthanalytics: recovered panic sending console actions", "panic", rec)
			}
		}()
		if err := sender.SendActions(ctx, actions); err != nil {
			slog.Error("growthanalytics: failed to send console actions to Moesif collector",
				"error", err, "actions", len(actions))
		}
	}()
}
