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
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations/webhook"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// queueBus is a Bus with no transport behind it, to drive the dispatcher's
// handlers directly. It shows the dispatcher needs only the Bus interface.
type queueBus struct {
	enqueued map[string][]byte
}

func (f *queueBus) Setup(context.Context) error                              { return nil }
func (f *queueBus) PublishEvent(context.Context, events.Event, []byte) error { return nil }
func (f *queueBus) SubscribeEvents(context.Context, string, func(events.Message)) (events.Subscription, error) {
	return stopNoop{}, nil
}

func (f *queueBus) Enqueue(_ context.Context, _ string, key string, body []byte) error {
	f.enqueued[key] = body
	return nil
}

func (f *queueBus) Consume(context.Context, string, string, func(events.Message)) (events.Subscription, error) {
	return stopNoop{}, nil
}
func (f *queueBus) Close() {}

type stopNoop struct{}

func (stopNoop) Stop() {}

// answeredMessage records how the dispatcher answered it.
type answeredMessage struct {
	data    []byte
	attempt int
	outcome string
	delay   time.Duration
}

func (m *answeredMessage) Data() []byte { return m.data }
func (m *answeredMessage) Attempt() int { return m.attempt }
func (m *answeredMessage) Ack() error   { m.outcome = "ack"; return nil }
func (m *answeredMessage) Retry(d time.Duration) error {
	m.outcome, m.delay = "retry", d
	return nil
}
func (m *answeredMessage) Drop() error { m.outcome = "drop"; return nil }

func discardSlog() *slog.Logger { return slog.New(slog.DiscardHandler) }

// dispatcherFixture is a dispatcher over a queueBus and a repository mock
// holding the given subscriptions, destinations and delivery rows.
type dispatcherFixture struct {
	mu            sync.Mutex
	subscriptions map[uuid.UUID]models.EventSubscription
	destinations  map[uuid.UUID]models.EventDestination
	deliveries    map[string]models.EventDelivery // by destination ID and event ID
	recorded      []models.EventDelivery
	repo          *repomocks.EventRepositoryMock
	bus           *queueBus
	driver        *fakeDestinationDriver
	dispatcher    *eventDispatcher
}

func deliveryKey(destination uuid.UUID, eventID string) string {
	return destination.String() + "/" + eventID
}

func newDispatcherFixture(t *testing.T) *dispatcherFixture {
	t.Helper()
	f := &dispatcherFixture{
		subscriptions: map[uuid.UUID]models.EventSubscription{},
		destinations:  map[uuid.UUID]models.EventDestination{},
		deliveries:    map[string]models.EventDelivery{},
		bus:           &queueBus{enqueued: map[string][]byte{}},
		driver:        &fakeDestinationDriver{},
	}
	f.repo = &repomocks.EventRepositoryMock{
		CandidateSubscriptionsFunc: func(_ context.Context, ouID, _, _ string) ([]models.EventSubscription, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			var out []models.EventSubscription
			for _, s := range f.subscriptions {
				if s.OUID == ouID && s.Enabled {
					out = append(out, s)
				}
			}
			return out, nil
		},
		DestinationsByIDFunc: func(_ context.Context, ouID string, ids []uuid.UUID) ([]models.EventDestination, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			var out []models.EventDestination
			for _, id := range ids {
				if d, ok := f.destinations[id]; ok && d.OUID == ouID {
					out = append(out, d)
				}
			}
			return out, nil
		},
		GetDestinationFunc: func(_ context.Context, ouID string, id uuid.UUID) (*models.EventDestination, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			d, ok := f.destinations[id]
			if !ok || d.OUID != ouID {
				return nil, gorm.ErrRecordNotFound
			}
			return &d, nil
		},
		SubscriptionsByIDFunc: func(_ context.Context, _ string, ids []uuid.UUID) ([]models.EventSubscription, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			var out []models.EventSubscription
			for _, id := range ids {
				if s, ok := f.subscriptions[id]; ok {
					out = append(out, s)
				}
			}
			return out, nil
		},
		RecordQueuedDeliveryFunc: func(_ context.Context, d *models.EventDelivery) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			key := deliveryKey(d.DestinationID, d.EventID)
			if _, ok := f.deliveries[key]; !ok {
				f.deliveries[key] = *d
			}
			return nil
		},
		GetDeliveryFunc: func(_ context.Context, destination uuid.UUID, eventID string) (*models.EventDelivery, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			d, ok := f.deliveries[deliveryKey(destination, eventID)]
			if !ok {
				return nil, gorm.ErrRecordNotFound
			}
			return &d, nil
		},
		RecordDeliveryFunc: func(_ context.Context, d *models.EventDelivery) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.deliveries[deliveryKey(d.DestinationID, d.EventID)] = *d
			f.recorded = append(f.recorded, *d)
			return nil
		},
		ClaimDeliveryFunc:   func(context.Context, uuid.UUID, time.Duration) (bool, error) { return true, nil },
		PruneDeliveriesFunc: func(context.Context, time.Time) (int64, error) { return 0, nil },
	}
	resolver := NewEventDestinationService(f.repo, eventdestinations.NewRegistry(f.driver), testEventKey, discardSlog())
	f.dispatcher = NewEventDispatcherService(f.bus, f.repo, resolver,
		config.Config{Events: config.EventsConfig{DeliveryMaxAttempts: 3}}, discardSlog()).(*eventDispatcher)
	return f
}

