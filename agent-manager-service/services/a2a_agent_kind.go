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
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"gorm.io/gorm"
)

// a2aAgentEnv is the agent and environment a per-environment Agent artifact exposes.
type a2aAgentEnv struct {
	ouID, projectName, agentName, environmentName string
	environmentUUID                               uuid.UUID
}

// A2AAgentKind exposes A2A agents through their environment's ingress gateway. A platform agent is recorded in
// platform_agent_exposures, an external agent in a2a_proxies (joined to its artifact on the handle). Its Render
// reads the platform store, or fetches the external origin's card, so only the worker renders it.
type A2AAgentKind struct {
	tx              TxRunner
	exposures       *PlatformAgentExposures
	proxyRepo       repositories.A2AProxyRepository
	cardRepo        repositories.A2AAgentCardRepository
	deploymentRepo  repositories.DeploymentRepository
	agentConfigRepo repositories.AgentConfigRepository
	ocClient        repositories.PlatformRepository
	fetcher         A2ACardFetcher
	apiKeyRepo      repositories.APIKeyRepository
	events          *GatewayEventsService
	encryptionKey   []byte
	logger          *slog.Logger
}

// NewA2AAgentKind builds the A2A agent kind.
func NewA2AAgentKind(
	tx TxRunner,
	exposures *PlatformAgentExposures,
	proxyRepo repositories.A2AProxyRepository,
	cardRepo repositories.A2AAgentCardRepository,
	deploymentRepo repositories.DeploymentRepository,
	agentConfigRepo repositories.AgentConfigRepository,
	ocClient repositories.PlatformRepository,
	fetcher A2ACardFetcher,
	apiKeyRepo repositories.APIKeyRepository,
	events *GatewayEventsService,
	encryptionKey []byte,
	logger *slog.Logger,
) *A2AAgentKind {
	return &A2AAgentKind{
		tx: tx, exposures: exposures, proxyRepo: proxyRepo, cardRepo: cardRepo,
		deploymentRepo: deploymentRepo, agentConfigRepo: agentConfigRepo, ocClient: ocClient, fetcher: fetcher,
		apiKeyRepo: apiKeyRepo, events: events, encryptionKey: encryptionKey, logger: logger,
	}
}

func (k *A2AAgentKind) Name() string        { return models.KindAgent }
func (k *A2AAgentKind) RendersInline() bool { return false }

func (k *A2AAgentKind) GatewayArtifactID(artifact models.ArtifactRef) string {
	return artifact.ArtifactUUID.String()
}

// DescribeRenderError is the reason a user is shown: the public part of the error, never internals.
func (k *A2AAgentKind) DescribeRenderError(err error) string {
	return userFacingA2APublishError(err)
}

func userFacingA2APublishError(cause error) string {
	if errors.Is(cause, errA2ANotProxyable) {
		return sanitizeA2ALastError(cause.Error())
	}
	return userFacingA2AError(cause, a2aPublishInternalError)
}

// resolvedUpstream is where the gateway sends an agent's traffic.
type resolvedUpstream struct {
	url        string
	transports []A2ATransport
	auth       *models.UpstreamAuth
}

// Render finds the artifact's exposure, else its proxy; neither means the record is gone.
func (k *A2AAgentKind) Render(ctx context.Context, artifact models.ArtifactRef) (reconcile.Rendered, error) {
	ctx = orgctx.WithResolvedOrg(ctx, orgctx.ResolvedOrg{OUID: artifact.OUID})
	exposure, err := k.exposures.exposure(ctx, nil, artifact)
	switch {
	case err == nil:
		return k.renderPlatform(ctx, artifact, exposure)
	case !errors.Is(err, repositories.ErrPlatformAgentExposureNotFound):
		return reconcile.Rendered{}, err
	}
	proxy, err := k.proxyRepo.GetByArtifact(ctx, artifact.OUID, artifact.ArtifactUUID)
	switch {
	case errors.Is(err, repositories.ErrA2AProxyNotFound):
		return reconcile.Rendered{}, reconcile.ErrRecordGone
	case err != nil:
		return reconcile.Rendered{}, fmt.Errorf("failed to read the A2A proxy: %w", err)
	}
	return k.renderExternal(ctx, artifact, proxy)
}

