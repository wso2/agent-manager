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

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// a2aPublicationKey names one agent-environment gateway publication.
type a2aPublicationKey struct {
	OUID            string
	ProjectName     string
	AgentName       string
	EnvironmentName string
	EnvironmentUUID uuid.UUID
	// AgentUUID is nil when unknown; the reconciler then checks the agent by name.
	AgentUUID *uuid.UUID
}

// a2aTeardown is an environment's gateway withdrawal, sent once its DB work has committed.
type a2aTeardown struct {
	artifactUUID uuid.UUID
	gatewayIDs   []string
	// dropsArtifact deletes the gateway's Agent with its API keys; otherwise it is undeployed and keeps them.
	dropsArtifact bool
}

// TxRunner opens one database transaction and runs fn inside it, so a unit test can hand fn a sentinel.
type TxRunner func(ctx context.Context, fn func(tx *gorm.DB) error) error

// A2APublisher holds the gateway-publication steps agentManagerService and a2aProxyService share.
type A2APublisher struct {
	tx             TxRunner
	pubRepo        repositories.A2APublicationRepository
	proxyRepo      repositories.A2AProxyRepository
	cardRepo       repositories.A2AAgentCardRepository
	artifactRepo   repositories.ArtifactRepository
	deploymentRepo repositories.DeploymentRepository
	gatewayRepo    repositories.GatewayRepository
	events         *GatewayEventsService
	logger         *slog.Logger
}

// NewA2APublisher creates an A2APublisher.
func NewA2APublisher(
	tx TxRunner,
	pubRepo repositories.A2APublicationRepository,
	proxyRepo repositories.A2AProxyRepository,
	cardRepo repositories.A2AAgentCardRepository,
	artifactRepo repositories.ArtifactRepository,
	deploymentRepo repositories.DeploymentRepository,
	gatewayRepo repositories.GatewayRepository,
	events *GatewayEventsService,
	logger *slog.Logger,
) *A2APublisher {
	return &A2APublisher{
		tx:             tx,
		pubRepo:        pubRepo,
		proxyRepo:      proxyRepo,
		cardRepo:       cardRepo,
		artifactRepo:   artifactRepo,
		deploymentRepo: deploymentRepo,
		gatewayRepo:    gatewayRepo,
		events:         events,
		logger:         logger,
	}
}

// a2aPublicationSourceFor says where the agent's gateway upstream comes from.
func a2aPublicationSourceFor(agent *models.AgentResponse) models.A2APublicationSource {
	if isExternalProvisioned(agent) {
		return models.A2APublicationSourceExternal
	}
	return models.A2APublicationSourcePlatform
}

// Enqueue queues the environment's publication; a non-nil tx makes it part of the caller's transaction.
func (p *A2APublisher) Enqueue(
	ctx context.Context, tx *gorm.DB, key a2aPublicationKey, source models.A2APublicationSource, artifactUUID uuid.UUID,
) error {
	pub := &models.A2APublication{
		OUID:            key.OUID,
		ProjectName:     key.ProjectName,
		AgentName:       key.AgentName,
		EnvironmentName: key.EnvironmentName,
		EnvironmentUUID: key.EnvironmentUUID,
		ArtifactUUID:    artifactUUID,
		AgentUUID:       key.AgentUUID,
		Source:          source,
	}
	if err := p.publications(tx).Enqueue(ctx, pub); err != nil {
		return fmt.Errorf("failed to queue the A2A gateway publication: %w", err)
	}
	return nil
}

