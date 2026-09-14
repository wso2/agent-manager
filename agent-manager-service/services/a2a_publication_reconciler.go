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
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

const (
	a2aReconcilerTickInterval = 30 * time.Second
	// a2aReconcilerLockID is this loop's own PostgreSQL advisory lock ID,
	// distinct from schedulerLockID and reconcilerLockID so the three background
	// loops never block each other.
	a2aReconcilerLockID  = int64(739281458)
	a2aReconcilerBatch   = 50
	a2aReconcilerRetryIn = 30 * time.Second

	// a2aPublicationAttemptBudget bounds how long an agent may fail to publish
	// an upstream before the row is called failed.
	//
	// The number is the agent startup budget (10 minutes, the point past which
	// the agent-api startup probe has already given up at least once, so nothing
	// is still starting) divided by the tick interval, with slack. Past it the
	// binding is not going to report a ServiceURL, and retrying forever would
	// only hide that from whoever has to fix it.
	a2aPublicationAttemptBudget = 30
)

// errUpstreamNotReady means the binding has not published a ServiceURL yet. It
// is the expected condition for the first few attempts after a deploy, not a
// misconfiguration.
var errUpstreamNotReady = errors.New("release binding has not published a service URL yet")

// A2APublicationReconcilerService drains the a2a_publications queue.
type A2APublicationReconcilerService interface {
	Start(ctx context.Context) error
	Stop() error
	// RunOnce drains the currently-due batch once. It is the same cycle the
	// ticker drives, exposed so a caller — a test, or an operator tool — can
	// advance the queue deterministically instead of waiting out a tick.
	RunOnce(ctx context.Context)
}

type a2aPublicationReconcilerService struct {
	pubRepo         repositories.A2APublicationRepository
	deploymentRepo  repositories.DeploymentRepository
	gatewayRepo     repositories.GatewayRepository
	agentConfigRepo repositories.AgentConfigRepository
	ocClient        client.OpenChoreoClient
	events          *GatewayEventsService
	logger          *slog.Logger
	stopCh          chan struct{}
	stopOnce        sync.Once
}

// NewA2APublicationReconcilerService creates an A2APublicationReconcilerService.
func NewA2APublicationReconcilerService(
	pubRepo repositories.A2APublicationRepository,
	deploymentRepo repositories.DeploymentRepository,
	gatewayRepo repositories.GatewayRepository,
	agentConfigRepo repositories.AgentConfigRepository,
	ocClient client.OpenChoreoClient,
	events *GatewayEventsService,
	logger *slog.Logger,
) A2APublicationReconcilerService {
	return &a2aPublicationReconcilerService{
		pubRepo:         pubRepo,
		deploymentRepo:  deploymentRepo,
		gatewayRepo:     gatewayRepo,
		agentConfigRepo: agentConfigRepo,
		ocClient:        ocClient,
		events:          events,
		logger:          logger,
		stopCh:          make(chan struct{}),
		stopOnce:        sync.Once{},
	}
}

func (s *a2aPublicationReconcilerService) Start(ctx context.Context) error {
	go s.runLoop(ctx)
	s.logger.Info("A2A publication reconciler started")
	return nil
}

func (s *a2aPublicationReconcilerService) Stop() error {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.logger.Info("A2A publication reconciler stopped")
	})
	return nil
}

func (s *a2aPublicationReconcilerService) runLoop(ctx context.Context) {
	ticker := time.NewTicker(a2aReconcilerTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runCycle(ctx)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// RunOnce drains the currently-due batch once.
func (s *a2aPublicationReconcilerService) RunOnce(ctx context.Context) {
	s.runCycle(ctx)
}

// runCycle claims the due batch under an advisory lock so only one replica
// scans at a time, then releases it before the slow OpenChoreo and event-hub
// calls — mirroring agentThunderReconcilerService.runCycle.
func (s *a2aPublicationReconcilerService) runCycle(ctx context.Context) {
	tx := db.GetDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		s.logger.Error("Failed to begin transaction for A2A publication advisory lock", "error", tx.Error)
		return
	}

	var locked bool
	if err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", a2aReconcilerLockID).Scan(&locked).Error; err != nil {
		s.logger.Error("Failed to try A2A publication advisory lock", "error", err)
		tx.Rollback()
		return
	}
	if !locked {
		tx.Rollback()
		return
	}

	due, err := s.pubRepo.FindDue(ctx, time.Now(), a2aReconcilerBatch)
	if err != nil {
		s.logger.Error("Failed to query due A2A publications", "error", err)
		tx.Rollback()
		return
	}
	if err := tx.Commit().Error; err != nil {
		s.logger.Error("Failed to commit A2A publication advisory lock transaction", "error", err)
		return
	}

	for _, pub := range due {
		s.publishOne(ctx, pub)
	}
}

// publishOne emits one agent-environment pair's Agent resource, or schedules a
// retry when it cannot yet.
func (s *a2aPublicationReconcilerService) publishOne(ctx context.Context, pub models.A2APublication) {
	if err := s.attemptPublish(ctx, pub); err != nil {
		s.recordAttemptFailure(ctx, pub, err)
		return
	}
	if err := s.pubRepo.MarkPublished(ctx, pub.ID); err != nil {
		s.logger.Error("Published A2A agent but failed to mark the queue row",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName, "error", err)
	}
}

