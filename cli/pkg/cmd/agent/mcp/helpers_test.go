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

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

// clientFn is the lazy client accessor shape every command's Options expects.
type clientFn = func(context.Context) (*aigateway.ClientWithResponses, error)

func unreachableClient(context.Context) (*aigateway.ClientWithResponses, error) {
	return nil, errors.New("client should not be constructed")
}

type fakePrompter struct {
	confirmDeletionErr error
	confirmDeletionArg string
	calls              int
}

func (p *fakePrompter) ConfirmDeletion(required string) error {
	p.calls++
	p.confirmDeletionArg = required
	return p.confirmDeletionErr
}

func (p *fakePrompter) Confirm(prompt string) (bool, error) { return false, nil }

// route describes one stubbed HTTP response keyed by "METHOD /path".
type route struct {
	status       int
	body         any
	transportErr error
	// capture, when non-nil, receives the decoded request body the handler saw.
	capture *map[string]any
}

func okJSON(body any) route { return route{status: http.StatusOK, body: body} }

// newClient spins up an httptest server whose responses are keyed by
// "METHOD /path", mirroring the agent package's status_test harness.
func newClient(t *testing.T, routes map[string]route) (func(context.Context) (*aigateway.ClientWithResponses, error), func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if got := r.URL.Query().Get("project"); got != "triage" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "NO_PROJECT", "message": "project = " + got})
			return
		}
		rt, ok := routes[key]
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "NOT_STUBBED", "message": key})
			return
		}
		if rt.transportErr != nil {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatalf("response writer does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack: %v", err)
			}
			_ = conn.Close()
			return
		}
		if rt.capture != nil {
			raw, _ := io.ReadAll(r.Body)
			m := map[string]any{}
			_ = json.Unmarshal(raw, &m)
			*rt.capture = m
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.status)
		if rt.body != nil {
			_ = json.NewEncoder(w).Encode(rt.body)
		}
	}))
	client, err := aigateway.NewClientWithResponses(srv.URL)
	if err != nil {
		srv.Close()
		t.Fatalf("new client: %v", err)
	}
	return func(context.Context) (*aigateway.ClientWithResponses, error) { return client, nil }, srv.Close
}

func baseScope() render.Scope {
	return render.Scope{Instance: "default", Org: "acme", Project: "triage", Agent: "order-bot"}
}

func newTestIO(jsonMode bool) (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
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

// Path constants for the mcp-configs endpoints under the test agent.
const (
	listPath = "GET /agents/order-bot/mcp-configs"
	postPath = "POST /agents/order-bot/mcp-configs"
)

func configPath(method, uuid string) string {
	return method + " /agents/order-bot/mcp-configs/" + uuid
}

func mustUUID(s string) openapi_types.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
