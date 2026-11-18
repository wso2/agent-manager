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

package gwsvc

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/services"
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

// Project scoping: these requests, and the role provider's, are keyed by organization and environment,
// as the routes they serve are, and carry no project. How roles, groups and MCP resource servers map
// to identity platform projects is an open decision. Every provider method takes a request struct, so
// a project can be added to it later without changing the interfaces.

// MCPResourceServerProvider registers MCP resource servers in an environment's identity provider. Every method is
// idempotent and acts on request.Environment only.
//
// EnsureMCPResourceServer makes sure the resource server and the given actions exist for the proxy
// in the environment; it never removes actions. RemoveMCPResourceServerAction removes one action;
// a missing resource server or action is success. DeleteMCPResourceServer removes the resource
// server from the environment; a missing one is success. Both also remove the scope, or every scope
// of the proxy, from the roles that hold it, so no role keeps a permission that no longer exists.
type MCPResourceServerProvider interface {
	EnsureMCPResourceServer(ctx context.Context, request MCPResourceServerRequest) (MCPResourceServerResult, error)
	RemoveMCPResourceServerAction(ctx context.Context, request MCPResourceServerRequest, action string) error
	DeleteMCPResourceServer(ctx context.Context, request MCPResourceServerRequest) error
}

// mcpResourceServerProviderFor returns the provider a call uses: the environment's own Thunder.
func mcpResourceServerProviderFor(resolver thundersvc.EnvThunderResolver, logger *slog.Logger) MCPResourceServerProvider {
	return &legacyMCPResourceServerProvider{resolver: resolver, logger: logger}
}

// legacyMCPResourceServerProvider registers resource servers in each environment's own Thunder.
// It also strips a removed scope from that Thunder's roles.
type legacyMCPResourceServerProvider struct {
	resolver thundersvc.EnvThunderResolver
	logger   *slog.Logger
}

func (p *legacyMCPResourceServerProvider) client(ctx context.Context, request MCPResourceServerRequest) (thundersvc.EnvIdentityClient, error) {
	client, err := services.ResolveEnvThunderIdentity(ctx, p.resolver, request.OrganizationID, request.Environment)
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
