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
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/notifyv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// budgetTestNow is Thursday 12 November 2026: in the week from Monday 9 November.
var budgetTestNow = time.Date(2026, 11, 12, 15, 30, 0, 0, time.UTC)

const (
	budgetTestOrg       = "0193f1c2-0000-7000-8000-000000000001"
	budgetTestOrgName   = "wso2"
	budgetTestProjectID = "0193f1c2-0000-7000-8000-0000000000a1"
)

func budgetI32(v int32) *int32 { return &v }

// Gateways identify consumers by token claims (attributes) today.
const (
	budgetGroupExpr = `'research' in request.Auth.Claims["groups"]`
	budgetEmailExpr = `request.Auth.Claims["email"]`
	budgetAdminExpr = `request.Auth.Claims["role"] == "admin"`
)

// recordingSender records the budgets sent to gateways.
type recordingSender struct{ sent []string }

func (r *recordingSender) SendBudget(_ context.Context, b *gwmodels.Budget, created bool) {
	event := "updated"
	if created {
		event = "created"
	}
	r.sent = append(r.sent, event+":"+b.Name+":"+b.Configuration.Status)
}

func (r *recordingSender) SendBudgetDeleted(_ context.Context, b *gwmodels.Budget) {
	r.sent = append(r.sent, "deleted:"+b.Name)
}

// budgetTestRequest is the design's example payload (Interfaces 2.2).
func budgetTestRequest() budgetRequest {
	return budgetRequest{
		Name:        "api-platform-per-agent-openai",
		DisplayName: strPtr("API Platform, per agent on OpenAI"),
		Description: strPtr("Every agent in API Platform gets $100 a month on OpenAI."),
		Limit: gwspec.BudgetLimit{
			Amount: "100.00", Period: "month", Action: "block",
			SplitBy: &gwspec.BudgetSplit{Type: "attribute", Expression: strPtr(budgetEmailExpr)},
		},
		Filters: []gwspec.BudgetFilter{
			{Type: "llmProvider", Id: strPtr("openai")},
			{Type: "attribute", Expression: strPtr(budgetGroupExpr)},
		},
		Exemptions: []gwspec.BudgetExemption{{Type: "attribute", Expression: strPtr(budgetAdminExpr)}},
		Warnings:   &gwspec.BudgetWarnings{Thresholds: []int32{80, 100}, Channels: []string{"finance-email"}},
	}
}

type budgetTestEnv struct {
	svc     BudgetService
	usage   []gwmodels.BudgetUsage
	budgets *gwrepomocks.BudgetRepositoryMock
	created []gwmodels.Budget
	store   map[string]gwmodels.Budget
	sender  *recordingSender
	// channels are the organization's notification channels: finance-email for the whole
	// organization, platform-slack for project api-platform only.
	channels testChannels
}

// testChannels stands in for notifications/v1's channel check.
type testChannels struct{}

func (testChannels) Notify(context.Context, *gorm.DB, notifyv1.Notification) error { return nil }

func (testChannels) Unusable(_ context.Context, _, project string, names []string) ([]notifyv1.Unusable, error) {
	var out []notifyv1.Unusable
	for _, name := range names {
		switch {
		case name == "finance-email":
		case name == "platform-slack" && project == "api-platform":
		case name == "platform-slack":
			out = append(out, notifyv1.Unusable{Name: name})
		default:
			out = append(out, notifyv1.Unusable{Name: name, Missing: true})
		}
	}
	return out, nil
}

// newBudgetTestEnv has: provider openai, proxy support-proxy, project api-platform with
// agent hotel-booking, an org-wide channel and a channel for
// project api-platform only.
// asKind is a request as the Budget kind the API takes.
func asKind(req budgetRequest) gwspec.Budget {
	kind := gwspec.Budget{
		ApiVersion: budgetAPIVersion, Kind: gwmodels.KindBudget, Metadata: gwspec.BudgetMetadata{Name: req.Name},
		Spec: gwspec.BudgetSpec{
			DisplayName: req.DisplayName, Description: req.Description, Status: req.Status, Limit: req.Limit,
			Filters: req.Filters, Exemptions: req.Exemptions, Warnings: req.Warnings,
		},
	}
	annotations := map[string]string{}
	if req.OrgId != nil {
		annotations[AnnotationOrgID] = *req.OrgId
	}
	if req.ProjectId != nil {
		annotations[AnnotationProjectID] = *req.ProjectId
	}
	if len(annotations) > 0 {
		kind.Metadata.Annotations = &annotations
	}
	kind.Metadata.Labels = req.Labels
	return kind
}

// kindProject is a returned budget's project, from its project-id annotation; nil for an
// organization budget.
func kindProject(b *gwspec.Budget) *string {
	if b.Metadata.Annotations == nil {
		return nil
	}
	if p, ok := (*b.Metadata.Annotations)[AnnotationProjectID]; ok {
		return &p
	}
	return nil
}

func (e *budgetTestEnv) create(ctx context.Context, ouID, createdBy string, req budgetRequest) (*gwspec.Budget, error) {
	return e.svc.Create(ctx, ouID, createdBy, asKind(req))
}

// update sends the stored budget as a kind with the update's editable fields, as a client
// that GETs the budget and PUTs it back would.
func (e *budgetTestEnv) update(ctx context.Context, ouID, name string, u budgetUpdateRequest) (*gwspec.Budget, error) {
	current, err := e.svc.Get(ctx, ouID, name)
	if err != nil {
		return nil, err
	}
	kind := *current
	kind.Status = nil
	kind.Spec.DisplayName, kind.Spec.Description, kind.Spec.Status = u.DisplayName, u.Description, u.Status
	kind.Spec.Limit.Amount, kind.Spec.Limit.Action, kind.Spec.Limit.BlockAt = u.Limit.Amount, u.Limit.Action, u.Limit.BlockAt
	kind.Spec.Exemptions, kind.Spec.Warnings = u.Exemptions, u.Warnings
	return e.svc.Update(ctx, ouID, name, kind)
}

