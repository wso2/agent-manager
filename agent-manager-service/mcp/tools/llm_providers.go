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
	"slices"
	"sort"
	"strings"
	"unicode"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	// guardrailPathAll applies a guardrail to every route and method, as the
	// console's agent and add-provider flows do. The provider's Guardrails tab
	// can also scope a guardrail to single routes, one path entry each; these
	// tools manage only the all-routes entry and leave route entries as they are.
	guardrailPathAll = "/*"
)

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
	Version    string `json:"version,omitempty"`
}
type addLLMProviderToCatalogInput struct {
	ProviderID string `json:"provider_id"`
}
type addLLMProviderToCatalogOutput struct {
	llmProviderItem
	Note string `json:"note"`
}
type listLLMGuardrailsInput struct {
	ProviderID string `json:"provider_id,omitempty"`
	Name       string `json:"name,omitempty"`
}
type guardrailInput struct {
	Name    string         `json:"name"`
	Version string         `json:"version"`
	Params  map[string]any `json:"params,omitempty"`
}

// llmProviderItem carries the provider's id (its handle) and no UUID: attach
// takes the handle only, so a UUID beside it would be copied there and fail.
type llmProviderItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Template string `json:"template"`
	Status   string `json:"status"`
	// InCatalog is false for a provider taken out of the catalog, which agents
	// cannot be attached to.
	InCatalog bool `json:"in_catalog"`
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
	Name    string `json:"name"`
	Version string `json:"version"`
	// AllRoutes and Params describe the all-routes entry; Routes are the
	// single routes the console scoped the guardrail to, each with its params.
	AllRoutes bool                 `json:"all_routes"`
	Params    map[string]any       `json:"params,omitempty"`
	Routes    []guardrailRouteItem `json:"routes,omitempty"`
}
type guardrailRouteItem struct {
	Path    string         `json:"path"`
	Methods []string       `json:"methods"`
	Params  map[string]any `json:"params,omitempty"`
}
type providerGatewayDeploymentItem struct {
	GatewayID string `json:"gateway_id"`
	Status    string `json:"status,omitempty"`
	// Environments are the names of the environments the gateway serves, the
	// names attach_llm_provider_to_agent takes.
	Environments []string `json:"environments"`
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

func guardrailSchema() map[string]any {
	return createSchema(map[string]any{
		"name":    stringProperty("Required. Guardrail policy name from list_llm_guardrails."),
		"version": stringProperty("Required. Guardrail policy version from list_llm_guardrails."),
		"params": map[string]any{
			"type": "object",
			"description": "Optional. Policy parameters, shaped by the `parameters` schema list_llm_guardrails returns for this policy. " +
				"Never put a credential here (an API key, token, secret or password): such a parameter is refused, and a guardrail that needs one is set in the console.",
		},
	}, []string{"name", "version"})
}

func (t *Toolsets) registerLLMProviderTools(server *gomcp.Server, reg *toolRegistry) {
	addTool(reg, server, &gomcp.Tool{
		Name: "list_llm_providers",
		Description: "List the LLM providers configured in your organization. " +
			"An LLM provider is an org-level connection to a model vendor (for example OpenAI) that agents reach through the AI gateway. " +
			"Use a provider's `id` as the `provider_id` argument of attach_llm_provider_to_agent; only providers with `in_catalog` true can be attached.",
		InputSchema: createSchema(map[string]any{
			"limit":  intProperty(fmt.Sprintf("Optional. Max providers to return (default %d, min %d, max %d).", utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)),
			"offset": intProperty(fmt.Sprintf("Optional. Pagination offset (default %d, min %d).", utils.DefaultOffset, utils.MinOffset)),
		}, nil),
	}, audit.ActionLLMProviderRead, listLLMProviders(t.LLMProviderToolset), rbac.LLMProviderRead)

	addTool(reg, server, &gomcp.Tool{
		Name: "get_llm_provider",
		Description: "Get one LLM provider's details: its template, upstream URL, the guardrails enforced on it, " +
			"and the gateways it is deployed to with each deployment's status and the environments that gateway serves. " +
			"Use it to check a provider is deployed before attach_llm_provider_to_agent. Credentials are never returned.",
		InputSchema: createSchema(map[string]any{
			"provider_id": stringProperty("Required. LLM provider id from list_llm_providers."),
		}, []string{"provider_id"}),
	}, audit.ActionLLMProviderRead, getLLMProvider(t.LLMProviderToolset, t.EnvironmentToolset), rbac.LLMProviderRead)

	addTool(reg, server, &gomcp.Tool{
		Name: "add_llm_provider_guardrails",
		Description: "Add guardrails to an existing LLM provider on all of its routes; they apply to every agent that uses it. " +
			"Adding a guardrail the provider already has, with the same name and version, updates its all-routes settings; " +
			"routes the guardrail was scoped to in the console are kept. A different version of a guardrail already on the provider is refused. " +
			"The provider is redeployed to the gateways it is already on so the change takes effect. " +
			"Call list_llm_guardrails for valid guardrails and get_llm_provider to see the current ones.",
		InputSchema: createSchema(map[string]any{
			"provider_id": stringProperty("Required. LLM provider id from list_llm_providers."),
			"guardrails":  arrayProperty("Required. Guardrails to add or replace.", guardrailSchema()),
		}, []string{"provider_id", "guardrails"}),
	}, audit.ActionLLMProviderUpdate, addLLMProviderGuardrails(t.LLMProviderToolset), rbac.LLMProviderUpdate)

	addTool(reg, server, &gomcp.Tool{
		Name: "remove_llm_provider_guardrail",
		Description: "Remove a guardrail from an LLM provider's routes, by name. " +
			"Only the all-routes guardrail is removed: routes the guardrail was scoped to in the console are kept, and are removed in the console. " +
			"The provider is redeployed to the gateways it is already on so the change takes effect. " +
			"Call get_llm_provider to see the provider's current guardrails.",
		InputSchema: createSchema(map[string]any{
			"provider_id": stringProperty("Required. LLM provider id from list_llm_providers."),
			"name":        stringProperty("Required. Name of the guardrail to remove, as get_llm_provider lists it."),
			"version":     stringProperty("Optional. Version of the guardrail to remove; required when the provider has more than one version of it."),
		}, []string{"provider_id", "name"}),
	}, audit.ActionLLMProviderUpdate, removeLLMProviderGuardrail(t.LLMProviderToolset), rbac.LLMProviderUpdate)

	addTool(reg, server, &gomcp.Tool{
		Name: "add_llm_provider_to_catalog",
		Description: "Put an LLM provider back in the catalog, so agents can be attached to it again with attach_llm_provider_to_agent. " +
			"list_llm_providers shows `in_catalog`; a provider already in the catalog is left as it is. " +
			"Taking a provider out of the catalog is done in the console.",
		InputSchema: createSchema(map[string]any{
			"provider_id": stringProperty("Required. LLM provider `id` from list_llm_providers."),
		}, []string{"provider_id"}),
	}, audit.ActionLLMProviderUpdate, addLLMProviderToCatalog(t.LLMProviderToolset), rbac.LLMProviderUpdate)

	addTool(reg, server, &gomcp.Tool{
		Name: "list_llm_guardrails",
		Description: "List the guardrails the organization's gateways can enforce on LLM traffic, such as PII masking, word-count or content-length checks. " +
			"Without `name` it returns each guardrail's name, version and description. " +
			"Call it again with `name` to get that guardrail's parameter schema, which says what `params` it takes. " +
			"Use the results as `guardrails` in add_llm_provider_guardrails or attach_llm_provider_to_agent.",
		InputSchema: createSchema(map[string]any{
			"provider_id": stringProperty("Optional. LLM provider id; limits the result to guardrails on the gateways that provider is deployed to."),
			"name":        stringProperty("Optional. A guardrail name from an earlier call; returns only that guardrail, with its parameter schema."),
		}, nil),
	}, audit.ActionLLMProviderRead, listLLMGuardrails(t.LLMProviderToolset), rbac.LLMProviderRead)
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

func getLLMProvider(handler LLMProviderToolsetHandler, envHandler EnvironmentToolsetHandler) func(context.Context, *gomcp.CallToolRequest, getLLMProviderInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getLLMProviderInput) (*gomcp.CallToolResult, any, error) {
		providerID := strings.TrimSpace(input.ProviderID)
		if providerID == "" {
			return nil, nil, fmt.Errorf("provider_id is required. Call list_llm_providers to see valid providers")
		}
		ouID := resolveOUID(ctx)
		provider, deployments, err := handler.GetLLMProvider(ctx, ouID, providerID)
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
			Guardrails:      toProviderGuardrailItems(cfg.Policies),
			Deployments:     []providerGatewayDeploymentItem{},
		}
		// Only the redacted URL and whether auth is set: the auth value is the
		// vendor key, and a URL can carry credentials of its own.
		if cfg.Upstream != nil && cfg.Upstream.Main != nil {
			out.UpstreamURL = redactURL(cfg.Upstream.Main.URL)
			out.UpstreamAuth = cfg.Upstream.Main.Auth != nil
		}
		for _, d := range deployments {
			// Archived rows are superseded history; showing them beside the live
			// deployment on the same gateway reads as a half-deployed provider.
			if d == nil || (d.Status != nil && *d.Status == models.DeploymentStatusArchived) {
				continue
			}
			item := providerGatewayDeploymentItem{GatewayID: d.GatewayUUID.String(), Environments: []string{}}
			if d.Status != nil {
				item.Status = string(*d.Status)
			}
			out.Deployments = append(out.Deployments, item)
		}
		if err := nameDeploymentEnvironments(ctx, handler, envHandler, ouID, out.Deployments); err != nil {
			return nil, nil, wrapToolError("get_llm_provider", err)
		}
		for _, mp := range provider.ModelProviders {
			if mp.Name != "" {
				out.ModelProviders = append(out.ModelProviders, mp.Name)
			}
		}
		return handleToolResult(out, nil)
	}
}

