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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The service is never reached when the project cannot be resolved, so these tests pass a
// nil service.
func TestUnknownProject_404(t *testing.T) {
	oc := &clientmocks.OpenChoreoClientMock{
		GetProjectFunc: func(context.Context, string, string) (*models.ProjectResponse, error) {
			return nil, utils.ErrProjectNotFound
		},
	}
	ctrl := NewProjectMCPProxyController(nil, oc)

	cases := map[string]struct {
		method  string
		body    string
		handler http.HandlerFunc
	}{
		"upsert": {http.MethodPut, `{"name":"x","source":{"componentName":"c","endpointName":"e"}}`, ctrl.UpsertProjectMCPProxy},
		"get":    {http.MethodGet, "", ctrl.GetProjectMCPProxy},
		"list":   {http.MethodGet, "", ctrl.ListProjectMCPProxies},
		"delete": {http.MethodDelete, "", ctrl.DeleteProjectMCPProxy},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/x", strings.NewReader(tc.body))
			req.SetPathValue(utils.PathParamProjName, "nope")
			req.SetPathValue(utils.PathParamProxyId, "orders")
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