func (f *dispatcherFixture) destination(availability string, projects ...string) models.EventDestination {
	d := models.EventDestination{
		ID: uuid.New(), OUID: "ou-1", Name: "d-" + uuid.NewString()[:4], Kind: "fake",
		Config: json.RawMessage(`{"dest":"x"}`), Availability: availability, Projects: projects,
	}
	f.destinations[d.ID] = d
	return d
}

func (f *dispatcherFixture) subscription(
	scope, project, agent string, destination uuid.UUID, types ...string,
) models.EventSubscription {
	s := matcherSubscription(scope, project, agent, types)
	s.DestinationID = destination
	f.subscriptions[s.ID] = s
	return s
}

func publish(t *testing.T, e events.Event) *answeredMessage {
	t.Helper()
	body, err := json.Marshal(e)
	require.NoError(t, err)
	return &answeredMessage{data: body, attempt: 1}
}

func TestEventDispatcher_FanOutOncePerDestination(t *testing.T) {
	f := newDispatcherFixture(t)
	slack := f.destination(models.DestinationAvailableAll)
	pager := f.destination(models.DestinationAvailableProjects, "payments")
	orgOnly := f.destination(models.DestinationAvailableOrg)
	orgAll := f.subscription("org", "", "", slack.ID, events.AllEvents)
	agentDeploys := f.subscription("agent", "payments", "refund-agent", slack.ID, "agent.deployed")
	projectDeploys := f.subscription("project", "payments", "", pager.ID, "agent.deployed")
	f.subscription("project", "payments", "", orgOnly.ID, "agent.deployed") // stranded: never sent
	f.subscription("agent", "payments", "refund-agent", pager.ID, "agent.updated")
	ctx := context.Background()

	evt := matcherEvent("agent.deployed", events.ScopeAgent, "payments", "refund-agent", "production")
	msg := publish(t, evt)
	f.dispatcher.handleEvent(ctx, msg)
	assert.Equal(t, "ack", msg.outcome)
	assert.Len(t, f.bus.enqueued, 2, "one delivery per destination, however many subscriptions match")
	assert.Contains(t, f.bus.enqueued, evt.ID+":"+slack.ID.String())
	assert.Contains(t, f.bus.enqueued, evt.ID+":"+pager.ID.String())

	toSlack := f.deliveries[deliveryKey(slack.ID, evt.ID)]
	wantIDs := []string{orgAll.ID.String(), agentDeploys.ID.String()}
	sort.Strings(wantIDs)
	assert.Equal(t, wantIDs, toSlack.SubscriptionIDs, "the delivery names both subscriptions")
	assert.Equal(t, models.EventDeliveryPending, toSlack.Status)
	assert.Equal(t, "agent", toSlack.EventScope)
	assert.Equal(t, "payments", toSlack.ProjectName)
	assert.Equal(t, "refund-agent", toSlack.AgentName)
	assert.Equal(t, "production", toSlack.Environment)
	assert.Equal(t, []string{projectDeploys.ID.String()}, f.deliveries[deliveryKey(pager.ID, evt.ID)].SubscriptionIDs)

	// A repeated fan-out (the ack was lost) queues the same work.
	f.dispatcher.handleEvent(ctx, publish(t, evt))
	assert.Len(t, f.bus.enqueued, 2)

	malformed := &answeredMessage{data: []byte("{"), attempt: 1}
	f.dispatcher.handleEvent(ctx, malformed)
	assert.Equal(t, "drop", malformed.outcome)
}

