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

package gwmcp

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	testOrgName     = "default-org"
	testProjectName = "default-project"
	testAgentName   = "default-agent"
	testEnvName     = "default-env"

	testEnvUUID       = "00000000-0000-0000-0000-0000000000e1"
	testTemplateID    = "openai"
	testProviderID    = "test-provider"
	testGatewayID     = "00000000-0000-0000-0000-0000000000a1"
	testProxyID       = "test-proxy"
	testGuardrailName = "pii-masking"
	// testSecret is an upstream credential: no tool output may contain it.
	testSecret = "Bearer sk-must-never-be-returned"
)

// MockToolsetHandler implements every toolset and the core reader the tools use.
type MockToolsetHandler struct {
	calls map[string][]interface{}
}

func NewMockToolsetHandler() *MockToolsetHandler {
	return &MockToolsetHandler{calls: make(map[string][]interface{})}
}

func (m *MockToolsetHandler) recordCall(method string, args ...interface{}) {
	m.calls[method] = append(m.calls[method], args)
}

func (m *MockToolsetHandler) ListEnvironments(_ context.Context, orgID string) ([]modkit.Environment, error) {
	m.recordCall("ListEnvironments", orgID)
	return []modkit.Environment{{ID: testEnvUUID, OrgID: orgID, Name: testEnvName, DisplayName: "Default Env"}}, nil
}

func (m *MockToolsetHandler) GetAgentByName(_ context.Context, orgID, projectName, name string) (modkit.Agent, error) {
	m.recordCall("GetAgentByName", orgID, projectName, name)
	if name != testAgentName {
		return modkit.Agent{}, modkit.ErrNotFound
	}
	return modkit.Agent{ID: "agent-1", OrgID: orgID, ProjectName: projectName, Name: name, Provisioning: string(utils.InternalAgent)}, nil
}

func (m *MockToolsetHandler) GetAgentDeployments(
	ctx context.Context, ouID string, projectName string, agentName string,
) ([]*gwmodels.DeploymentResponse, error) {
	m.recordCall("GetAgentDeployments", ouID, projectName, agentName)
	return []*gwmodels.DeploymentResponse{}, nil
}

// Environment Toolset Handler

func (m *MockToolsetHandler) ListLLMProviders(
	ctx context.Context, ouID string, limit int, offset int,
) ([]*gwmodels.LLMProvider, int, error) {
	m.recordCall("ListLLMProviders", ouID, limit, offset)
	p := &gwmodels.LLMProvider{
		TemplateHandle: testTemplateID, Status: "deployed", InCatalog: true,
		Artifact: &gwmodels.Artifact{Handle: testProviderID, Name: "Test Provider"},
	}
	return []*gwmodels.LLMProvider{p}, 1, nil
}

func (m *MockToolsetHandler) AddLLMProviderToCatalog(
	ctx context.Context, ouID string, providerID string,
) (*gwmodels.LLMProvider, error) {
	m.recordCall("AddLLMProviderToCatalog", ouID, providerID)
	return testLLMProvider(providerID), nil
}

func (m *MockToolsetHandler) GatewayEnvironmentIDs(
	ctx context.Context, gatewayIDs []string,
) (map[string][]string, error) {
	m.recordCall("GatewayEnvironmentIDs", gatewayIDs)
	out := make(map[string][]string, len(gatewayIDs))
	for _, id := range gatewayIDs {
		out[id] = []string{testEnvUUID}
	}
	return out, nil
}

func (m *MockToolsetHandler) ListLLMGuardrails(
	ctx context.Context, ouID string, providerID string,
) (*gwmodels.LLMPolicyAvailabilityResponse, error) {
	m.recordCall("ListLLMGuardrails", ouID, providerID)
	schema := map[string]interface{}{"type": "object", "properties": map[string]interface{}{"mask": map[string]interface{}{"type": "boolean"}}}
	return &gwmodels.LLMPolicyAvailabilityResponse{Count: 3, List: []gwmodels.LLMPolicyDefinition{
		{Name: testGuardrailName, Version: "v1", Parameters: schema},
		{Name: "word-count", Version: "v1", Parameters: schema},
		{Name: "cors", Version: "v1", Parameters: schema},
	}}, nil
}

