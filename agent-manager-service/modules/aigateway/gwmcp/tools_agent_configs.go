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
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

type attachLLMProviderInput struct {
	ProjectName  string           `json:"project_name"`
	AgentName    string           `json:"agent_name"`
	ProviderID   string           `json:"provider_id"`
	ConfigName   string           `json:"config_name,omitempty"`
	Environments []string         `json:"environments,omitempty"`
	Guardrails   []guardrailInput `json:"guardrails,omitempty"`
}
type attachMCPProxyInput struct {
	ProjectName  string   `json:"project_name"`
	AgentName    string   `json:"agent_name"`
	ProxyID      string   `json:"proxy_id"`
	ConfigName   string   `json:"config_name,omitempty"`
	Environments []string `json:"environments,omitempty"`
}

type envVarItem struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// attachProxyItem names the per-agent proxy an LLM attach created in one
// environment; its key is managed through the configuration in that environment.
type attachProxyItem struct {
	Environment string `json:"environment"`
	ProxyID     string `json:"proxy_id"`
}
type attachConfigOutput struct {
	ConfigID     string   `json:"config_id"`
	ConfigName   string   `json:"config_name"`
	ProjectName  string   `json:"project_name"`
	AgentName    string   `json:"agent_name"`
	Environments []string `json:"environments"`
	// SkippedProductionEnvironments are production environments a default
	// selection left out because the token cannot act on production.
	SkippedProductionEnvironments []string          `json:"skipped_production_environments,omitempty"`
	Proxies                       []attachProxyItem `json:"proxies,omitempty"`
	// UnrevokedKeyEnvironments are environments where an external agent's
	// issued key could not be revoked and must be regenerated in the console.
	UnrevokedKeyEnvironments []string     `json:"unrevoked_key_environments,omitempty"`
	Guardrails               []string     `json:"guardrails,omitempty"`
	EnvironmentVariables     []envVarItem `json:"environment_variables"`
	Note                     string       `json:"note"`
}

const attachNote = "The agent reads the connection from the environment variables listed here. " +
	"Set them where the agent runs and restart it to pick up the change."

// externalAttachNote is the note for an agent that runs outside the platform:
// nothing is injected or deployed, and the key the create issued was revoked
// rather than returned, so it says where in the console the user gets the URL
// and a key. target is the console panel's subject ("LLM Provider" or "MCP
// Server"); unrevoked are environments whose issued key could not be revoked.
func externalAttachNote(target, keyHint string, unrevoked []string) string {
	note := "This is an external agent: nothing is injected into it and there is nothing to build or deploy. " +
		"The API key the create issued was revoked rather than returned, so no key passes through this chat or is left unused. " +
		"Open this configuration on the agent in the console: \"Connect to " + target + "\" shows each environment's proxy URL, and " +
		keyHint + " generates a key, shown once there."
	if len(unrevoked) > 0 {
		note += " The issued key could not be revoked in " + strings.Join(unrevoked, ", ") + "; regenerate it there instead."
	}
	return note
}

// productionEnvironmentsNote is the environments argument's rule for production,
// the tier the deploy tools apply (requireProductionTier).
const productionEnvironmentsNote = "Production environments need amp:ai-gateway:deploy.production as well: without it a default leaves them out " +
	"(skipped_production_environments lists them), and naming one is refused."

const (
	externalLLMKeyHint = "its API key section"
	externalMCPKeyHint = "when the proxy uses API key security, its API key section"
)

