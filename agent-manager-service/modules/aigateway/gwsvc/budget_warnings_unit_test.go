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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconfig"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
)

// publishedEvent is one event the warning service raised.
type publishedEvent struct {
	eventType string
	data      map[string]any
}

// warningsEnv is a BudgetWarningService over mocks: one budget "per-agent" in project
// api-platform ($100 a month, warnings at 80 and 100, two channels), a usage store that
// keeps rows and warnings, and a publisher that records the events raised.
type warningsEnv struct {
	svc       *BudgetWarningService
	budget    *gwmodels.Budget
	rows      map[uuid.UUID]gwmodels.BudgetUsage
	warned    []gwmodels.BudgetWarning
	published []publishedEvent
	usage     *gwrepomocks.BudgetUsageRepositoryMock
}

func newWarningsEnv(t *testing.T) *warningsEnv {
	t.Helper()
	project := "api-platform"
	env := &warningsEnv{rows: map[uuid.UUID]gwmodels.BudgetUsage{}}
	env.budget = &gwmodels.Budget{
		ID: uuid.MustParse("0193f1c2-0000-7000-8000-0000000000b1"), Name: "per-agent", OrgID: budgetTestOrg, OrgName: budgetTestOrgName,
		Origin: gwmodels.BudgetOriginControlPlane,
		Configuration: gwmodels.BudgetConfig{
			DisplayName: "API Platform, per agent", Status: budgetStatusActive,
			Limit: gwmodels.BudgetLimit{
				Amount: "100.00", Currency: "USD", Period: "month", Action: "block", SplitBy: &gwmodels.BudgetSplit{Type: budgetTypeAgent},
			},
			Filters: []gwmodels.BudgetFilter{
				{Type: budgetTypeProject, ID: &project},
			},
			Warnings: &gwmodels.BudgetWarnings{Thresholds: []int32{80, 100}, Channels: []string{"all-email", "org-slack"}},
		},
	}
	budgets := &gwrepomocks.BudgetRepositoryMock{
		GetFunc: func(_ context.Context, ouID, name string) (*gwmodels.Budget, error) {
			if ouID == budgetTestOrg && name == env.budget.Name {
				return env.budget, nil
			}
			return nil, gwerr.ErrBudgetNotFound
		},
	}
	// record keeps warnings as the repository does: each key once, published when new.
	record := func(ctx context.Context, warnings []gwmodels.BudgetWarning, publish gwrepo.WarningPublisher) error {
		for _, w := range warnings {
			seen := false
			for _, old := range env.warned {
				seen = seen || (old.BudgetID == w.BudgetID && old.AllowanceID == w.AllowanceID &&
					old.PeriodStart.Equal(w.PeriodStart) && old.Amount == w.Amount)
			}
			if seen {
				continue
			}
			env.warned = append(env.warned, w)
			if publish != nil {
				if err := publish(ctx, nil, w); err != nil {
					return err
				}
			}
		}
		return nil
	}
	usage := &gwrepomocks.BudgetUsageRepositoryMock{
		RecordFunc: func(ctx context.Context, rows []gwmodels.BudgetUsage, warn func(gwmodels.BudgetUsage) []gwmodels.BudgetWarning,
			publish gwrepo.WarningPublisher,
		) error {
			for _, u := range rows {
				env.rows[u.AllowanceID] = u
				if warn != nil {
					if err := record(ctx, warn(u), publish); err != nil {
						return err
					}
				}
			}
			return nil
		},
		ForBudgetFunc: func(context.Context, uuid.UUID) ([]gwmodels.BudgetUsage, error) {
			out := []gwmodels.BudgetUsage{}
			for _, r := range env.rows {
				out = append(out, r)
			}
			return out, nil
		},
		RecordWarningsFunc: record,
	}
	gateways := &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*gwmodels.Gateway, error) { return &gwmodels.Gateway{OUID: budgetTestOrg}, nil },
	}
	env.usage = usage
	env.svc = NewBudgetWarningService(budgets, usage, gateways, nil, gwconfig.Settings{})
	env.svc.Use(func(_ context.Context, _ *gorm.DB, eventType string, data any) error {
		m, _ := data.(map[string]any)
		env.published = append(env.published, publishedEvent{eventType: eventType, data: m})
		return nil
	})
	return env
}

