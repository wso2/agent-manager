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

package openai

import (
	"context"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// scan is the state of one connection's scan.
type scan struct {
	c     *Connector
	conn  sa.Connection
	base  string
	calls *sa.Calls
	fs    *sa.Findings
}

// Scan lists the agents of every project with a project key.
func (c *Connector) Scan(ctx context.Context, conn sa.Connection) *sa.Result {
	s := &scan{c: c, conn: conn, calls: sa.NewCalls(), fs: sa.NewFindings()}
	s.run(ctx)
	return s.result()
}

func (s *scan) run(ctx context.Context) {
	base, err := baseURL(s.conn)
	if err != nil {
		s.calls.Record(callAgents, 0, &sa.CallError{Outcome: models.ShadowCallFailed, Message: sa.ScrubText(err.Error())})
		return
	}
	s.base = base
	s.listAgents(ctx)
}

func (s *scan) result() *sa.Result {
	findings := s.fs.All()
	for _, f := range findings {
		f.Attributes = sa.SanitizeAttributes(f.Attributes)
		f.DisplayName = sa.SanitizeName(f.DisplayName)
	}
	res := &sa.Result{Findings: findings, Calls: s.calls.Results()}
	failed := 0
	for _, r := range res.Calls {
		if r.Outcome != models.ShadowCallOK && r.Outcome != models.ShadowCallSkipped {
			failed++
		}
	}
	s.c.logger.Info("Shadow agent discovery OpenAI scan finished",
		"connectionId", s.conn.ID.String(), "projects", len(s.scopedProjects()),
		"agents", len(findings), "calls", len(res.Calls), "failedCalls", failed)
	return res
}

// pageLimit records that a listing stopped at its page bound. The listing
// is incomplete, so the call is recorded as failed.
func (s *scan) pageLimit(call string, pages int) {
	s.calls.Record(call+suffixPageLimit, 0, &sa.CallError{
		Outcome: models.ShadowCallFailed, Message: "stopped after " + plural(pages, "page", "pages") + "; later pages were not read",
	})
}

// scopedProjects returns the project IDs that have a project key, sorted.
func (s *scan) scopedProjects() []string {
	out := []string{}
	for _, id := range sortedKeys(s.conn.Secret.ScopedKeys) {
		if strings.TrimSpace(id) != "" && strings.TrimSpace(s.conn.Secret.ScopedKeys[id]) != "" {
			out = append(out, id)
		}
	}
	return out
}

func (s *scan) projectAPI(id string) *sa.API {
	return s.c.api(strings.TrimSpace(s.conn.Secret.ScopedKeys[id]))
}
