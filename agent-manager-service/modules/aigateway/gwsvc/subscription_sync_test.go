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
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

type planList []subplanv1.Plan

func (l planList) ListPlans(context.Context, string) ([]subplanv1.Plan, error) { return l, nil }

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

func (planList) SubscribedPlanIDs(context.Context, string, string, []string) ([]string, error) {
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

// subscriptionSource serves one subscription.
type subscriptionSource struct {
	planList
	sub *subplanv1.Subscription
}

func (s subscriptionSource) ListSubscriptions(_ context.Context, _, _, handle string) ([]subplanv1.Subscription, error) {
	if s.sub != nil && s.sub.ArtifactHandle == handle {
		return []subplanv1.Subscription{*s.sub}, nil
	}
	return nil, nil
}

func (s subscriptionSource) SubscribedPlanIDs(_ context.Context, _, _ string, handles []string) ([]string, error) {
	if s.sub != nil && slices.Contains(handles, s.sub.ArtifactHandle) {
		return []string{s.sub.PlanID}, nil
	}
	return nil, nil
}

func (s subscriptionSource) GetSubscription(_ context.Context, _ string, id string) (subplanv1.Subscription, bool, error) {
	if s.sub != nil && s.sub.ID == id {
		return *s.sub, true, nil
	}
	return subplanv1.Subscription{}, false, nil
}

type sentSubscription struct {
	org    string
	action subplanv1.Action
	apiID  string
	sub    subplanv1.Subscription
}

// subscriptionRecorder records the subscriptions sent, and the plans sent before them (order: what was sent,
// in turn, as "plan:<id>" or "sub:<apiId>").
type subscriptionRecorder struct {
	sent  []sentSubscription
	plans []sentChange
	order []string
}

func (r *subscriptionRecorder) PlanChanged(_ context.Context, org string, a subplanv1.Action, p subplanv1.Plan) error {
	r.plans = append(r.plans, sentChange{org, a, p})
	r.order = append(r.order, "plan:"+p.ID)
	return nil
}

func (r *subscriptionRecorder) SubscriptionChanged(_ context.Context, org string, a subplanv1.Action, apiID string, s subplanv1.Subscription) error {
	r.sent = append(r.sent, sentSubscription{org, a, apiID, s})
	r.order = append(r.order, "sub:"+apiID)
	return nil
}

// fakeDeployments maps an MCP proxy handle to its deployed artifacts.
type fakeDeployments map[string][]string

func (f fakeDeployments) DeployedArtifactIDs(_ context.Context, _, _, handle string) ([]string, error) {
	return f[handle], nil
}

func (f fakeDeployments) DeployedProxies(context.Context, string) ([]string, error) {
	var handles []string
	for handle := range f {
		handles = append(handles, handle)
	}
	return handles, nil
}

func (f fakeDeployments) ProxyOf(_ context.Context, _, artifactID string) (string, string, bool, error) {
	for handle, ids := range f {
		for _, id := range ids {
			if id == artifactID {
				return subplanv1.ArtifactKindMCP, handle, true, nil
			}
		}
	}
	return "", "", false, nil
}

func subscriptionEvent(t *testing.T, ch subplanv1.SubscriptionChange) modkit.Event {
	t.Helper()
	data, err := json.Marshal(ch)
	require.NoError(t, err)
	return modkit.Event{Type: "ai-hub.subscription_changed", OrgID: "org-1", Data: data}
}

func TestSubscriptionChangeHandler(t *testing.T) {
	ctx := context.Background()
	deployA, deployB := uuid.NewString(), uuid.NewString()
	artifacts := fakeDeployments{"docs-mcp": {deployA, deployB}}
	sub := subplanv1.Subscription{
		ID: uuid.NewString(), ArtifactKind: subplanv1.ArtifactKindMCP, ArtifactHandle: "docs-mcp", PlanID: "p-1",
		Token: "k-1", Status: subplanv1.SubscriptionActive, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	plan := subplanv1.Plan{ID: "p-1", Handle: "gold", Name: "Gold", Status: "ACTIVE"}
	owner := func() (subplanv1.Source, bool) { return subscriptionSource{planList: planList{plan}, sub: &sub}, true }
	change := func(handle string, action subplanv1.Action) subplanv1.SubscriptionChange {
		return subplanv1.SubscriptionChange{
			SubscriptionID: sub.ID, ArtifactKind: subplanv1.ArtifactKindMCP, ArtifactHandle: handle, Action: action,
		}
	}

	t.Run("a created subscription is sent for every deployment of the MCP proxy, with its key and plan", func(t *testing.T) {
		rec := &subscriptionRecorder{}
		require.NoError(t, SubscriptionChangeHandler(owner, artifacts, rec, rec)(ctx, subscriptionEvent(t, change("docs-mcp", subplanv1.Created))))
		require.Len(t, rec.sent, 2)
		for i, apiID := range []string{deployA, deployB} {
			want := sub
			want.ID = GatewaySubscriptionID(sub.ID, apiID)
			assert.Equal(t, sentSubscription{"org-1", subplanv1.Created, apiID, want}, rec.sent[i])
		}
		assert.NotEqual(t, rec.sent[0].sub.ID, rec.sent[1].sub.ID, "a gateway holding both deployments keeps both")
	})

	t.Run("its plan is sent first, as subscriptionPlan.created, which a gateway stores or replaces", func(t *testing.T) {
		for _, action := range []subplanv1.Action{subplanv1.Created, subplanv1.Updated} {
			rec := &subscriptionRecorder{}
			require.NoError(t, SubscriptionChangeHandler(owner, artifacts, rec, rec)(ctx, subscriptionEvent(t, change("docs-mcp", action))))
			assert.Equal(t, []sentChange{{"org-1", subplanv1.Created, plan}}, rec.plans)
			assert.Equal(t, []string{"plan:p-1", "sub:" + deployA, "sub:" + deployB}, rec.order)
		}
	})

	t.Run("a subscription whose plan is gone is sent without it", func(t *testing.T) {
		rec := &subscriptionRecorder{}
		noPlan := func() (subplanv1.Source, bool) { return subscriptionSource{sub: &sub}, true }
		require.NoError(t, SubscriptionChangeHandler(noPlan, artifacts, rec, rec)(ctx, subscriptionEvent(t, change("docs-mcp", subplanv1.Created))))
		assert.Empty(t, rec.plans)
		assert.Len(t, rec.sent, 2)
	})

	t.Run("a deleted subscription is sent by the same per-deployment ids", func(t *testing.T) {
		rec := &subscriptionRecorder{}
		require.NoError(t, SubscriptionChangeHandler(owner, artifacts, rec, rec)(ctx, subscriptionEvent(t, change("docs-mcp", subplanv1.Deleted))))
		require.Len(t, rec.sent, 2)
		assert.Equal(t, GatewaySubscriptionID(sub.ID, deployA), rec.sent[0].sub.ID)
		assert.Empty(t, rec.sent[0].sub.Token)
		assert.Empty(t, rec.plans, "a plan stays on the gateways when a subscription goes")
	})

	t.Run("nothing is sent for a proxy no gateway holds, or a gone subscription", func(t *testing.T) {
		rec := &subscriptionRecorder{}
		h := SubscriptionChangeHandler(owner, artifacts, rec, rec)
		require.NoError(t, h(ctx, subscriptionEvent(t, change("unknown-mcp", subplanv1.Created))))
		gone := func() (subplanv1.Source, bool) { return subscriptionSource{}, true }
		require.NoError(t, SubscriptionChangeHandler(gone, artifacts, rec, rec)(ctx, subscriptionEvent(t, change("docs-mcp", subplanv1.Updated))))
		assert.Empty(t, rec.sent)
		assert.Empty(t, rec.plans)
	})
}

func TestSubscribedPlans(t *testing.T) {
	ctx := context.Background()
	gold, silver := subplanv1.Plan{ID: "p-1", Handle: "gold"}, subplanv1.Plan{ID: "p-2", Handle: "silver"}
	sub := subplanv1.Subscription{ID: uuid.NewString(), ArtifactHandle: "docs-mcp", PlanID: "p-1"}
	source := subscriptionSource{planList: planList{gold, silver}, sub: &sub}

	got, err := SubscribedPlans(ctx, source, fakeDeployments{"docs-mcp": {uuid.NewString()}}, "org-1", planList{gold, silver})
	require.NoError(t, err)
	assert.Equal(t, []subplanv1.Plan{gold}, got, "only a plan a subscription is on")

	got, err = SubscribedPlans(ctx, source, fakeDeployments{}, "org-1", planList{gold, silver})
	require.NoError(t, err)
	assert.Empty(t, got, "a subscription to an MCP proxy no gateway holds doesn't count")
	deprecated := gold
	deprecated.Deprecated = true
	got, err = SubscribedPlans(ctx, source, fakeDeployments{"docs-mcp": {uuid.NewString()}}, "org-1", planList{deprecated, silver})
	require.NoError(t, err)
	assert.Equal(t, []subplanv1.Plan{deprecated}, got, "a deprecated plan still reaches gateways for its subscriptions")
}

func TestSubscriptionBroadcastPayloads(t *testing.T) {
	gw := &gwmodels.Gateway{UUID: uuid.New()}
	hub := &recordingEventHub{}
	b := NewSubscriptionPlanBroadcaster(stubOrgGateways{byOrg: map[string][]*gwmodels.Gateway{"org-1": {gw}}},
		NewGatewayEventsService(hub))
	sub := subplanv1.Subscription{ID: "s-1", PlanID: "p-1", Token: "k-1", Status: subplanv1.SubscriptionActive}

	require.NoError(t, b.SubscriptionChanged(context.Background(), "org-1", subplanv1.Created, "api-1", sub))
	typ, payload := envelope(t, hub.published[0])
	assert.Equal(t, "subscription.created", typ)
	assert.Equal(t, map[string]any{
		"apiId": "api-1", "subscriptionId": "s-1", "subscriptionToken": "k-1", "subscriptionPlanId": "p-1", "status": "ACTIVE",
	}, payload)

	hub.published = nil
	require.NoError(t, b.SubscriptionChanged(context.Background(), "org-1", subplanv1.Deleted, "api-1", subplanv1.Subscription{ID: "s-1"}))
	typ, payload = envelope(t, hub.published[0])
	assert.Equal(t, "subscription.deleted", typ)
	assert.Equal(t, map[string]any{"apiId": "api-1", "subscriptionId": "s-1"}, payload)
}

// A deployment the gateway now serves gets the proxy's subscriptions, each after its plan, sent to that gateway
// in the ack's transaction, under the IDs the gateway keeps them by for this artifact.
func TestReplaySubscriptionsAfterADeployment(t *testing.T) {
	ctx := context.Background()
	gold := subplanv1.Plan{ID: "p-1", Handle: "gold", Name: "Gold", Status: "ACTIVE"}
	sub := subplanv1.Subscription{ID: uuid.NewString(), ArtifactHandle: "docs-mcp", PlanID: "p-1", Token: "key-1",
		Status: subplanv1.SubscriptionActive}
	source := subscriptionSource{planList: planList{gold}, sub: &sub}
	artifact, gateway := uuid.New(), uuid.New()
	p := gwmodels.Placement{GatewayUUID: gateway}
	p.OUID, p.ArtifactUUID = "org-1", artifact
	tx := &gorm.DB{}

	t.Run("plan, then subscription, to the gateway that serves the artifact", func(t *testing.T) {
		hub := &recordingEventHub{}
		err := replaySubscriptions(ctx, tx, source, fakeDeployments{"docs-mcp": {artifact.String()}},
			NewGatewayEventsService(hub), p)
		require.NoError(t, err)
		require.Len(t, hub.published, 2)
		assert.Equal(t, "subscriptionPlan.created", string(hub.published[0].EventType))
		assert.Equal(t, "subscription.created", string(hub.published[1].EventType))
		for i, evt := range hub.published {
			assert.Equal(t, gateway.String(), evt.GatewayID)
			assert.Same(t, tx, hub.txs[i], "written in the ack's transaction")
		}
		var envelope struct{ Payload SubscriptionEvent }
		require.NoError(t, json.Unmarshal([]byte(hub.published[1].EventData), &envelope))
		payload := envelope.Payload
		assert.Equal(t, artifact.String(), payload.APIID)
		assert.Equal(t, GatewaySubscriptionID(sub.ID, artifact.String()), payload.SubscriptionID)
		assert.Equal(t, "key-1", payload.SubscriptionToken)
		assert.Equal(t, "p-1", payload.SubscriptionPlanID)
	})

	t.Run("an artifact of no MCP proxy, or one with no subscriptions, sends nothing", func(t *testing.T) {
		hub := &recordingEventHub{}
		require.NoError(t, replaySubscriptions(ctx, tx, source, fakeDeployments{}, NewGatewayEventsService(hub), p))
		require.NoError(t, replaySubscriptions(ctx, tx, subscriptionSource{planList: planList{gold}},
			fakeDeployments{"docs-mcp": {artifact.String()}}, NewGatewayEventsService(hub), p))
		assert.Empty(t, hub.published)
	})

	t.Run("without the plans module, AfterLive sends nothing", func(t *testing.T) {
		hub := &recordingEventHub{}
		kind := NewMCPProxyKind(nil, nil, nil, NewSubscriptionPlans(), NewGatewayEventsService(hub))
		require.NoError(t, kind.AfterLive(ctx, tx, p, uuid.New()))
		assert.Empty(t, hub.published)
		require.NoError(t, NewMCPProxyKind(nil, nil, nil, nil, nil).AfterLive(ctx, tx, p, uuid.New()))
	})
}
