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

package shadowagents

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// ---- evidence catalog ----

// TestEvidenceCatalog pins the evidence codes: one per agent listing, in
// display order, each with its platform, a name and the exact condition.
func TestEvidenceCatalog(t *testing.T) {
	want := []string{
		SigOpenAIAgentObject,
		SigAnthropicManagedAgent,
		SigBedrockAgentCoreRuntime, SigBedrockRegistryAgent, SigBedrockClassicAgent,
		SigGoogleAgentEngine, SigGoogleRegistryAgent,
		SigFoundryAgentObject, SigFoundryClassicAgent, SigFoundryPossibleDupOf,
	}
	providers := map[string]bool{}
	for _, p := range models.ShadowProviders {
		providers[p] = true
	}
	defs := Defs()
	got := make([]string, 0, len(defs))
	seen := map[string]bool{}
	for _, d := range defs {
		got = append(got, d.Code)
		assert.False(t, seen[d.Code], "duplicate code %s", d.Code)
		seen[d.Code] = true
		assert.True(t, providers[d.Provider], d.Code)
		assert.True(t, strings.HasPrefix(d.Code, d.Provider+"."), "code %s is prefixed by its provider", d.Code)
		assert.NotEmpty(t, d.Name, d.Code)
		assert.NotEmpty(t, d.Condition, d.Code)
		looked, ok := Def(d.Code)
		require.True(t, ok, d.Code)
		assert.Equal(t, d, looked)
	}
	assert.Equal(t, want, got, "the catalog, in display order")
	_, ok := Def("openai.plain_usage")
	assert.False(t, ok, "usage evidence is gone")

	// Defs returns a copy.
	defs[0].Name = "changed"
	d, _ := Def(defs[0].Code)
	assert.NotEqual(t, "changed", d.Name)
}

// forbiddenText are the sources a scan no longer reads and the features that
// are gone; no form, endpoint or permission text of an agent platform may
// mention them ("retires 31 March 2027", a platform date, is fine).
var forbiddenText = regexp.MustCompile(
	`(?i)deep inspection|\busage\b|cloudtrail|cloudwatch|\blogs?\b|monitoring|\bscores?\b|verdict|\bretire(ment|d)?\b|onboard`)

func TestCatalogIsConsistent(t *testing.T) {
	providers := map[string]bool{}
	for _, p := range models.ShadowProviders {
		providers[p] = true
	}
	for _, p := range Providers() {
		assert.True(t, providers[p.Provider], p.Provider)
		assert.NotEmpty(t, p.Endpoints, p.Provider)
		assert.NotEmpty(t, p.Permissions, p.Provider)
		fields := map[string]FieldSpec{}
		for _, f := range p.Fields {
			assert.NotContains(t, fields, f.Key, "%s: duplicate field %s", p.Provider, f.Key)
			fields[f.Key] = f
		}
		for _, e := range p.Endpoints {
			require.NoError(t, ValidateEndpoint(e, e.Default), "%s %s", p.Provider, e.Key)
			assert.True(t, IsOfficialHost(e, e.Default), "%s %s default is official", p.Provider, e.Key)
			if e.Option != "" {
				f, ok := fields[OptionKey(e.Option)]
				if assert.True(t, ok, "%s %s: option %s is a field", p.Provider, e.Key, e.Option) && e.Option != "roleArn" {
					assert.Equal(t, FieldCheckbox, f.Kind, "%s %s: option %s is a checkbox", p.Provider, e.Key, e.Option)
				}
			}
			if e.FillFrom != "" {
				f, ok := fields[e.FillFrom]
				if assert.True(t, ok, "%s %s: fillFrom %s is a field", p.Provider, e.Key, e.FillFrom) {
					assert.Equal(t, FieldList, f.Kind, "%s %s: fillFrom is a list", p.Provider, e.Key)
				}
			}
		}
		for _, f := range p.Fields {
			if f.Option != "" {
				o, ok := fields[OptionKey(f.Option)]
				if assert.True(t, ok, "%s %s: option %s is a field", p.Provider, f.Key, f.Option) {
					assert.Equal(t, FieldCheckbox, o.Kind, "%s %s: option is a checkbox", p.Provider, f.Key)
				}
			}
		}
		for _, perm := range p.Permissions {
			if perm.Policy != "" {
				assert.True(t, json.Valid([]byte(perm.Policy)), "%s policy is JSON", p.Provider)
			}
		}
		// The console renders a heading before the first field of a section,
		// so every field after it must belong to that section.
		section := ""
		for _, f := range p.Fields {
			if section != "" {
				assert.Equal(t, section, f.Section, "%s: field %s after section %q", p.Provider, f.Key, section)
			}
			if f.Section != "" {
				section = f.Section
			}
		}
		if p.SourceHost {
			continue
		}
		texts := []string{p.Description}
		for _, f := range p.Fields {
			texts = append(texts, f.Label, f.Help, f.Placeholder)
		}
		for _, e := range p.Endpoints {
			texts = append(texts, e.Label)
		}
		for _, perm := range p.Permissions {
			texts = append(texts, perm.Title, perm.Summary, perm.Note, perm.Policy)
			texts = append(texts, perm.Reads...)
			texts = append(texts, perm.Items...)
		}
		for _, s := range texts {
			assert.False(t, forbiddenText.MatchString(s), "%s: %q mentions a source no longer read", p.Provider, s)
		}
	}
}