func newBudgetTestEnv(t *testing.T) *budgetTestEnv {
	t.Helper()
	env := &budgetTestEnv{store: map[string]gwmodels.Budget{}, sender: &recordingSender{}}
	env.budgets = &gwrepomocks.BudgetRepositoryMock{
		CreateForControlPlaneFunc: func(_ context.Context, b *gwmodels.Budget, name string) error {
			if name == "taken" {
				return gwerr.ErrBudgetAlreadyExists
			}
			b.ID, b.Name = uuid.New(), name
			b.CreatedAt = time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
			b.UpdatedAt = b.CreatedAt
			env.created = append(env.created, *b)
			env.store[name] = *b
			return nil
		},
		GetFunc: func(_ context.Context, _, name string) (*gwmodels.Budget, error) {
			b, ok := env.store[name]
			if !ok {
				return nil, gwerr.ErrBudgetNotFound
			}
			return &b, nil
		},
		UpdateFunc: func(_ context.Context, b *gwmodels.Budget) error {
			b.UpdatedAt = b.CreatedAt.Add(time.Hour)
			env.store[b.Name] = *b
			return nil
		},
		DeleteFunc: func(_ context.Context, _, name string) error {
			delete(env.store, name)
			return nil
		},
		CreateManyForControlPlaneFunc: func(_ context.Context, bs []*gwmodels.Budget, names []string, updates []*gwmodels.Budget) error {
			for i := range bs {
				if _, ok := env.store[names[i]]; ok {
					return &gwrepo.BudgetNameTakenError{Name: names[i]}
				}
			}
			for i, b := range bs {
				b.ID, b.Name = uuid.New(), names[i]
				b.CreatedAt = time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
				b.UpdatedAt = b.CreatedAt
				env.created = append(env.created, *b)
				env.store[names[i]] = *b
			}
			for _, b := range updates {
				env.store[b.Name] = *b
			}
			return nil
		},
		TakenNamesFunc: func(_ context.Context, _ string, names []string) ([]string, error) {
			taken := []string{}
			for _, n := range names {
				if _, ok := env.store[n]; ok || n == "taken" || n == "openai" {
					taken = append(taken, n)
				}
			}
			return taken, nil
		},
		ListFunc: func(context.Context, string) ([]gwmodels.Budget, error) {
			out := []gwmodels.Budget{}
			for _, n := range sortedKinds(env.store) {
				out = append(out, env.store[n])
			}
			return out, nil
		},
	}
	artifacts := &gwrepomocks.ArtifactRepositoryMock{
		ExistsFunc: func(kind, handle, _ string) (bool, error) {
			return kind == gwmodels.KindLLMProvider && handle == "openai", nil
		},
	}
	platform := &gwrepomocks.PlatformRepositoryMock{
		GetProjectFunc: func(_ context.Context, _, name string) (*gwmodels.ProjectResponse, error) {
			if name != "api-platform" {
				return nil, gwerr.ErrProjectNotFound
			}
			return &gwmodels.ProjectResponse{UUID: budgetTestProjectID, Name: name}, nil
		},
		GetOrganizationFunc: func(context.Context, string) (*gwmodels.OrganizationResponse, error) {
			return &gwmodels.OrganizationResponse{Name: budgetTestOrgName, Handle: budgetTestOrgName}, nil
		},
		GetAgentFunc: func(_ context.Context, _, project, name string) (*gwmodels.AgentResponse, error) {
			if project != "api-platform" || name != "hotel-booking" {
				return nil, gwerr.ErrAgentNotFound
			}
			return &gwmodels.AgentResponse{Name: name}, nil
		},
	}
	usageRepo := &gwrepomocks.BudgetUsageRepositoryMock{
		ForBudgetFunc: func(_ context.Context, id uuid.UUID) ([]gwmodels.BudgetUsage, error) {
			out := []gwmodels.BudgetUsage{}
			for _, u := range env.usage {
				if u.BudgetID == id {
					out = append(out, u)
				}
			}
			return out, nil
		},
		ForBudgetsFunc: func(_ context.Context, _ string, ids []uuid.UUID) ([]gwmodels.BudgetUsage, error) {
			out := []gwmodels.BudgetUsage{}
			for _, u := range env.usage {
				for _, id := range ids {
					if u.BudgetID == id {
						out = append(out, u)
					}
				}
			}
			return out, nil
		},
	}
	env.svc = NewBudgetService(env.budgets, artifacts, platform, env.sender, nil, usageRepo, env.channels)
	env.svc.(*budgetService).now = func() time.Time { return budgetTestNow }
	return env
}

func TestBudgetService_CreateStoresTheBudgetKindWithDefaults(t *testing.T) {
	env := newBudgetTestEnv(t)
	resp, err := env.create(context.Background(), budgetTestOrg, "alice", budgetTestRequest())
	require.NoError(t, err)

	require.Len(t, env.created, 1)
	b := env.created[0]
	assert.Equal(t, budgetTestOrg, b.OrgID)
	assert.Equal(t, gwmodels.BudgetOriginControlPlane, b.Origin)
	assert.Nil(t, b.GatewayID)
	assert.Equal(t, "alice", b.CreatedBy)
	stored, err := json.Marshal(gwmodels.NewBudgetDocument(b.Name, b.ProjectName, b.Configuration))
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"apiVersion": "gateway.api-platform.wso2.com/v1", "kind": "Budget",
		"metadata": {"name": "api-platform-per-agent-openai"},
		"spec": {
		"displayName": "API Platform, per agent on OpenAI",
		"description": "Every agent in API Platform gets $100 a month on OpenAI.",
		"status": "active",
		"limit": {
			"amount": "100.00", "currency": "USD", "period": "month", "action": "block", "blockAt": 100,
			"splitBy": {"type": "attribute", "expression": "request.Auth.Claims[\"email\"]"}
		},
		"filters": [
			{"type": "llmProvider", "id": "openai"},
			{"type": "attribute", "expression": "'research' in request.Auth.Claims[\"groups\"]"}
		],
		"exemptions": [{"type": "attribute", "expression": "request.Auth.Claims[\"role\"] == \"admin\""}],
		"warnings": {"thresholds": [80, 100], "channels": ["finance-email"]}
	}}`, string(stored), "the Budget kind, as a gateway stores it")

	assert.Equal(t, []string{"finance-email"}, resp.Spec.Warnings.Channels, "stored and returned by name")
	assert.Equal(t, "api-platform-per-agent-openai", resp.Metadata.Name)
	assert.Equal(t, "active", *resp.Spec.Status)
	assert.Equal(t, "control_plane", resp.Status.Origin)
	assert.Equal(t, "USD", *resp.Spec.Limit.Currency)
	assert.Equal(t, int32(100), *resp.Spec.Limit.BlockAt)
	assert.Equal(t, "alice", *resp.Status.CreatedBy)
	assert.Equal(t, b.CreatedAt, resp.Status.CreatedAt)
	out, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"currentPeriod":{"periodStart":"2026-11-01","spent":"0.00","unpricedRequests":0}`,
		"a new budget has spent nothing this month")
}

func TestBudgetService_CreateAllowsAMinimalBudget(t *testing.T) {
	env := newBudgetTestEnv(t)
	resp, err := env.create(context.Background(), budgetTestOrg, "", budgetRequest{
		Name: "org-monthly", DisplayName: strPtr("Org monthly"),
		Limit: gwspec.BudgetLimit{Amount: "5000", Period: "month", Action: "allow"},
	})
	require.NoError(t, err)
	assert.Nil(t, resp.Spec.Limit.BlockAt, "blockAt is for block only")
	assert.Empty(t, resp.Spec.Filters)
	assert.NotNil(t, resp.Spec.Filters, "filters is always a list")
	assert.Nil(t, resp.Spec.Limit.SplitBy, "no splitBy: one shared limit")
	assert.Nil(t, resp.Status.CreatedBy)
}

