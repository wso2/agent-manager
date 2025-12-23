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

import "time"

// TraceQueryParams holds parameters for trace queries
type TraceQueryParams struct {
	ComponentUid   string
	EnvironmentUid string
	StartTime      string
	EndTime        string
	Limit          int
	Offset         int
	SortOrder      string
}

// TraceByIdAndServiceParams holds parameters for querying by both traceId and componentUid
type TraceByIdAndServiceParams struct {
	TraceID        string
	ComponentUid   string
	EnvironmentUid string
	SortOrder      string
	Limit          int
}

// Span represents a single trace span
type Span struct {
	TraceID         string                 `json:"traceId"`
	SpanID          string                 `json:"spanId"`
	ParentSpanID    string                 `json:"parentSpanId,omitempty"`
	Name            string                 `json:"name"`
	Service         string                 `json:"service"`
	StartTime       time.Time              `json:"startTime"`
	EndTime         time.Time              `json:"endTime,omitempty"`
	DurationInNanos int64                  `json:"durationInNanos"` // in nanoseconds
	Kind            string                 `json:"kind,omitempty"`
	Status          string                 `json:"status,omitempty"`
	Attributes      map[string]interface{} `json:"attributes,omitempty"`
	Resource        map[string]interface{} `json:"resource,omitempty"`
	AmpAttributes   *AmpAttributes         `json:"ampAttributes,omitempty"` // Custom AMP-specific attributes
}

// AmpAttributes holds custom attributes added by the AMP platform
type AmpAttributes struct {
	Kind   string      `json:"kind"`             // Semantic span kind: llm, tool, embedding, retriever, rerank, agent, task, unknown
	Input  interface{} `json:"input,omitempty"`  // Input data (type varies by kind)
	Output interface{} `json:"output,omitempty"` // Output data (type varies by kind)
	Status *SpanStatus `json:"status,omitempty"` // Execution status with error information
	Data   interface{} `json:"data,omitempty"`   // Kind-specific data: *LLMData, *ToolData, *EmbeddingData, *RetrieverData, etc.
}

// LLMData contains LLM-specific span information
type LLMData struct {
	Tools       []ToolDefinition `json:"tools,omitempty"`       // Available tools/functions
	Model       string           `json:"model,omitempty"`       // Model name (gen_ai.response.model or gen_ai.request.model)
	Vendor      string           `json:"vendor,omitempty"`      // LLM vendor/provider (gen_ai.system)
	Temperature *float64         `json:"temperature,omitempty"` // Temperature parameter
	TokenUsage  *LLMTokenUsage   `json:"tokenUsage,omitempty"`  // Token usage details
}

// ToolData contains tool execution span information
type ToolData struct {
	Name string `json:"name,omitempty"` // Tool/function name
}

// EmbeddingData contains embedding generation span information
// EmbeddingData contains embedding generation span information
type EmbeddingData struct {
	Model      string         `json:"model,omitempty"`      // Embedding model name
	Vendor     string         `json:"vendor,omitempty"`     // Embedding vendor/provider (gen_ai.system)
	TokenUsage *LLMTokenUsage `json:"tokenUsage,omitempty"` // Token usage details
}

// RetrieverData contains vector database retrieval span information
type RetrieverData struct {
	VectorDB string `json:"vectorDB,omitempty"` // Vector database system (e.g., Chroma, Pinecone)
	TopK     int    `json:"topK,omitempty"`     // Number of top results requested
}

// AgentData contains agent execution span information
type AgentData struct {
	Name         string           `json:"name,omitempty"`         // Agent name (from gen_ai.agent.name)
	Tools        []ToolDefinition `json:"tools,omitempty"`        // Available tools for the agent (from gen_ai.agent.tools)
	Model        string           `json:"model,omitempty"`        // Model used by the agent (from gen_ai.request.model)
	Framework    string           `json:"framework,omitempty"`    // Agent framework (from gen_ai.system, e.g., "strands-agents")
	SystemPrompt string           `json:"systemPrompt,omitempty"` // System prompt for the agent
	MaxIter      int              `json:"maxIter,omitempty"`      // Maximum iterations for the agent (from crewai.agent.max_iter)
	TokenUsage   *LLMTokenUsage   `json:"tokenUsage,omitempty"`   // Token usage details (aggregated from agent execution)
}

