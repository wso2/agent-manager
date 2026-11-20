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
	"strings"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// AgentDeleteWarning reports something the user must finish after an agent was deleted.
type AgentDeleteWarning struct {
	Code         string
	Message      string
	Environments []string
}

// outsidePipelineWarning reports AgentIDs left in environments that are no longer in the project pipeline.
// The agent is already deleted, so the identities there can only be removed through the identity provider.
func outsidePipelineWarning(environments []string) AgentDeleteWarning {
	return AgentDeleteWarning{
		Code: utils.ErrCodeAgentIdentityOutsidePipeline,
		Message: "The agent was deleted, but its identity may still exist in environments outside the current deployment pipeline: " +
			strings.Join(environments, ", ") + ". Remove it in those environments through your identity provider console.",
		Environments: environments,
	}
}

// ProjectAgentIdentityCleanup keeps the component and binding records available until CP cleanup succeeds.
// The second result lists environments outside the current pipeline that may still hold the AgentID.
type ProjectAgentIdentityCleanup interface {
	DeleteAgentWithIdentityCleanup(ctx context.Context, ouID, projectName, agentName string, deleteComponent func(context.Context) error) (bool, []string, error)
}

func (s *agentThunderProvisioningService) DeleteAgentWithIdentityCleanup(
	ctx context.Context, ouID, projectName, agentName string, deleteComponent func(context.Context) error,
) (bool, []string, error) {
	if s.identityManagementMode != thundersvc.IdentityManagementCPManaged {
		return false, nil, nil
	}
	outside, err := s.deleteCPIdentityBindings(ctx, ouID, projectName, agentName, deleteComponent)
	return true, outside, err
}

func (s *agentThunderProvisioningService) deleteCPIdentityBindings(
	ctx context.Context, ouID, projectName, agentName string, deleteComponent func(context.Context) error,
) ([]string, error) {
	if s.projectIdentityDeletionProvider == nil {
		return nil, errors.New("project AgentID deletion provider is not configured")
	}
	// Serialize CP provisioning and binding insertion before taking the deletion snapshot.
	releaseAgent, err := s.lockAgentIdentity(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, fmt.Errorf("lock project identity for deletion: %w", err)
	}
	defer releaseAgent()
	snapshot, err := s.repo.FindByAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, fmt.Errorf("read AgentID bindings for deletion: %w", err)
	}
	sort.Slice(snapshot, func(i, j int) bool { return snapshot[i].EnvironmentName < snapshot[j].EnvironmentName })
	bindings := make([]models.AgentThunderClient, 0, len(snapshot))
	for _, binding := range snapshot {
		release, lockErr := s.lockBinding(ctx, ouID, projectName, agentName, binding.EnvironmentName)
		if lockErr != nil {
			return nil, lockErr
		}
		defer release()
		current, getErr := s.repo.Get(ctx, ouID, projectName, agentName, binding.EnvironmentName)
		if errors.Is(getErr, repositories.ErrAgentThunderClientNotFound) {
			continue
		}
		if getErr != nil {
			return nil, fmt.Errorf("read locked AgentID binding: %w", getErr)
		}
		if current.ID != binding.ID {
			return nil, errors.New("agent identity changed during deletion; retry the request")
		}
		bindings = append(bindings, *current)
	}
	targets, outside, err := s.cpIdentityDeletionTargets(ctx, bindings)
	if err != nil {
		return nil, err
	}
	var failures []error
	blocked := make(map[string]bool)
	for _, target := range targets {
		if target.SourceEnvironment != "" && blocked[target.SourceEnvironment] {
			blocked[target.Environment] = true
			failures = append(failures, fmt.Errorf("cannot remove AgentID from %q until removal from %q succeeds", target.Environment, target.SourceEnvironment))
			continue
		}
		if err := s.projectIdentityDeletionProvider.DeleteAgentIdentity(ctx, target); err != nil {
			s.logger.Error("Failed to remove AgentID from pipeline environment; retaining agent identity bindings for retry",
				"ouID", ouID, "projectName", projectName, "agentName", agentName,
				"environment", target.Environment, "agentID", target.AgentID, "error", err)
			failure := fmt.Errorf("remove AgentID from environment %q: %w", target.Environment, err)
			if target.Source {
				return nil, failure
			}
			blocked[target.Environment] = true
			failures = append(failures, failure)
		}
	}
	if err := errors.Join(failures...); err != nil {
		return nil, err
	}
	if deleteComponent != nil {
		if err := deleteComponent(ctx); err != nil {
			return nil, err
		}
	}
	// FAILED prevents credential injection and automatic provisioning while preserving cleanup markers.
	for _, binding := range bindings {
		if err := s.repo.UpdateAfterAttempt(ctx, binding.ID, repositories.AgentThunderAttemptUpdate{
			Status:    models.AgentThunderStatusFailed,
			LastError: "Agent identity deletion is incomplete. Retry agent deletion.",
		}); err != nil {
			failures = append(failures, fmt.Errorf("disable deleted AgentID binding in %q: %w", binding.EnvironmentName, err))
			continue
		}
		if binding.ProvisioningType == models.AgentProvisioningTypeInternal {
			if err := s.deleteCredential(ctx, ouID, projectName, agentName, binding.EnvironmentName); err != nil {
				failures = append(failures, fmt.Errorf("delete AgentID credential in %q: %w", binding.EnvironmentName, err))
				continue
			}
			if isLegacySecretRefPath(binding.SecretRefPath) {
				if err := s.cleanupLegacySecretRef(ctx, ouID, agentName, binding.EnvironmentName); err != nil {
					failures = append(failures, fmt.Errorf("delete legacy AgentID reference in %q: %w", binding.EnvironmentName, err))
					continue
				}
			}
		}
		if err := s.repo.UpdateSecretRef(ctx, binding.ID, ""); err != nil {
			failures = append(failures, fmt.Errorf("clear deleted AgentID credential reference in %q: %w", binding.EnvironmentName, err))
		}
	}
	if err := errors.Join(failures...); err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(bindings))
	for _, binding := range bindings {
		ids = append(ids, binding.ID)
	}
	if err := s.repo.DeleteByIDs(ctx, ids); err != nil {
		return nil, fmt.Errorf("delete cleaned AgentID bindings: %w", err)
	}
	return outside, nil
}

