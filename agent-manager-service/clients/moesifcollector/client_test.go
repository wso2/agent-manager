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

package moesifcollector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/requests"
)

// sampleEvent is a fully populated event, so the wire-shape assertions below
// exercise every field rather than a minimal subset.
func sampleEvent() Event {
	return Event{
		Request: EventRequest{
			Time:      time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
			URI:       "https://api.example.com/api/v1/orgs/acme/projects/p1/agents",
			Verb:      http.MethodPost,
			IPAddress: "203.0.113.7",
		},
		Response: EventResponse{
			Time:   time.Date(2026, 9, 3, 10, 0, 1, 0, time.UTC).Format(time.RFC3339),
			Status: http.StatusCreated,
		},
		UserID:    "user-123",
		CompanyID: "org-456",
		Metadata: map[string]interface{}{
			"growth_action":    "amp.agent-development.create-agent",
			"environment":      "development",
			"deployment_model": "saas",
		},
	}
}

// captured records what the fake collector received, so each test asserts on
// the real bytes and headers that went over the wire.
type captured struct {
	path       string
	host       string
	authHeader string
	body       map[string]interface{}
}

// newFakeCollector starts a stub collector that records one request and
// replies with status. It returns the server and a pointer the test reads
// after SendEvent returns.
func newFakeCollector(t *testing.T, status int) (*httptest.Server, *captured) {
	t.Helper()
	got := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.host = r.Host
		got.authHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got.body); err != nil {
			t.Errorf("collector: decoding request body: %v", err)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func newTestClient(baseURL, token, hostHeader string) *Client {
	return NewClient(requests.NewRetryableHTTPClient(&http.Client{Timeout: 5 * time.Second}),
		baseURL, token, hostHeader)
}

// TestSendEvent_PostsToEventsPathWithBearerToken pins the two things the
// proxy routes and authenticates on: the /v1/events path and the caller's
// bearer JWT.
func TestSendEvent_PostsToEventsPathWithBearerToken(t *testing.T) {
	srv, got := newFakeCollector(t, http.StatusOK)

	c := newTestClient(srv.URL, "caller-jwt", "")
	if err := c.SendEvent(context.Background(), sampleEvent()); err != nil {
		t.Fatalf("SendEvent() error = %v, want nil", err)
	}

	if got.path != "/v1/events" {
		t.Errorf("path = %q, want %q", got.path, "/v1/events")
	}
	if got.authHeader != "Bearer caller-jwt" {
		t.Errorf("Authorization = %q, want %q", got.authHeader, "Bearer caller-jwt")
	}
}

// TestSendEvent_MetadataStaysTopLevel is the regression this package's doc
// comment warns about: nesting metadata inside "request" still returns 200
// from Moesif but silently leaves the Metadata panel empty, so every
// dimension the taxonomy reports would be lost with no error anywhere.
func TestSendEvent_MetadataStaysTopLevel(t *testing.T) {
	srv, got := newFakeCollector(t, http.StatusOK)

	c := newTestClient(srv.URL, "caller-jwt", "")
	if err := c.SendEvent(context.Background(), sampleEvent()); err != nil {
		t.Fatalf("SendEvent() error = %v, want nil", err)
	}

	meta, ok := got.body["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("body[metadata] = %#v, want a top-level object (sibling of request/response)", got.body["metadata"])
	}
	if meta["growth_action"] != "amp.agent-development.create-agent" {
		t.Errorf("metadata[growth_action] = %v, want the feature code", meta["growth_action"])
	}
	if meta["environment"] != "development" {
		t.Errorf("metadata[environment] = %v, want %q", meta["environment"], "development")
	}

	req, ok := got.body["request"].(map[string]interface{})
	if !ok {
		t.Fatalf("body[request] = %#v, want an object", got.body["request"])
	}
	if _, nested := req["metadata"]; nested {
		t.Error("metadata must not be nested inside request — Moesif accepts it with 200 but drops it")
	}

	// Bodies and headers are deliberately never forwarded: several tracked
	// routes return generated secrets in their response body.
	if _, present := req["body"]; present {
		t.Error("request.body must never be sent — tracked routes return generated secrets")
	}
	if _, present := req["headers"]; present {
		t.Error("request.headers must never be sent — they carry the caller's bearer token")
	}
}

// TestSendEvent_IdentityFieldsAreTopLevel keeps user_id/company_id where
// Moesif attributes them from, alongside metadata rather than inside it.
func TestSendEvent_IdentityFieldsAreTopLevel(t *testing.T) {
	srv, got := newFakeCollector(t, http.StatusOK)

	c := newTestClient(srv.URL, "caller-jwt", "")
	if err := c.SendEvent(context.Background(), sampleEvent()); err != nil {
		t.Fatalf("SendEvent() error = %v, want nil", err)
	}

	if got.body["user_id"] != "user-123" {
		t.Errorf("body[user_id] = %v, want %q", got.body["user_id"], "user-123")
	}
	if got.body["company_id"] != "org-456" {
		t.Errorf("body[company_id] = %v, want %q", got.body["company_id"], "org-456")
	}
}

// TestSendEvent_TrimsTrailingSlashOnBaseURL guards the URL join: a
// configured base URL ending in "/" must not produce "//v1/events", which
// the gateway routes differently.
func TestSendEvent_TrimsTrailingSlashOnBaseURL(t *testing.T) {
	srv, got := newFakeCollector(t, http.StatusOK)

	c := newTestClient(srv.URL+"/", "caller-jwt", "")
	if err := c.SendEvent(context.Background(), sampleEvent()); err != nil {
		t.Fatalf("SendEvent() error = %v, want nil", err)
	}

	if got.path != "/v1/events" {
		t.Errorf("path = %q, want %q (trailing slash on baseURL must be trimmed)", got.path, "/v1/events")
	}
}

// TestSendEvent_HostHeader covers both halves of the local-dev port-forward
// case: an explicit hostHeader overrides the outgoing Host, and an empty one
// leaves the URL's own host in place.
func TestSendEvent_HostHeader(t *testing.T) {
	t.Run("overrides Host when set", func(t *testing.T) {
		srv, got := newFakeCollector(t, http.StatusOK)

		const vhost = "moesif-collector.example.internal"
		c := newTestClient(srv.URL, "caller-jwt", vhost)
		if err := c.SendEvent(context.Background(), sampleEvent()); err != nil {
			t.Fatalf("SendEvent() error = %v, want nil", err)
		}

		if got.host != vhost {
			t.Errorf("Host = %q, want %q — the gateway routes on Host alone", got.host, vhost)
		}
	})

	t.Run("leaves Host alone when empty", func(t *testing.T) {
		srv, got := newFakeCollector(t, http.StatusOK)

		c := newTestClient(srv.URL, "caller-jwt", "")
		if err := c.SendEvent(context.Background(), sampleEvent()); err != nil {
			t.Fatalf("SendEvent() error = %v, want nil", err)
		}

		if got.host == "" {
			t.Error("Host = empty, want the URL's own host")
		}
	})
}

// TestSendEvent_NonSuccessStatusIsAnError makes sure a rejected event
// surfaces to the caller's error log instead of passing as delivered. 401 is
// the realistic case: the proxy refusing the forwarded caller JWT.
func TestSendEvent_NonSuccessStatusIsAnError(t *testing.T) {
	for _, status := range []int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusBadRequest,
		http.StatusInternalServerError,
	} {
		srv, _ := newFakeCollector(t, status)

		c := newTestClient(srv.URL, "caller-jwt", "")
		err := c.SendEvent(context.Background(), sampleEvent())
		if err == nil {
			t.Errorf("SendEvent() with collector status %d: error = nil, want non-nil", status)
		}
	}
}

// TestSendEvent_UnreachableCollectorIsAnError covers the transport failure
// path, where no response is produced at all.
//
// This one deliberately skips the retryable client: retrying a refused
// connection burns the default backoff (~5s) for no extra coverage, and what
// is under test is that Client turns a transport failure into an error, not
// the retry policy.
func TestSendEvent_UnreachableCollectorIsAnError(t *testing.T) {
	srv, _ := newFakeCollector(t, http.StatusOK)
	baseURL := srv.URL
	srv.Close() // nothing is listening now

	c := NewClient(&http.Client{Timeout: 5 * time.Second}, baseURL, "caller-jwt", "")
	if err := c.SendEvent(context.Background(), sampleEvent()); err == nil {
		t.Error("SendEvent() to an unreachable collector: error = nil, want non-nil")
	}
}
