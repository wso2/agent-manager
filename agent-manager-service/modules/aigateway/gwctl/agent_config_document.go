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

package gwctl

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconv"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

const (
	kindAgentModelConfig = "AgentModelConfig"
	kindAgentMcpConfig   = "AgentMcpConfig"
)

// agentConfigKind describes one of the two config kinds; they differ in the type stored and the key naming the bound resource.
type agentConfigKind struct {
	kind      string
	typ       string
	typeID    uint
	targetKey string
	label     string
}

var (
	modelConfigKind = agentConfigKind{kind: kindAgentModelConfig, typ: gwmodels.AgentConfigTypeLLM, typeID: gwmodels.AgentConfigTypeIDLLM, targetKey: "providerName", label: "model"}
	mcpConfigKind   = agentConfigKind{kind: kindAgentMcpConfig, typ: gwmodels.AgentConfigTypeMCP, typeID: gwmodels.AgentConfigTypeIDMCP, targetKey: "proxyName", label: "MCP"}
)

// agentConfigDocumentBody is a posted config document; pointers tell an absent field from an empty one.
type agentConfigDocumentBody struct {
	ApiVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name    string `json:"name"`
		Project string `json:"project"`
	} `json:"metadata"`
	Spec *struct {
		Description          *string                             `json:"description"`
		ProviderName         string                              `json:"providerName"`
		ProxyName            string                              `json:"proxyName"`
		Configuration        *gwspec.EnvProviderConfiguration    `json:"configuration"`
		EnvironmentVariables *[]gwspec.EnvironmentVariableConfig `json:"environmentVariables"`
	} `json:"spec"`
}

// decodeAgentConfigDocument decodes a posted config document and checks its envelope against the route.
func decodeAgentConfigDocument(body []byte, k agentConfigKind, project string) (agentConfigDocumentBody, string, error) {
	var doc agentConfigDocumentBody
	if err := json.Unmarshal(body, &doc); err != nil {
		return doc, "", fmt.Errorf("%w: %w", gwconv.ErrMalformedBody, err)
	}
	if doc.ApiVersion != gwconv.GroupVersion {
		return doc, "", fmt.Errorf("%w: apiVersion must be %q, got %q", gwconv.ErrDocument, gwconv.GroupVersion, doc.ApiVersion)
	}
	if doc.Kind != k.kind {
		return doc, "", fmt.Errorf("%w: kind must be %q on this route, got %q", gwconv.ErrDocument, k.kind, doc.Kind)
	}
	if strings.TrimSpace(doc.Metadata.Name) == "" {
		return doc, "", fmt.Errorf("%w: metadata.name is required - it is the configuration's name", gwconv.ErrDocument)
	}
	if doc.Metadata.Project != project {
		return doc, "", fmt.Errorf("%w: metadata.project must match the project query parameter", gwconv.ErrDocument)
	}
	if doc.Spec == nil {
		return doc, "", fmt.Errorf("%w: spec is required", gwconv.ErrDocument)
	}
	target := doc.Spec.ProviderName
	if k.typ == gwmodels.AgentConfigTypeMCP {
		target = doc.Spec.ProxyName
	}
	if strings.TrimSpace(target) == "" {
		return doc, "", fmt.Errorf("%w: spec.%s is required", gwconv.ErrDocument, k.targetKey)
	}
	return doc, target, nil
}

// envConfigOf turns a spec configuration into the stored per-environment form.
func envConfigOf(cfg *gwspec.EnvProviderConfiguration) gwmodels.EnvProviderConfiguration {
	if cfg == nil {
		return gwmodels.EnvProviderConfiguration{}
	}
	return gwmodels.EnvProviderConfiguration{
		Policies:   convertToModelPolicies(cfg),
		Resilience: gwconv.ConvertSpecToModelResilience(cfg.Resilience),
	}
}

func envVarsOf(vars []gwspec.EnvironmentVariableConfig) []gwmodels.EnvironmentVariableConfig {
	out := make([]gwmodels.EnvironmentVariableConfig, 0, len(vars))
	for _, v := range vars {
		out = append(out, gwmodels.EnvironmentVariableConfig{Key: v.Key, Name: v.Name})
	}
	return out
}

