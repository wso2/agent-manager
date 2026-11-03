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

package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// mcpArtifact is what one (endpoint, environment) artifact of an MCP proxy renders from.
type mcpArtifact struct {
	// deployable is the endpoint's config flattened for the environment (buildMCPProxyEnvArtifact).
	deployable  *models.MCPProxy
	proxyHandle string
	scopes      []models.MCPProxyScope
}

// MCPProxyKind deploys MCP proxies: one artifact per (endpoint, environment), recorded in
// mcp_proxy_endpoint_environments and placed on the environment's egress gateway. MCP proxies are never
// undeployed, only deleted. Gateways store each artifact under its UUID.
type MCPProxyKind struct {
	gatewayRepo    repositories.GatewayRepository
	deploymentRepo repositories.DeploymentRepository
	load           func(ctx context.Context, artifact models.ArtifactRef) (*mcpArtifact, error)
}

// NewMCPProxyKind reads MCP proxies through the request's transaction when there is one.
func NewMCPProxyKind(gatewayRepo repositories.GatewayRepository, deploymentRepo repositories.DeploymentRepository) *MCPProxyKind {
	return &MCPProxyKind{gatewayRepo: gatewayRepo, deploymentRepo: deploymentRepo, load: loadMCPArtifact}
}

// loadMCPArtifact finds the artifact's (endpoint, environment) binding; gorm.ErrRecordNotFound when it is gone.
func loadMCPArtifact(ctx context.Context, artifact models.ArtifactRef) (*mcpArtifact, error) {
	tx := db.DB(ctx)
	var binding models.MCPProxyEndpointEnvironment
	if err := tx.Where("artifact_uuid = ?", artifact.ArtifactUUID).Take(&binding).Error; err != nil {
		return nil, err
	}
	proxy, err := repositories.NewMCPProxyRepo(tx).GetByUUID(ctx, binding.MCPProxyUUID.String(), artifact.OUID)
	if err != nil {
		return nil, err
	}
	for i := range proxy.Endpoints {
		endpoint := &proxy.Endpoints[i]
		if endpoint.UUID != binding.EndpointUUID {
			continue
		}
		scopes, err := repositories.NewMCPProxyScopeRepository(tx).ListByProxy(ctx, proxy.UUID)
		if err != nil {
			return nil, fmt.Errorf("failed to load proxy scopes: %w", err)
		}
		return &mcpArtifact{
			deployable:  buildMCPProxyEnvArtifact(proxy, endpoint, &binding),
			proxyHandle: mcpSourceHandle(proxy),
			scopes:      scopes,
		}, nil
	}
	return nil, gorm.ErrRecordNotFound
}

// mcpSourceHandle is the proxy's own handle, which its scope policies are prefixed with.
func mcpSourceHandle(proxy *models.MCPProxy) string {
	if proxy.Artifact != nil && proxy.Artifact.Handle != "" {
		return proxy.Artifact.Handle
	}
	return proxy.Handle
}

func (k *MCPProxyKind) Name() string        { return "Mcp" }
func (k *MCPProxyKind) RendersInline() bool { return true }

// ArtifactKinds: a proxy's per-(endpoint, environment) artifacts are McpProxyMapping rows; McpProxy is the proxy.
func (k *MCPProxyKind) ArtifactKinds() []string {
	return []string{models.KindMCPProxy, models.KindMCPMapping}
}

func (k *MCPProxyKind) GatewayArtifactID(artifact models.ArtifactRef) string {
	return artifact.ArtifactUUID.String()
}

func (k *MCPProxyKind) Render(ctx context.Context, artifact models.ArtifactRef) (reconcile.Rendered, error) {
	a, err := k.load(ctx, artifact)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return reconcile.Rendered{}, reconcile.ErrRecordGone
	}
	if err != nil {
		return reconcile.Rendered{}, err
	}
	content, err := renderMCPProxy(a.deployable, a.proxyHandle, a.scopes)
	if err != nil {
		return reconcile.Rendered{}, reconcile.Permanent(fmt.Errorf("%w: invalid MCP proxy configuration: %w", utils.ErrInvalidInput, err))
	}
	return renderedDeployment(a.deployable.Handle, []byte(content)), nil
}

