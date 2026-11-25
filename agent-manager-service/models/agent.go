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

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AgentResponse struct {
	UUID           string          `json:"uuid"`
	Name           string          `json:"name"`
	DisplayName    string          `json:"displayName,omitempty"`
	Description    string          `json:"description,omitempty"`
	ProjectName    string          `json:"projectName"`
	CreatedAt      time.Time       `json:"createdAt"`
	Status         string          `json:"status,omitempty"`
	Provisioning   Provisioning    `json:"provisioning,omitempty"`
	Type           AgentType       `json:"type,omitempty"`
	Build          *Build          `json:"build,omitempty"`
	InputInterface *InputInterface `json:"inputInterface,omitempty"`
	// HealthChecks are the health checks saved in the build settings, which the
	// next build records. Nil when none are saved.
	HealthChecks   *HealthChecks   `json:"healthChecks,omitempty"`
	Configurations *Configurations `json:"configurations,omitempty"`
	KindName       string          `json:"kindName,omitempty"`
	// KindVersion is empty for agents created before the version was recorded.
	KindVersion string            `json:"kindVersion,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	// CreatedBy is best-effort: resolved from the audit-only requester id
	// captured at agent-creation time (see AgentThunderClient.RequestedBy).
	// Nil when unknown — pre-existing agents, deployments without AgentID
	// provisioning wired up, or when the id couldn't be resolved to a user.
	CreatedBy *AgentCreatedBy `json:"createdBy,omitempty"`
	// Registry is set only for the registry view (GET agent with view=registry).
	Registry *AgentRegistryRecord `json:"registry,omitempty"`
}

// AgentSummary is the lightweight, org-wide agent listing shape: just enough
// to render a name and resolve which project an agent belongs to.
type AgentSummary struct {
	Name               string `json:"name"`
	DisplayName        string `json:"displayName"`
	ProjectName        string `json:"projectName"`
	ProjectDisplayName string `json:"projectDisplayName"`
	// Registry is set only for the registry view (GET /orgs/{org}/agents?view=registry).
	Registry *AgentRegistryRecord `json:"registry,omitempty"`
}

// Agent hosting in the registry: built and deployed by the platform, or registered from outside.
const (
	AgentRegistryHostingPlatform = "platform"
	AgentRegistryHostingExternal = "external"
)

// AgentRegistryRecord is an agent's org-level registry record, read from existing data:
// its component, its AgentID record and its configurations.
type AgentRegistryRecord struct {
	Description string
	Hosting     string
	// Interface is the agent sub-type (chat-api, custom-api, a2a-agent); empty when not recorded.
	Interface   string
	KindName    string
	KindVersion string
	// CreatedBy is nil when the creator could not be resolved.
	CreatedBy    *AgentCreatedBy
	Labels       map[string]string
	CreatedAt    time.Time
	Environments []AgentRegistryEnvironment
}

// AgentRegistryEnvironment is the agent in one environment of its project pipeline.
type AgentRegistryEnvironment struct {
	Name             string
	DisplayName      string
	IsProduction     bool
	DeploymentStatus string
	// LastDeployedAt is nil when the agent was never deployed here.
	LastDeployedAt *time.Time
	LLMProviders   []AgentRegistryResourceRef
	MCPServers     []AgentRegistryResourceRef
	// Governance is set only on the single-agent view.
	Governance *AgentRegistryGovernance
}

// AgentRegistryGovernance is the agent's governance in one environment, read from existing config.
type AgentRegistryGovernance struct {
	// AgentIDStatus is empty when no AgentID was requested here.
	AgentIDStatus     string
	LLMConfigurations []AgentRegistryLLMConfiguration
	Monitors          []AgentRegistryMonitor
	InboundSecurity   AgentRegistryInboundSecurity
	// A2ASkills is nil when no A2A card is stored here.
	A2ASkills []AgentRegistryA2ASkill
}

// AgentRegistryMonitor is an evaluation monitor on the agent in one environment.
type AgentRegistryMonitor struct {
	Name        string
	DisplayName string
	Status      string
}

// AgentRegistryInboundSecurity is the security on the agent's endpoint in one environment.
type AgentRegistryInboundSecurity struct {
	APIKey bool
	OAuth  bool
}

// AgentRegistryA2ASkill is one skill on the agent's A2A card.
type AgentRegistryA2ASkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// AgentRegistryLLMConfiguration is one of the agent's LLM configurations in one environment:
// its name, its provider, and the guardrails on the configuration and on the provider.
type AgentRegistryLLMConfiguration struct {
	Name       string
	Provider   AgentRegistryResourceRef
	Guardrails []string
}

// AgentEnvironmentResources is what an agent is configured with in one environment: its LLM
// configurations (each with its provider and guardrails), and its MCP servers and their tools.
type AgentEnvironmentResources struct {
	LLMConfigurations []AgentRegistryLLMConfiguration
	MCPServers        []AgentRegistryResourceRef
}

// LLMProviders returns the distinct providers of the LLM configurations, in order.
func (r *AgentEnvironmentResources) LLMProviders() []AgentRegistryResourceRef {
	providers := []AgentRegistryResourceRef{}
	seen := make(map[string]bool)
	for _, c := range r.LLMConfigurations {
		if !seen[c.Provider.ID] {
			seen[c.Provider.ID] = true
			providers = append(providers, c.Provider)
		}
	}
	return providers
}

// AgentDeploymentState is one agent's deployment in one environment, read from its
// release binding: the status and when it was last deployed.
type AgentDeploymentState struct {
	ProjectName     string
	AgentName       string
	EnvironmentName string
	Status          string
	// LastDeployedAt is nil when the binding records no deploy and no creation time.
	LastDeployedAt *time.Time
}

// AgentRegistryResourceRef is an LLM provider or MCP server the agent can reach.
type AgentRegistryResourceRef struct {
	ID          string
	DisplayName string
	// Tools is set for MCP servers on the single-agent view only: every tool discovered on the server.
	Tools []string
}

// AgentCreatedBy identifies the user who created an agent, resolved
// best-effort from the platform Thunder identity store.
type AgentCreatedBy struct {
	// ID is the raw user id (JWT subject) captured at creation time.
	ID string `json:"id"`
	// Display is the resolved username/display name, empty if the user
	// couldn't be resolved (e.g. since deleted).
	Display string `json:"display,omitempty"`
}

// Configurations contains runtime configurations for an agent
type Configurations struct {
	EnableAutoInstrumentation *bool        `json:"enableAutoInstrumentation,omitempty"`
	InstrumentationVersion    *string      `json:"instrumentationVersion,omitempty"`
	Env                       []EnvVars    `json:"env,omitempty"`
	EnableApiKeySecurity      *bool        `json:"enableApiKeySecurity,omitempty"`
	CorsConfig                *CorsConfig  `json:"corsConfig,omitempty"`
	EnableOAuthSecurity       *bool        `json:"enableOAuthSecurity,omitempty"`
	OAuthConfig               *OAuthConfig `json:"oauthConfig,omitempty"`
	ResilienceTimeoutSeconds  *int32       `json:"resilienceTimeoutSeconds,omitempty"`
}

type CorsConfig struct {
	Enabled          *bool    `json:"enabled,omitempty"`
	AllowOrigin      []string `json:"allowOrigin,omitempty"`
	AllowMethods     []string `json:"allowMethods,omitempty"`
	AllowHeaders     []string `json:"allowHeaders,omitempty"`
	AllowCredentials *bool    `json:"allowCredentials,omitempty"`
}

// Default OAuth header values used when a config omits them. They mirror the
// gateway jwt-auth policy defaults so callers send a standard
// `Authorization: Bearer <token>` header.
const (
	DefaultOAuthHeaderName       = "Authorization"
	DefaultOAuthAuthHeaderPrefix = "Bearer"
	// DefaultOAuthForwardToken mirrors the gateway jwt-auth policy default: the
	// validated token header is forwarded to the upstream service unless disabled.
	DefaultOAuthForwardToken = true
)

// ReservedIdentityProviderName is the gateway identity provider used internally
// for platform trace ingestion auth. It lives in the gateway runtime config but
// is hidden from users: never mirrored into AMS, never shown in the UI, and never
// offered as an agent OAuth issuer.
const ReservedIdentityProviderName = "agent-manager-service"

// SystemIdentityProviderNames are the well-known identity providers seeded with
// the platform. They are mirrored with type=system and cannot be deleted.
var SystemIdentityProviderNames = map[string]bool{
	"ThunderKeyManager": true,
}

// EnvThunderIdentityProviderPrefix matches the env-Thunder release names
// (amp-thunder-<org>-<env>) that the gateway bootstrap job seeds as identity
// providers. Must stay in sync with thundersvc.ThunderReleaseName and
// deployments/scripts/thunder-naming.sh.
const EnvThunderIdentityProviderPrefix = "amp-thunder-"

// IsSystemIdentityProvider reports whether the given identity provider name is a
// platform-seeded system provider.
func IsSystemIdentityProvider(name string) bool {
	return SystemIdentityProviderNames[name] || strings.HasPrefix(name, EnvThunderIdentityProviderPrefix)
}

type OAuthConfig struct {
	Issuers          []string `json:"issuers,omitempty"`
	Audiences        []string `json:"audiences,omitempty"`
	HeaderName       string   `json:"headerName,omitempty"`
	AuthHeaderPrefix string   `json:"authHeaderPrefix,omitempty"`
	ForwardToken     bool     `json:"forwardToken"`
}

type AgentType struct {
	// Type of the agent
	Type string `json:"type"`
	// Sub-type of the agent
	SubType string `json:"subType,omitempty"`
	// Language of the agent (e.g. "python", "docker")
	Language string `json:"language,omitempty"`
}

type Provisioning struct {
	Type       string     `json:"type"`
	Repository Repository `json:"repository,omitempty"`
}

type Repository struct {
	Url       string `json:"url"`
	AppPath   string `json:"appPath"`
	Branch    string `json:"branch"`
	SecretRef string `json:"secretRef,omitempty"`
}

type Build struct {
	Type      string           `json:"type"` // "buildpack" or "docker"
	Buildpack *BuildpackConfig `json:"buildpack,omitempty"`
	Docker    *DockerConfig    `json:"docker,omitempty"`
}

type BuildpackConfig struct {
	Language        string `json:"language"`
	LanguageVersion string `json:"languageVersion,omitempty"`
	RunCommand      string `json:"runCommand,omitempty"`
}

type DockerConfig struct {
	DockerfilePath string `json:"dockerfilePath"`
}

// DB Model
type Agent struct {
	ID               uuid.UUID      `gorm:"column:id;primaryKey"`
	ProvisioningType string         `gorm:"column:provisioning_type"`
	Name             string         `gorm:"column:name"`
	DisplayName      string         `gorm:"column:display_name"`
	Description      string         `gorm:"column:description"`
	ProjectName      string         `gorm:"column:project_name"`
	OrgName          string         `gorm:"column:org_name"`
	CreatedAt        time.Time      `gorm:"column:created_at"`
	UpdatedAt        time.Time      `gorm:"column:updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"column:deleted_at"`
	AgentDetails     *InternalAgent `gorm:"foreignKey:ID;references:ID"`
}

type InternalAgent struct {
	ID           uuid.UUID              `gorm:"column:id;primaryKey"`
	WorkloadSpec map[string]interface{} `gorm:"column:workload_spec;type:jsonb;serializer:json"`
}
