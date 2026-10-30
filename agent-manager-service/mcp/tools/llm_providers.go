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
	"regexp"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	defaultLLMProviderVersion = "v1.0"
	// guardrailPathAll applies a guardrail to every route and method, as the
	// console does when a guardrail is attached to a provider or an agent.
	guardrailPathAll = "/*"
)

var (
	llmProviderVersionPattern = regexp.MustCompile(`^v\d+\.\d+$`)
	nonHandleChars            = regexp.MustCompile(`[^a-z0-9]+`)
)

// input structs
type listLLMProviderTemplatesInput struct {
	Limit  *int `json:"limit,omitempty"`
	Offset *int `json:"offset,omitempty"`
}
type listLLMProvidersInput struct {
	Limit  *int `json:"limit,omitempty"`
	Offset *int `json:"offset,omitempty"`
}
type getLLMProviderInput struct {
	ProviderID string `json:"provider_id"`
}
type addLLMProviderGuardrailsInput struct {
	ProviderID string           `json:"provider_id"`
	Guardrails []guardrailInput `json:"guardrails"`
}
type removeLLMProviderGuardrailInput struct {
	ProviderID string `json:"provider_id"`
	Name       string `json:"name"`
}
type listLLMGuardrailsInput struct {
	ProviderID string `json:"provider_id,omitempty"`
}
type guardrailInput struct {
	Name    string         `json:"name"`
	Version string         `json:"version"`
	Params  map[string]any `json:"params,omitempty"`
}
type createLLMProviderInput struct {
	Name        string           `json:"name"`
	Template    string           `json:"template"`
	APIKey      string           `json:"api_key,omitempty"`
	UpstreamURL string           `json:"upstream_url,omitempty"`
	Description string           `json:"description,omitempty"`
	Version     string           `json:"version,omitempty"`
	GatewayIDs  []string         `json:"gateway_ids,omitempty"`
	Guardrails  []guardrailInput `json:"guardrails,omitempty"`
}