// ShouldPublish is true for every internal A2A agent, and for an external one only while its environment is
// proxied; a non-nil tx reads the proxy row inside the caller's transaction.
func (p *A2APublisher) ShouldPublish(
	ctx context.Context, tx *gorm.DB, agent *models.AgentResponse, ouID, projectName string, envUUID uuid.UUID,
) (bool, error) {
	if !utils.IsA2AAgentSubType(agent.Type.SubType) {
		return false, nil
	}
	if !isExternalProvisioned(agent) {
		return true, nil
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

// SaveAndRepublish runs save in one transaction with the republish the agent's environment is owed afterwards, if
// any. An external agent is decided under its environment's transition lock, so a concurrent switch to direct cannot
// leave an external publication behind, and a save whose republish cannot be queued is rolled back whole.
func (p *A2APublisher) SaveAndRepublish(
	ctx context.Context, agent *models.AgentResponse, key a2aPublicationKey, artifactUUID uuid.UUID, save func(tx *gorm.DB) error,
) error {
	return p.tx(ctx, func(tx *gorm.DB) error {
		if isExternalProvisioned(agent) {
			if err := lockA2AProxyEnv(ctx, p.proxies(tx), key); err != nil {
				return err
			}
		}
		publish, err := p.ShouldPublish(ctx, tx, agent, key.OUID, key.ProjectName, key.EnvironmentUUID)
		if err != nil {
			return err
		}
		if err := save(tx); err != nil {
			return err
		}
		if !publish {
			return nil
		}
		return p.Enqueue(ctx, tx, key, a2aPublicationSourceFor(agent), artifactUUID)
	})
}

// GatewayURL is where the environment's ingress gateway serves the agent, or "" while none is mapped to it yet.
func (p *A2APublisher) GatewayURL(ouID string, envUUID uuid.UUID, agentName string) (string, error) {
	gateway, err := findIngressGateway(p.gatewayRepo, ouID, envUUID)
	if err != nil || gateway == nil {
		return "", err
	}
	return a2aGatewayURL(gateway, agentName), nil
}

// Publication returns the environment's publication row.
func (p *A2APublisher) Publication(ctx context.Context, ouID, projectName, agentName string, envUUID uuid.UUID) (*models.A2APublication, error) {
	return p.pubRepo.Get(ctx, ouID, projectName, agentName, envUUID)
}

// A2AGatewayStatus is what the gateway currently does for an agent in one environment.
type A2AGatewayStatus struct {
	GatewayURL string // "" while no ingress gateway is mapped
	Status     models.A2APublicationStatus
	LastError  string
	Transports []A2ATransport // nil when the row's stored transports are NULL
}

// a2aNoPublicationMsg reports an environment with no publication row, naming what re-queues one: a missing row
// never resolves on its own.
var a2aNoPublicationMsg = map[models.A2APublicationSource]string{
	models.A2APublicationSourceExternal: "no gateway publication is queued for this environment; refresh to publish it again",
	models.A2APublicationSourcePlatform: "no gateway publication is queued for this environment; redeploy the agent or save its deploy settings to publish it again",
}

// GatewayStatus reads the environment's gateway URL and publication; a missing publication reports failed.
func (p *A2APublisher) GatewayStatus(
	ctx context.Context, ouID, projectName, agentName string, envUUID uuid.UUID, source models.A2APublicationSource,
) (A2AGatewayStatus, error) {
	gatewayURL, err := p.GatewayURL(ouID, envUUID, agentName)
	if err != nil {
		return A2AGatewayStatus{}, err
	}
	pub, err := p.Publication(ctx, ouID, projectName, agentName, envUUID)
	switch {
	case errors.Is(err, repositories.ErrA2APublicationNotFound):
		return A2AGatewayStatus{GatewayURL: gatewayURL, Status: models.A2APublicationStatusFailed, LastError: a2aNoPublicationMsg[source]}, nil
	case err != nil:
		return A2AGatewayStatus{}, fmt.Errorf("failed to read the A2A publication: %w", err)
	}
	status := A2AGatewayStatus{GatewayURL: gatewayURL, Status: pub.Status, LastError: pub.LastError}
	if len(pub.PublishedTransports) > 0 {
		if err := json.Unmarshal(pub.PublishedTransports, &status.Transports); err != nil {
			return A2AGatewayStatus{}, fmt.Errorf("failed to read the published A2A transports: %w", err)
		}
	}
	return status, nil
}

// TeardownEnv drops one environment's publication and gateway deployments in tx but keeps the artifact and its API keys; send the result after commit.
func (p *A2APublisher) TeardownEnv(
	ctx context.Context, tx *gorm.DB, key a2aPublicationKey, artifactUUID uuid.UUID,
) (a2aTeardown, error) {
	// Read first: the deployment rows name the gateways holding the Agent.
	gatewayIDs := collectA2AAgentDeletionTargets(p.deploymentRepo, p.gatewayRepo, artifactUUID, key.OUID, true, p.logger)
	if artifactUUID != uuid.Nil {
		if err := p.deploymentRepo.WithTx(tx).DeleteForArtifact(ctx, artifactUUID, key.OUID); err != nil {
			return a2aTeardown{}, fmt.Errorf("failed to delete the A2A agent's gateway deployments: %w", err)
		}
	}
	err := p.publications(tx).DeleteForAgentEnv(ctx, key.OUID, key.ProjectName, key.AgentName, key.EnvironmentUUID)
	if err != nil {
		return a2aTeardown{}, fmt.Errorf("failed to delete the A2A publication: %w", err)
	}
	return a2aTeardown{artifactUUID: artifactUUID, gatewayIDs: gatewayIDs}, nil
}

// TeardownEnvAndDropArtifact is TeardownEnv that also deletes the artifact (uuid.Nil: none), so its API keys go too.
func (p *A2APublisher) TeardownEnvAndDropArtifact(
	ctx context.Context, tx *gorm.DB, key a2aPublicationKey, artifactUUID uuid.UUID,
) (a2aTeardown, error) {
	teardown, err := p.TeardownEnv(ctx, tx, key, artifactUUID)
	if err != nil {
		return a2aTeardown{}, err
	}
	teardown.dropsArtifact = true
	if artifactUUID == uuid.Nil {
		return teardown, nil
	}
	err = p.artifactRepo.Delete(tx, artifactUUID.String())
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return a2aTeardown{}, fmt.Errorf("failed to delete the agent's API artifact: %w", err)
	}
	return teardown, nil
}

// SendTeardown tells each gateway to stop serving the torn-down Agent; best effort.
func (p *A2APublisher) SendTeardown(ctx context.Context, t a2aTeardown) {
	sendA2AAgentWithdrawal(ctx, p.events, t.artifactUUID, t.gatewayIDs, !t.dropsArtifact, p.logger)
}

// DeleteForAgent drops the agent's publication and proxy rows; deleteAgentAPIArtifact tells the gateways.
func (p *A2APublisher) DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error {
	var errs []error
	if err := p.pubRepo.DeleteForAgent(ctx, ouID, projectName, agentName); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete A2A publications: %w", err))
	}
	if err := p.proxyRepo.DeleteForAgent(ctx, ouID, projectName, agentName); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete A2A proxies: %w", err))
	}
	return errors.Join(errs...)
}

