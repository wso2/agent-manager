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

package growthanalytics

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/moesifcollector"
	"github.com/wso2/agent-manager/agent-manager-service/config"
)

func consoleTestConfig() config.GrowthAnalyticsConfig {
	return config.GrowthAnalyticsConfig{
		Enabled:                true,
		ConsoleEnabled:         true,
		MoesifCollectorBaseURL: "http://collector.invalid/moesif-collector",
		DeploymentModel:        "saas",
		Environment:            "development",
	}
}

func testConsoleContext() ConsoleContext {
	return ConsoleContext{
		UserID:    "user-1",
		CompanyID: "ou-1",
		IPAddress: "203.0.113.7",
		UserAgent: "Mozilla/5.0",
		URI:       "https://console.example.com/",
	}
}

// TestConsoleEnabledRequiresAllThreeSwitches pins the AND: the separate
// console flag must be able to silence the console stream on its own, and must
// not be able to re-enable reporting that MOESIF_ENABLED turned off.
func TestConsoleEnabledRequiresAllThreeSwitches(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*config.GrowthAnalyticsConfig)
		enabled bool
	}{
		{"all on", func(*config.GrowthAnalyticsConfig) {}, true},
		{"console flag off silences console only", func(c *config.GrowthAnalyticsConfig) { c.ConsoleEnabled = false }, false},
		{"master flag off silences console too", func(c *config.GrowthAnalyticsConfig) { c.Enabled = false }, false},
		{"no collector URL", func(c *config.GrowthAnalyticsConfig) { c.MoesifCollectorBaseURL = "" }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ga := consoleTestConfig()
			tt.mutate(&ga)
			if got := ConsoleEnabled(ga); got != tt.enabled {
				t.Errorf("ConsoleEnabled() = %v, want %v", got, tt.enabled)
			}
		})
	}
}

// TestBuildConsoleActionsDropsUnknownActions is the data-quality guard: a
// browser can send any string, and only registered taxonomy names may become
// Moesif actions.
func TestBuildConsoleActionsDropsUnknownActions(t *testing.T) {
	inputs := []ConsoleActionInput{
		{Action: "amp.console.navigation.page-view"},
		{Action: "amp.console.not-a-real-action"},
		{Action: ""},
		{Action: "amp.console.intent.dialog-opened"},
	}

	actions, dropped := BuildConsoleActions(inputs, testConsoleContext(), consoleTestConfig())

	if len(actions) != 2 {
		t.Fatalf("built %d actions, want 2", len(actions))
	}
	if dropped != 2 {
		t.Errorf("dropped = %d, want 2", dropped)
	}
	for _, a := range actions {
		if !ConsoleActionsAllowed(a.ActionName) {
			t.Errorf("forwarded unregistered action %q", a.ActionName)
		}
	}
}

// TestBuildConsoleActionsDropsUnknownDimensions stops a client attaching
// arbitrary keys — which would create arbitrary Moesif fields — to an
// otherwise valid action.
func TestBuildConsoleActionsDropsUnknownDimensions(t *testing.T) {
	inputs := []ConsoleActionInput{{
		Action: "amp.console.intent.dialog-opened",
		Dimensions: map[string]interface{}{
			"entity":       "agent",   // declared
			"trigger":      "toolbar", // declared
			"api_key":      "secret",  // not declared — must not survive
			"seconds_open": 12.0,      // declared on a different action only
		},
	}}

	actions, dropped := BuildConsoleActions(inputs, testConsoleContext(), consoleTestConfig())
	if dropped != 0 || len(actions) != 1 {
		t.Fatalf("got %d actions / %d dropped, want 1/0", len(actions), dropped)
	}

	meta := actions[0].Metadata
	if meta["entity"] != "agent" || meta["trigger"] != "toolbar" {
		t.Errorf("declared dimensions missing: %v", meta)
	}
	if _, ok := meta["api_key"]; ok {
		t.Error("undeclared dimension api_key was forwarded")
	}
	if _, ok := meta["seconds_open"]; ok {
		t.Error("dimension declared on another action was forwarded")
	}
}

// TestBuildConsoleActionsRejectsNonScalarDimensions: a nested value is how a
// request body or form contents would ride into Moesif under a valid key.
func TestBuildConsoleActionsRejectsNonScalarDimensions(t *testing.T) {
	inputs := []ConsoleActionInput{{
		Action: "amp.console.friction.error-shown",
		Dimensions: map[string]interface{}{
			"operation":  map[string]interface{}{"body": "sensitive"},
			"status":     500.0,
			"error_code": []interface{}{"a", "b"},
		},
	}}

	actions, _ := BuildConsoleActions(inputs, testConsoleContext(), consoleTestConfig())
	meta := actions[0].Metadata
	if _, ok := meta["operation"]; ok {
		t.Error("object-valued dimension was forwarded")
	}
	if _, ok := meta["error_code"]; ok {
		t.Error("array-valued dimension was forwarded")
	}
	if meta["status"] != 500.0 {
		t.Errorf("scalar dimension status = %v, want 500", meta["status"])
	}
}

