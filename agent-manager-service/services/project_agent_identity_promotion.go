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
	"sort"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

const promotionTopologyPageSize = 100

var errProjectAgentIdentityPromotionUnavailable = errors.New("project AgentID promotion provider is not configured")

// ProjectAgentIdentityPromotionReconciler is an optional background hook used
// only by CP-managed deployments.
type ProjectAgentIdentityPromotionReconciler interface {
	ReconcileProjectAgentIdentityPromotions(ctx context.Context)
}

type promotionStep struct {
	Environment string
	Source      string
}

func promotionSteps(source string, environments []ProjectIdentityEnvironment) ([]promotionStep, error) {
	if source == "" {
		return nil, errors.New("identity source environment is required")
	}
	edges := make(map[string][]string, len(environments))
	known := make(map[string]struct{}, len(environments))
	for _, environment := range environments {
		if environment.Name == "" {
			return nil, errors.New("identity environment name is required")
		}
		known[environment.Name] = struct{}{}
		edges[environment.Name] = append([]string(nil), environment.PromotesTo...)
		sort.Strings(edges[environment.Name])
	}
	if _, ok := known[source]; !ok {
		return nil, fmt.Errorf("identity source environment %q is absent from the project topology", source)
	}

	steps := []promotionStep{{Environment: source}}
	seen := map[string]struct{}{source: {}}
	queue := []string{source}
	for len(queue) > 0 {
		from := queue[0]
		queue = queue[1:]
		for _, to := range edges[from] {
			if _, ok := known[to]; !ok {
				return nil, fmt.Errorf("identity promotion target %q is absent from the project topology", to)
			}
			if _, ok := seen[to]; ok {
				continue
			}
			seen[to] = struct{}{}
			steps = append(steps, promotionStep{Environment: to, Source: from})
			queue = append(queue, to)
		}
	}
	if len(seen) != len(known) {
		return nil, errors.New("identity project topology contains environments unreachable from its source")
	}
	return steps, nil
}

// projectPromotionSteps reads the project's pipeline once and returns the
// ordered promotion steps, the source environment first.
func (s *agentThunderProvisioningService) projectPromotionSteps(
	ctx context.Context, organizationID, projectName string,
) ([]promotionStep, error) {
	if s.pipelineReader == nil {
		return nil, errProjectPipelineReaderUnavailable
	}
	source, environments, err := s.pipelineReader.ProjectPipeline(ctx, organizationID, projectName)
	if err != nil {
		return nil, fmt.Errorf("resolve project promotion topology: %w", err)
	}
	return promotionSteps(source, environments)
}

// ensureProjectPromotionBindings writes the binding rows of one agent for every
// step of its project's topology. Safe to repeat: existing rows are never
// touched, and a concurrent writer winning an insert is treated as success.
func (s *agentThunderProvisioningService) ensureProjectPromotionBindings(
	ctx context.Context, ref repositories.AgentRef, requestedBy string,
) ([]models.AgentThunderClient, error) {
	steps, err := s.projectPromotionSteps(ctx, ref.OUID, ref.ProjectName)
	if err != nil {
		return nil, err
	}
	return s.writePromotionBindings(ctx, ref, steps, requestedBy)
}

// writePromotionBindings creates the missing rows for steps in order, source
// first, and returns only the rows this call inserted.
//
// An agent never gets a second source: if the pipeline source row is missing
// but the agent already has a source row in another environment, nothing is
// written. Re-homing an agent to a different source environment is not
// supported.
//
// When requestedBy is empty it is taken from the existing source row of the
// chain, so rows created later still carry the user the agent was created for.
func (s *agentThunderProvisioningService) writePromotionBindings(
	ctx context.Context, ref repositories.AgentRef, steps []promotionStep, requestedBy string,
) ([]models.AgentThunderClient, error) {
	created := make([]models.AgentThunderClient, 0, len(steps))
	requestedByResolved := requestedBy != ""
	for _, step := range steps {
		_, err := s.repo.Get(ctx, ref.OUID, ref.ProjectName, ref.AgentName, step.Environment)
		if err == nil {
			continue
		}
		if !errors.Is(err, repositories.ErrAgentThunderClientNotFound) {
			return created, fmt.Errorf("read project AgentID binding for %s: %w", step.Environment, err)
		}
		if step.Source == "" {
			existingHome, homeErr := s.existingHomeEnvironment(ctx, ref, step.Environment)
			if homeErr != nil {
				return created, homeErr
			}
			if existingHome != "" {
				s.logger.Warn("Project AgentID source changed; not creating a second source binding, re-homing is not supported",
					"ouID", ref.OUID, "projectName", ref.ProjectName, "agentName", ref.AgentName,
					"currentSource", existingHome, "pipelineSource", step.Environment)
				return created, nil
			}
		} else if !requestedByResolved {
			requestedByResolved = true
			if home, getErr := s.repo.Get(ctx, ref.OUID, ref.ProjectName, ref.AgentName, steps[0].Environment); getErr == nil {
				requestedBy = home.RequestedBy
			}
		}
		binding := models.AgentThunderClient{
			OUID: ref.OUID, ProjectName: ref.ProjectName, AgentName: ref.AgentName,
			EnvironmentName: step.Environment, PromotionSource: step.Source,
			ProvisioningType: ref.ProvisioningType, Status: models.AgentThunderStatusPending,
			RequestedBy: requestedBy,
		}
		if err := s.repo.Upsert(ctx, &binding); err != nil {
			return created, fmt.Errorf("write project AgentID binding for %s: %w", step.Environment, err)
		}
		// On an insert conflict the repository leaves the ID unset: another
		// writer owns that row and will attempt it.
		if binding.ID != uuid.Nil {
			created = append(created, binding)
		}
	}
	return created, nil
}

