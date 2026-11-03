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

// Package reconcile is the shared gateway reconciliation model: placement, the render queue, deploy, undeploy,
// restore, outside-input triggers and the worker. A gateway artifact kind plugs in through Kind, and the core never
// interprets its content or policies.
package reconcile

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// Rendered is one artifact's content as a deployment stores it.
type Rendered struct {
	Name    string
	Content []byte
}

// EventRef is what an artifact's deploy or undeploy event is built from. An undeploy carries no deployment: its
// DeploymentID and DeploymentName are empty, and the gateway undeploys whatever version it holds.
type EventRef struct {
	models.Placement
	DeploymentID   string
	DeploymentName string
	PerformedAt    time.Time
	// Tx is the transaction the event is written in: a kind reading its own tables for the event reads through it.
	Tx *gorm.DB
}

// Kind is what a gateway artifact kind supplies to the core.
type Kind interface {
	// Name is the kind the gateway knows: LlmProvider, LlmProxy, Mcp or Agent.
	Name() string
	// Render returns the artifact's content from the kind's own tables, read through db.DB(ctx): an inline render
	// runs in the saving transaction and must see its writes. It takes no gateway, so a save's validation render
	// and the worker's render produce the same thing. An environment-addressed kind re-checks its record and returns
	// ErrRecordGone when it is gone; InputNotReady when a dependency isn't there yet.
	Render(ctx context.Context, artifact models.ArtifactRef) (Rendered, error)
	// GatewayArtifactID is the id the gateway stores the artifact under: its UUID, or an LLM proxy's handle.
	GatewayArtifactID(artifact models.ArtifactRef) string
	DeployEvent(ref EventRef) eventhub.Event
	UndeployEvent(ref EventRef) eventhub.Event
	// DeleteEvent is built before the delete, while the artifact row still exists.
	DeleteEvent(artifact models.ArtifactRef) eventhub.Event
	// RendersInline is false for a kind whose Render calls outside AMS: only the worker renders it, outside any
	// transaction.
	RendersInline() bool
}

// ArtifactKinds is implemented by a kind that deploys artifacts whose artifacts.kind differs from its Name (MCP
// proxies and their mappings are both the gateway's Mcp). Without it, the kind deploys artifacts of kind Name.
type ArtifactKinds interface {
	ArtifactKinds() []string
}

// Record is one environment-addressed "should be deployed" record: the artifact in an environment.
type Record struct {
	models.ArtifactRef
	EnvironmentUUID uuid.UUID
	// RequestedGateway is a gateway the request named (MCP's RequestedGatewayUUID); Targets is not asked.
	RequestedGateway *uuid.UUID
	// Key orders the kind's records for UnplacedRecords' paging.
	Key string
}

// EnvironmentKind is a kind addressed by environment: its own per-environment records say what should be deployed,
// and Targets picks the gateway.
type EnvironmentKind interface {
	Kind
	// UnplacedRecords pages through records, after afterKey, whose artifact has no deployment_status row and whose
	// environment has a live gateway (UnplacedCondition). env nil means every environment of ouID; ouID "" means
	// the fleet.
	UnplacedRecords(ctx context.Context, ouID string, env *uuid.UUID, afterKey string, limit int) ([]Record, error)
	// Targets is the gateway the record goes to; ok is false when its environment has no suitable gateway.
	Targets(ctx context.Context, record Record) (gateway uuid.UUID, ok bool, err error)
	// PlacedOn reports whether any of the kind's records in env has its artifact placed on gateway
	// (PlacedCondition), read through tx.
	PlacedOn(ctx context.Context, tx *gorm.DB, env, gateway uuid.UUID) (bool, error)
}

// AfterLiveHook runs in the ack's transaction when a row moves to DEPLOYED. It must only touch the database.
type AfterLiveHook interface {
	AfterLive(ctx context.Context, tx *gorm.DB, placement models.Placement, deploymentID uuid.UUID) error
}

// DriftChecker reports whether a DEPLOYED placement's inputs moved since it was rendered. The worker's drift loop
// asks it and marks a drifted placement for a re-render.
type DriftChecker interface {
	Drifted(ctx context.Context, placement models.Placement) (reason string, drifted bool, err error)
}

// ErrorDescriber turns a render error into the reason a user is shown (render_reason).
type ErrorDescriber interface {
	DescribeRenderError(err error) string
}