// TestAgentListingCredentials pins who lists the agents: a project or
// workspace key, required, and nothing else (an Admin key cannot list agents).
func TestAgentListingCredentials(t *testing.T) {
	for _, provider := range []string{models.ShadowProviderOpenAI, models.ShadowProviderAnthropic} {
		p, ok := Provider(provider)
		require.True(t, ok)
		scoped, ok := p.Field(FieldScopedKeys)
		require.True(t, ok, provider)
		assert.Equal(t, FieldKeyMap, scoped.Kind, provider)
		assert.True(t, scoped.Required, "%s: the keys that list agents are required", provider)
		require.Len(t, p.Fields, 1, "%s: the keys that list agents are the only field", provider)
		_, admin := p.Field("apiKey")
		assert.False(t, admin, "%s: no Admin key, it cannot list agents", provider)
		for _, perm := range p.Permissions {
			assert.NotContains(t, perm.Title, "Admin", provider)
		}
		for _, cp := range providerCapabilities[provider] {
			assert.Equal(t, []string{FieldScopedKeys}, cp.Needs, "%s %s", provider, cp.Key)
		}
		_, hasChoice := CredentialChoiceOf(provider)
		assert.False(t, hasChoice, provider)
		_, deep := p.Field("deepInspection")
		assert.False(t, deep, provider)
	}
}

func TestBedrockServicesAndPolicy(t *testing.T) {
	p, ok := Provider(models.ShadowProviderBedrock)
	require.True(t, ok)
	var services []string
	for _, f := range p.Fields {
		if f.Section == SectionServices {
			services = append(services, f.Key)
			assert.True(t, IsServiceOption(f.Key), f.Key)
			assert.True(t, f.DefaultBool, f.Key)
		}
	}
	assert.Equal(t, []string{OptBedrockAgentCore, OptBedrockRegistry, OptBedrockClassicAgents}, services)
	var endpoints []string
	for _, e := range p.Endpoints {
		endpoints = append(endpoints, e.Key)
	}
	assert.Equal(t, []string{EPBedrockSTS, EPBedrockAgentCore, EPBedrockRegistry, EPBedrockAgent}, endpoints)

	var actions []string
	for _, perm := range p.Permissions {
		if perm.Policy == "" {
			continue
		}
		var doc struct {
			Statement []struct {
				Action []string `json:"Action"`
			} `json:"Statement"`
		}
		require.NoError(t, json.Unmarshal([]byte(perm.Policy), &doc))
		for _, st := range doc.Statement {
			actions = append(actions, st.Action...)
		}
	}
	assert.ElementsMatch(t, []string{
		"bedrock-agentcore:ListAgentRuntimes", "bedrock-agentcore:GetAgentRuntime",
		"agent-registry:ListRegistries", "agent-registry:ListRegistryRecords",
		"bedrock:ListAgents", "bedrock:GetAgent", "bedrock:ListAgentActionGroups",
	}, actions)
	choice, ok := CredentialChoiceOf(models.ShadowProviderBedrock)
	require.True(t, ok)
	assert.Equal(t, services, choice.Fields)
}

