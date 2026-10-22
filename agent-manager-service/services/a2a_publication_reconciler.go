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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
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
	// a2aPublicationAttemptTimeout bounds one row's OpenChoreo reads, origin card fetch and gateway writes.
	a2aPublicationAttemptTimeout = 30 * time.Second
	// a2aPublicationBatchBudget leaves the claim lease a minute to record the last outcome.
	a2aPublicationBatchBudget = repositories.A2APublicationClaimLease - time.Minute
)

// errUpstreamNotReady means the binding has not published a ServiceURL yet. It
// is the expected condition until the first build or deploy binds, not a
// misconfiguration.
var errUpstreamNotReady = newA2APublicError("release binding has not published a service URL yet", nil)

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
	tx              TxRunner
	pubRepo         repositories.A2APublicationRepository
	cardRepo        repositories.A2AAgentCardRepository
	proxyRepo       repositories.A2AProxyRepository
	deploymentRepo  repositories.DeploymentRepository
	gatewayRepo     repositories.GatewayRepository
	agentConfigRepo repositories.AgentConfigRepository
	ocClient        client.OpenChoreoClient
	fetcher         A2ACardFetcher
	encryptionKey   []byte
	events          *GatewayEventsService
	logger          *slog.Logger
	stopCh          chan struct{}
	stopOnce        sync.Once
	// driftCursor is the last published row id the drift scan checked.
	driftCursor    uuid.UUID
	attemptTimeout time.Duration
	batchBudget    time.Duration
}

// NewA2APublicationReconcilerService creates an A2APublicationReconcilerService.
func NewA2APublicationReconcilerService(
	tx TxRunner,
	pubRepo repositories.A2APublicationRepository,
	cardRepo repositories.A2AAgentCardRepository,
	proxyRepo repositories.A2AProxyRepository,
	deploymentRepo repositories.DeploymentRepository,
	gatewayRepo repositories.GatewayRepository,
	agentConfigRepo repositories.AgentConfigRepository,
	ocClient client.OpenChoreoClient,
	fetcher A2ACardFetcher,
	encryptionKey []byte,
	events *GatewayEventsService,
	logger *slog.Logger,
) A2APublicationReconcilerService {
	return &a2aPublicationReconcilerService{
		tx:              tx,
		pubRepo:         pubRepo,
		cardRepo:        cardRepo,
		proxyRepo:       proxyRepo,
		deploymentRepo:  deploymentRepo,
		gatewayRepo:     gatewayRepo,
		agentConfigRepo: agentConfigRepo,
		ocClient:        ocClient,
		fetcher:         fetcher,
		encryptionKey:   encryptionKey,
		events:          events,
		logger:          logger,
		stopCh:          make(chan struct{}),
		stopOnce:        sync.Once{},
		driftCursor:     uuid.Nil,
		attemptTimeout:  a2aPublicationAttemptTimeout,
		batchBudget:     a2aPublicationBatchBudget,
	}
}

func (s *a2aPublicationReconcilerService) Start(ctx context.Context) error {
	go func() {
		s.requeueFailedExternal(ctx)
		s.runLoop(ctx)
	}()
	s.logger.Info("A2A publication reconciler started")
	return nil
}

// requeueFailedExternal gives every failed external row its budget back before the first tick, off the startup path.
// It is deliberately blanket: during a rolling upgrade, pods from before the external source existed claim those rows,
// find no release binding and fail them with nothing to tell them apart, and a restart retrying a proxied agent's
// exhausted publication is what a refresh would do anyway.
func (s *a2aPublicationReconcilerService) requeueFailedExternal(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, s.attemptTimeout)
	defer cancel()
	n, err := s.pubRepo.RequeueFailedExternal(ctx)
	if err != nil {
		s.logger.Error("Failed to requeue failed external A2A publications at start; refresh their proxies to retry", "error", err)
		return
	}
	if n > 0 {
		s.logger.Info("Requeued failed external A2A publications at start", "count", n)
	}
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
	claimedAt := time.Now()
	due, err := s.pubRepo.ClaimDue(ctx, claimedAt, a2aReconcilerBatch)
	if err != nil {
		s.logger.Error("Failed to claim due A2A publications", "error", err)
		return
	}
	s.publishBatch(ctx, claimedAt, due)
	s.checkUpstreamDrift(ctx)
}

