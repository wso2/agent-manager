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

package google

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

func TestProvider(t *testing.T) {
	assert.Equal(t, models.ShadowProviderGoogle, New(http.DefaultClient, nil).Provider())
}

func codes(f *sa.Finding) []string {
	out := make([]string, 0, len(f.Signals))
	for _, s := range f.Signals {
		out = append(out, s.Code)
	}
	return out
}

func TestScanFindings(t *testing.T) {
	f := newFakeGoogle(t)
	c := newTestConnector(f)
	res := c.Scan(context.Background(), f.conn(t))

	require.Len(t, res.Calls, 2, "only the two agent listings are called: %+v", res.Calls)
	for _, call := range []string{callEngines, callAgents} {
		r := callResult(t, res, call)
		assert.Equal(t, models.ShadowCallOK, r.Outcome, "%s: %s", call, r.Message)
	}
	// 3 engines and 1 registry agent not on Agent Runtime. The registry
	// agents of engines A (twice: us-central1 and global) and B merge into
	// them; Google's built-in agent is left out.
	kinds := map[string]int{}
	for _, fd := range res.Findings {
		kinds[fd.Kind]++
		for _, s := range fd.Signals {
			d, ok := sa.Def(s.Code)
			if assert.True(t, ok, "%s: unknown evidence %s", fd.ExternalID, s.Code) {
				assert.Equal(t, models.ShadowProviderGoogle, d.Provider)
			}
		}
		assert.Empty(t, fd.OwnerHint, "no owner: the listings do not say who created an agent")
	}
	assert.Equal(t, map[string]int{models.ShadowKindRuntime: 3, models.ShadowKindPlatformAgent: 1}, kinds)

	t.Run("engine A: merged with its registry entry by RuntimeReference", func(t *testing.T) {
		a := findingOf(t, res, models.ShadowKindRuntime, engineA)
		assert.Equal(t, "support-agent", a.DisplayName)
		assert.Equal(t, "projects/my-agents-project/locations/us-central1", a.Scope)
		assert.Equal(t, "google-adk", a.Attributes["framework"])
		assert.Equal(t, "agents.global.org-111111111111.system.id.goog/resources/aiplatform/projects/123456789012/locations/us-central1/reasoningEngines/1234567890123456789", a.Attributes["serviceAccount"])
		assert.Equal(t, "https://github.com/example/support-agent.git", a.Attributes["repositoryUrl"])
		assert.Equal(t, map[string]string{"team": "cx"}, a.Attributes["labels"])
		assert.Equal(t, "us-central1", a.Attributes["location"])
		assert.Equal(t, testProject, a.Attributes["project"])
		assert.Equal(t, "//aiplatform.googleapis.com/projects/123456789012/locations/us-central1/reasoningEngines/1234567890123456789", a.Attributes["runtimeReference"])
		assert.Equal(t, []string{"Ticket triage"}, a.Attributes["skills"])
		assert.Equal(t, []string{"A2A_AGENT"}, a.Attributes["protocolType"])
		require.NotNil(t, a.ProviderCreatedAt)
		assert.Equal(t, time.Date(2026, 10, 1, 10, 0, 0, 123456000, time.UTC), *a.ProviderCreatedAt)
		assert.ElementsMatch(t, []string{sa.SigGoogleAgentEngine, sa.SigGoogleRegistryAgent}, codes(a))
		engine, _ := signalOf(a, sa.SigGoogleAgentEngine)
		assert.Equal(t, callEngines, engine.Source)
		reg, _ := signalOf(a, sa.SigGoogleRegistryAgent)
		assert.Equal(t, callAgents, reg.Source)
		assert.Equal(t, true, reg.Observed["a2a"])
	})

	t.Run("engine B: registry by agentId URN with the project number", func(t *testing.T) {
		b := findingOf(t, res, models.ShadowKindRuntime, engineB)
		assert.Equal(t, "orders-agent", b.DisplayName)
		assert.Equal(t, "langgraph", b.Attributes["framework"])
		assert.Equal(t, "orders-agent@my-agents-project.iam.gserviceaccount.com", b.Attributes["serviceAccount"])
		assert.NotContains(t, b.Attributes, "repositoryUrl")
		assert.Equal(t, "projects/123456789012/locations/us-central1/reasoningEngines/9876543210987654321", b.Attributes["runtimeReference"])
		assert.ElementsMatch(t, []string{sa.SigGoogleAgentEngine, sa.SigGoogleRegistryAgent}, codes(b))
	})

	t.Run("engine C: us multi-region, no registry entry", func(t *testing.T) {
		e := findingOf(t, res, models.ShadowKindRuntime, engineC)
		assert.Equal(t, "us", e.Attributes["location"])
		assert.Equal(t, "custom", e.Attributes["framework"])
		assert.Equal(t, []string{sa.SigGoogleAgentEngine}, codes(e))
	})

	t.Run("registry agent without a runtime engine", func(t *testing.T) {
		h := findingOf(t, res, models.ShadowKindPlatformAgent, "urn:agent:projects-123456789012:projects:123456789012:locations:us-central1:run:services:helpdesk-bot")
		assert.Equal(t, "helpdesk-bot", h.DisplayName)
		assert.Equal(t, []string{"A2A_AGENT"}, h.Attributes["protocolType"])
		assert.Equal(t, []string{"Answer FAQ", "ticket"}, h.Attributes["skills"])
		assert.Equal(t, "helpdesk-bot@my-agents-project.iam.gserviceaccount.com", h.Attributes["serviceAccount"])
		assert.Equal(t, "langchain", h.Attributes["framework"])
		assert.Equal(t, "2.1.0", h.Attributes["agentVersion"])
		assert.NotContains(t, h.Attributes, "runtimeReference")
		require.Len(t, h.Signals, 1)
		assert.Equal(t, sa.SigGoogleRegistryAgent, h.Signals[0].Code)
		assert.Equal(t, true, h.Signals[0].Observed["a2a"])
	})

	t.Run("every attribute key is allowed and survives sanitising", func(t *testing.T) {
		for _, fd := range res.Findings {
			clean := sa.SanitizeAttributes(fd.Attributes)
			assert.Equal(t, len(fd.Attributes), len(clean), "finding %s has a disallowed attribute: %v", fd.ExternalID, fd.Attributes)
		}
	})
}

