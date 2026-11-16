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

package bedrock

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/ratelimit"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

const (
	rtSupport   = "arn:aws:bedrock-agentcore:us-east-1:123456789012:runtime/support_agent-AbCdEf1234"
	rtOrdersMCP = "arn:aws:bedrock-agentcore:us-east-1:123456789012:runtime/orders_mcp-GhIjKl5678"
	rtBroken    = "arn:aws:bedrock-agentcore:us-east-1:123456789012:runtime/broken_agent-ZyXwVu9876"
	rtEU        = "arn:aws:bedrock-agentcore:eu-west-1:123456789012:runtime/eu_agent-QwErTy1234"
	recTravel   = "arn:aws:agent-registry:us-east-1:123456789012:registry/AbCdEf123456/record/Rec000000003"
)

type harness struct {
	fake *fakeAWS
	c    *Connector
	logs *bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	fake := newFakeAWS(t)
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c, ok := New(sa.NewHTTPClient(fake.srv.Client().Transport), logger).(*Connector)
	require.True(t, ok)
	// The SDK's standard retry policy, without the backoff wait.
	c.retryer = func() aws.Retryer {
		return retry.NewStandard(func(o *retry.StandardOptions) {
			o.MaxAttempts = 4
			o.Backoff = retry.BackoffDelayerFunc(func(int, error) (time.Duration, error) { return 0, nil })
			o.RateLimiter = ratelimit.None
		})
	}
	return &harness{fake: fake, c: c, logs: logs}
}

func (h *harness) conn(regions ...string) sa.Connection {
	if len(regions) == 0 {
		regions = []string{"us-east-1"}
	}
	eps := map[string]string{}
	for _, k := range []string{sa.EPBedrockSTS, sa.EPBedrockAgentCore, sa.EPBedrockRegistry, sa.EPBedrockAgent} {
		eps[k] = h.fake.srv.URL
	}
	return sa.Connection{
		ID: uuid.New(), OUID: "ou", Name: "aws", Provider: models.ShadowProviderBedrock,
		Config: models.ShadowConnectionConfig{Endpoints: eps, Regions: regions, AccessKeyID: testKeyID},
		Secret: models.ShadowConnectionSecret{SecretAccessKey: testSecret},
	}
}

func (h *harness) scan(t *testing.T, conn sa.Connection) *sa.Result {
	t.Helper()
	return h.c.Scan(context.Background(), conn)
}

func find(t *testing.T, res *sa.Result, kind, id string) *sa.Finding {
	t.Helper()
	for _, f := range res.Findings {
		if f.Kind == kind && f.ExternalID == id {
			return f
		}
	}
	t.Fatalf("no %s finding %q; have %v", kind, id, findingIDs(res))
	return nil
}

func findingIDs(res *sa.Result) []string {
	var out []string
	for _, f := range res.Findings {
		out = append(out, f.Kind+" "+f.ExternalID)
	}
	sort.Strings(out)
	return out
}

func signal(f *sa.Finding, code string) (sa.Signal, bool) {
	for _, s := range f.Signals {
		if s.Code == code {
			return s, true
		}
	}
	return sa.Signal{}, false
}

func codes(f *sa.Finding) []string {
	var out []string
	for _, s := range f.Signals {
		out = append(out, s.Code)
	}
	sort.Strings(out)
	return out
}

func call(res *sa.Result, name string) models.ShadowScanCallResult {
	for _, c := range res.Calls {
		if c.Call == name {
			return c
		}
	}
	return models.ShadowScanCallResult{Call: name, Outcome: "missing"}
}

// ---- the full scan ------------------------------------------------------------