// nameDeploymentEnvironments fills in the environments each deployment's gateway
// serves, by name, so a caller can pass them to attach_llm_provider_to_agent.
// A name lookup that fails leaves the environment's UUID, as get_mcp_proxy does.
func nameDeploymentEnvironments(
	ctx context.Context, handler LLMProviderToolsetHandler, envHandler EnvironmentToolsetHandler,
	ouID string, deployments []providerGatewayDeploymentItem,
) error {
	if len(deployments) == 0 {
		return nil
	}
	gatewayIDs := make([]string, 0, len(deployments))
	for _, d := range deployments {
		gatewayIDs = append(gatewayIDs, d.GatewayID)
	}
	envIDs, err := handler.GatewayEnvironmentIDs(ctx, gatewayIDs)
	if err != nil {
		return err
	}
	names := environmentNamesByUUID(ctx, envHandler, ouID)
	for i := range deployments {
		for _, id := range envIDs[deployments[i].GatewayID] {
			name := names[id]
			if name == "" {
				name = id
			}
			deployments[i].Environments = append(deployments[i].Environments, name)
		}
		sort.Strings(deployments[i].Environments)
	}
	return nil
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
		if err := requireAvailableGuardrails(ctx, toolName, handler, resolveOUID(ctx), providerID, added); err != nil {
			return nil, nil, err
		}
		return updateProviderGuardrails(ctx, toolName, handler, providerID, func(current []models.LLMPolicy) ([]models.LLMPolicy, error) {
			next := append([]models.LLMPolicy(nil), current...)
			for _, g := range added {
				if i := indexGuardrail(next, g.Name, g.Version); i >= 0 {
					// Same guardrail: update its all-routes settings and keep the
					// routes the console scoped it to.
					next[i] = withAllRoutesParams(next[i], g.Paths[0].Params)
					continue
				}
				for _, p := range next {
					if p.Name == g.Name {
						return nil, fmt.Errorf("%s: provider %q already has %s version %s. Add version %s to change its settings, or remove it first",
							toolName, providerID, g.Name, p.Version, p.Version)
					}
				}
				next = append(next, g)
			}
			return next, nil
		})
	}
}