func (k *A2AAgentKind) renderPlatform(
	ctx context.Context, artifact models.ArtifactRef, exposure *models.PlatformAgentExposure,
) (reconcile.Rendered, error) {
	url, err := k.exposures.serviceURL(ctx, exposure)
	if err != nil {
		return reconcile.Rendered{}, err
	}
	return k.render(ctx, artifact, exposureEnv(exposure), resolvedUpstream{url: url}, false)
}

func (k *A2AAgentKind) renderExternal(ctx context.Context, artifact models.ArtifactRef, proxy *models.A2AProxy) (reconcile.Rendered, error) {
	env := proxyEnv(proxy)
	origin, err := k.externalOrigin(ctx, env)
	if errors.Is(err, errA2AProxyGone) {
		return reconcile.Rendered{}, reconcile.ErrRecordGone
	}
	if err != nil {
		return reconcile.Rendered{}, err
	}
	body, err := k.fetcher.FetchOrigin(ctx, origin.cardURL, origin.header)
	if err != nil {
		return reconcile.Rendered{}, prefixA2APublicError("could not fetch the origin agent card", err)
	}
	upstreamURL, transports, err := deriveA2AProxyRoutes(origin.cardURL, body)
	if err != nil {
		return reconcile.Rendered{}, err
	}
	return k.render(ctx, artifact, env, resolvedUpstream{url: upstreamURL, transports: transports, auth: origin.auth}, true)
}

func (k *A2AAgentKind) render(
	ctx context.Context, artifact models.ArtifactRef, env a2aAgentEnv, upstream resolvedUpstream, external bool,
) (reconcile.Rendered, error) {
	cfg, err := k.agentConfigRepo.Get(ctx, env.ouID, env.projectName, env.agentName, env.environmentName)
	if err != nil {
		return reconcile.Rendered{}, fmt.Errorf("failed to load agent config: %w", err)
	}
	apiCfg := withA2AVersionHeader(resolveAPIConfig(cfg, nil, nil, nil, nil, false))
	if external {
		apiCfg.OAuthForwardToken = false
	}
	content, err := generateA2AAgentDeploymentYAML(A2AAgentDeploymentInput{
		ArtifactName: agentGatewayName(env.projectName, env.agentName, env.environmentUUID.String()),
		DisplayName:  env.agentName,
		AgentName:    env.agentName,
		UpstreamURL:  upstream.url,
		Transports:   upstream.transports,
		UpstreamAuth: upstream.auth,
		Policies:     buildPolicies(apiCfg),
		CardPolicies: buildCardPolicies(cfg.EffectiveCardCORS()),
	})
	if err != nil {
		return reconcile.Rendered{}, err
	}
	return renderedDeployment(env.agentName, []byte(content)), nil
}

// errA2AProxyGone is a proxied environment switched to direct while its artifact was being rendered.
var errA2AProxyGone = errors.New("the environment is no longer proxied")

// a2aOrigin is an external agent's origin card and the credential to fetch and call it with.
type a2aOrigin struct {
	cardURL string
	header  *A2ACardFetchHeader
	auth    *models.UpstreamAuth
}

