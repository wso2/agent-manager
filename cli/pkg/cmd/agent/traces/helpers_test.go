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

package traces

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clients/traceobssvc"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

func newTraceTestClient(t *testing.T, status int, body any) (func(context.Context) (*traceobssvc.Client, error), func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			if err := json.NewEncoder(w).Encode(body); err != nil {
				t.Errorf("encode response: %v", err)
			}
		}
	}))
	client, err := traceobssvc.NewClient(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("new client: %v", err)
	}
	return func(context.Context) (*traceobssvc.Client, error) { return client, nil }, server.Close
}

func traceBaseScope() render.Scope {
	return render.Scope{Instance: "default", Org: "acme", Project: "triage", Agent: "my-agent", Environment: "dev"}
}

func newTraceTestIO(jsonMode bool) (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	ios, _, out, errOut := iostreams.Test()
	ios.SetTerminal(!jsonMode, !jsonMode, !jsonMode)
	ios.JSON = jsonMode
	return ios, out, errOut
}

func decodeEnvelope(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("decode envelope: %v\nbody=%q", err, raw)
	}
	return m
}

// newAMTestClient returns an amsvc client whose GET /orgs/{org}/environments/{env}
// path returns the configured status.
func newAMTestClient(t *testing.T, envStatus int) (*amsvc.ClientWithResponses, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/environments/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(envStatus)
			if envStatus == http.StatusOK {
				_ = json.NewEncoder(w).Encode(amsvc.GatewayEnvironmentResponse{})
				return
			}
			_ = json.NewEncoder(w).Encode(amsvc.ErrorResponse{
				Code:    "ENVIRONMENT_NOT_FOUND",
				Message: "Environment not found",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	c, err := amsvc.NewClientWithResponses(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("amsvc client: %v", err)
	}
	return c, server.Close
}
