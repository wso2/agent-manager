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
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// costTestNow is in November 2026: the kept months are September to November.
var costTestNow = time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)

func costRowFor(month time.Month, project, agent, apiKey, user, provider, model, cost string, missing int64) gwmodels.CostUsage {
	opt := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	return gwmodels.CostUsage{
		OUID: budgetTestOrg, Month: time.Date(2026, month, 1, 0, 0, 0, 0, time.UTC), CostKey: uuid.New(),
		Project: opt(project), Agent: opt(agent), APIKey: opt(apiKey), UserID: opt(user),
		LLMProvider: provider, Model: opt(model), Cost: cost, MissingPriceRequests: missing,
	}
}

func newCostTestService(rows []gwmodels.CostUsage) (*costService, *gwrepomocks.CostUsageRepositoryMock) {
	repo := &gwrepomocks.CostUsageRepositoryMock{
		SinceFunc: func(_ context.Context, ouID string, from time.Time) ([]gwmodels.CostUsage, error) {
			out := []gwmodels.CostUsage{}
			for _, r := range rows {
				if r.OUID == ouID && !r.Month.Before(from) {
					out = append(out, r)
				}
			}
			return out, nil
		},
		RecordFunc:       func(context.Context, []gwmodels.CostUsage) error { return nil },
		DeleteBeforeFunc: func(context.Context, string, time.Time) (int64, error) { return 0, nil },
	}
	s := NewCostService(repo).(*costService)
	s.now = func() time.Time { return costTestNow }
	return s, repo
}

var costTestRows = []gwmodels.CostUsage{
	costRowFor(time.November, "api-platform", "support-agent", "support-prod", "", "openai", "gpt-4o", "80.14", 3),
	costRowFor(time.October, "api-platform", "support-agent", "support-prod", "", "openai", "gpt-4o", "100", 0),
	costRowFor(time.November, "api-platform", "hotel-booking", "", "", "anthropic", "claude", "20.006", 0),
	costRowFor(time.November, "", "", "", "alice@wso2.com", "openai", "gpt-4o", "5", 1),
	costRowFor(time.May, "api-platform", "support-agent", "", "", "openai", "gpt-4o", "999", 0),
}

// costs returns an entry's monthly costs, oldest first.
func costs(e gwspec.CostByGroup) []string {
	out := make([]string, len(e.Monthly))
	for i, m := range e.Monthly {
		out[i] = m.Cost
	}
	return out
}

func TestCostService_OrganizationTotalPerMonth(t *testing.T) {
	s, _ := newCostTestService(costTestRows)
	report, err := s.Costs(context.Background(), budgetTestOrg, "acme", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "USD", report.Currency)
	require.Len(t, report.CostByGroup, 1)
	g := report.CostByGroup[0]
	assert.Equal(t, "acme", *g.Id.Get(), "grouped by organization, the id is its handle")
	assert.Nil(t, g.Project)
	months := []string{}
	for _, m := range g.Monthly {
		months = append(months, m.Month)
	}
	assert.Equal(t, []string{"2026-09", "2026-10", "2026-11"}, months)
	assert.Equal(t, []string{"0.00", "100.00", "105.146"}, costs(g), "May is older than the kept months")
	assert.Equal(t, "205.146", g.Cost)
	assert.Equal(t, int64(4), g.UnpricedRequests.MissingPrice)
}

