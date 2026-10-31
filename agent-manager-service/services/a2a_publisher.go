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

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// A2APublisher holds the A2A gateway-publication steps agentManagerService and a2aProxyService share, on top of
// the exposure steps every agent kind shares.
type A2APublisher struct {
	tx             TxRunner
	exposureRepo   repositories.PlatformAgentExposureRepository
	proxyRepo      repositories.A2AProxyRepository
	cardRepo       repositories.A2AAgentCardRepository
	artifactRepo   repositories.ArtifactRepository
	deploymentRepo repositories.DeploymentRepository
	logger         *slog.Logger
	exposure       *AgentExposurePublisher
	kind           *A2AAgentKind
}

// NewA2APublisher creates an A2APublisher.
func NewA2APublisher(
	tx TxRunner,
	exposureRepo repositories.PlatformAgentExposureRepository,
	proxyRepo repositories.A2AProxyRepository,
	cardRepo repositories.A2AAgentCardRepository,
	artifactRepo repositories.ArtifactRepository,
	deploymentRepo repositories.DeploymentRepository,
	logger *slog.Logger,
	exposure *AgentExposurePublisher,
	kind *A2AAgentKind,
) *A2APublisher {
	return &A2APublisher{
		tx:             tx,
		exposureRepo:   exposureRepo,
		proxyRepo:      proxyRepo,
		cardRepo:       cardRepo,
		artifactRepo:   artifactRepo,
		deploymentRepo: deploymentRepo,
		logger:         logger,
		exposure:       exposure,
		kind:           kind,
	}
}

// ShouldPublish is true for every internal A2A agent, and for an external agent only while its environment is
// proxied; a non-nil tx reads the proxy row inside the caller's transaction.
func (p *A2APublisher) ShouldPublish(
	ctx context.Context, tx *gorm.DB, agent *models.AgentResponse, ouID, projectName string, envUUID uuid.UUID,
) (bool, error) {
	if !isExternalProvisioned(agent) {
		return utils.IsA2AAgentSubType(agent.Type.SubType), nil
	}
	_, err := p.proxies(tx).GetByEnvUUID(ctx, ouID, projectName, agent.Name, envUUID)
	switch {
	case errors.Is(err, repositories.ErrA2AProxyNotFound):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("failed to read the A2A proxy: %w", err)
	}
	return true, nil
}

// SaveAndRepublish runs save in one transaction with the redeploy the agent's environment is owed afterwards, if
// any. A platform agent's exposure is written and its render queued in that transaction. An external agent is
// decided under its environment's transition lock, so a concurrent switch to direct cannot be missed; while proxied,
// its placed rows are marked stale once the save commits. The save stands if that fails: the next save or proxy
// refresh re-renders them.
func (p *A2APublisher) SaveAndRepublish(
	ctx context.Context, agent *models.AgentResponse, key agentExposureKey, artifact models.ArtifactRef, save func(tx *gorm.DB) error,
) error {
	external := isExternalProvisioned(agent)
	var publish bool
	err := p.tx(ctx, func(tx *gorm.DB) error {
		if external {
			if err := lockA2AProxyEnv(ctx, p.proxies(tx), key); err != nil {
				return err
			}
		}
		var err error
		if publish, err = p.ShouldPublish(ctx, tx, agent, key.OUID, key.ProjectName, key.EnvironmentUUID); err != nil {
			return err
		}
		if err := save(tx); err != nil {
			return err
		}
		if !publish || external {
			return nil
		}
		return p.publish(ctx, tx, key, artifact)
	})
	if err != nil || !publish {
		return err
	}
	if !external {
		p.exposure.Nudge()
		return nil
	}
	if err := p.exposure.MarkStale(ctx, artifact, "the agent's deploy settings changed"); err != nil {
		p.logger.Warn("Saved the agent's deploy settings but couldn't re-render its gateway deployments",
			"artifactUUID", artifact.ArtifactUUID, "error", err)
	}
	return nil
}

func (p *A2APublisher) publish(ctx context.Context, tx *gorm.DB, key agentExposureKey, artifact models.ArtifactRef) error {
	return p.exposure.Publish(ctx, tx, p.kind, key, artifact, models.AgentExposureProtocolA2A)
}

