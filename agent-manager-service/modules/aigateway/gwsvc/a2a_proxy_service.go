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

package gwsvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
)

const (
	a2aUpstreamHeaderMaxLen = 255
	a2aUpstreamValueMaxLen  = 8192
)

// a2aUpstreamHeaderPattern is the gateway's set-headers name schema.
var a2aUpstreamHeaderPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// A2AProxyHooks is what the card service needs from the proxy service on a proxied environment.
type A2AProxyHooks interface {
	// Each decides direct vs proxied under the environment's transition lock.

	// StoreCardSource stores the URL; on a proxied environment it also parks the card and republishes.
	StoreCardSource(ctx context.Context, agent *modkit.Agent, ouID, projectName string, env *modkit.Environment, sourceURL string) error
	// RemoveCardSource deletes the card row; on a proxied environment it also switches to direct.
	RemoveCardSource(ctx context.Context, agent *modkit.Agent, ouID, projectName string, env *modkit.Environment) error
	// RefreshCardSource requeues the card fetch; on a proxied environment it republishes, and the fetch follows.
	RefreshCardSource(ctx context.Context, agent *modkit.Agent, ouID, projectName string, env *modkit.Environment) error
}

// A2AProxyService publishes external agents over A2A through their environment's ingress gateway.
type A2AProxyService interface {
	A2AProxyHooks
	GetA2AProxy(ctx context.Context, ouID, projectName, agentName, envName string) (*gwmodels.A2AProxyStatus, error)
	// SetA2AProxy enables the proxy or updates its upstream auth; a nil auth clears the stored credential.
	SetA2AProxy(ctx context.Context, ouID, projectName, agentName, envName string, auth *gwmodels.A2AProxyAuthUpdate) error
	DeleteA2AProxy(ctx context.Context, ouID, projectName, agentName, envName string) error
	RefreshA2AProxy(ctx context.Context, ouID, projectName, agentName, envName string) error
}

type a2aProxyService struct {
	perms           *Permissions
	tx              TxRunner
	proxyRepo       gwrepo.A2AProxyRepository
	cardRepo        gwrepo.A2AAgentCardRepository
	agentConfigRepo gwrepo.AgentConfigRepository
	artifactRepo    gwrepo.ArtifactRepository
	publisher       *A2APublisher
	ocClient        modkit.CoreAPI
	logger          *slog.Logger
}

// NewA2AProxyService creates an A2AProxyService.
func NewA2AProxyService(
	tx TxRunner,
	proxyRepo gwrepo.A2AProxyRepository,
	cardRepo gwrepo.A2AAgentCardRepository,
	agentConfigRepo gwrepo.AgentConfigRepository,
	artifactRepo gwrepo.ArtifactRepository,
	publisher *A2APublisher,
	ocClient modkit.CoreAPI,
	logger *slog.Logger,
	perms *Permissions,
) A2AProxyService {
	return &a2aProxyService{
		perms:           perms,
		tx:              tx,
		proxyRepo:       proxyRepo,
		cardRepo:        cardRepo,
		agentConfigRepo: agentConfigRepo,
		artifactRepo:    artifactRepo,
		publisher:       publisher,
		ocClient:        ocClient,
		logger:          logger,
	}
}

// a2aProxyTarget is the agent and environment one proxy operation acts on.
type a2aProxyTarget struct {
	agent *modkit.Agent
	env   *modkit.Environment
	key   agentExposureKey
}

func newA2AProxyTarget(agent *modkit.Agent, ouID, projectName string, env *modkit.Environment) (a2aProxyTarget, error) {
	envUUID, err := uuid.Parse(env.ID)
	if err != nil {
		return a2aProxyTarget{}, fmt.Errorf("invalid UUID for environment %s: %w", env.Name, err)
	}
	key := agentExposureKey{
		OUID: ouID, ProjectName: projectName, AgentName: agent.Name, EnvironmentName: env.Name, EnvironmentUUID: envUUID,
		AgentUUID: parseAgentUUID(agent.ID),
	}
	return a2aProxyTarget{agent: agent, env: env, key: key}, nil
}

