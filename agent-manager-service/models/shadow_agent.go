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

// Shadow Agent Discovery platforms (the API calls them providers). The agent
// platforms are scanned through their agent listing APIs.
const (
	ShadowProviderOpenAI    = "openai"
	ShadowProviderAnthropic = "anthropic"
	ShadowProviderBedrock   = "bedrock"
	ShadowProviderGoogle    = "google"
	ShadowProviderFoundry   = "foundry"
	// Source repository hosts (Method 2). Connections are stored now; the
	// source code analysis that scans them is added separately.
	ShadowProviderGitHub      = "github"
	ShadowProviderAzureDevOps = "azure_devops"
)

// ShadowProviders lists every supported platform, in display order.
var ShadowProviders = []string{
	ShadowProviderOpenAI, ShadowProviderAnthropic,
	ShadowProviderBedrock, ShadowProviderGoogle, ShadowProviderFoundry,
	ShadowProviderGitHub, ShadowProviderAzureDevOps,
}

// Finding kinds: what an agent is at its platform.
const (
	// ShadowKindPlatformAgent is an agent object the platform lists (OpenAI
	// agent, Anthropic Managed Agent, Bedrock classic agent or Agent Registry
	// agent record, Foundry agent, Google Agent Registry agent).
	ShadowKindPlatformAgent = "platform_agent"
	// ShadowKindRuntime is an AgentCore runtime or a Google Agent Runtime
	// engine (reasoning engine).
	ShadowKindRuntime = "runtime"
)

// Finding review statuses.
const (
	ShadowStatusNew       = "new"
	ShadowStatusReviewed  = "reviewed"
	ShadowStatusDismissed = "dismissed"
)

// Scan statuses.
const (
	ShadowScanQueued    = "queued"
	ShadowScanRunning   = "running"
	ShadowScanSucceeded = "succeeded"
	ShadowScanPartial   = "partial"
	ShadowScanFailed    = "failed"
)

// Outcomes of one provider call (or one connection) within a scan.
const (
	ShadowCallOK               = "ok"
	ShadowCallSkipped          = "skipped"
	ShadowCallAuthFailed       = "auth_failed"
	ShadowCallPermissionDenied = "permission_denied"
	ShadowCallNotFound         = "not_found"
	ShadowCallRateLimited      = "rate_limited"
	ShadowCallUnsupported      = "unsupported_response"
	ShadowCallFailed           = "failed"
)

// ShadowConnection is one platform account an org scans: the credential and
// where to reach the platform. The secret is AES-GCM encrypted and never
// returned by the API.
type ShadowConnection struct {
	ID       uuid.UUID              `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID     string                 `gorm:"column:ou_id;not null"`
	Name     string                 `gorm:"column:name;not null"`
	Provider string                 `gorm:"column:provider;not null"`
	Config   ShadowConnectionConfig `gorm:"column:config;type:jsonb;serializer:json;not null"`
	// SecretEncrypted is the AES-GCM encrypted JSON of ShadowConnectionSecret.
	SecretEncrypted []byte `gorm:"column:secret_encrypted;not null"`
	// SecretHints are redacted hints of the stored secrets ("sk-a...wxyz"),
	// so the UI can say which credential is stored without revealing it.
	SecretHints    map[string]string         `gorm:"column:secret_hints;type:jsonb;serializer:json;not null"`
	LastTestedAt   *time.Time                `gorm:"column:last_tested_at"`
	LastTestResult *ShadowConnectionTestInfo `gorm:"column:last_test_result;type:jsonb;serializer:json"`
	CreatedBy      string                    `gorm:"column:created_by;not null"`
	CreatedAt      time.Time                 `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt      time.Time                 `gorm:"column:updated_at;not null;default:NOW()"`
}

func (ShadowConnection) TableName() string { return "shadow_connections" }

