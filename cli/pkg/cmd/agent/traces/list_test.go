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
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/clients/traceobssvc"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func TestListTraces_TextOutput(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = false
	client, closeFn := newTraceTestClient(t, http.StatusOK, traceobssvc.TraceOverviewResponse{
		TotalCount: 1,
		Traces: []traceobssvc.TraceOverview{
			{
				TraceID:      "abc123",
				RootSpanName: "handle_request",
				SpanCount:    8,
			},
		},
	})
	defer closeFn()

	err := runListTraces(context.Background(), &ListTracesOptions{
		IO: ios, TraceClient: client, Scope: traceBaseScope(),
		Org: "acme", Proj: "triage", AgentName: "my-agent", Env: "dev",
		StartTime: "2026-05-12T00:00:00Z", EndTime: "2026-05-13T00:00:00Z",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "abc123") {
		t.Errorf("output should contain trace ID, got %q", got)
	}
}
