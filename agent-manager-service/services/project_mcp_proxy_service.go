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

package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	defaultProjectMCPProxyVersion = "v1"
	// projectMCPProxyCreatedBy is the creator recorded for proxies published through the
	// project routes, matching the org create path.
	projectMCPProxyCreatedBy = "system"
)

// ProjectMCPProxyService manages MCP proxies owned by a project. It reuses
// MCPProxyService for endpoint persistence and teardown, so the proxy rows and their
// cleanup are the ones the org path produces.
type ProjectMCPProxyService struct {
	base     *MCPProxyService
	ocClient occlient.OpenChoreoClient
}

// NewProjectMCPProxyService creates the project MCP proxy service.
func NewProjectMCPProxyService(base *MCPProxyService, ocClient occlient.OpenChoreoClient) *ProjectMCPProxyService {
	return &ProjectMCPProxyService{base: base, ocClient: ocClient}
}

// legacySSEMCPServerType is the component type of MCP servers that speak legacy HTTP+SSE
// (/sse + /messages), which the gateway does not proxy.
const legacySSEMCPServerType = "mi-mcp-server"

// ensureSourceSupported rejects a source component that is missing or is a legacy HTTP+SSE
// server, before anything is written. A lookup failure other than not-found is returned as is.
func (s *ProjectMCPProxyService) ensureSourceSupported(
	ctx context.Context, ouID, projectName string, source models.MCPProxySource,
) error {
	component, err := s.ocClient.GetComponent(ctx, ouID, projectName, source.ComponentName)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return fmt.Errorf("%w: source component %q not found", utils.ErrInvalidInput, source.ComponentName)
		}
		return fmt.Errorf("failed to get source component: %w", err)
	}
	// The client reports the type without its "deployment/" prefix; tolerate either form.
	typ := component.Type.Type
	if i := strings.LastIndex(typ, "/"); i >= 0 {
		typ = typ[i+1:]
	}
	if typ == legacySSEMCPServerType {
		return fmt.Errorf("%w: legacy HTTP+SSE MCP servers are not supported", utils.ErrInvalidInput)
	}
	return nil
}

// Upsert creates the project proxy, or updates it when it already exists in this project.
// created reports an insert. Sending the body a proxy already has changes nothing.
func (s *ProjectMCPProxyService) Upsert(
	ctx context.Context, ouID, projectName, projectUUID, handle string, req models.ProjectMCPProxyRequest,
) (*models.ProjectMCPProxyResponse, bool, error) {
	return s.UpsertWithCreateCheck(ctx, ouID, projectName, projectUUID, handle, req, true)
}

// UpsertWithCreateCheck is Upsert for a caller that may be allowed to update but not to
// create: when canCreate is false and the proxy does not exist it returns
// utils.ErrMCPProxyCreateNotPermitted without writing anything.
func (s *ProjectMCPProxyService) UpsertWithCreateCheck(
	ctx context.Context, ouID, projectName, projectUUID, handle string, req models.ProjectMCPProxyRequest, canCreate bool,
) (*models.ProjectMCPProxyResponse, bool, error) {
	handle = strings.TrimSpace(handle)
	desired, err := normalizeProjectMCPProxyRequest(handle, req)
	if err != nil {
		return nil, false, err
	}

	if err := s.ensureSourceSupported(ctx, ouID, projectName, desired.source); err != nil {
		return nil, false, err
	}

	existing, err := s.base.repo.GetInProject(ctx, handle, ouID, projectUUID)
	switch {
	case err == nil:
		resp, err := s.update(ctx, ouID, existing, desired)
		if err != nil {
			return nil, false, err
		}
		return resp, false, s.fillEnvironments(ctx, ouID, existing, resp)
	case !errors.Is(err, utils.ErrMCPProxyNotFound):
		return nil, false, fmt.Errorf("failed to get MCP proxy: %w", err)
	}

	// Handles are unique across the org, so a handle that is not in this project may still
	// be taken by an org-level proxy or another project's proxy.
	taken, err := s.base.repo.Exists(ctx, handle, ouID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check MCP proxy existence: %w", err)
	}
	if taken {
		return nil, false, handleInUseError(handle)
	}
	if !canCreate {
		return nil, false, utils.ErrMCPProxyCreateNotPermitted
	}

	created, err := s.create(ctx, ouID, projectName, projectUUID, handle, desired)
	if err != nil {
		if !errors.Is(err, utils.ErrMCPProxyHandleInUse) {
			return nil, false, err
		}
		// A concurrent ensure of the same proxy won the insert: converge on it as an update.
		existing, getErr := s.base.repo.GetInProject(ctx, handle, ouID, projectUUID)
		if getErr != nil {
			return nil, false, err
		}
		resp, updErr := s.update(ctx, ouID, existing, desired)
		if updErr != nil {
			return nil, false, updErr
		}
		return resp, false, s.fillEnvironments(ctx, ouID, existing, resp)
	}
	return projectMCPProxyResponse(created), true, nil
}

