//go:build integration

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

package tests

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations/webhook"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// webhookReceiver records what a webhook destination is sent.
type webhookReceiver struct {
	mu       sync.Mutex
	requests []receivedWebhook
	status   int
}

type receivedWebhook struct {
	header http.Header
	body   []byte
}

func (rcv *webhookReceiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	rcv.mu.Lock()
	defer rcv.mu.Unlock()
	rcv.requests = append(rcv.requests, receivedWebhook{header: r.Header.Clone(), body: body})
	w.WriteHeader(rcv.status)
}

func (rcv *webhookReceiver) received() []receivedWebhook {
	rcv.mu.Lock()
	defer rcv.mu.Unlock()
	return append([]receivedWebhook(nil), rcv.requests...)
}

// eventsWorld is a coreWorld whose webhook destinations may point at a local
// receiver, as a development install allows.
func eventsWorld(t *testing.T) coreWorld {
	t.Helper()
	cfg := config.GetConfig()
	prev := cfg.Events.AllowPrivateDestinations
	cfg.Events.AllowPrivateDestinations = true
	t.Cleanup(func() { cfg.Events.AllowPrivateDestinations = prev })
	return newCoreWorld(t)
}

func webhookDestination(name, url, availability string, projects ...string) spec.EventDestinationRequest {
	return spec.EventDestinationRequest{
		Name: name, Description: stringPtr("for " + name), Kind: models.DestinationKindWebhook,
		Config: map[string]interface{}{"url": url}, Availability: availability, Projects: projects,
	}
}

