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
	"encoding/json"
	"net/url"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	sa "github.com/wso2/agent-manager/agent-manager-service/services/shadowagents"
)

// listAgents lists the agents of every project that has a project key.
func (s *scan) listAgents(ctx context.Context) {
	ids := s.scopedProjects()
	if len(ids) == 0 {
		s.calls.Skip(callAgents, reasonNoProjectKey)
		return
	}
	for _, pid := range ids {
		if ctx.Err() != nil {
			break
		}
		var got []agent
		truncated, err := listAll(ctx, s.projectAPI(pid), s.base+"/agents", url.Values{"order": []string{"desc"}}, betaAgentsHeader(),
			listPageSize, maxListPages, func(a agent) bool {
				got = append(got, a)
				return true
			})
		// A 400/404 here usually means the beta contract changed; it is
		// recorded and the scan goes on.
		s.calls.Record(callAgents, len(got), err)
		if truncated {
			s.pageLimit(callAgents, maxListPages)
		}
		for _, a := range got {
			s.addAgent(pid, a)
		}
	}
}

func (s *scan) addAgent(projectID string, a agent) {
	if a.ID == "" {
		return
	}
	if _, dup := s.fs.Lookup(models.ShadowKindPlatformAgent, a.ID); dup {
		return
	}
	f := s.fs.Get(models.ShadowKindPlatformAgent, a.ID)
	f.DisplayName = a.Name
	if strings.TrimSpace(f.DisplayName) == "" {
		f.DisplayName = a.ID
	}
	f.Scope = projectID
	f.State = stateActive
	f.ProviderCreatedAt = unixTime(a.CreatedAt)
	tools, mcp := toolRefs(a.Tools)
	f.Attributes["model"] = a.Model
	f.Attributes["toolCount"] = int64(len(a.Tools))
	if len(tools) > 0 {
		f.Attributes["tools"] = tools
	}
	if len(mcp) > 0 {
		f.Attributes["mcpServers"] = mcp
	}
	f.AddSignal(sa.Signal{
		Code: sa.SigOpenAIAgentObject, Source: callAgents,
		Observed: map[string]any{"toolCount": int64(len(a.Tools)), "mcpServerCount": int64(len(mcp))},
	})
}

// toolRefs keeps each tool's type and name (an MCP tool's server label) and
// each MCP server's label and URL origin. Nothing else of a tool is kept.
func toolRefs(tools []agentTool) ([]sa.ToolRef, []sa.MCPServerRef) {
	refs := make([]sa.ToolRef, 0, len(tools))
	var mcp []sa.MCPServerRef
	for _, t := range tools {
		if t.Type == "" {
			continue
		}
		name := t.Name
		if t.Type == "mcp" {
			name = t.ServerLabel
			mcp = append(mcp, sa.MCPServerRef{Label: t.ServerLabel, Origin: mcpOrigin(t)})
		}
		refs = append(refs, sa.ToolRef{Type: t.Type, Name: name})
	}
	return refs, mcp
}

// mcpOrigin finds the scheme and host of an MCP tool's server. The
// persisted agent tool documents server_label, transport and
// connection_origin but not their inner shape, so the URL is looked for,
// leniently, in server_url, transport.url / transport.server_url and a
// URL-valued connection_origin. Only sa.Origin of it is kept.
func mcpOrigin(t agentTool) string {
	if o := sa.Origin(t.ServerURL); o != "" {
		return o
	}
	var tr struct {
		URL       string `json:"url"`
		ServerURL string `json:"server_url"`
	}
	if len(t.Transport) > 0 && json.Unmarshal(t.Transport, &tr) == nil {
		if o := sa.Origin(tr.ServerURL); o != "" {
			return o
		}
		if o := sa.Origin(tr.URL); o != "" {
			return o
		}
	}
	var origin string
	if len(t.ConnectionOrigin) > 0 && json.Unmarshal(t.ConnectionOrigin, &origin) == nil {
		if strings.HasPrefix(origin, "https://") || strings.HasPrefix(origin, "http://") {
			return sa.Origin(origin)
		}
	}
	return ""
}