// output structs
type llmProviderTemplateItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	EndpointURL string `json:"endpoint_url,omitempty"`
	AuthType    string `json:"auth_type,omitempty"`
}
type listLLMProviderTemplatesOutput struct {
	Total     int                       `json:"total"`
	Templates []llmProviderTemplateItem `json:"templates"`
}
type llmProviderItem struct {
	ID       string `json:"id"`
	UUID     string `json:"uuid"`
	Name     string `json:"name"`
	Template string `json:"template"`
	Status   string `json:"status"`
}
type listLLMProvidersOutput struct {
	Total     int               `json:"total"`
	Providers []llmProviderItem `json:"providers"`
}
type guardrailItem struct {
	Name        string         `json:"name"`
	Version     string         `json:"version"`
	DisplayName string         `json:"display_name,omitempty"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}
type listLLMGuardrailsOutput struct {
	Total      int             `json:"total"`
	Guardrails []guardrailItem `json:"guardrails"`
}
type providerDeploymentItem struct {
	GatewayID string `json:"gateway_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}
type providerGuardrailItem struct {
	Name    string         `json:"name"`
	Version string         `json:"version"`
	Params  map[string]any `json:"params,omitempty"`
}
type providerGatewayDeploymentItem struct {
	GatewayID string `json:"gateway_id"`
	Status    string `json:"status,omitempty"`
}
type getLLMProviderOutput struct {
	llmProviderItem
	Description    string                          `json:"description,omitempty"`
	Version        string                          `json:"version,omitempty"`
	UpstreamURL    string                          `json:"upstream_url,omitempty"`
	UpstreamAuth   bool                            `json:"upstream_auth_configured"`
	Guardrails     []providerGuardrailItem         `json:"guardrails"`
	Deployments    []providerGatewayDeploymentItem `json:"deployments"`
	ModelProviders []string                        `json:"model_providers,omitempty"`
}
type setGuardrailsOutput struct {
	ProviderID  string                   `json:"provider_id"`
	Guardrails  []providerGuardrailItem  `json:"guardrails"`
	Deployments []providerDeploymentItem `json:"redeployments"`
	Note        string                   `json:"note"`
}
type createLLMProviderOutput struct {
	llmProviderItem
	Guardrails  []string                 `json:"guardrails,omitempty"`
	Deployments []providerDeploymentItem `json:"deployments,omitempty"`
	Note        string                   `json:"note"`
}

func guardrailSchema() map[string]any {
	return createSchema(map[string]any{
		"name":    stringProperty("Required. Guardrail policy name from list_llm_guardrails."),
		"version": stringProperty("Required. Guardrail policy version from list_llm_guardrails."),
		"params": map[string]any{
			"type":        "object",
			"description": "Optional. Policy parameters, shaped by the `parameters` schema list_llm_guardrails returns for this policy.",
		},
	}, []string{"name", "version"})
}

func (t *Toolsets) registerLLMProviderTools(server *gomcp.Server, reg *toolRegistry) {
	addTool(reg, server, &gomcp.Tool{
		Name: "list_llm_provider_templates",
		Description: "List the LLM provider templates available in your organization, such as openai, anthropic or azure-openai. " +
			"A template supplies the provider type, its default upstream URL and how it authenticates. " +
			"Use the template `id` as the `template` argument of create_llm_provider.",
		InputSchema: createSchema(map[string]any{
			"limit":  intProperty(fmt.Sprintf("Optional. Max templates to return (default %d, min %d, max %d).", utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)),
			"offset": intProperty(fmt.Sprintf("Optional. Pagination offset (default %d, min %d).", utils.DefaultOffset, utils.MinOffset)),
		}, nil),
	}, audit.ActionLLMProviderTemplateRead, listLLMProviderTemplates(t.LLMProviderToolset), rbac.LLMProviderTemplateRead)

	addTool(reg, server, &gomcp.Tool{
		Name: "list_llm_providers",
		Description: "List the LLM providers configured in your organization. " +
			"An LLM provider is an org-level connection to a model vendor (for example OpenAI) that agents reach through the AI gateway. " +
			"Use a provider's `id` as the `provider` argument of attach_llm_provider_to_agent.",
		InputSchema: createSchema(map[string]any{
			"limit":  intProperty(fmt.Sprintf("Optional. Max providers to return (default %d, min %d, max %d).", utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)),
			"offset": intProperty(fmt.Sprintf("Optional. Pagination offset (default %d, min %d).", utils.DefaultOffset, utils.MinOffset)),
		}, nil),
	}, audit.ActionLLMProviderRead, listLLMProviders(t.LLMProviderToolset), rbac.LLMProviderRead)

	addTool(reg, server, &gomcp.Tool{
		Name: "get_llm_provider",
		Description: "Get one LLM provider's details: its template, upstream URL, the guardrails enforced on it, " +
			"and the gateways it is deployed to with each deployment's status. " +
			"Use it to check a provider is deployed before attach_llm_provider_to_agent. Credentials are never returned.",
		InputSchema: createSchema(map[string]any{
			"provider_id": stringProperty("Required. LLM provider id from list_llm_providers."),
		}, []string{"provider_id"}),
	}, audit.ActionLLMProviderRead, getLLMProvider(t.LLMProviderToolset), rbac.LLMProviderRead)

	addTool(reg, server, &gomcp.Tool{
		Name: "add_llm_provider_guardrails",
		Description: "Add guardrails to an existing LLM provider; they apply to every agent that uses it. " +
			"A guardrail with the same name as one already on the provider replaces it, so this also changes a guardrail's settings. " +
			"The provider is redeployed to the gateways it is already on so the change takes effect. " +
			"Call list_llm_guardrails for valid guardrails and get_llm_provider to see the current ones.",
		InputSchema: createSchema(map[string]any{
			"provider_id": stringProperty("Required. LLM provider id from list_llm_providers."),
			"guardrails":  arrayProperty("Required. Guardrails to add or replace.", guardrailSchema()),
		}, []string{"provider_id", "guardrails"}),
	}, audit.ActionLLMProviderUpdate, addLLMProviderGuardrails(t.LLMProviderToolset), rbac.LLMProviderUpdate)

	addTool(reg, server, &gomcp.Tool{
		Name: "remove_llm_provider_guardrail",
		Description: "Remove one guardrail from an LLM provider by name. " +
			"The provider is redeployed to the gateways it is already on so the change takes effect. " +
			"Call get_llm_provider to see the provider's current guardrails.",
		InputSchema: createSchema(map[string]any{
			"provider_id": stringProperty("Required. LLM provider id from list_llm_providers."),
			"name":        stringProperty("Required. Name of the guardrail to remove, as get_llm_provider lists it."),
		}, []string{"provider_id", "name"}),
	}, audit.ActionLLMProviderUpdate, removeLLMProviderGuardrail(t.LLMProviderToolset), rbac.LLMProviderUpdate)

	addTool(reg, server, &gomcp.Tool{
		Name: "list_llm_guardrails",
		Description: "List the guardrail policies the organization's gateways can enforce on LLM traffic, such as PII masking, content-length or word-count checks, " +
			"with each policy's parameter schema. Use the results as `guardrails` in create_llm_provider (applies to every agent using the provider) " +
			"or attach_llm_provider_to_agent (applies to one agent).",
		InputSchema: createSchema(map[string]any{
			"provider_id": stringProperty("Optional. LLM provider id; limits the result to guardrails on the gateways that provider is deployed to."),
		}, nil),
	}, audit.ActionLLMProviderRead, listLLMGuardrails(t.LLMProviderToolset), rbac.LLMProviderRead)

	addTool(reg, server, &gomcp.Tool{
		Name: "create_llm_provider",
		Description: "Create an org-level LLM provider from a template, optionally with guardrails, and optionally deploy it to gateways in the same call. " +
			"Call list_llm_provider_templates for `template`, list_gateways for `gateway_ids` and list_llm_guardrails for `guardrails`. " +
			"Agents can only use a provider that is deployed to a gateway. The api_key is stored by the gateway and never returned.",
		InputSchema: createSchema(map[string]any{
			"name":         stringProperty("Required. Display name of the provider. Its id is derived from it (lowercase, dashes)."),
			"template":     stringProperty("Required. Template id from list_llm_provider_templates, e.g. openai."),
			"api_key":      stringProperty("Optional. The vendor API key the gateway uses to call the upstream. Required unless the template needs no auth."),
			"upstream_url": stringProperty("Optional. Upstream base URL. Defaults to the template's endpoint URL."),
			"description":  stringProperty("Optional. Description of the provider."),
			"version":      stringProperty(fmt.Sprintf("Optional. Provider version in the form vMAJOR.MINOR (default %s).", defaultLLMProviderVersion)),
			"gateway_ids": arrayProperty("Optional. Gateway ids from list_gateways to deploy the provider to. Omit to create it without deploying.",
				map[string]any{"type": "string"}),
			"guardrails": arrayProperty("Optional. Guardrails enforced on every request through this provider.", guardrailSchema()),
		}, []string{"name", "template"}),
	}, audit.ActionLLMProviderCreate, createLLMProvider(t.LLMProviderToolset), rbac.LLMProviderCreate)
}

func listLLMProviderTemplates(handler LLMProviderToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listLLMProviderTemplatesInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listLLMProviderTemplatesInput) (*gomcp.CallToolResult, any, error) {
		limit, offset, err := resolvePagination(input.Limit, input.Offset)
		if err != nil {
			return nil, nil, err
		}
		templates, total, err := handler.ListLLMProviderTemplates(ctx, resolveOUID(ctx), limit, offset)
		if err != nil {
			return nil, nil, wrapToolError("list_llm_provider_templates", err)
		}
		items := make([]llmProviderTemplateItem, 0, len(templates))
		for _, tpl := range templates {
			if tpl == nil {
				continue
			}
			item := llmProviderTemplateItem{ID: tpl.Handle, Name: tpl.Name, Description: tpl.Description}
			if tpl.Metadata != nil {
				item.EndpointURL = tpl.Metadata.EndpointURL
				if tpl.Metadata.Auth != nil {
					item.AuthType = tpl.Metadata.Auth.Type
				}
			}
			items = append(items, item)
		}
		return handleToolResult(listLLMProviderTemplatesOutput{Total: total, Templates: items}, nil)
	}
}

