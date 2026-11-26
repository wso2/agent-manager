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

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/notifyv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconfig"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"
)

// warningsEnv is a BudgetWarningService over mocks: one budget "per-agent" in project
// api-platform ($100 a month, warnings at 80 and 100, two channels), a usage store that
// keeps the higher total per usage and period as the repository does, and a
// publisher that records the events raised.
type warningsEnv struct {
	svc       *BudgetWarningService
	budget    *gwmodels.Budget
	rows      map[uuid.UUID]gwmodels.BudgetUsage
	published []notifyv1.Notification
	usage     *gwrepomocks.BudgetUsageRepositoryMock
	// publishErr, when set, fails the publisher: the write is then not stored.
	publishErr error
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
				Amount: "100.00", Period: "month", Action: "block", SplitBy: &gwmodels.BudgetSplit{Type: budgetTypeAgent},
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
	usage := &gwrepomocks.BudgetUsageRepositoryMock{
		// Record keeps what the repository does: a row only rises, a write that changed
		// one hands warn the total it held before (0 for a new row), and a failed publish
		// stores nothing.
		RecordFunc: func(ctx context.Context, rows []gwmodels.BudgetUsage, warn gwrepo.WarnFunc, publish gwrepo.WarningPublisher) error {
			stored := map[uuid.UUID]gwmodels.BudgetUsage{}
			for k, v := range env.rows {
				stored[k] = v
			}
			for _, u := range rows {
				before, had := stored[u.BudgetUsageID]
				was := "0"
				if had && before.PeriodStart.Equal(u.PeriodStart) {
					was = before.Spent
					if !gwutil.DecimalGreater(u.Spent, was) {
						continue
					}
				}
				stored[u.BudgetUsageID] = u
				if warn == nil || publish == nil {
					continue
				}
				for _, n := range warn(was, u) {
					if err := publish(ctx, nil, n); err != nil {
						return err
					}
				}
			}
			env.rows = stored
			return nil
		},
		ForBudgetFunc: func(context.Context, uuid.UUID) ([]gwmodels.BudgetUsage, error) {
			out := []gwmodels.BudgetUsage{}
			for _, r := range env.rows {
				out = append(out, r)
			}
			return out, nil
		},
		DeletePeriodsBeforeFunc: func(context.Context, uuid.UUID, time.Time) (int64, error) {
			return 0, nil
		},
	}
	gateways := &gwrepomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(string) (*gwmodels.Gateway, error) { return &gwmodels.Gateway{OUID: budgetTestOrg}, nil },
	}
	env.usage = usage
	env.svc = NewBudgetWarningService(budgets, usage, gateways, nil, &failingNotifier{env: env}, gwconfig.Settings{})
	return env
}

// failingNotifier records the warnings sent, and fails while env.publishErr is set.
type failingNotifier struct{ env *warningsEnv }

func (n *failingNotifier) Notify(_ context.Context, _ *gorm.DB, in notifyv1.Notification) error {
	if n.env.publishErr != nil {
		return n.env.publishErr
	}
	n.env.published = append(n.env.published, in)
	return nil
}

func (*failingNotifier) Unusable(context.Context, string, string, []string) ([]notifyv1.Unusable, error) {
	return nil, nil
}

func usagePayload(t *testing.T, usages ...GatewayUsageEntry) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(GatewayUsageMessage{Usages: usages})
	require.NoError(t, err)
	return data
}

func supportAgentUsage(spent string) GatewayUsageEntry {
	return GatewayUsageEntry{
		Budget: "per-agent", PeriodStart: "2026-11-01", UsageUUID: "5d6e2c1a-8f3b-5c47-a1d9-7e2b4c6f0a83",
		Usage: map[string]string{"agent": "api-platform/support-agent"}, Spent: spent, UnpricedRequests: 3,
	}
}

func TestBudgetWarnings_ACrossedThresholdRaisesOneEvent(t *testing.T) {
	env := newWarningsEnv(t)
	env.svc.HandleUsage(context.Background(), "gw-1", usagePayload(t, supportAgentUsage("80.14")))

	row := env.rows[uuid.MustParse("5d6e2c1a-8f3b-5c47-a1d9-7e2b4c6f0a83")]
	assert.Equal(t, map[string]string{"project": "api-platform", "agent": "support-agent"}, row.Usage,
		"an agent is stored as project and agent")
	assert.Equal(t, "80.14", row.Spent)
	assert.True(t, row.PeriodStart.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)))

	require.Len(t, env.published, 1, "80% crossed, 100% not")
	e := env.published[0]
	assert.Equal(t, BudgetWarningEvent, e.Type)
	assert.Equal(t, budgetTestOrg, e.OrgID)
	assert.Equal(t, budgetTestOrgName, e.OrgName, "notifications name the organization by its handle")
	assert.Equal(t, []string{"all-email", "org-slack"}, e.Channels, "sent to the budget's channels")
	assert.Equal(t, "api-platform", e.Project)
	assert.Equal(t, "support-agent", e.Agent)
	assert.Equal(t, "per-agent", e.Subject)
	assert.Equal(t, "budget:"+env.budget.ID.String()+":2026-11-01:5d6e2c1a-8f3b-5c47-a1d9-7e2b4c6f0a83:80.00", e.ID,
		"the ID is the budget, period, usage and threshold amount")
	assert.Equal(t, "per-agent", e.Data["budget"])
	assert.Equal(t, "API Platform, per agent", e.Data["displayName"])
	assert.Equal(t, "80.14", e.Data["spent"])
	assert.EqualValues(t, 80, e.Data["threshold"])
	assert.Equal(t, "2026-12-01T00:00:00Z", e.Data["resetsAt"])
	assert.Equal(t, "Budget: API Platform, per agent\nReached: 80% of the monthly limit\n"+
		"Spent: $80.14 of $100.00\nFor: support-agent\nResets: 1 Dec (UTC)\nAt the limit: requests are blocked",
		e.Data["message"])

	// The same total again, or a higher one below 100%, raises nothing new.
	env.svc.HandleUsage(context.Background(), "gw-1", usagePayload(t, supportAgentUsage("85")))
	assert.Len(t, env.published, 1, "a threshold warns once per period")
}

