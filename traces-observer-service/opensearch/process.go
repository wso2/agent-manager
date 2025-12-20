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
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ParseSpans converts OpenSearch response to Span structs
func ParseSpans(response *SearchResponse) []Span {
	spans := make([]Span, 0, len(response.Hits.Hits))

	for _, hit := range response.Hits.Hits {
		span := parseSpan(hit.Source)
		spans = append(spans, span)
	}

	return spans
}

// parseSpan extracts span information from a source document
func parseSpan(source map[string]interface{}) Span {
	span := Span{}

	// Try standard OTEL fields first
	if traceID, ok := source["traceId"].(string); ok {
		span.TraceID = traceID
	}
	if spanID, ok := source["spanId"].(string); ok {
		span.SpanID = spanID
	}
	if parentSpanID, ok := source["parentSpanId"].(string); ok {
		span.ParentSpanID = parentSpanID
	}
	if name, ok := source["name"].(string); ok {
		span.Name = name
	}
	if kind, ok := source["kind"].(string); ok {
		span.Kind = kind
	}

	// Extract component UID from resource
	if resource, ok := source["resource"].(map[string]interface{}); ok {
		if componentUid, ok := resource["openchoreo.dev/component-uid"].(string); ok {
			span.Service = componentUid
		}

		// Store the complete resource object
		span.Resource = resource
	}

	// Parse timestamps
	if startTime, ok := source["startTime"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, startTime); err == nil {
			span.StartTime = t
		}
	}
	if endTime, ok := source["endTime"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, endTime); err == nil {
			span.EndTime = t
		}
	}

	// Parse duration - try durationInNanos field first
	if duration, ok := source["durationInNanos"].(float64); ok {
		span.DurationInNanos = int64(duration)
	} else if !span.StartTime.IsZero() && !span.EndTime.IsZero() {
		// Fallback: calculate duration from timestamps if durationInNanos not present
		span.DurationInNanos = span.EndTime.Sub(span.StartTime).Nanoseconds()
	}

	// Parse status
	if status, ok := source["status"].(map[string]interface{}); ok {
		if code, ok := status["code"].(string); ok {
			span.Status = code
		} else if code, ok := status["code"].(float64); ok {
			span.Status = fmt.Sprintf("%d", int(code))
		}
	}

	// Parse attributes
	if attributes, ok := source["attributes"].(map[string]interface{}); ok {
		span.Attributes = attributes
	}

	// Determine and add the semantic span type to AmpAttributes
	spanType := DetermineSpanType(span)
	ampAttrs := &AmpAttributes{
		Kind: string(spanType),
	}

	// For LLM spans, extract prompts and tools
	if spanType == SpanTypeLLM && span.Attributes != nil {
		ampAttrs.Input = ExtractPromptMessages(span.Attributes)
		ampAttrs.Output = ExtractCompletionMessages(span.Attributes)
		ampAttrs.Tools = ExtractToolDefinitions(span.Attributes)

		// Extract model information
		if responseModel, ok := span.Attributes["gen_ai.response.model"].(string); ok {
			ampAttrs.Model = responseModel
		} else if requestModel, ok := span.Attributes["gen_ai.request.model"].(string); ok {
			ampAttrs.Model = requestModel
		}

		// Extract temperature
		if temp, ok := span.Attributes["gen_ai.request.temperature"].(float64); ok {
			ampAttrs.Temperature = &temp
		}

		// Extract token usage
		var inputTokens, outputTokens, cacheReadTokens int
		if val, ok := span.Attributes["gen_ai.usage.input_tokens"].(float64); ok {
			inputTokens = int(val)
		}
		if val, ok := span.Attributes["gen_ai.usage.output_tokens"].(float64); ok {
			outputTokens = int(val)
		}
		if val, ok := span.Attributes["gen_ai.usage.cache_read_input_tokens"].(float64); ok {
			cacheReadTokens = int(val)
		}

		if inputTokens > 0 || outputTokens > 0 {
			ampAttrs.TokenUsage = &LLMTokenUsage{
				InputTokens:          inputTokens,
				OutputTokens:         outputTokens,
				CacheReadInputTokens: cacheReadTokens,
				TotalTokens:          inputTokens + outputTokens,
			}
		}
	}

	// For Tool spans, extract tool execution details
	if spanType == SpanTypeTool && span.Attributes != nil {
		name, toolInput, toolOutput, _ := ExtractToolExecutionDetails(span.Attributes, span.Status) // TODO we need to use tool status for tool spans
		ampAttrs.Name = name
		ampAttrs.Input = toolInput
		ampAttrs.Output = toolOutput
	}

	// Extract error status for all span types
	spanStatus := &SpanStatus{
		Error: false,
	}

	if span.Attributes != nil {
		// Check for error.type attribute (e.g., "OperationalError")
		if errorType, ok := span.Attributes["error.type"].(string); ok {
			spanStatus.Error = true
			spanStatus.ErrorType = errorType
		} else if isErrorStatus(span.Status) {
			// Fallback to generic error if span status indicates error but no error.type
			spanStatus.Error = true
		}
	}

	ampAttrs.Status = spanStatus
	span.AmpAttributes = ampAttrs

	return span
}