func TestBudgetService_CreateRefusesInvalidPayloads(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(*budgetRequest)
		code  string
		field string
	}{
		{"bad name", func(r *budgetRequest) { r.Name = "Team A" }, httpx.ErrCodeValidation, "name"},
		{"long display name", func(r *budgetRequest) { r.DisplayName = strPtr(strings.Repeat("x", 256)) }, httpx.ErrCodeValidation, "displayName"},
		{"zero amount", func(r *budgetRequest) { r.Limit.Amount = "0.00" }, httpx.ErrCodeValidation, "limit.amount"},
		{"three decimals", func(r *budgetRequest) { r.Limit.Amount = "1.005" }, httpx.ErrCodeValidation, "limit.amount"},
		{"exponent amount", func(r *budgetRequest) { r.Limit.Amount = "1e1000000" }, httpx.ErrCodeValidation, "limit.amount"},
		{"filter expression does not compile", func(r *budgetRequest) {
			r.Filters[1].Expression = strPtr("request.Headers[")
		}, gwerr.ErrCodeInvalidExpression, "filters[1].expression"},
		{"filter expression returns a string", func(r *budgetRequest) {
			r.Filters[1].Expression = strPtr("'research'")
		}, gwerr.ErrCodeInvalidExpression, "filters[1].expression"},
		{"splitBy expression returns bool", func(r *budgetRequest) {
			r.Limit.SplitBy.Expression = strPtr("1 == 1")
		}, gwerr.ErrCodeInvalidExpression, "limit.splitBy.expression"},
		{"splitBy expression does not compile", func(r *budgetRequest) {
			r.Limit.SplitBy.Expression = strPtr("request.Auth.Claims[")
		}, gwerr.ErrCodeInvalidExpression, "limit.splitBy.expression"},
		{"exemption expression does not compile", func(r *budgetRequest) {
			r.Exemptions[0].Expression = strPtr("request.Auth.Subject ==")
		}, gwerr.ErrCodeInvalidExpression, "exemptions[0].expression"},
		{"long filter expression", func(r *budgetRequest) {
			r.Filters[1].Expression = strPtr("true || " + strings.Repeat("x", maxBudgetExpression))
		}, httpx.ErrCodeValidation, "filters[1].expression"},
		{"long splitBy expression", func(r *budgetRequest) {
			r.Limit.SplitBy.Expression = strPtr("'x' + " + strings.Repeat("x", maxBudgetExpression))
		}, httpx.ErrCodeValidation, "limit.splitBy.expression"},
		{"long exemption expression", func(r *budgetRequest) {
			r.Exemptions[0].Expression = strPtr("true || " + strings.Repeat("x", maxBudgetExpression))
		}, httpx.ErrCodeValidation, "exemptions[0].expression"},
		{"other currency", func(r *budgetRequest) { r.Limit.Currency = strPtr("EUR") }, httpx.ErrCodeValidation, "limit.currency"},
		{"bad period", func(r *budgetRequest) { r.Limit.Period = "year" }, httpx.ErrCodeValidation, "limit.period"},
		{"bad action", func(r *budgetRequest) { r.Limit.Action = "warn" }, httpx.ErrCodeValidation, "limit.action"},
		{"blockAt out of range", func(r *budgetRequest) { r.Limit.BlockAt = budgetI32(101) }, httpx.ErrCodeValidation, "limit.blockAt"},
		{"blockAt with allow", func(r *budgetRequest) {
			r.Limit.Action, r.Limit.BlockAt = "allow", budgetI32(90)
		}, httpx.ErrCodeValidation, "limit.blockAt"},
		{"unknown splitBy type", func(r *budgetRequest) {
			r.Limit.SplitBy = &gwspec.BudgetSplit{Type: "team"}
		}, httpx.ErrCodeValidation, "limit.splitBy.type"},
		{"attribute splitBy without expression", func(r *budgetRequest) {
			r.Limit.SplitBy.Expression = nil
		}, httpx.ErrCodeValidation, "limit.splitBy.expression"},
		{"agent splitBy with expression", func(r *budgetRequest) {
			r.Limit.SplitBy.Type = "agent"
		}, httpx.ErrCodeValidation, "limit.splitBy.expression"},
		{"splitBy also a filter", func(r *budgetRequest) {
			r.Limit.SplitBy = &gwspec.BudgetSplit{Type: "llmProvider"}
		}, httpx.ErrCodeValidation, "limit.splitBy.type"},
		{"unknown project", func(r *budgetRequest) {
			r.Filters = append(r.Filters, gwspec.BudgetFilter{Type: "project", Id: strPtr("other")})
		}, gwerr.ErrCodeReferenceNotFound, "filters[2].id"},
		{"agent without its project", func(r *budgetRequest) {
			r.Filters = []gwspec.BudgetFilter{{Type: "agent", Id: strPtr("hotel-booking")}}
		}, httpx.ErrCodeValidation, "filters[0].project"},
		{"project on a project filter", func(r *budgetRequest) {
			r.Filters = []gwspec.BudgetFilter{{Type: "project", Id: strPtr("api-platform"), Project: strPtr("api-platform")}}
		}, httpx.ErrCodeValidation, "filters[0].project"},
		{"unknown agent", func(r *budgetRequest) {
			r.Filters = []gwspec.BudgetFilter{{Type: "agent", Project: strPtr("api-platform"), Id: strPtr("nope")}}
		}, gwerr.ErrCodeReferenceNotFound, "filters[0].id"},
		{"agent with only its project", func(r *budgetRequest) {
			r.Filters = []gwspec.BudgetFilter{{Type: "agent", Project: strPtr("api-platform")}}
		}, httpx.ErrCodeValidation, "filters[0].project"},
		{"llmProvider without id", func(r *budgetRequest) { r.Filters[0].Id = nil }, httpx.ErrCodeValidation, "filters[0].id"},
		{"api key with an empty id", func(r *budgetRequest) {
			r.Filters = []gwspec.BudgetFilter{{Type: "apiKey", Id: strPtr(" ")}}
		}, httpx.ErrCodeValidation, "filters[0].id"},
		{"id-less project used twice", func(r *budgetRequest) {
			r.Filters = []gwspec.BudgetFilter{{Type: "project"}, {Type: "project", Id: strPtr("api-platform")}}
		}, httpx.ErrCodeValidation, "filters[1].type"},
		{"project used twice", func(r *budgetRequest) {
			r.Filters = []gwspec.BudgetFilter{{Type: "project", Id: strPtr("api-platform")}, {Type: "project", Id: strPtr("api-platform")}}
		}, httpx.ErrCodeValidation, "filters[1].type"},
		{"unknown filter type", func(r *budgetRequest) {
			r.Filters = []gwspec.BudgetFilter{{Type: "team"}}
		}, httpx.ErrCodeValidation, "filters[0].type"},
		{"attribute without expression", func(r *budgetRequest) {
			r.Filters = append(r.Filters, gwspec.BudgetFilter{Type: "attribute"})
		}, httpx.ErrCodeValidation, "filters[2].expression"},
		{"attribute with id", func(r *budgetRequest) {
			r.Filters[1].Id = strPtr("x")
		}, httpx.ErrCodeValidation, "filters[1].id"},
		{"expression on an id filter", func(r *budgetRequest) {
			r.Filters[0].Expression = strPtr("true")
		}, httpx.ErrCodeValidation, "filters[0].expression"},
		{"too many filters", func(r *budgetRequest) {
			for len(r.Filters) <= maxBudgetFilters {
				r.Filters = append(r.Filters, gwspec.BudgetFilter{Type: "attribute", Expression: strPtr(budgetGroupExpr)})
			}
		}, httpx.ErrCodeValidation, "filters"},
		{"unknown exempt agent", func(r *budgetRequest) {
			r.Exemptions = []gwspec.BudgetExemption{{Type: "agent", Project: strPtr("api-platform"), Id: strPtr("nope")}}
		}, gwerr.ErrCodeReferenceNotFound, "exemptions[0].id"},
		{"attribute exemption without expression", func(r *budgetRequest) {
			r.Exemptions = []gwspec.BudgetExemption{{Type: "attribute"}}
		}, httpx.ErrCodeValidation, "exemptions[0]"},
		{"threshold out of range", func(r *budgetRequest) {
			r.Warnings.Thresholds = []int32{1001}
		}, httpx.ErrCodeValidation, "warnings.thresholds[0]"},
		{"repeated channel", func(r *budgetRequest) {
			r.Warnings.Channels = []string{"finance-email", "finance-email"}
		}, httpx.ErrCodeValidation, "warnings.channels[1]"},
		{"unknown channel", func(r *budgetRequest) {
			r.Warnings.Channels = []string{"pagerduty"}
		}, gwerr.ErrCodeChannelNotAvailable, "warnings.channels[0]"},
		{"project channel without that project", func(r *budgetRequest) {
			r.Filters = []gwspec.BudgetFilter{{Type: "llmProvider", Id: strPtr("openai")}}
			r.Warnings.Channels = []string{"finance-email", "platform-slack"}
		}, gwerr.ErrCodeChannelNotAvailable, "warnings.channels[1]"},
		{"unknown provider", func(r *budgetRequest) { r.Filters[0].Id = strPtr("anthropic") }, gwerr.ErrCodeReferenceNotFound, "filters[0].id"},
		{"name taken", func(r *budgetRequest) { r.Name = "taken" }, gwerr.ErrCodeBudgetAlreadyExists, "name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newBudgetTestEnv(t)
			req := budgetTestRequest()
			tt.edit(&req)
			_, err := env.create(context.Background(), budgetTestOrg, "alice", req)
			var be *BudgetError
			require.ErrorAs(t, err, &be)
			assert.Equal(t, tt.code, be.Code)
			assert.Equal(t, tt.field, be.Field)
			if tt.code != gwerr.ErrCodeBudgetAlreadyExists {
				assert.Empty(t, env.created, "nothing is stored")
			}
			assert.Empty(t, env.sender.sent, "nothing is sent to gateways")
		})
	}
}