func TestScanRequests(t *testing.T) {
	f := newFakeGoogle(t)
	c := newTestConnector(f)
	c.Scan(context.Background(), f.conn(t))

	t.Run("reasoningEngines.list: regional and multi-region hosts", func(t *testing.T) {
		paths := map[string]bool{}
		for _, r := range f.requests("engines") {
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "100", r.Query.Get("pageSize"))
			assert.Empty(t, r.Query.Get("filter"))
			assert.Equal(t, "application/json", r.Header.Get("Accept"))
			paths[r.Path] = true
		}
		assert.Equal(t, map[string]bool{
			"/regional/us-central1/v1/projects/my-agents-project/locations/us-central1/reasoningEngines": true,
			"/multiregion/us/v1/projects/my-agents-project/locations/us/reasoningEngines":                true,
			"/multiregion/eu/v1/projects/my-agents-project/locations/eu/reasoningEngines":                true,
		}, paths)
	})

	t.Run("Agent Registry: v1 agents in every location incl. global", func(t *testing.T) {
		paths := map[string]bool{}
		for _, r := range f.requests("agents") {
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "100", r.Query.Get("pageSize"))
			paths[r.Path] = true
		}
		want := map[string]bool{}
		for _, l := range []string{"us-central1", "global", "us", "eu"} {
			want["/v1/projects/my-agents-project/locations/"+l+"/agents"] = true
		}
		assert.Equal(t, want, paths)
	})

	t.Run("only the agent listings are called", func(t *testing.T) {
		for _, r := range f.requests("") {
			assert.Contains(t, []string{"engines", "agents"}, r.Route, r.Path)
		}
	})

	t.Run("one token for the whole scan", func(t *testing.T) {
		assert.Len(t, f.requests("token"), 1)
	})
}

