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

// Package openai is the Shadow Agent Discovery connector for the OpenAI API
// platform. Each project key lists its project's agents (GET /v1/agents,
// beta). No other endpoint is called.
package openai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// Call names, as stored in scan results and evidence sources.
const (
	callAgents         = "GET /v1/agents"
	suffixPageLimit    = " (page limit)"
	reasonNoProjectKey = "no project key"
	headerBeta         = "OpenAI-Beta"
	headerBetaAgents   = "agents=v1"
	stateActive        = "active"
)

// Bounds.
const (
	// listPageSize is the largest page the list endpoints accept (1-100).
	listPageSize = 100
	// maxListPages bounds one cursor listing (10,000 items).
	maxListPages = 100
)

// Connector scans one OpenAI organization.
type Connector struct {
	httpClient *http.Client
	logger     *slog.Logger
}

// New returns the OpenAI connector. httpClient is the redirect-guarded
// client (sa.NewHTTPClient); logger receives sanitized summaries only.
func New(httpClient *http.Client, logger *slog.Logger) sa.Connector {
	if httpClient == nil {
		httpClient = sa.NewHTTPClient(nil)
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Connector{httpClient: httpClient, logger: logger}
}

// Provider returns models.ShadowProviderOpenAI.
func (c *Connector) Provider() string { return models.ShadowProviderOpenAI }

// api returns a client that sends key as a bearer token.
func (c *Connector) api(key string) *sa.API {
	return &sa.API{
		HTTP:   c.httpClient,
		Logger: c.logger,
		Header: func(h http.Header) { h.Set("Authorization", "Bearer "+key) },
	}
}

// baseURL is the connection's OpenAI API base, e.g. https://api.openai.com/v1.
func baseURL(conn sa.Connection) (string, error) {
	base, err := sa.Expand(sa.EndpointOf(conn, sa.EPOpenAIAPI), nil)
	if err != nil {
		return "", err
	}
	if base == "" {
		return "", errors.New("the OpenAI API endpoint is not set")
	}
	return base, nil
}

func betaAgentsHeader() http.Header {
	return http.Header{headerBeta: []string{headerBetaAgents}}
}

// listAll follows an after/has_more listing, calling fn for every item
// until fn returns false. It returns true when maxPages was reached with
// more pages left.
func listAll[T any](ctx context.Context, api *sa.API, u string, q url.Values, hdr http.Header,
	pageSize, maxPages int, fn func(T) bool,
) (bool, error) {
	after := ""
	for page := 0; page < maxPages; page++ {
		pq := cloneValues(q)
		pq.Set("limit", strconv.Itoa(pageSize))
		if after != "" {
			pq.Set("after", after)
		}
		var p listPage[T]
		if err := api.DoJSON(ctx, sa.Request{Method: http.MethodGet, URL: u, Query: pq, Header: hdr}, &p); err != nil {
			return false, err
		}
		if p.Data == nil {
			return false, sa.Unsupported("the response has no data list")
		}
		for _, it := range *p.Data {
			if !fn(it) {
				return false, nil
			}
		}
		if !p.HasMore {
			return false, nil
		}
		next := ""
		if p.LastID != nil {
			next = *p.LastID
		}
		if next == "" || next == after {
			return false, sa.Unsupported("has_more is true but last_id does not advance")
		}
		after = next
	}
	return true, nil
}

func cloneValues(q url.Values) url.Values {
	out := url.Values{}
	for k, v := range q {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// statusOf returns the HTTP status of a failed call, or 0.
func statusOf(err error) int {
	var ce *sa.CallError
	if errors.As(err, &ce) {
		return ce.Status
	}
	return 0
}

func unixTime(sec int64) *time.Time {
	if sec <= 0 {
		return nil
	}
	t := time.Unix(sec, 0).UTC()
	return &t
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
