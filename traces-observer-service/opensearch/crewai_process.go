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
	"fmt"
	"strings"
)

// IsCrewAISpan checks if a span is from CrewAI framework
// It verifies both gen_ai.system == "crewai" and the presence of crewai.* attributes
func IsCrewAISpan(attrs map[string]interface{}) bool {
	if attrs == nil {
		return false
	}

	// Check if gen_ai.system is "crewai"
	if val, ok := attrs["gen_ai.system"].(string); ok && strings.ToLower(val) == "crewai" {
		return true
	}

	// Verify that at least one crewai.* attribute exists
	for key := range attrs {
		if strings.HasPrefix(key, "crewai.") {
			return true
		}
	}

	return false
}

// ExtractCrewAISpanInputOutput extracts input and output from CrewAI span attributes
// This is a generic method that works for any CrewAI span (workflow, task, or agent)
// Input: crewai.crew.tasks_output - contains task outputs
// Output: crewai.crew.result - contains the result
// Returns nil when attributes are not found
func ExtractCrewAISpanInputOutput(attrs map[string]interface{}) (input interface{}, output interface{}) {
	// Return nil if no attributes
	if attrs == nil {
		return nil, nil
	}

	// Extract input from crewai.crew.tasks_output
	if tasksVal, ok := attrs["crewai.crew.tasks_output"]; ok {
		if tasksStr, ok := tasksVal.(string); ok {
			input = tasksStr
		}
	}

	// Extract output from crewai.crew.result
	if resultVal, ok := attrs["crewai.crew.result"]; ok {
		if resultStr, ok := resultVal.(string); ok {
			output = resultStr
		}
	}

	return input, output
}

// ExtractCrewAIRootSpanInputOutput extracts input and output from CrewAI workflow spans
// Input: crewai.crew.tasks - contains task definitions for all agents
// Output: crewai.crew.result - contains the final workflow result
// Returns nil when attributes are not found
func ExtractCrewAIRootSpanInputOutput(rootSpan *Span) (input interface{}, output interface{}) {
	if rootSpan == nil || rootSpan.Attributes == nil {
		return nil, nil
	}

	// Use the generic extraction method
	return ExtractCrewAISpanInputOutput(rootSpan.Attributes)
}

// PopulateCrewAIAgentAttributes extracts and populates CrewAI-specific agent attributes
func PopulateCrewAIAgentAttributes(ampAttrs *AmpAttributes, attrs map[string]interface{}) {
	// Set agent-specific data
	agentData := AgentData{
		Framework: "crewai",
	}

	// Extract input and output using the generic method
	ampAttrs.Input, ampAttrs.Output = ExtractCrewAISpanInputOutput(attrs)

	// Extract workflow/agent name
	// First try crewai.agent.role (for individual agent spans)
	if name, ok := attrs["crewai.agent.role"].(string); ok {
		agentData.Name = strings.TrimSpace(name)
	} else if name, ok := attrs["crewai.crew.name"].(string); ok {
		// Fallback to crewai.crew.name (for crew/workflow spans)
		agentData.Name = name
	}

	// Extract tools from crewai.agent.tools (for agent spans)
	agentData.Tools = extractCrewAIAgentTools(attrs)

	// Extract system prompt from crewai.agent.* attributes
	agentData.SystemPrompt = extractCrewAISystemPrompt(attrs)

	// Extract max iterations from crewai.agent.max_iter
	if maxIter, ok := attrs["crewai.agent.max_iter"].(float64); ok {
		agentData.MaxIter = int(maxIter)
	}

	// Extract token usage from crewai.crew.token_usage
	// Format: "total_tokens=57062 prompt_tokens=46376 cached_prompt_tokens=0 completion_tokens=10686 successful_requests=10"
	if tokenUsageStr, ok := attrs["crewai.crew.token_usage"].(string); ok {
		agentData.TokenUsage = parseCrewAITokenUsage(tokenUsageStr)
	}

	ampAttrs.Data = agentData
}

// extractCrewAIAgentTools extracts tool definitions from crewai.agent.tools attribute
// Uses the common parseToolsJSON method to handle multiple formats:
// - JSON array of tool names: ["tool1", "tool2"]
// - JSON array of tool objects: [{"name": "tool1", "description": "...", "parameters": "..."}]
// Returns array of ToolDefinition objects
func extractCrewAIAgentTools(attrs map[string]interface{}) []ToolDefinition {
	toolsJSON, ok := attrs["crewai.agent.tools"].(string)
	if !ok || toolsJSON == "" {
		return nil
	}

	// Use the common tool parsing method from process.go
	return parseToolsJSON(toolsJSON)
}

// extractCrewAISystemPrompt extracts and formats system prompt from CrewAI agent attributes
// Combines crewai.agent.role, crewai.agent.goal, and crewai.agent.backstory into a formatted prompt
func extractCrewAISystemPrompt(attrs map[string]interface{}) string {
	var role, goal, backstory string

	// Extract role
	if r, ok := attrs["crewai.agent.role"].(string); ok {
		role = strings.TrimSpace(r)
	}

	// Extract goal
	if g, ok := attrs["crewai.agent.goal"].(string); ok {
		goal = strings.TrimSpace(g)
	}

	// Extract backstory
	if b, ok := attrs["crewai.agent.backstory"].(string); ok {
		backstory = strings.TrimSpace(b)
	}

	// Only create system prompt if we have at least one field
	if role == "" && goal == "" && backstory == "" {
		return ""
	}

	// Build formatted system prompt
	var prompt strings.Builder

	if role != "" {
		prompt.WriteString("role: >\n  ")
		prompt.WriteString(role)
		prompt.WriteString("\n")
	}

	if goal != "" {
		prompt.WriteString("goal: >\n  ")
		prompt.WriteString(goal)
		prompt.WriteString("\n")
	}

	if backstory != "" {
		prompt.WriteString("backstory: >\n  ")
		prompt.WriteString(backstory)
	}

	return strings.TrimSpace(prompt.String())
}

// parseCrewAITokenUsage parses CrewAI token usage string into LLMTokenUsage struct
// Format: "total_tokens=57062 prompt_tokens=46376 cached_prompt_tokens=0 completion_tokens=10686 successful_requests=10"
func parseCrewAITokenUsage(tokenUsageStr string) *LLMTokenUsage {
	if tokenUsageStr == "" {
		return nil
	}

	usage := &LLMTokenUsage{}

	// Split by space and parse key=value pairs
	pairs := strings.Fields(tokenUsageStr)
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		// Parse numeric values
		var numValue int
		if _, err := fmt.Sscanf(value, "%d", &numValue); err != nil {
			continue
		}

		switch key {
		case "total_tokens":
			usage.TotalTokens = numValue
		case "prompt_tokens":
			usage.InputTokens = numValue
		case "completion_tokens":
			usage.OutputTokens = numValue
		case "cached_prompt_tokens":
			usage.CacheReadInputTokens = numValue
		}
	}

	// Only return if we have valid data
	if usage.TotalTokens > 0 || usage.InputTokens > 0 || usage.OutputTokens > 0 {
		return usage
	}

	return nil
}
