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

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// A project binding secured with identity is an OAuth resource server in its environment's identity service, as an
// org proxy's identity endpoint is: its identifier is the binding's public URL, so a token requested for that URL
// (RFC 8707 resource) is accepted by the binding's mcp-auth policy. AMS registers and removes the resource server;
// the clients that call it (AM agents through their AgentID, or apps the caller manages in the identity service
// itself) and the roles that let them are managed outside the binding routes.

// ensureBindingResourceServer registers the resource server of the proxy's binding in envName when that binding uses
// identity. It is best effort and returns nothing: a failure is logged, and the next PUT of the binding, which the
// caller's ensure loop sends, registers it again. A binding with no gateway yet has no public URL to register, so it
// is skipped until a PUT after placement or a role write that grants its scopes, which ensures it as for an org proxy.
func (s *ProjectMCPProxyService) ensureBindingResourceServer(
	ctx context.Context, ouID string, proxy *models.MCPProxy, envUUID uuid.UUID, envName string,
) {
	endpoint, ee := resolveMCPEndpointForEnv(proxy, envUUID.String())
	if endpoint == nil || ee == nil || !mcpIdentityEnabled(endpoint.Configuration.Security) || s.base.resolver == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ensureResourceServerPerEnvTimeout)
	defer cancel()
	handle := proxyHandleOf(proxy)
	// The proxy's scopes are the resource server's actions, which roles grant and the gateway enforces per tool. With
	// none, any token issued for the resource is enough. Ensure never removes actions, so a failed lookup loses none.
	var actions []string
	if s.base.mcpProxyScopeRepo != nil {
		scopes, err := s.base.mcpProxyScopeRepo.ListByProxy(ctx, proxy.UUID)
		if err != nil {
			s.base.logger.Warn("project MCP binding resource server: listing scopes failed", "proxy", handle, "error", err)
		}
		for _, scope := range scopes {
			actions = append(actions, scope.Action)
		}
	}
	_, err := s.base.ensureProxyResourceServerInEnv(ctx, ouID, envUUID, envName, proxy, actions)
	switch {
	case err == nil:
		s.base.logger.Info("project MCP binding resource server registered", "proxy", handle, "env", envName)
	case errors.Is(err, ErrMCPProxyNotDeployedToEnvironment):
		s.base.logger.Info("project MCP binding resource server deferred: the binding has no gateway yet",
			"proxy", handle, "env", envName)
	case errors.Is(err, thundersvc.ErrThunderNotProvisioned):
		s.base.logger.Info("project MCP binding resource server skipped: the environment has no identity provider yet",
			"proxy", handle, "env", envName)
	default:
		s.base.logger.Warn("project MCP binding resource server registration failed", "proxy", handle, "env", envName,
			"error", err)
	}
}

// ensureBindingResourceServers registers the resource server of every identity binding of the proxy: after a change
// to the proxy's context, which every binding's public URL, and so its resource identifier, carries.
func (s *ProjectMCPProxyService) ensureBindingResourceServers(ctx context.Context, ouID string, proxy *models.MCPProxy) {
	for i := range proxy.Endpoints {
		endpoint := &proxy.Endpoints[i]
		if len(endpoint.Environments) != 1 || !mcpIdentityEnabled(endpoint.Configuration.Security) {
			continue
		}
		s.ensureBindingResourceServer(ctx, ouID, proxy, endpoint.Environments[0].EnvironmentUUID, endpoint.Name)
	}
}