func TestScanPagination(t *testing.T) {
	f := newFakeGoogle(t)
	c := newTestConnector(f)
	res := c.Scan(context.Background(), f.conn(t))

	tokens := func(route, loc string) []string {
		var out []string
		for _, r := range f.requests(route) {
			if strings.Contains(r.Path, "/locations/"+loc+"/") {
				out = append(out, r.Query.Get("pageToken"))
			}
		}
		return out
	}
	assert.Equal(t, []string{"", "engines-page-2"}, tokens("engines", "us-central1"))
	assert.Equal(t, []string{"", "agents-page-2"}, tokens("agents", "us-central1"))

	// Items add up over pages and scopes.
	assert.Equal(t, 3, callResult(t, res, callEngines).Items)
	assert.Equal(t, 5, callResult(t, res, callAgents).Items)
}

// TestScanPageLimitIsRecorded: a listing cut at its page bound is
// incomplete, so it is recorded as a failed call and the connection's
// listing is not complete.
func TestScanPageLimitIsRecorded(t *testing.T) {
	f := newFakeGoogle(t)
	f.setOverride("engines", func(w http.ResponseWriter, _ *http.Request) {
		serveJSON(w, `{"reasoningEngines":[],"nextPageToken":"more"}`)
	})
	c := newTestConnector(f)
	conn := f.conn(t)
	conn.Config.IncludeGlobalAndMultiRegion = false
	res := c.Scan(context.Background(), conn)
	assert.Len(t, f.requests("engines"), maxEnginePages)
	assert.Equal(t, models.ShadowCallOK, callResult(t, res, callEngines).Outcome)
	limit := callResult(t, res, callEngines+" (page limit)")
	assert.Equal(t, models.ShadowCallFailed, limit.Outcome)
	assert.Contains(t, limit.Message, "stopped after 20 pages")
}

func TestRetention(t *testing.T) {
	f := newFakeGoogle(t)
	c := newTestConnector(f)
	res := c.Scan(context.Background(), f.conn(t))
	out, err := json.Marshal(res)
	require.NoError(t, err)
	info := c.Test(context.Background(), f.conn(t))
	testOut, err := json.Marshal(info)
	require.NoError(t, err)
	for _, m := range retentionMarkers {
		assert.NotContains(t, string(out), m, "result")
		assert.NotContains(t, string(testOut), m, "test result")
		assert.NotContains(t, c.logs.String(), m, "logs")
	}
	// No query strings in logs either.
	assert.NotContains(t, c.logs.String(), "pageToken")
	assert.Contains(t, c.logs.String(), "Shadow agent discovery provider call")
}

func TestScanTokenEndpointRefusesKey(t *testing.T) {
	f := newFakeGoogle(t)
	f.setOverride("token", serveError(http.StatusBadRequest, "token_error.json"))
	c := newTestConnector(f)
	res := c.Scan(context.Background(), f.conn(t))

	first := callResult(t, res, callEngines)
	assert.Equal(t, models.ShadowCallAuthFailed, first.Outcome)
	assert.Contains(t, first.Message, "token endpoint refused")
	assert.Equal(t, models.ShadowCallSkipped, callResult(t, res, callAgents).Outcome)
	assert.Empty(t, f.requests(""), "no API call is made without a token")
	assert.Empty(t, res.Findings)
}

func TestScanInvalidServiceAccountKey(t *testing.T) {
	f := newFakeGoogle(t)
	c := newTestConnector(f)
	conn := f.conn(t)
	conn.Secret.ServiceAccountJSON = `{"type":"authorized_user"}`
	res := c.Scan(context.Background(), conn)
	r := callResult(t, res, callToken)
	assert.Equal(t, models.ShadowCallAuthFailed, r.Outcome)
	assert.Contains(t, r.Message, "not a valid JSON key file")
	assert.Empty(t, f.requests("token"))
	assert.Empty(t, f.requests(""))
}