func TestFoundryRoleListsAgentsOnly(t *testing.T) {
	p, ok := Provider(models.ShadowProviderFoundry)
	require.True(t, ok)
	var role struct {
		Actions     []string `json:"Actions"`
		DataActions []string `json:"DataActions"`
	}
	for _, perm := range p.Permissions {
		if perm.Policy != "" {
			require.NoError(t, json.Unmarshal([]byte(perm.Policy), &role))
		}
	}
	assert.ElementsMatch(t, []string{
		"Microsoft.Resources/subscriptions/read",
		"Microsoft.CognitiveServices/accounts/read",
		"Microsoft.CognitiveServices/accounts/projects/read",
	}, role.Actions)
	assert.Equal(t, []string{"Microsoft.CognitiveServices/accounts/AIServices/agents/read"}, role.DataActions)
	for _, a := range append(role.Actions, role.DataActions...) {
		assert.NotContains(t, strings.ToLower(a), "metrics")
		assert.NotContains(t, strings.ToLower(a), "deployments")
		assert.NotContains(t, strings.ToLower(a), "responses")
	}
}

func TestGoogleRegistryIsTheOnlyService(t *testing.T) {
	p, ok := Provider(models.ShadowProviderGoogle)
	require.True(t, ok)
	var services []string
	for _, f := range p.Fields {
		if f.Section == SectionServices {
			services = append(services, f.Key)
		}
	}
	assert.Equal(t, []string{OptGoogleRegistry}, services)
	assert.True(t, IsServiceOption(OptGoogleRegistry))
	for _, e := range p.Endpoints {
		assert.NotContains(t, []string{"monitoring", "logging"}, e.Key)
	}
}

// TestCapabilitiesAreConsistent checks what a scan lists against the
// catalog: every capability's evidence exists and belongs to the provider,
// every need is a field or option of the provider, every evidence code the
// provider can produce is covered by some capability, and credential
// choices name optional service options.
func TestCapabilitiesAreConsistent(t *testing.T) {
	for _, p := range Providers() {
		caps := Capabilities(p.Provider)
		require.NotEmpty(t, caps, p.Provider)
		covered := map[string]bool{}
		keys := map[string]bool{}
		for _, c := range caps {
			assert.NotEmpty(t, c.Title, c.Key)
			assert.NotEmpty(t, c.Description, c.Key)
			assert.False(t, keys[c.Key], "duplicate capability key %s", c.Key)
			keys[c.Key] = true
			for _, code := range c.Signals {
				d, ok := Def(code)
				if assert.True(t, ok, "%s: unknown evidence %s", c.Key, code) {
					assert.Equal(t, p.Provider, d.Provider, "%s: evidence %s of another provider", c.Key, code)
				}
				covered[code] = true
			}
			for _, need := range c.Needs {
				_, ok := p.Field(need)
				assert.True(t, ok, "%s: need %q is not a field of %s", c.Key, need, p.Provider)
			}
			if !p.SourceHost {
				assert.False(t, forbiddenText.MatchString(c.Title+" "+c.Description), "%s mentions a source no longer read", c.Key)
			}
		}
		for _, d := range Defs() {
			if d.Provider == p.Provider {
				assert.True(t, covered[d.Code], "%s: evidence %s is in no capability", p.Provider, d.Code)
			}
		}
		if choice, ok := CredentialChoiceOf(p.Provider); ok {
			assert.NotEmpty(t, choice.Message)
			for _, k := range choice.Fields {
				f, ok := p.Field(k)
				if assert.True(t, ok, "%s: credential choice field %s", p.Provider, k) {
					assert.False(t, f.Required, "%s: a credential choice field must be optional", k)
					assert.Equal(t, FieldCheckbox, f.Kind, k)
					assert.True(t, IsServiceOption(k), "%s: a checkbox in a choice must be a service option", k)
				}
			}
		}
	}
}

