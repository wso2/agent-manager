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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/utils/ssrf"
)

// mcpProxyHandleRe constrains proxy handles to kebab-case. The handle becomes the
// Thunder resource-server handle (delimiter ":"), so it must never contain ":" and
// must fit Thunder's 100-char handle cap; kebab is a strict subset of both.
var mcpProxyHandleRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// maxMCPProxyDeploymentHandleLength bounds the per-(endpoint,environment) gateway artifact
// name/displayName that deployment builds via mcpProxyEnvArtifactHandle. The gateway rejects a
// displayName outside 1-100 chars, and that rejection happens asynchronously after Create/Update
// has already returned 201/200, so a too-long handle deploys nothing while the caller sees
// success — surfacing later only as 404s on the MCP route. Validate it up front instead and fail
// with a clear, actionable error.
const maxMCPProxyDeploymentHandleLength = 100

const (
	mcpJSONRPCVersion      = "2.0"
	mcpProtocolVersion     = "2025-06-18"
	mcpMethodInitialize    = "initialize"
	mcpMethodInitialized   = "notifications/initialized"
	mcpMethodToolsList     = "tools/list"
	mcpMethodPromptsList   = "prompts/list"
	mcpMethodResourcesList = "resources/list"
	mcpClientName          = "agent-manager-mcp-client"
	mcpClientVersion       = "1.0.0"
	mcpSessionHeader       = "Mcp-Session-Id"
	mcpRequestTimeout      = 10 * time.Second
	maxMCPResponseBody     = 10 << 20
)

// MCPProxyService handles MCP proxy operations.
type MCPProxyService struct {
	db                     *gorm.DB
	repo                   repositories.MCPProxyRepository
	endpointRepo           repositories.MCPProxyEndpointRepository
	deploymentRepo         repositories.DeploymentRepository
	gatewayRepo            repositories.GatewayRepository
	envMCPMappingRepo      repositories.EnvAgentMCPMappingRepository
	artifactRepo           repositories.ArtifactRepository
	agentConfigRepo        repositories.AgentConfigurationRepository
	mcpProxyScopeRepo      repositories.MCPProxyScopeRepository
	gatewayEventsService   *GatewayEventsService
	apiKeyBroadcaster      apiKeyBroadcaster
	infraManager           InfraResourceManager
	agentIdentityInjection AgentIdentityInjectionService
	client                 *http.Client
	logger                 *slog.Logger
	encryptionKey          []byte
	resolver               thundersvc.EnvThunderResolver
}

// NewMCPProxyService creates a new MCP proxy service.
func NewMCPProxyService(
	db *gorm.DB,
	repo repositories.MCPProxyRepository,
	endpointRepo repositories.MCPProxyEndpointRepository,
	deploymentRepo repositories.DeploymentRepository,
	gatewayRepo repositories.GatewayRepository,
	envMCPMappingRepo repositories.EnvAgentMCPMappingRepository,
	agentConfigRepo repositories.AgentConfigurationRepository,
	gatewayEventsService *GatewayEventsService,
	apiKeyRepo repositories.APIKeyRepository,
	infraManager InfraResourceManager,
	agentIdentityInjection AgentIdentityInjectionService,
	logger *slog.Logger,
	encryptionKey []byte,
	mcpProxyScopeRepo repositories.MCPProxyScopeRepository,
	resolver thundersvc.EnvThunderResolver,
) *MCPProxyService {
	return &MCPProxyService{
		db:                   db,
		repo:                 repo,
		endpointRepo:         endpointRepo,
		deploymentRepo:       deploymentRepo,
		gatewayRepo:          gatewayRepo,
		envMCPMappingRepo:    envMCPMappingRepo,
		agentConfigRepo:      agentConfigRepo,
		artifactRepo:         repositories.NewArtifactRepo(db),
		mcpProxyScopeRepo:    mcpProxyScopeRepo,
		gatewayEventsService: gatewayEventsService,
		apiKeyBroadcaster: apiKeyBroadcaster{
			gatewayRepo:    gatewayRepo,
			gatewayService: gatewayEventsService,
			apiKeyRepo:     apiKeyRepo,
		},
		infraManager:           infraManager,
		agentIdentityInjection: agentIdentityInjection,
		client:                 ssrf.NewClient(mcpRequestTimeout),
		logger:                 logger,
		encryptionKey:          encryptionKey,
		resolver:               resolver,
	}
}

// mcpDeployErrorIsFatal reports whether a deployMCPProxyEndpoints failure must fail the
// request. Fatal means one of the four placement errors — caller misconfiguration that
// must surface, or an invalid gatewayId returns 201 with nothing deployed. Everything
// else is non-fatal by design: errNoGatewayForEnvironment is a timing condition and
// unexpected infra errors are best-effort — both are logged and retried on the next
// update. Fatal errors take precedence in a join.
func mcpDeployErrorIsFatal(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, errNoEgressGatewayForEnvironment) ||
		errors.Is(err, errAmbiguousEgressGateway) ||
		errors.Is(err, errInvalidEgressGateway) ||
		errors.Is(err, errPlacementFixed)
}

// Create creates a new MCP proxy and its endpoints.
//
// Each endpoint is the deployable proxy definition; one endpoint deploys to 1..N
// environments. Within the proxy an environment maps to at most one endpoint
// (uq_proxy_env_single), so the (endpoint, env) rows carry the stable per-deployment
// gateway artifact identity that agents later reuse.
func (s *MCPProxyService) Create(ctx context.Context, orgUUID, createdBy string, req *models.MCPProxyDTO) (*models.MCPProxyDTO, error) {
	if req == nil {
		return nil, utils.ErrInvalidInput
	}

	handle := strings.TrimSpace(req.ID)
	name := strings.TrimSpace(req.Name)
	version := strings.TrimSpace(req.Version)
	if handle == "" || name == "" || version == "" {
		return nil, utils.ErrInvalidInput
	}

	if len(handle) > 100 || !mcpProxyHandleRe.MatchString(handle) {
		return nil, fmt.Errorf("%w: proxy id must be kebab-case (lowercase letters, digits, single hyphens) and at most 100 characters", utils.ErrInvalidInput)
	}

	if err := validateMCPEndpoints(ctx, req.Endpoints); err != nil {
		return nil, err
	}
	if err := validateMCPProxyDeploymentHandleLengths(handle, req.Endpoints); err != nil {
		return nil, err
	}
	if err := s.validateMCPEndpointSecurity(ctx, orgUUID, req.Endpoints, nil); err != nil {
		return nil, err
	}
	if err := s.validateMCPEndpointPlacement(orgUUID, req.Endpoints, nil); err != nil {
		return nil, err
	}
	endpoints, err := s.buildMCPEndpointsForStorage(req.Endpoints, nil)
	if err != nil {
		return nil, err
	}

	exists, err := s.repo.Exists(ctx, handle, orgUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to check MCP proxy existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("%w: proxy id %q is already in use", utils.ErrMCPProxyExists, handle)
	}

	proxy := &models.MCPProxy{
		Description: valueOrEmpty(req.Description),
		CreatedBy:   createdBy,
		Status:      models.StatusCreated,
		Configuration: models.MCPProxyConfig{
			Name:        name,
			Version:     version,
			Context:     req.Context,
			Vhost:       req.Vhost,
			SpecVersion: valueOrEmpty(req.McpSpecVersion),
		},
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.Create(ctx, tx, proxy, handle, name, version, orgUUID); err != nil {
			return err
		}
		return s.persistMCPEndpoints(ctx, tx, proxy.UUID, handle, version, endpoints, orgUUID)
	}); err != nil {
		if mapped := s.mapMCPProxyWriteError(err, handle, orgUUID); mapped != nil {
			return nil, mapped
		}
		return nil, fmt.Errorf("failed to create MCP proxy: %w", err)
	}

	created, err := s.repo.GetByHandle(ctx, handle, orgUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMCPProxyNotFound
		}
		return nil, fmt.Errorf("failed to retrieve MCP proxy: %w", err)
	}

	// Each endpoint deploys one gateway artifact per target environment, immediately on
	// creation. These are the only artifacts the proxy contributes to any gateway; agent
	// configurations that reference it deploy nothing and instead reuse the endpoint's
	// per-environment artifacts. Best-effort: an environment whose gateway is not yet
	// active is skipped and deploys on the next update.
	applyRequestedGatewayUUIDs(created, req.Endpoints)
	if err := s.deployMCPProxyEndpoints(ctx, created, orgUUID); err != nil {
		if mcpDeployErrorIsFatal(err) {
			return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
		}
		s.logger.Warn("Failed to deploy one or more MCP proxy endpoint artifacts", "proxy_id", created.UUID, "error", err)
	}
	return convertModelMCPProxyToSpec(created), nil
}

// List retrieves MCP proxies for an organization.
func (s *MCPProxyService) List(ctx context.Context, orgUUID string, limit, offset int) (*models.MCPProxyListResponse, error) {
	proxies, err := s.repo.List(ctx, orgUUID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP proxies: %w", err)
	}

	total, err := s.repo.Count(ctx, orgUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to count MCP proxies: %w", err)
	}

	resp := &models.MCPProxyListResponse{
		Count: len(proxies),
		List:  make([]models.MCPProxyListItem, 0, len(proxies)),
		Pagination: models.PaginationInfo{
			Count:  total,
			Limit:  limit,
			Offset: offset,
		},
	}
	for _, proxy := range proxies {
		resp.List = append(resp.List, convertModelMCPProxyToListItem(proxy))
	}
	return resp, nil
}

