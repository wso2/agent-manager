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

	// Populate span-type-specific attributes
	if span.Attributes != nil {
		switch spanType {
		case SpanTypeLLM:
			populateLLMAttributes(ampAttrs, span.Attributes)
		case SpanTypeTool:
			populateToolAttributes(ampAttrs, span.Attributes, span.Status)
		case SpanTypeEmbedding:
			populateEmbeddingAttributes(ampAttrs, span.Attributes)
		case SpanTypeRetriever:
			populateRetrieverAttributes(ampAttrs, span.Attributes)
		case SpanTypeAgent:
			// Check if this is a CrewAI workflow span and delegate to CrewAI processor
			if IsCrewAISpan(span.Attributes) {
				PopulateCrewAIAgentAttributes(ampAttrs, span.Attributes)
			} else {
				populateAgentAttributes(ampAttrs, span.Attributes)
			}
		case SpanTypeChain:
			populateChainAttributes(ampAttrs, span.Attributes)
		}

	}

	// Extract error status for all span types
	ampAttrs.Status = extractSpanStatus(span.Attributes, span.Status)
	span.AmpAttributes = ampAttrs

	return span
}

// populateLLMAttributes extracts and populates LLM-specific attributes
func populateLLMAttributes(ampAttrs *AmpAttributes, attrs map[string]interface{}) {
	// Set common Input/Output fields
	ampAttrs.Input = ExtractPromptMessages(attrs)
	ampAttrs.Output = ExtractCompletionMessages(attrs)

	// Set LLM-specific data
	llmData := LLMData{
		Tools: ExtractToolDefinitions(attrs),
	}

	// Extract model information
	if responseModel, ok := attrs["gen_ai.response.model"].(string); ok {
		llmData.Model = responseModel
	} else if requestModel, ok := attrs["gen_ai.request.model"].(string); ok {
		llmData.Model = requestModel
	}

	// Extract vendor (gen_ai.system)
	if vendor, ok := attrs["gen_ai.system"].(string); ok {
		llmData.Vendor = vendor
	}

	// Extract temperature
	if temp, ok := attrs["gen_ai.request.temperature"].(float64); ok {
		llmData.Temperature = &temp
	}

	// Extract token usage
	llmData.TokenUsage = extractTokenUsageFromAttributes(attrs)

	ampAttrs.Data = llmData
}

// populateToolAttributes extracts and populates tool-specific attributes
func populateToolAttributes(ampAttrs *AmpAttributes, attrs map[string]interface{}, spanStatus string) {
	name, toolInput, toolOutput, _ := ExtractToolExecutionDetails(attrs, spanStatus)

	// Set common Input/Output fields
	ampAttrs.Input = toolInput
	ampAttrs.Output = toolOutput

	// Set tool-specific data
	toolData := ToolData{
		Name: name,
	}

	ampAttrs.Data = toolData
}

// populateEmbeddingAttributes extracts and populates embedding-specific attributes
func populateEmbeddingAttributes(ampAttrs *AmpAttributes, attrs map[string]interface{}) {
	// Set common Input field (documents to embed)
	ampAttrs.Input = ExtractEmbeddingDocuments(attrs)

	// Set embedding-specific data
	embeddingData := EmbeddingData{}

	// Extract model information
	if responseModel, ok := attrs["gen_ai.response.model"].(string); ok {
		embeddingData.Model = responseModel
	} else if requestModel, ok := attrs["gen_ai.request.model"].(string); ok {
		embeddingData.Model = requestModel
	}

	// Extract vendor (gen_ai.system)
	if vendor, ok := attrs["gen_ai.system"].(string); ok {
		embeddingData.Vendor = vendor
	}

	// Extract token usage
	embeddingData.TokenUsage = extractTokenUsageFromAttributes(attrs)

	ampAttrs.Data = embeddingData
}

// populateRetrieverAttributes extracts and populates retriever/vector DB-specific attributes
func populateRetrieverAttributes(ampAttrs *AmpAttributes, attrs map[string]interface{}) {
	retrieverData := RetrieverData{}

	// Extract vector DB system
	if dbSystem, ok := attrs["db.system"].(string); ok {
		retrieverData.VectorDB = dbSystem
	}

	// Extract top_k parameter
	if topK, ok := attrs["db.vector.query.top_k"].(float64); ok {
		retrieverData.TopK = int(topK)
	}

	ampAttrs.Data = retrieverData
}