// agentConfigCreateFromDocument turns a POST body into the create request for the default environment.
func agentConfigCreateFromDocument(body []byte, k agentConfigKind, project string) (gwmodels.CreateAgentModelConfigRequest, error) {
	doc, target, err := decodeAgentConfigDocument(body, k, project)
	if err != nil {
		return gwmodels.CreateAgentModelConfigRequest{}, err
	}
	req := gwmodels.CreateAgentModelConfigRequest{
		Name:        doc.Metadata.Name,
		Description: getString(doc.Spec.Description),
		Type:        k.typ,
		EnvMappings: map[string]gwmodels.EnvModelConfigRequest{
			gwmodels.DefaultEnvironmentName: {ProviderName: target, Configuration: envConfigOf(doc.Spec.Configuration)},
		},
	}
	if doc.Spec.EnvironmentVariables != nil {
		req.EnvironmentVariables = envVarsOf(*doc.Spec.EnvironmentVariables)
	}
	return req, nil
}

// agentConfigUpdateFromDocument turns a PUT body into the update request, keeping what it omits.
func agentConfigUpdateFromDocument(body []byte, k agentConfigKind, project string, stored *gwmodels.AgentModelConfigResponse) (gwmodels.UpdateAgentModelConfigRequest, error) {
	doc, target, err := decodeAgentConfigDocument(body, k, project)
	if err != nil {
		return gwmodels.UpdateAgentModelConfigRequest{}, err
	}
	cfg := envConfigOf(doc.Spec.Configuration)
	if doc.Spec.Configuration == nil {
		if proxy := defaultEnvProxy(stored); proxy != nil {
			cfg = gwmodels.EnvProviderConfiguration{Policies: proxy.Policies, Resilience: proxy.Resilience}
		}
	}
	req := gwmodels.UpdateAgentModelConfigRequest{
		Name:        doc.Metadata.Name,
		Description: stored.Description,
		EnvMappings: map[string]gwmodels.EnvModelConfigRequest{
			gwmodels.DefaultEnvironmentName: {ProviderName: target, Configuration: cfg},
		},
	}
	if doc.Spec.Description != nil {
		req.Description = *doc.Spec.Description
	}
	if doc.Spec.EnvironmentVariables != nil {
		req.EnvironmentVariables = envVarsOf(*doc.Spec.EnvironmentVariables)
	}
	return req, nil
}

// defaultEnvProxy returns the binding's proxy in the default environment, if it has one.
func defaultEnvProxy(m *gwmodels.AgentModelConfigResponse) *gwmodels.LLMProxyInfo {
	env, ok := m.EnvModelConfig[gwmodels.DefaultEnvironmentName]
	if !ok {
		return nil
	}
	return env.LLMProxy
}

// agentConfigParts renders a stored config's metadata, spec and status; the two kinds wrap them.
func agentConfigParts(m *gwmodels.AgentModelConfigResponse) (gwspec.ProjectScopedDocumentMetadata, string, *gwspec.EnvProviderConfiguration, *gwspec.AgentConfigStatus) {
	meta := gwspec.ProjectScopedDocumentMetadata{Name: m.Name, Project: m.ProjectName}
	if m.UUID != "" {
		uid := m.UUID
		meta.Uid = &uid
	}
	if !m.CreatedAt.IsZero() {
		created := m.CreatedAt
		meta.CreationTimestamp = &created
	}
	if !m.UpdatedAt.IsZero() {
		updated := m.UpdatedAt
		meta.UpdateTimestamp = &updated
	}
	proxy := defaultEnvProxy(m)
	if proxy == nil {
		return meta, "", nil, nil
	}
	target := getString(proxy.ProviderName)
	cfg := &gwspec.EnvProviderConfiguration{
		Policies:   convertToSpecPolicies(&proxy.Policies),
		Resilience: gwconv.ConvertModelToSpecResilience(proxy.Resilience),
	}
	status := &gwspec.AgentConfigStatus{
		Url:              proxy.URL,
		ProxyUuid:        proxy.ProxyUUID,
		ProxyName:        proxy.ProxyName,
		ProviderUuid:     proxy.ProviderUUID,
		ProviderPolicies: convertToSpecPolicies(&proxy.ProviderPolicies),
	}
	if proxy.AuthHeaderName != nil && *proxy.AuthHeaderName != "" {
		in := "header"
		if proxy.AuthIn != nil && *proxy.AuthIn != "" {
			in = *proxy.AuthIn
		}
		status.AuthInfo = &gwspec.AuthInfo{Type: "api-key", In: in, Name: *proxy.AuthHeaderName, Value: proxy.APIKey}
	}
	return meta, target, cfg, status
}