func listLLMProviders(handler LLMProviderToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listLLMProvidersInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listLLMProvidersInput) (*gomcp.CallToolResult, any, error) {
		limit, offset, err := resolvePagination(input.Limit, input.Offset)
		if err != nil {
			return nil, nil, err
		}
		providers, total, err := handler.ListLLMProviders(ctx, resolveOUID(ctx), limit, offset)
		if err != nil {
			return nil, nil, wrapToolError("list_llm_providers", err)
		}
		items := make([]llmProviderItem, 0, len(providers))
		for _, p := range providers {
			if p != nil {
				items = append(items, toLLMProviderItem(p))
			}
		}
		return handleToolResult(listLLMProvidersOutput{Total: total, Providers: items}, nil)
	}
}

func getLLMProvider(handler LLMProviderToolsetHandler) func(context.Context, *gomcp.CallToolRequest, getLLMProviderInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getLLMProviderInput) (*gomcp.CallToolResult, any, error) {
		providerID := strings.TrimSpace(input.ProviderID)
		if providerID == "" {
			return nil, nil, fmt.Errorf("provider_id is required. Call list_llm_providers to see valid providers")
		}
		provider, deployments, err := handler.GetLLMProvider(ctx, resolveOUID(ctx), providerID)
		if err != nil {
			return nil, nil, wrapToolError("get_llm_provider", err)
		}
		if provider == nil {
			return nil, nil, fmt.Errorf("get_llm_provider: provider service returned an empty response")
		}

		cfg := provider.Configuration
		out := getLLMProviderOutput{
			llmProviderItem: toLLMProviderItem(provider),
			Description:     provider.Description,
			Version:         cfg.Version,
			Guardrails:      []providerGuardrailItem{},
			Deployments:     []providerGatewayDeploymentItem{},
		}
		// Only the URL and whether auth is set: the auth value is the vendor key.
		if cfg.Upstream != nil && cfg.Upstream.Main != nil {
			out.UpstreamURL = cfg.Upstream.Main.URL
			out.UpstreamAuth = cfg.Upstream.Main.Auth != nil
		}
		for _, p := range cfg.Policies {
			g := providerGuardrailItem{Name: p.Name, Version: p.Version}
			if len(p.Paths) > 0 {
				g.Params = p.Paths[0].Params
			}
			out.Guardrails = append(out.Guardrails, g)
		}
		for _, d := range deployments {
			// Archived rows are superseded history; showing them beside the live
			// deployment on the same gateway reads as a half-deployed provider.
			if d == nil || (d.Status != nil && *d.Status == models.DeploymentStatusArchived) {
				continue
			}
			item := providerGatewayDeploymentItem{GatewayID: d.GatewayUUID.String()}
			if d.Status != nil {
				item.Status = string(*d.Status)
			}
			out.Deployments = append(out.Deployments, item)
		}
		for _, mp := range provider.ModelProviders {
			if mp.Name != "" {
				out.ModelProviders = append(out.ModelProviders, mp.Name)
			}
		}
		return handleToolResult(out, nil)
	}
}