// ExtractTokenUsage aggregates token usage from GenAI spans in a trace
func ExtractTokenUsage(spans []Span) *TokenUsage {
	var inputTokens, outputTokens int

	for _, span := range spans {
		// Check if this is a GenAI span by looking for gen_ai.* attributes
		if span.Attributes != nil {
			// Try to extract input tokens (gen_ai.usage.input_tokens or gen_ai.prompt_tokens)
			if val, ok := span.Attributes["gen_ai.usage.input_tokens"].(float64); ok {
				inputTokens += int(val)
			} else if val, ok := span.Attributes["gen_ai.usage.prompt_tokens"].(float64); ok {
				inputTokens += int(val)
			}

			// Try to extract output tokens (gen_ai.usage.output_tokens or gen_ai.completion_tokens)
			if val, ok := span.Attributes["gen_ai.usage.output_tokens"].(float64); ok {
				outputTokens += int(val)
			} else if val, ok := span.Attributes["gen_ai.usage.completion_tokens"].(float64); ok {
				outputTokens += int(val)
			}
		}
	}

	// Only return token usage if we found some tokens
	if inputTokens > 0 || outputTokens > 0 {
		return &TokenUsage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			TotalTokens:  inputTokens + outputTokens,
		}
	}

	return nil
}

// ExtractTraceStatus analyzes spans to determine trace status and error information
func ExtractTraceStatus(spans []Span) *TraceStatus {
	var errorCount int

	for _, span := range spans {
		// Check if span status indicates an error
		// OTEL status codes: "UNSET" (0), "OK" (1), "ERROR" (2)
		// Some systems use string values, others use numeric codes
		if isErrorStatus(span.Status) {
			errorCount++
		}
	}

	return &TraceStatus{
		ErrorCount: errorCount,
	}
}

// isErrorStatus checks if a status string indicates an error
func isErrorStatus(status string) bool {
	// Check for common error status values
	switch strings.ToLower(status) {
	case "error", "failed", "2":
		return true
	default:
		return false
	}
}

