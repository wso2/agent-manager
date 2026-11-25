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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
)

type planList []subplanv1.Plan

func (l planList) ListPlans(context.Context, string) ([]subplanv1.Plan, error) { return l, nil }

func (planList) ChangeEvent() string { return "ai-hub.plan_changed" }

func (planList) CountOffersWith(subplanv1.OfferCounter) {}

func (l planList) GetPlan(_ context.Context, _ string, id string) (subplanv1.Plan, bool, error) {
	for _, p := range l {
		if p.ID == id {
			return p, true, nil
		}
	}
	return subplanv1.Plan{}, false, nil
}

func (planList) ListSubscriptions(context.Context, string, string, string) ([]subplanv1.Subscription, error) {
	return nil, nil
}

func (planList) GetSubscription(context.Context, string, string) (subplanv1.Subscription, bool, error) {
	return subplanv1.Subscription{}, false, nil
}

func (planList) SubscriptionChangeEvent() string { return "ai-hub.subscription_changed" }

type sentChange struct {
	org    string
	action subplanv1.Action
	plan   subplanv1.Plan
}

type changeRecorder struct{ sent []sentChange }

func (r *changeRecorder) PlanChanged(_ context.Context, org string, a subplanv1.Action, p subplanv1.Plan) error {
	r.sent = append(r.sent, sentChange{org, a, p})
	return nil
}

// overtakingRecorder records sends; during runs once, inside the first send, as an overlapping delivery would.
type overtakingRecorder struct {
	sent   []sentChange
	during func()
}

func (r *overtakingRecorder) PlanChanged(_ context.Context, org string, a subplanv1.Action, p subplanv1.Plan) error {
	if f := r.during; f != nil {
		r.during = nil
		f()
	}
	r.sent = append(r.sent, sentChange{org, a, p})
	return nil
}

func planChangeEvent(t *testing.T, ch subplanv1.PlanChange) modkit.Event {
	t.Helper()
	data, err := json.Marshal(ch)
	require.NoError(t, err)
	return modkit.Event{Type: "ai-hub.plan_changed", OrgID: "org-1", Data: data}
}

func TestPlanChangeHandler(t *testing.T) {
	ctx := context.Background()
	limit := 100
	gold := subplanv1.Plan{ID: "p1", Handle: "gold", Name: "Gold", ThrottleLimitCount: &limit, ThrottleLimitUnit: "HOUR"}
	owner := func() (subplanv1.Source, bool) { return planList{gold}, true }

	t.Run("a created or updated plan is sent as the owner has it now", func(t *testing.T) {
		rec := &changeRecorder{}
		h := PlanChangeHandler(owner, rec)
		require.NoError(t, h(ctx, planChangeEvent(t, subplanv1.PlanChange{PlanID: "p1", Handle: "gold", Action: subplanv1.Updated})))
		require.Len(t, rec.sent, 1)
		assert.Equal(t, sentChange{"org-1", subplanv1.Updated, gold}, rec.sent[0])
	})

	t.Run("a deleted plan is sent by id, handle and name", func(t *testing.T) {
		rec := &changeRecorder{}
		h := PlanChangeHandler(owner, rec)
		require.NoError(t, h(ctx, planChangeEvent(t, subplanv1.PlanChange{
			OrgID: "org-2", PlanID: "p9", Handle: "old", PlanName: "Old", Action: subplanv1.Deleted,
		})))
		assert.Equal(t, []sentChange{{"org-2", subplanv1.Deleted, subplanv1.Plan{ID: "p9", Handle: "old", Name: "Old"}}}, rec.sent)
	})

	t.Run("nothing is sent while the owner is inactive, or for a plan deleted since", func(t *testing.T) {
		rec := &changeRecorder{}
		inactive := PlanChangeHandler(func() (subplanv1.Source, bool) { return nil, false }, rec)
		require.NoError(t, inactive(ctx, planChangeEvent(t, subplanv1.PlanChange{PlanID: "p1", Action: subplanv1.Created})))
		gone := PlanChangeHandler(owner, rec)
		require.NoError(t, gone(ctx, planChangeEvent(t, subplanv1.PlanChange{PlanID: "p2", Action: subplanv1.Updated})))
		assert.Empty(t, rec.sent)
	})

	t.Run("a delivery that read an older state than one already sent sends nothing", func(t *testing.T) {
		now := time.Now()
		newer, older := gold, gold
		newer.UpdatedAt, older.Name, older.UpdatedAt = now, "Gold (older)", now.Add(-time.Minute)
		current := newer
		rec := &changeRecorder{}
		h := PlanChangeHandler(func() (subplanv1.Source, bool) { return planList{current}, true }, rec)
		ch := subplanv1.PlanChange{PlanID: "p1", Handle: "gold", Action: subplanv1.Updated}
		require.NoError(t, h(ctx, planChangeEvent(t, ch)))
		current = older // an overlapping delivery that read the plan before the newer change committed
		require.NoError(t, h(ctx, planChangeEvent(t, ch)))
		current = newer // a redelivery of the newer state is sent again
		require.NoError(t, h(ctx, planChangeEvent(t, ch)))
		require.Len(t, rec.sent, 2)
		assert.Equal(t, "Gold", rec.sent[0].plan.Name)
		assert.Equal(t, "Gold", rec.sent[1].plan.Name)
	})

	t.Run("a delivery overtaken while it sends sends the current state again", func(t *testing.T) {
		now := time.Now()
		newer, older := gold, gold
		newer.UpdatedAt, older.Name, older.UpdatedAt = now, "Gold (older)", now.Add(-time.Minute)
		current := older
		ch := subplanv1.PlanChange{PlanID: "p1", Handle: "gold", Action: subplanv1.Updated}
		var h modkit.EventHandler
		rec := &overtakingRecorder{}
		// While the older state is being sent, the newer change commits and its delivery runs to the end.
		rec.during = func() {
			current = newer
			require.NoError(t, h(ctx, planChangeEvent(t, ch)))
		}
		h = PlanChangeHandler(func() (subplanv1.Source, bool) { return planList{current}, true }, rec)
		require.NoError(t, h(ctx, planChangeEvent(t, ch)))
		require.Len(t, rec.sent, 3, "older, newer (overtaking), then newer again")
		assert.Equal(t, "Gold", rec.sent[len(rec.sent)-1].plan.Name, "the last send is the newest state")
	})

	t.Run("an unknown action is an error", func(t *testing.T) {
		h := PlanChangeHandler(owner, &changeRecorder{})
		assert.Error(t, h(ctx, planChangeEvent(t, subplanv1.PlanChange{PlanID: "p1", Action: "renamed"})))
	})
}
