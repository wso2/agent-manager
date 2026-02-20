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

import "errors"

var (
	// Resource not found errors
	ErrProjectNotFound            = errors.New("project not found")
	ErrAgentAlreadyExists         = errors.New("agent already exists")
	ErrAgentNotFound              = errors.New("agent not found")
	ErrOrganizationNotFound       = errors.New("organization not found")
	ErrBuildNotFound              = errors.New("build not found")
	ErrEnvironmentNotFound        = errors.New("environment not found")
	ErrOrganizationAlreadyExists  = errors.New("organization already exists")
	ErrProjectAlreadyExists       = errors.New("project already exists")
	ErrDeploymentPipelineNotFound = errors.New("deployment pipeline not found")
	ErrProjectHasAssociatedAgents = errors.New("project has associated agents")
	ErrMonitorNotFound            = errors.New("monitor not found")
	ErrMonitorAlreadyExists       = errors.New("monitor already exists")
	ErrMonitorRunNotFound         = errors.New("monitor run not found")
	ErrMonitorAlreadyStopped      = errors.New("monitor already stopped")
	ErrMonitorAlreadyActive       = errors.New("monitor already active")
	ErrEvaluatorNotFound          = errors.New("evaluator not found")
	ErrInvalidInput               = errors.New("invalid input")
	ErrImmutableFieldChange       = errors.New("cannot change immutable field")

	// Request errors
	ErrBadRequest = errors.New("bad request")

	// Authorization errors
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")

	// Server errors
	ErrServiceUnavailable = errors.New("service unavailable")

	// Gateway-related errors
	ErrGatewayNotFound          = errors.New("gateway not found")
	ErrGatewayAlreadyExists     = errors.New("gateway already exists")
	ErrInvalidAdapterType       = errors.New("invalid adapter type")
	ErrGatewayUnreachable       = errors.New("gateway unreachable")
	ErrInvalidGatewayConfig     = errors.New("invalid gateway configuration")
	ErrEnvironmentAlreadyExists = errors.New("environment already exists")
	ErrEnvironmentHasGateways   = errors.New("environment has associated gateways")

	// LLM Provider-related errors
	ErrProviderNotFound        = errors.New("provider not found")
	ErrProviderAlreadyExists   = errors.New("provider already exists")
	ErrProviderHasDeployments  = errors.New("provider has active deployments")
	ErrDeploymentNotFound      = errors.New("deployment not found")
	ErrDeploymentFailed        = errors.New("deployment failed")
	ErrPolicyNotSupported      = errors.New("policy not supported by gateway")
	ErrInvalidProviderConfig   = errors.New("invalid provider configuration")
	ErrSystemTemplateImmutable = errors.New("system templates cannot be modified or deleted")
	ErrSystemTemplateOverride  = errors.New("cannot create user template with same handle as system template")

	// API Platform integration errors
	ErrHandleExists                = errors.New("handle already exists")
	ErrGatewayHasAssociatedAPIs    = errors.New("gateway has associated APIs")
	ErrGatewayHasDeployments       = errors.New("cannot delete gateway: it has active API deployments. Please undeploy all APIs before deleting the gateway")
	ErrAPINotFound                 = errors.New("API not found")
	ErrDeploymentNotActive         = errors.New("deployment not active")
	ErrLLMProviderTemplateNotFound = errors.New("LLM provider template not found")
	ErrLLMProviderTemplateExists   = errors.New("LLM provider template already exists")
	ErrLLMProviderNotFound         = errors.New("LLM provider not found")
	ErrLLMProviderExists           = errors.New("LLM provider already exists")
	ErrLLMProxyNotFound            = errors.New("LLM proxy not found")
	ErrLLMProxyExists              = errors.New("LLM proxy already exists")
	ErrBaseDeploymentNotFound      = errors.New("base deployment not found")
	ErrDeploymentIsDeployed        = errors.New("deployment is currently deployed")
	ErrDeploymentAlreadyDeployed   = errors.New("deployment already deployed")
	ErrGatewayIDMismatch           = errors.New("gateway ID mismatch")
	ErrDeploymentNameRequired      = errors.New("deployment name required")
	ErrDeploymentBaseRequired      = errors.New("deployment base required")
	ErrDeploymentGatewayIDRequired = errors.New("deployment gateway ID required")
	ErrInvalidDeploymentStatus     = errors.New("invalid deployment status")
	ErrArtifactNotFound            = errors.New("artifact not found")
	ErrArtifactExists              = errors.New("artifact already exists")
	ErrDevPortalNotFound           = errors.New("devportal not found")
	ErrAPIAlreadyPublished         = errors.New("api is already published to devportal")
	ErrAPIPublicationNotFound      = errors.New("api publication not found")

	// Implementation status errors
	ErrNotImplemented = errors.New("not implemented")

	// Agent Configuration errors
	ErrAgentConfigNotFound      = errors.New("agent configuration not found")
	ErrAgentConfigAlreadyExists = errors.New("agent configuration already exists for this agent")
)
