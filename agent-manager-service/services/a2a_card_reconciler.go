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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

const (
	a2aCardTickInterval = 5 * time.Second
	a2aCardBatch        = 50
	a2aCardBaseBackoff  = 5 * time.Second
	a2aCardMaxBackoff   = time.Minute
	// a2aCardAttemptTimeout bounds one row's OpenChoreo reads, DNS check and fetch.
	a2aCardAttemptTimeout = 30 * time.Second
	// a2aCardBatchBudget leaves the claim lease a minute to record the last outcome.
	a2aCardBatchBudget = repositories.A2AAgentCardClaimLease - time.Minute
	// a2aCardAttemptBudget is about 10 minutes of backoff.
	a2aCardAttemptBudget = 14
	// a2aCardUnrecordableError replaces a cause the store refused to record.
	a2aCardUnrecordableError = "agent card fetch failed; the error could not be recorded"
	// a2aCardUnstorableError replaces a card the store refuses to take as text.
	a2aCardUnstorableError = "the agent card contains characters that cannot be stored"
)

// errA2ACardEndpointNotReady means the agent's environment has no public endpoint URL yet.
var errA2ACardEndpointNotReady = newA2APublicError("agent has no public endpoint in this environment yet", nil)

// errA2ACardRolloutInProgress means the agent's latest release is not serving in this environment yet.
var errA2ACardRolloutInProgress = newA2APublicError("agent is still rolling out in this environment", nil)

// errA2ACardNotDeployed means the agent has no live binding in the row's environment.
var errA2ACardNotDeployed = errors.New("agent is not deployed in this environment")

// A2ACardReconcilerService drains the a2a_agent_cards fetch queue.
type A2ACardReconcilerService interface {
	Start(ctx context.Context) error
	Stop() error
	// RunOnce drains the currently-due batch once.
	RunOnce(ctx context.Context)
}

type a2aCardReconcilerService struct {
	cardRepo    repositories.A2AAgentCardRepository
	proxyRepo   repositories.A2AProxyRepository
	gatewayRepo repositories.GatewayRepository
	fetcher     A2ACardFetcher
	ocClient    client.OpenChoreoClient
	logger      *slog.Logger
	stopCh      chan struct{}
	stopOnce    sync.Once

	attemptTimeout time.Duration
	batchBudget    time.Duration
}

// NewA2ACardReconcilerService creates an A2ACardReconcilerService.
func NewA2ACardReconcilerService(
	cardRepo repositories.A2AAgentCardRepository,
	proxyRepo repositories.A2AProxyRepository,
	gatewayRepo repositories.GatewayRepository,
	fetcher A2ACardFetcher,
	ocClient client.OpenChoreoClient,
	logger *slog.Logger,
) A2ACardReconcilerService {
	return &a2aCardReconcilerService{
		cardRepo:    cardRepo,
		proxyRepo:   proxyRepo,
		gatewayRepo: gatewayRepo,
		fetcher:     fetcher,
		ocClient:    ocClient,
		logger:      logger,
		stopCh:      make(chan struct{}),
		stopOnce:    sync.Once{},

		attemptTimeout: a2aCardAttemptTimeout,
		batchBudget:    a2aCardBatchBudget,
	}
}

func (s *a2aCardReconcilerService) Start(ctx context.Context) error {
	go func() {
		s.requeueFetchedWithoutProjection(ctx)
		s.runLoop(ctx)
	}()
	s.logger.Info("A2A card reconciler started")
	return nil
}

func (s *a2aCardReconcilerService) Stop() error {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.logger.Info("A2A card reconciler stopped")
	})
	return nil
}

func (s *a2aCardReconcilerService) runLoop(ctx context.Context) {
	ticker := time.NewTicker(a2aCardTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.RunOnce(ctx)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *a2aCardReconcilerService) RunOnce(ctx context.Context) {
	s.requeueFetchedWithoutProjection(ctx)
	claimedAt := time.Now()
	due, err := s.cardRepo.ClaimDue(ctx, claimedAt, a2aCardBatch)
	if err != nil {
		s.logger.Error("Failed to claim due A2A agent cards", "error", err)
		return
	}
	drainWithinLease(ctx, s.logger, claimedAt.Add(s.batchBudget), due, "A2A card", s.fetchOne)
}

// requeueFetchedWithoutProjection runs at start and on every tick, not only in migration 049: while a rolling upgrade
// lasts, a pod from before the projection can claim a requeued row and store its card without one. Once every pod
// writes the projection the update matches nothing.
func (s *a2aCardReconcilerService) requeueFetchedWithoutProjection(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, s.attemptTimeout)
	defer cancel()
	n, err := s.cardRepo.RequeueFetchedWithoutProjection(ctx)
	if err != nil {
		s.logger.Error("Failed to requeue A2A agent cards stored without a projection; retrying next tick",
			"error", err)
		return
	}
	if n > 0 {
		s.logger.Info("Requeued A2A agent cards stored without a projection", "count", n)
	}
}

