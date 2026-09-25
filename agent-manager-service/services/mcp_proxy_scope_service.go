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
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// MCPProxyScopeService manages per-proxy MCP scopes: validation, CRUD, and
// (in later tasks) Thunder cleanup + re-emit.
type MCPProxyScopeService interface {
	List(ctx context.Context, ouID, proxyHandle string) (*models.MCPProxyScopesResult, error)
	Create(ctx context.Context, ouID, orgName, proxyHandle string, in models.MCPProxyScopeInput) (*models.MCPProxyScopeResult, error)
	Update(ctx context.Context, ouID, orgName, proxyHandle, action string, in models.MCPProxyScopeUpdateInput) (*models.MCPProxyScopeResult, error)
	Delete(ctx context.Context, ouID, orgName, proxyHandle, action string) error
	ListEnvironmentScopes(ctx context.Context, ouID, envName string) ([]models.EnvironmentScopeEntry, error)
}

// MCPProxyRedeployer is the narrow MCPProxyService surface scope mutations
// need: gateway re-emission and resource-server ensure after a DB write.
type MCPProxyRedeployer interface {
	RedeployMCPProxy(ctx context.Context, proxy *models.MCPProxy, ouID string) error
	// EnsureResourceServersForProxy registers proxy's resource server and
	// actions in every identity-secured, deployed environment.
	EnsureResourceServersForProxy(ctx context.Context, ouID string, proxy *models.MCPProxy, actions []string)
}

type mcpProxyScopeService struct {
	scopeRepo    repositories.MCPProxyScopeRepository
	proxyRepo    repositories.MCPProxyRepository
	infraManager InfraResourceManager
	resolver     thundersvc.EnvThunderResolver
	proxySvc     MCPProxyRedeployer
	logger       *slog.Logger
}

// NewMCPProxyScopeService creates a new per-proxy MCP scope service.
func NewMCPProxyScopeService(
	scopeRepo repositories.MCPProxyScopeRepository,
	proxyRepo repositories.MCPProxyRepository,
	infraManager InfraResourceManager,
	resolver thundersvc.EnvThunderResolver,
	proxySvc MCPProxyRedeployer,
	logger *slog.Logger,
) MCPProxyScopeService {
	return &mcpProxyScopeService{
		scopeRepo:    scopeRepo,
		proxyRepo:    proxyRepo,
		infraManager: infraManager,
		resolver:     resolver,
		proxySvc:     proxySvc,
		logger:       logger,
	}
}

