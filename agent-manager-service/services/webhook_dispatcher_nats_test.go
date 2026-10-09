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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
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

// End to end over JetStream: an event is published, fanned out to the
// matching endpoint only, signed, retried after a 503 and recorded as
// delivered. Runs on the embedded in-memory server; set NATS_TEST_URL (e.g.
// nats://localhost:4222) to run it against a real NATS server instead.
func TestWebhookDispatcher_EndToEndOverNATS(t *testing.T) {
	saved := webhookRetryBackoff
	webhookRetryBackoff = []time.Duration{200 * time.Millisecond}
	t.Cleanup(func() { webhookRetryBackoff = saved })

	var mu sync.Mutex
	var received []*http.Request
	var bodies [][]byte
	calls := 0
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable) // first attempt fails
			return
		}
		received = append(received, r.Clone(context.Background()))
		bodies = append(bodies, body)
	}))
	defer receiver.Close()

	secret := testSigningSecret()
	encrypted, err := utils.EncryptBytes([]byte(secret), testWebhookKey)
	require.NoError(t, err)
	ou := "ou-" + uuid.NewString()[:8]
	match := models.WebhookEndpoint{
		ID: uuid.New(), OUID: ou, Scope: models.WebhookScopeAgent, ProjectName: "p1", AgentName: "a1",
		Environments: []string{"dev"}, URL: receiver.URL, EventTypes: []string{"agent.deployed"}, Enabled: true,
		SecretEncrypted: encrypted,
	}
	unsubscribed := match
	unsubscribed.ID = uuid.New()
	unsubscribed.EventTypes = []string{"agent.updated"}

	repo, _ := newWebhookRepoMock()
	var recMu sync.Mutex
	recorded := map[string]models.WebhookDelivery{}
	repo.MatchingEndpointsFunc = func(_ context.Context, ouID, scope, project, agent, env string) ([]models.WebhookEndpoint, error) {
		if ouID == ou && scope == "agent" && project == "p1" && agent == "a1" && env == "dev" {
			return []models.WebhookEndpoint{match, unsubscribed}, nil
		}
		return nil, nil
	}
	repo.GetEndpointByIDFunc = func(_ context.Context, id uuid.UUID) (*models.WebhookEndpoint, error) {
		e := match
		return &e, nil
	}
	repo.RecordQueuedDeliveryFunc = func(context.Context, *models.WebhookDelivery) error { return nil }
	repo.RecordDeliveryFunc = func(_ context.Context, d *models.WebhookDelivery) error {
		recMu.Lock()
		defer recMu.Unlock()
		recorded[d.EndpointID.String()] = *d
		return nil
	}
	repo.PruneDeliveriesFunc = func(context.Context, time.Time) (int64, error) { return 0, nil }

	var bus *events.NATSBus
	if natsURL := os.Getenv("NATS_TEST_URL"); natsURL != "" {
		bus, err = events.Connect(natsURL, "dispatcher-test", discardLogger())
	} else {
		bus, err = events.StartEmbedded(discardLogger())
	}
	require.NoError(t, err)
	defer bus.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	require.NoError(t, bus.Setup(ctx))

	cfg := config.Config{Webhooks: config.WebhooksConfig{DispatcherEnabled: true, MaxAttempts: 3, AllowPrivateEndpoints: true}}
	d := NewWebhookDispatcherService(bus, repo, NewWebhookSender(cfg), testWebhookKey, cfg, discardLogger())
	require.NoError(t, d.Start(ctx))
	defer d.Stop()

	pub := events.NewAsyncPublisher(bus, discardLogger())
	pub.Start()
	evt := events.New("evt_"+uuid.NewString(), "agent.deployed", events.ScopeAgent, ou, "p1", "a1", time.Now())
	evt.Environment = "dev"
	evt.Data = map[string]any{"imageId": "img-1"}
	pub.Publish(ctx, evt)
	pub.Publish(ctx, evt) // a duplicate publish is dropped by JetStream
	pub.Stop()

	// The first attempt gets 503 and is retried after the first backoff step.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	}, 30*time.Second, 200*time.Millisecond)
	time.Sleep(time.Second) // nothing further arrives

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received, 1, "one delivery: the duplicate publish and the unsubscribed endpoint send nothing")
	req, body := received[0], bodies[0]
	assert.Equal(t, evt.ID, req.Header.Get(WebhookHeaderID))
	assert.Equal(t, "com.wso2.agentmanager.agent.deployed", req.Header.Get(WebhookHeaderEventType))
	assert.Equal(t, "application/cloudevents+json", req.Header.Get("Content-Type"))
	ts, _ := strconv.ParseInt(req.Header.Get(WebhookHeaderTimestamp), 10, 64)
	want, err := SignWebhook(secret, evt.ID, time.Unix(ts, 0), body)
	require.NoError(t, err)
	assert.Equal(t, want, req.Header.Get(WebhookHeaderSignature), "the signature verifies")

	var got events.Event
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Equal(t, "img-1", got.Data.(map[string]any)["imageId"])

	recMu.Lock()
	defer recMu.Unlock()
	d1 := recorded[match.ID.String()]
	assert.Equal(t, models.WebhookDeliveryDelivered, d1.Status)
	assert.Equal(t, 2, d1.Attempts)
	_, sentToUnsubscribed := recorded[unsubscribed.ID.String()]
	assert.False(t, sentToUnsubscribed)
}
