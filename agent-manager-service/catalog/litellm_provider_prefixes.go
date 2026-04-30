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

package catalog

// liteLLMProviderPrefixes maps gateway LLM provider template handles to the
// model prefix expected by LiteLLM (e.g. "bedrock", "azure", "openai").
var liteLLMProviderPrefixes = map[string]string{
	"openai":          "openai",
	"anthropic":       "anthropic",
	"gemini":          "gemini",
	"groq":            "groq",
	"mistral":         "mistral",
	"mistralai":       "mistral",
	"awsbedrock":      "bedrock",
	"azure-openai":    "azure",
	"azureai-foundry": "azure_ai",
}

// GetLiteLLMPrefix resolves a gateway provider TemplateHandle to the LiteLLM
// model prefix string. Returns ("", false) for unknown handles — the caller
// should store the bare model name and let the evaluation job apply its default.
func GetLiteLLMPrefix(templateHandle string) (string, bool) {
	prefix, ok := liteLLMProviderPrefixes[templateHandle]
	return prefix, ok
}