// ListAvailableMCPPolicies returns policy versions reported by active gateways in the organization.
func (s *MCPProxyService) ListAvailableMCPPolicies(ctx context.Context, orgUUID string) (*models.MCPPolicyAvailabilityResponse, error) {
	_ = ctx
	if s.gatewayRepo == nil {
		return &models.MCPPolicyAvailabilityResponse{List: []models.MCPPolicyAvailableItem{}}, nil
	}

	active := true
	gateways, err := s.gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID: orgUUID,
		Status:         &active,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list active gateways: %w", err)
	}

	var available map[string]models.MCPPolicyAvailableItem
	seenGateway := false
	for _, gateway := range gateways {
		if gateway == nil {
			continue
		}
		gatewayPolicies := map[string]models.MCPPolicyAvailableItem{}
		for _, policy := range extractGatewayPolicyManifestItems(gatewayManifest(gateway)) {
			if policy.Name == "" || policy.Version == "" {
				continue
			}
			key := policy.Name + "\x00" + policy.Version
			gatewayPolicies[key] = policy
		}
		if !seenGateway {
			available = gatewayPolicies
			seenGateway = true
			continue
		}
		for key := range available {
			if _, ok := gatewayPolicies[key]; !ok {
				delete(available, key)
			}
		}
	}

	if available == nil {
		available = map[string]models.MCPPolicyAvailableItem{}
	}
	items := make([]models.MCPPolicyAvailableItem, 0, len(available))
	for _, item := range available {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].Version < items[j].Version
		}
		return items[i].Name < items[j].Name
	})

	return &models.MCPPolicyAvailabilityResponse{
		Count: int32(len(items)),
		List:  items,
	}, nil
}

// Get retrieves an MCP proxy by handle.
func (s *MCPProxyService) Get(ctx context.Context, orgUUID, proxyID string) (*models.MCPProxyDTO, error) {
	handle := strings.TrimSpace(proxyID)
	if handle == "" {
		return nil, utils.ErrInvalidInput
	}

	proxy, err := s.repo.GetByHandle(ctx, handle, orgUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMCPProxyNotFound
		}
		return nil, fmt.Errorf("failed to get MCP proxy: %w", err)
	}

	dto := convertModelMCPProxyToSpec(proxy)
	if dto != nil && s.deploymentRepo != nil {
		// Deployments are keyed by each (endpoint, environment) artifact UUID (the proxy's
		// and endpoint's own UUIDs are never deployed), so aggregate deployed gateways across
		// those artifacts and report a per-(endpoint,environment) Deployed/Undeployed status.
		gatewaySet := map[string]struct{}{}
		// Map (endpoint index, env UUID) -> artifact UUID for status lookup.
		artifactByEndpointEnv := map[uuid.UUID]map[string]uuid.UUID{}
		for _, endpoint := range proxy.Endpoints {
			byEnv := map[string]uuid.UUID{}
			for _, ee := range endpoint.Environments {
				byEnv[ee.EnvironmentUUID.String()] = ee.ArtifactUUID
			}
			artifactByEndpointEnv[endpoint.UUID] = byEnv
		}
		for i := range dto.Endpoints {
			endpointDTO := &dto.Endpoints[i]
			byEnv := artifactByEndpointEnv[proxy.Endpoints[i].UUID]
			for j := range endpointDTO.Environments {
				envBinding := &endpointDTO.Environments[j]
				status := models.MCPDeploymentStatusUndeployed
				artifactUUID := byEnv[envBinding.EnvironmentUUID]
				if artifactUUID != uuid.Nil {
					gatewayIDs, err := s.deploymentRepo.GetDeployedGatewaysByProvider(artifactUUID, orgUUID)
					if err != nil {
						s.logger.Warn("Failed to list deployed gateways for MCP proxy endpoint environment",
							"proxy_id", proxy.UUID, "endpoint", endpointDTO.ID, "environment", envBinding.EnvironmentUUID, "ou_id", orgUUID, "error", err)
					} else if len(gatewayIDs) > 0 {
						status = models.MCPDeploymentStatusDeployed
						for _, gatewayID := range gatewayIDs {
							gatewaySet[gatewayID] = struct{}{}
						}
						// Exactly one gateway per (artifact, environment) is the invariant, so
						// ids[0] is deterministic.
						envBinding.GatewayID = &gatewayIDs[0]
					}
				}
				envBinding.DeploymentStatus = status
			}
		}
		gateways := make([]string, 0, len(gatewaySet))
		for gatewayID := range gatewaySet {
			gateways = append(gateways, gatewayID)
		}
		dto.Gateways = gateways
	}
	return dto, nil
}

// Update modifies an existing MCP proxy and redeploys it to active gateways. Returns
// the DTO for the response and the underlying model so the caller can cascade further
// work (e.g. redeploying agent-scoped mapping artifacts).
func (s *MCPProxyService) Update(ctx context.Context, orgUUID, proxyID string, req *models.MCPProxyDTO) (*models.MCPProxyDTO, error) {
	if req == nil {
		return nil, utils.ErrInvalidInput
	}

	handle := strings.TrimSpace(proxyID)
	if handle == "" {
		return nil, utils.ErrInvalidInput
	}
	if strings.TrimSpace(req.ID) != "" && strings.TrimSpace(req.ID) != handle {
		return nil, utils.ErrInvalidInput
	}

	name := strings.TrimSpace(req.Name)
	version := strings.TrimSpace(req.Version)
	if name == "" || version == "" {
		return nil, utils.ErrInvalidInput
	}

	// Validate the endpoints (structure + SSRF) outside the transaction so network checks
	// don't hold the row lock. Auth is encrypted inside the transaction because it may need
	// to preserve the previously stored secret when the client omits it.
	if err := validateMCPEndpoints(ctx, req.Endpoints); err != nil {
		return nil, err
	}
	if err := validateMCPProxyDeploymentHandleLengths(handle, req.Endpoints); err != nil {
		return nil, err
	}
	// Load the proxy's current (environment -> artifact) mapping so the identity-security
	// probe can anchor each environment on its existing deployment instead of re-selecting
	// from the environment — mirrors indexExistingMCPEndpoints below, done here (outside
	// the transaction) because the probe itself performs DB reads. A lookup failure is
	// tolerated; GetByHandleForUpdate inside the transaction is the authoritative
	// existence check.
	var existingArtifactByEnv map[string]uuid.UUID
	if existingProxy, err := s.repo.GetByHandle(ctx, handle, orgUUID); err == nil && existingProxy != nil {
		existingArtifactByEnv = indexExistingMCPEndpoints(existingProxy.Endpoints).artifactByEnv
	}
	if err := s.validateMCPEndpointSecurity(ctx, orgUUID, req.Endpoints, existingArtifactByEnv); err != nil {
		return nil, err
	}
	if err := s.validateMCPEndpointPlacement(orgUUID, req.Endpoints, existingArtifactByEnv); err != nil {
		return nil, err
	}

	// Captured inside the transaction so removed-(endpoint,env) artifacts can be torn down
	// from their gateways after the update commits. Keyed by environment UUID because
	// uq_proxy_env_single guarantees at most one artifact per environment per proxy, so an
	// environment's artifact identity is stable even if it moves between endpoints.
	previousEnvArtifacts := map[string]uuid.UUID{}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		proxy, err := s.repo.GetByHandleForUpdate(ctx, tx, handle, orgUUID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return utils.ErrMCPProxyNotFound
			}
			return fmt.Errorf("failed to get MCP proxy before update: %w", err)
		}

		// Index the existing graph so buildMCPEndpointsForStorage can preserve per-env
		// artifact UUIDs (agent bindings resolve (proxy, env) -> artifact_uuid) and per-
		// endpoint upstream secrets when the client omits credentials.
		existing := indexExistingMCPEndpoints(proxy.Endpoints)
		for env, artifactUUID := range existing.artifactByEnv {
			previousEnvArtifacts[env] = artifactUUID
		}

		endpoints, err := s.buildMCPEndpointsForStorage(req.Endpoints, &existing)
		if err != nil {
			return err
		}

		proxy.Description = valueOrEmpty(req.Description)
		proxy.Name = name
		proxy.Version = version
		proxy.Configuration = models.MCPProxyConfig{
			Name:        name,
			Version:     version,
			Context:     req.Context,
			Vhost:       req.Vhost,
			SpecVersion: valueOrEmpty(req.McpSpecVersion),
		}

		if err := s.repo.Update(ctx, tx, proxy, orgUUID); err != nil {
			return err
		}

		// Endpoint config is fully specified on each PUT, so reconcile by replacing the
		// endpoint set: delete existing endpoints (cascade removes their join rows) and
		// re-insert. Preserved env artifact UUIDs keep agent bindings resolving.
		for _, endpoint := range proxy.Endpoints {
			if err := s.endpointRepo.DeleteEndpoint(ctx, tx, endpoint.UUID); err != nil &&
				!errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("failed to delete existing MCP proxy endpoint: %w", err)
			}
		}
		proxyHandle := handle
		if proxy.Artifact != nil && proxy.Artifact.Handle != "" {
			proxyHandle = proxy.Artifact.Handle
		}
		return s.persistMCPEndpoints(ctx, tx, proxy.UUID, proxyHandle, version, endpoints, orgUUID)
	}); err != nil {
		if errors.Is(err, utils.ErrMCPProxyNotFound) || errors.Is(err, utils.ErrInvalidInput) {
			return nil, err
		}
		if mapped := s.mapMCPProxyWriteError(err, handle, orgUUID); mapped != nil {
			return nil, mapped
		}
		return nil, fmt.Errorf("failed to update MCP proxy: %w", err)
	}

	updated, err := s.repo.GetByHandle(ctx, handle, orgUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMCPProxyNotFound
		}
		return nil, fmt.Errorf("failed to retrieve MCP proxy: %w", err)
	}

	// Redeploy the (endpoint,env) artifacts so they pick up the new upstream / auth /
	// policies / context, and tear down artifacts for environments no longer bound.
	applyRequestedGatewayUUIDs(updated, req.Endpoints)
	deployErr := s.deployMCPProxyEndpoints(ctx, updated, orgUUID)

	// Run teardown and refresh unconditionally: the DB transaction that unbound environments
	// already committed, and this is the only chance to clean up the orphaned artifacts.
	stillBound := map[string]struct{}{}
	for _, endpoint := range updated.Endpoints {
		for _, ee := range endpoint.Environments {
			stillBound[ee.EnvironmentUUID.String()] = struct{}{}
		}
	}
	var removedArtifacts []uuid.UUID
	for env, artifactUUID := range previousEnvArtifacts {
		if _, ok := stillBound[env]; !ok {
			removedArtifacts = append(removedArtifacts, artifactUUID)
		}
	}
	if len(removedArtifacts) > 0 {
		if err := s.deleteMCPProxyEnvironmentArtifacts(ctx, removedArtifacts, orgUUID); err != nil {
			s.logger.Warn("Failed to delete removed MCP proxy endpoint artifacts", "proxy_id", updated.UUID, "error", err)
		}
	}

	// The org-level proxy owns and (re)deploys the per-environment gateway artifacts above;
	// agents that reference this proxy read its endpoint at their own deploy time via the
	// stored DB mapping. A proxy update can still change what scopes an already-running
	// agent's AgentID token should carry, so best-effort refresh every agent bound to this
	// proxy so their next-minted token reflects the change immediately instead of waiting
	// for their next deploy/promote.
	//
	// Detached onto its own goroutine, off the request path: cost scales with
	// (agents bound to this proxy) x (environments each is deployed to), which
	// can be large enough to risk a gateway timeout on what is already a
	// best-effort step. Uses context.WithoutCancel (not context.Background())
	// so it still carries request-scoped values like a correlation ID,
	// without being cancelled the moment this handler returns.
	go s.refreshAgentsBoundToProxy(context.WithoutCancel(ctx), updated, orgUUID)

	// Now evaluate the deployment error after cleanup has run.
	if deployErr != nil {
		if mcpDeployErrorIsFatal(deployErr) {
			return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, deployErr)
		}
		s.logger.Warn("Failed to redeploy one or more MCP proxy endpoint artifacts", "proxy_id", updated.UUID, "error", deployErr)
	}

	return convertModelMCPProxyToSpec(updated), nil
}

