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
	LabelKeyOrganizationName LabelKeys = "openchoreo.dev/organization"
	LabelKeyProjectName      LabelKeys = "openchoreo.dev/project"
	LabelKeyComponentName    LabelKeys = "openchoreo.dev/component"
	LabelKeyEnvironmentName LabelKeys = "openchoreo.dev/environment"
)

type AnnotationKeys string

const (
	AnnotationKeyDisplayName AnnotationKeys = "openchoreo.dev/display-name"
	AnnotationKeyDescription AnnotationKeys = "openchoreo.dev/description"
)

type BuildTemplateNames string

const (
	GoogleBuildpackBuildTemplate    BuildTemplateNames = "buildpack-ci"
	BallerinaBuildpackBuildTemplate BuildTemplateNames = "ballerina-buildpack-ci"
)

const (
	AgentComponentType string = "agent-component"
	GoogleEntryPoint   string = "google-entry-point"
	LanguageVersion    string = "language-version"
	LanguageVersionKey string = "language-version-key"
)



type WorkflowConditionType string
const (
	ConditionWorkloadUpdated WorkflowConditionType = "WorkloadUpdated"
	ConditionWorkflowFailed WorkflowConditionType = "WorkflowFailed"
	ConditionWorkflowSucceeded WorkflowConditionType ="WorkflowSucceeded"
	ConditionWorkflowRunning WorkflowConditionType = "WorkflowRunning"
)
type BuildStatus string
const (
	statusCompleted = "BuildCompleted"
	statusPending = "BuildPending"
	statusFailed = "BuildFailed"
	statusSucceeded = "WorkloadUpdated"
	statusRunning = "BuildRunning"
	statusInitiated = "BuildInitiated"
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
	MainContainerName                    = "main"
	DevEnvironmentName                   = "development"
	DevEnvironmentDisplayName            = "Development"
	DefaultDisplayName                   = "Default"
	DefaultName                          = "default"
)

// Resource constants
const (
	DefaultCPURequest    = "100m"
	DefaultMemoryRequest = "256Mi"
	DefaultCPULimit      = "500m"
	DefaultMemoryLimit   = "512Mi"
	DefaultReplicaCount = 1
)
