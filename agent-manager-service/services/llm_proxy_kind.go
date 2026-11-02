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

// LLMProxyKind deploys LLM proxies. Gateways store a proxy under its handle. A proxy is deployed to a gateway the
// public route names, or to its environment's egress gateway from an agent's model configuration
// (env_agent_model_mapping) or a monitor (monitor_llm_mapping).
type LLMProxyKind struct {
	gatewayRepo    repositories.GatewayRepository
	deploymentRepo repositories.DeploymentRepository
	load           func(ctx context.Context, artifact models.ArtifactRef) (*models.LLMProxy, *models.LLMProvider, error)
	vhostOf        func(tx *gorm.DB, artifact models.ArtifactRef) string
}

// NewLLMProxyKind reads proxies and their providers through the request's transaction when there is one.
func NewLLMProxyKind(gatewayRepo repositories.GatewayRepository, deploymentRepo repositories.DeploymentRepository) *LLMProxyKind {
	return &LLMProxyKind{
		gatewayRepo: gatewayRepo, deploymentRepo: deploymentRepo, load: loadLLMProxy, vhostOf: proxyEventVhost,
	}
}

func loadLLMProxy(ctx context.Context, artifact models.ArtifactRef) (*models.LLMProxy, *models.LLMProvider, error) {
	tx := db.DB(ctx)
	proxy, err := repositories.NewLLMProxyRepo(tx).GetByIDCtx(ctx, artifact.Handle, artifact.OUID)
	if err != nil {
		return nil, nil, err
	}
	provider, err := repositories.NewLLMProviderRepo(tx).GetByUUID(proxy.Configuration.Provider, artifact.OUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, reconcile.Permanent(utils.ErrLLMProviderNotFound)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get provider: %w", err)
	}
	return proxy, provider, nil
}

func (k *LLMProxyKind) Name() string        { return models.KindLLMProxy }
func (k *LLMProxyKind) RendersInline() bool { return true }

func (k *LLMProxyKind) GatewayArtifactID(artifact models.ArtifactRef) string { return artifact.Handle }

// Render re-checks the proxy itself: a proxy deployed by the public route has no per-environment record, and an
// agent's or monitor's proxy is deleted with its record.
func (k *LLMProxyKind) Render(ctx context.Context, artifact models.ArtifactRef) (reconcile.Rendered, error) {
	proxy, provider, err := k.load(ctx, artifact)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return reconcile.Rendered{}, reconcile.ErrRecordGone
	}
	if err != nil {
		return reconcile.Rendered{}, err
	}
	content, err := renderLLMProxy(proxy, provider)
	if err != nil {
		return reconcile.Rendered{}, reconcile.Permanent(fmt.Errorf("%w: invalid LLM proxy configuration: %w", utils.ErrInvalidInput, err))
	}
	return renderedDeployment(artifact.Handle, []byte(content)), nil
}

// proxyEventVhost is the vhost a proxy's events carry: the proxy row's, read in the event's transaction.
func proxyEventVhost(tx *gorm.DB, artifact models.ArtifactRef) string {
	if tx == nil {
		return ""
	}
	proxy, err := repositories.NewLLMProxyRepo(tx).GetByIDCtx(context.Background(), artifact.Handle, artifact.OUID)
	if err != nil {
		return ""
	}
	return proxyVhost(proxy)
}

func (k *LLMProxyKind) DeployEvent(ref reconcile.EventRef) eventhub.Event {
	payload := &models.LLMProxyDeploymentEvent{
		ProxyID:        ref.Handle,
		DeploymentID:   ref.DeploymentID,
		PerformedAt:    ref.PerformedAt,
		Vhost:          k.vhost(ref),
		Environment:    "production",
		GatewayID:      ref.GatewayUUID.String(),
		OrganizationID: ref.OUID,
		Status:         string(models.DeploymentStatusDeployed),
	}
	return eventhub.NewGatewayEvent("llmproxy.deployed", "CREATE", payload.ProxyID, payload)
}

func (k *LLMProxyKind) UndeployEvent(ref reconcile.EventRef) eventhub.Event {
	payload := &models.LLMProxyUndeploymentEvent{
		ProxyID:        ref.Handle,
		PerformedAt:    ref.PerformedAt,
		Vhost:          k.vhost(ref),
		Environment:    "production",
		GatewayID:      ref.GatewayUUID.String(),
		OrganizationID: ref.OUID,
	}
	return eventhub.NewGatewayEvent("llmproxy.undeployed", "DELETE", payload.ProxyID, payload)
}

// DeleteEvent drops the proxy's config from the gateway: an undeploy keeps it and its policy chains.
func (k *LLMProxyKind) DeleteEvent(artifact models.ArtifactRef) eventhub.Event {
	payload := &models.LLMProxyDeletionEvent{ProxyID: artifact.Handle}
	return eventhub.NewGatewayEvent("llmproxy.deleted", "DELETE", payload.ProxyID, payload)
}

func (k *LLMProxyKind) vhost(ref reconcile.EventRef) string {
	if k.vhostOf == nil {
		return ""
	}
	return k.vhostOf(ref.Tx, ref.ArtifactRef)
}

