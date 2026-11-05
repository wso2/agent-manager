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
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// mcpUpstreamPath is the path an MCP server serves Streamable HTTP on when its endpoint declares none. The gateway
// forwards to exactly the upstream URL's path, so the deployed upstream must carry it.
const mcpUpstreamPath = "/mcp"

// mcpUpstreamURL is the gateway upstream for an MCP server reachable at serviceURL. The service URL already carries
// the endpoint's own path when it declares one, and that path is the MCP path; /mcp is added only when it has none.
func mcpUpstreamURL(serviceURL string) string {
	trimmed := strings.TrimRight(serviceURL, "/")
	if u, err := url.Parse(trimmed); err == nil && u.Path != "" {
		return trimmed
	}
	return trimmed + mcpUpstreamPath
}

// A project proxy's environment binding is its own mcp_proxy_endpoints row (handle and name are the
// environment name) carrying exactly that environment's mcp_proxy_endpoint_environments row; a proxy has one
// endpoint per binding and none before its first. The endpoint configuration is what differs per environment (the
// resolved in-cluster upstream, security, policies and upstream auth), and every reader that resolves a proxy's
// config for an environment (resolveMCPEndpointForEnv) already goes through the endpoint bound to it, so agents
// and the gateway read the binding's own config.

// PutEnvironment binds the project proxy to envName, or re-binds it, and deploys it. created reports a new
// binding. In order (spec §3): (1) resolve the upstream from the source component's release binding,
// (2) pick the gateway, (3) validate security, (4) check the gateway context against every other artifact on that
// gateway, (5) store, render and deploy in one transaction. Steps 1-4 store nothing when they refuse.
//
// An identical PUT on a Deployed or Deploying binding is a no-op, returned before step 4, so a binding that is
// live is never refused over an artifact that came to overlap it; on a Failed binding it re-queues the deploy.
func (s *ProjectMCPProxyService) PutEnvironment(
	ctx context.Context, ouID, projectName, projectUUID, handle, envName string, req models.MCPProxyEnvironmentRequest,
) (*models.MCPProxyEnvironmentResponse, bool, error) {
	if projectName == "" {
		return nil, false, fmt.Errorf("%w: project is required", utils.ErrInvalidInput)
	}
	proxy, err := s.getInProject(ctx, ouID, projectUUID, handle)
	if err != nil {
		return nil, false, err
	}
	envName = strings.TrimSpace(envName)
	envUUID, err := s.resolveEnvironment(ctx, ouID, envName)
	if err != nil {
		return nil, false, err
	}
	requestedGateway, err := requestedBindingGateway(req.GatewayID)
	if err != nil {
		return nil, false, err
	}
	source := projectMCPProxySource(proxy)
	releaseBindingName := strings.TrimSpace(valueOrEmpty(req.ReleaseBindingName))

	// (1) Upstream. AMS resolved this URL itself from OpenChoreo, so unlike a user-supplied upstream
	// (validateMCPEndpoints) it is not put through the SSRF check: it is an in-cluster service address by design.
	// A component deployed on several tracks has a release binding per track in the environment; releaseBindingName
	// picks one, and without it such a component is refused rather than one of its bindings picked at random.
	serviceURL, err := s.ocClient.GetReleaseBindingEndpointServiceURL(
		ctx, ouID, projectName, source.ComponentName, source.EndpointName, envName, releaseBindingName)
	switch {
	case errors.Is(err, occlient.ErrAmbiguousReleaseBinding):
		return nil, false, fmt.Errorf("%w: set releaseBindingName to pick one: %w", utils.ErrInvalidInput, err)
	case errors.Is(err, occlient.ErrReleaseBindingMismatch):
		return nil, false, fmt.Errorf("%w: releaseBindingName: %w", utils.ErrInvalidInput, err)
	case errors.Is(err, occlient.ErrReleaseBindingNotFound), errors.Is(err, occlient.ErrEndpointNotFound):
		return nil, false, fmt.Errorf("%w: %w", utils.ErrMCPUpstreamNotReady, err)
	case err != nil:
		return nil, false, fmt.Errorf("failed to resolve the source component's endpoint: %w", err)
	case strings.TrimSpace(serviceURL) == "":
		return nil, false, fmt.Errorf("%w: component %q endpoint %q has no service URL in %q yet",
			utils.ErrMCPUpstreamNotReady, source.ComponentName, source.EndpointName, envName)
	}
	upstreamURL := mcpUpstreamURL(strings.TrimSpace(serviceURL))

	endpoint, ee := resolveMCPEndpointForEnv(proxy, envUUID.String())
	exists := ee != nil
	selfArtifact := uuid.Nil
	if exists {
		selfArtifact = ee.ArtifactUUID
	}

	// (2) Gateway: the environment's egress gateway, anchored on where the binding is placed, as the MCP kind's
	// Targets picks it. None is not an error: the binding is stored and the placement pass deploys it later.
	gateway, err := s.bindingGateway(ctx, ouID, envUUID, selfArtifact, requestedGateway)
	if err != nil {
		return nil, false, err
	}

	// (3) Security, policies and upstream auth.
	security, err := normalizeBindingSecurity(req.Security)
	if err != nil {
		return nil, false, err
	}
	policies, err := normalizeBindingPolicies(req.Policies)
	if err != nil {
		return nil, false, err
	}
	auth, err := normalizeBindingUpstreamAuth(req.UpstreamAuth)
	if err != nil {
		return nil, false, err
	}

	desired := models.MCPEndpointConfig{
		Upstream:           &models.UpstreamEndpoint{URL: upstreamURL, Auth: auth},
		Policies:           policies,
		Security:           security,
		ReleaseBindingName: releaseBindingName,
	}
	if exists && s.sameBindingConfig(endpoint.Configuration, desired) {
		shown, err := s.shownStatus(ctx, ouID, ee.ArtifactUUID)
		if err != nil {
			return nil, false, err
		}
		status := ""
		if shown != nil {
			status = models.MCPDeploymentStatusOf(models.UserStatusOf(*shown))
		}
		live := status == models.MCPDeploymentStatusDeployed || status == models.MCPDeploymentStatusDeploying
		if live || (shown == nil && gateway == nil) {
			resp, err := s.environmentResponse(ctx, ouID, proxy, endpoint, ee)
			return resp, false, err
		}
	}

	// (4) Context, checked after the no-op: an identical PUT on a live binding is never refused. The deploy checks
	// again under the gateway lock; this check refuses before anything is stored.
	if gateway != nil {
		if err := s.checkBindingContext(ctx, ouID, *gateway, selfArtifact, mcpGatewayContext(proxy, projectName), proxy.Configuration.Vhost); err != nil {
			return nil, false, err
		}
	}

	// (5) Store, render and deploy in one transaction.
	stored, err := s.storeBindingConfig(desired)
	if err != nil {
		return nil, false, err
	}
	if err := s.base.tx(ctx, func(tx *gorm.DB) error {
		return s.saveAndDeployBinding(ctx, tx, ouID, proxyHandleOf(proxy), envName, envUUID, gateway, desired, stored)
	}); err != nil {
		switch {
		case errors.Is(err, utils.ErrMCPContextInUse), errors.Is(err, utils.ErrMCPProxyNotFound):
			return nil, false, err
		case errors.Is(err, errPlacementFixed):
			return nil, false, fmt.Errorf("%w: gatewayId: %w", utils.ErrImmutableFieldChange, err)
		case errors.Is(err, errAmbiguousEgressGateway):
			return nil, false, fmt.Errorf("%w: %w: %w", utils.ErrInvalidInput, utils.ErrMCPAmbiguousGateway, err)
		}
		if rejected := mcpDeployRejection(err); rejected != nil {
			return nil, false, rejected
		}
		return nil, false, fmt.Errorf("failed to save the MCP proxy binding: %w", err)
	}

	saved, err := s.getInProject(ctx, ouID, projectUUID, handle)
	if err != nil {
		return nil, false, err
	}
	endpoint, ee = resolveMCPEndpointForEnv(saved, envUUID.String())
	if ee == nil {
		return nil, false, fmt.Errorf("the saved binding of %q in %q is missing", handle, envName)
	}
	resp, err := s.environmentResponse(ctx, ouID, saved, endpoint, ee)
	return resp, !exists, err
}

