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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// projectBudgetRequest is a budget limiting project api-platform's usage: each of its
// agents gets $100. It is an organization budget with a project filter.
func projectBudgetRequest(name string) budgetRequest {
	return budgetRequest{
		Name: name, DisplayName: strPtr(name),
		Limit: gwspec.BudgetLimit{
			Amount: "100", Period: "month", Action: "block", SplitBy: &gwspec.BudgetSplit{Type: "agent"},
		},
		Filters: []gwspec.BudgetFilter{{Type: "project", Id: strPtr("api-platform")}},
	}
}

func TestBudgetService_AnOrganizationBudgetHasNoProject(t *testing.T) {
	env := newBudgetTestEnv(t)
	resp, err := env.create(context.Background(), budgetTestOrg, "alice", budgetTestRequest())
	require.NoError(t, err)
	out, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "projectId")
}

func TestBudgetService_TheOrganizationIsTheOneInThePath(t *testing.T) {
	env := newBudgetTestEnv(t)
	_, err := env.create(context.Background(), budgetTestOrg, "alice", budgetTestRequest())
	require.NoError(t, err)
	assert.Equal(t, budgetTestOrgName, env.created[0].OrgName, "the kind has no organization: it is the path's")
}

func TestBudgetService_RefusesWhatIsNotABudgetKind(t *testing.T) {
	env := newBudgetTestEnv(t)
	for _, tc := range []struct {
		name, field string
		edit        func(*gwspec.Budget)
	}{
		{"another apiVersion", "apiVersion", func(k *gwspec.Budget) { k.ApiVersion = "v2" }},
		{"another kind", "kind", func(k *gwspec.Budget) { k.Kind = "LlmProvider" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind := asKind(budgetTestRequest())
			tc.edit(&kind)
			_, err := env.svc.Create(context.Background(), budgetTestOrg, "alice", kind)
			var be *BudgetError
			require.ErrorAs(t, err, &be)
			assert.Equal(t, tc.field, be.Field)
		})
	}
}

func TestBudgetService_UpdateRefusesChangesToFixedFields(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		edit        func(*gwspec.Budget)
	}{
		{"rename", "metadata.name", func(k *gwspec.Budget) { k.Metadata.Name = "renamed" }},
		{"period", "spec.limit.period", func(k *gwspec.Budget) { k.Spec.Limit.Period = "week" }},
		{"splitBy", "spec.limit.splitBy", func(k *gwspec.Budget) {
			k.Spec.Limit.SplitBy = &gwspec.BudgetSplit{Type: "apiKey"}
		}},
		{"no splitBy", "spec.limit.splitBy", func(k *gwspec.Budget) { k.Spec.Limit.SplitBy = nil }},
		{"filters", "spec.filters", func(k *gwspec.Budget) {
			k.Spec.Filters = append(k.Spec.Filters, gwspec.BudgetFilter{Type: "attribute", Expression: strPtr(budgetGroupExpr)})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newBudgetTestEnv(t)
			created, err := env.create(context.Background(), budgetTestOrg, "alice", projectBudgetRequest("api-platform-agents"))
			require.NoError(t, err)
			kind := *created
			kind.Status = nil
			tc.edit(&kind)
			_, err = env.svc.Update(context.Background(), budgetTestOrg, "api-platform-agents", kind)
			var be *BudgetError
			require.ErrorAs(t, err, &be)
			assert.Equal(t, httpx.ErrCodeValidation, be.Code)
			assert.Equal(t, tc.field, be.Field)
		})
	}
}

func TestBudgetService_TheReturnedKindCanBeSentBack(t *testing.T) {
	env := newBudgetTestEnv(t)
	created, err := env.create(context.Background(), budgetTestOrg, "alice", projectBudgetRequest("api-platform-agents"))
	require.NoError(t, err)
	kind := *created
	kind.Spec.Limit.Amount = "150.00"
	updated, err := env.svc.Update(context.Background(), budgetTestOrg, "api-platform-agents", kind)
	require.NoError(t, err, "the project filter and the defaults round-trip")
	assert.Equal(t, "150.00", updated.Spec.Limit.Amount)
}

