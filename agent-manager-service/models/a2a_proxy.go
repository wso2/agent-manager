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

// A2AProxy marks an external agent's environment as published through the
// environment's ingress gateway; the row existing is what makes it proxied.
type A2AProxy struct {
	ID              uuid.UUID  `gorm:"column:id;primaryKey"`
	OUID            string     `gorm:"column:org_id"`
	ProjectName     string     `gorm:"-"`
	AgentName       string     `gorm:"-"`
	AgentUUID       *uuid.UUID `gorm:"column:agent_id"`
	EnvironmentUUID uuid.UUID  `gorm:"column:environment_id"`
	EnvironmentName string     `gorm:"-"`
	// ArtifactUUID is the proxied environment's agent artifact, which the gateway serves the agent under.
	ArtifactUUID uuid.UUID `gorm:"column:artifact_id"`
	// UpstreamAuthHeader is empty when the gateway adds no credential upstream.
	UpstreamAuthHeader string `gorm:"column:upstream_auth_header"`
	// UpstreamAuthSecretID is the secrets row holding the header value.
	UpstreamAuthSecretID *uuid.UUID `gorm:"column:upstream_auth_secret_id"`
	// UpstreamAuthSecret is the header value as base64 AES-256-GCM ciphertext, read from and written to the secrets
	// row by the repository; it is never returned.
	UpstreamAuthSecret string    `gorm:"-"`
	CreatedAt          time.Time `gorm:"column:created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
}

func (A2AProxy) TableName() string { return "a2a_proxies" }

func (p *A2AProxy) ref() agentRef {
	return agentRef{&p.OUID, &p.ProjectName, &p.AgentName, &p.AgentUUID, p.EnvironmentUUID, &p.EnvironmentName}
}

// BeforeCreate resolves the agent's id from its names.
func (p *A2AProxy) BeforeCreate(tx *gorm.DB) error { return p.ref().resolve(tx) }

// AfterFind fills the agent's and environment's names.
func (p *A2AProxy) AfterFind(tx *gorm.DB) error { return p.ref().fill(tx) }

// UpstreamAuth is the gateway's upstream credential as a secretRef, or nil when there is none.
func (p *A2AProxy) UpstreamAuth() *UpstreamAuth {
	if p.UpstreamAuthHeader == "" || p.UpstreamAuthSecret == "" {
		return nil
	}
	authType, header, secret := AuthTypeAPIKey, p.UpstreamAuthHeader, p.UpstreamAuthSecret
	return &UpstreamAuth{Type: &authType, Header: &header, SecretRef: &secret}
}

// A2AProxyAuthUpdate is a requested upstream credential; a nil Value keeps the stored secret.
type A2AProxyAuthUpdate struct {
	Header string
	Value  *string
}

// A2AProxyStatus is a proxied environment's deployment to its ingress gateway as the A2A proxy API reports it; the
// secret is never part of it.
type A2AProxyStatus struct {
	// Status is the shared deployment status the user is shown (UserStatusOf): never PENDING or ARCHIVED.
	Status DeploymentStatus
	// StatusReason says why, when there is something to say.
	StatusReason       string
	UpstreamAuthHeader string
	// GatewayURL is where the environment's ingress gateway serves the agent; empty until one is mapped.
	GatewayURL string
}
