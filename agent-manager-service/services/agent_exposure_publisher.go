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
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// agentExposureKey names one agent-environment gateway exposure.
type agentExposureKey struct {
	OUID            string
	ProjectName     string
	AgentName       string
	EnvironmentName string
	EnvironmentUUID uuid.UUID
	// AgentUUID is nil when unknown; the reconciler then checks the agent by name.
	AgentUUID *uuid.UUID
}

// TxRunner opens one database transaction and runs fn inside it, so a unit test can hand fn a sentinel.
type TxRunner func(ctx context.Context, fn func(tx *gorm.DB) error) error

// NewTxRunner runs each transaction on db.
func NewTxRunner(db *gorm.DB) TxRunner {
	return func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return db.WithContext(ctx).Transaction(fn)
	}
}

const (
	// noGatewayReason is shown for an exposed environment with no ingress gateway: nothing could place it.
	noGatewayReason = "no gateway in this environment"
	// notPlacedReason is shown while an environment with a gateway waits for the placement pass.
	notPlacedReason = "waiting to be placed on the environment's gateway"
)

// AgentExposurePublisher holds the platform-only exposure steps every agent kind shares: recording an exposure,
// queueing its render on the environment's ingress gateway, reading what the user is shown, and tearing it down.
type AgentExposurePublisher struct {
	tx             TxRunner
	exposureRepo   repositories.PlatformAgentExposureRepository
	artifactRepo   repositories.ArtifactRepository
	deploymentRepo repositories.DeploymentRepository
	gatewayRepo    repositories.GatewayRepository
	core           *reconcile.Core
	logger         *slog.Logger
}

// NewAgentExposurePublisher creates an AgentExposurePublisher.
func NewAgentExposurePublisher(
	tx TxRunner,
	exposureRepo repositories.PlatformAgentExposureRepository,
	artifactRepo repositories.ArtifactRepository,
	deploymentRepo repositories.DeploymentRepository,
	gatewayRepo repositories.GatewayRepository,
	core *reconcile.Core,
	logger *slog.Logger,
) *AgentExposurePublisher {
	return &AgentExposurePublisher{
		tx:             tx,
		exposureRepo:   exposureRepo,
		artifactRepo:   artifactRepo,
		deploymentRepo: deploymentRepo,
		gatewayRepo:    gatewayRepo,
		core:           core,
		logger:         logger,
	}
}

// Expose records that a platform agent is exposed in the environment; a non-nil tx makes it part of the caller's
// transaction.
func (p *AgentExposurePublisher) Expose(
	ctx context.Context, tx *gorm.DB, key agentExposureKey, artifactUUID uuid.UUID, protocol models.AgentExposureProtocol,
) error {
	exposure := &models.PlatformAgentExposure{
		OUID:            key.OUID,
		ProjectName:     key.ProjectName,
		AgentName:       key.AgentName,
		EnvironmentName: key.EnvironmentName,
		EnvironmentUUID: key.EnvironmentUUID,
		ArtifactUUID:    artifactUUID,
		AgentUUID:       key.AgentUUID,
		Protocol:        protocol,
	}
	if err := p.exposures(tx).Upsert(ctx, exposure); err != nil {
		return fmt.Errorf("failed to record the agent's gateway exposure: %w", err)
	}
	return nil
}

// Publish exposes a platform agent in the environment and queues its render on the environment's ingress gateway,
// in tx. A row already there, whatever its status, is made due now: an agent deploy or promote is when its endpoint
// is about to appear. The caller nudges the worker after commit.
func (p *AgentExposurePublisher) Publish(
	ctx context.Context, tx *gorm.DB, kind reconcile.EnvironmentKind, key agentExposureKey, artifact models.ArtifactRef,
	protocol models.AgentExposureProtocol,
) error {
	if err := p.Expose(ctx, tx, key, artifact.ArtifactUUID, protocol); err != nil {
		return err
	}
	return p.DeployRecord(ctx, tx, kind, key.EnvironmentUUID, artifact)
}