// checkBindingContext refuses (utils.ErrMCPContextInUse) a binding whose gateway context, on its vhost or else the
// gateway's, overlaps another artifact's on gateway. self, the binding's own artifact, is excluded, and grandfathered
// when it is already deployed there at that route.
func (s *ProjectMCPProxyService) checkBindingContext(
	ctx context.Context, ouID string, gateway, self uuid.UUID, gatewayContext string, vhost *string,
) error {
	gw, err := s.base.gatewayRepo.GetByUUID(gateway.String())
	if err != nil {
		return fmt.Errorf("failed to read gateway %s: %w", gateway, err)
	}
	return reconcile.CheckContext(ctx, s.base.deploymentRepo, ouID, gw, self,
		reconcile.NewRoute(gatewayContext, valueOrEmpty(vhost), gw.Vhost))
}

// saveAndDeployBinding writes the binding in tx, the proxy row locked, and deploys it on gateway, or with no gateway
// only renders it (DeployRecord). A Failed binding with unchanged input is not rewritten, only re-deployed.
func (s *ProjectMCPProxyService) saveAndDeployBinding(
	ctx context.Context, tx *gorm.DB, ouID, proxyHandle, envName string, envUUID uuid.UUID, gateway *uuid.UUID,
	desired, stored models.MCPEndpointConfig,
) error {
	// Serialize binding writes on one gateway, taken first as the gateway mapping path does, so two bindings
	// with overlapping contexts can't both pass the context check before either deploys.
	if gateway != nil && s.base.gatewayRepo != nil {
		if err := s.base.gatewayRepo.AcquireGatewayLock(tx, gateway.String()); err != nil {
			return fmt.Errorf("failed to lock the gateway: %w", err)
		}
	}
	locked, err := s.base.repo.GetByHandleForUpdate(ctx, tx, proxyHandle, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrMCPProxyNotFound
		}
		return fmt.Errorf("failed to lock the MCP proxy: %w", err)
	}
	endpoint, ee := resolveMCPEndpointForEnv(locked, envUUID.String())
	var artifactUUID uuid.UUID
	var artifactHandle string
	if ee == nil {
		endpointHandle := bindingEndpointHandle(proxyHandle, envName)
		artifactUUID = uuid.New()
		artifactHandle = mcpProxyEnvArtifactHandle(proxyHandle, endpointHandle, envUUID.String())
		if err := s.createBinding(ctx, tx, ouID, locked, endpointHandle, envName, envUUID, artifactUUID, artifactHandle, stored); err != nil {
			return err
		}
	} else {
		artifactUUID = ee.ArtifactUUID
		artifactHandle = mcpProxyEnvArtifactHandle(proxyHandle, endpoint.Handle, envUUID.String())
		if !s.sameBindingConfig(endpoint.Configuration, desired) {
			if err := s.base.endpointRepo.UpdateEndpoint(ctx, tx, &models.MCPProxyEndpoint{
				UUID: endpoint.UUID, Handle: endpoint.Handle, Name: endpoint.Name, Status: endpoint.Status,
				Configuration: stored,
			}); err != nil {
				return fmt.Errorf("failed to update the MCP proxy binding: %w", err)
			}
		}
	}
	if s.base.deploymentRepo != nil {
		// The deploy re-checks the context under the gateway lock (reconcile.Core), so two bindings with
		// overlapping contexts can't both pass the check made before the store.
		if err := s.base.deploymentRepo.LockStatusRows(ctx, tx, ouID, []uuid.UUID{artifactUUID}); err != nil {
			return err
		}
	}
	return s.base.deployMCPArtifact(ctx, tx, &models.MCPProxy{UUID: artifactUUID, Handle: artifactHandle}, ouID, envUUID, gateway)
}

