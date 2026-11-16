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
	"net/url"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// Connection test check names.
const (
	checkSettings = "Connection settings"
	checkToken    = "Service account key"
	checkEngines  = "Agent Runtime engines (reasoningEngines.list)"
	checkAgents   = "Agent Registry agents (agents.list)"
)

type probe struct {
	name    string
	option  string
	unlocks []string
	run     func(ctx context.Context) error
}

// Test implements sa.Connector: one cheap read per listing in the first
// configured project and location. The connection is OK when every
// selected listing works.
func (c *Connector) Test(ctx context.Context, conn sa.Connection) models.ShadowConnectionTestInfo {
	st, problems := readSettings(conn)
	if len(st.projects) == 0 || len(st.runtimeLocations) == 0 {
		return models.ShadowConnectionTestInfo{OK: false, Checks: []models.ShadowConnectionCheck{{
			Name: checkSettings, Outcome: models.ShadowCallFailed, Message: sa.ScrubText(strings.Join(problems, "; ")),
		}}}
	}
	api, err := c.newAPI(ctx, conn)
	if err != nil {
		return models.ShadowConnectionTestInfo{OK: false, Checks: []models.ShadowConnectionCheck{{
			Name: checkToken, Outcome: sa.OutcomeOf(err), Message: sa.MessageOf(err),
			Unlocks: []string{sa.SigGoogleAgentEngine, sa.SigGoogleRegistryAgent},
		}}}
	}
	project := st.projects[0]
	runtimeLoc := st.runtimeLocations[0]
	one := url.Values{"pageSize": {"1"}}

	probes := []probe{
		{checkEngines, "", []string{sa.SigGoogleAgentEngine}, func(ctx context.Context) error {
			base, err := runtimeBase(conn, runtimeLoc)
			if err != nil {
				return &sa.CallError{Outcome: models.ShadowCallFailed, Message: err.Error()}
			}
			u := base + "/v1/projects/" + url.PathEscape(project) + "/locations/" + url.PathEscape(runtimeLoc) + "/reasoningEngines"
			return api.GetJSON(ctx, u, one, &listEnginesResponse{})
		}},
		{checkAgents, sa.OptGoogleRegistry, []string{sa.SigGoogleRegistryAgent}, func(ctx context.Context) error {
			u, err := registryURL(conn, project, runtimeLoc, "agents")
			if err != nil {
				return &sa.CallError{Outcome: models.ShadowCallFailed, Message: err.Error()}
			}
			return api.GetJSON(ctx, u, one, &listAgentsResponse{})
		}},
	}

	info := models.ShadowConnectionTestInfo{}
	authFailed, failed := false, 0
	for _, p := range probes {
		check := models.ShadowConnectionCheck{Name: p.name, Unlocks: p.unlocks}
		if p.option != "" && !sa.ServiceEnabled(conn.Config, p.option) {
			check.Outcome, check.Message = models.ShadowCallSkipped, sa.ReasonServiceOff
			info.Checks = append(info.Checks, check)
			continue
		}
		if authFailed {
			check.Outcome, check.Message = models.ShadowCallSkipped, "not called: authentication failed"
			info.Checks = append(info.Checks, check)
			continue
		}
		err := p.run(ctx)
		check.Outcome, check.Message = sa.OutcomeOf(err), sa.MessageOf(err)
		if err != nil {
			failed++
		}
		if check.Outcome == models.ShadowCallAuthFailed {
			authFailed = true
		}
		info.Checks = append(info.Checks, check)
	}
	info.OK = failed == 0 && !authFailed
	return info
}
