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
	ErrCodeValidation     = "VALIDATION_ERROR"
	ErrCodeNotFound       = "NOT_FOUND"
	ErrCodeConflict       = "CONFLICT"
	ErrCodeBadRequest     = "BAD_REQUEST"
	ErrCodeUnauthorized   = "UNAUTHORIZED"
	ErrCodeForbidden      = "FORBIDDEN"
	ErrCodeImmutableField = "IMMUTABLE_FIELD"
	ErrCodeInvalidInput   = "INVALID_INPUT"
	// ErrCodeHandleInUse: the MCP proxy handle belongs to an org-level or another project's proxy.
	ErrCodeHandleInUse = "HANDLE_IN_USE"
	// ErrCodeProjectOwned: an org route tried to mutate a project-scoped MCP proxy.
	ErrCodeProjectOwned = "PROJECT_OWNED"
	// ErrCodeMCPProxyInUse: AM agents still use the MCP proxy, so it cannot be deleted.
	ErrCodeMCPProxyInUse = "MCP_PROXY_IN_USE"
	// ErrCodeUpstreamNotReady: the project MCP proxy's source component isn't deployed in the environment yet.
	ErrCodeUpstreamNotReady = "UPSTREAM_NOT_READY"
	// ErrCodeSourceComponentNotFound: the project MCP proxy's source component is not in the project.
	ErrCodeSourceComponentNotFound = "SOURCE_COMPONENT_NOT_FOUND"
	// ErrCodeContextInUse: an artifact's gateway context overlaps another artifact's on that gateway.
	ErrCodeContextInUse = "CONTEXT_IN_USE"
	// ErrCodeAmbiguousGateway: the binding's environment has several egress gateways and no gatewayId was given.
	ErrCodeAmbiguousGateway = "AMBIGUOUS_GATEWAY"
	// ErrCodeEntitlementDenied: the organization's subscription does not allow the create (402).
	ErrCodeEntitlementDenied = "ENTITLEMENT_DENIED"

	// Server error codes (5xx)
	ErrCodeInternalError      = "INTERNAL_ERROR"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	ErrCodeNotImplemented     = "NOT_IMPLEMENTED"

	// Resource-specific error codes
	ErrCodeAgentNotFound           = "AGENT_NOT_FOUND"
	ErrCodeAgentAlreadyExists      = "AGENT_ALREADY_EXISTS"
	ErrCodeProjectNotFound         = "PROJECT_NOT_FOUND"
	ErrCodeProjectAlreadyExists    = "PROJECT_ALREADY_EXISTS"
	ErrCodeOrganizationNotFound    = "ORGANIZATION_NOT_FOUND"
	ErrCodeBuildNotFound           = "BUILD_NOT_FOUND"
	ErrCodeEnvironmentNotFound     = "ENVIRONMENT_NOT_FOUND"
	ErrCodeGatewayNotFound         = "GATEWAY_NOT_FOUND"
	ErrCodeGatewayAlreadyExists    = "GATEWAY_ALREADY_EXISTS"
	ErrCodeGatewayEnvironmentFixed = "GATEWAY_ENVIRONMENT_FIXED"
	ErrCodeEnvironmentHasNoGateway = "ENVIRONMENT_HAS_NO_GATEWAY"
	ErrCodeMonitorNotFound         = "MONITOR_NOT_FOUND"
	ErrCodeMonitorAlreadyExists    = "MONITOR_ALREADY_EXISTS"
	ErrCodeProviderNotFound        = "PROVIDER_NOT_FOUND"
	ErrCodeProviderAlreadyExists   = "PROVIDER_ALREADY_EXISTS"
	ErrCodeDeploymentNotFound      = "DEPLOYMENT_NOT_FOUND"
	ErrCodeAPINotFound             = "API_NOT_FOUND"
	ErrCodeLLMProviderNotFound     = "LLM_PROVIDER_NOT_FOUND"
	ErrCodeLLMProxyNotFound        = "LLM_PROXY_NOT_FOUND"
	ErrCodeArtifactNotFound        = "ARTIFACT_NOT_FOUND"
	ErrCodeAgentConfigNotFound     = "AGENT_CONFIG_NOT_FOUND"
	ErrCodeGitSecretNotFound       = "GIT_SECRET_NOT_FOUND"
	ErrCodeGitSecretAlreadyExists  = "GIT_SECRET_ALREADY_EXISTS"
	ErrCodeGitSecretInvalidType    = "GIT_SECRET_INVALID_TYPE"

	// Build publisher error codes, answered to a build's workload step
	ErrCodeBuildPublisherDisabled = "BUILD_PUBLISHER_DISABLED"
	ErrCodeBuildNotAgent          = "BUILD_NOT_AGENT"
	ErrCodeWorkflowNotEnabled     = "WORKFLOW_NOT_ENABLED"
	ErrCodeAgentNotSourceBuilt    = "AGENT_NOT_SOURCE_BUILT"
)
