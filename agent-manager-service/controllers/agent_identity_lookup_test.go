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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// lookupAgentService fails the roles and groups lookups with one error.
type lookupAgentService struct {
	services.AgentManagerService
	err error
}

func (s lookupAgentService) GetAgentRoles(context.Context, string, string, string, string) ([]thundersvc.ThunderRole, error) {
	return nil, s.err
}

func (s lookupAgentService) GetAgentGroups(context.Context, string, string, string, string) ([]thundersvc.ThunderGroup, error) {
	return nil, s.err
}

func agentLookupRequest(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path+"?environment=qa", nil)
	req.SetPathValue("orgName", "o1")
	req.SetPathValue("projName", "default")
	req.SetPathValue("agentName", "scout-agent")
	return req.WithContext(middleware.WithResolvedOrg(req.Context(), middleware.ResolvedOrg{OUID: "ou-org"}))
}

// An environment with no identity provider holds no roles or groups for the agent: that answers 404
// with a message, not the 500 it used to, which the console showed as a failure.
func TestAgentRolesInAnEnvironmentWithoutAnIdentityProvider(t *testing.T) {
	cases := map[string]struct {
		err  error
		want int
	}{
		"no identity provider": {fmt.Errorf("resolve: %w", thundersvc.ErrThunderNotProvisioned), http.StatusNotFound},
		"no role provider":     {services.ErrIdentityRoleUnavailable, http.StatusServiceUnavailable},
		"anything else":        {errors.New("boom"), http.StatusInternalServerError},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := NewAgentController(lookupAgentService{err: test.err}, nil)
			w := httptest.NewRecorder()
			ctrl.GetAgentRoles(w, agentLookupRequest("/orgs/o1/projects/default/agents/scout-agent/roles"))
			assert.Equal(t, test.want, w.Code)
			assert.NotContains(t, w.Body.String(), "boom")
		})
	}
}

func TestAgentGroupsInAnEnvironmentWithoutAnIdentityProvider(t *testing.T) {
	cases := map[string]struct {
		err  error
		want int
	}{
		"no identity provider": {fmt.Errorf("resolve: %w", thundersvc.ErrThunderNotProvisioned), http.StatusNotFound},
		"no group provider":    {services.ErrIdentityGroupUnavailable, http.StatusServiceUnavailable},
		"anything else":        {errors.New("boom"), http.StatusInternalServerError},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := NewAgentController(lookupAgentService{err: test.err}, nil)
			w := httptest.NewRecorder()
			ctrl.GetAgentGroups(w, agentLookupRequest("/orgs/o1/projects/default/agents/scout-agent/groups"))
			assert.Equal(t, test.want, w.Code)
			assert.NotContains(t, w.Body.String(), "boom")
		})
	}
}
