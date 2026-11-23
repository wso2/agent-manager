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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// importCSV is the design's example (Interfaces 2.5), in the test environment's names.
const importCSV = `name,projectId,displayName,amount,period,action,blockAt,splitBy,splitByExpression,llmProvider,project,agent,apiKey,attributes,exemptions,thresholds,channels
wso2-monthly,,WSO2 monthly ceiling,10000,month,block,99,,,,,,,,"[{""type"": ""project"", ""id"": ""api-platform""}]",50;80;100,finance-email
api-platform-agents,api-platform,"API Platform, per agent",100,month,block,,agent,,openai,,,,,"[{""type"": ""agent"", ""project"": ""api-platform"", ""id"": ""hotel-booking""}]",80;100,platform-slack
api-platform-cap,,API Platform cap,500,month,block,,llmProvider,,,api-platform,,,,,80,finance-email
rnd-per-user,,"R&D, per user",20,day,block,,attribute,"request.Auth.Claims[""email""]",,,,,"[""'research' in request.Auth.Claims[\""groups\""]""]",,80;100,finance-email
ci-key-daily,,,20,day,block,,,,,,,ci-pipeline,,,,
hotel-booking-daily,,,5,day,allow,,,,,,api-platform/hotel-booking,,,,,
`

func importBudgets(t *testing.T, env *budgetTestEnv, file string, dryRun bool) *gwspec.BudgetImportResult {
	t.Helper()
	result, err := env.svc.Import(context.Background(), budgetTestOrg, "alice", strings.NewReader(file), dryRun, "")
	require.NoError(t, err)
	return result
}

func TestBudgetImport_CreatesEveryRow(t *testing.T) {
	env := newBudgetTestEnv(t)
	result := importBudgets(t, env, importCSV, false)

	assert.True(t, result.Applied)
	assert.False(t, result.DryRun)
	assert.Equal(t, gwspec.BudgetImportSummary{Valid: 6, Created: 6}, result.Summary)
	require.Len(t, result.Results, 6)
	assert.Equal(t, int32(2), result.Results[0].Row, "the header is row 1")
	assert.Equal(t, "wso2-monthly", *result.Results[0].Name)
	assert.Empty(t, result.Results[0].Errors)

	require.Len(t, env.created, 6)
	byName := map[string]gwmodels.Budget{}
	for _, b := range env.created {
		byName[b.Name] = b
		assert.Equal(t, "alice", b.CreatedBy)
		assert.Equal(t, gwmodels.BudgetOriginControlPlane, b.Origin)
	}

	org := byName["wso2-monthly"].Configuration
	assert.Equal(t, int32(99), *org.Limit.BlockAt)
	assert.Equal(t, []int32{50, 80, 100}, org.Warnings.Thresholds)
	assert.Equal(t, []string{"finance-email"}, org.Warnings.Channels, "channels are named in the file and stored by name")
	assert.Nil(t, byName["wso2-monthly"].ProjectID)

	agents := byName["api-platform-agents"]
	require.NotNil(t, agents.ProjectID, "projectId makes it the project's budget")
	assert.Equal(t, budgetTestProjectID, agents.ProjectID.String())
	stored, err := json.Marshal(agents.Configuration.Filters)
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"type": "llmProvider", "id": "openai"},
		{"type": "project", "id": "api-platform"}
	]`, string(stored), "the llmProvider column, then the owning project, added last")
	assert.Equal(t, &gwmodels.BudgetSplit{Type: "agent"}, agents.Configuration.Limit.SplitBy)
	assert.Equal(t, "hotel-booking", *agents.Configuration.Exemptions[0].ID)

	limitedProject := byName["api-platform-cap"]
	assert.Nil(t, limitedProject.ProjectID, "the project column is a filter, not the owner")
	assert.Equal(t, "llmProvider", limitedProject.Configuration.Limit.SplitBy.Type)
	assert.Equal(t, "api-platform", *limitedProject.Configuration.Filters[0].ID)

	users := byName["rnd-per-user"].Configuration
	require.Len(t, users.Filters, 1)
	assert.Equal(t, "attribute", users.Filters[0].Type, "each attributes item is an attribute filter")
	assert.Equal(t, `'research' in request.Auth.Claims["groups"]`, *users.Filters[0].Expression)
	assert.Equal(t, `request.Auth.Claims["email"]`, *users.Limit.SplitBy.Expression)

	key := byName["ci-key-daily"]
	assert.Equal(t, "ci-key-daily", key.Configuration.DisplayName, "no displayName: the name is shown")
	assert.Equal(t, "apiKey", key.Configuration.Filters[0].Type)
	assert.Nil(t, key.Configuration.Limit.SplitBy, "no splitBy: one shared limit")
	assert.Nil(t, key.Configuration.Warnings)

	agent := byName["hotel-booking-daily"].Configuration.Filters
	require.Len(t, agent, 1, "an agent carries its project: no project filter is added")
	assert.Equal(t, "hotel-booking", *agent[0].ID)
	assert.Equal(t, "api-platform", *agent[0].Project)

	assert.Len(t, env.sender.sent, 6, "each saved budget goes to the gateways")
}

func TestBudgetImport_DryRunSavesNothing(t *testing.T) {
	env := newBudgetTestEnv(t)
	result := importBudgets(t, env, importCSV, true)
	assert.True(t, result.DryRun)
	assert.False(t, result.Applied)
	assert.Equal(t, int32(6), result.Summary.Valid)
	assert.Empty(t, env.budgets.CreateManyForControlPlaneCalls())
	assert.Empty(t, env.sender.sent)
}

func TestBudgetImport_ReportsEachRowsErrorsAndSavesNothing(t *testing.T) {
	env := newBudgetTestEnv(t)
	createTestBudget(t, env, "existing")
	env.created, env.sender.sent = nil, nil
	file := `name,amount,period,action,blockAt,projectId,project,agent,exemptions,thresholds,channels