// ExtractRootSpanInputOutput extracts input and output from the root span (parent node)
// by analyzing the "traceloop.entity.input" and "traceloop.entity.output" attributes
// Input path: traceloop.entity.input -> inputs (ensure it's JSON)
// Output path: traceloop.entity.output -> outputs -> messages[-1] -> kwargs -> content
func ExtractRootSpanInputOutput(rootSpan *Span) (input string, output string) {
	if rootSpan == nil || rootSpan.Attributes == nil {
		return "", ""
	}

	// Extract input from traceloop.entity.input attribute
	// Path: input -> inputs (make sure it's JSON)
	if inputVal, ok := rootSpan.Attributes["traceloop.entity.input"]; ok {
		if inputStr, ok := inputVal.(string); ok {
			// Try to parse as JSON
			var inputMap map[string]interface{}
			if err := json.Unmarshal([]byte(inputStr), &inputMap); err == nil {
				// Navigate to inputs field
				if nestedInputs, ok := inputMap["inputs"]; ok {
					// Convert to JSON string
					if nestedBytes, err := json.Marshal(nestedInputs); err == nil {
						input = string(nestedBytes)
					} else {
						input = inputStr // Fallback to original
					}
				} else {
					// No "inputs" field, return the whole JSON
					input = inputStr
				}
			} else {
				// Not valid JSON, return as-is
				input = inputStr
			}
		}
	}

	// Extract output from traceloop.entity.output attribute
	// Path: output -> outputs -> messages[-1] -> kwargs -> content
	if outputVal, ok := rootSpan.Attributes["traceloop.entity.output"]; ok {
		if outputStr, ok := outputVal.(string); ok {
			// Try to parse as JSON
			var outputMap map[string]interface{}
			if err := json.Unmarshal([]byte(outputStr), &outputMap); err == nil {
				// Navigate to outputs field
				if outputs, ok := outputMap["outputs"]; ok {
					// Try to navigate to messages[-1] -> kwargs -> content
					if outputsMap, ok := outputs.(map[string]interface{}); ok {
						if messages, ok := outputsMap["messages"].([]interface{}); ok && len(messages) > 0 {
							// Get the last message
							lastMessage := messages[len(messages)-1]
							if lastMessageMap, ok := lastMessage.(map[string]interface{}); ok {
								if kwargs, ok := lastMessageMap["kwargs"].(map[string]interface{}); ok {
									if content, ok := kwargs["content"].(string); ok {
										output = content
									} else {
										// content is not a string, try to marshal it
										if contentBytes, err := json.Marshal(kwargs["content"]); err == nil {
											output = string(contentBytes)
										}
									}
								} else {
									// No kwargs, return the whole last message as JSON
									if msgBytes, err := json.Marshal(lastMessage); err == nil {
										output = string(msgBytes)
									}
								}
							}
						} else {
							// No messages array or empty, return outputs as JSON
							if outputsBytes, err := json.Marshal(outputs); err == nil {
								output = string(outputsBytes)
							}
						}
					} else {
						// outputs is not a map, return it as JSON
						if outputsBytes, err := json.Marshal(outputs); err == nil {
							output = string(outputsBytes)
						}
					}
				} else {
					// No "outputs" field, return the whole JSON
					output = outputStr
				}
			} else {
				// Not valid JSON, return as-is
				output = outputStr
			}
		}
	}

	return input, output
}

