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

import "github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"

// BuiltInLLMProviderTemplates contains all built-in LLM provider templates
// These templates are immutable and available globally across all organizations
var BuiltInLLMProviderTemplates = []*gwmodels.LLMProviderTemplate{
	{
		Handle:   "openai",
		Name:     "OpenAI",
		IsSystem: true,
		OUID:     "",
		Metadata: &gwmodels.LLMProviderTemplateMetadata{
			EndpointURL: "https://api.openai.com/v1",
			Auth: &gwmodels.LLMProviderTemplateAuth{
				Type:        "api-key",
				Header:      "Authorization",
				ValuePrefix: "Bearer ",
			},
			LogoURL:        "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/openai/icon.png",
			OpenapiSpecURL: "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/openai/openapi.yaml",
		},
		PromptTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.prompt_tokens",
		},
		CompletionTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.completion_tokens",
		},
		TotalTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.total_tokens",
		},
		RemainingTokens: &gwmodels.ExtractionIdentifier{
			Location:   "header",
			Identifier: "x-ratelimit-remaining-tokens",
		},
		RequestModel: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
		ResponseModel: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
	},
	{
		Handle:   "anthropic",
		Name:     "Anthropic",
		IsSystem: true,
		OUID:     "",
		Metadata: &gwmodels.LLMProviderTemplateMetadata{
			EndpointURL: "https://api.anthropic.com",
			Auth: &gwmodels.LLMProviderTemplateAuth{
				Type:   "api-key",
				Header: "x-api-key",
			},
			LogoURL:        "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/anthropic.claude/icon.png",
			OpenapiSpecURL: "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/anthropic.claude/openapi.yaml",
		},
		PromptTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.input_tokens",
		},
		CompletionTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.output_tokens",
		},
		RemainingTokens: &gwmodels.ExtractionIdentifier{
			Location:   "header",
			Identifier: "anthropic-ratelimit-tokens-remaining",
		},
		RequestModel: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
		ResponseModel: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
	},
	{
		Handle:   "awsbedrock",
		Name:     "AWS Bedrock",
		IsSystem: true,
		OUID:     "",
		Metadata: &gwmodels.LLMProviderTemplateMetadata{
			EndpointURL: "https://bedrock-runtime.us-east-1.amazonaws.com",
			// "bearer" is not a gateway upstream auth type; api-key sets the header verbatim.
			Auth: &gwmodels.LLMProviderTemplateAuth{
				Type:        "api-key",
				Header:      "Authorization",
				ValuePrefix: "Bearer ",
			},
			LogoURL:        "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/aws.bedrock/icon.png",
			OpenapiSpecURL: "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/aws.bedrock/openapi.yaml",
		},
		PromptTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.inputTokens",
		},
		CompletionTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.outputTokens",
		},
		TotalTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.totalTokens",
		},
		RequestModel: &gwmodels.ExtractionIdentifier{
			Location:   "pathParam",
			Identifier: "model/([A-Za-z0-9.:-]+)/",
		},
		ResponseModel: &gwmodels.ExtractionIdentifier{
			Location:   "pathParam",
			Identifier: "model/([A-Za-z0-9.:-]+)/",
		},
	},
	{
		Handle:   "azure-openai",
		Name:     "Azure OpenAI",
		IsSystem: true,
		OUID:     "",
		Metadata: &gwmodels.LLMProviderTemplateMetadata{
			Auth: &gwmodels.LLMProviderTemplateAuth{
				Type:   "api-key",
				Header: "api-key",
			},
			LogoURL:        "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/azure.openai/icon.png",
			OpenapiSpecURL: "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/azure.openai/openapi.yaml",
		},
		PromptTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.prompt_tokens",
		},
		CompletionTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.completion_tokens",
		},
		TotalTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.total_tokens",
		},
		RemainingTokens: &gwmodels.ExtractionIdentifier{
			Location:   "header",
			Identifier: "x-ratelimit-remaining-tokens",
		},
		RequestModel: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
		ResponseModel: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
	},
	{
		Handle:   "azureai-foundry",
		Name:     "Azure AI Foundry",
		IsSystem: true,
		OUID:     "",
		Metadata: &gwmodels.LLMProviderTemplateMetadata{
			Auth: &gwmodels.LLMProviderTemplateAuth{
				Type:   "api-key",
				Header: "api-key",
			},
			LogoURL: "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/azure.openai/icon.png",
		},
		PromptTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.prompt_tokens",
		},
		CompletionTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.completion_tokens",
		},
		TotalTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.total_tokens",
		},
		RemainingTokens: &gwmodels.ExtractionIdentifier{
			Location:   "header",
			Identifier: "x-ratelimit-remaining-tokens",
		},
		RequestModel: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
		ResponseModel: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
	},
	{
		Handle:   "gemini",
		Name:     "Gemini",
		IsSystem: true,
		OUID:     "",
		Metadata: &gwmodels.LLMProviderTemplateMetadata{
			EndpointURL: "https://generativelanguage.googleapis.com",
			Auth: &gwmodels.LLMProviderTemplateAuth{
				Type:   "api-key",
				Header: "x-goog-api-key",
			},
		},
		PromptTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usageMetadata.promptTokenCount",
		},
		CompletionTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usageMetadata.candidatesTokenCount",
		},
		TotalTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usageMetadata.totalTokenCount",
		},
		RemainingTokens: &gwmodels.ExtractionIdentifier{
			Location:   "header",
			Identifier: "x-ratelimit-remaining-tokens",
		},
		RequestModel: &gwmodels.ExtractionIdentifier{
			Location:   "pathParam",
			Identifier: "models/([A-Za-z0-9.-]+)",
		},
		ResponseModel: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.modelVersion",
		},
	},
	{
		Handle:   "mistralai",
		Name:     "Mistral",
		IsSystem: true,
		OUID:     "",
		Metadata: &gwmodels.LLMProviderTemplateMetadata{
			EndpointURL: "https://api.mistral.ai",
			Auth: &gwmodels.LLMProviderTemplateAuth{
				Type:        "api-key",
				Header:      "Authorization",
				ValuePrefix: "Bearer ",
			},
			LogoURL:        "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/mistral/icon.png",
			OpenapiSpecURL: "https://raw.githubusercontent.com/nomadxd/openapi-connectors/main/openapi/mistral/openapi.yaml",
		},
		PromptTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.prompt_tokens",
		},
		CompletionTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.completion_tokens",
		},
		TotalTokens: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.usage.total_tokens",
		},
		RemainingTokens: &gwmodels.ExtractionIdentifier{
			Location:   "header",
			Identifier: "x-ratelimit-remaining-tokens",
		},
		RequestModel: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
		ResponseModel: &gwmodels.ExtractionIdentifier{
			Location:   "payload",
			Identifier: "$.model",
		},
	},
}
