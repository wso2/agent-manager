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
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// PromotedIdentityResource names one identity resource (a role or a group)
// carried with the agent. Only the id and the name travel: the target keeps the
// same id, and role assignments and group members are carried by id.
type PromotedIdentityResource struct {
	ID   string
	Name string
}

// IdentityPromotionErrorKind classifies why an identity promotion was refused.
type IdentityPromotionErrorKind string

const (
	// IdentityPromotionUnavailable means an identity service could not be reached
	// or could not complete the promotion right now. Retrying may succeed.
	IdentityPromotionUnavailable IdentityPromotionErrorKind = "unavailable"
	// IdentityPromotionForbidden means the caller or deployment may not promote
	// into the target environment.
	IdentityPromotionForbidden IdentityPromotionErrorKind = "forbidden"
	// IdentityPromotionConflict means the target holds state that the promotion
	// would overwrite, or the two environments disagree.
	IdentityPromotionConflict IdentityPromotionErrorKind = "conflict"
	// IdentityPromotionMissingValues means the target environment lacks values
	// the carried resources reference. MissingValues names them.
	IdentityPromotionMissingValues IdentityPromotionErrorKind = "missing_values"
	// IdentityPromotionBlocked means the promotion cannot proceed until the user
	// changes something, for example a role that references a resource server
	// Agent Manager does not manage.
	IdentityPromotionBlocked IdentityPromotionErrorKind = "blocked"
)

// IdentityPromotionError is the typed refusal of an identity promotion.
// Message is shown to the user, so it must name resources (roles, groups,
// resource servers) and never carry a secret, a token, an internal hostname or
// a raw upstream response. MissingValues holds value NAMES, which are for the
// deployment that raised the error: only their count is ever rendered. Cause is
// for logs and errors.Is/As; it is never rendered to the user.
type IdentityPromotionError struct {
	Kind          IdentityPromotionErrorKind
	Message       string
	MissingValues []string
	Cause         error
}

// Error renders the kind, the user safe message and how many values are missing.
// The value names and the cause are deliberately left out: both can name internal
// components, and this string reaches logs.
func (e *IdentityPromotionError) Error() string {
	if e == nil {
		return "identity promotion error"
	}
	msg := e.Message
	if msg == "" {
		msg = "the identity promotion was refused"
	}
	if len(e.MissingValues) > 0 {
		msg += fmt.Sprintf(" (%d required value(s) are missing)", len(e.MissingValues))
	}
	return "identity promotion " + string(e.Kind) + ": " + msg
}

