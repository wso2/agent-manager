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

package openchoreosvc

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

type AnnotationKeys string

const (
	AnnotationKeyDisplayName AnnotationKeys = "openchoreo.dev/display-name"
	AnnotationKeyDescription AnnotationKeys = "openchoreo.dev/description"
)

type TraceAttributeKeys string

const (
	TraceAttributeKeyEnvironment TraceAttributeKeys = "openchoreo.dev/environment-uid"
	TraceAttributeKeyComponent   TraceAttributeKeys = "openchoreo.dev/component-uid"
)

type WorkflowConditionType string

const (
	ConditionWorkloadUpdated   WorkflowConditionType = "WorkloadUpdated"
	ConditionWorkflowFailed    WorkflowConditionType = "WorkflowFailed"
	ConditionWorkflowSucceeded WorkflowConditionType = "WorkflowSucceeded"
	ConditionWorkflowRunning   WorkflowConditionType = "WorkflowRunning"
	ConditionWorkflowPending   WorkflowConditionType = "WorkflowPending"
	ConditionWorkflowCompleted WorkflowConditionType = "WorkflowCompleted"
)

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

// Deployment status values
const (
	DeploymentStatusFailed      = "failed"
	DeploymentStatusNotDeployed = "not-deployed"
	DeploymentStatusSuspended   = "suspended"
	DeploymentStatusInProgress  = "in-progress"
	DeploymentStatusActive      = "active"
	DeploymentStatusNotReady    = "not-ready"
)

const (
	EndpointTypeDefault = "DEFAULT"
	EndpointTypeCustom  = "CUSTOM"
)

const (
	MainContainerName         = "main"
	DevEnvironmentName        = "development"
	DevEnvironmentDisplayName = "Development"
	DefaultDisplayName        = "Default"
	DefaultName               = "default"
)

// Resource constants
const (
	DefaultCPURequest    = "100m"
	DefaultMemoryRequest = "256Mi"
	DefaultCPULimit      = "500m"
	DefaultMemoryLimit   = "512Mi"
	DefaultReplicaCount  = 1
)

const (
	GithubImageRegistry = "ghcr.io/wso2"
	ImageName           = "amp-python-instrumentation-provider"
)