func TestEventDispatcher_EventNobodyWants(t *testing.T) {
	f := newDispatcherFixture(t)
	dest := f.destination(models.DestinationAvailableAll)
	f.subscription("agent", "payments", "refund-agent", dest.ID, events.AllEvents)

	msg := publish(t, matcherEvent("gateway.created", events.ScopeOrg, "", "", ""))
	f.dispatcher.handleEvent(context.Background(), msg)
	assert.Equal(t, "ack", msg.outcome)
	assert.Empty(t, f.bus.enqueued)
	assert.Empty(t, f.repo.DestinationsByIDCalls(), "no destination is loaded when no subscription matches")
}

func TestEventDispatcher_FanOutRetriesOnStoreErrors(t *testing.T) {
	f := newDispatcherFixture(t)
	f.repo.CandidateSubscriptionsFunc = func(context.Context, string, string, string) ([]models.EventSubscription, error) {
		return nil, errors.New("db down")
	}
	msg := publish(t, matcherEvent("agent.deployed", events.ScopeAgent, "payments", "refund-agent", ""))
	f.dispatcher.handleEvent(context.Background(), msg)
	assert.Equal(t, "retry", msg.outcome)
	assert.Equal(t, eventFanoutRetryDelay, msg.delay)
}

func TestEventDispatcher_DeliveryDecisions(t *testing.T) {
	f := newDispatcherFixture(t)
	dest := f.destination(models.DestinationAvailableAll)
	sub := f.subscription("agent", "payments", "refund-agent", dest.ID, "agent.deployed")
	ctx := context.Background()
	evt := matcherEvent("agent.deployed", events.ScopeAgent, "payments", "refund-agent", "")
	f.dispatcher.handleEvent(ctx, publish(t, evt))
	body := f.bus.enqueued[evt.ID+":"+dest.ID.String()]
	require.NotNil(t, body)
	last := func() models.EventDelivery { return f.recorded[len(f.recorded)-1] }

	// A retryable failure is retried with the first backoff step.
	f.driver.result = eventdestinations.Result{StatusCode: http.StatusServiceUnavailable, Err: errors.New("down"), Retryable: true}
	m := &answeredMessage{data: body, attempt: 1}
	f.dispatcher.handleDelivery(ctx, m)
	assert.Equal(t, "retry", m.outcome)
	assert.Equal(t, eventRetryBackoff[0], m.delay)
	assert.Equal(t, models.EventDeliveryPending, last().Status)
	assert.Equal(t, 1, last().Attempts)
	assert.Equal(t, "down", last().LastError)
	require.NotNil(t, last().ResponseCode)
	assert.Equal(t, http.StatusServiceUnavailable, *last().ResponseCode)

	// A second actual send keeps retrying even if the bus has redelivered
	// the job many times for infrastructure failures.
	m = &answeredMessage{data: body, attempt: 20}
	f.dispatcher.handleDelivery(ctx, m)
	assert.Equal(t, "retry", m.outcome)
	assert.Equal(t, eventRetryBackoff[1], m.delay)
	assert.Equal(t, 2, last().Attempts)

	// The last attempt gives up.
	m = &answeredMessage{data: body, attempt: 3}
	f.dispatcher.handleDelivery(ctx, m)
	assert.Equal(t, "drop", m.outcome)
	assert.Equal(t, models.EventDeliveryFailed, last().Status)
	assert.Equal(t, 3, last().Attempts)

	// Each remaining scenario starts with a pending delivery.
	resetPending := func() {
		key := deliveryKey(dest.ID, evt.ID)
		row := f.deliveries[key]
		row.Status, row.Attempts = models.EventDeliveryPending, 0
		f.deliveries[key] = row
	}
	resetPending()
	// A failure the driver calls final is not retried.
	f.driver.result = eventdestinations.Result{StatusCode: http.StatusNotFound, Err: errors.New("gone")}
	m = &answeredMessage{data: body, attempt: 1}
	f.dispatcher.handleDelivery(ctx, m)
	assert.Equal(t, "drop", m.outcome)

	resetPending()
	// Success is recorded as delivered.
	f.driver.result = eventdestinations.Result{StatusCode: http.StatusOK}
	m = &answeredMessage{data: body, attempt: 2}
	f.dispatcher.handleDelivery(ctx, m)
	assert.Equal(t, "ack", m.outcome)
	assert.Equal(t, models.EventDeliveryDelivered, last().Status)
	assert.NotNil(t, last().DeliveredAt)
	assert.Empty(t, last().LastError)

	// A job whose delivery is already recorded as delivered is not sent again.
	sent := f.driver.sends()
	m = &answeredMessage{data: body, attempt: 3}
	f.dispatcher.handleDelivery(ctx, m)
	assert.Equal(t, "ack", m.outcome)
	assert.Equal(t, sent, f.driver.sends(), "no second send")

	// A delivery nobody wants any more is skipped, not sent.
	key := deliveryKey(dest.ID, evt.ID)
	pending := f.deliveries[key]
	pending.Status = models.EventDeliveryPending
	f.deliveries[key] = pending
	disabled := f.subscriptions[sub.ID]
	disabled.Enabled = false
	f.subscriptions[sub.ID] = disabled
	m = &answeredMessage{data: body, attempt: 2}
	f.dispatcher.handleDelivery(ctx, m)
	assert.Equal(t, "drop", m.outcome)
	assert.Equal(t, models.EventDeliverySkipped, last().Status)
	assert.Equal(t, sent, f.driver.sends())

	// A destination deleted since takes its delivery rows with it: the job
	// is dropped.
	delete(f.deliveries, key)
	m = &answeredMessage{data: body, attempt: 1}
	f.dispatcher.handleDelivery(ctx, m)
	assert.Equal(t, "drop", m.outcome)
}