func TestScanUnauthorizedStopsEarly(t *testing.T) {
	f := newFakeGoogle(t)
	f.setOverride("engines", serveError(http.StatusUnauthorized, "error_401.json"))
	c := newTestConnector(f)
	res := c.Scan(context.Background(), f.conn(t))
	r := callResult(t, res, callEngines)
	assert.Equal(t, models.ShadowCallAuthFailed, r.Outcome)
	assert.Contains(t, r.Message, "UNAUTHENTICATED")
	assert.Len(t, f.requests(""), 1, "the scan stops after the first 401")
	assert.Equal(t, models.ShadowCallSkipped, callResult(t, res, callAgents).Outcome)
}

func TestScanRegistryDisabledIsPermissionDenied(t *testing.T) {
	f := newFakeGoogle(t)
	f.setOverride("agents", serveError(http.StatusForbidden, "error_403_service_disabled.json"))
	c := newTestConnector(f)
	res := c.Scan(context.Background(), f.conn(t))

	agents := callResult(t, res, callAgents)
	assert.Equal(t, models.ShadowCallPermissionDenied, agents.Outcome)
	assert.Contains(t, agents.Message, "PERMISSION_DENIED")
	assert.Contains(t, agents.Message, "4 of 4 scopes failed")
	assert.Equal(t, models.ShadowCallOK, callResult(t, res, callEngines).Outcome)
	require.Len(t, res.Findings, 3)
	for _, fd := range res.Findings {
		assert.Equal(t, []string{sa.SigGoogleAgentEngine}, codes(fd), fd.ExternalID)
	}
}

func TestScanRateLimitedThenOK(t *testing.T) {
	f := newFakeGoogle(t)
	calls := 0
	f.setOverride("engines", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			serveError(http.StatusTooManyRequests, "error_429.json")(w, r)
			return
		}
		f.defaultHandler("engines")(w, r)
	})
	c := newTestConnector(f)
	conn := f.conn(t)
	conn.Config.IncludeGlobalAndMultiRegion = false
	res := c.Scan(context.Background(), conn)
	assert.Equal(t, models.ShadowCallOK, callResult(t, res, callEngines).Outcome)
	assert.Equal(t, 3, calls, "the 429 is retried, then both pages are read")
	findingOf(t, res, models.ShadowKindRuntime, engineA)
	findingOf(t, res, models.ShadowKindRuntime, engineB)
}

func TestScanMalformedJSON(t *testing.T) {
	f := newFakeGoogle(t)
	f.setOverride("engines", func(w http.ResponseWriter, _ *http.Request) { serveJSON(w, `{"reasoningEngines": [`) })
	c := newTestConnector(f)
	res := c.Scan(context.Background(), f.conn(t))
	r := callResult(t, res, callEngines)
	assert.Equal(t, models.ShadowCallUnsupported, r.Outcome)
	// The registry still runs. Without the engines, the registry agents of
	// engines A and B are platform agents of their own.
	assert.Equal(t, models.ShadowCallOK, callResult(t, res, callAgents).Outcome)
	kinds := map[string]int{}
	for _, fd := range res.Findings {
		kinds[fd.Kind]++
	}
	assert.Equal(t, map[string]int{models.ShadowKindPlatformAgent: 3}, kinds)
}

func TestScanWrongShapeIsUnsupported(t *testing.T) {
	f := newFakeGoogle(t)
	f.setOverride("agents", func(w http.ResponseWriter, _ *http.Request) { serveJSON(w, `{"agents": {"not": "a list"}}`) })
	c := newTestConnector(f)
	res := c.Scan(context.Background(), f.conn(t))
	assert.Equal(t, models.ShadowCallUnsupported, callResult(t, res, callAgents).Outcome)
	assert.Equal(t, models.ShadowCallOK, callResult(t, res, callEngines).Outcome)
}

