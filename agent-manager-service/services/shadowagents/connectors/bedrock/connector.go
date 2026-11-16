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

// Package bedrock is the Amazon Bedrock / AgentCore connector of Shadow
// Agent Discovery. Per region it lists AgentCore runtimes, AWS Agent
// Registry agent records and classic Bedrock agents. It uses the vendored
// aws-sdk-go-v2 clients; see evidence/impl_notes_bedrock.md for the wire
// protocol each one speaks.
package bedrock

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// Call names, as recorded in scan results and evidence sources.
const (
	callListRuntimes   = "AgentCore ListAgentRuntimes"
	callGetRuntime     = "AgentCore GetAgentRuntime"
	callListRegistries = "Agent Registry ListRegistries"
	callListRecords    = "Agent Registry ListRegistryRecords"
	callListAgents     = "Bedrock Agents ListAgents"
	callGetAgent       = "Bedrock Agents GetAgent"
	callListActions    = "Bedrock Agents ListAgentActionGroups"
	callConfigure      = "AWS client configuration"
)

// Bounds.
const (
	// maxPages bounds every listing (per region and parent resource).
	maxPages = 50
	// pageSize is the maxResults sent to the list APIs (all accept 100).
	pageSize = 100
)

// Connector scans Amazon Bedrock and AgentCore in the connection's regions.
type Connector struct {
	http   *http.Client
	logger *slog.Logger
	// retryer, when set, replaces the SDK's standard retryer.
	retryer func() aws.Retryer
}

// New returns the Bedrock connector. httpClient is the redirect-guarded
// client every SDK client uses.
func New(httpClient *http.Client, logger *slog.Logger) sa.Connector {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Connector{http: httpClient, logger: logger, retryer: nil}
}

// Provider implements sa.Connector.
func (c *Connector) Provider() string { return models.ShadowProviderBedrock }

// Scan implements sa.Connector.
func (c *Connector) Scan(ctx context.Context, conn sa.Connection) *sa.Result {
	s := newScan(c, conn)
	s.run(ctx)
	res := &sa.Result{Findings: s.fs.All(), Calls: s.calls.Results()}
	c.logger.Info("Shadow agent discovery Bedrock scan finished",
		"connectionId", conn.ID.String(), "regions", len(conn.Config.Regions),
		"agents", len(res.Findings), "calls", len(res.Calls))
	return res
}

// scan is the state of one Scan.
type scan struct {
	c     *Connector
	conn  sa.Connection
	calls *sa.Calls
	fs    *sa.Findings
	log   *slog.Logger

	regions []*regionClients

	// runtimeByARN and runtimeByID (region + "|" + id) find a runtime
	// finding from a registry record's provenance.
	runtimeByARN map[string]*sa.Finding
	runtimeByID  map[string]*sa.Finding
	// mcpRuntimes are the runtimes (by ARN) that host an MCP server: they are
	// not agents, so they are not findings, and a registry record about one
	// is not one either.
	mcpRuntimes map[string]bool
}

func newScan(c *Connector, conn sa.Connection) *scan {
	return &scan{
		c: c, conn: conn,
		calls: sa.NewCalls(), fs: sa.NewFindings(), log: c.logger,
		runtimeByARN: map[string]*sa.Finding{}, runtimeByID: map[string]*sa.Finding{}, mcpRuntimes: map[string]bool{},
	}
}

func (s *scan) run(ctx context.Context) {
	regions := cleanRegions(s.conn.Config.Regions)
	if len(regions) == 0 {
		s.calls.Record(callConfigure, 0, &sa.CallError{Outcome: models.ShadowCallFailed, Message: "no regions are configured"})
		return
	}
	for _, region := range regions {
		rc, err := newRegionClients(s.conn, s.c.http, region, s.c.retryer)
		if err != nil {
			s.calls.Record(callConfigure, 0, &sa.CallError{Outcome: models.ShadowCallFailed, Message: sa.ScrubText(region + ": " + err.Error())})
			continue
		}
		s.regions = append(s.regions, rc)
	}
	if len(s.regions) == 0 {
		return
	}

	on := func(opt string) bool { return sa.ServiceEnabled(s.conn.Config, opt) }
	s.skipUnselected()

	// Runtimes and classic agents first, in every region, so registry
	// records can be merged into the runtimes they describe.
	for i, rc := range s.regions {
		if ctx.Err() != nil {
			break
		}
		if on(sa.OptBedrockAgentCore) {
			err := s.scanRuntimes(ctx, rc)
			if i == 0 && isAuthFailure(err) {
				// Authentication failed on the very first call: every other
				// call would fail the same way.
				s.log.Warn("Shadow agent discovery Bedrock authentication failed; stopping the scan",
					"connectionId", s.conn.ID.String(), "region", rc.region)
				return
			}
		}
		if on(sa.OptBedrockClassicAgents) {
			s.scanClassicAgents(ctx, rc)
		}
	}
	if on(sa.OptBedrockRegistry) {
		for _, rc := range s.regions {
			if ctx.Err() != nil {
				break
			}
			s.scanRegistry(ctx, rc)
		}
	}
}

// serviceCalls are the calls of each optional service.
var serviceCalls = []struct {
	option string
	calls  []string
}{
	{sa.OptBedrockAgentCore, []string{callListRuntimes, callGetRuntime}},
	{sa.OptBedrockClassicAgents, []string{callListAgents, callGetAgent, callListActions}},
	{sa.OptBedrockRegistry, []string{callListRegistries, callListRecords}},
}

// skipUnselected records the calls of the services the user unticked.
func (s *scan) skipUnselected() {
	for _, sc := range serviceCalls {
		if sa.ServiceEnabled(s.conn.Config, sc.option) {
			continue
		}
		for _, call := range sc.calls {
			s.calls.Skip(call, sa.ReasonServiceOff)
		}
	}
}

func cleanRegions(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, r := range in {
		r = strings.TrimSpace(r)
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

// paginate drives an SDK paginator: next fetches one page and returns its
// item count. It stops at the first error or after limit pages.
func paginate(ctx context.Context, limit int, hasMore func() bool, next func(context.Context) (int, error)) (int, error) {
	items, pages := 0, 0
	for hasMore() {
		if pages == limit {
			return items, truncatedError(limit)
		}
		if err := ctx.Err(); err != nil {
			return items, awsError(err)
		}
		n, err := next(ctx)
		pages++
		if err != nil {
			return items, awsError(err)
		}
		items += n
	}
	return items, nil
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func timePtr(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}
