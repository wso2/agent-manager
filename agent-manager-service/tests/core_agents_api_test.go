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

// agentDocument wraps a flat agent body as an Agent document in project.
func agentDocument(project string, flat map[string]any) map[string]any {
	spec := map[string]any{}
	for k, v := range flat {
		if k != "name" {
			spec[k] = v
		}
	}
	return map[string]any{
		"apiVersion": "ai-control-plane.choreolabs.com/v1",
		"kind":       "Agent",
		"metadata":   map[string]any{"name": flat["name"], "project": project},
		"spec":       spec,
	}
}

// agentNames is the handle of every agent on a list page.
func agentNames(list spec.AgentListResponse) []string {
	names := []string{}
	for _, a := range list.List {
		names = append(names, a.Metadata.Name)
	}
	return names
}

// The agent routes over the real database: create, list with paging and
// label filters, the org-wide listing, get, update, name generation and
// delete, with each refusal's status and code and the organization boundary.
func TestCoreAgentsAPI(t *testing.T) {
	w := newCoreWorld(t)
	mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, projectsPath(""), projectDocument("apps", "Apps", nil))
	mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, projectsPath(""), projectDocument("tools", "Tools", nil))
	agents := agentsPath("")
	inApps := func(name string) string { return agentsPath("/%s?project=apps", name) }

	t.Run("create", func(t *testing.T) {
		a := decodeAs[spec.AgentDocument](t, mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, agents,
			agentDocument("apps", externalA2A("support", "Support", map[string]string{"team": "care"}))))
		assert.Equal(t, "support", a.Metadata.Name)
		assert.Equal(t, "apps", a.Metadata.Project)
		assert.Equal(t, "external", a.Spec.Provisioning.Type)
		mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, agents, agentDocument("apps", externalA2A("billing", "Billing", map[string]string{"team": "money"})))
		mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, agents, agentDocument("apps", externalA2A("triage", "Triage", map[string]string{"team": "care"})))
		mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, agents, agentDocument("tools", externalA2A("linter", "Linter", nil)))
	})

	t.Run("create refusals", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			body   any
			status int
			code   string
		}{
			{"malformed body", "{", http.StatusBadRequest, ""},
			{"a flat body is not a document", externalA2A("flat", "Flat", nil), http.StatusBadRequest, ""},
			{"invalid name", agentDocument("apps", externalA2A("Bad Name", "x", nil)), http.StatusBadRequest, "VALIDATION_ERROR"},
			{"invalid label", agentDocument("apps", externalA2A("labelled", "x", map[string]string{"Bad Key!": "v"})), http.StatusBadRequest, "VALIDATION_ERROR"},
			{"name taken", agentDocument("apps", externalA2A("support", "Again", nil)), http.StatusConflict, utils.ErrCodeAgentAlreadyExists},
			{"unknown project", agentDocument("nowhere", externalA2A("lost", "Lost", nil)), http.StatusNotFound, utils.ErrCodeProjectNotFound},
		} {
			t.Run(tc.name, func(t *testing.T) {
				status, body := call(t, w.admin, http.MethodPost, agents, tc.body)
				require.Equal(t, tc.status, status, string(body))
				if tc.code != "" {
					assert.Equal(t, tc.code, errorOf(t, body).Code)
				}
			})
		}
	})

	t.Run("list pages and filters", func(t *testing.T) {
		all := decodeAs[spec.AgentListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, agents+"?project=apps", nil))
		assert.Equal(t, int32(3), all.Pagination.GetTotal())
		assert.Equal(t, int32(3), all.Count)
		assert.ElementsMatch(t, []string{"support", "billing", "triage"}, agentNames(all), "only this project's agents")

		page := decodeAs[spec.AgentListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, agents+"?project=apps&limit=2&offset=2", nil))
		assert.Len(t, page.List, 1)
		assert.Equal(t, int32(3), page.Pagination.GetTotal())

		care := decodeAs[spec.AgentListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, agents+"?project=apps&label=team:care", nil))
		assert.Equal(t, int32(2), care.Pagination.GetTotal())
		for _, a := range care.List {
			assert.Equal(t, "care", a.Spec.GetLabels()["team"])
		}

		for _, q := range []string{"limit=0", "limit=501", "offset=-1", "label=team"} {
			status, _ := call(t, w.reader, http.MethodGet, agents+"?project=apps&"+q, nil)
			assert.Equal(t, http.StatusBadRequest, status, q)
		}
		status, body := call(t, w.reader, http.MethodGet, agents+"?project=nowhere", nil)
		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, utils.ErrCodeProjectNotFound, errorOf(t, body).Code)
	})

	t.Run("without a project the list spans the org and names each agent's project", func(t *testing.T) {
		org := decodeAs[spec.AgentListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, agents+"?limit=500", nil))
		byName := map[string]spec.AgentDocument{}
		for _, a := range org.List {
			byName[a.Metadata.Name] = a
		}
		require.Len(t, byName, 4)
		assert.Equal(t, "tools", byName["linter"].Metadata.Project)
		assert.Equal(t, int32(4), org.Pagination.GetTotal())
	})

	t.Run("get", func(t *testing.T) {
		a := decodeAs[spec.AgentDocument](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, inApps("support"), nil))
		assert.Equal(t, "Support", a.Spec.DisplayName)
		assert.Equal(t, "about support", a.Spec.GetDescription())
		assert.Equal(t, "external-agent-api", a.Spec.AgentType.Type)
		status, body := call(t, w.reader, http.MethodGet, inApps("ghost"), nil)
		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, utils.ErrCodeAgentNotFound, errorOf(t, body).Code)
		status, _ = call(t, w.reader, http.MethodGet, agentsPath("/support"), nil)
		assert.Equal(t, http.StatusBadRequest, status, "the project is required")
	})

	t.Run("update", func(t *testing.T) {
		current := decodeAs[spec.AgentDocument](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, inApps("support"), nil))
		current.Spec.DisplayName = "Support Desk"
		current.Spec.Description = stringPtr("helps")
		current.Spec.Labels = &map[string]string{"team": "care", "tier": "gold"}
		a := decodeAs[spec.AgentDocument](t, mustCall(t, w.admin, http.StatusOK, http.MethodPut, inApps("support"), current))
		assert.Equal(t, "Support Desk", a.Spec.DisplayName)
		got := decodeAs[spec.AgentDocument](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, inApps("support"), nil))
		assert.Equal(t, "helps", got.Spec.GetDescription(), "the change is stored")
		assert.Equal(t, "gold", got.Spec.GetLabels()["tier"])

		partial := agentDocument("apps", map[string]any{"name": "support", "displayName": "Support Desk"})
		mustCall(t, w.admin, http.StatusOK, http.MethodPut, inApps("support"), partial)
		kept := decodeAs[spec.AgentDocument](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, inApps("support"), nil))
		assert.Equal(t, "helps", kept.Spec.GetDescription(), "an omitted description keeps its value")
		assert.Equal(t, "gold", kept.Spec.GetLabels()["tier"], "omitted labels keep their value")

		changed := agentDocument("apps", map[string]any{"name": "support", "displayName": "x", "provisioning": map[string]any{"type": "internal"}})
		status, _ := call(t, w.admin, http.MethodPut, inApps("support"), changed)
		assert.Equal(t, http.StatusBadRequest, status, "provisioning is fixed at creation")
		status, _ = call(t, w.admin, http.MethodPut, inApps("support"), agentDocument("apps", map[string]any{"name": "support"}))
		assert.Equal(t, http.StatusBadRequest, status)
		status, _ = call(t, w.admin, http.MethodPut, inApps("ghost"), agentDocument("apps", map[string]any{"name": "ghost", "displayName": "x"}))
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
		status, _ := call(t, w.reader, http.MethodPost, agents, agentDocument("apps", externalA2A("sneaky", "Sneaky", nil)))
		assert.Equal(t, http.StatusForbidden, status)
		status, _ = call(t, w.reader, http.MethodDelete, inApps("billing"), nil)
		assert.Equal(t, http.StatusForbidden, status)
		mustCall(t, w.reader, http.StatusNotFound, http.MethodGet, inApps("sneaky"), nil)
		mustCall(t, w.reader, http.StatusOK, http.MethodGet, inApps("billing"), nil)
	})

	t.Run("another organization's token sees none of them", func(t *testing.T) {
		org := decodeAs[spec.AgentListResponse](t, mustCall(t, w.stranger, http.StatusOK, http.MethodGet, agents, nil))
		assert.Empty(t, org.List)
		mustCall(t, w.stranger, http.StatusNotFound, http.MethodGet, inApps("support"), nil)
		mustCall(t, w.stranger, http.StatusNotFound, http.MethodGet, agents+"?project=apps", nil)
	})

	t.Run("delete", func(t *testing.T) {
		mustCall(t, w.admin, http.StatusNoContent, http.MethodDelete, inApps("triage"), nil)
		mustCall(t, w.reader, http.StatusNotFound, http.MethodGet, inApps("triage"), nil)
		list := decodeAs[spec.AgentListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, agents+"?project=apps", nil))
		assert.Equal(t, int32(2), list.Pagination.GetTotal())

		status, body := call(t, w.admin, http.MethodDelete, projectsPath("/apps"), nil)
		assert.Equal(t, http.StatusConflict, status, "a project with agents is not deleted")
		assert.Equal(t, "Project has associated agents", errorOf(t, body).Message)
	})
}
