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

package gwrepo

import (
	"context"
	"errors"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// PlatformRepository looks up the core's organizations, projects and agents by name, as
// budgets reference them: through the core API, which the module may not bypass.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg gwrepomocks -out gwrepomocks/platform_repository_mock.go . PlatformRepository:PlatformRepositoryMock
type PlatformRepository interface {
	// GetOrganization returns the organization, or gwerr.ErrOrganizationNotFound.
	GetOrganization(ctx context.Context, ouID string) (*gwmodels.OrganizationResponse, error)
	// GetProject returns the organization's project by name, or gwerr.ErrProjectNotFound.
	GetProject(ctx context.Context, ouID, name string) (*gwmodels.ProjectResponse, error)
	// GetAgent returns the project's agent by name, or gwerr.ErrProjectNotFound or
	// gwerr.ErrAgentNotFound.
	GetAgent(ctx context.Context, ouID, project, name string) (*gwmodels.AgentResponse, error)
}

type platformRepo struct{ core modkit.CoreAPI }

// NewPlatformRepository creates a PlatformRepository on the core API.
func NewPlatformRepository(core modkit.CoreAPI) PlatformRepository { return platformRepo{core: core} }

func (r platformRepo) GetOrganization(ctx context.Context, ouID string) (*gwmodels.OrganizationResponse, error) {
	org, err := r.core.GetOrg(ctx, ouID)
	if errors.Is(err, modkit.ErrNotFound) {
		return nil, gwerr.ErrOrganizationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &gwmodels.OrganizationResponse{UUID: org.ID, Name: org.Handle, Handle: org.Handle}, nil
}

func (r platformRepo) GetProject(ctx context.Context, ouID, name string) (*gwmodels.ProjectResponse, error) {
	p, err := r.core.GetProjectByName(ctx, ouID, name)
	if errors.Is(err, modkit.ErrNotFound) {
		return nil, gwerr.ErrProjectNotFound
	}
	if err != nil {
		return nil, err
	}
	return &gwmodels.ProjectResponse{UUID: p.ID, Name: p.Name}, nil
}

func (r platformRepo) GetAgent(ctx context.Context, ouID, project, name string) (*gwmodels.AgentResponse, error) {
	a, err := r.core.GetAgentByName(ctx, ouID, project, name)
	if errors.Is(err, modkit.ErrNotFound) {
		if _, perr := r.GetProject(ctx, ouID, project); perr != nil {
			return nil, perr
		}
		return nil, gwerr.ErrAgentNotFound
	}
	if err != nil {
		return nil, err
	}
	if a.ProjectName != project {
		return nil, gwerr.ErrAgentNotFound
	}
	return &gwmodels.AgentResponse{UUID: a.ID, Name: a.Name}, nil
}