// DropAgentArtifact deletes one of a deleted agent's per-environment artifacts with its exposure, proxy and card
// rows in that environment, and writes its delete events, in tx. A REST or chat agent's artifact, never sent to a
// gateway as an A2A Agent, gets no events; isA2AAgent is false also when the agent could not be read, and rows the
// artifact was sent with then stand in for it.
func (p *A2APublisher) DropAgentArtifact(
	ctx context.Context, tx *gorm.DB, key agentExposureKey, artifact models.ArtifactRef, isA2AAgent bool,
) error {
	err := p.exposure.exposures(tx).DeleteForAgentEnv(ctx, key.OUID, key.ProjectName, key.AgentName, key.EnvironmentUUID)
	if err != nil {
		return fmt.Errorf("failed to delete the A2A agent's exposure: %w", err)
	}
	err = p.proxies(tx).Delete(ctx, key.OUID, key.ProjectName, key.AgentName, key.EnvironmentUUID)
	if err != nil && !errors.Is(err, repositories.ErrA2AProxyNotFound) {
		return fmt.Errorf("failed to delete the A2A proxy: %w", err)
	}
	if p.cardRepo != nil {
		err := p.cardRepo.WithTx(tx).DeleteForAgentEnvUUID(ctx, key.OUID, key.ProjectName, key.AgentName, key.EnvironmentUUID)
		if err != nil {
			return fmt.Errorf("failed to delete the A2A agent card: %w", err)
		}
	}
	if !isA2AAgent {
		sent, err := p.deploymentRepo.WithTx(tx).SentGateways(ctx, artifact.OUID, artifact.ArtifactUUID)
		if err != nil {
			return fmt.Errorf("failed to list the agent's gateways: %w", err)
		}
		if len(sent) == 0 {
			return p.exposure.deleteArtifact(tx, artifact)
		}
	}
	return p.exposure.DropArtifact(ctx, tx, artifact)
}

// DeleteForAgent drops the agent's exposure and proxy rows; deleteAgentAPIArtifact tells the gateways.
func (p *A2APublisher) DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error {
	var errs []error
	if err := p.exposureRepo.DeleteForAgent(ctx, ouID, projectName, agentName); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete A2A agent exposures: %w", err))
	}
	if err := p.proxyRepo.DeleteForAgent(ctx, ouID, projectName, agentName); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete A2A proxies: %w", err))
	}
	return errors.Join(errs...)
}

// TeardownEnvironment removes a deleted environment's A2A rows and A2A agent artifacts, each artifact in one
// transaction with its delete events; one failure does not stop the rest. Exposures name the platform agents and
// proxies the external ones. Call it before the environment's card rows go: they name the agents left by a switch
// to direct, whose artifacts neither covers.
func (p *A2APublisher) TeardownEnvironment(ctx context.Context, ouID string, envUUID uuid.UUID) error {
	exposures, err := p.exposureRepo.ListForEnvironment(ctx, ouID, envUUID)
	if err != nil {
		return fmt.Errorf("failed to list the environment's A2A agent exposures: %w", err)
	}
	proxies, err := p.proxyRepo.ListForEnvironment(ctx, ouID, envUUID)
	if err != nil {
		return fmt.Errorf("failed to list the environment's A2A proxies: %w", err)
	}
	var errs []error
	dropped := map[uuid.UUID]struct{}{}
	for _, exposure := range exposures {
		shape, ok := agentGatewayShapeForProtocol(exposure.Protocol)
		if !ok {
			errs = append(errs, fmt.Errorf("agent %s: unknown exposure protocol %q", exposure.AgentName, exposure.Protocol))
			continue
		}
		artifact := models.ArtifactRef{OUID: ouID, ArtifactUUID: exposure.ArtifactUUID, Kind: shape.ArtifactKind}
		if err := p.exposure.dropArtifact(ctx, artifact); err != nil {
			errs = append(errs, fmt.Errorf("agent %s: %w", exposure.AgentName, err))
			continue
		}
		dropped[exposure.ArtifactUUID] = struct{}{}
	}
	agents := make([]agentInEnvironment, 0, len(proxies))
	for _, proxy := range proxies {
		agents = append(agents, agentInEnvironment{projectName: proxy.ProjectName, agentName: proxy.AgentName})
	}
	if err := p.dropAgentsArtifacts(ctx, ouID, envUUID, agents, dropped); err != nil {
		errs = append(errs, err)
	}
	if err := p.dropLeftoverArtifacts(ctx, ouID, envUUID, dropped); err != nil {
		errs = append(errs, err)
	}
	if err := p.exposureRepo.DeleteForEnvironment(ctx, ouID, envUUID); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete the environment's A2A agent exposures: %w", err))
	}
	if err := p.proxyRepo.DeleteForEnvironment(ctx, ouID, envUUID); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete the environment's A2A proxies: %w", err))
	}
	return errors.Join(errs...)
}