// createBinding inserts the binding's endpoint, its gateway artifact row and the (endpoint, environment) row.
func (s *ProjectMCPProxyService) createBinding(
	ctx context.Context, tx *gorm.DB, ouID string, proxy *models.MCPProxy, endpointHandle, envName string,
	envUUID, artifactUUID uuid.UUID, artifactHandle string, config models.MCPEndpointConfig,
) error {
	if len(artifactHandle) > maxMCPProxyDeploymentHandleLength {
		return fmt.Errorf("%w: proxy id and environment name produce a %d-character gateway deployment name, exceeding the limit of %d",
			utils.ErrInvalidInput, len(artifactHandle), maxMCPProxyDeploymentHandleLength)
	}
	now := time.Now()
	endpoint := orphanedBindingEndpoint(proxy, endpointHandle)
	if endpoint != nil {
		// An environment removed before its binding's endpoint was dropped with it left this row behind; it holds
		// the handle (uq_mcp_endpoint_handle), so the new binding takes it over.
		if err := s.base.endpointRepo.UpdateEndpoint(ctx, tx, &models.MCPProxyEndpoint{
			UUID: endpoint.UUID, Handle: endpoint.Handle, Name: envName, Status: endpoint.Status, Configuration: config,
		}); err != nil {
			return fmt.Errorf("failed to reuse the MCP proxy binding's endpoint: %w", err)
		}
	} else {
		endpoint = &models.MCPProxyEndpoint{
			UUID: uuid.New(), MCPProxyUUID: proxy.UUID, Handle: endpointHandle, Name: envName,
			Status: models.StatusCreated, Configuration: config, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.base.endpointRepo.CreateEndpoint(ctx, tx, endpoint); err != nil {
			return err
		}
	}
	version := proxy.Configuration.Version
	if proxy.Artifact != nil && proxy.Artifact.Version != "" {
		version = proxy.Artifact.Version
	}
	if err := s.base.artifactRepo.Create(tx, &models.Artifact{
		UUID: artifactUUID, Handle: artifactHandle, Name: artifactHandle, Version: version,
		Kind: models.KindMCPMapping, OUID: ouID, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return fmt.Errorf("failed to create the MCP proxy binding artifact: %w", err)
	}
	return s.base.endpointRepo.AddEndpointEnvironment(ctx, tx, &models.MCPProxyEndpointEnvironment{
		MCPProxyUUID: proxy.UUID, EndpointUUID: endpoint.UUID, EnvironmentUUID: envUUID,
		ArtifactUUID: artifactUUID, Status: models.StatusCreated,
	})
}

// orphanedBindingEndpoint is the proxy's endpoint with handle that is bound to no environment, or nil.
func orphanedBindingEndpoint(proxy *models.MCPProxy, handle string) *models.MCPProxyEndpoint {
	for i := range proxy.Endpoints {
		if proxy.Endpoints[i].Handle == handle && len(proxy.Endpoints[i].Environments) == 0 {
			return &proxy.Endpoints[i]
		}
	}
	return nil
}

// GetEnvironment returns the project proxy's binding in envName, with its deployment status.
func (s *ProjectMCPProxyService) GetEnvironment(
	ctx context.Context, ouID, projectName, projectUUID, handle, envName string,
) (*models.MCPProxyEnvironmentResponse, error) {
	proxy, err := s.getInProject(ctx, ouID, projectUUID, handle)
	if err != nil {
		return nil, err
	}
	return s.environmentOf(ctx, ouID, proxy, envName)
}

// GetEnvironmentInDeletedProject is GetEnvironment for a project OpenChoreo no longer has: the binding of the proxy
// stored under projectName, whatever project UUID it carries.
func (s *ProjectMCPProxyService) GetEnvironmentInDeletedProject(
	ctx context.Context, ouID, projectName, handle, envName string,
) (*models.MCPProxyEnvironmentResponse, error) {
	proxy, err := s.getInDeletedProject(ctx, ouID, projectName, handle)
	if err != nil {
		return nil, err
	}
	return s.environmentOf(ctx, ouID, proxy, envName)
}

func (s *ProjectMCPProxyService) environmentOf(
	ctx context.Context, ouID string, proxy *models.MCPProxy, envName string,
) (*models.MCPProxyEnvironmentResponse, error) {
	endpoint, ee := bindingByEnvironmentName(proxy, strings.TrimSpace(envName))
	if ee == nil {
		return nil, utils.ErrMCPProxyEnvironmentNotBound
	}
	return s.environmentResponse(ctx, ouID, proxy, endpoint, ee)
}

// DeleteEnvironment drops the project proxy's binding in envName with its gateway artifact. MCP has no undeploy:
// the artifact is deleted, and its delete events go to every gateway it was sent, in the same transaction.
//
// It refuses while an AM agent is mapped to the proxy in that environment (ErrMCPProxyHasMappings, as the proxy
// delete does): the agent's keys were sent against this binding's artifact, and a later re-bind mints a new one
// they are never re-sent to. The proxy row is locked as PUT locks it, so a PUT and a DELETE on the same binding
// don't interleave.
func (s *ProjectMCPProxyService) DeleteEnvironment(ctx context.Context, ouID, projectName, projectUUID, handle, envName string) error {
	proxy, err := s.getInProject(ctx, ouID, projectUUID, handle)
	if err != nil {
		return err
	}
	return s.deleteEnvironmentOf(ctx, ouID, proxy, envName)
}

// DeleteEnvironmentInDeletedProject is DeleteEnvironment for a project OpenChoreo no longer has: it drops the binding
// of the proxy stored under projectName, whatever project UUID it carries, with the same teardown and in-use refusal.
func (s *ProjectMCPProxyService) DeleteEnvironmentInDeletedProject(ctx context.Context, ouID, projectName, handle, envName string) error {
	proxy, err := s.getInDeletedProject(ctx, ouID, projectName, handle)
	if err != nil {
		return err
	}
	return s.deleteEnvironmentOf(ctx, ouID, proxy, envName)
}

func (s *ProjectMCPProxyService) deleteEnvironmentOf(ctx context.Context, ouID string, proxy *models.MCPProxy, envName string) error {
	envName = strings.TrimSpace(envName)
	_, ee := bindingByEnvironmentName(proxy, envName)
	if ee == nil {
		return utils.ErrMCPProxyEnvironmentNotBound
	}
	if err := s.ensureBindingNotInUse(ctx, proxy.UUID, ee.EnvironmentUUID); err != nil {
		return err
	}
	return s.base.tx(ctx, func(tx *gorm.DB) error {
		locked, err := s.base.repo.GetByHandleForUpdate(ctx, tx, proxyHandleOf(proxy), ouID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return utils.ErrMCPProxyNotFound
			}
			return fmt.Errorf("failed to lock the MCP proxy: %w", err)
		}
		// Re-read under the lock: a concurrent DELETE may have dropped it, or a PUT re-created it.
		endpoint, ee := bindingByEnvironmentName(locked, envName)
		if ee == nil {
			return utils.ErrMCPProxyEnvironmentNotBound
		}
		ref := models.ArtifactRef{OUID: ouID, ArtifactUUID: ee.ArtifactUUID, Kind: models.KindMCPMapping}
		events, err := s.base.core.PrepareDelete(ctx, tx, ref)
		if err != nil {
			return err
		}
		// Deleting the artifact cascades the (endpoint, environment) row.
		if err := deleteMCPArtifactRows(tx, ee.ArtifactUUID, ouID); err != nil {
			return fmt.Errorf("failed to delete the MCP proxy binding artifact: %w", err)
		}
		if err := s.base.endpointRepo.DeleteEndpoint(ctx, tx, endpoint.UUID); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return events.Publish(ctx, tx)
	})
}

// ensureBindingNotInUse refuses with ErrMCPProxyHasMappings when an agent is mapped to the proxy in envUUID: the
// proxy delete's mapping check (ensureMCPProxyNotInUse), scoped to the one environment. The proxy delete's second
// check, agent_mcp_config_proxy, is environment-independent: a connection that holds only that row has no keys on
// this binding, and the binding reconcile completes it once the proxy is bound again, so it does not hold a binding.
func (s *ProjectMCPProxyService) ensureBindingNotInUse(ctx context.Context, proxyUUID, envUUID uuid.UUID) error {
	if s.base.envMCPMappingRepo == nil {
		return nil
	}
	mappings, err := s.base.envMCPMappingRepo.ListByMCPProxy(ctx, proxyUUID)
	if err != nil {
		return fmt.Errorf("failed to list MCP mappings before deleting the binding: %w", err)
	}
	for i := range mappings {
		if mappings[i].EnvironmentUUID == envUUID {
			return utils.ErrMCPProxyHasMappings
		}
	}
	return nil
}

// bindingByEnvironmentName is the proxy's binding in the environment named envName: the endpoint named after it
// that carries its one environment row.
func bindingByEnvironmentName(proxy *models.MCPProxy, envName string) (*models.MCPProxyEndpoint, *models.MCPProxyEndpointEnvironment) {
	if envName == "" {
		return nil, nil
	}
	for i := range proxy.Endpoints {
		endpoint := &proxy.Endpoints[i]
		if endpoint.Name == envName && len(endpoint.Environments) == 1 {
			return endpoint, &endpoint.Environments[0]
		}
	}
	return nil, nil
}

// bindingEndpointHandle is the endpoint handle of the binding in envName. It stays clear of the proxy handle, which
// endpoint rows created before bindings had their own endpoints used.
func bindingEndpointHandle(proxyHandle, envName string) string {
	if envName == proxyHandle {
		return envName + "-env"
	}
	return envName
}

func (s *ProjectMCPProxyService) resolveEnvironment(ctx context.Context, ouID, envName string) (uuid.UUID, error) {
	if envName == "" {
		return uuid.Nil, utils.ErrEnvironmentNotFound
	}
	env, err := s.ocClient.GetEnvironment(ctx, ouID, envName)
	if errors.Is(err, utils.ErrNotFound) || errors.Is(err, utils.ErrEnvironmentNotFound) || (err == nil && env == nil) {
		return uuid.Nil, fmt.Errorf("%w: %q", utils.ErrEnvironmentNotFound, envName)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to get environment %q: %w", envName, err)
	}
	envUUID, err := uuid.Parse(env.UUID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("environment %q has an invalid id %q: %w", envName, env.UUID, err)
	}
	return envUUID, nil
}

func requestedBindingGateway(gatewayID *string) (*string, error) {
	if gatewayID == nil || strings.TrimSpace(*gatewayID) == "" {
		return nil, nil //nolint:nilnil // no gateway requested
	}
	id := strings.TrimSpace(*gatewayID)
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("%w: gatewayId must be a UUID", utils.ErrInvalidInput)
	}
	return &id, nil
}