// recordAttemptFailure retries within the budget and gives up past it.
func (s *a2aPublicationReconcilerService) recordAttemptFailure(ctx context.Context, pub models.A2APublication, cause error) {
	if pub.AttemptCount+1 >= a2aPublicationAttemptBudget {
		s.logger.Error("A2A agent never reached its gateway within the attempt budget",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName,
			"attempts", pub.AttemptCount+1, "error", cause)
		if err := s.pubRepo.MarkFailed(ctx, pub.ID, cause.Error()); err != nil {
			s.logger.Error("Failed to mark A2A publication failed", "error", err)
		}
		return
	}
	s.logger.Debug("A2A agent not publishable yet, will retry",
		"agentName", pub.AgentName, "environment", pub.EnvironmentName,
		"attempt", pub.AttemptCount+1, "reason", cause)
	if err := s.pubRepo.MarkAttemptFailed(ctx, pub.ID, cause.Error(), time.Now().Add(a2aReconcilerRetryIn)); err != nil {
		s.logger.Error("Failed to schedule A2A publication retry", "error", err)
	}
}

func (s *a2aPublicationReconcilerService) attemptPublish(ctx context.Context, pub models.A2APublication) error {
	upstreamURL, err := s.ocClient.GetReleaseBindingServiceURL(ctx, pub.OUID, pub.AgentName, pub.EnvironmentName)
	if err != nil {
		return fmt.Errorf("failed to read release binding service URL: %w", err)
	}
	if upstreamURL == "" {
		return errUpstreamNotReady
	}

	gateway, err := s.resolveGateway(pub)
	if err != nil {
		return err
	}

	// The policy chain is exactly what a chat/custom agent gets: the persisted
	// per-environment config, run through the same buildPolicies. No A2A-specific
	// policy exists in M1 — the gateway's own Agent rules supply the rest.
	cfg, err := s.agentConfigRepo.Get(ctx, pub.OUID, pub.ProjectName, pub.AgentName, pub.EnvironmentName)
	if err != nil {
		return fmt.Errorf("failed to load agent config: %w", err)
	}
	policies := buildPolicies(resolveAPIConfig(cfg, nil, nil, nil, nil, false))

	yamlStr, err := generateA2AAgentDeploymentYAML(A2AAgentDeploymentInput{
		ArtifactName: a2aAgentEnvArtifactName(pub.ProjectName, pub.AgentName, pub.EnvironmentUUID.String()),
		DisplayName:  pub.AgentName,
		AgentName:    pub.AgentName,
		Vhost:        gateway.Vhost,
		UpstreamURL:  upstreamURL,
		Policies:     policies,
	})
	if err != nil {
		return err
	}

	deploymentID := uuid.New()
	deployed := models.DeploymentStatusDeployed
	deployment := &models.Deployment{
		DeploymentID: deploymentID,
		Name:         fmt.Sprintf("%s-deployment", pub.AgentName),
		ArtifactUUID: pub.ArtifactUUID,
		OUID:         pub.OUID,
		GatewayUUID:  gateway.UUID,
		Content:      []byte(yamlStr),
		Status:       &deployed,
	}
	// The deployments row is not optional bookkeeping: GET /agents/{agentId}
	// serves the Agent YAML back out of it when the gateway fetches after the
	// event, so an event without a row is an event the gateway cannot act on.
	if err := s.deploymentRepo.CreateWithLimitEnforcement(deployment, maxDeploymentsPerAPI+deploymentLimitBuffer); err != nil {
		return fmt.Errorf("failed to create A2A agent deployment row: %w", err)
	}

	event := &models.AgentDeploymentEvent{
		AgentID:      pub.ArtifactUUID.String(),
		DeploymentID: deploymentID.String(),
		PerformedAt:  time.Now().Truncate(time.Millisecond),
	}
	if err := s.events.BroadcastAgentDeploymentEvent(gateway.UUID.String(), event); err != nil {
		return fmt.Errorf("failed to broadcast agent deployment event: %w", err)
	}

	s.logger.Info("Published A2A agent to gateway",
		"agentName", pub.AgentName, "environment", pub.EnvironmentName,
		"artifactID", pub.ArtifactUUID, "gateway", gateway.Name, "upstream", upstreamURL)
	return nil
}

// resolveGateway picks the environment's gateway. An A2A agent is inbound
// traffic, so it belongs on the environment's INGRESS gateway — the same slot a
// REST agent's api-configuration trait targets via the apiGatewayName
// convention — not on an egress gateway, which hosts outbound LLM/MCP artifacts.
func (s *a2aPublicationReconcilerService) resolveGateway(pub models.A2APublication) (*models.Gateway, error) {
	envID := pub.EnvironmentUUID.String()
	gateways, err := s.gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID: pub.OUID,
		EnvironmentID:  &envID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list gateways for environment %s: %w", envID, err)
	}
	for _, gw := range gateways {
		if gw != nil && gw.IsIngressCapable() {
			return gw, nil
		}
	}
	// An environment has at most one ingress gateway, and it is registered by the
	// bootstrap job, so absence is a timing condition rather than a
	// misconfiguration — retry.
	return nil, fmt.Errorf("no ingress gateway is mapped to environment %s yet", envID)
}