// ExtractPromptMessages extracts and orders prompt messages from LLM span attributes
// Handles attributes in the format: gen_ai.prompt.0.role, gen_ai.prompt.0.content, etc.
// Also handles tool calls: gen_ai.prompt.{index}.tool_calls.{tool_index}.{field}
func ExtractPromptMessages(attrs map[string]interface{}) []PromptMessage {
	// Map to store messages by index
	messageMap := make(map[int]*PromptMessage)
	// Map to store tool calls for each message: messageIndex -> toolCallIndex -> ToolCall
	toolCallsMap := make(map[int]map[int]*ToolCall)
	maxIndex := -1

	// Iterate through attributes to find prompt messages
	for key, value := range attrs {
		// Check if it's a gen_ai.prompt.* attribute
		if strings.HasPrefix(key, "gen_ai.prompt.") {
			// Parse the index and field name
			// Format: gen_ai.prompt.{index}.{field} or gen_ai.prompt.{index}.tool_calls.{tool_index}.{field}
			parts := strings.Split(key, ".")
			if len(parts) >= 4 {
				// Extract message index
				var msgIndex int
				if _, err := fmt.Sscanf(parts[2], "%d", &msgIndex); err == nil {
					// Initialize message if not exists
					if messageMap[msgIndex] == nil {
						messageMap[msgIndex] = &PromptMessage{}
					}

					// Update max index
					if msgIndex > maxIndex {
						maxIndex = msgIndex
					}

					fieldName := parts[3]

					// Handle regular fields
					if fieldName == "role" {
						if role, ok := value.(string); ok {
							messageMap[msgIndex].Role = role
						}
					} else if fieldName == "content" {
						if content, ok := value.(string); ok {
							// Only set content if it's not empty or just empty quotes
							if content != "" && content != "\"\"" {
								messageMap[msgIndex].Content = content
							}
						}
					} else if fieldName == "tool_calls" && len(parts) >= 6 {
						// Handle tool calls: gen_ai.prompt.{msgIndex}.tool_calls.{toolIndex}.{field}
						var toolIndex int
						if _, err := fmt.Sscanf(parts[4], "%d", &toolIndex); err == nil {
							toolField := parts[5]

							// Initialize tool calls map for this message if needed
							if toolCallsMap[msgIndex] == nil {
								toolCallsMap[msgIndex] = make(map[int]*ToolCall)
							}

							// Initialize tool call if not exists
							if toolCallsMap[msgIndex][toolIndex] == nil {
								toolCallsMap[msgIndex][toolIndex] = &ToolCall{}
							}

							// Set the appropriate tool call field
							if toolField == "id" {
								if id, ok := value.(string); ok {
									toolCallsMap[msgIndex][toolIndex].ID = id
								}
							} else if toolField == "name" {
								if name, ok := value.(string); ok {
									toolCallsMap[msgIndex][toolIndex].Name = name
								}
							} else if toolField == "arguments" {
								if args, ok := value.(string); ok {
									toolCallsMap[msgIndex][toolIndex].Arguments = args
								}
							}
						}
					}
				}
			}
		}
	}

	// Attach tool calls to their respective messages
	for msgIndex, toolCalls := range toolCallsMap {
		if msg := messageMap[msgIndex]; msg != nil {
			// Find max tool call index
			maxToolIndex := -1
			for toolIndex := range toolCalls {
				if toolIndex > maxToolIndex {
					maxToolIndex = toolIndex
				}
			}

			// Convert tool calls map to ordered slice
			if maxToolIndex >= 0 {
				msg.ToolCalls = make([]ToolCall, 0, maxToolIndex+1)
				for i := 0; i <= maxToolIndex; i++ {
					if tc := toolCalls[i]; tc != nil && tc.Name != "" {
						msg.ToolCalls = append(msg.ToolCalls, *tc)
					}
				}
			}
		}
	}

	// Convert map to ordered slice
	if maxIndex < 0 {
		return nil
	}

	messages := make([]PromptMessage, 0, maxIndex+1)
	for i := 0; i <= maxIndex; i++ {
		if msg := messageMap[i]; msg != nil && msg.Role != "" {
			messages = append(messages, *msg)
		}
	}

	return messages
}

