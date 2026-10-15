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

package framework

import "time"

// ---------------------------------------------------------------------------
// Error
// ---------------------------------------------------------------------------

// ErrorResponse matches the standard API error envelope.
type ErrorResponse struct {
	Message string `json:"message"`
	Code    string `json:"code"`
	Reason  string `json:"reason,omitempty"`
}

// ---------------------------------------------------------------------------
// Organization
// ---------------------------------------------------------------------------

type OrganizationResponse struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName"`
	Description string    `json:"description"`
	Namespace   string    `json:"namespace"`
	CreatedAt   time.Time `json:"createdAt"`
}

type OrganizationListItem struct {
	Name string `json:"name"`
}

type OrganizationListResponse struct {
	Organizations []OrganizationListItem `json:"organizations"`
	Total         int                    `json:"total"`
	Limit         int                    `json:"limit"`
	Offset        int                    `json:"offset"`
}

// ---------------------------------------------------------------------------
// Project
// ---------------------------------------------------------------------------
// Agent
// ---------------------------------------------------------------------------

type Repository struct {
	URL     string `json:"url"`
	Branch  string `json:"branch"`
	AppPath string `json:"appPath,omitempty"`
}

type AgentKindRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Provisioning struct {
	Type       string        `json:"type"`
	Repository *Repository   `json:"repository,omitempty"`
	AgentKind  *AgentKindRef `json:"agentKind,omitempty"`
}

type AgentType struct {
	Type    string `json:"type"`
	SubType string `json:"subType,omitempty"`
}

type EnvironmentVariable struct {
	Key         string `json:"key"`
	Value       string `json:"value,omitempty"`
	IsSensitive bool   `json:"isSensitive,omitempty"`
	SecretRef   string `json:"secretRef,omitempty"`
}

type Configurations struct {
	Env                       []EnvironmentVariable `json:"env,omitempty"`
	EnableAutoInstrumentation *bool                 `json:"enableAutoInstrumentation,omitempty"`
}

type InputInterfaceSchema struct {
	Path string `json:"path"`
}

type InputInterface struct {
	Type     string                `json:"type"`
	Port     int                   `json:"port,omitempty"`
	BasePath string                `json:"basePath,omitempty"`
	Schema   *InputInterfaceSchema `json:"schema,omitempty"`
}

type BuildpackConfig struct {
	Language        string `json:"language"`
	LanguageVersion string `json:"languageVersion,omitempty"`
	RunCommand      string `json:"runCommand,omitempty"`
}

type DockerConfig struct {
	DockerfilePath string `json:"dockerfilePath"`
}

type BuildConfig struct {
	Type      string           `json:"type"`
	Buildpack *BuildpackConfig `json:"buildpack,omitempty"`
	Docker    *DockerConfig    `json:"docker,omitempty"`
}

type RuntimeConfigs struct {
	Env             []EnvironmentVariable `json:"env,omitempty"`
	Language        string                `json:"language,omitempty"`
	LanguageVersion string                `json:"languageVersion,omitempty"`
	RunCommand      string                `json:"runCommand,omitempty"`
}

