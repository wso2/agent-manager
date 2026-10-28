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

package a2acard

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
)

// route describes one stubbed HTTP response keyed by "METHOD /path".
type route struct {
	status int
	body   any
	// capture, when non-nil, receives the decoded request body the handler saw.
	capture *map[string]any
}

// newClient serves routes keyed by "METHOD /path"; unrouted requests get a 500.
func newClient(t *testing.T, routes map[string]route) *amsvc.ClientWithResponses {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		rt, ok := routes[key]
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "NOT_STUBBED", "message": key})
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
	t.Cleanup(srv.Close)
	client, err := amsvc.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

func envPath(env, suffix string) string {
	return "/orgs/acme/projects/triage/agents/order-bot/environments/" + env + "/" + suffix
}