func TestScanFindsTheAgents(t *testing.T) {
	h := newHarness(t)
	res := h.scan(t, h.conn())

	// Two runtimes (the MCP-server runtime is not an agent), one registry
	// record of its own, two classic agents. MCP, gateway and skill records
	// are not agents either.
	assert.Equal(t, []string{
		"platform_agent " + recTravel,
		"platform_agent " + classicAgentExternalID("us-east-1", "AB12CD34EF"),
		"platform_agent " + classicAgentExternalID("us-east-1", "XY98ZW76VU"),
		"runtime " + rtBroken,
		"runtime " + rtSupport,
	}, findingIDs(res))

	// support_agent: READY runtime, merged with its registry record.
	sup := find(t, res, models.ShadowKindRuntime, rtSupport)
	assert.Equal(t, "support_agent", sup.DisplayName)
	assert.Equal(t, "us-east-1", sup.Scope)
	assert.Equal(t, "READY", sup.State)
	assert.Equal(t, []string{sa.SigBedrockAgentCoreRuntime, sa.SigBedrockRegistryAgent}, codes(sup))
	rs, _ := signal(sup, sa.SigBedrockAgentCoreRuntime)
	assert.Equal(t, "READY", rs.Observed["status"])
	assert.Equal(t, callListRuntimes, rs.Source)
	rec, _ := signal(sup, sa.SigBedrockRegistryAgent)
	assert.Equal(t, callListRecords, rec.Source)
	assert.Equal(t, "AGENT", sup.Attributes["recordType"])
	assert.Equal(t, "arn:aws:iam::123456789012:role/AgentCoreRuntimeRole", sup.Attributes["roleArn"])
	assert.Equal(t, "HTTP", sup.Attributes["serverProtocol"])
	assert.Equal(t, "container", sup.Attributes["agentKind"])
	assert.Equal(t, "123456789012.dkr.ecr.us-east-1.amazonaws.com/support-agent:latest", sup.Attributes["image"])
	assert.Equal(t, "3", sup.Attributes["agentVersion"])
	assert.Equal(t, rtSupport, sup.Attributes["resourceId"])
	require.NotNil(t, sup.ProviderCreatedAt)
	assert.Equal(t, time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC), *sup.ProviderCreatedAt)

	// broken_agent: not READY, still an agent, its status observed.
	broken := find(t, res, models.ShadowKindRuntime, rtBroken)
	assert.Equal(t, "CREATE_FAILED", broken.State)
	bs, _ := signal(broken, sa.SigBedrockAgentCoreRuntime)
	assert.Equal(t, "CREATE_FAILED", bs.Observed["status"])
	assert.Equal(t, "A2A", broken.Attributes["serverProtocol"])

	// A registry AGENT record that names no runtime is its own agent.
	travel := find(t, res, models.ShadowKindPlatformAgent, recTravel)
	assert.Equal(t, "Travel planner", travel.DisplayName)
	assert.Equal(t, "APPROVED", travel.State)
	assert.Equal(t, []string{sa.SigBedrockRegistryAgent}, codes(travel))

	// Classic agents, with GetAgent details and both pages of action groups.
	billing := find(t, res, models.ShadowKindPlatformAgent, classicAgentExternalID("us-east-1", "AB12CD34EF"))
	assert.Equal(t, "billing-helper", billing.DisplayName)
	assert.Equal(t, "PREPARED", billing.State)
	assert.Equal(t, []string{sa.SigBedrockClassicAgent}, codes(billing))
	assert.Equal(t, "anthropic.claude-3-5-sonnet-20240620-v1:0", billing.Attributes["model"])
	assert.Equal(t, "DEFAULT", billing.Attributes["orchestrationType"])
	assert.Equal(t, "arn:aws:iam::123456789012:role/service-role/AmazonBedrockExecutionRoleForAgents_billing", billing.Attributes["roleArn"])
	assert.Equal(t, []sa.ToolRef{{Type: "action_group", Name: "lookup_invoice"}, {Type: "action_group", Name: "UserInputAction"}}, billing.Attributes["tools"])
	hr := find(t, res, models.ShadowKindPlatformAgent, classicAgentExternalID("us-east-1", "XY98ZW76VU"))
	assert.Equal(t, "NOT_PREPARED", hr.State)

	// Every call succeeded; nothing else was called.
	for _, c := range res.Calls {
		assert.Equal(t, models.ShadowCallOK, c.Outcome, c.Call)
	}
	ops := map[string]bool{}
	for _, op := range h.fake.allOps() {
		ops[op] = true
	}
	assert.Equal(t, map[string]bool{
		"ListAgentRuntimes": true, "GetAgentRuntime": true, "ListRegistries": true, "ListRegistryRecords": true,
		"ListAgents": true, "GetAgent": true, "ListAgentActionGroups": true,
	}, ops)
	for _, f := range res.Findings {
		_, ok := sa.Def(f.Signals[0].Code)
		assert.True(t, ok, "%s evidence is in the catalog", f.ExternalID)
	}
}