func TestBudgetService_CreateKeepsRealErrors(t *testing.T) {
	env := newBudgetTestEnv(t)
	dbDown := errors.New("db down")
	env.budgets.CreateForControlPlaneFunc = func(context.Context, *gwmodels.Budget, string) error { return dbDown }
	_, err := env.create(context.Background(), budgetTestOrg, "alice", budgetTestRequest())
	assert.ErrorIs(t, err, dbDown)
	var be *BudgetError
	assert.False(t, errors.As(err, &be), "a database failure is not a refused request")

	_, err = env.create(context.Background(), "", "alice", budgetTestRequest())
	assert.ErrorIs(t, err, gwerr.ErrOrganizationNotFound)
}

func TestBuildBudgetKind(t *testing.T) {
	env := newBudgetTestEnv(t)
	_, err := env.create(context.Background(), budgetTestOrg, "alice", budgetTestRequest())
	require.NoError(t, err)
	cfg := env.created[0].Configuration

	full, err := json.Marshal(BuildBudgetKind("api-platform-per-agent-openai", budgetTestOrgName, cfg, false))
	require.NoError(t, err)
	assert.Contains(t, string(full), `"apiVersion":"gateway.api-platform.wso2.com/v1","kind":"Budget","metadata":{"name":"api-platform-per-agent-openai","annotations":{"gateway.api-platform.wso2.com/org-id":"wso2"}}`)
	assert.Contains(t, string(full), `"displayName":"API Platform, per agent on OpenAI"`)
	assert.Contains(t, string(full), `"warnings"`)

	gw, err := json.Marshal(BuildBudgetKind("api-platform-per-agent-openai", budgetTestOrgName, cfg, true))
	require.NoError(t, err)
	assert.Contains(t, string(gw), `"annotations":{"gateway.api-platform.wso2.com/org-id":"wso2"}`, "gateways get the organization too")
	for _, absent := range []string{"displayName", "description", "warnings"} {
		assert.NotContains(t, string(gw), `"`+absent+`"`, "gateways never get %s", absent)
	}
	assert.Equal(t, "API Platform, per agent on OpenAI", cfg.DisplayName, "the stored budget is not changed")
}

func TestGatewayBudgetConfigMapsTheKindsSpec(t *testing.T) {
	cfg, project, err := gatewayBudgetConfig("research-monthly", json.RawMessage(`{
		"apiVersion": "gateway.api-platform.wso2.com/v1", "kind": "Budget",
		"metadata": {"name": "research-monthly"},
		"spec": {
			"limit": {"amount": "500.00", "period": "month", "action": "block", "blockAt": 95, "splitBy": {"type": "agent"}},
			"filters": [
				{"type": "llmProvider", "id": "openai"},
				{"type": "attribute", "expression": "'research' in request.Auth.Claims[\"groups\"]"}
			]
		}
	}`))
	require.NoError(t, err)
	assert.Empty(t, project, "no project-id annotation: an organization budget")
	assert.Equal(t, "research-monthly", cfg.DisplayName, "a gateway budget shows its name")
	assert.Equal(t, "active", cfg.Status)
	assert.Equal(t, "USD", cfg.Limit.Currency)
	assert.Equal(t, int32(95), *cfg.Limit.BlockAt)
	assert.Equal(t, &gwmodels.BudgetSplit{Type: "agent"}, cfg.Limit.SplitBy)
	require.Len(t, cfg.Filters, 2)
	assert.Equal(t, "openai", *cfg.Filters[0].ID)
	assert.Equal(t, "attribute", cfg.Filters[1].Type)
	assert.NotNil(t, cfg.Exemptions)

	cfg, _, err = gatewayBudgetConfig("all", json.RawMessage(`{"spec": {"limit": {"amount": "1", "period": "day", "action": "allow"}}}`))
	require.NoError(t, err)
	assert.NotNil(t, cfg.Filters, "no filters: an empty list, all LLM usage")

	_, _, err = gatewayBudgetConfig("x", json.RawMessage(`not json`))
	assert.Error(t, err)
}

// createTestBudget creates the design's example budget, named name.
func createTestBudget(t *testing.T, env *budgetTestEnv, name string) {
	t.Helper()
	req := budgetTestRequest()
	req.Name = name
	_, err := env.create(context.Background(), budgetTestOrg, "alice", req)
	require.NoError(t, err)
}

func budgetErrorCode(t *testing.T, err error) string {
	t.Helper()
	var be *BudgetError
	require.ErrorAs(t, err, &be)
	return be.Code
}

func TestBudgetService_GetReturnsTheBudgetOrNotFound(t *testing.T) {
	env := newBudgetTestEnv(t)
	createTestBudget(t, env, "team-a")
	resp, err := env.svc.Get(context.Background(), budgetTestOrg, "team-a")
	require.NoError(t, err)
	assert.Equal(t, "team-a", resp.Metadata.Name)
	assert.Equal(t, "control_plane", resp.Status.Origin)

	_, err = env.svc.Get(context.Background(), budgetTestOrg, "missing")
	assert.Equal(t, gwerr.ErrCodeBudgetNotFound, budgetErrorCode(t, err))
}

