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
	"fmt"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type attachLLMProviderInput struct {
	ProjectName  string           `json:"project_name"`
	AgentName    string           `json:"agent_name"`
	Provider     string           `json:"provider"`
	ConfigName   string           `json:"config_name,omitempty"`
	Environments []string         `json:"environments,omitempty"`
	Guardrails   []guardrailInput `json:"guardrails,omitempty"`
}
type attachMCPProxyInput struct {
	ProjectName  string   `json:"project_name"`
	AgentName    string   `json:"agent_name"`
	Proxy        string   `json:"proxy"`
	ConfigName   string   `json:"config_name,omitempty"`
	Environments []string `json:"environments,omitempty"`
}

type envVarItem struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}
type attachConfigOutput struct {
	ConfigID             string       `json:"config_id"`
	ConfigName           string       `json:"config_name"`
	ProjectName          string       `json:"project_name"`
	AgentName            string       `json:"agent_name"`
	Environments         []string     `json:"environments"`
	Guardrails           []string     `json:"guardrails,omitempty"`
	EnvironmentVariables []envVarItem `json:"environment_variables"`
	Note                 string       `json:"note"`
}

const attachNote = "The agent reads the connection from the environment variables listed here. " +
	"An agent that is already running picks them up on its next deployment; use build_agent and deploy_agent, " +
	"or update_deployment_state with redeploy."

func (t *Toolsets) registerAgentConfigTools(server *gomcp.Server, reg *toolRegistry) {
	addTool(reg, server, &gomcp.Tool{
		Name: "attach_llm_provider_to_agent",
		Description: "Give an agent access to an LLM provider through the AI gateway, optionally with guardrails that apply to this agent only. " +
			"The platform creates a per-agent proxy and injects its URL and key into the agent as environment variables. " +
			"Call list_llm_providers for `provider` and list_llm_guardrails for `guardrails`. The provider must be in the catalog (`in_catalog` true) and already deployed to a gateway.",
		InputSchema: createSchema(map[string]any{
			"project_name": stringProperty("Required. Project the agent belongs to."),
			"agent_name":   stringProperty("Required. Name of the agent."),
			"provider":     stringProperty("Required. LLM provider `id` from list_llm_providers, not its name."),
			"config_name":  stringProperty("Optional. Name of the agent's model configuration; also the prefix of the injected variables. Defaults to the provider id."),
			"environments": arrayProperty("Optional. Environment names to configure. Defaults to every environment where the provider is deployed to a gateway this agent can use.", map[string]any{"type": "string"}),
			"guardrails":   arrayProperty("Optional. Guardrails for this agent's LLM traffic, on top of any on the provider.", guardrailSchema()),
		}, []string{"project_name", "agent_name", "provider"}),
	}, audit.ActionAgentUpdate, attachLLMProvider(t.AgentConfigToolset, t.EnvironmentToolset), rbac.AgentUpdate)

	addTool(reg, server, &gomcp.Tool{
		Name: "attach_mcp_proxy_to_agent",
		Description: "Give an agent access to an MCP proxy's tools through the AI gateway. " +
			"The platform injects the proxy URL and a key into the agent as environment variables. " +
			"Call list_mcp_proxies for `proxy`. Every environment you configure is recorded, including ones the proxy is not deployed to yet: " +
			"there the agent gets empty values until the proxy is deployed to that environment, and the platform then binds it. " +
			"get_mcp_proxy shows where the proxy is deployed.",
		InputSchema: createSchema(map[string]any{
			"project_name": stringProperty("Required. Project the agent belongs to."),
			"agent_name":   stringProperty("Required. Name of the agent."),
			"proxy":        stringProperty("Required. MCP proxy id from list_mcp_proxies."),
			"config_name":  stringProperty("Optional. Name of the agent's MCP configuration; also the prefix of the injected variables. Defaults to the proxy id."),
			"environments": arrayProperty("Optional. Environment names to configure. Defaults to every environment, including ones the proxy is not deployed to yet.", map[string]any{"type": "string"}),
		}, []string{"project_name", "agent_name", "proxy"}),
	}, audit.ActionAgentUpdate, attachMCPProxy(t.AgentConfigToolset, t.EnvironmentToolset), rbac.AgentUpdate)
}

func attachLLMProvider(handler AgentConfigToolsetHandler, envHandler EnvironmentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, attachLLMProviderInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input attachLLMProviderInput) (*gomcp.CallToolResult, any, error) {
		const toolName = "attach_llm_provider_to_agent"
		policies, err := toModelGuardrails(input.Guardrails)
		if err != nil {
			return nil, nil, err
		}
		out, err := attachConfig(ctx, toolName, handler, envHandler, models.AgentConfigTypeLLM,
			input.ProjectName, input.AgentName, input.Provider, "provider", input.ConfigName, input.Environments, policies)
		if err != nil {
			return nil, nil, err
		}
		for _, p := range policies {
			out.Guardrails = append(out.Guardrails, p.Name)
		}
		return handleToolResult(out, nil)
	}
}

