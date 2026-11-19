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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

type stubOrgGateways struct {
	byOrg map[string][]*gwmodels.Gateway
	err   error
}

func (s stubOrgGateways) GetByOrganizationID(_ context.Context, ouID string) ([]*gwmodels.Gateway, error) {
	return s.byOrg[ouID], s.err
}

// envelope decodes a published gateway event as the gateway-controller reads it.
func envelope(t *testing.T, evt eventhub.Event) (string, map[string]any) {
	t.Helper()
	var env struct {
		Type    string         `json:"type"`
		Payload map[string]any `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(evt.EventData), &env))
	return env.Type, env.Payload
}

func TestSubscriptionPlanBroadcasterSendsToEveryGatewayOfTheOrg(t *testing.T) {
	gw1, gw2 := &gwmodels.Gateway{UUID: uuid.New()}, &gwmodels.Gateway{UUID: uuid.New()}
	hub := &recordingEventHub{}
	b := NewSubscriptionPlanBroadcaster(stubOrgGateways{byOrg: map[string][]*gwmodels.Gateway{
		"org-1": {gw1, gw2}, "org-2": {{UUID: uuid.New()}},
	}}, NewGatewayEventsService(hub))
	count := 1000
	plan := subplanv1.Plan{
		ID: "11111111-1111-1111-1111-111111111111", Handle: "gold", Name: "Gold", Status: "ACTIVE",
		StopOnQuotaReach: true, ThrottleLimitCount: &count, ThrottleLimitUnit: "HOUR",
		Conditions: []subplanv1.Condition{{
			ID: "requests", Type: "request-count",
			Params: json.RawMessage(`{"limits":[{"count":1000,"timeAmount":1,"timeUnit":"HOUR"}],"onExceed":"reject"}`),
		}},
	}

	require.NoError(t, b.PlanChanged(context.Background(), "org-1", subplanv1.Created, plan))
	require.Len(t, hub.published, 2, "one event per gateway of the organization, none to others")
	assert.ElementsMatch(t, []string{gw1.UUID.String(), gw2.UUID.String()},
		[]string{hub.published[0].GatewayID, hub.published[1].GatewayID})
	typ, payload := envelope(t, hub.published[0])
	assert.Equal(t, "subscriptionPlan.created", typ)
	assert.Equal(t, plan.ID, payload["planId"])
	assert.Equal(t, "gold", payload["handle"])
	assert.Equal(t, "Gold", payload["planName"])
	assert.Equal(t, "free", payload["billingPlan"])
	assert.Equal(t, true, payload["stopOnQuotaReach"])
	assert.EqualValues(t, 1000, payload["throttleLimitCount"])
	assert.Equal(t, "HOUR", payload["throttleLimitUnit"])
	assert.Equal(t, "ACTIVE", payload["status"])
	assert.Len(t, payload["conditions"], 1, "the conditions travel with the legacy limit")

	hub.published = nil
	require.NoError(t, b.PlanChanged(context.Background(), "org-1", subplanv1.Updated, plan))
	typ, _ = envelope(t, hub.published[0])
	assert.Equal(t, "subscriptionPlan.updated", typ)

	hub.published = nil
	require.NoError(t, b.PlanChanged(context.Background(), "org-1", subplanv1.Deleted, plan))
	typ, payload = envelope(t, hub.published[0])
	assert.Equal(t, "subscriptionPlan.deleted", typ)
	assert.Equal(t, map[string]any{"planId": plan.ID, "handle": "gold", "planName": "Gold"}, payload)
}

func TestSubscriptionPlanBroadcasterLeavesOutTheLimitAPlanDoesNotHave(t *testing.T) {
	hub := &recordingEventHub{}
	b := NewSubscriptionPlanBroadcaster(stubOrgGateways{byOrg: map[string][]*gwmodels.Gateway{
		"org-1": {{UUID: uuid.New()}},
	}}, NewGatewayEventsService(hub))
	require.NoError(t, b.PlanChanged(context.Background(), "org-1", subplanv1.Created,
		subplanv1.Plan{ID: "p", Handle: "unlimited", Name: "Unlimited", Status: "ACTIVE", StopOnQuotaReach: true}))
	_, payload := envelope(t, hub.published[0])
	assert.NotContains(t, payload, "throttleLimitCount")
	assert.NotContains(t, payload, "throttleLimitUnit")
}

func TestSubscriptionPlanBroadcasterReportsFailuresButSendsToTheRest(t *testing.T) {
	failing := uuid.New()
	hub := &recordingEventHub{fail: func(evt eventhub.Event) error {
		if evt.GatewayID == failing.String() {
			return errors.New("hub down")
		}
		return nil
	}}
	b := NewSubscriptionPlanBroadcaster(stubOrgGateways{byOrg: map[string][]*gwmodels.Gateway{
		"org-1": {{UUID: failing}, {UUID: uuid.New()}},
	}}, NewGatewayEventsService(hub))
	err := b.PlanChanged(context.Background(), "org-1", subplanv1.Updated, subplanv1.Plan{ID: "p", Handle: "h", Name: "N"})
	assert.Error(t, err)
	assert.Len(t, hub.published, 1, "the other gateway still gets the event")

	assert.Error(t, b.PlanChanged(context.Background(), "org-1", subplanv1.Action("renamed"), subplanv1.Plan{ID: "p"}))
	assert.Error(t, NewSubscriptionPlanBroadcaster(stubOrgGateways{err: errors.New("db down")}, NewGatewayEventsService(hub)).
		PlanChanged(context.Background(), "org-1", subplanv1.Created, subplanv1.Plan{ID: "p"}))
}