// Unwrap exposes the cause to errors.Is and errors.As.
func (e *IdentityPromotionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// AgentIdentityResourceQuery asks for everything an agent identity carries
// besides the application itself.
type AgentIdentityResourceQuery struct {
	OUID        string
	ProjectName string
	AgentName   string
	// ThunderAgentID is the carried agent application's id.
	ThunderAgentID string
	// DefaultEnvironment is the lowest environment of the project pipeline. Roles
	// and groups exist only there, and travel with the agent.
	DefaultEnvironment string
	TargetEnvironment  string
}

// AgentIdentityResources is what a promotion carries with the agent
// application: its roles, its groups, and the MCP resource servers its roles
// reference, each with the identifier it serves under in the target.
type AgentIdentityResources struct {
	Roles              []PromotedIdentityResource
	Groups             []PromotedIdentityResource
	MCPResourceServers []MCPResourceServerPromotion
}

// AgentIdentityResourceResolver reads the agent's roles and groups from the
// default environment. Implementations return an *IdentityPromotionError of
// kind IdentityPromotionBlocked when a carried role references a resource
// server whose target identifier cannot be computed.
type AgentIdentityResourceResolver interface {
	ResolveAgentIdentityResources(ctx context.Context, query AgentIdentityResourceQuery) (AgentIdentityResources, error)
}

// AgentIdentityResourceResolverSetter backfills the resolver once the MCP
// services exist; the provisioning service is built before them.
type AgentIdentityResourceResolverSetter interface {
	SetAgentIdentityResourceResolver(AgentIdentityResourceResolver)
}

var _ AgentIdentityResourceResolver = (*MCPProxyService)(nil)

// ResolveAgentIdentityResources reads, from the default environment, the roles
// assigned to the agent, the groups it belongs to and the roles of those
// groups, then maps every resource server those roles reference to an MCP
// proxy so its identifier in the target environment can be computed. Native
// administrator roles are never carried.
func (s *MCPProxyService) ResolveAgentIdentityResources(ctx context.Context, query AgentIdentityResourceQuery) (AgentIdentityResources, error) {
	var out AgentIdentityResources
	if s.resolver == nil {
		return out, &IdentityPromotionError{
			Kind:    IdentityPromotionUnavailable,
			Message: "the identity provider is not configured for this deployment",
			Cause:   thundersvc.ErrIdentityCapabilityUnavailable,
		}
	}
	client, err := ResolveEnvThunderIdentity(ctx, s.resolver, query.OUID, query.DefaultEnvironment)
	if err != nil {
		if errors.Is(err, thundersvc.ErrIdentityEnvironmentUnsupported) {
			return out, identityHomeMismatchError(err)
		}
		return out, unavailableIdentity("the identity provider of the default environment is not reachable", err)
	}
	thunderOU, err := client.GetDefaultOUID(ctx)
	if err != nil {
		return out, unavailableIdentity("the default organization unit of the identity provider could not be read", err)
	}

	roles, err := client.GetAgentRoles(ctx, query.ThunderAgentID)
	if err != nil {
		return out, unavailableIdentity("the roles of the agent could not be read", err)
	}
	groups, err := client.GetAgentGroups(ctx, thunderOU, query.ThunderAgentID)
	if err != nil {
		return out, unavailableIdentity("the groups of the agent could not be read", err)
	}
	roleByID := map[string]thundersvc.ThunderRole{}
	addRole := func(role thundersvc.ThunderRole) {
		if isHiddenIdentityRole(role) {
			s.logger.Warn("Skipping a native administrator role that is assigned to the agent", "role", role.Name)
			return
		}
		roleByID[role.ID] = role
	}
	for _, role := range roles {
		addRole(role)
	}
	for _, group := range groups {
		groupRoles, err := client.GetGroupRoles(ctx, group.ID)
		if err != nil {
			return out, unavailableIdentity(fmt.Sprintf("the roles of group %q could not be read", group.Name), err)
		}
		for _, role := range groupRoles {
			addRole(role)
		}
		out.Groups = append(out.Groups, PromotedIdentityResource{ID: group.ID, Name: group.Name})
	}

	targetEnvUUID, err := s.EnvironmentUUIDByName(ctx, query.OUID, query.TargetEnvironment)
	if err != nil {
		return out, err
	}
	roleIDs := make([]string, 0, len(roleByID))
	for id := range roleByID {
		roleIDs = append(roleIDs, id)
	}
	sort.Strings(roleIDs)
	proxies := map[string]*resolvedRoleProxy{}
	for _, id := range roleIDs {
		// The list responses may omit permissions, so read each carried role.
		role, err := client.GetRole(ctx, id)
		if err != nil {
			return out, unavailableIdentity(fmt.Sprintf("role %q could not be read", roleByID[id].Name), err)
		}
		out.Roles = append(out.Roles, PromotedIdentityResource{ID: role.ID, Name: role.Name})
		for _, grant := range role.Permissions {
			for _, scope := range grant.Permissions {
				handle, ok := scopeProxyHandle(scope)
				if !ok {
					return out, blockedRoleResource(role.Name, "a permission that is not an MCP server scope")
				}
				resolved, err := s.resolveRoleProxy(ctx, query, targetEnvUUID, handle, proxies)
				if err != nil {
					return out, err
				}
				if blocked := resolved.blockedBy(role.Name, handle, query.TargetEnvironment); blocked != nil {
					return out, blocked
				}
				if !resolved.carried {
					resolved.carried = true
					out.MCPResourceServers = append(out.MCPResourceServers, MCPResourceServerPromotion{
						ProxyHandle: proxyHandleOf(resolved.proxy), DisplayName: resolved.display, Identifier: resolved.identifier,
					})
				}
			}
		}
	}
	return out, nil
}

// resolvedRoleProxy caches what one MCP proxy a carried role references resolved
// to, so it is looked up once however many roles use it.
type resolvedRoleProxy struct {
	proxy      *models.MCPProxy
	display    string
	identifier string
	// unmanaged: Agent Manager has no such proxy. notDeployed: the proxy is not
	// deployed to the target.
	unmanaged, notDeployed bool
	carried                bool
}

func (r *resolvedRoleProxy) blockedBy(roleName, handle, target string) *IdentityPromotionError {
	switch {
	case r.unmanaged:
		return blockedRoleResource(roleName, fmt.Sprintf("MCP server %q, which Agent Manager does not manage", handle))
	case r.notDeployed:
		return blockedRoleResource(roleName, fmt.Sprintf("MCP server %q, which is not deployed to environment %q", handle, target))
	}
	return nil
}

func (s *MCPProxyService) resolveRoleProxy(ctx context.Context, query AgentIdentityResourceQuery, targetEnvUUID uuid.UUID,
	handle string, cache map[string]*resolvedRoleProxy,
) (*resolvedRoleProxy, error) {
	if resolved, ok := cache[handle]; ok {
		return resolved, nil
	}
	resolved := &resolvedRoleProxy{}
	proxy, err := s.repo.GetByHandle(ctx, handle, query.OUID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		resolved.unmanaged = true
		cache[handle] = resolved
		return resolved, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve MCP proxy %q: %w", handle, err)
	}
	resolved.proxy = proxy
	resolved.display = proxyHandleOf(proxy)
	if proxy.Artifact != nil && proxy.Artifact.Name != "" {
		resolved.display = proxy.Artifact.Name
	}
	identifier, err := s.MCPResourceServerIdentifier(ctx, query.OUID, targetEnvUUID, proxy)
	switch {
	case errors.Is(err, ErrMCPProxyNotDeployedToEnvironment):
		resolved.notDeployed = true
	case err != nil:
		return nil, fmt.Errorf("compute MCP resource server identifier for %q: %w", handle, err)
	default:
		resolved.identifier = identifier
	}
	cache[handle] = resolved
	return resolved, nil
}

// identityHomeMismatchMessage is shown when the pipeline's first environment is
// not where identity is managed.
const identityHomeMismatchMessage = "this project's first environment is not where identity is managed"

// identityHomeMismatchError is the typed refusal for a project whose pipeline
// starts in an environment that receives its identity by promotion. Retrying
// never helps, so it is a conflict.
func identityHomeMismatchError(cause error) *IdentityPromotionError {
	return &IdentityPromotionError{Kind: IdentityPromotionConflict, Message: identityHomeMismatchMessage, Cause: cause}
}

// scopeProxyHandle returns the proxy handle of a "<proxy-handle>:<action>" scope.
func scopeProxyHandle(scope string) (string, bool) {
	idx := strings.Index(scope, ":")
	if idx <= 0 || idx == len(scope)-1 {
		return "", false
	}
	return scope[:idx], true
}

func isHiddenIdentityRole(role thundersvc.ThunderRole) bool {
	return role.Name == thundersvc.NativeAdministratorRoleName || role.Name == thundersvc.AMPSystemClientRoleName
}

func unavailableIdentity(message string, cause error) *IdentityPromotionError {
	return &IdentityPromotionError{Kind: IdentityPromotionUnavailable, Message: message, Cause: cause}
}

func blockedRoleResource(roleName, what string) *IdentityPromotionError {
	return &IdentityPromotionError{
		Kind:    IdentityPromotionBlocked,
		Message: fmt.Sprintf("Role %q references %s", roleName, what),
	}
}

// mergeMCPResourceServers adds the extra servers whose proxy handle is not
// already listed. The first occurrence wins, so the agent's own connections keep
// their entry.
func mergeMCPResourceServers(base, extra []MCPResourceServerPromotion) []MCPResourceServerPromotion {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]bool, len(base)+len(extra))
	merged := make([]MCPResourceServerPromotion, 0, len(base)+len(extra))
	for _, list := range [][]MCPResourceServerPromotion{base, extra} {
		for _, server := range list {
			if seen[server.ProxyHandle] {
				continue
			}
			seen[server.ProxyHandle] = true
			merged = append(merged, server)
		}
	}
	return merged
}
