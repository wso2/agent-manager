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

// Package anthropic is the Shadow Agent Discovery connector for the Claude
// Developer Platform. Each workspace key lists its workspace's Managed
// Agents (GET /v1/agents, managed-agents beta). No other endpoint is
// called. See evidence/anthropic.md.
package anthropic

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// Headers and pinned versions (anthropic.md §2.0, §2.1).
const (
	headerVersion   = "anthropic-version"
	headerBeta      = "anthropic-beta"
	headerWorkspace = "anthropic-workspace-id"

	apiVersion        = "2026-10-21"
	betaManagedAgents = "managed-agents-2026-04-01"
)

// Call names, as stored in scan results and evidence sources.
const (
	callAgents = "GET /v1/agents"
)

// Bounds.
const (
	// maxAgentPages bounds the agents list (100 a page).
	maxAgentPages = 50
)

// Connector scans Anthropic.
type Connector struct {
	http   *http.Client
	logger *slog.Logger
	// sleep waits between retried requests (tests replace it).
	sleep func(ctx context.Context, d time.Duration) error
}

// New returns the Anthropic connector. httpClient is the redirect-guarded
// client of sa.NewHTTPClient.
func New(httpClient *http.Client, logger *slog.Logger) sa.Connector {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Connector{http: httpClient, logger: logger, sleep: sleepCtx}
}

// Provider implements sa.Connector.
func (c *Connector) Provider() string { return models.ShadowProviderAnthropic }

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// scan is the state of one connection's scan.
type scan struct {
	c        *Connector
	conn     sa.Connection
	calls    *sa.Calls
	findings *sa.Findings
}

// Scan implements sa.Connector.
func (c *Connector) Scan(ctx context.Context, conn sa.Connection) *sa.Result {
	s := c.newScan(conn)
	s.firstParty(ctx)
	res := s.result()
	c.logger.Info("Anthropic shadow agent scan finished",
		"connectionId", conn.ID.String(), "agents", len(res.Findings), "calls", len(res.Calls))
	return res
}

func (c *Connector) newScan(conn sa.Connection) *scan {
	return &scan{c: c, conn: conn, calls: sa.NewCalls(), findings: sa.NewFindings()}
}

func (s *scan) newAPI(header func(http.Header), authorize func(context.Context, *http.Request, []byte) error) *sa.API {
	return &sa.API{HTTP: s.c.http, Header: header, Authorize: authorize, Logger: s.c.logger, Sleep: s.c.sleep}
}

// firstPartyPlatform builds the first-party platform: each workspace's key
// as a bearer token for its workspace routes (anthropic.md §4 C).
func (s *scan) firstPartyPlatform() (*platform, error) {
	base, err := sa.Expand(sa.EndpointOf(s.conn, sa.EPAnthropicAPI), nil)
	if err != nil {
		return nil, err
	}
	p := &platform{base: base}
	p.wsAPI = func(ws string) (*sa.API, bool) {
		key := strings.TrimSpace(s.conn.Secret.ScopedKeys[ws])
		if key == "" {
			return nil, false
		}
		return s.newAPI(func(h http.Header) {
			h.Set("Authorization", "Bearer "+key)
			h.Set(headerVersion, apiVersion)
		}, nil), true
	}
	return p, nil
}

// firstParty lists the Managed Agents of every workspace that has a key.
func (s *scan) firstParty(ctx context.Context) {
	p, err := s.firstPartyPlatform()
	if err != nil {
		s.calls.Record(callAgents, 0, err)
		return
	}
	s.workspaces(ctx, p, sortedKeys(s.conn.Secret.ScopedKeys), "no workspace keys")
}

// result builds the scan result.
func (s *scan) result() *sa.Result {
	all := s.findings.All()
	for _, f := range all {
		f.Attributes = sa.SanitizeAttributes(f.Attributes)
		f.DisplayName = sa.SanitizeName(f.DisplayName)
	}
	return &sa.Result{Findings: all, Calls: s.calls.Results()}
}

// agentFinding returns the finding of a Managed Agent.
func (s *scan) agentFinding(id string) *sa.Finding {
	return s.findings.Get(models.ShadowKindPlatformAgent, id)
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if strings.TrimSpace(k) != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func parseTime(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}