// Without a publisher (the module not yet activated) spend is recorded and nothing is raised.
func TestBudgetWarnings_NoPublisherRecordsSpendWithoutWarnings(t *testing.T) {
	env := newWarningsEnv(t)
	env.svc.notifier = nil
	env.svc.HandleUsage(context.Background(), "gw-1", usagePayload(t, supportAgentUsage("99")))
	assert.Len(t, env.rows, 1, "spend is recorded")
	assert.Empty(t, env.published)
}

func TestBudgetWarnings_SkipsUnknownBudgetsAndInvalidTotals(t *testing.T) {
	env := newWarningsEnv(t)
	bad := supportAgentUsage("-1")
	unknown := supportAgentUsage("500")
	unknown.Budget = "gone"
	noUUID := supportAgentUsage("500")
	noUUID.UsageUUID = "x"
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

// An edit warns nothing by itself: a lowered limit does not warn for spend already past
// the new threshold, and raising it gives new amounts, which warn as the spend crosses them.
func TestBudgetWarnings_AnEditWarnsNothingAndNewAmountsWarnAgain(t *testing.T) {
	env := newWarningsEnv(t)
	handle := func(spent string) {
		env.svc.HandleUsage(context.Background(), "gw-1", usagePayload(t, supportAgentUsage(spent)))
	}
	handle("60")
	assert.Empty(t, env.published)

	env.budget.Configuration.Limit.Amount = "70.00"
	handle("61")
	assert.Empty(t, env.published, "60 was already past 80% of the lowered $70 limit: nothing crosses it")

	handle("70")
	require.Len(t, env.published, 1, "100% of $70 is crossed by the next write")
	assert.EqualValues(t, 100, env.published[0].Data["threshold"])

	env.budget.Configuration.Limit.Amount = "200.00"
	handle("150")
	handle("160")
	require.Len(t, env.published, 2, "the raised limit has new amounts: 80% of $200 is $160")
	assert.EqualValues(t, 80, env.published[1].Data["threshold"])
	assert.Contains(t, env.published[1].ID, ":160.00")
}

// A threshold is crossed by the write that takes the total from below it to or past it,
// and by no other.
func TestBudgetWarnings_ACrossingWarnsOnceWhateverFollows(t *testing.T) {
	handle := func(env *warningsEnv, period, spent string) {
		u := supportAgentUsage(spent)
		u.PeriodStart = period
		env.svc.HandleUsage(context.Background(), "gw-1", usagePayload(t, u))
	}

	t.Run("exactly reaching a threshold crosses it", func(t *testing.T) {
		env := newWarningsEnv(t)
		handle(env, "2026-11-01", "79.99")
		assert.Empty(t, env.published)
		handle(env, "2026-11-01", "80.00")
		require.Len(t, env.published, 1)
		assert.EqualValues(t, 80, env.published[0].Data["threshold"])
	})

	t.Run("a later write above a crossed threshold does not warn again", func(t *testing.T) {
		env := newWarningsEnv(t)
		handle(env, "2026-11-01", "85")
		handle(env, "2026-11-01", "90")
		handle(env, "2026-11-01", "90")
		handle(env, "2026-11-01", "84")
		assert.Len(t, env.published, 1)
	})

	t.Run("one write crossing several thresholds warns for each, once", func(t *testing.T) {
		env := newWarningsEnv(t)
		handle(env, "2026-11-01", "120")
		require.Len(t, env.published, 2)
		got := []any{env.published[0].Data["threshold"], env.published[1].Data["threshold"]}
		assert.ElementsMatch(t, []any{80, 100}, got)
		handle(env, "2026-11-01", "130")
		assert.Len(t, env.published, 2)
	})

	t.Run("a new period starts again", func(t *testing.T) {
		env := newWarningsEnv(t)
		handle(env, "2026-11-01", "85")
		handle(env, "2026-12-01", "10")
		assert.Len(t, env.published, 1)
		handle(env, "2026-12-01", "85")
		require.Len(t, env.published, 2, "the same threshold warns again in the next period")
		assert.Contains(t, env.published[1].ID, ":2026-12-01:")
	})

	t.Run("a failed publish stores nothing, so the next flush crosses it again", func(t *testing.T) {
		env := newWarningsEnv(t)
		env.svc.flush = time.Hour
		ctx := context.Background()
		require.NoError(t, env.svc.Start(ctx))
		defer func() { require.NoError(t, env.svc.Stop()) }()
		env.publishErr = errors.New("queue unavailable")
		env.svc.HandleUsage(ctx, "gw-1", usagePayload(t, supportAgentUsage("85")))
		env.svc.writePending(ctx)
		assert.Empty(t, env.rows, "the spend was not stored")
		assert.Empty(t, env.published)

		env.publishErr = nil
		env.svc.writePending(ctx)
		require.Len(t, env.published, 1, "the held total crosses 80% on the next flush")
		assert.Equal(t, "85", env.rows[uuid.MustParse("5d6e2c1a-8f3b-5c47-a1d9-7e2b4c6f0a83")].Spent)
	})
}

func TestBudgetWarnings_LabelsAndPeriods(t *testing.T) {
	assert.Equal(t, "Spend", usageLabel(nil))
	assert.Equal(t, "alice@wso2.com, openai", usageLabel(map[string]string{"attribute": "alice@wso2.com", "llmProvider": "openai"}))
	start := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, start.AddDate(0, 0, 1), periodEnd("day", start))
	assert.Equal(t, start.AddDate(0, 0, 7), periodEnd("week", start))
	assert.Equal(t, time.Date(2026, 12, 30, 0, 0, 0, 0, time.UTC), periodEnd("month", start))
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
	assert.True(t, h.HandleMessage("gw-1", []byte(`{"type":"budget.usage","payload":{"usages":[]}}`)))
	assert.Equal(t, "gw-1", usage.gateway)
	assert.JSONEq(t, `{"usages":[]}`, string(usage.payload))
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
// whether or not it carries usages, and even when its budget is unknown.
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
	data, err := json.Marshal(GatewayUsageMessage{Usages: []GatewayUsageEntry{unknown}, Costs: []GatewayCostUsage{cost}})
	require.NoError(t, err)

	env.svc.HandleUsage(context.Background(), "gw-1", data)

	assert.Equal(t, budgetTestOrg, recorder.ouID)
	assert.Equal(t, []GatewayCostUsage{cost}, recorder.costs)
}

