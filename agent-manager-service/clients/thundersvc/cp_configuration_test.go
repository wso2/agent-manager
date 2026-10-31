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
	"testing"
)

type testCPAuthorizer func(context.Context, EnvironmentIdentityTarget, *http.Request) error

func (f testCPAuthorizer) Authorize(ctx context.Context, target EnvironmentIdentityTarget, req *http.Request) error {
	return f(ctx, target, req)
}

func cpTestTarget(serverURL string) EnvironmentIdentityTarget {
	return EnvironmentIdentityTarget{
		Mode: IdentityManagementCPManaged, OUID: "org-1", OrgNamespace: "namespace-1",
		Environment: "dev", DeploymentID: "org-1", GatewayID: "gateway-dev", ManagementURL: serverURL,
		DefaultEnvironment: true,
	}
}

func bearerAuthorizer(token string) testCPAuthorizer {
	return func(_ context.Context, _ EnvironmentIdentityTarget, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

func TestCPConfigurationClientActivateCapturesAppliesAndVerifies(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer management-token" || r.Header.Get("X-Deployment-Id") != "org-1" {
			t.Errorf("request has incorrect management authorization or deployment scope")
		}
		switch r.URL.Path {
		case "/configuration-versions":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["current"] != true {
				t.Errorf("capture must keep the configuration current: %v, %v", body, err)
			}
			_, _ = w.Write([]byte(`{"version":"version-1"}`))
		case "/gateways/gateway-dev/apply":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["version"] != "version-1" {
				t.Errorf("apply must name the captured version: %v, %v", body, err)
			}
			if _, hasSelection := body["selection"]; hasSelection {
				t.Errorf("apply must keep the gateway's standing selection, got %v", body)
			}
			if _, confirms := body["confirmRemovals"]; confirms {
				t.Errorf("apply must never confirm removals, got %v", body)
			}
			_, _ = w.Write([]byte(`{"missing":null}`))
		case "/gateways/gateway-dev/applied-version":
			_, _ = w.Write([]byte(`{"gatewayId":"gateway-dev","appliedVersion":"version-1"}`))
		default:
			t.Errorf("unexpected CP path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewCPConfigurationClient(bearerAuthorizer("management-token"))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Activate(context.Background(), cpTestTarget(server.URL), "AgentID update"); err != nil {
		t.Fatal(err)
	}
	want := []string{"POST /configuration-versions", "POST /gateways/gateway-dev/apply", "GET /gateways/gateway-dev/applied-version"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("CP calls = %v, want %v", calls, want)
	}
}

func TestCPConfigurationClientRejectsUnsafeTargets(t *testing.T) {
	client, err := NewCPConfigurationClient(bearerAuthorizer("management-token"))
	if err != nil {
		t.Fatal(err)
	}
	target := cpTestTarget("https://user:password@cp.example.com")
	if err := client.Activate(context.Background(), target, ""); err == nil {
		t.Fatal("management URL with embedded credentials should be rejected")
	}
	target = cpTestTarget("https://cp.example.com")
	target.GatewayID = ""
	if err := client.Activate(context.Background(), target, ""); err == nil {
		t.Fatal("missing exact gateway should be rejected")
	}
	if _, err := NewCPConfigurationClient(nil); err == nil {
		t.Fatal("a missing authorizer should be rejected")
	}
}

func TestCPConfigurationClientFailsClosedAndRedactsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("secret-content"))
	}))
	defer server.Close()
	target := cpTestTarget(server.URL)
	client, err := NewCPConfigurationClient(bearerAuthorizer("management-token"))
	if err != nil {
		t.Fatal(err)
	}
	err = client.Activate(context.Background(), target, "")
	if err == nil || !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "secret-content") {
		t.Fatalf("expected redacted HTTP 403, got %v", err)
	}

	client, err = NewCPConfigurationClient(testCPAuthorizer(func(context.Context, EnvironmentIdentityTarget, *http.Request) error {
		return errors.New("authorization unavailable")
	}))
	if err != nil {
		t.Fatal(err)
	}
	err = client.Activate(context.Background(), target, "")
	if err == nil || !strings.Contains(err.Error(), "authorization unavailable") {
		t.Fatalf("expected authorization failure, got %v", err)
	}
}

func TestCPConfigurationClientRejectsIncompleteActivation(t *testing.T) {
	tests := []struct {
		name, applyBody, appliedVersion string
	}{
		{name: "missing values", applyBody: `{"missing":{"secrets":["AGENT_SECRET"]}}`, appliedVersion: "version-1"},
		{name: "wrong applied version", applyBody: `{}`, appliedVersion: "version-older"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/configuration-versions":
					_, _ = w.Write([]byte(`{"version":"version-1"}`))
				case "/gateways/gateway-dev/apply":
					_, _ = w.Write([]byte(test.applyBody))
				case "/gateways/gateway-dev/applied-version":
					_, _ = w.Write([]byte(`{"gatewayId":"gateway-dev","appliedVersion":"` + test.appliedVersion + `"}`))
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
			}))
			defer server.Close()
			client, err := NewCPConfigurationClient(bearerAuthorizer("management-token"))
			if err != nil {
				t.Fatal(err)
			}
			if err := client.Activate(context.Background(), cpTestTarget(server.URL), ""); err == nil {
				t.Fatal("incomplete activation must fail")
			}
		})
	}
}

func TestCPConfigurationClientRejectsAuthorizerThatRedirectsOrDropsScope(t *testing.T) {
	target := cpTestTarget("https://cp.example.com")
	redirecting := testCPAuthorizer(func(_ context.Context, _ EnvironmentIdentityTarget, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer token")
		req.URL.Host = "attacker.example.com"
		return nil
	})
	rescoping := testCPAuthorizer(func(_ context.Context, _ EnvironmentIdentityTarget, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("X-Deployment-Id", "other-org")
		return nil
	})
	for name, authorizer := range map[string]testCPAuthorizer{"redirect": redirecting, "rescope": rescoping} {
		client, err := NewCPConfigurationClient(authorizer)
		if err != nil {
			t.Fatal(err)
		}
		if err := client.Activate(context.Background(), target, ""); err == nil {
			t.Fatalf("%s: authorizer tampering must be rejected", name)
		}
	}
}
