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

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// projectBudgetRequest is a budget of project api-platform: each of its agents gets $100.
func projectBudgetRequest(name string) budgetRequest {
	return budgetRequest{
		Name: name, ProjectId: strPtr("api-platform"), DisplayName: strPtr(name),
		Limit: gwspec.BudgetLimit{
			Amount: "100", Period: "month", Action: "block", SplitBy: &gwspec.BudgetSplit{Type: "agent"},
		},
		Filters: []gwspec.BudgetFilter{{Type: "project", Id: strPtr("api-platform")}},
	}
}

func TestBudgetService_AProjectBudgetCountsOnlyItsProject(t *testing.T) {
	env := newBudgetTestEnv(t)
	resp, err := env.create(context.Background(), budgetTestOrg, "alice", projectBudgetRequest("api-platform-agents"))
	require.NoError(t, err)

	require.Len(t, env.created, 1)
	b := env.created[0]
	require.NotNil(t, b.ProjectID)
	assert.Equal(t, budgetTestProjectID, b.ProjectID.String(), "the owning project is stored by ID")
	require.Len(t, b.Configuration.Filters, 1, "the filters as sent: nothing is added")
	project := b.Configuration.Filters[0]
	assert.Equal(t, "project", project.Type)
	assert.Equal(t, "api-platform", *project.ID)
	assert.Equal(t, "agent", b.Configuration.Limit.SplitBy.Type)

	require.NotNil(t, kindProject(resp))
	assert.Equal(t, "api-platform", *kindProject(resp))

	kind := BuildBudgetKind(b.Name, budgetTestOrgName, b.Configuration, true)
	out, err := json.Marshal(kind)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"filters":[{"type":"project","id":"api-platform"}]`,
		"gateways get the project as a filter")
}

func TestBudgetService_AnOrganizationBudgetHasNoProject(t *testing.T) {
	env := newBudgetTestEnv(t)
	resp, err := env.create(context.Background(), budgetTestOrg, "alice", budgetTestRequest())
	require.NoError(t, err)
	assert.Nil(t, env.created[0].ProjectID)
	assert.Nil(t, kindProject(resp))
	out, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "projectId")
}

func TestBudgetService_AProjectBudgetMayNameItsOwnProjectFilter(t *testing.T) {
	env := newBudgetTestEnv(t)
	req := projectBudgetRequest("api-platform-total")
	req.Limit.SplitBy = nil
	_, err := env.create(context.Background(), budgetTestOrg, "alice", req)
	require.NoError(t, err)
	require.Len(t, env.created[0].Configuration.Filters, 1, "not added twice")
}

func TestBudgetService_AProjectBudgetGetsItsProjectFilter(t *testing.T) {
	env := newBudgetTestEnv(t)
	ctx := context.Background()
	sent := projectBudgetRequest("api-platform-agents")
	sent.Filters = []gwspec.BudgetFilter{{Type: "llmProvider", Id: strPtr("openai")}}
	resp, err := env.create(ctx, budgetTestOrg, "alice", sent)
	require.NoError(t, err)

	stored := env.created[0].Configuration.Filters
	require.Len(t, stored, 2)
	assert.Equal(t, "llmProvider", stored[0].Type, "as sent")
	assert.Equal(t, "project", stored[1].Type, "added last")
	assert.Equal(t, "api-platform", *stored[1].ID)
	require.Len(t, resp.Spec.Filters, 2, "the response lists it")
	assert.Equal(t, "project", resp.Spec.Filters[1].Type)

	// The body first sent, without the project filter, can be sent again as an update.
	sent.Limit.Amount = "150"
	updated, err := env.svc.Update(ctx, budgetTestOrg, sent.Name, asKind(sent))
	require.NoError(t, err)
	assert.Equal(t, "150", updated.Spec.Limit.Amount)
}

func TestBudgetService_OwnerRefusals(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(*budgetRequest)
		code  string
		field string
	}{
		{"an empty project annotation", func(r *budgetRequest) { r.ProjectId = strPtr(" ") }, httpx.ErrCodeValidation, "metadata.annotations"},
		{"unknown project", func(r *budgetRequest) { r.ProjectId = strPtr("nowhere") }, gwerr.ErrCodeReferenceNotFound, "metadata.annotations"},
		{"another project's filter", func(r *budgetRequest) {
			r.Filters = []gwspec.BudgetFilter{{Type: "project", Id: strPtr("support")}}
		}, httpx.ErrCodeValidation, "filters[0].id"},
		{"another project's agent", func(r *budgetRequest) {
			r.Limit.SplitBy = nil
			r.Filters = []gwspec.BudgetFilter{{Type: "agent", Project: strPtr("support"), Id: strPtr("triage-bot")}}
		}, httpx.ErrCodeValidation, "filters[0].project"},
		{"an id-less project filter", func(r *budgetRequest) {
			r.Filters = []gwspec.BudgetFilter{{Type: "project"}}
		}, httpx.ErrCodeValidation, "filters[0].id"},
		{"split by project", func(r *budgetRequest) {
			r.Filters = nil
			r.Limit.SplitBy = &gwspec.BudgetSplit{Type: "project"}
		}, httpx.ErrCodeValidation, "limit.splitBy.type"},
		{"exempting a project", func(r *budgetRequest) {
			r.Exemptions = []gwspec.BudgetExemption{{Type: "project", Id: strPtr("api-platform")}}
		}, httpx.ErrCodeValidation, "exemptions[0].type"},
		{"exempting another project's agent", func(r *budgetRequest) {
			r.Exemptions = []gwspec.BudgetExemption{{Type: "agent", Project: strPtr("support"), Id: strPtr("triage-bot")}}
		}, httpx.ErrCodeValidation, "exemptions[0].project"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newBudgetTestEnv(t)
			req := projectBudgetRequest("x")
			tc.edit(&req)
			_, err := env.create(context.Background(), budgetTestOrg, "alice", req)
			var be *BudgetError
			require.ErrorAs(t, err, &be)
			assert.Equal(t, tc.code, be.Code)
			assert.Equal(t, tc.field, be.Field)
			assert.Empty(t, env.created, "nothing is stored")
		})
	}
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

func TestBudgetService_ListsAProjectsBudgets(t *testing.T) {
	env := newBudgetTestEnv(t)
	createTestBudget(t, env, "org-wide")
	_, err := env.create(context.Background(), budgetTestOrg, "alice", projectBudgetRequest("api-platform-agents"))
	require.NoError(t, err)

	all, err := env.svc.List(context.Background(), budgetTestOrg, "", nil, nil, "", "", 20, 0)
	require.NoError(t, err)
	assert.Equal(t, int32(2), all.Total, "the organization sees every budget")

	mine, err := env.svc.List(context.Background(), budgetTestOrg, "api-platform", nil, nil, "", "", 20, 0)
	require.NoError(t, err)
	require.Equal(t, int32(1), mine.Total)
	assert.Equal(t, "api-platform-agents", mine.Budgets[0].Metadata.Name)

	none, err := env.svc.List(context.Background(), budgetTestOrg, "support", nil, nil, "", "", 20, 0)
	require.NoError(t, err)
	assert.Zero(t, none.Total)
}

func TestBudgetService_UpdateKeepsAProjectBudgetsExemptionsInItsProject(t *testing.T) {
	env := newBudgetTestEnv(t)
	_, err := env.create(context.Background(), budgetTestOrg, "alice", projectBudgetRequest("api-platform-agents"))
	require.NoError(t, err)
	update := budgetUpdateRequest{
		DisplayName: strPtr("API Platform agents"),
		Limit:       budgetUpdateLimit{Amount: "150", Action: "block"},
		Exemptions:  []gwspec.BudgetExemption{{Type: "agent", Project: strPtr("api-platform"), Id: strPtr("hotel-booking")}},
	}
	resp, err := env.update(context.Background(), budgetTestOrg, "api-platform-agents", update)
	require.NoError(t, err)
	require.NotNil(t, kindProject(resp))
	assert.Equal(t, "api-platform", *kindProject(resp))
	assert.Len(t, resp.Spec.Filters, 1, "the project filter is kept, not added again")

	update.Exemptions = []gwspec.BudgetExemption{{Type: "agent", Project: strPtr("support"), Id: strPtr("triage-bot")}}
	_, err = env.update(context.Background(), budgetTestOrg, "api-platform-agents", update)
	assert.Equal(t, httpx.ErrCodeValidation, budgetErrorCode(t, err))
}

func TestGatewayResourceSync_TheProjectAnnotationMakesAProjectBudget(t *testing.T) {
	gw := &gwmodels.Gateway{UUID: resourceSyncGateway, OUID: resourceSyncOrg}
	var saved []gwmodels.Budget
	repo := &gwrepomocks.BudgetRepositoryMock{
		SaveForGatewayFunc: func(_ context.Context, b *gwmodels.Budget, name string) (bool, error) {
			b.ID, b.Name = uuid.New(), name
			saved = append(saved, *b)
			return true, nil
		},
	}
	platform := &gwrepomocks.PlatformRepositoryMock{
		GetProjectFunc: func(_ context.Context, ouID, name string) (*gwmodels.ProjectResponse, error) {
			assert.Equal(t, resourceSyncOrg, ouID)
			if name != "api-platform" {
				return nil, gwerr.ErrProjectNotFound
			}
			return &gwmodels.ProjectResponse{UUID: budgetTestProjectID, Name: name}, nil
		},
	}
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
			annotated("dp-no-filter", "unscoped-daily", "api-platform"),
			annotated("dp-unknown", "nowhere-daily", "nowhere"),
			annotated("dp-other", "other-daily", "api-platform", map[string]any{"type": "project", "id": "support"}),
			budgetItem("dp-org", "Budget", "org-daily"),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, gwmodels.GatewayResourceStatusSynced, resp.Resources["dp-project"].Status)
	assert.Equal(t, GatewayResourceErrInvalid, resp.Resources["dp-no-filter"].Error, "a project budget lists its project filter")
	assert.Equal(t, GatewayResourceErrReferenceNotFound, resp.Resources["dp-unknown"].Error)
	assert.Equal(t, GatewayResourceErrInvalid, resp.Resources["dp-other"].Error, "a project budget counts only its project")
	assert.Equal(t, gwmodels.GatewayResourceStatusSynced, resp.Resources["dp-org"].Status)

	require.Len(t, saved, 2)
	byName := map[string]gwmodels.Budget{saved[0].Name: saved[0], saved[1].Name: saved[1]}
	project := byName["api-platform-daily"]
	require.NotNil(t, project.ProjectID)
	assert.Equal(t, budgetTestProjectID, project.ProjectID.String())
	assert.Equal(t, "api-platform", project.ProjectName, "the stored kind keeps its project annotation")
	require.Len(t, project.Configuration.Filters, 1)
	assert.Equal(t, "api-platform", *project.Configuration.Filters[0].ID)
	assert.Nil(t, byName["org-daily"].ProjectID, "no annotation: an organization budget")
}

func TestBudgetService_UpdateRefusesChangesToFixedFields(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		edit        func(*gwspec.Budget)
	}{
		{"rename", "metadata.name", func(k *gwspec.Budget) { k.Metadata.Name = "renamed" }},
		{"move to the organization", "metadata.annotations", func(k *gwspec.Budget) { k.Metadata.Annotations = nil }},
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
	assert.Equal(t, "api-platform", *kindProject(created))
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
	assert.Equal(t, "api-platform", (*resp.Metadata.Annotations)[AnnotationProjectID])
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
	platform := &gwrepomocks.PlatformRepositoryMock{
		GetOrganizationFunc: func(_ context.Context, ouID string) (*gwmodels.OrganizationResponse, error) {
			assert.Equal(t, resourceSyncOrg, ouID)
			return &gwmodels.OrganizationResponse{Name: "wso2", Handle: "wso2"}, nil
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
	assert.Nil(t, env.created[0].ProjectID, "still an organization budget")
	require.Len(t, resp.Spec.Filters, 1)

	sent.Limit.Amount = "60"
	updated, err := env.svc.Update(ctx, budgetTestOrg, sent.Name, asKind(sent))
	require.NoError(t, err)
	assert.Equal(t, "60", updated.Spec.Limit.Amount)
}

func TestBudgetService_AProjectBudgetMayCountAnyOfItsAgents(t *testing.T) {
	env := newBudgetTestEnv(t)
	req := projectBudgetRequest("api-platform-any-agent")
	req.Filters = append(req.Filters, gwspec.BudgetFilter{Type: "agent"})
	req.Limit.SplitBy = nil // one limit for all of them
	_, err := env.create(context.Background(), budgetTestOrg, "alice", req)
	require.NoError(t, err, "an id-less agent filter is any agent of the project")
	stored := env.created[0].Configuration.Filters
	require.Len(t, stored, 2)
	assert.Nil(t, stored[1].ID)
}
