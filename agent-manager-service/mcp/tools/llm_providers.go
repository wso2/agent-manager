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
	"errors"
	"fmt"
	"sort"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type listLLMProvidersInput struct {
	Limit  *int `json:"limit,omitempty"`
	Offset *int `json:"offset,omitempty"`
}

type addLLMProviderToCatalogInput struct {
	ProviderID string `json:"provider_id"`
}

type attachLLMProviderInput struct {
	ProjectName  string   `json:"project_name"`
	AgentName    string   `json:"agent_name"`
	ProviderID   string   `json:"provider_id"`
	Environments []string `json:"environments,omitempty"`
	ConfigName   *string  `json:"config_name,omitempty"`
	Description  *string  `json:"description,omitempty"`
	URLEnvVar    *string  `json:"url_env_var,omitempty"`
	APIKeyEnvVar *string  `json:"api_key_env_var,omitempty"`
}

type llmProviderItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Template  string `json:"template"`
	Status    string `json:"status"`
	InCatalog bool   `json:"in_catalog"`
}

type listLLMProvidersOutput struct {
	Total     int               `json:"total"`
	Providers []llmProviderItem `json:"providers"`
}

type addLLMProviderToCatalogOutput struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	InCatalog bool   `json:"in_catalog"`
	Changed   bool   `json:"changed"`
}

type attachedEnvironment struct {
	Name         string `json:"name"`
	ProxyURL     string `json:"proxy_url,omitempty"`
	APIKeyIssued bool   `json:"api_key_issued"`
}

type attachedEnvVar struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type attachLLMProviderOutput struct {
	ConfigID             string                `json:"config_id"`
	ConfigName           string                `json:"config_name"`
	ProjectName          string                `json:"project_name"`
	AgentName            string                `json:"agent_name"`
	ProviderID           string                `json:"provider_id"`
	Environments         []attachedEnvironment `json:"environments"`
	EnvironmentVariables []attachedEnvVar      `json:"environment_variables"`
	Note                 string                `json:"note,omitempty"`
}

const apiKeyNotReturnedNote = "The platform issued an LLM proxy API key for each environment marked api_key_issued. " +
	"This tool does not return API key values. To get a value, rotate the key of this model configuration in the Agent Platform console; rotation also replaces the issued key."

func boolPtr(v bool) *bool { return &v }

func (t *Toolsets) registerLLMProviderTools(server *gomcp.Server, reg *toolRegistry) {
	addTool(reg, server, &gomcp.Tool{
		Name:  "list_llm_providers",
		Title: "List LLM providers",
		Description: "List the LLM service providers in your organization (resolved from the caller's token). " +
			"An LLM provider is an organization-level connection to a model vendor, served through an AI gateway. " +
			"Each item shows its id, template, status, and whether it is in the organization's catalog. " +
			"Use the id as provider_id for add_llm_provider_to_catalog and attach_llm_provider.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
		InputSchema: createSchema(map[string]any{
			"limit":  intProperty(fmt.Sprintf("Optional. Max providers to return (default %d, min %d, max %d).", utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)),
			"offset": intProperty(fmt.Sprintf("Optional. Pagination offset (default %d, min %d).", utils.DefaultOffset, utils.MinOffset)),
		}, nil),
	}, audit.ActionLLMProviderRead, listLLMProviders(t.LLMProviderToolset), rbac.LLMProviderRead)

	addTool(reg, server, &gomcp.Tool{
		Name:  "add_llm_provider_to_catalog",
		Title: "Add LLM provider to catalog",
		Description: "Add an LLM provider to your organization's catalog. " +
			"Only providers in the catalog can be attached to agents. New providers are in the catalog by default, " +
			"so this is necessary only for a provider that was removed from it. Calling it again has no effect.",
		Annotations: &gomcp.ToolAnnotations{
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
			OpenWorldHint:   boolPtr(false),
		},
		InputSchema: createSchema(map[string]any{
			"provider_id": stringProperty("Required. LLM provider id (handle) or UUID. Use list_llm_providers to discover valid ids."),
		}, []string{"provider_id"}),
	}, audit.ActionLLMProviderUpdateCatalog, addLLMProviderToCatalog(t.LLMProviderToolset), rbac.LLMProviderUpdate)

	// needs the environment toolset to resolve the default environment
	if t.EnvironmentToolset != nil {
		addTool(reg, server, &gomcp.Tool{
			Name:  "attach_llm_provider",
			Title: "Attach LLM provider to agent",
			Description: "Attach an LLM provider from the catalog to an agent as a new model configuration. " +
				"The platform creates an LLM proxy for the agent in each environment and gives the agent the proxy URL and API key " +
				"through environment variables. The provider must be in the catalog and deployed to a gateway in each environment. " +
				"API key values are never returned.",
			Annotations: &gomcp.ToolAnnotations{
				DestructiveHint: boolPtr(false),
				OpenWorldHint:   boolPtr(false),
			},
			InputSchema: createSchema(map[string]any{
				"project_name": stringProperty("Required. Project name where the agent exists."),
				"agent_name":   stringProperty("Required. Name of the agent to attach the provider to."),
				"provider_id":  stringProperty("Required. LLM provider id (handle) from list_llm_providers."),
				"environments": arrayProperty("Optional. Environment names to attach the provider in. If omitted, the organization's only environment is used; required when more than one environment is configured. Use list_environments to discover valid names.",
					map[string]any{"type": "string"}),
				"config_name":     stringProperty("Optional. Name of the agent's model configuration (letters, digits, spaces, hyphens and underscores; max 255). Defaults to provider_id. Must be unique for the agent."),
				"description":     stringProperty("Optional. Description of the model configuration."),
				"url_env_var":     stringProperty("Optional. Name of the environment variable that gives the agent the LLM proxy URL. Defaults to <CONFIG_NAME>_URL."),
				"api_key_env_var": stringProperty("Optional. Name of the environment variable that gives the agent the LLM proxy API key. Defaults to <CONFIG_NAME>_API_KEY."),
			}, []string{"project_name", "agent_name", "provider_id"}),
		}, audit.ActionAgentUpdate, attachLLMProvider(t.LLMProviderToolset, t.EnvironmentToolset), rbac.AgentUpdate)
	}
}