type EnvModelConfigRequest struct {
	// ProviderName references an LLM provider (type "llm"); MCPProxyName references
	// an MCP proxy (type "mcp"). Exactly one is set depending on the config type.
	ProviderName  string                 `json:"providerName,omitempty"`
	MCPProxyName  string                 `json:"mcpProxyName,omitempty"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
}

type AuthInfo struct {
	Type  string  `json:"type"`
	In    string  `json:"in"`
	Name  string  `json:"name"`
	Value *string `json:"value,omitempty"`
}

type ProviderConfig struct {
	ProviderName string      `json:"providerName"`
	ProxyUuid    string      `json:"proxyUuid"`
	URL          string      `json:"url"`
	AuthInfo     *AuthInfo   `json:"authInfo,omitempty"`
	Policies     []LLMPolicy `json:"policies,omitempty"`
	Status       *string     `json:"status,omitempty"`
}

type EnvProviderConfigMapping struct {
	EnvironmentName string          `json:"environmentName"`
	Configuration   *ProviderConfig `json:"configuration,omitempty"`
}

type EnvironmentVariableConfig struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type ModelConfigRequest struct {
	ProviderName         string                      `json:"providerName"`
	Configuration        map[string]interface{}      `json:"configuration,omitempty"`
	EnvironmentVariables []EnvironmentVariableConfig `json:"environmentVariables,omitempty"`
}

type CreateAgentRequest struct {
	Name           string               `json:"name"`
	DisplayName    string               `json:"displayName"`
	Description    string               `json:"description,omitempty"`
	Provisioning   Provisioning         `json:"provisioning"`
	AgentType      AgentType            `json:"agentType"`
	Build          *BuildConfig         `json:"build,omitempty"`
	Configurations *Configurations      `json:"configurations,omitempty"`
	RuntimeConfigs *RuntimeConfigs      `json:"runtimeConfigs,omitempty"`
	InputInterface *InputInterface      `json:"inputInterface,omitempty"`
	ModelConfig    []ModelConfigRequest `json:"modelConfig,omitempty"`
}

type UpdateAgentRequest struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

type AgentResponse struct {
	UUID         string       `json:"uuid"`
	Name         string       `json:"name"`
	DisplayName  string       `json:"displayName"`
	Description  string       `json:"description"`
	ProjectName  string       `json:"projectName"`
	Status       string       `json:"status"`
	Provisioning Provisioning `json:"provisioning"`
	AgentType    AgentType    `json:"agentType"`
	CreatedAt    time.Time    `json:"createdAt"`
}

type AgentListResponse struct {
	Agents []AgentResponse `json:"agents"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

type AgentIdentityEnvironmentView struct {
	EnvironmentName  string `json:"environmentName"`
	ProvisioningType string `json:"provisioningType"`
	Status           string `json:"status"`
	AgentID          string `json:"agentId,omitempty"`
	ClientID         string `json:"clientId,omitempty"`
	LastError        string `json:"lastError,omitempty"`
	RequestedBy      string `json:"requestedBy,omitempty"`
}

type AgentIdentityActionRequest struct {
	Environment string `json:"environment"`
}

type AgentRegenerateSecretResponse struct {
	EnvironmentName        string `json:"environmentName"`
	ProvisioningType       string `json:"provisioningType"`
	ClientID               string `json:"clientId"`
	ClientSecret           string `json:"clientSecret"`
	Status                 string `json:"status"`
	WorkloadRefreshWarning string `json:"workloadRefreshWarning,omitempty"`
}

type AgentRevokeSecretResponse struct {
	EnvironmentName        string `json:"environmentName"`
	ClientID               string `json:"clientId"`
	Status                 string `json:"status"`
	WorkloadRefreshWarning string `json:"workloadRefreshWarning,omitempty"`
}

// ---------------------------------------------------------------------------
// Agent Catalog / Kinds
// ---------------------------------------------------------------------------

// KindConfigSchemaEntry defines a single runtime configuration parameter
// exposed by a published agent kind.
type KindConfigSchemaEntry struct {
	Name         string  `json:"name"`
	IsSecret     bool    `json:"isSecret"`
	IsMandatory  bool    `json:"isMandatory"`
	DefaultValue *string `json:"defaultValue"`
}

type PublishKindRequest struct {
	KindName        string                  `json:"kindName"`
	KindDisplayName string                  `json:"kindDisplayName"`
	KindDescription string                  `json:"kindDescription,omitempty"`
	Version         string                  `json:"version"`
	BuildName       string                  `json:"buildName"`
	ConfigSchema    []KindConfigSchemaEntry `json:"configSchema,omitempty"`
}

type PublishKindResponse struct {
	KindName    string `json:"kindName"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

type TokenRequest struct {
	ExpiresIn string `json:"expires_in,omitempty"`
}

type TokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	IssuedAt  int64  `json:"issued_at"`
	TokenType string `json:"token_type"`
}

// ---------------------------------------------------------------------------

type CreateProjectRequest struct {
	Name               string  `json:"name"`
	DisplayName        string  `json:"displayName"`
	Description        *string `json:"description,omitempty"`
	DeploymentPipeline string  `json:"deploymentPipeline"`
}

type UpdateProjectRequest struct {
	DisplayName        string `json:"displayName"`
	Description        string `json:"description"`
	DeploymentPipeline string `json:"deploymentPipeline"`
}

type ProjectResponse struct {
	UUID               string    `json:"uuid"`
	Name               string    `json:"name"`
	OrgName            string    `json:"orgName"`
	DisplayName        string    `json:"displayName"`
	Description        string    `json:"description"`
	DeploymentPipeline string    `json:"deploymentPipeline"`
	CreatedAt          time.Time `json:"createdAt"`
}

type ProjectListItem struct {
	UUID               string    `json:"uuid"`
	Name               string    `json:"name"`
	OrgName            string    `json:"orgName"`
	DisplayName        string    `json:"displayName"`
	Description        string    `json:"description"`
	DeploymentPipeline string    `json:"deploymentPipeline"`
	CreatedAt          time.Time `json:"createdAt"`
}

type ProjectListResponse struct {
	Projects []ProjectListItem `json:"projects"`
	Total    int               `json:"total"`
	Limit    int               `json:"limit"`
	Offset   int               `json:"offset"`
}

// ---------------------------------------------------------------------------
// Environment
// ---------------------------------------------------------------------------

type CreateEnvironmentRequest struct {
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	Description  string `json:"description,omitempty"`
	DataplaneRef string `json:"dataplaneRef"`
	DnsPrefix    string `json:"dnsPrefix"`
	IsProduction *bool  `json:"isProduction,omitempty"`
}

type UpdateEnvironmentRequest struct {
	DisplayName *string `json:"displayName,omitempty"`
	Description *string `json:"description,omitempty"`
}

type EnvironmentResponse struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	DataplaneRef string    `json:"dataplaneRef"`
	DisplayName  string    `json:"displayName"`
	IsProduction bool      `json:"isProduction"`
	DnsPrefix    string    `json:"dnsPrefix"`
	CreatedAt    time.Time `json:"createdAt"`
}

// EnvironmentListResponse is a JSON array of Environment objects.
// The API returns a bare array, not an envelope.
type EnvironmentListResponse []EnvironmentResponse

type ThunderInstanceResponse struct {
	EnvName      string `json:"envName"`
	DisplayName  string `json:"displayName"`
	IsProduction bool   `json:"isProduction"`
	IssuerURL    string `json:"issuerUrl"`
	TokenURL     string `json:"tokenUrl"`
	JWKSURL      string `json:"jwksUrl"`
	Namespace    string `json:"namespace"`
}

type ThunderInstanceListResponse struct {
	ThunderInstances []ThunderInstanceResponse `json:"thunderInstances"`
}

// ---------------------------------------------------------------------------
// Gateway
// ---------------------------------------------------------------------------

type CreateGatewayRequest struct {
	Name           string   `json:"name"`
	DisplayName    string   `json:"displayName"`
	GatewayType    string   `json:"gatewayType"`
	Vhost          string   `json:"vhost"`
	RuntimeUrl     string   `json:"runtimeUrl,omitempty"`
	Region         string   `json:"region,omitempty"`
	IsCritical     *bool    `json:"isCritical,omitempty"`
	EnvironmentIds []string `json:"environmentIds,omitempty"`
}

type UpdateGatewayRequest struct {
	DisplayName *string `json:"displayName,omitempty"`
	RuntimeUrl  *string `json:"runtimeUrl,omitempty"`
	IsCritical  *bool   `json:"isCritical,omitempty"`
}

type GatewayResponse struct {
	UUID             string          `json:"uuid"`
	OrganizationName string          `json:"organizationName"`
	Name             string          `json:"name"`
	DisplayName      string          `json:"displayName"`
	GatewayType      string          `json:"gatewayType"`
	Vhost            string          `json:"vhost"`
	RuntimeUrl       string          `json:"runtimeUrl,omitempty"`
	Region           string          `json:"region,omitempty"`
	IsCritical       bool            `json:"isCritical"`
	Status           string          `json:"status"`
	Environments     []GatewayEnvRef `json:"environments,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

// GatewayEnvRef is an environment associated with a gateway, as returned in the
// gateways list response.
type GatewayEnvRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type GatewayListResponse struct {
	Gateways []GatewayResponse `json:"gateways"`
	Total    int               `json:"total"`
	Limit    int               `json:"limit"`
	Offset   int               `json:"offset"`
}

// ---------------------------------------------------------------------------
// Evaluator
// ---------------------------------------------------------------------------

type EvaluatorResponse struct {
	ID          string   `json:"id"`
	Identifier  string   `json:"identifier"`
	DisplayName string   `json:"displayName"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Provider    string   `json:"provider"`
	Level       string   `json:"level"`
	Tags        []string `json:"tags"`
	IsBuiltin   bool     `json:"isBuiltin"`
	Type        string   `json:"type,omitempty"`
	Source      string   `json:"source,omitempty"`
}

type EvaluatorListResponse struct {
	Evaluators []EvaluatorResponse `json:"evaluators"`
	Total      int                 `json:"total"`
	Limit      int                 `json:"limit"`
	Offset     int                 `json:"offset"`
}

type CreateCustomEvaluatorRequest struct {
	Identifier  string `json:"identifier,omitempty"`
	DisplayName string `json:"displayName"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	Level       string `json:"level"`
	Source      string `json:"source"`
	Version     string `json:"version,omitempty"`
}

type UpdateCustomEvaluatorRequest struct {
	DisplayName string   `json:"displayName,omitempty"`
	Description string   `json:"description,omitempty"`
	Source      string   `json:"source,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// ---------------------------------------------------------------------------
// Deployment Pipeline
// ---------------------------------------------------------------------------

type DeploymentPipelineResponse struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName"`
	Description string    `json:"description"`
	OrgName     string    `json:"orgName"`
	CreatedAt   time.Time `json:"createdAt"`
}

type DeploymentPipelineListResponse struct {
	DeploymentPipelines []DeploymentPipelineResponse `json:"deploymentPipelines"`
	Total               int                          `json:"total"`
	Limit               int                          `json:"limit"`
	Offset              int                          `json:"offset"`
}

// ---------------------------------------------------------------------------
// Catalog / Agent Kinds
// ---------------------------------------------------------------------------

// AgentKindListItem is one entry in the /agent-kinds list response. The /agent-kinds
// endpoint is the source of truth for published kinds, so kind-existence checks query this.
type AgentKindListItem struct {
	Name          string `json:"name"`
	LatestVersion string `json:"latestVersion"`
}

type AgentKindListResponse struct {
	Kinds []AgentKindListItem `json:"kinds"`
	Total int                 `json:"total"`
}

// ---------------------------------------------------------------------------
// LLM Common Types
// ---------------------------------------------------------------------------

type UpstreamAuth struct {
	Type   string  `json:"type"`
	Header *string `json:"header,omitempty"`
	Value  *string `json:"value,omitempty"`
}

type UpstreamEndpoint struct {
	URL  *string       `json:"url,omitempty"`
	Ref  *string       `json:"ref,omitempty"`
	Auth *UpstreamAuth `json:"auth,omitempty"`
}

type UpstreamConfig struct {
	Main    *UpstreamEndpoint `json:"main,omitempty"`
	Sandbox *UpstreamEndpoint `json:"sandbox,omitempty"`
}

type LLMPolicyPath struct {
	Path    string         `json:"path"`
	Methods []string       `json:"methods"`
	Params  map[string]any `json:"params,omitempty"`
}

type LLMPolicy struct {
	Name    string          `json:"name"`
	Version string          `json:"version"`
	Paths   []LLMPolicyPath `json:"paths"`
}

// ---------------------------------------------------------------------------
// LLM Provider Template
// ---------------------------------------------------------------------------

type CreateLLMProviderTemplateRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type LLMProviderTemplateMetadataAuth struct {
	Type        string `json:"type"`
	Header      string `json:"header"`
	ValuePrefix string `json:"valuePrefix,omitempty"`
}

type LLMProviderTemplateMetadata struct {
	EndpointURL string                           `json:"endpointUrl,omitempty"`
	Auth        *LLMProviderTemplateMetadataAuth `json:"auth,omitempty"`
}

type LLMProviderTemplateResponse struct {
	UUID        string                       `json:"uuid"`
	ID          string                       `json:"id"`
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Metadata    *LLMProviderTemplateMetadata `json:"metadata,omitempty"`
	CreatedAt   time.Time                    `json:"createdAt,omitempty"`
}

type LLMProviderTemplateListResponse struct {
	Templates []LLMProviderTemplateResponse `json:"templates"`
	Total     int                           `json:"total"`
	Limit     int                           `json:"limit"`
	Offset    int                           `json:"offset"`
}

// ---------------------------------------------------------------------------
// LLM Provider
// ---------------------------------------------------------------------------

type SecurityAPIKey struct {
	Enabled bool   `json:"enabled"`
	Key     string `json:"key"`
	In      string `json:"in"`
}

type SecurityIdentity struct {
	Enabled bool `json:"enabled"`
}

type SecurityConfig struct {
	Enabled  bool              `json:"enabled"`
	APIKey   *SecurityAPIKey   `json:"apiKey,omitempty"`
	Identity *SecurityIdentity `json:"identity,omitempty"`
}

type LLMAccessControl struct {
	Mode       string   `json:"mode"`
	Exceptions []string `json:"exceptions,omitempty"`
}

type CreateLLMProviderRequest struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Description   *string           `json:"description,omitempty"`
	Version       string            `json:"version"`
	Context       string            `json:"context"`
	Template      string            `json:"template"`
	Upstream      UpstreamConfig    `json:"upstream"`
	Security      *SecurityConfig   `json:"security,omitempty"`
	AccessControl *LLMAccessControl `json:"accessControl,omitempty"`
	Policies      []LLMPolicy       `json:"policies,omitempty"`
	Gateways      []string          `json:"gateways,omitempty"`
}

type LLMProviderResponse struct {
	UUID      string         `json:"uuid"`
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Version   string         `json:"version"`
	Context   string         `json:"context"`
	Template  string         `json:"template"`
	Upstream  UpstreamConfig `json:"upstream"`
	Policies  []LLMPolicy    `json:"policies,omitempty"`
	Status    string         `json:"status"`
	Gateways  []string       `json:"gateways,omitempty"`
	CreatedAt *time.Time     `json:"createdAt,omitempty"`
}

type LLMProviderListResponse struct {
	Providers []LLMProviderResponse `json:"llmProviders"`
	Total     int                   `json:"total"`
	Limit     int                   `json:"limit"`
	Offset    int                   `json:"offset"`
}

// ---------------------------------------------------------------------------
// LLM Proxy
// ---------------------------------------------------------------------------

type LLMProxyConfig struct {
	Name     *string     `json:"name,omitempty"`
	Version  *string     `json:"version,omitempty"`
	Context  *string     `json:"context,omitempty"`
	Vhost    *string     `json:"vhost,omitempty"`
	Provider *string     `json:"provider,omitempty"`
	Policies []LLMPolicy `json:"policies,omitempty"`
}

type CreateLLMProxyRequest struct {
	Description   *string        `json:"description,omitempty"`
	ProviderUuid  string         `json:"providerUuid"`
	Configuration LLMProxyConfig `json:"configuration"`
}

type UpdateLLMProxyRequest struct {
	Description   *string         `json:"description,omitempty"`
	ProviderUuid  *string         `json:"providerUuid,omitempty"`
	Configuration *LLMProxyConfig `json:"configuration,omitempty"`
}

type LLMProxyResponse struct {
	UUID          string         `json:"uuid"`
	ProjectID     string         `json:"projectId"`
	Description   *string        `json:"description,omitempty"`
	ProviderUuid  string         `json:"providerUuid"`
	Status        string         `json:"status"`
	Configuration LLMProxyConfig `json:"configuration"`
}

type LLMProxyListResponse struct {
	Proxies []LLMProxyResponse `json:"llmProxies"`
	Total   int                `json:"total"`
	Limit   int                `json:"limit"`
	Offset  int                `json:"offset"`
}

// ---------------------------------------------------------------------------
// MCP Proxy
// ---------------------------------------------------------------------------

// MCPPolicy is a policy attached to an MCP proxy. Unlike LLMPolicy it carries a
// flat params map rather than per-path entries.
type MCPPolicy struct {
	Name               string         `json:"name"`
	Version            string         `json:"version"`
	DisplayName        string         `json:"displayName,omitempty"`
	ExecutionCondition *string        `json:"executionCondition,omitempty"`
	Params             map[string]any `json:"params,omitempty"`
}

// MCPProxyCapabilities holds the capabilities discovered from the upstream MCP server.
type MCPProxyCapabilities struct {
	Tools     []map[string]any `json:"tools,omitempty"`
	Prompts   []map[string]any `json:"prompts,omitempty"`
	Resources []map[string]any `json:"resources,omitempty"`
}

// MCPEndpointEnvironment is one endpoint→environment binding on an MCP proxy. On write only
// EnvironmentUUID is honoured (the endpoint is deployed to that env); on read DeploymentStatus
// reports the per-environment status ("Deployed"/"Undeployed"). Within one proxy an environment
// maps to at most one endpoint.
type MCPEndpointEnvironment struct {
	EnvironmentUUID  string `json:"environmentUuid"`
	DeploymentStatus string `json:"deploymentStatus,omitempty"`
}

// MCPProxyEndpoint is one deployable endpoint of an MCP proxy. It carries the upstream
// (URL + auth under Upstream.Main), policies, capabilities, security, and tool-scope bindings,
// and is deployed to one or more environments. ID is unique within the parent proxy.
type MCPProxyEndpoint struct {
	ID           string                   `json:"id"`
	Name         string                   `json:"name,omitempty"`
	Upstream     UpstreamConfig           `json:"upstream"`
	Policies     []MCPPolicy              `json:"policies,omitempty"`
	Capabilities *MCPProxyCapabilities    `json:"capabilities,omitempty"`
	Security     *SecurityConfig          `json:"security,omitempty"`
	Environments []MCPEndpointEnvironment `json:"environments,omitempty"`
}

type CreateMCPProxyRequest struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Version        string  `json:"version"`
	Description    *string `json:"description,omitempty"`
	Context        *string `json:"context,omitempty"`
	Vhost          *string `json:"vhost,omitempty"`
	McpSpecVersion *string `json:"mcpSpecVersion,omitempty"`
	// Endpoints are the deployable endpoint definitions. Each endpoint is deployed to the
	// environments listed in its Environments block on create/update.
	Endpoints []MCPProxyEndpoint `json:"endpoints,omitempty"`
}

