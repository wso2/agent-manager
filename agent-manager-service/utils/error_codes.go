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

package utils

// Error codes for programmatic error handling by frontend applications.
// These codes are stable and should not change once defined.
const (
	// Client error codes (4xx)
	ErrCodeImmutableField = "IMMUTABLE_FIELD"
	// ErrCodeHandleInUse: the MCP proxy handle belongs to an org-level or another project's proxy.
	ErrCodeHandleInUse = "HANDLE_IN_USE"
	// ErrCodeMCPProxyInUse: AM agents still use the MCP proxy, so it cannot be deleted.
	ErrCodeMCPProxyInUse = "MCP_PROXY_IN_USE"
	// ErrCodeUpstreamNotReady: the project MCP proxy's source component isn't deployed in the environment yet.
	ErrCodeUpstreamNotReady = "UPSTREAM_NOT_READY"
	// ErrCodeAmbiguousGateway: the binding's environment has several egress gateways and no gatewayId was given.
	ErrCodeAmbiguousGateway = "AMBIGUOUS_GATEWAY"

	// Resource-specific error codes
	ErrCodeAgentNotFound           = "AGENT_NOT_FOUND"
	ErrCodeAgentAlreadyExists      = "AGENT_ALREADY_EXISTS"
	ErrCodeProjectNotFound         = "PROJECT_NOT_FOUND"
	ErrCodeProjectAlreadyExists    = "PROJECT_ALREADY_EXISTS"
	ErrCodeOrganizationNotFound    = "ORGANIZATION_NOT_FOUND"
	ErrCodeEnvironmentNotFound     = "ENVIRONMENT_NOT_FOUND"
	ErrCodeGatewayNotFound         = "GATEWAY_NOT_FOUND"
	ErrCodeGatewayAlreadyExists    = "GATEWAY_ALREADY_EXISTS"
	ErrCodeEnvironmentHasNoGateway = "ENVIRONMENT_HAS_NO_GATEWAY"
	ErrCodeProviderAlreadyExists   = "PROVIDER_ALREADY_EXISTS"
	ErrCodeDeploymentNotFound      = "DEPLOYMENT_NOT_FOUND"
	ErrCodeAPINotFound             = "API_NOT_FOUND"
	ErrCodeLLMProviderNotFound     = "LLM_PROVIDER_NOT_FOUND"
	ErrCodeLLMProxyNotFound        = "LLM_PROXY_NOT_FOUND"
	ErrCodeArtifactNotFound        = "ARTIFACT_NOT_FOUND"
	ErrCodeAgentConfigNotFound     = "AGENT_CONFIG_NOT_FOUND"
)