// fetchOne fetches one row's card and records the outcome.
func (s *a2aCardReconcilerService) fetchOne(ctx context.Context, row models.A2AAgentCard) {
	attemptCtx, ctx, cancel := a2aAttemptContexts(ctx, s.attemptTimeout)
	defer cancel()
	fetched, err := s.attemptFetch(attemptCtx, row)
	if errors.Is(err, errA2ACardNotDeployed) {
		s.dropUndeployedCard(ctx, row)
		return
	}
	if err != nil {
		s.recordAttemptFailure(ctx, row, fetched.releaseName, err)
		return
	}
	hash, err := a2aCardHash(fetched.body)
	if err != nil {
		s.recordAttemptFailure(ctx, row, fetched.releaseName, err)
		return
	}
	projection, err := a2aCardProjection(fetched.body)
	if err != nil {
		s.recordAttemptFailure(ctx, row, fetched.releaseName, err)
		return
	}
	err = s.cardRepo.MarkFetched(ctx, row, fetched.body, hash, fetched.url, fetched.releaseName, projection)
	switch {
	case errors.Is(err, repositories.ErrA2AAgentCardSuperseded):
		s.logSuperseded(row)
	case isUnstorableTextError(err):
		s.recordAttemptFailure(ctx, row, fetched.releaseName, newA2APublicError(a2aCardUnstorableError, err))
	case err != nil:
		s.recordAttemptFailure(ctx, row, fetched.releaseName, fmt.Errorf("failed to store agent card: %w", err))
	}
}

// isUnstorableTextError reports Postgres refusing text it cannot represent: a \u0000 escape or invalid UTF-8.
func isUnstorableTextError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "22P05" || pgErr.Code == "22021")
}

// dropUndeployedCard removes the row as undeploy would; a row re-queued meanwhile is kept.
func (s *a2aCardReconcilerService) dropUndeployedCard(ctx context.Context, row models.A2AAgentCard) {
	err := s.cardRepo.DeleteIfUnchanged(ctx, row)
	switch {
	case errors.Is(err, repositories.ErrA2AAgentCardSuperseded):
		s.logSuperseded(row)
	case err != nil:
		s.logger.Warn("Failed to drop the card row of an undeployed A2A agent",
			"agentName", row.AgentName, "environment", row.EnvironmentName, "error", err)
	default:
		s.logger.Info("A2A agent is not deployed in this environment; dropped its card row",
			"agentName", row.AgentName, "environment", row.EnvironmentName)
	}
}

// a2aCardFetch is a fetched card, the URL it came from and, for platform cards, the release that served it.
type a2aCardFetch struct {
	url         string
	releaseName string
	body        json.RawMessage
}

func (s *a2aCardReconcilerService) attemptFetch(ctx context.Context, row models.A2AAgentCard) (a2aCardFetch, error) {
	switch row.Source {
	case models.A2AAgentCardSourceExternal:
		return s.fetchExternalCard(ctx, row)
	case models.A2AAgentCardSourcePlatform:
		return s.fetchPlatformCard(ctx, row)
	default:
		return a2aCardFetch{}, fmt.Errorf("unknown agent card source %q", row.Source)
	}
}

// fetchExternalCard fetches through the gateway while the environment is proxied, since only the gateway's copy advertises gateway URLs; otherwise from the registered URL, SSRF-guarded.
func (s *a2aCardReconcilerService) fetchExternalCard(ctx context.Context, row models.A2AAgentCard) (a2aCardFetch, error) {
	_, err := s.proxyRepo.GetByEnvUUID(ctx, row.OUID, row.ProjectName, row.AgentName, row.EnvironmentUUID)
	switch {
	case errors.Is(err, repositories.ErrA2AProxyNotFound):
		return s.fetchRegisteredCard(ctx, row)
	case err != nil:
		return a2aCardFetch{}, fmt.Errorf("failed to read the A2A proxy: %w", err)
	}
	return s.fetchCardThroughGateway(ctx, row)
}

func (s *a2aCardReconcilerService) fetchRegisteredCard(ctx context.Context, row models.A2AAgentCard) (a2aCardFetch, error) {
	body, err := s.fetcher.FetchOrigin(ctx, row.SourceURL, nil)
	return a2aCardFetch{url: row.SourceURL, body: body}, err
}

