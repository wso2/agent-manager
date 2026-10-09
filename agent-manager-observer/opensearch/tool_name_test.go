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

package opensearch

import "testing"

func TestToolNameFromSpanName(t *testing.T) {
	cases := []struct {
		in       string
		wantTool string
		wantOK   bool
	}{
		{"execute_tool search_issues", "search_issues", true},
		{"search_issues.tool", "search_issues", true},
		{"execute_tool", "", true},
		{"execute_task tools", "", false},
		{"tools.task", "", false},
		{"ChatOpenAI.chat", "", false},
		{"initialize.mcp", "", false},
		{"  Execute_Tool  GetWeather ", "GetWeather", true},
		{"mcp.client.search.tools", "mcp.client.search", true},
		{"get_weather.function", "get_weather", true},
		{"tool", "", true},
		// Named after the bare tool, as OpenInference does: not recognised.
		{"search_issues", "", false},
	}
	for _, tc := range cases {
		tool, ok := ToolNameFromSpanName(tc.in)
		if tool != tc.wantTool || ok != tc.wantOK {
			t.Errorf("ToolNameFromSpanName(%q) = (%q, %t), want (%q, %t)", tc.in, tool, ok, tc.wantTool, tc.wantOK)
		}
	}
}
