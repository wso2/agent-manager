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

// AgentKindRef identifies the published Agent Kind version an agent was instantiated from.
// Non-nil only for kind-sourced internal agents.
type AgentKindRef struct {
	Name    string
	Version string
}

// CreateComponentRequest contains data for creating a component (agent) in OpenChoreo
type CreateComponentRequest struct {
	Name             string
	DisplayName      string
	Description      string
	ProvisioningType ProvisioningType
	Repository       *RepositoryConfig // nil for external or kind-sourced agents
	AgentKind        *AgentKindRef     // nil unless kind-sourced internal agent
	AgentType        AgentTypeConfig
	Build            *BuildConfig          // nil for external or kind-sourced agents
	Configurations   *Configurations       // nil for external agents or if no env vars
	InputInterface   *InputInterfaceConfig // nil unless custom-api
	HealthChecks     *HealthChecks         // nil for defaults or external agents
	// Labels holds user-defined labels to attach to the component alongside
	// the system-managed ones. User label keys can never collide with system
	// keys (all system keys are namespaced "openchoreo.dev/...", which user
	// keys are validated to never contain).
	Labels map[string]string
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

// Configurations contains environment variables and file mounts for runtime
type Configurations struct {
	Env   []EnvVar
	Files []FileVar
}

// InputInterfaceConfig contains the endpoint configuration for custom-api agents
type InputInterfaceConfig struct {
	Type       string
	Port       int32
	SchemaPath string
	// SchemaContent holds the resolved OpenAPI spec content, when available up front
	// (e.g. copied from a source agent when instantiating from an Agent Kind, which
	// has no git checkout/build step to resolve SchemaPath into content itself).
	SchemaContent string
	BasePath      string
}

// UpdateComponentRequest contains data for updating a component (patch operation)
type UpdateComponentBasicInfoRequest struct {
	DisplayName string
	Description string
	// Labels holds the caller's desired full set of user-defined labels. A
	// nil pointer means "leave labels unchanged"; a non-nil (possibly empty)
	// map replaces the entire user-label set while leaving system-managed
	// labels untouched.
	Labels *map[string]string
}

// UpdateComponentBuildParametersRequest contains data for updating build parameters of a component
type UpdateComponentBuildParametersRequest struct {
	Repository     *RepositoryConfig     // nil if no change
	Build          *BuildConfig          // nil if no change
	InputInterface *InputInterfaceConfig // nil if no change
	HealthChecks   *HealthChecks         // nil if no change
	AgentType      AgentTypeConfig       // Required for determining endpoint defaults
}

// ComponentDeploymentConfigRequest contains Component CR changes applied immediately before deploy.
type ComponentDeploymentConfigRequest struct {
	TraitsToAttach []TraitRequest
	TraitsToDetach []TraitType
	Env            []EnvVar
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

// ComponentParameters represents the component type parameters (must match agent-api.yaml schema)
type ComponentParameters struct {
	Exposed   bool            `json:"exposed"`
	Resources *ResourceConfig `json:"resources,omitempty"`
	// RoutePath is a bare path segment with no leading slash: the chart template
	// supplies the "/" when it renders the HTTPRoute path from this parameter.
	RoutePath string `json:"routePath,omitempty"`
	// Probes holds the agent's build-time health checks; nil leaves the defaults.
	Probes *HealthChecks `json:"probes,omitempty"`
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

// Health check (probe) check types.
const (
	ProbeTypeTCP  = "tcp"
	ProbeTypeHTTP = "http"
)

// probesKey is where health checks are stored: the Component's parameters.probes (build time)
// and a ReleaseBinding's componentTypeEnvironmentConfigs.probes (per-environment wait times).
const probesKey = "probes"

// ProbeTimings holds one health check's wait times. Fields are pointers so a partial value (a
// build-parameters update, an environment override) carries only the fields it sets. The JSON
// names match the probes schemas in component-types/agent-api.yaml.
type ProbeTimings struct {
	InitialDelaySeconds *int32 `json:"initialDelaySeconds,omitempty"`
	PeriodSeconds       *int32 `json:"periodSeconds,omitempty"`
	TimeoutSeconds      *int32 `json:"timeoutSeconds,omitempty"`
	FailureThreshold    *int32 `json:"failureThreshold,omitempty"`
}

// HealthCheck is one health check: what it checks and how long it waits. A nil Port means the
// agent's endpoint port. ProbeTimings is embedded, so its fields are HealthCheck's own, in Go
// and in JSON.
type HealthCheck struct {
	Enabled *bool   `json:"enabled,omitempty"`
	Type    *string `json:"type,omitempty"`
	Port    *int32  `json:"port,omitempty"`
	Path    *string `json:"path,omitempty"`
	ProbeTimings
}

// HealthChecks is an agent's health checks, set at build time and stored as the Component's
// parameters.probes.
type HealthChecks struct {
	Startup   *HealthCheck `json:"startup,omitempty"`
	Readiness *HealthCheck `json:"readiness,omitempty"`
	Liveness  *HealthCheck `json:"liveness,omitempty"`
}

// HealthCheckTimings holds one environment's wait-time overrides, stored on its ReleaseBinding
// as componentTypeEnvironmentConfigs.probes. A nil check or field follows the build-time value.
type HealthCheckTimings struct {
	Startup   *ProbeTimings `json:"startup,omitempty"`
	Readiness *ProbeTimings `json:"readiness,omitempty"`
	Liveness  *ProbeTimings `json:"liveness,omitempty"`
}

// CreateEnvironmentRequest contains data for creating an environment
type CreateEnvironmentRequest struct {
	Name          string
	DisplayName   string
	Description   string
	IsolationTier string
	DataplaneRef  string
	IsProduction  bool
	Gateway       *GatewaySpec
}

// UpdateEnvironmentRequest contains data for updating an environment
type UpdateEnvironmentRequest struct {
	DisplayName  *string
	Description  *string
	IsProduction *bool
	Gateway      *GatewaySpec
}

// GatewaySpec is the subset of the OC EnvironmentSpec.Gateway configuration that
// callers can set. OC-only runtime fields (gateway resource Name/Namespace,
// listener Name) are filled in by the client when building the OC payload.
type GatewaySpec struct {
	Ingress *GatewayNetworkSpec
	Egress  *GatewayNetworkSpec
}

// GatewayNetworkSpec splits a network direction (ingress/egress) into external
// and internal endpoints.
type GatewayNetworkSpec struct {
	External *GatewayEndpointSpec
	Internal *GatewayEndpointSpec
}

// GatewayEndpointSpec is the listener configuration for one endpoint.
type GatewayEndpointSpec struct {
	HTTP  *GatewayListenerSpec
	HTTPS *GatewayListenerSpec
	TLS   *GatewayListenerSpec
}

// GatewayListenerSpec is the subset of GatewayListenerSpec exposed through the
// agent-manager API; ListenerName is constructed by the client.
type GatewayListenerSpec struct {
	Port *int32
	Host *string
}

// DeployRequest contains data for deploying a component
// Env vars and file mounts are deliberately not here: they are per-environment and belong on the
// environment's ReleaseBinding workloadOverrides, not on the component-wide Workload that every
// environment renders from.
type DeployRequest struct {
	ImageID string
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

// FileVar represents a file mount configuration for deployment
type FileVar struct {
	Key       string
	MountPath string
	Value     string
	ValueFrom *EnvVarValueFrom
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
	SchemaContent  string   `json:"schemaContent,omitempty"`
}

// CreateSecretReferenceRequest contains data for creating a SecretReference CR
type CreateSecretReferenceRequest struct {
	Namespace       string   // Namespace where SecretReference will be created
	Name            string   // Name of the SecretReference
	ProjectName     string   // Project name for labels
	ComponentName   string   // Component name for labels
	KVPath          string   // Path in the backing KV store
	SecretKeys      []string // Keys to extract from KV path
	RefreshInterval string   // How often to refresh (e.g., "1h", "15s")
	// TemplateAnnotations are set on the generated Secret's template metadata.
	// Changing an annotation is a real spec change, so an update carrying a fresh
	// value (e.g. a rotated-at timestamp) forces the OpenChoreo controller to
	// re-sync the Secret from the KV store immediately instead of waiting for the
	// next RefreshInterval tick — used after credential rotation.
	TemplateAnnotations map[string]string
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