// externalOrigin reads the proxy and its registered card URL under the environment's proxy lock, so a concurrent
// set or switch to direct is either seen whole or not at all.
func (k *A2AAgentKind) externalOrigin(ctx context.Context, env a2aAgentEnv) (*a2aOrigin, error) {
	key := agentExposureKey{
		OUID: env.ouID, ProjectName: env.projectName, AgentName: env.agentName,
		EnvironmentName: env.environmentName, EnvironmentUUID: env.environmentUUID,
	}
	var origin *a2aOrigin
	err := k.tx(ctx, func(tx *gorm.DB) error {
		proxy, err := lockedA2AProxy(ctx, k.proxyRepo.WithTx(tx), key)
		if err != nil {
			return err
		}
		if proxy == nil {
			return errA2AProxyGone
		}
		card, err := k.cardRepo.WithTx(tx).Get(ctx, env.ouID, env.projectName, env.agentName, env.environmentName)
		if errors.Is(err, repositories.ErrA2AAgentCardNotFound) {
			return reconcile.Permanent(fmt.Errorf("%w: no card URL is registered for the environment", errA2ANotProxyable))
		}
		if err != nil {
			return fmt.Errorf("failed to read the agent card: %w", err)
		}
		if _, err := a2aProxyUpstream(card.SourceURL); err != nil {
			return reconcile.Permanent(err)
		}
		header, err := k.originCardHeader(proxy)
		if err != nil {
			return err
		}
		origin = &a2aOrigin{cardURL: card.SourceURL, header: header, auth: proxy.UpstreamAuth()}
		return nil
	})
	return origin, err
}

func (k *A2AAgentKind) originCardHeader(proxy *models.A2AProxy) (*A2ACardFetchHeader, error) {
	if proxy.UpstreamAuthHeader == "" || proxy.UpstreamAuthSecret == "" {
		return nil, nil //nolint:nilnil // A proxy without a stored credential fetches the card unauthenticated.
	}
	value, err := utils.DecryptSecretRef(proxy.UpstreamAuthSecret, k.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt the upstream secret: %w", err)
	}
	return &A2ACardFetchHeader{Name: proxy.UpstreamAuthHeader, Value: value}, nil
}

func exposureEnv(e *models.PlatformAgentExposure) a2aAgentEnv {
	return a2aAgentEnv{
		ouID: e.OUID, projectName: e.ProjectName, agentName: e.AgentName,
		environmentName: e.EnvironmentName, environmentUUID: e.EnvironmentUUID,
	}
}

func proxyEnv(p *models.A2AProxy) a2aAgentEnv {
	return a2aAgentEnv{
		ouID: p.OUID, projectName: p.ProjectName, agentName: p.AgentName,
		environmentName: p.EnvironmentName, environmentUUID: p.EnvironmentUUID,
	}
}

// DeployEvent tells the gateway to fetch /agents/{agentId}, keyed on the artifact UUID.
func (k *A2AAgentKind) DeployEvent(ref reconcile.EventRef) eventhub.Event {
	payload := &models.AgentDeploymentEvent{
		AgentID: ref.ArtifactUUID.String(), DeploymentID: ref.DeploymentID, PerformedAt: ref.PerformedAt,
	}
	return eventhub.NewGatewayEvent("agent.deployed", "CREATE", payload.AgentID, payload)
}

// UndeployEvent stops the gateway serving the Agent but keeps it, and its API keys, for a redeploy.
func (k *A2AAgentKind) UndeployEvent(ref reconcile.EventRef) eventhub.Event {
	payload := &models.AgentUndeploymentEvent{AgentID: ref.ArtifactUUID.String(), PerformedAt: ref.PerformedAt}
	return eventhub.NewGatewayEvent("agent.undeployed", "DELETE", payload.AgentID, payload)
}

// DeleteEvent drops the Agent together with its API keys.
func (k *A2AAgentKind) DeleteEvent(artifact models.ArtifactRef) eventhub.Event {
	payload := &models.AgentDeletionEvent{AgentID: artifact.ArtifactUUID.String()}
	return eventhub.NewGatewayEvent("agent.deleted", "DELETE", payload.AgentID, payload)
}