func removeLLMProviderGuardrail(handler LLMProviderToolsetHandler) func(context.Context, *gomcp.CallToolRequest, removeLLMProviderGuardrailInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input removeLLMProviderGuardrailInput) (*gomcp.CallToolResult, any, error) {
		const toolName = "remove_llm_provider_guardrail"
		providerID, name, version := strings.TrimSpace(input.ProviderID), strings.TrimSpace(input.Name), strings.TrimSpace(input.Version)
		if providerID == "" {
			return nil, nil, fmt.Errorf("provider_id is required. Call list_llm_providers to see valid providers")
		}
		if name == "" {
			return nil, nil, fmt.Errorf("name is required. Call get_llm_provider to see the provider's guardrails")
		}
		return updateProviderGuardrails(ctx, toolName, handler, providerID, func(current []models.LLMPolicy) ([]models.LLMPolicy, error) {
			match := -1
			var versions []string
			for i, p := range current {
				if p.Name == name && (version == "" || p.Version == version) {
					match = i
					versions = append(versions, p.Version)
				}
			}
			switch {
			case match < 0 && version != "":
				return nil, fmt.Errorf("%s: provider %q has no guardrail %q at version %s. Call get_llm_provider to see its guardrails", toolName, providerID, name, version)
			case match < 0:
				return nil, fmt.Errorf("%s: provider %q has no guardrail named %q. Call get_llm_provider to see its guardrails", toolName, providerID, name)
			case len(versions) > 1:
				return nil, fmt.Errorf("%s: provider %q has versions %s of %q. Pass version to choose one", toolName, providerID, strings.Join(versions, ", "), name)
			}

			p := current[match]
			routes := routeScopedPaths(p.Paths)
			if len(p.Paths) > 0 && len(routes) == len(p.Paths) {
				return nil, fmt.Errorf("%s: %q on provider %q applies only to routes chosen in the console (%s). Remove it from those routes in the console",
					toolName, name, providerID, strings.Join(routeLabels(routes), ", "))
			}
			next := make([]models.LLMPolicy, 0, len(current))
			next = append(next, current[:match]...)
			if len(routes) > 0 {
				p.Paths = routes
				next = append(next, p)
			}
			return append(next, current[match+1:]...), nil
		})
	}
}