func usagePayload(t *testing.T, allowances ...GatewayAllowanceUsage) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(GatewayUsageMessage{Allowances: allowances})
	require.NoError(t, err)
	return data
}

func supportAgentUsage(spent string) GatewayAllowanceUsage {
	return GatewayAllowanceUsage{
		Budget: "per-agent", PeriodStart: "2026-11-01", AllowanceUUID: "5d6e2c1a-8f3b-5c47-a1d9-7e2b4c6f0a83",
		Allowance: map[string]string{"agent": "api-platform/support-agent"}, Spent: spent, UnpricedRequests: 3,
	}
}

func TestBudgetWarnings_ACrossedThresholdRaisesOneEvent(t *testing.T) {
	env := newWarningsEnv(t)
	env.svc.HandleUsage(context.Background(), "gw-1", usagePayload(t, supportAgentUsage("80.14")))

	row := env.rows[uuid.MustParse("5d6e2c1a-8f3b-5c47-a1d9-7e2b4c6f0a83")]
	assert.Equal(t, map[string]string{"project": "api-platform", "agent": "support-agent"}, row.Allowance,
		"an agent is stored as project and agent")
	assert.Equal(t, "80.14", row.Spent)
	assert.True(t, row.PeriodStart.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)))

	require.Len(t, env.published, 1, "80% crossed, 100% not")
	w := env.warned[0]
	assert.Equal(t, env.budget.ID, w.BudgetID)
	assert.Equal(t, "80.00", w.Amount)
	assert.Equal(t, 80, w.Threshold)
	e := env.published[0]
	assert.Equal(t, gwaudit.BudgetThresholdCrossed, e.eventType)
	assert.Equal(t, budgetTestOrg, e.data["orgId"])
	assert.Equal(t, budgetTestOrgName, e.data["orgName"], "notifications name the organization by its handle")
	assert.Equal(t, "api-platform", e.data["project"])
	assert.Equal(t, "support-agent", e.data["agent"])
	assert.Equal(t, "per-agent", e.data["subject"])
	assert.Equal(t, "per-agent", e.data["budget"])
	assert.Equal(t, "API Platform, per agent", e.data["displayName"])
	assert.Equal(t, "80.14", e.data["spent"])
	assert.EqualValues(t, 80, e.data["threshold"])
	assert.Equal(t, "2026-11-01", e.data["periodStart"])
	assert.Equal(t, "2026-12-01T00:00:00Z", e.data["resetsAt"])
	assert.Equal(t, []string{"all-email", "org-slack"}, e.data["channels"])
	assert.Equal(t, "support-agent at 80% ($80.14 of $100.00), resets 1 Dec", e.data["message"])

	// The same total again, or a higher one below 100%, raises nothing new.
	env.svc.HandleUsage(context.Background(), "gw-1", usagePayload(t, supportAgentUsage("85")))
	assert.Len(t, env.published, 1, "a threshold warns once per period")
}

// Without a publisher (the module not yet activated) spend is recorded and nothing is raised.
func TestBudgetWarnings_NoPublisherRecordsSpendWithoutWarnings(t *testing.T) {
	env := newWarningsEnv(t)
	env.svc.publish.Store(nil)
	env.svc.HandleUsage(context.Background(), "gw-1", usagePayload(t, supportAgentUsage("99")))
	assert.Len(t, env.rows, 1, "spend is recorded")
	assert.Empty(t, env.warned)
	env.svc.CheckBudget(context.Background(), env.budget)
	assert.Empty(t, env.published)
}

func TestBudgetWarnings_SkipsUnknownBudgetsAndInvalidTotals(t *testing.T) {
	env := newWarningsEnv(t)
	bad := supportAgentUsage("-1")
	unknown := supportAgentUsage("500")
	unknown.Budget = "gone"
	noUUID := supportAgentUsage("500")
	noUUID.AllowanceUUID = "x"
	env.svc.HandleUsage(context.Background(), "gw-1", usagePayload(t, bad, unknown, noUUID))
	assert.Empty(t, env.rows)
	assert.Empty(t, env.published)
}

func TestBudgetWarnings_NoWarningsWithoutThresholdsOrWhenDisabled(t *testing.T) {
	env := newWarningsEnv(t)
	env.budget.Configuration.Status = budgetStatusInactive
	env.svc.HandleUsage(context.Background(), "gw-1", usagePayload(t, supportAgentUsage("99")))
	assert.Len(t, env.rows, 1, "spend is still recorded")
	assert.Empty(t, env.published)
}

