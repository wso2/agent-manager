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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// storedProxyService is the project proxy service over a repository holding proxies (handle -> proxy). It records
// the (org, project name) of every lookup by stored project name.
func storedProxyService(oc *clientmocks.OpenChoreoClientMock, lookups *[]string, proxies ...*models.MCPProxy) *services.ProjectMCPProxyService {
	repo := &repomocks.MCPProxyRepositoryMock{
		GetInProjectByNameFunc: func(_ context.Context, handle, ouID, projectName string) (*models.MCPProxy, error) {
			*lookups = append(*lookups, ouID+"/"+projectName)
			for _, p := range proxies {
				if p.Artifact.Handle == handle && p.ProjectUUID != nil && *p.ProjectName == projectName && ouID == "ou" {
					return p, nil
				}
			}
			return nil, utils.ErrMCPProxyNotFound
		},
		ListFunc: func(_ context.Context, ouID string, f repositories.MCPProxyListFilter) ([]*models.MCPProxy, int64, error) {
			*lookups = append(*lookups, ouID+"/"+*f.ProjectName)
			out := []*models.MCPProxy{}
			for _, p := range proxies {
				if f.ProjectUUID == nil && p.ProjectUUID != nil && *p.ProjectName == *f.ProjectName && ouID == "ou" {
					out = append(out, p)
				}
			}
			return out, int64(len(out)), nil
		},
	}
	base := services.NewMCPProxyService(nil, repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, slog.New(slog.DiscardHandler), nil,
		nil, nil, nil, nil, oc)
	return services.NewProjectMCPProxyService(base, oc)
}

func projectRouteRequest(method, body string) *http.Request {
	req := httptest.NewRequest(method, "/x", strings.NewReader(body))
	req.SetPathValue(utils.PathParamProjName, "p2")
	req.SetPathValue(utils.PathParamProxyId, "orders")
	req.SetPathValue("envName", "development")
	return req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou", OuHandle: "org"}))
}

// A project OpenChoreo no longer has still serves what it published: the read and delete routes reach the proxies
// stored under its name, in the caller's org only; PUT still refuses, since nothing can be deployed into it.
func TestDeletedProject_ReadAndDeleteRoutesReachStoredProxies(t *testing.T) {
	oc := &clientmocks.OpenChoreoClientMock{
		GetProjectFunc: func(context.Context, string, string) (*models.ProjectResponse, error) {
			return nil, fmt.Errorf("project p2: %w", utils.ErrProjectNotFound)
		},
	}
	staleUUID, projName := "deleted-uuid", "p2"
	orphan := &models.MCPProxy{
		ProjectUUID: &staleUUID, ProjectName: &projName,
		Artifact:      &models.Artifact{Handle: "orders"},
		Configuration: models.MCPProxyConfig{Name: "Orders"},
	}
	var lookups []string
	ctrl := NewProjectMCPProxyController(storedProxyService(oc, &lookups, orphan), oc)

	rec := httptest.NewRecorder()
	ctrl.GetProjectMCPProxy(rec, projectRouteRequest(http.MethodGet, ""))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got struct{ ID, ProjectName string }
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	assert.Equal(t, "orders", got.ID)
	assert.Equal(t, "p2", got.ProjectName)

	rec = httptest.NewRecorder()
	ctrl.ListProjectMCPProxies(rec, projectRouteRequest(http.MethodGet, ""))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var list struct{ Count int }
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&list))
	assert.Equal(t, 1, list.Count)

	rec = httptest.NewRecorder()
	ctrl.GetProjectMCPProxyEnvironment(rec, projectRouteRequest(http.MethodGet, ""))
	assert.Equal(t, http.StatusNotFound, rec.Code, "the proxy is reached; it has no binding in the environment")
	assert.Contains(t, rec.Body.String(), utils.ErrMCPProxyEnvironmentNotBound.Error())

	assert.Equal(t, []string{"ou/p2", "ou/p2", "ou/p2"}, lookups, "looked up in the caller's org by the path's project name")

	for name, tc := range map[string]struct {
		body    string
		handler http.HandlerFunc
	}{
		"upsert":  {`{"name":"x","source":{"componentName":"c","endpointName":"e"}}`, ctrl.UpsertProjectMCPProxy},
		"put env": {`{}`, ctrl.PutProjectMCPProxyEnvironment},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.handler(rec, projectRouteRequest(http.MethodPut, tc.body))
			assert.Equal(t, http.StatusNotFound, rec.Code)
			var body struct{ Code string }
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
			assert.Equal(t, utils.ErrCodeProjectNotFound, body.Code)
		})
	}
	assert.Len(t, lookups, 3, "PUT never reaches the stored proxies")
}

