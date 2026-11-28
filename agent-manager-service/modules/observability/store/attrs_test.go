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

package store

import "testing"

func TestToolName(t *testing.T) {
	for name, want := range map[string]string{
		"execute_tool search_issues":  "search_issues",
		"  Execute_Tool  get_weather": "get_weather",
		"search_issues.tool":          "search_issues",
		"mcp.client.search.tools":     "mcp.client.search",
		"get_weather.function":        "get_weather",
		"execute_tool":                "",
		"execute_toolbox":             "",
		"openclaw.tool.execution":     "",
		"execute_task tools":          "",
		"invoke_agent x.tool":         "",
		"chat tool.tool":              "",
		"initialize.mcp":              "",
		"search_issues":               "",
		"tool":                        "",
	} {
		if got := ToolName(name); got != want {
			t.Errorf("ToolName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestIsLLMLeaf(t *testing.T) {
	for name, want := range map[string]bool{
		"chat": true, "Chat gpt-4o": true, "openai.chat": true, " ChatOpenAI.chat ": true,
		"chatbot": false, "invoke_agent chat": false, "openai.chats": false, "": false,
	} {
		if got := IsLLMLeaf(name); got != want {
			t.Errorf("IsLLMLeaf(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestMCPServer(t *testing.T) {
	for _, tc := range []struct {
		name, output, want string
	}{
		{"initialize.mcp", `{"serverInfo":{"name":" github ","title":"GitHub"}}`, "github"},
		{" Initialize.MCP ", `{"serverInfo":{"title":"GitHub"}}`, "GitHub"},
		{"initialize.mcp", `{"serverInfo":{"name":"github"`, ""},
		{"initialize.mcp", ``, ""},
		{"tools/list.mcp", `{"serverInfo":{"name":"github"}}`, ""},
	} {
		if got := (Attrs{AttrEntityOutput: tc.output}).MCPServer(tc.name); got != tc.want {
			t.Errorf("MCPServer(%q, %s) = %q, want %q", tc.name, tc.output, got, tc.want)
		}
	}
}

func TestTokens(t *testing.T) {
	for _, tc := range []struct {
		name  string
		attrs Attrs
		want  int64
	}{
		{"input and output", Attrs{"gen_ai.usage.input_tokens": "100", "gen_ai.usage.output_tokens": "50"}, 150},
		{"legacy keys", Attrs{"gen_ai.usage.prompt_tokens": "40", "gen_ai.usage.completion_tokens": "2"}, 42},
		{"a float reads its integer part", Attrs{"gen_ai.usage.input_tokens": "12.7"}, 12},
		{"the first key present wins", Attrs{"gen_ai.usage.input_tokens": "x", "gen_ai.usage.prompt_tokens": "40"}, 0},
		{"output only", Attrs{"gen_ai.usage.output_tokens": "7"}, 7},
		{"zero is no usage", Attrs{"gen_ai.usage.input_tokens": "0", "gen_ai.usage.output_tokens": "0"}, 0},
		{"no usage", Attrs{}, 0},
	} {
		if got := tc.attrs.Tokens(); got != tc.want {
			t.Errorf("%s: Tokens() = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestConversationID(t *testing.T) {
	if got := (Attrs{"session.id": "s", "gen_ai.conversation.id": "c"}).ConversationID(); got != "c" {
		t.Errorf("ConversationID = %q, want the first key's value", got)
	}
	if got := (Attrs{"gen_ai.conversation.id": "", "langsmith.metadata.thread_id": "7"}).ConversationID(); got != "7" {
		t.Errorf("ConversationID = %q, want the first non-empty value", got)
	}
}

func TestIsError(t *testing.T) {
	for _, tc := range []struct {
		name  string
		attrs Attrs
		code  string
		want  bool
	}{
		{"error.type, even empty", Attrs{"error.type": ""}, "Ok", true},
		{"tool status", Attrs{"gen_ai.tool.status": "FAILED"}, "", true},
		{"HTTP 4xx", Attrs{"http.status_code": "404"}, "", true},
		{"HTTP 3xx", Attrs{"http.status_code": "302"}, "", false},
		{"status code", Attrs{}, "Error", true},
		{"status code 2", Attrs{}, "2", true},
		{"ok", Attrs{}, "Ok", false},
	} {
		if got := tc.attrs.IsError(tc.code); got != tc.want {
			t.Errorf("%s: IsError = %v, want %v", tc.name, got, tc.want)
		}
	}
}