// populateAgentAttributes extracts and populates agent-specific attributes
func populateAgentAttributes(ampAttrs *AmpAttributes, attrs map[string]interface{}) {
	// For standard agent spans, use traceloop.entity attributes
	if input, ok := attrs["traceloop.entity.input"].(string); ok {
		ampAttrs.Input = input
	}
	if output, ok := attrs["traceloop.entity.output"].(string); ok {
		ampAttrs.Output = output
	}

	// Set agent-specific data
	agentData := AgentData{}

	// Extract agent name from gen_ai.agent.name
	if name, ok := attrs["gen_ai.agent.name"].(string); ok {
		agentData.Name = name
	}

	// Extract agent tools from gen_ai.agent.tools
	agentData.Tools = extractAgentTools(attrs)

	// Extract model from gen_ai.request.model
	if model, ok := attrs["gen_ai.request.model"].(string); ok {
		agentData.Model = model
	}

	// Extract framework from gen_ai.system
	if framework, ok := attrs["gen_ai.system"].(string); ok {
		agentData.Framework = framework
	}

	// Extract system prompt
	agentData.SystemPrompt = extractAgentSystemPrompt(attrs)

	// Extract token usage
	agentData.TokenUsage = extractTokenUsageFromAttributes(attrs)

	ampAttrs.Data = agentData
}

// populateChainAttributes extracts and populates chain/task/workflow-specific attributes
func populateChainAttributes(ampAttrs *AmpAttributes, attrs map[string]interface{}) {
	// Check if this is a CrewAI chain/task span and delegate to CrewAI processor
	if IsCrewAISpan(attrs) {
		// Extract input and output using CrewAI extraction
		ampAttrs.Input, ampAttrs.Output = ExtractCrewAISpanInputOutput(attrs)
		return
	}

	// For standard chain/task spans, extract from traceloop.entity attributes
	ampAttrs.Input, ampAttrs.Output = extractSpanInputOutput(attrs)
}

// parseToolsJSON is a common method to parse tools from JSON string
// Supports two formats:
// 1. Array of strings: ["tool1", "tool2"] -> creates ToolDefinition with just name
// 2. Array of tool objects: [{"name": "tool1", "description": "...", "parameters": "..."}]
// Returns array of ToolDefinition objects
func parseToolsJSON(toolsJSON string) []ToolDefinition {
	if toolsJSON == "" {
		return nil
	}

	// First, try to parse as array of ToolDefinition objects
	var toolDefs []ToolDefinition
	if err := json.Unmarshal([]byte(toolsJSON), &toolDefs); err == nil && len(toolDefs) > 0 {
		// Successfully parsed as ToolDefinition array
		return toolDefs
	}

	// If that fails, try to parse as array of strings
	var toolNames []string
	if err := json.Unmarshal([]byte(toolsJSON), &toolNames); err == nil {
		// Successfully parsed as string array, convert to ToolDefinition
		result := make([]ToolDefinition, len(toolNames))
		for i, name := range toolNames {
			result[i] = ToolDefinition{Name: name}
		}
		return result
	}

	// If both fail, return the raw string as a single ToolDefinition
	return []ToolDefinition{{Name: toolsJSON}}
}

// extractAgentTools extracts tool definitions from gen_ai.agent.tools attribute
// The attribute can contain:
// - JSON array of tool names: ["tool1", "tool2"]
// - JSON array of tool objects: [{"name": "tool1", "description": "...", "parameters": "..."}]
// Returns array of ToolDefinition objects
func extractAgentTools(attrs map[string]interface{}) []ToolDefinition {
	toolsJSON, ok := attrs["gen_ai.agent.tools"].(string)
	if !ok || toolsJSON == "" {
		return nil
	}

	return parseToolsJSON(toolsJSON)
}

