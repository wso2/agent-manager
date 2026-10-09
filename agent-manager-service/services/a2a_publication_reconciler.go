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
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	a2aReconcilerTickInterval = 30 * time.Second
	a2aReconcilerBatch        = 50
	// a2aReconcilerRetryIn sits just under the tick so a retry is due on the next tick.
	a2aReconcilerRetryIn = a2aReconcilerTickInterval - 5*time.Second
	// a2aDriftCheckBatch caps how many published rows one tick re-checks.
	a2aDriftCheckBatch = 50

	// a2aPublicationAttemptBudget bounds how many failed attempts, one per tick,
	// a row gets before it is called failed: 30 ticks of 30s is about 15 minutes.
	// Waiting for the binding's ServiceURL is not charged, since a first source
	// build can legitimately take longer; this budget covers real failures such
	// as a missing ingress gateway or a broadcast that keeps erroring.
	a2aPublicationAttemptBudget = 30
)

// errUpstreamNotReady means the binding has not published a ServiceURL yet. It
// is the expected condition until the first build or deploy binds, not a
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
	cardRepo        repositories.A2AAgentCardRepository
	deploymentRepo  repositories.DeploymentRepository
	gatewayRepo     repositories.GatewayRepository
	agentConfigRepo repositories.AgentConfigRepository
	ocClient        client.OpenChoreoClient
	events          *GatewayEventsService
	logger          *slog.Logger
	stopCh          chan struct{}
	stopOnce        sync.Once
	// driftCursor is the last published row id the drift scan checked.
	driftCursor uuid.UUID
}

