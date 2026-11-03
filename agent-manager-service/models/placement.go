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

// ArtifactRef names an artifact without a gateway: what a kind renders, and what a delete event is built from.
type ArtifactRef struct {
	OUID         string
	ArtifactUUID uuid.UUID
	Kind         string
	Handle       string
}

// Placement is one deployment_status row: an artifact on a gateway.
type Placement struct {
	ArtifactRef
	GatewayUUID uuid.UUID
}

// ClaimedRender is a placement leased for rendering. Token is the leased next_attempt_at; a write-back applies
// only while the row still holds it.
type ClaimedRender struct {
	Placement
	Token        time.Time
	AttemptCount int
	// ClaimedAt is the claim's now: the attempt's time, recorded with its outcome.
	ClaimedAt time.Time
	// LastDelay is how long the row waited before this claim, from its last outcome to when it fell due. A row that
	// keeps waiting for an input doubles it; zero after a request queued the row.
	LastDelay time.Duration
}

// GatewaySyncRow is one GET /deployments entry before the kind maps the artifact to the id the gateway stores.
type GatewaySyncRow struct {
	ArtifactRef
	DeploymentID string
	State        string
	DeployedAt   time.Time
}

// The states GET /deployments lists: the row's desired state, which is what the gateway reconciles towards.
const (
	GatewaySyncStateDeployed   = "deployed"
	GatewaySyncStateUndeployed = "undeployed"
)

// GatewayDeploymentContent is one deployment a gateway fetches in its startup sync, with the artifact it belongs to.
type GatewayDeploymentContent struct {
	ArtifactRef
	DeploymentID uuid.UUID
	Content      []byte
}