// fillEnvironments sets resp's environments to proxy's bindings, read with the context resp reports (an update
// may just have changed it).
func (s *ProjectMCPProxyService) fillEnvironments(
	ctx context.Context, ouID string, proxy *models.MCPProxy, resp *models.ProjectMCPProxyResponse,
) error {
	current := *proxy
	contextPath := resp.Context
	current.Configuration.Context = &contextPath
	environments, err := s.environments(ctx, ouID, &current)
	if err != nil {
		return err
	}
	resp.Environments = environments
	return nil
}

// Get returns the project's proxy. A handle in another project, or an org proxy, is not found.
func (s *ProjectMCPProxyService) Get(
	ctx context.Context, ouID, projectUUID, handle string,
) (*models.ProjectMCPProxyResponse, error) {
	proxy, err := s.getInProject(ctx, ouID, projectUUID, handle)
	if err != nil {
		return nil, err
	}
	resp := projectMCPProxyResponse(proxy)
	if err := s.fillEnvironments(ctx, ouID, proxy, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// List returns the project's own proxies, optionally narrowed by source component and endpoint.
func (s *ProjectMCPProxyService) List(
	ctx context.Context, ouID, projectUUID, component, endpoint string, limit, offset int,
) (*models.ProjectMCPProxyListResponse, error) {
	proxies, total, err := s.base.repo.List(ctx, ouID, repositories.MCPProxyListFilter{
		ProjectUUID: &projectUUID,
		Component:   component,
		Endpoint:    endpoint,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP proxies: %w", err)
	}
	resp := &models.ProjectMCPProxyListResponse{
		Count:      len(proxies),
		List:       make([]models.ProjectMCPProxyListItem, 0, len(proxies)),
		Pagination: models.PaginationInfo{Count: int(total), Limit: limit, Offset: offset},
	}
	for _, proxy := range proxies {
		item := projectMCPProxyListItem(proxy)
		environments, err := s.environments(ctx, ouID, proxy)
		if err != nil {
			return nil, err
		}
		for _, env := range environments {
			item.Environments = append(item.Environments, models.ProjectMCPProxyEnvironmentSummary{
				EnvironmentName:        env.EnvironmentName,
				DeploymentStatus:       env.DeploymentStatus,
				DeploymentStatusReason: env.DeploymentStatusReason,
				PublicURL:              env.PublicURL,
			})
		}
		resp.List = append(resp.List, item)
	}
	return resp, nil
}

// Delete removes the project's proxy and tears down every environment's gateway artifact.
// It refuses with utils.ErrMCPProxyHasMappings while agents use the proxy.
func (s *ProjectMCPProxyService) Delete(ctx context.Context, ouID, projectUUID, handle string) error {
	proxy, err := s.getInProject(ctx, ouID, projectUUID, handle)
	if err != nil {
		return err
	}
	if err := s.base.ensureMCPProxyNotInUse(ctx, ouID, proxy); err != nil {
		return err
	}
	return s.base.deleteLoadedMCPProxy(ctx, ouID, proxyHandleOf(proxy), proxy)
}

func (s *ProjectMCPProxyService) getInProject(
	ctx context.Context, ouID, projectUUID, handle string,
) (*models.MCPProxy, error) {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return nil, utils.ErrInvalidInput
	}
	proxy, err := s.base.repo.GetInProject(ctx, handle, ouID, projectUUID)
	if err != nil {
		if errors.Is(err, utils.ErrMCPProxyNotFound) {
			return nil, utils.ErrMCPProxyNotFound
		}
		return nil, fmt.Errorf("failed to get MCP proxy: %w", err)
	}
	return proxy, nil
}

// desiredProjectMCPProxy is a request with its defaults applied.
type desiredProjectMCPProxy struct {
	name, description, version, context, specVersion string
	source                                           models.MCPProxySource
}

func normalizeProjectMCPProxyRequest(handle string, req models.ProjectMCPProxyRequest) (desiredProjectMCPProxy, error) {
	d := desiredProjectMCPProxy{
		name:        strings.TrimSpace(req.Name),
		description: req.Description,
		version:     strings.TrimSpace(req.Version),
		context:     strings.TrimSpace(req.Context),
		specVersion: strings.TrimSpace(req.McpSpecVersion),
		source: models.MCPProxySource{
			ComponentName: strings.TrimSpace(req.Source.ComponentName),
			EndpointName:  strings.TrimSpace(req.Source.EndpointName),
		},
	}
	if handle == "" || len(handle) > 100 || !mcpProxyHandleRe.MatchString(handle) {
		return d, fmt.Errorf("%w: proxy id must be kebab-case (lowercase letters, digits, single hyphens) and at most 100 characters", utils.ErrInvalidInput)
	}
	if d.name == "" {
		return d, fmt.Errorf("%w: name is required", utils.ErrInvalidInput)
	}
	if d.source.ComponentName == "" || d.source.EndpointName == "" {
		return d, fmt.Errorf("%w: source.componentName and source.endpointName are required", utils.ErrInvalidInput)
	}
	if d.version == "" {
		d.version = defaultProjectMCPProxyVersion
	}
	if d.context == "" {
		d.context = "/" + handle
	} else if !strings.HasPrefix(d.context, "/") {
		return d, fmt.Errorf("%w: context must start with \"/\"", utils.ErrInvalidInput)
	}
	return d, nil
}

func (s *ProjectMCPProxyService) create(
	ctx context.Context, ouID, projectName, projectUUID, handle string, d desiredProjectMCPProxy,
) (*models.MCPProxy, error) {
	ctxPath := d.context
	source := d.source
	proxy := &models.MCPProxy{
		Description: d.description,
		CreatedBy:   projectMCPProxyCreatedBy,
		Status:      models.StatusCreated,
		Configuration: models.MCPProxyConfig{
			Name:        d.name,
			Version:     d.version,
			Context:     &ctxPath,
			SpecVersion: d.specVersion,
			Source:      &source,
		},
	}
	// A project proxy has no endpoint until it is bound: each environment binding is its own endpoint row
	// (PutEnvironment), since the upstream and security differ per environment.
	artifactName := models.MCPProxyArtifactName(&projectName, d.name)

	if err := s.base.tx(ctx, func(tx *gorm.DB) error {
		return s.base.repo.Create(ctx, tx, proxy, handle, artifactName, d.version, ouID, &projectUUID, &projectName)
	}); err != nil {
		if mapped := s.base.mapMCPProxyWriteError(err, handle, ouID); mapped != nil {
			return nil, mapped
		}
		return nil, fmt.Errorf("failed to create MCP proxy: %w", err)
	}

	created, err := s.base.repo.GetInProject(ctx, handle, ouID, projectUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve MCP proxy: %w", err)
	}
	return created, nil
}

// update applies the request to an existing project proxy. A body equal to the stored
// proxy writes nothing.
func (s *ProjectMCPProxyService) update(
	ctx context.Context, ouID string, existing *models.MCPProxy, d desiredProjectMCPProxy,
) (*models.ProjectMCPProxyResponse, error) {
	cfg := existing.Configuration
	if cfg.Source != nil && *cfg.Source != d.source {
		return nil, fmt.Errorf("%w: source", utils.ErrImmutableFieldChange)
	}

	storedContext := ""
	if cfg.Context != nil {
		storedContext = *cfg.Context
	}
	unchanged := cfg.Source != nil &&
		cfg.Name == d.name && cfg.Version == d.version && storedContext == d.context &&
		cfg.SpecVersion == d.specVersion && existing.Description == d.description
	if unchanged {
		return projectMCPProxyResponse(existing), nil
	}

	updated := *existing
	ctxPath := d.context
	source := d.source
	updated.Description = d.description
	updated.Configuration.Name = d.name
	updated.Configuration.Version = d.version
	updated.Configuration.Context = &ctxPath
	updated.Configuration.SpecVersion = d.specVersion
	updated.Configuration.Source = &source

	if err := s.base.tx(ctx, func(tx *gorm.DB) error {
		return s.base.repo.Update(ctx, tx, &updated, ouID)
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMCPProxyNotFound
		}
		if mapped := s.base.mapMCPProxyWriteError(err, proxyHandleOf(existing), ouID); mapped != nil {
			return nil, mapped
		}
		return nil, fmt.Errorf("failed to update MCP proxy: %w", err)
	}
	// The proxy's name, version and context are part of every bound environment's gateway artifact.
	s.rerenderBindings(ctx, ouID, &updated)
	// The artifact timestamp moved with the update.
	if updated.Artifact != nil {
		art := *updated.Artifact
		art.UpdatedAt = time.Now()
		updated.Artifact = &art
	}
	return projectMCPProxyResponse(&updated), nil
}

func handleInUseError(handle string) error {
	return fmt.Errorf("%w: proxy id %q", utils.ErrMCPProxyHandleInUse, handle)
}

func projectMCPProxySource(proxy *models.MCPProxy) models.MCPProxySource {
	if proxy.Configuration.Source != nil {
		return *proxy.Configuration.Source
	}
	return models.MCPProxySource{}
}

func projectMCPProxyTimes(proxy *models.MCPProxy) (created, updated *time.Time) {
	if proxy.Artifact != nil {
		return timePtrIfNotZero(proxy.Artifact.CreatedAt), timePtrIfNotZero(proxy.Artifact.UpdatedAt)
	}
	return timePtrIfNotZero(proxy.CreatedAt), timePtrIfNotZero(proxy.UpdatedAt)
}

func projectMCPProxyResponse(proxy *models.MCPProxy) *models.ProjectMCPProxyResponse {
	createdAt, updatedAt := projectMCPProxyTimes(proxy)
	return &models.ProjectMCPProxyResponse{
		ID:             proxyHandleOf(proxy),
		Name:           proxy.Configuration.Name,
		Description:    proxy.Description,
		Version:        proxy.Configuration.Version,
		Context:        valueOrEmpty(proxy.Configuration.Context),
		McpSpecVersion: proxy.Configuration.SpecVersion,
		ProjectName:    valueOrEmpty(proxy.ProjectName),
		Source:         projectMCPProxySource(proxy),
		Environments:   []models.MCPProxyEnvironmentResponse{},
		CreatedBy:      proxy.CreatedBy,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
}

func projectMCPProxyListItem(proxy *models.MCPProxy) models.ProjectMCPProxyListItem {
	createdAt, updatedAt := projectMCPProxyTimes(proxy)
	return models.ProjectMCPProxyListItem{
		ID:           proxyHandleOf(proxy),
		Name:         proxy.Configuration.Name,
		Description:  proxy.Description,
		Version:      proxy.Configuration.Version,
		Context:      valueOrEmpty(proxy.Configuration.Context),
		Source:       projectMCPProxySource(proxy),
		Environments: []models.ProjectMCPProxyEnvironmentSummary{},
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
}
