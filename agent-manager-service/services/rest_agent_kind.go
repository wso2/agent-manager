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
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// RestAgentKind exposes REST agents (chat-api, custom-api) as a gateway RestApi on their environment's ingress
// gateway. The exposure is recorded in platform_agent_exposures; Render reads the agent's endpoint from the platform store,
// so only the worker renders it.
type RestAgentKind struct {
	exposures       *PlatformAgentExposures
	agentConfigRepo repositories.AgentConfigRepository
	deploymentRepo  repositories.DeploymentRepository
	apiKeyRepo      repositories.APIKeyRepository
	events          *GatewayEventsService
	logger          *slog.Logger
}

// NewRestAgentKind builds the REST agent kind.
func NewRestAgentKind(
	exposures *PlatformAgentExposures,
	agentConfigRepo repositories.AgentConfigRepository,
	deploymentRepo repositories.DeploymentRepository,
	apiKeyRepo repositories.APIKeyRepository,
	events *GatewayEventsService,
	logger *slog.Logger,
) *RestAgentKind {
	return &RestAgentKind{
		exposures: exposures, agentConfigRepo: agentConfigRepo, deploymentRepo: deploymentRepo,
		apiKeyRepo: apiKeyRepo, events: events, logger: logger,
	}
}

func (k *RestAgentKind) Name() string        { return models.KindRestAPI }
func (k *RestAgentKind) RendersInline() bool { return false }

func (k *RestAgentKind) GatewayArtifactID(artifact models.ArtifactRef) string {
	return artifact.ArtifactUUID.String()
}

// DescribeRenderError is the reason a user is shown: the public part of the error, never internals.
func (k *RestAgentKind) DescribeRenderError(err error) string {
	return userFacingA2APublishError(err)
}

// Render builds the RestApi from the agent's exposure, endpoint and environment config.
func (k *RestAgentKind) Render(ctx context.Context, artifact models.ArtifactRef) (reconcile.Rendered, error) {
	ctx = orgctx.WithResolvedOrg(ctx, orgctx.ResolvedOrg{OUID: artifact.OUID})
	exposure, err := k.exposures.exposure(ctx, nil, artifact)
	if errors.Is(err, repositories.ErrPlatformAgentExposureNotFound) {
		return reconcile.Rendered{}, reconcile.ErrRecordGone
	}
	if err != nil {
		return reconcile.Rendered{}, err
	}
	url, err := k.exposures.serviceURL(ctx, exposure)
	if err != nil {
		return reconcile.Rendered{}, err
	}
	cfg, err := k.agentConfigRepo.Get(ctx, exposure.OUID, exposure.ProjectName, exposure.AgentName, exposure.EnvironmentName)
	if err != nil {
		return reconcile.Rendered{}, fmt.Errorf("failed to load agent config: %w", err)
	}
	timeoutSeconds, err := resolveResilienceTimeoutSeconds(cfg, nil, true)
	if err != nil {
		return reconcile.Rendered{}, err
	}
	content, err := generateRestAPIDeploymentYAML(RestAPIDeploymentInput{
		ProjectName:              exposure.ProjectName,
		AgentName:                exposure.AgentName,
		EnvironmentUUID:          exposure.EnvironmentUUID.String(),
		UpstreamURL:              url,
		Policies:                 buildPolicies(resolveAPIConfig(cfg, nil, nil, nil, nil, false)),
		ResilienceTimeoutSeconds: timeoutSeconds,
	})
	if err != nil {
		return reconcile.Rendered{}, err
	}
	return renderedDeployment(exposure.AgentName, content), nil
}

// DeployEvent tells the gateway to fetch /apis/{apiId}, keyed on the artifact UUID.
func (k *RestAgentKind) DeployEvent(ref reconcile.EventRef) eventhub.Event {
	payload := &models.APIDeploymentEvent{
		APIID: ref.ArtifactUUID.String(), DeploymentID: ref.DeploymentID, PerformedAt: ref.PerformedAt,
	}
	return eventhub.NewGatewayEvent("api.deployed", "CREATE", payload.APIID, payload)
}

// UndeployEvent stops the gateway serving the API but keeps it, and its API keys, for a redeploy.
func (k *RestAgentKind) UndeployEvent(ref reconcile.EventRef) eventhub.Event {
	payload := &models.APIUndeploymentEvent{APIID: ref.ArtifactUUID.String(), PerformedAt: ref.PerformedAt}
	return eventhub.NewGatewayEvent("api.undeployed", "DELETE", payload.APIID, payload)
}

// DeleteEvent drops the API together with its API keys.
func (k *RestAgentKind) DeleteEvent(artifact models.ArtifactRef) eventhub.Event {
	payload := &models.APIDeletionEvent{APIID: artifact.ArtifactUUID.String()}
	return eventhub.NewGatewayEvent("api.deleted", "DELETE", payload.APIID, payload)
}

// UnplacedRecords are the REST exposures whose artifact has no row yet, in environments with an ingress gateway.
func (k *RestAgentKind) UnplacedRecords(
	ctx context.Context, ouID string, env *uuid.UUID, afterKey string, limit int,
) ([]reconcile.Record, error) {
	arm, args := unplacedExposureArm(models.AgentExposureProtocolREST, ouID, env)
	records, err := pageUnplacedRecords(ctx, db.DB(ctx), arm, args, afterKey, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list unplaced REST agents: %w", err)
	}
	return records, nil
}

// PlacedOn: a REST exposure in env whose artifact is placed on gateway.
func (k *RestAgentKind) PlacedOn(ctx context.Context, tx *gorm.DB, env, gateway uuid.UUID) (bool, error) {
	arm, args := placedExposureArm(models.AgentExposureProtocolREST, env, gateway)
	return placedOn(ctx, tx, arm, args)
}

// Targets is the environment's ingress gateway.
func (k *RestAgentKind) Targets(ctx context.Context, record reconcile.Record) (uuid.UUID, bool, error) {
	return k.exposures.target(ctx, record)
}

// AfterLive replays the agent's API keys to the gateway that has just started serving it.
func (k *RestAgentKind) AfterLive(ctx context.Context, tx *gorm.DB, p models.Placement, _ uuid.UUID) error {
	return replayAgentKeys(ctx, tx, k.apiKeyRepo, k.events, p, time.Now())
}

// Drifted reports an agent whose release binding now serves a URL other than the one its current deployment was
// rendered with.
func (k *RestAgentKind) Drifted(ctx context.Context, p models.Placement) (string, bool, error) {
	exposure, err := k.exposures.exposure(ctx, nil, p.ArtifactRef)
	if errors.Is(err, repositories.ErrPlatformAgentExposureNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	ctx = orgctx.WithResolvedOrg(ctx, orgctx.ResolvedOrg{OUID: p.OUID})
	serving, err := k.exposures.serviceURL(ctx, exposure)
	if errors.Is(err, reconcile.ErrInputNotReady) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	deployed, err := deployedUpstream(ctx, k.deploymentRepo, p, func(y RestAPIDeploymentYAML) string {
		return y.Spec.Upstream.Main.URL
	})
	if err != nil {
		return "", false, err
	}
	if serving != deployed {
		return endpointChangedReason, true, nil
	}
	return "", false, nil
}
