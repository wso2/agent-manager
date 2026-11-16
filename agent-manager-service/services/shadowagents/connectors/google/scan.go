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

package google

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// scanner is the state of one scan of one connection.
type scanner struct {
	c        *Connector
	conn     sa.Connection
	api      *sa.API
	calls    *sa.Calls
	findings *sa.Findings

	// engines are the Agent Runtime engines found, in order.
	engines []*engine
	// aliases maps a configured project ID to the other names (project
	// numbers) its resources were seen under.
	aliases map[string]map[string]bool

	authFailed bool
	recorded   map[string]bool
	// deferredSkips are calls skipped in some scope; they are recorded as
	// skipped only if no scope ran them.
	deferredSkips map[string]string
	skipOrder     []string
}

// engine is one Agent Runtime engine and its finding.
type engine struct {
	finding *sa.Finding
	// project is the configured project the engine was listed in.
	project string
	// nameProject is the project segment of the engine's resource name (a
	// project ID or number).
	nameProject string
	location    string
	id          string
}

// Scan implements sa.Connector.
func (c *Connector) Scan(ctx context.Context, conn sa.Connection) *sa.Result {
	s := &scanner{
		c: c, conn: conn, calls: sa.NewCalls(), findings: sa.NewFindings(),
		aliases: map[string]map[string]bool{}, recorded: map[string]bool{}, deferredSkips: map[string]string{},
	}
	st, problems := readSettings(conn)
	if len(problems) > 0 {
		s.calls.Record(callConfig, 0, &sa.CallError{Outcome: models.ShadowCallFailed, Message: sa.ScrubText(strings.Join(problems, "; "))})
	}
	if len(st.projects) == 0 {
		return s.result()
	}
	api, err := c.newAPI(ctx, conn)
	if err != nil {
		s.record(callToken, 0, err)
		return s.result()
	}
	s.api = api

	// Engines first: registry agents are merged into the engines they run on.
	for _, p := range st.projects {
		for _, l := range st.runtimeLocations {
			s.listEngines(ctx, p, l)
		}
	}
	if sa.ServiceEnabled(conn.Config, sa.OptGoogleRegistry) {
		for _, p := range st.projects {
			for _, l := range st.registryLocations {
				s.listAgents(ctx, p, l)
			}
		}
	} else {
		s.calls.Skip(callAgents, sa.ReasonServiceOff)
	}
	return s.result()
}

func (s *scanner) result() *sa.Result {
	for _, call := range s.skipOrder {
		if !s.recorded[call] {
			s.calls.Skip(call, s.deferredSkips[call])
		}
	}
	all := s.findings.All()
	c := map[string]int{}
	for _, f := range all {
		c[f.Kind]++
	}
	s.c.logger.Info("Shadow agent discovery Google scan finished",
		"connectionId", s.conn.ID.String(), "agents", len(all),
		"engines", c[models.ShadowKindRuntime], "registryAgents", c[models.ShadowKindPlatformAgent],
		"authFailed", s.authFailed)
	return &sa.Result{Findings: all, Calls: s.calls.Results()}
}

// record records one scope of a call. An authentication failure stops the
// rest of the scan: the token is shared by every call.
func (s *scanner) record(call string, items int, err error) {
	s.calls.Record(call, items, err)
	s.recorded[call] = true
	if sa.OutcomeOf(err) == models.ShadowCallAuthFailed {
		s.authFailed = true
	}
}

// proceed reports whether call may run; when the scan stopped it records
// the call as skipped.
func (s *scanner) proceed(ctx context.Context, call string) bool {
	switch {
	case s.authFailed:
		s.calls.Skip(call, "not called: authentication failed")
		return false
	case ctx.Err() != nil:
		s.calls.Skip(call, "not called: the scan was cancelled")
		return false
	}
	return true
}

// deferSkip notes that call was not needed in one scope.
func (s *scanner) deferSkip(call, reason string) {
	if _, ok := s.deferredSkips[call]; !ok {
		s.deferredSkips[call] = reason
		s.skipOrder = append(s.skipOrder, call)
	}
}

// paginate fetches pages until the token runs out, an error, or maxPages,
// and records the call once for the scope. Hitting maxPages is recorded as
// a failed "(page limit)" entry: the listing is incomplete.
func (s *scanner) paginate(ctx context.Context, call string, maxPages int, fetch func(pageToken string) (string, int, error)) {
	items := 0
	token := ""
	for page := 1; ; page++ {
		next, n, err := fetch(token)
		items += n
		if err != nil {
			s.record(call, items, err)
			return
		}
		if next == "" {
			break
		}
		if page == maxPages {
			s.calls.Record(call+" (page limit)", 0, &sa.CallError{
				Outcome: models.ShadowCallFailed, Message: fmt.Sprintf("stopped after %d pages; later pages were not read", maxPages),
			})
			break
		}
		if ctx.Err() != nil {
			s.record(call, items, &sa.CallError{Outcome: models.ShadowCallFailed, Message: "cancelled: " + ctx.Err().Error()})
			return
		}
		token = next
	}
	s.record(call, items, nil)
}

func (s *scanner) addAlias(project, other string) {
	if other == "" || other == project {
		return
	}
	if s.aliases[project] == nil {
		s.aliases[project] = map[string]bool{}
	}
	s.aliases[project][other] = true
}

func (s *scanner) hasEngines(project string) bool {
	for _, e := range s.engines {
		if e.project == project {
			return true
		}
	}
	return false
}

// engineRef is a reference to a reasoning engine: project (ID or number),
// location and engine ID.
type engineRef struct {
	project  string
	location string
	id       string
}

var (
	engineNameRE = regexp.MustCompile(`projects/([^/]+)/locations/([^/]+)/reasoningEngines/([^/?#]+)`)
	engineURNRE  = regexp.MustCompile(`projects:([^:]+):locations:([^:]+):aiplatform:reasoningEngines:([^:/]+)$`)
	projectSegRE = regexp.MustCompile(`^projects/([^/]+)/`)
)

// parseEngineRef finds projects/P/locations/L/reasoningEngines/ID in a
// resource name, a //aiplatform.googleapis.com/... URI or a principal.
func parseEngineRef(s string) (engineRef, bool) {
	m := engineNameRE.FindStringSubmatch(s)
	if m == nil {
		return engineRef{}, false
	}
	return engineRef{project: m[1], location: strings.ToLower(m[2]), id: m[3]}, true
}

// parseEngineURN parses an Agent Runtime agentId:
// urn:agent:projects-N:projects:N:locations:L:aiplatform:reasoningEngines:ID.
func parseEngineURN(s string) (engineRef, bool) {
	m := engineURNRE.FindStringSubmatch(s)
	if m == nil {
		return engineRef{}, false
	}
	return engineRef{project: m[1], location: strings.ToLower(m[2]), id: m[3]}, true
}

// findEngine resolves a reference to a known engine. The project in a
// reference may be a number where the engine name has an ID (or the other
// way round), so it matches the engine's name segment, its configured
// project or any alias seen for that project. When no project matches,
// a unique (location, engine ID) pair still identifies the engine: engine
// IDs are long random numbers.
func (s *scanner) findEngine(ref engineRef) *engine {
	var cands []*engine
	for _, e := range s.engines {
		if e.id == ref.id && e.location == ref.location {
			cands = append(cands, e)
		}
	}
	for _, e := range cands {
		if e.nameProject == ref.project || e.project == ref.project || s.aliases[e.project][ref.project] {
			return e
		}
	}
	if len(cands) == 1 {
		return cands[0]
	}
	return nil
}

func scopeOf(project, location string) string {
	return "projects/" + project + "/locations/" + location
}