// ExtractCompletionMessages extracts and orders completion/output messages from LLM span attributes
// Handles attributes in the format: gen_ai.completion.0.role, gen_ai.completion.0.content, etc.
// Also handles tool calls: gen_ai.completion.{index}.tool_calls.{tool_index}.{field}
func ExtractCompletionMessages(attrs map[string]interface{}) []PromptMessage {
	// Map to store messages by index
	messageMap := make(map[int]*PromptMessage)
	// Map to store tool calls for each message: messageIndex -> toolCallIndex -> ToolCall
	toolCallsMap := make(map[int]map[int]*ToolCall)
	maxIndex := -1

	// Iterate through attributes to find completion messages
	for key, value := range attrs {
		// Check if it's a gen_ai.completion.* attribute
		if strings.HasPrefix(key, "gen_ai.completion.") {
			// Parse the index and field name
			// Format: gen_ai.completion.{index}.{field} or gen_ai.completion.{index}.tool_calls.{tool_index}.{field}
			parts := strings.Split(key, ".")
			if len(parts) >= 4 {
				// Extract message index
				var msgIndex int
				if _, err := fmt.Sscanf(parts[2], "%d", &msgIndex); err == nil {
					// Initialize message if not exists
					if messageMap[msgIndex] == nil {
						messageMap[msgIndex] = &PromptMessage{}
					}

					// Update max index
					if msgIndex > maxIndex {
						maxIndex = msgIndex
					}

					fieldName := parts[3]

					// Handle regular fields
					if fieldName == "role" {
						if role, ok := value.(string); ok {
							messageMap[msgIndex].Role = role
						}
					} else if fieldName == "content" {
						if content, ok := value.(string); ok {
							// Only set content if it's not empty or just empty quotes
							if content != "" && content != "\"\"" {
								messageMap[msgIndex].Content = content
							}
						}
					} else if fieldName == "tool_calls" && len(parts) >= 6 {
						// Handle tool calls: gen_ai.completion.{msgIndex}.tool_calls.{toolIndex}.{field}
						var toolIndex int
						if _, err := fmt.Sscanf(parts[4], "%d", &toolIndex); err == nil {
							toolField := parts[5]

							// Initialize tool calls map for this message if needed
							if toolCallsMap[msgIndex] == nil {
								toolCallsMap[msgIndex] = make(map[int]*ToolCall)
							}

							// Initialize tool call if not exists
							if toolCallsMap[msgIndex][toolIndex] == nil {
								toolCallsMap[msgIndex][toolIndex] = &ToolCall{}
							}

							// Set the appropriate tool call field
							if toolField == "id" {
								if id, ok := value.(string); ok {
									toolCallsMap[msgIndex][toolIndex].ID = id
								}
							} else if toolField == "name" {
								if name, ok := value.(string); ok {
									toolCallsMap[msgIndex][toolIndex].Name = name
								}
							} else if toolField == "arguments" {
								if args, ok := value.(string); ok {
									toolCallsMap[msgIndex][toolIndex].Arguments = args
								}
							}
						}
					}
				}
			}
		}
	}

	// Attach tool calls to their respective messages
	for msgIndex, toolCalls := range toolCallsMap {
		if msg := messageMap[msgIndex]; msg != nil {
			// Find max tool call index
			maxToolIndex := -1
			for toolIndex := range toolCalls {
				if toolIndex > maxToolIndex {
					maxToolIndex = toolIndex
				}
			}

			// Convert tool calls map to ordered slice
			if maxToolIndex >= 0 {
				msg.ToolCalls = make([]ToolCall, 0, maxToolIndex+1)
				for i := 0; i <= maxToolIndex; i++ {
					if tc := toolCalls[i]; tc != nil && tc.Name != "" {
						msg.ToolCalls = append(msg.ToolCalls, *tc)
					}
				}
			}
		}
	}

	// Convert map to ordered slice
	if maxIndex < 0 {
		return nil
	}

	messages := make([]PromptMessage, 0, maxIndex+1)
	for i := 0; i <= maxIndex; i++ {
		if msg := messageMap[i]; msg != nil && msg.Role != "" {
			messages = append(messages, *msg)
		}
	}

	return messages
}

// ExtractToolDefinitions extracts tool/function definitions from LLM span attributes
// Handles attributes in the format: llm.request.functions.0.name, llm.request.functions.0.description, etc.
func ExtractToolDefinitions(attrs map[string]interface{}) []ToolDefinition {
	// Map to store tools by index
	toolMap := make(map[int]*ToolDefinition)
	maxIndex := -1

	// Iterate through attributes to find tool definitions
	for key, value := range attrs {
		// Check if it's a llm.request.functions.* attribute
		if strings.HasPrefix(key, "llm.request.functions.") {
			// Parse the index and field name
			// Format: llm.request.functions.{index}.{field}
			parts := strings.Split(key, ".")
			if len(parts) >= 5 { // Need at least 5 parts to access parts[4]
				// Extract index
				var index int
				if _, err := fmt.Sscanf(parts[3], "%d", &index); err == nil {
					fieldName := parts[4]

					// Initialize tool if not exists
					if toolMap[index] == nil {
						toolMap[index] = &ToolDefinition{}
					}

					// Update max index
					if index > maxIndex {
						maxIndex = index
					}

					// Set the appropriate field
					if fieldName == "name" {
						if name, ok := value.(string); ok {
							toolMap[index].Name = name
						}
					} else if fieldName == "description" {
						if desc, ok := value.(string); ok {
							toolMap[index].Description = desc
						}
					} else if fieldName == "parameters" {
						if params, ok := value.(string); ok {
							toolMap[index].Parameters = params
						}
					}
				}
			}
		}
	}

	// Convert map to ordered slice
	if maxIndex < 0 {
		return nil
	}

	tools := make([]ToolDefinition, 0, maxIndex+1)
	for i := 0; i <= maxIndex; i++ {
		if tool := toolMap[i]; tool != nil && tool.Name != "" {
			tools = append(tools, *tool)
		}
	}

	return tools
}

