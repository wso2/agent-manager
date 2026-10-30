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

package thundersvc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// cpManagedPlaceholderToken fills the bearer slot the shared client request
// builders write. cpAuthorizingTransport replaces it with the real credential.
const cpManagedPlaceholderToken = "cp-managed"

type cpClientKey struct {
	ouID, orgNamespace, environment, deploymentID, gatewayID, managementURL, agentOwnerID string
}

// cpIdentityBackend serves identity management for CP-managed environments. It
// reuses the existing client against the CP management URL, so every call is
// scoped to the target deployment and authorized by the injected authorizer.
type cpIdentityBackend struct {
	authorizer ManagementRequestAuthorizer
	config     *CPConfigurationClient

	mu      sync.Mutex
	clients map[cpClientKey]*cpManagedClient
}

// NewCPIdentityBackend returns a backend whose requests are authorized by
// authorizer. The authorizer must bind the target organization to its credential.
func NewCPIdentityBackend(authorizer ManagementRequestAuthorizer) (CPEnvironmentIdentityBackend, error) {
	config, err := NewCPConfigurationClient(authorizer)
	if err != nil {
		return nil, err
	}
	return &cpIdentityBackend{authorizer: authorizer, config: config, clients: map[cpClientKey]*cpManagedClient{}}, nil
}

func (b *cpIdentityBackend) Resolve(_ context.Context, target EnvironmentIdentityTarget) (ThunderClient, error) {
	return b.client(target)
}

func (b *cpIdentityBackend) ResolveIdentity(_ context.Context, target EnvironmentIdentityTarget) (EnvIdentityClient, error) {
	return b.client(target)
}

// client returns one shared client per target, so the per-resource-server
// serialization inside the client keeps working across requests.
func (b *cpIdentityBackend) client(target EnvironmentIdentityTarget) (*cpManagedClient, error) {
	if err := target.validateManagement(); err != nil {
		return nil, err
	}
	key := cpClientKey{
		target.OUID, target.OrgNamespace, target.Environment, target.DeploymentID,
		target.GatewayID, target.ManagementURL, target.AgentOwnerID,
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if existing, ok := b.clients[key]; ok {
		return existing, nil
	}
	created := &cpManagedClient{
		thunderClient: &thunderClient{
			baseURL: strings.TrimRight(target.ManagementURL, "/"),
			httpClient: newCPHTTPClient(&cpAuthorizingTransport{
				base: http.DefaultTransport, authorizer: b.authorizer, target: target,
			}),
			cpManaged: true,
		},
		target: target,
		config: b.config,
	}
	b.clients[key] = created
	return created, nil
}

// cpManagedClient is the shared Thunder client plus exact-gateway activation.
type cpManagedClient struct {
	*thunderClient
	target EnvironmentIdentityTarget
	config *CPConfigurationClient
	// activateMu keeps one capture-and-apply running per target; the CP also
	// coalesces, this avoids needless contention.
	activateMu sync.Mutex
}

// CreateAgentIdentity creates the agent through the CP. When the caller names no
// owner, the deployment's configured AgentOwnerID owns it, so the agent can later
// be promoted into data planes that do not know the management credential.
func (c *cpManagedClient) CreateAgentIdentity(ctx context.Context, ouID, name, owner string,
) (thunderAgentID, clientID, clientSecret string, created bool, err error) {
	if owner == "" {
		owner = c.target.AgentOwnerID
	}
	return c.thunderClient.CreateAgentIdentity(ctx, ouID, name, owner)
}

func (c *cpManagedClient) ActivateAgentIdentity(ctx context.Context, _ string) error {
	return c.activate(ctx, "AgentID change from Agent Manager")
}

func (c *cpManagedClient) ActivateMCPResourceServer(ctx context.Context, _ string) error {
	return c.activate(ctx, "MCP resource server change from Agent Manager")
}

func (c *cpManagedClient) ActivateIdentityChange(ctx context.Context) error {
	return c.activate(ctx, "Role and group change from Agent Manager")
}

func (c *cpManagedClient) ActivateDeletedMCPResourceServer(ctx context.Context, _ string) error {
	return c.activate(ctx, "MCP resource server removal from Agent Manager")
}

func (c *cpManagedClient) activate(ctx context.Context, note string) error {
	c.activateMu.Lock()
	defer c.activateMu.Unlock()
	return c.config.Activate(ctx, c.target, note)
}

// cpAuthorizingTransport scopes each request to the target deployment and
// replaces any credential with the one from the injected authorizer. It only
// sends to the target's management host.
type cpAuthorizingTransport struct {
	base       http.RoundTripper
	authorizer ManagementRequestAuthorizer
	target     EnvironmentIdentityTarget
}

func (t *cpAuthorizingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	management, err := url.Parse(t.target.ManagementURL)
	if err != nil || req.URL.Scheme != management.Scheme || req.URL.Host != management.Host {
		return nil, errors.New("CP request destination is outside the management URL")
	}
	scoped := req.Clone(req.Context())
	if err := authorizeCPRequest(req.Context(), t.authorizer, t.target, scoped); err != nil {
		return nil, fmt.Errorf("authorize CP management request: %w", err)
	}
	return t.base.RoundTrip(scoped)
}