// A project that does not exist and left nothing behind is 404 PROJECT_NOT_FOUND on every route.
func TestUnknownProject_404(t *testing.T) {
	oc := &clientmocks.OpenChoreoClientMock{
		GetProjectFunc: func(context.Context, string, string) (*models.ProjectResponse, error) {
			return nil, utils.ErrProjectNotFound
		},
	}
	var lookups []string
	ctrl := NewProjectMCPProxyController(storedProxyService(oc, &lookups), oc)

	cases := map[string]struct {
		method  string
		body    string
		handler http.HandlerFunc
	}{
		"upsert":  {http.MethodPut, `{"name":"x","source":{"componentName":"c","endpointName":"e"}}`, ctrl.UpsertProjectMCPProxy},
		"get":     {http.MethodGet, "", ctrl.GetProjectMCPProxy},
		"list":    {http.MethodGet, "", ctrl.ListProjectMCPProxies},
		"delete":  {http.MethodDelete, "", ctrl.DeleteProjectMCPProxy},
		"put env": {http.MethodPut, `{}`, ctrl.PutProjectMCPProxyEnvironment},
		"get env": {http.MethodGet, "", ctrl.GetProjectMCPProxyEnvironment},
		"del env": {http.MethodDelete, "", ctrl.DeleteProjectMCPProxyEnvironment},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/x", strings.NewReader(tc.body))
			req.SetPathValue(utils.PathParamProjName, "nope")
			req.SetPathValue(utils.PathParamProxyId, "orders")
			req.SetPathValue("envName", "development")
			req = req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou", OuHandle: "org"}))
			rec := httptest.NewRecorder()

			tc.handler(rec, req)

			assert.Equal(t, http.StatusNotFound, rec.Code)
			var body struct{ Code string }
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
			assert.Equal(t, utils.ErrCodeProjectNotFound, body.Code)
		})
	}
}

func TestBindingErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("%w: x", utils.ErrMCPUpstreamNotReady), http.StatusConflict, utils.ErrCodeUpstreamNotReady},
		{fmt.Errorf("%w: x", utils.ErrMCPContextInUse), http.StatusConflict, utils.ErrCodeContextInUse},
		{utils.ErrMCPProxyNotFound, http.StatusNotFound, utils.ErrCodeNotFound},
		{utils.ErrMCPProxyEnvironmentNotBound, http.StatusNotFound, utils.ErrCodeNotFound},
		{fmt.Errorf("%w: x", utils.ErrEnvironmentNotFound), http.StatusNotFound, utils.ErrCodeEnvironmentNotFound},
		{fmt.Errorf("%w: %w", utils.ErrInvalidInput, utils.ErrMCPJWTSecurityNotAvailable), http.StatusBadRequest, utils.ErrCodeValidation},
		{utils.ErrMCPProxyHasMappings, http.StatusConflict, utils.ErrCodeMCPProxyInUse},
		{fmt.Errorf("%w: %w: two gateways", utils.ErrInvalidInput, utils.ErrMCPAmbiguousGateway), http.StatusBadRequest, utils.ErrCodeAmbiguousGateway},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		require.True(t, writeProjectMCPBindingError(rec, tc.err))
		assert.Equal(t, tc.status, rec.Code, tc.err.Error())
		var body struct{ Code, Message string }
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
		assert.Equal(t, tc.code, body.Code)
		if errors.Is(tc.err, utils.ErrMCPJWTSecurityNotAvailable) {
			assert.Equal(t, "jwt security is not available yet", body.Message)
		}
	}
	assert.False(t, writeProjectMCPBindingError(httptest.NewRecorder(), errors.New("boom")), "unexpected errors are left to the caller (500)")
}

func TestProjectLookupFailure_500(t *testing.T) {
	oc := &clientmocks.OpenChoreoClientMock{
		GetProjectFunc: func(context.Context, string, string) (*models.ProjectResponse, error) {
			return nil, errors.New("openchoreo down")
		},
	}
	ctrl := NewProjectMCPProxyController(nil, oc)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.SetPathValue(utils.PathParamProjName, "p")
	req = req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou", OuHandle: "org"}))
	rec := httptest.NewRecorder()

	ctrl.ListProjectMCPProxies(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