// monitorEnvironmentUUID reads a monitor's environment id, stored as text, as a UUID; NULL when it isn't one.
const monitorEnvironmentUUID = `(CASE WHEN mon.environment_id ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
	THEN mon.environment_id::uuid END)`

// UnplacedRecords are the agents' and monitors' proxies with no row yet,
// in environments with a gateway of the kind's role.
func (k *LLMProxyKind) UnplacedRecords(
	ctx context.Context, ouID string, env *uuid.UUID, afterKey string, limit int,
) ([]reconcile.Record, error) {
	agents, agentArgs := reconcile.UnplacedCondition(reconcile.RecordColumns{
		OUID: "a.ou_id", ArtifactUUID: "a.uuid", EnvironmentUUID: "m.environment_uuid",
	}, models.EgressGatewayRoles, ouID, env)
	monitors, monitorArgs := reconcile.UnplacedCondition(reconcile.RecordColumns{
		OUID: "a.ou_id", ArtifactUUID: "a.uuid", EnvironmentUUID: monitorEnvironmentUUID,
	}, models.EgressGatewayRoles, ouID, env)
	query := `SELECT * FROM (
			SELECT a.ou_id, a.uuid AS artifact_uuid, a.handle, m.environment_uuid
			FROM env_agent_model_mapping m JOIN artifacts a ON a.uuid = m.llm_proxy_uuid
			WHERE ` + agents + `
			UNION
			SELECT a.ou_id, a.uuid, a.handle, ` + monitorEnvironmentUUID + `
			FROM monitor_llm_mapping m JOIN artifacts a ON a.uuid = m.llm_proxy_uuid
			JOIN monitors mon ON mon.id = m.monitor_id
			WHERE ` + monitors + `
		) r WHERE r.artifact_uuid::text || '/' || r.environment_uuid::text > ?
		ORDER BY r.artifact_uuid::text || '/' || r.environment_uuid::text LIMIT ?`
	args := append(append(agentArgs, monitorArgs...), afterKey, limit)
	var rows []struct {
		OUID            string    `gorm:"column:ou_id"`
		ArtifactUUID    uuid.UUID `gorm:"column:artifact_uuid"`
		Handle          string    `gorm:"column:handle"`
		EnvironmentUUID uuid.UUID `gorm:"column:environment_uuid"`
	}
	if err := db.DB(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list unplaced LLM proxies: %w", err)
	}
	records := make([]reconcile.Record, 0, len(rows))
	for _, r := range rows {
		records = append(records, reconcile.Record{
			ArtifactRef: models.ArtifactRef{
				OUID: r.OUID, ArtifactUUID: r.ArtifactUUID, Kind: models.KindLLMProxy, Handle: r.Handle,
			},
			EnvironmentUUID: r.EnvironmentUUID,
			Key:             r.ArtifactUUID.String() + "/" + r.EnvironmentUUID.String(),
		})
	}
	return records, nil
}

// PlacedOn: an agent's or monitor's proxy in env placed on gateway. A proxy deployed by the public route has no
// per-environment record, so it never counts.
func (k *LLMProxyKind) PlacedOn(ctx context.Context, tx *gorm.DB, env, gateway uuid.UUID) (bool, error) {
	agents, agentArgs := reconcile.PlacedCondition(reconcile.RecordColumns{
		OUID: "a.ou_id", ArtifactUUID: "a.uuid", EnvironmentUUID: "m.environment_uuid",
	}, env, gateway)
	monitors, monitorArgs := reconcile.PlacedCondition(reconcile.RecordColumns{
		OUID: "a.ou_id", ArtifactUUID: "a.uuid", EnvironmentUUID: monitorEnvironmentUUID,
	}, env, gateway)
	return placedOn(ctx, tx, `SELECT 1 FROM env_agent_model_mapping m JOIN artifacts a ON a.uuid = m.llm_proxy_uuid
			WHERE `+agents+`
		UNION ALL
		SELECT 1 FROM monitor_llm_mapping m JOIN artifacts a ON a.uuid = m.llm_proxy_uuid
			JOIN monitors mon ON mon.id = m.monitor_id
			WHERE `+monitors, append(agentArgs, monitorArgs...))
}

// Targets is the environment's egress gateway, anchored on where the proxy, or else its provider, is placed, so
// a proxy lands next to its provider.
func (k *LLMProxyKind) Targets(ctx context.Context, record reconcile.Record) (uuid.UUID, bool, error) {
	proxy, _, err := k.load(ctx, record.ArtifactRef)
	if err != nil {
		return uuid.Nil, false, err
	}
	deployments, gateways := placementReposIn(ctx, k.deploymentRepo, k.gatewayRepo)
	anchors, err := deployments.PlacedGateways(ctx, record.OUID, record.ArtifactUUID)
	if err != nil {
		return uuid.Nil, false, err
	}
	if providerUUID, err := uuid.Parse(proxy.Configuration.Provider); err == nil {
		providerGateways, err := deployments.PlacedGateways(ctx, record.OUID, providerUUID)
		if err != nil {
			return uuid.Nil, false, err
		}
		anchors = append(anchors, providerGateways...)
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
