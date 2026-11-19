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

package tools

import (
	"context"
	"encoding/json"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

const testProjectProxyID = "orders-mcp"

// projectProxyMock owns one org-level proxy and, in testProjectName, one project proxy (as an integration
// publishes it) bound in two environments: one with discovered tools, one still discovering them.
type projectProxyMock struct {
	*MockToolsetHandler
}

func (m projectProxyMock) ListMCPProxies(
	ctx context.Context, ouID string, projectName string, limit int, offset int,
) (*models.MCPProxyListResponse, error) {
	m.recordCall("ListMCPProxies", ouID, projectName, limit, offset)
	orgID, orgName := testProxyID, "Org Proxy"
	list := []models.MCPProxyListItem{{ID: &orgID, Name: &orgName}}
	if projectName == testProjectName {
		id, name, project := testProjectProxyID, "Orders MCP", testProjectName
		list = append(list, models.MCPProxyListItem{ID: &id, Name: &name, ProjectName: &project})
	}
	return &models.MCPProxyListResponse{
		Count: len(list), List: list, Pagination: models.PaginationInfo{Count: len(list), Limit: limit, Offset: offset},
	}, nil
}

func (m projectProxyMock) GetMCPProxy(ctx context.Context, ouID string, proxyID string) (*models.MCPProxyDTO, error) {
	if proxyID != testProjectProxyID {
		return m.MockToolsetHandler.GetMCPProxy(ctx, ouID, proxyID)
	}
	m.recordCall("GetMCPProxy", ouID, proxyID)
	project := testProjectName
	// Get leaves out a project proxy's endpoint rows: they are its bindings.
	return &models.MCPProxyDTO{ID: proxyID, Name: "Orders MCP", Version: "1.0.0", ProjectName: &project}, nil
}

func (m projectProxyMock) GetProjectMCPProxy(
	ctx context.Context, ouID string, projectName string, proxyID string,
) (*models.ProjectMCPProxyResponse, error) {
	m.recordCall("GetProjectMCPProxy", ouID, projectName, proxyID)
	tools := []map[string]interface{}{{"name": "get_order"}, {"name": "cancel_order"}}
	return &models.ProjectMCPProxyResponse{
		ID: proxyID, Name: "Orders MCP", Version: "1.0.0", ProjectName: projectName,
		Environments: []models.MCPProxyEnvironmentResponse{
			{
				EnvironmentName:     "development",
				DeploymentStatus:    "Deployed",
				PublicURL:           "https://dev-gw.example.com/default-project/orders/mcp",
				Capabilities:        &models.MCPProxyCapabilities{Tools: &tools},
				CapabilitiesSource:  models.MCPCapabilitiesSourceDiscovered,
				CapabilityDiscovery: &models.MCPCapabilityDiscoveryStatus{Status: "Discovered"},
			},
			{
				EnvironmentName:  "production",
				DeploymentStatus: "Deploying",
				CapabilityDiscovery: &models.MCPCapabilityDiscoveryStatus{
					Status: "Pending", LastError: "the binding is not deployed yet",
				},
			},
		},
	}, nil
}

func callMCPProxyTool(t *testing.T, session *gomcp.ClientSession, name string, args map[string]any, out any) {
	t.Helper()
	res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text := res.Content[0].(*gomcp.TextContent).Text
	if res.IsError {
		t.Fatalf("%s returned an error: %s", name, text)
	}
	if err := json.Unmarshal([]byte(text), out); err != nil {
		t.Fatalf("decode %s: %v (%s)", name, err, text)
	}
}

// A project's own proxies, such as the MCP servers its integrations publish, are listed when the project is named,
// with the project as their owner; without it the list keeps to the organization's proxies.
func TestListMCPProxiesIncludesAProjectsProxies(t *testing.T) {
	mock := projectProxyMock{NewMockToolsetHandler()}
	session := setupTestServerWithToolsets(t, &Toolsets{MCPProxyToolset: mock})

	var withProject listMCPProxiesOutput
	callMCPProxyTool(t, session, "list_mcp_proxies", map[string]any{"project_name": testProjectName}, &withProject)
	if len(withProject.Proxies) != 2 {
		t.Fatalf("with project_name: got %+v, want the org proxy and the project's", withProject.Proxies)
	}
	if p := withProject.Proxies[1]; p.ID != testProjectProxyID || p.Project != testProjectName {
		t.Errorf("project proxy: got %+v", p)
	}
	if p := withProject.Proxies[0]; p.Project != "" {
		t.Errorf("org proxy carries a project: %+v", p)
	}

	var orgOnly listMCPProxiesOutput
	callMCPProxyTool(t, session, "list_mcp_proxies", map[string]any{}, &orgOnly)
	if len(orgOnly.Proxies) != 1 || orgOnly.Proxies[0].ID != testProxyID {
		t.Errorf("without project_name: got %+v, want the org proxy only", orgOnly.Proxies)
	}

	calls := mock.calls["ListMCPProxies"]
	if len(calls) != 2 || calls[0].([]interface{})[1] != testProjectName || calls[1].([]interface{})[1] != "" {
		t.Errorf("project passed to the handler: got %v", calls)
	}
}

// get_mcp_proxy on a project's proxy reports each environment's tools from its binding, including discovered ones,
// and where discovery stands.
func TestGetMCPProxyReportsAProjectProxysToolsPerEnvironment(t *testing.T) {
	mock := projectProxyMock{NewMockToolsetHandler()}
	session := setupTestServerWithToolsets(t, &Toolsets{MCPProxyToolset: mock})

	var out getMCPProxyOutput
	callMCPProxyTool(t, session, "get_mcp_proxy", map[string]any{"proxy_id": testProjectProxyID}, &out)

	if out.Project != testProjectName || len(out.Environments) != 2 {
		t.Fatalf("got %+v", out)
	}
	dev, prod := out.Environments[0], out.Environments[1]
	if dev.Name != "development" || dev.DeploymentStatus != "Deployed" || dev.ToolsSource != "discovered" ||
		dev.ToolDiscovery != "Discovered" || dev.URL != "https://dev-gw.example.com/default-project/orders/mcp" {
		t.Errorf("development: got %+v", dev)
	}
	if len(dev.Tools) != 2 || dev.Tools[0] != "get_order" || dev.Tools[1] != "cancel_order" {
		t.Errorf("development tools: got %v", dev.Tools)
	}
	if prod.ToolDiscovery != "Pending" || prod.ToolDiscoveryError == "" || len(prod.Tools) != 0 {
		t.Errorf("production: got %+v", prod)
	}

	calls := mock.calls["GetProjectMCPProxy"]
	if len(calls) != 1 {
		t.Fatalf("GetProjectMCPProxy calls: got %v", calls)
	}
	if args := calls[0].([]interface{}); args[1] != testProjectName || args[2] != testProjectProxyID {
		t.Errorf("GetProjectMCPProxy args: got %v", args)
	}
}

// An org-level proxy keeps its endpoint view and is not read as a project's.
func TestGetMCPProxyKeepsTheOrgProxyView(t *testing.T) {
	mock := projectProxyMock{NewMockToolsetHandler()}
	session := setupTestServerWithToolsets(t, &Toolsets{MCPProxyToolset: mock})

	var out getMCPProxyOutput
	callMCPProxyTool(t, session, "get_mcp_proxy", map[string]any{"proxy_id": testProxyID}, &out)

	if out.Project != "" || len(out.Environments) != 0 || len(out.Endpoints) != 1 || len(out.Endpoints[0].Tools) != 1 {
		t.Errorf("got %+v", out)
	}
	if _, called := mock.calls["GetProjectMCPProxy"]; called {
		t.Error("an org proxy was read through the project view")
	}
}
