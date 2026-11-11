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

// UNIT tests for infraResourceManager. No `//go:build integration` tag, so this
// runs in the fast unit tier with the OpenChoreo client mocked
// (repomocks.PlatformRepositoryMock). The goal is to assert the service's OWN
// logic — error propagation, validation gates, pagination, filtering, and the
// project↔pipeline linkage fan-out — without touching the database.
//
// An unconfigured mock method panics, so leaving a method nil asserts it must
// not be reached on that code path.
package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// newInfraManager wires the service with the mocked OpenChoreo client and a
// discard logger. Note: strPtr is already declared in llm_deployment_service_test.go.
func newInfraManager(oc *repomocks.PlatformRepositoryMock) InfraResourceManager {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewInfraResourceManager(oc, logger)
}

// okOrg returns a GetOrganization func that always resolves — used to get past
// the org-existence validation gate that fronts almost every method.
func okOrg() func(context.Context, string) (*models.OrganizationResponse, error) {
	return func(_ context.Context, ouID string) (*models.OrganizationResponse, error) {
		return &models.OrganizationResponse{Name: ouID}, nil
	}
}

// -----------------------------------------------------------------------------
// ListOrganizations — OpenChoreo-backed list with in-memory pagination.
// -----------------------------------------------------------------------------

func TestInfraResourceManager_ListOrganizations(t *testing.T) {
	t.Run("lists all orgs with pagination", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{
			ListOrganizationsFunc: func(_ context.Context) ([]*models.OrganizationResponse, error) {
				return []*models.OrganizationResponse{{Name: "a"}, {Name: "b"}, {Name: "c"}}, nil
			},
		}

		orgs, total, err := newInfraManager(oc).ListOrganizations(context.Background(), 2, 1)

		require.NoError(t, err)
		assert.Equal(t, int32(3), total)
		require.Len(t, orgs, 2)
		assert.Equal(t, "b", orgs[0].Name)
		assert.Equal(t, "c", orgs[1].Name)
	})

	t.Run("propagates ListOrganizations error", func(t *testing.T) {
		boom := errors.New("oc down")
		oc := &repomocks.PlatformRepositoryMock{
			ListOrganizationsFunc: func(_ context.Context) ([]*models.OrganizationResponse, error) {
				return nil, boom
			},
		}

		_, _, err := newInfraManager(oc).ListOrganizations(context.Background(), 10, 0)

		assert.ErrorIs(t, err, boom)
	})
}

// -----------------------------------------------------------------------------
// GetOrganization — straight error propagation + happy path.
// -----------------------------------------------------------------------------

func TestInfraResourceManager_GetOrganization(t *testing.T) {
	const org = "acme"

	t.Run("propagates client error", func(t *testing.T) {
		boom := errors.New("oc down")
		oc := &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
				return nil, boom
			},
		}
		_, err := newInfraManager(oc).GetOrganization(context.Background(), org)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("returns the org on success", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{GetOrganizationFunc: okOrg()}
		got, err := newInfraManager(oc).GetOrganization(context.Background(), org)
		require.NoError(t, err)
		assert.Equal(t, org, got.Name)
	})
}

// -----------------------------------------------------------------------------
// CreateProject — create error and happy-path transformation.
// -----------------------------------------------------------------------------