// DeployRecord queues the render of the agent's artifact in the environment, in tx.
func (p *AgentExposurePublisher) DeployRecord(
	ctx context.Context, tx *gorm.DB, kind reconcile.EnvironmentKind, envUUID uuid.UUID, artifact models.ArtifactRef,
) error {
	_, err := p.core.DeployRecord(ctx, tx, kind, reconcile.Record{ArtifactRef: artifact, EnvironmentUUID: envUUID})
	if err != nil {
		return fmt.Errorf("failed to queue the agent's gateway deploy: %w", err)
	}
	return nil
}

// MarkStale re-renders the agent's artifact wherever it is placed, because an input outside it changed.
func (p *AgentExposurePublisher) MarkStale(ctx context.Context, artifact models.ArtifactRef, reason string) error {
	return p.core.MarkStale(ctx, artifact.Kind, artifact.OUID, artifact.ArtifactUUID, nil, reason)
}

// Nudge wakes the worker for a render a committed request queued.
func (p *AgentExposurePublisher) Nudge() {
	p.core.Nudge()
}

// GatewayURL is where the environment's ingress gateway serves the agent, or "" while none is mapped to it yet.
func (p *AgentExposurePublisher) GatewayURL(ouID string, envUUID uuid.UUID, agentName string) (string, error) {
	gateway, err := findIngressGateway(p.gatewayRepo, ouID, envUUID)
	if err != nil || gateway == nil {
		return "", err
	}
	return agentGatewayURL(gateway, agentName), nil
}

// ShownStatus is what the user is shown for the agent's deployment in the environment: its artifact's row on the
// ingress gateway. With no row, an environment without a gateway is UNDEPLOYED with noGatewayReason, and one with a
// gateway is still owed its deploy by the placement pass.
func (p *AgentExposurePublisher) ShownStatus(ctx context.Context, key agentExposureKey, hasGateway bool) (models.UserStatus, error) {
	rows, err := p.statusRecords(ctx, key)
	if err != nil {
		return models.UserStatus{}, err
	}
	return shownStatus(rows, hasGateway), nil
}

// AgentURL is where an agent is reachable and what the user is shown for it.
type AgentURL struct {
	// Gateway is the agent's URL on the ingress gateway, set once a placed row is on that gateway.
	Gateway string
	// Direct is set while the environment has a gateway but the agent has no exposure in it, such as a legacy release
	// that still renders its own route, so the agent is reached directly.
	Direct bool
	// Status is what the user is shown for the agent's gateway deployment; nil while the agent has no exposure in
	// the environment, since the gateway is not involved.
	Status *models.UserStatus
}

// For is the URL to show: the gateway's, else directURL while direct, else "".
func (u AgentURL) For(directURL string) string {
	switch {
	case u.Gateway != "":
		return u.Gateway
	case u.Direct:
		return directURL
	default:
		return ""
	}
}

// ResolveURL reads the environment's ingress gateway, the agent's exposure and its rows once and says where the
// agent is reached.
func (p *AgentExposurePublisher) ResolveURL(ctx context.Context, key agentExposureKey) (AgentURL, error) {
	gateway, err := findIngressGateway(p.gatewayRepo, key.OUID, key.EnvironmentUUID)
	if err != nil {
		return AgentURL{}, err
	}
	rows, err := p.statusRecords(ctx, key)
	if err != nil {
		return AgentURL{}, err
	}
	exposed, err := p.isExposed(ctx, key)
	if err != nil {
		return AgentURL{}, err
	}
	var url AgentURL
	if exposed {
		status := shownStatus(rows, gateway != nil)
		url.Status = &status
	}
	if gateway == nil {
		return url, nil
	}
	switch {
	case isPlacedOn(rows, gateway.UUID):
		url.Gateway = agentGatewayURL(gateway, key.AgentName)
	case !exposed:
		url.Direct = true
	}
	return url, nil
}

func (p *AgentExposurePublisher) isExposed(ctx context.Context, key agentExposureKey) (bool, error) {
	_, err := p.exposureRepo.Get(ctx, key.OUID, key.ProjectName, key.AgentName, key.EnvironmentUUID)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, repositories.ErrPlatformAgentExposureNotFound):
		return false, nil
	default:
		return false, fmt.Errorf("failed to read the agent's exposure: %w", err)
	}
}

