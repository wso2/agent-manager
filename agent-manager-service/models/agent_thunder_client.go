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

// AgentThunderClient is the GORM model for the agent_thunder_clients table — the
// binding record for one agent's AgentID in one environment.
type AgentThunderClient struct {
	ID               uuid.UUID             `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID             string                `gorm:"column:ou_id;not null"`
	ProjectName      string                `gorm:"column:project_name;not null"`
	AgentName        string                `gorm:"column:agent_name;not null"`
	EnvironmentName  string                `gorm:"column:environment_name;not null"`
	ProvisioningType AgentProvisioningType `gorm:"column:provisioning_type;not null"`
	ThunderAgentID   string                `gorm:"column:thunder_agent_id;not null;default:''"`
	ThunderClientID  string                `gorm:"column:thunder_client_id;not null;default:''"`
	// SecretRefPath stores the provider-managed SecretReference name for the
	// credential (internal agents only). The column name is retained for schema
	// compatibility.
	SecretRefPath string             `gorm:"column:secret_ref_path;not null;default:''"`
	Status        AgentThunderStatus `gorm:"column:status;not null;default:'pending'"`
	// RequestedBy is the calling user's own subject (from AMS's existing
	// incoming-request auth), captured for audit purposes only. Thunder's own
	// "owner" field cannot carry this — each env-Thunder is a fully isolated
	// instance with its own entity store, so a platform-Thunder user ID does
	// not resolve there (verified live: env-Thunder rejects it with AGT-1039
	// "owner not found"). This is AMS's own record of who asked, independent
	// of what Thunder's API will accept.
	RequestedBy     string     `gorm:"column:requested_by;not null;default:''"`
	AttemptCount    int        `gorm:"column:attempt_count;not null;default:0"`
	LastError       string     `gorm:"column:last_error;not null;default:''"`
	LastAttemptedAt *time.Time `gorm:"column:last_attempted_at"`
	NextRetryAt     *time.Time `gorm:"column:next_retry_at"`
	CreatedAt       time.Time  `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;not null;default:NOW()"`
}

func (AgentThunderClient) TableName() string { return "agent_thunder_clients" }

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