// cardRow is the environment's external card row with sourceURL registered.
func (t a2aProxyTarget) cardRow(sourceURL string) *gwmodels.A2AAgentCard {
	return &gwmodels.A2AAgentCard{
		OUID: t.key.OUID, ProjectName: t.key.ProjectName, AgentName: t.key.AgentName,
		EnvironmentName: t.key.EnvironmentName, EnvironmentUUID: t.key.EnvironmentUUID,
		Source: gwmodels.A2AAgentCardSourceExternal, SourceURL: sourceURL, AgentUUID: t.key.AgentUUID,
	}
}

func (s *a2aProxyService) GetA2AProxy(ctx context.Context, ouID, projectName, agentName, envName string) (*gwmodels.A2AProxyStatus, error) {
	// Resolved like every other operation: a row a failed cleanup left under a re-created name must not answer.
	t, err := s.target(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		return nil, err
	}
	proxy, err := s.proxyRepo.GetByEnvUUID(ctx, t.key.OUID, t.key.ProjectName, t.key.AgentName, t.key.EnvironmentUUID)
	if errors.Is(err, gwrepo.ErrA2AProxyNotFound) {
		return nil, gwerr.ErrEnvironmentNotProxied
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the A2A proxy: %w", err)
	}
	gatewayURL, err := s.publisher.exposure.GatewayURL(t.key.OUID, t.key.EnvironmentUUID, t.key.AgentName)
	if err != nil {
		return nil, err
	}
	shown, err := s.publisher.exposure.ShownStatus(ctx, t.key, gatewayURL != "")
	if err != nil {
		return nil, err
	}
	return &gwmodels.A2AProxyStatus{
		Status:             shown.Status,
		StatusReason:       shown.Reason,
		UpstreamAuthHeader: proxy.UpstreamAuthHeader,
		GatewayURL:         gatewayURL,
	}, nil
}

func (s *a2aProxyService) SetA2AProxy(
	ctx context.Context, ouID, projectName, agentName, envName string, auth *gwmodels.A2AProxyAuthUpdate,
) error {
	if err := validateA2AUpstreamAuth(auth); err != nil {
		return err
	}
	t, err := s.target(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		return err
	}
	// A concurrent first-time setup can create the proxy row between staging
	// and locking, under another ID. Stage again for the row that won; the
	// cleanup job removes the stale staging.
	for range a2aStageAttempts {
		err = s.setA2AProxy(ctx, ouID, projectName, agentName, t, auth)
		if !errors.Is(err, gwrepo.ErrA2AStagedForOtherID) {
			return err
		}
	}
	return err
}

// a2aStageAttempts bounds how often SetA2AProxy stages again after losing a first-time setup race.
const a2aStageAttempts = 3

