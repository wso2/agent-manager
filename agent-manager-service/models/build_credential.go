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
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// PublishBuildWorkloadRequest is what a build's workload step sends to the build
// publisher endpoint: the two documents amp-generate-workload builds.
type PublishBuildWorkloadRequest struct {
	// Workload is the generated Workload ({metadata, spec}). Only
	// spec.container.image and spec.endpoints are taken from it; its name,
	// namespace and owner are derived from the verified build instead.
	Workload json.RawMessage `json:"workload"`
	// WorkloadOverrides is the build's environment configuration
	// ({container: {env, files}}), or null when it has none.
	WorkloadOverrides json.RawMessage `json:"workloadOverrides,omitempty"`
}

// OrgBuildCredential stores where an organization's build client lives.
//
// The build client is an OAuth2 client provisioned per organization. A source
// build's workload step exchanges it for an org-scoped token and calls the build
// publisher endpoint with it. Only the client ID and the name of the
// SecretReference holding it are kept here: this service never uses the build
// client itself, so unlike OrgSchedulerCredential no copy of the secret is stored.
//
// A build receives the SecretReference by name, never a secret store location:
// the name resolves only inside the build's own organization namespace, so a run
// cannot be pointed at another organization's secret.
type OrgBuildCredential struct {
	ID       uuid.UUID `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID     string    `gorm:"column:ou_id;not null;uniqueIndex:uq_org_build_creds_ou_id"`
	OrgUUID  string    `gorm:"column:org_uuid;not null;default:''"`
	ClientID string    `gorm:"column:client_id;not null"`
	// AppID is the build client's Thunder application ID.
	AppID         string    `gorm:"column:app_id;not null;default:''"`
	SecretRefName string    `gorm:"column:secret_ref_name;not null"`
	CreatedAt     time.Time `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt     time.Time `gorm:"column:updated_at;not null;default:NOW()"`
}

func (OrgBuildCredential) TableName() string { return "org_build_credentials" }