type MCPProxyResponse struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Version        string  `json:"version"`
	Description    *string `json:"description,omitempty"`
	Context        *string `json:"context,omitempty"`
	Vhost          *string `json:"vhost,omitempty"`
	McpSpecVersion *string `json:"mcpSpecVersion,omitempty"`
	InCatalog      bool    `json:"inCatalog"`
	// Gateways aggregates the gateways across all per-endpoint/per-environment artifacts deployed.
	Gateways  []string           `json:"gateways,omitempty"`
	Endpoints []MCPProxyEndpoint `json:"endpoints,omitempty"`
	CreatedBy *string            `json:"createdBy,omitempty"`
	CreatedAt *time.Time         `json:"createdAt,omitempty"`
}

type MCPProxyListItem struct {
	ID      *string `json:"id,omitempty"`
	Name    *string `json:"name,omitempty"`
	Version *string `json:"version,omitempty"`
	Status  *string `json:"status,omitempty"`
}

// MCPProxyListResponse mirrors the org-scoped list payload ({count, list}).
type MCPProxyListResponse struct {
	Count int                `json:"count"`
	List  []MCPProxyListItem `json:"list"`
}

type MCPProxyScopeRequest struct {
	Action      string   `json:"action"`
	Description string   `json:"description,omitempty"`
	Tools       []string `json:"tools"`
}