func (t *Tools) registerAgentConfigTools(reg Registrar) {
	addTool(reg, toolDef{
		Name: "attach_llm_provider_to_agent",
		Description: "Give an agent access to an LLM provider through the AI gateway, optionally with guardrails that apply to this agent only. " +
			"The platform creates a per-agent proxy and injects its URL and key into the agent as environment variables. " +
			"Call list_llm_providers for `provider_id` and list_llm_guardrails for `guardrails`. The provider must be in the catalog (`in_catalog` true) and already deployed to a gateway.",
		InputSchema: createSchema(map[string]any{
			"project_name": stringProperty("Required. Project the agent belongs to."),
			"agent_name":   stringProperty("Required. Name of the agent."),
			"provider_id":  stringProperty("Required. LLM provider `id` from list_llm_providers, not its name."),
			"config_name":  stringProperty("Optional. Name of the agent's model configuration; also the prefix of the injected variables. Defaults to the provider id."),
			"environments": arrayProperty("Optional. Environment names to configure. Defaults to every environment where the provider is deployed to a gateway this agent can use. "+
				productionEnvironmentsNote, map[string]any{"type": "string"}),
			"guardrails": arrayProperty("Optional. Guardrails for this agent's LLM traffic, on top of any on the provider.", guardrailSchema()),
		}, []string{"project_name", "agent_name", "provider_id"}),
	}, gwaudit.AgentConfigUpdate, attachLLMProvider(t), rbac.AgentUpdate)

	addTool(reg, toolDef{
		Name: "attach_mcp_proxy_to_agent",
		Description: "Give an agent access to an MCP proxy's tools through the AI gateway. " +
			"The platform injects the proxy URL and a key into the agent as environment variables. " +
			"Call list_mcp_proxies for `proxy_id`. Every environment you configure is recorded, including ones the proxy is not deployed to yet: " +
			"there the agent gets empty values until the proxy is deployed to that environment, and the platform then binds it. " +
			"get_mcp_proxy shows where the proxy is deployed.",
		InputSchema: createSchema(map[string]any{
			"project_name": stringProperty("Required. Project the agent belongs to."),
			"agent_name":   stringProperty("Required. Name of the agent."),
			"proxy_id":     stringProperty("Required. MCP proxy `id` from list_mcp_proxies."),
			"config_name":  stringProperty("Optional. Name of the agent's MCP configuration; also the prefix of the injected variables. Defaults to the proxy id."),
			"environments": arrayProperty("Optional. Environment names to configure. Defaults to every environment, including ones the proxy is not deployed to yet. "+
				productionEnvironmentsNote, map[string]any{"type": "string"}),
		}, []string{"project_name", "agent_name", "proxy_id"}),
	}, gwaudit.AgentConfigUpdate, attachMCPProxy(t), rbac.AgentUpdate)
}

func attachLLMProvider(t *Tools) func(context.Context, attachLLMProviderInput) (any, error) {
	return func(ctx context.Context, input attachLLMProviderInput) (any, error) {
		const toolName = "attach_llm_provider_to_agent"
		policies, err := toModelGuardrails(input.Guardrails)
		if err != nil {
			return nil, err
		}
		// A blank provider_id is reported by attachConfig.
		if providerID := strings.TrimSpace(input.ProviderID); providerID != "" {
			if err := requireAvailableGuardrails(ctx, toolName, t.LLMProviders, modkit.OrgID(ctx), providerID, policies); err != nil {
				return nil, err
			}
		}
		out, err := attachConfig(ctx, toolName, t, gwmodels.AgentConfigTypeLLM,
			input.ProjectName, input.AgentName, input.ProviderID, "provider_id", input.ConfigName, input.Environments, policies)
		if err != nil {
			return nil, err
		}
		for _, p := range policies {
			out.Guardrails = append(out.Guardrails, p.Name)
		}
		return out, nil
	}
}

func attachMCPProxy(t *Tools) func(context.Context, attachMCPProxyInput) (any, error) {
	return func(ctx context.Context, input attachMCPProxyInput) (any, error) {
		out, err := attachConfig(ctx, "attach_mcp_proxy_to_agent", t, gwmodels.AgentConfigTypeMCP,
			input.ProjectName, input.AgentName, input.ProxyID, "proxy_id", input.ConfigName, input.Environments, nil)
		if err != nil {
			return nil, err
		}
		return out, nil
	}
}

