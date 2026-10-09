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
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// fakeBus is a Bus with no transport behind it, to drive the dispatcher's
// handlers directly. It shows the dispatcher needs only the Bus interface.
type fakeBus struct {
	enqueued map[string][]byte
}

func (f *fakeBus) Setup(context.Context) error                              { return nil }
func (f *fakeBus) PublishEvent(context.Context, events.Event, []byte) error { return nil }
func (f *fakeBus) SubscribeEvents(context.Context, string, func(events.Message)) (events.Subscription, error) {
	return noopSubscription{}, nil
}

func (f *fakeBus) Enqueue(_ context.Context, _ string, key string, body []byte) error {
	f.enqueued[key] = body
	return nil
}

func (f *fakeBus) Consume(context.Context, string, string, func(events.Message)) (events.Subscription, error) {
	return noopSubscription{}, nil
}

type noopSubscription struct{}

func (noopSubscription) Stop() {}
func (f *fakeBus) Close()      {}

// fakeMessage records how the dispatcher answered it.
type fakeMessage struct {
	data    []byte
	attempt int
	outcome string
	delay   time.Duration
}

func (m *fakeMessage) Data() []byte { return m.data }
func (m *fakeMessage) Attempt() int { return m.attempt }
func (m *fakeMessage) Ack() error   { m.outcome = "ack"; return nil }
func (m *fakeMessage) Retry(d time.Duration) error {
	m.outcome, m.delay = "retry", d
	return nil
}
func (m *fakeMessage) Drop() error { m.outcome = "drop"; return nil }

func TestWebhookDispatcher_FanOutAndRetryDecisions(t *testing.T) {
	encrypted, err := utils.EncryptBytes([]byte(testSigningSecret()), testWebhookKey)
	require.NoError(t, err)
	ep := models.WebhookEndpoint{
		ID: uuid.New(), OUID: "ou-1", Scope: models.WebhookScopeOrg, URL: "https://h.example.com",
		EventTypes: []string{"project.created"}, Enabled: true, SecretEncrypted: encrypted,
	}
	other := ep
	other.ID = uuid.New()
	other.EventTypes = []string{"project.deleted"}

	repo, _ := newWebhookRepoMock()
	recorded := map[string]models.WebhookDelivery{}
	repo.MatchingEndpointsFunc = func(context.Context, string, string, string, string, string) ([]models.WebhookEndpoint, error) {
		return []models.WebhookEndpoint{ep, other}, nil
	}
	repo.GetEndpointByIDFunc = func(context.Context, uuid.UUID) (*models.WebhookEndpoint, error) {
		e := ep
		return &e, nil
	}
	repo.RecordQueuedDeliveryFunc = func(context.Context, *models.WebhookDelivery) error { return nil }
	repo.RecordDeliveryFunc = func(_ context.Context, d *models.WebhookDelivery) error {
		recorded[d.Status] = *d
		return nil
	}
	sender := &fakeWebhookSender{}
	bus := &fakeBus{enqueued: map[string][]byte{}}
	d := NewWebhookDispatcherService(bus, repo, sender, testWebhookKey,
		config.Config{Webhooks: config.WebhooksConfig{MaxAttempts: 3}}, discardLogger()).(*webhookDispatcher)
	ctx := context.Background()

	// Fan-out queues one delivery, for the subscribed endpoint only.
	evt, _ := json.Marshal(events.New("evt_1", "project.created", events.ScopeOrg, "ou-1", "", "", time.Now()))
	in := &fakeMessage{data: evt, attempt: 1}
	d.handleEvent(ctx, in)
	assert.Equal(t, "ack", in.outcome)
	require.Len(t, bus.enqueued, 1)
	body := bus.enqueued["evt_1:"+ep.ID.String()]
	require.NotNil(t, body)

	// A 503 is retried with the first backoff step.
	sender.code, sender.err = http.StatusServiceUnavailable, assert.AnError
	m := &fakeMessage{data: body, attempt: 1}
	d.handleDelivery(ctx, m)
	assert.Equal(t, "retry", m.outcome)
	assert.Equal(t, webhookRetryBackoff[0], m.delay)
	assert.Equal(t, models.WebhookDeliveryPending, recorded[models.WebhookDeliveryPending].Status)

	// The last attempt gives up.
	m = &fakeMessage{data: body, attempt: 3}
	d.handleDelivery(ctx, m)
	assert.Equal(t, "drop", m.outcome)
	assert.Equal(t, 3, recorded[models.WebhookDeliveryFailed].Attempts)

	// A 404 is not retried.
	sender.code = http.StatusNotFound
	m = &fakeMessage{data: body, attempt: 1}
	d.handleDelivery(ctx, m)
	assert.Equal(t, "drop", m.outcome)

	// A 2xx is delivered.
	sender.code, sender.err = http.StatusOK, nil
	m = &fakeMessage{data: body, attempt: 2}
	d.handleDelivery(ctx, m)
	assert.Equal(t, "ack", m.outcome)
	assert.Equal(t, 2, recorded[models.WebhookDeliveryDelivered].Attempts)

	// Garbage is dropped, not retried forever.
	m = &fakeMessage{data: []byte("not json"), attempt: 1}
	d.handleDelivery(ctx, m)
	assert.Equal(t, "drop", m.outcome)
}