// ShadowConnectionConfig is a connection's non-secret settings. Fields apply
// to the providers named in their comment; others leave them empty.
type ShadowConnectionConfig struct {
	// Endpoints holds the base URL of every provider endpoint the connector
	// calls, keyed by endpoint name. Values may contain placeholders such as
	// {region}, {location} or {account}; the user sees the official values
	// prefilled and may change them.
	Endpoints map[string]string `json:"endpoints"`

	// Foundry and Azure DevOps (with UseEntra): the Entra app. Foundry: the
	// subscriptions to scan.
	TenantID         string   `json:"tenantId,omitempty"`
	ClientID         string   `json:"clientId,omitempty"`
	AllSubscriptions bool     `json:"allSubscriptions,omitempty"`
	SubscriptionIDs  []string `json:"subscriptionIds,omitempty"`
	// Foundry: also list classic (assistants-style) agents.
	IncludeClassicAgents bool `json:"includeClassicAgents,omitempty"`

	// Bedrock: regions, the role to assume and the access key that assumes it.
	Regions     []string `json:"regions,omitempty"`
	RoleARN     string   `json:"roleArn,omitempty"`
	ExternalID  string   `json:"externalId,omitempty"`
	AccessKeyID string   `json:"accessKeyId,omitempty"`

	// Google: projects and locations to scan.
	ProjectIDs []string `json:"projectIds,omitempty"`
	Locations  []string `json:"locations,omitempty"`
	// IncludeGlobalAndMultiRegion adds the "global" Agent Registry location
	// and the us/eu multi-region Agent Runtime locations.
	IncludeGlobalAndMultiRegion bool `json:"includeGlobalAndMultiRegion,omitempty"`

	// OpenAI project IDs / Anthropic workspace IDs that have a scoped key in
	// the secret (the keys themselves are in the secret).
	ScopedKeyIDs []string `json:"scopedKeyIds,omitempty"`

	// GitHub, Azure DevOps: the organization (or GitHub user account) whose
	// repositories are scanned.
	Organization string `json:"organization,omitempty"`
	// GitHub: every repository the token can read, or only Repositories.
	AllRepositories bool     `json:"allRepositories,omitempty"`
	Repositories    []string `json:"repositories,omitempty"`
	// Azure DevOps: every project the credential can read, or only Projects.
	AllProjects bool     `json:"allProjects,omitempty"`
	Projects    []string `json:"projects,omitempty"`
	// Azure DevOps: sign in with a Microsoft Entra service principal
	// (TenantID, ClientID and the client secret) instead of a PAT.
	UseEntra bool `json:"useEntra,omitempty"`
	// GitHub, Azure DevOps: also read CI pipeline definitions and .env
	// example files.
	ReadCIAndEnvFiles bool `json:"readCiAndEnvFiles,omitempty"`

	// DisabledServices are the optional services (Bedrock, Google) the user
	// unticked: the scan does not call them. Listing the disabled ones keeps
	// every service on for connections saved before the option existed.
	DisabledServices []string `json:"disabledServices,omitempty"`
}

// ShadowConnectionSecret is the secret part of a connection. It exists in
// plain text only in memory while a connector runs; it is stored encrypted
// and never returned, logged or audited.
type ShadowConnectionSecret struct {
	// ScopedKeys maps an OpenAI project ID or an Anthropic workspace ID to a
	// read-only key for it: each key lists its project's or workspace's agents.
	// (An Admin API key stored by an earlier version is not decoded, so it is
	// dropped when the platform is next saved.)
	ScopedKeys map[string]string `json:"scopedKeys,omitempty"`
	// ClientSecret is the Entra app's client secret (Foundry, Azure DevOps).
	ClientSecret string `json:"clientSecret,omitempty"`
	// SecretAccessKey and SessionToken complete the AWS access key.
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
	SessionToken    string `json:"sessionToken,omitempty"`
	// ServiceAccountJSON is a Google service account key file.
	ServiceAccountJSON string `json:"serviceAccountJson,omitempty"`
	// PersonalAccessToken is a GitHub fine-grained or Azure DevOps personal
	// access token.
	PersonalAccessToken string `json:"personalAccessToken,omitempty"`
}

// ShadowConnectionTestInfo is the outcome of a connection test: one entry
// per probed capability.
type ShadowConnectionTestInfo struct {
	OK     bool                    `json:"ok"`
	Checks []ShadowConnectionCheck `json:"checks"`
}

// ShadowConnectionCheck is one probed call of a connection test.
type ShadowConnectionCheck struct {
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
	Message string `json:"message,omitempty"`
	// Unlocks names the evidence this call provides, so a failed check says
	// which agents a scan will miss.
	Unlocks []string `json:"unlocks,omitempty"`
}

// ShadowScan is one discovery run over some of an org's connections.
type ShadowScan struct {
	ID            uuid.UUID                    `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID          string                       `gorm:"column:ou_id;not null"`
	Status        string                       `gorm:"column:status;not null"`
	ConnectionIDs []string                     `gorm:"column:connection_ids;type:jsonb;serializer:json;not null"`
	Results       []ShadowScanConnectionResult `gorm:"column:results;type:jsonb;serializer:json;not null"`
	FindingsCount int                          `gorm:"column:findings_count;not null"`
	RequestedBy   string                       `gorm:"column:requested_by;not null"`
	ClaimedUntil  *time.Time                   `gorm:"column:claimed_until"`
	Attempts      int                          `gorm:"column:attempts;not null"`
	StartedAt     *time.Time                   `gorm:"column:started_at"`
	FinishedAt    *time.Time                   `gorm:"column:finished_at"`
	CreatedAt     time.Time                    `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt     time.Time                    `gorm:"column:updated_at;not null;default:NOW()"`
}