func addLLMProviderToCatalog(handler LLMProviderToolsetHandler) func(context.Context, *gomcp.CallToolRequest, addLLMProviderToCatalogInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input addLLMProviderToCatalogInput) (*gomcp.CallToolResult, any, error) {
		const toolName = "add_llm_provider_to_catalog"
		providerID := strings.TrimSpace(input.ProviderID)
		if providerID == "" {
			return nil, nil, fmt.Errorf("provider_id is required. Call list_llm_providers to see valid providers")
		}
		provider, err := handler.AddLLMProviderToCatalog(ctx, resolveOUID(ctx), providerID)
		if err != nil {
			return nil, nil, wrapToolError(toolName, err)
		}
		if provider == nil {
			return nil, nil, fmt.Errorf("%s: provider service returned an empty response", toolName)
		}
		return handleToolResult(addLLMProviderToCatalogOutput{
			llmProviderItem: toLLMProviderItem(provider),
			Note:            "The provider is in the catalog; agents can be attached to it with attach_llm_provider_to_agent.",
		}, nil)
	}
}

// updateProviderGuardrails reads the provider's current guardrails, applies
// change, and saves the result, reporting the redeployments that carried it to
// the gateways.
func updateProviderGuardrails(
	ctx context.Context, toolName string, handler LLMProviderToolsetHandler, providerID string,
	change func([]models.LLMPolicy) ([]models.LLMPolicy, error),
) (*gomcp.CallToolResult, any, error) {
	// change's own errors already name the tool and say what to do, so they go
	// back as they are; only service errors are wrapped.
	var next []models.LLMPolicy
	var changeErr error
	updated, results, err := handler.UpdateLLMProviderGuardrails(ctx, resolveOUID(ctx), providerID, func(current []models.LLMPolicy) ([]models.LLMPolicy, error) {
		next, changeErr = change(current)
		return next, changeErr
	})
	if changeErr != nil {
		return nil, nil, changeErr
	}
	if err != nil {
		return nil, nil, wrapToolError(toolName, err)
	}

	saved := next
	if updated != nil {
		saved = updated.Configuration.Policies
	}
	out := setGuardrailsOutput{ProviderID: providerID, Guardrails: toProviderGuardrailItems(saved), Deployments: []providerDeploymentItem{}}
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

// indexGuardrail finds a policy by name and version, the pair the console keys
// guardrails by; -1 when the provider has none.
func indexGuardrail(policies []models.LLMPolicy, name, version string) int {
	for i, p := range policies {
		if p.Name == name && p.Version == version {
			return i
		}
	}
	return -1
}

func isAllRoutesPath(p models.LLMPolicyPath) bool {
	return p.Path == guardrailPathAll && (len(p.Methods) == 0 || slices.Contains(p.Methods, "*"))
}

// routeScopedPaths returns the paths the console scoped a guardrail to, leaving
// out the all-routes entry.
func routeScopedPaths(paths []models.LLMPolicyPath) []models.LLMPolicyPath {
	var routes []models.LLMPolicyPath
	for _, p := range paths {
		if !isAllRoutesPath(p) {
			routes = append(routes, p)
		}
	}
	return routes
}

// withAllRoutesParams sets the params of the policy's all-routes entry, adding
// that entry when the policy only had route entries, and keeps every route entry.
func withAllRoutesParams(p models.LLMPolicy, params map[string]any) models.LLMPolicy {
	paths := make([]models.LLMPolicyPath, 0, len(p.Paths)+1)
	found := false
	for _, path := range p.Paths {
		if isAllRoutesPath(path) {
			if found {
				continue
			}
			path.Params, found = params, true
		}
		paths = append(paths, path)
	}
	if !found {
		paths = append(paths, models.LLMPolicyPath{Path: guardrailPathAll, Methods: []string{"*"}, Params: params})
	}
	p.Paths = paths
	return p
}

func routeLabels(paths []models.LLMPolicyPath) []string {
	labels := make([]string, 0, len(paths))
	for _, p := range paths {
		labels = append(labels, strings.Join(p.Methods, ",")+" "+p.Path)
	}
	return labels
}

// toProviderGuardrailItems shows each guardrail with its full scope, so a
// guardrail the console scoped to single routes never reads as provider-wide.
// It is never nil, so an empty list is [] in JSON.
func toProviderGuardrailItems(policies []models.LLMPolicy) []providerGuardrailItem {
	items := make([]providerGuardrailItem, 0, len(policies))
	for _, p := range policies {
		g := providerGuardrailItem{Name: p.Name, Version: p.Version}
		for _, path := range p.Paths {
			if !isAllRoutesPath(path) {
				g.Routes = append(g.Routes, guardrailRouteItem{Path: path.Path, Methods: path.Methods, Params: path.Params})
			} else if !g.AllRoutes {
				g.AllRoutes, g.Params = true, path.Params
			}
		}
		items = append(items, g)
	}
	return items
}

func listLLMGuardrails(handler LLMProviderToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listLLMGuardrailsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listLLMGuardrailsInput) (*gomcp.CallToolResult, any, error) {
		resp, err := handler.ListLLMGuardrails(ctx, resolveOUID(ctx), strings.TrimSpace(input.ProviderID))
		if err != nil {
			return nil, nil, wrapToolError("list_llm_guardrails", err)
		}
		var policies []models.LLMPolicyDefinition
		if resp != nil {
			policies = resp.List
		}

		// One guardrail with its schema. The full list with every schema runs to
		// well over 100 KB, past what MCP clients pass to the model, so schemas
		// are only returned one at a time.
		if name := strings.TrimSpace(input.Name); name != "" {
			for _, p := range policies {
				if p.Name == name && !nonGuardrailPolicies[p.Name] {
					item := guardrailItem{Name: p.Name, Version: p.Version, DisplayName: p.DisplayName, Description: p.Description, Parameters: p.Parameters}
					return handleToolResult(listLLMGuardrailsOutput{Total: 1, Guardrails: []guardrailItem{item}}, nil)
				}
			}
			return nil, nil, fmt.Errorf("list_llm_guardrails: no guardrail named %q. Call list_llm_guardrails without name to see valid guardrails", name)
		}

		items := []guardrailItem{}
		for _, p := range policies {
			if nonGuardrailPolicies[p.Name] {
				continue
			}
			items = append(items, guardrailItem{Name: p.Name, Version: p.Version, DisplayName: p.DisplayName, Description: p.Description})
		}
		return handleToolResult(listLLMGuardrailsOutput{Total: len(items), Guardrails: items}, nil)
	}
}