func (s *a2aProxyService) setA2AProxy(
	ctx context.Context, ouID, projectName, agentName string, t a2aProxyTarget, auth *gwmodels.A2AProxyAuthUpdate,
) error {
	var err error
	var preparedID uuid.UUID
	if auth != nil && auth.Value != nil {
		if preparer, ok := s.proxyRepo.(interface {
			PrepareUpstream(context.Context, string, string, string, uuid.UUID, string) (context.Context, uuid.UUID, error)
		}); ok {
			ctx, preparedID, err = preparer.PrepareUpstream(ctx, ouID, projectName, agentName, t.key.EnvironmentUUID, *auth.Value)
			if err != nil {
				return err
			}
		}
	}
	err = s.tx(ctx, func(tx *gorm.DB) error {
		existing, err := s.lockedProxy(ctx, tx, t)
		if err != nil {
			return err
		}
		if existing != nil && preparedID != uuid.Nil && existing.ID != preparedID {
			return gwrepo.ErrA2AStagedForOtherID
		}
		cardURL, err := s.requireCardSource(ctx, tx, t)
		if err != nil {
			return err
		}
		proxy, authChanged, err := s.proxyRow(t, existing, auth)
		if err != nil {
			return err
		}
		if err := requireHTTPSForSecret(cardURL, proxy.HasUpstreamSecret()); err != nil {
			return err
		}
		// A PUT can set, replace or clear the credential the gateway sends
		// upstream, so its route is fail-closed.
		annotateA2AProxy(ctx, t, proxy.UpstreamAuthHeader, authChanged)
		artifact, err := s.ensureArtifact(tx, t)
		if err != nil {
			return err
		}
		proxy.ID, proxy.ArtifactUUID = preparedID, artifact.ArtifactUUID
		if err := s.proxyRepo.WithTx(tx).Upsert(ctx, proxy); err != nil {
			return fmt.Errorf("failed to save the A2A proxy: %w", err)
		}
		if existing == nil {
			if err := s.switchFromDirect(ctx, tx, t); err != nil {
				return err
			}
		}
		return s.publisher.exposure.DeployRecord(ctx, tx, s.publisher.kind, t.key.EnvironmentUUID, artifact)
	})
	if err == nil {
		s.publisher.exposure.Nudge()
		// The route's record follows the agent-gone check, so it says whether
		// the credential change stayed.
		err = s.dropIfAgentGone(ctx, t)
	}
	return err
}

func (s *a2aProxyService) DeleteA2AProxy(ctx context.Context, ouID, projectName, agentName, envName string) error {
	t, err := s.target(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		return err
	}
	return s.removeProxy(ctx, t, requeueDirectCardFetch, nil)
}

func (s *a2aProxyService) RefreshA2AProxy(ctx context.Context, ouID, projectName, agentName, envName string) error {
	t, err := s.target(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		return err
	}
	return s.republish(ctx, t, nil)
}

func (s *a2aProxyService) StoreCardSource(
	ctx context.Context, agent *modkit.Agent, ouID, projectName string, env *modkit.Environment, sourceURL string,
) error {
	t, err := newA2AProxyTarget(agent, ouID, projectName, env)
	if err != nil {
		return err
	}
	err = s.tx(ctx, func(tx *gorm.DB) error {
		// Decided under the lock, so a concurrent switch to proxied cannot miss the origin check.
		proxy, err := s.lockedProxy(ctx, tx, t)
		if err != nil {
			return err
		}
		if proxy != nil {
			if err := requireProxyableCardURL(sourceURL); err != nil {
				return err
			}
			if err := s.requireSecretStaysOnOrigin(ctx, tx, t, proxy, sourceURL); err != nil {
				return err
			}
		}
		cards := s.cardRepo.WithTx(tx)
		if err := cards.Enqueue(ctx, t.cardRow(sourceURL)); err != nil {
			return fmt.Errorf("failed to store the agent card URL: %w", err)
		}
		if proxy == nil {
			return nil
		}
		// The new URL's card is shown only once the gateway fronts the new origin.
		err = cards.ClearCard(ctx, t.key.OUID, t.key.ProjectName, t.key.AgentName, t.key.EnvironmentUUID)
		if err != nil {
			return fmt.Errorf("failed to park the agent card: %w", err)
		}
		artifact, err := s.ensureArtifact(tx, t)
		if err != nil {
			return err
		}
		return s.publisher.exposure.DeployRecord(ctx, tx, s.publisher.kind, t.key.EnvironmentUUID, artifact)
	})
	if err != nil {
		return err
	}
	s.publisher.exposure.Nudge()
	return s.dropIfAgentGone(ctx, t)
}

func (s *a2aProxyService) RemoveCardSource(
	ctx context.Context, agent *modkit.Agent, ouID, projectName string, env *modkit.Environment,
) error {
	t, err := newA2AProxyTarget(agent, ouID, projectName, env)
	if err != nil {
		return err
	}
	return s.removeProxy(ctx, t, deleteCardRow, deleteCardRow)
}