func envVarsSpec(vars []gwmodels.EnvironmentVariableConfig) []gwspec.EnvironmentVariableConfig {
	out := make([]gwspec.EnvironmentVariableConfig, 0, len(vars))
	for _, v := range vars {
		out = append(out, gwspec.EnvironmentVariableConfig{Key: v.Key, Name: v.Name})
	}
	return out
}

// agentModelConfigDocument renders a stored LLM binding as a document.
func agentModelConfigDocument(m *gwmodels.AgentModelConfigResponse) gwspec.AgentModelConfigDocument {
	meta, target, cfg, status := agentConfigParts(m)
	return gwspec.AgentModelConfigDocument{
		ApiVersion: gwconv.GroupVersion,
		Kind:       kindAgentModelConfig,
		Metadata:   meta,
		Spec: gwspec.AgentModelConfigSpec{
			Description:          getStringPtr(m.Description),
			ProviderName:         target,
			Configuration:        cfg,
			EnvironmentVariables: envVarsSpec(m.EnvironmentVariables),
		},
		Status: status,
	}
}

// agentMcpConfigDocument renders a stored MCP binding as a document.
func agentMcpConfigDocument(m *gwmodels.AgentModelConfigResponse) gwspec.AgentMcpConfigDocument {
	meta, target, cfg, status := agentConfigParts(m)
	return gwspec.AgentMcpConfigDocument{
		ApiVersion: gwconv.GroupVersion,
		Kind:       kindAgentMcpConfig,
		Metadata:   meta,
		Spec: gwspec.AgentMcpConfigSpec{
			Description:          getStringPtr(m.Description),
			ProxyName:            target,
			Configuration:        cfg,
			EnvironmentVariables: envVarsSpec(m.EnvironmentVariables),
		},
		Status: status,
	}
}

// agentConfigDocumentOf renders a stored config as the document of its kind.
func agentConfigDocumentOf(k agentConfigKind, m *gwmodels.AgentModelConfigResponse) any {
	if k.typ == gwmodels.AgentConfigTypeMCP {
		return agentMcpConfigDocument(m)
	}
	return agentModelConfigDocument(m)
}

// agentConfigListOf renders one page of configs in the list envelope of its kind.
func agentConfigListOf(k agentConfigKind, items []*gwmodels.AgentModelConfigResponse, page gwmodels.PaginationInfo) any {
	pagination := gwspec.PaginationInfo{Count: int32(page.Count), Offset: int32(page.Offset), Limit: int32(page.Limit)}
	if k.typ == gwmodels.AgentConfigTypeMCP {
		list := make([]gwspec.AgentMcpConfigDocument, 0, len(items))
		for _, m := range items {
			list = append(list, agentMcpConfigDocument(m))
		}
		return gwspec.AgentMcpConfigListResponse{Count: int32(len(list)), List: list, Pagination: pagination}
	}
	list := make([]gwspec.AgentModelConfigDocument, 0, len(items))
	for _, m := range items {
		list = append(list, agentModelConfigDocument(m))
	}
	return gwspec.AgentModelConfigListResponse{Count: int32(len(list)), List: list, Pagination: pagination}
}