func TestEventDispatcher_RetriesWhenOutcomeCannotBeStored(t *testing.T) {
	for _, tc := range []struct {
		name       string
		result     eventdestinations.Result
		attempt    int
		disable    bool
		unreadable bool
		status     string
		outcome    string
	}{
		{name: "permanent failure", result: eventdestinations.Result{Err: errors.New("invalid target")}, attempt: 1, status: models.EventDeliveryFailed, outcome: "drop"},
		{name: "attempts exhausted", result: eventdestinations.Result{Err: errors.New("unavailable"), Retryable: true}, attempt: 3, status: models.EventDeliveryFailed, outcome: "drop"},
		{name: "pending", result: eventdestinations.Result{Err: errors.New("unavailable"), Retryable: true}, attempt: 1, status: models.EventDeliveryPending, outcome: "retry"},
		{name: "skipped", attempt: 1, disable: true, status: models.EventDeliverySkipped, outcome: "drop"},
		{name: "unreadable destination", attempt: 1, unreadable: true, status: models.EventDeliveryFailed, outcome: "drop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDispatcherFixture(t)
			dest := f.destination(models.DestinationAvailableAll)
			sub := f.subscription("agent", "payments", "refund-agent", dest.ID, "agent.deployed")
			ctx := context.Background()
			evt := matcherEvent("agent.deployed", events.ScopeAgent, "payments", "refund-agent", "")
			f.dispatcher.handleEvent(ctx, publish(t, evt))
			body := f.bus.enqueued[evt.ID+":"+dest.ID.String()]
			require.NotNil(t, body)
			f.driver.result = tc.result
			key := deliveryKey(dest.ID, evt.ID)
			prior := f.deliveries[key]
			prior.Attempts = tc.attempt - 1
			f.deliveries[key] = prior
			if tc.disable {
				sub.Enabled = false
				f.subscriptions[sub.ID] = sub
			}
			if tc.unreadable {
				dest.Kind = "unknown"
				f.destinations[dest.ID] = dest
			}
			persist := f.repo.RecordDeliveryFunc
			f.repo.RecordDeliveryFunc = func(_ context.Context, delivery *models.EventDelivery) error {
				assert.Equal(t, tc.status, delivery.Status)
				return errors.New("database unavailable")
			}
			msg := &answeredMessage{data: body, attempt: tc.attempt}
			f.dispatcher.handleDelivery(ctx, msg)
			assert.Equal(t, "retry", msg.outcome, "failed persistence must not acknowledge or terminate the job")
			assert.Equal(t, eventFanoutRetryDelay, msg.delay)
			assert.Empty(t, f.recorded)

			// Once persistence recovers, the redelivered job can finish.
			f.repo.RecordDeliveryFunc = persist
			msg = &answeredMessage{data: body, attempt: tc.attempt}
			f.dispatcher.handleDelivery(ctx, msg)
			assert.Equal(t, tc.outcome, msg.outcome)
			require.Len(t, f.recorded, 1)
			assert.Equal(t, tc.status, f.recorded[0].Status)
			if tc.disable || tc.unreadable {
				assert.Zero(t, f.driver.sends())
			}
		})
	}
}