// attachConfig creates an agent model or MCP configuration that points every
// selected environment at the same provider or proxy, exactly as the REST
// create does after converting its request.
func attachConfig(
	ctx context.Context, toolName string, t *Tools,
	configType, projectName, agentName, target, targetArg, configName string,
	environments []string, policies []gwmodels.LLMPolicy,
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
	if err := gwutil.ValidateConfigName(configName); err != nil {
		return nil, fmt.Errorf("%s: invalid config_name: %w", toolName, err)
	}

	handler := t.AgentConfigs
	ouID := modkit.OrgID(ctx)
	// Read before the create, so a failed lookup never leaves a config behind
	// with the wrong note.
	external, err := isExternalAgent(ctx, t.Core, ouID, projectName, agentName)
	if err != nil {
		return nil, wrapToolError(toolName, err)
	}
	var envNames []string
	if configType == gwmodels.AgentConfigTypeLLM && !hasEnvironmentNames(environments) {
		// Create refuses the whole config on any environment where the provider is not placed, so
		// "every environment" would fail for a provider deployed to only some of them.
		envNames, err = defaultProviderEnvironments(ctx, toolName, handler, ouID, projectName, agentName, target)
	} else {
		envNames, err = resolveEnvironmentNames(ctx, toolName, t.Core, ouID, environments)
	}
	if err != nil {
		return nil, err
	}
	envNames, skipped, err := requireProductionTier(ctx, toolName, t, ouID, envNames, hasEnvironmentNames(environments))
	if err != nil {
		return nil, err
	}

	mappings := make(map[string]gwmodels.EnvModelConfigRequest, len(envNames))
	for _, env := range envNames {
		mappings[env] = gwmodels.EnvModelConfigRequest{
			ProviderName:  target,
			Configuration: gwmodels.EnvProviderConfiguration{Policies: policies},
		}
	}
	req := gwmodels.CreateAgentModelConfigRequest{Name: configName, Type: configType, EnvMappings: mappings}

	resp, err := handler.CreateAgentConfig(ctx, ouID, projectName, agentName, req)
	if err != nil {
		return nil, wrapToolError(toolName, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("%s: configuration service returned an empty response", toolName)
	}

	out := &attachConfigOutput{
		ConfigID:                      resp.UUID,
		ConfigName:                    resp.Name,
		ProjectName:                   projectName,
		AgentName:                     agentName,
		Environments:                  envNames,
		SkippedProductionEnvironments: skipped,
		EnvironmentVariables:          []envVarItem{},
		Note:                          attachNote,
	}
	if external {
		out.UnrevokedKeyEnvironments = revokeIssuedKeys(ctx, handler, resp, ouID, projectName, agentName, configType)
		if configType == gwmodels.AgentConfigTypeMCP {
			out.Note = externalAttachNote("MCP Server", externalMCPKeyHint, out.UnrevokedKeyEnvironments)
		} else {
			out.Note = externalAttachNote("LLM Provider", externalLLMKeyHint, out.UnrevokedKeyEnvironments)
		}
	}
	if len(skipped) > 0 {
		out.Note += fmt.Sprintf(" Production environments %s were left out: configuring them needs %s.",
			strings.Join(skipped, ", "), gwperm.DeployProduction.Scope())
	}
	if configType == gwmodels.AgentConfigTypeLLM {
		out.Proxies = attachedProxies(resp, envNames)
	}
	// Only the variable names go back to the model: the response can carry a
	// freshly issued key for external agents, and that must not land in a chat.
	for _, v := range resp.EnvironmentVariables {
		out.EnvironmentVariables = append(out.EnvironmentVariables, envVarItem{Name: v.Name, Key: v.Key})
	}
	return out, nil
}

// requireProductionTier applies the deploy tools' environment tier to an attach:
// configuring a production environment needs ai-gateway:deploy.production on
// top of agent:update. Environments the caller named are refused when the token
// lacks it. A default selection leaves the production ones out and returns them
// as skipped, unless nothing would be left.
func requireProductionTier(
	ctx context.Context, toolName string, t *Tools, ouID string,
	envNames []string, named bool,
) ([]string, []string, error) {
	if t.Can != nil && t.Can(ctx, short(gwperm.DeployProduction)) {
		return envNames, nil, nil
	}
	if t.Core == nil {
		return nil, nil, fmt.Errorf("%s: environment lookup is not available", toolName)
	}
	envs, err := t.Core.ListEnvironments(ctx, ouID)
	if err != nil {
		return nil, nil, wrapToolError(toolName, err)
	}
	production := make(map[string]bool, len(envs))
	for _, env := range envs {
		if env.IsProduction {
			production[env.Name] = true
		}
	}
	var allowed, refused []string
	for _, name := range envNames {
		if production[name] {
			refused = append(refused, name)
		} else {
			allowed = append(allowed, name)
		}
	}
	if len(refused) == 0 {
		return envNames, nil, nil
	}
	if !named && len(allowed) > 0 {
		return allowed, refused, nil
	}
	return nil, nil, fmt.Errorf("%s: %s is required to configure production environment %s. "+
		"Pass only non-production environments, or use a token with that scope",
		toolName, gwperm.DeployProduction.Scope(), strings.Join(refused, ", "))
}

// attachedProxies lists the per-agent LLM proxy each environment got, so a
// caller can name the proxy whose key the user manages, without the key.
func attachedProxies(resp *gwmodels.AgentModelConfigResponse, envNames []string) []attachProxyItem {
	proxies := []attachProxyItem{}
	for _, env := range envNames {
		cfg, ok := resp.EnvModelConfig[env]
		if !ok || cfg.LLMProxy == nil || cfg.LLMProxy.ProxyName == nil {
			continue
		}
		proxies = append(proxies, attachProxyItem{Environment: env, ProxyID: *cfg.LLMProxy.ProxyName})
	}
	return proxies
}

// isExternalAgent reports whether the agent runs outside the platform, read
// before the create so a failed lookup never leaves a config behind.
func isExternalAgent(ctx context.Context, agents CoreReader, ouID, projectName, agentName string) (bool, error) {
	if agents == nil {
		return false, nil
	}
	agent, err := agents.GetAgentByName(ctx, ouID, projectName, agentName)
	if err != nil {
		return false, err
	}
	return agent.Provisioning == string(gwutil.ExternalAgent), nil
}

// revokeIssuedKeys revokes the key the create issued in each of an external
// agent's environments, since the tool withholds it and nobody else holds it.
// It returns the environments where that failed, for the user to regenerate.
func revokeIssuedKeys(
	ctx context.Context, handler AgentConfigToolsetHandler, resp *gwmodels.AgentModelConfigResponse,
	ouID, projectName, agentName, configType string,
) []string {
	envs := make([]string, 0, len(resp.EnvModelConfig))
	for env, cfg := range resp.EnvModelConfig {
		if cfg.LLMProxy != nil {
			envs = append(envs, env)
		}
	}
	sort.Strings(envs)
	var failed []string
	for _, env := range envs {
		if err := handler.RevokeConfigAPIKeys(ctx, ouID, projectName, agentName, configType, resp.UUID, env); err != nil {
			slog.WarnContext(ctx, "could not revoke an external agent's withheld config API key",
				"configID", resp.UUID, "environment", env, "error", err)
			failed = append(failed, env)
		}
	}
	return failed
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
func resolveEnvironmentNames(ctx context.Context, toolName string, core CoreReader, ouID string, names []string) ([]string, error) {
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
	if core == nil {
		return nil, fmt.Errorf("%s: environment lookup is not available", toolName)
	}
	envs, err := core.ListEnvironments(ctx, ouID)
	if err != nil {
		return nil, wrapToolError(toolName, err)
	}
	for _, env := range envs {
		out = append(out, env.Name)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: no environments are configured for your organization", toolName)
	}
	return out, nil
}