// TeardownEnvironment removes a deleted environment's A2A rows, deployments and A2A agent artifacts, then tells the
// gateways; one failure does not stop the rest. Call it before the environment's card rows go: they name the agents.
func (p *A2APublisher) TeardownEnvironment(ctx context.Context, ouID string, envUUID uuid.UUID) error {
	pubs, err := p.pubRepo.ListForEnvironment(ctx, ouID, envUUID)
	if err != nil {
		return fmt.Errorf("failed to list the environment's A2A publications: %w", err)
	}
	var errs []error
	var teardowns []a2aTeardown
	for _, pub := range pubs {
		teardown, err := p.teardownDeletedEnv(ctx, pub)
		if err != nil {
			errs = append(errs, err)
		}
		teardowns = append(teardowns, teardown)
	}
	// A switch to direct drops the publication but keeps the artifact, so find the rest through the card rows.
	swept, err := p.dropLeftoverArtifacts(ctx, ouID, envUUID, teardowns)
	if err != nil {
		errs = append(errs, err)
	}
	teardowns = append(teardowns, swept...)
	// Rows a failed teardown left, and proxies that never published, go regardless: the environment is gone.
	if err := p.pubRepo.DeleteForEnvironment(ctx, ouID, envUUID); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete the environment's A2A publications: %w", err))
	}
	if err := p.proxyRepo.DeleteForEnvironment(ctx, ouID, envUUID); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete the environment's A2A proxies: %w", err))
	}
	for _, teardown := range teardowns {
		p.SendTeardown(ctx, teardown)
	}
	return errors.Join(errs...)
}

// EnvironmentUUIDs lists every environment UUID with publication or proxy rows recorded under name, even when one listing fails.
func (p *A2APublisher) EnvironmentUUIDs(ctx context.Context, ouID, name string) ([]uuid.UUID, error) {
	var errs []error
	pubs, err := p.pubRepo.ListEnvironmentUUIDs(ctx, ouID, name)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to list A2A publication environments: %w", err))
	}
	proxies, err := p.proxyRepo.ListEnvironmentUUIDs(ctx, ouID, name)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to list A2A proxy environments: %w", err))
	}
	return append(pubs, proxies...), errors.Join(errs...)
}