// A message is written in one transaction.
func TestBudgetWarnings_OneMessageIsOneWrite(t *testing.T) {
	env := newWarningsEnv(t)
	other := supportAgentUsage("10")
	other.UsageUUID = "6e7f3d2b-9a4c-5d58-b2ea-8f3c5d7a1b94"
	other.Usage = map[string]string{"agent": "api-platform/billing-agent"}
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

	// Two gateways report the same usage; one lags behind the other.
	for _, spent := range []string{"10.00", "45.50", "30.00", "80.14", "79.00"} {
		env.svc.HandleUsage(ctx, "gw-1", usagePayload(t, supportAgentUsage(spent)))
		env.svc.HandleUsage(ctx, "gw-2", usagePayload(t, supportAgentUsage("12.00")))
	}
	assert.Empty(t, env.usage.RecordCalls(), "nothing is written before the flush")

	require.NoError(t, env.svc.Stop())
	calls := env.usage.RecordCalls()
	require.Len(t, calls, 1, "ten reports for one usage are one write")
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
	env.usage.RecordFunc = func(context.Context, []gwmodels.BudgetUsage, gwrepo.WarnFunc, gwrepo.WarningPublisher) error {
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

func TestBudgetWarnings_ANewPeriodDeletesThePastOnes(t *testing.T) {
	env := newWarningsEnv(t)
	type call struct {
		budget uuid.UUID
		before time.Time
	}
	var calls []call
	fail := true
	env.usage.DeletePeriodsBeforeFunc = func(_ context.Context, budgetID uuid.UUID, before time.Time) (int64, error) {
		calls = append(calls, call{budgetID, before})
		if fail {
			return 0, errors.New("database unavailable")
		}
		return 1, nil
	}
	report := func(period, spent string) {
		u := supportAgentUsage(spent)
		u.PeriodStart = period
		env.svc.HandleUsage(context.Background(), "gw-1", usagePayload(t, u))
	}
	nov := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	dec := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

	report("2026-11-01", "10.00")
	require.Len(t, calls, 1, "the first write of a period deletes the earlier ones")
	assert.Equal(t, call{env.budget.ID, nov}, calls[0])

	fail = false
	report("2026-11-01", "11.00")
	require.Len(t, calls, 2, "a failed delete is tried again with the next write")

	report("2026-11-01", "12.00")
	assert.Len(t, calls, 2, "once per period")

	report("2026-12-01", "1.00")
	require.Len(t, calls, 3, "the next period deletes again")
	assert.Equal(t, call{env.budget.ID, dec}, calls[2])

	report("2026-11-01", "13.00")
	assert.Len(t, calls, 3, "a late report of a past period deletes nothing")
}