// bindingGateway is the gateway the binding deploys on, or nil when its environment has none designated for egress,
// which the MCP kind's Targets also treats as no gateway.
func (s *ProjectMCPProxyService) bindingGateway(
	ctx context.Context, ouID string, envUUID, artifactUUID uuid.UUID, requested *string,
) (*uuid.UUID, error) {
	var placed []string
	if artifactUUID != uuid.Nil && s.base.deploymentRepo != nil {
		ids, err := s.base.deploymentRepo.PlacedGateways(ctx, ouID, artifactUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to list the binding's placements: %w", err)
		}
		placed = ids
	}
	gateway, err := resolveEgressGatewayForArtifact(s.base.gatewayRepo, ouID, envUUID, placed, requested)
	switch {
	case errors.Is(err, errNoGatewayForEnvironment), errors.Is(err, errNoEgressGatewayForEnvironment):
		return nil, nil //nolint:nilnil // no gateway: stored, placed later
	case errors.Is(err, errPlacementFixed):
		return nil, fmt.Errorf("%w: gatewayId: %w", utils.ErrImmutableFieldChange, err)
	case errors.Is(err, errAmbiguousEgressGateway):
		return nil, fmt.Errorf("%w: %w: %w", utils.ErrInvalidInput, utils.ErrMCPAmbiguousGateway, err)
	case mcpDeployErrorIsFatal(err):
		return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
	case err != nil:
		return nil, fmt.Errorf("failed to resolve the environment's gateway: %w", err)
	}
	// A self-hosted gateway is customer-run: it has no route to the in-cluster upstream, and must not be sent the
	// internal hostname or the upstream auth. The environment never infers one, so only a named one, or a placement
	// on one, gets here.
	if !gateway.IsManaged() {
		return nil, fmt.Errorf("%w: gatewayId: project MCP proxies deploy only on managed gateways, and %s is self-hosted",
			utils.ErrInvalidInput, gateway.UUID)
	}
	return &gateway.UUID, nil
}