func TestSanitizeAttributesAllowList(t *testing.T) {
	in := map[string]any{
		"model":        "gpt-4.1",
		"instructions": "SECRET-PROMPT-MARKER you are a bot",
		"system":       "SECRET-PROMPT-MARKER",
		"env":          map[string]string{"OPENAI_API_KEY": "sk-test-SECRETSECRETSECRET"},
		"tools":        []ToolRef{{Type: "function", Name: "lookup"}},
		"mcpServers":   []MCPServerRef{{Label: "github", Origin: "https://user:pw@mcp.example.com/sk-test-SECRETSECRETSECRET/sse?token=abc"}},
		"labels":       map[string]string{"team": "payments"},
		"description":  strings.Repeat("x", 1000),
		"unsupported":  struct{ A int }{1},
		"toolCount":    3,
		"models":       []string{"gpt-4.1"},
		"sessionCount": 4,
		"apiSources":   []string{"agents_api"},
	}
	out := SanitizeAttributes(in)
	b, err := json.Marshal(out)
	require.NoError(t, err)
	s := string(b)
	assert.NotContains(t, s, "SECRET-PROMPT-MARKER")
	assert.NotContains(t, s, "SECRETSECRET")
	assert.NotContains(t, s, "token=abc")
	assert.NotContains(t, s, "user:pw")
	assert.NotContains(t, out, "instructions")
	assert.NotContains(t, out, "env")
	assert.Equal(t, []MCPServerRef{{Label: "github", Origin: "https://mcp.example.com"}}, out["mcpServers"])
	assert.Len(t, out["description"], maxAttrString)
	assert.Equal(t, 3, out["toolCount"])
	for _, k := range []string{"models", "sessionCount", "apiSources"} {
		assert.NotContains(t, out, k, "%s is no longer kept", k)
	}
}

// TestAllowedAttributes filters stored attributes an earlier version kept.
func TestAllowedAttributes(t *testing.T) {
	out := AllowedAttributes(map[string]any{
		"model": "gpt-4.1", "framework": "google-adk", "models": []any{"x"}, "sessionCount": 3.0,
		"lastSessionAt": "2026-11-01", "invocationLogging": true, "deepInspection": true, "principalType": "IAMUser",
	})
	assert.Equal(t, map[string]any{"model": "gpt-4.1", "framework": "google-adk"}, out)
	assert.Empty(t, AllowedAttributes(nil))
}

func TestOriginAndRepositoryURL(t *testing.T) {
	assert.Equal(t, "https://h.example.com:8443", Origin("https://u:p@h.example.com:8443/a/b?x=1#f"))
	assert.Equal(t, "", Origin("not a url"))
	assert.Equal(t, "https://github.com/acme/agent", RepositoryURL("https://token@github.com/acme/agent/?ref=main"))
	assert.Equal(t, "", RepositoryURL("git@github.com:acme/agent.git"))
}

func TestScrubTextRemovesCredentials(t *testing.T) {
	msg := "invalid key sk-proj-ABCDEFGH12345678 Authorization: Bearer abc.def.ghi " +
		"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig AKIAABCDEFGHIJKLMNOP url?api_key=topsecret&x=1"
	out := ScrubText(msg)
	for _, secret := range []string{"sk-proj-ABCDEFGH12345678", "abc.def.ghi", "eyJhbGciOiJIUzI1NiJ9", "AKIAABCDEFGHIJKLMNOP", "topsecret"} {
		assert.NotContains(t, out, secret)
	}
	assert.LessOrEqual(t, len([]rune(ScrubText(strings.Repeat("a ", 400)))), maxMessageChars)
}

func TestSecretHint(t *testing.T) {
	assert.Equal(t, "", SecretHint(""))
	assert.Equal(t, "••••", SecretHint("short"))
	assert.Equal(t, "sk-a…wxyz", SecretHint("sk-admin-1234567890wxyz"))
}

func TestMinimiseOwnerHint(t *testing.T) {
	h := MinimiseOwnerHint(models.ShadowOwnerHint{Name: "  Alice ", Email: strings.Repeat("a", 400) + "@x.com"})
	assert.Equal(t, "Alice", h.Name)
	assert.LessOrEqual(t, len(h.Email), 254)
}

// ---- endpoints ----