func (m *MockToolsetHandler) GetLLMProvider(
	ctx context.Context, ouID string, providerID string,
) (*gwmodels.LLMProvider, []*gwmodels.Deployment, error) {
	m.recordCall("GetLLMProvider", ouID, providerID)
	status, archived := gwmodels.DeploymentStatusDeployed, gwmodels.DeploymentStatusArchived
	return testLLMProvider(providerID), []*gwmodels.Deployment{{Status: &status}, {Status: &archived}}, nil
}

// testLLMProvider is the provider the mock serves: deployed, with an upstream
// credential and one all-routes guardrail.
func testLLMProvider(providerID string) *gwmodels.LLMProvider {
	authType, header, value := "api-key", "Authorization", testSecret
	return &gwmodels.LLMProvider{
		TemplateHandle: testTemplateID, Status: "deployed", InCatalog: true,
		Artifact: &gwmodels.Artifact{Handle: providerID, Name: "Test Provider"},
		Configuration: gwmodels.LLMProviderConfig{
			Version: "v1.0",
			Upstream: &gwmodels.UpstreamConfig{Main: &gwmodels.UpstreamEndpoint{
				URL:  "https://api.openai.com/v1",
				Auth: &gwmodels.UpstreamAuth{Type: &authType, Header: &header, Value: &value},
			}},
			Policies: []gwmodels.LLMPolicy{{
				Name: testGuardrailName, Version: "v1",
				Paths: []gwmodels.LLMPolicyPath{{Path: "/*", Methods: []string{"*"}, Params: map[string]interface{}{"mask": true}}},
			}},
		},
	}
}

// UpdateLLMProviderGuardrails applies change to the guardrails GetLLMProvider
// serves. It records the call only when something is saved, with the saved
// guardrails as args[2], so "not called" means nothing was saved.
func (m *MockToolsetHandler) UpdateLLMProviderGuardrails(
	ctx context.Context, ouID string, providerID string, change func([]gwmodels.LLMPolicy) ([]gwmodels.LLMPolicy, error),
) (*gwmodels.LLMProvider, []gwmodels.GatewayDeploymentResult, error) {
	return m.saveGuardrails(ouID, providerID, testLLMProvider(providerID).Configuration.Policies, change)
}

func (m *MockToolsetHandler) saveGuardrails(
	ouID, providerID string, current []gwmodels.LLMPolicy, change func([]gwmodels.LLMPolicy) ([]gwmodels.LLMPolicy, error),
) (*gwmodels.LLMProvider, []gwmodels.GatewayDeploymentResult, error) {
	guardrails, err := change(current)
	if err != nil {
		return nil, nil, err
	}
	m.recordCall("UpdateLLMProviderGuardrails", ouID, providerID, guardrails)
	p := &gwmodels.LLMProvider{Artifact: &gwmodels.Artifact{Handle: providerID}}
	p.Configuration.Policies = guardrails
	return p, []gwmodels.GatewayDeploymentResult{{GatewayID: testGatewayID, Success: true}}, nil
}

// MCP Proxy Toolset Handler

func (m *MockToolsetHandler) ListMCPProxies(
	ctx context.Context, ouID string, limit int, offset int,
) (*gwmodels.MCPProxyListResponse, error) {
	m.recordCall("ListMCPProxies", ouID, limit, offset)
	id, name := testProxyID, "Test Proxy"
	return &gwmodels.MCPProxyListResponse{
		Count:      1,
		List:       []gwmodels.MCPProxyListItem{{ID: &id, Name: &name}},
		Pagination: gwmodels.PaginationInfo{Count: 1, Limit: limit, Offset: offset},
	}, nil
}

func (m *MockToolsetHandler) GetMCPProxy(
	ctx context.Context, ouID string, proxyID string,
) (*gwmodels.MCPProxyDTO, error) {
	m.recordCall("GetMCPProxy", ouID, proxyID)
	authType, header, value := "api-key", "Authorization", testSecret
	tools := []map[string]interface{}{{"name": "search"}}
	return &gwmodels.MCPProxyDTO{
		ID: proxyID, Name: "Test Proxy", Version: "1.0.0",
		Endpoints: []gwmodels.MCPProxyEndpointDTO{{
			ID: "default",
			Upstream: gwmodels.UpstreamConfig{Main: &gwmodels.UpstreamEndpoint{
				URL:  "https://mcp.example.com/mcp",
				Auth: &gwmodels.UpstreamAuth{Type: &authType, Header: &header, Value: &value},
			}},
			Capabilities: &gwmodels.MCPProxyCapabilities{Tools: &tools},
			Environments: []gwmodels.MCPEndpointEnvironmentDTO{{EnvironmentUUID: testEnvUUID, DeploymentStatus: "Deployed"}},
		}},
	}, nil
}

