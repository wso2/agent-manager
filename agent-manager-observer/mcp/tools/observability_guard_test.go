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

package tools

import (
	"context"
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/rbac"
)

// tokenWithAudience builds a throwaway HS256-signed token carrying aud. The
// tool guard re-parses without verifying the signature, so an arbitrary key
// is fine.
func tokenWithAudience(t *testing.T, aud string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"aud": aud})
	signed, err := token.SignedString([]byte("k"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func requestWithAuth(authHeader string) *gomcp.CallToolRequest {
	req := &gomcp.CallToolRequest{Extra: &gomcp.RequestExtra{Header: http.Header{}}}
	if authHeader != "" {
		req.Extra.Header.Set("Authorization", authHeader)
	}
	return req
}

// The three observability tools must reject publisher-audience tokens even
// with RBAC disabled — publishers are confined to their implicit trace-read
// permission, so the /mcp trace-tool carve-out can't be used to read
// logs/build-logs/metrics.
func TestObservabilityTools_RejectPublisherToken(t *testing.T) {
	validLogsInput := runtimeLogsInput{
		Organization: testOrgName,
		Project:      testProjectName,
		Agent:        testAgentName,
		Environment:  testEnvName,
	}
	validBuildLogsInput := buildLogsInput{Organization: testOrgName, BuildName: testBuildName}
	validMetricsInput := metricsInput{
		Organization: testOrgName,
		Project:      testProjectName,
		Agent:        testAgentName,
		Environment:  testEnvName,
	}

	tests := []struct {
		name string
		call func(fake *fakeObserverClient, req *gomcp.CallToolRequest) error
		// upstreamMethod is the fakeObserverClient method the tool would call if
		// the guard let it through; it must NOT be recorded for a publisher token.
		upstreamMethod string
	}{
		{
			name: "get_runtime_logs",
			call: func(fake *fakeObserverClient, req *gomcp.CallToolRequest) error {
				_, _, err := getRuntimeLogs(controllers.NewObservabilityController(fake), requireToolPermission(false, rbac.LogRead))(context.Background(), req, validLogsInput)
				return err
			},
			upstreamMethod: "QueryLogs",
		},
		{
			name: "get_build_logs",
			call: func(fake *fakeObserverClient, req *gomcp.CallToolRequest) error {
				_, _, err := getBuildLogs(controllers.NewObservabilityController(fake), requireToolPermission(false, rbac.BuildLogRead))(context.Background(), req, validBuildLogsInput)
				return err
			},
			upstreamMethod: "QueryLogs",
		},
		{
			name: "get_metrics",
			call: func(fake *fakeObserverClient, req *gomcp.CallToolRequest) error {
				_, _, err := getMetrics(controllers.NewObservabilityController(fake), requireToolPermission(false, rbac.MetricRead))(context.Background(), req, validMetricsInput)
				return err
			},
			upstreamMethod: "QueryMetrics",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name+" publisher token rejected", func(t *testing.T) {
			fake := newFakeObserverClient()
			req := requestWithAuth("Bearer " + tokenWithAudience(t, "amp-publisher-acme"))
			if err := tc.call(fake, req); err == nil {
				t.Fatal("expected publisher token to be rejected, got nil error")
			}
			if len(fake.calls[tc.upstreamMethod]) != 0 {
				t.Errorf("expected upstream %s not to be called for a publisher token, got %d calls",
					tc.upstreamMethod, len(fake.calls[tc.upstreamMethod]))
			}
		})

		t.Run(tc.name+" normal token passes", func(t *testing.T) {
			fake := newFakeObserverClient()
			req := requestWithAuth("Bearer " + tokenWithAudience(t, "localhost"))
			if err := tc.call(fake, req); err != nil {
				t.Fatalf("expected non-publisher token to pass, got error: %v", err)
			}
			if len(fake.calls[tc.upstreamMethod]) == 0 {
				t.Errorf("expected upstream %s to be called for a non-publisher token", tc.upstreamMethod)
			}
		})
	}
}
