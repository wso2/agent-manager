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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// The gateway's toolsets, as its MCP tools call them.
type DeploymentToolsetHandler interface {
	GetAgentDeployments(ctx context.Context, ouID string, projectName string, agentName string) ([]*gwmodels.DeploymentResponse, error)
}

type LLMProviderToolsetHandler interface {
	ListLLMProviders(ctx context.Context, ouID string, limit int, offset int) ([]*gwmodels.LLMProvider, int, error)
	ListLLMGuardrails(ctx context.Context, ouID string, providerID string) (*gwmodels.LLMPolicyAvailabilityResponse, error)
	// AddLLMProviderToCatalog puts a provider back in the catalog.
	AddLLMProviderToCatalog(ctx context.Context, ouID string, providerID string) (*gwmodels.LLMProvider, error)
	// GatewayEnvironmentIDs returns the UUIDs of the environments each gateway serves.
	GatewayEnvironmentIDs(ctx context.Context, gatewayIDs []string) (map[string][]string, error)
	GetLLMProvider(ctx context.Context, ouID string, providerID string) (*gwmodels.LLMProvider, []*gwmodels.Deployment, error)
	// UpdateLLMProviderGuardrails reads the provider once, saves change(its guardrails) and redeploys it. An
	// error from change is returned as is, and nothing is saved.
	UpdateLLMProviderGuardrails(ctx context.Context, ouID string, providerID string, change func([]gwmodels.LLMPolicy) ([]gwmodels.LLMPolicy, error)) (*gwmodels.LLMProvider, []gwmodels.GatewayDeploymentResult, error)
}

type MCPProxyToolsetHandler interface {
	ListMCPProxies(ctx context.Context, ouID string, limit int, offset int) (*gwmodels.MCPProxyListResponse, error)
	GetMCPProxy(ctx context.Context, ouID string, proxyID string) (*gwmodels.MCPProxyDTO, error)
}

type AgentConfigToolsetHandler interface {
	CreateAgentConfig(ctx context.Context, ouID string, projectName string, agentName string, req gwmodels.CreateAgentModelConfigRequest) (*gwmodels.AgentModelConfigResponse, error)
	// ProviderEnvironments lists the environments where the agent can use the LLM provider.
	ProviderEnvironments(ctx context.Context, ouID string, projectName string, agentName string, providerHandle string) ([]string, error)
	// RevokeConfigAPIKeys revokes the API keys of an external agent's configuration in one environment.
	RevokeConfigAPIKeys(ctx context.Context, ouID, projectName, agentName, configType, configID, envName string) error
}

// CoreReader is what the tools read from the core: rt.Core() is one.
type CoreReader interface {
	ListEnvironments(ctx context.Context, orgID string) ([]modkit.Environment, error)
	GetAgentByName(ctx context.Context, orgID, projectName, name string) (modkit.Agent, error)
}

// Registrar adds a tool to the core's MCP server: modkit.Runtime is one.
type Registrar interface {
	MCPTool(tool modkit.MCPTool, perm string, h modkit.MCPToolHandler, opts ...modkit.RouteOption)
}

// Tools are the gateway's MCP tools. A nil toolset leaves its tools out.
type Tools struct {
	Deployments  DeploymentToolsetHandler
	LLMProviders LLMProviderToolsetHandler
	MCPProxies   MCPProxyToolsetHandler
	AgentConfigs AgentConfigToolsetHandler
	Core         CoreReader
	// Can reports whether the caller holds a permission: rt.Can.
	Can func(ctx context.Context, perm string) bool
}

// Register adds the tools to reg.
func (t *Tools) Register(reg Registrar) {
	if t.Deployments != nil {
		t.registerDeploymentTools(reg)
	}
	if t.LLMProviders != nil {
		t.registerLLMProviderTools(reg)
	}
	if t.MCPProxies != nil {
		t.registerMCPProxyTools(reg)
	}
	if t.AgentConfigs != nil {
		t.registerAgentConfigTools(reg)
	}
}

// toolDef is a tool as it is declared here, with its schema as a map.
type toolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// addTool registers a typed tool. Arguments are decoded strictly, so a
// misspelled property is refused rather than dropped and defaulted. Every
// error the handler returns is shown to the caller, as the tool's own
// messages tell the model how to correct the call. A tool whose action is a
// read is not audited; any other is recorded under its action.
func addTool[T any](reg Registrar, def toolDef, action gwaudit.Action, h func(ctx context.Context, in T) (any, error), perm string) {
	schema, err := json.Marshal(def.InputSchema)
	if err != nil {
		panic(fmt.Sprintf("mcp tool %s: %v", def.Name, err))
	}
	read := isRead(action)
	tool := modkit.MCPTool{Name: def.Name, Description: def.Description, InputSchema: schema, ReadOnly: read}
	var opts []modkit.RouteOption
	if !read {
		opts = append(opts, modkit.Audit(short(action)))
	}
	reg.MCPTool(tool, short(perm), func(ctx context.Context, args json.RawMessage) (any, error) {
		var in T
		dec := json.NewDecoder(bytes.NewReader(args))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&in); err != nil {
			return nil, &modkit.Error{Message: fmt.Sprintf("%s: invalid arguments: %v", def.Name, err)}
		}
		out, err := h(ctx, in)
		if err != nil {
			return nil, &modkit.Error{Message: err.Error()}
		}
		return out, nil
	}, opts...)
}

