// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
	"testing"
)

func TestParseSpans(t *testing.T) {
	response := &SearchResponse{}
	response.Hits.Hits = []struct {
		Source map[string]interface{} `json:"_source"`
	}{
		{
			Source: map[string]interface{}{
				"traceId":         "trace-1",
				"spanId":          "span-1",
				"parentSpanId":    "",
				"name":            "root-span",
				"kind":            "INTERNAL",
				"startTime":       "2025-01-15T10:00:00.000000000Z",
				"endTime":         "2025-01-15T10:00:01.000000000Z",
				"durationInNanos": float64(1000000000),
				"resource": map[string]interface{}{
					"openchoreo.dev/component-uid": "comp-1",
				},
			},
		},
		{
			Source: map[string]interface{}{
				"traceId":      "trace-1",
				"spanId":       "span-2",
				"parentSpanId": "span-1",
				"name":         "child-span",
			},
		},
	}

	spans := ParseSpans(response)
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	root := spans[0]
	if root.TraceID != "trace-1" {
		t.Errorf("expected traceId 'trace-1', got %q", root.TraceID)
	}
	if root.SpanID != "span-1" {
		t.Errorf("expected spanId 'span-1', got %q", root.SpanID)
	}
	if root.ParentSpanID != "" {
		t.Errorf("expected empty parentSpanId, got %q", root.ParentSpanID)
	}
	if root.Name != "root-span" {
		t.Errorf("expected name 'root-span', got %q", root.Name)
	}
	if root.Kind != "INTERNAL" {
		t.Errorf("expected kind 'INTERNAL', got %q", root.Kind)
	}
	if root.DurationInNanos != 1000000000 {
		t.Errorf("expected duration 1000000000, got %d", root.DurationInNanos)
	}
	if root.Service != "comp-1" {
		t.Errorf("expected service 'comp-1', got %q", root.Service)
	}

	child := spans[1]
	if child.ParentSpanID != "span-1" {
		t.Errorf("expected parentSpanId 'span-1', got %q", child.ParentSpanID)
	}
}

func TestParseSpans_Empty(t *testing.T) {
	response := &SearchResponse{}
	spans := ParseSpans(response)
	if len(spans) != 0 {
		t.Errorf("expected 0 spans, got %d", len(spans))
	}
}

func TestParseSpan_DurationFallback(t *testing.T) {
	// When durationInNanos is not present, it should calculate from timestamps
	response := &SearchResponse{}
	response.Hits.Hits = []struct {
		Source map[string]interface{} `json:"_source"`
	}{
		{
			Source: map[string]interface{}{
				"traceId":   "trace-1",
				"spanId":    "span-1",
				"name":      "test-span",
				"startTime": "2025-01-15T10:00:00.000000000Z",
				"endTime":   "2025-01-15T10:00:02.000000000Z",
			},
		},
	}

	spans := ParseSpans(response)
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	// 2 seconds = 2,000,000,000 nanoseconds
	if spans[0].DurationInNanos != 2000000000 {
		t.Errorf("expected duration 2000000000, got %d", spans[0].DurationInNanos)
	}
}

func TestParseSpan_Status(t *testing.T) {
	response := &SearchResponse{}
	response.Hits.Hits = []struct {
		Source map[string]interface{} `json:"_source"`
	}{
		{
			Source: map[string]interface{}{
				"traceId": "trace-1",
				"spanId":  "span-1",
				"name":    "test-span",
				"status": map[string]interface{}{
					"code": "OK",
				},
			},
		},
	}

	spans := ParseSpans(response)
	if spans[0].Status != "OK" {
		t.Errorf("expected status 'OK', got %q", spans[0].Status)
	}
}