// nonGuardrailPolicies are gateway policies that are not LLM guardrails: auth,
// CORS, rate limiting (managed separately), MCP-only and mediation policies.
// The gateways report them alongside the guardrails, and the console hides them
// from its guardrail picker with this same list; keep the two in step:
// console/workspaces/libs/api-client/src/hooks/guardrails.ts,
// NON_GUARDRAIL_POLICY_EXCLUDELIST.
var nonGuardrailPolicies = map[string]bool{
	"api-key-auth":             true,
	"basic-auth":               true,
	"jwt-auth":                 true,
	"cors":                     true,
	"advanced-ratelimit":       true,
	"basic-ratelimit":          true,
	"token-based-ratelimit":    true,
	"llm-cost-based-ratelimit": true,
	"llm-cost":                 true,
	"mcp-acl-list":             true,
	"mcp-auth":                 true,
	"mcp-authz":                true,
	"mcp-rewrite":              true,
	"mcp-ratelimit":            true,
	"respond":                  true,
	"redirect":                 true,
	"semantic-tool-filtering":  true,
	"analytics-header-filter":  true,
	"backend-jwt":              true,
	"dynamic-endpoint":         true,
	"host-rewrite":             true,
	"json-xml-mediator":        true,
	"log-message":              true,
	"remove-headers":           true,
	"request-rewrite":          true,
	"set-headers":              true,
	"subscription-validation":  true,
}