// short is a gateway permission or audit action without the module prefix,
// as the Runtime takes it; a core permission is kept as is.
func short[S ~string](s S) string {
	return strings.TrimPrefix(string(s), gwperm.Module+":")
}

func isRead[S ~string](action S) bool {
	name := short(action)
	for _, a := range gwaudit.Actions {
		if a.Name == name {
			return a.Class == "read"
		}
	}
	return false
}

// resolvePagination applies the standard limit/offset defaults and bounds.
func resolvePagination(limitPtr *int, offsetPtr *int) (int, int, error) {
	limit := gwutil.DefaultLimit
	if limitPtr != nil {
		limit = *limitPtr
	}
	if limit < gwutil.MinLimit || limit > gwutil.MaxLimit {
		return 0, 0, fmt.Errorf("limit must be between %d and %d", gwutil.MinLimit, gwutil.MaxLimit)
	}
	offset := gwutil.DefaultOffset
	if offsetPtr != nil {
		offset = *offsetPtr
	}
	// Services take the offset as an int32 and add the limit to it.
	if offset < gwutil.MinOffset || offset > math.MaxInt32 {
		return 0, 0, fmt.Errorf("offset must be between %d and %d", gwutil.MinOffset, math.MaxInt32)
	}
	return limit, offset, nil
}

// createSchema builds an object schema that refuses properties it does not
// declare, as the strict decoding in addTool does.
func createSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func intProperty(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func arrayProperty(description string, itemSchema map[string]any) map[string]any {
	return map[string]any{"type": "array", "description": description, "items": itemSchema}
}

// wrapToolError turns the errors a model can act on into a message saying
// what to call next.
func wrapToolError(toolName string, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, gwerr.ErrOrganizationNotFound):
		return fmt.Errorf("%s: the organization on your token was not found. Re-authenticate and try again", toolName)
	case errors.Is(err, gwerr.ErrProjectNotFound):
		return fmt.Errorf("%s: invalid project name. Call list_projects to see valid projects", toolName)
	case errors.Is(err, gwerr.ErrAgentNotFound), errors.Is(err, modkit.ErrNotFound):
		return fmt.Errorf("%s: invalid agent name. Call list_agents or list_project_agent_pairs to see valid agents", toolName)
	case errors.Is(err, gwerr.ErrEnvironmentNotFound):
		return fmt.Errorf("%s: invalid environment name. Call list_environments to see valid environments", toolName)
	case errors.Is(err, gwerr.ErrLLMProviderNotFound):
		return fmt.Errorf("%s: LLM provider not found. Call list_llm_providers to see valid providers", toolName)
	case errors.Is(err, gwerr.ErrMCPProxyNotFound):
		return fmt.Errorf("%s: MCP proxy not found. Call list_mcp_proxies to see valid proxies", toolName)
	case errors.Is(err, gwerr.ErrAgentConfigAlreadyExists):
		return fmt.Errorf("%s: the agent already has a configuration with this name. Pass a different config_name", toolName)
	case errors.Is(err, httpx.ErrNotFound):
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "namespace not found") || strings.Contains(msg, "organization not found"):
			return fmt.Errorf("%s: the organization on your token was not found. Re-authenticate and try again", toolName)
		case strings.Contains(msg, "project not found"):
			return fmt.Errorf("%s: invalid project name. Call list_projects to see valid projects", toolName)
		case strings.Contains(msg, "agent not found") || strings.Contains(msg, "component not found"):
			return fmt.Errorf("%s: invalid agent name. Call list_agents or list_project_agent_pairs to see valid agents", toolName)
		}
	}
	return fmt.Errorf("%s: %w", toolName, err)
}

// credentialQueryHints are substrings of query parameter names that carry
// secrets, such as key, api-key, access_token, client_secret or sig.
var credentialQueryHints = []string{"key", "token", "secret", "pass", "auth", "sig", "credential"}

// redactURL drops what in a URL could be a credential before it reaches a
// model: userinfo, credential-named query parameters and the fragment. A URL
// that does not parse is dropped whole, since it cannot be redacted safely.
func redactURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u.User = nil
	u.Fragment, u.RawFragment = "", ""
	if u.RawQuery != "" {
		q := u.Query()
		for name := range q {
			lower := strings.ToLower(name)
			for _, hint := range credentialQueryHints {
				if strings.Contains(lower, hint) {
					q.Del(name)
					break
				}
			}
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}