// normalizeBindingSecurity is the binding's security with its defaults applied. Omitted, it is apiKey in an
// X-API-Key header. Present, it is apiKey when that variant is given and not disabled, and no security when
// security.enabled is false or no variant is enabled (the contract's "none"). Phase 1 refuses jwt and identity.
func normalizeBindingSecurity(in *models.MCPProxyEnvironmentSecurity) (*models.SecurityConfig, error) {
	if in == nil {
		return defaultMCPProxySecurity(nil), nil
	}
	if in.JWT != nil && !isExplicitlyFalse(in.JWT.Enabled) {
		return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, utils.ErrMCPJWTSecurityNotAvailable)
	}
	if in.Identity != nil && !isExplicitlyFalse(in.Identity.Enabled) {
		return nil, fmt.Errorf("%w: identity security is not available on project MCP proxies", utils.ErrInvalidInput)
	}
	// Only an explicit enabled false turns security off. Security left on with no variant defaults to the api
	// key, as an omitted security does, so a caller asking for security never gets an open endpoint.
	if isExplicitlyFalse(in.Enabled) {
		disabled := false
		return &models.SecurityConfig{Enabled: &disabled}, nil
	}
	if in.APIKey != nil && isExplicitlyFalse(in.APIKey.Enabled) {
		return nil, fmt.Errorf("%w: security is enabled but no variant is; set security.enabled to false to turn it off",
			utils.ErrInvalidInput)
	}
	var apiKey *models.APIKeySecurity
	if in.APIKey != nil {
		copied := *in.APIKey
		copied.Key = strings.TrimSpace(copied.Key)
		copied.In = strings.ToLower(strings.TrimSpace(copied.In))
		apiKey = &copied
	}
	security := defaultMCPProxySecurity(&models.SecurityConfig{Enabled: in.Enabled, APIKey: apiKey})
	if security.APIKey.In != "header" && security.APIKey.In != "query" {
		return nil, fmt.Errorf("%w: security.apiKey.in must be header or query", utils.ErrInvalidInput)
	}
	return security, nil
}

