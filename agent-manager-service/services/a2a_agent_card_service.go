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
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

const a2aCardSourceURLMaxLen = 2048

// a2aCardHostNotPublicMsg replaces DNS detail that would reveal internal hosts and resolvers.
const a2aCardHostNotPublicMsg = "url host must be publicly reachable"

// A2AAgentCardService reads and manages stored A2A agent cards.
type A2AAgentCardService interface {
	GetA2AAgentCard(ctx context.Context, ouID, projectName, agentName, envName string) (*models.A2AAgentCard, error)
	RefreshA2AAgentCard(ctx context.Context, ouID, projectName, agentName, envName string) error
	SetA2ACardSource(ctx context.Context, ouID, projectName, agentName, envName, sourceURL string) error
	DeleteA2ACardSource(ctx context.Context, ouID, projectName, agentName, envName string) error
}

type a2aAgentCardService struct {
	ocClient client.OpenChoreoClient
	cardRepo repositories.A2AAgentCardRepository
	logger   *slog.Logger
}

// NewA2AAgentCardService creates an A2AAgentCardService.
func NewA2AAgentCardService(
	ocClient client.OpenChoreoClient,
	cardRepo repositories.A2AAgentCardRepository,
	logger *slog.Logger,
) A2AAgentCardService {
	return &a2aAgentCardService{ocClient: ocClient, cardRepo: cardRepo, logger: logger}
}

func (s *a2aAgentCardService) GetA2AAgentCard(
	ctx context.Context, ouID, projectName, agentName, envName string,
) (*models.A2AAgentCard, error) {
	// Rows exist only for live A2A agents, so OpenChoreo is asked only on a miss.
	row, err := s.cardRepo.Get(ctx, ouID, projectName, agentName, envName)
	if err == nil {
		return row, nil
	}
	if !errors.Is(err, repositories.ErrA2AAgentCardNotFound) {
		return nil, fmt.Errorf("failed to read agent card: %w", err)
	}
	if _, err := s.getAgent(ctx, ouID, projectName, agentName); err != nil {
		return nil, err
	}
	// No row means no fetch is queued; a synthetic pending would never resolve.
	return nil, utils.ErrAgentCardNotFound
}

func (s *a2aAgentCardService) RefreshA2AAgentCard(
	ctx context.Context, ouID, projectName, agentName, envName string,
) error {
	agent, err := s.getA2AAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return err
	}
	env, err := requireEnvironmentTier(ctx, s.ocClient, s.logger, ouID, envName)
	if err != nil {
		return err
	}
	if !isExternalProvisioned(agent) {
		return s.enqueue(ctx, agent, ouID, projectName, env, models.A2AAgentCardSourcePlatform, "")
	}
	// Update-only, so a concurrent remove can't be undone by an upsert.
	if err := s.cardRepo.Requeue(ctx, ouID, projectName, agentName, envName); err != nil {
		if errors.Is(err, repositories.ErrA2AAgentCardNotFound) {
			return utils.ErrAgentCardNotFound
		}
		return fmt.Errorf("failed to queue agent card fetch: %w", err)
	}
	return s.dropIfAgentGone(ctx, agent, ouID, projectName, envName)
}

func (s *a2aAgentCardService) SetA2ACardSource(
	ctx context.Context, ouID, projectName, agentName, envName, sourceURL string,
) error {
	agent, err := s.getExternalA2AAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return err
	}
	env, err := requireEnvironmentTier(ctx, s.ocClient, s.logger, ouID, envName)
	if err != nil {
		return err
	}
	sourceURL = strings.TrimSpace(sourceURL)
	if err := validateCardSourceURL(ctx, sourceURL); err != nil {
		return err
	}
	err = s.enqueue(ctx, agent, ouID, projectName, env, models.A2AAgentCardSourceExternal, sourceURL)
	recordCardSourceChange(ctx, audit.ActionAgentSetCardSource, agent, ouID, projectName, envName, err,
		audit.Detail("sourceUrl", sourceURL))
	return err
}

func (s *a2aAgentCardService) DeleteA2ACardSource(
	ctx context.Context, ouID, projectName, agentName, envName string,
) error {
	agent, err := s.getExternalA2AAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return err
	}
	if _, err := requireEnvironmentTier(ctx, s.ocClient, s.logger, ouID, envName); err != nil {
		return err
	}
	err = s.cardRepo.DeleteForAgentEnv(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		err = fmt.Errorf("failed to delete agent card: %w", err)
	}
	recordCardSourceChange(ctx, audit.ActionAgentRemoveCardSource, agent, ouID, projectName, envName, err)
	return err
}