func TestBudgetService_ListFiltersAndPages(t *testing.T) {
	env := newBudgetTestEnv(t)
	for _, n := range []string{"a", "b", "c"} {
		createTestBudget(t, env, n)
	}
	other := env.store["c"]
	other.Configuration.Filters = []gwmodels.BudgetFilter{}
	other.Configuration.Exemptions = []gwmodels.BudgetExemption{}
	env.store["c"] = other

	page, err := env.svc.List(context.Background(), budgetTestOrg, "", nil, nil, "", "", 2, 0)
	require.NoError(t, err)
	assert.Equal(t, int32(3), page.Total)
	assert.Equal(t, int32(2), page.Limit)
	require.Len(t, page.Budgets, 2)
	assert.Equal(t, "a", page.Budgets[0].Metadata.Name)

	page, err = env.svc.List(context.Background(), budgetTestOrg, "", nil, nil, "", "", 0, 2)
	require.NoError(t, err)
	assert.Equal(t, int32(20), page.Limit, "default page size")
	require.Len(t, page.Budgets, 1)
	assert.Equal(t, "c", page.Budgets[0].Metadata.Name)

	page, err = env.svc.List(context.Background(), budgetTestOrg, "", nil, nil, "", "", 20, 10)
	require.NoError(t, err)
	assert.Empty(t, page.Budgets)
	assert.Equal(t, int32(3), page.Total)

	for filter, want := range map[string]int{
		"llmProvider:openai":   2,
		"project:api-platform": 0,
		"agent:hotel-booking":  0,
		"llmProvider:other":    0,
	} {
		page, err = env.svc.List(context.Background(), budgetTestOrg, "", []string{filter}, nil, "", "", 20, 0)
		require.NoError(t, err)
		assert.Len(t, page.Budgets, want, filter)
	}
	page, err = env.svc.List(context.Background(), budgetTestOrg, "", []string{"project:api-platform", "llmProvider:other"}, nil, "", "", 20, 0)
	require.NoError(t, err)
	assert.Empty(t, page.Budgets, "filters combine with AND")

	for _, bad := range [][]string{{"team:a"}, {"project"}, {"project:"}} {
		_, err = env.svc.List(context.Background(), budgetTestOrg, "", bad, nil, "", "", 20, 0)
		assert.Equal(t, httpx.ErrCodeValidation, budgetErrorCode(t, err), bad)
	}
	_, err = env.svc.List(context.Background(), budgetTestOrg, "", nil, nil, "", "", 101, 0)
	assert.Equal(t, httpx.ErrCodeValidation, budgetErrorCode(t, err))
}