// mcpScopeActionRe constrains scope actions. No ":" (the scope string splits on the
// first ":") and no "/" (Thunder would accept it; we keep scope strings flat).
// 100 mirrors Thunder's per-handle cap; the combined permission column allows 1000.
var mcpScopeActionRe = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,100}$`)

// proxyHandleOf returns the proxy's artifact handle, the authoritative source
// (models.MCPProxy.Handle is a gorm:"-" transient that repos never populate).
func proxyHandleOf(proxy *models.MCPProxy) string {
	if proxy.Artifact != nil && proxy.Artifact.Handle != "" {
		return proxy.Artifact.Handle
	}
	return proxy.Handle
}

func (s *mcpProxyScopeService) resolveProxy(ctx context.Context, ouID, proxyHandle string) (*models.MCPProxy, error) {
	proxy, err := s.proxyRepo.GetByHandle(ctx, proxyHandle, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: proxy handle %q", utils.ErrMCPProxyNotFound, proxyHandle)
		}
		return nil, fmt.Errorf("failed to resolve MCP proxy %q: %w", proxyHandle, err)
	}
	return proxy, nil
}

// proxyToolUnion collects every discovered tool name across the proxy's
// endpoints. An empty map means "no capabilities stored" — validation is then
// permissive (strict-when-known policy).
func proxyToolUnion(proxy *models.MCPProxy) map[string]struct{} {
	union := map[string]struct{}{}
	for i := range proxy.Endpoints {
		caps := proxy.Endpoints[i].Configuration.Capabilities
		if caps == nil || caps.Tools == nil {
			continue
		}
		for _, tool := range *caps.Tools {
			if name, ok := tool["name"].(string); ok && name != "" {
				union[name] = struct{}{}
			}
		}
	}
	return union
}

// validateScopeTools normalizes a scope's tool list and, when the proxy's
// capabilities are known, restricts it to tools the proxy actually discovered.
// An empty list is legal and yields an empty (never nil) slice: a scope bound to
// no tools is a declared-but-unbound catalog entry, grantable to a role but
// enforced on nothing — see
// TestAppendMCPIdentityAuthPolicies_ScopeWithNoToolsEmitsAuthOnly. Non-nil
// matters — tools is a jsonb column, and nil would persist as null.
func validateScopeTools(tools []string, union map[string]struct{}) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tools))
	for _, tl := range tools {
		tl = strings.TrimSpace(tl)
		if tl == "" {
			return nil, fmt.Errorf("%w: tool names must be non-empty", utils.ErrInvalidInput)
		}
		if _, dup := seen[tl]; dup {
			return nil, fmt.Errorf("%w: duplicate tool %q", utils.ErrInvalidInput, tl)
		}
		if len(union) > 0 {
			if _, known := union[tl]; !known {
				return nil, fmt.Errorf("%w: tool %q is not among the proxy's discovered tools", utils.ErrInvalidInput, tl)
			}
		}
		seen[tl] = struct{}{}
		out = append(out, tl)
	}
	sort.Strings(out)
	return out, nil
}

func (s *mcpProxyScopeService) Create(ctx context.Context, ouID, orgName, proxyHandle string, in models.MCPProxyScopeInput) (*models.MCPProxyScopeResult, error) {
	_ = orgName // unused: env-Thunder calls below are resolved by ouID, not orgName

	if !mcpScopeActionRe.MatchString(in.Action) {
		return nil, fmt.Errorf("%w: action must match ^[A-Za-z0-9._\\-]{1,100}$", utils.ErrInvalidInput)
	}

	proxy, err := s.resolveProxy(ctx, ouID, proxyHandle)
	if err != nil {
		return nil, err
	}

	if _, err := s.scopeRepo.Get(ctx, proxy.UUID, in.Action); err == nil {
		return nil, fmt.Errorf("%w: scope action %q already exists on proxy %q", utils.ErrConflict, in.Action, proxyHandle)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check scope existence: %w", err)
	}

	union := proxyToolUnion(proxy)
	tools, err := validateScopeTools(in.Tools, union)
	if err != nil {
		return nil, err
	}

	scope := &models.MCPProxyScope{
		MCPProxyUUID: proxy.UUID,
		Action:       in.Action,
		Description:  in.Description,
		Tools:        tools,
	}
	if err := s.scopeRepo.Create(ctx, scope); err != nil {
		// A concurrent create can win the race between the Get check above and
		// this insert; the unique constraint (uq_mcp_proxy_scopes_proxy_action)
		// then fires. Map it to a conflict so the API returns 409.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("%w: scope action %q already exists on proxy %q", utils.ErrConflict, in.Action, proxyHandle)
		}
		return nil, fmt.Errorf("failed to create mcp proxy scope: %w", err)
	}

	go s.ensureResourceServerEverywhere(context.WithoutCancel(ctx), ouID, proxy)

	if err := s.proxySvc.RedeployMCPProxy(ctx, proxy, ouID); err != nil {
		return nil, fmt.Errorf("scope created but gateway re-emission failed (retry by redeploying the proxy): %w", err)
	}

	return &models.MCPProxyScopeResult{
		ProxyHandle: proxyHandleOf(proxy),
		Scope:       *scope,
	}, nil
}

func (s *mcpProxyScopeService) Update(ctx context.Context, ouID, orgName, proxyHandle, action string, in models.MCPProxyScopeUpdateInput) (*models.MCPProxyScopeResult, error) {
	_ = orgName // unused: env-Thunder calls below are resolved by ouID, not orgName

	proxy, err := s.resolveProxy(ctx, ouID, proxyHandle)
	if err != nil {
		return nil, err
	}

	existing, err := s.scopeRepo.Get(ctx, proxy.UUID, action)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrScopeNotFound
		}
		return nil, fmt.Errorf("failed to load scope: %w", err)
	}

	if in.Description != nil {
		existing.Description = *in.Description
	}
	if in.Tools != nil {
		union := proxyToolUnion(proxy)
		tools, err := validateScopeTools(in.Tools, union)
		if err != nil {
			return nil, err
		}
		existing.Tools = tools
	}

	if err := s.scopeRepo.Update(ctx, existing); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrScopeNotFound
		}
		return nil, fmt.Errorf("failed to update mcp proxy scope: %w", err)
	}

	go s.ensureResourceServerEverywhere(context.WithoutCancel(ctx), ouID, proxy)

	if err := s.proxySvc.RedeployMCPProxy(ctx, proxy, ouID); err != nil {
		return nil, fmt.Errorf("scope updated but gateway re-emission failed (retry by redeploying the proxy): %w", err)
	}

	return &models.MCPProxyScopeResult{
		ProxyHandle: proxyHandleOf(proxy),
		Scope:       *existing,
	}, nil
}

func (s *mcpProxyScopeService) Delete(ctx context.Context, ouID, orgName, proxyHandle, action string) error {
	_ = orgName // unused: cleanup now resolves env-Thunder by ouID, not orgName

	proxy, err := s.resolveProxy(ctx, ouID, proxyHandle)
	if err != nil {
		return err
	}

	if err := s.scopeRepo.Delete(ctx, proxy.UUID, action); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrScopeNotFound
		}
		return fmt.Errorf("failed to delete mcp proxy scope: %w", err)
	}

	s.cleanupDeletedScope(ctx, ouID, proxy, action)

	if err := s.proxySvc.RedeployMCPProxy(ctx, proxy, ouID); err != nil {
		return fmt.Errorf("scope deleted but gateway re-emission failed (retry by redeploying the proxy): %w", err)
	}
	return nil
}

// ensureResourceServerEverywhere loads proxy's current scopes and delegates
// to MCPProxyService.EnsureResourceServersForProxy.
func (s *mcpProxyScopeService) ensureResourceServerEverywhere(ctx context.Context, ouID string, proxy *models.MCPProxy) {
	hasIdentityEndpoint := false
	for i := range proxy.Endpoints {
		if mcpIdentityEnabled(proxy.Endpoints[i].Configuration.Security) {
			hasIdentityEndpoint = true
			break
		}
	}
	if !hasIdentityEndpoint {
		return
	}

	scopes, err := s.scopeRepo.ListByProxy(ctx, proxy.UUID)
	if err != nil {
		s.logger.Warn("ensure resource server (scope save): listing scopes failed", "proxy", proxyHandleOf(proxy), "error", err)
		return
	}
	actions := make([]string, 0, len(scopes))
	for _, sc := range scopes {
		actions = append(actions, sc.Action)
	}
	s.proxySvc.EnsureResourceServersForProxy(ctx, ouID, proxy, actions)
}

// cleanupDeletedScope best-effort removes the deleted scope's Thunder action and
// strips the dangling permission string from every role, in every environment the
// proxy is deployed to. Never returns an error — failures log. Does not gate on
// the endpoint's identity-security flag: a role can reference (and Thunder-ensure)
// a proxy's resource server regardless of that flag (agent_identity_controller.go
// resolveScopeGroups performs no such check), so cleanup must attempt the same
// unconditionally or it leaks the resource server whenever security was disabled.
func (s *mcpProxyScopeService) cleanupDeletedScope(ctx context.Context, ouID string, proxy *models.MCPProxy, action string) {
	handle := proxyHandleOf(proxy)
	scopeStr := handle + ":" + action

	envs, err := s.infraManager.ListOrgEnvironments(ctx, ouID)
	if err != nil {
		s.logger.Warn("scope cleanup: listing environments failed", "scope", scopeStr, "error", err)
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
				s.logger.Warn("scope cleanup: env-Thunder unavailable", "env", name, "scope", scopeStr, "error", err)
				continue
			}
			rsID, err := client.DeleteProxyResourceServerAction(ctx, handle, action)
			if err != nil {
				s.logger.Warn("scope cleanup: action delete failed", "env", name, "scope", scopeStr, "error", err)
				continue
			}
			if rsID == "" {
				continue // RS never projected into this env — nothing to sweep
			}
			sweepRolePermission(ctx, s.logger, client, name, scopeStr)
		}
	}
}

// sweepRolePermission strips scopeStr from every role carrying it, regardless of
// which resource-server group holds it — RemoveRolePermissions uses the ID from
// the matched group itself, so the caller doesn't need a resource-server ID up
// front (e.g. after a whole-RS delete, which returns none). A package-level func
// so both MCPProxyService.Delete and mcpProxyScopeService.cleanupDeletedScope can
// share it without duplicating the env-Thunder-OU-trap-prone ListRoles walk.
// ListRoles("") pages all roles WITH permissions populated, so filtering is
// in-memory (same pattern as GetGroupRoles, identity_client.go).
func sweepRolePermission(ctx context.Context, logger *slog.Logger, client thundersvc.EnvIdentityClient, envName, scopeStr string) {
	const pageSize = 50
	for offset := 0; ; {
		roles, total, err := client.ListRoles(ctx, "", offset, pageSize)
		if err != nil {
			logger.Warn("scope cleanup: role sweep aborted", "env", envName, "scope", scopeStr, "error", err)
			return
		}
		for _, role := range roles {
			for _, grp := range role.Permissions {
				if !slices.Contains(grp.Permissions, scopeStr) {
					continue
				}
				if err := client.RemoveRolePermissions(ctx, role.ID, thundersvc.RolePermissionRequest{
					ResourceServerID: grp.ResourceServerID, Permissions: []string{scopeStr},
				}); err != nil {
					logger.Warn("scope cleanup: permission strip failed", "env", envName, "role", role.ID, "scope", scopeStr, "error", err)
				}
			}
		}
		offset += len(roles)
		if offset >= total || len(roles) == 0 {
			return
		}
	}
}

func (s *mcpProxyScopeService) List(ctx context.Context, ouID, proxyHandle string) (*models.MCPProxyScopesResult, error) {
	proxy, err := s.resolveProxy(ctx, ouID, proxyHandle)
	if err != nil {
		return nil, err
	}

	scopes, err := s.scopeRepo.ListByProxy(ctx, proxy.UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list mcp proxy scopes: %w", err)
	}

	return &models.MCPProxyScopesResult{
		ProxyHandle: proxyHandleOf(proxy),
		Scopes:      scopes,
	}, nil
}

func (s *mcpProxyScopeService) ListEnvironmentScopes(ctx context.Context, ouID, envName string) ([]models.EnvironmentScopeEntry, error) {
	envUUID, err := environmentUUIDByName(ctx, s.infraManager, ouID, envName)
	if err != nil {
		return nil, err
	}

	const pageSize = 100
	proxyByUUID := map[uuid.UUID]*models.MCPProxy{}
	var ordered []uuid.UUID
	for offset := 0; ; offset += pageSize {
		proxies, err := s.proxyRepo.List(ctx, ouID, pageSize, offset)
		if err != nil {
			return nil, fmt.Errorf("failed to list mcp proxies: %w", err)
		}
		if len(proxies) == 0 {
			break
		}

		for _, proxy := range proxies {
			endpoint, ee := resolveMCPEndpointForEnv(proxy, envUUID.String())
			if endpoint == nil {
				continue
			}
			if !mcpIdentityEnabled(endpoint.Configuration.Security) {
				continue
			}
			if ee.ArtifactUUID == uuid.Nil {
				continue
			}
			if _, ok := proxyByUUID[proxy.UUID]; !ok {
				proxyByUUID[proxy.UUID] = proxy
				ordered = append(ordered, proxy.UUID)
			}
		}

		if len(proxies) < pageSize {
			break
		}
	}

	if len(ordered) == 0 {
		return []models.EnvironmentScopeEntry{}, nil
	}

	scopes, err := s.scopeRepo.ListByProxyUUIDs(ctx, ordered)
	if err != nil {
		return nil, fmt.Errorf("failed to list mcp proxy scopes: %w", err)
	}

	entries := make([]models.EnvironmentScopeEntry, 0, len(scopes))
	for _, sc := range scopes {
		proxy, ok := proxyByUUID[sc.MCPProxyUUID]
		if !ok {
			continue
		}
		handle := proxyHandleOf(proxy)
		entries = append(entries, models.EnvironmentScopeEntry{
			Scope:        sc.ScopeString(handle),
			Description:  sc.Description,
			MCPProxyID:   handle,
			MCPProxyName: proxy.Artifact.Name,
		})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Scope < entries[j].Scope })
	return entries, nil
}