func TestScanSettings(t *testing.T) {
	t.Run("invalid locations and projects never reach a URL", func(t *testing.T) {
		f := newFakeGoogle(t)
		c := newTestConnector(f)
		conn := f.conn(t)
		conn.Config.ProjectIDs = []string{testProject, "bad/project\"", testProject}
		conn.Config.Locations = []string{"US-Central1", "evil.example.com/x#"}
		conn.Config.IncludeGlobalAndMultiRegion = false
		res := c.Scan(context.Background(), conn)
		cfg := callResult(t, res, callConfig)
		assert.Equal(t, models.ShadowCallFailed, cfg.Outcome)
		assert.Contains(t, cfg.Message, "is not a valid location")
		assert.Contains(t, cfg.Message, "is not a valid project ID")
		for _, r := range f.requests("") {
			assert.NotContains(t, r.Path, "evil")
			assert.NotContains(t, r.Path, "bad")
		}
		assert.Len(t, f.requests("engines"), 2, "us-central1 (lower-cased, de-duplicated) only, two pages")
	})

	t.Run("no project", func(t *testing.T) {
		f := newFakeGoogle(t)
		c := newTestConnector(f)
		conn := f.conn(t)
		conn.Config.ProjectIDs = nil
		res := c.Scan(context.Background(), conn)
		assert.Equal(t, models.ShadowCallFailed, callResult(t, res, callConfig).Outcome)
		assert.Empty(t, f.requests(""))
		assert.Empty(t, f.requests("token"))
	})

	t.Run("global in locations is a registry location only", func(t *testing.T) {
		st, problems := readSettings(sa.Connection{Config: models.ShadowConnectionConfig{
			ProjectIDs: []string{"p-1"}, Locations: []string{"europe-west1", "global"},
		}})
		assert.Empty(t, problems)
		assert.Equal(t, []string{"europe-west1"}, st.runtimeLocations)
		assert.Equal(t, []string{"europe-west1", "global"}, st.registryLocations)
	})

	t.Run("multi-regions add us and eu engines and the global registry", func(t *testing.T) {
		st, problems := readSettings(sa.Connection{Config: models.ShadowConnectionConfig{
			ProjectIDs: []string{"p-1"}, Locations: []string{"europe-west1"}, IncludeGlobalAndMultiRegion: true,
		}})
		assert.Empty(t, problems)
		assert.Equal(t, []string{"europe-west1", "us", "eu"}, st.runtimeLocations)
		assert.Equal(t, []string{"europe-west1", "global", "us", "eu"}, st.registryLocations)
	})
}

func TestScanEmptyProjectRaisesNothing(t *testing.T) {
	f := newFakeGoogle(t)
	for _, route := range []string{"engines", "agents"} {
		f.setOverride(route, func(w http.ResponseWriter, _ *http.Request) { serveJSON(w, `{}`) })
	}
	c := newTestConnector(f)
	res := c.Scan(context.Background(), f.conn(t))
	assert.Empty(t, res.Findings)
	assert.Equal(t, models.ShadowCallOK, callResult(t, res, callEngines).Outcome)
	assert.Equal(t, models.ShadowCallOK, callResult(t, res, callAgents).Outcome)
}