// ExtractToolExecutionDetails extracts tool execution information from tool span attributes
// Returns: name, input, output, status
func ExtractToolExecutionDetails(attrs map[string]interface{}, spanStatus string) (string, string, string, string) {
	var name, input, output, status string

	// Extract tool name - prioritize traceloop.entity.name
	if entityName, ok := attrs["traceloop.entity.name"].(string); ok {
		name = entityName
	} else if toolName, ok := attrs["tool.name"].(string); ok {
		name = toolName
	} else if funcName, ok := attrs["function.name"].(string); ok {
		name = funcName
	} else if genAIName, ok := attrs["gen_ai.tool.name"].(string); ok {
		name = genAIName
	}

	// Extract tool input - prioritize traceloop.entity.input with "inputs" extraction
	if traceloopInput, ok := attrs["traceloop.entity.input"].(string); ok && traceloopInput != "" {
		// Try to parse as JSON and extract "inputs" field
		var inputMap map[string]interface{}
		if err := json.Unmarshal([]byte(traceloopInput), &inputMap); err == nil {
			if inputs, ok := inputMap["inputs"]; ok {
				// Convert inputs to JSON string
				if inputsJSON, err := json.Marshal(inputs); err == nil {
					input = string(inputsJSON)
				} else {
					input = traceloopInput // Fallback to original
				}
			} else {
				input = traceloopInput // No "inputs" field, use whole JSON
			}
		} else {
			input = traceloopInput // Not valid JSON, use as-is
		}
	} else if toolInput, ok := attrs["tool.input"].(string); ok {
		input = toolInput
	} else if toolArgs, ok := attrs["tool.arguments"].(string); ok {
		input = toolArgs
	} else if funcArgs, ok := attrs["function.arguments"].(string); ok {
		input = funcArgs
	}

	// Extract tool output - prioritize traceloop.entity.output
	if entityOutput, ok := attrs["traceloop.entity.output"].(string); ok {
		output = entityOutput
	} else if toolOutput, ok := attrs["tool.output"].(string); ok {
		output = toolOutput
	} else if toolResult, ok := attrs["tool.result"].(string); ok {
		output = toolResult
	} else if funcResult, ok := attrs["function.result"].(string); ok {
		output = funcResult
	}

	// Determine status
	// First check if there's an explicit tool status attribute
	if toolStatus, ok := attrs["tool.status"].(string); ok {
		status = toolStatus
	} else {
		// Fall back to span status
		if isErrorStatus(spanStatus) {
			status = "error"
		} else if spanStatus == "OK" || spanStatus == "1" {
			status = "success"
		} else {
			status = "success" // Default to success if status is unclear
		}
	}

	return name, input, output, status
}