func TestBudgetWarnings_AnEditCrossingStoredSpendWarns(t *testing.T) {
	env := newWarningsEnv(t)
	env.svc.HandleUsage(context.Background(), "gw-1", usagePayload(t, supportAgentUsage("60")))
	assert.Empty(t, env.published)

	env.budget.Configuration.Limit.Amount = "70.00"
	env.svc.CheckBudget(context.Background(), env.budget)
	require.Len(t, env.published, 1, "60 is past 80% of the lowered $70 limit")
	assert.Equal(t, "56.00", env.warned[0].Amount, "the warning is keyed by the new threshold amount, so it re-arms")
	env.svc.CheckBudget(context.Background(), env.budget)
	assert.Len(t, env.published, 1, "a warning is raised once")
}

func TestBudgetWarnings_LabelsAndPeriods(t *testing.T) {
	assert.Equal(t, "Spend", allowanceLabel(nil))
	assert.Equal(t, "alice@wso2.com, openai", allowanceLabel(map[string]string{"attribute": "alice@wso2.com", "llmProvider": "openai"}))
	start := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, start.AddDate(0, 0, 1), periodEnd("day", start))
	assert.Equal(t, start.AddDate(0, 0, 7), periodEnd("week", start))
	assert.Equal(t, time.Date(2026, 12, 30, 0, 0, 0, 0, time.UTC), periodEnd("month", start))
}

// recordingWarner records the budgets an edit checks.
type recordingWarner struct{ checked []string }

func (w *recordingWarner) CheckBudget(_ context.Context, b *gwmodels.Budget) {
	w.checked = append(w.checked, b.Name)
}

