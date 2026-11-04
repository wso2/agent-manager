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

package card

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

const agentPath = "/orgs/acme/projects/triage/agents/order-bot"

func envPath(env, suffix string) string {
	return agentPath + "/environments/" + env + "/" + suffix
}

func unreachableClient(context.Context) (*amsvc.ClientWithResponses, error) {
	return nil, errors.New("client should not be constructed")
}

// route describes one stubbed HTTP response keyed by "METHOD /path".
type route struct {
	status int
	body   any
}

func okJSON(body any) route { return route{status: http.StatusOK, body: body} }

// newClient serves routes keyed by "METHOD /path"; unrouted requests get a 500.
func newClient(t *testing.T, routes map[string]route) func(context.Context) (*amsvc.ClientWithResponses, error) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		rt, ok := routes[key]
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "NOT_STUBBED", "message": key})
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rt.status)
		if rt.body != nil {
			_ = json.NewEncoder(w).Encode(rt.body)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := amsvc.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return func(context.Context) (*amsvc.ClientWithResponses, error) { return client, nil }
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