// recordCardSourceChange emits the fail-open audit record shared by both card-source actions.
func recordCardSourceChange(
	ctx context.Context, action audit.Action, agent *models.AgentResponse,
	ouID, projectName, envName string, result error, extra ...audit.Option,
) {
	opts := append([]audit.Option{
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAgent, agent.UUID, agent.Name),
		audit.Project(projectName),
		audit.Environment(envName),
		audit.Detail("agentName", agent.Name),
		audit.Result(result),
	}, extra...)
	audit.Record(ctx, action, opts...)
}

func (s *a2aAgentCardService) enqueue(
	ctx context.Context, agent *models.AgentResponse, ouID, projectName string, env *models.EnvironmentResponse,
	source models.A2AAgentCardSource, sourceURL string,
) error {
	envUUID, err := uuid.Parse(env.UUID)
	if err != nil {
		return fmt.Errorf("invalid UUID for environment %s: %w", env.Name, err)
	}
	card := &models.A2AAgentCard{
		OUID:            ouID,
		ProjectName:     projectName,
		AgentName:       agent.Name,
		EnvironmentName: env.Name,
		EnvironmentUUID: envUUID,
		Source:          source,
		SourceURL:       sourceURL,
	}
	if err := s.cardRepo.Enqueue(ctx, card); err != nil {
		return fmt.Errorf("failed to queue agent card fetch: %w", err)
	}
	return s.dropIfAgentGone(ctx, agent, ouID, projectName, env.Name)
}

// dropIfAgentGone deletes the just-written row when the agent was deleted or replaced during the upsert.
func (s *a2aAgentCardService) dropIfAgentGone(
	ctx context.Context, agent *models.AgentResponse, ouID, projectName, envName string,
) error {
	current, err := s.getAgent(ctx, ouID, projectName, agent.Name)
	switch {
	case errors.Is(err, utils.ErrAgentNotFound):
	case err != nil:
		return err
	case current.UUID == agent.UUID:
		return nil
	}
	if err := s.cardRepo.DeleteForAgentEnv(ctx, ouID, projectName, agent.Name, envName); err != nil {
		return fmt.Errorf("failed to drop agent card of a deleted agent: %w", err)
	}
	return utils.ErrAgentNotFound
}

func (s *a2aAgentCardService) getAgent(
	ctx context.Context, ouID, projectName, agentName string,
) (*models.AgentResponse, error) {
	agent, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, translateAgentError(err)
	}
	return agent, nil
}

func (s *a2aAgentCardService) getA2AAgent(
	ctx context.Context, ouID, projectName, agentName string,
) (*models.AgentResponse, error) {
	agent, err := s.getAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, err
	}
	if !utils.IsA2AAgentSubType(agent.Type.SubType) {
		return nil, utils.ErrAgentNotA2A
	}
	return agent, nil
}

func (s *a2aAgentCardService) getExternalA2AAgent(
	ctx context.Context, ouID, projectName, agentName string,
) (*models.AgentResponse, error) {
	agent, err := s.getA2AAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, err
	}
	if !isExternalProvisioned(agent) {
		return nil, utils.ErrAgentCardSourceNotExternal
	}
	return agent, nil
}

func isExternalProvisioned(agent *models.AgentResponse) bool {
	return agent.Provisioning.Type == string(utils.ExternalAgent)
}

// validateCardSourceURL wraps every rejection in utils.ErrInvalidURL.
func validateCardSourceURL(ctx context.Context, sourceURL string) error {
	if sourceURL == "" {
		return fmt.Errorf("%w: url is required", utils.ErrInvalidURL)
	}
	if utf8.RuneCountInString(sourceURL) > a2aCardSourceURLMaxLen {
		return fmt.Errorf("%w: url must be at most %d characters", utils.ErrInvalidURL, a2aCardSourceURLMaxLen)
	}
	if err := ssrf.ValidateURL(ctx, sourceURL); errors.Is(err, ssrf.ErrHostNotPublic) {
		return fmt.Errorf("%w: %s", utils.ErrInvalidURL, a2aCardHostNotPublicMsg)
	} else if err != nil {
		return fmt.Errorf("%w: %w", utils.ErrInvalidURL, err)
	}
	return nil
}