func TestBudgetService_UpdateChecksWarnings(t *testing.T) {
	env := newBudgetTestEnv(t)
	resp, err := env.create(context.Background(), budgetTestOrg, "alice", budgetTestRequest())
	require.NoError(t, err)
	warner := &recordingWarner{}
	env.svc.(*budgetService).warner = warner

	_, err = env.update(context.Background(), budgetTestOrg, resp.Metadata.Name, budgetUpdateRequest{
		DisplayName: resp.Spec.DisplayName, Status: strPtr("active"),
		Limit: budgetUpdateLimit{Amount: "50.00", Action: "block"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{resp.Metadata.Name}, warner.checked)
}

// usageRecorder records the usage messages the gateway message handler passes on.
type usageRecorder struct {
	gateway string
	payload json.RawMessage
}

func (u *usageRecorder) HandleUsage(_ context.Context, gatewayID string, payload json.RawMessage) {
	u.gateway, u.payload = gatewayID, payload
}

func TestGatewayMessages_UsageGoesToTheWarningService(t *testing.T) {
	usage := &usageRecorder{}
	h := NewDeploymentAckHandler(nil, nil, usage)
	assert.True(t, h.HandleMessage("gw-1", []byte(`{"type":"budget.usage","payload":{"allowances":[]}}`)))
	assert.Equal(t, "gw-1", usage.gateway)
	assert.JSONEq(t, `{"allowances":[]}`, string(usage.payload))
	assert.False(t, NewDeploymentAckHandler(nil, nil, nil).HandleMessage("gw-1", []byte(`{"type":"budget.usage","payload":{}}`)),
		"without budgets the message is not handled")
}

// recordedCosts records the cost totals passed on.
type recordedCosts struct {
	ouID  string
	costs []GatewayCostUsage
}

func (r *recordedCosts) RecordCosts(_ context.Context, ouID string, costs []GatewayCostUsage) error {
	r.ouID, r.costs = ouID, append(r.costs, costs...)
	return nil
}

// A usage message's costs (AI cost stats) are recorded for the gateway's organization,
// whether or not it carries allowances, and even when its budget is unknown.
func TestBudgetWarnings_UsageCostsAreRecorded(t *testing.T) {
	env := newWarningsEnv(t)
	recorder := &recordedCosts{}
	env.svc.costs = recorder
	cost := GatewayCostUsage{
		Month: "2026-11-01", CostKey: uuid.New().String(), Project: "api-platform", Agent: "support-agent",
		LLMProvider: "openai", Model: "gpt-4o", Cost: "80.14",
	}
	unknown := supportAgentUsage("1")
	unknown.Budget = "gone"
	data, err := json.Marshal(GatewayUsageMessage{Allowances: []GatewayAllowanceUsage{unknown}, Costs: []GatewayCostUsage{cost}})
	require.NoError(t, err)

	env.svc.HandleUsage(context.Background(), "gw-1", data)

	assert.Equal(t, budgetTestOrg, recorder.ouID)
	assert.Equal(t, []GatewayCostUsage{cost}, recorder.costs)
}

// A message is written in one transaction.
func TestBudgetWarnings_OneMessageIsOneWrite(t *testing.T) {
	env := newWarningsEnv(t)
	other := supportAgentUsage("10")
	other.AllowanceUUID = "6e7f3d2b-9a4c-5d58-b2ea-8f3c5d7a1b94"
	other.Allowance = map[string]string{"agent": "api-platform/billing-agent"}
	env.svc.HandleUsage(context.Background(), "gw-1", usagePayload(t, supportAgentUsage("80.14"), other))

	require.Len(t, env.usage.RecordCalls(), 1)
	assert.Len(t, env.usage.RecordCalls()[0].Rows, 2)
	assert.Len(t, env.rows, 2)
}

// Once started, usage is recorded by the worker, not on the gateway's connection.
func TestBudgetWarnings_StartedUsageIsRecordedByTheWorker(t *testing.T) {
	env := newWarningsEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, env.svc.Start(ctx))
	defer func() { require.NoError(t, env.svc.Stop()) }()

	env.svc.HandleUsage(ctx, "gw-1", usagePayload(t, supportAgentUsage("80.14")))
	assert.Eventually(t, func() bool { return len(env.usage.RecordCalls()) == 1 }, 5*time.Second, 10*time.Millisecond)
}

func TestBudgetWarnings_ReportsAreCoalescedAndWrittenOncePerFlush(t *testing.T) {
	env := newWarningsEnv(t)
	env.svc.flush = time.Hour // only Stop writes, so every report below is coalesced
	ctx := context.Background()
	require.NoError(t, env.svc.Start(ctx))

	// Two gateways report the same allowance; one lags behind the other.
	for _, spent := range []string{"10.00", "45.50", "30.00", "80.14", "79.00"} {
		env.svc.HandleUsage(ctx, "gw-1", usagePayload(t, supportAgentUsage(spent)))
		env.svc.HandleUsage(ctx, "gw-2", usagePayload(t, supportAgentUsage("12.00")))
	}
	assert.Empty(t, env.usage.RecordCalls(), "nothing is written before the flush")

	require.NoError(t, env.svc.Stop())
	calls := env.usage.RecordCalls()
	require.Len(t, calls, 1, "ten reports for one allowance are one write")
	require.Len(t, calls[0].Rows, 1)
	assert.Equal(t, "80.14", calls[0].Rows[0].Spent, "the highest total reported is written")
	assert.Equal(t, int64(3), calls[0].Rows[0].UnpricedRequests)
}

func TestBudgetWarnings_TotalsAFailedWriteLostAreWrittenByTheNextFlush(t *testing.T) {
	env := newWarningsEnv(t)
	env.svc.flush = time.Hour // flushes are driven by hand below
	ctx := context.Background()
	require.NoError(t, env.svc.Start(ctx))
	defer func() { require.NoError(t, env.svc.Stop()) }()

	written := env.usage.RecordFunc
	env.usage.RecordFunc = func(context.Context, []gwmodels.BudgetUsage, func(gwmodels.BudgetUsage) []gwmodels.BudgetWarning, gwrepo.WarningPublisher) error {
		return errors.New("database unavailable")
	}
	env.svc.HandleUsage(ctx, "gw-1", usagePayload(t, supportAgentUsage("80.14")))
	env.svc.writePending(ctx)
	assert.Empty(t, env.rows, "the write failed")

	// The gateway sends nothing more; the next flush still writes the total.
	env.usage.RecordFunc = written
	env.svc.writePending(ctx)
	row, ok := env.rows[uuid.MustParse("5d6e2c1a-8f3b-5c47-a1d9-7e2b4c6f0a83")]
	require.True(t, ok, "the kept total is written by the next flush")
	assert.Equal(t, "80.14", row.Spent)
}