// Agent Config Toolset Handler

func (m *MockToolsetHandler) ProviderEnvironments(
	ctx context.Context, ouID string, projectName string, agentName string, providerHandle string,
) ([]string, error) {
	m.recordCall("ProviderEnvironments", ouID, projectName, agentName, providerHandle)
	return []string{testEnvName}, nil
}

func (m *MockToolsetHandler) RevokeConfigAPIKeys(
	ctx context.Context, ouID, projectName, agentName, configType, configID, envName string,
) error {
	m.recordCall("RevokeConfigAPIKeys", ouID, projectName, agentName, configType, configID, envName)
	return nil
}

func (m *MockToolsetHandler) CreateAgentConfig(
	ctx context.Context, ouID string, projectName string, agentName string, req gwmodels.CreateAgentModelConfigRequest,
) (*gwmodels.AgentModelConfigResponse, error) {
	m.recordCall("CreateAgentConfig", ouID, projectName, agentName, req)
	// One per-agent proxy per environment, named the way the service names it.
	envConfig := make(map[string]gwmodels.EnvModelConfigResponse, len(req.EnvMappings))
	for env := range req.EnvMappings {
		proxyName := req.Name + "-" + env + "-proxy"
		envConfig[env] = gwmodels.EnvModelConfigResponse{EnvironmentName: env, LLMProxy: &gwmodels.LLMProxyInfo{ProxyName: &proxyName}}
	}
	return &gwmodels.AgentModelConfigResponse{
		UUID:           "config-uuid",
		Name:           req.Name,
		Type:           req.Type,
		EnvModelConfig: envConfig,
		EnvironmentVariables: []gwmodels.EnvironmentVariableConfig{
			{Name: "OPENAI_URL", Key: "url"},
			{Name: "OPENAI_API_KEY", Key: "apikey"},
		},
	}, nil
}

// testSession runs the registered tools the way the core's MCP server runs a
// module's tools, minus its scope check: arguments go to the handler as
// JSON, a returned error is an error result showing its message, and a result
// is its JSON text.
type testSession struct {
	tools  map[string]registeredTool
	scopes []string
}

type registeredTool struct {
	tool modkit.MCPTool
	perm string
	opts modkit.RouteOptions
	h    modkit.MCPToolHandler
}

func (s *testSession) MCPTool(tool modkit.MCPTool, perm string, h modkit.MCPToolHandler, opts ...modkit.RouteOption) {
	s.tools[tool.Name] = registeredTool{tool: tool, perm: perm, opts: modkit.Options(opts), h: h}
}

func (s *testSession) can(_ context.Context, perm string) bool {
	full := rbac.Permission(perm)
	if !strings.Contains(perm, ":") {
		full = rbac.Permission(gwperm.Module + ":" + perm)
	}
	for _, sc := range s.scopes {
		if sc == full.Scope() {
			return true
		}
	}
	return false
}