func (s *a2aProxyService) RefreshCardSource(
	ctx context.Context, agent *modkit.Agent, ouID, projectName string, env *modkit.Environment,
) error {
	t, err := newA2AProxyTarget(agent, ouID, projectName, env)
	if err != nil {
		return err
	}
	return s.republish(ctx, t, requeueCardFetch)
}

// target resolves an external agent and an environment the caller may act on.
func (s *a2aProxyService) target(ctx context.Context, ouID, projectName, agentName, envName string) (a2aProxyTarget, error) {
	agent, err := getExternalAgent(ctx, s.ocClient, ouID, projectName, agentName, gwerr.ErrA2AProxyAgentNotExternal)
	if err != nil {
		return a2aProxyTarget{}, err
	}
	env, err := RequireEnvironmentTier(ctx, s.perms, s.ocClient, s.logger, ouID, envName)
	if err != nil {
		return a2aProxyTarget{}, err
	}
	return newA2AProxyTarget(agent, ouID, projectName, env)
}

// lockedProxy takes the environment's transition lock and reads its proxy row under it; nil while direct.
func (s *a2aProxyService) lockedProxy(ctx context.Context, tx *gorm.DB, t a2aProxyTarget) (*gwmodels.A2AProxy, error) {
	return lockedA2AProxy(ctx, s.proxyRepo.WithTx(tx), t.key)
}

// lockedCard reads the environment's card row under the transition lockedProxy took; nil when it has none.
func (s *a2aProxyService) lockedCard(ctx context.Context, tx *gorm.DB, t a2aProxyTarget) (*gwmodels.A2AAgentCard, error) {
	card, err := s.cardRepo.WithTx(tx).Get(ctx, t.key.OUID, t.key.ProjectName, t.key.AgentName, t.key.EnvironmentName)
	if errors.Is(err, gwrepo.ErrA2AAgentCardNotFound) {
		return nil, nil //nolint:nilnil // No row means no card source is registered.
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the agent card: %w", err)
	}
	return card, nil
}

// requireCardSource returns the environment's registered, proxyable card URL; the 409 when there is none to proxy.
func (s *a2aProxyService) requireCardSource(ctx context.Context, tx *gorm.DB, t a2aProxyTarget) (string, error) {
	card, err := s.lockedCard(ctx, tx, t)
	switch {
	case err != nil:
		return "", err
	case card == nil || card.Source != gwmodels.A2AAgentCardSourceExternal || card.SourceURL == "":
		return "", gwerr.ErrA2AProxyNeedsCardSource
	}
	if err := requireProxyableCardURL(card.SourceURL); err != nil {
		return "", err
	}
	return card.SourceURL, nil
}

// requireHTTPSForSecret refuses to store a credential the control plane and the gateway would send to cardURL in cleartext.
func requireHTTPSForSecret(cardURL string, hasSecret bool) error {
	if !hasSecret {
		return nil
	}
	u, err := url.Parse(cardURL)
	if err != nil || !strings.EqualFold(u.Scheme, "https") {
		return gwerr.ErrA2AProxyUpstreamAuthNeedsHTTPS
	}
	return nil
}

// requireSecretStaysOnOrigin refuses a card URL on another origin while a stored secret would be sent there.
func (s *a2aProxyService) requireSecretStaysOnOrigin(
	ctx context.Context, tx *gorm.DB, t a2aProxyTarget, proxy *gwmodels.A2AProxy, sourceURL string,
) error {
	if !proxy.HasUpstreamSecret() {
		return nil
	}
	card, err := s.lockedCard(ctx, tx, t)
	switch {
	case err != nil:
		return err
	case card == nil:
		return gwerr.ErrA2AProxyOriginChangeNeedsAuthReset
	}
	registered, regErr := url.Parse(card.SourceURL)
	next, nextErr := url.Parse(sourceURL)
	if regErr != nil || nextErr != nil || !isSameOrigin(registered, next) {
		return gwerr.ErrA2AProxyOriginChangeNeedsAuthReset
	}
	return nil
}

