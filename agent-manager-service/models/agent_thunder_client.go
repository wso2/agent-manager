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

// AgentProvisioningType identifies who ends up holding an AgentID's credential.
type AgentProvisioningType string

const (
	// AgentProvisioningTypeInternal is an agent whose workload the platform runs and
	// manages. Its secret is stored in the vault and never shown to a human.
	AgentProvisioningTypeInternal AgentProvisioningType = "internal"
	// AgentProvisioningTypeExternal is an agent the operator runs and hosts themselves.
	// Its secret is shown once and never persisted long-term by the platform.
	AgentProvisioningTypeExternal AgentProvisioningType = "external"
)

// AgentThunderStatus is the provisioning status of one binding (Section 6 of the
// AgentID architecture doc): PENDING -> IN_PROGRESS -> COMPLETED, or FAILED after
// the retry budget (5 attempts) is exhausted.
type AgentThunderStatus string

const (
	AgentThunderStatusPending    AgentThunderStatus = "pending"
	AgentThunderStatusInProgress AgentThunderStatus = "in_progress"
	AgentThunderStatusCompleted  AgentThunderStatus = "completed"
	AgentThunderStatusFailed     AgentThunderStatus = "failed"
)

// AgentThunderClient is one agent's AgentID binding in one environment. Services name the agent and environment;
// the hooks translate them to the stored ids. Lite agents are all external and environments have no promotion
// pipeline, so the provisioning type, secret reference and promotion fields are not stored.
type AgentThunderClient struct {
	ID               uuid.UUID             `gorm:"column:id;primaryKey"`
	OUID             string                `gorm:"column:org_id"`
	AgentUUID        *uuid.UUID            `gorm:"column:agent_id"`
	EnvironmentUUID  uuid.UUID             `gorm:"column:environment_id"`
	ProjectName      string                `gorm:"-"`
	AgentName        string                `gorm:"-"`
	EnvironmentName  string                `gorm:"-"`
	ProvisioningType AgentProvisioningType `gorm:"-"`
	ThunderAgentID   string                `gorm:"column:thunder_agent_id"`
	ThunderClientID  string                `gorm:"column:thunder_client_id"`
	SecretRefPath    string                `gorm:"-"`
	Status           AgentThunderStatus    `gorm:"column:status"`
	// RequestedBy is the calling user's own subject, kept for audit: an env-Thunder is an isolated instance, so a
	// platform-Thunder user id does not resolve there as Thunder's own owner field.
	RequestedBy  string `gorm:"column:requested_by"`
	AttemptCount int    `gorm:"column:attempt_count"`
	LastError    string `gorm:"column:last_error"`
	// LastAttemptedAt is when the running attempt was claimed; ClaimedUntil is when that claim goes stale.
	LastAttemptedAt *time.Time `gorm:"-"`
	ClaimedUntil    *time.Time `gorm:"column:claimed_until"`
	NextRetryAt     *time.Time `gorm:"column:next_retry_at"`
	PromotionSource string     `gorm:"-"`
	SyncAfter       *time.Time `gorm:"-"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (AgentThunderClient) TableName() string { return "agent_identities" }

func (c *AgentThunderClient) ref() agentRef {
	return agentRef{&c.OUID, &c.ProjectName, &c.AgentName, &c.AgentUUID, c.EnvironmentUUID, &c.EnvironmentName}
}

// BeforeCreate resolves the agent's and environment's ids from their names.
func (c *AgentThunderClient) BeforeCreate(tx *gorm.DB) error {
	if err := resolveEnvironment(tx, c.OUID, c.EnvironmentName, &c.EnvironmentUUID); err != nil {
		return err
	}
	return c.ref().resolve(tx)
}

// AfterFind fills the names and the Lite defaults of the fields that are not stored.
func (c *AgentThunderClient) AfterFind(tx *gorm.DB) error {
	c.ProvisioningType = AgentProvisioningTypeExternal
	return c.ref().fill(tx)
}

// AgentIdentityEnvironmentView is one environment's AgentID binding, shaped for
// display to a caller. This is a safe, side-effect-free read: it never carries
// a secret — external agents never have one stored to begin with, and
// RegenerateSecret is the only way to obtain one.
type AgentIdentityEnvironmentView struct {
	EnvironmentName  string                `json:"environmentName"`
	ProvisioningType AgentProvisioningType `json:"provisioningType"`
	Status           AgentThunderStatus    `json:"status"`
	// AgentID is Thunder's own UUID for this identity (the /agents resource ID) —
	// distinct from ClientID, which is the OAuth2 client_id. Empty until
	// provisioning actually reaches Thunder (status pending with no attempt yet).
	AgentID   string `json:"agentId,omitempty"`
	ClientID  string `json:"clientId,omitempty"`
	LastError string `json:"lastError,omitempty"`
	// RequestedBy is AMS's own record of who triggered this binding — see the
	// field doc on AgentThunderClient.RequestedBy for why this is tracked here
	// rather than via Thunder's own owner field.
	RequestedBy string `json:"requestedBy,omitempty"`
}

// AgentIdentityActionRequest is the request body for POST .../identities
// (regenerate). Unlike the GET/PUT/DELETE identity endpoints, which take
// ?environment= since they carry no body, POST parameters live in the body.
type AgentIdentityActionRequest struct {
	Environment string `json:"environment"`
}

// AgentRegenerateSecretStatus is the fixed value of AgentRegenerateSecretResponse.Status —
// regenerate has exactly one successful outcome, so this isn't an enum in
// practice, just a named constant so the literal string exists in one place.
const AgentRegenerateSecretStatus = "regenerated"

// AgentRegenerateSecretResponse is returned by the regenerate-secret endpoint.
// ClientSecret is always populated, for both Internal and External agents —
// the caller just explicitly asked to rotate this credential and already
// holds agent:update, so there is no reason to make them fetch it separately.
// ProvisioningType is included so a caller can tell which kind of agent this
// binding belongs to without cross-referencing GetAgentIdentity.
type AgentRegenerateSecretResponse struct {
	EnvironmentName  string                `json:"environmentName"`
	ProvisioningType AgentProvisioningType `json:"provisioningType"`
	ClientID         string                `json:"clientId"`
	ClientSecret     string                `json:"clientSecret"`
	Status           string                `json:"status"`
	// WorkloadRefreshWarning is set only when the rotation itself succeeded
	// (the response above is otherwise unaffected) but the best-effort push of
	// the new secret into the already-running pod failed. Without this, the
	// pod keeps serving the now-invalidated old secret via its env var until
	// something else redeploys it — silently, since the rotation still
	// reports success. Empty on the ordinary success path.
	WorkloadRefreshWarning string `json:"workloadRefreshWarning,omitempty"`
}

// AgentRevokeSecretStatus is the fixed value of AgentRevokeSecretResponse.Status —
// revoke has exactly one successful outcome, so this isn't an enum in practice,
// just a named constant so the literal string exists in one place.
const AgentRevokeSecretStatus = "revoked"

// AgentRevokeSecretResponse is returned by the revoke-secret endpoint. There is
// never a clientSecret here — revoke is a kill switch, not a rotation; an
// explicit regenerate afterward is required to get a usable credential again.
type AgentRevokeSecretResponse struct {
	EnvironmentName string `json:"environmentName"`
	ClientID        string `json:"clientId"`
	Status          string `json:"status"`
	// WorkloadRefreshWarning is set when the revoke succeeded but the
	// best-effort cleanup of the running pod's credential couldn't be
	// completed or verified — e.g. the deployment pipeline couldn't be
	// resolved. Empty on the ordinary success path.
	WorkloadRefreshWarning string `json:"workloadRefreshWarning,omitempty"`
}
