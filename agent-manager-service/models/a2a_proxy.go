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
)

// A2AProxy marks an external A2A agent's environment as published through the
// environment's ingress gateway; the row existing is what makes it proxied.
type A2AProxy struct {
	ID              uuid.UUID `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID            string    `gorm:"column:ou_id;not null"`
	ProjectName     string    `gorm:"column:project_name;not null"`
	AgentName       string    `gorm:"column:agent_name;not null"`
	EnvironmentUUID uuid.UUID `gorm:"column:environment_uuid;type:uuid;not null"`
	EnvironmentName string    `gorm:"column:environment_name;not null"`
	// UpstreamAuthHeader is empty when the gateway adds no credential upstream.
	UpstreamAuthHeader string `gorm:"column:upstream_auth_header;not null;default:''"`
	// UpstreamAuthSecret is the header value as base64 AES-256-GCM ciphertext; it is never returned.
	UpstreamAuthSecret string    `gorm:"column:upstream_auth_secret;not null;default:''"`
	CreatedAt          time.Time `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt          time.Time `gorm:"column:updated_at;not null;default:NOW()"`
}

func (A2AProxy) TableName() string { return "a2a_proxies" }

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

// A2AProxyStatus is a proxied environment's gateway publication state; the secret is never part of it.
type A2AProxyStatus struct {
	Status             A2APublicationStatus
	LastError          string
	UpstreamAuthHeader string
	// GatewayURL is where the environment's ingress gateway serves the agent; empty until one is mapped.
	GatewayURL string
}