// DetermineSpanType analyzes a span's attributes to determine its semantic type
func DetermineSpanType(span Span) SpanType {
	if span.Attributes == nil {
		return SpanTypeUnknown
	}

	// First, check if Traceloop has already set the span kind
	if traceloopKind, ok := span.Attributes["traceloop.span.kind"].(string); ok {
		switch traceloopKind {
		case "llm":
			return SpanTypeLLM
		case "embedding":
			return SpanTypeEmbedding
		case "tool":
			return SpanTypeTool
		case "retriever":
			return SpanTypeRetriever
		case "rerank":
			return SpanTypeRerank
		case "agent":
			return SpanTypeAgent
		case "task", "workflow":
			return SpanTypeChain
		}
	}

	// Fallback to attribute-based detection if traceloop.span.kind is not present
	// Check for LLM operations
	if hasLLMAttributes(span.Attributes) {
		return SpanTypeLLM
	}

	// Check for Embedding operations
	if hasEmbeddingAttributes(span.Attributes) {
		return SpanTypeEmbedding
	}

	// Check for Tool/Function calls
	if hasToolAttributes(span.Attributes) {
		return SpanTypeTool
	}

	// Check for Retriever operations
	if hasRetrieverAttributes(span.Attributes) {
		return SpanTypeRetriever
	}

	// Check for Rerank operations
	if hasRerankAttributes(span.Attributes) {
		return SpanTypeRerank
	}

	// Check for Agent orchestration
	if hasAgentAttributes(span.Attributes, span.Name) {
		return SpanTypeAgent
	}

	// Check for Task/Workflow operations
	if hasTaskAttributes(span.Attributes, span.Name) {
		return SpanTypeChain
	}

	// Final fallback: check span name for hints
	// Names like "crewai.workflow", "LangGraph.task", "LangGraph.agent"
	if spanType := determineSpanTypeFromName(span.Name); spanType != SpanTypeUnknown {
		return spanType
	}

	return SpanTypeUnknown
}

// determineSpanTypeFromName infers span type from the span name
// Checks the last segment after splitting by "." for type hints
func determineSpanTypeFromName(name string) SpanType {
	if name == "" {
		return SpanTypeUnknown
	}

	// Split by "." and get the last segment
	parts := strings.Split(name, ".")
	if len(parts) == 0 {
		return SpanTypeUnknown
	}

	lastSegment := strings.ToLower(parts[len(parts)-1])

	// Map common suffixes to span types
	switch lastSegment {
	case "llm", "chat", "completion":
		return SpanTypeLLM
	case "embedding", "embed":
		return SpanTypeEmbedding
	case "tool", "function":
		return SpanTypeTool
	case "retriever", "retrieve", "retrieval":
		return SpanTypeRetriever
	case "rerank", "reranker":
		return SpanTypeRerank
	case "agent":
		return SpanTypeAgent
	case "task", "workflow":
		return SpanTypeChain
	default:
		return SpanTypeUnknown
	}
}

// hasLLMAttributes checks if span has LLM/chat completion attributes
func hasLLMAttributes(attrs map[string]interface{}) bool {
	// Check for gen_ai.operation.name with chat/completion
	if opName, ok := attrs["gen_ai.operation.name"].(string); ok {
		if opName == "chat" || opName == "completion" || opName == "text_completion" {
			return true
		}
	}

	// Check for LLM model attributes (but verify it's not an embedding operation)
	if _, ok := attrs["gen_ai.request.model"].(string); ok {
		// Check gen_ai.operation.name - if it's embedding, this is not an LLM
		if opName, ok := attrs["gen_ai.operation.name"].(string); ok {
			if opName == "embedding" || opName == "embeddings" {
				return false
			}
		}
		// Don't assume it's LLM just because it has a model - could be undetected embedding
	}

	// Check for response attributes specific to LLM
	if _, ok := attrs["gen_ai.response.finish_reasons"]; ok {
		return true
	}

	// Traceloop specific: llm.request.type (but exclude embeddings)
	if reqType, ok := attrs["llm.request.type"].(string); ok {
		return reqType != "embedding"
	}

	return false
}

// hasEmbeddingAttributes checks if span has embedding generation attributes
func hasEmbeddingAttributes(attrs map[string]interface{}) bool {
	// Check for gen_ai.operation.name = embedding
	if opName, ok := attrs["gen_ai.operation.name"].(string); ok {
		if opName == "embedding" || opName == "embeddings" {
			return true
		}
	}

	// Check for embedding-specific attributes
	if _, ok := attrs["gen_ai.embedding.dimension"].(float64); ok {
		return true
	}

	// Traceloop specific
	if reqType, ok := attrs["llm.request.type"].(string); ok {
		if reqType == "embedding" {
			return true
		}
	}

	return false
}

