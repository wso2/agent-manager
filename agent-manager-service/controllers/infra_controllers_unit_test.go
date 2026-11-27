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

package controllers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func orgRequest(method, url, body string) *http.Request {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.SetPathValue(utils.PathParamProjName, "proj")
	req.SetPathValue(utils.PathParamAgentName, "agent")
	ctx := middleware.WithResolvedOrg(req.Context(), orgctx.ResolvedOrg{OUID: callerOU, OuHandle: "acme"})
	return req.WithContext(ctx)
}

// --- Environments ---

type stubEnvironments struct {
	services.EnvironmentService
	get       func(envID string) (*models.GatewayEnvironmentResponse, error)
	list      func(limit, offset int32) (*models.EnvironmentListResponse, error)
	instances func() (*models.ThunderInstanceListResponse, error)
}

func (s *stubEnvironments) GetEnvironment(_ context.Context, _ string, envID string) (*models.GatewayEnvironmentResponse, error) {
	return s.get(envID)
}

func (s *stubEnvironments) ListEnvironments(_ context.Context, _ string, limit, offset int32) (*models.EnvironmentListResponse, error) {
	return s.list(limit, offset)
}

func (s *stubEnvironments) ListThunderInstances(context.Context, string) (*models.ThunderInstanceListResponse, error) {
	return s.instances()
}

func TestHandleEnvironmentErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		msg    string
	}{
		{utils.ErrEnvironmentNotFound, http.StatusNotFound, "Environment not found"},
		{utils.ErrResourceBeingDeleted, http.StatusConflict, "An environment with this name is still being deleted, try again shortly"},
		{httpx.ErrConflict, http.StatusConflict, "Environment already exists"},
		{httpx.ErrInvalidInput, http.StatusBadRequest, "Invalid input"},
		{errThunder, http.StatusInternalServerError, "fallback"},
	} {
		w := httptest.NewRecorder()
		HandleEnvironmentErrors(w, fmt.Errorf("wrapped: %w", tc.err), "fallback")
		assert.Equal(t, tc.status, w.Code, tc.err.Error())
		assert.Equal(t, tc.msg, decodeError(t, w).Message)
	}
}

func TestEnvironmentController(t *testing.T) {
	created := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	env := models.GatewayEnvironmentResponse{
		UUID: "env-1", Name: "prod", DisplayName: "Production", IsProduction: true,
		Description: "live", IsolationTier: "dedicated", CreatedAt: created,
	}
	svc := &stubEnvironments{
		get: func(envID string) (*models.GatewayEnvironmentResponse, error) {
			if envID != "env-1" {
				return nil, utils.ErrEnvironmentNotFound
			}
			return &env, nil
		},
		list: func(limit, offset int32) (*models.EnvironmentListResponse, error) {
			assert.Equal(t, int32(5), limit)
			assert.Equal(t, int32(2), offset)
			return &models.EnvironmentListResponse{Environments: []models.GatewayEnvironmentResponse{env}}, nil
		},
		instances: func() (*models.ThunderInstanceListResponse, error) {
			return &models.ThunderInstanceListResponse{ThunderInstances: []models.ThunderInstanceResponse{{EnvName: "prod"}}}, nil
		},
	}
	ctrl := NewEnvironmentController(svc)

	r := orgRequest(http.MethodGet, "/x", "")
	r.SetPathValue("envID", "env-1")
	w := httptest.NewRecorder()
	ctrl.GetEnvironment(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	got := decodeMap(t, w)
	assert.Equal(t, "dedicated", got["isolationTier"])
	assert.Equal(t, true, got["isProduction"])

	r = orgRequest(http.MethodGet, "/x", "")
	r.SetPathValue("envID", "other")
	w = httptest.NewRecorder()
	ctrl.GetEnvironment(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = httptest.NewRecorder()
	ctrl.ListEnvironments(w, orgRequest(http.MethodGet, "/x?limit=5&offset=2", ""))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"name":"prod"`)

	w = httptest.NewRecorder()
	ctrl.ListThunderInstances(w, orgRequest(http.MethodGet, "/x", ""))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"envName":"prod"`)

	failing := NewEnvironmentController(&stubEnvironments{
		list:      func(int32, int32) (*models.EnvironmentListResponse, error) { return nil, errThunder },
		instances: func() (*models.ThunderInstanceListResponse, error) { return nil, errThunder },
	})
	w = httptest.NewRecorder()
	failing.ListEnvironments(w, orgRequest(http.MethodGet, "/x", ""))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	w = httptest.NewRecorder()
	failing.ListThunderInstances(w, orgRequest(http.MethodGet, "/x", ""))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "Failed to list Thunder instances", decodeError(t, w).Message)

	for _, q := range []string{"limit=0", "limit=101", "offset=-1"} {
		w = httptest.NewRecorder()
		failing.ListEnvironments(w, orgRequest(http.MethodGet, "/x?"+q, ""))
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
	}
}