func addLLMProviderGuardrails(handler LLMProviderToolsetHandler) func(context.Context, *gomcp.CallToolRequest, addLLMProviderGuardrailsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input addLLMProviderGuardrailsInput) (*gomcp.CallToolResult, any, error) {
		const toolName = "add_llm_provider_guardrails"
		providerID := strings.TrimSpace(input.ProviderID)
		if providerID == "" {
			return nil, nil, fmt.Errorf("provider_id is required. Call list_llm_providers to see valid providers")
		}
		if len(input.Guardrails) == 0 {
			return nil, nil, fmt.Errorf("guardrails is required. Call list_llm_guardrails to see valid guardrails")
		}
		added, err := toModelGuardrails(input.Guardrails)
		if err != nil {
			return nil, nil, err
		}
		return updateProviderGuardrails(ctx, toolName, handler, providerID, func(current []models.LLMPolicy) ([]models.LLMPolicy, error) {
			next := make([]models.LLMPolicy, 0, len(current)+len(added))
			replaced := make(map[string]bool, len(added))
			for _, p := range current {
				if g, ok := findGuardrail(added, p.Name); ok {
					next = append(next, g)
					replaced[p.Name] = true
					continue
				}
				next = append(next, p)
			}
			for _, g := range added {
				if !replaced[g.Name] {
					next = append(next, g)
					replaced[g.Name] = true
				}
			}
			return next, nil
		})
	}
}