func TestEventDispatcher_DeliveredButNotRecordedIsNotSentAgain(t *testing.T) {
	f := newDispatcherFixture(t)
	dest := f.destination(models.DestinationAvailableAll)
	f.subscription("agent", "payments", "refund-agent", dest.ID, "agent.deployed")
	ctx := context.Background()
	evt := matcherEvent("agent.deployed", events.ScopeAgent, "payments", "refund-agent", "")
	f.dispatcher.handleEvent(ctx, publish(t, evt))
	body := f.bus.enqueued[evt.ID+":"+dest.ID.String()]
	require.NotNil(t, body)
	f.repo.RecordDeliveryFunc = func(context.Context, *models.EventDelivery) error {
		return errors.New("database unavailable")
	}
	msg := &answeredMessage{data: body, attempt: 1}
	f.dispatcher.handleDelivery(ctx, msg)
	assert.Equal(t, "ack", msg.outcome, "a sent event is not queued to be sent again")
	assert.Equal(t, 1, f.driver.sends())
}

func TestEventDispatcher_ClaimedDeliveryIsNotSentTwice(t *testing.T) {
	f := newDispatcherFixture(t)
	dest := f.destination(models.DestinationAvailableAll)
	f.subscription("agent", "payments", "refund-agent", dest.ID, "agent.deployed")
	ctx := context.Background()
	evt := matcherEvent("agent.deployed", events.ScopeAgent, "payments", "refund-agent", "")
	f.dispatcher.handleEvent(ctx, publish(t, evt))
	body := f.bus.enqueued[evt.ID+":"+dest.ID.String()]
	require.NotNil(t, body)

	// Another worker is sending this delivery: the redelivered job waits out
	// its claim instead of sending.
	f.repo.ClaimDeliveryFunc = func(context.Context, uuid.UUID, time.Duration) (bool, error) { return false, nil }
	msg := &answeredMessage{data: body, attempt: 2}
	f.dispatcher.handleDelivery(ctx, msg)
	assert.Equal(t, "retry", msg.outcome)
	assert.Equal(t, eventDeliveryClaim, msg.delay)
	assert.Zero(t, f.driver.sends())

	f.repo.ClaimDeliveryFunc = func(context.Context, uuid.UUID, time.Duration) (bool, error) {
		return false, errors.New("database unavailable")
	}
	msg = &answeredMessage{data: body, attempt: 2}
	f.dispatcher.handleDelivery(ctx, msg)
	assert.Equal(t, "retry", msg.outcome)
	assert.Equal(t, eventFanoutRetryDelay, msg.delay)
	assert.Zero(t, f.driver.sends())
}

func TestEventDispatcher_PanicIsRetriedNotFatal(t *testing.T) {
	f := newDispatcherFixture(t)
	msg := &answeredMessage{data: []byte("{}"), attempt: 1}
	assert.NotPanics(t, func() {
		f.dispatcher.recovering(msg, time.Minute, func() { panic("driver bug") })
	})
	assert.Equal(t, "retry", msg.outcome)
	assert.Equal(t, time.Minute, msg.delay)
}

func TestEventDispatcher_SkipsWhenTheDestinationWasNarrowed(t *testing.T) {
	f := newDispatcherFixture(t)
	dest := f.destination(models.DestinationAvailableAll)
	f.subscription("project", "payments", "", dest.ID, "agent.deployed")
	ctx := context.Background()
	evt := matcherEvent("agent.deployed", events.ScopeAgent, "payments", "refund-agent", "")
	f.dispatcher.handleEvent(ctx, publish(t, evt))
	body := f.bus.enqueued[evt.ID+":"+dest.ID.String()]
	require.NotNil(t, body)

	narrowed := f.destinations[dest.ID]
	narrowed.Availability = models.DestinationAvailableOrg
	f.destinations[dest.ID] = narrowed
	m := &answeredMessage{data: body, attempt: 1}
	f.dispatcher.handleDelivery(ctx, m)
	assert.Equal(t, "drop", m.outcome)
	assert.Zero(t, f.driver.sends(), "a project that lost the destination sends nothing to it")
}

