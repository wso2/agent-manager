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
	"net/http"
	"net/url"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// Test lists one agent with each project key. OK means every project key
// can list its project's agents.
func (c *Connector) Test(ctx context.Context, conn sa.Connection) models.ShadowConnectionTestInfo {
	info := models.ShadowConnectionTestInfo{Checks: []models.ShadowConnectionCheck{}}
	base, err := baseURL(conn)
	if err != nil {
		info.Checks = append(info.Checks, models.ShadowConnectionCheck{
			Name: "Agents", Outcome: models.ShadowCallFailed, Message: sa.ScrubText(err.Error()),
		})
		return info
	}
	add := func(name string, unlocks []string, err error) bool {
		info.Checks = append(info.Checks, models.ShadowConnectionCheck{
			Name: name, Outcome: sa.OutcomeOf(err), Message: sa.MessageOf(err), Unlocks: unlocks,
		})
		return err == nil
	}
	get := func(api *sa.API, path string, q url.Values, hdr http.Header) error {
		q.Set("limit", "1")
		var out map[string]any
		return api.DoJSON(ctx, sa.Request{Method: http.MethodGet, URL: base + path, Query: q, Header: hdr}, &out)
	}

	ok := true
	projectChecks := 0
	for _, pid := range sortedKeys(conn.Secret.ScopedKeys) {
		key := strings.TrimSpace(conn.Secret.ScopedKeys[pid])
		if key == "" {
			continue
		}
		projectChecks++
		if !add("Agents (project "+pid+")", []string{sa.SigOpenAIAgentObject},
			get(c.api(key), "/agents", url.Values{}, betaAgentsHeader())) {
			ok = false
		}
	}
	if projectChecks == 0 {
		info.Checks = append(info.Checks, models.ShadowConnectionCheck{
			Name: "Agents", Outcome: models.ShadowCallSkipped, Message: reasonNoProjectKey,
		})
	}
	info.OK = ok && projectChecks > 0
	c.logger.Info("Shadow agent discovery OpenAI connection test finished",
		"connectionId", conn.ID.String(), "ok", info.OK, "checks", len(info.Checks))
	return info
}