func removeLLMProviderGuardrail(handler LLMProviderToolsetHandler) func(context.Context, *gomcp.CallToolRequest, removeLLMProviderGuardrailInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input removeLLMProviderGuardrailInput) (*gomcp.CallToolResult, any, error) {
		const toolName = "remove_llm_provider_guardrail"
		providerID, name := strings.TrimSpace(input.ProviderID), strings.TrimSpace(input.Name)
		if providerID == "" {
			return nil, nil, fmt.Errorf("provider_id is required. Call list_llm_providers to see valid providers")
		}
		if name == "" {
			return nil, nil, fmt.Errorf("name is required. Call get_llm_provider to see the provider's guardrails")
		}
		return updateProviderGuardrails(ctx, toolName, handler, providerID, func(current []models.LLMPolicy) ([]models.LLMPolicy, error) {
			next := make([]models.LLMPolicy, 0, len(current))
			for _, p := range current {
				if p.Name != name {
					next = append(next, p)
				}
			}
			if len(next) == len(current) {
				return nil, fmt.Errorf("%s: provider %q has no guardrail named %q. Call get_llm_provider to see its guardrails", toolName, providerID, name)
			}
			return next, nil
		})
	}
}

// updateProviderGuardrails reads the provider's current guardrails, applies
// change, and saves the result, reporting the redeployments that carried it to
// the gateways.
func updateProviderGuardrails(
	ctx context.Context, toolName string, handler LLMProviderToolsetHandler, providerID string,
	change func([]models.LLMPolicy) ([]models.LLMPolicy, error),
) (*gomcp.CallToolResult, any, error) {
	ouID := resolveOUID(ctx)
	current, _, err := handler.GetLLMProvider(ctx, ouID, providerID)
	if err != nil {
		return nil, nil, wrapToolError(toolName, err)
	}
	if current == nil {
		return nil, nil, fmt.Errorf("%s: provider service returned an empty response", toolName)
	}
	next, err := change(current.Configuration.Policies)
	if err != nil {
		return nil, nil, err
	}
	updated, results, err := handler.SetLLMProviderGuardrails(ctx, ouID, providerID, next)
	if err != nil {
		return nil, nil, wrapToolError(toolName, err)
	}

	out := setGuardrailsOutput{ProviderID: providerID, Guardrails: []providerGuardrailItem{}, Deployments: []providerDeploymentItem{}}
	saved := next
	if updated != nil {
		saved = updated.Configuration.Policies
	}
	for _, p := range saved {
		g := providerGuardrailItem{Name: p.Name, Version: p.Version}
		if len(p.Paths) > 0 {
			g.Params = p.Paths[0].Params
		}
		out.Guardrails = append(out.Guardrails, g)
	}
	failed := 0
	for _, d := range results {
		out.Deployments = append(out.Deployments, providerDeploymentItem{GatewayID: d.GatewayID, Success: d.Success, Error: d.Error})
		if !d.Success {
			failed++
		}
	}
	switch {
	case len(results) == 0:
		out.Note = "Guardrails saved. The provider is not deployed to any gateway, so they take effect when it is deployed."
	case failed > 0:
		out.Note = fmt.Sprintf("Guardrails saved; %d of %d gateway redeployments failed, so those gateways still enforce the old guardrails. See redeployments for the errors.", failed, len(results))
	default:
		out.Note = "Guardrails saved and the provider redeployed; they now apply to every agent using it."
	}
	return handleToolResult(out, nil)
}

func findGuardrail(guardrails []models.LLMPolicy, name string) (models.LLMPolicy, bool) {
	for _, g := range guardrails {
		if g.Name == name {
			return g, true
		}
	}
	return models.LLMPolicy{}, false
}