type MCPProxyScopeResponse struct {
	Action      string   `json:"action"`
	Scope       string   `json:"scope"`
	Description string   `json:"description,omitempty"`
	Tools       []string `json:"tools"`
}

type AgentIdentityRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
}

type AgentIdentityRoleResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AgentIdentityAssignment struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type AgentIdentityAssignmentsRequest struct {
	Assignments []AgentIdentityAssignment `json:"assignments"`
}

type AgentIdentityRoleAssignmentsResponse struct {
	Agents []AgentIdentityAssignment `json:"agents"`
}

type AgentIdentityAgent struct {
	AgentName      string `json:"agentName"`
	ProjectName    string `json:"projectName"`
	Status         string `json:"status"`
	ThunderAgentID string `json:"thunderAgentId,omitempty"`
}

type AgentIdentityAgentListResponse struct {
	Agents []AgentIdentityAgent `json:"agents"`
}

type SecurityRuntimeProbeResponse struct {
	NonRoot                    bool   `json:"non_root"`
	RootFilesystemReadOnly     bool   `json:"root_filesystem_read_only"`
	TmpWritable                bool   `json:"tmp_writable"`
	ServiceAccountTokenPresent bool   `json:"service_account_token_present"`
	EffectiveCapabilitiesDrop  bool   `json:"effective_capabilities_dropped"`
	NoNewPrivileges            bool   `json:"no_new_privileges"`
	NoNewPrivilegesEvidence    string `json:"no_new_privileges_evidence"`
	SeccompEnabled             bool   `json:"seccomp_enabled"`
	SyscallConfined            bool   `json:"syscall_confined"`
	SyscallConfinementEvidence string `json:"syscall_confinement_evidence"`
}