// proxyRow applies the PUT's secret semantics (no auth clears, a header alone keeps, a value replaces).
func (s *a2aProxyService) proxyRow(
	t a2aProxyTarget, existing *gwmodels.A2AProxy, auth *gwmodels.A2AProxyAuthUpdate,
) (*gwmodels.A2AProxy, bool, error) {
	row := &gwmodels.A2AProxy{
		OUID: t.key.OUID, ProjectName: t.key.ProjectName, AgentName: t.key.AgentName,
		EnvironmentUUID: t.key.EnvironmentUUID, EnvironmentName: t.key.EnvironmentName,
	}
	stored := existing != nil && existing.HasUpstreamSecret()
	switch {
	case auth == nil:
		return row, stored, nil
	case auth.Value == nil && !stored:
		return nil, false, fmt.Errorf("%w: a value is required, since no secret is stored", gwerr.ErrInvalidUpstreamAuth)
	case auth.Value == nil:
		row.UpstreamAuthHeader, row.UpstreamAuthSecret = auth.Header, existing.UpstreamAuthSecret
		return row, auth.Header != existing.UpstreamAuthHeader, nil
	}
	row.UpstreamAuthHeader, row.UpstreamAuthValue = auth.Header, auth.Value
	return row, true, nil
}

// switchFromDirect defaults the inbound settings and parks the card until the gateway has the Agent.
func (s *a2aProxyService) switchFromDirect(ctx context.Context, tx *gorm.DB, t a2aProxyTarget) error {
	if err := ensureDefaultAgentConfig(ctx, s.agentConfigRepo.WithTx(tx), t.key); err != nil {
		return err
	}
	err := s.cardRepo.WithTx(tx).ClearCard(ctx, t.key.OUID, t.key.ProjectName, t.key.AgentName, t.key.EnvironmentUUID)
	if errors.Is(err, gwrepo.ErrA2AAgentCardNotFound) {
		return gwerr.ErrA2AProxyNeedsCardSource
	}
	if err != nil {
		return fmt.Errorf("failed to park the agent card: %w", err)
	}
	return nil
}

// ensureArtifact creates the proxied environment's agent artifact, which the gateway serves the agent and its keys under.
func (s *a2aProxyService) ensureArtifact(tx *gorm.DB, t a2aProxyTarget) (gwmodels.ArtifactRef, error) {
	artifact, err := ensureAgentEnvAPIArtifact(tx, s.artifactRepo, t.key.OUID, t.key.ProjectName, t.key.AgentName, t.env.ID, gwmodels.KindAgent)
	if err != nil {
		return gwmodels.ArtifactRef{}, fmt.Errorf("failed to ensure the agent's API artifact: %w", err)
	}
	return agentArtifactRef(artifact), nil
}

func agentArtifactRef(artifact *gwmodels.Artifact) gwmodels.ArtifactRef {
	return gwmodels.ArtifactRef{OUID: artifact.OUID, ArtifactUUID: artifact.UUID, Kind: artifact.Kind, Handle: artifact.Handle}
}

// republish re-derives the routes from the origin card, then refetches it; a direct environment gets ifDirect (nil: ErrEnvironmentNotProxied).
func (s *a2aProxyService) republish(ctx context.Context, t a2aProxyTarget, ifDirect a2aCardStep) error {
	var artifact gwmodels.ArtifactRef
	err := s.tx(ctx, func(tx *gorm.DB) error {
		proxy, err := s.lockedProxy(ctx, tx, t)
		if err != nil {
			return err
		}
		if proxy == nil {
			return onDirect(ctx, tx, s.cardRepo, t, ifDirect)
		}
		artifact, err = s.ensureArtifact(tx, t)
		return err
	})
	if err != nil {
		return err
	}
	if artifact.ArtifactUUID != uuid.Nil {
		if err := s.publisher.exposure.MarkStale(ctx, artifact, "the agent's origin card was refreshed"); err != nil {
			return err
		}
	}
	return s.dropIfAgentGone(ctx, t)
}

