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
// Enums and Constants
// -----------------------------------------------------------------------------

// TraitType defines the type of trait that can be attached to a component
type TraitType string

// ProvisioningType defines how a component is provisioned
type ProvisioningType string

// -----------------------------------------------------------------------------
// Request Types - used for creating/updating resources via the client
// -----------------------------------------------------------------------------

// CreateProjectRequest contains data for creating a project
type CreateProjectRequest struct {
	Name               string
	DisplayName        string
	Description        string
	DeploymentPipeline string
}

// PatchProjectRequest contains data for patching a project
type PatchProjectRequest struct {
	DisplayName        string
	Description        string
	DeploymentPipeline string
}

// CreateComponentRequest contains data for creating a component (agent) in OpenChoreo
type CreateComponentRequest struct {
	Name             string
	DisplayName      string
	Description      string
	ProvisioningType ProvisioningType
	Repository       *RepositoryConfig // nil for external agents
	AgentType        AgentTypeConfig
	Build            *BuildConfig          // nil for external agents
	Configurations   *Configurations       // nil for external agents or if no env vars
	InputInterface   *InputInterfaceConfig // nil unless custom-api
}

// RepositoryConfig contains the source repository details
type RepositoryConfig struct {
	URL     string
	Branch  string
	AppPath string
}

// AgentTypeConfig contains the agent type and sub-type
type AgentTypeConfig struct {
	Type    string
	SubType string
}

// BuildConfig contains the build configuration (buildpack or docker)
type BuildConfig struct {
	Type      string           // "buildpack" or "docker"
	Buildpack *BuildpackConfig // non-nil if Type is "buildpack"
	Docker    *DockerConfig    // non-nil if Type is "docker"
}

// BuildpackConfig contains buildpack-specific configuration
type BuildpackConfig struct {
	Language        string
	LanguageVersion string
	RunCommand      string
}

// DockerConfig contains docker-specific configuration
type DockerConfig struct {
	DockerfilePath string
}

// Configurations contains environment variables for runtime
type Configurations struct {
	Env []EnvVar
}

// InputInterfaceConfig contains the endpoint configuration for custom-api agents
type InputInterfaceConfig struct {
	Type       string
	Port       int32
	SchemaPath string
	BasePath   string
}

// UpdateComponentRequest contains data for updating a component (patch operation)
type UpdateComponentBasicInfoRequest struct {
	DisplayName string
	Description string
}

// UpdateComponentBuildParametersRequest contains data for updating build parameters of a component
type UpdateComponentBuildParametersRequest struct {
	Repository     *RepositoryConfig     // nil if no change
	Build          *BuildConfig          // nil if no change
	InputInterface *InputInterfaceConfig // nil if no change
}

// UpdateComponentResourceConfigsRequest contains data for updating resource configurations of a component
type UpdateComponentResourceConfigsRequest struct {
	Replicas  *int32          // nil if no change
	Resources *ResourceConfig // nil if no change
}

// ResourceConfig contains CPU and memory resource configurations
type ResourceConfig struct {
	Requests *ResourceRequests // nil if no change
	Limits   *ResourceLimits   // nil if no change
}

// ResourceRequests contains resource requests
type ResourceRequests struct {
	CPU    string
	Memory string
}

// ResourceLimits contains resource limits
type ResourceLimits struct {
	CPU    string
	Memory string
}

// ComponentResourceConfigsResponse contains resource configurations response
type ComponentResourceConfigsResponse struct {
	Replicas             *int32          // Current replicas (env-specific or default)
	Resources            *ResourceConfig // Current resources (env-specific or default)
	DefaultReplicas      *int32          // Component-level default replicas (only when env provided)
	DefaultResources     *ResourceConfig // Component-level default resources (only when env provided)
	IsDefaultsOverridden *bool           // Whether env-specific overrides exist (only when env provided)
}

// DeployRequest contains data for deploying a component
type DeployRequest struct {
	ImageID string
	Env     []EnvVar
}

// EnvVar represents an environment variable for deployment
type EnvVar struct {
	Key   string
	Value string
}

// -----------------------------------------------------------------------------
// Internal workflow parameter types — used to parse the parameters map stored
// in a ComponentWorkflowRunResponse back into structured fields.
// -----------------------------------------------------------------------------

type workflowParameters struct {
	BuildpackConfigs buildpackConfigs   `json:"buildpackConfigs"`
	Endpoints        []workflowEndpoint `json:"endpoints"`
}

type buildpackConfigs struct {
	Language         string `json:"language"`
	LanguageVersion  string `json:"languageVersion,omitempty"`
	GoogleEntryPoint string `json:"googleEntryPoint,omitempty"`
}

type workflowEndpoint struct {
	Name           string `json:"name"`
	Port           int32  `json:"port"`
	Type           string `json:"type"`
	SchemaFilePath string `json:"schemaFilePath,omitempty"`
}