func TestBudgetService_UpdateReplacesTheEditableFields(t *testing.T) {
	env := newBudgetTestEnv(t)
	createTestBudget(t, env, "team-a")
	inactive := "inactive"
	resp, err := env.update(context.Background(), budgetTestOrg, "team-a", budgetUpdateRequest{
		DisplayName: strPtr("Team A"),
		Status:      &inactive,
		Limit:       budgetUpdateLimit{Amount: "150.00", Action: "block", BlockAt: budgetI32(90)},
		Warnings:    &gwspec.BudgetWarnings{Thresholds: []int32{100}, Channels: []string{"finance-email"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "Team A", *resp.Spec.DisplayName)
	assert.Equal(t, "inactive", *resp.Spec.Status)
	assert.Nil(t, resp.Spec.Description, "omitted fields are replaced, not kept")
	assert.Empty(t, resp.Spec.Exemptions)
	assert.Equal(t, "150.00", resp.Spec.Limit.Amount)
	assert.Equal(t, int32(90), *resp.Spec.Limit.BlockAt)
	assert.Equal(t, "month", resp.Spec.Limit.Period, "period is fixed")
	assert.Equal(t, "USD", *resp.Spec.Limit.Currency, "currency is fixed")
	assert.Len(t, resp.Spec.Filters, 2, "filters are fixed")
	assert.Equal(t, "openai", *resp.Spec.Filters[0].Id, "filters are fixed")
	assert.Equal(t, "attribute", resp.Spec.Limit.SplitBy.Type, "splitBy is fixed")
	assert.Equal(t, "inactive", env.store["team-a"].Configuration.Status)

	resp, err = env.update(context.Background(), budgetTestOrg, "team-a", budgetUpdateRequest{
		DisplayName: strPtr("Team A"), Limit: budgetUpdateLimit{Amount: "150.00", Action: "allow"},
	})
	require.NoError(t, err)
	assert.Equal(t, "active", *resp.Spec.Status, "status defaults to active")
	assert.Nil(t, resp.Spec.Limit.BlockAt)
}

func TestFixedFieldsUnchanged_FiltersAndSplitBy(t *testing.T) {
	stored := &gwmodels.Budget{Configuration: gwmodels.BudgetConfig{
		Limit:   gwmodels.BudgetLimit{Period: "month"},
		Filters: []gwmodels.BudgetFilter{},
	}}
	request := budgetRequest{Limit: gwspec.BudgetLimit{Period: "month"}}
	require.NoError(t, fixedFieldsUnchanged(stored, request), "no filters: omitted and empty are the same")
	request.Filters = []gwspec.BudgetFilter{}
	require.NoError(t, fixedFieldsUnchanged(stored, request))

	request.Filters = []gwspec.BudgetFilter{{Type: "llmProvider", Id: strPtr("openai")}}
	err := fixedFieldsUnchanged(stored, request)
	require.Error(t, err)
	var be *BudgetError
	require.ErrorAs(t, err, &be)
	assert.Equal(t, "spec.filters", be.Field, "filters cannot change")

	request.Filters = nil
	request.Limit.SplitBy = &gwspec.BudgetSplit{Type: "agent"}
	require.ErrorAs(t, fixedFieldsUnchanged(stored, request), &be)
	assert.Equal(t, "spec.limit.splitBy", be.Field, "splitBy cannot change")

	stored.Configuration.Limit.SplitBy = &gwmodels.BudgetSplit{Type: "agent"}
	require.NoError(t, fixedFieldsUnchanged(stored, request))
}

func TestBudgetService_UpdateRefusals(t *testing.T) {
	ok := func() budgetUpdateRequest {
		return budgetUpdateRequest{DisplayName: strPtr("Team A"), Limit: budgetUpdateLimit{Amount: "150.00", Action: "block"}}
	}
	tests := []struct {
		name   string
		budget string
		edit   func(*budgetUpdateRequest)
		code   string
	}{
		{"not found", "missing", func(*budgetUpdateRequest) {}, gwerr.ErrCodeBudgetNotFound},
		{"gateway budget", "from-gateway", func(*budgetUpdateRequest) {}, gwerr.ErrCodeBudgetReadOnly},
		{"bad status", "team-a", func(r *budgetUpdateRequest) { r.Status = strPtr("paused") }, httpx.ErrCodeValidation},
		{"bad amount", "team-a", func(r *budgetUpdateRequest) { r.Limit.Amount = "0" }, httpx.ErrCodeValidation},
		// Amounts a gateway's schema refuses are refused here too, so none is saved unenforced.
		{"leading zero", "team-a", func(r *budgetUpdateRequest) { r.Limit.Amount = "0100" }, httpx.ErrCodeValidation},
		{"13 whole digits", "team-a", func(r *budgetUpdateRequest) { r.Limit.Amount = "1000000000000" }, httpx.ErrCodeValidation},
		{"blockAt with allow", "team-a", func(r *budgetUpdateRequest) {
			r.Limit.Action, r.Limit.BlockAt = "allow", budgetI32(50)
		}, httpx.ErrCodeValidation},
		{"unknown exempt agent", "team-a", func(r *budgetUpdateRequest) {
			r.Exemptions = []gwspec.BudgetExemption{{Type: "agent", Project: strPtr("api-platform"), Id: strPtr("nope")}}
		}, gwerr.ErrCodeReferenceNotFound},
		{"unknown channel", "team-a", func(r *budgetUpdateRequest) {
			r.Warnings = &gwspec.BudgetWarnings{Thresholds: []int32{80}, Channels: []string{"pagerduty"}}
		}, gwerr.ErrCodeChannelNotAvailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newBudgetTestEnv(t)
			createTestBudget(t, env, "team-a")
			gw := env.store["team-a"]
			gw.Name, gw.Origin = "from-gateway", gwmodels.BudgetOriginGateway
			env.store["from-gateway"] = gw
			before := env.store["team-a"].Configuration

			req := ok()
			tt.edit(&req)
			_, err := env.update(context.Background(), budgetTestOrg, tt.budget, req)
			assert.Equal(t, tt.code, budgetErrorCode(t, err))
			assert.Equal(t, before, env.store["team-a"].Configuration, "nothing is stored")
			assert.Empty(t, env.budgets.UpdateCalls())
		})
	}
}

func TestBudgetService_DeleteRemovesControlPlaneBudgetsOnly(t *testing.T) {
	env := newBudgetTestEnv(t)
	createTestBudget(t, env, "team-a")
	gw := env.store["team-a"]
	gw.Name, gw.Origin = "from-gateway", gwmodels.BudgetOriginGateway
	env.store["from-gateway"] = gw

	require.NoError(t, env.svc.Delete(auditableCtx(t), budgetTestOrg, "team-a"))
	assert.NotContains(t, env.store, "team-a")

	err := env.svc.Delete(auditableCtx(t), budgetTestOrg, "team-a")
	assert.Equal(t, gwerr.ErrCodeBudgetNotFound, budgetErrorCode(t, err))
	err = env.svc.Delete(auditableCtx(t), budgetTestOrg, "from-gateway")
	assert.Equal(t, gwerr.ErrCodeBudgetReadOnly, budgetErrorCode(t, err))
	assert.Contains(t, env.store, "from-gateway")

	dbDown := errors.New("db down")
	env.budgets.GetFunc = func(context.Context, string, string) (*gwmodels.Budget, error) { return nil, dbDown }
	assert.ErrorIs(t, env.svc.Delete(auditableCtx(t), budgetTestOrg, "x"), dbDown, "a real error is not masked as not found")
}

func TestBudgetService_SendsBudgetsToGateways(t *testing.T) {
	env := newBudgetTestEnv(t)
	createTestBudget(t, env, "team-a")
	inactive := "inactive"
	_, err := env.update(context.Background(), budgetTestOrg, "team-a", budgetUpdateRequest{
		DisplayName: strPtr("Team A"), Status: &inactive, Limit: budgetUpdateLimit{Amount: "150.00", Action: "block"},
	})
	require.NoError(t, err)
	require.NoError(t, env.svc.Delete(auditableCtx(t), budgetTestOrg, "team-a"))
	assert.Equal(t, []string{"created:team-a:active", "updated:team-a:inactive", "deleted:team-a"}, env.sender.sent,
		"disabling is an update")

	_, err = env.update(context.Background(), budgetTestOrg, "team-a", budgetUpdateRequest{
		DisplayName: strPtr("x"), Limit: budgetUpdateLimit{Amount: "1", Action: "allow"},
	})
	require.Error(t, err)
	assert.Len(t, env.sender.sent, 3, "a refused request sends nothing")
}

func TestBuildBudgetKind_ForGatewayKeepsTheShape(t *testing.T) {
	cfg := gwmodels.BudgetConfig{
		DisplayName: "Org", Status: "active",
		Limit:   gwmodels.BudgetLimit{Amount: "10", Currency: "USD", Period: "day", Action: "allow", SplitBy: &gwmodels.BudgetSplit{Type: "agent"}},
		Filters: []gwmodels.BudgetFilter{}, Exemptions: []gwmodels.BudgetExemption{},
	}
	gw, err := json.Marshal(BuildBudgetKind("org", budgetTestOrgName, cfg, true))
	require.NoError(t, err)
	assert.Contains(t, string(gw), `"splitBy":{"type":"agent"}`)
	assert.Contains(t, string(gw), `"filters":[]`, "no filters: all LLM usage")
}

func TestBudgetService_GatewayBudgetsTakeOnlyAgentManagerFields(t *testing.T) {
	env := newBudgetTestEnv(t)
	gw := uuid.New()
	env.store["from-gateway"] = gwmodels.Budget{
		ID: uuid.New(), Name: "from-gateway", OrgID: budgetTestOrg, Origin: gwmodels.BudgetOriginGateway, GatewayID: &gw,
		Configuration: gwmodels.BudgetConfig{
			DisplayName: "from-gateway", Status: "active",
			// As a gateway pushes it: no blockAt means block at 100%.
			Limit:      gwmodels.BudgetLimit{Amount: "500.00", Currency: "USD", Period: "month", Action: "block"},
			Filters:    []gwmodels.BudgetFilter{{Type: "attribute", Expression: strPtr(budgetGroupExpr)}},
			Exemptions: []gwmodels.BudgetExemption{},
		},
	}
	same := func() budgetUpdateRequest {
		return budgetUpdateRequest{
			DisplayName: strPtr("Research, monthly"), Description: strPtr("Set on the research gateway."),
			Limit:    budgetUpdateLimit{Amount: "500.00", Action: "block"},
			Warnings: &gwspec.BudgetWarnings{Thresholds: []int32{80, 100}, Channels: []string{"finance-email"}},
		}
	}

	resp, err := env.update(context.Background(), budgetTestOrg, "from-gateway", same())
	require.NoError(t, err)
	assert.Equal(t, "Research, monthly", *resp.Spec.DisplayName)
	assert.Equal(t, "Set on the research gateway.", *resp.Spec.Description)
	require.NotNil(t, resp.Spec.Warnings)
	assert.Equal(t, "gateway_api", resp.Status.Origin)
	stored := env.store["from-gateway"]
	assert.Equal(t, gwmodels.BudgetOriginGateway, stored.Origin)
	assert.Equal(t, "Research, monthly", stored.Configuration.DisplayName)
	assert.Empty(t, env.sender.sent, "fields gateways never get: nothing is sent")

	for name, edit := range map[string]func(*budgetUpdateRequest){
		"amount":  func(r *budgetUpdateRequest) { r.Limit.Amount = "600.00" },
		"blockAt": func(r *budgetUpdateRequest) { r.Limit.BlockAt = budgetI32(90) },
		"action":  func(r *budgetUpdateRequest) { r.Limit.Action = "allow" },
		"status":  func(r *budgetUpdateRequest) { r.Status = strPtr("inactive") },
		"exemptions": func(r *budgetUpdateRequest) {
			r.Exemptions = []gwspec.BudgetExemption{{Type: "attribute", Expression: strPtr(budgetAdminExpr)}}
		},
	} {
		req := same()
		edit(&req)
		_, err := env.update(context.Background(), budgetTestOrg, "from-gateway", req)
		assert.Equal(t, gwerr.ErrCodeBudgetReadOnly, budgetErrorCode(t, err), name)
	}
	assert.Equal(t, "500.00", env.store["from-gateway"].Configuration.Limit.Amount)

	err = env.svc.Delete(auditableCtx(t), budgetTestOrg, "from-gateway")
	assert.Equal(t, gwerr.ErrCodeBudgetReadOnly, budgetErrorCode(t, err), "only the gateway deletes its budget")
}

func TestBudgetService_ProjectAgentAndAPIKeyBudgets(t *testing.T) {
	env := newBudgetTestEnv(t)
	req := budgetTestRequest()
	req.Limit.SplitBy = &gwspec.BudgetSplit{Type: "apiKey"}
	req.Filters = []gwspec.BudgetFilter{
		{Type: "agent", Project: strPtr("api-platform"), Id: strPtr("hotel-booking")},
	}
	req.Exemptions = []gwspec.BudgetExemption{{Type: "attribute", Expression: strPtr(budgetAdminExpr)}}
	resp, err := env.create(context.Background(), budgetTestOrg, "alice", req)
	require.NoError(t, err)
	require.Len(t, resp.Spec.Filters, 1, "an agent filter carries its project: no project filter is added")
	require.NotNil(t, env.created[0].Configuration.Filters[0].Project, "the agent is stored with its project")
	assert.Equal(t, "api-platform", *resp.Spec.Filters[0].Project, "and returned with it")
	assert.Equal(t, "apiKey", resp.Spec.Limit.SplitBy.Type)

	kind := BuildBudgetKind(resp.Metadata.Name, budgetTestOrgName, env.created[0].Configuration, true)
	agent := kind.Spec.Filters[0]
	require.NotNil(t, agent.Project, "gateways get the agent with its project")
	assert.Equal(t, "api-platform", *agent.Project)
	assert.Equal(t, "hotel-booking", *agent.ID)

	for _, filters := range [][]string{
		{"agent:hotel-booking", "project:api-platform"},
		{"project:api-platform"},
	} {
		page, err := env.svc.List(context.Background(), budgetTestOrg, "", filters, nil, "", "", 20, 0)
		require.NoError(t, err)
		assert.Len(t, page.Budgets, 1, "an agent filter's project matches project:%v", filters)
	}

	req = budgetTestRequest()
	req.Name = "api-platform-project"
	req.Limit.SplitBy = nil
	req.Filters = []gwspec.BudgetFilter{{Type: "project", Id: strPtr("api-platform")}, {Type: "apiKey", Id: strPtr("ci-pipeline")}}
	_, err = env.create(context.Background(), budgetTestOrg, "alice", req)
	require.NoError(t, err)
	page, err := env.svc.List(context.Background(), budgetTestOrg, "", []string{"project:api-platform"}, nil, "", "", 20, 0)
	require.NoError(t, err)
	assert.Len(t, page.Budgets, 2, "a project filter matches too")
	page, err = env.svc.List(context.Background(), budgetTestOrg, "", []string{"apiKey:ci-pipeline"}, nil, "", "", 20, 0)
	require.NoError(t, err)
	assert.Len(t, page.Budgets, 1)
}

func spendRow(budgetID uuid.UUID, allowance map[string]string, start time.Time, spent string, unpriced int64) gwmodels.BudgetUsage {
	return gwmodels.BudgetUsage{
		BudgetID: budgetID, AllowanceID: uuid.New(), Allowance: allowance, PeriodStart: start, Spent: spent, UnpricedRequests: unpriced,
	}
}

func TestBudgetPeriodStart(t *testing.T) {
	assert.Equal(t, time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC), budgetPeriodStart("day", budgetTestNow))
	assert.Equal(t, time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC), budgetPeriodStart("week", budgetTestNow), "weeks start Monday")
	sunday := time.Date(2026, 11, 15, 23, 0, 0, 0, time.UTC)
	assert.Equal(t, time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC), budgetPeriodStart("week", sunday), "Sunday ends the week")
	assert.Equal(t, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), budgetPeriodStart("month", budgetTestNow))
	ist := time.FixedZone("IST", 5*3600+1800)
	assert.Equal(t, time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
		budgetPeriodStart("day", time.Date(2026, 11, 1, 2, 0, 0, 0, ist)), "UTC calendar, whatever the caller's zone")
}

