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
type TraitKind string

const (
	TraitKindClusterTrait TraitKind = "ClusterTrait"
	TraitKindTrait        TraitKind = "Trait"
)

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
	URL       string
	Branch    string
	AppPath   string
	SecretRef string // Optional: name of the git secret for authentication
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
	AgentType      AgentTypeConfig       // Required for determining endpoint defaults
}

// UpdateComponentResourceConfigsRequest contains data for updating resource configurations of a component
type UpdateComponentResourceConfigsRequest struct {
	Replicas    *int32             // nil if no change
	Resources   *ResourceConfig    // nil if no change
	AutoScaling *AutoScalingConfig // nil if no change
}

// ResourceConfig contains CPU and memory resource configurations
type ResourceConfig struct {
	Requests *ResourceRequests `json:"requests,omitempty"`
	Limits   *ResourceLimits   `json:"limits,omitempty"`
}

// ResourceRequests contains resource requests
type ResourceRequests struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// ResourceLimits contains resource limits
type ResourceLimits struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// AutoScalingConfig contains autoscaling configuration (must match hpa-trait.yaml envOverrides schema)
type AutoScalingConfig struct {
	Enabled                        *bool  `json:"enabled,omitempty"`
	MinReplicas                    *int32 `json:"minReplicas,omitempty"`
	MaxReplicas                    *int32 `json:"maxReplicas,omitempty"`
	TargetCPUUtilizationPercentage *int32 `json:"cpuUtilizationPercentage,omitempty"`
}

// CORSConfig contains CORS configuration
type CORSConfig struct {
	AllowOrigin  []string `json:"allowOrigin,omitempty"`
	AllowMethods []string `json:"allowMethods,omitempty"`
	AllowHeaders []string `json:"allowHeaders,omitempty"`
}

// ComponentParameters represents the component type parameters (must match agent-api.yaml schema)
type ComponentParameters struct {
	Exposed bool `json:"exposed"`
}

// EnvOverrideParameters represents environment-specific overrides (must match agent-api.yaml envOverrides schema)
type EnvOverrideParameters struct {
	Replicas        *int               `json:"replicas,omitempty"`
	Resources       *ResourceConfig    `json:"resources,omitempty"`
	ImagePullPolicy string             `json:"imagePullPolicy,omitempty"`
	RestartedAt     string             `json:"restartedAt,omitempty"`
	Autoscaling     *AutoScalingConfig `json:"autoscaling,omitempty"`
}

// ComponentResourceConfigsResponse contains resource configurations response
type ComponentResourceConfigsResponse struct {
	Replicas    *int32             // Current replicas
	Resources   *ResourceConfig    // Current resources
	AutoScaling *AutoScalingConfig // Current autoscaling configuration (if applicable)
}

// DeployRequest contains data for deploying a component
type DeployRequest struct {
	ImageID string
	Env     []EnvVar
}

// EnvVar represents an environment variable for deployment
type EnvVar struct {
	Key       string
	Value     string
	ValueFrom *EnvVarValueFrom
}

// EnvVarValueFrom represents a source for the value of an EnvVar
type EnvVarValueFrom struct {
	SecretKeyRef *SecretKeyRef
}

// SecretKeyRef selects a key of a Secret
type SecretKeyRef struct {
	Name string // Name of the secret
	Key  string // Key within the secret
}

// -----------------------------------------------------------------------------
// Internal workflow parameter types — used to parse the parameters map stored
// in a ComponentWorkflowRunResponse back into structured fields.
// -----------------------------------------------------------------------------

type workflowParameters struct {
	BuildEnv  []buildEnvVar      `json:"buildEnv"`
	Endpoints []workflowEndpoint `json:"endpoints"`
}

// buildEnvVar represents a build environment variable
type buildEnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type workflowEndpoint struct {
	Name           string   `json:"name"`
	Port           int32    `json:"port"`
	Type           string   `json:"type"`
	BasePath       string   `json:"basePath"`
	Visibility     []string `json:"visibility"`
	SchemaFilePath string   `json:"schemaFilePath,omitempty"`
}

// CreateSecretReferenceRequest contains data for creating a SecretReference CR
type CreateSecretReferenceRequest struct {
	Namespace       string   // Namespace where SecretReference will be created
	Name            string   // Name of the SecretReference
	ProjectName     string   // Project name for labels
	ComponentName   string   // Component name for labels
	KVPath          string   // Path in OpenBao KV store
	SecretKeys      []string // Keys to extract from KV path
	RefreshInterval string   // How often to refresh (e.g., "1h", "15s")
}

// SecretReferenceInfo contains info about a SecretReference CR
type SecretReferenceInfo struct {
	Name      string                 // Name of the SecretReference
	Namespace string                 // Namespace of the SecretReference
	Data      []SecretDataSourceInfo // Data sources in the SecretReference
}

// SecretDataSourceInfo contains info about a secret data source
type SecretDataSourceInfo struct {
	SecretKey string        // Key in the K8s secret
	RemoteRef RemoteRefInfo // Reference to the remote secret store
}

// RemoteRefInfo contains info about the remote reference
type RemoteRefInfo struct {
	Key      string // Path/Key in the remote secret store
	Property string // Property within the key (optional)
}

// -----------------------------------------------------------------------------
// Git Secret Types
// -----------------------------------------------------------------------------

// GitSecretType defines the type of git secret
type GitSecretType string

const (
	GitSecretTypeBasicAuth GitSecretType = "basic-auth"
)

// CreateGitSecretRequest contains data for creating a git secret via OpenChoreo
type CreateGitSecretRequest struct {
	Name       string        // Name of the git secret
	SecretType GitSecretType // Type of secret: "basic-auth"
	Username   string        // Username for basic auth (optional)
	Token      string        // Token/password for basic auth
}

// GitSecretInfo contains info about a git secret
type GitSecretInfo struct {
	Name              string // Name of the git secret
	Namespace         string // Namespace of the git secret
	WorkflowPlaneKind string // Kind of workflow plane (ClusterWorkflowPlane or WorkflowPlane)
	WorkflowPlaneName string // Name of the workflow plane
}