// UnplacedRecords are the exposures and proxies whose artifact has no row yet,
// in environments with a gateway of the kind's role.
func (k *A2AAgentKind) UnplacedRecords(
	ctx context.Context, ouID string, env *uuid.UUID, afterKey string, limit int,
) ([]reconcile.Record, error) {
	exposed, exposedArgs := unplacedExposureArm(models.AgentExposureProtocolA2A, ouID, env)
	proxied, proxiedArgs := reconcile.UnplacedCondition(reconcile.RecordColumns{
		OUID: "a.org_id", ArtifactUUID: "a.id", EnvironmentUUID: "p.environment_id",
	}, models.IngressGatewayRoles, ouID, env)
	arms := exposed + `
			UNION ALL
			SELECT a.org_id, a.id AS artifact_id, a.handle, a.kind, p.environment_id
			FROM a2a_proxies p ` + repositories.A2AProxyArtifactJoin + `
			WHERE ` + proxied
	records, err := pageUnplacedRecords(ctx, db.DB(ctx), arms, append(exposedArgs, proxiedArgs...), afterKey, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list unplaced A2A agents: %w", err)
	}
	return records, nil
}

// PlacedOn: an exposure or external agent proxy in env whose artifact is placed on gateway.
func (k *A2AAgentKind) PlacedOn(ctx context.Context, tx *gorm.DB, env, gateway uuid.UUID) (bool, error) {
	exposed, exposedArgs := placedExposureArm(models.AgentExposureProtocolA2A, env, gateway)
	proxied, proxiedArgs := reconcile.PlacedCondition(reconcile.RecordColumns{
		OUID: "a.org_id", ArtifactUUID: "a.id", EnvironmentUUID: "p.environment_id",
	}, env, gateway)
	return placedOn(ctx, tx, exposed+`
		UNION ALL
		SELECT 1 AS placed FROM a2a_proxies p `+repositories.A2AProxyArtifactJoin+`
			WHERE `+proxied, append(exposedArgs, proxiedArgs...))
}

// Targets is the environment's ingress gateway.
func (k *A2AAgentKind) Targets(ctx context.Context, record reconcile.Record) (uuid.UUID, bool, error) {
	return k.exposures.target(ctx, record)
}

// AfterLive queues the agent's card fetch once the gateway serves it, then replays its API keys to that gateway.
func (k *A2AAgentKind) AfterLive(ctx context.Context, tx *gorm.DB, p models.Placement, _ uuid.UUID) error {
	if err := k.queueCardFetch(ctx, tx, p); err != nil {
		return err
	}
	return replayAgentKeys(ctx, tx, k.apiKeyRepo, k.events, p, time.Now())
}

// queueCardFetch: a platform card carries the component UUID from the exposure, so the card reconciler can tell a
// replaced agent's card apart; an external card was queued when its URL was registered and is only requeued.
func (k *A2AAgentKind) queueCardFetch(ctx context.Context, tx *gorm.DB, p models.Placement) error {
	exposure, err := k.exposures.exposure(ctx, tx, p.ArtifactRef)
	switch {
	case err == nil:
		return k.cardRepo.WithTx(tx).Enqueue(ctx, &models.A2AAgentCard{
			OUID: exposure.OUID, ProjectName: exposure.ProjectName, AgentName: exposure.AgentName,
			EnvironmentName: exposure.EnvironmentName, EnvironmentUUID: exposure.EnvironmentUUID,
			Source: models.A2AAgentCardSourcePlatform, AgentUUID: exposure.AgentUUID,
		})
	case !errors.Is(err, repositories.ErrPlatformAgentExposureNotFound):
		return err
	}
	proxy, err := k.proxyRepo.WithTx(tx).GetByArtifact(ctx, p.OUID, p.ArtifactUUID)
	if errors.Is(err, repositories.ErrA2AProxyNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	err = k.cardRepo.WithTx(tx).Requeue(ctx, proxy.OUID, proxy.ProjectName, proxy.AgentName, proxy.EnvironmentName)
	if errors.Is(err, repositories.ErrA2AAgentCardNotFound) {
		return nil
	}
	return err
}

// Drifted never reports drift: only platform-hosted agents have a release
// binding whose endpoint can move, and there is no agent runtime.
func (k *A2AAgentKind) Drifted(context.Context, models.Placement) (string, bool, error) {
	return "", false, nil
}
