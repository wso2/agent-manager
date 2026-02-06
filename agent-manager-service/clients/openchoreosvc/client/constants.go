//
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
//

package client

// -----------------------------------------------------------------------------
// Trait types
// -----------------------------------------------------------------------------

const (
	TraitOTELInstrumentation TraitType = "python-otel-instrumentation-trait"
	TraitAPIManagement       TraitType = "api-configuration"
)

// -----------------------------------------------------------------------------
// Provisioning types
// -----------------------------------------------------------------------------

const (
	ProvisioningInternal ProvisioningType = "internal"
	ProvisioningExternal ProvisioningType = "external"
)

// -----------------------------------------------------------------------------
// Component type identifiers
// -----------------------------------------------------------------------------
type ComponentType string

const (
	ComponentTypeInternalAgentAPI ComponentType = "deployment/agent-api"
	ComponentTypeExternalAgentAPI ComponentType = "proxy/external-agent-api"
)

// -----------------------------------------------------------------------------
// Workflow names
// -----------------------------------------------------------------------------

const (
	WorkflowNameGoogleCloudBuildpacks = "amp-google-cloud-buildpacks"
	WorkflowNameBallerinaBuilpack     = "amp-ballerina-buildpack"
)

// -----------------------------------------------------------------------------
// Schema types
// -----------------------------------------------------------------------------

const (
	SchemaTypeREST = "REST"
)

// -----------------------------------------------------------------------------
// OTEL instrumentation image
// -----------------------------------------------------------------------------

const (
	InstrumentationImageRegistry = "ghcr.io/wso2"
	InstrumentationImageName     = "amp-python-instrumentation-provider"
)

// -----------------------------------------------------------------------------
// Trace attribute keys
// -----------------------------------------------------------------------------

const (
	TraceAttributeKeyEnvironment = "openchoreo.dev/environment-uid"
	TraceAttributeKeyComponent   = "openchoreo.dev/component-uid"
)

// -----------------------------------------------------------------------------
// Deployment status values
// -----------------------------------------------------------------------------

const (
	DeploymentStatusFailed      = "failed"
	DeploymentStatusNotDeployed = "not-deployed"
	DeploymentStatusInProgress  = "in-progress"
	DeploymentStatusActive      = "active"
)

// -----------------------------------------------------------------------------
// OpenChoreo binding status values
// -----------------------------------------------------------------------------

const (
	BindingStatusReady       = "Ready"
	BindingStatusActive      = "Active"
	BindingStatusFailed      = "Failed"
	BindingStatusError       = "Error"
	BindingStatusProgressing = "Progressing"
	BindingStatusPending     = "Pending"
)

// -----------------------------------------------------------------------------
// OpenChoreo resource API version
// -----------------------------------------------------------------------------

const (
	ResourceAPIVersion = "openchoreo.dev/v1alpha1"
)

// -----------------------------------------------------------------------------
// Kubernetes resource kinds
// -----------------------------------------------------------------------------

const (
	ResourceKindProject    = "Project"
	ResourceKindComponent  = "Component"
	ResourceKindHTTPRoute  = "HTTPRoute"
	ResourceKindDeployment = "Deployment"
)

// -----------------------------------------------------------------------------
// OpenChoreo annotation keys
// -----------------------------------------------------------------------------

const (
	AnnotationKeyDisplayName = "openchoreo.dev/display-name"
	AnnotationKeyDescription = "openchoreo.dev/description"
)

// / -----------------------------------------------------------------------------
// OpenChoreo label keys
// -----------------------------------------------------------------------------
type LabelKeys string

const (
	LabelKeyOrganizationName     LabelKeys = "openchoreo.dev/organization"
	LabelKeyProjectName          LabelKeys = "openchoreo.dev/project"
	LabelKeyComponentName        LabelKeys = "openchoreo.dev/component"
	LabelKeyEnvironmentName      LabelKeys = "openchoreo.dev/environment"
	LabelKeyAgentSubType         LabelKeys = "openchoreo.dev/agent-sub-type"
	LabelKeyAgentLanguage        LabelKeys = "openchoreo.dev/agent-language"
	LabelKeyAgentLanguageVersion LabelKeys = "openchoreo.dev/agent-language-version"
	LabelKeyProvisioningType     LabelKeys = "openchoreo.dev/provisioning-type"
)

// -----------------------------------------------------------------------------
// Container and endpoint constants
// -----------------------------------------------------------------------------

const (
	MainContainerName        = "main"
	EndpointVisibilityPublic = "Public"
)

// -----------------------------------------------------------------------------
//  Workflow Run Status (from OpenChoreo ComponentWorkflowRun )
// -----------------------------------------------------------------------------

const (
	WorkflowStatusPending   = "Pending"
	WorkflowStatusRunning   = "Running"
	WorkflowStatusSucceeded = "Succeeded"
	WorkflowStatusFailed    = "Failed"
	WorkflowStatusCompleted = "Completed"
)

// -----------------------------------------------------------------------------
// Internal Build Status (for UI representation)
// -----------------------------------------------------------------------------

type BuildStatus string

const (
	BuildStatusInitiated BuildStatus = "BuildInitiated"
	BuildStatusTriggered BuildStatus = "BuildTriggered"
	BuildStatusRunning   BuildStatus = "BuildRunning"
	BuildStatusCompleted BuildStatus = "BuildCompleted"
	BuildStatusSucceeded BuildStatus = "BuildSucceeded"
	BuildStatusFailed    BuildStatus = "BuildFailed"
	WorkloadUpdated      BuildStatus = "WorkloadUpdated"
)

type BuildStepStatus string

const (
	BuildStepStatusPending   BuildStepStatus = "Pending"
	BuildStepStatusRunning   BuildStepStatus = "Running"
	BuildStepStatusSucceeded BuildStepStatus = "Succeeded"
	BuildStepStatusFailed    BuildStepStatus = "Failed"
)

// Build step indices
const (
	StepIndexInitiated = iota
	StepIndexTriggered
	StepIndexRunning
	StepIndexCompleted
	StepIndexWorkloadUpdated
)

// Resource constants
const (
	DefaultCPURequest    = "100m"
	DefaultMemoryRequest = "256Mi"
	DefaultCPULimit      = "500m"
	DefaultMemoryLimit   = "512Mi"
	DefaultReplicaCount  = 1
)