func TestCostService_HighestCostFirst(t *testing.T) {
	s, _ := newCostTestService(costTestRows)
	report, err := s.Costs(context.Background(), budgetTestOrg, "acme", "agent", nil)
	require.NoError(t, err)
	require.Len(t, report.CostByGroup, 3)
	first := report.CostByGroup[0]
	assert.Equal(t, "api-platform", *first.Project, "an agent names its project")
	assert.Equal(t, "support-agent", *first.Id.Get())
	assert.Equal(t, "180.14", first.Cost)
	assert.Equal(t, "hotel-booking", *report.CostByGroup[1].Id.Get())
	last := report.CostByGroup[2]
	assert.Nil(t, last.Id.Get(), "requests with no agent are an entry of their own")
	assert.True(t, last.Id.IsSet())
	assert.Equal(t, "5.00", last.Cost)

	body, err := json.Marshal(last)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"id":null`)
}

func TestCostService_ProjectOnlyForAgents(t *testing.T) {
	s, _ := newCostTestService(costTestRows)
	report, err := s.Costs(context.Background(), budgetTestOrg, "acme", "model", nil)
	require.NoError(t, err)
	for _, e := range report.CostByGroup {
		assert.Nil(t, e.Project, "only agent entries name a project")
		body, err := json.Marshal(e)
		require.NoError(t, err)
		assert.NotContains(t, string(body), `"project"`)
	}
}

func TestCostService_Filters(t *testing.T) {
	s, _ := newCostTestService(costTestRows)
	report, err := s.Costs(context.Background(), budgetTestOrg, "acme", "model",
		[]string{"agent:api-platform/support-agent,api-platform/hotel-booking", "llmProvider:openai"})
	require.NoError(t, err)
	require.Len(t, report.CostByGroup, 1, "a comma list is any of them; a repeated filter is AND")
	assert.Equal(t, "gpt-4o", *report.CostByGroup[0].Id.Get())
	assert.Equal(t, "180.14", report.CostByGroup[0].Cost)

	report, err = s.Costs(context.Background(), budgetTestOrg, "acme", "user", []string{"user:alice@wso2.com"})
	require.NoError(t, err)
	require.Len(t, report.CostByGroup, 1)
	assert.Equal(t, "alice@wso2.com", *report.CostByGroup[0].Id.Get())
}

func TestCostService_RefusesBadQueries(t *testing.T) {
	s, _ := newCostTestService(nil)
	for _, tc := range []struct {
		groupBy string
		filters []string
		field   string
	}{
		{"environment", nil, "groupBy"},
		{"", []string{"environment:production"}, "filter"},
		{"", []string{"project"}, "filter"},
		{"", []string{"project:"}, "filter"},
	} {
		_, err := s.Costs(context.Background(), budgetTestOrg, "acme", tc.groupBy, tc.filters)
		var be *BudgetError
		require.ErrorAs(t, err, &be, "%v", tc)
		assert.Equal(t, httpx.ErrCodeValidation, be.Code)
		assert.Equal(t, tc.field, be.Field)
	}
	report, err := s.Costs(context.Background(), budgetTestOrg, "acme", "project", nil)
	require.NoError(t, err)
	assert.Empty(t, report.CostByGroup, "no costs yet")
	assert.NotNil(t, report.CostByGroup)
}

func TestCostService_RecordsReportsAndSkipsBadOrOldOnes(t *testing.T) {
	s, repo := newCostTestService(nil)
	key := uuid.New().String()
	require.NoError(t, s.RecordCosts(context.Background(), budgetTestOrg, []GatewayCostUsage{
		{Month: "2026-11-01", CostKey: key, Project: "api-platform", Agent: "support-agent", LLMProvider: "openai", Model: "gpt-4o", Cost: "80.14", MissingPriceRequests: 3},
		{Month: "2026-08-01", CostKey: uuid.New().String(), LLMProvider: "openai", Cost: "1"},
		{Month: "not-a-date", CostKey: uuid.New().String(), LLMProvider: "openai", Cost: "1"},
		{Month: "2026-11-01", CostKey: "x", LLMProvider: "openai", Cost: "1"},
		{Month: "2026-11-01", CostKey: uuid.New().String(), Cost: "1"},
		{Month: "2026-11-01", CostKey: uuid.New().String(), LLMProvider: "openai", Cost: "ten"},
	}))
	calls := repo.RecordCalls()
	require.Len(t, calls, 1, "one write per message")
	require.Len(t, calls[0].Rows, 1, "older than the kept months, or unreadable: skipped")
	row := calls[0].Rows[0]
	assert.Equal(t, key, row.CostKey.String())
	assert.Equal(t, "api-platform", *row.Project)
	assert.Nil(t, row.APIKey, "empty values are stored as none")
	assert.Equal(t, int64(3), row.MissingPriceRequests)
}

func TestCostService_ANewMonthDropsTheOldestOnceOnTheFirstWrite(t *testing.T) {
	s, repo := newCostTestService(nil)
	report := func() {
		require.NoError(t, s.RecordCosts(context.Background(), budgetTestOrg, []GatewayCostUsage{
			{Month: s.now().UTC().Format("2006-01") + "-01", CostKey: uuid.New().String(), LLMProvider: "openai", Cost: "1"},
		}))
	}
	report()
	report()
	require.Len(t, repo.DeleteBeforeCalls(), 1, "deleted once per month, on the organization's first write")
	assert.Equal(t, budgetTestOrg, repo.DeleteBeforeCalls()[0].OuID)
	assert.Equal(t, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), repo.DeleteBeforeCalls()[0].Before)

	// December starts: the first write drops September.
	s.now = func() time.Time { return time.Date(2026, 12, 1, 0, 5, 0, 0, time.UTC) }
	report()
	report()
	require.Len(t, repo.DeleteBeforeCalls(), 2)
	assert.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), repo.DeleteBeforeCalls()[1].Before)
}

func TestUSDString_KeepsFractionsOfACent(t *testing.T) {
	for in, want := range map[string]string{"0": "0.00", "1.5": "1.50", "0.0325": "0.0325", "1.045850": "1.04585", "2/3": "0.666667"} {
		r, ok := new(big.Rat).SetString(in)
		require.True(t, ok)
		assert.Equal(t, want, usdString(r), in)
	}
}

func TestParseCostFilters_AgentNeedsItsProject(t *testing.T) {
	for _, bad := range []string{"agent:support-agent", "agent:/support-agent", "agent:api-platform/", "agent:api-platform/a,lonely"} {
		_, err := parseCostFilters([]string{bad})
		var be *BudgetError
		require.ErrorAs(t, err, &be, bad)
		assert.Equal(t, "filter", be.Field)
	}
	got, err := parseCostFilters([]string{"agent:api-platform/support-agent,api-platform/billing"})
	require.NoError(t, err)
	assert.Equal(t, []string{"api-platform/support-agent", "api-platform/billing"}, got[0].values)
}