func TestBuildConsoleActionsTruncatesLongValues(t *testing.T) {
	long := strings.Repeat("x", maxDimensionValueLength*2)
	inputs := []ConsoleActionInput{{
		Action:     "amp.console.navigation.page-view",
		Dimensions: map[string]interface{}{"route": long},
	}}

	actions, _ := BuildConsoleActions(inputs, testConsoleContext(), consoleTestConfig())
	got, _ := actions[0].Metadata["route"].(string)
	if len(got) != maxDimensionValueLength {
		t.Errorf("route length = %d, want %d", len(got), maxDimensionValueLength)
	}
}

// TestBuildConsoleActionsMetadataIdentifiesConsole is what the whole split
// rests on: both streams say "Agent Manager", and only source tells them apart.
func TestBuildConsoleActionsMetadataIdentifiesConsole(t *testing.T) {
	inputs := []ConsoleActionInput{{
		Action: "amp.console.navigation.page-view",
		Page:   "/orgs/acme/projects/p1",
	}}

	actions, _ := BuildConsoleActions(inputs, testConsoleContext(), consoleTestConfig())
	meta := actions[0].Metadata

	if meta["platform"] != "Agent Manager" {
		t.Errorf("platform = %v, want \"Agent Manager\"", meta["platform"])
	}
	if meta["source"] != SourceConsole {
		t.Errorf("source = %v, want %q", meta["source"], SourceConsole)
	}
	if meta["growth_action"] != "amp.console.navigation.page-view" {
		t.Errorf("growth_action = %v", meta["growth_action"])
	}
	if meta["environment"] != "development" || meta["deployment_model"] != "saas" {
		t.Errorf("deployment context missing: %v", meta)
	}
	if meta["route_path"] != "/orgs/acme/projects/p1" {
		t.Errorf("route_path = %v", meta["route_path"])
	}
}

// TestBuildConsoleActionsRouteDoesNotClobberPageDimension: three allowlisted
// actions declare their own "page" dimension, so the action's route must not
// be written under that key.
func TestBuildConsoleActionsRouteDoesNotClobberPageDimension(t *testing.T) {
	inputs := []ConsoleActionInput{{
		Action:     "amp.console.navigation.tab-switch",
		Page:       "/orgs/acme/agents/a1/configure",
		Dimensions: map[string]interface{}{"page": "configure-agent", "tab": "mcp"},
	}}

	actions, _ := BuildConsoleActions(inputs, testConsoleContext(), consoleTestConfig())
	meta := actions[0].Metadata

	if meta["page"] != "configure-agent" {
		t.Errorf("page dimension = %v, want the reported dimension, not the route", meta["page"])
	}
	if meta["route_path"] != "/orgs/acme/agents/a1/configure" {
		t.Errorf("route_path = %v", meta["route_path"])
	}
}

// TestBuildConsoleActionsSanitizesURIs: page and Referer are caller-supplied,
// and a query string is where identifiers and typed input hide.
func TestBuildConsoleActionsSanitizesURIs(t *testing.T) {
	base := testConsoleContext()
	base.URI = "https://console.example.com/search?q=secret-agent#frag"

	inputs := []ConsoleActionInput{
		{Action: "amp.console.navigation.page-view", Page: "/orgs/acme/traces?selectedTrace=abc123"},
		{Action: "amp.console.navigation.page-view"},
	}

	actions, _ := BuildConsoleActions(inputs, base, consoleTestConfig())

	if got := actions[0].Request.URI; got != "/orgs/acme/traces" {
		t.Errorf("page URI = %q, want the query stripped", got)
	}
	if got := actions[0].Metadata["route_path"]; got != "/orgs/acme/traces" {
		t.Errorf("route_path = %v, want the query stripped", got)
	}
	if got := actions[1].Request.URI; got != "https://console.example.com/search" {
		t.Errorf("referer URI = %q, want the query and fragment stripped", got)
	}
}