// refreshAgentsBoundToProxy brings every agent bound to proxyUUID's live AgentID scope
// list in line with what it should now be (recomputing the scope list), across every
// environment they're deployed to. Uses ReconcileForEnvironment (not InjectForEnvironment)
// so an agent whose scopes didn't actually change from this proxy update never gets its
// pod needlessly rolled. Purely best-effort: the caller runs this on a detached goroutine
// after the proxy update already succeeded, so a failure here must never surface as an
// error from Update — it's logged and the corresponding agent simply picks up the change
// on its next deploy/promote/rotation instead.
func (s *MCPProxyService) refreshAgentsBoundToProxy(ctx context.Context, proxy *models.MCPProxy, orgUUID string) {
	mappings, err := s.envMCPMappingRepo.ListByMCPProxy(ctx, proxy.UUID)
	if err != nil {
		s.logger.Warn("Failed to list agent bindings for scope refresh", "proxy_uuid", proxy.UUID, "error", err)
		return
	}
	if len(mappings) == 0 {
		return
	}

	envs, err := s.infraManager.ListOrgEnvironments(ctx, orgUUID)
	if err != nil {
		s.logger.Warn("Failed to list environments for scope refresh", "proxy_uuid", proxy.UUID, "error", err)
		return
	}
	uuidToEnvName := make(map[string]string, len(envs))
	for _, e := range envs {
		uuidToEnvName[e.UUID] = e.Name
	}

	for _, mapping := range mappings {
		envName := uuidToEnvName[mapping.EnvironmentUUID.String()]
		if envName == "" {
			continue // environment since deleted — nothing to refresh
		}
		config, err := s.agentConfigRepo.GetByUUID(ctx, mapping.ConfigUUID, orgUUID)
		if err != nil {
			s.logger.Warn("Failed to resolve agent configuration for scope refresh",
				"proxy_uuid", proxy.UUID, "config_uuid", mapping.ConfigUUID, "error", err)
			continue
		}
		if err := s.agentIdentityInjection.ReconcileForEnvironment(ctx, orgUUID, config.ProjectName, config.AgentID, envName); err != nil {
			s.logger.Warn("Failed to refresh agent identity credentials after MCP proxy change",
				"agent_name", config.AgentID, "env_name", envName, "error", err)
		}
	}
}

// Delete removes an MCP proxy by handle. MCP proxy mappings are deployable artifacts
// derived from an MCP proxy, so the source proxy cannot be deleted while mappings exist.
func (s *MCPProxyService) Delete(ctx context.Context, orgUUID, orgName, proxyID string) error {
	_ = orgName // unused: cleanup now resolves env-Thunder by orgUUID (ouID), not orgName

	handle := strings.TrimSpace(proxyID)
	if handle == "" {
		return utils.ErrInvalidInput
	}

	proxy, err := s.repo.GetByHandle(ctx, handle, orgUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrMCPProxyNotFound
		}
		return fmt.Errorf("failed to get MCP proxy before delete: %w", err)
	}

	var mappings []models.EnvAgentMCPMapping
	if s.envMCPMappingRepo != nil {
		mappings, err = s.envMCPMappingRepo.ListByMCPProxy(ctx, proxy.UUID)
		if err != nil {
			return fmt.Errorf("failed to list MCP mappings before delete: %w", err)
		}
	}
	if len(mappings) > 0 {
		return utils.ErrMCPProxyHasMappings
	}

	// Capture the proxy's scope rows before the row delete cascades them away
	// (mcp_proxy_scopes FK is ON DELETE CASCADE) — the Thunder cleanup below needs
	// every scope string to strip from role permissions.
	scopes, _ := s.mcpProxyScopeRepo.ListByProxy(ctx, proxy.UUID)

	// Tear down the per-(endpoint,env) gateway artifacts this proxy deployed before removing
	// the proxy row. The endpoint and join rows cascade-delete with the parent proxy; here we
	// only need to broadcast deletion and remove the backing artifacts rows. Best-effort.
	var artifactUUIDs []uuid.UUID
	endpointEnvs, err := s.endpointRepo.ListEndpointEnvironmentsByProxy(ctx, proxy.UUID)
	if err != nil {
		return fmt.Errorf("failed to list MCP proxy endpoint environments before delete: %w", err)
	}
	for _, ee := range endpointEnvs {
		if ee.ArtifactUUID != uuid.Nil {
			artifactUUIDs = append(artifactUUIDs, ee.ArtifactUUID)
		}
	}
	if err := s.deleteMCPProxyEnvironmentArtifacts(ctx, artifactUUIDs, orgUUID); err != nil {
		s.logger.Warn("Failed to delete MCP proxy endpoint artifacts during proxy delete", "proxy_id", proxy.UUID, "error", err)
	}

	if err := s.repo.Delete(ctx, handle, orgUUID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrMCPProxyNotFound
		}
		return fmt.Errorf("failed to delete MCP proxy: %w", err)
	}

	s.cleanupProxyResourceServers(ctx, orgUUID, proxy, scopes)

	return nil
}

// cleanupProxyResourceServers best-effort tears down the proxy's per-environment Thunder
// resource servers and strips their scope strings from every role, after the proxy row
// (and its cascaded scope rows) has already been deleted. Same endpoints->join-rows walk
// and env-UUID->name map as cleanupDeletedScope; never fails the caller. Does not gate on
// the endpoint's identity-security flag — see cleanupDeletedScope for why: RS ensure at
// role-write time doesn't check it either, so skipping cleanup here leaks the RS whenever
// security was disabled.
func (s *MCPProxyService) cleanupProxyResourceServers(ctx context.Context, ouID string, proxy *models.MCPProxy, scopes []models.MCPProxyScope) {
	handle := proxyHandleOf(proxy)
	scopeStrs := make([]string, len(scopes))
	for i, scope := range scopes {
		scopeStrs[i] = scope.ScopeString(handle)
	}

	envs, err := s.infraManager.ListOrgEnvironments(ctx, ouID)
	if err != nil {
		s.logger.Warn("proxy delete: listing environments failed", "proxy", handle, "error", err)
		return
	}
	envName := make(map[string]string, len(envs)) // env UUID -> name (resolver keys on names)
	for _, env := range envs {
		envName[env.UUID] = env.Name
	}

	for i := range proxy.Endpoints {
		endpoint := &proxy.Endpoints[i]
		for j := range endpoint.Environments {
			name, ok := envName[endpoint.Environments[j].EnvironmentUUID.String()]
			if !ok {
				continue // environment no longer exists — nothing to clean
			}
			client, err := ResolveEnvThunderIdentity(ctx, s.resolver, ouID, name)
			if err != nil {
				s.logger.Warn("proxy delete: env-Thunder unavailable", "env", name, "proxy", handle, "error", err)
				continue
			}
			if err := client.DeleteProxyResourceServer(ctx, handle); err != nil {
				s.logger.Warn("proxy delete: resource server delete failed", "env", name, "proxy", handle, "error", err)
				continue
			}
			for _, scopeStr := range scopeStrs {
				sweepRolePermission(ctx, s.logger, client, name, scopeStr)
			}
		}
	}
}

