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

package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// Test implements sa.Connector: it lists one Managed Agent with each
// workspace key. The connection is OK when every workspace key can list its
// Managed Agents.
func (c *Connector) Test(ctx context.Context, conn sa.Connection) models.ShadowConnectionTestInfo {
	s := c.newScan(conn)
	var checks []models.ShadowConnectionCheck
	p, err := s.firstPartyPlatform()
	if err != nil {
		checks = append(checks, failedCheck("Managed Agents", err, signalNames(sa.SigAnthropicManagedAgent)))
		return models.ShadowConnectionTestInfo{OK: false, Checks: checks}
	}
	ok := true
	wsChecks := 0
	for _, ws := range sortedKeys(conn.Secret.ScopedKeys) {
		api, has := p.wsAPI(ws)
		if !has {
			continue
		}
		wsChecks++
		ch := s.probe(ctx, api, "Workspace "+ws+": Managed Agents", p.wsURL(ws, "/v1/agents"), url.Values{"limit": {"1"}},
			p.wsHeaders(ws, betaManagedAgents), signalNames(sa.SigAnthropicManagedAgent))
		checks = append(checks, ch)
		ok = ok && ch.Outcome == models.ShadowCallOK
	}
	if wsChecks == 0 {
		checks = append(checks, models.ShadowConnectionCheck{
			Name: "Managed Agents", Outcome: models.ShadowCallSkipped, Message: "no workspace keys",
		})
	}
	return models.ShadowConnectionTestInfo{OK: ok && wsChecks > 0, Checks: checks}
}

// probe makes one GET and checks the response has a data list.
func (s *scan) probe(ctx context.Context, api *sa.API, name, u string, q url.Values, h http.Header, unlocks []string) models.ShadowConnectionCheck {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var env struct {
		Data *json.RawMessage `json:"data"`
	}
	err := api.DoJSON(ctx, sa.Request{Method: http.MethodGet, URL: u, Query: q, Header: h}, &env)
	if err == nil && env.Data == nil {
		err = errNoData()
	}
	return models.ShadowConnectionCheck{Name: name, Outcome: sa.OutcomeOf(err), Message: sa.MessageOf(err), Unlocks: unlocks}
}

func failedCheck(name string, err error, unlocks []string) models.ShadowConnectionCheck {
	return models.ShadowConnectionCheck{Name: name, Outcome: sa.OutcomeOf(err), Message: sa.MessageOf(err), Unlocks: unlocks}
}

// signalNames returns the catalog names of evidence codes.
func signalNames(codes ...string) []string {
	out := make([]string, 0, len(codes))
	for _, c := range codes {
		if d, ok := sa.Def(c); ok {
			out = append(out, d.Name)
		}
	}
	return out
}