// teardownDeletedEnv removes one publication together with its artifact, which a deleted environment no longer needs.
// On failure it still returns the teardown to send; agent deletion sweeps the artifact left behind.
func (p *A2APublisher) teardownDeletedEnv(ctx context.Context, pub models.A2APublication) (a2aTeardown, error) {
	key := a2aPublicationKey{
		OUID: pub.OUID, ProjectName: pub.ProjectName, AgentName: pub.AgentName,
		EnvironmentName: pub.EnvironmentName, EnvironmentUUID: pub.EnvironmentUUID,
	}
	var teardown a2aTeardown
	err := p.tx(ctx, func(tx *gorm.DB) error {
		var err error
		teardown, err = p.TeardownEnvAndDropArtifact(ctx, tx, key, pub.ArtifactUUID)
		return err
	})
	if err != nil {
		// The environment's rows go regardless, so the gateways must drop the Agent now or never.
		return a2aTeardown{
			artifactUUID:  pub.ArtifactUUID,
			gatewayIDs:    collectA2AAgentDeletionTargets(p.deploymentRepo, p.gatewayRepo, pub.ArtifactUUID, pub.OUID, true, p.logger),
			dropsArtifact: true,
		}, fmt.Errorf("agent %s: %w", pub.AgentName, err)
	}
	return teardown, nil
}

// dropLeftoverArtifacts deletes the artifacts of the environment's A2A agents that no publication teardown covered.
// The card rows name those agents; a REST or chat agent's artifact for the environment is not touched.
func (p *A2APublisher) dropLeftoverArtifacts(ctx context.Context, ouID string, envUUID uuid.UUID, done []a2aTeardown) ([]a2aTeardown, error) {
	cards, err := p.cardRepo.ListForEnvironment(ctx, ouID, envUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the environment's A2A agent cards: %w", err)
	}
	covered := make(map[uuid.UUID]struct{}, len(done))
	for _, t := range done {
		covered[t.artifactUUID] = struct{}{}
	}
	var errs []error
	var teardowns []a2aTeardown
	for _, card := range cards {
		artifact, err := agentEnvAPIArtifact(p.artifactRepo, ouID, card.ProjectName, card.AgentName, envUUID.String())
		if errors.Is(err, utils.ErrArtifactNotFound) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to read agent %s's artifact: %w", card.AgentName, err))
			continue
		}
		if _, ok := covered[artifact.UUID]; ok {
			continue
		}
		// Read before the delete: the deployment rows cascade with the artifact.
		gatewayIDs := collectA2AAgentDeletionTargets(p.deploymentRepo, p.gatewayRepo, artifact.UUID, ouID, true, p.logger)
		err = p.tx(ctx, func(tx *gorm.DB) error { return p.artifactRepo.Delete(tx, artifact.UUID.String()) })
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			errs = append(errs, fmt.Errorf("failed to delete agent artifact %s: %w", artifact.Handle, err))
			continue
		}
		teardowns = append(teardowns, a2aTeardown{artifactUUID: artifact.UUID, gatewayIDs: gatewayIDs, dropsArtifact: true})
	}
	return teardowns, errors.Join(errs...)
}

func (p *A2APublisher) publications(tx *gorm.DB) repositories.A2APublicationRepository {
	if tx == nil {
		return p.pubRepo
	}
	return p.pubRepo.WithTx(tx)
}

func (p *A2APublisher) proxies(tx *gorm.DB) repositories.A2AProxyRepository {
	if tx == nil {
		return p.proxyRepo
	}
	return p.proxyRepo.WithTx(tx)
}

// lockA2AProxyEnv takes the environment's transition lock on proxies, which must be bound to the transaction that holds it.
func lockA2AProxyEnv(ctx context.Context, proxies repositories.A2AProxyRepository, key a2aPublicationKey) error {
	if err := proxies.LockAgentEnv(ctx, key.OUID, key.ProjectName, key.AgentName, key.EnvironmentUUID); err != nil {
		return fmt.Errorf("failed to lock the A2A proxy: %w", err)
	}
	return nil
}

// lockedA2AProxy takes the environment's transition lock and reads its proxy row under it; nil while direct.
func lockedA2AProxy(ctx context.Context, proxies repositories.A2AProxyRepository, key a2aPublicationKey) (*models.A2AProxy, error) {
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