// cleanUpBindingIdentity removes the resource server of a binding that left identity, and the roles' grants of the
// proxy's scopes in envName, best effort. With a cleanupID, a successful removal clears the binding's IdentityCleanupID
// if it is still that one, so later identical PUTs leave grants made since alone while a newer pending removal stays.
// When the proxy's scopes cannot be read it removes nothing, since deleting the resource server without them would
// leave their grants with nothing to sweep them; the ID stays and an identical PUT retries.
func (s *ProjectMCPProxyService) cleanUpBindingIdentity(
	ctx context.Context, ouID string, proxy *models.MCPProxy, envName string, endpointUUID uuid.UUID, cleanupID string,
) bool {
	if s.base.resolver == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ensureResourceServerPerEnvTimeout)
	defer cancel()
	actions, err := s.proxyScopeActions(ctx, proxy)
	if err != nil {
		s.base.logger.Warn("project MCP binding identity cleanup deferred: listing scopes failed",
			"proxy", proxyHandleOf(proxy), "env", envName, "error", err)
		return false
	}
	if !s.removeBindingResourceServer(ctx, ouID, proxy, envName, actions) {
		return false
	}
	if cleanupID != "" {
		if err := s.base.endpointRepo.ClearIdentityCleanupID(ctx, endpointUUID, cleanupID); err != nil {
			s.base.logger.Warn("project MCP binding identity cleanup: clearing the cleanup ID failed",
				"proxy", proxyHandleOf(proxy), "env", envName, "error", err)
		}
	}
	return true
}

// syncBindingIdentity brings a saved binding's identity service state in line with it, best effort: a pending removal
// of its old resource server runs first, and a binding on identity is registered only once nothing is pending, so a
// removal that fails is retried by the next PUT before the binding registers again.
func (s *ProjectMCPProxyService) syncBindingIdentity(
	ctx context.Context, ouID string, proxy *models.MCPProxy, envUUID uuid.UUID, envName string, endpoint *models.MCPProxyEndpoint,
) {
	if cleanupID := endpoint.Configuration.IdentityCleanupID; cleanupID != "" {
		if !s.cleanUpBindingIdentity(ctx, ouID, proxy, envName, endpoint.UUID, cleanupID) {
			return
		}
	}
	s.ensureBindingResourceServer(ctx, ouID, proxy, envUUID, envName)
}

// purgeBindingResourceServer removes the resource server of a deleted binding that was on identity or had a removal
// pending, best effort: with the binding gone no identical PUT can retry it, and a binding created in the environment
// later removes a leftover before it registers. actions are the proxy's scope actions,
// read before the binding was deleted so a failed read stops the delete instead. A resource server already gone, or an
// environment without an identity service, is not an error.
func (s *ProjectMCPProxyService) purgeBindingResourceServer(
	ctx context.Context, ouID string, proxy *models.MCPProxy, envName string, actions []string,
) {
	if s.base.resolver == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ensureResourceServerPerEnvTimeout)
	defer cancel()
	s.removeBindingResourceServer(ctx, ouID, proxy, envName, actions)
}

// proxyScopeActions are the proxy's scope actions, as the proxy delete passes them, so a deployment can clean up what
// refers to them. Without a scope store there are none.
func (s *ProjectMCPProxyService) proxyScopeActions(ctx context.Context, proxy *models.MCPProxy) ([]string, error) {
	if s.base.mcpProxyScopeRepo == nil {
		return nil, nil
	}
	scopes, err := s.base.mcpProxyScopeRepo.ListByProxy(ctx, proxy.UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the MCP proxy's scopes: %w", err)
	}
	actions := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		actions = append(actions, scope.Action)
	}
	return actions, nil
}

// removeBindingResourceServer deletes the proxy's resource server in envName and drops actions from its roles, and
// reports whether that succeeded. An environment without an identity service has nothing to remove.
func (s *ProjectMCPProxyService) removeBindingResourceServer(
	ctx context.Context, ouID string, proxy *models.MCPProxy, envName string, actions []string,
) bool {
	handle := proxyHandleOf(proxy)
	request := MCPResourceServerRequest{
		OrganizationID: ouID, ProxyHandle: handle, DisplayName: mcpResourceServerDisplayName(proxy), Environment: envName,
		Actions: actions,
	}
	err := mcpResourceServerProviderFor(s.base.resolver, s.base.logger).DeleteMCPResourceServer(ctx, request)
	switch {
	case err == nil:
		return true
	case errors.Is(err, thundersvc.ErrThunderNotProvisioned):
		// No identity provider in the environment, so no resource server to remove.
		s.base.logger.Info("project MCP binding resource server delete skipped: the environment has no identity provider",
			"proxy", handle, "env", envName)
		return true
	default:
		s.base.logger.Warn("project MCP binding resource server delete failed", "proxy", handle, "env", envName,
			"error", err)
		return false
	}
}