type SecurityNetworkProbeResponse struct {
	Target     string `json:"target"`
	Outcome    string `json:"outcome"`
	Evidence   string `json:"evidence"`
	HTTPStatus *int   `json:"http_status"`
}

type SecurityIdentityProbeResponse struct {
	Configured      bool     `json:"configured"`
	TokenMinted     bool     `json:"token_minted"`
	StatusCode      *int     `json:"status_code"`
	RequestedScopes []string `json:"requested_scopes"`
	GrantedScopes   []string `json:"granted_scopes"`
	OAuthError      string   `json:"oauth_error"`
}

type SecurityMCPProbeResponse struct {
	Tool           string `json:"tool"`
	Phase          string `json:"phase"`
	TokenMinted    bool   `json:"token_minted"`
	HTTPStatus     *int   `json:"http_status"`
	Authorized     bool   `json:"authorized"`
	ResultReceived bool   `json:"result_received"`
	Error          string `json:"error"`
}

// MCPServerInfoFetchRequest is the body for the discover/fetch-server-info endpoint.
type MCPServerInfoFetchRequest struct {
	URL  string        `json:"url"`
	Auth *UpstreamAuth `json:"auth,omitempty"`
}

// MCPServerInfoFetchResponse is the capabilities payload returned by discovery.
type MCPServerInfoFetchResponse struct {
	ServerInfo map[string]any   `json:"serverInfo,omitempty"`
	Tools      []map[string]any `json:"tools,omitempty"`
	Prompts    []map[string]any `json:"prompts,omitempty"`
	Resources  []map[string]any `json:"resources,omitempty"`
}