// RemoveEnvironmentFromEndpoints removes an environment's endpoint bindings from a proxy
// when that environment is deleted. For every endpoint of the proxy bound to the given
// environment it tears down the (endpoint,env) gateway artifact and deletes the join row.
// The endpoint rows themselves remain (they may still be bound to other environments).
// Best-effort: per-binding failures are aggregated and returned but never abort the sweep.
func (s *MCPProxyService) RemoveEnvironmentFromEndpoints(ctx context.Context, proxy *models.MCPProxy, envUUID uuid.UUID, orgUUID string) error {
	if proxy == nil {
		return nil
	}
	var errs []error
	for i := range proxy.Endpoints {
		endpoint := &proxy.Endpoints[i]
		for j := range endpoint.Environments {
			ee := &endpoint.Environments[j]
			if ee.EnvironmentUUID != envUUID {
				continue
			}
			// Tear down the single gateway artifact this (endpoint,env) deployed before
			// removing the binding row.
			if ee.ArtifactUUID != uuid.Nil {
				if err := s.deleteMCPProxyEnvironmentArtifacts(ctx, []uuid.UUID{ee.ArtifactUUID}, orgUUID); err != nil {
					errs = append(errs, fmt.Errorf("delete env %s artifact for proxy %s endpoint %s: %w",
						envUUID, proxy.UUID, endpoint.Handle, err))
				}
			}
			if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				return s.endpointRepo.RemoveEndpointEnvironment(ctx, tx, endpoint.UUID, envUUID)
			}); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				errs = append(errs, fmt.Errorf("remove env %s binding from proxy %s endpoint %s: %w",
					envUUID, proxy.UUID, endpoint.Handle, err))
			}
		}
	}
	return errors.Join(errs...)
}

func extractGatewayPolicyManifestItems(value interface{}) []models.MCPPolicyAvailableItem {
	seen := map[string]struct{}{}
	items := make([]models.MCPPolicyAvailableItem, 0)
	var walk func(interface{})

	add := func(name, version string) {
		name = strings.TrimSpace(name)
		version = strings.TrimSpace(version)
		if name == "" || version == "" {
			return
		}
		key := name + "\x00" + version
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		items = append(items, models.MCPPolicyAvailableItem{Name: name, Version: version})
	}

	stringValue := func(v interface{}) string {
		if s, ok := v.(string); ok {
			return s
		}
		return ""
	}

	walk = func(current interface{}) {
		switch typed := current.(type) {
		case []interface{}:
			for _, item := range typed {
				walk(item)
			}
		case []map[string]interface{}:
			for _, item := range typed {
				walk(item)
			}
		case map[string]interface{}:
			name := stringValue(firstMapValue(typed, "name", "policyName", "id"))
			version := stringValue(firstMapValue(typed, "version", "policyVersion"))
			if name != "" && version != "" {
				add(name, version)
			}
			if name != "" {
				if versions, ok := firstMapValue(typed, "versions", "policyVersions").([]interface{}); ok {
					for _, rawVersion := range versions {
						add(name, stringValue(rawVersion))
					}
				}
				if versions, ok := firstMapValue(typed, "versions", "policyVersions").([]string); ok {
					for _, rawVersion := range versions {
						add(name, rawVersion)
					}
				}
			}
			for _, nested := range typed {
				walk(nested)
			}
		}
	}

	walk(value)
	return items
}

func firstMapValue(values map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}

func (s *MCPProxyService) prepareMCPUpstreamAuthForStorage(existing, updated *models.UpstreamAuth) (*models.UpstreamAuth, error) {
	auth := preserveUpstreamAuthCredential(existing, updated)
	if auth == nil {
		return nil, nil //nolint:nilnil // A nil auth value is valid when upstream auth is omitted.
	}

	if auth.Header != nil {
		header := strings.TrimSpace(*auth.Header)
		auth.Header = &header
	}
	if auth.Value != nil && *auth.Value == "" {
		auth.Value = nil
	}
	// A newly supplied plaintext value supersedes any preserved encrypted secret,
	// keeping Value and SecretRef mutually exclusive for Validate.
	if auth.Value != nil {
		auth.SecretRef = nil
	}

	if err := auth.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
	}

	hasHeader := auth.Header != nil && *auth.Header != ""
	hasCredential := auth.Value != nil || auth.SecretRef != nil
	if !hasHeader && !hasCredential {
		return nil, nil //nolint:nilnil // Empty auth fields mean the endpoint should not store auth.
	}
	if hasHeader != hasCredential {
		return nil, fmt.Errorf("%w: authentication header and value must be provided together", utils.ErrInvalidInput)
	}

	if auth.Value != nil {
		encrypted, err := utils.EncryptBytes([]byte(*auth.Value), s.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt upstream auth: %w", err)
		}
		encoded := base64.StdEncoding.EncodeToString(encrypted)
		auth.SecretRef = &encoded
		auth.Value = nil
	}

	return auth, nil
}

// FetchServerInfo fetches server information from an MCP backend.
func (s *MCPProxyService) FetchServerInfo(ctx context.Context, req *models.MCPServerInfoFetchRequest) (*models.MCPServerInfoFetchResponse, error) {
	if req == nil || req.URL == nil || strings.TrimSpace(*req.URL) == "" {
		return nil, utils.ErrInvalidInput
	}
	if req.ProxyID != nil && strings.TrimSpace(*req.ProxyID) != "" {
		return nil, fmt.Errorf("proxyId refresh is not supported yet: %w", utils.ErrInvalidInput)
	}

	endpointURL := strings.TrimSpace(*req.URL)
	if err := ssrf.ValidateURL(ctx, endpointURL); err != nil {
		return nil, fmt.Errorf("%w: %w", utils.ErrInvalidURL, err)
	}

	headerName, headerValue := authHeader(req.Auth)
	return s.fetchMCPServerInfo(ctx, endpointURL, headerName, headerValue)
}