// fetchCardThroughGateway uses the plain client: the URL is platform-derived and resolves to loopback in local setups.
func (s *a2aCardReconcilerService) fetchCardThroughGateway(ctx context.Context, row models.A2AAgentCard) (a2aCardFetch, error) {
	gateway, err := resolveIngressGateway(s.gatewayRepo, row.OUID, row.EnvironmentUUID)
	if err != nil {
		return a2aCardFetch{}, err
	}
	url := a2aGatewayURL(gateway, row.AgentName) + a2aAgentCardPath
	body, err := s.fetcher.Fetch(ctx, url)
	if err != nil {
		return a2aCardFetch{url: url}, prefixA2APublicError(url, err)
	}
	return a2aCardFetch{url: url, body: body}, nil
}

// fetchPlatformCard waits out a rollout first, since until it finishes the old release can still answer.
func (s *a2aCardReconcilerService) fetchPlatformCard(ctx context.Context, row models.A2AAgentCard) (a2aCardFetch, error) {
	rollout, err := s.ocClient.GetReleaseBindingRollout(ctx, row.OUID, row.AgentName, row.EnvironmentName)
	if err != nil {
		return a2aCardFetch{}, fmt.Errorf("failed to read agent rollout: %w", err)
	}
	if !rollout.Deployed {
		return a2aCardFetch{}, errA2ACardNotDeployed
	}
	if !rollout.Serving {
		return a2aCardFetch{}, errA2ACardRolloutInProgress
	}
	attempt := a2aCardFetch{releaseName: rollout.ReleaseName}
	if rollout.ExternalURL == "" {
		return attempt, errA2ACardEndpointNotReady
	}
	attempt.url = strings.TrimRight(rollout.ExternalURL, "/") + a2aAgentCardPath
	body, err := s.fetcher.Fetch(ctx, attempt.url)
	if err != nil {
		// The derived URL is otherwise invisible to the user.
		return attempt, prefixA2APublicError(attempt.url, err)
	}
	attempt.body = body
	return attempt, nil
}

func (s *a2aCardReconcilerService) recordAttemptFailure(ctx context.Context, row models.A2AAgentCard, releaseName string, cause error) {
	lastErr := userFacingA2AError(cause, a2aCardInternalError)
	warnIfInternalA2AError(s.logger, lastErr, a2aCardInternalError, cause, "A2A agent card fetch failed internally",
		"agentName", row.AgentName, "environment", row.EnvironmentName)
	err := s.markAttemptFailed(ctx, row, cause, lastErr, releaseName)
	if err != nil && !errors.Is(err, repositories.ErrA2AAgentCardSuperseded) {
		s.logger.Error("Failed to record A2A agent card attempt; retrying with a fixed message",
			"agentName", row.AgentName, "error", err)
		err = s.markAttemptFailed(ctx, row, cause, a2aCardUnrecordableError, releaseName)
	}
	switch {
	case errors.Is(err, repositories.ErrA2AAgentCardSuperseded):
		s.logSuperseded(row)
	case err != nil:
		s.logger.Error("Failed to record A2A agent card attempt", "agentName", row.AgentName, "error", err)
	}
}

// markAttemptFailed charges one attempt, failing the row once the budget is spent.
func (s *a2aCardReconcilerService) markAttemptFailed(ctx context.Context, row models.A2AAgentCard, cause error, lastErr, releaseName string) error {
	if row.AttemptCount+1 >= a2aCardAttemptBudget {
		s.logger.Warn("A2A agent card could not be fetched within the attempt budget",
			"agentName", row.AgentName, "environment", row.EnvironmentName, "error", a2aLogError(cause))
		return s.cardRepo.MarkFailed(ctx, row, lastErr, releaseName)
	}
	s.logger.Debug("A2A agent card not fetchable yet, will retry",
		"agentName", row.AgentName, "environment", row.EnvironmentName,
		"attempt", row.AttemptCount+1, "reason", a2aLogError(cause))
	return s.cardRepo.MarkAttemptFailed(ctx, row, lastErr, releaseName, time.Now().Add(a2aCardRetryIn(row.AttemptCount)))
}

func (s *a2aCardReconcilerService) logSuperseded(row models.A2AAgentCard) {
	s.logger.Info("A2A agent card was re-enqueued during the attempt; leaving it for the next tick",
		"agentName", row.AgentName, "environment", row.EnvironmentName)
}

// a2aCardRetryIn is min(5s · 2^attempt, 60s).
func a2aCardRetryIn(attempt int) time.Duration {
	delay := a2aCardBaseBackoff
	for i := 0; i < attempt && delay < a2aCardMaxBackoff; i++ {
		delay *= 2
	}
	return min(delay, a2aCardMaxBackoff)
}

// a2aCardHash is SHA-256 over the card re-encoded with sorted keys, so reformatting is not a change.
func a2aCardHash(card json.RawMessage) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(card))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return "", fmt.Errorf("failed to hash agent card: %w", err)
	}
	canonical, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("failed to hash agent card: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}
