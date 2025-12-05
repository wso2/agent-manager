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
	LabelKeyComponentType    LabelKeys = "agent-manager/component-type"
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

// Build condition types
type BuildConditionType string

const (
	ConditionBuildInitiated  BuildConditionType = "BuildInitiated"
	ConditionBuildTriggered  BuildConditionType = "BuildTriggered"
	ConditionBuildCompleted  BuildConditionType = "BuildCompleted"
	ConditionWorkloadUpdated BuildConditionType = "WorkloadUpdated"
)

const (
	statusUnknown   = "Unknown"
	statusCompleted = "Completed"
)

// ServiceBinding condition types
const (
	ConditionActive         = "Active"
	ConditionFailed         = "Failed"
	ConditionInProgress     = "InProgress"
	ConditionNotYetDeployed = "NotYetDeployed"
	ConditionSuspended      = "Suspended"
)

// Deployment status values
const (
	DeploymentStatusFailed      = "failed"
	DeploymentStatusNotDeployed = "not-deployed"
	DeploymentStatusSuspended   = "suspended"
	DeploymentStatusInProgress  = "in-progress"
	DeploymentStatusActive      = "active"
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
	DefaultAPIClassNameWithCORS          = "default-with-cors"
	ObservabilityEnabledServiceClassName = "default-otel-supported"
	DefaultServiceClassName              = "default"
)

// Resource constants
const (
	DefaultCPURequest    = "100m"
	DefaultMemoryRequest = "64Mi"
	DefaultCPULimit      = "400m"
	DefaultMemoryLimit   = "256Mi"
)

const (
	BuildPlaneKind = "BuildPlane"
	DataPlaneKind  = "DataPlane"
)