// NewA2APublicationReconcilerService creates an A2APublicationReconcilerService.
func NewA2APublicationReconcilerService(
	pubRepo repositories.A2APublicationRepository,
	cardRepo repositories.A2AAgentCardRepository,
	deploymentRepo repositories.DeploymentRepository,
	gatewayRepo repositories.GatewayRepository,
	agentConfigRepo repositories.AgentConfigRepository,
	ocClient client.OpenChoreoClient,
	events *GatewayEventsService,
	logger *slog.Logger,
) A2APublicationReconcilerService {
	return &a2aPublicationReconcilerService{
		pubRepo:         pubRepo,
		cardRepo:        cardRepo,
		deploymentRepo:  deploymentRepo,
		gatewayRepo:     gatewayRepo,
		agentConfigRepo: agentConfigRepo,
		ocClient:        ocClient,
		events:          events,
		logger:          logger,
		stopCh:          make(chan struct{}),
		stopOnce:        sync.Once{},
		driftCursor:     uuid.Nil,
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

// runCycle publishes the due batch. ClaimDue leases each row, so replicas
// running concurrently never take the same row.
func (s *a2aPublicationReconcilerService) runCycle(ctx context.Context) {
	due, err := s.pubRepo.ClaimDue(ctx, time.Now(), a2aReconcilerBatch)
	if err != nil {
		s.logger.Error("Failed to claim due A2A publications", "error", err)
		return
	}

	for _, pub := range due {
		s.publishOne(ctx, pub)
	}
	s.checkUpstreamDrift(ctx)
}

// checkUpstreamDrift requeues published rows whose binding now reports a
// different upstream, e.g. after a rebuild on a new port. It checks one page of
// rows per tick and resumes after it on the next, wrapping at the end.
func (s *a2aPublicationReconcilerService) checkUpstreamDrift(ctx context.Context) {
	rows, err := s.pubRepo.FindPublished(ctx, s.driftCursor, a2aDriftCheckBatch)
	if err != nil {
		s.logger.Error("Failed to query published A2A publications for drift", "error", err)
		return
	}
	if len(rows) < a2aDriftCheckBatch {
		s.driftCursor = uuid.Nil
	} else {
		s.driftCursor = rows[len(rows)-1].ID
	}

	for _, pub := range rows {
		rollout, err := s.ocClient.GetReleaseBindingRollout(ctx, pub.OUID, pub.AgentName, pub.EnvironmentName)
		if err != nil {
			continue
		}
		current := rollout.ServiceURL
		if current == "" || current == pub.PublishedUpstreamURL {
			s.refetchCardOfNewRelease(ctx, pub, rollout)
			continue
		}
		s.logger.Info("A2A agent upstream drifted from what its gateway was given; republishing",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName,
			"publishedUpstream", pub.PublishedUpstreamURL, "currentUpstream", current)
		err = s.pubRepo.Requeue(ctx, pub)
		switch {
		case errors.Is(err, repositories.ErrA2APublicationSuperseded):
			s.logSuperseded(pub)
		case err != nil:
			s.logger.Error("Failed to requeue drifted A2A publication",
				"agentName", pub.AgentName, "environment", pub.EnvironmentName, "error", err)
		}
	}
}

// refetchCardOfNewRelease re-queues a platform card fetched from an older release than the one now serving.
func (s *a2aPublicationReconcilerService) refetchCardOfNewRelease(ctx context.Context, pub models.A2APublication, rollout client.ReleaseBindingRollout) {
	if !rollout.Serving || rollout.ReleaseName == "" {
		return
	}
	card, err := s.cardRepo.Get(ctx, pub.OUID, pub.ProjectName, pub.AgentName, pub.EnvironmentName)
	if err != nil {
		if !errors.Is(err, repositories.ErrA2AAgentCardNotFound) {
			s.logger.Warn("Failed to read A2A agent card for release drift",
				"agentName", pub.AgentName, "environment", pub.EnvironmentName, "error", err)
		}
		return
	}
	if card.Source != models.A2AAgentCardSourcePlatform || card.Status == models.A2AAgentCardStatusPending ||
		card.ReleaseName == rollout.ReleaseName {
		return
	}
	s.logger.Info("A2A agent serves a new release; re-fetching its card",
		"agentName", pub.AgentName, "environment", pub.EnvironmentName,
		"cardRelease", card.ReleaseName, "currentRelease", rollout.ReleaseName)
	s.enqueueCardFetch(ctx, pub)
}

// publishOne emits one agent-environment pair's Agent resource, or schedules a
// retry when it cannot yet.
func (s *a2aPublicationReconcilerService) publishOne(ctx context.Context, pub models.A2APublication) {
	upstreamURL, err := s.attemptPublish(ctx, pub)
	if err != nil {
		s.recordAttemptFailure(ctx, pub, err)
		return
	}
	err = s.pubRepo.MarkPublished(ctx, pub, upstreamURL)
	switch {
	case errors.Is(err, repositories.ErrA2APublicationSuperseded):
		s.logSuperseded(pub)
	case err != nil:
		s.logger.Error("Published A2A agent but failed to mark the queue row",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName, "error", err)
	default:
		s.enqueueCardFetch(ctx, pub)
	}
}

// enqueueCardFetch queues a fetch of the card the gateway now serves; best effort.
func (s *a2aPublicationReconcilerService) enqueueCardFetch(ctx context.Context, pub models.A2APublication) {
	card := &models.A2AAgentCard{
		OUID:            pub.OUID,
		ProjectName:     pub.ProjectName,
		AgentName:       pub.AgentName,
		EnvironmentName: pub.EnvironmentName,
		EnvironmentUUID: pub.EnvironmentUUID,
		Source:          models.A2AAgentCardSourcePlatform,
	}
	if err := s.cardRepo.Enqueue(ctx, card); err != nil {
		s.logger.Warn("Published A2A agent but failed to queue its card fetch",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName, "error", err)
		return
	}
	s.dropCardIfAgentGone(ctx, pub)
}

// dropCardIfAgentGone removes a card row written after a racing DeleteAgent cleared the agent's rows.
func (s *a2aPublicationReconcilerService) dropCardIfAgentGone(ctx context.Context, pub models.A2APublication) {
	_, err := s.ocClient.GetComponent(ctx, pub.OUID, pub.ProjectName, pub.AgentName)
	if !errors.Is(err, utils.ErrNotFound) {
		return
	}
	if err := s.cardRepo.DeleteForAgentEnv(ctx, pub.OUID, pub.ProjectName, pub.AgentName, pub.EnvironmentName); err != nil {
		s.logger.Warn("Failed to drop the card row of a deleted A2A agent",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName, "error", err)
	}
}

// logSuperseded notes an attempt whose row was re-enqueued while it ran. The
// row stays pending, so the next tick publishes the newer config; nothing is
// wrong.
func (s *a2aPublicationReconcilerService) logSuperseded(pub models.A2APublication) {
	s.logger.Info("A2A publication was re-enqueued during the attempt; leaving it pending for the next tick",
		"agentName", pub.AgentName, "environment", pub.EnvironmentName)
}

// recordAttemptFailure waits out a not-ready binding, retries other failures
// within the budget, and gives up past it.
func (s *a2aPublicationReconcilerService) recordAttemptFailure(ctx context.Context, pub models.A2APublication, cause error) {
	if errors.Is(cause, errUpstreamNotReady) {
		s.recordWaiting(ctx, pub, cause)
		return
	}
	if pub.AttemptCount+1 >= a2aPublicationAttemptBudget {
		s.logger.Error("A2A agent never reached its gateway within the attempt budget",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName,
			"attempts", pub.AttemptCount+1, "error", cause)
		err := s.pubRepo.MarkFailed(ctx, pub, cause.Error())
		switch {
		case errors.Is(err, repositories.ErrA2APublicationSuperseded):
			s.logSuperseded(pub)
		case err != nil:
			s.logger.Error("Failed to mark A2A publication failed", "error", err)
		}
		return
	}
	s.logger.Debug("A2A agent not publishable yet, will retry",
		"agentName", pub.AgentName, "environment", pub.EnvironmentName,
		"attempt", pub.AttemptCount+1, "reason", cause)
	err := s.pubRepo.MarkAttemptFailed(ctx, pub, cause.Error(), time.Now().Add(a2aReconcilerRetryIn))
	switch {
	case errors.Is(err, repositories.ErrA2APublicationSuperseded):
		s.logSuperseded(pub)
	case err != nil:
		s.logger.Error("Failed to schedule A2A publication retry", "error", err)
	}
}

// recordWaiting reschedules a row whose binding has no ServiceURL yet.
func (s *a2aPublicationReconcilerService) recordWaiting(ctx context.Context, pub models.A2APublication, cause error) {
	s.logger.Debug("A2A agent binding not ready yet, will check again",
		"agentName", pub.AgentName, "environment", pub.EnvironmentName)
	err := s.pubRepo.MarkWaiting(ctx, pub, cause.Error(), time.Now().Add(a2aReconcilerRetryIn))
	switch {
	case errors.Is(err, repositories.ErrA2APublicationSuperseded):
		s.logSuperseded(pub)
	case err != nil:
		s.logger.Error("Failed to schedule A2A publication retry", "error", err)
	}
}

// attemptPublish returns the upstream URL it gave the gateway.
func (s *a2aPublicationReconcilerService) attemptPublish(ctx context.Context, pub models.A2APublication) (string, error) {
	upstreamURL, err := s.ocClient.GetReleaseBindingServiceURL(ctx, pub.OUID, pub.AgentName, pub.EnvironmentName)
	if err != nil {
		return "", fmt.Errorf("failed to read release binding service URL: %w", err)
	}
	if upstreamURL == "" {
		return "", errUpstreamNotReady
	}

	gateway, err := s.resolveGateway(pub)
	if err != nil {
		return "", err
	}

	// The policy chain is the persisted per-environment config run through the
	// same buildPolicies a chat/custom agent gets, plus the one header every A2A
	// client must send. The public card route runs its own list.
	cfg, err := s.agentConfigRepo.Get(ctx, pub.OUID, pub.ProjectName, pub.AgentName, pub.EnvironmentName)
	if err != nil {
		return "", fmt.Errorf("failed to load agent config: %w", err)
	}
	policies := buildPolicies(withA2AVersionHeader(resolveAPIConfig(cfg, nil, nil, nil, nil, false)))

	yamlStr, err := generateA2AAgentDeploymentYAML(A2AAgentDeploymentInput{
		ArtifactName: a2aAgentEnvArtifactName(pub.ProjectName, pub.AgentName, pub.EnvironmentUUID.String()),
		DisplayName:  pub.AgentName,
		AgentName:    pub.AgentName,
		UpstreamURL:  upstreamURL,
		Policies:     policies,
		CardPolicies: buildCardPolicies(cfg.EffectiveCardCORS()),
	})
	if err != nil {
		return "", err
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
		return "", fmt.Errorf("failed to create A2A agent deployment row: %w", err)
	}

	event := &models.AgentDeploymentEvent{
		AgentID:      pub.ArtifactUUID.String(),
		DeploymentID: deploymentID.String(),
		PerformedAt:  time.Now().Truncate(time.Millisecond),
	}
	if err := s.events.BroadcastAgentDeploymentEvent(gateway.UUID.String(), event); err != nil {
		return "", fmt.Errorf("failed to broadcast agent deployment event: %w", err)
	}

	s.logger.Info("Published A2A agent to gateway",
		"agentName", pub.AgentName, "environment", pub.EnvironmentName,
		"artifactID", pub.ArtifactUUID, "gateway", gateway.Name, "upstream", upstreamURL)
	return upstreamURL, nil
}

// resolveGateway picks the environment's gateway. An A2A agent is inbound
// traffic, so it belongs on the environment's INGRESS gateway — the same slot a
// REST agent's api-configuration trait targets via the apiGatewayName
// convention — not on an egress gateway, which hosts outbound LLM/MCP artifacts.
func (s *a2aPublicationReconcilerService) resolveGateway(pub models.A2APublication) (*models.Gateway, error) {
	envID := pub.EnvironmentUUID.String()
	gateways, err := s.gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID:      pub.OUID,
		FunctionalityTypeIn: models.IngressGatewayRoles,
		EnvironmentID:       &envID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list gateways for environment %s: %w", envID, err)
	}
	for _, gw := range gateways {
		if gw != nil {
			return gw, nil
		}
	}
	// An environment has at most one ingress gateway, and it is registered by the
	// bootstrap job, so absence is a timing condition rather than a
	// misconfiguration — retry.
	return nil, fmt.Errorf("no ingress gateway is mapped to environment %s yet", envID)
}
