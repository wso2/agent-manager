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
	"strings"
	"sync"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// MCPResourceServerRequest names one MCP proxy's resource server in one environment.
type MCPResourceServerRequest struct {
	// OrganizationID is the organization unit ID (OUID) the resource server belongs to. It is
	// always set; a provider must not fall back to a default organization unit.
	OrganizationID string
	// ProxyHandle is the proxy's stable handle. Scope permissions are "<ProxyHandle>:<action>".
	ProxyHandle string
	// DisplayName is the proxy's display name.
	DisplayName string
	// Environment is the one environment to act on. Agent Manager decides which environments a
	// proxy is deployed to; the provider only acts on the one it is given.
	Environment string
	// Identifier is the absolute public URL the proxy is invoked at in Environment (RFC 8707).
	// Empty on remove and delete.
	Identifier string
	// Actions are the scope actions to make sure exist (Ensure). On Delete they are the actions
	// the proxy had, so a deployment can clean up what refers to them.
	Actions []string
}

// MCPResourceServerResult is what an ensure produced.
type MCPResourceServerResult struct {
	// ResourceServerID is the identity service's ID of the resource server.
	ResourceServerID string
}

// MCPResourceServerProvider is implemented by deployments that register MCP resource servers
// through an identity control plane instead of in each environment's own Thunder. It is used only
// in CP_MANAGED identity mode. Every method is idempotent and acts on request.Environment only.
//
// EnsureMCPResourceServer makes sure the resource server and the given actions exist for the proxy
// in the environment; it never removes actions. RemoveMCPResourceServerAction removes one action;
// a missing resource server or action is success. DeleteMCPResourceServer removes the resource
// server from the environment; a missing one is success. A call that cannot finish yet because the
// environment is not ready returns an error wrapping ErrMCPResourceServerEnvironmentNotReady.
type MCPResourceServerProvider interface {
	EnsureMCPResourceServer(ctx context.Context, request MCPResourceServerRequest) (MCPResourceServerResult, error)
	RemoveMCPResourceServerAction(ctx context.Context, request MCPResourceServerRequest, action string) error
	DeleteMCPResourceServer(ctx context.Context, request MCPResourceServerRequest) error
}

var (
	// ErrMCPResourceServerUnavailable means MCP resource servers cannot be managed: CP_MANAGED
	// identity mode is on and no provider is configured. The per-environment Thunder is never
	// used instead.
	ErrMCPResourceServerUnavailable = errors.New("MCP resource server provider is not configured")
	// ErrMCPResourceServerEnvironmentNotReady marks a retryable condition: the environment cannot
	// take the resource server yet.
	ErrMCPResourceServerEnvironmentNotReady = errors.New("environment is not ready for the MCP resource server")
)

// mcpResourceServerSetting is the process-wide choice made once at startup, as with
// SetGatewayManifestCacheBackend: the services that need it are built by wire, and reading it when
// a call is made keeps the construction order out of the picture.
var mcpResourceServerSetting struct {
	mu       sync.RWMutex
	mode     thundersvc.IdentityManagementMode
	provider MCPResourceServerProvider
}

// SetMCPResourceServerProvider chooses where MCP resource servers are registered. In
// LEGACY_DIRECT mode (the default) they go to each environment's own Thunder and provider is
// ignored. In CP_MANAGED mode they go to provider; with a nil provider every operation reports
// ErrMCPResourceServerUnavailable and the legacy client is never used.
func SetMCPResourceServerProvider(mode thundersvc.IdentityManagementMode, provider MCPResourceServerProvider) {
	mcpResourceServerSetting.mu.Lock()
	defer mcpResourceServerSetting.mu.Unlock()
	mcpResourceServerSetting.mode = mode
	mcpResourceServerSetting.provider = provider
	switch {
	case mode != thundersvc.IdentityManagementCPManaged && provider != nil:
		slog.Warn("MCP resource server provider ignored: identity management mode is not CP_MANAGED", "mode", string(mode))
	case mode == thundersvc.IdentityManagementCPManaged && provider == nil:
		slog.Warn("CP_MANAGED identity mode without an MCP resource server provider: MCP resource servers cannot be managed until one is configured")
	}
}

// mcpResourceServerProviderFor returns what a call should use: the legacy per-environment
// implementation, the configured provider, or the fail-closed one.
func mcpResourceServerProviderFor(resolver thundersvc.EnvThunderResolver, logger *slog.Logger) MCPResourceServerProvider {
	mcpResourceServerSetting.mu.RLock()
	mode, provider := mcpResourceServerSetting.mode, mcpResourceServerSetting.provider
	mcpResourceServerSetting.mu.RUnlock()

	if mode != thundersvc.IdentityManagementCPManaged {
		return &legacyMCPResourceServerProvider{resolver: resolver, logger: logger}
	}
	if provider == nil {
		return unavailableMCPResourceServerProvider{}
	}
	return checkedMCPResourceServerProvider{next: provider}
}

// legacyMCPResourceServerProvider registers resource servers in each environment's own Thunder,
// exactly as Agent Manager always has. It also strips a removed scope from that Thunder's roles.
type legacyMCPResourceServerProvider struct {
	resolver thundersvc.EnvThunderResolver
	logger   *slog.Logger
}

