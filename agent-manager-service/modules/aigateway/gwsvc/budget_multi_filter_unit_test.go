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

package gwsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// Filters of different types are ANDed; filters of one type are alternatives (OR), listed
// by repeating the type. Attribute filters stay ANDed.

func TestBudgetService_FiltersOfOneTypeAreAlternatives(t *testing.T) {
	env := newBudgetTestEnv(t)
	req := budgetTestRequest()
	req.Limit.SplitBy = nil
	req.Filters = []gwspec.BudgetFilter{
		{Type: "llmProvider", Id: strPtr("openai")},
		{Type: "project", Id: strPtr("api-platform")},
		{Type: "project", Id: strPtr("billing")},
		{Type: "agent", Project: strPtr("api-platform"), Id: strPtr("hotel-booking")},
		{Type: "agent", Project: strPtr("billing"), Id: strPtr("invoice-bot")},
		{Type: "attribute", Expression: strPtr(budgetGroupExpr)},
		{Type: "attribute", Expression: strPtr(budgetAdminExpr)},
	}
	req.Warnings = nil
	resp, err := env.create(context.Background(), budgetTestOrg, "alice", req)
	require.NoError(t, err)
	require.Len(t, resp.Spec.Filters, 7, "every entry is kept, in the order listed")
	assert.Equal(t, "billing", *resp.Spec.Filters[2].Id)
	assert.Equal(t, "invoice-bot", *resp.Spec.Filters[4].Id)
	assert.Equal(t, "billing", *resp.Spec.Filters[4].Project)
}

func TestBudgetService_AnIdLessTypeMayBeSplitWhileAnotherTypeHasAlternatives(t *testing.T) {
	env := newBudgetTestEnv(t)
	req := budgetTestRequest()
	req.Warnings = nil
	req.Filters = []gwspec.BudgetFilter{
		{Type: "project", Id: strPtr("api-platform")},
		{Type: "project", Id: strPtr("billing")},
		{Type: "agent"},
	}
	req.Limit.SplitBy = &gwspec.BudgetSplit{Type: "agent"}
	resp, err := env.create(context.Background(), budgetTestOrg, "alice", req)
	require.NoError(t, err)
	assert.Equal(t, "agent", resp.Spec.Limit.SplitBy.Type, "one limit for each agent of either project")
}

func TestBudgetService_AChannelMustSuitEveryProjectTheBudgetNames(t *testing.T) {
	env := newBudgetTestEnv(t)
	// platform-slack is for project api-platform only.
	req := budgetTestRequest()
	req.Warnings = &gwspec.BudgetWarnings{Thresholds: []int32{80}, Channels: []string{"finance-email", "platform-slack"}}
	req.Filters = []gwspec.BudgetFilter{{Type: "project", Id: strPtr("api-platform")}}
	req.Limit.SplitBy = nil
	_, err := env.create(context.Background(), budgetTestOrg, "alice", req)
	require.NoError(t, err, "the one project may use it")

	req = budgetTestRequest()
	req.Name = "two-projects"
	req.Warnings = &gwspec.BudgetWarnings{Thresholds: []int32{80}, Channels: []string{"finance-email", "platform-slack"}}
	req.Filters = []gwspec.BudgetFilter{
		{Type: "project", Id: strPtr("api-platform")},
		{Type: "project", Id: strPtr("billing")},
	}
	req.Limit.SplitBy = nil
	_, err = env.create(context.Background(), budgetTestOrg, "alice", req)
	var be *BudgetError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, gwerr.ErrCodeChannelNotAvailable, be.Code, "billing may not use it, so neither may the budget")
	assert.Equal(t, "warnings.channels[1]", be.Field)

	req.Warnings.Channels = []string{"finance-email"}
	_, err = env.create(context.Background(), budgetTestOrg, "alice", req)
	require.NoError(t, err, "an organization-wide channel suits every project")
}

func TestBudgetProjects_AreEveryProjectTheFiltersName(t *testing.T) {
	str := func(s string) *string { return &s }
	cfg := func(f ...gwmodels.BudgetFilter) *gwmodels.BudgetConfig { return &gwmodels.BudgetConfig{Filters: f} }
	cases := []struct {
		name     string
		cfg      *gwmodels.BudgetConfig
		projects []string
		project  string
	}{
		{"none", cfg(), nil, ""},
		{"one project", cfg(gwmodels.BudgetFilter{Type: "project", ID: str("a")}), []string{"a"}, "a"},
		{"two projects", cfg(gwmodels.BudgetFilter{Type: "project", ID: str("a")}, gwmodels.BudgetFilter{Type: "project", ID: str("b")}), []string{"a", "b"}, ""},
		{"any project", cfg(gwmodels.BudgetFilter{Type: "project"}), nil, ""},
		{"agents of two projects", cfg(
			gwmodels.BudgetFilter{Type: "agent", Project: str("a"), ID: str("x")},
			gwmodels.BudgetFilter{Type: "agent", Project: str("b"), ID: str("y")},
			gwmodels.BudgetFilter{Type: "agent", Project: str("a"), ID: str("z")}), []string{"a", "b"}, ""},
		{"project filters win over agent projects", cfg(
			gwmodels.BudgetFilter{Type: "agent", Project: str("b"), ID: str("y")},
			gwmodels.BudgetFilter{Type: "project", ID: str("a")}), []string{"a"}, "a"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.projects, budgetProjects(tc.cfg), tc.name)
		assert.Equal(t, tc.project, budgetProject(tc.cfg), tc.name)
	}
}

