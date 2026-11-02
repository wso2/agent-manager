// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0

package services

import (
	"context"
	"errors"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// ProjectAgentIdentityRequest identifies one project-scoped AgentID operation.
// ProjectID and ProjectName are separate because deployments may eventually map
// human-readable names to immutable identity-platform project IDs.
type ProjectAgentIdentityRequest struct {
	OrganizationID   string
	ProjectID        string
	ProjectName      string
	Environment      string
	AgentName        string
	ProvisioningType models.AgentProvisioningType
	ExistingAgentID  string
	ExistingClientID string
	NeedsSecret      bool
}

type ProjectAgentIdentityResult struct {
	AgentID      string
	ClientID     string
	ClientSecret string
}

// ProjectAgentIdentityProvider is implemented by deployments that can create
// project-scoped AgentIDs through an identity control plane. SourceEnvironment
// must use the deployment's authoritative project mapping. Provision must be
// idempotent for ExistingAgentID, must reject non-source environments, and must
// not persist the returned secret. Activate must be idempotent.
type ProjectAgentIdentityProvider interface {
	SourceEnvironment(ctx context.Context, organizationID, projectID, projectName string) (string, error)
	Provision(ctx context.Context, request ProjectAgentIdentityRequest) (ProjectAgentIdentityResult, error)
	Activate(ctx context.Context, request ProjectAgentIdentityRequest, agentID string) error
}

// ErrProjectAgentIdentityRequiresPromotion tells callers not to create another
// identity directly in a non-source environment.
var ErrProjectAgentIdentityRequiresPromotion = errors.New("project AgentID must be promoted to this environment")

// ProjectAgentIdentityProviderSetter is an optional deployment hook. The
// open-source runtime defaults to LEGACY_DIRECT and needs no provider.
type ProjectAgentIdentityProviderSetter interface {
	SetProjectAgentIdentityProvider(mode thundersvc.IdentityManagementMode, provider ProjectAgentIdentityProvider) error
}

func (s *agentThunderProvisioningService) SetProjectAgentIdentityProvider(
	mode thundersvc.IdentityManagementMode, provider ProjectAgentIdentityProvider,
) error {
	switch mode {
	case "", thundersvc.IdentityManagementLegacyDirect:
		s.identityManagementMode = thundersvc.IdentityManagementLegacyDirect
		s.projectIdentityProvider = nil
		return nil
	case thundersvc.IdentityManagementCPManaged:
		if provider == nil {
			return errors.New("CP_MANAGED identity mode requires a project AgentID provider")
		}
		s.identityManagementMode = mode
		s.projectIdentityProvider = provider
		return nil
	default:
		return errors.New("project AgentID provisioning supports only LEGACY_DIRECT or CP_MANAGED mode")
	}
}

var _ ProjectAgentIdentityProviderSetter = (*agentThunderProvisioningService)(nil)
