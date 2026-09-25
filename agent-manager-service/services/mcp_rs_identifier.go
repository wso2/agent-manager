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
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// ensureResourceServerPerEnvTimeout bounds one environment's ensure attempt
// inside EnsureResourceServersForProxy's fan-out. Thunder's HTTP client
// already times out a single request at 30s (see thundersvc.httpClientTimeout),
// but EnsureResourceServer can make several such calls per environment, so
// without its own bound one slow environment could stall the whole fan-out
// far past that.
const ensureResourceServerPerEnvTimeout = 15 * time.Second

// ErrMCPProxyNotDeployedToEnvironment means the proxy has no deployed
// (endpoint, environment) binding to anchor a resource identifier on.
var ErrMCPProxyNotDeployedToEnvironment = errors.New("MCP proxy is not deployed to this environment")

// environmentUUIDByName resolves an environment's UUID from its name, wrapping
// a missing environment in utils.ErrEnvironmentNotFound.
func environmentUUIDByName(ctx context.Context, infra InfraResourceManager, ouID, envName string) (uuid.UUID, error) {
	envs, err := infra.ListOrgEnvironments(ctx, ouID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to list org environments: %w", err)
	}
	for _, env := range envs {
		if env.Name != envName {
			continue
		}
		envUUID, err := uuid.Parse(env.UUID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to parse environment UUID %q: %w", env.UUID, err)
		}
		return envUUID, nil
	}
	return uuid.Nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
}

// EnvironmentUUIDByName resolves the named environment's UUID for the org.
// Callers deriving identifiers for several proxies in one environment resolve
// once and pass the UUID to each MCPResourceServerIdentifier call.
func (s *MCPProxyService) EnvironmentUUIDByName(ctx context.Context, ouID, envName string) (uuid.UUID, error) {
	return environmentUUIDByName(ctx, s.infraManager, ouID, envName)
}

// MCPResourceServerIdentifier derives the absolute public URI the proxy is
// invoked at in the given environment — the value the env-Thunder resource
// server's identifier must carry.
func (s *MCPProxyService) MCPResourceServerIdentifier(ctx context.Context, ouID string, envID uuid.UUID, proxy *models.MCPProxy) (string, error) {
	_ = ctx
	_, ee := resolveMCPEndpointForEnv(proxy, envID.String())
	if ee == nil || ee.ArtifactUUID == uuid.Nil {
		return "", fmt.Errorf("%w: proxy %q, environment %s", ErrMCPProxyNotDeployedToEnvironment, proxyHandleOf(proxy), envID)
	}

	deployed, err := s.deploymentRepo.GetDeployedGatewaysByProvider(ee.ArtifactUUID, ouID)
	if err != nil {
		return "", fmt.Errorf("failed to list deployed gateways for MCP artifact %s: %w", ee.ArtifactUUID, err)
	}
	gateway, err := resolveEgressGatewayForArtifact(s.gatewayRepo, ouID, envID, deployed, nil)
	if err != nil {
		if errors.Is(err, errNoGatewayForEnvironment) || errors.Is(err, errNoEgressGatewayForEnvironment) {
			return "", fmt.Errorf("%w: proxy %q, environment %s", ErrMCPProxyNotDeployedToEnvironment, proxyHandleOf(proxy), envID)
		}
		return "", err
	}

	return buildMCPProxyURL(gateway, proxy.Configuration), nil
}

// EnsureResourceServer ensures proxy's resource server and given actions
// exist in envID's env-Thunder, so role creation and scope save register the
// identical server. Returns the resource server's Thunder ID.
func (s *MCPProxyService) EnsureResourceServer(
	ctx context.Context, ouID string, envID uuid.UUID, client thundersvc.EnvIdentityClient,
	proxy *models.MCPProxy, actions []string,
) (string, error) {
	identifier, err := s.MCPResourceServerIdentifier(ctx, ouID, envID, proxy)
	if err != nil {
		return "", err
	}
	handle := proxyHandleOf(proxy)
	name := handle
	if proxy.Artifact != nil && proxy.Artifact.Name != "" {
		name = proxy.Artifact.Name
	}
	return client.EnsureProxyResourceServer(ctx, handle, name, identifier, actions)
}

// EnsureResourceServersForProxy best-effort registers proxy's resource server
// and actions in every identity-secured, deployed environment. Called from
// both scope save and proxy save, so whichever happens first fills in what
// the other missed. Never returns an error: failures are logged and skipped.
func (s *MCPProxyService) EnsureResourceServersForProxy(ctx context.Context, ouID string, proxy *models.MCPProxy, actions []string) {
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

	handle := proxyHandleOf(proxy)
	envs, err := s.infraManager.ListOrgEnvironments(ctx, ouID)
	if err != nil {
		s.logger.Warn("ensure resource server: listing environments failed", "proxy", handle, "error", err)
		return
	}
	envName := make(map[string]string, len(envs)) // env UUID -> name (resolver keys on names)
	for _, env := range envs {
		envName[env.UUID] = env.Name
	}

	for i := range proxy.Endpoints {
		endpoint := &proxy.Endpoints[i]
		if !mcpIdentityEnabled(endpoint.Configuration.Security) {
			continue
		}
		for j := range endpoint.Environments {
			ee := &endpoint.Environments[j]
			if ee.ArtifactUUID == uuid.Nil {
				continue // not actually deployed there yet
			}
			name, ok := envName[ee.EnvironmentUUID.String()]
			if !ok {
				continue // environment no longer exists
			}
			client, err := ResolveEnvThunderIdentity(ctx, s.resolver, ouID, name)
			if err != nil {
				s.logger.Warn("ensure resource server: env-thunder unavailable", "proxy", handle, "env", name, "error", err)
				continue
			}
			envCtx, cancel := context.WithTimeout(ctx, ensureResourceServerPerEnvTimeout)
			_, err = s.EnsureResourceServer(envCtx, ouID, ee.EnvironmentUUID, client, proxy, actions)
			cancel()
			if err != nil {
				s.logger.Warn("ensure resource server: ensure failed", "proxy", handle, "env", name, "error", err)
				continue
			}
			s.logger.Info("ensure resource server: registered", "proxy", handle, "env", name, "actions", actions)
		}
	}
}

// ensureResourceServersForCurrentScopes loads proxy's current scopes and
// delegates to EnsureResourceServersForProxy.
func (s *MCPProxyService) ensureResourceServersForCurrentScopes(ctx context.Context, ouID string, proxy *models.MCPProxy) {
	scopes, err := s.mcpProxyScopeRepo.ListByProxy(ctx, proxy.UUID)
	if err != nil {
		s.logger.Warn("ensure resource server (proxy save): listing scopes failed", "proxy", proxyHandleOf(proxy), "error", err)
		return
	}
	actions := make([]string, 0, len(scopes))
	for _, sc := range scopes {
		actions = append(actions, sc.Action)
	}
	s.EnsureResourceServersForProxy(ctx, ouID, proxy, actions)
}
