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
	"time"
)

type AgentResponse struct {
	UUID           string            `json:"uuid"`
	Name           string            `json:"name"`
	DisplayName    string            `json:"displayName,omitempty"`
	Description    string            `json:"description,omitempty"`
	ProjectName    string            `json:"projectName"`
	CreatedAt      time.Time         `json:"createdAt"`
	Status         string            `json:"status,omitempty"`
	Provisioning   Provisioning      `json:"provisioning,omitempty"`
	Type           AgentType         `json:"type,omitempty"`
	Configurations *Configurations   `json:"configurations,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	// CreatedBy is best-effort: resolved from the audit-only requester id
	// captured at agent-creation time (see AgentThunderClient.RequestedBy).
	// Nil when unknown — pre-existing agents, deployments without AgentID
	// provisioning wired up, or when the id couldn't be resolved to a user.
	CreatedBy *AgentCreatedBy `json:"createdBy,omitempty"`
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

// EnvVars represents environment variables
type EnvVars struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	IsSensitive bool   `json:"isSensitive,omitempty"`
	SecretRef   string `json:"secretRef,omitempty"`
	SecretKey   string `json:"secretKey,omitempty"` // The key within the secret (e.g., "api-key")
	IsSystem    bool   `json:"isSystem,omitempty"`  // Platform-injected (e.g. LLM_PROVIDER_URL); read-only for users
}
