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

type EndpointType string

const (
	EndpointTypeDefault EndpointType = "DEFAULT"
	EndpointTypeCustom  EndpointType = "CUSTOM"
)

type ResourceType string

const (
	ResourceTypeAgent       ResourceType = "agent"
	ResourceTypeProject     ResourceType = "project"
	ResourceTypeEnvironment ResourceType = "environment"
)

// Name generation constants
const (
	MaxResourceNameLength     = 25
	RandomSuffixLength        = 2
	ValidCandidateLength      = MaxResourceNameLength - RandomSuffixLength - 1 // 1 for hyphen
	MaxNameGenerationAttempts = 10                                             // Prevent infinite loop
	NameGenerationAlphabet    = "abcdefghijklmnopqrstuvwxyz"
)

// File mount shape limits. The content size caps are operator-configurable;
// see config.FileMountLimitsConfig (FILE_MOUNT_MAX_FILE_BYTES,
// FILE_MOUNT_MAX_TOTAL_BYTES).
const (
	MaxFileMountKeyLength = 253  // matches k8s metadata.name limit
	MaxMountPathLength    = 4095 // Linux PATH_MAX minus one for safety
)

// Caps for the repository and build inputs that reach the build workflow. Each
// of these becomes an argument to git or to the container build running inside
// the build pod, so each is bounded on its own rather than only by the overall
// request size.
const (
	MaxRepositoryURLLength = 2048 // generous URL bound; the host itself is pinned separately
	MaxGitBranchLength     = 255  // git has no hard limit, but refs are filenames
	MaxSafePathLength      = 1024 // application / Dockerfile / schema / base path
)

// Path parameter names used in HTTP routes
const (
	PathParamOrgName      = "orgName"
	PathParamProjName     = "projName"
	PathParamAgentName    = "agentName"
	PathParamBuildName    = "buildName"
	PathParamTraceId      = "traceId"
	PathParamProviderId   = "providerId"
	PathParamTemplateId   = "templateId"
	PathParamProxyId      = "proxyId"
	PathParamConfigId     = "configId"
	PathParamGatewayId    = "gatewayId"
	PathParamPipelineName = "pipelineName"
	PathParamEnvID        = "envID"
	PathParamDeploymentId = "deploymentId"
	PathParamMonitorName  = "monitorName"
	PathParamMonitorId    = "monitorId"
	PathParamRunId        = "runId"
	PathParamEvaluatorId  = "evaluatorId"
	PathParamSecretName   = "secretName"
	PathParamUserID       = "userID"
	PathParamGroupID      = "groupID"
	PathParamRoleID       = "roleID"
	PathParamKindName     = "kindName"
	PathParamVersionTag   = "versionTag"
	PathParamScopeName    = "scopeName"
	PathParamScopeAction  = "scopeAction"
)

// Pagination constants
const (
	DefaultLimit  = 50
	MinLimit      = 1
	MaxLimit      = 100
	DefaultOffset = 0
	MinOffset     = 0
)

// Deployment state constants
const (
	DeploymentStateActive   = "Active"
	DeploymentStateUndeploy = "Undeploy"
)

// Git secret constants
const (
	GitSecretTypeBasicAuth = "basic-auth"
)

// Isolation tier constants. Tier values are the API-facing names stored on the
// environment; the RuntimeClass names are what the rendered pod spec requests
// (kata-deploy registers the QEMU variant under "kata-qemu").
const (
	IsolationTierGvisor = "gvisor"
	IsolationTierKata   = "kata"

	RuntimeClassGvisor   = "gvisor"
	RuntimeClassKataQemu = "kata-qemu"
)
