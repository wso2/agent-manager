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
	UUID           string            `json:"uuid"`
	Name           string            `json:"name"`
	DisplayName    string            `json:"displayName,omitempty"`
	Description    string            `json:"description,omitempty"`
	ProjectName    string            `json:"projectName"`
	CreatedAt      time.Time         `json:"createdAt"`
	Status         string            `json:"status,omitempty"`
	Provisioning   Provisioning      `json:"provisioning,omitempty"`
	Type           AgentType         `json:"type,omitempty"`
	Build          *Build            `json:"build,omitempty"`
	InputInterface *InputInterface   `json:"inputInterface,omitempty"`
	Configurations *Configurations   `json:"configurations,omitempty"`
	KindName       string            `json:"kindName,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
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