// extractSystemPrompt extracts the system prompt for an agent
func extractAgentSystemPrompt(attrs map[string]interface{}) string {
	// Look for system prompt in various possible locations

	// First check gen_ai.system_instructions (OTEL format)
	// Can be a JSON array of instruction parts
	if systemInstructions, ok := attrs["gen_ai.system_instructions"].(string); ok && systemInstructions != "" {
		// Try to parse as JSON array first
		var instructions []map[string]interface{}
		if err := json.Unmarshal([]byte(systemInstructions), &instructions); err == nil {
			// Extract text content from parts
			var parts []string
			for _, instruction := range instructions {
				if partType, ok := instruction["type"].(string); ok && partType == "text" {
					if content, ok := instruction["content"].(string); ok && content != "" {
						parts = append(parts, content)
					}
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, "\n")
			}
		}
		// If not JSON or parsing failed, return as-is
		return systemInstructions
	}

	// Check gen_ai.prompt.0.content with role=system (Traceloop format)
	if role, ok := attrs["gen_ai.prompt.0.role"].(string); ok && role == "system" {
		if content, ok := attrs["gen_ai.prompt.0.content"].(string); ok {
			return content
		}
	}

	// Check for a dedicated system_prompt attribute if it exists
	if systemPrompt, ok := attrs["system_prompt"].(string); ok {
		return systemPrompt
	}

	return ""
}

// extractSpanStatus determines the error status of a span
func extractSpanStatus(attrs map[string]interface{}, spanStatus string) *SpanStatus {
	status := &SpanStatus{
		Error: false,
	}

	if attrs != nil {
		if errorType, ok := attrs["error.type"].(string); ok {
			status.Error = true
			status.ErrorType = errorType
		} else if toolStatus, ok := attrs["gen_ai.tool.status"].(string); ok && isErrorStatus(toolStatus) {
			status.Error = true
			status.ErrorType = "ToolExecutionError"
		}
	} else if isErrorStatus(spanStatus) {
		// Handle case where attributes are nil but status is still error
		status.Error = true
	}

	return status
}

// extractTokenUsageFromAttributes extracts token usage from span attributes
// Supports both standard gen_ai.usage.* and legacy prompt_tokens/completion_tokens attributes
func extractTokenUsageFromAttributes(attrs map[string]interface{}) *LLMTokenUsage {
	var inputTokens, outputTokens, cacheReadTokens int

	// Try to extract input tokens (gen_ai.usage.input_tokens or gen_ai.usage.prompt_tokens)
	if val, ok := attrs["gen_ai.usage.input_tokens"].(float64); ok {
		inputTokens = int(val)
	} else if val, ok := attrs["gen_ai.usage.prompt_tokens"].(float64); ok {
		inputTokens = int(val)
	}

	// Try to extract output tokens (gen_ai.usage.output_tokens or gen_ai.usage.completion_tokens)
	if val, ok := attrs["gen_ai.usage.output_tokens"].(float64); ok {
		outputTokens = int(val)
	} else if val, ok := attrs["gen_ai.usage.completion_tokens"].(float64); ok {
		outputTokens = int(val)
	}

	// Try to extract cache read tokens
	if val, ok := attrs["gen_ai.usage.cache_read_input_tokens"].(float64); ok {
		cacheReadTokens = int(val)
	}

	// Only return token usage if we found some tokens
	if inputTokens > 0 || outputTokens > 0 {
		return &LLMTokenUsage{
			InputTokens:          inputTokens,
			OutputTokens:         outputTokens,
			CacheReadInputTokens: cacheReadTokens,
			TotalTokens:          inputTokens + outputTokens,
		}
	}

	return nil
}

