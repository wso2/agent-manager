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

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func TestMetrics_TextOutput(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = false
	client, _, closeFn := newTestClient(t, http.StatusOK, amsvc.MetricsResponse{
		CpuUsage:    []amsvc.MetricDataPoint{{Time: "2026-05-13T10:00:00Z", Value: 0.012}},
		CpuRequests: []amsvc.MetricDataPoint{{Time: "2026-05-13T10:00:00Z", Value: 0.1}},
		CpuLimits:   []amsvc.MetricDataPoint{{Time: "2026-05-13T10:00:00Z", Value: 0.2}},
		Memory:      []amsvc.MetricDataPoint{{Time: "2026-05-13T10:00:00Z", Value: 67108864}},
	})
	defer closeFn()

	err := runMetrics(context.Background(), &MetricsOptions{
		IO: ios, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "my-agent", Env: "dev",
		StartTime: "2026-05-12T00:00:00Z", EndTime: "2026-05-13T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(strings.ToUpper(got), "CPU") {
		t.Errorf("output should contain CPU, got %q", got)
	}
}

func TestMetrics_RejectsExternalAgent(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.JSON = false
	client, primary, closeFn := newExternalAgentClient(t)
	defer closeFn()

	err := runMetrics(context.Background(), &MetricsOptions{
		IO: ios, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "ext-agent", Env: "dev",
		StartTime: "2026-05-12T00:00:00Z", EndTime: "2026-05-13T00:00:00Z",
	})
	if err == nil {
		t.Fatal("expected error for externally-provisioned agent")
	}
	if primary.called {
		t.Errorf("metrics endpoint was called despite external agent: %s %s", primary.method, primary.path)
	}
}

func TestMetrics_JSONOutput(t *testing.T) {
	ios, out, _ := newTestIO(true)
	client, _, closeFn := newTestClient(t, http.StatusOK, amsvc.MetricsResponse{
		CpuUsage: []amsvc.MetricDataPoint{{Time: "2026-05-13T10:00:00Z", Value: 0.012}},
	})
	defer closeFn()

	err := runMetrics(context.Background(), &MetricsOptions{
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