func TestValidateEndpoint(t *testing.T) {
	spec := EndpointSpec{Key: "x", Label: "X", Placeholders: []string{"region"}, OfficialHostSuffixes: []string{".amazonaws.com"}}
	require.NoError(t, ValidateEndpoint(spec, "https://svc.{region}.amazonaws.com"))
	require.NoError(t, ValidateEndpoint(spec, "http://localhost:9000"))
	for _, bad := range []string{"", "ftp://x.com", "https://u:p@x.com", "https://x.com?a=1", "https://x.com#f", "https://{location}.x.com", "notaurl"} {
		assert.Error(t, ValidateEndpoint(spec, bad), bad)
	}
	assert.True(t, IsOfficialHost(spec, "https://svc.{region}.amazonaws.com"))
	assert.False(t, IsOfficialHost(spec, "https://amazonaws.com.evil.example"))
	assert.False(t, IsOfficialHost(spec, "http://svc.us-east-1.amazonaws.com"), "official endpoints use https")
	assert.False(t, IsOfficialHost(spec, "https://localhost:9000"))
}

func TestExpand(t *testing.T) {
	got, err := Expand("https://{location}-aiplatform.googleapis.com/", map[string]string{"location": "us-central1"})
	require.NoError(t, err)
	assert.Equal(t, "https://us-central1-aiplatform.googleapis.com", got)
	_, err = Expand("https://svc.{region}.amazonaws.com", map[string]string{})
	assert.Error(t, err)
}

// ---- HTTP client ----

func newTestAPI(srv *httptest.Server) *API {
	return &API{HTTP: srv.Client(), Sleep: func(context.Context, time.Duration) error { return nil }}
}

func TestAPIRetriesOn429WithRetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"slow down"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	var out struct{ OK bool }
	require.NoError(t, newTestAPI(srv).GetJSON(context.Background(), srv.URL+"/x", nil, &out))
	assert.True(t, out.OK)
	assert.Equal(t, int32(2), calls.Load())
}

func TestAPIGivesUpOnLongRetryAfter(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	err := newTestAPI(srv).GetJSON(context.Background(), srv.URL, nil, nil)
	assert.Equal(t, models.ShadowCallRateLimited, OutcomeOf(err))
}

func TestAPIClassifiesErrorsAndScrubsMessages(t *testing.T) {
	tests := []struct {
		status  int
		body    string
		outcome string
	}{
		{401, `{"error":{"message":"Incorrect API key provided: sk-proj-ABCDEFGH12345678","type":"invalid_request_error"}}`, models.ShadowCallAuthFailed},
		{403, `{"error":"Your key does not have the api.usage.read scope"}`, models.ShadowCallPermissionDenied},
		{404, `{"type":"error","error":{"type":"not_found_error","message":"Not found"}}`, models.ShadowCallNotFound},
		{400, `{"error":{"code":"InvalidFilter","message":"bad filter"}}`, models.ShadowCallFailed},
	}
	for _, tt := range tests {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tt.status)
			_, _ = w.Write([]byte(tt.body))
		}))
		err := newTestAPI(srv).GetJSON(context.Background(), srv.URL, nil, nil)
		srv.Close()
		assert.Equal(t, tt.outcome, OutcomeOf(err), tt.body)
		assert.NotContains(t, MessageOf(err), "sk-proj-ABCDEFGH12345678")
	}
}

func TestAPIMalformedJSONIsUnsupported(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data": "not-a-list"`))
	}))
	defer srv.Close()
	var out struct{ Data []string }
	err := newTestAPI(srv).GetJSON(context.Background(), srv.URL, nil, &out)
	assert.Equal(t, models.ShadowCallUnsupported, OutcomeOf(err))
}

func TestHTTPClientRefusesCrossHostRedirect(t *testing.T) {
	var leaked atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked.Store(true)
		}
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusFound)
	}))
	defer srv.Close()
	api := &API{HTTP: NewHTTPClient(nil), Header: func(h http.Header) { h.Set("Authorization", "Bearer secret") }}
	_, err := api.Do(context.Background(), Request{Method: http.MethodGet, URL: srv.URL})
	require.Error(t, err, "a 302 to another host is not followed and is not a success")
	assert.False(t, leaked.Load(), "the credential never reached the other host")
}