// hasToolAttributes checks if span has tool/function call attributes
func hasToolAttributes(attrs map[string]interface{}) bool {
	// Check for tool call attributes
	if _, ok := attrs["gen_ai.tool.name"].(string); ok {
		return true
	}

	// Check for function call attributes
	if _, ok := attrs["function.name"].(string); ok {
		return true
	}

	// Traceloop specific: tool.* namespace
	if _, ok := attrs["tool.name"].(string); ok {
		return true
	}

	// OpenAI specific function call
	if _, ok := attrs["llm.tool_calls"]; ok {
		return true
	}

	return false
}

// hasRetrieverAttributes checks if span has retriever/vector DB attributes
func hasRetrieverAttributes(attrs map[string]interface{}) bool {
	// Check for vector database operations
	if dbSystem, ok := attrs["db.system"].(string); ok {
		vectorDBs := []string{"pinecone", "weaviate", "qdrant", "milvus", "chroma", "chromadb"}
		for _, vdb := range vectorDBs {
			if dbSystem == vdb {
				return true
			}
		}
	}

	// Check for retrieval-specific operations
	if opName, ok := attrs["db.operation"].(string); ok {
		if opName == "query" || opName == "search" || opName == "retrieve" {
			return true
		}
	}

	// Traceloop specific: retriever namespace
	if _, ok := attrs["retriever.query"].(string); ok {
		return true
	}

	// Check for vector search attributes
	if _, ok := attrs["vector.query"].(string); ok {
		return true
	}

	return false
}

// hasRerankAttributes checks if span has reranking attributes
func hasRerankAttributes(attrs map[string]interface{}) bool {
	// Check for rerank operation
	if opName, ok := attrs["gen_ai.operation.name"].(string); ok {
		if opName == "rerank" || opName == "reranking" {
			return true
		}
	}

	// Traceloop specific
	if _, ok := attrs["rerank.model"].(string); ok {
		return true
	}

	// Check for reranker model names
	if model, ok := attrs["gen_ai.request.model"].(string); ok {
		// Common reranker models
		if model == "rerank-english-v2.0" || model == "rerank-multilingual-v2.0" {
			return true
		}
	}

	return false
}

// hasAgentAttributes checks if span has agent orchestration attributes
func hasAgentAttributes(attrs map[string]interface{}, spanName string) bool {
	// Check traceloop.span.kind attribute
	if kind, ok := attrs["traceloop.span.kind"].(string); ok {
		kindLower := strings.ToLower(kind)
		if kindLower == "agent" {
			return true
		}
	}

	// Check the span name suffix (after the last dot)
	// Example: "my_agent.agent" -> "agent"
	if spanName != "" {
		parts := strings.Split(spanName, ".")
		if len(parts) > 0 {
			lastPart := strings.ToLower(parts[len(parts)-1])
			if lastPart == "agent" {
				return true
			}
		}
	}

	// Check for agent name attribute
	if _, ok := attrs["agent.name"].(string); ok {
		return true
	}

	// Check for workflow/agent type
	if spanType, ok := attrs["traceloop.entity.type"].(string); ok {
		spanTypeLower := strings.ToLower(spanType)
		if spanTypeLower == "agent" || spanTypeLower == "workflow" {
			return true
		}
	}

	return false
}

// hasTaskAttributes checks if span has task/workflow attributes
func hasTaskAttributes(attrs map[string]interface{}, spanName string) bool {
	// Check traceloop.span.kind attribute
	if kind, ok := attrs["traceloop.span.kind"].(string); ok {
		kindLower := strings.ToLower(kind)
		if kindLower == "task" || kindLower == "workflow" {
			return true
		}
	}

	// Check the span name suffix (after the last dot)
	// Example: "tools_condition.task" -> "task"
	if spanName != "" {
		parts := strings.Split(spanName, ".")
		if len(parts) > 0 {
			lastPart := strings.ToLower(parts[len(parts)-1])
			if lastPart == "task" || lastPart == "workflow" {
				return true
			}
		}
	}

	// Check for workflow-related attributes as fallback
	if _, ok := attrs["workflow.name"].(string); ok {
		return true
	}

	return false
}
