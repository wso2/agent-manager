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

package services

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// An event naming its destinations (notifications/v1) goes to exactly those, whatever
// the subscriptions say, and receivers never see the list.
func TestEventDispatcher_DirectEventGoesOnlyToItsDestinations(t *testing.T) {
	f := newDispatcherFixture(t)
	named := f.destination(models.DestinationAvailableAll)
	subscribed := f.destination(models.DestinationAvailableAll)
	f.subscription("org", "", "", subscribed.ID, events.AllEvents)
	gone := "6b0f2a3e-1c4d-4e5f-9a8b-7c6d5e4f3a2b"
	ctx := context.Background()

	evt := events.New("budget:b1:2026-11-01:a1:80.00", "ai-gateway.budget.threshold_crossed", events.ScopeOrg, "ou-1", "", "", f.dispatcher.now())
	evt.Data = map[string]any{"message": "Spend at 80%"}
	evt.Destinations = []string{named.ID.String(), gone}
	msg := publish(t, evt)
	f.dispatcher.handleEvent(ctx, msg)
	assert.Equal(t, "ack", msg.outcome)
	require.Len(t, f.bus.enqueued, 1, "only the named destination that still exists")
	body := f.bus.enqueued[evt.ID+":"+named.ID.String()]
	require.NotNil(t, body)
	queued := f.deliveries[deliveryKey(named.ID, evt.ID)]
	assert.Empty(t, queued.SubscriptionIDs, "no subscription sent it")
	assert.Equal(t, "ai-gateway.budget.threshold_crossed", queued.EventType)

	delivery := &answeredMessage{data: body, attempt: 1}
	f.dispatcher.handleDelivery(ctx, delivery)
	assert.Equal(t, "ack", delivery.outcome)
	require.Equal(t, 1, f.driver.sends(), "sent although no subscription wants it")
	var sent map[string]any
	require.NoError(t, json.Unmarshal(f.driver.msgs[0].Body, &sent))
	assert.NotContains(t, sent, "destinations", "receivers never see the destination list")
	assert.Equal(t, evt.ID, sent["id"])

	// Sent once: the same event again is not delivered twice.
	f.dispatcher.handleEvent(ctx, publish(t, evt))
	again := &answeredMessage{data: f.bus.enqueued[evt.ID+":"+named.ID.String()], attempt: 1}
	f.dispatcher.handleDelivery(ctx, again)
	assert.Equal(t, 1, f.driver.sends())
}