// existingHomeEnvironment returns the environment of the agent's existing
// source binding (empty PromotionSource) other than pipelineSource, or "" when
// there is none.
func (s *agentThunderProvisioningService) existingHomeEnvironment(
	ctx context.Context, ref repositories.AgentRef, pipelineSource string,
) (string, error) {
	existing, err := s.repo.FindByAgent(ctx, ref.OUID, ref.ProjectName, ref.AgentName)
	if err != nil {
		return "", fmt.Errorf("read project AgentID bindings: %w", err)
	}
	for _, binding := range existing {
		if binding.PromotionSource == "" && binding.EnvironmentName != pipelineSource {
			return binding.EnvironmentName, nil
		}
	}
	return "", nil
}

type projectKey struct{ organizationID, projectName string }

type projectStepsResult struct {
	steps []promotionStep
	err   error
}

// ReconcileProjectAgentIdentityPromotions backfills target bindings when a
// project gains an environment after its AgentIDs were created. The pipeline
// of each project is read once per pass and shared by all of its agents. It is
// cheap and idempotent, so concurrent replicas converge; the caller may
// additionally serialize passes across replicas.
func (s *agentThunderProvisioningService) ReconcileProjectAgentIdentityPromotions(ctx context.Context) {
	if s.identityManagementMode != thundersvc.IdentityManagementCPManaged || s.projectIdentityProvider == nil {
		return
	}
	if _, ok := s.projectIdentityProvider.(ProjectAgentIdentityPromotionProvider); !ok {
		return
	}
	cache := make(map[projectKey]projectStepsResult)
	var after *repositories.AgentRef
	for {
		refs, err := s.repo.ListHomeAgents(ctx, after, promotionTopologyPageSize)
		if err != nil {
			s.logger.Error("Failed to list project AgentIDs for promotion reconciliation", "error", err)
			return
		}
		for _, ref := range refs {
			if ctx.Err() != nil {
				return
			}
			key := projectKey{ref.OUID, ref.ProjectName}
			resolved, ok := cache[key]
			if !ok {
				resolved.steps, resolved.err = s.projectPromotionSteps(ctx, ref.OUID, ref.ProjectName)
				cache[key] = resolved
				if resolved.err != nil {
					s.logger.Warn("Failed to resolve project AgentID promotion topology",
						"ouID", ref.OUID, "projectName", ref.ProjectName, "error", resolved.err)
				}
			}
			if resolved.err != nil {
				continue
			}
			if len(resolved.steps) == 0 {
				s.logger.Warn("Project AgentID promotion topology has no source",
					"ouID", ref.OUID, "projectName", ref.ProjectName, "agentName", ref.AgentName)
				continue
			}
			if resolved.steps[0].Environment != ref.EnvironmentName {
				s.logger.Warn("Project AgentID source changed; automatic home migration is not supported",
					"ouID", ref.OUID, "projectName", ref.ProjectName, "agentName", ref.AgentName,
					"currentSource", ref.EnvironmentName, "pipelineSource", resolved.steps[0].Environment)
				continue
			}
			if _, err := s.writePromotionBindings(ctx, ref, resolved.steps, ref.RequestedBy); err != nil {
				s.logger.Warn("Failed to reconcile project AgentID promotion bindings",
					"ouID", ref.OUID, "projectName", ref.ProjectName, "agentName", ref.AgentName, "error", err)
			}
		}
		if len(refs) < promotionTopologyPageSize {
			return
		}
		last := refs[len(refs)-1]
		after = &last
	}
}

var _ ProjectAgentIdentityPromotionReconciler = (*agentThunderProvisioningService)(nil)
