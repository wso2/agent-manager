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
	"encoding/json"
	"net/http"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/clients/traceobssvc"
)

func TestExportTraces_OutputsJSON(t *testing.T) {
	ios, out, _ := newTraceTestIO(false)
	ios.JSON = true
	client, closeFn := newTraceTestClient(t, http.StatusOK, traceobssvc.TraceExportResponse{
		TotalCount: 1,
		Traces: []traceobssvc.FullTrace{
			{
				TraceOverview: traceobssvc.TraceOverview{TraceID: "abc123", SpanCount: 3},
				Spans: []traceobssvc.Span{
					{SpanID: "s1", Name: "root"},
				},
			},
		},
	})
	defer closeFn()

	err := runExportTraces(context.Background(), &ExportTracesOptions{
		IO: ios, TraceClient: client, Scope: traceBaseScope(),
		Org: "acme", Proj: "triage", AgentName: "my-agent", Env: "dev",
		StartTime: "2026-05-12T00:00:00Z", EndTime: "2026-05-13T00:00:00Z",
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("output should be valid JSON: %v\ngot: %s", err, out.String())
	}
	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data key with object value, got %v", result)
	}
	if _, ok := data["traces"]; !ok {
		t.Errorf("expected traces key in data, got %v", data)
	}
}