func TestDetermineSpanType(t *testing.T) {
	tests := []struct {
		name     string
		span     Span
		expected SpanType
	}{
		{
			name:     "unknown with nil attributes",
			span:     Span{},
			expected: SpanTypeUnknown,
		},
		{
			name: "LLM via traceloop.span.kind",
			span: Span{
				Attributes: map[string]interface{}{
					"traceloop.span.kind": "llm",
				},
			},
			expected: SpanTypeLLM,
		},
		{
			name: "embedding via traceloop.span.kind",
			span: Span{
				Attributes: map[string]interface{}{
					"traceloop.span.kind": "embedding",
				},
			},
			expected: SpanTypeEmbedding,
		},
		{
			name: "tool via traceloop.span.kind",
			span: Span{
				Attributes: map[string]interface{}{
					"traceloop.span.kind": "tool",
				},
			},
			expected: SpanTypeTool,
		},
		{
			name: "retriever via traceloop.span.kind",
			span: Span{
				Attributes: map[string]interface{}{
					"traceloop.span.kind": "retriever",
				},
			},
			expected: SpanTypeRetriever,
		},
		{
			name: "rerank via traceloop.span.kind",
			span: Span{
				Attributes: map[string]interface{}{
					"traceloop.span.kind": "rerank",
				},
			},
			expected: SpanTypeRerank,
		},
		{
			name: "agent via traceloop.span.kind",
			span: Span{
				Attributes: map[string]interface{}{
					"traceloop.span.kind": "agent",
				},
			},
			expected: SpanTypeAgent,
		},
		{
			name: "chain via traceloop.span.kind task",
			span: Span{
				Attributes: map[string]interface{}{
					"traceloop.span.kind": "task",
				},
			},
			expected: SpanTypeChain,
		},
		{
			name: "chain via traceloop.span.kind workflow",
			span: Span{
				Attributes: map[string]interface{}{
					"traceloop.span.kind": "workflow",
				},
			},
			expected: SpanTypeChain,
		},
		{
			name: "LLM via gen_ai.operation.name chat",
			span: Span{
				Attributes: map[string]interface{}{
					"gen_ai.operation.name": "chat",
				},
			},
			expected: SpanTypeLLM,
		},
		{
			name: "embedding via gen_ai.operation.name",
			span: Span{
				Attributes: map[string]interface{}{
					"gen_ai.operation.name": "embedding",
				},
			},
			expected: SpanTypeEmbedding,
		},
		{
			name: "tool via gen_ai.tool.name",
			span: Span{
				Attributes: map[string]interface{}{
					"gen_ai.tool.name": "search",
				},
			},
			expected: SpanTypeTool,
		},
		{
			name: "agent via gen_ai.agent.name",
			span: Span{
				Attributes: map[string]interface{}{
					"gen_ai.agent.name": "my-agent",
				},
			},
			expected: SpanTypeAgent,
		},
		{
			name: "retriever via db.system chroma",
			span: Span{
				Attributes: map[string]interface{}{
					"db.system": "chroma",
				},
			},
			expected: SpanTypeRetriever,
		},
		{
			name: "rerank via gen_ai.operation.name rerank",
			span: Span{
				Attributes: map[string]interface{}{
					"gen_ai.operation.name": "rerank",
				},
			},
			expected: SpanTypeRerank,
		},
		{
			name: "crewai task via crewai.task attributes",
			span: Span{
				Attributes: map[string]interface{}{
					"crewai.task.name":    "research",
					"traceloop.span.kind": "task",
				},
			},
			expected: SpanTypeCrewAITask,
		},
		{
			name: "LLM via span name suffix",
			span: Span{
				Name: "openai.chat",
				Attributes: map[string]interface{}{
					"some.attr": "value",
				},
			},
			expected: SpanTypeLLM,
		},
		{
			name: "agent via span name suffix",
			span: Span{
				Name: "crewai.agent",
				Attributes: map[string]interface{}{
					"some.attr": "value",
				},
			},
			expected: SpanTypeAgent,
		},
		{
			name: "unknown span type",
			span: Span{
				Name: "some-operation",
				Attributes: map[string]interface{}{
					"some.attr": "value",
				},
			},
			expected: SpanTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineSpanType(tt.span)
			if got != tt.expected {
				t.Errorf("DetermineSpanType() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestExtractTokenUsage(t *testing.T) {
	t.Run("aggregates across multiple spans", func(t *testing.T) {
		spans := []Span{
			{
				Attributes: map[string]interface{}{
					"gen_ai.usage.input_tokens":  float64(100),
					"gen_ai.usage.output_tokens": float64(50),
				},
			},
			{
				Attributes: map[string]interface{}{
					"gen_ai.usage.input_tokens":  float64(200),
					"gen_ai.usage.output_tokens": float64(100),
				},
			},
		}

		usage := ExtractTokenUsage(spans)
		if usage == nil {
			t.Fatal("expected token usage, got nil")
		}
		if usage.InputTokens != 300 {
			t.Errorf("expected input tokens 300, got %d", usage.InputTokens)
		}
		if usage.OutputTokens != 150 {
			t.Errorf("expected output tokens 150, got %d", usage.OutputTokens)
		}
		if usage.TotalTokens != 450 {
			t.Errorf("expected total tokens 450, got %d", usage.TotalTokens)
		}
	})

	t.Run("returns nil when no GenAI spans", func(t *testing.T) {
		spans := []Span{
			{
				Attributes: map[string]interface{}{
					"some.other.attr": "value",
				},
			},
		}

		usage := ExtractTokenUsage(spans)
		if usage != nil {
			t.Errorf("expected nil, got %+v", usage)
		}
	})

	t.Run("returns nil for nil attributes", func(t *testing.T) {
		spans := []Span{{}}
		usage := ExtractTokenUsage(spans)
		if usage != nil {
			t.Errorf("expected nil, got %+v", usage)
		}
	})
}

func TestExtractTokenUsageFromAttributes(t *testing.T) {
	t.Run("gen_ai.usage format with float64", func(t *testing.T) {
		attrs := map[string]interface{}{
			"gen_ai.usage.input_tokens":  float64(100),
			"gen_ai.usage.output_tokens": float64(50),
		}
		usage := extractTokenUsageFromAttributes(attrs)
		if usage == nil {
			t.Fatal("expected token usage, got nil")
		}
		if usage.InputTokens != 100 {
			t.Errorf("expected 100, got %d", usage.InputTokens)
		}
		if usage.OutputTokens != 50 {
			t.Errorf("expected 50, got %d", usage.OutputTokens)
		}
		if usage.TotalTokens != 150 {
			t.Errorf("expected 150, got %d", usage.TotalTokens)
		}
	})

	t.Run("prompt_tokens format", func(t *testing.T) {
		attrs := map[string]interface{}{
			"gen_ai.usage.prompt_tokens":     float64(200),
			"gen_ai.usage.completion_tokens": float64(80),
		}
		usage := extractTokenUsageFromAttributes(attrs)
		if usage == nil {
			t.Fatal("expected token usage, got nil")
		}
		if usage.InputTokens != 200 {
			t.Errorf("expected 200, got %d", usage.InputTokens)
		}
		if usage.OutputTokens != 80 {
			t.Errorf("expected 80, got %d", usage.OutputTokens)
		}
	})

	t.Run("string token values", func(t *testing.T) {
		attrs := map[string]interface{}{
			"gen_ai.usage.input_tokens":  "300",
			"gen_ai.usage.output_tokens": "120",
		}
		usage := extractTokenUsageFromAttributes(attrs)
		if usage == nil {
			t.Fatal("expected token usage, got nil")
		}
		if usage.InputTokens != 300 {
			t.Errorf("expected 300, got %d", usage.InputTokens)
		}
	})

	t.Run("cache read tokens", func(t *testing.T) {
		attrs := map[string]interface{}{
			"gen_ai.usage.input_tokens":            float64(100),
			"gen_ai.usage.output_tokens":           float64(50),
			"gen_ai.usage.cache_read_input_tokens": float64(25),
		}
		usage := extractTokenUsageFromAttributes(attrs)
		if usage == nil {
			t.Fatal("expected token usage, got nil")
		}
		if usage.CacheReadInputTokens != 25 {
			t.Errorf("expected cache read tokens 25, got %d", usage.CacheReadInputTokens)
		}
	})

	t.Run("returns nil when no tokens found", func(t *testing.T) {
		attrs := map[string]interface{}{
			"some.other": "value",
		}
		usage := extractTokenUsageFromAttributes(attrs)
		if usage != nil {
			t.Errorf("expected nil, got %+v", usage)
		}
	})
}

func TestExtractTraceStatus(t *testing.T) {
	t.Run("counts errors from error.type attribute", func(t *testing.T) {
		spans := []Span{
			{Attributes: map[string]interface{}{"error.type": "RuntimeError"}},
			{Attributes: map[string]interface{}{"some.attr": "ok"}},
			{Attributes: map[string]interface{}{"error.type": "TimeoutError"}},
		}
		status := ExtractTraceStatus(spans)
		if status.ErrorCount != 2 {
			t.Errorf("expected error count 2, got %d", status.ErrorCount)
		}
	})

	t.Run("counts errors from span status", func(t *testing.T) {
		spans := []Span{
			{Status: "error"},
			{Status: "OK"},
		}
		status := ExtractTraceStatus(spans)
		if status.ErrorCount != 1 {
			t.Errorf("expected error count 1, got %d", status.ErrorCount)
		}
	})

	t.Run("no errors", func(t *testing.T) {
		spans := []Span{
			{Status: "OK"},
			{Attributes: map[string]interface{}{"some.attr": "value"}},
		}
		status := ExtractTraceStatus(spans)
		if status.ErrorCount != 0 {
			t.Errorf("expected error count 0, got %d", status.ErrorCount)
		}
	})
}

func TestExtractPromptMessages(t *testing.T) {
	t.Run("OTEL format gen_ai.input.messages", func(t *testing.T) {
		attrs := map[string]interface{}{
			"gen_ai.input.messages": `[{"role":"system","content":"You are helpful"},{"role":"user","content":"Hello"}]`,
		}
		messages := ExtractPromptMessages(attrs)
		if len(messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(messages))
		}
		if messages[0].Role != "system" {
			t.Errorf("expected role 'system', got %q", messages[0].Role)
		}
		if messages[0].Content != "You are helpful" {
			t.Errorf("expected content 'You are helpful', got %q", messages[0].Content)
		}
		if messages[1].Role != "user" {
			t.Errorf("expected role 'user', got %q", messages[1].Role)
		}
	})

	t.Run("Traceloop format gen_ai.prompt.*", func(t *testing.T) {
		attrs := map[string]interface{}{
			"gen_ai.prompt.0.role":    "system",
			"gen_ai.prompt.0.content": "You are a bot",
			"gen_ai.prompt.1.role":    "user",
			"gen_ai.prompt.1.content": "Hi there",
		}
		messages := ExtractPromptMessages(attrs)
		if len(messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(messages))
		}
		if messages[0].Role != "system" {
			t.Errorf("expected 'system', got %q", messages[0].Role)
		}
		if messages[1].Content != "Hi there" {
			t.Errorf("expected 'Hi there', got %q", messages[1].Content)
		}
	})
}

func TestExtractCompletionMessages(t *testing.T) {
	t.Run("OTEL format gen_ai.output.messages", func(t *testing.T) {
		attrs := map[string]interface{}{
			"gen_ai.output.messages": `[{"role":"assistant","content":"Hello! How can I help?"}]`,
		}
		messages := ExtractCompletionMessages(attrs)
		if len(messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(messages))
		}
		if messages[0].Role != "assistant" {
			t.Errorf("expected role 'assistant', got %q", messages[0].Role)
		}
		if messages[0].Content != "Hello! How can I help?" {
			t.Errorf("unexpected content %q", messages[0].Content)
		}
	})

	t.Run("OTEL format with tool calls", func(t *testing.T) {
		attrs := map[string]interface{}{
			"gen_ai.output.messages": `[{"role":"assistant","toolCalls":[{"id":"tc-1","name":"search","arguments":"{\"query\":\"test\"}"}]}]`,
		}
		messages := ExtractCompletionMessages(attrs)
		if len(messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(messages))
		}
		if len(messages[0].ToolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(messages[0].ToolCalls))
		}
		tc := messages[0].ToolCalls[0]
		if tc.Name != "search" {
			t.Errorf("expected tool name 'search', got %q", tc.Name)
		}
		if tc.ID != "tc-1" {
			t.Errorf("expected tool call ID 'tc-1', got %q", tc.ID)
		}
	})

	t.Run("Traceloop format gen_ai.completion.*", func(t *testing.T) {
		attrs := map[string]interface{}{
			"gen_ai.completion.0.role":    "assistant",
			"gen_ai.completion.0.content": "Sure, I can help.",
		}
		messages := ExtractCompletionMessages(attrs)
		if len(messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(messages))
		}
		if messages[0].Content != "Sure, I can help." {
			t.Errorf("unexpected content %q", messages[0].Content)
		}
	})
}

