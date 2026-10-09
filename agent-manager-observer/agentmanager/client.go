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

// Package agentmanager is the observer's client for agent-manager-service.
package agentmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/middleware"
)

const (
	// scoreLookupTimeout bounds one score lookup, well under the 10 s export selection budget.
	scoreLookupTimeout = 3 * time.Second
	// maxIdleConnsPerHost keeps a connection open for each trace request running a score lookup.
	maxIdleConnsPerHost = 16
	// maxScoreResponseBytes caps the score response read; 50 traces take about 4 KB.
	maxScoreResponseBytes = 1 << 20
)

// ErrForbidden means the service rejected the caller's token (401 or 403).
var ErrForbidden = errors.New("agent-manager-service rejected the caller's token")

// Client reads evaluation scores from agent-manager-service with the caller's token.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient returns a client for the service at baseURL that keeps its connections open.
func NewClient(baseURL string) *Client {
	return newClient(baseURL, scoreLookupTimeout)
}

// newClient is NewClient with a given per-call timeout.
func newClient(baseURL string, timeout time.Duration) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = maxIdleConnsPerHost
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Transport: transport, Timeout: timeout},
	}
}

// traceScoresResponse is the part of getAgentTraceScores' response the observer reads.
type traceScoresResponse struct {
	Traces []struct {
		TraceID string   `json:"traceId"`
		Score   *float64 `json:"score"`
	} `json:"traces"`
}

// TraceScores returns the mean non-skipped score of each listed trace scored
// between start and end, only evaluator's rows when it is set. A trace with no
// scores is absent, and one with only skipped scores is nil.
func (c *Client) TraceScores(
	ctx context.Context,
	org, project, agent string,
	start, end time.Time,
	traceIDs []string,
	evaluator string,
) (map[string]*float64, error) {
	token := middleware.BearerTokenFromContext(ctx)
	if token == "" {
		return nil, fmt.Errorf("agentmanager.TraceScores: no caller token: %w", ErrForbidden)
	}
	query := url.Values{}
	query.Set("startTime", start.UTC().Format(time.RFC3339Nano))
	query.Set("endTime", end.UTC().Format(time.RFC3339Nano))
	query.Set("traceIds", strings.Join(traceIDs, ","))
	if evaluator != "" {
		query.Set("evaluator", evaluator)
	}
	endpoint := fmt.Sprintf("%s/api/v1/orgs/%s/projects/%s/agents/%s/scores?%s", c.baseURL,
		url.PathEscape(org), url.PathEscape(project), url.PathEscape(agent), query.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("agentmanager.TraceScores: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("agentmanager.TraceScores: %w", err)
	}
	defer func() {
		// Drain so the connection is reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxScoreResponseBytes))
		_ = resp.Body.Close()
	}()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("agentmanager.TraceScores: status %d: %w", resp.StatusCode, ErrForbidden)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("agentmanager.TraceScores: status %d", resp.StatusCode)
	}

	var body traceScoresResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxScoreResponseBytes)).Decode(&body); err != nil {
		return nil, fmt.Errorf("agentmanager.TraceScores: decode response: %w", err)
	}
	scores := make(map[string]*float64, len(body.Traces))
	for _, t := range body.Traces {
		scores[t.TraceID] = t.Score
	}
	return scores, nil
}