func isExplicitlyFalse(v *bool) bool {
	return v != nil && !*v
}

func normalizeBindingPolicies(in []models.MCPPolicy) ([]models.MCPPolicy, error) {
	if len(in) == 0 {
		return nil, nil
	}
	for i, policy := range in {
		if strings.TrimSpace(policy.Name) == "" || strings.TrimSpace(policy.Version) == "" {
			return nil, fmt.Errorf("%w: policies[%d] needs a name and a version", utils.ErrInvalidInput, i)
		}
	}
	return copyMCPPolicies(in), nil
}

// normalizeBindingUpstreamAuth is the requested upstream auth, its value still in plain text; nil for none.
func normalizeBindingUpstreamAuth(in *models.MCPProxyEnvironmentUpstreamAuth) (*models.UpstreamAuth, error) {
	if in == nil {
		return nil, nil //nolint:nilnil // no upstream auth
	}
	switch typ := strings.ToLower(strings.TrimSpace(in.Type)); typ {
	case "", "none":
		return nil, nil //nolint:nilnil // no upstream auth
	case "api-key":
		header, value := strings.TrimSpace(in.Header), in.Value
		if header == "" || value == "" {
			return nil, fmt.Errorf("%w: upstreamAuth needs both header and value", utils.ErrInvalidInput)
		}
		return &models.UpstreamAuth{Type: &typ, Header: &header, Value: &value}, nil
	default:
		return nil, fmt.Errorf("%w: upstreamAuth.type must be api-key or none", utils.ErrInvalidInput)
	}
}

