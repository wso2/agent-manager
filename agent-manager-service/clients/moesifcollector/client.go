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

// Package moesifcollector is a thin client for the moesif-collector-api
// OpenChoreo proxy component: an authenticated reverse proxy in front of
// Moesif's ingestion API (https://api.moesif.net). The proxy validates a
// bearer JWT issued by platform-idp and injects the real Moesif Application
// ID server-side, so callers never handle that credential — only the JWT.
package moesifcollector

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/clients/requests"
)

// Event is a single API-call event, in the shape the moesif-collector-api's
// POST /v1/events operation expects. Metadata must stay top-level (a sibling
// of Request/Response) — nesting it inside Request silently fails to
// populate Moesif's Metadata panel.
type Event struct {
	Request   EventRequest           `json:"request"`
	Response  EventResponse          `json:"response"`
	UserID    string                 `json:"user_id,omitempty"`
	CompanyID string                 `json:"company_id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// EventRequest is the request side of an Event. Body/headers are
// deliberately not included: several routes this is used from return
// generated secrets (API keys, tokens, identity secrets) in the response
// body, and the taxonomy this reports doesn't need either.
type EventRequest struct {
	Time      string `json:"time"`
	URI       string `json:"uri"`
	Verb      string `json:"verb"`
	IPAddress string `json:"ip_address,omitempty"`
}

// EventResponse is the response side of an Event.
type EventResponse struct {
	Time   string `json:"time"`
	Status int    `json:"status"`
}

// Action is a single user action, in the shape the moesif-collector-api's
// POST /v1/actions operation expects. Actions are a distinct object type in
// Moesif from Events: events model API calls, actions model what a person did
// in a UI. Console telemetry is reported as actions so it never lands in — or
// skews the metrics computed over — the API-call event stream, while still
// joining to it on user_id/company_id.
type Action struct {
	ActionName   string                 `json:"action_name"`
	Request      ActionRequest          `json:"request"`
	UserID       string                 `json:"user_id,omitempty"`
	CompanyID    string                 `json:"company_id,omitempty"`
	SessionToken string                 `json:"session_token,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ActionRequest is the request context of an Action: where and when the user
// was when they did it. Moesif requires time and uri on every action.
type ActionRequest struct {
	Time            string `json:"time"`
	URI             string `json:"uri"`
	IPAddress       string `json:"ip_address,omitempty"`
	UserAgentString string `json:"user_agent_string,omitempty"`
}

// Client posts events to a moesif-collector-api instance.
type Client struct {
	httpClient requests.HttpClient
	baseURL    string
	token      string
	hostHeader string
}

// NewClient builds a Client.
//
//   - baseURL is the proxy's base URL, e.g.
//     "http://<collector-host>:<port>/<collector-path>"
//     when reached directly inside the OpenChoreo data plane, or
//     "http://localhost:18080/moesif-collector" for local dev through a
//     `kubectl port-forward` of the internal gateway.
//   - token is a bearer JWT issued by platform-idp. It's short-lived (~1h);
//     the caller is responsible for keeping it fresh — this client does not
//     retry on expiry.
//   - hostHeader, if non-empty, overrides the outgoing Host header. Required
//     for local dev through the port-forward above, since the gateway
//     routes purely on Host and localhost doesn't match the real vhost
//     name. Leave empty when baseURL's own host is already the real vhost
//     (i.e. reached directly inside the data plane).
func NewClient(httpClient requests.HttpClient, baseURL, token, hostHeader string) *Client {
	return &Client{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		hostHeader: hostHeader,
	}
}

// SendEvent posts a single event to POST /v1/events.
func (c *Client) SendEvent(ctx context.Context, evt Event) error {
	req := &requests.HttpRequest{
		Name:   "moesifcollector.SendEvent",
		URL:    c.baseURL + "/v1/events",
		Method: http.MethodPost,
	}
	req.SetJson(evt)
	req.SetHeader("Authorization", "Bearer "+c.token)
	if c.hostHeader != "" {
		req.SetHost(c.hostHeader)
	}

	result := requests.SendRequest(ctx, c.httpClient, req)
	if err := result.Err(); err != nil {
		return fmt.Errorf("moesifcollector: send event: %w", err)
	}
	if status := result.StatusCode(); status < 200 || status >= 300 {
		return fmt.Errorf("moesifcollector: send event: unexpected status %d", status)
	}
	return nil
}

// SendActions posts a batch of user actions to POST /v1/actions/batch.
//
// Always the batch endpoint, even for one action: console telemetry arrives
// pre-batched from the browser, and using a single path keeps the proxy's
// allowed-operation list and this client's error handling from having to care
// how many actions a flush happened to carry. An empty slice is a no-op rather
// than an empty POST.
func (c *Client) SendActions(ctx context.Context, actions []Action) error {
	if len(actions) == 0 {
		return nil
	}

	req := &requests.HttpRequest{
		Name:   "moesifcollector.SendActions",
		URL:    c.baseURL + "/v1/actions/batch",
		Method: http.MethodPost,
	}
	req.SetJson(actions)
	req.SetHeader("Authorization", "Bearer "+c.token)
	if c.hostHeader != "" {
		req.SetHost(c.hostHeader)
	}

	result := requests.SendRequest(ctx, c.httpClient, req)
	if err := result.Err(); err != nil {
		return fmt.Errorf("moesifcollector: send actions: %w", err)
	}
	if status := result.StatusCode(); status < 200 || status >= 300 {
		return fmt.Errorf("moesifcollector: send actions: unexpected status %d", status)
	}
	return nil
}
