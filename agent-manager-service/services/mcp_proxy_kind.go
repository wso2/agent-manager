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
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

// mcpArtifact is what one (endpoint, environment) artifact of an MCP proxy renders from.
type mcpArtifact struct {
	// deployable is the endpoint's config flattened for the environment (buildMCPProxyEnvArtifact).
	deployable  *models.MCPProxy
	proxyHandle string
	scopes      []models.MCPProxyScope
	// projectScoped: a project proxy's binding, whose upstream AMS resolved itself.
	projectScoped bool
}

// newMCPArtifact is what the binding ee of endpoint renders from.
func newMCPArtifact(
	proxy *models.MCPProxy, endpoint *models.MCPProxyEndpoint, ee *models.MCPProxyEndpointEnvironment,
	scopes []models.MCPProxyScope,
) *mcpArtifact {
	return &mcpArtifact{
		deployable:    buildMCPProxyEnvArtifact(proxy, endpoint, ee),
		proxyHandle:   mcpSourceHandle(proxy),
		scopes:        scopes,
		projectScoped: proxy.ProjectUUID != nil,
	}
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
	if err := tx.Where("artifact_id = ?", artifact.ArtifactUUID).Take(&binding).Error; err != nil {
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
		return newMCPArtifact(proxy, endpoint, &binding, scopes), nil
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

// DescribeRenderError is the reason a binding shows for a failed render: CONTEXT_IN_USE for a context clash, the
// error itself otherwise.
func (k *MCPProxyKind) DescribeRenderError(err error) string {
	if errors.Is(err, utils.ErrMCPContextInUse) {
		return utils.ErrCodeContextInUse
	}
	return err.Error()
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
		OUID: "a.org_id", ArtifactUUID: "a.id", EnvironmentUUID: "b.environment_id",
	}, models.EgressGatewayRoles, ouID, env)
	var rows []struct {
		OUID            string    `gorm:"column:org_id"`
		ArtifactUUID    uuid.UUID `gorm:"column:artifact_id"`
		Kind            string    `gorm:"column:kind"`
		Handle          string    `gorm:"column:handle"`
		EnvironmentUUID uuid.UUID `gorm:"column:environment_id"`
	}
	q := db.DB(ctx).Table("mcp_proxy_endpoint_environments b").
		Select("a.org_id, a.id AS artifact_id, a.kind, a.handle, b.environment_id").
		Joins("JOIN artifacts a ON a.id = b.artifact_id").
		Where(condition, args...)
	if afterKey != "" {
		q = q.Where("a.id > ?", afterKey)
	}
	err := q.Order("a.id").Limit(limit).Scan(&rows).Error
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
		OUID: "a.org_id", ArtifactUUID: "a.id", EnvironmentUUID: "b.environment_id",
	}, env, gateway)
	return placedOn(ctx, tx, `SELECT 1 AS placed FROM mcp_proxy_endpoint_environments b JOIN artifacts a ON a.id = b.artifact_id
		WHERE `+condition, args)
}

// Targets is the environment's egress gateway, anchored on where the artifact is already placed. A managed
// target is not taken unless the upstream is public: a private upstream was accepted for a self-hosted
// gateway, and an anchor that has gone away falls back to the environment's managed gateway.
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
	if gatewayNeedsPublicUpstream(gateway) {
		public, err := k.upstreamIsPublic(ctx, record, gateway)
		if err != nil || !public {
			return uuid.Nil, false, err
		}
	}
	if !gateway.IsManaged() {
		projectScoped, err := k.isProjectScoped(ctx, record)
		if err != nil || projectScoped {
			return uuid.Nil, false, err
		}
	}
	return gateway.UUID, true, nil
}

// isProjectScoped reports whether the artifact is a project proxy's binding. A project binding's upstream is an
// in-cluster address that a customer-run, self-hosted gateway has no route to, so it is never placed on one. An
// artifact that is gone is not: Render reports it.
func (k *MCPProxyKind) isProjectScoped(ctx context.Context, record reconcile.Record) (bool, error) {
	a, err := k.load(ctx, record.ArtifactRef)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if a.projectScoped {
		logger.GetLogger(ctx).Warn("Not placing a project MCP proxy binding on a self-hosted gateway",
			"ouID", record.OUID, "artifactUUID", record.ArtifactUUID, "environmentUUID", record.EnvironmentUUID)
	}
	return a.projectScoped, nil
}

// upstreamIsPublic re-runs the private-address check on the artifact's upstreams for a managed target. An
// artifact that is gone passes: Render reports it. A project proxy's binding passes too: its upstream is the
// in-cluster address AMS resolved from the project's own release binding, never a caller-supplied URL.
func (k *MCPProxyKind) upstreamIsPublic(ctx context.Context, record reconcile.Record, gateway *models.Gateway) (bool, error) {
	a, err := k.load(ctx, record.ArtifactRef)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if a.projectScoped {
		return true, nil
	}
	upstream := a.deployable.Configuration.Upstream
	for _, endpoint := range []*models.UpstreamEndpoint{upstream.Main, upstream.Sandbox} {
		if endpoint == nil {
			continue
		}
		if err := ssrf.ValidateURL(ctx, strings.TrimSpace(endpoint.URL)); err != nil {
			logger.GetLogger(ctx).Warn("Not placing MCP proxy on a managed gateway; its upstream is not public",
				"ouID", record.OUID, "artifactUUID", record.ArtifactUUID, "environmentUUID", record.EnvironmentUUID,
				"gatewayUUID", gateway.UUID, "error", err)
			return false, nil
		}
	}
	return true, nil
}
