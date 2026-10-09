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

import (
	"os"
	"testing"
)

func TestIsMCPHandshakeSpan(t *testing.T) {
	cases := map[string]bool{
		"initialize.mcp":     true,
		" Initialize.MCP ":   true,
		"tools/list.mcp":     false,
		"search_issues.tool": false,
		"initialize":         false,
		"":                   false,
	}
	for name, want := range cases {
		if got := IsMCPHandshakeSpan(name); got != want {
			t.Errorf("IsMCPHandshakeSpan(%q) = %t, want %t", name, got, want)
		}
	}
}

// The server comes from serverInfo in the initialize output; anything else gives none.
func TestMCPServerFromHandshake(t *testing.T) {
	// The initialize.mcp output from the IT helpdesk export of 2026-09-29.
	exported, err := os.ReadFile("testdata/mcp_initialize_output.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cases := []struct {
		name  string
		attrs map[string]interface{}
		want  string
	}{
		{name: "exported output", attrs: output(string(exported)), want: "github-mcp-server"},
		{name: "truncated output", attrs: output(string(exported[:len(exported)/2]))},
		{name: "invalid JSON", attrs: output("{not json")},
		{name: "no serverInfo", attrs: output(`{"protocolVersion":"2025-11-25","capabilities":{}}`)},
		{name: "serverInfo not an object", attrs: output(`{"serverInfo":"github"}`)},
		{name: "title when name is empty", attrs: output(`{"serverInfo":{"name":" ","title":"GitHub MCP Server"}}`), want: "GitHub MCP Server"},
		{name: "name over title", attrs: output(`{"serverInfo":{"name":"jira","title":"Jira"}}`), want: "jira"},
		{name: "output not a string", attrs: map[string]interface{}{"traceloop.entity.output": map[string]interface{}{"serverInfo": "x"}}},
		{name: "JSON null", attrs: output("null")},
		{name: "no output"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MCPServerFromHandshake(tc.attrs); got != tc.want {
				t.Errorf("MCPServerFromHandshake = %q, want %q", got, tc.want)
			}
		})
	}
}

// output is span attributes with this traceloop.entity.output.
func output(s string) map[string]interface{} {
	return map[string]interface{}{"traceloop.entity.output": s}
}