func toLLMProviderItem(p *models.LLMProvider) llmProviderItem {
	item := llmProviderItem{
		Template:  p.TemplateHandle,
		Status:    p.Status,
		InCatalog: p.InCatalog,
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
		if path := credentialParam(g.Params, ""); path != "" {
			return fmt.Errorf("guardrails[%d] (%s): parameter %q looks like a credential, and a credential must not pass through an MCP tool call. "+
				"Set this guardrail in the console instead, where the value never reaches the model", i, strings.TrimSpace(g.Name), path)
		}
	}
	return nil
}

// requireAvailableGuardrails refuses, before anything is saved, a guardrail the
// gateways do not report, at the version given, and a gateway policy that is
// not a guardrail (jwt-auth, cors): either would otherwise be stored on the
// provider and pushed to its gateways. Guardrails are checked against what the
// provider's gateways report; a provider on no gateway yet reports nothing, so
// it is checked against the org's gateways, where it can be deployed.
func requireAvailableGuardrails(
	ctx context.Context, toolName string, handler LLMProviderToolsetHandler, ouID, providerID string, guardrails []models.LLMPolicy,
) error {
	if len(guardrails) == 0 {
		return nil
	}
	if handler == nil {
		return fmt.Errorf("%s: guardrail lookup is not available", toolName)
	}
	resp, err := handler.ListLLMGuardrails(ctx, ouID, providerID)
	if err != nil {
		return wrapToolError(toolName, err)
	}
	if resp == nil || len(resp.List) == 0 {
		if resp, err = handler.ListLLMGuardrails(ctx, ouID, ""); err != nil {
			return wrapToolError(toolName, err)
		}
	}
	versions := map[string][]string{}
	if resp != nil {
		for _, p := range resp.List {
			if !nonGuardrailPolicies[p.Name] {
				versions[p.Name] = append(versions[p.Name], p.Version)
			}
		}
	}
	for _, g := range guardrails {
		if nonGuardrailPolicies[g.Name] {
			return fmt.Errorf("%s: %s is a gateway policy, not a guardrail. Call list_llm_guardrails to see valid guardrails", toolName, g.Name)
		}
		available := versions[g.Name]
		if len(available) == 0 {
			return fmt.Errorf("%s: no guardrail named %q is available on the provider's gateways. "+
				"Call list_llm_guardrails with provider_id to see valid guardrails", toolName, g.Name)
		}
		if !slices.Contains(available, g.Version) {
			return fmt.Errorf("%s: %s version %s is not available on the provider's gateways; available: %s version %s",
				toolName, g.Name, g.Version, g.Name, strings.Join(available, ", "))
		}
	}
	return nil
}

// credentialParamNames are parameter names, lowercased with separators
// removed, that carry a credential. Policy schemas mark no parameter as
// sensitive, so the name is the signal; vendor keys a gateway holds itself are
// systemParameters and never reach params.
var credentialParamNames = []string{
	"apikey", "secret", "password", "passwd", "passphrase", "credential",
	"accesskey", "privatekey", "subscriptionkey", "accountkey",
	"accesstoken", "authtoken", "bearertoken",
}

// credentialParam returns the dotted path of the first parameter in params
// whose name holds a credential, at any depth, or "". A name that is exactly
// "key" or "token" counts too, but names such as keywords or maxTokens do not.
func credentialParam(params map[string]any, prefix string) string {
	for name, value := range params {
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		if isCredentialName(name) {
			return path
		}
		switch v := value.(type) {
		case map[string]any:
			if p := credentialParam(v, path); p != "" {
				return p
			}
		case []any:
			for i, item := range v {
				if m, ok := item.(map[string]any); ok {
					if p := credentialParam(m, fmt.Sprintf("%s[%d]", path, i)); p != "" {
						return p
					}
				}
			}
		}
	}
	return ""
}

func isCredentialName(name string) bool {
	normalized := strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r == '.' || r == ' ' {
			return -1
		}
		return unicode.ToLower(r)
	}, name)
	if normalized == "key" || normalized == "token" {
		return true
	}
	for _, c := range credentialParamNames {
		if strings.Contains(normalized, c) {
			return true
		}
	}
	return false
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