func listLLMProviders(handler LLMProviderToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listLLMProvidersInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listLLMProvidersInput) (*gomcp.CallToolResult, any, error) {
		ouID := resolveOUID(ctx)

		limit, offset, err := resolvePagination(input.Limit, input.Offset)
		if err != nil {
			return nil, nil, err
		}

		providers, total, err := handler.ListLLMProviders(ctx, ouID, limit, offset)
		if err != nil {
			return nil, nil, wrapToolError("list_llm_providers", err)
		}

		items := make([]llmProviderItem, 0, len(providers))
		for _, p := range providers {
			if p == nil {
				continue
			}
			items = append(items, toLLMProviderItem(p))
		}
		return handleToolResult(listLLMProvidersOutput{Total: total, Providers: items}, nil)
	}
}

func addLLMProviderToCatalog(handler LLMProviderToolsetHandler) func(context.Context, *gomcp.CallToolRequest, addLLMProviderToCatalogInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input addLLMProviderToCatalogInput) (*gomcp.CallToolResult, any, error) {
		providerID := strings.TrimSpace(input.ProviderID)
		if providerID == "" {
			return nil, nil, fmt.Errorf("provider_id is required")
		}

		ouID := resolveOUID(ctx)

		provider, changed, err := handler.AddLLMProviderToCatalog(ctx, ouID, providerID)
		if err != nil {
			return nil, nil, wrapToolError("add_llm_provider_to_catalog", err)
		}
		if provider == nil {
			return nil, nil, fmt.Errorf("add_llm_provider_to_catalog: LLM provider service returned an empty response")
		}
		item := toLLMProviderItem(provider)
		return handleToolResult(addLLMProviderToCatalogOutput{
			ID:        item.ID,
			Name:      item.Name,
			InCatalog: provider.InCatalog,
			Changed:   changed,
		}, nil)
	}
}