// writeAgentConfigError maps a config service error to its response.
func writeAgentConfigError(w http.ResponseWriter, log *slog.Logger, k agentConfigKind, op string, err error) {
	switch {
	case errors.Is(err, gwerr.ErrAgentConfigAlreadyExists):
		if k.typ == gwmodels.AgentConfigTypeMCP {
			httpx.WriteErrorResponse(w, http.StatusConflict, "Agent MCP configuration already exists")
			return
		}
		httpx.WriteErrorResponse(w, http.StatusConflict, "Agent configuration already exists")
	case errors.Is(err, gwerr.ErrAgentConfigNotFound):
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
	case errors.Is(err, gwerr.ErrAgentNotFound):
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
	case errors.Is(err, gwerr.ErrLLMProviderNotFound):
		httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
	case errors.Is(err, gwerr.ErrMCPProxyNotFound):
		httpx.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
	case errors.Is(err, httpx.ErrInvalidInput):
		httpx.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, httpx.ErrUnauthorized):
		httpx.WriteErrorResponse(w, http.StatusUnauthorized, "Unauthorized access")
	case errors.Is(err, httpx.ErrForbidden):
		httpx.WriteErrorResponse(w, http.StatusForbidden, "Forbidden")
	case errors.Is(err, gwerr.ErrLLMProxyExists):
		httpx.WriteErrorResponse(w, http.StatusConflict, "LLM proxy name collision: another agent in this org already uses the same model-config name")
	default:
		log.Error(op+": failed", "error", err)
		if writeIfNotImplemented(w, err) {
			return
		}
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Failed to %s agent %s configuration", strings.ToLower(strings.TrimSuffix(op, "Config")), k.label))
	}
}

// writeAgentConfigDocumentError answers a body that is not a valid config document.
func writeAgentConfigDocumentError(w http.ResponseWriter, log *slog.Logger, op string, err error) {
	if errors.Is(err, gwconv.ErrMalformedBody) {
		log.Error(op+": failed to decode the document", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	log.Warn(op+": the document was refused", "error", err)
	httpx.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
}

// readConfigBody reads a posted document, capped at 1 MB.
func readConfigBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", gwconv.ErrMalformedBody, err)
	}
	return body, nil
}

// configIDOf parses the configId path value, answering 400 when it is not a UUID.
func configIDOf(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	configUUID, err := uuid.Parse(r.PathValue(gwutil.PathParamConfigId))
	if err != nil {
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return uuid.Nil, false
	}
	return configUUID, true
}

// getConfigOfKind loads a config and answers 404 when it is of the other kind.
func (c *agentConfigurationController) getConfigOfKind(w http.ResponseWriter, r *http.Request, k agentConfigKind, op string, configUUID uuid.UUID) (*gwmodels.AgentModelConfigResponse, bool) {
	stored, err := c.agentConfigService.Get(r.Context(), configUUID, modkit.OrgID(r.Context()), gwutil.Project(r), r.PathValue(gwutil.PathParamAgentName))
	if err != nil {
		writeAgentConfigError(w, logger.GetLogger(r.Context()), k, op, err)
		return nil, false
	}
	if stored.Type != k.typ {
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
		return nil, false
	}
	return stored, true
}

func (c *agentConfigurationController) createConfig(w http.ResponseWriter, r *http.Request, k agentConfigKind, op string) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	body, err := readConfigBody(w, r)
	if err != nil {
		writeAgentConfigDocumentError(w, log, op, err)
		return
	}
	project := gwutil.Project(r)
	req, err := agentConfigCreateFromDocument(body, k, project)
	if err != nil {
		writeAgentConfigDocumentError(w, log, op, err)
		return
	}
	if err := gwutil.ValidateConfigName(req.Name); err != nil {
		httpx.WriteValidationErrorResponse(w, err)
		return
	}
	created, err := c.agentConfigService.Create(ctx, modkit.OrgID(ctx), project, r.PathValue(gwutil.PathParamAgentName), req, callerSubject(ctx))
	if err != nil {
		writeAgentConfigError(w, log, k, op, err)
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusCreated, agentConfigDocumentOf(k, created))
}

func (c *agentConfigurationController) getConfig(w http.ResponseWriter, r *http.Request, k agentConfigKind, op string) {
	configUUID, ok := configIDOf(w, r)
	if !ok {
		return
	}
	stored, ok := c.getConfigOfKind(w, r, k, op, configUUID)
	if !ok {
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, agentConfigDocumentOf(k, stored))
}

