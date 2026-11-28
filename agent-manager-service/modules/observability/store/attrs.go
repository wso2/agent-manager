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

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The span names and attributes a trace's filters read, as the observer reads
// them. Stores that filter in Go call the functions here; the ClickHouse store
// builds its SQL from the same names and keys.

// Semantic span kinds.
const (
	KindLLM       = "llm"
	KindEmbedding = "embedding"
	KindTool      = "tool"
	KindRetriever = "retriever"
	KindRerank    = "rerank"
	KindAgent     = "agent"
	KindChain     = "chain"
	KindUnknown   = "unknown"
)

// Span attributes and names the filters read.
const (
	AttrErrorType    = "error.type"
	AttrToolStatus   = "gen_ai.tool.status"
	AttrHTTPStatus   = "http.status_code"
	AttrEntityOutput = "traceloop.entity.output"
	// MCPHandshakeName is the span an MCP client opens a session with.
	MCPHandshakeName = "initialize.mcp"
	// ToolPrefix starts a tool span's name; the tool's name follows a space.
	ToolPrefix = "execute_tool"
)

var (
	// ErrorStatuses mark an error in a status code or AttrToolStatus, lowercased.
	ErrorStatuses = []string{"error", "failed", "2"}
	// InputTokenKeys and OutputTokenKeys hold a span's token usage; the first key present is read.
	InputTokenKeys  = []string{"gen_ai.usage.input_tokens", "gen_ai.usage.prompt_tokens"}
	OutputTokenKeys = []string{"gen_ai.usage.output_tokens", "gen_ai.usage.completion_tokens"}
	// ModelKeys hold an LLM span's model; the first key present is read.
	ModelKeys = []string{"gen_ai.response.model", "gen_ai.request.model"}
	// ConversationIDKeys hold a span's conversation ID; the first non-empty value is read.
	ConversationIDKeys = []string{
		"gen_ai.conversation.id",
		"session.id",
		"langfuse.session.id",
		"traceloop.association.properties.session_id",
		"traceloop.association.properties.conversation_id",
		"traceloop.association.properties.thread_id",
		"langsmith.metadata.session_id",
		"langsmith.metadata.thread_id",
	}
	// NonToolPrefixes start span names that are never tools.
	NonToolPrefixes = []string{"invoke_agent", "execute_agent", "create_agent", "execute_task", "execute_event_loop_cycle"}
	// OperationLLM and OperationEmbedding are the first word of an "{operation} {model}" span name.
	OperationLLM       = []string{"chat", "text_completion", "completion", "generate_content"}
	OperationEmbedding = []string{"embeddings", "embedding"}
	// ToolSegments end a tool span's dotted name.
	ToolSegments = []string{"tool", "tools", "function", "functions"}
)

var openClawSpanKinds = map[string]string{
	"openclaw.harness.run":       KindAgent,
	"openclaw.run":               KindAgent,
	"openclaw.model.usage":       KindLLM,
	"openclaw.model.call":        KindLLM,
	"openclaw.tool.execution":    KindTool,
	"openclaw.context.assembled": KindChain,
}

// SpanKind classifies a span by its name.
func SpanKind(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return KindUnknown
	}
	if kind, ok := openClawSpanKinds[lower]; ok {
		return kind
	}
	switch {
	case strings.HasPrefix(lower, "invoke_agent"), strings.HasPrefix(lower, "execute_agent"),
		strings.HasPrefix(lower, "create_agent"):
		return KindAgent
	case strings.HasPrefix(lower, ToolPrefix):
		return KindTool
	case strings.HasPrefix(lower, "execute_task"), strings.HasPrefix(lower, "execute_event_loop_cycle"):
		return KindChain
	}
	if i := strings.IndexByte(lower, ' '); i > 0 {
		switch {
		case slices.Contains(OperationLLM, lower[:i]):
			return KindLLM
		case slices.Contains(OperationEmbedding, lower[:i]):
			return KindEmbedding
		}
	}
	parts := strings.Split(lower, ".")
	switch last := parts[len(parts)-1]; {
	case slices.Contains([]string{"llm", "chat", "chats", "completion", "completions"}, last):
		return KindLLM
	case slices.Contains([]string{"embedding", "embeddings", "embed"}, last):
		return KindEmbedding
	case slices.Contains(ToolSegments, last):
		return KindTool
	case slices.Contains([]string{"retriever", "retrievers", "retrieve", "retrieval"}, last):
		return KindRetriever
	case slices.Contains([]string{"rerank", "reranker", "rerankers"}, last):
		return KindRerank
	case slices.Contains([]string{"agent", "agents"}, last):
		return KindAgent
	case slices.Contains([]string{"task", "tasks", "workflow", "workflows"}, last):
		return KindChain
	}
	return KindUnknown
}