func (s *agentThunderProvisioningService) cpIdentityDeletionTargets(
	ctx context.Context, bindings []models.AgentThunderClient,
) ([]ProjectAgentIdentityDeleteRequest, []string, error) {
	if len(bindings) == 0 {
		return nil, nil, nil
	}
	var source *models.AgentThunderClient
	hasIdentity := false
	for i := range bindings {
		binding := &bindings[i]
		if binding.ThunderAgentID == "" {
			continue
		}
		hasIdentity = true
		if binding.PromotionSource == "" {
			if source != nil {
				return nil, nil, errors.New("multiple source AgentIDs require cleanup before agent deletion")
			}
			source = binding
		}
	}
	if !hasIdentity {
		return nil, nil, nil
	}
	if source == nil {
		return nil, nil, errors.New("source AgentID binding is missing; cannot safely delete project identities")
	}
	projectID, err := s.projectIdentityID(ctx, source.OUID, source.ProjectName)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve identity project for deletion: %w", err)
	}
	steps, err := s.projectPromotionSteps(ctx, source.OUID, source.ProjectName)
	if err != nil {
		return nil, nil, err
	}
	if len(steps) == 0 {
		return nil, nil, errors.New("project identity deletion pipeline is empty")
	}
	pipeline := make(map[string]promotionStep, len(steps))
	for _, step := range steps {
		pipeline[step.Environment] = step
	}
	byEnvironment := make(map[string]models.AgentThunderClient, len(bindings))
	requestFor := func(binding models.AgentThunderClient) ProjectAgentIdentityDeleteRequest {
		id := binding.ThunderAgentID
		if id == "" {
			id = source.ThunderAgentID
		}
		return ProjectAgentIdentityDeleteRequest{
			OrganizationID: source.OUID, ProjectID: projectID, ProjectName: source.ProjectName,
			AgentName: source.AgentName, Environment: binding.EnvironmentName, AgentID: id,
			ClientID: binding.ThunderClientID,
			// The caller sets the pipeline position of each target.
			Source: false, SourceEnvironment: "",
		}
	}
	// Environments outside the current pipeline are never blocked on: the pipeline is cleaned up and the
	// agent deleted first, then the user is told where an identity may remain. An environment is left out of
	// the warning only when the provider confirms the AgentID is gone; an unanswered check counts as present.
	var outside []string
	checker, canCheck := s.projectIdentityDeletionProvider.(ProjectAgentIdentityPresenceProvider)
	for _, binding := range bindings {
		byEnvironment[binding.EnvironmentName] = binding
		if _, ok := pipeline[binding.EnvironmentName]; ok {
			continue
		}
		if canCheck {
			exists, err := checker.AgentIdentityExists(ctx, requestFor(binding))
			if err == nil && !exists {
				continue
			}
			if err != nil {
				s.logger.Warn("Could not confirm AgentID absence outside the pipeline; reporting it for manual cleanup",
					"environment", binding.EnvironmentName, "error", err)
			}
		}
		outside = append(outside, binding.EnvironmentName)
	}
	sort.Strings(outside)
	if steps[0].Environment != source.EnvironmentName {
		return nil, nil, fmt.Errorf("%w: the identity source changed from %q to %q; reconcile the identity in the identity provider before deleting this agent", utils.ErrConflict, source.EnvironmentName, steps[0].Environment)
	}
	targets := make([]ProjectAgentIdentityDeleteRequest, 0, len(steps))
	for _, step := range steps {
		binding, ok := byEnvironment[step.Environment]
		if !ok {
			binding.EnvironmentName = step.Environment
		}
		request := requestFor(binding)
		request.Source = step.Source == ""
		request.SourceEnvironment = step.Source
		targets = append(targets, request)
	}
	return targets, outside, nil
}