func TestBudgetService_CurrentPeriodSumsThisPeriodsSpend(t *testing.T) {
	env := newBudgetTestEnv(t)
	createTestBudget(t, env, "team-a")
	createTestBudget(t, env, "team-b")
	a, b := env.store["team-a"].ID, env.store["team-b"].ID
	november := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	october := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	env.usage = []gwmodels.BudgetUsage{
		spendRow(a, map[string]string{"user": "alice"}, november, "80.144", 2),
		spendRow(a, map[string]string{"user": "bob"}, november, "20.006", 1),
		spendRow(a, map[string]string{"user": "carol"}, october, "99", 7),
		spendRow(b, map[string]string{"user": "dave"}, november, "5", 0),
	}

	got, err := env.svc.Get(context.Background(), budgetTestOrg, "team-a")
	require.NoError(t, err)
	require.NotNil(t, got.Status.CurrentPeriod)
	assert.Equal(t, gwspec.BudgetCurrentPeriod{PeriodStart: "2026-11-01", Spent: "100.15", UnpricedRequests: 3}, *got.Status.CurrentPeriod,
		"October's row counts as $0")

	list, err := env.svc.List(context.Background(), budgetTestOrg, "", nil, nil, "", "", 20, 0)
	require.NoError(t, err)
	spent := map[string]string{}
	for _, r := range list.Budgets {
		spent[r.Metadata.Name] = r.Status.CurrentPeriod.Spent
	}
	assert.Equal(t, map[string]string{"team-a": "100.15", "team-b": "5.00"}, spent)
}

func TestBudgetService_UsageListsAllowancesByHighestSpend(t *testing.T) {
	env := newBudgetTestEnv(t)
	createTestBudget(t, env, "team-a")
	id := env.store["team-a"].ID
	november := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	env.usage = []gwmodels.BudgetUsage{
		spendRow(id, map[string]string{"user": "bob"}, november, "20", 1),
		spendRow(id, map[string]string{"user": "alice"}, november, "80.14", 2),
		spendRow(id, map[string]string{"user": "carol"}, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "99", 7),
		spendRow(id, map[string]string{"user": "erin"}, november, "20", 0),
	}

	page, err := env.svc.Usage(context.Background(), budgetTestOrg, "team-a", 3, 0)
	require.NoError(t, err)
	assert.Equal(t, "2026-11-01", page.PeriodStart)
	assert.Equal(t, int32(4), page.Total)
	require.Len(t, page.Allowances, 3)
	assert.Equal(t, gwspec.BudgetAllowanceUsage{Allowance: map[string]string{"user": "alice"}, Spent: "80.14", UnpricedRequests: 2}, page.Allowances[0])
	assert.Equal(t, "bob", page.Allowances[1].Allowance["user"], "equal spend: ordered by allowance")
	assert.Equal(t, "erin", page.Allowances[2].Allowance["user"])

	last, err := env.svc.Usage(context.Background(), budgetTestOrg, "team-a", 3, 3)
	require.NoError(t, err)
	require.Len(t, last.Allowances, 1)
	assert.Equal(t, gwspec.BudgetAllowanceUsage{Allowance: map[string]string{"user": "carol"}, Spent: "0.00", UnpricedRequests: 0}, last.Allowances[0],
		"a row from an earlier period has spent nothing this period")

	_, err = env.svc.Usage(context.Background(), budgetTestOrg, "nope", 20, 0)
	assert.Equal(t, gwerr.ErrCodeBudgetNotFound, budgetErrorCode(t, err))
	_, err = env.svc.Usage(context.Background(), budgetTestOrg, "team-a", 101, 0)
	assert.Equal(t, httpx.ErrCodeValidation, budgetErrorCode(t, err))
}

