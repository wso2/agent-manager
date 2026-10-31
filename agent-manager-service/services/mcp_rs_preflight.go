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

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// MCPResourceServerPreflightQuery asks that every MCP server an agent carries
// has its resource server in the home environment.
type MCPResourceServerPreflightQuery struct {
	OUID string
	// HomeEnvironment is where identity is managed: the project pipeline's lowest
	// environment. It is the only environment a resource server is ever created in.
	HomeEnvironment string
	ProxyHandles    []string
}

// MCPResourceServerPreflightNote is one MCP server that stops a promotion. Message
// is written for the user and names the server by its display name only.
type MCPResourceServerPreflightNote struct {
	ResourceName string
	Message      string
}

// MCPResourceServerPreflightResult reports what the preflight found. Blockers
// stop a promotion.
type MCPResourceServerPreflightResult struct {
	Blockers []MCPResourceServerPreflightNote
}

// AgentMCPResourceServerEnsurer is an optional capability of an
// AgentMCPResourceServerResolver. EnsureResourceServersForAgent is the safety net
// before a promotion: the MCP side owns resource servers, but the promotion
// carries them, so each must exist in the home environment first.
type AgentMCPResourceServerEnsurer interface {
	EnsureResourceServersForAgent(ctx context.Context, query MCPResourceServerPreflightQuery) (MCPResourceServerPreflightResult, error)
}

var _ AgentMCPResourceServerEnsurer = (*MCPProxyService)(nil)

type preflightMissing struct {
	proxy      *models.MCPProxy
	display    string
	identifier string
}

// EnsureResourceServersForAgent verifies, for every named MCP proxy, that it is
// deployed in the home environment and that its resource server exists there,
// and creates the missing ones through the same idempotent ensure the MCP side
// uses, with the proxy's full action list. It only ever touches the home
// environment. It checks everything first and creates only when nothing blocks,
// then activates once for all the servers it created.
func (s *MCPProxyService) EnsureResourceServersForAgent(ctx context.Context, q MCPResourceServerPreflightQuery,
) (MCPResourceServerPreflightResult, error) {
	var result MCPResourceServerPreflightResult
	handles := uniqueSorted(q.ProxyHandles)
	if len(handles) == 0 {
		return result, nil
	}
	if s.resolver == nil {
		return result, unavailableIdentity("the identity provider is not configured for this deployment", thundersvc.ErrIdentityCapabilityUnavailable)
	}
	homeEnv, err := s.EnvironmentUUIDByName(ctx, q.OUID, q.HomeEnvironment)
	if err != nil {
		return result, err
	}
	client, err := ResolveEnvThunderIdentity(ctx, s.resolver, q.OUID, q.HomeEnvironment)
	if err != nil {
		if errors.Is(err, thundersvc.ErrIdentityEnvironmentUnsupported) {
			return result, identityHomeMismatchError(err)
		}
		return result, unavailableIdentity("the identity provider of the first environment is not reachable", err)
	}
	reader, _ := client.(thundersvc.ProxyResourceServerReader)

	var missing []preflightMissing
	for _, handle := range handles {
		proxy, err := s.repo.GetByHandle(ctx, handle, q.OUID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result.Blockers = append(result.Blockers, MCPResourceServerPreflightNote{
				ResourceName: handle,
				Message:      fmt.Sprintf("%s is not an MCP server that Agent Manager manages.", handle),
			})
			continue
		}
		if err != nil {
			return result, fmt.Errorf("resolve MCP proxy %q: %w", handle, err)
		}
		display := proxyHandleOf(proxy)
		if proxy.Artifact != nil && proxy.Artifact.Name != "" {
			display = proxy.Artifact.Name
		}
		identifier, err := s.MCPResourceServerIdentifier(ctx, q.OUID, homeEnv, proxy)
		if errors.Is(err, ErrMCPProxyNotDeployedToEnvironment) {
			result.Blockers = append(result.Blockers, MCPResourceServerPreflightNote{
				ResourceName: display,
				Message:      fmt.Sprintf("%s is not deployed to %s, where identity is managed. Deploy it there, then try again.", display, q.HomeEnvironment),
			})
			continue
		}
		if err != nil {
			return result, fmt.Errorf("compute MCP resource server identifier for %q: %w", handle, err)
		}
		present := false
		if reader != nil {
			if present, err = reader.ProxyResourceServerExists(ctx, handle, identifier); err != nil {
				return result, unavailableIdentity(fmt.Sprintf("the MCP server access of %s could not be checked", display), err)
			}
		}
		if !present {
			missing = append(missing, preflightMissing{proxy: proxy, display: display, identifier: identifier})
		}
	}
	if len(result.Blockers) > 0 {
		return result, nil
	}

	for _, m := range missing {
		actions, err := s.proxyScopeActions(ctx, m.proxy)
		if err != nil {
			return result, err
		}
		if _, err := s.EnsureResourceServerWithoutActivation(ctx, q.OUID, homeEnv, client, m.proxy, actions); err != nil {
			return result, unavailableIdentity(fmt.Sprintf("the MCP server access of %s could not be registered in %s", m.display, q.HomeEnvironment), err)
		}
	}
	if len(missing) > 0 {
		if err := activateMCPResourceServer(ctx, client, ""); err != nil {
			return result, unavailableIdentity("the MCP server access was registered but could not be activated; try again", err)
		}
	}
	return result, nil
}

// proxyScopeActions lists the actions the proxy defines, so the ensure registers
// the full set and not just the ones one role uses.
func (s *MCPProxyService) proxyScopeActions(ctx context.Context, proxy *models.MCPProxy) ([]string, error) {
	scopes, err := s.mcpProxyScopeRepo.ListByProxy(ctx, proxy.UUID)
	if err != nil {
		return nil, fmt.Errorf("list scopes of MCP proxy %q: %w", proxyHandleOf(proxy), err)
	}
	actions := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		actions = append(actions, scope.Action)
	}
	return actions, nil
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