func listLLMGuardrails(handler LLMProviderToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listLLMGuardrailsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listLLMGuardrailsInput) (*gomcp.CallToolResult, any, error) {
		resp, err := handler.ListLLMGuardrails(ctx, resolveOUID(ctx), strings.TrimSpace(input.ProviderID))
		if err != nil {
			return nil, nil, wrapToolError("list_llm_guardrails", err)
		}
		items := []guardrailItem{}
		if resp != nil {
			for _, p := range resp.List {
				items = append(items, guardrailItem{
					Name:        p.Name,
					Version:     p.Version,
					DisplayName: p.DisplayName,
					Description: p.Description,
					Parameters:  p.Parameters,
				})
			}
		}
		return handleToolResult(listLLMGuardrailsOutput{Total: len(items), Guardrails: items}, nil)
	}
}

func createLLMProvider(handler LLMProviderToolsetHandler) func(context.Context, *gomcp.CallToolRequest, createLLMProviderInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input createLLMProviderInput) (*gomcp.CallToolResult, any, error) {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return nil, nil, fmt.Errorf("name is required")
		}
		templateID := strings.TrimSpace(input.Template)
		if templateID == "" {
			return nil, nil, fmt.Errorf("template is required. Call list_llm_provider_templates to see valid templates")
		}
		handle := toHandle(name)
		if handle == "" {
			return nil, nil, fmt.Errorf("name must contain at least one letter or digit")
		}
		version := strings.TrimSpace(input.Version)
		if version == "" {
			version = defaultLLMProviderVersion
		}
		if !llmProviderVersionPattern.MatchString(version) {
			return nil, nil, fmt.Errorf("version must look like v1.0")
		}
		guardrails, err := toSpecGuardrails(input.Guardrails)
		if err != nil {
			return nil, nil, err
		}

		ouID := resolveOUID(ctx)
		template, err := findLLMProviderTemplate(ctx, handler, ouID, templateID)
		if err != nil {
			return nil, nil, err
		}

		upstreamURL := strings.TrimSpace(input.UpstreamURL)
		if upstreamURL == "" && template.Metadata != nil {
			upstreamURL = template.Metadata.EndpointURL
		}
		if upstreamURL == "" {
			return nil, nil, fmt.Errorf("upstream_url is required: template %q has no default endpoint URL", templateID)
		}

		req := &spec.CreateLLMProviderRequest{
			Id:            handle,
			Name:          name,
			Version:       version,
			Context:       "/" + handle,
			Template:      template.Handle,
			Upstream:      spec.UpstreamConfig{Main: &spec.UpstreamEndpoint{Url: &upstreamURL}},
			AccessControl: &spec.LLMAccessControl{Mode: "allow_all", Exceptions: []spec.RouteException{}},
			Gateways:      input.GatewayIDs,
		}
		if desc := strings.TrimSpace(input.Description); desc != "" {
			req.Description = &desc
		}
		if len(guardrails) > 0 {
			req.Policies = guardrails
		}
		// Built the way the console builds it: the template decides the header
		// and value prefix, and an upstream key turns on API-key security at the
		// gateway so agents need their own key to reach the provider.
		if apiKey := strings.TrimSpace(input.APIKey); apiKey != "" {
			authType, header, value := "api-key", "Authorization", apiKey
			if template.Metadata != nil && template.Metadata.Auth != nil {
				if template.Metadata.Auth.Type != "" {
					authType = template.Metadata.Auth.Type
				}
				if template.Metadata.Auth.Header != "" {
					header = template.Metadata.Auth.Header
				}
				value = template.Metadata.Auth.ValuePrefix + apiKey
			}
			req.Upstream.Main.Auth = &spec.UpstreamAuth{Type: authType, Header: &header, Value: &value}
			enabled, in := true, "header"
			req.Security = &spec.SecurityConfig{Enabled: &enabled, ApiKey: &spec.APIKeySecurity{Enabled: &enabled, In: &in}}
		}

		created, deployments, err := handler.CreateLLMProvider(ctx, ouID, req)
		if err != nil {
			return nil, nil, wrapToolError("create_llm_provider", err)
		}
		if created == nil {
			return nil, nil, fmt.Errorf("create_llm_provider: provider service returned an empty response")
		}

		out := createLLMProviderOutput{llmProviderItem: toLLMProviderItem(created)}
		for _, g := range input.Guardrails {
			out.Guardrails = append(out.Guardrails, g.Name)
		}
		failed := 0
		for _, d := range deployments {
			out.Deployments = append(out.Deployments, providerDeploymentItem{GatewayID: d.GatewayID, Success: d.Success, Error: d.Error})
			if !d.Success {
				failed++
			}
		}
		switch {
		case len(input.GatewayIDs) == 0:
			out.Note = "Provider created but not deployed. Agents can only use it once it is deployed to a gateway; recreate with gateway_ids or deploy it from the console."
		case failed > 0:
			out.Note = fmt.Sprintf("Provider created; %d of %d gateway deployments failed. See deployments for the errors.", failed, len(deployments))
		default:
			out.Note = "Provider created and deployed. Call attach_llm_provider_to_agent to let an agent use it."
		}
		return handleToolResult(out, nil)
	}
}