func TestBudgetService_ABudgetCanBeCreatedInactive(t *testing.T) {
	env := newBudgetTestEnv(t)
	req := budgetTestRequest()
	req.Status = strPtr("inactive")
	resp, err := env.create(context.Background(), budgetTestOrg, "alice", req)
	require.NoError(t, err)
	assert.Equal(t, "inactive", *resp.Spec.Status)
}

// Every budget names its organization by the org-id annotation, so a budget means the same
// once there are several organizations: one sent without it gets the organization in the
// path, and the response, the stored budget and the gateways' copy carry it.
func TestBudgetService_EveryBudgetNamesItsOrganization(t *testing.T) {
	env := newBudgetTestEnv(t)
	ctx := context.Background()

	resp, err := env.create(ctx, budgetTestOrg, "alice", projectBudgetRequest("no-org"))
	require.NoError(t, err)
	require.NotNil(t, resp.Metadata.Annotations)
	assert.Equal(t, budgetTestOrgName, (*resp.Metadata.Annotations)[AnnotationOrgID], "added when left out")
	assert.Equal(t, map[string]string{AnnotationOrgID: budgetTestOrgName}, *resp.Metadata.Annotations,
		"the project filter is a filter: the budget names only its organization")
	gw := BuildBudgetKind(env.created[0].Name, env.created[0].OrgName, env.created[0].Configuration, true)
	assert.Equal(t, budgetTestOrgName, gw.Metadata.Annotations[AnnotationOrgID], "gateways get it too")

	got, err := env.svc.Get(ctx, budgetTestOrg, "no-org")
	require.NoError(t, err)
	assert.Equal(t, budgetTestOrgName, (*got.Metadata.Annotations)[AnnotationOrgID])

	same := projectBudgetRequest("same-org")
	same.OrgId = strPtr(budgetTestOrgName)
	_, err = env.create(ctx, budgetTestOrg, "alice", same)
	require.NoError(t, err, "naming the organization in the path is accepted")

	org := budgetRequest{Name: "org-wide", Limit: gwspec.BudgetLimit{Amount: "500", Period: "month", Action: "block"}}
	resp, err = env.create(ctx, budgetTestOrg, "alice", org)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{AnnotationOrgID: budgetTestOrgName}, *resp.Metadata.Annotations,
		"an organization budget names only its organization")
}

// A budget naming another organization, or none, is refused, on create and on update.
func TestBudgetService_RefusesAnotherOrganization(t *testing.T) {
	env := newBudgetTestEnv(t)
	ctx := context.Background()
	var refused *BudgetError

	other := projectBudgetRequest("acme-agents")
	other.OrgId = strPtr("acme")
	_, err := env.create(ctx, budgetTestOrg, "alice", other)
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, "metadata.annotations", refused.Field)
	assert.Contains(t, refused.Message, AnnotationOrgID)
	assert.Empty(t, env.created)

	empty := projectBudgetRequest("blank-org")
	empty.OrgId = strPtr("  ")
	_, err = env.create(ctx, budgetTestOrg, "alice", empty)
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, "metadata.annotations", refused.Field)

	_, err = env.create(ctx, budgetTestOrg, "alice", projectBudgetRequest("api-platform-agents"))
	require.NoError(t, err)
	current, err := env.svc.Get(ctx, budgetTestOrg, "api-platform-agents")
	require.NoError(t, err)
	kind := *current
	kind.Status = nil
	(*kind.Metadata.Annotations)[AnnotationOrgID] = "acme"
	_, err = env.svc.Update(ctx, budgetTestOrg, "api-platform-agents", kind)
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, "metadata.annotations", refused.Field)

	delete(*kind.Metadata.Annotations, AnnotationOrgID)
	_, err = env.svc.Update(ctx, budgetTestOrg, "api-platform-agents", kind)
	require.NoError(t, err, "left out on update, it is the organization in the path")
}