// IsLLMLeaf reports whether a span name is a leaf LLM call: "chat", "chat {model}" or "{vendor}.chat".
func IsLLMLeaf(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(lower, ".chat") || lower == "chat" || strings.HasPrefix(lower, "chat ")
}

// ToolName is the tool a span's name calls, or "" when it is not a named tool span.
func ToolName(name string) string {
	if SpanKind(name) != KindTool {
		return ""
	}
	trimmed := strings.TrimSpace(name)
	if _, fixed := openClawSpanKinds[strings.ToLower(trimmed)]; fixed {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(trimmed), ToolPrefix) {
		rest := trimmed[len(ToolPrefix):]
		if !strings.HasPrefix(rest, " ") {
			return ""
		}
		return strings.TrimSpace(rest)
	}
	i := strings.LastIndexByte(trimmed, '.')
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(trimmed[:i])
}

// IsErrorCode reports whether an OTel status code is an error.
func IsErrorCode(code string) bool {
	return strings.EqualFold(code, "error")
}

// Attrs are a span's attributes as strings, as the span table stores them.
type Attrs map[string]string

// StringAttrs is m with every value formatted as a string.
func StringAttrs(m map[string]any) Attrs {
	out := make(Attrs, len(m))
	for k, v := range m {
		out[k] = fmt.Sprint(v)
	}
	return out
}

// first is the value of the first key present.
func (a Attrs) first(keys []string) (string, bool) {
	for _, k := range keys {
		if v, ok := a[k]; ok {
			return v, true
		}
	}
	return "", false
}

// IsError reports whether a span with these attributes and status code failed.
func (a Attrs) IsError(statusCode string) bool {
	if _, ok := a[AttrErrorType]; ok {
		return true
	}
	if slices.Contains(ErrorStatuses, strings.ToLower(a[AttrToolStatus])) {
		return true
	}
	if status, err := strconv.ParseFloat(a[AttrHTTPStatus], 64); err == nil && status >= 400 {
		return true
	}
	return slices.Contains(ErrorStatuses, strings.ToLower(statusCode))
}

// leadingInt matches the integer a token count starts with.
var leadingInt = regexp.MustCompile(`^\s*-?[0-9]+`)

// LeadingIntPattern is leadingInt's pattern without leading space, for SQL.
const LeadingIntPattern = `^-?[0-9]+`

// count is the integer v starts with, or 0.
func count(v string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(leadingInt.FindString(v)), 10, 64)
	return n
}

// Usage is a span's input and output tokens; both 0 when neither is positive.
func (a Attrs) Usage() (in, out int64) {
	i, _ := a.first(InputTokenKeys)
	o, _ := a.first(OutputTokenKeys)
	in, out = count(i), count(o)
	if in <= 0 && out <= 0 {
		return 0, 0
	}
	return in, out
}

// Tokens is a span's input plus output tokens; 0 when it reports neither.
func (a Attrs) Tokens() int64 {
	in, out := a.Usage()
	return in + out
}

// Model is an LLM span's model, or "".
func (a Attrs) Model() string {
	m, _ := a.first(ModelKeys)
	return m
}

// ConversationID is a span's conversation ID, or "".
func (a Attrs) ConversationID() string {
	for _, k := range ConversationIDKeys {
		if v := a[k]; v != "" {
			return v
		}
	}
	return ""
}

// MCPServer is the server an MCP handshake span names, or "" for any other span.
func (a Attrs) MCPServer(spanName string) string {
	if !strings.EqualFold(strings.TrimSpace(spanName), MCPHandshakeName) {
		return ""
	}
	var out struct {
		ServerInfo struct {
			Name  string `json:"name"`
			Title string `json:"title"`
		} `json:"serverInfo"`
	}
	if json.Unmarshal([]byte(a[AttrEntityOutput]), &out) != nil {
		return ""
	}
	if name := strings.TrimSpace(out.ServerInfo.Name); name != "" {
		return name
	}
	return strings.TrimSpace(out.ServerInfo.Title)
}