func authHeader(auth *models.UpstreamAuth) (string, string) {
	if auth == nil || auth.Header == nil || auth.Value == nil {
		return "", ""
	}
	return strings.TrimSpace(*auth.Header), *auth.Value
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func timePtrIfNotZero(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func copyMCPPolicies(policies []models.MCPPolicy) []models.MCPPolicy {
	if len(policies) == 0 {
		return nil
	}
	out := make([]models.MCPPolicy, len(policies))
	copy(out, policies)
	return out
}

func copyMCPCapabilities(capabilities *models.MCPProxyCapabilities) *models.MCPProxyCapabilities {
	if capabilities == nil {
		return nil
	}
	return &models.MCPProxyCapabilities{
		Prompts:   capabilities.Prompts,
		Resources: capabilities.Resources,
		Tools:     capabilities.Tools,
	}
}

func sanitizeMCPUpstreamEndpointForResponse(endpoint *models.UpstreamEndpoint) *models.UpstreamEndpoint {
	if endpoint == nil {
		return nil
	}
	sanitized := *endpoint
	sanitized.Auth = sanitizeMCPUpstreamAuthForResponse(endpoint.Auth)
	return &sanitized
}

// buildMCPEndpointDTOsForResponse maps the proxy's endpoints to response DTOs, stripping
// plaintext upstream credentials (and never exposing SecretRef via the sanitizer) from
// each endpoint. Environment bindings are reported as {environmentUuid} entries; the
// per-(endpoint,env) DeploymentStatus is filled in by the Get handler from the join rows.
func buildMCPEndpointDTOsForResponse(endpoints []models.MCPProxyEndpoint) []models.MCPProxyEndpointDTO {
	if len(endpoints) == 0 {
		return nil
	}
	out := make([]models.MCPProxyEndpointDTO, 0, len(endpoints))
	for _, endpoint := range endpoints {
		cfg := endpoint.Configuration
		dto := models.MCPProxyEndpointDTO{
			ID:           endpoint.Handle,
			Name:         endpoint.Name,
			Upstream:     models.UpstreamConfig{Main: sanitizeMCPUpstreamEndpointForResponse(cfg.Upstream)},
			Resilience:   cfg.Resilience,
			Capabilities: copyMCPCapabilities(cfg.Capabilities),
			Security:     cfg.Security,
		}
		if policies := copyMCPPolicies(cfg.Policies); policies != nil {
			dto.Policies = &policies
		}
		envs := make([]models.MCPEndpointEnvironmentDTO, 0, len(endpoint.Environments))
		for _, ee := range endpoint.Environments {
			envs = append(envs, models.MCPEndpointEnvironmentDTO{
				EnvironmentUUID: ee.EnvironmentUUID.String(),
			})
		}
		dto.Environments = envs
		out = append(out, dto)
	}
	return out
}

func sanitizeMCPUpstreamAuthForResponse(auth *models.UpstreamAuth) *models.UpstreamAuth {
	if auth == nil {
		return nil
	}
	sanitized := *auth
	sanitized.Value = nil
	// SecretRef is the AES-256-GCM ciphertext of the same credential and is not
	// declared on the spec's UpstreamAuth, so it must not reach a response body.
	sanitized.SecretRef = nil
	return &sanitized
}

func defaultMCPProxySecurity(security *models.SecurityConfig) *models.SecurityConfig {
	enabled := true
	if security != nil {
		out := *security
		if out.Enabled == nil {
			out.Enabled = &enabled
		}
		if !isBoolTrue(out.Enabled) {
			return &out
		}
		if out.Identity != nil && isBoolTrue(out.Identity.Enabled) {
			// Agent Identity mode: no API-key defaulting. Mutual exclusion is
			// enforced in validateMCPEnvironments before this runs.
			identity := *out.Identity
			out.Identity = &identity
			out.APIKey = nil
			return &out
		}
		if out.APIKey == nil {
			out.APIKey = &models.APIKeySecurity{}
		} else {
			apiKey := *out.APIKey
			out.APIKey = &apiKey
		}
		if out.APIKey.Enabled == nil {
			out.APIKey.Enabled = &enabled
		}
		if isBoolTrue(out.APIKey.Enabled) {
			if strings.TrimSpace(out.APIKey.Key) == "" {
				out.APIKey.Key = "X-API-Key"
			}
			if strings.TrimSpace(out.APIKey.In) == "" {
				out.APIKey.In = "header"
			}
		}
		return &out
	}
	return &models.SecurityConfig{
		Enabled: &enabled,
		APIKey: &models.APIKeySecurity{
			Enabled: &enabled,
			Key:     "X-API-Key",
			In:      "header",
		},
	}
}

func convertModelMCPProxyToSpec(proxy *models.MCPProxy) *models.MCPProxyDTO {
	if proxy == nil {
		return nil
	}
	id := proxy.Handle
	name := proxy.Name
	version := proxy.Version
	createdAt := proxy.CreatedAt
	updatedAt := proxy.UpdatedAt
	inCatalog := false
	if proxy.Artifact != nil {
		id = proxy.Artifact.Handle
		name = proxy.Artifact.Name
		version = proxy.Artifact.Version
		createdAt = proxy.Artifact.CreatedAt
		updatedAt = proxy.Artifact.UpdatedAt
		inCatalog = proxy.Artifact.InCatalog
	}
	if name == "" {
		name = proxy.Configuration.Name
	}
	if version == "" {
		version = proxy.Configuration.Version
	}
	return &models.MCPProxyDTO{
		Context:        proxy.Configuration.Context,
		CreatedAt:      timePtrIfNotZero(createdAt),
		CreatedBy:      stringPtrIfNotEmpty(proxy.CreatedBy),
		Description:    stringPtrIfNotEmpty(proxy.Description),
		ID:             id,
		InCatalog:      inCatalog,
		McpSpecVersion: stringPtrIfNotEmpty(proxy.Configuration.SpecVersion),
		Name:           name,
		Endpoints:      buildMCPEndpointDTOsForResponse(proxy.Endpoints),
		UpdatedAt:      timePtrIfNotZero(updatedAt),
		Version:        version,
		Vhost:          proxy.Configuration.Vhost,
	}
}

func convertModelMCPProxyToListItem(proxy *models.MCPProxy) models.MCPProxyListItem {
	status := proxy.Status
	id := proxy.Handle
	name := proxy.Name
	version := proxy.Version
	createdAt := proxy.CreatedAt
	updatedAt := proxy.UpdatedAt
	if proxy.Artifact != nil {
		id = proxy.Artifact.Handle
		name = proxy.Artifact.Name
		version = proxy.Artifact.Version
		createdAt = proxy.Artifact.CreatedAt
		updatedAt = proxy.Artifact.UpdatedAt
	}
	if name == "" {
		name = proxy.Configuration.Name
	}
	if version == "" {
		version = proxy.Configuration.Version
	}
	return models.MCPProxyListItem{
		Context:        proxy.Configuration.Context,
		CreatedAt:      timePtrIfNotZero(createdAt),
		CreatedBy:      stringPtrIfNotEmpty(proxy.CreatedBy),
		Description:    stringPtrIfNotEmpty(proxy.Description),
		ID:             stringPtrIfNotEmpty(id),
		McpSpecVersion: stringPtrIfNotEmpty(proxy.Configuration.SpecVersion),
		Name:           stringPtrIfNotEmpty(name),
		Status:         stringPtrIfNotEmpty(status),
		UpdatedAt:      timePtrIfNotZero(updatedAt),
		Version:        stringPtrIfNotEmpty(version),
	}
}

// validateMCPEndpoints enforces the endpoint request structure: at least one endpoint,
// each with a non-empty handle (unique within the proxy), a valid SSRF-safe upstream URL,
// and at least one valid target environment UUID. It also enforces the hard rule that an
// environment maps to at most one endpoint within the proxy. It performs the network
// checks, so call it outside a DB transaction.
// validateMCPProxyDeploymentHandleLengths rejects a proxy whose deployed gateway artifact
// name/displayName would exceed the gateway's length limit. The console derives short endpoint
// handles, but a blank-name endpoint whose handle falls back to a long upstream hostname, or a
// direct API caller supplying its own long id, can push the composite past the limit. Catching
// it here turns a silent async deploy failure into an immediate, explicit 400.
func validateMCPProxyDeploymentHandleLengths(proxyHandle string, endpoints []models.MCPProxyEndpointDTO) error {
	for _, endpoint := range endpoints {
		endpointHandle := strings.TrimSpace(endpoint.ID)
		for _, env := range endpoint.Environments {
			// Build the exact string deployment will emit, so this check stays in lockstep with
			// the derivation instead of re-deriving its length by hand.
			artifactHandle := mcpProxyEnvArtifactHandle(proxyHandle, endpointHandle, env.EnvironmentUUID)
			if len(artifactHandle) > maxMCPProxyDeploymentHandleLength {
				return fmt.Errorf(
					"%w: endpoint %q would produce a %d-character gateway deployment name, exceeding the limit of %d; shorten the endpoint name/id or the proxy id",
					utils.ErrInvalidInput, endpointHandle, len(artifactHandle), maxMCPProxyDeploymentHandleLength,
				)
			}
		}
	}
	return nil
}

func validateMCPEndpoints(ctx context.Context, endpoints []models.MCPProxyEndpointDTO) error {
	if len(endpoints) == 0 {
		return fmt.Errorf("%w: at least one endpoint is required", utils.ErrInvalidInput)
	}
	seenHandles := make(map[string]struct{}, len(endpoints))
	// Enforces uq_proxy_env_single at the service layer: no environment may be bound to
	// two endpoints within the same proxy.
	envToEndpoint := map[string]string{}
	for _, endpoint := range endpoints {
		handle := strings.TrimSpace(endpoint.ID)
		if handle == "" {
			return fmt.Errorf("%w: an endpoint is missing an id", utils.ErrInvalidInput)
		}
		if _, dup := seenHandles[handle]; dup {
			return fmt.Errorf("%w: duplicate endpoint id %q", utils.ErrInvalidInput, handle)
		}
		seenHandles[handle] = struct{}{}

		// Structural security check (no network I/O): apiKey and identity are mutually
		// exclusive.
		if sec := endpoint.Security; sec != nil && isBoolTrue(sec.Enabled) {
			apiKeyOn := sec.APIKey != nil && isBoolTrue(sec.APIKey.Enabled)
			identityOn := sec.Identity != nil && isBoolTrue(sec.Identity.Enabled)
			if apiKeyOn && identityOn {
				return fmt.Errorf("%w: endpoint %q: apiKey and identity security are mutually exclusive", utils.ErrInvalidInput, handle)
			}
		}

		if endpoint.Upstream.Main == nil || strings.TrimSpace(endpoint.Upstream.Main.URL) == "" {
			return fmt.Errorf("%w: endpoint %q is missing an upstream url", utils.ErrInvalidInput, handle)
		}
		if err := ssrf.ValidateURL(ctx, strings.TrimSpace(endpoint.Upstream.Main.URL)); err != nil {
			return fmt.Errorf("%w: endpoint %q upstream url: %w", utils.ErrInvalidURL, handle, err)
		}
		if err := endpoint.Resilience.Validate(); err != nil {
			return fmt.Errorf("%w: endpoint %q: %w", utils.ErrInvalidInput, handle, err)
		}

		if len(endpoint.Environments) == 0 {
			return fmt.Errorf("%w: endpoint %q must target at least one environment", utils.ErrInvalidInput, handle)
		}
		seenEnvsInEndpoint := make(map[string]struct{}, len(endpoint.Environments))
		for _, env := range endpoint.Environments {
			envUUID := strings.TrimSpace(env.EnvironmentUUID)
			if envUUID == "" {
				return fmt.Errorf("%w: endpoint %q has an environment binding missing an environment id", utils.ErrInvalidInput, handle)
			}
			if _, err := uuid.Parse(envUUID); err != nil {
				return fmt.Errorf("%w: endpoint %q has an invalid environment id %q: %w", utils.ErrInvalidInput, handle, envUUID, err)
			}
			if _, dup := seenEnvsInEndpoint[envUUID]; dup {
				return fmt.Errorf("%w: endpoint %q lists environment %q more than once", utils.ErrInvalidInput, handle, envUUID)
			}
			seenEnvsInEndpoint[envUUID] = struct{}{}
			if owner, bound := envToEndpoint[envUUID]; bound {
				return fmt.Errorf("%w: environment %q is targeted by endpoints %q and %q", utils.ErrMCPEnvAlreadyBound, envUUID, owner, handle)
			}
			envToEndpoint[envUUID] = handle
		}
	}
	return nil
}

// validateMCPEndpointSecurity enforces the cross-resource identity-mode rule: an
// identity-mode endpoint's target environments must each have a gateway advertising
// mcp-auth v1 + mcp-authz v1 in its policy manifest. It performs DB reads, so call
// it outside a transaction.
//
// existingArtifactByEnv is the proxy's current (environment UUID -> artifact UUID)
// mapping on update (nil on create, from indexExistingMCPEndpoints/GetByHandle — see
// callers). When an environment already has a deployed artifact, the probe anchors on
// that artifact's actual gateway — mirroring deployMCPProxyEndpoints — instead of
// re-selecting from the environment, so a second egress gateway never breaks validation
// for a binding that already works.
func (s *MCPProxyService) validateMCPEndpointSecurity(
	ctx context.Context, orgName string, endpoints []models.MCPProxyEndpointDTO, existingArtifactByEnv map[string]uuid.UUID,
) error {
	for _, endpoint := range endpoints {
		handle := strings.TrimSpace(endpoint.ID)
		if endpoint.Security == nil || !isBoolTrue(endpoint.Security.Enabled) ||
			endpoint.Security.Identity == nil || !isBoolTrue(endpoint.Security.Identity.Enabled) {
			continue
		}
		for _, env := range endpoint.Environments {
			envIDStr := strings.TrimSpace(env.EnvironmentUUID)
			envUUID, err := uuid.Parse(envIDStr)
			if err != nil {
				continue // already rejected by validateMCPEndpoints
			}

			var candidates []*models.Gateway
			if artifactUUID, ok := existingArtifactByEnv[envIDStr]; ok && artifactUUID != uuid.Nil {
				var deployed []string
				if s.deploymentRepo != nil {
					ids, depErr := s.deploymentRepo.GetDeployedGatewaysByProvider(artifactUUID, orgName)
					if depErr != nil {
						return fmt.Errorf("endpoint %q environment %q: list deployed gateways: %w", handle, env.EnvironmentUUID, depErr)
					}
					deployed = ids
				}
				gateway, gwErr := resolveEgressGatewayForArtifact(s.gatewayRepo, orgName, envUUID, deployed, nil)
				if errors.Is(gwErr, errNoGatewayForEnvironment) {
					continue // no gateway yet: allowed, deploys later; policies checked when one exists
				}
				if gwErr != nil {
					return fmt.Errorf("endpoint %q environment %q: resolve gateway: %w", handle, env.EnvironmentUUID, gwErr)
				}
				candidates = []*models.Gateway{gateway}
			} else {
				// No existing deployment to anchor on (create, or a genuinely new
				// binding): this is a read-only policy probe, not a placement decision —
				// actual placement is resolved at deploy time — so multiple egress-capable
				// gateways are tolerated here rather than treated as ambiguous. Accept only
				// if every candidate supports the required policies.
				gws, gwErr := egressCandidatesForEnvironment(s.gatewayRepo, orgName, envUUID)
				if errors.Is(gwErr, errNoGatewayForEnvironment) {
					continue
				}
				if gwErr != nil {
					return fmt.Errorf("endpoint %q environment %q: resolve gateway: %w", handle, env.EnvironmentUUID, gwErr)
				}
				candidates = gws
			}

			for _, gateway := range candidates {
				if !gatewayHasMCPIdentityPolicies(gateway) {
					return fmt.Errorf("%w: endpoint %q environment %q: gateway %q does not support mcp-auth/mcp-authz v1 policies required for Agent Identity security", utils.ErrInvalidInput, handle, env.EnvironmentUUID, gateway.Name)
				}
			}
		}
	}
	return nil
}

// validateMCPEndpointPlacement resolves the egress gateway for every (endpoint, environment)
// binding before anything is written, so a caller-supplied gatewayId that cannot host the
// binding fails the request with nothing persisted. Without this the proxy commits first and
// deployMCPProxyEndpoints raises the same 400 afterward, leaving a row that turns the client's
// corrected retry into a 409 on create. Mirrors CreateAndDeploy's pre-Create placement check
// for LLM providers. It performs DB reads, so call it outside a transaction.
//
// Only the fatal placement errors are enforced here; every other outcome (no gateway mapped
// yet, transient lookup failures) is left to the deploy step, which is best-effort by design
// and retries on the next update. Deliberately conservative: a pre-check that hard-failed on
// infra blips would turn today's tolerated conditions into rejected requests.
//
// existingArtifactByEnv is the proxy's current (environment -> artifact) mapping, nil on
// create. An environment with a deployed artifact anchors on that artifact's gateway, so
// errPlacementFixed also surfaces before the write rather than after it.
func (s *MCPProxyService) validateMCPEndpointPlacement(
	orgUUID string, endpoints []models.MCPProxyEndpointDTO, existingArtifactByEnv map[string]uuid.UUID,
) error {
	for _, endpoint := range endpoints {
		handle := strings.TrimSpace(endpoint.ID)
		for _, env := range endpoint.Environments {
			envIDStr := strings.TrimSpace(env.EnvironmentUUID)
			envUUID, err := uuid.Parse(envIDStr)
			if err != nil {
				continue // already rejected by validateMCPEndpoints
			}

			deployed, ok := s.anchorGatewaysForEnvironment(orgUUID, existingArtifactByEnv[envIDStr])
			if !ok {
				// Anchor unknown: skip rather than resolve without it, since environment-only
				// selection could reject a binding the anchor would have resolved cleanly.
				continue
			}

			_, err = resolveEgressGatewayForArtifact(s.gatewayRepo, orgUUID, envUUID, deployed, requestedGatewayUUID(env))
			if mcpDeployErrorIsFatal(err) {
				return fmt.Errorf("%w: endpoint %q environment %q: %w", utils.ErrInvalidInput, handle, envIDStr, err)
			}
		}
	}
	return nil
}

// anchorGatewaysForEnvironment returns the gateways artifactUUID is deployed to, and whether
// that set could be determined at all. A nil artifact is a new binding: no anchor to look up,
// which is a known-empty set rather than an unknown one.
func (s *MCPProxyService) anchorGatewaysForEnvironment(orgUUID string, artifactUUID uuid.UUID) ([]string, bool) {
	if artifactUUID == uuid.Nil || s.deploymentRepo == nil {
		return nil, true
	}
	deployed, err := s.deploymentRepo.GetDeployedGatewaysByProvider(artifactUUID, orgUUID)
	if err != nil {
		s.logger.Warn("Skipping MCP placement pre-check; could not list existing deployments",
			"artifact_uuid", artifactUUID, "error", err)
		return nil, false
	}
	return deployed, true
}

// requestedGatewayUUID extracts the caller's gatewayId for one environment binding, treating
// blank as unset. Matches applyRequestedGatewayUUIDs, which feeds the deploy step the same value.
func requestedGatewayUUID(env models.MCPEndpointEnvironmentDTO) *string {
	if env.GatewayID == nil {
		return nil
	}
	gwID := strings.TrimSpace(*env.GatewayID)
	if gwID == "" {
		return nil
	}
	return &gwID
}

// egressCandidatesForEnvironment returns every egress-capable gateway mapped to the
// environment, never erroring on multiplicity (unlike resolveEgressGatewayForEnvironment)
// — used only by the identity-security probe above, which checks policy support rather
// than choosing a placement.
func egressCandidatesForEnvironment(repo repositories.GatewayRepository, ouID string, envUUID uuid.UUID) ([]*models.Gateway, error) {
	envIDStr := envUUID.String()
	gateways, err := repo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID: ouID,
		EnvironmentID:  &envIDStr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list gateways for environment %s: %w", envIDStr, err)
	}
	if len(gateways) == 0 {
		return nil, errNoGatewayForEnvironment
	}
	candidates := make([]*models.Gateway, 0, len(gateways))
	for _, gw := range gateways {
		if gw.IsEgressCapable() {
			candidates = append(candidates, gw)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("%w (environment %s)", errNoEgressGatewayForEnvironment, envIDStr)
	}
	return candidates, nil
}

// gatewayHasMCPIdentityPolicies reports whether the gateway's policy manifest
// advertises both mcp-auth v1 and mcp-authz v1 (the policies identity mode emits).
func gatewayHasMCPIdentityPolicies(gateway *models.Gateway) bool {
	need := map[string]bool{"mcp-auth\x00v1": false, "mcp-authz\x00v1": false}
	for _, item := range extractGatewayPolicyManifestItems(gatewayManifest(gateway)) {
		key := item.Name + "\x00" + normalizePolicyVersionToMajor(item.Version)
		if _, ok := need[key]; ok {
			need[key] = true
		}
	}
	return need["mcp-auth\x00v1"] && need["mcp-authz\x00v1"]
}

// storableMCPEndpoint is the persistence-ready form of one endpoint: the endpoint config
// (with encrypted auth) plus the (environment UUID -> stable artifact UUID) bindings.
type storableMCPEndpoint struct {
	handle       string
	name         string
	config       models.MCPEndpointConfig
	envArtifacts map[string]uuid.UUID // environment UUID -> artifact UUID
}

// existingMCPEndpointIndex indexes a proxy's currently-stored endpoints so an update can
// preserve stable per-environment artifact UUIDs (agent bindings resolve (proxy, env) ->
// artifact_uuid) and per-endpoint upstream secrets when the client omits credentials.
type existingMCPEndpointIndex struct {
	// authByEndpointHandle carries the stored upstream auth for credential preservation.
	authByEndpointHandle map[string]*models.UpstreamAuth
	// artifactByEnv is keyed by environment UUID: uq_proxy_env_single guarantees one
	// artifact per environment per proxy, so the identity survives an endpoint remap.
	artifactByEnv map[string]uuid.UUID
}

func indexExistingMCPEndpoints(endpoints []models.MCPProxyEndpoint) existingMCPEndpointIndex {
	idx := existingMCPEndpointIndex{
		authByEndpointHandle: map[string]*models.UpstreamAuth{},
		artifactByEnv:        map[string]uuid.UUID{},
	}
	for _, endpoint := range endpoints {
		if endpoint.Configuration.Upstream != nil {
			idx.authByEndpointHandle[strings.TrimSpace(endpoint.Handle)] = endpoint.Configuration.Upstream.Auth
		}
		for _, ee := range endpoint.Environments {
			if ee.ArtifactUUID != uuid.Nil {
				idx.artifactByEnv[ee.EnvironmentUUID.String()] = ee.ArtifactUUID
			}
		}
	}
	return idx
}

// buildMCPEndpointsForStorage normalizes incoming endpoint DTOs for persistence: it encrypts
// each endpoint's upstream auth (preserving a previously stored secret when the client omits
// the credential, matched by endpoint handle), applies default security, and allocates a
// stable artifact UUID per (endpoint, environment) binding — preserving the previous one when
// the environment stays bound. Call validateMCPEndpoints first. existing may be nil on create.
// This performs no network I/O, so it is safe to call inside a transaction.
func (s *MCPProxyService) buildMCPEndpointsForStorage(incoming []models.MCPProxyEndpointDTO, existing *existingMCPEndpointIndex) ([]storableMCPEndpoint, error) {
	authByHandle := map[string]*models.UpstreamAuth{}
	artifactByEnv := map[string]uuid.UUID{}
	if existing != nil {
		authByHandle = existing.authByEndpointHandle
		artifactByEnv = existing.artifactByEnv
	}

	out := make([]storableMCPEndpoint, 0, len(incoming))
	for _, incomingEndpoint := range incoming {
		handle := strings.TrimSpace(incomingEndpoint.ID)
		config := models.MCPEndpointConfig{
			Resilience:   incomingEndpoint.Resilience,
			Policies:     copyMCPPolicies(policiesFromPtr(incomingEndpoint.Policies)),
			Capabilities: copyMCPCapabilities(incomingEndpoint.Capabilities),
			Security:     defaultMCPProxySecurity(incomingEndpoint.Security),
		}
		if incomingEndpoint.Upstream.Main != nil {
			endpoint := *incomingEndpoint.Upstream.Main
			endpoint.URL = strings.TrimSpace(endpoint.URL)
			auth, err := s.prepareMCPUpstreamAuthForStorage(authByHandle[handle], incomingEndpoint.Upstream.Main.Auth)
			if err != nil {
				return nil, err
			}
			endpoint.Auth = auth
			config.Upstream = &endpoint
		}

		envArtifacts := make(map[string]uuid.UUID, len(incomingEndpoint.Environments))
		for _, env := range incomingEndpoint.Environments {
			envUUID := strings.TrimSpace(env.EnvironmentUUID)
			// The single gateway artifact deployed for this (endpoint, environment) is
			// identified by a stable UUID: preserve the previously stored one when the
			// environment stays bound, mint a new one otherwise.
			artifactUUID := artifactByEnv[envUUID]
			if artifactUUID == uuid.Nil {
				artifactUUID = uuid.New()
			}
			envArtifacts[envUUID] = artifactUUID
		}

		out = append(out, storableMCPEndpoint{
			handle:       handle,
			name:         strings.TrimSpace(incomingEndpoint.Name),
			config:       config,
			envArtifacts: envArtifacts,
		})
	}
	return out, nil
}

// applyRequestedGatewayUUIDs copies each request DTO's caller-specified gatewayId onto the
// matching (endpoint, environment) binding of a freshly-reloaded proxy, transiently and
// in-memory only — RequestedGatewayUUID is gorm:"-", so this never persists. Create/Update
// reload the proxy from the DB (losing any in-memory state) before deploying, so this must
// run on that reloaded copy, right before deployMCPProxyEndpoints. uq_proxy_env_single
// guarantees at most one binding per environment per proxy, so keying by environment UUID
// alone is unambiguous.
func applyRequestedGatewayUUIDs(proxy *models.MCPProxy, endpoints []models.MCPProxyEndpointDTO) {
	requested := map[string]*string{}
	for _, endpoint := range endpoints {
		for _, env := range endpoint.Environments {
			envUUID := strings.TrimSpace(env.EnvironmentUUID)
			if envUUID == "" || env.GatewayID == nil {
				continue
			}
			gwID := strings.TrimSpace(*env.GatewayID)
			if gwID == "" {
				continue
			}
			requested[envUUID] = &gwID
		}
	}
	if len(requested) == 0 {
		return
	}
	for i := range proxy.Endpoints {
		endpoint := &proxy.Endpoints[i]
		for j := range endpoint.Environments {
			ee := &endpoint.Environments[j]
			if gwID, ok := requested[ee.EnvironmentUUID.String()]; ok {
				ee.RequestedGatewayUUID = gwID
			}
		}
	}
}

// persistMCPEndpoints inserts the endpoint rows and their (endpoint, environment) join rows
// for the proxy. The artifacts row backing each (endpoint, env) deployment is created lazily
// at deploy time (ensureMCPProxyEnvArtifactRow); here we only record the intended identity.
func (s *MCPProxyService) persistMCPEndpoints(ctx context.Context, tx *gorm.DB, proxyUUID uuid.UUID, proxyHandle, proxyVersion string, endpoints []storableMCPEndpoint, ouID string) error {
	now := time.Now()
	for _, endpoint := range endpoints {
		endpointUUID := uuid.New()
		row := &models.MCPProxyEndpoint{
			UUID:          endpointUUID,
			MCPProxyUUID:  proxyUUID,
			Handle:        endpoint.handle,
			Name:          endpoint.name,
			Status:        models.StatusCreated,
			Configuration: endpoint.config,
		}
		if err := s.endpointRepo.CreateEndpoint(ctx, tx, row); err != nil {
			return err
		}
		for envUUIDStr, artifactUUID := range endpoint.envArtifacts {
			envUUID, err := uuid.Parse(envUUIDStr)
			if err != nil {
				return fmt.Errorf("%w: invalid environment id %q: %w", utils.ErrInvalidInput, envUUIDStr, err)
			}
			// The join row's artifact_uuid FKs to artifacts (fk_endpoint_env_artifact), so
			// the backing artifacts row must exist before the join row is inserted. Create
			// it now, in the same transaction, with the same stable handle the deploy path
			// (ensureMCPProxyEnvArtifactRow) computes — so deploy-time creation no-ops. On
			// update a preserved environment keeps its stable artifact UUID and row (join
			// rows cascade-delete, artifacts rows do not); the row is keyed by that UUID
			// (its primary key), so existence is checked by UUID — a preserved artifact
			// whose environment remapped to a different endpoint (new handle) is still
			// found, avoiding a primary-key collision on re-create.
			artifactHandle := mcpProxyEnvArtifactHandle(proxyHandle, endpoint.handle, envUUIDStr)
			if _, err := s.artifactRepo.GetByUUID(artifactUUID.String(), ouID); errors.Is(err, utils.ErrArtifactNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
				if err := s.artifactRepo.Create(tx.WithContext(ctx), &models.Artifact{
					UUID:      artifactUUID,
					Handle:    artifactHandle,
					Name:      artifactHandle,
					Version:   proxyVersion,
					Kind:      models.KindMCPMapping,
					OUID:      ouID,
					CreatedAt: now,
					UpdatedAt: now,
				}); err != nil {
					return fmt.Errorf("failed to create MCP proxy endpoint environment artifact: %w", err)
				}
			} else if err != nil {
				return fmt.Errorf("failed to check MCP proxy endpoint environment artifact: %w", err)
			}
			if err := s.endpointRepo.AddEndpointEnvironment(ctx, tx, &models.MCPProxyEndpointEnvironment{
				MCPProxyUUID:         proxyUUID,
				EndpointUUID:         endpointUUID,
				EnvironmentUUID:      envUUID,
				ArtifactUUID:         artifactUUID,
				Status:               models.StatusCreated,
				RequestedGatewayUUID: nil,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// mapMCPProxyWriteError maps Postgres unique-violation errors from the write path to
// friendly sentinels. It distinguishes the proxy-handle collision (uq_artifact_handle_ou_id)
// from the name+version collision (uq_artifact_name_version_ou_id) and the
// environment-already-bound collision (uq_proxy_env_single / uq_endpoint_env). Returns nil
// when err is not a recognized unique violation, so the caller falls back to a generic error.
//
// uq_artifact_handle_ou_id is a table-wide constraint shared by every artifact kind (LLM
// providers, MCP proxies, ...), not just MCP proxies — a handle collision with a
// differently-kinded artifact must not be reported as "MCP proxy already exists". handle
// and orgUUID let it resolve the conflicting artifact's actual kind before deciding.
func (s *MCPProxyService) mapMCPProxyWriteError(err error, handle, orgUUID string) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return nil
	}
	switch pgErr.ConstraintName {
	case "uq_proxy_env_single", "uq_endpoint_env":
		return fmt.Errorf("%w", utils.ErrMCPEnvAlreadyBound)
	case "uq_mcp_endpoint_handle":
		return fmt.Errorf("%w: duplicate endpoint id", utils.ErrInvalidInput)
	case "uq_artifact_handle_ou_id":
		if conflicting, getErr := s.artifactRepo.GetByHandle(handle, orgUUID); getErr == nil &&
			conflicting != nil && conflicting.Kind != models.KindMCPProxy {
			return fmt.Errorf("%w: handle already in use by another artifact", utils.ErrArtifactExists)
		}
		return fmt.Errorf("%w: handle already in use", utils.ErrMCPProxyExists)
	case "uq_artifact_name_version_ou_id":
		return fmt.Errorf("%w: another artifact already uses this name and version", utils.ErrInvalidInput)
	default:
		return nil
	}
}

// policiesFromPtr dereferences the DTO's optional policies slice pointer.
func policiesFromPtr(policies *[]models.MCPPolicy) []models.MCPPolicy {
	if policies == nil {
		return nil
	}
	return *policies
}

func (s *MCPProxyService) fetchMCPServerInfo(ctx context.Context, endpointURL string, headerName string, headerValue string) (*models.MCPServerInfoFetchResponse, error) {
	sessionID, serverInfo, err := s.initializeMCPServer(ctx, endpointURL, headerName, headerValue)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MCP server: %w", err)
	}

	notifyReq := mcpJSONRPCRequest{
		JSONRPC: mcpJSONRPCVersion,
		Method:  mcpMethodInitialized,
	}
	if _, err := s.postJSONRPCWithSession(ctx, endpointURL, notifyReq, sessionID, headerName, headerValue); err != nil {
		return nil, fmt.Errorf("failed to send notification: %w", err)
	}

	resp := &models.MCPServerInfoFetchResponse{}
	if serverInfo != nil {
		resp.ServerInfo = &serverInfo
	}

	if tools := s.fetchTools(ctx, endpointURL, sessionID, headerName, headerValue); len(tools) > 0 {
		resp.Tools = &tools
	}
	if prompts := s.fetchPrompts(ctx, endpointURL, sessionID, headerName, headerValue); len(prompts) > 0 {
		resp.Prompts = &prompts
	}
	if resources := s.fetchResources(ctx, endpointURL, sessionID, headerName, headerValue); len(resources) > 0 {
		resp.Resources = &resources
	}

	return resp, nil
}

func (s *MCPProxyService) initializeMCPServer(ctx context.Context, endpointURL string, headerName string, headerValue string) (string, map[string]interface{}, error) {
	initReq := mcpJSONRPCRequest{
		JSONRPC: mcpJSONRPCVersion,
		ID:      1,
		Method:  mcpMethodInitialize,
		Params: map[string]interface{}{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]interface{}{"roots": map[string]bool{"listChanged": true}},
			"clientInfo":      map[string]string{"name": mcpClientName, "version": mcpClientVersion},
		},
	}

	body, headers, err := s.postJSONRPC(ctx, endpointURL, initReq, "", headerName, headerValue)
	if err != nil {
		return "", nil, err
	}

	var initResult mcpInitializeResult
	if err := json.Unmarshal(body, &initResult); err != nil {
		return "", nil, fmt.Errorf("failed to parse initialize response: %w, body: %s", err, string(body))
	}
	if initResult.Error != nil {
		return "", nil, fmt.Errorf("initialize request returned an error: %s", initResult.Error.Message)
	}

	return headers.Get(mcpSessionHeader), initResult.Result.ServerInfo, nil
}

func (s *MCPProxyService) fetchTools(ctx context.Context, endpointURL string, sessionID string, headerName string, headerValue string) []map[string]interface{} {
	req := mcpJSONRPCRequest{JSONRPC: mcpJSONRPCVersion, ID: 2, Method: mcpMethodToolsList}
	body, err := s.postJSONRPCWithSession(ctx, endpointURL, req, sessionID, headerName, headerValue)
	if err != nil {
		s.logger.Warn("Failed to fetch MCP tools, continuing with available info", "error", err)
		return nil
	}
	var result mcpToolsResult
	if err := json.Unmarshal(body, &result); err != nil {
		s.logger.Warn("Failed to parse MCP tools response, continuing with available info", "error", err)
		return nil
	}
	if result.Error != nil {
		s.logger.Warn("tools/list returned an error, continuing with available info", "error", result.Error.Message)
		return nil
	}
	return result.Result.Tools
}

func (s *MCPProxyService) fetchPrompts(ctx context.Context, endpointURL string, sessionID string, headerName string, headerValue string) []map[string]interface{} {
	req := mcpJSONRPCRequest{JSONRPC: mcpJSONRPCVersion, ID: 3, Method: mcpMethodPromptsList}
	body, err := s.postJSONRPCWithSession(ctx, endpointURL, req, sessionID, headerName, headerValue)
	if err != nil {
		s.logger.Warn("Failed to fetch MCP prompts, continuing with available info", "error", err)
		return nil
	}
	var result mcpPromptsResult
	if err := json.Unmarshal(body, &result); err != nil {
		s.logger.Warn("Failed to parse MCP prompts response, continuing with available info", "error", err)
		return nil
	}
	if result.Error != nil {
		s.logger.Warn("prompts/list returned an error, continuing with available info", "error", result.Error.Message)
		return nil
	}
	return result.Result.Prompts
}

func (s *MCPProxyService) fetchResources(ctx context.Context, endpointURL string, sessionID string, headerName string, headerValue string) []map[string]interface{} {
	req := mcpJSONRPCRequest{JSONRPC: mcpJSONRPCVersion, ID: 4, Method: mcpMethodResourcesList}
	body, err := s.postJSONRPCWithSession(ctx, endpointURL, req, sessionID, headerName, headerValue)
	if err != nil {
		s.logger.Warn("Failed to fetch MCP resources, continuing with available info", "error", err)
		return nil
	}
	var result mcpResourcesResult
	if err := json.Unmarshal(body, &result); err != nil {
		s.logger.Warn("Failed to parse MCP resources response, continuing with available info", "error", err)
		return nil
	}
	if result.Error != nil {
		s.logger.Warn("resources/list returned an error, continuing with available info", "error", result.Error.Message)
		return nil
	}
	return result.Result.Resources
}

func (s *MCPProxyService) postJSONRPCWithSession(ctx context.Context, endpointURL string, payload interface{}, sessionID string, headerName string, headerValue string) ([]byte, error) {
	body, _, err := s.postJSONRPC(ctx, endpointURL, payload, sessionID, headerName, headerValue)
	return body, err
}

func (s *MCPProxyService) postJSONRPC(ctx context.Context, endpointURL string, payload interface{}, sessionID string, headerName string, headerValue string) ([]byte, http.Header, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewBuffer(data))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	setSafeHeader(httpReq.Header, headerName, headerValue)
	if sessionID != "" {
		httpReq.Header.Set(mcpSessionHeader, sessionID)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, utils.ErrInvalidURL) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("%w: failed to reach MCP server: %w", utils.ErrURLUnreachable, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			s.logger.Warn("Failed to close MCP server response body", "error", closeErr)
		}
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMCPResponseBody+1))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if len(body) > maxMCPResponseBody {
		return nil, nil, fmt.Errorf("%w: response body exceeds %d bytes", utils.ErrMCPResponseTooLarge, maxMCPResponseBody)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, nil, utils.ErrMCPServerUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}
	if isMCPEventStream(resp) {
		data, err := parseMCPEventStream(body)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse event stream: %w, body: %s", err, string(body))
		}
		body = data
	}

	return body, resp.Header, nil
}