func attachMCPProxy(handler AgentConfigToolsetHandler, envHandler EnvironmentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, attachMCPProxyInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input attachMCPProxyInput) (*gomcp.CallToolResult, any, error) {
		out, err := attachConfig(ctx, "attach_mcp_proxy_to_agent", handler, envHandler, models.AgentConfigTypeMCP,
			input.ProjectName, input.AgentName, input.Proxy, "proxy", input.ConfigName, input.Environments, nil)
		if err != nil {
			return nil, nil, err
		}
		return handleToolResult(out, nil)
	}
}

// attachConfig creates an agent model or MCP configuration that points every
// selected environment at the same provider or proxy, exactly as the REST
// create does after converting its request.
func attachConfig(
	ctx context.Context, toolName string,
	handler AgentConfigToolsetHandler, envHandler EnvironmentToolsetHandler,
	configType, projectName, agentName, target, targetArg, configName string,
	environments []string, policies []models.LLMPolicy,
) (*attachConfigOutput, error) {
	projectName, agentName, target = strings.TrimSpace(projectName), strings.TrimSpace(agentName), strings.TrimSpace(target)
	if projectName == "" {
		return nil, fmt.Errorf("project_name is required")
	}
	if agentName == "" {
		return nil, fmt.Errorf("agent_name is required")
	}
	if target == "" {
		return nil, fmt.Errorf("%s is required", targetArg)
	}
	configName = strings.TrimSpace(configName)
	if configName == "" {
		configName = target
	}
	if err := utils.ValidateConfigName(configName); err != nil {
		return nil, fmt.Errorf("%s: invalid config_name: %w", toolName, err)
	}

	ouID := resolveOUID(ctx)
	var envNames []string
	var err error
	if configType == models.AgentConfigTypeLLM && !hasEnvironmentNames(environments) {
		// Create refuses the whole config on any environment where the provider is not placed, so
		// "every environment" would fail for a provider deployed to only some of them.
		envNames, err = defaultProviderEnvironments(ctx, toolName, handler, ouID, projectName, agentName, target)
	} else {
		envNames, err = resolveEnvironmentNames(ctx, toolName, envHandler, ouID, environments)
	}
	if err != nil {
		return nil, err
	}

	mappings := make(map[string]models.EnvModelConfigRequest, len(envNames))
	for _, env := range envNames {
		mappings[env] = models.EnvModelConfigRequest{
			ProviderName:  target,
			Configuration: models.EnvProviderConfiguration{Policies: policies},
		}
	}
	req := models.CreateAgentModelConfigRequest{Name: configName, Type: configType, EnvMappings: mappings}

	resp, err := handler.CreateAgentConfig(ctx, ouID, projectName, agentName, req)
	if err != nil {
		return nil, wrapToolError(toolName, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("%s: configuration service returned an empty response", toolName)
	}

	out := &attachConfigOutput{
		ConfigID:             resp.UUID,
		ConfigName:           resp.Name,
		ProjectName:          projectName,
		AgentName:            agentName,
		Environments:         envNames,
		EnvironmentVariables: []envVarItem{},
		Note:                 attachNote,
	}
	// Only the variable names go back to the model: the response can carry a
	// freshly issued key for external agents, and that must not land in a chat.
	for _, v := range resp.EnvironmentVariables {
		out.EnvironmentVariables = append(out.EnvironmentVariables, envVarItem{Name: v.Name, Key: v.Key})
	}
	return out, nil
}

func hasEnvironmentNames(names []string) bool {
	for _, n := range names {
		if strings.TrimSpace(n) != "" {
			return true
		}
	}
	return false
}

// defaultProviderEnvironments is every environment where the agent can use the LLM provider,
// which is what the console offers when it lists providers per environment.
func defaultProviderEnvironments(ctx context.Context, toolName string, handler AgentConfigToolsetHandler, ouID, projectName, agentName, provider string) ([]string, error) {
	names, err := handler.ProviderEnvironments(ctx, ouID, projectName, agentName, provider)
	if err != nil {
		return nil, wrapToolError(toolName, err)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%s: LLM provider %q is not deployed to a gateway agent %q can use in any environment. "+
			"Deploy it from the LLM provider's Deployment tab, then retry; get_llm_provider shows where it is deployed", toolName, provider, agentName)
	}
	return names, nil
}

// resolveEnvironmentNames returns the given environment names, trimmed, or
// every environment in the org when none are given.
func resolveEnvironmentNames(ctx context.Context, toolName string, handler EnvironmentToolsetHandler, ouID string, names []string) ([]string, error) {
	out := make([]string, 0, len(names))
	for _, n := range names {
		// Create rejects an unknown name before writing anything, so names are not looked up here.
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	if handler == nil {
		return nil, fmt.Errorf("%s: environment lookup is not available", toolName)
	}
	// Read every page: attaching to only the first page would leave the
	// remaining environments unconfigured without saying so.
	for offset := int32(utils.DefaultOffset); ; {
		result, err := handler.ListEnvironments(ctx, ouID, int32(utils.MaxLimit), offset)
		if err != nil {
			return nil, wrapToolError(toolName, err)
		}
		if result == nil || len(result.Environments) == 0 {
			break
		}
		for _, env := range result.Environments {
			out = append(out, env.Name)
		}
		offset += int32(len(result.Environments))
		if offset >= result.Total {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no environments are configured for your organization", toolName)
	}
	return out, nil
}