func TestBudgetCountsAgent_Alternatives(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name    string
		filters []gwmodels.BudgetFilter
		want    bool
	}{
		{"one of two projects", []gwmodels.BudgetFilter{{Type: "project", ID: str("support")}, {Type: "project", ID: str("online-shop")}}, true},
		{"neither of two projects", []gwmodels.BudgetFilter{{Type: "project", ID: str("support")}, {Type: "project", ID: str("billing")}}, false},
		{"one of two agents", []gwmodels.BudgetFilter{
			{Type: "agent", Project: str("support"), ID: str("triage")},
			{Type: "agent", Project: str("online-shop"), ID: str("returns-bot")},
		}, true},
		{"neither of two agents", []gwmodels.BudgetFilter{
			{Type: "agent", Project: str("support"), ID: str("triage")},
			{Type: "agent", Project: str("online-shop"), ID: str("checkout")},
		}, false},
		{"any agent of either of two projects", []gwmodels.BudgetFilter{
			{Type: "project", ID: str("support")}, {Type: "project", ID: str("online-shop")}, {Type: "agent"},
		}, true},
		{"any agent of two other projects", []gwmodels.BudgetFilter{
			{Type: "project", ID: str("support")}, {Type: "project", ID: str("billing")}, {Type: "agent"},
		}, false},
	}
	for _, tc := range cases {
		c := &gwmodels.BudgetConfig{Filters: tc.filters}
		assert.Equal(t, tc.want, budgetCountsAgent(c, "online-shop", "returns-bot"), tc.name)
	}
}

func TestNormalizeBudgetFilters_OrGroups(t *testing.T) {
	entries := func(typ string, n int) []gwspec.BudgetFilter {
		out := make([]gwspec.BudgetFilter, n)
		for i := range out {
			id := fmt.Sprintf("%s-%d", typ, i)
			out[i] = gwspec.BudgetFilter{Type: typ, Id: &id}
		}
		return out
	}
	str := func(s string) *string { return &s }

	got, err := normalizeBudgetFilters(entries("project", maxBudgetFilters))
	require.NoError(t, err, "the cap counts every entry, and %d fit", maxBudgetFilters)
	assert.Len(t, got, maxBudgetFilters)

	_, err = normalizeBudgetFilters(entries("project", maxBudgetFilters+1))
	var be *BudgetError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, "filters", be.Field)

	got, err = normalizeBudgetFilters([]gwspec.BudgetFilter{
		{Type: "apiKey", Id: str("k1")},
		{Type: "apiKey", Id: str("k2")},
		{Type: "apiKey", Id: str("k3")},
		{Type: "agent", Project: str("a"), Id: str("x")},
		{Type: "agent", Project: str("b"), Id: str("x")},
	})
	require.NoError(t, err, "the same agent name in two projects is two different agents")
	assert.Len(t, got, 5)

	for _, tc := range []struct {
		name  string
		in    []gwspec.BudgetFilter
		field string
	}{
		{"an apiKey without an id among named ones", []gwspec.BudgetFilter{{Type: "apiKey", Id: str("k1")}, {Type: "apiKey"}}, "filters[1].id"},
		{"an agent without an id among named ones", []gwspec.BudgetFilter{{Type: "agent"}, {Type: "agent", Project: str("a"), Id: str("x")}}, "filters[0].id"},
		{"two id-less projects", []gwspec.BudgetFilter{{Type: "project"}, {Type: "project"}}, "filters[0].id"},
		{"an apiKey listed twice", []gwspec.BudgetFilter{{Type: "apiKey", Id: str("k1")}, {Type: "llmProvider", Id: str("p")}, {Type: "apiKey", Id: str("k1")}}, "filters[2]"},
	} {
		_, err := normalizeBudgetFilters(tc.in)
		var be *BudgetError
		require.ErrorAs(t, err, &be, tc.name)
		assert.Equal(t, httpx.ErrCodeValidation, be.Code, tc.name)
		assert.Equal(t, tc.field, be.Field, tc.name)
	}
}

// A budget a gateway created keeps the filters it was sent, repeated types included: the
// control plane does not check or reorder them.
func TestGatewayResourceSync_KeepsRepeatedFiltersAsSent(t *testing.T) {
	gw := &gwmodels.Gateway{UUID: resourceSyncGateway, OUID: resourceSyncOrg}
	var saved []gwmodels.Budget
	repo := &gwrepomocks.BudgetRepositoryMock{
		SaveForGatewayFunc: func(_ context.Context, b *gwmodels.Budget, name string) (bool, error) {
			b.ID = uuid.New()
			b.Name = name
			saved = append(saved, *b)
			return true, nil
		},
	}
	svc := NewGatewayResourceSyncService(gatewayRepoFor(gw, nil), repo, nil)

	doc, err := json.Marshal(map[string]any{
		"apiVersion": "gateway.api-platform.wso2.com/v1", "kind": "Budget",
		"metadata": map[string]any{"name": "two-projects"},
		"spec": map[string]any{
			"limit": map[string]any{"amount": "10.00", "period": "day", "action": "block"},
			"filters": []map[string]any{
				{"type": "project", "id": "online-shop"},
				{"type": "project", "id": "support"},
				{"type": "llmProvider", "id": "openai"},
			},
		},
	})
	require.NoError(t, err)
	item := budgetItem("dp-or", "Budget", "two-projects")
	item.Configuration = doc

	resp, err := svc.Sync(context.Background(), resourceSyncGateway.String(), resourceSyncOrg,
		gwmodels.GatewayResourcesRequest{Resources: []gwmodels.GatewayResourceItem{item}})
	require.NoError(t, err)
	assert.Equal(t, 1, resp.Success)
	require.Len(t, saved, 1)
	filters := saved[0].Configuration.Filters
	require.Len(t, filters, 3)
	assert.Equal(t, "online-shop", *filters[0].ID)
	assert.Equal(t, "support", *filters[1].ID)
	assert.Equal(t, "llmProvider", filters[2].Type)
}
