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
// is skipped until a PUT after placement.
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

// deleteBindingResourceServer removes the resource server of a removed identity binding from envName's identity
// service, best effort. The proxy delete removes every environment's (cleanupProxyResourceServers); this is the
// same for one binding.
func (s *ProjectMCPProxyService) deleteBindingResourceServer(
	ctx context.Context, ouID string, proxy *models.MCPProxy, envName string, security *models.SecurityConfig,
) {
	if !mcpIdentityEnabled(security) || s.base.resolver == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ensureResourceServerPerEnvTimeout)
	defer cancel()
	handle := proxyHandleOf(proxy)
	request := MCPResourceServerRequest{
		OrganizationID: ouID, ProxyHandle: handle, DisplayName: mcpResourceServerDisplayName(proxy), Environment: envName,
	}
	if err := mcpResourceServerProviderFor(s.base.resolver, s.base.logger).DeleteMCPResourceServer(ctx, request); err != nil {
		s.base.logger.Warn("project MCP binding resource server delete failed", "proxy", handle, "env", envName,
			"error", err)
	}
}