func TestEncodeQueryKeepsArrayBrackets(t *testing.T) {
	q := map[string][]string{"group_by[]": {"api_key_id", "model"}, "filter": {"a[b]"}}
	assert.Equal(t, "filter=a%5Bb%5D&group_by[]=api_key_id&group_by[]=model", encodeQuery(q))
}

func TestAuthorizeErrorIsAuthFailed(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	api := newTestAPI(srv)
	api.Authorize = func(context.Context, *http.Request, []byte) error { return errors.New("token endpoint refused") }
	err := api.GetJSON(context.Background(), srv.URL, nil, nil)
	assert.Equal(t, models.ShadowCallAuthFailed, OutcomeOf(err))
}

// ---- calls, findings, usage ----

func TestCallsMergeScopesWorstOutcome(t *testing.T) {
	c := NewCalls()
	c.Record("GET /x", 2, nil)
	c.Record("GET /x", 3, &CallError{Outcome: models.ShadowCallPermissionDenied, Status: 403, Message: "no"})
	c.Record("GET /x", 1, nil)
	c.Skip("GET /y", ReasonServiceOff)
	res := c.Results()
	require.Len(t, res, 2)
	assert.Equal(t, models.ShadowCallPermissionDenied, res[0].Outcome)
	assert.Equal(t, 6, res[0].Items)
	assert.Contains(t, res[0].Message, "1 of 3 scopes failed")
	assert.Equal(t, models.ShadowCallSkipped, res[1].Outcome)
	assert.False(t, c.OK("GET /x"))
}

func TestFindingsIndex(t *testing.T) {
	fs := NewFindings()
	a := fs.Get(models.ShadowKindRuntime, "arn:a")
	b := fs.Get(models.ShadowKindPlatformAgent, "agent_b")
	c := fs.Get(models.ShadowKindPlatformAgent, "agent_c")
	assert.Same(t, a, fs.Get(models.ShadowKindRuntime, "arn:a"), "Get returns the same finding")
	assert.NotSame(t, a, fs.Get(models.ShadowKindPlatformAgent, "arn:a"), "the kind is part of the key")
	got, ok := fs.Lookup(models.ShadowKindPlatformAgent, "agent_b")
	assert.True(t, ok)
	assert.Same(t, b, got)
	_, ok = fs.Lookup(models.ShadowKindPlatformAgent, "missing")
	assert.False(t, ok)
	assert.Equal(t, models.ShadowKindRuntime, a.Kind)
	assert.Equal(t, "arn:a", a.ExternalID)
	assert.NotNil(t, a.Attributes)

	fs.Remove(models.ShadowKindPlatformAgent, "agent_b")
	fs.Remove(models.ShadowKindPlatformAgent, "never-there")
	all := fs.All()
	require.Len(t, all, 3)
	assert.Same(t, a, all[0])
	assert.Same(t, c, all[1], "first-seen order is kept after a removal")
	_, ok = fs.Lookup(models.ShadowKindPlatformAgent, "agent_b")
	assert.False(t, ok)
	all[0] = nil
	assert.Same(t, a, fs.All()[0], "All returns a copy")
}

func TestAddSignalMergesSameCodeAndSource(t *testing.T) {
	f := NewFindings().Get(models.ShadowKindRuntime, "arn:a")
	f.AddSignal(Signal{Code: SigBedrockAgentCoreRuntime, Source: "ListAgentRuntimes", Observed: map[string]any{"status": "READY", "n": int64(1)}})
	f.AddSignal(Signal{Code: SigBedrockAgentCoreRuntime, Source: "ListAgentRuntimes", Observed: map[string]any{"status": "UPDATING", "n": 2}})
	f.AddSignal(Signal{Code: SigBedrockAgentCoreRuntime, Source: "another call"})
	f.AddSignal(Signal{Code: SigBedrockRegistryAgent, Source: "ListRegistryRecords"})
	require.Len(t, f.Signals, 3)
	assert.Equal(t, "READY", f.Signals[0].Observed["status"], "a label keeps its first value")
	assert.Equal(t, int64(3), f.Signals[0].Observed["n"], "counts are summed")
	assert.NotNil(t, f.Signals[1].Observed, "observed is never nil")
	assert.Equal(t, SigBedrockRegistryAgent, f.Signals[2].Code)
}
