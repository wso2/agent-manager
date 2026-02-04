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
	RuntimeConfigs   *RuntimeConfigs       // nil for external agents
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

// RuntimeConfigs contains runtime configuration for internal agents
type RuntimeConfigs struct {
	Language        string
	LanguageVersion string
	RunCommand      string
	Env             []EnvVar
}

// InputInterfaceConfig contains the endpoint configuration for custom-api agents
type InputInterfaceConfig struct {
	Type       string
	Port       int32
	SchemaPath string
	BasePath	string
}

// UpdateComponentRequest contains data for updating a component (patch operation)
type UpdateComponentRequest struct {
	AutoDeploy *bool
	Parameters map[string]any
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
