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