func (s *testSession) CallTool(ctx context.Context, p *gomcp.CallToolParams) (*gomcp.CallToolResult, error) {
	t, ok := s.tools[p.Name]
	if !ok {
		return nil, errors.New("unknown tool " + p.Name)
	}
	args, err := json.Marshal(p.Arguments)
	if err != nil {
		return nil, err
	}
	if p.Arguments == nil {
		args = []byte(`{}`)
	}
	out, err := t.h(modkit.WithOrgID(ctx, testOrgName), args)
	if err != nil {
		var shown *modkit.Error
		if !errors.As(err, &shown) {
			return nil, err
		}
		return &gomcp.CallToolResult{IsError: true, Content: []gomcp.Content{&gomcp.TextContent{Text: shown.Message}}}, nil
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return &gomcp.CallToolResult{Content: []gomcp.Content{&gomcp.TextContent{Text: string(data)}}}, nil
}

func setupTestServer(t *testing.T) (*testSession, *MockToolsetHandler) {
	t.Helper()
	mock := NewMockToolsetHandler()
	tools := &Tools{Deployments: mock, LLMProviders: mock, MCPProxies: mock, AgentConfigs: mock, Core: mock}
	return setupTestServerWithToolsets(t, tools), mock
}

func setupTestServerWithToolsets(t *testing.T, tools *Tools) *testSession {
	t.Helper()
	return setupTestServerWithScopes(t, tools, unionScopes())
}

// setupTestServerWithScopes registers tools for a caller holding scopes, a
// space-separated list.
func setupTestServerWithScopes(t *testing.T, tools *Tools, scopes string) *testSession {
	t.Helper()
	s := &testSession{tools: map[string]registeredTool{}, scopes: strings.Fields(scopes)}
	tools.Can = s.can
	tools.Register(s)
	return s
}

// toolTestSpec declares a tool's expected surface and one wiring call.
type toolTestSpec struct {
	name                string
	toolset             string
	permissions         []rbac.Permission
	descriptionKeywords []string
	descriptionMinLen   int
	requiredParams      []string
	optionalParams      []string
	testArgs            map[string]any
	expectedMethod      string
	validateCall        func(t *testing.T, args []interface{})
}

var allToolSpecs = func() []toolTestSpec {
	specs := deploymentToolSpecs()
	specs = append(specs, llmProviderToolSpecs()...)
	specs = append(specs, mcpProxyToolSpecs()...)
	specs = append(specs, agentConfigToolSpecs()...)
	return specs
}()

// unionScopes covers every permission any tool declares.
func unionScopes() string {
	seen := map[string]bool{}
	var scopes []string
	for _, spec := range allToolSpecs {
		for _, perm := range spec.permissions {
			if !seen[perm.Scope()] {
				seen[perm.Scope()] = true
				scopes = append(scopes, perm.Scope())
			}
		}
	}
	return strings.Join(scopes, " ")
}

// TestToolSpecs checks each tool against its spec: its permission and audit
// action, its description and parameters, and that a call reaches the
// toolset method with the arguments the spec expects.
func TestToolSpecs(t *testing.T) {
	session, _ := setupTestServer(t)
	if len(session.tools) != len(allToolSpecs) {
		t.Errorf("registered %d tools, the specs list %d", len(session.tools), len(allToolSpecs))
	}
	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			tool, ok := session.tools[spec.name]
			if !ok {
				t.Fatalf("tool %q not registered", spec.name)
			}
			if len(spec.permissions) != 1 || tool.perm != short(spec.permissions[0]) {
				t.Errorf("registered permission %q, spec expects %v", tool.perm, spec.permissions)
			}
			if read := isRead(tool.opts.AuditAction); tool.tool.ReadOnly != (tool.opts.AuditAction == "") || read {
				t.Errorf("read-only %v with audit action %q: a change is recorded under its action, a read not at all",
					tool.tool.ReadOnly, tool.opts.AuditAction)
			}
			desc := strings.ToLower(tool.tool.Description)
			if len(desc) < spec.descriptionMinLen {
				t.Errorf("description too short: got %d chars, want >= %d", len(desc), spec.descriptionMinLen)
			}
			for _, kw := range spec.descriptionKeywords {
				if !strings.Contains(desc, strings.ToLower(kw)) {
					t.Errorf("description missing keyword %q", kw)
				}
			}
			var schema struct {
				Properties map[string]any `json:"properties"`
				Required   []string       `json:"required"`
			}
			if err := json.Unmarshal(tool.tool.InputSchema, &schema); err != nil {
				t.Fatalf("input schema: %v", err)
			}
			for _, p := range append(append([]string(nil), spec.requiredParams...), spec.optionalParams...) {
				if _, ok := schema.Properties[p]; !ok {
					t.Errorf("parameter %q is not in the schema", p)
				}
			}
			if !slices.Equal(schema.Required, spec.requiredParams) && (len(schema.Required) != 0 || len(spec.requiredParams) != 0) {
				t.Errorf("required %v, spec expects %v", schema.Required, spec.requiredParams)
			}
			if spec.expectedMethod == "" {
				return
			}
			s, mock := setupTestServer(t)
			if _, err := s.CallTool(context.Background(), &gomcp.CallToolParams{Name: spec.name, Arguments: spec.testArgs}); err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			calls := mock.calls[spec.expectedMethod]
			if len(calls) == 0 {
				t.Fatalf("expected method %q was not called; calls: %v", spec.expectedMethod, mock.calls)
			}
			spec.validateCall(t, calls[0].([]interface{}))
		})
	}
}