// ---------------------------------------------------------------------------
// LLM API Key
// ---------------------------------------------------------------------------

type CreateLLMAPIKeyRequest struct {
	Name        *string `json:"name,omitempty"`
	DisplayName *string `json:"displayName,omitempty"`
	ExpiresAt   *string `json:"expiresAt,omitempty"`
}

type CreateLLMAPIKeyResponse struct {
	Status  string  `json:"status"`
	Message string  `json:"message"`
	KeyID   *string `json:"keyId,omitempty"`
	ApiKey  *string `json:"apiKey,omitempty"`
}

// ---------------------------------------------------------------------------
// Agent API Key
// ---------------------------------------------------------------------------

type CreateAgentAPIKeyRequest struct {
	DisplayName string `json:"displayName"`
	ExpiresAt   string `json:"expiresAt"`
}

type CreateAgentAPIKeyResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	KeyID   string `json:"keyId"`
	ApiKey  string `json:"apiKey"`
}

// ---------------------------------------------------------------------------
// Monitor
// ---------------------------------------------------------------------------

type MonitorEvaluator struct {
	Identifier  string         `json:"identifier"`
	DisplayName string         `json:"displayName"`
	Config      map[string]any `json:"config,omitempty"`
}

type MonitorLLMProviderRef struct {
	ProviderName string `json:"providerName"`
}

type CreateMonitorRequest struct {
	Name            string                 `json:"name"`
	DisplayName     string                 `json:"displayName"`
	Description     string                 `json:"description,omitempty"`
	EnvironmentName string                 `json:"environmentName"`
	Evaluators      []MonitorEvaluator     `json:"evaluators"`
	Type            string                 `json:"type"`
	LLMProvider     *MonitorLLMProviderRef `json:"llmProvider,omitempty"`
	IntervalMinutes int                    `json:"intervalMinutes,omitempty"`
	SamplingRate    *float64               `json:"samplingRate,omitempty"`
	TraceStart      *time.Time             `json:"traceStart,omitempty"`
	TraceEnd        *time.Time             `json:"traceEnd,omitempty"`
}

type UpdateMonitorRequest struct {
	DisplayName     string                 `json:"displayName,omitempty"`
	Evaluators      []MonitorEvaluator     `json:"evaluators,omitempty"`
	LLMProvider     *MonitorLLMProviderRef `json:"llmProvider,omitempty"`
	IntervalMinutes int                    `json:"intervalMinutes,omitempty"`
	SamplingRate    *float64               `json:"samplingRate,omitempty"`
}

type MonitorResponse struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	DisplayName     string             `json:"displayName"`
	Description     string             `json:"description"`
	Type            string             `json:"type"`
	OrgName         string             `json:"orgName"`
	ProjectName     string             `json:"projectName"`
	AgentName       string             `json:"agentName"`
	EnvironmentName string             `json:"environmentName"`
	Evaluators      []MonitorEvaluator `json:"evaluators"`
	SamplingRate    float64            `json:"samplingRate"`
	Status          string             `json:"status"`
	CreatedAt       time.Time          `json:"createdAt"`
}

type MonitorListResponse struct {
	Monitors []MonitorResponse `json:"monitors"`
	Total    int               `json:"total"`
}

// ---------------------------------------------------------------------------
// Monitor Runs & Scores
// ---------------------------------------------------------------------------

type EvaluatorScoreSummary struct {
	EvaluatorName string         `json:"evaluatorName"`
	Level         string         `json:"level"`
	Count         int32          `json:"count"`
	SkippedCount  int32          `json:"skippedCount"`
	Aggregations  map[string]any `json:"aggregations"`
}

