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

// Package google is the Shadow Agent Discovery connector for Google Agent
// Platform (Gemini Enterprise Agent Platform): it lists Agent Runtime
// engines (reasoningEngines) and Agent Registry agents.
package google

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// Call names, as recorded in scan results and evidence sources. One name
// per call; the projects and locations it runs in are its scopes.
const (
	callToken   = "POST /token (service account key)"
	callConfig  = "connection settings"
	callEngines = "GET /v1/projects/{project}/locations/{location}/reasoningEngines"
	callAgents  = "GET /v1/projects/{project}/locations/{location}/agents"
)

// Bounds and documented limits (see the evidence log, evidence/google.md).
const (
	// enginesPageSize: no default or maximum is documented for
	// reasoningEngines.list; a project holds at most 100 Agent Platform
	// resources per region, so 100 lists a region in one page.
	enginesPageSize = 100
	// registryPageSize is Agent Registry's "Maximum pagination size 100".
	registryPageSize = 100

	maxEnginePages   = 20
	maxRegistryPages = 20
	// builtinAgentPrefix marks Google's own auto-registered agents
	// (Workspace, Gemini Enterprise); they are not shadow agents.
	builtinAgentPrefix = "urn:agent:googleapis.com:"
)

// Locations Agent Runtime serves from the multi-region host, and the
// global Agent Registry location.
var (
	multiRegions   = []string{"us", "eu"}
	globalLocation = "global"
)

var (
	// projectRE accepts project IDs (and domain-scoped IDs) and numbers;
	// anything else could change a request URL or a log filter.
	projectRE = regexp.MustCompile(`^[a-z0-9][a-z0-9.:-]{0,99}$`)
	// locationRE: a location is filled into a host name, so only DNS label
	// characters are accepted.
	locationRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

// Connector scans Google Agent Platform.
type Connector struct {
	httpClient *http.Client
	logger     *slog.Logger
	// sleep waits between retries.
	sleep func(ctx context.Context, d time.Duration) error
}

// New returns the Google Agent Platform connector. httpClient must be the
// redirect-guarded client from sa.NewHTTPClient.
func New(httpClient *http.Client, logger *slog.Logger) sa.Connector {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Connector{httpClient: httpClient, logger: logger, sleep: sleepContext}
}

// Provider implements sa.Connector.
func (c *Connector) Provider() string { return models.ShadowProviderGoogle }

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// newAPI builds the REST client: OAuth access tokens from the service
// account key, fetched from the connection's token endpoint.
func (c *Connector) newAPI(ctx context.Context, conn sa.Connection) (*sa.API, error) {
	ts, err := sa.GoogleTokenSource(ctx, c.httpClient, conn.Secret.ServiceAccountJSON, sa.EndpointOf(conn, sa.EPGoogleToken))
	if err != nil {
		return nil, &sa.CallError{Outcome: models.ShadowCallAuthFailed, Message: err.Error()}
	}
	return &sa.API{HTTP: c.httpClient, Authorize: sa.BearerAuthorizer(ts), Logger: c.logger, Sleep: c.sleep}, nil
}

// settings are a connection's validated projects and locations.
type settings struct {
	projects          []string
	runtimeLocations  []string
	registryLocations []string
}

// readSettings validates the projects and locations. Invalid values are
// left out and reported; they are never put into a URL or filter.
func readSettings(conn sa.Connection) (settings, []string) {
	var st settings
	var problems []string
	for _, p := range dedupe(conn.Config.ProjectIDs) {
		if !projectRE.MatchString(p) {
			problems = append(problems, fmt.Sprintf("project %q is not a valid project ID", sa.Truncate(p, 64)))
			continue
		}
		st.projects = append(st.projects, p)
	}
	var locs []string
	for _, l := range dedupe(conn.Config.Locations) {
		if !locationRE.MatchString(l) {
			problems = append(problems, fmt.Sprintf("location %q is not a valid location", sa.Truncate(l, 64)))
			continue
		}
		locs = append(locs, l)
	}
	for _, l := range locs {
		// Agent Runtime has no global location.
		if l != globalLocation {
			st.runtimeLocations = append(st.runtimeLocations, l)
		}
		st.registryLocations = append(st.registryLocations, l)
	}
	if conn.Config.IncludeGlobalAndMultiRegion {
		st.runtimeLocations = dedupe(append(st.runtimeLocations, multiRegions...))
		st.registryLocations = dedupe(append(st.registryLocations, append([]string{globalLocation}, multiRegions...)...))
	}
	if len(st.projects) == 0 {
		problems = append(problems, "no valid Google Cloud project ID is configured")
	}
	if len(st.runtimeLocations) == 0 {
		problems = append(problems, "no Agent Runtime location is configured")
	}
	return st, problems
}

// dedupe trims, lower-cases and de-duplicates values, keeping their order.
func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func isMultiRegion(location string) bool {
	for _, m := range multiRegions {
		if location == m {
			return true
		}
	}
	return false
}

// runtimeBase returns the Agent Runtime base URL for a location: the
// multi-region host for us/eu, the regional host otherwise.
func runtimeBase(conn sa.Connection, location string) (string, error) {
	key := sa.EPGoogleAIPlatform
	if isMultiRegion(location) {
		key = sa.EPGoogleAIPlatformRep
	}
	return sa.ValueEndpoint(conn, key, "location", location)
}

func baseURL(conn sa.Connection, key string) (string, error) {
	return sa.Expand(sa.EndpointOf(conn, key), nil)
}

func parseTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil
	}
	t = t.UTC()
	return &t
}