func TestExtractToolDefinitions(t *testing.T) {
	t.Run("OTEL format gen_ai.input.tools", func(t *testing.T) {
		attrs := map[string]interface{}{
			"gen_ai.input.tools": `[{"name":"search","description":"Search the web","parameters":{"type":"object"}}]`,
		}
		tools := ExtractToolDefinitions(attrs)
		if len(tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(tools))
		}
		if tools[0].Name != "search" {
			t.Errorf("expected 'search', got %q", tools[0].Name)
		}
		if tools[0].Description != "Search the web" {
			t.Errorf("expected 'Search the web', got %q", tools[0].Description)
		}
	})

	t.Run("OTEL format gen_ai.tool.definitions", func(t *testing.T) {
		attrs := map[string]interface{}{
			"gen_ai.tool.definitions": `[{"name":"calc","description":"Calculate"}]`,
		}
		tools := ExtractToolDefinitions(attrs)
		if len(tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(tools))
		}
		if tools[0].Name != "calc" {
			t.Errorf("expected 'calc', got %q", tools[0].Name)
		}
	})

	t.Run("Traceloop format llm.request.functions", func(t *testing.T) {
		attrs := map[string]interface{}{
			"llm.request.functions.0.name":        "search",
			"llm.request.functions.0.description": "Search tool",
			"llm.request.functions.1.name":        "calculate",
			"llm.request.functions.1.description": "Calculator",
		}
		tools := ExtractToolDefinitions(attrs)
		if len(tools) != 2 {
			t.Fatalf("expected 2 tools, got %d", len(tools))
		}
	})

	t.Run("no tools found", func(t *testing.T) {
		attrs := map[string]interface{}{
			"some.attr": "value",
		}
		tools := ExtractToolDefinitions(attrs)
		if tools != nil {
			t.Errorf("expected nil, got %+v", tools)
		}
	})
}