type MonitorRunResponse struct {
	ID           string                  `json:"id"`
	RunName      string                  `json:"runName"`
	MonitorName  *string                 `json:"monitorName,omitempty"`
	Evaluators   []MonitorEvaluator      `json:"evaluators"`
	TraceStart   time.Time               `json:"traceStart"`
	TraceEnd     time.Time               `json:"traceEnd"`
	StartedAt    *time.Time              `json:"startedAt,omitempty"`
	CompletedAt  *time.Time              `json:"completedAt,omitempty"`
	Status       string                  `json:"status"`
	ErrorMessage *string                 `json:"errorMessage,omitempty"`
	Scores       []EvaluatorScoreSummary `json:"scores,omitempty"`
}

type MonitorRunListResponse struct {
	Runs  []MonitorRunResponse `json:"runs"`
	Total int32                `json:"total"`
}

type MonitorRunScoresResponse struct {
	RunID       string                  `json:"runId"`
	MonitorName string                  `json:"monitorName"`
	Evaluators  []EvaluatorScoreSummary `json:"evaluators"`
}

// ---------------------------------------------------------------------------
// Agent Configuration (env vars, resource configs)
// ---------------------------------------------------------------------------

type ConfigurationItem struct {
	Key         string `json:"key"`
	Value       string `json:"value,omitempty"`
	IsSensitive bool   `json:"isSensitive"`
	IsSystem    bool   `json:"isSystem"`
	SecretRef   string `json:"secretRef,omitempty"`
}

type ConfigurationFile struct {
	Key         string `json:"key,omitempty"`
	MountPath   string `json:"mountPath,omitempty"`
	IsSensitive bool   `json:"isSensitive,omitempty"`
	SecretRef   string `json:"secretRef,omitempty"`
}

// AgentConfigurations mirrors the API's "configurations" object, which groups
// environment variables and file mounts separately (not a flat list).
type AgentConfigurations struct {
	Env   []ConfigurationItem `json:"env"`
	Files []ConfigurationFile `json:"files"`
}

type ConfigurationResponse struct {
	ProjectName    string              `json:"projectName"`
	AgentName      string              `json:"agentName"`
	Environment    string              `json:"environment"`
	Configurations AgentConfigurations `json:"configurations"`
}

type ResourceRequests struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type ResourceLimits struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type ResourceConfig struct {
	Requests ResourceRequests `json:"requests"`
	Limits   ResourceLimits   `json:"limits"`
}

type AutoScalingConfig struct {
	Enabled     bool `json:"enabled"`
	MinReplicas int  `json:"minReplicas,omitempty"`
	MaxReplicas int  `json:"maxReplicas,omitempty"`
}

type UpdateAgentResourceConfigsRequest struct {
	Replicas    int               `json:"replicas"`
	Resources   ResourceConfig    `json:"resources"`
	AutoScaling AutoScalingConfig `json:"autoScaling"`
}

type AgentResourceConfigsResponse struct {
	Replicas    int               `json:"replicas"`
	Resources   ResourceConfig    `json:"resources"`
	AutoScaling AutoScalingConfig `json:"autoScaling"`
}

// ---------------------------------------------------------------------------
// Agent Model Configuration
// ---------------------------------------------------------------------------

type CreateAgentModelConfigRequest struct {
	Name                 string                           `json:"name"`
	Description          string                           `json:"description,omitempty"`
	Type                 string                           `json:"type"`
	EnvMappings          map[string]EnvModelConfigRequest `json:"envMappings"`
	EnvironmentVariables []EnvironmentVariableConfig      `json:"environmentVariables,omitempty"`
}

type UpdateAgentModelConfigRequest struct {
	Name                 string                           `json:"name,omitempty"`
	Description          string                           `json:"description,omitempty"`
	EnvMappings          map[string]EnvModelConfigRequest `json:"envMappings,omitempty"`
	EnvironmentVariables []EnvironmentVariableConfig      `json:"environmentVariables,omitempty"`
}

type AgentModelConfigResponse struct {
	UUID                 string                              `json:"uuid"`
	Name                 string                              `json:"name"`
	Description          string                              `json:"description"`
	AgentID              string                              `json:"agentId"`
	Type                 string                              `json:"type"`
	OrganizationName     string                              `json:"organizationName"`
	ProjectName          string                              `json:"projectName"`
	EnvMappings          map[string]EnvProviderConfigMapping `json:"envMappings"`
	EnvironmentVariables []EnvironmentVariableConfig         `json:"environmentVariables"`
	CreatedAt            time.Time                           `json:"createdAt"`
	UpdatedAt            time.Time                           `json:"updatedAt"`
}

type AgentModelConfigListResponse struct {
	Configs []AgentModelConfigResponse `json:"configs"`
}

// ---------------------------------------------------------------------------
// Traces (agent-manager-observer)
// ---------------------------------------------------------------------------

type TraceOverview struct {
	TraceID   string    `json:"traceId"`
	RootSpan  string    `json:"rootSpanName,omitempty"`
	StartTime time.Time `json:"startTime"`
	Duration  int64     `json:"duration,omitempty"`
	SpanCount int       `json:"spanCount,omitempty"`
}

type TraceOverviewListResponse struct {
	Traces []TraceOverview `json:"traces"`
	Total  int             `json:"total,omitempty"`
}

type SpanSummary struct {
	SpanID    string    `json:"spanId"`
	SpanName  string    `json:"spanName"`
	StartTime time.Time `json:"startTime"`
	Duration  int64     `json:"duration,omitempty"`
}