// --- Agent tokens ---

type stubTokens struct {
	gen  func(req services.GenerateTokenRequest) (*spec.TokenResponse, error)
	jwks func() (*spec.JWKS, error)
}

func (s *stubTokens) GenerateToken(_ context.Context, req services.GenerateTokenRequest) (*spec.TokenResponse, error) {
	return s.gen(req)
}

func (s *stubTokens) GetJWKS(context.Context) (*spec.JWKS, error) { return s.jwks() }

func TestAgentTokenController_GenerateToken(t *testing.T) {
	withClaims := func(r *http.Request, ouID string) *http.Request {
		return r.WithContext(jwtassertion.ContextWithTokenClaims(r.Context(), &jwtassertion.TokenClaims{OuId: ouID}))
	}

	t.Run("a token without an org identity is refused", func(t *testing.T) {
		for name, r := range map[string]*http.Request{
			"no claims": orgRequest(http.MethodPost, "/x?project=proj", ""),
			"no OU":     withClaims(orgRequest(http.MethodPost, "/x?project=proj", ""), ""),
		} {
			w := httptest.NewRecorder()
			NewAgentTokenController(&stubTokens{}).GenerateToken(w, r)
			assert.Equal(t, http.StatusForbidden, w.Code, name)
		}
	})

	t.Run("passes the project, agent, environment and expiry through", func(t *testing.T) {
		ctrl := NewAgentTokenController(&stubTokens{gen: func(req services.GenerateTokenRequest) (*spec.TokenResponse, error) {
			assert.Equal(t, services.GenerateTokenRequest{
				OrgName: callerOU, ProjectName: "proj", AgentName: "agent", Environment: "dev", ExpiresIn: "1h", OrgId: callerOU,
			}, req)
			return &spec.TokenResponse{Token: "signed"}, nil
		}})
		w := httptest.NewRecorder()
		ctrl.GenerateToken(w, withClaims(orgRequest(http.MethodPost, "/x?project=proj&environment=dev", `{"expires_in":"1h"}`), callerOU))
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "signed", decodeMap(t, w)["token"])
	})

	t.Run("the project query parameter is required", func(t *testing.T) {
		w := httptest.NewRecorder()
		NewAgentTokenController(&stubTokens{}).GenerateToken(w, withClaims(orgRequest(http.MethodPost, "/x", ""), callerOU))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "project query parameter is required", decodeError(t, w).Message)
	})

	t.Run("a malformed body", func(t *testing.T) {
		w := httptest.NewRecorder()
		NewAgentTokenController(&stubTokens{}).GenerateToken(w, withClaims(orgRequest(http.MethodPost, "/x?project=proj", "{"), callerOU))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	for err, want := range map[error]struct {
		status int
		msg    string
	}{
		utils.ErrOrganizationNotFound: {http.StatusNotFound, "Organization not found"},
		utils.ErrProjectNotFound:      {http.StatusNotFound, "Project not found"},
		utils.ErrAgentNotFound:        {http.StatusNotFound, "Agent not found"},
		utils.ErrEnvironmentNotFound:  {http.StatusNotFound, "Environment not found"},
		fmt.Errorf("%w: expiresIn must be a duration", httpx.ErrInvalidInput): {http.StatusBadRequest, "expiresIn must be a duration"},
		errThunder: {http.StatusInternalServerError, "Failed to generate token"},
	} {
		ctrl := NewAgentTokenController(&stubTokens{gen: func(services.GenerateTokenRequest) (*spec.TokenResponse, error) { return nil, err }})
		w := httptest.NewRecorder()
		ctrl.GenerateToken(w, withClaims(orgRequest(http.MethodPost, "/x?project=proj", ""), callerOU))
		assert.Equal(t, want.status, w.Code, err.Error())
		assert.Contains(t, decodeError(t, w).Message, want.msg)
	}
}

func TestAgentTokenController_GetJWKS(t *testing.T) {
	w := httptest.NewRecorder()
	NewAgentTokenController(&stubTokens{jwks: func() (*spec.JWKS, error) {
		return &spec.JWKS{Keys: []spec.JWK{{Kid: "k1"}}}, nil
	}}).GetJWKS(w, httptest.NewRequest(http.MethodGet, "/auth/external/jwks.json", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"kid":"k1"`)

	w = httptest.NewRecorder()
	NewAgentTokenController(&stubTokens{jwks: func() (*spec.JWKS, error) { return nil, errThunder }}).
		GetJWKS(w, httptest.NewRequest(http.MethodGet, "/auth/external/jwks.json", nil))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "Failed to retrieve JWKS", decodeError(t, w).Message)
}

// --- Projects (failure branches the API cannot reach) ---

type stubProjects struct {
	services.InfraResourceManager
	err error
}

func (s *stubProjects) ListOrganizations(context.Context, int, int) ([]*models.OrganizationResponse, int32, error) {
	return nil, 0, s.err
}

func (s *stubProjects) GetOrganization(context.Context, string) (*models.OrganizationResponse, error) {
	return nil, s.err
}

func (s *stubProjects) ListProjects(context.Context, string, int, int) ([]*models.ProjectResponse, int32, error) {
	return nil, 0, s.err
}

func (s *stubProjects) GetProject(context.Context, string, string) (*models.ProjectResponse, error) {
	return nil, s.err
}

func (s *stubProjects) CreateProject(context.Context, string, spec.CreateProjectRequest) (*models.ProjectResponse, error) {
	return nil, s.err
}

func (s *stubProjects) UpdateProject(context.Context, string, string, spec.UpdateProjectRequest) (*models.ProjectResponse, error) {
	return nil, s.err
}

func (s *stubProjects) DeleteProject(context.Context, string, string) error { return s.err }

func (s *stubProjects) ListOrgEnvironments(context.Context, string) ([]*models.EnvironmentResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []*models.EnvironmentResponse{{UUID: "env-1", Name: "default"}}, nil
}

func TestInfraResourceController_UpstreamFailures(t *testing.T) {
	ctrl := NewInfraResourceController(&stubProjects{err: errThunder}, &recordingCleaner{})
	for name, tc := range map[string]struct {
		h    http.HandlerFunc
		body string
		msg  string
	}{
		"ListOrganizations": {ctrl.ListOrganizations, "", "Failed to list organizations"},
		"GetOrganization":   {ctrl.GetOrganization, "", "Failed to get organization"},
		"ListProjects":      {ctrl.ListProjects, "", "Failed to list projects"},
		"GetProject":        {ctrl.GetProject, "", "Failed to get project"},
		"CreateProject":     {ctrl.CreateProject, projectDoc("p", `{"displayName":"P"}`), "Failed to create project"},
		"UpdateProject":     {ctrl.UpdateProject, projectDoc("proj", `{"displayName":"P"}`), "Failed to update project"},
		"DeleteProject":     {ctrl.DeleteProject, "", "Failed to delete project"},
	} {
		w := httptest.NewRecorder()
		tc.h(w, orgRequest(http.MethodPost, "/x", tc.body))
		assert.Equal(t, http.StatusInternalServerError, w.Code, name)
		assert.Equal(t, tc.msg, decodeError(t, w).Message, name)
	}
}

func TestInfraResourceController_GetProjectOrgNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	NewInfraResourceController(&stubProjects{err: utils.ErrOrganizationNotFound}, nil).GetProject(w, orgRequest(http.MethodGet, "/x", ""))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "Organization not found", decodeError(t, w).Message)
}

func TestInfraResourceController_DeleteProjectRemovesItsSubscriptionsOnlyOnSuccess(t *testing.T) {
	cleaner := &projectCleaner{}
	w := httptest.NewRecorder()
	NewInfraResourceController(&stubProjects{}, cleaner).DeleteProject(w, orgRequest(http.MethodDelete, "/x", ""))
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, []string{callerOU + "/proj"}, cleaner.projects)

	cleaner = &projectCleaner{}
	w = httptest.NewRecorder()
	NewInfraResourceController(&stubProjects{err: utils.ErrProjectNotFound}, cleaner).DeleteProject(w, orgRequest(http.MethodDelete, "/x", ""))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Empty(t, cleaner.projects)
}

func TestInfraResourceController_ListOrgEnvironments(t *testing.T) {
	w := httptest.NewRecorder()
	NewInfraResourceController(&stubProjects{}, nil).ListOrgEnvironments(w, orgRequest(http.MethodGet, "/x", ""))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"name":"default"`)

	w = httptest.NewRecorder()
	NewInfraResourceController(&stubProjects{err: utils.ErrOrganizationNotFound}, nil).ListOrgEnvironments(w, orgRequest(http.MethodGet, "/x", ""))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

type projectCleaner struct{ projects []string }

func (c *projectCleaner) RemoveForProject(_ context.Context, ouID, projectName string) {
	c.projects = append(c.projects, ouID+"/"+projectName)
}

func (c *projectCleaner) RemoveForAgent(context.Context, string, string, string) {}