func TestInfraResourceManager_CreateProject(t *testing.T) {
	const org = "acme"
	payload := spec.CreateProjectRequest{
		Name:        "proj-a",
		DisplayName: "Project A",
		Description: strPtr("a description"),
	}

	t.Run("propagates create error", func(t *testing.T) {
		boom := errors.New("create failed")
		oc := &repomocks.PlatformRepositoryMock{
			CreateProjectFunc: func(context.Context, string, repositories.CreateProjectInput) (*models.ProjectResponse, error) {
				return nil, boom
			},
		}
		_, err := newInfraManager(oc).CreateProject(context.Background(), org, payload)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("maps request and returns the created project on success", func(t *testing.T) {
		var captured repositories.CreateProjectInput
		created := &models.ProjectResponse{Name: "proj-a", OrgName: org, Description: "a description"}
		oc := &repomocks.PlatformRepositoryMock{
			CreateProjectFunc: func(_ context.Context, ouID string, in repositories.CreateProjectInput) (*models.ProjectResponse, error) {
				assert.Equal(t, org, ouID)
				captured = in
				return created, nil
			},
		}
		got, err := newInfraManager(oc).CreateProject(context.Background(), org, payload)

		require.NoError(t, err)
		// The service maps the spec payload onto the repository input.
		assert.Equal(t, "proj-a", captured.Name)
		assert.Equal(t, "Project A", captured.DisplayName)
		assert.Equal(t, "a description", captured.Description)
		assert.Same(t, created, got)
	})
}

// -----------------------------------------------------------------------------
// UpdateProject — patch error and refetch happy path.
// -----------------------------------------------------------------------------

func TestInfraResourceManager_UpdateProject(t *testing.T) {
	const org, proj = "acme", "proj-a"
	payload := spec.UpdateProjectRequest{
		DisplayName: "New Name",
		Description: "new desc",
	}

	t.Run("propagates patch error (refetch not reached)", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{
			PatchProjectFunc: func(context.Context, string, string, *string, *string) error {
				return utils.ErrProjectNotFound
			},
			// GetProjectFunc nil => must not be called.
		}
		_, err := newInfraManager(oc).UpdateProject(context.Background(), org, proj, payload)
		assert.ErrorIs(t, err, utils.ErrProjectNotFound)
	})

	t.Run("returns the refetched project on success", func(t *testing.T) {
		var gotDisplayName, gotDescription string
		oc := &repomocks.PlatformRepositoryMock{
			PatchProjectFunc: func(_ context.Context, _, _ string, displayName, description *string) error {
				gotDisplayName, gotDescription = *displayName, *description
				return nil
			},
			GetProjectFunc: func(_ context.Context, _, _ string) (*models.ProjectResponse, error) {
				return &models.ProjectResponse{Name: proj, DisplayName: "New Name"}, nil
			},
		}
		got, err := newInfraManager(oc).UpdateProject(context.Background(), org, proj, payload)
		require.NoError(t, err)
		assert.Equal(t, "New Name", got.DisplayName)
		assert.Equal(t, "New Name", gotDisplayName)
		assert.Equal(t, "new desc", gotDescription)
	})
}

// -----------------------------------------------------------------------------
// ListProjects — org gate, list error wrapping, and pagination slicing.
// -----------------------------------------------------------------------------

func TestInfraResourceManager_ListProjects(t *testing.T) {
	const org = "acme"

	t.Run("wraps list error", func(t *testing.T) {
		boom := errors.New("list boom")
		oc := &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: okOrg(),
			ListProjectsFunc: func(_ context.Context, _ string) ([]*models.ProjectResponse, error) {
				return nil, boom
			},
		}
		_, _, err := newInfraManager(oc).ListProjects(context.Background(), org, 10, 0)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("paginates and reports the unpaginated total", func(t *testing.T) {
		all := []*models.ProjectResponse{
			{Name: "p1"}, {Name: "p2"}, {Name: "p3"}, {Name: "p4"},
		}
		oc := &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: okOrg(),
			ListProjectsFunc: func(_ context.Context, _ string) ([]*models.ProjectResponse, error) {
				return all, nil
			},
		}
		page, total, err := newInfraManager(oc).ListProjects(context.Background(), org, 2, 1)
		require.NoError(t, err)
		assert.Equal(t, int32(4), total)
		require.Len(t, page, 2)
		assert.Equal(t, "p2", page[0].Name)
		assert.Equal(t, "p3", page[1].Name)
	})

	t.Run("clamps offset beyond the end to an empty page", func(t *testing.T) {
		all := []*models.ProjectResponse{{Name: "p1"}}
		oc := &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: okOrg(),
			ListProjectsFunc: func(_ context.Context, _ string) ([]*models.ProjectResponse, error) {
				return all, nil
			},
		}
		page, total, err := newInfraManager(oc).ListProjects(context.Background(), org, 10, 5)
		require.NoError(t, err)
		assert.Equal(t, int32(1), total)
		assert.Empty(t, page)
	})
}

// -----------------------------------------------------------------------------
// DeleteProject — idempotency on not-found, the associated-agents guard, and
// the happy path.
// -----------------------------------------------------------------------------