func (p *legacyMCPResourceServerProvider) client(ctx context.Context, request MCPResourceServerRequest) (thundersvc.EnvIdentityClient, error) {
	client, err := ResolveEnvThunderIdentity(ctx, p.resolver, request.OrganizationID, request.Environment)
	if err != nil {
		return nil, fmt.Errorf("env-Thunder unavailable: %w", err)
	}
	return client, nil
}

func (p *legacyMCPResourceServerProvider) EnsureMCPResourceServer(ctx context.Context, request MCPResourceServerRequest) (MCPResourceServerResult, error) {
	client, err := p.client(ctx, request)
	if err != nil {
		return MCPResourceServerResult{}, err
	}
	id, err := client.EnsureProxyResourceServer(ctx, request.ProxyHandle, request.DisplayName, request.Identifier, request.Actions)
	if err != nil {
		return MCPResourceServerResult{}, err
	}
	return MCPResourceServerResult{ResourceServerID: id}, nil
}

func (p *legacyMCPResourceServerProvider) RemoveMCPResourceServerAction(ctx context.Context, request MCPResourceServerRequest, action string) error {
	client, err := p.client(ctx, request)
	if err != nil {
		return err
	}
	rsID, err := client.DeleteProxyResourceServerAction(ctx, request.ProxyHandle, action)
	if err != nil {
		return err
	}
	if rsID == "" {
		return nil // the resource server was never projected into this environment, so no role holds it
	}
	sweepRolePermission(ctx, p.log(), client, request.Environment, request.ProxyHandle+":"+action)
	return nil
}

func (p *legacyMCPResourceServerProvider) DeleteMCPResourceServer(ctx context.Context, request MCPResourceServerRequest) error {
	client, err := p.client(ctx, request)
	if err != nil {
		return err
	}
	if err := client.DeleteProxyResourceServer(ctx, request.ProxyHandle); err != nil {
		return err
	}
	for _, action := range request.Actions {
		sweepRolePermission(ctx, p.log(), client, request.Environment, request.ProxyHandle+":"+action)
	}
	return nil
}

func (p *legacyMCPResourceServerProvider) log() *slog.Logger {
	if p.logger != nil {
		return p.logger
	}
	return slog.Default()
}

// unavailableMCPResourceServerProvider is used in CP_MANAGED mode without a provider. It fails
// closed: the per-environment Thunder is not touched.
type unavailableMCPResourceServerProvider struct{}

func (unavailableMCPResourceServerProvider) EnsureMCPResourceServer(context.Context, MCPResourceServerRequest) (MCPResourceServerResult, error) {
	return MCPResourceServerResult{}, ErrMCPResourceServerUnavailable
}

func (unavailableMCPResourceServerProvider) RemoveMCPResourceServerAction(context.Context, MCPResourceServerRequest, string) error {
	return ErrMCPResourceServerUnavailable
}

func (unavailableMCPResourceServerProvider) DeleteMCPResourceServer(context.Context, MCPResourceServerRequest) error {
	return ErrMCPResourceServerUnavailable
}

// checkedMCPResourceServerProvider refuses a request that does not name the organization, proxy and
// environment before it reaches a deployment's provider, so a provider can rely on them.
type checkedMCPResourceServerProvider struct {
	next MCPResourceServerProvider
}

func checkMCPResourceServerRequest(request MCPResourceServerRequest, needIdentifier bool) error {
	switch {
	case strings.TrimSpace(request.OrganizationID) == "":
		return fmt.Errorf("%w: the organization unit ID is required for an MCP resource server", utils.ErrInvalidInput)
	case strings.TrimSpace(request.ProxyHandle) == "":
		return fmt.Errorf("%w: the proxy handle is required for an MCP resource server", utils.ErrInvalidInput)
	case strings.TrimSpace(request.Environment) == "":
		return fmt.Errorf("%w: the environment is required for an MCP resource server", utils.ErrInvalidInput)
	case needIdentifier && strings.TrimSpace(request.Identifier) == "":
		return fmt.Errorf("%w: the identifier is required to ensure an MCP resource server", utils.ErrInvalidInput)
	}
	return nil
}

func (p checkedMCPResourceServerProvider) EnsureMCPResourceServer(ctx context.Context, request MCPResourceServerRequest) (MCPResourceServerResult, error) {
	if err := checkMCPResourceServerRequest(request, true); err != nil {
		return MCPResourceServerResult{}, err
	}
	return p.next.EnsureMCPResourceServer(ctx, request)
}

func (p checkedMCPResourceServerProvider) RemoveMCPResourceServerAction(ctx context.Context, request MCPResourceServerRequest, action string) error {
	if err := checkMCPResourceServerRequest(request, false); err != nil {
		return err
	}
	if strings.TrimSpace(action) == "" {
		return fmt.Errorf("%w: the action is required to remove an MCP resource server action", utils.ErrInvalidInput)
	}
	return p.next.RemoveMCPResourceServerAction(ctx, request, action)
}

func (p checkedMCPResourceServerProvider) DeleteMCPResourceServer(ctx context.Context, request MCPResourceServerRequest) error {
	if err := checkMCPResourceServerRequest(request, false); err != nil {
		return err
	}
	return p.next.DeleteMCPResourceServer(ctx, request)
}
