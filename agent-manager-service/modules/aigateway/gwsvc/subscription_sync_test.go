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
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// subscriptionSource serves one subscription.
type subscriptionSource struct {
	planList
	sub *subplanv1.Subscription
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

type subscriptionRecorder struct{ sent []sentSubscription }

func (r *subscriptionRecorder) SubscriptionChanged(_ context.Context, org string, a subplanv1.Action, apiID string, s subplanv1.Subscription) error {
	r.sent = append(r.sent, sentSubscription{org, a, apiID, s})
	return nil
}

// fakeDeployments maps an MCP proxy handle to its deployed artifacts.
type fakeDeployments map[string][]string

func (f fakeDeployments) DeployedArtifactIDs(_ context.Context, _, _, handle string) ([]string, error) {
	return f[handle], nil
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
	owner := func() (subplanv1.Source, bool) { return subscriptionSource{sub: &sub}, true }
	change := func(handle string, action subplanv1.Action) subplanv1.SubscriptionChange {
		return subplanv1.SubscriptionChange{
			SubscriptionID: sub.ID, ArtifactKind: subplanv1.ArtifactKindMCP, ArtifactHandle: handle, Action: action,
		}
	}

	t.Run("a created subscription is sent for every deployment of the MCP proxy, with its key and plan", func(t *testing.T) {
		rec := &subscriptionRecorder{}
		require.NoError(t, SubscriptionChangeHandler(owner, artifacts, rec)(ctx, subscriptionEvent(t, change("docs-mcp", subplanv1.Created))))
		require.Len(t, rec.sent, 2)
		for i, apiID := range []string{deployA, deployB} {
			want := sub
			want.ID = GatewaySubscriptionID(sub.ID, apiID)
			assert.Equal(t, sentSubscription{"org-1", subplanv1.Created, apiID, want}, rec.sent[i])
		}
		assert.NotEqual(t, rec.sent[0].sub.ID, rec.sent[1].sub.ID, "a gateway holding both deployments keeps both")
	})

	t.Run("a deleted subscription is sent by the same per-deployment ids", func(t *testing.T) {
		rec := &subscriptionRecorder{}
		require.NoError(t, SubscriptionChangeHandler(owner, artifacts, rec)(ctx, subscriptionEvent(t, change("docs-mcp", subplanv1.Deleted))))
		require.Len(t, rec.sent, 2)
		assert.Equal(t, GatewaySubscriptionID(sub.ID, deployA), rec.sent[0].sub.ID)
		assert.Empty(t, rec.sent[0].sub.Token)
	})

	t.Run("nothing is sent for a proxy no gateway holds, or a gone subscription", func(t *testing.T) {
		rec := &subscriptionRecorder{}
		h := SubscriptionChangeHandler(owner, artifacts, rec)
		require.NoError(t, h(ctx, subscriptionEvent(t, change("unknown-mcp", subplanv1.Created))))
		gone := func() (subplanv1.Source, bool) { return subscriptionSource{}, true }
		require.NoError(t, SubscriptionChangeHandler(gone, artifacts, rec)(ctx, subscriptionEvent(t, change("docs-mcp", subplanv1.Updated))))
		assert.Empty(t, rec.sent)
	})
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