// a2aCardStep is what happens to the card row when an environment stops being proxied.
type a2aCardStep func(ctx context.Context, cards gwrepo.A2AAgentCardRepository, key agentExposureKey) error

// requeueDirectCardFetch replaces the gateway's card, which advertised gateway URLs, with a fresh one from the origin.
func requeueDirectCardFetch(ctx context.Context, cards gwrepo.A2AAgentCardRepository, key agentExposureKey) error {
	err := cards.ClearCard(ctx, key.OUID, key.ProjectName, key.AgentName, key.EnvironmentUUID)
	if errors.Is(err, gwrepo.ErrA2AAgentCardNotFound) {
		return nil // the source was removed meanwhile, so there is nothing to fetch
	}
	if err != nil {
		return fmt.Errorf("failed to clear the proxied card: %w", err)
	}
	if err := cards.Requeue(ctx, key.OUID, key.ProjectName, key.AgentName, key.EnvironmentName); err != nil {
		return fmt.Errorf("failed to queue the direct card fetch: %w", err)
	}
	return nil
}

// requeueCardFetch makes the direct card row due in place; an upsert could recreate a removed row.
func requeueCardFetch(ctx context.Context, cards gwrepo.A2AAgentCardRepository, key agentExposureKey) error {
	err := cards.Requeue(ctx, key.OUID, key.ProjectName, key.AgentName, key.EnvironmentName)
	if errors.Is(err, gwrepo.ErrA2AAgentCardNotFound) {
		return gwerr.ErrAgentCardNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to queue agent card fetch: %w", err)
	}
	return nil
}

// onDirect applies step to a direct environment's card row, or refuses with gwerr.ErrEnvironmentNotProxied.
func onDirect(ctx context.Context, tx *gorm.DB, cards gwrepo.A2AAgentCardRepository, t a2aProxyTarget, step a2aCardStep) error {
	if step == nil {
		return gwerr.ErrEnvironmentNotProxied
	}
	return step(ctx, cards.WithTx(tx), t.key)
}

// deleteCardRow drops the card row together with its removed source.
func deleteCardRow(ctx context.Context, cards gwrepo.A2AAgentCardRepository, key agentExposureKey) error {
	if err := cards.DeleteForAgentEnv(ctx, key.OUID, key.ProjectName, key.AgentName, key.EnvironmentName); err != nil {
		return fmt.Errorf("failed to delete the agent card: %w", err)
	}
	return nil
}

// removeProxy switches to direct, discarding the secret, so it is refused unrecorded; a direct environment gets ifDirect (nil: ErrEnvironmentNotProxied).
func (s *a2aProxyService) removeProxy(ctx context.Context, t a2aProxyTarget, cardStep, ifDirect a2aCardStep) error {
	err := s.tx(ctx, func(tx *gorm.DB) error {
		existing, err := s.lockedProxy(ctx, tx, t)
		if err != nil {
			return err
		}
		if existing == nil {
			return onDirect(ctx, tx, s.cardRepo, t, ifDirect)
		}
		annotateA2AProxy(ctx, t, existing.UpstreamAuthHeader, existing.HasUpstreamSecret())
		return s.switchToDirect(ctx, tx, t, cardStep)
	})
	return err
}

