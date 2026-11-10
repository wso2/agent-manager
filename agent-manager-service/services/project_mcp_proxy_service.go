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
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
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
	// discoveries is the bindings' capability discovery queue; nil shows and queues none.
	discoveries repositories.MCPCapabilityDiscoveryRepository
}

// NewProjectMCPProxyService creates the project MCP proxy service.
func NewProjectMCPProxyService(
	base *MCPProxyService, ocClient occlient.OpenChoreoClient, discoveries repositories.MCPCapabilityDiscoveryRepository,
) *ProjectMCPProxyService {
	return &ProjectMCPProxyService{base: base, ocClient: ocClient, discoveries: discoveries}
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
			return fmt.Errorf("%w: %q", utils.ErrMCPSourceComponentNotFound, source.ComponentName)
		}
		return fmt.Errorf("failed to get source component: %w", err)
	}
	// The lookup is by name in the namespace, so a same-named component of another project can come back. Its
	// release bindings are never this project's, so its bindings could never resolve an upstream: refuse it as not
	// found here, without saying where it lives.
	if component.ProjectName != projectName {
		return fmt.Errorf("%w: %q is not in project %q", utils.ErrMCPSourceComponentNotFound, source.ComponentName, projectName)
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

	if desired.source != nil {
		if err := s.ensureSourceSupported(ctx, ouID, projectName, *desired.source); err != nil {
			return nil, false, err
		}
	} else if err := ssrf.ValidateURL(ctx, desired.upstreamURL); err != nil {
		// A URL proxy's upstream is called from a managed gateway as given, so it must be public.
		return nil, false, fmt.Errorf("%w: upstream.url: %w", utils.ErrInvalidInput, err)
	}

	existing, err := s.base.repo.GetInProject(ctx, handle, ouID, projectUUID)
	switch {
	case err == nil:
		resp, err := s.update(ctx, ouID, existing, desired, "")
		if err != nil {
			return nil, false, err
		}
		return resp, false, s.fillEnvironments(ctx, ouID, existing, resp)
	case !errors.Is(err, utils.ErrMCPProxyNotFound):
		return nil, false, fmt.Errorf("failed to get MCP proxy: %w", err)
	}

	// A project deleted in OpenChoreo leaves its proxies under its old UUID. Project names are
	// unique at any time, so a proxy stored under this project's name with another UUID was
	// left by a deleted project of the same name: this re-created project adopts it.
	orphan, err := s.base.repo.GetInProjectByName(ctx, handle, ouID, projectName)
	switch {
	case err == nil && orphan.ProjectUUID != nil && *orphan.ProjectUUID != projectUUID:
		resp, err := s.update(ctx, ouID, orphan, desired, projectUUID)
		if err != nil {
			return nil, false, err
		}
		return resp, false, s.fillEnvironments(ctx, ouID, orphan, resp)
	case err != nil && !errors.Is(err, utils.ErrMCPProxyNotFound):
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
		resp, updErr := s.update(ctx, ouID, existing, desired, "")
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

// Get returns the project's proxy, or one a deleted project of the same name left (getInProjectOrOrphan). A handle in
// another project, or an org proxy, is not found.
func (s *ProjectMCPProxyService) Get(
	ctx context.Context, ouID, projectName, projectUUID, handle string,
) (*models.ProjectMCPProxyResponse, error) {
	proxy, err := s.getInProjectOrOrphan(ctx, ouID, projectName, projectUUID, handle)
	if err != nil {
		return nil, err
	}
	return s.proxyResponse(ctx, ouID, proxy)
}

// GetInDeletedProject is Get for a project OpenChoreo no longer has: it returns the proxy stored under
// projectName, whatever project UUID it carries. A handle stored under another project name, or an org
// proxy, is not found.
func (s *ProjectMCPProxyService) GetInDeletedProject(
	ctx context.Context, ouID, projectName, handle string,
) (*models.ProjectMCPProxyResponse, error) {
	proxy, err := s.getInDeletedProject(ctx, ouID, projectName, handle)
	if err != nil {
		return nil, err
	}
	return s.proxyResponse(ctx, ouID, proxy)
}

func (s *ProjectMCPProxyService) proxyResponse(ctx context.Context, ouID string, proxy *models.MCPProxy) (*models.ProjectMCPProxyResponse, error) {
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
	return s.list(ctx, ouID, repositories.MCPProxyListFilter{
		ProjectUUID: &projectUUID,
		Component:   component,
		Endpoint:    endpoint,
		Limit:       limit,
		Offset:      offset,
	})
}

// ListInDeletedProject is List for a project OpenChoreo no longer has: the proxies stored under projectName,
// whatever project UUID they carry.
func (s *ProjectMCPProxyService) ListInDeletedProject(
	ctx context.Context, ouID, projectName, component, endpoint string, limit, offset int,
) (*models.ProjectMCPProxyListResponse, error) {
	if strings.TrimSpace(projectName) == "" {
		return nil, utils.ErrInvalidInput
	}
	return s.list(ctx, ouID, repositories.MCPProxyListFilter{
		ProjectName: &projectName,
		Component:   component,
		Endpoint:    endpoint,
		Limit:       limit,
		Offset:      offset,
	})
}

func (s *ProjectMCPProxyService) list(
	ctx context.Context, ouID string, filter repositories.MCPProxyListFilter,
) (*models.ProjectMCPProxyListResponse, error) {
	limit, offset := filter.Limit, filter.Offset
	proxies, total, err := s.base.repo.List(ctx, ouID, filter)
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

// Delete removes the project's proxy, or one a deleted project of the same name left (getInProjectOrOrphan), and
// tears down every environment's gateway artifact. It refuses with utils.ErrMCPProxyHasMappings while agents use the
// proxy.
func (s *ProjectMCPProxyService) Delete(ctx context.Context, ouID, projectName, projectUUID, handle string) error {
	proxy, err := s.getInProjectOrOrphan(ctx, ouID, projectName, projectUUID, handle)
	if err != nil {
		return err
	}
	return s.deleteLoaded(ctx, ouID, proxy)
}

// DeleteInDeletedProject is Delete for a project OpenChoreo no longer has: it removes the proxy stored under
// projectName, whatever project UUID it carries, with the same teardown and in-use refusal.
func (s *ProjectMCPProxyService) DeleteInDeletedProject(ctx context.Context, ouID, projectName, handle string) error {
	proxy, err := s.getInDeletedProject(ctx, ouID, projectName, handle)
	if err != nil {
		return err
	}
	return s.deleteLoaded(ctx, ouID, proxy)
}

func (s *ProjectMCPProxyService) deleteLoaded(ctx context.Context, ouID string, proxy *models.MCPProxy) error {
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

// getInProjectOrOrphan is getInProject that also finds a proxy left by a deleted project of the same name: one stored
// under projectName with another project UUID. Project names are unique at any time, so that proxy's project is gone
// and was re-created, and the re-created project's routes read and remove what it left, as the routes of a project
// that is not re-created do (getInDeletedProject). A re-created project that has the proxy's source adopts it on PUT
// instead.
func (s *ProjectMCPProxyService) getInProjectOrOrphan(
	ctx context.Context, ouID, projectName, projectUUID, handle string,
) (*models.MCPProxy, error) {
	proxy, err := s.getInProject(ctx, ouID, projectUUID, handle)
	if !errors.Is(err, utils.ErrMCPProxyNotFound) || strings.TrimSpace(projectName) == "" {
		return proxy, err
	}
	orphan, err := s.getInDeletedProject(ctx, ouID, projectName, handle)
	if err != nil {
		return nil, err
	}
	if orphan.ProjectUUID == nil || *orphan.ProjectUUID == projectUUID {
		return nil, utils.ErrMCPProxyNotFound
	}
	return orphan, nil
}

// getInDeletedProject loads the project proxy stored under projectName, whatever project UUID it carries: the
// lookup for a project OpenChoreo no longer has. It never matches an org proxy or another project name's proxy.
func (s *ProjectMCPProxyService) getInDeletedProject(
	ctx context.Context, ouID, projectName, handle string,
) (*models.MCPProxy, error) {
	handle = strings.TrimSpace(handle)
	if handle == "" || strings.TrimSpace(projectName) == "" {
		return nil, utils.ErrInvalidInput
	}
	proxy, err := s.base.repo.GetInProjectByName(ctx, handle, ouID, projectName)
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
	// Exactly one of source and upstreamURL is set.
	source      *models.MCPProxySource
	upstreamURL string
}

func normalizeProjectMCPProxyRequest(handle string, req models.ProjectMCPProxyRequest) (desiredProjectMCPProxy, error) {
	d := desiredProjectMCPProxy{
		name:        strings.TrimSpace(req.Name),
		description: req.Description,
		version:     strings.TrimSpace(req.Version),
		context:     strings.TrimSpace(req.Context),
		specVersion: strings.TrimSpace(req.McpSpecVersion),
	}
	if req.Source != nil {
		d.source = &models.MCPProxySource{
			ComponentName: strings.TrimSpace(req.Source.ComponentName),
			EndpointName:  strings.TrimSpace(req.Source.EndpointName),
		}
	}
	if req.Upstream != nil {
		d.upstreamURL = strings.TrimSpace(req.Upstream.URL)
	}
	if handle == "" || len(handle) > 100 || !mcpProxyHandleRe.MatchString(handle) {
		return d, fmt.Errorf("%w: proxy id must be kebab-case (lowercase letters, digits, single hyphens) and at most 100 characters", utils.ErrInvalidInput)
	}
	if d.name == "" {
		return d, fmt.Errorf("%w: name is required", utils.ErrInvalidInput)
	}
	switch {
	case d.source != nil && req.Upstream != nil:
		return d, fmt.Errorf("%w: set either source or upstream, not both", utils.ErrInvalidInput)
	case d.source != nil:
		if d.source.ComponentName == "" || d.source.EndpointName == "" {
			return d, fmt.Errorf("%w: source.componentName and source.endpointName are required", utils.ErrInvalidInput)
		}
	case req.Upstream != nil:
		if _, err := ssrf.ValidateURLShape(d.upstreamURL); err != nil {
			return d, fmt.Errorf("%w: upstream.url: %w", utils.ErrInvalidInput, err)
		}
	default:
		return d, fmt.Errorf("%w: source or upstream is required", utils.ErrInvalidInput)
	}
	if d.version == "" {
		d.version = defaultProjectMCPProxyVersion
	}
	if d.context == "" {
		d.context = "/" + handle
	} else if !strings.HasPrefix(d.context, "/") {
		return d, fmt.Errorf("%w: context must start with \"/\"", utils.ErrInvalidInput)
	}
	// The gateway would resolve ., .. or an empty segment to another path than the one stored and checked for
	// clashes, so the context must be a clean path.
	for _, segment := range strings.Split(strings.TrimPrefix(d.context, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return d, fmt.Errorf("%w: context must not have empty, \".\" or \"..\" segments", utils.ErrInvalidInput)
		}
	}
	return d, nil
}

func (s *ProjectMCPProxyService) create(
	ctx context.Context, ouID, projectName, projectUUID, handle string, d desiredProjectMCPProxy,
) (*models.MCPProxy, error) {
	ctxPath := d.context
	proxy := &models.MCPProxy{
		Description: d.description,
		CreatedBy:   projectMCPProxyCreatedBy,
		Status:      models.StatusCreated,
		Configuration: models.MCPProxyConfig{
			Name:        d.name,
			Version:     d.version,
			Context:     &ctxPath,
			SpecVersion: d.specVersion,
		},
	}
	d.applyUpstream(&proxy.Configuration)
	// A project proxy has no endpoint until it is bound: each environment binding is its own endpoint row
	// (PutEnvironment), since the upstream and security differ per environment.
	artifactName := models.MCPProxyArtifactName(&projectName, handle, d.name)

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
// proxy writes nothing. A non-empty adoptInto is the UUID of the project adopting the proxy
// (a re-created project taking over what its deleted predecessor left): the proxy is moved
// to it in the same transaction as the update, even when the body changes nothing else.
func (s *ProjectMCPProxyService) update(
	ctx context.Context, ouID string, existing *models.MCPProxy, d desiredProjectMCPProxy, adoptInto string,
) (*models.ProjectMCPProxyResponse, error) {
	cfg := existing.Configuration
	storedURL := projectMCPProxyUpstreamURL(existing)
	switch {
	case cfg.Source != nil && (d.source == nil || *cfg.Source != *d.source):
		return nil, fmt.Errorf("%w: source", utils.ErrImmutableFieldChange)
	case cfg.Source == nil && storedURL != "" && d.source != nil:
		// A proxy registered by URL stays one; its URL may change.
		return nil, fmt.Errorf("%w: upstream (a proxy registered by URL cannot take a source)", utils.ErrImmutableFieldChange)
	}
	adopting := adoptInto != "" && existing.ProjectUUID != nil && *existing.ProjectUUID != adoptInto

	storedContext := ""
	if cfg.Context != nil {
		storedContext = *cfg.Context
	}
	unchanged := (cfg.Source != nil || storedURL != "") && storedURL == d.upstreamURL &&
		cfg.Name == d.name && cfg.Version == d.version && storedContext == d.context &&
		cfg.SpecVersion == d.specVersion && existing.Description == d.description
	if unchanged && !adopting {
		return projectMCPProxyResponse(existing), nil
	}

	updated := *existing
	ctxPath := d.context
	updated.Description = d.description
	updated.Configuration.Name = d.name
	updated.Configuration.Version = d.version
	updated.Configuration.Context = &ctxPath
	updated.Configuration.SpecVersion = d.specVersion
	d.applyUpstream(&updated.Configuration)

	if err := s.base.tx(ctx, func(tx *gorm.DB) error {
		if adopting {
			if err := s.base.repo.ReassignProject(ctx, tx, existing.UUID, ouID,
				valueOrEmpty(existing.ProjectName), *existing.ProjectUUID, adoptInto); err != nil {
				return err
			}
		}
		if unchanged {
			return nil
		}
		if storedURL != d.upstreamURL {
			if err := s.updateBindingUpstreams(ctx, tx, existing, d.upstreamURL); err != nil {
				return err
			}
		}
		if mcpGatewayContext(&updated, "") != mcpGatewayContext(existing, "") {
			if err := s.checkContextChange(ctx, tx, ouID, existing, mcpGatewayContext(&updated, "")); err != nil {
				return err
			}
		}
		return s.base.repo.Update(ctx, tx, &updated, ouID)
	}); err != nil {
		if errors.Is(err, utils.ErrMCPContextInUse) || errors.Is(err, utils.ErrMCPProxyNotFound) ||
			errors.Is(err, utils.ErrInvalidInput) {
			return nil, err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMCPProxyNotFound
		}
		if mapped := s.base.mapMCPProxyWriteError(err, proxyHandleOf(existing), ouID); mapped != nil {
			return nil, mapped
		}
		return nil, fmt.Errorf("failed to update MCP proxy: %w", err)
	}
	if adopting {
		updated.ProjectUUID = &adoptInto
	}
	if unchanged {
		// Only the project moved, and no gateway artifact carries it.
		return projectMCPProxyResponse(&updated), nil
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

// checkContextChange refuses (utils.ErrMCPContextInUse) a new gateway context for the proxy that would clash on a
// gateway one of its bindings is placed on, before the change is stored: the bindings' re-renders would otherwise
// fail while the proxy reads back a context its gateways don't serve. The gateways are locked in tx in UUID order,
// before the proxy row is written, as a binding deploy locks its gateway first.
func (s *ProjectMCPProxyService) checkContextChange(
	ctx context.Context, tx *gorm.DB, ouID string, proxy *models.MCPProxy, gatewayContext string,
) error {
	if s.base.deploymentRepo == nil || s.base.gatewayRepo == nil {
		return nil
	}
	placed := map[uuid.UUID][]uuid.UUID{}
	for _, artifact := range mcpProxyArtifacts(proxy, ouID) {
		gateways, err := s.base.deploymentRepo.PlacedGateways(ctx, ouID, artifact.ArtifactUUID)
		if err != nil {
			return fmt.Errorf("failed to list the gateways the binding is placed on: %w", err)
		}
		for _, gw := range gateways {
			gateway, err := uuid.Parse(gw)
			if err != nil {
				return fmt.Errorf("invalid gateway id %q: %w", gw, err)
			}
			placed[gateway] = append(placed[gateway], artifact.ArtifactUUID)
		}
	}
	gateways := slices.SortedFunc(maps.Keys(placed), func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	for _, gateway := range gateways {
		if err := s.base.gatewayRepo.AcquireGatewayLock(tx, gateway.String()); err != nil {
			return fmt.Errorf("failed to lock the gateway: %w", err)
		}
	}
	for _, gateway := range gateways {
		for _, artifact := range placed[gateway] {
			if err := s.checkBindingContext(ctx, ouID, gateway, artifact, gatewayContext, proxy.Configuration.Vhost); err != nil {
				return err
			}
		}
	}
	return nil
}

func handleInUseError(handle string) error {
	return fmt.Errorf("%w: proxy id %q", utils.ErrMCPProxyHandleInUse, handle)
}

// applyUpstream sets what the proxy fronts on cfg: its source, or its public upstream URL.
func (d desiredProjectMCPProxy) applyUpstream(cfg *models.MCPProxyConfig) {
	if d.source != nil {
		source := *d.source
		cfg.Source = &source
		cfg.Upstream = models.UpstreamConfig{}
		return
	}
	cfg.Source = nil
	cfg.Upstream = models.UpstreamConfig{Main: &models.UpstreamEndpoint{URL: d.upstreamURL}}
}

// updateBindingUpstreams points every binding of a proxy registered by URL at its new URL, in tx. The bindings are
// re-rendered once the proxy is stored (rerenderBindings).
func (s *ProjectMCPProxyService) updateBindingUpstreams(
	ctx context.Context, tx *gorm.DB, proxy *models.MCPProxy, upstreamURL string,
) error {
	for i := range proxy.Endpoints {
		endpoint := proxy.Endpoints[i]
		if endpoint.Configuration.Upstream == nil {
			continue
		}
		upstream := *endpoint.Configuration.Upstream
		// The stored credential is sent to the URL, so it may not follow the URL to another origin.
		if upstream.Auth != nil && !sameURLOrigin(upstream.URL, upstreamURL) {
			return fmt.Errorf("%w: upstream.url moves to another origin while a binding has upstream auth; "+
				"clear its upstreamAuth first", utils.ErrInvalidInput)
		}
		upstream.URL = upstreamURL
		endpoint.Configuration.Upstream = &upstream
		if err := s.base.endpointRepo.UpdateEndpoint(ctx, tx, &endpoint); err != nil {
			return fmt.Errorf("failed to update the MCP proxy binding's upstream: %w", err)
		}
	}
	return nil
}

// sameURLOrigin reports whether two URLs share scheme, host and port; a URL that does not parse matches nothing.
func sameURLOrigin(a, b string) bool {
	ua, errA := url.Parse(strings.TrimSpace(a))
	ub, errB := url.Parse(strings.TrimSpace(b))
	if errA != nil || errB != nil || ua.Host == "" || ub.Host == "" {
		return false
	}
	return isSameOrigin(ua, ub)
}

// projectMCPProxySource is the component endpoint the proxy fronts, or nil for a proxy registered by URL.
func projectMCPProxySource(proxy *models.MCPProxy) *models.MCPProxySource {
	if proxy.Configuration.Source == nil {
		return nil
	}
	source := *proxy.Configuration.Source
	return &source
}

// projectMCPProxyUpstreamURL is the public URL a proxy registered by URL fronts, or "" for a sourced proxy.
func projectMCPProxyUpstreamURL(proxy *models.MCPProxy) string {
	if proxy.Configuration.Source != nil || proxy.Configuration.Upstream.Main == nil {
		return ""
	}
	return strings.TrimSpace(proxy.Configuration.Upstream.Main.URL)
}

func projectMCPProxyUpstream(proxy *models.MCPProxy) *models.ProjectMCPProxyUpstream {
	if url := projectMCPProxyUpstreamURL(proxy); url != "" {
		return &models.ProjectMCPProxyUpstream{URL: url}
	}
	return nil
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
		Upstream:       projectMCPProxyUpstream(proxy),
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
		Upstream:     projectMCPProxyUpstream(proxy),
		Environments: []models.ProjectMCPProxyEnvironmentSummary{},
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}
}