func (p *AgentExposurePublisher) statusRecords(ctx context.Context, key agentExposureKey) ([]models.DeploymentStatusRecord, error) {
	artifact, err := agentEnvAPIArtifact(
		p.artifactRepo, key.OUID, key.ProjectName, key.AgentName, key.EnvironmentUUID.String(), models.AgentArtifactKinds...,
	)
	switch {
	case err == nil:
		return p.deploymentRepo.StatusRecords(ctx, key.OUID, artifact.UUID)
	case errors.Is(err, utils.ErrArtifactNotFound):
		return nil, nil
	default:
		return nil, fmt.Errorf("failed to read the agent's artifact: %w", err)
	}
}

// isPlacedOn is reconcile.PlacedCondition applied to the artifact's rows.
func isPlacedOn(rows []models.DeploymentStatusRecord, gateway uuid.UUID) bool {
	return slices.ContainsFunc(rows, func(row models.DeploymentStatusRecord) bool {
		return row.GatewayUUID == gateway &&
			(row.StatusDesired == models.DeploymentStatusDeployed || row.NextAttemptAt != nil)
	})
}

func shownStatus(rows []models.DeploymentStatusRecord, hasGateway bool) models.UserStatus {
	if row := models.ShownStatusRecord(rows); row != nil {
		return models.UserStatusOf(*row)
	}
	if !hasGateway {
		return models.UserStatus{Status: models.DeploymentStatusUndeployed, Reason: noGatewayReason}
	}
	return models.UserStatus{Status: models.DeploymentStatusDeploying, Reason: notPlacedReason}
}

// TeardownEnv undeploys one environment's artifact from every gateway it is placed on and drops its exposure, in tx.
// The artifact and its API keys stay for a later redeploy. The undeploy events are written in tx.
func (p *AgentExposurePublisher) TeardownEnv(ctx context.Context, tx *gorm.DB, key agentExposureKey, artifact models.ArtifactRef) error {
	if artifact.ArtifactUUID != uuid.Nil {
		gateways, err := p.deploymentRepo.WithTx(tx).PlacedGateways(ctx, key.OUID, artifact.ArtifactUUID)
		if err != nil {
			return fmt.Errorf("failed to list the agent's gateways: %w", err)
		}
		slices.Sort(gateways)
		for _, gateway := range gateways {
			placement := models.Placement{ArtifactRef: artifact, GatewayUUID: uuid.MustParse(gateway)}
			if err := p.core.Undeploy(ctx, tx, placement); err != nil {
				return fmt.Errorf("failed to undeploy the agent: %w", err)
			}
		}
	}
	err := p.exposures(tx).DeleteForAgentEnv(ctx, key.OUID, key.ProjectName, key.AgentName, key.EnvironmentUUID)
	if err != nil {
		return fmt.Errorf("failed to delete the agent's exposure: %w", err)
	}
	return nil
}

// DropArtifact deletes the agent's per-environment artifact (its exposure, deployments and API keys cascade) and
// writes its delete events to every gateway it was sent, in tx.
func (p *AgentExposurePublisher) DropArtifact(ctx context.Context, tx *gorm.DB, artifact models.ArtifactRef) error {
	events, err := p.core.PrepareDelete(ctx, tx, artifact)
	if err != nil {
		return err
	}
	if err := p.deleteArtifact(tx, artifact); err != nil {
		return err
	}
	return events.Publish(ctx, tx)
}

func (p *AgentExposurePublisher) dropArtifact(ctx context.Context, artifact models.ArtifactRef) error {
	return p.tx(ctx, func(tx *gorm.DB) error { return p.DropArtifact(ctx, tx, artifact) })
}

func (p *AgentExposurePublisher) deleteArtifact(tx *gorm.DB, artifact models.ArtifactRef) error {
	err := p.artifactRepo.Delete(tx, artifact.ArtifactUUID.String())
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to delete the agent's API artifact: %w", err)
	}
	return nil
}

func (p *AgentExposurePublisher) exposures(tx *gorm.DB) repositories.PlatformAgentExposureRepository {
	if tx == nil {
		return p.exposureRepo
	}
	return p.exposureRepo.WithTx(tx)
}