func TestEventDispatcher_StartWithoutBusIsANoop(t *testing.T) {
	d := NewEventDispatcherService(nil, &repomocks.EventRepositoryMock{}, nil, config.Config{}, discardSlog())
	require.NoError(t, d.Start(context.Background()))
	require.NoError(t, d.Stop())
}

func TestEventBackoff(t *testing.T) {
	assert.Equal(t, eventRetryBackoff[0], eventBackoff(0))
	assert.Equal(t, eventRetryBackoff[1], eventBackoff(2))
	assert.Equal(t, eventRetryBackoff[len(eventRetryBackoff)-1], eventBackoff(99))
}

// End to end over JetStream: an event is published, matched by an agent
// subscription and an org one sending to the same webhook, delivered once,
// signed by the webhook driver, retried after a 503 and recorded as
// delivered. Runs on the embedded in-memory server; set NATS_TEST_URL (e.g.
// nats://localhost:4222) to use a real NATS server.
func TestEventDispatcher_EndToEndOverNATS(t *testing.T) {
	saved := eventRetryBackoff
	eventRetryBackoff = []time.Duration{200 * time.Millisecond}
	t.Cleanup(func() { eventRetryBackoff = saved })

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
			w.WriteHeader(http.StatusServiceUnavailable) // the first attempt fails
			return
		}
		received = append(received, r.Clone(context.Background()))
		bodies = append(bodies, body)
	}))
	defer receiver.Close()

	secret := webhook.SecretPrefix + base64.StdEncoding.EncodeToString([]byte("test-signing-key-for-unit-tests!"))
	encrypted, err := utils.EncryptBytes([]byte(secret), testEventKey)
	require.NoError(t, err)
	ou := "ou-" + uuid.NewString()[:8]
	cfgJSON, _ := json.Marshal(webhook.Config{URL: receiver.URL})

	f := newDispatcherFixture(t)
	dest := models.EventDestination{
		ID: uuid.New(), OUID: ou, Name: "CI", Kind: models.DestinationKindWebhook, Config: cfgJSON,
		SecretEncrypted: encrypted, Availability: models.DestinationAvailableAll,
	}
	f.destinations[dest.ID] = dest
	match := matcherSubscription("agent", "p1", "a1", []string{"agent.deployed"}, "dev")
	match.OUID, match.DestinationID = ou, dest.ID
	orgAll := matcherSubscription("org", "", "", []string{events.AllEvents})
	orgAll.OUID, orgAll.DestinationID = ou, dest.ID
	unsubscribed := matcherSubscription("agent", "p1", "a1", []string{"agent.updated"})
	unsubscribed.OUID, unsubscribed.DestinationID = ou, dest.ID
	for _, s := range []models.EventSubscription{match, orgAll, unsubscribed} {
		f.subscriptions[s.ID] = s
	}

	var bus *events.NATSBus
	if natsURL := os.Getenv("NATS_TEST_URL"); natsURL != "" {
		bus, err = events.Connect(natsURL, "", "dispatcher-test", discardSlog())
	} else {
		bus, err = events.StartEmbedded(discardSlog())
	}
	require.NoError(t, err)
	defer bus.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	require.NoError(t, bus.Setup(ctx))

	cfg := config.Config{Events: config.EventsConfig{DeliveryMaxAttempts: 3, AllowPrivateDestinations: true}}
	resolver := NewEventDestinationService(f.repo, eventdestinations.NewRegistry(webhook.New(true)), testEventKey, discardSlog())
	d := NewEventDispatcherService(bus, f.repo, resolver, cfg, discardSlog())
	require.NoError(t, d.Start(ctx))
	defer func() { _ = d.Stop() }()

	evt := events.New("evt_"+uuid.NewString(), "agent.deployed", events.ScopeAgent, ou, "p1", "a1", time.Now())
	evt.Environment = "dev"
	evt.Data = map[string]any{"imageId": "img-1"}
	bodyBytes, err := json.Marshal(evt)
	require.NoError(t, err)
	require.NoError(t, bus.PublishEvent(ctx, evt, bodyBytes))
	require.NoError(t, bus.PublishEvent(ctx, evt, bodyBytes)) // JetStream drops the duplicate.

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	}, 30*time.Second, 200*time.Millisecond)
	time.Sleep(time.Second) // nothing further arrives

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received, 1, "one delivery: two matching subscriptions share the destination")
	req, body := received[0], bodies[0]
	assert.Equal(t, evt.ID, req.Header.Get(webhook.HeaderID))
	assert.Equal(t, "com.wso2.agentmanager.agent.deployed", req.Header.Get(webhook.HeaderEventType))
	assert.Equal(t, events.MediaType, req.Header.Get("Content-Type"))
	ts, _ := strconv.ParseInt(req.Header.Get(webhook.HeaderTimestamp), 10, 64)
	want, err := webhook.Sign(secret, evt.ID, time.Unix(ts, 0), body)
	require.NoError(t, err)
	assert.Equal(t, want, req.Header.Get(webhook.HeaderSignature), "the signature verifies")

	var got events.Event
	require.NoError(t, json.Unmarshal(body, &got))
	assert.Equal(t, "img-1", got.Data.(map[string]any)["imageId"])

	f.mu.Lock()
	defer f.mu.Unlock()
	delivery := f.deliveries[deliveryKey(dest.ID, evt.ID)]
	assert.Equal(t, models.EventDeliveryDelivered, delivery.Status)
	assert.Equal(t, 2, delivery.Attempts)
	wantIDs := []string{match.ID.String(), orgAll.ID.String()}
	sort.Strings(wantIDs)
	assert.Equal(t, wantIDs, delivery.SubscriptionIDs)
}