// switchToDirect undeploys the agent's artifact from its gateways (keeping the artifact and its keys), deletes the
// proxy row and applies cardStep, in tx. The undeploy comes first, so the placement pass can't re-place the artifact
// from the proxy row in between.
func (s *a2aProxyService) switchToDirect(ctx context.Context, tx *gorm.DB, t a2aProxyTarget, cardStep a2aCardStep) error {
	artifact, err := s.artifact(t)
	if err != nil {
		return err
	}
	if err := s.publisher.exposure.TeardownEnv(ctx, tx, t.key, artifact); err != nil {
		return err
	}
	err = s.proxyRepo.WithTx(tx).Delete(ctx, t.key.OUID, t.key.ProjectName, t.key.AgentName, t.key.EnvironmentUUID)
	if err != nil {
		return fmt.Errorf("failed to delete the A2A proxy: %w", err)
	}
	return cardStep(ctx, s.cardRepo.WithTx(tx), t.key)
}

// artifact is the environment's agent artifact; its ArtifactUUID is uuid.Nil when it has none.
func (s *a2aProxyService) artifact(t a2aProxyTarget) (gwmodels.ArtifactRef, error) {
	artifact, err := agentEnvAPIArtifact(s.artifactRepo, t.key.OUID, t.key.ProjectName, t.key.AgentName, t.env.ID, gwmodels.KindAgent)
	if errors.Is(err, gwerr.ErrArtifactNotFound) {
		return gwmodels.ArtifactRef{OUID: t.key.OUID, Kind: gwmodels.KindAgent}, nil
	}
	if err != nil {
		return gwmodels.ArtifactRef{}, fmt.Errorf("failed to read the agent's API artifact: %w", err)
	}
	return agentArtifactRef(artifact), nil
}

// dropIfAgentGone undoes this request's writes when the agent was deleted or replaced meanwhile; the deletion could not
// see them. An unanswered check keeps the committed writes: failing the request now would report a false failure.
func (s *a2aProxyService) dropIfAgentGone(ctx context.Context, t a2aProxyTarget) error {
	gone, err := agentGoneOrReplaced(ctx, s.ocClient, t.key.OUID, t.key.ProjectName, t.key.AgentName, t.agent.ID)
	if err != nil {
		s.logger.Warn("Could not confirm the agent still exists after an A2A proxy write; keeping it",
			"agentName", t.key.AgentName, "environment", t.key.EnvironmentName, "error", err)
		return nil
	}
	if !gone {
		return nil
	}
	if err := s.forget(ctx, t); err != nil {
		s.logger.Error("Failed to drop the A2A proxy of a deleted agent",
			"agentName", t.key.AgentName, "environment", t.key.EnvironmentName, "error", err)
	}
	return gwerr.ErrAgentNotFound
}

// forget removes the proxy row, the card row and the environment's artifact, whose exposure and deployments cascade;
// the artifact's delete events are written last in the same transaction.
func (s *a2aProxyService) forget(ctx context.Context, t a2aProxyTarget) error {
	return s.tx(ctx, func(tx *gorm.DB) error {
		if _, err := s.lockedProxy(ctx, tx, t); err != nil {
			return err
		}
		artifact, err := s.artifact(t)
		if err != nil {
			return err
		}
		err = s.proxyRepo.WithTx(tx).Delete(ctx, t.key.OUID, t.key.ProjectName, t.key.AgentName, t.key.EnvironmentUUID)
		if err != nil && !errors.Is(err, gwrepo.ErrA2AProxyNotFound) {
			return fmt.Errorf("failed to delete the A2A proxy: %w", err)
		}
		// StoreCardSource writes the card row, so this service clears it as well.
		err = s.cardRepo.WithTx(tx).DeleteForAgentEnv(ctx, t.key.OUID, t.key.ProjectName, t.key.AgentName, t.key.EnvironmentName)
		if err != nil {
			return fmt.Errorf("failed to delete the agent card: %w", err)
		}
		if artifact.ArtifactUUID == uuid.Nil {
			return nil
		}
		return s.publisher.exposure.DropArtifact(ctx, tx, artifact)
	})
}