// publishBatch stops before the claim lease runs out, so another replica never re-claims a row mid-publish.
func (s *a2aPublicationReconcilerService) publishBatch(ctx context.Context, claimedAt time.Time, due []models.A2APublication) {
	drainWithinLease(ctx, s.logger, claimedAt.Add(s.batchBudget), due, "A2A publication", s.publishOne)
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
		pubCtx := actingAsOrg(ctx, pub)
		rollout, err := s.ocClient.GetReleaseBindingRollout(pubCtx, pub.OUID, pub.AgentName, pub.EnvironmentName)
		if err != nil {
			continue
		}
		current := rollout.ServiceURL
		if current == "" || current == pub.PublishedUpstreamURL {
			s.refetchCardOfNewRelease(pubCtx, pub, rollout)
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

// actingAsOrg carries the publication's org the way a request would. The
// OpenChoreo client stamps its impersonation header from the context's resolved
// org, which only the HTTP middleware sets, so without this a reconciler call
// reaches OpenChoreo with no org and a platform that requires one rejects it.
func actingAsOrg(ctx context.Context, pub models.A2APublication) context.Context {
	return orgctx.WithResolvedOrg(ctx, orgctx.ResolvedOrg{OUID: pub.OUID})
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
		card.ReleaseName == rollout.ReleaseName || card.AttemptedReleaseName == rollout.ReleaseName {
		return
	}
	s.logger.Info("A2A agent serves a new release; re-fetching its card",
		"agentName", pub.AgentName, "environment", pub.EnvironmentName,
		"cardRelease", card.ReleaseName, "currentRelease", rollout.ReleaseName)
	s.enqueueCardFetch(ctx, pub)
}

// publishOne emits one agent-environment pair's Agent resource, or schedules a retry when it cannot yet.
func (s *a2aPublicationReconcilerService) publishOne(ctx context.Context, pub models.A2APublication) {
	ctx = actingAsOrg(ctx, pub)
	attemptCtx, ctx, cancel := a2aAttemptContexts(ctx, s.attemptTimeout)
	defer cancel()
	published, err := s.attemptPublish(attemptCtx, pub)
	if err != nil {
		s.recordAttemptFailure(ctx, pub, err)
		return
	}
	transports, err := json.Marshal(published.transports)
	if err != nil {
		s.recordAttemptFailure(ctx, pub, fmt.Errorf("failed to encode the published transports: %w", err))
		return
	}
	err = s.pubRepo.MarkPublished(ctx, pub, published.upstreamURL, transports)
	switch {
	case errors.Is(err, repositories.ErrA2APublicationSuperseded):
		s.logSuperseded(pub)
		s.withdrawIfTornDown(ctx, pub, published.gatewayID)
	case err != nil:
		s.logger.Error("Published A2A agent but failed to mark the queue row",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName, "error", err)
	default:
		s.enqueueCardFetch(ctx, pub)
	}
}

// withdrawIfTornDown takes back an Agent published after its environment was torn down mid-attempt.
func (s *a2aPublicationReconcilerService) withdrawIfTornDown(ctx context.Context, pub models.A2APublication, gatewayID string) {
	_, err := s.pubRepo.Get(ctx, pub.OUID, pub.ProjectName, pub.AgentName, pub.EnvironmentUUID)
	switch {
	case err == nil:
		return
	case !errors.Is(err, repositories.ErrA2APublicationNotFound):
		s.logger.Error("Failed to check whether a superseded A2A publication was torn down; its Agent may stay served",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName, "artifactID", pub.ArtifactUUID, "error", err)
		return
	}
	s.logger.Info("A2A publication was torn down during the attempt; withdrawing the Agent it published",
		"agentName", pub.AgentName, "environment", pub.EnvironmentName)
	if err := s.deploymentRepo.DeleteForArtifact(ctx, pub.ArtifactUUID, pub.OUID); err != nil {
		s.logger.Error("Failed to delete the deployment of a torn-down A2A publication",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName, "error", err)
	}
	// Undeploy: a switch to direct keeps the artifact, and with it the keys a switch back reuses.
	sendA2AAgentWithdrawal(ctx, s.events, pub.ArtifactUUID, []string{gatewayID}, true, s.logger)
}

// enqueueCardFetch queues a fetch of the card the gateway now serves; best effort.
func (s *a2aPublicationReconcilerService) enqueueCardFetch(ctx context.Context, pub models.A2APublication) {
	if pub.Source == models.A2APublicationSourceExternal {
		s.requeueExternalCard(ctx, pub)
		return
	}
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

// requeueExternalCard makes the parked external row due in place; an upsert would rewrite its registered source.
func (s *a2aPublicationReconcilerService) requeueExternalCard(ctx context.Context, pub models.A2APublication) {
	err := s.cardRepo.Requeue(ctx, pub.OUID, pub.ProjectName, pub.AgentName, pub.EnvironmentName)
	switch {
	case errors.Is(err, repositories.ErrA2AAgentCardNotFound):
		s.logger.Info("Published A2A agent has no card row any more; skipping its card fetch",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName)
		return
	case err != nil:
		s.logger.Warn("Published A2A agent but failed to queue its card fetch",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName, "error", err)
		return
	}
	s.dropCardIfAgentGone(ctx, pub)
}

// releaseParkedCard reports a publish that will not happen on the card parked for it, instead of leaving it pending.
func (s *a2aPublicationReconcilerService) releaseParkedCard(ctx context.Context, pub models.A2APublication, lastErr string) {
	err := s.cardRepo.FailParked(ctx, pub.OUID, pub.ProjectName, pub.AgentName, pub.EnvironmentUUID,
		sanitizeA2ALastError("the gateway could not publish the agent: "+lastErr))
	if err != nil {
		s.logger.Warn("Failed to report the gateway failure on the parked agent card",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName, "error", err)
	}
}

// dropCardIfAgentGone removes a card row written after a racing DeleteAgent, even if the name was reused.
func (s *a2aPublicationReconcilerService) dropCardIfAgentGone(ctx context.Context, pub models.A2APublication) {
	current, err := s.ocClient.GetComponent(ctx, pub.OUID, pub.ProjectName, pub.AgentName)
	switch {
	case errors.Is(err, utils.ErrNotFound):
	case err != nil:
		return
	case pub.AgentUUID == nil || current.UUID == "" || current.UUID == pub.AgentUUID.String():
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

// recordAttemptFailure waits out a not-ready binding, fails an unproxyable row at once, and retries the rest within the budget.
func (s *a2aPublicationReconcilerService) recordAttemptFailure(ctx context.Context, pub models.A2APublication, cause error) {
	if errors.Is(cause, errUpstreamNotReady) {
		s.recordWaiting(ctx, pub, cause)
		return
	}
	lastErr := userFacingA2APublishError(cause)
	warnIfInternalA2AError(s.logger, lastErr, a2aPublishInternalError, cause, "A2A publication failed internally",
		"agentName", pub.AgentName, "environment", pub.EnvironmentName)
	if errors.Is(cause, errA2ANotProxyable) || pub.AttemptCount+1 >= a2aPublicationAttemptBudget {
		s.logger.Error("A2A agent cannot reach its gateway; giving up",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName,
			"attempts", pub.AttemptCount+1, "error", a2aLogError(cause))
		err := s.pubRepo.MarkFailed(ctx, pub, lastErr)
		switch {
		case errors.Is(err, repositories.ErrA2APublicationSuperseded):
			s.logSuperseded(pub)
		case err != nil:
			s.logger.Error("Failed to mark A2A publication failed", "error", err)
		case pub.Source == models.A2APublicationSourceExternal:
			s.releaseParkedCard(ctx, pub, lastErr)
		}
		return
	}
	s.logger.Debug("A2A agent not publishable yet, will retry",
		"agentName", pub.AgentName, "environment", pub.EnvironmentName,
		"attempt", pub.AttemptCount+1, "reason", a2aLogError(cause))
	err := s.pubRepo.MarkAttemptFailed(ctx, pub, lastErr, time.Now().Add(a2aReconcilerRetryIn))
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
	err := s.pubRepo.MarkWaiting(ctx, pub, userFacingA2APublishError(cause), time.Now().Add(a2aReconcilerRetryIn))
	switch {
	case errors.Is(err, repositories.ErrA2APublicationSuperseded):
		s.logSuperseded(pub)
	case err != nil:
		s.logger.Error("Failed to schedule A2A publication retry", "error", err)
	}
}

// userFacingA2APublishError is the last_error a reader may see; unproxyable reasons come from the card itself.
func userFacingA2APublishError(cause error) string {
	if errors.Is(cause, errA2ANotProxyable) {
		return sanitizeA2ALastError(cause.Error())
	}
	return userFacingA2AError(cause, a2aPublishInternalError)
}

// resolvedUpstream is where the gateway forwards to; nil transports keep M1's /rpc and /rest.
type resolvedUpstream struct {
	url        string
	transports []A2ATransport
	auth       *models.UpstreamAuth
}

// a2aPublishResult is where an attempt published the Agent and the transports its route binds.
type a2aPublishResult struct {
	upstreamURL string
	gatewayID   string
	transports  []A2ATransport
}

// a2aOrigin is a proxied external agent's registered card URL and the stored credential to fetch it with.
type a2aOrigin struct {
	cardURL string
	header  *A2ACardFetchHeader
	auth    *models.UpstreamAuth
}

// resolveUpstream reads a platform agent's release binding, or returns the origin a proxied external agent's is derived from.
func (s *a2aPublicationReconcilerService) resolveUpstream(ctx context.Context, pub models.A2APublication) (resolvedUpstream, *a2aOrigin, error) {
	if pub.Source == models.A2APublicationSourceExternal {
		origin, err := s.resolveExternalOrigin(ctx, pub)
		return resolvedUpstream{}, origin, err
	}
	upstreamURL, err := s.ocClient.GetReleaseBindingServiceURL(ctx, pub.OUID, pub.AgentName, pub.EnvironmentName)
	if err != nil {
		return resolvedUpstream{}, nil, fmt.Errorf("failed to read release binding service URL: %w", err)
	}
	if upstreamURL == "" {
		return resolvedUpstream{}, nil, errUpstreamNotReady
	}
	return resolvedUpstream{url: upstreamURL}, nil, nil
}

// resolveExternalOrigin reads the proxy and the registered card URL, refusing one the gateway cannot front.
func (s *a2aPublicationReconcilerService) resolveExternalOrigin(ctx context.Context, pub models.A2APublication) (*a2aOrigin, error) {
	proxy, card, err := s.lockedProxyAndCard(ctx, pub)
	if err != nil {
		return nil, err
	}
	if _, err := a2aProxyUpstream(card.SourceURL); err != nil {
		return nil, err
	}
	header, err := s.originCardHeader(proxy)
	if err != nil {
		return nil, err
	}
	return &a2aOrigin{cardURL: card.SourceURL, header: header, auth: proxy.UpstreamAuth()}, nil
}

// lockedProxyAndCard reads both rows under the environment's transition lock, so a credential reset and a card URL
// move cannot interleave the reads and pair the old secret with the new origin.
func (s *a2aPublicationReconcilerService) lockedProxyAndCard(
	ctx context.Context, pub models.A2APublication,
) (proxy *models.A2AProxy, card *models.A2AAgentCard, err error) {
	key := a2aPublicationKey{
		OUID: pub.OUID, ProjectName: pub.ProjectName, AgentName: pub.AgentName,
		EnvironmentName: pub.EnvironmentName, EnvironmentUUID: pub.EnvironmentUUID,
	}
	err = s.tx(ctx, func(tx *gorm.DB) error {
		proxy, err = lockedA2AProxy(ctx, s.proxyRepo.WithTx(tx), key)
		if err != nil {
			return err
		}
		if proxy == nil {
			return fmt.Errorf("%w: the environment is no longer proxied", errA2ANotProxyable)
		}
		card, err = s.cardRepo.WithTx(tx).Get(ctx, pub.OUID, pub.ProjectName, pub.AgentName, pub.EnvironmentName)
		if errors.Is(err, repositories.ErrA2AAgentCardNotFound) {
			return fmt.Errorf("%w: no card URL is registered for the environment", errA2ANotProxyable)
		}
		if err != nil {
			return fmt.Errorf("failed to read the agent card: %w", err)
		}
		return nil
	})
	return proxy, card, err
}

// fetchExternalUpstream re-derives the routes from the origin card at every publish, fetched SSRF-guarded as its host may resolve elsewhere now.
func (s *a2aPublicationReconcilerService) fetchExternalUpstream(ctx context.Context, origin *a2aOrigin) (resolvedUpstream, error) {
	body, err := s.fetcher.FetchOrigin(ctx, origin.cardURL, origin.header)
	if err != nil {
		return resolvedUpstream{}, prefixA2APublicError("could not fetch the origin agent card", err)
	}
	upstreamURL, transports, err := deriveA2AProxyRoutes(origin.cardURL, body)
	if err != nil {
		return resolvedUpstream{}, err
	}
	return resolvedUpstream{url: upstreamURL, transports: transports, auth: origin.auth}, nil
}

// originCardHeader is the stored upstream credential as a fetch header; sameOriginClient refuses off-origin redirects while it is attached.
func (s *a2aPublicationReconcilerService) originCardHeader(proxy *models.A2AProxy) (*A2ACardFetchHeader, error) {
	if proxy.UpstreamAuthHeader == "" || proxy.UpstreamAuthSecret == "" {
		return nil, nil //nolint:nilnil // A proxy without a stored credential fetches the card unauthenticated.
	}
	value, err := utils.DecryptSecretRef(proxy.UpstreamAuthSecret, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt the upstream secret: %w", err)
	}
	return &A2ACardFetchHeader{Name: proxy.UpstreamAuthHeader, Value: value}, nil
}

// attemptPublish emits the Agent resource and reports where it went.
func (s *a2aPublicationReconcilerService) attemptPublish(ctx context.Context, pub models.A2APublication) (a2aPublishResult, error) {
	upstream, origin, err := s.resolveUpstream(ctx, pub)
	if err != nil {
		return a2aPublishResult{}, err
	}
	gateway, err := resolveIngressGateway(s.gatewayRepo, pub.OUID, pub.EnvironmentUUID)
	if err != nil {
		return a2aPublishResult{}, err
	}

	// Policies are the per-environment config as a chat/custom agent gets them, plus the A2A version header; the card route has its own.
	cfg, err := s.agentConfigRepo.Get(ctx, pub.OUID, pub.ProjectName, pub.AgentName, pub.EnvironmentName)
	if err != nil {
		return a2aPublishResult{}, fmt.Errorf("failed to load agent config: %w", err)
	}
	// The third-party origin is only called once every local lookup has succeeded.
	if origin != nil {
		if upstream, err = s.fetchExternalUpstream(ctx, origin); err != nil {
			return a2aPublishResult{}, err
		}
	}
	apiCfg := withA2AVersionHeader(resolveAPIConfig(cfg, nil, nil, nil, nil, false))
	if origin != nil {
		// A third-party origin never receives the caller's token, whatever the saved config says.
		apiCfg.OAuthForwardToken = false
	}
	policies := buildPolicies(apiCfg)
	transports := a2aEffectiveTransports(upstream.transports)

	yamlStr, err := generateA2AAgentDeploymentYAML(A2AAgentDeploymentInput{
		ArtifactName: a2aAgentEnvArtifactName(pub.ProjectName, pub.AgentName, pub.EnvironmentUUID.String()),
		DisplayName:  pub.AgentName,
		AgentName:    pub.AgentName,
		UpstreamURL:  upstream.url,
		Transports:   transports,
		UpstreamAuth: upstream.auth,
		Policies:     policies,
		CardPolicies: buildCardPolicies(cfg.EffectiveCardCORS()),
	})
	if err != nil {
		return a2aPublishResult{}, err
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
	// The gateway fetches the Agent YAML from this row after the event, so the row must exist first.
	if err := s.deploymentRepo.CreateWithLimitEnforcement(deployment, maxDeploymentsPerAPI+deploymentLimitBuffer); err != nil {
		return a2aPublishResult{}, fmt.Errorf("failed to create A2A agent deployment row: %w", err)
	}

	event := &models.AgentDeploymentEvent{
		AgentID:      pub.ArtifactUUID.String(),
		DeploymentID: deploymentID.String(),
		PerformedAt:  time.Now().Truncate(time.Millisecond),
	}
	if err := s.events.BroadcastAgentDeploymentEvent(gateway.UUID.String(), event); err != nil {
		return a2aPublishResult{}, fmt.Errorf("failed to broadcast agent deployment event: %w", err)
	}

	s.logger.Info("Published A2A agent to gateway",
		"agentName", pub.AgentName, "environment", pub.EnvironmentName,
		"artifactID", pub.ArtifactUUID, "gateway", gateway.Name, "upstream", upstream.url)
	return a2aPublishResult{upstreamURL: upstream.url, gatewayID: gateway.UUID.String(), transports: transports}, nil
}