func TestInfraResourceManager_DeleteProject(t *testing.T) {
	const org, proj = "acme", "proj-a"

	t.Run("refuses deletion when only another product's components remain", func(t *testing.T) {
		// A shared project can hold components no agent list would show. Counting agents
		// instead of components here would delete the project — and with it, via the
		// project-cleanup finalizer, another product's components.
		oc := &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: okOrg(),
			CountProjectAgentsFunc: func(_ context.Context, _, _ string) (int, error) {
				return 1, nil
			},
			ListAgentsFunc: func(_ context.Context, _, _ string) ([]*models.AgentResponse, error) {
				return []*models.AgentResponse{}, nil // no agents, yet not empty
			},
			// DeleteProjectFunc nil => must not be reached.
		}
		err := newInfraManager(oc).DeleteProject(auditableCtx(t), org, proj)
		assert.ErrorIs(t, err, utils.ErrProjectHasAssociatedAgents)
	})

	t.Run("idempotent when the component count reports project not found", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: okOrg(),
			CountProjectAgentsFunc: func(_ context.Context, _, _ string) (int, error) {
				return 0, utils.ErrProjectNotFound
			},
			// DeleteProjectFunc nil => must not be reached.
		}
		err := newInfraManager(oc).DeleteProject(auditableCtx(t), org, proj)
		require.NoError(t, err)
	})

	t.Run("refuses deletion when agents are associated", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: okOrg(),
			CountProjectAgentsFunc: func(_ context.Context, _, _ string) (int, error) {
				return 1, nil
			},
		}
		err := newInfraManager(oc).DeleteProject(auditableCtx(t), org, proj)
		assert.ErrorIs(t, err, utils.ErrProjectHasAssociatedAgents)
	})

	t.Run("deletes when no agents exist", func(t *testing.T) {
		deleted := false
		oc := &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: okOrg(),
			CountProjectAgentsFunc: func(_ context.Context, _, _ string) (int, error) {
				return 0, nil
			},
			DeleteProjectFunc: func(_ context.Context, _, _ string) error {
				deleted = true
				return nil
			},
		}
		err := newInfraManager(oc).DeleteProject(auditableCtx(t), org, proj)
		require.NoError(t, err)
		assert.True(t, deleted, "expected DeleteProject to be invoked")
	})

	t.Run("idempotent when DeleteProject reports project not found", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: okOrg(),
			CountProjectAgentsFunc: func(_ context.Context, _, _ string) (int, error) {
				return 0, nil
			},
			DeleteProjectFunc: func(_ context.Context, _, _ string) error {
				return utils.ErrProjectNotFound
			},
		}
		err := newInfraManager(oc).DeleteProject(auditableCtx(t), org, proj)
		require.NoError(t, err)
	})

	t.Run("surfaces a non-not-found list error", func(t *testing.T) {
		boom := errors.New("list boom")
		oc := &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: okOrg(),
			CountProjectAgentsFunc: func(_ context.Context, _, _ string) (int, error) {
				return 0, boom
			},
		}
		err := newInfraManager(oc).DeleteProject(auditableCtx(t), org, proj)
		assert.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, utils.ErrProjectHasAssociatedAgents)
	})
}

// -----------------------------------------------------------------------------
// GetProject — org gate then project propagation.
// -----------------------------------------------------------------------------

func TestInfraResourceManager_GetProject(t *testing.T) {
	const org, proj = "acme", "proj-a"

	t.Run("propagates project error", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: okOrg(),
			GetProjectFunc: func(_ context.Context, _, _ string) (*models.ProjectResponse, error) {
				return nil, utils.ErrProjectNotFound
			},
		}
		_, err := newInfraManager(oc).GetProject(context.Background(), org, proj)
		assert.ErrorIs(t, err, utils.ErrProjectNotFound)
	})

	t.Run("returns the project on success", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: okOrg(),
			GetProjectFunc: func(_ context.Context, _, _ string) (*models.ProjectResponse, error) {
				return &models.ProjectResponse{Name: proj}, nil
			},
		}
		got, err := newInfraManager(oc).GetProject(context.Background(), org, proj)
		require.NoError(t, err)
		assert.Equal(t, proj, got.Name)
	})
}

// -----------------------------------------------------------------------------
// ListOrgDeploymentPipelines — pagination over the client result.
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// ListOrgEnvironments — org gate then env propagation.
// -----------------------------------------------------------------------------

func TestInfraResourceManager_ListOrgEnvironments(t *testing.T) {
	const org = "acme"

	t.Run("propagates env error", func(t *testing.T) {
		boom := errors.New("env boom")
		oc := &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: okOrg(),
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return nil, boom
			},
		}
		_, err := newInfraManager(oc).ListOrgEnvironments(context.Background(), org)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("returns environments on success", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: okOrg(),
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return []*models.EnvironmentResponse{{Name: "dev"}, {Name: "prod"}}, nil
			},
		}
		got, err := newInfraManager(oc).ListOrgEnvironments(context.Background(), org)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "dev", got[0].Name)
	})
}

// -----------------------------------------------------------------------------
// GetProjectDeploymentPipeline — org gate then pipeline propagation.
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// CreateOrgDeploymentPipeline — slugified name, create error, optional project
// linkage fan-out (GetProject + PatchProject) and its error branch.
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// UpdateOrgDeploymentPipeline — pure error propagation / pass-through.
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// DeleteOrgDeploymentPipeline — the referencing-projects guard (the core logic),
// the list-error wrap, and the happy path.
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// GetDataplanes — org gate then dataplane propagation.
// -----------------------------------------------------------------------------