// ExtractTokenUsage aggregates token usage from GenAI spans in a trace
func ExtractTokenUsage(spans []Span) *TokenUsage {
	var inputTokens, outputTokens int

	for _, span := range spans {
		// Check if this is a GenAI span by looking for gen_ai.* attributes
		if span.Attributes != nil {
			// Use the helper method to extract token usage from attributes
			if usage := extractTokenUsageFromAttributes(span.Attributes); usage != nil {
				inputTokens += usage.InputTokens
				outputTokens += usage.OutputTokens
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
		// Check for errors using the same logic as individual span processing
		hasError := false

		if span.Attributes != nil {
			if _, ok := span.Attributes["error.type"].(string); ok {
				hasError = true
			} else if toolStatus, ok := span.Attributes["gen_ai.tool.status"].(string); ok && isErrorStatus(toolStatus) {
				hasError = true
			}
		}

		// Fallback to span status if no error attributes found
		if !hasError && isErrorStatus(span.Status) {
			hasError = true
		}

		if hasError {
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

// extractSpanInputOutput extracts input and output from traceloop.entity.* attributes
// This is a generic method that works for any span with traceloop.entity.input and traceloop.entity.output
// Input path: traceloop.entity.input -> inputs (ensure it's JSON)
// Output path: traceloop.entity.output -> outputs -> messages[-1] -> kwargs -> content
// Returns nil when attributes are not found
func extractSpanInputOutput(attrs map[string]interface{}) (input interface{}, output interface{}) {
	// Return nil if no attributes
	if attrs == nil {
		return nil, nil
	}

	// Extract input from traceloop.entity.input attribute
	// Path: input -> inputs (make sure it's JSON)
	if inputVal, ok := attrs["traceloop.entity.input"]; ok {
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
	if outputVal, ok := attrs["traceloop.entity.output"]; ok {
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

// ExtractRootSpanInputOutput extracts input and output from the root span (parent node)
// by analyzing the "traceloop.entity.input" and "traceloop.entity.output" attributes
// Input path: traceloop.entity.input -> inputs (ensure it's JSON)
// Output path: traceloop.entity.output -> outputs -> messages[-1] -> kwargs -> content
// For CrewAI workflows: delegates to ExtractCrewAIRootSpanInputOutput
// Returns nil when attributes are not found
func ExtractRootSpanInputOutput(rootSpan *Span) (input interface{}, output interface{}) {
	if rootSpan == nil || rootSpan.Attributes == nil {
		return nil, nil
	}

	// Use the generic extraction method
	return extractSpanInputOutput(rootSpan.Attributes)
}

// ExtractPromptMessages extracts and orders prompt messages from LLM span attributes
// Handles two formats:
// 1. OTEL format: gen_ai.input.messages (JSON array)
// 2. Traceloop format: gen_ai.prompt.{index}.{field}
func ExtractPromptMessages(attrs map[string]interface{}) []PromptMessage {
	// First, try OTEL format (gen_ai.input.messages)
	if messagesJSON, ok := attrs["gen_ai.input.messages"].(string); ok && messagesJSON != "" {
		messages := parseOTELMessages(messagesJSON)
		if len(messages) > 0 {
			return messages
		}
	}

	// Fallback to Traceloop format (gen_ai.prompt.*)
	return extractTraceloopPromptMessages(attrs)
}

// parseOTELMessages parses OTEL format messages from JSON string
// Format: [{"role": "user", "parts": [{"type": "text", "content": "..."}, {"type": "tool_call", ...}]}]
func parseOTELMessages(messagesJSON string) []PromptMessage {
	if messagesJSON == "" {
		return nil
	}

	// Parse JSON array
	var rawMessages []map[string]interface{}
	if err := json.Unmarshal([]byte(messagesJSON), &rawMessages); err != nil {
		return nil
	}

	messages := make([]PromptMessage, 0, len(rawMessages))
	for _, rawMsg := range rawMessages {
		msg := PromptMessage{}

		// Extract role
		if role, ok := rawMsg["role"].(string); ok {
			msg.Role = role
		}

		// Extract parts
		if parts, ok := rawMsg["parts"].([]interface{}); ok {
			var contentParts []string
			var toolCalls []ToolCall

			for _, part := range parts {
				if partMap, ok := part.(map[string]interface{}); ok {
					partType, _ := partMap["type"].(string)

					switch partType {
					case "text":
						// Extract text content
						if content, ok := partMap["content"].(string); ok && content != "" {
							contentParts = append(contentParts, content)
						}

					case "tool_call":
						// Extract tool call
						tc := ToolCall{}
						if id, ok := partMap["id"].(string); ok {
							tc.ID = id
						}
						if name, ok := partMap["name"].(string); ok {
							tc.Name = name
						}
						// Arguments can be object or string
						if args, ok := partMap["arguments"]; ok {
							if argsStr, ok := args.(string); ok {
								tc.Arguments = argsStr
							} else {
								// Convert to JSON string
								if argsBytes, err := json.Marshal(args); err == nil {
									tc.Arguments = string(argsBytes)
								}
							}
						}
						if tc.Name != "" {
							toolCalls = append(toolCalls, tc)
						}

					case "tool_call_response":
						// For tool role messages, extract result as content
						if result, ok := partMap["result"].(string); ok && result != "" {
							contentParts = append(contentParts, result)
						}
					}
				}
			}

			// Combine content parts
			if len(contentParts) > 0 {
				msg.Content = strings.Join(contentParts, "\n")
			}

			// Add tool calls
			if len(toolCalls) > 0 {
				msg.ToolCalls = toolCalls
			}
		}

		// Only add message if it has a role
		if msg.Role != "" {
			messages = append(messages, msg)
		}
	}

	return messages
}

// extractTraceloopPromptMessages extracts prompt messages in Traceloop format
// Format: gen_ai.prompt.{index}.{field}
func extractTraceloopPromptMessages(attrs map[string]interface{}) []PromptMessage {
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
							switch toolField {
							case "id":
								if id, ok := value.(string); ok {
									toolCallsMap[msgIndex][toolIndex].ID = id
								}
							case "name":
								if name, ok := value.(string); ok {
									toolCallsMap[msgIndex][toolIndex].Name = name
								}
							case "arguments":
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
// Handles two formats:
// 1. OTEL format: gen_ai.output.messages (JSON array)
// 2. Traceloop format: gen_ai.completion.{index}.{field}
func ExtractCompletionMessages(attrs map[string]interface{}) []PromptMessage {
	// First, try OTEL format (gen_ai.output.messages)
	if messagesJSON, ok := attrs["gen_ai.output.messages"].(string); ok && messagesJSON != "" {
		messages := parseOTELMessages(messagesJSON)
		if len(messages) > 0 {
			return messages
		}
	}

	// Fallback to Traceloop format (gen_ai.completion.*)
	return extractTraceloopCompletionMessages(attrs)
}

// extractTraceloopCompletionMessages extracts completion messages in Traceloop format
// Format: gen_ai.completion.{index}.{field}
func extractTraceloopCompletionMessages(attrs map[string]interface{}) []PromptMessage {
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
							switch toolField {
							case "id":
								if id, ok := value.(string); ok {
									toolCallsMap[msgIndex][toolIndex].ID = id
								}
							case "name":
								if name, ok := value.(string); ok {
									toolCallsMap[msgIndex][toolIndex].Name = name
								}
							case "arguments":
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
// Handles two formats:
// 1. OTEL format: gen_ai.tool.definitions (JSON array)
// 2. Traceloop format: llm.request.functions.{index}.{field}
func ExtractToolDefinitions(attrs map[string]interface{}) []ToolDefinition {
	// First, try OTEL format (gen_ai.tool.definitions)
	if toolsJSON, ok := attrs["gen_ai.tool.definitions"].(string); ok && toolsJSON != "" {
		tools := parseOTELToolDefinitions(toolsJSON)
		if len(tools) > 0 {
			return tools
		}
	}

	// Fallback to Traceloop format (llm.request.functions.*)
	return extractTraceloopToolDefinitions(attrs)
}

// parseOTELToolDefinitions parses OTEL format tool definitions from JSON string
// Format: [{"type": "function", "name": "...", "description": "...", "parameters": {...}}]
func parseOTELToolDefinitions(toolsJSON string) []ToolDefinition {
	if toolsJSON == "" {
		return nil
	}

	// Parse JSON array
	var rawTools []map[string]interface{}
	if err := json.Unmarshal([]byte(toolsJSON), &rawTools); err != nil {
		return nil
	}

	tools := make([]ToolDefinition, 0, len(rawTools))
	for _, rawTool := range rawTools {
		tool := ToolDefinition{}

		// Extract name
		if name, ok := rawTool["name"].(string); ok {
			tool.Name = name
		}

		// Extract description
		if desc, ok := rawTool["description"].(string); ok {
			tool.Description = desc
		}

		// Extract parameters (convert to JSON string)
		if params, ok := rawTool["parameters"]; ok {
			if paramsStr, ok := params.(string); ok {
				tool.Parameters = paramsStr
			} else {
				// Convert to JSON string
				if paramsBytes, err := json.Marshal(params); err == nil {
					tool.Parameters = string(paramsBytes)
				}
			}
		}

		// Only add tool if it has a name
		if tool.Name != "" {
			tools = append(tools, tool)
		}
	}

	return tools
}

// extractTraceloopToolDefinitions extracts tool definitions in Traceloop format
// Format: llm.request.functions.{index}.{field}
func extractTraceloopToolDefinitions(attrs map[string]interface{}) []ToolDefinition {
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
	} else if toolID, ok := attrs["tool_name"].(string); ok { // crewai legacy
		name = toolID
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

// ExtractEmbeddingDocuments extracts documents from embedding span attributes
// Looks for gen_ai.prompt.N.content attributes and returns them as a slice
func ExtractEmbeddingDocuments(attrs map[string]interface{}) []string {
	// Map to store documents by index
	documentMap := make(map[int]string)
	maxIndex := -1

	// Iterate through attributes to find gen_ai.prompt.N.content
	for key, value := range attrs {
		// Check if it's a gen_ai.prompt.*.content attribute
		if strings.HasPrefix(key, "gen_ai.prompt.") && strings.HasSuffix(key, ".content") {
			// Parse the index
			// Format: gen_ai.prompt.{index}.content
			parts := strings.Split(key, ".")
			if len(parts) == 4 {
				var index int
				if _, err := fmt.Sscanf(parts[2], "%d", &index); err == nil {
					// Convert value to string using fmt.Sprint
					content := fmt.Sprint(value)
					if content != "" {
						documentMap[index] = content
						if index > maxIndex {
							maxIndex = index
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

	documents := make([]string, 0, maxIndex+1)
	for i := 0; i <= maxIndex; i++ {
		if doc, exists := documentMap[i]; exists {
			documents = append(documents, doc)
		}
	}

	return documents
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

	// Check for Tool/Function calls
	if hasToolAttributes(span.Attributes) {
		return SpanTypeTool
	}

	// Check for Agent orchestration
	if hasAgentAttributes(span.Attributes) {
		return SpanTypeAgent
	}

	// Check for Embedding operations
	if hasEmbeddingAttributes(span.Attributes) {
		return SpanTypeEmbedding
	}

	// Check for Retriever operations
	if hasRetrieverAttributes(span.Attributes) {
		return SpanTypeRetriever
	}

	// Check for Rerank operations
	if hasRerankAttributes(span.Attributes) {
		return SpanTypeRerank
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

func hasLLMAttributes(attrs map[string]interface{}) bool {
	// Check for gen_ai.operation.name (as requested)
	if opName, ok := attrs["gen_ai.operation.name"].(string); ok {
		if opName == "chat" || opName == "completion" || opName == "text_completion" {
			return true
		}
	}

	// Check for gen_ai.prompt (Starting with this as requested)
	if _, ok := attrs["gen_ai.prompt"]; ok {
		return true
	}

	// Check for response attributes specific to LLM (finish reasons, etc.)
	if _, ok := attrs["gen_ai.response.finish_reasons"]; ok {
		return true
	}

	// Traceloop / Legacy compatibility (excluding embeddings)
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

	// CrewAI specific: function.* namespace
	if _, ok := attrs["tool_name"].(string); ok {
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
	// Check for Traceloop vector DB query attributes (db.query.*)
	for key := range attrs {
		if strings.HasPrefix(key, "db.query.") {
			return true
		}
	}

	// Check for vector database system
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
		// Common reranker models - check if model name contains these patterns
		if strings.Contains(model, "rerank-english") || strings.Contains(model, "rerank-multilingual") {
			return true
		}
	}

	return false
}

// hasAgentAttributes checks if span has agent orchestration attributes
func hasAgentAttributes(attrs map[string]interface{}) bool {
	val, ok := attrs["gen_ai.agent.name"]
	if !ok || val == nil {
		return false
	}

	// Type assert to string and check length
	strVal, isString := val.(string)
	return isString && len(strVal) > 0
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
