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

func (s *agentThunderProvisioningService) projectPromotionSteps(
	ctx context.Context, organizationID, projectID, projectName string,
) ([]promotionStep, error) {
	source, err := s.projectIdentityProvider.SourceEnvironment(ctx, organizationID, projectID, projectName)
	if err != nil {
		return nil, fmt.Errorf("resolve project identity source environment: %w", err)
	}
	promoter, ok := s.projectIdentityProvider.(ProjectAgentIdentityPromotionProvider)
	if !ok {
		return nil, errProjectAgentIdentityPromotionUnavailable
	}
	environments, err := promoter.Environments(ctx, organizationID, projectID, projectName)
	if err != nil {
		return nil, fmt.Errorf("resolve project identity environments: %w", err)
	}
	return promotionSteps(source, environments)
}

func (s *agentThunderProvisioningService) ensureProjectPromotionBindings(
	ctx context.Context, ref repositories.AgentRef, requestedBy string,
) ([]models.AgentThunderClient, error) {
	source, err := s.projectIdentityProvider.SourceEnvironment(ctx, ref.OUID, ref.ProjectName, ref.ProjectName)
	if err != nil {
		return nil, fmt.Errorf("resolve project identity source environment: %w", err)
	}
	steps := []promotionStep{{Environment: source}}
	bindings := make([]models.AgentThunderClient, 0, len(steps))
	write := func(step promotionStep) error {
		if _, getErr := s.repo.Get(ctx, ref.OUID, ref.ProjectName, ref.AgentName, step.Environment); getErr == nil {
			return nil
		} else if !errors.Is(getErr, repositories.ErrAgentThunderClientNotFound) {
			return fmt.Errorf("read project AgentID binding for %s: %w", step.Environment, getErr)
		}
		binding := models.AgentThunderClient{
			OUID: ref.OUID, ProjectName: ref.ProjectName, AgentName: ref.AgentName,
			EnvironmentName: step.Environment, PromotionSource: step.Source,
			ProvisioningType: ref.ProvisioningType, Status: models.AgentThunderStatusPending,
			RequestedBy: requestedBy,
		}
		if upsertErr := s.repo.Upsert(ctx, &binding); upsertErr != nil {
			return fmt.Errorf("write project AgentID binding for %s: %w", step.Environment, upsertErr)
		}
		if binding.ID != uuid.Nil {
			bindings = append(bindings, binding)
		}
		return nil
	}
	if err := write(steps[0]); err != nil {
		return bindings, err
	}

	promoter, ok := s.projectIdentityProvider.(ProjectAgentIdentityPromotionProvider)
	if !ok {
		return bindings, errProjectAgentIdentityPromotionUnavailable
	}
	environments, err := promoter.Environments(ctx, ref.OUID, ref.ProjectName, ref.ProjectName)
	if err != nil {
		return bindings, fmt.Errorf("resolve project identity environments: %w", err)
	}
	steps, err = promotionSteps(source, environments)
	if err != nil {
		return bindings, err
	}
	for _, step := range steps {
		if err := write(step); err != nil {
			return bindings, err
		}
	}
	return bindings, nil
}

// ReconcileProjectAgentIdentityPromotions backfills target bindings when a
// project gains an environment after its AgentIDs were created.
func (s *agentThunderProvisioningService) ReconcileProjectAgentIdentityPromotions(ctx context.Context) {
	if s.identityManagementMode != thundersvc.IdentityManagementCPManaged || s.projectIdentityProvider == nil {
		return
	}
	if _, ok := s.projectIdentityProvider.(ProjectAgentIdentityPromotionProvider); !ok {
		return
	}
	var after *repositories.AgentRef
	for {
		refs, err := s.repo.ListHomeAgents(ctx, after, promotionTopologyPageSize)
		if err != nil {
			s.logger.Error("Failed to list project AgentIDs for promotion reconciliation", "error", err)
			return
		}
		for _, ref := range refs {
			if _, err := s.ensureProjectPromotionBindings(ctx, ref, ""); err != nil {
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