// The event routes over the real database: destinations an org admin
// registers, subscriptions at org, project and agent level that inherit the
// wider ones, the delivery log, and a test event that really reaches the
// webhook, signed with the destination's secret.
func TestCoreEventsAPI(t *testing.T) {
	w := eventsWorld(t)
	rcv := &webhookReceiver{status: http.StatusNoContent}
	hook := httptest.NewServer(rcv)
	t.Cleanup(hook.Close)

	mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, w.path("/projects"), spec.CreateProjectRequest{Name: "shop", DisplayName: "Shop"})
	mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, w.path("/projects"), spec.CreateProjectRequest{Name: "lab", DisplayName: "Lab"})
	mustCall(t, w.admin, http.StatusAccepted, http.MethodPost, w.path("/projects/shop/agents"), externalA2A("cashier", "Cashier", nil))

	t.Run("catalog", func(t *testing.T) {
		types := decodeAs[spec.EventTypeListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/events/event-types"), nil))
		assert.NotEmpty(t, types.EventTypes)
		agentTypes := decodeAs[spec.EventTypeListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/events/event-types?scope=agent"), nil))
		assert.NotEmpty(t, agentTypes.EventTypes)
		assert.Less(t, len(agentTypes.EventTypes), len(types.EventTypes), "an agent subscription receives fewer types than the whole catalog")
		status, body := call(t, w.reader, http.MethodGet, w.path("/events/event-types?scope=galaxy"), nil)
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Contains(t, errorOf(t, body).Message, "scope must be org, project or agent")

		kinds := decodeAs[spec.EventDestinationKindListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/events/destination-kinds"), nil))
		names := []string{}
		for _, k := range kinds.Kinds {
			names = append(names, k.Kind)
		}
		assert.Contains(t, names, models.DestinationKindWebhook)
	})

	var everywhere, shopOnly spec.EventDestinationResponse
	t.Run("register destinations", func(t *testing.T) {
		everywhere = decodeAs[spec.EventDestinationResponse](t, mustCall(t, w.admin, http.StatusCreated, http.MethodPost,
			w.path("/events/destinations"), webhookDestination("everywhere", hook.URL+"/hook?token=s3cr3t-path", "all")))
		require.NotNil(t, everywhere.Secret, "a webhook's signing secret is shown once, when it is created")
		assert.Equal(t, "webhook", everywhere.Kind)
		assert.Equal(t, hook.URL+"/hook?token=s3cr3t-path", everywhere.Config["url"])
		assert.Equal(t, []string{}, everywhere.Projects)

		shopOnly = decodeAs[spec.EventDestinationResponse](t, mustCall(t, w.admin, http.StatusCreated, http.MethodPost,
			w.path("/events/destinations"), webhookDestination("shop-only", hook.URL, "projects", "shop")))
		assert.Equal(t, []string{"shop"}, shopOnly.Projects)
	})

	t.Run("destination refusals", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			body   any
			status int
			msg    string
		}{
			{"malformed body", "{", http.StatusBadRequest, "Invalid request body"},
			{"no config", spec.EventDestinationRequest{Name: "x", Kind: "webhook", Availability: "all"}, http.StatusBadRequest, "config must be an object"},
			{"no url", spec.EventDestinationRequest{Name: "x", Kind: "webhook", Availability: "all", Config: map[string]interface{}{}}, http.StatusBadRequest, ""},
			{"unknown kind", spec.EventDestinationRequest{Name: "x", Kind: "pigeon", Availability: "all", Config: map[string]interface{}{"url": hook.URL}}, http.StatusBadRequest, ""},
			{"projects but none chosen", webhookDestination("x", hook.URL, "projects"), http.StatusBadRequest, ""},
			{"name taken", webhookDestination("everywhere", hook.URL, "all"), http.StatusConflict, "An event destination with this name already exists"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				status, body := call(t, w.admin, http.MethodPost, w.path("/events/destinations"), tc.body)
				require.Equal(t, tc.status, status, string(body))
				if tc.msg != "" {
					assert.Equal(t, tc.msg, errorOf(t, body).Message)
				}
			})
		}
		status, _ := call(t, w.reader, http.MethodPost, w.path("/events/destinations"), webhookDestination("reader", hook.URL, "all"))
		assert.Equal(t, http.StatusForbidden, status)
	})

	t.Run("list and get, with the url redacted for a reader", func(t *testing.T) {
		list := decodeAs[spec.EventDestinationListResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodGet, w.path("/events/destinations"), nil))
		require.Len(t, list.Destinations, 2)
		for _, d := range list.Destinations {
			assert.Nil(t, d.Secret, "the secret is never listed")
		}

		got := decodeAs[spec.EventDestinationResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/events/destinations/%s", everywhere.Id), nil))
		assert.Equal(t, "everywhere", got.Name)
		assert.NotContains(t, got.Config["url"], "s3cr3t-path", "a reader cannot see a credential in the url")
		assert.Empty(t, got.Subscriptions)

		mustCall(t, w.reader, http.StatusNotFound, http.MethodGet, w.path("/events/destinations/%s", uuid.NewString()), nil)
		mustCall(t, w.reader, http.StatusNotFound, http.MethodGet, w.path("/events/destinations/not-an-id"), nil)
		mustCall(t, w.stranger, http.StatusNotFound, http.MethodGet, w.path("/events/destinations/%s", everywhere.Id), nil)
		theirs := decodeAs[spec.EventDestinationListResponse](t, mustCall(t, w.stranger, http.StatusOK, http.MethodGet, w.path("/events/destinations"), nil))
		assert.Empty(t, theirs.Destinations)
	})

	t.Run("projects see the destinations made available to them", func(t *testing.T) {
		names := func(path string) []string {
			list := decodeAs[spec.EventDestinationSummaryListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, path, nil))
			out := []string{}
			for _, d := range list.Destinations {
				out = append(out, d.Name)
			}
			return out
		}
		assert.ElementsMatch(t, []string{"everywhere", "shop-only"}, names(w.path("/projects/shop/events/destinations")))
		assert.ElementsMatch(t, []string{"everywhere"}, names(w.path("/projects/lab/events/destinations")))
		assert.ElementsMatch(t, []string{"everywhere", "shop-only"}, names(w.path("/projects/shop/agents/cashier/events/destinations")))
		// A project name nobody uses is not looked up: it is offered only
		// what every project is.
		assert.ElementsMatch(t, []string{"everywhere"}, names(w.path("/projects/nowhere/events/destinations")))
	})

	var orgSub, agentSub spec.EventSubscriptionResponse
	t.Run("subscriptions at each level", func(t *testing.T) {
		orgSub = decodeAs[spec.EventSubscriptionResponse](t, mustCall(t, w.admin, http.StatusCreated, http.MethodPost, w.path("/events/subscriptions"),
			spec.EventSubscriptionRequest{Name: "All projects", DestinationId: everywhere.Id, EventTypes: []string{"project.created"}}))
		assert.Equal(t, models.EventScopeOrg, orgSub.Scope)
		assert.True(t, orgSub.Enabled, "a subscription is enabled unless it says otherwise")
		assert.Equal(t, "everywhere", orgSub.Destination.Name)
		assert.Equal(t, []string{}, orgSub.Environments)

		mustCall(t, w.admin, http.StatusCreated, http.MethodPost, w.path("/projects/shop/events/subscriptions"),
			spec.EventSubscriptionRequest{Name: "Shop agents", DestinationId: shopOnly.Id, EventTypes: []string{"*"}})

		disabled := false
		agentSub = decodeAs[spec.EventSubscriptionResponse](t, mustCall(t, w.admin, http.StatusCreated, http.MethodPost,
			w.path("/projects/shop/agents/cashier/events/subscriptions"),
			spec.EventSubscriptionRequest{Name: "Cashier changes", DestinationId: shopOnly.Id, EventTypes: []string{"agent.updated"}, Enabled: &disabled}))
		assert.Equal(t, "cashier", agentSub.AgentName)
		assert.False(t, agentSub.Enabled)

		list := decodeAs[spec.EventSubscriptionListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet,
			w.path("/projects/shop/agents/cashier/events/subscriptions"), nil))
		require.Len(t, list.Subscriptions, 1)
		assert.Equal(t, "Cashier changes", list.Subscriptions[0].Name)
		inherited := []string{}
		for _, s := range list.Inherited {
			inherited = append(inherited, s.Name)
		}
		// The project's subscription to every event reaches the agent; the
		// org's, to project.created only, cannot happen at an agent.
		assert.Equal(t, []string{"Shop agents"}, inherited)

		got := decodeAs[spec.EventDestinationResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodGet, w.path("/events/destinations/%s", shopOnly.Id), nil))
		assert.Len(t, got.Subscriptions, 2, "a destination lists the subscriptions using it")
		assert.Equal(t, int32(2), got.UsedBy)
	})

	t.Run("subscription refusals", func(t *testing.T) {
		for _, tc := range []struct {
			name, path string
			body       any
			status     int
		}{
			{"malformed", w.path("/events/subscriptions"), "{", http.StatusBadRequest},
			{"destination not an id", w.path("/events/subscriptions"), spec.EventSubscriptionRequest{Name: "x", DestinationId: "nope", EventTypes: []string{"*"}}, http.StatusBadRequest},
			{"no events", w.path("/events/subscriptions"), spec.EventSubscriptionRequest{Name: "x", DestinationId: everywhere.Id}, http.StatusBadRequest},
			{
				"an org event on an agent", w.path("/projects/shop/agents/cashier/events/subscriptions"),
				spec.EventSubscriptionRequest{Name: "x", DestinationId: everywhere.Id, EventTypes: []string{"project.created"}},
				http.StatusBadRequest,
			},
			{"unknown destination", w.path("/events/subscriptions"), spec.EventSubscriptionRequest{Name: "x", DestinationId: uuid.NewString(), EventTypes: []string{"*"}}, http.StatusBadRequest},
			{"unknown project", w.path("/projects/nowhere/events/subscriptions"), spec.EventSubscriptionRequest{Name: "x", DestinationId: everywhere.Id, EventTypes: []string{"*"}}, http.StatusNotFound},
			{"unknown agent", w.path("/projects/shop/agents/ghost/events/subscriptions"), spec.EventSubscriptionRequest{Name: "x", DestinationId: everywhere.Id, EventTypes: []string{"*"}}, http.StatusNotFound},
		} {
			t.Run(tc.name, func(t *testing.T) {
				status, body := call(t, w.admin, http.MethodPost, tc.path, tc.body)
				assert.Equal(t, tc.status, status, string(body))
			})
		}
		status, _ := call(t, w.admin, http.MethodPost, w.path("/projects/lab/events/subscriptions"),
			spec.EventSubscriptionRequest{Name: "x", DestinationId: shopOnly.Id, EventTypes: []string{"*"}})
		assert.NotEqual(t, http.StatusCreated, status, "a destination made available to shop is not available to lab")
		status, _ = call(t, w.reader, http.MethodPost, w.path("/events/subscriptions"),
			spec.EventSubscriptionRequest{Name: "x", DestinationId: everywhere.Id, EventTypes: []string{"*"}})
		assert.Equal(t, http.StatusForbidden, status)
	})

	t.Run("get, update and delete a subscription", func(t *testing.T) {
		agentSubs := w.path("/projects/shop/agents/cashier/events/subscriptions/%s", agentSub.Id)
		got := decodeAs[spec.EventSubscriptionResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, agentSubs, nil))
		assert.Equal(t, "Cashier changes", got.Name)
		mustCall(t, w.reader, http.StatusNotFound, http.MethodGet, w.path("/events/subscriptions/%s", agentSub.Id), nil)
		mustCall(t, w.reader, http.StatusNotFound, http.MethodGet, w.path("/events/subscriptions/not-an-id"), nil)

		updated := decodeAs[spec.EventSubscriptionResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodPut, agentSubs,
			spec.EventSubscriptionRequest{Name: "Cashier, all", Description: stringPtr("every change"), DestinationId: shopOnly.Id, EventTypes: []string{"*"}}))
		assert.Equal(t, "Cashier, all", updated.Name)
		assert.True(t, updated.Enabled)
		assert.Equal(t, "every change", updated.Description)
		status, _ := call(t, w.admin, http.MethodPut, agentSubs, "{")
		assert.Equal(t, http.StatusBadRequest, status)
		mustCall(t, w.admin, http.StatusNotFound, http.MethodPut, w.path("/events/subscriptions/%s", uuid.NewString()),
			spec.EventSubscriptionRequest{Name: "x", DestinationId: everywhere.Id, EventTypes: []string{"*"}})

		mustCall(t, w.admin, http.StatusNoContent, http.MethodDelete, agentSubs, nil)
		mustCall(t, w.reader, http.StatusNotFound, http.MethodGet, agentSubs, nil)
		mustCall(t, w.admin, http.StatusNotFound, http.MethodDelete, agentSubs, nil)
	})

	t.Run("a destination in use cannot be deleted", func(t *testing.T) {
		status, body := call(t, w.admin, http.MethodDelete, w.path("/events/destinations/%s", everywhere.Id), nil)
		assert.Equal(t, http.StatusConflict, status)
		assert.Contains(t, errorOf(t, body).Message, "used by")
		mustCall(t, w.admin, http.StatusOK, http.MethodGet, w.path("/events/destinations/%s", everywhere.Id), nil)
	})

	t.Run("send a test event", func(t *testing.T) {
		before := len(rcv.received())
		res := decodeAs[spec.EventDestinationTestResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodPost,
			w.path("/events/destinations/%s/test", everywhere.Id), nil))
		assert.True(t, res.Delivered)
		require.NotNil(t, res.StatusCode)
		assert.Equal(t, int32(http.StatusNoContent), *res.StatusCode)

		got := rcv.received()
		require.Len(t, got, before+1, "the receiver got exactly one request")
		req := got[len(got)-1]
		assert.Equal(t, "com.wso2.agentmanager.destination.test", req.header.Get(webhook.HeaderEventType))
		ts, err := strconv.ParseInt(req.header.Get(webhook.HeaderTimestamp), 10, 64)
		require.NoError(t, err)
		want, err := webhook.Sign(*everywhere.Secret, req.header.Get(webhook.HeaderID), time.Unix(ts, 0), req.body)
		require.NoError(t, err)
		assert.Equal(t, want, req.header.Get(webhook.HeaderSignature), "signed with the secret shown at creation")

		rcv.mu.Lock()
		rcv.status = http.StatusInternalServerError
		rcv.mu.Unlock()
		res = decodeAs[spec.EventDestinationTestResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodPost,
			w.path("/events/destinations/%s/test", everywhere.Id), nil))
		assert.False(t, res.Delivered, "a failing receiver is reported, not hidden")
		require.NotNil(t, res.Error)

		mustCall(t, w.admin, http.StatusNotFound, http.MethodPost, w.path("/events/destinations/%s/test", uuid.NewString()), nil)
	})

	t.Run("rotate the signing secret", func(t *testing.T) {
		rotated := decodeAs[spec.EventDestinationSecretResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodPost,
			w.path("/events/destinations/%s/rotate-secret", everywhere.Id), nil))
		assert.NotEmpty(t, rotated.Secret)
		assert.NotEqual(t, *everywhere.Secret, rotated.Secret)
		mustCall(t, w.admin, http.StatusNotFound, http.MethodPost, w.path("/events/destinations/%s/rotate-secret", uuid.NewString()), nil)
		mustCall(t, w.admin, http.StatusNotFound, http.MethodPost, w.path("/events/destinations/x/rotate-secret"), nil)
	})

	t.Run("update a destination", func(t *testing.T) {
		updated := decodeAs[spec.EventDestinationResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodPut,
			w.path("/events/destinations/%s", shopOnly.Id), webhookDestination("shop-and-lab", hook.URL+"/v2", "projects", "shop", "lab")))
		assert.Equal(t, "shop-and-lab", updated.Name)
		assert.ElementsMatch(t, []string{"shop", "lab"}, updated.Projects)

		status, _ := call(t, w.admin, http.MethodPut, w.path("/events/destinations/%s", shopOnly.Id),
			spec.EventDestinationRequest{Name: "x", Kind: models.DestinationKindNotify, Availability: "all", Config: map[string]interface{}{"url": hook.URL}})
		assert.Equal(t, http.StatusBadRequest, status, "a destination's kind cannot change")
		status, _ = call(t, w.admin, http.MethodPut, w.path("/events/destinations/%s", shopOnly.Id), webhookDestination("everywhere", hook.URL, "all"))
		assert.Equal(t, http.StatusConflict, status, "the new name is taken")
		mustCall(t, w.admin, http.StatusNotFound, http.MethodPut, w.path("/events/destinations/%s", uuid.NewString()), webhookDestination("y", hook.URL, "all"))
		status, _ = call(t, w.admin, http.MethodPut, w.path("/events/destinations/%s", shopOnly.Id), "{")
		assert.Equal(t, http.StatusBadRequest, status)
	})

	t.Run("the delivery log", func(t *testing.T) {
		code := http.StatusBadGateway
		delivery := models.EventDelivery{
			ID: uuid.Must(uuid.NewV7()), OUID: w.org, DestinationID: uuid.MustParse(everywhere.Id),
			SubscriptionIDs: []string{orgSub.Id}, EventID: "evt_" + uuid.NewString(), EventType: "project.created",
			EventScope: models.EventScopeOrg, Status: models.EventDeliveryFailed, Attempts: 3,
			ResponseCode: &code, LastError: "upstream said no",
		}
		require.NoError(t, repositories.NewEventRepository(db.GetDB(), nil).RecordDelivery(context.Background(), &delivery))

		log := decodeAs[spec.EventDeliveryListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/events/deliveries"), nil))
		require.Equal(t, int32(1), log.Total)
		d := log.Deliveries[0]
		assert.Equal(t, delivery.EventID, d.EventId)
		assert.Equal(t, models.EventDeliveryFailed, d.Status)
		assert.Equal(t, int32(3), d.Attempts)
		require.NotNil(t, d.ResponseCode)
		assert.Equal(t, int32(code), *d.ResponseCode)
		require.NotNil(t, d.Destination)
		assert.Equal(t, "everywhere", d.Destination.Name)
		require.Len(t, d.Subscriptions, 1)
		assert.Equal(t, "All projects", d.Subscriptions[0].Name)

		for q, want := range map[string]int32{
			"status=" + models.EventDeliveryFailed:    1,
			"status=" + models.EventDeliveryDelivered: 0,
			"destinationId=" + shopOnly.Id:            0,
			"subscriptionId=" + orgSub.Id:             1,
			"limit=1&offset=0":                        1,
		} {
			got := decodeAs[spec.EventDeliveryListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/events/deliveries?%s", q), nil))
			assert.Equal(t, want, got.Total, q)
		}
		for _, q := range []string{"limit=0", "limit=x", "offset=-1", "destinationId=nope", "subscriptionId=nope"} {
			status, _ := call(t, w.reader, http.MethodGet, w.path("/events/deliveries?%s", q), nil)
			assert.Equal(t, http.StatusBadRequest, status, q)
		}

		project := decodeAs[spec.EventDeliveryListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/projects/shop/events/deliveries"), nil))
		assert.Zero(t, project.Total, "an org event is not in a project's log")
		agent := decodeAs[spec.EventDeliveryListResponse](t, mustCall(t, w.reader, http.StatusOK, http.MethodGet, w.path("/projects/shop/agents/cashier/events/deliveries"), nil))
		assert.Zero(t, agent.Total)
		theirs := decodeAs[spec.EventDeliveryListResponse](t, mustCall(t, w.stranger, http.StatusOK, http.MethodGet, w.path("/events/deliveries"), nil))
		assert.Zero(t, theirs.Total)
	})

	t.Run("delete a destination once nothing uses it", func(t *testing.T) {
		subs := decodeAs[spec.EventSubscriptionListResponse](t, mustCall(t, w.admin, http.StatusOK, http.MethodGet, w.path("/events/subscriptions"), nil))
		for _, s := range subs.Subscriptions {
			mustCall(t, w.admin, http.StatusNoContent, http.MethodDelete, w.path("/events/subscriptions/%s", s.Id), nil)
		}
		mustCall(t, w.admin, http.StatusNoContent, http.MethodDelete, w.path("/events/destinations/%s", everywhere.Id), nil)
		mustCall(t, w.admin, http.StatusNotFound, http.MethodGet, w.path("/events/destinations/%s", everywhere.Id), nil)
		mustCall(t, w.admin, http.StatusNotFound, http.MethodDelete, w.path("/events/destinations/%s", everywhere.Id), nil)
		status, _ := call(t, w.reader, http.MethodDelete, w.path("/events/destinations/%s", shopOnly.Id), nil)
		assert.Equal(t, http.StatusForbidden, status)
	})
}
