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

import "testing"

// Returns the test specs for tools registered by registerObservabilityTools.
// New tools added to observability.go must have a spec here — registration_test.go fails the build otherwise.
func observabilityToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "get_runtime_logs",
			toolset:             "observability",
			descriptionKeywords: []string{"log"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name"},
			optionalParams:      []string{"org_name", "environment", "start_time", "end_time", "limit", "sort_order", "log_levels", "search_phrase"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"agent_name":   testAgentName,
			},
			expectedMethod: "GetRuntimeLogs",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
				if got, want := args[1], testProjectName; got != want {
					t.Errorf("projectName: got %v, want %q", got, want)
				}
				if got, want := args[2], testAgentName; got != want {
					t.Errorf("agentName: got %v, want %q", got, want)
				}
			},
		},
		{
			name:                "get_metrics",
			toolset:             "observability",
			descriptionKeywords: []string{"metric"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name"},
			optionalParams:      []string{"org_name", "environment", "start_time", "end_time"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"agent_name":   testAgentName,
			},
			expectedMethod: "GetMetrics",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
				if got, want := args[1], testProjectName; got != want {
					t.Errorf("projectName: got %v, want %q", got, want)
				}
				if got, want := args[2], testAgentName; got != want {
					t.Errorf("agentName: got %v, want %q", got, want)
				}
			},
		},
		{
			name:                "list_traces",
			toolset:             "observability",
			descriptionKeywords: []string{"trace"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name"},
			optionalParams:      []string{"org_name", "environment", "start_time", "end_time", "limit", "offset", "sort_order", "include_io"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"agent_name":   testAgentName,
			},
			expectedMethod: "ListTraces",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
				if got, want := args[1], testProjectName; got != want {
					t.Errorf("projectName: got %v, want %q", got, want)
				}
				if got, want := args[2], testAgentName; got != want {
					t.Errorf("agentName: got %v, want %q", got, want)
				}
			},
		},
		{
			name:                "get_traces",
			toolset:             "observability",
			descriptionKeywords: []string{"trace", "span"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name"},
			optionalParams:      []string{"org_name", "environment", "start_time", "end_time", "limit", "offset", "sort_order"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"agent_name":   testAgentName,
			},
			expectedMethod: "ExportTraces",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
				if got, want := args[1], testProjectName; got != want {
					t.Errorf("projectName: got %v, want %q", got, want)
				}
				if got, want := args[2], testAgentName; got != want {
					t.Errorf("agentName: got %v, want %q", got, want)
				}
			},
		},
		{
			name:                "get_trace_details",
			toolset:             "observability",
			descriptionKeywords: []string{"trace", "span"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name", "trace_id"},
			optionalParams:      []string{"org_name", "environment"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"agent_name":   testAgentName,
				"trace_id":     "test-trace-id",
			},
			expectedMethod: "GetTraceDetails",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
				if got, want := args[1], testProjectName; got != want {
					t.Errorf("projectName: got %v, want %q", got, want)
				}
				if got, want := args[2], testAgentName; got != want {
					t.Errorf("agentName: got %v, want %q", got, want)
				}
				if got, want := args[3], "test-trace-id"; got != want {
					t.Errorf("traceID: got %v, want %q", got, want)
				}
			},
		},
		{
			name:                "get_span_details",
			toolset:             "observability",
			descriptionKeywords: []string{"span"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name", "trace_id", "span_id"},
			optionalParams:      []string{"org_name", "environment"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"agent_name":   testAgentName,
				"trace_id":     "test-trace-id",
				"span_id":      "test-span-id",
			},
			expectedMethod: "GetSpanDetails",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
				if got, want := args[1], testProjectName; got != want {
					t.Errorf("projectName: got %v, want %q", got, want)
				}
				if got, want := args[2], testAgentName; got != want {
					t.Errorf("agentName: got %v, want %q", got, want)
				}
				if got, want := args[3], "test-trace-id"; got != want {
					t.Errorf("traceID: got %v, want %q", got, want)
				}
				if got, want := args[4], "test-span-id"; got != want {
					t.Errorf("spanID: got %v, want %q", got, want)
				}
			},
		},
	}
}