good,100,month,block,,,,,,,
bad-amount,ten,month,block,,,,,,,
bad-block-at,100,month,block,most,,,,,,
no-such-agent,100,month,block,,,,api-platform/ghost,,,
any-channel-name,100,month,block,,,,,,80,pagerduty
bad-json,100,month,block,,,,,[{,,
other-project,100,month,block,,api-platform,support,,,,
existing,100,month,block,,,,,,,
good,50,day,allow,,,,,,,
bad-agent,100,month,block,,,,ghost,,,
`
	result := importBudgets(t, env, file, false)
	assert.False(t, result.Applied)
	// Channels are kept by name and not checked: webhook subscriptions deliver warnings.
	assert.Equal(t, gwspec.BudgetImportSummary{Valid: 2, Error: 8, Created: 2}, result.Summary)
	assert.Empty(t, env.budgets.CreateManyForControlPlaneCalls(), "one bad row and nothing is saved")
	assert.Empty(t, env.sender.sent)

	type cell struct{ column, code string }
	got := map[int32]cell{}
	for _, r := range result.Results {
		if len(r.Errors) > 0 {
			column := ""
			if r.Errors[0].Column != nil {
				column = *r.Errors[0].Column
			}
			got[r.Row] = cell{column, r.Errors[0].Code}
			assert.NotEmpty(t, r.Errors[0].Message)
		}
	}
	assert.Equal(t, map[int32]cell{
		3:  {"amount", httpx.ErrCodeValidation},
		4:  {"blockAt", httpx.ErrCodeValidation},
		5:  {"agent", gwerr.ErrCodeReferenceNotFound},
		7:  {"exemptions", httpx.ErrCodeValidation},
		8:  {"project", httpx.ErrCodeValidation},
		9:  {"name", gwerr.ErrCodeBudgetAlreadyExists},
		10: {"name", gwerr.ErrCodeBudgetAlreadyExists},
		11: {"agent", httpx.ErrCodeValidation},
	}, got)
}

func TestBudgetImport_RefusesAFileItCannotRead(t *testing.T) {
	many := "name,amount,period,action\n" + strings.Repeat("x,1,day,allow\n", maxBudgetImportRows+1)
	cases := map[string]string{
		"empty":            "",
		"header only":      "name,amount,period,action\n",
		"unknown column":   "name,amount,period,action,owner\nx,1,day,allow,me\n",
		"no orgId column":  "name,orgId,amount,period,action\nx,wso2,1,day,allow\n",
		"repeated column":  "name,amount,period,action,name\nx,1,day,allow,y\n",
		"missing required": "name,amount,period\nx,1,day\n",
		"short row":        "name,amount,period,action\nx,1,day\n",
		"bad quotes":       "name,amount,period,action\n\"x,1,day,allow\n",
		"too many rows":    many,
	}
	for name, file := range cases {
		t.Run(name, func(t *testing.T) {
			env := newBudgetTestEnv(t)
			_, err := env.svc.Import(context.Background(), budgetTestOrg, "alice", strings.NewReader(file), false, "")
			assert.Equal(t, gwerr.ErrCodeInvalidCSV, budgetErrorCode(t, err))
			assert.Empty(t, env.created)
		})
	}
}

func TestBudgetImport_ReadsAnyColumnOrderAndAByteOrderMark(t *testing.T) {
	env := newBudgetTestEnv(t)
	result := importBudgets(t, env, "\xEF\xBB\xBFaction, period ,amount,name\nallow,day,5,tiny\n", false)
	require.True(t, result.Applied)
	require.Len(t, env.created, 1)
	b := env.created[0]
	assert.Equal(t, "tiny", b.Name)
	assert.Equal(t, "5", b.Configuration.Limit.Amount)
	assert.Equal(t, "allow", b.Configuration.Limit.Action)
}

func TestBudgetImport_ANameTakenWhileSavingSavesNothing(t *testing.T) {
	env := newBudgetTestEnv(t)
	env.budgets.CreateManyForControlPlaneFunc = func(context.Context, []*gwmodels.Budget, []string, []*gwmodels.Budget) error {
		return &gwrepo.BudgetNameTakenError{Name: "second"}
	}
	result := importBudgets(t, env, "name,amount,period,action\nfirst,1,day,allow\nsecond,1,day,allow\n", false)
	assert.False(t, result.Applied)
	assert.Equal(t, gwspec.BudgetImportSummary{Valid: 1, Error: 1, Created: 1}, result.Summary)
	assert.Equal(t, "name", *result.Results[1].Errors[0].Column)
	assert.Equal(t, gwerr.ErrCodeBudgetAlreadyExists, result.Results[1].Errors[0].Code)
	assert.Empty(t, env.sender.sent)
}

func TestBudgetService_DisplayNameDefaultsToTheName(t *testing.T) {
	env := newBudgetTestEnv(t)
	req := budgetTestRequest()
	req.DisplayName = nil
	resp, err := env.create(context.Background(), budgetTestOrg, "alice", req)
	require.NoError(t, err)
	assert.Equal(t, req.Name, *resp.Spec.DisplayName)

	updated, err := env.update(context.Background(), budgetTestOrg, req.Name, budgetUpdateRequest{
		DisplayName: strPtr(" "), Limit: budgetUpdateLimit{Amount: "100.00", Action: "block"},
	})
	require.NoError(t, err)
	assert.Equal(t, req.Name, *updated.Spec.DisplayName)
}

// existingBudgetEnv has a control-plane budget "team-a" ($100 a month, project api-platform
// filter) and a gateway's budget "from-gateway".
func existingBudgetEnv(t *testing.T) *budgetTestEnv {
	t.Helper()
	env := newBudgetTestEnv(t)
	_, err := env.create(context.Background(), budgetTestOrg, "alice", budgetRequest{
		Name: "team-a", DisplayName: strPtr("Team A"),
		Limit:   gwspec.BudgetLimit{Amount: "100", Period: "month", Action: "block"},
		Filters: []gwspec.BudgetFilter{{Type: "project", Id: strPtr("api-platform")}},
	})
	require.NoError(t, err)
	gw := env.store["team-a"]
	gw.Name, gw.Origin = "from-gateway", gwmodels.BudgetOriginGateway
	env.store["from-gateway"] = gw
	env.created, env.sender.sent = nil, nil
	return env
}

const existingCSV = "name,displayName,amount,period,action,project,thresholds,channels\n" +
	"team-a,Team A (raised),250,month,allow,api-platform,80,finance-email\n" +
	"new-one,,10,day,block,,,\n"

func TestBudgetImport_AnExistingNameFailsWithAHintByDefault(t *testing.T) {
	env := existingBudgetEnv(t)
	result := importBudgets(t, env, existingCSV, false)
	assert.False(t, result.Applied)
	require.Len(t, result.Results[0].Errors, 1)
	e := result.Results[0].Errors[0]
	assert.Equal(t, gwerr.ErrCodeBudgetAlreadyExists, e.Code)
	assert.Equal(t, "name", *e.Column)
	assert.Contains(t, e.Message, "onExisting=update")
	assert.Contains(t, e.Message, "onExisting=skip")
	assert.Empty(t, env.created, "nothing is saved")
}

func TestBudgetImport_SkipLeavesAnExistingBudgetAsItIs(t *testing.T) {
	env := existingBudgetEnv(t)
	result, err := env.svc.Import(context.Background(), budgetTestOrg, "alice", strings.NewReader(existingCSV), false, OnExistingSkip)
	require.NoError(t, err)
	assert.True(t, result.Applied)
	assert.Equal(t, gwspec.BudgetImportSummary{Valid: 2, Created: 1, Skipped: 1}, result.Summary)
	assert.Equal(t, "skip", *result.Results[0].Action)
	assert.Equal(t, "create", *result.Results[1].Action)
	assert.Equal(t, "100", env.store["team-a"].Configuration.Limit.Amount, "left as it is")
	assert.Equal(t, []string{"created:new-one:active"}, env.sender.sent)
}

func TestBudgetImport_UpdateReplacesTheEditableFields(t *testing.T) {
	env := existingBudgetEnv(t)
	result, err := env.svc.Import(context.Background(), budgetTestOrg, "alice", strings.NewReader(existingCSV), false, OnExistingUpdate)
	require.NoError(t, err)
	require.True(t, result.Applied, "%+v", result.Results)
	assert.Equal(t, gwspec.BudgetImportSummary{Valid: 2, Created: 1, Updated: 1}, result.Summary)
	assert.Equal(t, "update", *result.Results[0].Action)
	updated := env.store["team-a"].Configuration
	assert.Equal(t, "250", updated.Limit.Amount)
	assert.Equal(t, "allow", updated.Limit.Action)
	assert.Equal(t, "Team A (raised)", updated.DisplayName)
	assert.Equal(t, []string{"finance-email"}, updated.Warnings.Channels)
	assert.Equal(t, "active", updated.Status, "the status stays")
	assert.ElementsMatch(t, []string{"updated:team-a:active", "created:new-one:active"}, env.sender.sent)
}

func TestBudgetImport_UpdateRefusesFixedChangesAndGatewayBudgets(t *testing.T) {
	env := existingBudgetEnv(t)
	file := "name,amount,period,action,project\n" +
		"team-a,250,day,allow,api-platform\n" +
		"from-gateway,250,month,allow,api-platform\n" +
		"openai,1,day,allow,\n"
	result, err := env.svc.Import(context.Background(), budgetTestOrg, "alice", strings.NewReader(file), true, OnExistingUpdate)
	require.NoError(t, err)
	require.Equal(t, int32(3), result.Summary.Error)

	period := result.Results[0].Errors[0]
	assert.Equal(t, "period", *period.Column, "the period is fixed after creation")
	assert.Equal(t, httpx.ErrCodeValidation, period.Code)
	assert.Equal(t, gwerr.ErrCodeBudgetReadOnly, result.Results[1].Errors[0].Code, "a gateway's budget is changed on the gateway")
	other := result.Results[2].Errors[0]
	assert.Equal(t, gwerr.ErrCodeBudgetAlreadyExists, other.Code, "a name held by another kind is never overwritten")
	assert.Contains(t, other.Message, "another resource")
}

func TestBudgetImport_RefusesAnUnknownOnExisting(t *testing.T) {
	env := newBudgetTestEnv(t)
	_, err := env.svc.Import(context.Background(), budgetTestOrg, "alice", strings.NewReader(existingCSV), true, "overwrite")
	assert.Equal(t, httpx.ErrCodeValidation, budgetErrorCode(t, err))
}

// A row lists every column that is wrong on its own, not only the first.
func TestBudgetImport_ListsEveryBadColumnOfARow(t *testing.T) {
	env := newBudgetTestEnv(t)
	file := "name,amount,period,action,splitBy,blockAt,thresholds\n" +
		"Bad Row,0100,decade,maybe,each,150,80;5000\n"
	result, err := env.svc.Import(context.Background(), budgetTestOrg, "alice", strings.NewReader(file), true, "")
	require.NoError(t, err)
	require.Len(t, result.Results, 1)
	columns := map[string]bool{}
	for _, e := range result.Results[0].Errors {
		columns[e.GetColumn()] = true
	}
	for _, c := range []string{"name", "amount", "period", "action", "splitBy", "blockAt", "thresholds"} {
		assert.True(t, columns[c], "column %s is reported", c)
	}
	assert.Empty(t, env.created, "nothing is stored")
}

func TestBudgetImport_AStarIsTheIdLessFilter(t *testing.T) {
	env := newBudgetTestEnv(t)
	file := "name,amount,period,action,splitBy,project,agent,apiKey\n" +
		"any-agent-daily,5,day,block,llmProvider,*,*,*\n"
	result := importBudgets(t, env, file, false)
	require.Len(t, result.Results, 1)
	assert.Empty(t, result.Results[0].Errors)
	require.Len(t, env.created, 1)
	stored, err := json.Marshal(env.created[0].Configuration.Filters)
	require.NoError(t, err)
	assert.JSONEq(t, `[{"type": "project"}, {"type": "agent"}, {"type": "apiKey"}]`, string(stored),
		"'*' is any project, agent or API key")

	env = newBudgetTestEnv(t)
	result = importBudgets(t, env, "name,amount,period,action,llmProvider\nany-provider,5,day,block,*\n", true)
	require.Len(t, result.Results, 1)
	assert.NotEmpty(t, result.Results[0].Errors, "an llmProvider filter always names its provider")
}

func TestBudgetImport_ReadsLabelsAndPointsAtABadOne(t *testing.T) {
	env := newBudgetTestEnv(t)
	file := "name,amount,period,action,labels\n" +
		"labelled,5,day,block,shop; finance\n"
	result := importBudgets(t, env, file, false)
	require.Len(t, result.Results, 1)
	assert.Empty(t, result.Results[0].Errors)
	require.Len(t, env.created, 1)
	assert.Equal(t, []string{"shop", "finance"}, env.created[0].Configuration.Labels)

	env = newBudgetTestEnv(t)
	result = importBudgets(t, env, "name,amount,period,action,labels\nbad-label,5,day,block,two words\n", true)
	require.Len(t, result.Results, 1)
	require.NotEmpty(t, result.Results[0].Errors)
	assert.Equal(t, "labels", *result.Results[0].Errors[0].Column, "the error points at the labels cell")
}