// storeBindingConfig is desired as it is stored: the upstream credential encrypted.
func (s *ProjectMCPProxyService) storeBindingConfig(desired models.MCPEndpointConfig) (models.MCPEndpointConfig, error) {
	stored := desired
	if desired.Upstream == nil || desired.Upstream.Auth == nil || desired.Upstream.Auth.Value == nil {
		return stored, nil
	}
	encoded, err := utils.EncryptSecretRef(*desired.Upstream.Auth.Value, s.base.encryptionKey)
	if err != nil {
		return models.MCPEndpointConfig{}, fmt.Errorf("failed to encrypt upstream auth: %w", err)
	}
	auth := *desired.Upstream.Auth
	auth.Value, auth.SecretRef = nil, &encoded
	upstream := *desired.Upstream
	upstream.Auth = &auth
	stored.Upstream = &upstream
	return stored, nil
}

// sameBindingConfig reports whether the stored binding config already is desired, comparing the stored credential
// by its decrypted value.
func (s *ProjectMCPProxyService) sameBindingConfig(stored, desired models.MCPEndpointConfig) bool {
	storedURL, desiredURL := "", ""
	var storedAuth, desiredAuth *models.UpstreamAuth
	if stored.Upstream != nil {
		storedURL, storedAuth = stored.Upstream.URL, stored.Upstream.Auth
	}
	if desired.Upstream != nil {
		desiredURL, desiredAuth = desired.Upstream.URL, desired.Upstream.Auth
	}
	return storedURL == desiredURL &&
		stored.ReleaseBindingName == desired.ReleaseBindingName &&
		s.sameUpstreamAuth(storedAuth, desiredAuth) &&
		sameJSON(stored.Security, desired.Security) &&
		sameJSON(nilIfEmptyPolicies(stored.Policies), nilIfEmptyPolicies(desired.Policies)) &&
		stored.Resilience == nil && stored.Capabilities == nil
}

func (s *ProjectMCPProxyService) sameUpstreamAuth(stored, desired *models.UpstreamAuth) bool {
	if stored == nil || desired == nil {
		return stored == nil && desired == nil
	}
	if valueOrEmpty(stored.Type) != valueOrEmpty(desired.Type) || valueOrEmpty(stored.Header) != valueOrEmpty(desired.Header) {
		return false
	}
	storedValue := valueOrEmpty(stored.Value)
	if stored.SecretRef != nil {
		decrypted, err := utils.DecryptSecretRef(*stored.SecretRef, s.base.encryptionKey)
		if err != nil {
			return false
		}
		storedValue = decrypted
	}
	return storedValue == valueOrEmpty(desired.Value)
}

func nilIfEmptyPolicies(policies []models.MCPPolicy) []models.MCPPolicy {
	if len(policies) == 0 {
		return nil
	}
	return policies
}

func sameJSON(a, b any) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(ja) == string(jb)
}

// shownStatus is the binding's status row a reader is shown, or nil when it has none (no gateway yet).
func (s *ProjectMCPProxyService) shownStatus(ctx context.Context, ouID string, artifactUUID uuid.UUID) (*models.DeploymentStatusRecord, error) {
	if s.base.deploymentRepo == nil {
		return nil, nil //nolint:nilnil // no deployment tracking
	}
	rows, err := s.base.deploymentRepo.StatusRecords(ctx, ouID, artifactUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to read the binding's deployment status: %w", err)
	}
	return models.ShownStatusRecord(rows), nil
}

