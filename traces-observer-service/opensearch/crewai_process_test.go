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
	"strings"
	"testing"
)

func TestIsCrewAISpan(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]interface{}
		want  bool
	}{
		{
			name:  "nil attributes",
			attrs: nil,
			want:  false,
		},
		{
			name:  "empty attributes",
			attrs: map[string]interface{}{},
			want:  false,
		},
		{
			name: "gen_ai.system is crewai",
			attrs: map[string]interface{}{
				"gen_ai.system": "crewai",
			},
			want: true,
		},
		{
			name: "gen_ai.system is CrewAI (case insensitive)",
			attrs: map[string]interface{}{
				"gen_ai.system": "CrewAI",
			},
			want: true,
		},
		{
			name: "has crewai.* attribute",
			attrs: map[string]interface{}{
				"crewai.agent.role": "researcher",
			},
			want: true,
		},
		{
			name: "no crewai indicators",
			attrs: map[string]interface{}{
				"gen_ai.system":    "openai",
				"gen_ai.agent.name": "my-agent",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCrewAISpan(tt.attrs); got != tt.want {
				t.Errorf("IsCrewAISpan() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractCrewAISpanInputOutput(t *testing.T) {
	t.Run("extracts tasks_output and result", func(t *testing.T) {
		attrs := map[string]interface{}{
			"crewai.crew.tasks_output": "task output data",
			"crewai.crew.result":       "final result",
		}
		input, output := ExtractCrewAISpanInputOutput(attrs)
		if input != "task output data" {
			t.Errorf("expected input 'task output data', got %v", input)
		}
		if output != "final result" {
			t.Errorf("expected output 'final result', got %v", output)
		}
	})

	t.Run("nil attributes", func(t *testing.T) {
		input, output := ExtractCrewAISpanInputOutput(nil)
		if input != nil || output != nil {
			t.Errorf("expected nil/nil, got %v/%v", input, output)
		}
	})

	t.Run("missing attributes", func(t *testing.T) {
		attrs := map[string]interface{}{
			"some.other": "value",
		}
		input, output := ExtractCrewAISpanInputOutput(attrs)
		if input != nil || output != nil {
			t.Errorf("expected nil/nil, got %v/%v", input, output)
		}
	})
}

func TestExtractCrewAIRootSpanInputOutput(t *testing.T) {
	t.Run("nil span", func(t *testing.T) {
		input, output := ExtractCrewAIRootSpanInputOutput(nil)
		if input != nil || output != nil {
			t.Errorf("expected nil/nil, got %v/%v", input, output)
		}
	})

	t.Run("valid span", func(t *testing.T) {
		span := &Span{
			Attributes: map[string]interface{}{
				"crewai.crew.tasks_output": "tasks",
				"crewai.crew.result":       "result",
			},
		}
		input, output := ExtractCrewAIRootSpanInputOutput(span)
		if input != "tasks" {
			t.Errorf("expected 'tasks', got %v", input)
		}
		if output != "result" {
			t.Errorf("expected 'result', got %v", output)
		}
	})
}

func TestPopulateCrewAIAgentAttributes(t *testing.T) {
	t.Run("full CrewAI agent attributes", func(t *testing.T) {
		ampAttrs := &AmpAttributes{}
		attrs := map[string]interface{}{
			"crewai.agent.role":      "Researcher",
			"crewai.agent.goal":      "Find information",
			"crewai.agent.backstory": "Expert researcher with years of experience",
			"crewai.agent.tools":     `["search", "browse"]`,
			"crewai.agent.max_iter":  float64(10),
			"crewai.crew.token_usage": "total_tokens=1000 prompt_tokens=800 completion_tokens=200 cached_prompt_tokens=50",
			"crewai.crew.tasks_output": "task data",
			"crewai.crew.result":       "final result",
		}

		PopulateCrewAIAgentAttributes(ampAttrs, attrs)

		agentData, ok := ampAttrs.Data.(AgentData)
		if !ok {
			t.Fatal("expected AgentData type")
		}

		if agentData.Framework != "crewai" {
			t.Errorf("expected framework 'crewai', got %q", agentData.Framework)
		}
		if agentData.Name != "Researcher" {
			t.Errorf("expected name 'Researcher', got %q", agentData.Name)
		}
		if len(agentData.Tools) != 2 {
			t.Fatalf("expected 2 tools, got %d", len(agentData.Tools))
		}
		if agentData.Tools[0].Name != "search" {
			t.Errorf("expected tool 'search', got %q", agentData.Tools[0].Name)
		}
		if agentData.MaxIter != 10 {
			t.Errorf("expected maxIter 10, got %d", agentData.MaxIter)
		}
		if agentData.TokenUsage == nil {
			t.Fatal("expected token usage, got nil")
		}
		if agentData.TokenUsage.TotalTokens != 1000 {
			t.Errorf("expected total tokens 1000, got %d", agentData.TokenUsage.TotalTokens)
		}
		if agentData.TokenUsage.InputTokens != 800 {
			t.Errorf("expected input tokens 800, got %d", agentData.TokenUsage.InputTokens)
		}
		if agentData.TokenUsage.OutputTokens != 200 {
			t.Errorf("expected output tokens 200, got %d", agentData.TokenUsage.OutputTokens)
		}
		if agentData.TokenUsage.CacheReadInputTokens != 50 {
			t.Errorf("expected cache read tokens 50, got %d", agentData.TokenUsage.CacheReadInputTokens)
		}

		// Check system prompt
		if !strings.Contains(agentData.SystemPrompt, "Researcher") {
			t.Errorf("expected system prompt to contain 'Researcher', got %q", agentData.SystemPrompt)
		}
		if !strings.Contains(agentData.SystemPrompt, "Find information") {
			t.Errorf("expected system prompt to contain 'Find information', got %q", agentData.SystemPrompt)
		}

		// Check input/output
		if ampAttrs.Input != "task data" {
			t.Errorf("expected input 'task data', got %v", ampAttrs.Input)
		}
		if ampAttrs.Output != "final result" {
			t.Errorf("expected output 'final result', got %v", ampAttrs.Output)
		}
	})

	t.Run("crew name fallback", func(t *testing.T) {
		ampAttrs := &AmpAttributes{}
		attrs := map[string]interface{}{
			"crewai.crew.name": "research-crew",
		}

		PopulateCrewAIAgentAttributes(ampAttrs, attrs)

		agentData := ampAttrs.Data.(AgentData)
		if agentData.Name != "research-crew" {
			t.Errorf("expected 'research-crew', got %q", agentData.Name)
		}
	})
}

func TestParseCrewAITokenUsage(t *testing.T) {
	t.Run("valid token usage string", func(t *testing.T) {
		usage := parseCrewAITokenUsage("total_tokens=57062 prompt_tokens=46376 cached_prompt_tokens=100 completion_tokens=10686 successful_requests=10")
		if usage == nil {
			t.Fatal("expected token usage, got nil")
		}
		if usage.TotalTokens != 57062 {
			t.Errorf("expected total 57062, got %d", usage.TotalTokens)
		}
		if usage.InputTokens != 46376 {
			t.Errorf("expected input 46376, got %d", usage.InputTokens)
		}
		if usage.OutputTokens != 10686 {
			t.Errorf("expected output 10686, got %d", usage.OutputTokens)
		}
		if usage.CacheReadInputTokens != 100 {
			t.Errorf("expected cache 100, got %d", usage.CacheReadInputTokens)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		usage := parseCrewAITokenUsage("")
		if usage != nil {
			t.Errorf("expected nil, got %+v", usage)
		}
	})

	t.Run("no valid token data", func(t *testing.T) {
		usage := parseCrewAITokenUsage("successful_requests=10")
		if usage != nil {
			t.Errorf("expected nil, got %+v", usage)
		}
	})
}

func TestExtractCrewAISystemPrompt(t *testing.T) {
	t.Run("all fields present", func(t *testing.T) {
		attrs := map[string]interface{}{
			"crewai.agent.role":      "Researcher",
			"crewai.agent.goal":      "Find truth",
			"crewai.agent.backstory": "Expert in research",
		}
		prompt := extractCrewAISystemPrompt(attrs)
		if !strings.Contains(prompt, "role: >") {
			t.Errorf("expected 'role: >' in prompt, got %q", prompt)
		}
		if !strings.Contains(prompt, "Researcher") {
			t.Errorf("expected 'Researcher' in prompt, got %q", prompt)
		}
		if !strings.Contains(prompt, "goal: >") {
			t.Errorf("expected 'goal: >' in prompt, got %q", prompt)
		}
		if !strings.Contains(prompt, "backstory: >") {
			t.Errorf("expected 'backstory: >' in prompt, got %q", prompt)
		}
	})

	t.Run("no fields present", func(t *testing.T) {
		attrs := map[string]interface{}{}
		prompt := extractCrewAISystemPrompt(attrs)
		if prompt != "" {
			t.Errorf("expected empty string, got %q", prompt)
		}
	})

	t.Run("only role present", func(t *testing.T) {
		attrs := map[string]interface{}{
			"crewai.agent.role": "Writer",
		}
		prompt := extractCrewAISystemPrompt(attrs)
		if !strings.Contains(prompt, "Writer") {
			t.Errorf("expected 'Writer' in prompt, got %q", prompt)
		}
		if strings.Contains(prompt, "goal:") {
			t.Errorf("should not contain 'goal:' when not set, got %q", prompt)
		}
	})
}
