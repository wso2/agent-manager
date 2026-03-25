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

	// Server error codes (5xx)
	ErrCodeInternalError      = "INTERNAL_ERROR"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"

	// Resource-specific error codes
	ErrCodeAgentNotFound          = "AGENT_NOT_FOUND"
	ErrCodeAgentAlreadyExists     = "AGENT_ALREADY_EXISTS"
	ErrCodeProjectNotFound        = "PROJECT_NOT_FOUND"
	ErrCodeProjectAlreadyExists   = "PROJECT_ALREADY_EXISTS"
	ErrCodeOrganizationNotFound   = "ORGANIZATION_NOT_FOUND"
	ErrCodeBuildNotFound          = "BUILD_NOT_FOUND"
	ErrCodeEnvironmentNotFound    = "ENVIRONMENT_NOT_FOUND"
	ErrCodeGatewayNotFound        = "GATEWAY_NOT_FOUND"
	ErrCodeGatewayAlreadyExists   = "GATEWAY_ALREADY_EXISTS"
	ErrCodeMonitorNotFound        = "MONITOR_NOT_FOUND"
	ErrCodeMonitorAlreadyExists   = "MONITOR_ALREADY_EXISTS"
	ErrCodeProviderNotFound       = "PROVIDER_NOT_FOUND"
	ErrCodeProviderAlreadyExists  = "PROVIDER_ALREADY_EXISTS"
	ErrCodeDeploymentNotFound     = "DEPLOYMENT_NOT_FOUND"
	ErrCodeAPINotFound            = "API_NOT_FOUND"
	ErrCodeLLMProviderNotFound    = "LLM_PROVIDER_NOT_FOUND"
	ErrCodeLLMProxyNotFound       = "LLM_PROXY_NOT_FOUND"
	ErrCodeArtifactNotFound       = "ARTIFACT_NOT_FOUND"
	ErrCodeAgentConfigNotFound    = "AGENT_CONFIG_NOT_FOUND"
	ErrCodeGitSecretNotFound      = "GIT_SECRET_NOT_FOUND"
	ErrCodeGitSecretAlreadyExists = "GIT_SECRET_ALREADY_EXISTS"
	ErrCodeGitSecretInvalidType   = "GIT_SECRET_INVALID_TYPE"
)
