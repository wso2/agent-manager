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
	"net/url"

	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// workspaces lists the Managed Agents of each workspace that has a key.
func (s *scan) workspaces(ctx context.Context, p *platform, ids []string, noneReason string) {
	if len(ids) == 0 {
		s.calls.Skip(p.call(callAgents), noneReason)
		return
	}
	for _, ws := range ids {
		if ctx.Err() != nil {
			return
		}
		api, ok := p.wsAPI(ws)
		if !ok {
			continue
		}
		s.listAgents(ctx, p, api, ws)
	}
}

// listAgents lists the workspace's Managed Agents, archived ones included,
// and makes each a platform_agent finding (anthropic.md §2.1). The system
// prompt and the custom tools' schemas are never decoded.
func (s *scan) listAgents(ctx context.Context, p *platform, api *sa.API, ws string) {
	q := url.Values{"include_archived": {"true"}, "limit": {"100"}}
	call := p.call(callAgents)
	n, err := listByCursor(ctx, api, p.wsURL(ws, "/v1/agents"), q, p.wsHeaders(ws, betaManagedAgents), maxAgentPages,
		func(items []agent) bool {
			for i := range items {
				s.addAgent(p, ws, &items[i], call)
			}
			return true
		})
	s.calls.Record(call, n, err)
}

func (s *scan) addAgent(p *platform, ws string, a *agent, source string) {
	if a.ID == "" {
		return
	}
	f := s.agentFinding(a.ID)
	f.DisplayName = a.Name
	if f.DisplayName == "" {
		f.DisplayName = a.ID
	}
	f.Scope = p.wsScope(ws)
	archived := a.ArchivedAt != nil && *a.ArchivedAt != ""
	f.State = "active"
	if archived {
		f.State = "archived"
	}
	if t, ok := parseTime(a.CreatedAt); ok {
		f.ProviderCreatedAt = &t
	}
	attrs := f.Attributes
	attrs["workspace"] = ws
	attrs["archived"] = archived
	attrs["state"] = f.State
	if a.Version > 0 {
		attrs["agentVersion"] = a.Version
	}
	if a.Model != nil && a.Model.ID != "" {
		attrs["model"] = a.Model.ID
	}
	if d := deref(a.Description); d != "" {
		attrs["description"] = d
	}
	if a.ExecutionIdentity != nil && a.ExecutionIdentity.Type == "aws_role" && a.ExecutionIdentity.RoleARN != "" {
		attrs["roleArn"] = a.ExecutionIdentity.RoleARN
	}
	var tools []sa.ToolRef
	for _, t := range a.Tools {
		switch t.Type {
		case "mcp_toolset":
			tools = append(tools, sa.ToolRef{Type: t.Type, Name: t.MCPServerName})
		case "custom":
			tools = append(tools, sa.ToolRef{Type: t.Type, Name: t.Name})
		default: // agent_toolset_20260401 and future toolsets
			named := false
			for _, c := range t.Configs {
				if c.Enabled != nil && !*c.Enabled {
					continue
				}
				name := c.Name
				if name == "" {
					name = c.Type
				}
				tools = append(tools, sa.ToolRef{Type: t.Type, Name: name})
				named = true
			}
			if !named {
				tools = append(tools, sa.ToolRef{Type: t.Type})
			}
		}
	}
	if len(tools) > 0 {
		attrs["tools"] = tools
	}
	attrs["toolCount"] = len(a.Tools)
	var mcp []sa.MCPServerRef
	for _, m := range a.MCPServers {
		mcp = append(mcp, sa.MCPServerRef{Label: m.Name, Origin: sa.Origin(m.URL)})
	}
	if len(mcp) > 0 {
		attrs["mcpServers"] = mcp
	}
	var skills []string
	for _, sk := range a.Skills {
		if sk.SkillID != "" {
			skills = append(skills, sk.SkillID)
		}
	}
	if len(skills) > 0 {
		attrs["skills"] = skills
	}
	f.AddSignal(sa.Signal{Code: sa.SigAnthropicManagedAgent, Source: source, Observed: map[string]any{"archived": archived}})
}