func (ShadowScan) TableName() string { return "shadow_scans" }

// ShadowScanConnectionResult is how one connection fared in a scan.
type ShadowScanConnectionResult struct {
	ConnectionID   string                 `json:"connectionId"`
	ConnectionName string                 `json:"connectionName"`
	Provider       string                 `json:"provider"`
	Outcome        string                 `json:"outcome"`
	Message        string                 `json:"message,omitempty"`
	FindingsCount  int                    `json:"findingsCount"`
	Calls          []ShadowScanCallResult `json:"calls"`
}

// ShadowScanCallResult is the outcome of one provider call (all its pages
// and scopes) in a scan. Messages are sanitised: no bodies, no secrets.
type ShadowScanCallResult struct {
	Call    string `json:"call"`
	Outcome string `json:"outcome"`
	Message string `json:"message,omitempty"`
	Items   int    `json:"items"`
}

// ShadowOwnerHint is the minimised personal data kept to say who created an
// agent, only when the platform's agent listing says so: no more than a
// name, an email and a username or principal.
type ShadowOwnerHint struct {
	Name      string     `json:"name,omitempty"`
	Email     string     `json:"email,omitempty"`
	Username  string     `json:"username,omitempty"`
	Type      string     `json:"type,omitempty"`
	Source    string     `json:"source,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
}

// ShadowFindingRelation links a finding to another one that may be the same
// agent.
type ShadowFindingRelation struct {
	FindingID string `json:"findingId"`
	// Relation is "possible_duplicate".
	Relation string `json:"relation"`
	Reason   string `json:"reason,omitempty"`
	// Source is "correlation" for relations found across connections.
	Source string `json:"source,omitempty"`
}

// ShadowFinding is an agent a platform lists, as one connection found it:
// what the platform reports about it and its review status. The same agent
// found through two connections is two rows with the same provider, kind
// and external ID; the API shows them as one agent.
type ShadowFinding struct {
	ID                uuid.UUID               `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID              string                  `gorm:"column:ou_id;not null"`
	ConnectionID      uuid.UUID               `gorm:"column:connection_id;type:uuid;not null"`
	Provider          string                  `gorm:"column:provider;not null"`
	Kind              string                  `gorm:"column:kind;not null"`
	ExternalID        string                  `gorm:"column:external_id;not null"`
	DisplayName       string                  `gorm:"column:display_name;not null"`
	Scope             string                  `gorm:"column:scope;not null"`
	State             string                  `gorm:"column:state;not null"`
	Status            string                  `gorm:"column:status;not null"`
	OwnerHint         ShadowOwnerHint         `gorm:"column:owner_hint;type:jsonb;serializer:json;not null"`
	Attributes        map[string]any          `gorm:"column:attributes;type:jsonb;serializer:json;not null"`
	Related           []ShadowFindingRelation `gorm:"column:related;type:jsonb;serializer:json;not null"`
	ProviderCreatedAt *time.Time              `gorm:"column:provider_created_at"`
	FirstSeenAt       time.Time               `gorm:"column:first_seen_at;not null"`
	LastSeenAt        time.Time               `gorm:"column:last_seen_at;not null"`
	LastScanID        *uuid.UUID              `gorm:"column:last_scan_id;type:uuid"`
	// Present is false once a complete listing of its connection no longer
	// returns the agent: it was deleted at the platform, or is out of the
	// connection's scope now.
	Present   bool      `gorm:"column:present;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:NOW()"`
}

func (ShadowFinding) TableName() string { return "shadow_findings" }

// ShadowFindingSignal is one piece of evidence on a finding: the listing
// that returned the agent, the exact condition, and what was observed
// (flags and short labels only).
type ShadowFindingSignal struct {
	ID         uuid.UUID      `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID       string         `gorm:"column:ou_id;not null"`
	FindingID  uuid.UUID      `gorm:"column:finding_id;type:uuid;not null"`
	Code       string         `gorm:"column:code;not null"`
	Source     string         `gorm:"column:source;not null"`
	Condition  string         `gorm:"column:condition;not null"`
	Observed   map[string]any `gorm:"column:observed;type:jsonb;serializer:json;not null"`
	ScanID     *uuid.UUID     `gorm:"column:scan_id;type:uuid"`
	DetectedAt time.Time      `gorm:"column:detected_at;not null"`
}

func (ShadowFindingSignal) TableName() string { return "shadow_finding_signals" }
