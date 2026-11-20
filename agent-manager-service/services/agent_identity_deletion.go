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

// ProjectAgentIdentityCleanup keeps the component and binding records available until CP cleanup succeeds.
type ProjectAgentIdentityCleanup interface {
	DeleteAgentWithIdentityCleanup(ctx context.Context, ouID, projectName, agentName string, deleteComponent func(context.Context) error) (bool, error)
}

func (s *agentThunderProvisioningService) DeleteAgentWithIdentityCleanup(
	ctx context.Context, ouID, projectName, agentName string, deleteComponent func(context.Context) error,
) (bool, error) {
	if s.identityManagementMode != thundersvc.IdentityManagementCPManaged {
		return false, nil
	}
	return true, s.deleteCPIdentityBindings(ctx, ouID, projectName, agentName, deleteComponent)
}

func (s *agentThunderProvisioningService) deleteCPIdentityBindings(
	ctx context.Context, ouID, projectName, agentName string, deleteComponent func(context.Context) error,
) error {
	if s.projectIdentityDeletionProvider == nil {
		return errors.New("project AgentID deletion provider is not configured")
	}
	snapshot, err := s.repo.FindByAgent(ctx, ouID, projectName, agentName)
	if err != nil {
		return fmt.Errorf("read AgentID bindings for deletion: %w", err)
	}
	sort.Slice(snapshot, func(i, j int) bool { return snapshot[i].EnvironmentName < snapshot[j].EnvironmentName })
	bindings := make([]models.AgentThunderClient, 0, len(snapshot))
	for _, binding := range snapshot {
		release, lockErr := s.lockBinding(ctx, ouID, projectName, agentName, binding.EnvironmentName)
		if lockErr != nil {
			return lockErr
		}
		defer release()
		current, getErr := s.repo.Get(ctx, ouID, projectName, agentName, binding.EnvironmentName)
		if errors.Is(getErr, repositories.ErrAgentThunderClientNotFound) {
			continue
		}
		if getErr != nil {
			return fmt.Errorf("read locked AgentID binding: %w", getErr)
		}
		if current.ID != binding.ID {
			return errors.New("agent identity changed during deletion; retry the request")
		}
		bindings = append(bindings, *current)
	}
	targets, err := s.cpIdentityDeletionTargets(ctx, bindings)
	if err != nil {
		return err
	}
	var failures []error
	for _, target := range targets {
		if err := s.projectIdentityDeletionProvider.DeleteAgentIdentity(ctx, target); err != nil {
			s.logger.Error("Failed to remove AgentID from pipeline environment; retaining agent identity bindings for retry",
				"ouID", ouID, "projectName", projectName, "agentName", agentName,
				"environment", target.Environment, "agentID", target.AgentID, "error", err)
			failures = append(failures, fmt.Errorf("remove AgentID from environment %q: %w", target.Environment, err))
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	if deleteComponent != nil {
		if err := deleteComponent(ctx); err != nil {
			return err
		}
	}
	// Keep the lifecycle locks through credential cleanup so a recreation cannot lose its new secret.
	for _, binding := range bindings {
		if err := s.repo.UpdateSecretRef(ctx, binding.ID, ""); err != nil {
			return fmt.Errorf("clear deleted AgentID credential reference: %w", err)
		}
	}
	for _, binding := range bindings {
		if binding.ProvisioningType != models.AgentProvisioningTypeInternal {
			continue
		}
		if err := s.deleteCredential(ctx, ouID, projectName, agentName, binding.EnvironmentName); err != nil {
			return fmt.Errorf("delete AgentID credential in %q: %w", binding.EnvironmentName, err)
		}
		if isLegacySecretRefPath(binding.SecretRefPath) {
			if err := s.cleanupLegacySecretRef(ctx, ouID, agentName, binding.EnvironmentName); err != nil {
				return err
			}
		}
	}
	ids := make([]uuid.UUID, 0, len(bindings))
	for _, binding := range bindings {
		ids = append(ids, binding.ID)
	}
	if err := s.repo.DeleteByIDs(ctx, ids); err != nil {
		return fmt.Errorf("delete cleaned AgentID bindings: %w", err)
	}
	return nil
}

func (s *agentThunderProvisioningService) cpIdentityDeletionTargets(
	ctx context.Context, bindings []models.AgentThunderClient,
) ([]ProjectAgentIdentityDeleteRequest, error) {
	if len(bindings) == 0 {
		return nil, nil
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
				return nil, errors.New("multiple source AgentIDs require cleanup before agent deletion")
			}
			source = binding
		}
	}
	if !hasIdentity {
		return nil, nil
	}
	if source == nil {
		return nil, errors.New("source AgentID binding is missing; cannot safely delete project identities")
	}
	projectID, err := s.projectIdentityID(ctx, source.OUID, source.ProjectName)
	if err != nil {
		return nil, fmt.Errorf("resolve identity project for deletion: %w", err)
	}
	targets := make([]ProjectAgentIdentityDeleteRequest, 0, len(bindings))
	known := make(map[string]bool, len(bindings))
	appendTarget := func(binding models.AgentThunderClient) {
		id := binding.ThunderAgentID
		if id == "" {
			id = source.ThunderAgentID
		}
		known[binding.EnvironmentName] = true
		targets = append(targets, ProjectAgentIdentityDeleteRequest{
			OrganizationID: source.OUID, ProjectID: projectID, ProjectName: source.ProjectName,
			AgentName: source.AgentName, Environment: binding.EnvironmentName, AgentID: id,
			ClientID: binding.ThunderClientID, Source: binding.EnvironmentName == source.EnvironmentName,
		})
	}
	appendTarget(*source)
	for _, binding := range bindings {
		if !known[binding.EnvironmentName] {
			appendTarget(binding)
		}
	}
	if s.pipelineReader == nil {
		return nil, errProjectPipelineReaderUnavailable
	}
	_, environments, err := s.pipelineReader.ProjectPipeline(ctx, source.OUID, source.ProjectName)
	if err != nil {
		return nil, fmt.Errorf("resolve all project identity deletion environments: %w", err)
	}
	for _, environment := range environments {
		if environment.Name == "" {
			return nil, errors.New("project pipeline contains an unnamed environment")
		}
		if !known[environment.Name] {
			appendTarget(models.AgentThunderClient{EnvironmentName: environment.Name})
		}
	}
	return targets, nil
}