type SpanSummaryListResponse struct {
	Spans []SpanSummary `json:"spans"`
}

// ---------------------------------------------------------------------------
// Builds
// ---------------------------------------------------------------------------

type BuildOverview struct {
	BuildName string     `json:"buildName"`
	Status    *string    `json:"status,omitempty"`
	ImageID   *string    `json:"imageId,omitempty"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

type BuildsListResponse struct {
	Builds []BuildOverview `json:"builds"`
	Total  int32           `json:"total"`
	Limit  int32           `json:"limit"`
	Offset int32           `json:"offset"`
}

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Log       string    `json:"log"`
	LogLevel  string    `json:"logLevel"`
}

type LogsResponse struct {
	Logs       []LogEntry `json:"logs"`
	TotalCount int32      `json:"totalCount"`
	TookMs     float32    `json:"tookMs"`
}

type BuildStep struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type BuildDetailsResponse struct {
	BuildName       string      `json:"buildName"`
	Status          *string     `json:"status,omitempty"`
	ImageID         *string     `json:"imageId,omitempty"`
	Percent         *float32    `json:"percent,omitempty"`
	Steps           []BuildStep `json:"steps,omitempty"`
	DurationSeconds *int32      `json:"durationSeconds,omitempty"`
	StartedAt       time.Time   `json:"startedAt"`
	EndedAt         *time.Time  `json:"endedAt,omitempty"`
}

// ---------------------------------------------------------------------------
// Deployments
// ---------------------------------------------------------------------------

type DeployAgentRequest struct {
	ImageID                   string                `json:"imageId"`
	Env                       []EnvironmentVariable `json:"env,omitempty"`
	EnableAutoInstrumentation *bool                 `json:"enableAutoInstrumentation,omitempty"`
}

type DeployAgentResponse struct {
	AgentName   string `json:"agentName"`
	ProjectName string `json:"projectName"`
	ImageID     string `json:"imageId"`
	Environment string `json:"environment"`
}

type DeploymentEndpoint struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Visibility string `json:"visibility"`
}

type DeploymentDetailsResponse struct {
	ImageID                string               `json:"imageId"`
	Status                 string               `json:"status"`
	LastDeployed           time.Time            `json:"lastDeployed"`
	Endpoints              []DeploymentEndpoint `json:"endpoints"`
	EnvironmentDisplayName *string              `json:"environmentDisplayName,omitempty"`
}

// ---------------------------------------------------------------------------
// Endpoints
// ---------------------------------------------------------------------------

type EndpointSchema struct {
	Content string `json:"content"`
}

type EndpointConfiguration struct {
	URL          string          `json:"url"`
	EndpointName string          `json:"endpointName"`
	Visibility   string          `json:"visibility"`
	Schema       *EndpointSchema `json:"schema,omitempty"`
}

// ---------------------------------------------------------------------------
// Agent Chat Invocation
// ---------------------------------------------------------------------------

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AgentChatRequest struct {
	Messages []ChatMessage `json:"messages"`
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

type MetricDataPoint struct {
	Time  string  `json:"time"`
	Value float64 `json:"value"`
}

type MetricsResponse struct {
	CPUUsage       []MetricDataPoint `json:"cpuUsage"`
	CPURequests    []MetricDataPoint `json:"cpuRequests"`
	CPULimits      []MetricDataPoint `json:"cpuLimits"`
	Memory         []MetricDataPoint `json:"memory"`
	MemoryRequests []MetricDataPoint `json:"memoryRequests"`
	MemoryLimits   []MetricDataPoint `json:"memoryLimits"`
}

// --- Deployment pipeline & promotion ---

// TargetEnvironmentRef references a target environment within a promotion path.
type TargetEnvironmentRef struct {
	Name string `json:"name"`
}

// PromotionPath defines a source environment and the environments it can promote to.
type PromotionPath struct {
	SourceEnvironmentRef  string                 `json:"sourceEnvironmentRef"`
	TargetEnvironmentRefs []TargetEnvironmentRef `json:"targetEnvironmentRefs"`
}

// CreateDeploymentPipelineRequest is the body for creating an org deployment pipeline.
type CreateDeploymentPipelineRequest struct {
	DisplayName    string          `json:"displayName"`
	Description    *string         `json:"description,omitempty"`
	ProjectName    *string         `json:"projectName,omitempty"`
	PromotionPaths []PromotionPath `json:"promotionPaths"`
}

// PromoteAgentRequest is the body for promoting an agent from one environment to another.
type PromoteAgentRequest struct {
	SourceEnvironment         string                `json:"sourceEnvironment"`
	TargetEnvironment         string                `json:"targetEnvironment"`
	UseConfigFromSourceEnv    *bool                 `json:"useConfigFromSourceEnv,omitempty"`
	Env                       []EnvironmentVariable `json:"env,omitempty"`
	EnableAutoInstrumentation *bool                 `json:"enableAutoInstrumentation,omitempty"`
}

// PromoteAgentResponse is the 202 body returned by the promote endpoint.
type PromoteAgentResponse struct {
	AgentName         string `json:"agentName"`
	ProjectName       string `json:"projectName"`
	SourceEnvironment string `json:"sourceEnvironment"`
	TargetEnvironment string `json:"targetEnvironment"`
}
