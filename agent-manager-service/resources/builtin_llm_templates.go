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

package resources

import "github.com/wso2/ai-agent-management-platform/agent-manager-service/models"

// BuiltInLLMProviderTemplates contains all built-in LLM provider templates
// These templates are immutable and available globally across all organizations
var BuiltInLLMProviderTemplates = []*models.LLMProviderTemplate{
	{
		Handle: "openai",
		Name:   "OpenAI",
		Metadata: &models.LLMProviderTemplateMetadata{
			EndpointURL: "https://api.openai.com",
			Auth: &models.LLMProviderTemplateAuth{
				Type:        "apiKey",
				Header:      "Authorization",
				ValuePrefix: "Bearer ",
			},
			LogoURL:        "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/openai/icon.png",
			OpenapiSpecURL: "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/openai/openapi.yaml",
		},
		PromptTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.prompt_tokens",
		},
		CompletionTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.completion_tokens",
		},
		TotalTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.total_tokens",
		},
		RemainingTokens: &models.ExtractionIdentifier{
			Location:   "header",
			Identifier: "x-ratelimit-remaining-tokens",
		},
		RequestModel: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
		ResponseModel: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
	},
	{
		Handle: "anthropic",
		Name:   "Anthropic",
		Metadata: &models.LLMProviderTemplateMetadata{
			EndpointURL: "https://api.anthropic.com",
			Auth: &models.LLMProviderTemplateAuth{
				Type:   "apiKey",
				Header: "x-api-key",
			},
			LogoURL:        "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/anthropic.claude/icon.png",
			OpenapiSpecURL: "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/anthropic.claude/openapi.yaml",
		},
		PromptTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.input_tokens",
		},
		CompletionTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.output_tokens",
		},
		RemainingTokens: &models.ExtractionIdentifier{
			Location:   "header",
			Identifier: "anthropic-ratelimit-tokens-remaining",
		},
		RequestModel: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
		ResponseModel: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
	},
	{
		Handle: "awsbedrock",
		Name:   "AWS Bedrock",
		Metadata: &models.LLMProviderTemplateMetadata{
			LogoURL:        "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/aws.bedrock/icon.png",
			OpenapiSpecURL: "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/aws.bedrock/openapi.yaml",
		},
		PromptTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.inputTokens",
		},
		CompletionTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.outputTokens",
		},
		TotalTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.totalTokens",
		},
		RequestModel: &models.ExtractionIdentifier{
			Location:   "pathParam",
			Identifier: "model/([A-Za-z0-9.:-]+)/",
		},
		ResponseModel: &models.ExtractionIdentifier{
			Location:   "pathParam",
			Identifier: "model/([A-Za-z0-9.:-]+)/",
		},
	},
	{
		Handle: "azure-openai",
		Name:   "Azure OpenAI",
		Metadata: &models.LLMProviderTemplateMetadata{
			Auth: &models.LLMProviderTemplateAuth{
				Type:   "apiKey",
				Header: "api-key",
			},
			LogoURL:        "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/azure.openai/icon.png",
			OpenapiSpecURL: "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/azure.openai/openapi.yaml",
		},
		PromptTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.prompt_tokens",
		},
		CompletionTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.completion_tokens",
		},
		TotalTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.total_tokens",
		},
		RemainingTokens: &models.ExtractionIdentifier{
			Location:   "header",
			Identifier: "x-ratelimit-remaining-tokens",
		},
		RequestModel: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
		ResponseModel: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
	},
	{
		Handle: "azureai-foundry",
		Name:   "Azure AI Foundry",
		Metadata: &models.LLMProviderTemplateMetadata{
			Auth: &models.LLMProviderTemplateAuth{
				Type:   "apiKey",
				Header: "api-key",
			},
			LogoURL: "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/azure.openai/icon.png",
		},
		PromptTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.prompt_tokens",
		},
		CompletionTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.completion_tokens",
		},
		TotalTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.total_tokens",
		},
		RemainingTokens: &models.ExtractionIdentifier{
			Location:   "header",
			Identifier: "x-ratelimit-remaining-tokens",
		},
		RequestModel: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
		ResponseModel: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
	},
	{
		Handle: "gemini",
		Name:   "Gemini",
		Metadata: &models.LLMProviderTemplateMetadata{
			EndpointURL: "https://generativelanguage.googleapis.com",
			Auth: &models.LLMProviderTemplateAuth{
				Type:   "apiKey",
				Header: "x-goog-api-key",
			},
		},
		PromptTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usageMetadata.promptTokenCount",
		},
		CompletionTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usageMetadata.candidatesTokenCount",
		},
		TotalTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usageMetadata.totalTokenCount",
		},
		RemainingTokens: &models.ExtractionIdentifier{
			Location:   "header",
			Identifier: "x-ratelimit-remaining-tokens",
		},
		RequestModel: &models.ExtractionIdentifier{
			Location:   "pathParam",
			Identifier: "models/([A-Za-z0-9.-]+)",
		},
		ResponseModel: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.modelVersion",
		},
	},
	{
		Handle: "mistralai",
		Name:   "Mistral",
		Metadata: &models.LLMProviderTemplateMetadata{
			EndpointURL: "https://api.mistral.ai",
			Auth: &models.LLMProviderTemplateAuth{
				Type:        "apiKey",
				Header:      "Authorization",
				ValuePrefix: "Bearer ",
			},
			LogoURL:        "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/mistral/icon.png",
			OpenapiSpecURL: "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/mistral/openapi.yaml",
		},
		PromptTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.prompt_tokens",
		},
		CompletionTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.completion_tokens",
		},
		TotalTokens: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.total_tokens",
		},
		RemainingTokens: &models.ExtractionIdentifier{
			Location:   "header",
			Identifier: "x-ratelimit-remaining-tokens",
		},
		RequestModel: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
		ResponseModel: &models.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
	},
}