// EnvironmentUUIDs lists every environment UUID with exposure or proxy rows recorded under name, even when one listing fails.
func (p *A2APublisher) EnvironmentUUIDs(ctx context.Context, ouID, name string) ([]uuid.UUID, error) {
	var errs []error
	exposed, err := p.exposureRepo.ListEnvironmentUUIDs(ctx, ouID, name)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to list A2A agent exposure environments: %w", err))
	}
	proxies, err := p.proxyRepo.ListEnvironmentUUIDs(ctx, ouID, name)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to list A2A proxy environments: %w", err))
	}
	return append(exposed, proxies...), errors.Join(errs...)
}

// agentInEnvironment names an agent whose artifact in the environment is found by its handle.
type agentInEnvironment struct {
	projectName string
	agentName   string
}

// dropLeftoverArtifacts deletes the artifacts of the environment's A2A agents that no exposure or proxy covered:
// agents left by a switch to direct. The card rows name those agents; a REST or chat agent's artifact for the
// environment is not touched.
func (p *A2APublisher) dropLeftoverArtifacts(ctx context.Context, ouID string, envUUID uuid.UUID, dropped map[uuid.UUID]struct{}) error {
	cards, err := p.cardRepo.ListForEnvironment(ctx, ouID, envUUID)
	if err != nil {
		return fmt.Errorf("failed to list the environment's A2A agent cards: %w", err)
	}
	agents := make([]agentInEnvironment, 0, len(cards))
	for _, card := range cards {
		agents = append(agents, agentInEnvironment{projectName: card.ProjectName, agentName: card.AgentName})
	}
	return p.dropAgentsArtifacts(ctx, ouID, envUUID, agents, dropped)
}

// dropAgentsArtifacts deletes each agent's artifact in the environment that is not already in dropped, adding it.
func (p *A2APublisher) dropAgentsArtifacts(
	ctx context.Context, ouID string, envUUID uuid.UUID, agents []agentInEnvironment, dropped map[uuid.UUID]struct{},
) error {
	var errs []error
	for _, agent := range agents {
		artifact, err := agentEnvAPIArtifact(p.artifactRepo, ouID, agent.projectName, agent.agentName, envUUID.String(), models.AgentArtifactKinds...)
		if errors.Is(err, utils.ErrArtifactNotFound) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to read agent %s's artifact: %w", agent.agentName, err))
			continue
		}
		if _, ok := dropped[artifact.UUID]; ok {
			continue
		}
		ref := models.ArtifactRef{OUID: ouID, ArtifactUUID: artifact.UUID, Kind: artifact.Kind, Handle: artifact.Handle}
		if err := p.exposure.dropArtifact(ctx, ref); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete agent artifact %s: %w", artifact.Handle, err))
			continue
		}
		dropped[artifact.UUID] = struct{}{}
	}
	return errors.Join(errs...)
}

func (p *A2APublisher) proxies(tx *gorm.DB) repositories.A2AProxyRepository {
	if tx == nil {
		return p.proxyRepo
	}
	return p.proxyRepo.WithTx(tx)
}

// lockA2AProxyEnv takes the environment's transition lock on proxies, which must be bound to the transaction that holds it.
func lockA2AProxyEnv(ctx context.Context, proxies repositories.A2AProxyRepository, key agentExposureKey) error {
	if err := proxies.LockAgentEnv(ctx, key.OUID, key.ProjectName, key.AgentName, key.EnvironmentUUID); err != nil {
		return fmt.Errorf("failed to lock the A2A proxy: %w", err)
	}
	return nil
}

// lockedA2AProxy takes the environment's transition lock and reads its proxy row under it; nil while direct.
func lockedA2AProxy(ctx context.Context, proxies repositories.A2AProxyRepository, key agentExposureKey) (*models.A2AProxy, error) {
	if err := lockA2AProxyEnv(ctx, proxies, key); err != nil {
		return nil, err
	}
	proxy, err := proxies.GetByEnvUUID(ctx, key.OUID, key.ProjectName, key.AgentName, key.EnvironmentUUID)
	if errors.Is(err, repositories.ErrA2AProxyNotFound) {
		return nil, nil //nolint:nilnil // No row means the environment is direct.
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the A2A proxy: %w", err)
	}
	return proxy, nil
}
