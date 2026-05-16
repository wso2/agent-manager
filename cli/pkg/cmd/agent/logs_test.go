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

package agent

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func TestLogs_TextOutput(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = false
	client, _, closeFn := newTestClient(t, http.StatusOK, amsvc.LogsResponse{
		Logs: []amsvc.LogEntry{
			{Timestamp: time.Date(2026, 5, 13, 10, 23, 1, 0, time.UTC), LogLevel: "INFO", Log: "Agent initialized"},
			{Timestamp: time.Date(2026, 5, 13, 10, 23, 5, 0, time.UTC), LogLevel: "ERROR", Log: "Failed to invoke tool"},
		},
		TotalCount: 2,
		TookMs:     12.5,
	})
	defer closeFn()

	err := runLogs(context.Background(), &LogsOptions{
		IO: ios, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "my-agent", Env: "dev",
		StartTime: "2026-05-12T00:00:00Z", EndTime: "2026-05-13T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Agent initialized") {
		t.Errorf("output should contain log message, got %q", got)
	}
	if !strings.Contains(got, "ERROR") {
		t.Errorf("output should contain log level, got %q", got)
	}
}

func TestLogs_RejectsExternalAgent(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.JSON = false
	client, primary, closeFn := newExternalAgentClient(t)
	defer closeFn()

	err := runLogs(context.Background(), &LogsOptions{
		IO: ios, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "ext-agent", Env: "dev",
		StartTime: "2026-05-12T00:00:00Z", EndTime: "2026-05-13T00:00:00Z",
	})
	if err == nil {
		t.Fatal("expected error for externally-provisioned agent")
	}
	if primary.called {
		t.Errorf("logs endpoint was called despite external agent: %s %s", primary.method, primary.path)
	}
}

func TestLogs_JSONOutput(t *testing.T) {
	ios, out, _ := newTestIO(true)
	client, _, closeFn := newTestClient(t, http.StatusOK, amsvc.LogsResponse{
		Logs:       []amsvc.LogEntry{{Timestamp: time.Now(), LogLevel: "INFO", Log: "test"}},
		TotalCount: 1,
		TookMs:     5,
	})
	defer closeFn()

	err := runLogs(context.Background(), &LogsOptions{
		IO: ios, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "my-agent", Env: "dev",
		StartTime: "2026-05-12T00:00:00Z", EndTime: "2026-05-13T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	if _, ok := env["data"]; !ok {
		t.Fatal("expected data key in JSON envelope")
	}
}