// environmentResponse is the binding as returned: status in MCP's words from its status row, as the org Get
// reads it; NO_GATEWAY with no status when it has no row.
func (s *ProjectMCPProxyService) environmentResponse(
	ctx context.Context, ouID string, proxy *models.MCPProxy, endpoint *models.MCPProxyEndpoint, ee *models.MCPProxyEndpointEnvironment,
) (*models.MCPProxyEnvironmentResponse, error) {
	cfg := endpoint.Configuration
	resp := &models.MCPProxyEnvironmentResponse{
		EnvironmentName:    endpoint.Name,
		EnvironmentUUID:    ee.EnvironmentUUID.String(),
		Context:            valueOrEmpty(proxy.Configuration.Context),
		ReleaseBindingName: cfg.ReleaseBindingName,
		Security:           bindingSecurityResponse(cfg.Security),
		Policies:           copyMCPPolicies(cfg.Policies),
		UpdatedAt:          timePtrIfNotZero(endpoint.UpdatedAt),
	}
	if cfg.Upstream != nil && cfg.Upstream.Auth != nil {
		resp.UpstreamAuth = &models.MCPProxyEnvironmentUpstreamAuthRedacted{
			Type: valueOrEmpty(cfg.Upstream.Auth.Type), Header: valueOrEmpty(cfg.Upstream.Auth.Header),
		}
	}
	shown, err := s.shownStatus(ctx, ouID, ee.ArtifactUUID)
	if err != nil {
		return nil, err
	}
	if shown == nil {
		resp.DeploymentStatusReason = models.MCPDeploymentStatusReasonNoGateway
		return resp, nil
	}
	status := models.UserStatusOf(*shown)
	resp.DeploymentStatus = models.MCPDeploymentStatusOf(status)
	resp.DeploymentStatusReason = status.Reason
	resp.GatewayID = shown.GatewayUUID.String()
	if s.base.gatewayRepo == nil {
		return resp, nil
	}
	gateway, err := s.base.gatewayRepo.GetByUUID(resp.GatewayID)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, utils.ErrGatewayNotFound):
		return resp, nil
	case err != nil:
		return nil, fmt.Errorf("failed to get the binding's gateway: %w", err)
	}
	if gateway != nil && strings.TrimSpace(gateway.Vhost) != "" {
		gatewayContext := mcpGatewayContext(proxy, "")
		resp.PublicURL = buildMCPProxyURL(gateway, models.MCPProxyConfig{Context: &gatewayContext, Vhost: proxy.Configuration.Vhost})
	}
	return resp, nil
}

func bindingSecurityResponse(security *models.SecurityConfig) *models.MCPProxyEnvironmentSecurity {
	if security == nil {
		return nil
	}
	out := &models.MCPProxyEnvironmentSecurity{Enabled: security.Enabled}
	if security.APIKey != nil {
		apiKey := *security.APIKey
		out.APIKey = &apiKey
	}
	return out
}

// environments are the proxy's bindings as returned, in endpoint order.
func (s *ProjectMCPProxyService) environments(ctx context.Context, ouID string, proxy *models.MCPProxy) ([]models.MCPProxyEnvironmentResponse, error) {
	out := []models.MCPProxyEnvironmentResponse{}
	for i := range proxy.Endpoints {
		endpoint := &proxy.Endpoints[i]
		for j := range endpoint.Environments {
			resp, err := s.environmentResponse(ctx, ouID, proxy, endpoint, &endpoint.Environments[j])
			if err != nil {
				return nil, err
			}
			out = append(out, *resp)
		}
	}
	return out, nil
}

// rerenderBindings re-renders the proxy's bound environments after the proxy changed (its name, context or
// version reach the gateway artifact). Best-effort: a binding whose render fails shows it as its status.
func (s *ProjectMCPProxyService) rerenderBindings(ctx context.Context, ouID string, proxy *models.MCPProxy) {
	if s.base.core == nil {
		return
	}
	for _, artifact := range mcpProxyArtifacts(proxy, ouID) {
		if err := s.base.core.MarkStale(ctx, artifact.Kind, ouID, artifact.ArtifactUUID, nil, "the MCP proxy changed"); err != nil {
			s.base.logger.Warn("Failed to redeploy a project MCP proxy binding after the proxy changed",
				"proxyUUID", proxy.UUID, "artifactUUID", artifact.ArtifactUUID, "ouID", ouID, "error", err)
		}
	}
}
