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

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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
	ClientSecret     string
	NeedsSecret      bool
}

type ProjectAgentIdentityResult struct {
	AgentID      string
	ClientID     string
	ClientSecret string
}

// ProjectIdentityEnvironment is one environment returned by the identity
// platform. PromotesTo contains only valid outgoing promotion edges.
type ProjectIdentityEnvironment struct {
	Name       string
	PromotesTo []string
}

// ProjectAgentIdentityPromotionRequest identifies one AgentID promotion. The
// provider owns previewing the change and selecting only ExistingAgentID.
type ProjectAgentIdentityPromotionRequest struct {
	OrganizationID     string
	ProjectID          string
	ProjectName        string
	AgentName          string
	SourceEnvironment  string
	TargetEnvironment  string
	ProvisioningType   models.AgentProvisioningType
	ExistingAgentID    string
	ExistingClientID   string
	TargetClientID     string
	TargetClientSecret string
	NeedsSecret        bool
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

// ProjectAgentIdentityPromotionProvider extends source provisioning with
// project-topology discovery and AgentID-only promotion.
type ProjectAgentIdentityPromotionProvider interface {
	ProjectAgentIdentityProvider
	Environments(ctx context.Context, organizationID, projectID, projectName string) ([]ProjectIdentityEnvironment, error)
	Promote(ctx context.Context, request ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error)
}

// ErrProjectAgentIdentityRequiresPromotion tells callers not to create another
// identity directly in a non-source environment.
var ErrProjectAgentIdentityRequiresPromotion = fmt.Errorf(
	"%w: project AgentID must be promoted to this environment", utils.ErrConflict,
)

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