func TestBudgetService_ListSearchesStatusAndAgent(t *testing.T) {
	env := newBudgetTestEnv(t)
	ctx := context.Background()
	str := func(s string) *string { return &s }
	for _, n := range []string{"shop-weekly", "payments-daily", "returns-only", "other-agent"} {
		createTestBudget(t, env, n)
	}
	set := func(name, display, status string, filters ...gwmodels.BudgetFilter) {
		b := env.store[name]
		b.Configuration.DisplayName, b.Configuration.Status = display, status
		b.Configuration.Filters, b.Configuration.Exemptions = filters, []gwmodels.BudgetExemption{}
		env.store[name] = b
	}
	set("shop-weekly", "Shop weekly", "active", gwmodels.BudgetFilter{Type: "project", ID: str("online-shop")})
	set("payments-daily", "Payments daily", "inactive")
	set("returns-only", "Returns bot", "active",
		gwmodels.BudgetFilter{Type: "agent", Project: str("online-shop"), ID: str("returns-bot")})
	set("other-agent", "Checkout bot", "active",
		gwmodels.BudgetFilter{Type: "agent", Project: str("online-shop"), ID: str("checkout")})

	names := func(filters []string, search, status string) []string {
		page, err := env.svc.List(ctx, budgetTestOrg, "", filters, nil, search, status, 20, 0)
		require.NoError(t, err)
		out := []string{}
		for _, b := range page.Budgets {
			out = append(out, b.Metadata.Name)
		}
		return out
	}
	assert.Equal(t, []string{"shop-weekly"}, names(nil, " SHOP ", ""), "display name, any case")
	assert.Equal(t, []string{"payments-daily"}, names(nil, "payments-d", ""), "name")
	assert.Equal(t, []string{"payments-daily"}, names(nil, "", "inactive"))
	assert.Equal(t, []string{"other-agent", "returns-only", "shop-weekly"}, names(nil, "", "active"), "by display name")
	assert.Equal(t, []string{"returns-only", "shop-weekly"}, names([]string{"agent:online-shop/returns-bot"}, "", ""),
		"the agent's own budget and its project's budget that names no agent")

	_, err := env.svc.List(ctx, budgetTestOrg, "", nil, nil, "", "paused", 20, 0)
	assert.Equal(t, httpx.ErrCodeValidation, budgetErrorCode(t, err))
}

func TestBudgetService_IdLessFiltersMatchAnyValue(t *testing.T) {
	env := newBudgetTestEnv(t)
	req := budgetTestRequest()
	req.Limit.SplitBy = &gwspec.BudgetSplit{Type: "llmProvider"}
	req.Filters = []gwspec.BudgetFilter{{Type: "project"}, {Type: "agent"}, {Type: "apiKey"}}
	resp, err := env.create(context.Background(), budgetTestOrg, "alice", req)
	require.NoError(t, err, "no id is any project, agent or API key; no reference is checked")
	stored := env.created[0].Configuration.Filters
	require.Len(t, stored, 3)
	for _, f := range stored {
		assert.Nil(t, f.ID, "%s is stored without an id", f.Type)
		assert.Nil(t, f.Project, "%s is stored without a project", f.Type)
	}
	assert.Equal(t, "llmProvider", resp.Spec.Limit.SplitBy.Type)
	assert.Empty(t, budgetProject(&env.created[0].Configuration), "an id-less project filter names no project")

	for _, filter := range []string{"project:api-platform", "agent:hotel-booking", "apiKey:ci-pipeline"} {
		page, err := env.svc.List(context.Background(), budgetTestOrg, "", []string{filter}, nil, "", "", 20, 0)
		require.NoError(t, err)
		assert.Empty(t, page.Budgets, "an id-less filter names no value: %s", filter)
	}
}

func TestBudgetCountsAgent_AnyAgent(t *testing.T) {
	str := func(s string) *string { return &s }
	anyAgent := gwmodels.BudgetFilter{Type: "agent"}
	cases := []struct {
		name    string
		filters []gwmodels.BudgetFilter
		want    bool
	}{
		{"any agent, no project: every agent", []gwmodels.BudgetFilter{anyAgent}, true},
		{"any agent of the agent's project", []gwmodels.BudgetFilter{{Type: "project", ID: str("online-shop")}, anyAgent}, true},
		{"any agent of another project", []gwmodels.BudgetFilter{{Type: "project", ID: str("support")}, anyAgent}, false},
		{"any agent of any project", []gwmodels.BudgetFilter{{Type: "project"}, anyAgent}, true},
		{"any project, no agent filter", []gwmodels.BudgetFilter{{Type: "project"}}, false},
		{"another agent", []gwmodels.BudgetFilter{{Type: "agent", Project: str("online-shop"), ID: str("checkout")}}, false},
	}
	for _, tc := range cases {
		c := &gwmodels.BudgetConfig{Filters: tc.filters}
		assert.Equal(t, tc.want, budgetCountsAgent(c, "online-shop", "returns-bot"), tc.name)
	}
}

func TestBudgetService_AnIdLessFilterMayBeSplitByItsType(t *testing.T) {
	env := newBudgetTestEnv(t)
	req := budgetTestRequest()
	req.Filters = []gwspec.BudgetFilter{{Type: "agent"}}
	req.Limit.SplitBy = &gwspec.BudgetSplit{Type: "agent"}
	resp, err := env.create(context.Background(), budgetTestOrg, "alice", req)
	require.NoError(t, err, "any agent, one limit for each agent")
	assert.Equal(t, "agent", resp.Spec.Limit.SplitBy.Type)
}

func TestBudgetService_LabelsAreStoredFilteredAndSearched(t *testing.T) {
	env := newBudgetTestEnv(t)
	ctx := context.Background()
	req := budgetTestRequest()
	req.Labels = []string{"finance", "prod"}
	resp, err := env.create(ctx, budgetTestOrg, "alice", req)
	require.NoError(t, err)
	assert.Equal(t, []string{"finance", "prod"}, resp.Metadata.Labels)

	raw, err := json.Marshal(gwmodels.NewBudgetDocument(env.created[0].Name, "", env.created[0].Configuration))
	require.NoError(t, err)
	doc := string(raw)
	assert.Contains(t, doc, `"metadata":{"name":"`+env.created[0].Name+`","labels":["finance","prod"]`, "stored under metadata.labels")
	assert.NotContains(t, doc, `"spec":{"labels"`)

	list := func(labels []string, search string) int {
		page, err := env.svc.List(ctx, budgetTestOrg, "", nil, labels, search, "", 20, 0)
		require.NoError(t, err)
		return len(page.Budgets)
	}
	assert.Equal(t, 1, list([]string{"finance"}, ""))
	assert.Equal(t, 1, list([]string{"finance", "prod"}, ""), "every label must match")
	assert.Equal(t, 0, list([]string{"finance", "dev"}, ""))
	assert.Equal(t, 1, list(nil, "FIN"), "search matches a label, any case")

	for _, bad := range [][]string{{"two words"}, {"finance", "finance"}, {"-dash"}, {"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}} {
		r := budgetTestRequest()
		r.Name = "bad-labels"
		r.Labels = bad
		_, err = env.create(ctx, budgetTestOrg, "alice", r)
		assert.Equal(t, httpx.ErrCodeValidation, budgetErrorCode(t, err), "%v", bad)
	}
}

func TestBudgetService_RefusesAnExpressionTooCostlyForGateways(t *testing.T) {
	env := newBudgetTestEnv(t)
	list := "[" + strings.Repeat("1,", 99) + "1]"
	costly := list + ".all(a, " + list + ".all(b, " + list + ".all(c, a == b)))"
	req := budgetTestRequest()
	req.Filters = append(req.Filters, gwspec.BudgetFilter{Type: "attribute", Expression: &costly})
	_, err := env.create(context.Background(), budgetTestOrg, "alice", req)
	assert.Equal(t, gwerr.ErrCodeInvalidExpression, budgetErrorCode(t, err))

	cheap := `'R&D' in request.Auth.Claims["groups"]`
	req = budgetTestRequest()
	req.Filters = append(req.Filters, gwspec.BudgetFilter{Type: "attribute", Expression: &cheap})
	_, err = env.create(context.Background(), budgetTestOrg, "alice", req)
	require.NoError(t, err, "a claim check on an empty request errors, but is not too costly")
}