func setSafeHeader(headers http.Header, headerName string, headerValue string) {
	if strings.TrimSpace(headerName) == "" {
		return
	}
	switch strings.ToLower(headerName) {
	case strings.ToLower(mcpSessionHeader), "content-type", "accept":
		return
	default:
		headers.Set(headerName, headerValue)
	}
}

func parseMCPEventStream(body []byte) ([]byte, error) {
	lines := bytes.Split(body, []byte("\n"))
	var eventData bytes.Buffer
	hasData := false
	flushEvent := func() []byte {
		if !hasData {
			return nil
		}
		data := eventData.Bytes()
		eventData.Reset()
		hasData = false
		if len(data) > 0 && !bytes.Equal(data, []byte("{}")) {
			return data
		}
		return nil
	}

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			if data := flushEvent(); data != nil {
				return data, nil
			}
			continue
		}
		data, ok := bytes.CutPrefix(line, []byte("data:"))
		if !ok {
			continue
		}
		data = bytes.TrimSpace(data)
		if hasData {
			eventData.WriteByte('\n')
		}
		eventData.Write(data)
		hasData = true
	}
	if data := flushEvent(); data != nil {
		return data, nil
	}
	return nil, errors.New("no data found in event stream")
}

func isMCPEventStream(resp *http.Response) bool {
	return strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
}

type mcpJSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type mcpJSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpInitializeResult struct {
	Result struct {
		ProtocolVersion string                 `json:"protocolVersion"`
		ServerInfo      map[string]interface{} `json:"serverInfo"`
		Capabilities    map[string]interface{} `json:"capabilities"`
	} `json:"result"`
	Error *mcpJSONRPCError `json:"error"`
}

type mcpToolsResult struct {
	Result struct {
		Tools []map[string]interface{} `json:"tools"`
	} `json:"result"`
	Error *mcpJSONRPCError `json:"error"`
}

type mcpPromptsResult struct {
	Result struct {
		Prompts []map[string]interface{} `json:"prompts"`
	} `json:"result"`
	Error *mcpJSONRPCError `json:"error"`
}

type mcpResourcesResult struct {
	Result struct {
		Resources []map[string]interface{} `json:"resources"`
	} `json:"result"`
	Error *mcpJSONRPCError `json:"error"`
}
