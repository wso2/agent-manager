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

package utils

import (
	"testing"
)

func TestLoadLLMProviderTemplatesFromDirectory(t *testing.T) {
	// Test with the actual templates directory
	templates, err := LoadLLMProviderTemplatesFromDirectory("../resources/default-llm-provider-templates")
	if err != nil {
		t.Fatalf("Failed to load templates: %v", err)
	}

	if len(templates) == 0 {
		t.Fatal("Expected at least one template, got zero")
	}

	t.Logf("Successfully loaded %d templates", len(templates))

	// Verify expected templates
	expectedTemplates := map[string]string{
		"openai":         "OpenAI",
		"anthropic":      "Anthropic",
		"mistral":        "Mistral",
		"azureopenai":    "Azure OpenAI",
		"awsbedrock":     "AWS Bedrock",
		"azureaifoundry": "Azure AI Foundry",
		"gemini":         "Gemini",
	}

	foundTemplates := make(map[string]bool)
	for _, tmpl := range templates {
		foundTemplates[tmpl.Handle] = true

		// Verify required fields
		if tmpl.Handle == "" {
			t.Errorf("Template has empty handle")
		}
		if tmpl.Name == "" {
			t.Errorf("Template %s has empty name", tmpl.Handle)
		}

		// Check if it's an expected template
		if expectedName, exists := expectedTemplates[tmpl.Handle]; exists {
			if tmpl.Name != expectedName {
				t.Logf("Template %s: expected name '%s', got '%s'", tmpl.Handle, expectedName, tmpl.Name)
			}
		}

		t.Logf("  - %s (handle: %s)", tmpl.Name, tmpl.Handle)
	}

	// Verify we found the core templates
	coreTemplates := []string{"openai", "anthropic"}
	for _, core := range coreTemplates {
		if !foundTemplates[core] {
			t.Errorf("Expected to find %s template, but it was not loaded", core)
		}
	}
}

func TestLoadLLMProviderTemplatesFromDirectory_EmptyPath(t *testing.T) {
	_, err := LoadLLMProviderTemplatesFromDirectory("")
	if err == nil {
		t.Error("Expected error for empty path, got nil")
	}
}

func TestLoadLLMProviderTemplatesFromDirectory_NonExistentPath(t *testing.T) {
	_, err := LoadLLMProviderTemplatesFromDirectory("/non/existent/path")
	if err == nil {
		t.Error("Expected error for non-existent path, got nil")
	}
}