func TestScanRetention(t *testing.T) {
	h := newHarness(t)
	res := h.scan(t, h.conn("us-east-1", "eu-west-1"))
	b, err := json.Marshal(res)
	require.NoError(t, err)
	assert.NotContains(t, string(b), markerPrompt, "descriptions are never kept")
	assert.NotContains(t, string(b), markerKey, "environment variables are never kept")
	assert.NotContains(t, h.logs.String(), markerPrompt)
	assert.NotContains(t, h.logs.String(), testSecret)
}

func TestScanRequests(t *testing.T) {
	h := newHarness(t)
	h.scan(t, h.conn())
	f := h.fake

	// Pagination: every paginated call fetched two pages, the second with
	// the first page's token.
	pages := map[string]func(capturedReq) string{
		"ListAgentRuntimes":     func(r capturedReq) string { return r.query.Get("nextToken") },
		"ListRegistries":        jsonField("nextToken"),
		"ListRegistryRecords":   jsonField("nextToken"),
		"ListAgents":            jsonField("nextToken"),
		"ListAgentActionGroups": jsonField("nextToken"),
	}
	want := map[string]string{
		"ListAgentRuntimes": "rt-page-2", "ListRegistries": "reg-page-2", "ListRegistryRecords": "rec-page-2",
		"ListAgents": "ag-page-2", "ListAgentActionGroups": "agp-page-2",
	}
	for op, tok := range pages {
		var got []string
		for _, r := range f.requests(op) {
			got = append(got, tok(r))
		}
		assert.Contains(t, got, "", "%s first page", op)
		assert.Contains(t, got, want[op], "%s second page", op)
	}

	// restJson1: AgentCore.
	rt := f.requests("ListAgentRuntimes")[0]
	assert.Equal(t, "POST", rt.method)
	assert.Equal(t, "/runtimes/", rt.path)
	assert.Equal(t, url.Values{"maxResults": {"100"}}, rt.query)
	assert.Empty(t, rt.body, "ListAgentRuntimes has no body")
	assert.Contains(t, rt.header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential="+testKeyID+"/")
	assert.NotEmpty(t, rt.header.Get("X-Amz-Date"))
	get := f.requests("GetAgentRuntime")
	require.Len(t, get, 3, "every runtime is read, the MCP one included (that is how it is recognised)")
	assert.Equal(t, "GET", get[0].method)
	assert.Equal(t, "/runtimes/support_agent-AbCdEf1234/", get[0].path)
	assert.Empty(t, get[0].query)

	// restJson1: agent-registry-control (POST lists with JSON bodies).
	lr := f.requests("ListRegistries")[0]
	assert.Equal(t, "POST", lr.method)
	assert.Equal(t, "application/json", lr.header.Get("Content-Type"))
	assert.JSONEq(t, `{"maxResults":100}`, string(lr.body))
	rr := f.requests("ListRegistryRecords")[0]
	assert.Equal(t, "/registries/AbCdEf123456/records-list", rr.path)
	assert.JSONEq(t, `{"maxResults":100}`, string(rr.body))

	// restJson1: bedrock-agent.
	la := f.requests("ListAgents")[0]
	assert.Equal(t, "POST", la.method)
	assert.Equal(t, "/agents/", la.path)
	assert.Equal(t, "application/json", la.header.Get("Content-Type"))
	assert.JSONEq(t, `{"maxResults":100}`, string(la.body))
	assert.Equal(t, "/agents/AB12CD34EF/", f.requests("GetAgent")[0].path)
	ag := f.requests("ListAgentActionGroups")[0]
	assert.Equal(t, "POST", ag.method)
	assert.Equal(t, "/agents/AB12CD34EF/agentversions/DRAFT/actiongroups/", ag.path)
	assert.JSONEq(t, `{"maxResults":100}`, string(ag.body))

	// No STS call without a role; every request carried the right signing
	// name (checked by the fake).
	assert.Empty(t, f.requests("AssumeRole"))
}

func jsonField(name string) func(capturedReq) string {
	return func(r capturedReq) string {
		var m map[string]any
		_ = json.Unmarshal(r.body, &m)
		s, _ := m[name].(string)
		return s
	}
}

// A registry AGENT record about a runtime that hosts an MCP server is not an
// agent; one that names a runtime through an endpoint ARN is merged into it.
func TestScanRegistryProvenance(t *testing.T) {
	h := newHarness(t)
	h.fake.variant["ListRegistryRecords"] = "provenance"
	res := h.scan(t, h.conn())
	for _, f := range res.Findings {
		assert.NotContains(t, f.ExternalID, "Rec000000007", "the record about the MCP runtime is dropped")
		assert.NotEqual(t, rtOrdersMCP, f.ExternalID, "the MCP-server runtime is not an agent")
	}
	broken := find(t, res, models.ShadowKindRuntime, rtBroken)
	assert.Equal(t, []string{sa.SigBedrockAgentCoreRuntime, sa.SigBedrockRegistryAgent}, codes(broken),
		"merged through the runtime ID of an endpoint ARN")
	assert.Equal(t, "AGENT", broken.Attributes["recordType"])
}

func TestScanMultiRegion(t *testing.T) {
	h := newHarness(t)
	res := h.scan(t, h.conn("us-east-1", "eu-west-1"))
	eu := find(t, res, models.ShadowKindRuntime, rtEU)
	assert.Equal(t, "eu-west-1", eu.Scope)
	assert.Equal(t, "eu-west-1", eu.Attributes["region"])
	assert.Equal(t, "A2A", eu.Attributes["serverProtocol"])
	assert.True(t, len(res.Findings) == 6, "the us-east-1 agents and the eu-west-1 runtime: %v", findingIDs(res))
	regions := map[string]bool{}
	for _, r := range h.fake.requests("ListAgentRuntimes") {
		regions[r.region] = true
	}
	assert.Equal(t, map[string]bool{"us-east-1": true, "eu-west-1": true}, regions)
	// Calls in both regions merge into one result per call.
	assert.Equal(t, models.ShadowCallOK, call(res, callListRuntimes).Outcome)
}

// ---- services --------------------------------------------------------------------

// TestScanUnselectedServices: an unticked service is never called, its calls
// are recorded as "not selected", and its agents are absent.
func TestScanUnselectedServices(t *testing.T) {
	h := newHarness(t)
	conn := h.conn()
	conn.Config.DisabledServices = []string{sa.OptBedrockRegistry, sa.OptBedrockClassicAgents}
	res := h.scan(t, conn)
	for _, op := range []string{"ListRegistries", "ListRegistryRecords", "ListAgents", "GetAgent", "ListAgentActionGroups"} {
		assert.Empty(t, h.fake.requests(op), "%s must not be called", op)
	}
	for _, c := range []string{callListRegistries, callListRecords, callListAgents, callGetAgent, callListActions} {
		r := call(res, c)
		assert.Equal(t, models.ShadowCallSkipped, r.Outcome, c)
		assert.Equal(t, sa.ReasonServiceOff, r.Message, c)
	}
	assert.Equal(t, []string{"runtime " + rtBroken, "runtime " + rtSupport}, findingIDs(res))
	sup := find(t, res, models.ShadowKindRuntime, rtSupport)
	assert.Equal(t, []string{sa.SigBedrockAgentCoreRuntime}, codes(sup), "no registry evidence when the registry is off")
}

func TestScanOnlyRegistry(t *testing.T) {
	h := newHarness(t)
	conn := h.conn()
	conn.Config.DisabledServices = []string{sa.OptBedrockAgentCore, sa.OptBedrockClassicAgents}
	res := h.scan(t, conn)
	ops := map[string]bool{}
	for _, op := range h.fake.allOps() {
		ops[op] = true
	}
	assert.Equal(t, map[string]bool{"ListRegistries": true, "ListRegistryRecords": true}, ops)
	// Without the runtimes, the AGENT record about support_agent is its own
	// agent; the MCP and gateway records are not agents by their type.
	assert.Equal(t, []string{
		"platform_agent arn:aws:agent-registry:us-east-1:123456789012:registry/AbCdEf123456/record/Rec000000001",
		"platform_agent " + recTravel,
	}, findingIDs(res))
}

// ---- errors ---------------------------------------------------------------------

func TestScanAuthFailureStopsEarly(t *testing.T) {
	h := newHarness(t)
	h.fake.failWith("ListAgentRuntimes", fakeErr{status: 401, code: "UnrecognizedClientException"})
	res := h.scan(t, h.conn())
	require.Len(t, res.Calls, 1)
	assert.Equal(t, callListRuntimes, res.Calls[0].Call)
	assert.Equal(t, models.ShadowCallAuthFailed, res.Calls[0].Outcome)
	assert.Empty(t, res.Findings)
	assert.Equal(t, []string{"ListAgentRuntimes"}, h.fake.allOps())
}

func TestScanAuthFailure403Code(t *testing.T) {
	h := newHarness(t)
	h.fake.failWith("ListAgentRuntimes", fakeErr{status: 403, code: "InvalidSignatureException"})
	res := h.scan(t, h.conn())
	require.Len(t, res.Calls, 1)
	assert.Equal(t, models.ShadowCallAuthFailed, res.Calls[0].Outcome)
	assert.Contains(t, res.Calls[0].Message, "InvalidSignatureException")
}

func TestScanPermissionDeniedOnOneCall(t *testing.T) {
	h := newHarness(t)
	h.fake.failWith("ListRegistries", fakeErr{status: 403, code: "AccessDeniedException"})
	res := h.scan(t, h.conn())
	c := call(res, callListRegistries)
	assert.Equal(t, models.ShadowCallPermissionDenied, c.Outcome)
	assert.Contains(t, c.Message, "AccessDeniedException")
	assert.Equal(t, models.ShadowCallOK, call(res, callListRuntimes).Outcome)
	assert.Equal(t, models.ShadowCallOK, call(res, callListAgents).Outcome)
	assert.Len(t, res.Findings, 4, "the runtimes and classic agents are still found")
}

func TestScanGetRuntimeFails(t *testing.T) {
	h := newHarness(t)
	h.fake.failWith("GetAgentRuntime", fakeErr{status: 403, code: "AccessDeniedException"})
	res := h.scan(t, h.conn())
	assert.Equal(t, models.ShadowCallPermissionDenied, call(res, callGetRuntime).Outcome)
	sup := find(t, res, models.ShadowKindRuntime, rtSupport)
	assert.NotContains(t, sup.Attributes, "roleArn", "the details GetAgentRuntime gives are missing")
}

func TestScanThrottleThenSuccess(t *testing.T) {
	h := newHarness(t)
	h.fake.failWith("ListAgents", fakeErr{status: 429, code: "ThrottlingException", retryAfter: "0"})
	res := h.scan(t, h.conn())
	assert.Equal(t, models.ShadowCallOK, call(res, callListAgents).Outcome)
	find(t, res, models.ShadowKindPlatformAgent, classicAgentExternalID("us-east-1", "AB12CD34EF"))
	assert.Len(t, h.fake.requests("ListAgents"), 3, "one retry, then two pages")
}

func TestScanThrottleExhausted(t *testing.T) {
	h := newHarness(t)
	throttle := fakeErr{status: 429, code: "ThrottlingException"}
	h.fake.failWith("ListAgents", throttle, throttle, throttle, throttle)
	res := h.scan(t, h.conn())
	assert.Equal(t, models.ShadowCallRateLimited, call(res, callListAgents).Outcome)
}

func TestScanMalformedResponse(t *testing.T) {
	h := newHarness(t)
	h.fake.failWith("ListAgents", fakeErr{raw: `{"agentSummaries": [`})
	res := h.scan(t, h.conn())
	assert.Equal(t, models.ShadowCallUnsupported, call(res, callListAgents).Outcome)
	assert.Equal(t, models.ShadowCallOK, call(res, callListRuntimes).Outcome)
}

func TestScanAssumeRole(t *testing.T) {
	h := newHarness(t)
	conn := h.conn()
	conn.Config.RoleARN = "arn:aws:iam::123456789012:role/AmpShadowDiscovery"
	conn.Config.ExternalID = "ext-123"
	res := h.scan(t, conn)
	assert.Equal(t, models.ShadowCallOK, call(res, callListRuntimes).Outcome)
	ar := h.fake.requests("AssumeRole")
	require.NotEmpty(t, ar)
	form, err := url.ParseQuery(string(ar[0].body))
	require.NoError(t, err)
	assert.Equal(t, "AssumeRole", form.Get("Action"))
	assert.Equal(t, "2011-06-15", form.Get("Version"))
	assert.Equal(t, conn.Config.RoleARN, form.Get("RoleArn"))
	assert.Equal(t, "ext-123", form.Get("ExternalId"))
	assert.Equal(t, "wso2-agent-manager-shadow-discovery", form.Get("RoleSessionName"))
	assert.Contains(t, ar[0].header.Get("Content-Type"), "application/x-www-form-urlencoded")
	assert.Contains(t, ar[0].header.Get("Authorization"), "Credential="+testKeyID+"/")
	rt := h.fake.requests("ListAgentRuntimes")[0]
	assert.Contains(t, rt.header.Get("Authorization"), "Credential=ASIAIOSFODNN7EXAMPLE/")
	assert.Equal(t, "FwoGZXIvYXdzEXAMPLESESSIONTOKEN", rt.header.Get("X-Amz-Security-Token"))
}

func TestScanAssumeRoleDenied(t *testing.T) {
	h := newHarness(t)
	h.fake.failWith("AssumeRole", fakeErr{status: 403, code: "AccessDenied"})
	conn := h.conn()
	conn.Config.RoleARN = "arn:aws:iam::123456789012:role/AmpShadowDiscovery"
	res := h.scan(t, conn)
	require.Len(t, res.Calls, 1)
	assert.Equal(t, models.ShadowCallAuthFailed, res.Calls[0].Outcome)
	assert.Contains(t, res.Calls[0].Message, "AccessDenied")
}

func TestScanNoRegions(t *testing.T) {
	h := newHarness(t)
	conn := h.conn()
	conn.Config.Regions = []string{" "}
	res := h.scan(t, conn)
	require.Len(t, res.Calls, 1)
	assert.Equal(t, models.ShadowCallFailed, res.Calls[0].Outcome)
	assert.Empty(t, h.fake.allOps())
}

// ---- Test() -------------------------------------------------------------------

func TestConnectionTest(t *testing.T) {
	h := newHarness(t)
	info := h.c.Test(context.Background(), h.conn())
	assert.True(t, info.OK)
	require.Len(t, info.Checks, 3)
	want := map[string][]string{
		callListRuntimes:   {sa.SigBedrockAgentCoreRuntime},
		callListRegistries: {sa.SigBedrockRegistryAgent},
		callListAgents:     {sa.SigBedrockClassicAgent},
	}
	for _, c := range info.Checks {
		assert.Equal(t, models.ShadowCallOK, c.Outcome, c.Name)
		assert.Equal(t, want[c.Name], c.Unlocks, c.Name)
	}
	assert.Equal(t, "1", h.fake.requests("ListAgentRuntimes")[0].query.Get("maxResults"))
	assert.JSONEq(t, `{"maxResults":1}`, string(h.fake.requests("ListAgents")[0].body))
	assert.Empty(t, h.fake.requests("GetAgentRuntime"), "the test reads one page only")
}

func TestConnectionTestAuthFailure(t *testing.T) {
	h := newHarness(t)
	h.fake.failWith("ListAgentRuntimes", fakeErr{status: 401, code: "UnrecognizedClientException"})
	info := h.c.Test(context.Background(), h.conn())
	assert.False(t, info.OK)
	require.Len(t, info.Checks, 1, "the credential was refused: nothing else is called")
	assert.Equal(t, models.ShadowCallAuthFailed, info.Checks[0].Outcome)
	assert.Equal(t, []string{sa.SigBedrockAgentCoreRuntime}, info.Checks[0].Unlocks)
}

func TestConnectionTestPartialPermissions(t *testing.T) {
	h := newHarness(t)
	h.fake.failWith("ListRegistries", fakeErr{status: 403, code: "AccessDeniedException"})
	info := h.c.Test(context.Background(), h.conn())
	assert.False(t, info.OK, "every selected service must be listable")
	byName := map[string]models.ShadowConnectionCheck{}
	for _, c := range info.Checks {
		byName[c.Name] = c
	}
	assert.Equal(t, models.ShadowCallPermissionDenied, byName[callListRegistries].Outcome)
	assert.Equal(t, models.ShadowCallOK, byName[callListRuntimes].Outcome)
	assert.Equal(t, models.ShadowCallOK, byName[callListAgents].Outcome)
}

func TestConnectionTestUnselectedServices(t *testing.T) {
	h := newHarness(t)
	h.fake.failWith("ListRegistries", fakeErr{status: 403, code: "AccessDeniedException"})
	conn := h.conn()
	conn.Config.DisabledServices = []string{sa.OptBedrockRegistry, sa.OptBedrockClassicAgents}
	info := h.c.Test(context.Background(), conn)
	assert.True(t, info.OK, "only the selected service is checked, and it works")
	skipped := 0
	for _, c := range info.Checks {
		if c.Outcome == models.ShadowCallSkipped {
			skipped++
			assert.Equal(t, sa.ReasonServiceOff, c.Message, c.Name)
		}
	}
	assert.Equal(t, 2, skipped)
	assert.Empty(t, h.fake.requests("ListRegistries"))
	assert.Empty(t, h.fake.requests("ListAgents"))
}

func TestConnectionTestNoRegion(t *testing.T) {
	h := newHarness(t)
	conn := h.conn()
	conn.Config.Regions = nil
	info := h.c.Test(context.Background(), conn)
	assert.False(t, info.OK)
	require.Len(t, info.Checks, 1)
	assert.Equal(t, models.ShadowCallFailed, info.Checks[0].Outcome)
}

// ---- units ----------------------------------------------------------------------

func TestPaginateBound(t *testing.T) {
	n, err := paginate(context.Background(), 3, func() bool { return true }, func(context.Context) (int, error) { return 2, nil })
	assert.Equal(t, 6, n)
	require.Error(t, err)
	assert.Contains(t, sa.MessageOf(err), "stopped after 3 pages")
}

func TestAWSErrorScrubsURLs(t *testing.T) {
	err := awsError(&url.Error{Op: "Post", URL: "https://bedrock-agentcore-control.us-east-1.amazonaws.com/runtimes/?nextToken=abc123", Err: context.DeadlineExceeded})
	assert.NotContains(t, sa.MessageOf(err), "nextToken")
	assert.True(t, strings.HasPrefix(sa.OutcomeOf(err), "failed"))
}

func TestArnHelpers(t *testing.T) {
	assert.Equal(t, "broken_agent-ZyXwVu9876", arnResourceID(rtBroken+"/runtime-endpoint/DEFAULT", "runtime/"))
	assert.Equal(t, "", arnResourceID("arn:aws:iam::1:role/x", "runtime/"))
	assert.Equal(t, "eu-west-1", arnRegion(rtEU, "us-east-1"))
	assert.Equal(t, "us-east-1", arnRegion("not-an-arn", "us-east-1"))
}