// TestRegistryAgentResolution pins how a registry agent finds its engine:
// by RuntimeReference, else the agentId URN, else the runtime identity
// principal; with none of them it is a platform agent of its own, and
// Google's built-in agents are left out.
func TestRegistryAgentResolution(t *testing.T) {
	newScanner := func() *scanner {
		s := &scanner{c: &Connector{}, findings: sa.NewFindings(), aliases: map[string]map[string]bool{}}
		s.addEngine(testProject, "us-central1", &reasoningEngine{Name: engineA})
		return s
	}
	attr := func(field, value string) json.RawMessage {
		b, err := json.Marshal(map[string]string{field: value})
		require.NoError(t, err)
		return b
	}
	merged := func(t *testing.T, s *scanner) {
		t.Helper()
		assert.Len(t, s.findings.All(), 1, "no finding of its own")
		_, ok := signalOf(s.engines[0].finding, sa.SigGoogleRegistryAgent)
		assert.True(t, ok, "merged into the engine")
	}

	t.Run("RuntimeReference", func(t *testing.T) {
		s := newScanner()
		s.addRegistryAgent(testProject, "us-central1", &registryAgent{
			AgentID: "some-agent", Name: "projects/1/locations/us-central1/agents/a",
			Attributes: map[string]json.RawMessage{attrRuntimeReference: attr("uri", "//aiplatform.googleapis.com/"+engineA)},
		})
		merged(t, s)
	})

	t.Run("agentId URN", func(t *testing.T) {
		s := newScanner()
		s.addRegistryAgent(testProject, "us-central1", &registryAgent{
			AgentID: "urn:agent:projects-123456789012:projects:123456789012:locations:us-central1:aiplatform:reasoningEngines:1234567890123456789",
			Name:    "projects/123456789012/locations/us-central1/agents/b",
		})
		merged(t, s)
	})

	t.Run("runtime identity principal", func(t *testing.T) {
		s := newScanner()
		s.addRegistryAgent(testProject, "us-central1", &registryAgent{
			AgentID: "another-agent", Name: "projects/1/locations/us-central1/agents/c",
			Attributes: map[string]json.RawMessage{attrRuntimeIdentity: attr("principal",
				"principal://agents.global.org-1.system.id.goog/resources/aiplatform/"+engineA)},
		})
		merged(t, s)
	})

	t.Run("built-in Google agents are left out", func(t *testing.T) {
		s := newScanner()
		s.addRegistryAgent(testProject, "global", &registryAgent{AgentID: "urn:agent:googleapis.com:projects:1:locations:global:x", Name: "projects/1/locations/global/agents/x"})
		assert.Len(t, s.findings.All(), 1)
		_, ok := signalOf(s.engines[0].finding, sa.SigGoogleRegistryAgent)
		assert.False(t, ok)
	})

	t.Run("an unknown engine: a platform agent with the reference", func(t *testing.T) {
		s := newScanner()
		s.addRegistryAgent(testProject, "us-central1", &registryAgent{
			AgentID: "urn:agent:projects-1:projects:1:locations:us-central1:aiplatform:reasoningEngines:42",
			Name:    "projects/1/locations/us-central1/agents/a",
		})
		f, ok := s.findings.Lookup(models.ShadowKindPlatformAgent, "urn:agent:projects-1:projects:1:locations:us-central1:aiplatform:reasoningEngines:42")
		require.True(t, ok)
		assert.Equal(t, "projects/1/locations/us-central1/reasoningEngines/42", f.Attributes["runtimeReference"])
		assert.Equal(t, []string{sa.SigGoogleRegistryAgent}, codes(f))
	})

	t.Run("no ID at all: nothing", func(t *testing.T) {
		s := newScanner()
		s.addRegistryAgent(testProject, "us-central1", &registryAgent{})
		assert.Len(t, s.findings.All(), 1)
	})
}