// listConfigs reads each config of the page in full, since a list item must carry the bound resource.
func (c *agentConfigurationController) listConfigs(w http.ResponseWriter, r *http.Request, k agentConfigKind, op string) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID, project, agentName := modkit.OrgID(ctx), gwutil.Project(r), r.PathValue(gwutil.PathParamAgentName)
	limit := getIntQueryParam(r, "limit", 20)
	if limit < 1 {
		limit = 20
	}
	limit = min(limit, 100)
	offset := max(getIntQueryParam(r, "offset", 0), 0)

	page, err := c.agentConfigService.ListByType(ctx, ouID, project, agentName, k.typeID, limit, offset)
	if err != nil {
		writeAgentConfigError(w, log, k, op, err)
		return
	}
	items := make([]*gwmodels.AgentModelConfigResponse, 0, len(page.Configs))
	for _, item := range page.Configs {
		configUUID, err := uuid.Parse(item.UUID)
		if err != nil {
			writeAgentConfigError(w, log, k, op, fmt.Errorf("config %q has an invalid UUID: %w", item.UUID, err))
			return
		}
		full, err := c.agentConfigService.Get(ctx, configUUID, ouID, project, agentName)
		if errors.Is(err, gwerr.ErrAgentConfigNotFound) {
			continue
		}
		if err != nil {
			writeAgentConfigError(w, log, k, op, err)
			return
		}
		items = append(items, full)
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, agentConfigListOf(k, items, page.Pagination))
}

func (c *agentConfigurationController) updateConfig(w http.ResponseWriter, r *http.Request, k agentConfigKind, op string) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	configUUID, ok := configIDOf(w, r)
	if !ok {
		return
	}
	body, err := readConfigBody(w, r)
	if err != nil {
		writeAgentConfigDocumentError(w, log, op, err)
		return
	}
	project := gwutil.Project(r)
	doc, _, err := decodeAgentConfigDocument(body, k, project)
	if err != nil {
		writeAgentConfigDocumentError(w, log, op, err)
		return
	}
	if err := gwutil.ValidateConfigName(doc.Metadata.Name); err != nil {
		httpx.WriteValidationErrorResponse(w, err)
		return
	}
	stored, ok := c.getConfigOfKind(w, r, k, op, configUUID)
	if !ok {
		return
	}
	req, err := agentConfigUpdateFromDocument(body, k, project, stored)
	if err != nil {
		writeAgentConfigDocumentError(w, log, op, err)
		return
	}
	updated, err := c.agentConfigService.Update(ctx, configUUID, modkit.OrgID(ctx), project, r.PathValue(gwutil.PathParamAgentName), req)
	if err != nil {
		writeAgentConfigError(w, log, k, op, err)
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, agentConfigDocumentOf(k, updated))
}

// CreateAgentModelConfig handles POST /agents/{agentName}/model-configs.
func (c *agentConfigurationController) CreateAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	c.createConfig(w, r, modelConfigKind, "CreateConfig")
}

// GetAgentModelConfig handles GET /agents/{agentName}/model-configs/{configId}.
func (c *agentConfigurationController) GetAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	c.getConfig(w, r, modelConfigKind, "GetConfig")
}

// ListAgentModelConfigs handles GET /agents/{agentName}/model-configs.
func (c *agentConfigurationController) ListAgentModelConfigs(w http.ResponseWriter, r *http.Request) {
	c.listConfigs(w, r, modelConfigKind, "ListConfig")
}

// UpdateAgentModelConfig handles PUT /agents/{agentName}/model-configs/{configId}.
func (c *agentConfigurationController) UpdateAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	c.updateConfig(w, r, modelConfigKind, "UpdateConfig")
}

// CreateAgentMCPConfig handles POST /agents/{agentName}/mcp-configs.
func (c *agentConfigurationController) CreateAgentMCPConfig(w http.ResponseWriter, r *http.Request) {
	c.createConfig(w, r, mcpConfigKind, "CreateConfig")
}

// GetAgentMCPConfig handles GET /agents/{agentName}/mcp-configs/{configId}.
func (c *agentConfigurationController) GetAgentMCPConfig(w http.ResponseWriter, r *http.Request) {
	c.getConfig(w, r, mcpConfigKind, "GetConfig")
}

// ListAgentMCPConfigs handles GET /agents/{agentName}/mcp-configs.
func (c *agentConfigurationController) ListAgentMCPConfigs(w http.ResponseWriter, r *http.Request) {
	c.listConfigs(w, r, mcpConfigKind, "ListConfig")
}

// UpdateAgentMCPConfig handles PUT /agents/{agentName}/mcp-configs/{configId}.
func (c *agentConfigurationController) UpdateAgentMCPConfig(w http.ResponseWriter, r *http.Request) {
	c.updateConfig(w, r, mcpConfigKind, "UpdateConfig")
}