func TestEventDispatcher_EditedFiltersCancelQueuedDelivery(t *testing.T) {
	for _, field := range []string{"event type", "environment"} {
		t.Run(field, func(t *testing.T) {
			f := newDispatcherFixture(t)
			dest := f.destination(models.DestinationAvailableAll)
			sub := f.subscription("agent", "payments", "refund-agent", dest.ID, "agent.deployed")
			evt := matcherEvent("agent.deployed", events.ScopeAgent, "payments", "refund-agent", "dev")
			f.dispatcher.handleEvent(context.Background(), publish(t, evt))
			if field == "event type" {
				sub.EventTypes = []string{"agent.updated"}
			} else {
				sub.Environments = []string{"prod"}
			}
			f.subscriptions[sub.ID] = sub
			msg := &answeredMessage{data: f.bus.enqueued[evt.ID+":"+dest.ID.String()], attempt: 2}
			f.dispatcher.handleDelivery(context.Background(), msg)
			assert.Zero(t, f.driver.sends(), "edited subscription excludes this event")
			assert.Equal(t, models.EventDeliverySkipped, f.recorded[0].Status)
		})
	}
}

func TestEventDispatcher_StoreRetriesDoNotConsumeSendBudget(t *testing.T) {
	f := newDispatcherFixture(t)
	dest := f.destination(models.DestinationAvailableAll)
	f.subscription("agent", "payments", "refund-agent", dest.ID, "agent.deployed")
	evt := matcherEvent("agent.deployed", events.ScopeAgent, "payments", "refund-agent", "")
	f.dispatcher.handleEvent(context.Background(), publish(t, evt))
	body := f.bus.enqueued[evt.ID+":"+dest.ID.String()]
	get := f.repo.GetDeliveryFunc
	f.repo.GetDeliveryFunc = func(context.Context, uuid.UUID, string) (*models.EventDelivery, error) {
		return nil, errors.New("synthetic DB outage")
	}
	for i := 1; i <= 3; i++ {
		f.dispatcher.handleDelivery(context.Background(), &answeredMessage{data: body, attempt: i})
	}
	f.repo.GetDeliveryFunc = get
	f.driver.result = eventdestinations.Result{Err: errors.New("temporary destination failure"), Retryable: true}
	msg := &answeredMessage{data: body, attempt: 4}
	f.dispatcher.handleDelivery(context.Background(), msg)
	assert.Equal(t, 1, f.driver.sends())
	assert.Equal(t, "retry", msg.outcome, "first actual send should retain retries")
}