// A gateway's pushed budget may name its own organization, or none; naming another fails
// the item.
func TestGatewayResourceSync_ABudgetNamingAnotherOrganizationFails(t *testing.T) {
	gw := &gwmodels.Gateway{UUID: resourceSyncGateway, OUID: resourceSyncOrg}
	var saved []string
	repo := &gwrepomocks.BudgetRepositoryMock{
		SaveForGatewayFunc: func(_ context.Context, b *gwmodels.Budget, name string) (bool, error) {
			b.ID, b.Name = uuid.New(), name
			saved = append(saved, name)
			return true, nil
		},
	}
	platform := &gwtest.Core{
		GetOrgFunc: func(_ context.Context, ouID string) (modkit.Org, error) {
			assert.Equal(t, resourceSyncOrg, ouID)
			return modkit.Org{ID: ouID, Handle: "wso2"}, nil
		},
	}
	withOrg := func(id, name, org string) gwmodels.GatewayResourceItem {
		doc, _ := json.Marshal(map[string]any{
			"apiVersion": "gateway.api-platform.wso2.com/v1", "kind": "Budget",
			"metadata": map[string]any{"name": name, "annotations": map[string]string{AnnotationOrgID: org}},
			"spec":     map[string]any{},
		})
		now := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
		return gwmodels.GatewayResourceItem{DPID: id, Configuration: doc, CreatedAt: now, UpdatedAt: now}
	}
	svc := NewGatewayResourceSyncService(gatewayRepoFor(gw, nil), repo, platform)

	resp, err := svc.Sync(context.Background(), resourceSyncGateway.String(), resourceSyncOrg, gwmodels.GatewayResourcesRequest{
		Resources: []gwmodels.GatewayResourceItem{
			withOrg("dp-own", "own-daily", "wso2"),
			withOrg("dp-other", "other-daily", "acme"),
			budgetItem("dp-none", "Budget", "none-daily"),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, gwmodels.GatewayResourceStatusSynced, resp.Resources["dp-own"].Status)
	assert.Equal(t, GatewayResourceErrInvalid, resp.Resources["dp-other"].Error)
	assert.Equal(t, gwmodels.GatewayResourceStatusSynced, resp.Resources["dp-none"].Status, "left out: the gateway's organization")
	assert.ElementsMatch(t, []string{"own-daily", "none-daily"}, saved)
}

func TestBudgetService_AnAgentFilterGetsNoProjectFilter(t *testing.T) {
	env := newBudgetTestEnv(t)
	ctx := context.Background()
	sent := budgetRequest{
		Name: "hotel-booking-monthly", DisplayName: strPtr("Hotel booking"),
		Limit:   gwspec.BudgetLimit{Amount: "50", Period: "month", Action: "block"},
		Filters: []gwspec.BudgetFilter{{Type: "agent", Project: strPtr("api-platform"), Id: strPtr("hotel-booking")}},
	}
	resp, err := env.create(ctx, budgetTestOrg, "alice", sent)
	require.NoError(t, err)

	stored := env.created[0].Configuration.Filters
	require.Len(t, stored, 1, "the agent carries its project: nothing is added")
	assert.Equal(t, "api-platform", *stored[0].Project)
	require.Len(t, resp.Spec.Filters, 1)

	sent.Limit.Amount = "60"
	updated, err := env.svc.Update(ctx, budgetTestOrg, sent.Name, asKind(sent))
	require.NoError(t, err)
	assert.Equal(t, "60", updated.Spec.Limit.Amount)
}

// A budget belongs to the organization: the project-id annotation, which would make a
// project its owner, is refused on create and on update, whatever it names. To limit a
// project, a budget lists it as a filter.
func TestBudgetService_RefusesTheProjectIDAnnotation(t *testing.T) {
	env := newBudgetTestEnv(t)
	ctx := context.Background()
	annotate := func(k gwspec.Budget, project string) gwspec.Budget {
		annotations := map[string]string{AnnotationProjectID: project}
		k.Metadata.Annotations = &annotations
		return k
	}
	for _, project := range []string{"api-platform", "nowhere", " "} {
		_, err := env.svc.Create(ctx, budgetTestOrg, "alice", annotate(asKind(budgetTestRequest()), project))
		var be *BudgetError
		require.ErrorAs(t, err, &be, project)
		assert.Equal(t, httpx.ErrCodeValidation, be.Code)
		assert.Equal(t, "metadata.annotations", be.Field)
		assert.Contains(t, be.Message, "add a project filter")
	}
	assert.Empty(t, env.created, "nothing is stored")

	created, err := env.create(ctx, budgetTestOrg, "alice", budgetTestRequest())
	require.NoError(t, err)
	kind := annotate(*created, "api-platform")
	kind.Status = nil
	_, err = env.svc.Update(ctx, budgetTestOrg, created.Metadata.Name, kind)
	var be *BudgetError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, "metadata.annotations", be.Field)
}

// A gateway's budget may carry the gateway's own project-id annotation (the gateway adds
// the project filter itself): here it is no owner. The budget is stored as the
// organization's, with its filters as the gateway sent them, and no project is looked up.
func TestGatewayResourceSync_TheProjectAnnotationOwnsNothing(t *testing.T) {
	gw := &gwmodels.Gateway{UUID: resourceSyncGateway, OUID: resourceSyncOrg}
	var saved []gwmodels.Budget
	repo := &gwrepomocks.BudgetRepositoryMock{
		SaveForGatewayFunc: func(_ context.Context, b *gwmodels.Budget, name string) (bool, error) {
			b.ID, b.Name = uuid.New(), name
			saved = append(saved, *b)
			return true, nil
		},
	}
	platform := &gwtest.Core{} // a project lookup would panic
	annotated := func(id, name, project string, filters ...map[string]any) gwmodels.GatewayResourceItem {
		doc, _ := json.Marshal(map[string]any{
			"apiVersion": "gateway.api-platform.wso2.com/v1", "kind": "Budget",
			"metadata": map[string]any{
				"name": name, "annotations": map[string]string{AnnotationProjectID: project},
			},
			"spec": map[string]any{"filters": filters},
		})
		now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
		return gwmodels.GatewayResourceItem{DPID: id, Configuration: doc, CreatedAt: now, UpdatedAt: now}
	}
	svc := NewGatewayResourceSyncService(gatewayRepoFor(gw, nil), repo, platform)

	resp, err := svc.Sync(context.Background(), resourceSyncGateway.String(), resourceSyncOrg, gwmodels.GatewayResourcesRequest{
		Resources: []gwmodels.GatewayResourceItem{
			annotated("dp-project", "api-platform-daily", "api-platform", map[string]any{"type": "project", "id": "api-platform"}),
			annotated("dp-unknown", "nowhere-daily", "nowhere"),
			budgetItem("dp-org", "Budget", "org-daily"),
		},
	})
	require.NoError(t, err)
	for _, id := range []string{"dp-project", "dp-unknown", "dp-org"} {
		assert.Equal(t, gwmodels.GatewayResourceStatusSynced, resp.Resources[id].Status, id)
	}
	require.Len(t, saved, 3)
	byName := map[string]gwmodels.Budget{}
	for _, b := range saved {
		byName[b.Name] = b
		assert.Equal(t, gwmodels.BudgetOriginGateway, b.Origin)
	}
	require.Len(t, byName["api-platform-daily"].Configuration.Filters, 1, "the filters are kept as sent")
	assert.Equal(t, "api-platform", *byName["api-platform-daily"].Configuration.Filters[0].ID)
	assert.Empty(t, byName["nowhere-daily"].Configuration.Filters, "none is added for the annotation")
}