func TestEngineReferences(t *testing.T) {
	ref, ok := parseEngineRef("//aiplatform.googleapis.com/projects/123/locations/us-central1/reasoningEngines/77")
	assert.True(t, ok)
	assert.Equal(t, engineRef{project: "123", location: "us-central1", id: "77"}, ref)
	ref, ok = parseEngineRef("principal://agents.global.org-1.system.id.goog/resources/aiplatform/projects/123/locations/eu/reasoningEngines/88")
	assert.True(t, ok)
	assert.Equal(t, engineRef{project: "123", location: "eu", id: "88"}, ref)
	ref, ok = parseEngineRef("projects/123/locations/us/reasoningEngines/99/operations/5")
	assert.True(t, ok)
	assert.Equal(t, "99", ref.id)
	_, ok = parseEngineRef("//run.googleapis.com/projects/123/locations/us-central1/services/x")
	assert.False(t, ok)
	ref, ok = parseEngineURN("urn:agent:projects-123:projects:123:locations:us-central1:aiplatform:reasoningEngines:77")
	assert.True(t, ok)
	assert.Equal(t, engineRef{project: "123", location: "us-central1", id: "77"}, ref)
	_, ok = parseEngineURN("urn:agent:projects-123:projects:123:locations:us-central1:run:services:x")
	assert.False(t, ok)

	t.Run("project number vs project ID", func(t *testing.T) {
		s := &scanner{findings: sa.NewFindings(), aliases: map[string]map[string]bool{}}
		s.addEngine("proj-a", "us-central1", &reasoningEngine{Name: "projects/proj-a/locations/us-central1/reasoningEngines/1"})
		s.addEngine("proj-b", "us-central1", &reasoningEngine{Name: "projects/proj-b/locations/us-central1/reasoningEngines/1"})
		// Two engines share the ID: a number with no known alias matches neither.
		assert.Nil(t, s.findEngine(engineRef{project: "555", location: "us-central1", id: "1"}))
		s.addAlias("proj-b", "555")
		assert.Equal(t, "proj-b", s.findEngine(engineRef{project: "555", location: "us-central1", id: "1"}).project)
		assert.Equal(t, "proj-a", s.findEngine(engineRef{project: "proj-a", location: "us-central1", id: "1"}).project)
		// A unique engine ID resolves even when the project is unknown.
		s.addEngine("proj-a", "europe-west1", &reasoningEngine{Name: "projects/proj-a/locations/europe-west1/reasoningEngines/2"})
		assert.NotNil(t, s.findEngine(engineRef{project: "999", location: "europe-west1", id: "2"}))
		assert.Nil(t, s.findEngine(engineRef{project: "proj-a", location: "us-east1", id: "2"}))
	})
}

func TestConnectionTest(t *testing.T) {
	t.Run("both listings", func(t *testing.T) {
		f := newFakeGoogle(t)
		c := newTestConnector(f)
		info := c.Test(context.Background(), f.conn(t))
		assert.True(t, info.OK)
		require.Len(t, info.Checks, 2)
		assert.Equal(t, checkEngines, info.Checks[0].Name)
		assert.Equal(t, []string{sa.SigGoogleAgentEngine}, info.Checks[0].Unlocks)
		assert.Equal(t, checkAgents, info.Checks[1].Name)
		assert.Equal(t, []string{sa.SigGoogleRegistryAgent}, info.Checks[1].Unlocks)
		for _, ch := range info.Checks {
			assert.Equal(t, models.ShadowCallOK, ch.Outcome, ch.Name)
		}
		for _, r := range f.requests("") {
			assert.Equal(t, http.MethodGet, r.Method)
			assert.Equal(t, "1", r.Query.Get("pageSize"), r.Path)
		}
		require.Len(t, f.requests("engines"), 1)
		assert.Contains(t, f.requests("engines")[0].Path, "/regional/us-central1/")
		require.Len(t, f.requests("agents"), 1)
		assert.Contains(t, f.requests("agents")[0].Path, "/locations/us-central1/agents")
	})

	t.Run("registry denied: not OK, the check says what is missed", func(t *testing.T) {
		f := newFakeGoogle(t)
		f.setOverride("agents", serveError(http.StatusForbidden, "error_403_service_disabled.json"))
		c := newTestConnector(f)
		info := c.Test(context.Background(), f.conn(t))
		assert.False(t, info.OK, "every selected listing must work")
		assert.Equal(t, models.ShadowCallOK, info.Checks[0].Outcome)
		assert.Equal(t, models.ShadowCallPermissionDenied, info.Checks[1].Outcome)
		assert.Equal(t, []string{sa.SigGoogleRegistryAgent}, info.Checks[1].Unlocks)
	})

	t.Run("token refused", func(t *testing.T) {
		f := newFakeGoogle(t)
		f.setOverride("token", serveError(http.StatusBadRequest, "token_error.json"))
		c := newTestConnector(f)
		info := c.Test(context.Background(), f.conn(t))
		assert.False(t, info.OK)
		require.Len(t, info.Checks, 2)
		assert.Equal(t, models.ShadowCallAuthFailed, info.Checks[0].Outcome)
		assert.Equal(t, models.ShadowCallSkipped, info.Checks[1].Outcome)
		assert.Empty(t, f.requests(""))
	})

	t.Run("engines denied: not OK", func(t *testing.T) {
		f := newFakeGoogle(t)
		f.setOverride("engines", serveError(http.StatusForbidden, "error_403_service_disabled.json"))
		c := newTestConnector(f)
		info := c.Test(context.Background(), f.conn(t))
		assert.False(t, info.OK)
		assert.Equal(t, models.ShadowCallPermissionDenied, info.Checks[0].Outcome)
		assert.Equal(t, models.ShadowCallOK, info.Checks[1].Outcome)
	})

	t.Run("invalid key and missing settings", func(t *testing.T) {
		f := newFakeGoogle(t)
		c := newTestConnector(f)
		conn := f.conn(t)
		conn.Secret.ServiceAccountJSON = "not json"
		info := c.Test(context.Background(), conn)
		assert.False(t, info.OK)
		assert.Equal(t, checkToken, info.Checks[0].Name)
		assert.Equal(t, models.ShadowCallAuthFailed, info.Checks[0].Outcome)
		assert.ElementsMatch(t, []string{sa.SigGoogleAgentEngine, sa.SigGoogleRegistryAgent}, info.Checks[0].Unlocks)

		conn = f.conn(t)
		conn.Config.Locations = nil
		conn.Config.IncludeGlobalAndMultiRegion = false
		info = c.Test(context.Background(), conn)
		assert.False(t, info.OK)
		assert.Equal(t, checkSettings, info.Checks[0].Name)
		assert.Empty(t, f.requests(""))
	})
}