// SpanStatus represents the execution status of a span
type SpanStatus struct {
	Error     bool   `json:"error"`               // Whether the span has an error
	ErrorType string `json:"errorType,omitempty"` // Error type from error.type attribute (only if error is true)
}

// LLMTokenUsage represents token usage for a single LLM span
type LLMTokenUsage struct {
	InputTokens          int `json:"inputTokens"`
	OutputTokens         int `json:"outputTokens"`
	CacheReadInputTokens int `json:"cacheReadInputTokens,omitempty"`
	TotalTokens          int `json:"totalTokens"`
}

// PromptMessage represents a single message in a conversation
type PromptMessage struct {
	Role      string     `json:"role"`                // system, user, assistant, tool
	Content   string     `json:"content,omitempty"`   // The message content (text)
	ToolCalls []ToolCall `json:"toolCalls,omitempty"` // Tool calls made by assistant (for assistant role with tool calls)
}

// ToolCall represents a tool/function call made by the assistant
type ToolCall struct {
	ID        string `json:"id"`        // Tool call ID
	Name      string `json:"name"`      // Function/tool name
	Arguments string `json:"arguments"` // JSON arguments for the tool
}

// ToolDefinition represents a tool/function available to the LLM
type ToolDefinition struct {
	Name        string `json:"name"`                  // Function name
	Description string `json:"description,omitempty"` // Function description
	Parameters  string `json:"parameters,omitempty"`  // JSON schema of parameters
}

// TraceResponse represents the response for trace queries
type TraceResponse struct {
	Spans      []Span       `json:"spans"`
	TotalCount int          `json:"totalCount"`
	TokenUsage *TokenUsage  `json:"tokenUsage,omitempty"` // Aggregated token usage from GenAI spans
	Status     *TraceStatus `json:"status,omitempty"`     // Trace status including error information
}

// TraceDetailResponse represents detailed information for a single trace
type TraceDetailResponse struct {
	TraceID    string   `json:"traceId"`
	Spans      []Span   `json:"spans"`
	TotalSpans int      `json:"totalSpans"`
	Duration   int64    `json:"duration"` // Total trace duration in microseconds
	Services   []string `json:"services"` // List of services involved
}

// TraceOverview represents a single trace overview with root span info
type TraceOverview struct {
	TraceID         string       `json:"traceId"`
	RootSpanID      string       `json:"rootSpanId"`
	RootSpanName    string       `json:"rootSpanName"`
	RootSpanKind    string       `json:"rootSpanKind"` // Semantic kind of the root span (llm, tool, etc.)
	StartTime       string       `json:"startTime"`
	EndTime         string       `json:"endTime"`
	DurationInNanos int64        `json:"durationInNanos"` // Total trace duration in nanoseconds
	SpanCount       int          `json:"spanCount"`
	TokenUsage      *TokenUsage  `json:"tokenUsage,omitempty"` // Aggregated token usage from GenAI spans
	Status          *TraceStatus `json:"status,omitempty"`     // Trace status including error information
	Input           interface{}  `json:"input,omitempty"`      // Input from root span (nil if not found)
	Output          interface{}  `json:"output,omitempty"`     // Output from root span (nil if not found)
}

// TraceStatus represents the status of a trace
type TraceStatus struct {
	ErrorCount int `json:"errorCount"` // Number of spans with errors (0 means no errors)
}

// SpanType represents the semantic type/kind of a span
type SpanType string

const (
	SpanTypeLLM       SpanType = "llm"       // LLM/Chat completion operations
	SpanTypeEmbedding SpanType = "embedding" // Embedding generation operations
	SpanTypeTool      SpanType = "tool"      // Tool/Function calls
	SpanTypeRetriever SpanType = "retriever" // Vector DB retrieval operations
	SpanTypeRerank    SpanType = "rerank"    // Reranking operations
	SpanTypeAgent     SpanType = "agent"     // Agent orchestration
	SpanTypeChain     SpanType = "chain"     // Generic tasks/workflows
	SpanTypeUnknown   SpanType = "unknown"   // Unknown/unclassified spans
)

// TokenUsage represents aggregated token usage from GenAI spans
type TokenUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
}

// TraceOverviewResponse represents the response for trace overview queries
type TraceOverviewResponse struct {
	Traces     []TraceOverview `json:"traces"`
	TotalCount int             `json:"totalCount"`
}

// SearchResponse represents OpenSearch search response
type SearchResponse struct {
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []struct {
			Source map[string]interface{} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}
