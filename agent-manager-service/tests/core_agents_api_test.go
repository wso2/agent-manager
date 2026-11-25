//go:build integration

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

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// externalA2A is a create request for an external A2A agent: one the
// platform neither runs nor publishes through a gateway.
func externalA2A(name, display string, labels map[string]string) map[string]any {
	req := map[string]any{
		"name":         name,
		"displayName":  display,
		"description":  "about " + name,
		"provisioning": map[string]any{"type": "external"},
		"agentType":    map[string]any{"type": "external-agent-api", "subType": "a2a-agent"},
	}
	if labels != nil {
		req["labels"] = labels
	}
	return req
}

// The agent routes over the real database: create, list with paging and
// label filters, the org-wide listing, get, update, name generation and
// delete, with each refusal's status and code and the organization boundary.
func TestCoreAgentsAPI(t *testing.T) {
	w := newCoreWorld(t)
	mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, w.path("/projects"), spec.CreateProjectRequest{Name: "apps", DisplayName: "Apps"})
	mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, w.path("/projects"), spec.CreateProjectRequest{Name: "tools", DisplayName: "Tools"})
	agents := w.path("/projects/apps/agents")

	t.Run("create", func(t *testing.T) {
		a := decodeAs[spec.AgentResponse](t, mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, agents,
			externalA2A("support", "Support", map[string]string{"team": "care"})))
		assert.Equal(t, "support", a.Name)
		assert.Equal(t, "apps", a.ProjectName)
		assert.Equal(t, "external", a.Provisioning.Type)
		mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, agents, externalA2A("billing", "Billing", map[string]string{"team": "money"}))
		mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, agents, externalA2A("triage", "Triage", map[string]string{"team": "care"}))
		mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, w.path("/projects/tools/agents"), externalA2A("linter", "Linter", nil))
	})

	t.Run("create refusals", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			path   string
			body   any
			status int
			code   string
		}{
			{"malformed body", agents, "{", http.StatusBadRequest, ""},
			{"invalid name", agents, externalA2A("Bad Name", "x", nil), http.StatusBadRequest, "VALIDATION_ERROR"},
			{"invalid label", agents, externalA2A("labelled", "x", map[string]string{"Bad Key!": "v"}), http.StatusBadRequest, "VALIDATION_ERROR"},
			{"name taken", agents, externalA2A("support", "Again", nil), http.StatusConflict, utils.ErrCodeAgentAlreadyExists},
			{"unknown project", w.path("/projects/nowhere/agents"), externalA2A("lost", "Lost", nil), http.StatusNotFound, utils.ErrCodeProjectNotFound},
		} {
			t.Run(tc.name, func(t *testing.T) {
				status, body := call(t, w.admin, http.MethodPost, tc.path, tc.body)
				require.Equal(t, tc.status, status, string(body))
				if tc.code != "" {
					assert.Equal(t, tc.code, errorOf(t, body).Code)
				}
			})
		}
	})

	t.Run("list pages and filters", func(t *testing.T) {
		all := decodeAs[spec.AgentListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, agents, nil))
		assert.Equal(t, int32(3), all.Total)
		names := []string{}
		for _, a := range all.Agents {
			names = append(names, a.Name)
		}
		assert.ElementsMatch(t, []string{"support", "billing", "triage"}, names, "only this project's agents")

		page := decodeAs[spec.AgentListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, agents+"?limit=2&offset=2", nil))
		assert.Len(t, page.Agents, 1)
		assert.Equal(t, int32(3), page.Total)

		care := decodeAs[spec.AgentListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, agents+"?label=team:care", nil))
		assert.Equal(t, int32(2), care.Total)
		for _, a := range care.Agents {
			require.NotNil(t, a.Labels)
			assert.Equal(t, "care", (*a.Labels)["team"])
		}

		for _, q := range []string{"limit=0", "limit=101", "offset=-1", "label=team"} {
			status, _ := call(t, w.reader, http.MethodGet, agents+"?"+q, nil)
			assert.Equal(t, http.StatusBadRequest, status, q)
		}
		status, body := call(t, w.reader, http.MethodGet, w.path("/projects/nowhere/agents"), nil)
		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, utils.ErrCodeProjectNotFound, errorOf(t, body).Code)
	})

	t.Run("org-wide listing names each agent's project", func(t *testing.T) {
		org := decodeAs[spec.AgentSummaryListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/agents"), nil))
		byName := map[string]spec.AgentSummary{}
		for _, a := range org.Agents {
			byName[a.Name] = a
		}
		require.Len(t, byName, 4)
		assert.Equal(t, "tools", byName["linter"].ProjectName)
		assert.Equal(t, "Tools", byName["linter"].ProjectDisplayName)
	})

	t.Run("get", func(t *testing.T) {
		a := decodeAs[spec.AgentResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, agents+"/support", nil))
		assert.Equal(t, "Support", a.DisplayName)
		assert.Equal(t, "about support", a.Description)
		status, body := call(t, w.reader, http.MethodGet, agents+"/ghost", nil)
		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, utils.ErrCodeAgentNotFound, errorOf(t, body).Code)
	})

	t.Run("update", func(t *testing.T) {
		labels := map[string]string{"team": "care", "tier": "gold"}
		a := decodeAs[spec.AgentResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodPut, agents+"/support",
			spec.UpdateAgentBasicInfoRequest{DisplayName: "Support Desk", Description: "helps", Labels: &labels}))
		assert.Equal(t, "Support Desk", a.DisplayName)
		got := decodeAs[spec.AgentResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, agents+"/support", nil))
		assert.Equal(t, "helps", got.Description, "the change is stored")
		require.NotNil(t, got.Labels)
		assert.Equal(t, "gold", (*got.Labels)["tier"])

		status, _ := call(t, w.admin, http.MethodPut, agents+"/support", spec.UpdateAgentBasicInfoRequest{})
		assert.Equal(t, http.StatusBadRequest, status)
		status, _ = call(t, w.admin, http.MethodPut, agents+"/ghost", spec.UpdateAgentBasicInfoRequest{DisplayName: "x"})
		assert.Equal(t, http.StatusNotFound, status)
	})

	t.Run("generate a free name", func(t *testing.T) {
		free := decodeAs[spec.ResourceNameResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodPost, w.path("/utils/generate-name"),
			spec.ResourceNameRequest{DisplayName: "Fresh Agent!", ResourceType: "agent", ProjectName: stringPtr("apps")}))
		assert.Equal(t, "fresh-agent", free.Name)

		taken := decodeAs[spec.ResourceNameResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodPost, w.path("/utils/generate-name"),
			spec.ResourceNameRequest{DisplayName: "Support", ResourceType: "agent", ProjectName: stringPtr("apps")}))
		assert.NotEqual(t, "support", taken.Name, "a taken name gets a suffix")
		assert.Regexp(t, `^support-[a-z0-9]+$`, taken.Name)

		project := decodeAs[spec.ResourceNameResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodPost, w.path("/utils/generate-name"),
			spec.ResourceNameRequest{DisplayName: "Apps", ResourceType: "project"}))
		assert.NotEqual(t, "apps", project.Name)

		status, _ := call(t, w.admin, http.MethodPost, w.path("/utils/generate-name"), spec.ResourceNameRequest{DisplayName: "x", ResourceType: "planet"})
		assert.Equal(t, http.StatusBadRequest, status)
	})

	t.Run("a reader cannot change agents", func(t *testing.T) {
		status, _ := call(t, w.reader, http.MethodPost, agents, externalA2A("sneaky", "Sneaky", nil))
		assert.Equal(t, http.StatusForbidden, status)
		status, _ = call(t, w.reader, http.MethodDelete, agents+"/billing", nil)
		assert.Equal(t, http.StatusForbidden, status)
		mustCall(t, w.reader, http.StatusNotFound, http.MethodGet, agents+"/sneaky", nil)
		mustCall(t, w.reader, http.StatusOK, http.MethodGet, agents+"/billing", nil)
	})

	t.Run("another organization's token sees none of them", func(t *testing.T) {
		org := decodeAs[spec.AgentSummaryListResponse](t, mustCall(t, w.stranger, http.StatusOK, http.MethodGet, w.path("/agents"), nil))
		assert.Empty(t, org.Agents)
		mustCall(t, w.stranger, http.StatusNotFound, http.MethodGet, agents+"/support", nil)
		mustCall(t, w.stranger, http.StatusNotFound, http.MethodGet, agents, nil)
	})

	t.Run("delete", func(t *testing.T) {
		mustCall(t, w.admin, http.StatusNoContent, http.MethodDelete, agents+"/triage", nil)
		mustCall(t, w.reader, http.StatusNotFound, http.MethodGet, agents+"/triage", nil)
		list := decodeAs[spec.AgentListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, agents, nil))
		assert.Equal(t, int32(2), list.Total)

		status, body := call(t, w.admin, http.MethodDelete, w.path("/projects/apps"), nil)
		assert.Equal(t, http.StatusConflict, status, "a project with agents is not deleted")
		assert.Equal(t, "Project has associated agents", errorOf(t, body).Message)
	})
}
