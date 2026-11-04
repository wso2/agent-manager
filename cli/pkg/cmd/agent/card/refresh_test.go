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
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
)

func refreshOpts(client func(context.Context) (*amsvc.ClientWithResponses, error), jsonMode bool) (*RefreshOptions, func() (string, string)) {
	ios, out, errOut := newTestIO(jsonMode)
	return &RefreshOptions{
			IO: ios, Client: client, Scope: baseScope(),
			Org: "acme", Proj: "triage", AgentName: "order-bot", Env: "dev",
		},
		func() (string, string) { return out.String(), errOut.String() }
}

var refreshKey = "POST " + envPath("dev", "agent-card/refresh")

func Test_runRefresh_queues(t *testing.T) {
	o, got := refreshOpts(newClient(t, map[string]route{refreshKey: {status: http.StatusAccepted}}), false)
	if err := runRefresh(context.Background(), o); err != nil {
		t.Fatalf("runRefresh: %v", err)
	}
	if _, errOut := got(); !strings.Contains(errOut, "refresh queued") {
		t.Errorf("errOut = %q", errOut)
	}
}

func Test_runRefresh_json(t *testing.T) {
	o, got := refreshOpts(newClient(t, map[string]route{refreshKey: {status: http.StatusAccepted}}), true)
	if err := runRefresh(context.Background(), o); err != nil {
		t.Fatalf("runRefresh: %v", err)
	}
	out, _ := got()
	data := decodeEnvelope(t, out)["data"].(map[string]any)
	if data["environment"] != "dev" || data["queued"] != true {
		t.Errorf("data = %v", data)
	}
}

func Test_runRefresh_passesServer404(t *testing.T) {
	const msg = "Agent is not deployed in this environment"
	o, got := refreshOpts(newClient(t, map[string]route{
		refreshKey: {status: http.StatusNotFound, body: map[string]any{"code": "NOT_FOUND", "message": msg}},
	}), true)
	if err := runRefresh(context.Background(), o); err == nil {
		t.Fatal("expected error")
	}
	out, _ := got()
	if e := decodeEnvelope(t, out)["error"].(map[string]any); e["message"] != msg {
		t.Errorf("message = %v", e["message"])
	}
}

func Test_runRefresh_rejectsBadAgentName(t *testing.T) {
	o, _ := refreshOpts(unreachableClient, true)
	o.AgentName = "a/b"
	err := runRefresh(context.Background(), o)
	var ce clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != clierr.InvalidFlag {
		t.Fatalf("err = %v, want INVALID_FLAG", err)
	}
}
