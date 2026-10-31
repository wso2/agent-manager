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

package thundersvc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func newTestCPBackend(t *testing.T, authorizer ManagementRequestAuthorizer) CPEnvironmentIdentityBackend {
	t.Helper()
	backend, err := NewCPIdentityBackend(authorizer)
	if err != nil {
		t.Fatal(err)
	}
	return backend
}

func TestCPIdentityBackendCreatesAgentThroughControlPlaneAndActivates(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/oauth2/token" {
			t.Error("CP mode must never mint a DP system token")
		}
		if r.Header.Get("Authorization") != "Bearer cp-token" || r.Header.Get("X-Deployment-Id") != "org-1" {
			t.Errorf("%s %s lacks the CP credential or deployment scope", r.Method, r.URL.Path)
		}
		switch r.URL.Path {
		case "/agents":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"agent-1","inboundAuthConfig":[{"config":{"clientId":"client-1","clientSecret":"secret-1"}}]}`))
		case "/configuration-versions":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`[{"version":"v1"}]`))
			} else {
				_, _ = w.Write([]byte(`{"version":"v1"}`))
			}
		case "/gateways/gateway-dev/apply":
			_, _ = w.Write([]byte(`{}`))
		case "/gateways/gateway-dev/applied-version":
			_, _ = w.Write([]byte(`{"gatewayId":"gateway-dev","appliedVersion":"v1"}`))
		default:
			t.Errorf("unexpected CP path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	backend := newTestCPBackend(t, bearerAuthorizer("cp-token"))
	client, err := backend.Resolve(context.Background(), cpTestTarget(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	agentID, clientID, secret, created, err := client.CreateAgentIdentity(context.Background(), "ou-1", "agent", "")
	if err != nil || agentID != "agent-1" || clientID != "client-1" || secret != "secret-1" || !created {
		t.Fatalf("CreateAgentIdentity = %q %q %q %v %v", agentID, clientID, secret, created, err)
	}
	activator, ok := client.(EnvironmentIdentityActivator)
	if !ok {
		t.Fatal("CP client must support exact-gateway activation")
	}
	if err := activator.ActivateAgentIdentity(context.Background(), agentID); err != nil {
		t.Fatal(err)
	}
	want := "POST /agents,POST /configuration-versions,POST /gateways/gateway-dev/apply,GET /gateways/gateway-dev/applied-version,GET /configuration-versions"
	if got := strings.Join(calls, ","); got != want {
		t.Fatalf("CP calls = %s, want %s", got, want)
	}
}

func TestCPIdentityBackendReusesClientPerTargetOnly(t *testing.T) {
	backend := newTestCPBackend(t, bearerAuthorizer("cp-token"))
	target := cpTestTarget("https://cp.example.com")
	first, err := backend.Resolve(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	second, err := backend.ResolveIdentity(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if first != ThunderClient(second.(*cpManagedClient)) {
		t.Fatal("one target must share one client")
	}
	other := target
	other.Environment, other.GatewayID = "stage", "gateway-stage"
	third, err := backend.Resolve(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("a different environment must not share a client")
	}
}

func TestCPIdentityBackendRejectsIncompleteTarget(t *testing.T) {
	backend := newTestCPBackend(t, bearerAuthorizer("cp-token"))
	target := cpTestTarget("https://cp.example.com")
	target.DeploymentID = ""
	if _, err := backend.Resolve(context.Background(), target); err == nil {
		t.Fatal("a target without a deployment must be rejected")
	}
	if _, err := NewCPIdentityBackend(nil); err == nil {
		t.Fatal("a missing authorizer must be rejected")
	}
}

func TestCPIdentityBackendNeverSendsOutsideManagementHost(t *testing.T) {
	var reached atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Store(true) }))
	defer other.Close()
	management := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer management.Close()

	backend := newTestCPBackend(t, bearerAuthorizer("cp-token"))
	client, err := backend.Resolve(context.Background(), cpTestTarget(management.URL))
	if err != nil {
		t.Fatal(err)
	}
	managed := client.(*cpManagedClient)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, other.URL+"/agents", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := managed.httpClient.Do(req); err == nil || reached.Load() {
		t.Fatalf("a request outside the management host must be refused before it is sent, err=%v", err)
	}
}

func TestCPIdentityBackendDoesNotSendWhenAuthorizationFails(t *testing.T) {
	var reached atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Store(true) }))
	defer server.Close()
	backend := newTestCPBackend(t, testCPAuthorizer(func(context.Context, EnvironmentIdentityTarget, *http.Request) error {
		return errors.New("authorization unavailable")
	}))
	client, err := backend.ResolveIdentity(context.Background(), cpTestTarget(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetDefaultOUID(context.Background()); err == nil || reached.Load() {
		t.Fatalf("a request must not leave without authorization, err=%v", err)
	}
}

func TestCPIdentityBackendGivesNewAgentsTheDeploymentsOwner(t *testing.T) {
	var owners []any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agents" {
			t.Errorf("unexpected CP path %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		owners = append(owners, body["owner"])
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"agent-1","inboundAuthConfig":[{"config":{"clientId":"c","clientSecret":"s"}}]}`))
	}))
	defer server.Close()
	backend := newTestCPBackend(t, bearerAuthorizer("cp-token"))

	withOwner := cpTestTarget(server.URL)
	withOwner.AgentOwnerID = "owner-everywhere"
	client, err := backend.Resolve(context.Background(), withOwner)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := client.CreateAgentIdentity(context.Background(), "ou-1", "agent", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := client.CreateAgentIdentity(context.Background(), "ou-1", "agent", "explicit-owner"); err != nil {
		t.Fatal(err)
	}

	// A target without a configured owner must fail before sending a CP request.
	plain := cpTestTarget(server.URL)
	plain.AgentOwnerID = ""
	_, err = backend.Resolve(context.Background(), plain)
	if err == nil {
		t.Fatal("a CP target without an agent owner must be rejected")
	}

	if len(owners) != 2 || owners[0] != "owner-everywhere" || owners[1] != "explicit-owner" {
		t.Fatalf("owners sent to the CP = %v, want the configured one and the explicit one", owners)
	}
}
