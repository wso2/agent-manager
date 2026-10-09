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
	// a2aCardLastErrorMaxRunes keeps last_error readable in the console.
	a2aCardLastErrorMaxRunes = 1024
	// a2aCardUnrecordableError replaces a cause the store refused to record.
	a2aCardUnrecordableError = "agent card fetch failed; the error could not be recorded"
)

// errA2ACardEndpointNotReady means the agent's environment has no public endpoint URL yet.
var errA2ACardEndpointNotReady = errors.New("agent has no public endpoint in this environment yet")

// errA2ACardRolloutInProgress means the agent's latest release is not serving in this environment yet.
var errA2ACardRolloutInProgress = errors.New("agent is still rolling out in this environment")

// A2ACardReconcilerService drains the a2a_agent_cards fetch queue.
type A2ACardReconcilerService interface {
	Start(ctx context.Context) error
	Stop() error
	// RunOnce drains the currently-due batch once.
	RunOnce(ctx context.Context)
}

type a2aCardReconcilerService struct {
	cardRepo repositories.A2AAgentCardRepository
	fetcher  A2ACardFetcher
	ocClient client.OpenChoreoClient
	logger   *slog.Logger
	stopCh   chan struct{}
	stopOnce sync.Once

	attemptTimeout time.Duration
	batchBudget    time.Duration
}

// NewA2ACardReconcilerService creates an A2ACardReconcilerService.
func NewA2ACardReconcilerService(
	cardRepo repositories.A2AAgentCardRepository,
	fetcher A2ACardFetcher,
	ocClient client.OpenChoreoClient,
	logger *slog.Logger,
) A2ACardReconcilerService {
	return &a2aCardReconcilerService{
		cardRepo: cardRepo,
		fetcher:  fetcher,
		ocClient: ocClient,
		logger:   logger,
		stopCh:   make(chan struct{}),
		stopOnce: sync.Once{},

		attemptTimeout: a2aCardAttemptTimeout,
		batchBudget:    a2aCardBatchBudget,
	}
}

func (s *a2aCardReconcilerService) Start(ctx context.Context) error {
	go s.runLoop(ctx)
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
	claimedAt := time.Now()
	due, err := s.cardRepo.ClaimDue(ctx, claimedAt, a2aCardBatch)
	if err != nil {
		s.logger.Error("Failed to claim due A2A agent cards", "error", err)
		return
	}
	batchCtx, cancel := context.WithDeadline(ctx, claimedAt.Add(s.batchBudget))
	defer cancel()
	for i, row := range due {
		if batchCtx.Err() != nil {
			s.logger.Info("A2A card batch ran out of lease; leaving the rest for a later claim", "remaining", len(due)-i)
			return
		}
		s.fetchOne(batchCtx, row)
	}
}

// fetchOne fetches one row's card and records the outcome.
func (s *a2aCardReconcilerService) fetchOne(ctx context.Context, row models.A2AAgentCard) {
	attemptCtx, cancel := context.WithTimeout(ctx, s.attemptTimeout)
	defer cancel()
	// The outcome must be recorded even when the attempt ran out of time.
	ctx = context.WithoutCancel(ctx)
	fetched, err := s.attemptFetch(attemptCtx, row)
	if err != nil {
		s.recordAttemptFailure(ctx, row, err)
		return
	}
	hash, err := a2aCardHash(fetched.body)
	if err != nil {
		s.recordAttemptFailure(ctx, row, err)
		return
	}
	err = s.cardRepo.MarkFetched(ctx, row, fetched.body, hash, fetched.url, fetched.releaseName)
	switch {
	case errors.Is(err, repositories.ErrA2AAgentCardSuperseded):
		s.logSuperseded(row)
	case err != nil:
		s.recordAttemptFailure(ctx, row, fmt.Errorf("failed to store agent card: %w", err))
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
		body, err := s.fetcher.Fetch(ctx, row.SourceURL, true)
		return a2aCardFetch{url: row.SourceURL, body: body}, err
	case models.A2AAgentCardSourcePlatform:
		return s.fetchPlatformCard(ctx, row)
	default:
		return a2aCardFetch{}, fmt.Errorf("unknown agent card source %q", row.Source)
	}
}

// fetchPlatformCard waits out a rollout first, since until it finishes the old release can still answer.
func (s *a2aCardReconcilerService) fetchPlatformCard(ctx context.Context, row models.A2AAgentCard) (a2aCardFetch, error) {
	rollout, err := s.ocClient.GetReleaseBindingRollout(ctx, row.OUID, row.AgentName, row.EnvironmentName)
	if err != nil {
		return a2aCardFetch{}, fmt.Errorf("failed to read agent rollout: %w", err)
	}
	if !rollout.Serving {
		return a2aCardFetch{}, errA2ACardRolloutInProgress
	}
	if rollout.ExternalURL == "" {
		return a2aCardFetch{}, errA2ACardEndpointNotReady
	}
	url := strings.TrimRight(rollout.ExternalURL, "/") + a2aAgentCardPath
	body, err := s.fetcher.Fetch(ctx, url, false)
	if err != nil {
		// The derived URL is otherwise invisible to the user.
		return a2aCardFetch{url: url}, fmt.Errorf("%s: %w", url, err)
	}
	return a2aCardFetch{url: url, releaseName: rollout.ReleaseName, body: body}, nil
}

func (s *a2aCardReconcilerService) recordAttemptFailure(ctx context.Context, row models.A2AAgentCard, cause error) {
	lastErr := sanitizeA2ACardError(cause.Error())
	err := s.markAttemptFailed(ctx, row, cause, lastErr)
	if err != nil && !errors.Is(err, repositories.ErrA2AAgentCardSuperseded) {
		s.logger.Error("Failed to record A2A agent card attempt; retrying with a fixed message",
			"agentName", row.AgentName, "error", err)
		err = s.markAttemptFailed(ctx, row, cause, a2aCardUnrecordableError)
	}
	switch {
	case errors.Is(err, repositories.ErrA2AAgentCardSuperseded):
		s.logSuperseded(row)
	case err != nil:
		s.logger.Error("Failed to record A2A agent card attempt", "agentName", row.AgentName, "error", err)
	}
}

// markAttemptFailed charges one attempt, failing the row once the budget is spent.
func (s *a2aCardReconcilerService) markAttemptFailed(ctx context.Context, row models.A2AAgentCard, cause error, lastErr string) error {
	if row.AttemptCount+1 >= a2aCardAttemptBudget {
		s.logger.Warn("A2A agent card could not be fetched within the attempt budget",
			"agentName", row.AgentName, "environment", row.EnvironmentName, "error", cause)
		return s.cardRepo.MarkFailed(ctx, row, lastErr)
	}
	s.logger.Debug("A2A agent card not fetchable yet, will retry",
		"agentName", row.AgentName, "environment", row.EnvironmentName,
		"attempt", row.AttemptCount+1, "reason", cause)
	return s.cardRepo.MarkAttemptFailed(ctx, row, lastErr, time.Now().Add(a2aCardRetryIn(row.AttemptCount)))
}

// sanitizeA2ACardError makes msg storable in a Postgres TEXT column and bounded in length.
func sanitizeA2ACardError(msg string) string {
	msg = strings.ReplaceAll(strings.ToValidUTF8(msg, "\uFFFD"), "\x00", "")
	if runes := []rune(msg); len(runes) > a2aCardLastErrorMaxRunes {
		msg = string(runes[:a2aCardLastErrorMaxRunes-1]) + "…"
	}
	return msg
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