// TestBuildConsoleActionsBoundsCallerSuppliedLengths stops an oversized URI or
// session token reaching the collector, which the generated types do not check.
func TestBuildConsoleActionsBoundsCallerSuppliedLengths(t *testing.T) {
	inputs := []ConsoleActionInput{{
		Action:    "amp.console.navigation.page-view",
		Page:      "/" + strings.Repeat("p", maxURILength*2),
		SessionID: strings.Repeat("s", maxSessionIDLength*2),
	}}

	actions, _ := BuildConsoleActions(inputs, testConsoleContext(), consoleTestConfig())

	if len(actions[0].Request.URI) != maxURILength {
		t.Errorf("URI length = %d, want %d", len(actions[0].Request.URI), maxURILength)
	}
	if len(actions[0].SessionToken) != maxSessionIDLength {
		t.Errorf("session token length = %d, want %d", len(actions[0].SessionToken), maxSessionIDLength)
	}
}

// TestBuildConsoleActionsIdentityComesFromServer: the payload must not be able
// to claim a different user or company.
func TestBuildConsoleActionsIdentityComesFromServer(t *testing.T) {
	inputs := []ConsoleActionInput{{
		Action:    "amp.console.session.start",
		SessionID: "sess-42",
	}}

	actions, _ := BuildConsoleActions(inputs, testConsoleContext(), consoleTestConfig())
	a := actions[0]

	if a.UserID != "user-1" || a.CompanyID != "ou-1" {
		t.Errorf("identity = %q/%q, want user-1/ou-1", a.UserID, a.CompanyID)
	}
	if a.SessionToken != "sess-42" {
		t.Errorf("session token = %q", a.SessionToken)
	}
	if a.Request.IPAddress != "203.0.113.7" || a.Request.UserAgentString != "Mozilla/5.0" {
		t.Errorf("request context = %+v", a.Request)
	}
}

// TestBuildConsoleActionsTimeFallback: an absent browser timestamp falls back
// to receipt time, a present one is preserved so within-flush ordering holds.
func TestBuildConsoleActionsTimeFallback(t *testing.T) {
	stamped := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	inputs := []ConsoleActionInput{
		{Action: "amp.console.navigation.page-view", OccurredAt: stamped},
		{Action: "amp.console.navigation.page-view"},
	}

	actions, _ := BuildConsoleActions(inputs, testConsoleContext(), consoleTestConfig())

	if got := actions[0].Request.Time; got != stamped.Format(time.RFC3339) {
		t.Errorf("stamped action time = %q, want %q", got, stamped.Format(time.RFC3339))
	}
	if actions[1].Request.Time == "" {
		t.Error("unstamped action got no fallback time")
	}
}

// TestBuildConsoleActionsPageOverridesURI keeps each action pinned to the page
// it happened on rather than to whatever page happened to trigger the flush.
func TestBuildConsoleActionsPageOverridesURI(t *testing.T) {
	inputs := []ConsoleActionInput{
		{Action: "amp.console.navigation.page-view", Page: "/orgs/acme/agents"},
		{Action: "amp.console.navigation.page-view"},
	}

	actions, _ := BuildConsoleActions(inputs, testConsoleContext(), consoleTestConfig())
	if actions[0].Request.URI != "/orgs/acme/agents" {
		t.Errorf("URI = %q, want the action's own page", actions[0].Request.URI)
	}
	if actions[1].Request.URI != "https://console.example.com/" {
		t.Errorf("URI = %q, want the batch's origin", actions[1].Request.URI)
	}
}

type fakeActionSender struct {
	mu   sync.Mutex
	got  [][]moesifcollector.Action
	done chan struct{}
	err  error
}

func (f *fakeActionSender) SendActions(_ context.Context, actions []moesifcollector.Action) error {
	f.mu.Lock()
	f.got = append(f.got, actions)
	f.mu.Unlock()
	close(f.done)
	return f.err
}

func TestReportConsoleActionsSendsAsynchronously(t *testing.T) {
	fake := &fakeActionSender{done: make(chan struct{})}
	original := newConsoleSender
	newConsoleSender = func(config.GrowthAnalyticsConfig, string) consoleActionSender { return fake }
	t.Cleanup(func() { newConsoleSender = original })

	actions, _ := BuildConsoleActions(
		[]ConsoleActionInput{{Action: "amp.console.navigation.page-view"}},
		testConsoleContext(), consoleTestConfig())

	ReportConsoleActions(context.Background(), consoleTestConfig(), "caller-jwt", actions)

	select {
	case <-fake.done:
	case <-time.After(2 * time.Second):
		t.Fatal("batch was never sent")
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.got) != 1 || len(fake.got[0]) != 1 {
		t.Fatalf("sender saw %v", fake.got)
	}
}