func TestEventDispatcherTerminalRedeliveryIsAcknowledged(t *testing.T) {
	for _, status := range []string{models.EventDeliveryDelivered, models.EventDeliverySkipped, models.EventDeliveryFailed} {
		t.Run(status, func(t *testing.T) {
			f := newDispatcherFixture(t)
			dest := f.destination(models.DestinationAvailableAll)
			f.subscription("agent", "payments", "refund-agent", dest.ID, "agent.deployed")
			evt := matcherEvent("agent.deployed", events.ScopeAgent, "payments", "refund-agent", "")
			f.dispatcher.handleEvent(context.Background(), publish(t, evt))
			key := deliveryKey(dest.ID, evt.ID)
			row := f.deliveries[key]
			row.Status = status
			f.deliveries[key] = row
			// No destination lookup or claim is needed for a completed delivery.
			f.repo.GetDestinationFunc = nil
			f.repo.ClaimDeliveryFunc = nil
			msg := &answeredMessage{data: f.bus.enqueued[evt.ID+":"+dest.ID.String()], attempt: 2}
			f.dispatcher.handleDelivery(context.Background(), msg)
			assert.Equal(t, "ack", msg.outcome)
			assert.Zero(t, f.driver.sends())
		})
	}
}

func TestEventDispatcher_MonitorAlertDeliveryEligibility(t *testing.T) {
	for _, tc := range []struct {
		name         string
		availability string
		projects     []string
		wanted       bool
		lookupErr    error
		outcome      string
		checked      bool
		sends        int
	}{
		{name: "active alert", availability: models.DestinationAvailableAll, wanted: true, outcome: "ack", checked: true, sends: 1},
		{name: "allowed project", availability: models.DestinationAvailableProjects, projects: []string{"payments"}, wanted: true, outcome: "ack", checked: true, sends: 1},
		{name: "org only destination", availability: models.DestinationAvailableOrg, wanted: true, outcome: "drop"},
		{name: "project access removed", availability: models.DestinationAvailableProjects, projects: []string{"another-project"}, wanted: true, outcome: "drop"},
		{name: "disabled or deleted alert", availability: models.DestinationAvailableAll, outcome: "drop", checked: true},
		{name: "settings lookup fails", availability: models.DestinationAvailableAll, lookupErr: errors.New("database unavailable"), outcome: "retry", checked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDispatcherFixture(t)
			dest := f.destination(tc.availability, tc.projects...)
			alertID := uuid.New()
			evt := matcherEvent("monitor.alert.score_threshold_breached", events.ScopeAgent, "payments", "refund-agent", "production")
			payload, err := json.Marshal(evt)
			require.NoError(t, err)
			body, err := json.Marshal(eventDeliveryJob{DestinationID: dest.ID.String(), Event: payload})
			require.NoError(t, err)
			delivery := models.EventDelivery{ID: uuid.New(), OUID: dest.OUID, DestinationID: dest.ID, MonitorAlertID: &alertID, EventID: evt.ID, ProjectName: evt.Project, AgentName: evt.Agent, Status: models.EventDeliveryPending}
			f.deliveries[deliveryKey(dest.ID, evt.ID)] = delivery
			f.repo.MonitorAlertWantsFunc = func(_ context.Context, ouID string, gotAlert, gotDestination uuid.UUID, project, agent string) (bool, error) {
				assert.Equal(t, dest.OUID, ouID)
				assert.Equal(t, alertID, gotAlert)
				assert.Equal(t, dest.ID, gotDestination)
				assert.Equal(t, "payments", project)
				assert.Equal(t, "refund-agent", agent)
				return tc.wanted, tc.lookupErr
			}
			// Monitor alerts are shared configuration; personal subscriptions must never be loaded.
			f.repo.SubscriptionsByIDFunc = nil
			msg := &answeredMessage{data: body, attempt: 1}
			f.dispatcher.handleDelivery(context.Background(), msg)
			assert.Equal(t, tc.outcome, msg.outcome)
			assert.Equal(t, tc.sends, f.driver.sends())
			assert.Equal(t, tc.checked, len(f.repo.MonitorAlertWantsCalls()) == 1)
			if tc.outcome == "retry" {
				assert.Equal(t, eventFanoutRetryDelay, msg.delay)
				assert.Empty(t, f.recorded, "store errors must not mark the alert skipped or consume an attempt")
			} else {
				require.Len(t, f.recorded, 1)
				if tc.sends == 0 {
					assert.Equal(t, models.EventDeliverySkipped, f.recorded[0].Status)
					assert.Zero(t, f.recorded[0].Attempts)
				} else {
					assert.Equal(t, models.EventDeliveryDelivered, f.recorded[0].Status)
				}
			}
		})
	}
}