func attachLLMProvider(handler LLMProviderToolsetHandler, envHandler EnvironmentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, attachLLMProviderInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input attachLLMProviderInput) (*gomcp.CallToolResult, any, error) {
		const toolName = "attach_llm_provider"
		projectName := strings.TrimSpace(input.ProjectName)
		agentName := strings.TrimSpace(input.AgentName)
		providerID := strings.TrimSpace(input.ProviderID)
		if projectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if agentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		if providerID == "" {
			return nil, nil, fmt.Errorf("provider_id is required")
		}

		configName := providerID
		if name := normalizeOptionalString(input.ConfigName); name != nil {
			configName = *name
		}
		if err := utils.ValidateConfigName(configName); err != nil {
			return nil, nil, fmt.Errorf("config_name: %w", err)
		}

		ouID := resolveOUID(ctx)

		envNames, err := resolveAttachEnvironments(ctx, toolName, envHandler, ouID, input.Environments)
		if err != nil {
			return nil, nil, err
		}

		envMappings := make(map[string]models.EnvModelConfigRequest, len(envNames))
		for _, name := range envNames {
			envMappings[name] = models.EnvModelConfigRequest{ProviderName: providerID}
		}

		var envVars []models.EnvironmentVariableConfig
		if name := normalizeOptionalString(input.URLEnvVar); name != nil {
			envVars = append(envVars, models.EnvironmentVariableConfig{Key: "url", Name: *name})
		}
		if name := normalizeOptionalString(input.APIKeyEnvVar); name != nil {
			envVars = append(envVars, models.EnvironmentVariableConfig{Key: "apikey", Name: *name})
		}

		resp, err := handler.AttachLLMProvider(ctx, ouID, projectName, agentName, models.CreateAgentModelConfigRequest{
			Name:                 configName,
			Description:          utils.StrPointerAsStr(normalizeOptionalString(input.Description), ""),
			Type:                 models.AgentConfigTypeLLM,
			EnvMappings:          envMappings,
			EnvironmentVariables: envVars,
		})
		if err != nil {
			if errors.Is(err, utils.ErrInvalidInput) && strings.Contains(err.Error(), "must be in catalog") {
				return nil, nil, fmt.Errorf("%s: the LLM provider is not in the catalog. Call add_llm_provider_to_catalog first", toolName)
			}
			return nil, nil, wrapToolError(toolName, err)
		}
		if resp == nil {
			return nil, nil, fmt.Errorf("%s: agent configuration service returned an empty response", toolName)
		}
		return handleToolResult(toAttachLLMProviderOutput(resp, projectName, agentName, providerID, envNames), nil)
	}
}

// resolveAttachEnvironments dedupes explicit environment names, or resolves
// the org's only environment when none are given. The service validates the
// names before it writes anything.
func resolveAttachEnvironments(ctx context.Context, toolName string, envHandler EnvironmentToolsetHandler, ouID string, requested []string) ([]string, error) {
	seen := make(map[string]struct{}, len(requested))
	names := make([]string, 0, len(requested))
	for _, raw := range requested {
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, fmt.Errorf("environments must not contain empty names")
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	if len(names) > 0 {
		sort.Strings(names)
		return names, nil
	}
	name, err := resolveTokenEnvironment(ctx, toolName, envHandler, ouID, "environments", "")
	if err != nil {
		return nil, err
	}
	return []string{name}, nil
}

// toLLMProviderItem copies only identity and state; the provider's upstream
// configuration holds credentials and is never part of a tool result.
func toLLMProviderItem(p *models.LLMProvider) llmProviderItem {
	item := llmProviderItem{
		Template:  p.TemplateHandle,
		Status:    p.Status,
		InCatalog: p.InCatalog,
	}
	if p.Artifact != nil {
		item.ID = p.Artifact.Handle
		item.Name = p.Artifact.Name
	}
	return item
}

func toAttachLLMProviderOutput(resp *models.AgentModelConfigResponse, projectName, agentName, providerID string, envNames []string) attachLLMProviderOutput {
	out := attachLLMProviderOutput{
		ConfigID:             resp.UUID,
		ConfigName:           resp.Name,
		ProjectName:          projectName,
		AgentName:            agentName,
		ProviderID:           providerID,
		Environments:         make([]attachedEnvironment, 0, len(envNames)),
		EnvironmentVariables: make([]attachedEnvVar, 0, len(resp.EnvironmentVariables)),
	}
	// The service creates a proxy API key for every proxy. It returns the value
	// only for an external agent; a platform agent gets the key injected.
	keyWithheld := false
	for _, name := range envNames {
		env := attachedEnvironment{Name: name}
		if cfg, ok := resp.EnvModelConfig[name]; ok && cfg.LLMProxy != nil {
			env.APIKeyIssued = true
			if cfg.LLMProxy.URL != nil {
				env.ProxyURL = *cfg.LLMProxy.URL
			}
			keyWithheld = keyWithheld || (cfg.LLMProxy.APIKey != nil && *cfg.LLMProxy.APIKey != "")
		}
		out.Environments = append(out.Environments, env)
	}
	for _, ev := range resp.EnvironmentVariables {
		out.EnvironmentVariables = append(out.EnvironmentVariables, attachedEnvVar{Key: ev.Key, Name: ev.Name})
	}
	if keyWithheld {
		out.Note = apiKeyNotReturnedNote
	}
	return out
}