func findLLMProviderTemplate(ctx context.Context, handler LLMProviderToolsetHandler, ouID, templateID string) (*models.LLMProviderTemplate, error) {
	templates, _, err := handler.ListLLMProviderTemplates(ctx, ouID, utils.MaxLimit, utils.DefaultOffset)
	if err != nil {
		return nil, wrapToolError("create_llm_provider", err)
	}
	for _, tpl := range templates {
		if tpl != nil && strings.EqualFold(tpl.Handle, templateID) {
			return tpl, nil
		}
	}
	return nil, fmt.Errorf("create_llm_provider: template %q not found. Call list_llm_provider_templates to see valid templates", templateID)
}

func toLLMProviderItem(p *models.LLMProvider) llmProviderItem {
	item := llmProviderItem{
		UUID:     p.UUID.String(),
		Template: p.TemplateHandle,
		Status:   p.Status,
	}
	if p.Artifact != nil {
		item.ID, item.Name = p.Artifact.Handle, p.Artifact.Name
	}
	return item
}

func validateGuardrails(guardrails []guardrailInput) error {
	for i, g := range guardrails {
		if strings.TrimSpace(g.Name) == "" || strings.TrimSpace(g.Version) == "" {
			return fmt.Errorf("guardrails[%d]: name and version are required. Call list_llm_guardrails to see valid guardrails", i)
		}
	}
	return nil
}

func toSpecGuardrails(guardrails []guardrailInput) ([]spec.LLMPolicy, error) {
	if err := validateGuardrails(guardrails); err != nil {
		return nil, err
	}
	out := make([]spec.LLMPolicy, 0, len(guardrails))
	for _, g := range guardrails {
		params := g.Params
		if params == nil {
			params = map[string]any{}
		}
		out = append(out, spec.LLMPolicy{
			Name:    strings.TrimSpace(g.Name),
			Version: strings.TrimSpace(g.Version),
			Paths:   []spec.LLMPolicyPath{{Path: guardrailPathAll, Methods: []string{"*"}, Params: params}},
		})
	}
	return out, nil
}

func toModelGuardrails(guardrails []guardrailInput) ([]models.LLMPolicy, error) {
	if err := validateGuardrails(guardrails); err != nil {
		return nil, err
	}
	out := make([]models.LLMPolicy, 0, len(guardrails))
	for _, g := range guardrails {
		params := g.Params
		if params == nil {
			params = map[string]any{}
		}
		out = append(out, models.LLMPolicy{
			Name:    strings.TrimSpace(g.Name),
			Version: strings.TrimSpace(g.Version),
			Paths:   []models.LLMPolicyPath{{Path: guardrailPathAll, Methods: []string{"*"}, Params: params}},
		})
	}
	return out, nil
}

// toHandle derives a resource handle from a display name the way the console
// does: lowercase, runs of anything else collapsed to one dash.
func toHandle(name string) string {
	return strings.Trim(nonHandleChars.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-")
}
