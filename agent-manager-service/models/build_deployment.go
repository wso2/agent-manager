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

package models

import "time"

// DeploymentResponse represents deployment information
type DeploymentResponse struct {
	AgentName                  string                      `json:"agentName"`
	ProjectName                string                      `json:"projectName"`
	ImageId                    string                      `json:"imageId"`
	Status                     string                      `json:"status"`
	Environment                string                      `json:"environment"`
	EnvironmentDisplayName     string                      `json:"environmentDisplayName"`
	PromotionTargetEnvironment *PromotionTargetEnvironment `json:"promotionTargetEnvironment,omitempty"`
	LastDeployedAt             *time.Time                  `json:"lastDeployedAt,omitempty"`
	Endpoints                  []Endpoint                  `json:"endpoints"`
	// KindVersion is the published Agent Kind version whose image this deployment
	// runs, resolved from ImageId. It is per-environment and reflects what is
	// actually deployed, unlike the component's creation-time kind-version label,
	// which does not move when an agent is redeployed on a newer version. Empty
	// for source-built agents and when the image matches no published version.
	KindVersion string `json:"kindVersion,omitempty"`
	// EnvironmentUUID is the environment's id, read with the deployments; empty on an entry synthesized for a
	// kind-sourced agent.
	EnvironmentUUID string `json:"-"`
	// GatewayStatus and GatewayStatusReason are the agent's deployment on its environment's ingress gateway; empty
	// for an agent the gateway does not serve.
	GatewayStatus       string `json:"gatewayStatus,omitempty"`
	GatewayStatusReason string `json:"gatewayStatusReason,omitempty"`
	// HealthChecks are the health checks the environment runs: those of the build it runs.
	// Nil when the build has none, so the defaults apply.
	HealthChecks *HealthChecks `json:"healthChecks,omitempty"`
}

// PromotionTargetEnvironment represents environment promotion targets
type PromotionTargetEnvironment struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

// EndpointsResponse represents detailed endpoint information
type EndpointsResponse struct {
	Endpoint
	Schema EndpointSchema `json:"schema"`
}

// EndpointSchema represents the schema for an endpoint
type EndpointSchema struct {
	Content string `json:"content"`
}

// Endpoint represents endpoint configuration
type Endpoint struct {
	URL        string `json:"url"`
	Name       string `json:"name,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

// EnvVars represents environment variables
type EnvVars struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	IsSensitive bool   `json:"isSensitive,omitempty"`
	SecretRef   string `json:"secretRef,omitempty"`
	SecretKey   string `json:"secretKey,omitempty"` // The key within the secret (e.g., "api-key")
	IsSystem    bool   `json:"isSystem,omitempty"`  // Platform-injected (e.g. LLM_PROVIDER_URL); read-only for users
}

// FileMountEntry represents a file mount configuration
type FileMountEntry struct {
	Key         string `json:"key"`
	MountPath   string `json:"mountPath"`
	Value       string `json:"value,omitempty"`
	IsSensitive bool   `json:"isSensitive,omitempty"`
	SecretRef   string `json:"secretRef,omitempty"`
}

// Build represents a build instance
type BuildResponse struct {
	UUID            string          `json:"uuid"`
	Name            string          `json:"name"`
	AgentName       string          `json:"agentName"`
	ProjectName     string          `json:"projectName"`
	Status          string          `json:"status"`
	StartedAt       time.Time       `json:"startedAt"`
	ImageId         string          `json:"imageId,omitempty"`
	EndedAt         *time.Time      `json:"endedAt,omitempty"`
	BuildParameters BuildParameters `json:"buildParameters"`
	// HealthChecks are the health checks this build recorded when it started. Nil when it
	// has none, so an environment running this build uses the defaults.
	HealthChecks *HealthChecks `json:"healthChecks,omitempty"`
}

// BuildParameters represents parameters for a build
type BuildParameters struct {
	RepoUrl         string `json:"repoUrl"`
	AppPath         string `json:"appPath"`
	Branch          string `json:"branch"`
	CommitID        string `json:"commitId"`
	Language        string `json:"language"`
	LanguageVersion string `json:"languageVersion,omitempty"`
	RunCommand      string `json:"runCommand,omitempty"`
	DockerfilePath  string `json:"dockerfilePath,omitempty"`
}

// BuildStep represents a step in the build process
type BuildStep struct {
	Type       string     `json:"type"`
	Status     string     `json:"status"`
	Message    string     `json:"message"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// InputInterface represents endpoint configuration
type InputInterface struct {
	Type       string                `json:"type"`
	Port       int32                 `json:"port,omitempty"`
	BasePath   string                `json:"basePath"`
	Visibility []string              `json:"visibility"`
	Schema     *InputInterfaceSchema `json:"schema,omitempty"`
}

// HealthChecks are an agent's startup, readiness and liveness checks. They are
// saved in the agent's build settings under the same field names, so each build
// records its own copy. A check that is nil uses the default.
type HealthChecks struct {
	Startup   *HealthCheck `json:"startup,omitempty"`
	Readiness *HealthCheck `json:"readiness,omitempty"`
	Liveness  *HealthCheck `json:"liveness,omitempty"`
}

// HealthCheck is one health check. A field that is nil uses its default.
type HealthCheck struct {
	Enabled             *bool   `json:"enabled,omitempty"`
	Type                *string `json:"type,omitempty"`
	Path                *string `json:"path,omitempty"`
	Port                *int32  `json:"port,omitempty"`
	InitialDelaySeconds *int32  `json:"initialDelaySeconds,omitempty"`
	PeriodSeconds       *int32  `json:"periodSeconds,omitempty"`
	TimeoutSeconds      *int32  `json:"timeoutSeconds,omitempty"`
	FailureThreshold    *int32  `json:"failureThreshold,omitempty"`
}

// InputInterfaceSchema represents schema configuration
type InputInterfaceSchema struct {
	Path    string `json:"path,omitempty"`
	Content string `json:"content,omitempty"`
}

// BuildDetails represents detailed build information
type BuildDetailsResponse struct {
	BuildResponse
	Percent         float32         `json:"percent,omitempty"`
	Steps           []BuildStep     `json:"steps,omitempty"`
	DurationSeconds int32           `json:"durationSeconds,omitempty"`
	InputInterface  *InputInterface `json:"inputInterface,omitempty"`
}