func (k *MCPProxyKind) DeployEvent(ref reconcile.EventRef) eventhub.Event {
	payload := &models.MCPProxyDeploymentEvent{
		ProxyID: ref.ArtifactUUID.String(), DeploymentID: ref.DeploymentID, PerformedAt: ref.PerformedAt,
	}
	return eventhub.NewGatewayEvent("mcpproxy.deployed", "CREATE", payload.ProxyID, payload)
}

// UndeployEvent is sent only when the core undeploys a row whose binding went away before it rendered.
func (k *MCPProxyKind) UndeployEvent(ref reconcile.EventRef) eventhub.Event {
	payload := &models.MCPProxyUndeploymentEvent{ProxyID: ref.ArtifactUUID.String(), PerformedAt: ref.PerformedAt}
	return eventhub.NewGatewayEvent("mcpproxy.undeployed", "DELETE", payload.ProxyID, payload)
}

func (k *MCPProxyKind) DeleteEvent(artifact models.ArtifactRef) eventhub.Event {
	payload := &models.MCPProxyDeletionEvent{ProxyID: artifact.ArtifactUUID.String()}
	return eventhub.NewGatewayEvent("mcpproxy.deleted", "DELETE", payload.ProxyID, payload)
}

// UnplacedRecords are the (endpoint, environment) bindings with no row yet,
// in environments with a gateway of the kind's role.
func (k *MCPProxyKind) UnplacedRecords(
	ctx context.Context, ouID string, env *uuid.UUID, afterKey string, limit int,
) ([]reconcile.Record, error) {
	condition, args := reconcile.UnplacedCondition(reconcile.RecordColumns{
		OUID: "a.ou_id", ArtifactUUID: "a.uuid", EnvironmentUUID: "b.environment_uuid",
	}, models.EgressGatewayRoles, ouID, env)
	var rows []struct {
		OUID            string    `gorm:"column:ou_id"`
		ArtifactUUID    uuid.UUID `gorm:"column:artifact_uuid"`
		Kind            string    `gorm:"column:kind"`
		Handle          string    `gorm:"column:handle"`
		EnvironmentUUID uuid.UUID `gorm:"column:environment_uuid"`
	}
	err := db.DB(ctx).Raw(`SELECT a.ou_id, a.uuid AS artifact_uuid, a.kind, a.handle, b.environment_uuid
		FROM mcp_proxy_endpoint_environments b JOIN artifacts a ON a.uuid = b.artifact_uuid
		WHERE `+condition+` AND a.uuid::text > ?
		ORDER BY a.uuid::text LIMIT ?`, append(args, afterKey, limit)...).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list unplaced MCP proxy artifacts: %w", err)
	}
	records := make([]reconcile.Record, 0, len(rows))
	for _, r := range rows {
		records = append(records, reconcile.Record{
			ArtifactRef:     models.ArtifactRef{OUID: r.OUID, ArtifactUUID: r.ArtifactUUID, Kind: r.Kind, Handle: r.Handle},
			EnvironmentUUID: r.EnvironmentUUID,
			Key:             r.ArtifactUUID.String(),
		})
	}
	return records, nil
}

// PlacedOn: an (endpoint, environment) binding in env whose artifact is placed on gateway.
func (k *MCPProxyKind) PlacedOn(ctx context.Context, tx *gorm.DB, env, gateway uuid.UUID) (bool, error) {
	condition, args := reconcile.PlacedCondition(reconcile.RecordColumns{
		OUID: "a.ou_id", ArtifactUUID: "a.uuid", EnvironmentUUID: "b.environment_uuid",
	}, env, gateway)
	return placedOn(ctx, tx, `SELECT 1 FROM mcp_proxy_endpoint_environments b JOIN artifacts a ON a.uuid = b.artifact_uuid
		WHERE `+condition, args)
}

// Targets is the environment's egress gateway, anchored on where the artifact is already placed.
func (k *MCPProxyKind) Targets(ctx context.Context, record reconcile.Record) (uuid.UUID, bool, error) {
	deployments, gateways := placementReposIn(ctx, k.deploymentRepo, k.gatewayRepo)
	anchors, err := deployments.PlacedGateways(ctx, record.OUID, record.ArtifactUUID)
	if err != nil {
		return uuid.Nil, false, err
	}
	gateway, err := resolveEgressGatewayForArtifact(gateways, record.OUID, record.EnvironmentUUID, anchors, nil)
	if errors.Is(err, errNoGatewayForEnvironment) || errors.Is(err, errNoEgressGatewayForEnvironment) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return gateway.UUID, true, nil
}
