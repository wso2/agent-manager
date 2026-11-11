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

package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AgentConfig is an agent's gateway settings in one environment. Services name the agent and environment; the hooks
// translate them to the stored ids. Lite hosts no agents, so the instrumentation fields are not stored.
type AgentConfig struct {
	ID                        uuid.UUID  `gorm:"-"`
	OUID                      string     `gorm:"column:org_id"`
	AgentUUID                 *uuid.UUID `gorm:"column:agent_id;primaryKey"`
	EnvironmentUUID           uuid.UUID  `gorm:"column:environment_id;primaryKey"`
	ProjectName               string     `gorm:"-"`
	AgentName                 string     `gorm:"-"`
	EnvironmentName           string     `gorm:"-"`
	EnableAutoInstrumentation bool       `gorm:"-"`
	// InstrumentationVersion is the AMP instrumentation version selected for the
	// agent. Nil means "use the platform default".
	InstrumentationVersion *string   `gorm:"-"`
	CreatedAt              time.Time `gorm:"column:created_at"`
	UpdatedAt              time.Time `gorm:"column:updated_at"`
	EnableApiKeySecurity   bool      `gorm:"column:api_key_security"`
	CORSEnabled            bool      `gorm:"column:cors_enabled"`
	CORSAllowOrigins       []string  `gorm:"column:cors_allow_origins;serializer:json"`
	CORSAllowMethods       []string  `gorm:"column:cors_allow_methods;serializer:json"`
	CORSAllowHeaders       []string  `gorm:"column:cors_allow_headers;serializer:json"`
	CORSAllowCredentials   bool      `gorm:"column:cors_allow_credentials"`
	// OAuth security. EnableApiKeySecurity and EnableOAuthSecurity are mutually
	// exclusive — enforced at the service layer before persistence.
	EnableOAuthSecurity      bool     `gorm:"column:oauth_enabled"`
	OAuthIssuers             []string `gorm:"column:oauth_issuers;serializer:json"`
	OAuthAudiences           []string `gorm:"column:oauth_audiences;serializer:json"`
	OAuthHeaderName          string   `gorm:"column:oauth_header_name"`
	OAuthAuthHeaderPrefix    string   `gorm:"column:oauth_header_prefix"`
	OAuthForwardToken        bool     `gorm:"column:oauth_forward_token"`
	ResilienceTimeoutSeconds *int32   `gorm:"column:timeout_seconds"`
	// Public Agent Card CORS override (A2A agents only). A nil CardCORSEnabled
	// means the card inherits the agent's CORS, and the other three are nil too.
	CardCORSEnabled          *bool    `gorm:"column:card_cors_enabled"`
	CardCORSAllowOrigins     []string `gorm:"column:card_cors_allow_origins;serializer:json"`
	CardCORSAllowHeaders     []string `gorm:"column:card_cors_allow_headers;serializer:json"`
	CardCORSAllowCredentials *bool    `gorm:"column:card_cors_allow_credentials"`
}

func (AgentConfig) TableName() string { return "agent_gateway_settings" }

func (c *AgentConfig) ref() agentRef {
	return agentRef{&c.OUID, &c.ProjectName, &c.AgentName, &c.AgentUUID, c.EnvironmentUUID, &c.EnvironmentName}
}

// Resolve sets the stored agent and environment ids from the names.
func (c *AgentConfig) Resolve(tx *gorm.DB) error {
	if err := resolveEnvironment(tx, c.OUID, c.EnvironmentName, &c.EnvironmentUUID); err != nil {
		return err
	}
	return c.ref().resolve(tx)
}

// AfterFind fills the agent's and environment's names.
func (c *AgentConfig) AfterFind(tx *gorm.DB) error { return c.ref().fill(tx) }