// TestReportConsoleActionsSurvivesCancelledRequest: the request context is
// cancelled as soon as the 202 is written, which is before the send finishes.
func TestReportConsoleActionsSurvivesCancelledRequest(t *testing.T) {
	fake := &fakeActionSender{done: make(chan struct{})}
	original := newConsoleSender
	newConsoleSender = func(config.GrowthAnalyticsConfig, string) consoleActionSender { return fake }
	t.Cleanup(func() { newConsoleSender = original })

	ctx, cancel := context.WithCancel(context.Background())
	actions, _ := BuildConsoleActions(
		[]ConsoleActionInput{{Action: "amp.console.navigation.page-view"}},
		testConsoleContext(), consoleTestConfig())

	ReportConsoleActions(ctx, consoleTestConfig(), "caller-jwt", actions)
	cancel()

	select {
	case <-fake.done:
	case <-time.After(2 * time.Second):
		t.Fatal("send was abandoned when the request context was cancelled")
	}
}

// TestReportConsoleActionsWithoutTokenDropsBatch: the route is authenticated,
// so a missing JWT is a bug, and the batch must not go out unauthenticated.
func TestReportConsoleActionsWithoutTokenDropsBatch(t *testing.T) {
	fake := &fakeActionSender{done: make(chan struct{})}
	original := newConsoleSender
	newConsoleSender = func(config.GrowthAnalyticsConfig, string) consoleActionSender { return fake }
	t.Cleanup(func() { newConsoleSender = original })

	actions, _ := BuildConsoleActions(
		[]ConsoleActionInput{{Action: "amp.console.navigation.page-view"}},
		testConsoleContext(), consoleTestConfig())

	ReportConsoleActions(context.Background(), consoleTestConfig(), "", actions)

	select {
	case <-fake.done:
		t.Fatal("batch was sent with no caller JWT")
	case <-time.After(200 * time.Millisecond):
	}
}

// TestConsoleTaxonomyIsDisjointFromEndpointCodes enforces the design rule that
// console actions never duplicate an endpoint feature code: an action exists
// here only because the API cannot observe it.
func TestConsoleTaxonomyIsDisjointFromEndpointCodes(t *testing.T) {
	for action := range consoleActions {
		if !strings.HasPrefix(action, "amp.console.") {
			t.Errorf("console action %q must live under the amp.console. namespace "+
				"so it cannot collide with an endpoint feature code", action)
		}
		if len(strings.Split(action, ".")) < 4 {
			t.Errorf("console action %q should read amp.console.<category>.<action>", action)
		}
	}
}

// TestReportConsoleActionsBoundsInFlightSends: the per-user rate limit bounds
// any one caller, but a slow collector holding every send open for the full
// timeout must not grow goroutines with traffic. A full pool sheds the batch.
func TestReportConsoleActionsBoundsInFlightSends(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, maxInFlightConsoleSends+1)

	// A private semaphore, so the slot counts asserted here cannot be skewed by
	// sends another test left in flight, and this test's own held slots cannot
	// leak into whichever test runs next.
	originalSender, originalSlots := newConsoleSender, consoleSendSlots
	testSlots := make(chan struct{}, maxInFlightConsoleSends)
	consoleSendSlots = testSlots
	newConsoleSender = func(config.GrowthAnalyticsConfig, string) consoleActionSender {
		return &blockingActionSender{started: started, release: release}
	}
	t.Cleanup(func() {
		// Unblock the senders, then wait for each to return its slot before
		// restoring the package state. Draining the channel from here instead
		// would race the owners' own releases and could block one of them.
		close(release)
		deadline := time.Now().Add(2 * time.Second)
		for len(testSlots) > 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if n := len(testSlots); n > 0 {
			t.Errorf("%d console sends still held a slot after release", n)
		}
		consoleSendSlots = originalSlots
		newConsoleSender = originalSender
	})

	actions, _ := BuildConsoleActions(
		[]ConsoleActionInput{{Action: "amp.console.navigation.page-view"}},
		testConsoleContext(), consoleTestConfig())

	// Fill every slot and wait until each send is actually in flight.
	for i := 0; i < maxInFlightConsoleSends; i++ {
		ReportConsoleActions(context.Background(), consoleTestConfig(), "caller-jwt", actions)
	}
	for i := 0; i < maxInFlightConsoleSends; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d of %d sends started", i, maxInFlightConsoleSends)
		}
	}

	// The next one has no slot, so it is dropped rather than spawning.
	ReportConsoleActions(context.Background(), consoleTestConfig(), "caller-jwt", actions)
	select {
	case <-started:
		t.Error("a send started with the pool full; in-flight work is unbounded")
	case <-time.After(200 * time.Millisecond):
	}
}

type blockingActionSender struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingActionSender) SendActions(_ context.Context, _ []moesifcollector.Action) error {
	b.started <- struct{}{}
	<-b.release
	return nil
}