func TestExtractToolExecutionDetails(t *testing.T) {
	t.Run("traceloop entity attributes", func(t *testing.T) {
		attrs := map[string]interface{}{
			"traceloop.entity.name":   "search_tool",
			"traceloop.entity.input":  `{"inputs": "query text"}`,
			"traceloop.entity.output": "result text",
		}
		name, input, output, status := ExtractToolExecutionDetails(attrs, "OK")
		if name != "search_tool" {
			t.Errorf("expected name 'search_tool', got %q", name)
		}
		if input != `"query text"` {
			t.Errorf("unexpected input %q", input)
		}
		if output != "result text" {
			t.Errorf("unexpected output %q", output)
		}
		if status != "success" {
			t.Errorf("expected status 'success', got %q", status)
		}
	})

	t.Run("error status", func(t *testing.T) {
		attrs := map[string]interface{}{
			"gen_ai.tool.name": "failing_tool",
		}
		_, _, _, status := ExtractToolExecutionDetails(attrs, "error")
		if status != "error" {
			t.Errorf("expected 'error', got %q", status)
		}
	})
}

func TestRecursiveJSONParser(t *testing.T) {
	t.Run("parses regular JSON", func(t *testing.T) {
		result, err := RecursiveJSONParser(`{"key":"value"}`, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("expected map, got %T", result)
		}
		if m["key"] != "value" {
			t.Errorf("expected 'value', got %v", m["key"])
		}
	})

	t.Run("parses nested stringified JSON", func(t *testing.T) {
		// JSON string containing a JSON string
		result, err := RecursiveJSONParser(`"{\"key\":\"value\"}"`, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		m, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("expected map, got %T", result)
		}
		if m["key"] != "value" {
			t.Errorf("expected 'value', got %v", m["key"])
		}
	})

	t.Run("returns error on max depth", func(t *testing.T) {
		_, err := RecursiveJSONParser(`"{\"key\":\"value\"}"`, 0)
		if err == nil {
			t.Error("expected error for max depth exceeded, got nil")
		}
	})

	t.Run("returns plain string when not JSON", func(t *testing.T) {
		result, err := RecursiveJSONParser(`"hello world"`, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "hello world" {
			t.Errorf("expected 'hello world', got %v", result)
		}
	})

	t.Run("parses JSON array", func(t *testing.T) {
		result, err := RecursiveJSONParser(`[1, 2, 3]`, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		arr, ok := result.([]interface{})
		if !ok {
			t.Fatalf("expected array, got %T", result)
		}
		if len(arr) != 3 {
			t.Errorf("expected 3 elements, got %d", len(arr))
		}
	})
}

func TestExtractRootSpanInputOutput(t *testing.T) {
	t.Run("nil span", func(t *testing.T) {
		input, output := ExtractRootSpanInputOutput(nil)
		if input != nil || output != nil {
			t.Errorf("expected nil/nil, got %v/%v", input, output)
		}
	})

	t.Run("nil attributes", func(t *testing.T) {
		span := &Span{}
		input, output := ExtractRootSpanInputOutput(span)
		if input != nil || output != nil {
			t.Errorf("expected nil/nil, got %v/%v", input, output)
		}
	})

	t.Run("extracts from traceloop.entity attributes", func(t *testing.T) {
		span := &Span{
			Attributes: map[string]interface{}{
				"traceloop.entity.input":  `{"inputs": "hello"}`,
				"traceloop.entity.output": "plain output text",
			},
		}
		input, output := ExtractRootSpanInputOutput(span)
		if input == nil {
			t.Error("expected non-nil input")
		}
		if output != "plain output text" {
			t.Errorf("expected 'plain output text', got %v", output)
		}
	})
}

func TestExtractTokenUsageFromEntityOutput(t *testing.T) {
	t.Run("nil span", func(t *testing.T) {
		usage := ExtractTokenUsageFromEntityOutput(nil)
		if usage != nil {
			t.Errorf("expected nil, got %+v", usage)
		}
	})

	t.Run("extracts from response_metadata.token_usage", func(t *testing.T) {
		span := &Span{
			Attributes: map[string]interface{}{
				"traceloop.entity.output": `{"outputs":{"messages":[{"kwargs":{"response_metadata":{"token_usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}}}]}}`,
			},
		}
		usage := ExtractTokenUsageFromEntityOutput(span)
		if usage == nil {
			t.Fatal("expected token usage, got nil")
		}
		if usage.InputTokens != 100 {
			t.Errorf("expected 100, got %d", usage.InputTokens)
		}
		if usage.OutputTokens != 50 {
			t.Errorf("expected 50, got %d", usage.OutputTokens)
		}
		if usage.TotalTokens != 150 {
			t.Errorf("expected 150, got %d", usage.TotalTokens)
		}
	})

	t.Run("extracts from usage_metadata fallback", func(t *testing.T) {
		span := &Span{
			Attributes: map[string]interface{}{
				"traceloop.entity.output": `{"outputs":{"messages":[{"kwargs":{"usage_metadata":{"input_tokens":200,"output_tokens":80,"total_tokens":280}}}]}}`,
			},
		}
		usage := ExtractTokenUsageFromEntityOutput(span)
		if usage == nil {
			t.Fatal("expected token usage, got nil")
		}
		if usage.InputTokens != 200 {
			t.Errorf("expected 200, got %d", usage.InputTokens)
		}
		if usage.OutputTokens != 80 {
			t.Errorf("expected 80, got %d", usage.OutputTokens)
		}
	})

	t.Run("returns nil when no token info", func(t *testing.T) {
		span := &Span{
			Attributes: map[string]interface{}{
				"traceloop.entity.output": `{"outputs":{"messages":[{"kwargs":{"content":"hello"}}]}}`,
			},
		}
		usage := ExtractTokenUsageFromEntityOutput(span)
		if usage != nil {
			t.Errorf("expected nil, got %+v", usage)
		}
	})
}

func TestExtractIntValue(t *testing.T) {
	tests := []struct {
		name   string
		input  interface{}
		want   int
		wantOk bool
	}{
		{"int", 42, 42, true},
		{"int64", int64(100), 100, true},
		{"float64", float64(55), 55, true},
		{"string", "123", 123, true},
		{"invalid string", "abc", 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractIntValue(tt.input)
			if ok != tt.wantOk {
				t.Errorf("ok = %v, want %v", ok, tt.wantOk)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExtractFloatValue(t *testing.T) {
	tests := []struct {
		name   string
		input  interface{}
		want   float64
		wantOk bool
	}{
		{"float64", float64(3.14), 3.14, true},
		{"float32", float32(2.5), 2.5, true},
		{"int", 42, 42.0, true},
		{"int64", int64(100), 100.0, true},
		{"string", "1.5", 1.5, true},
		{"invalid string", "abc", 0, false},
		{"nil", nil, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractFloatValue(tt.input)
			if ok != tt.wantOk {
				t.Errorf("ok = %v, want %v", ok, tt.wantOk)
			}
			if tt.wantOk && got != tt.want {
				t.Errorf("got %f, want %f", got, tt.want)
			}
		})
	}
}

func TestIsErrorStatus(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"error", true},
		{"Error", true},
		{"ERROR", true},
		{"failed", true},
		{"Failed", true},
		{"2", true},
		{"OK", false},
		{"1", false},
		{"success", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := isErrorStatus(tt.status); got != tt.want {
				t.Errorf("isErrorStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestExtractSpanStatus(t *testing.T) {
	t.Run("error.type attribute", func(t *testing.T) {
		attrs := map[string]interface{}{"error.type": "RuntimeError"}
		status := extractSpanStatus(attrs, "")
		if !status.Error {
			t.Error("expected error=true")
		}
		if status.ErrorType != "RuntimeError" {
			t.Errorf("expected 'RuntimeError', got %q", status.ErrorType)
		}
	})

	t.Run("gen_ai.tool.status error", func(t *testing.T) {
		attrs := map[string]interface{}{"gen_ai.tool.status": "error"}
		status := extractSpanStatus(attrs, "")
		if !status.Error {
			t.Error("expected error=true")
		}
		if status.ErrorType != "ToolExecutionError" {
			t.Errorf("expected 'ToolExecutionError', got %q", status.ErrorType)
		}
	})

	t.Run("http.status_code >= 400", func(t *testing.T) {
		attrs := map[string]interface{}{"http.status_code": float64(500)}
		status := extractSpanStatus(attrs, "")
		if !status.Error {
			t.Error("expected error=true")
		}
		if status.ErrorType != "500" {
			t.Errorf("expected '500', got %q", status.ErrorType)
		}
	})

	t.Run("fallback to span status", func(t *testing.T) {
		attrs := map[string]interface{}{"some.attr": "value"}
		status := extractSpanStatus(attrs, "error")
		if !status.Error {
			t.Error("expected error=true from span status")
		}
	})

	t.Run("no error", func(t *testing.T) {
		attrs := map[string]interface{}{"some.attr": "value"}
		status := extractSpanStatus(attrs, "OK")
		if status.Error {
			t.Error("expected error=false")
		}
	})

	t.Run("nil attributes with error status", func(t *testing.T) {
		status := extractSpanStatus(nil, "error")
		if !status.Error {
			t.Error("expected error=true from span status")
		}
	})
}

func TestParseToolsJSON(t *testing.T) {
	t.Run("array of strings", func(t *testing.T) {
		tools := parseToolsJSON(`["tool1", "tool2"]`)
		if len(tools) != 2 {
			t.Fatalf("expected 2 tools, got %d", len(tools))
		}
		if tools[0].Name != "tool1" {
			t.Errorf("expected 'tool1', got %q", tools[0].Name)
		}
	})

	t.Run("array of objects", func(t *testing.T) {
		tools := parseToolsJSON(`[{"name":"search","description":"Search tool"}]`)
		if len(tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(tools))
		}
		if tools[0].Name != "search" {
			t.Errorf("expected 'search', got %q", tools[0].Name)
		}
		if tools[0].Description != "Search tool" {
			t.Errorf("expected 'Search tool', got %q", tools[0].Description)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		tools := parseToolsJSON("")
		if tools != nil {
			t.Errorf("expected nil, got %+v", tools)
		}
	})

	t.Run("raw string fallback", func(t *testing.T) {
		tools := parseToolsJSON("just-a-tool-name")
		if len(tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(tools))
		}
		if tools[0].Name != "just-a-tool-name" {
			t.Errorf("expected 'just-a-tool-name', got %q", tools[0].Name)
		}
	})
}

func TestExtractEmbeddingDocuments(t *testing.T) {
	t.Run("extracts ordered documents", func(t *testing.T) {
		attrs := map[string]interface{}{
			"gen_ai.prompt.0.content": "Document 1",
			"gen_ai.prompt.1.content": "Document 2",
			"gen_ai.prompt.2.content": "Document 3",
		}
		docs := ExtractEmbeddingDocuments(attrs)
		if len(docs) != 3 {
			t.Fatalf("expected 3 documents, got %d", len(docs))
		}
		if docs[0] != "Document 1" {
			t.Errorf("expected 'Document 1', got %q", docs[0])
		}
		if docs[2] != "Document 3" {
			t.Errorf("expected 'Document 3', got %q", docs[2])
		}
	})

	t.Run("returns nil when no documents", func(t *testing.T) {
		attrs := map[string]interface{}{
			"some.attr": "value",
		}
		docs := ExtractEmbeddingDocuments(attrs)
		if docs != nil {
			t.Errorf("expected nil, got %v", docs)
		}
	})
}