// annotateA2AProxy names the agent and the upstream credential change the
// request's audit record is about.
func annotateA2AProxy(ctx context.Context, t a2aProxyTarget, upstreamAuthHeader string, authChanged bool) {
	modkit.AuditResource(ctx, gwaudit.ResourceAgent, t.agent.ID, t.agent.Name)
	modkit.AuditDetail(ctx, "agentName", t.agent.Name)
	modkit.AuditDetail(ctx, "upstreamAuthHeader", upstreamAuthHeader)
	modkit.AuditDetail(ctx, "upstreamAuthChanged", authChanged)
}

// ensureDefaultAgentConfig gives a newly proxied environment a platform A2A agent's inbound defaults, minus forwarding the
// caller's token to the third-party origin; a saved config wins.
func ensureDefaultAgentConfig(ctx context.Context, configs gwrepo.AgentConfigRepository, key agentExposureKey) error {
	api := withA2AVersionHeader(resolveAPIConfig(nil, nil, nil, nil, nil, true))
	api.OAuthForwardToken = false
	err := configs.CreateIfAbsent(ctx, agentConfigFromAPIConfig(key.OUID, key.ProjectName, key.AgentName, key.EnvironmentName, api))
	if err != nil {
		return fmt.Errorf("failed to write the default agent config: %w", err)
	}
	return nil
}

// validateA2AUpstreamAuth enforces what the gateway carries safely: it pastes into single-quoted YAML, doubles '%' and renders {{ }}.
func validateA2AUpstreamAuth(auth *gwmodels.A2AProxyAuthUpdate) error {
	if auth == nil {
		return nil
	}
	switch {
	case !a2aUpstreamHeaderPattern.MatchString(auth.Header):
		return fmt.Errorf("%w: header may contain only letters, digits, '-' and '_'", gwerr.ErrInvalidUpstreamAuth)
	case len(auth.Header) > a2aUpstreamHeaderMaxLen:
		return fmt.Errorf("%w: header must be at most %d characters", gwerr.ErrInvalidUpstreamAuth, a2aUpstreamHeaderMaxLen)
	case isReservedUpstreamHeader(auth.Header):
		return fmt.Errorf("%w: the %s header cannot be set upstream", gwerr.ErrInvalidUpstreamAuth, auth.Header)
	}
	if auth.Value == nil {
		return nil
	}
	value := *auth.Value
	switch {
	case value == "":
		return fmt.Errorf("%w: value must not be empty; omit upstreamAuth to clear it", gwerr.ErrInvalidUpstreamAuth)
	case len(value) > a2aUpstreamValueMaxLen:
		return fmt.Errorf("%w: value must be at most %d bytes", gwerr.ErrInvalidUpstreamAuth, a2aUpstreamValueMaxLen)
	case strings.ContainsAny(value, "'\r\n"):
		return fmt.Errorf("%w: value must not contain quotes or line breaks", gwerr.ErrInvalidUpstreamAuth)
	case strings.Contains(value, "%"):
		return fmt.Errorf("%w: value must not contain '%%'", gwerr.ErrInvalidUpstreamAuth)
	case strings.IndexFunc(value, unicode.IsControl) >= 0:
		return fmt.Errorf("%w: value must not contain control characters", gwerr.ErrInvalidUpstreamAuth)
	case strings.Contains(value, "{{"):
		return fmt.Errorf("%w: value must not contain '{{'", gwerr.ErrInvalidUpstreamAuth)
	}
	return nil
}

// a2aReservedUpstreamHeaders (lowercase) frame or route the request, so the gateway must own them.
var a2aReservedUpstreamHeaders = []string{
	"host", "content-length", "transfer-encoding", "connection", "upgrade", "te", "keep-alive", "trailer",
}

func isReservedUpstreamHeader(header string) bool {
	name := strings.ToLower(header)
	return strings.HasPrefix(name, "proxy-") || slices.Contains(a2aReservedUpstreamHeaders, name)
}