func TestScanServiceAccountKeyNotKnownToGoogle(t *testing.T) {
	f := newFakeGoogle(t)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(other)
	require.NoError(t, err)
	conn := f.conn(t)
	conn.Secret.ServiceAccountJSON = keyFileJSON(t, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))

	res := newTestConnector(f).Scan(context.Background(), conn)
	assert.Equal(t, models.ShadowCallAuthFailed, callResult(t, res, callEngines).Outcome)
	assert.Len(t, f.requests("token"), 1)
	assert.Empty(t, f.requests(""))
}

// TestScanUnselectedServices: with the Agent Registry unticked, only the
// engines are listed; the registry call is recorded as "not selected".
func TestScanUnselectedServices(t *testing.T) {
	f := newFakeGoogle(t)
	c := newTestConnector(f)
	conn := f.conn(t)
	conn.Config.DisabledServices = []string{sa.OptGoogleRegistry}
	res := c.Scan(context.Background(), conn)

	assert.Empty(t, f.requests("agents"), "the registry must not be called")
	assert.NotEmpty(t, f.requests("engines"))
	r := callResult(t, res, callAgents)
	assert.Equal(t, models.ShadowCallSkipped, r.Outcome)
	assert.Equal(t, sa.ReasonServiceOff, r.Message)
	require.Len(t, res.Findings, 3)
	for _, fd := range res.Findings {
		assert.Equal(t, models.ShadowKindRuntime, fd.Kind, "only engines without the registry")
		assert.Equal(t, []string{sa.SigGoogleAgentEngine}, codes(fd), fd.ExternalID)
	}
}

func TestConnectionTestUnselectedServices(t *testing.T) {
	f := newFakeGoogle(t)
	c := newTestConnector(f)
	conn := f.conn(t)
	conn.Config.DisabledServices = []string{sa.OptGoogleRegistry}
	info := c.Test(context.Background(), conn)
	assert.True(t, info.OK, "an unticked service does not fail the test")
	require.Len(t, info.Checks, 2)
	assert.Equal(t, models.ShadowCallOK, info.Checks[0].Outcome)
	assert.Equal(t, models.ShadowCallSkipped, info.Checks[1].Outcome)
	assert.Equal(t, sa.ReasonServiceOff, info.Checks[1].Message)
	assert.Empty(t, f.requests("agents"))
}
