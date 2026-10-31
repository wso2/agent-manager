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
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// mcpSyncRecorder is an identity sync trigger that records the MCP side's calls.
type mcpSyncRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *mcpSyncRecorder) add(call string) {
	r.mu.Lock()
	r.calls = append(r.calls, call)
	r.mu.Unlock()
}

func (r *mcpSyncRecorder) RoleChanged(_ context.Context, ouID, env, roleID string) {
	r.add("role:" + ouID + ":" + env + ":" + roleID)
}

func (r *mcpSyncRecorder) MCPProxyChanged(_ context.Context, ouID, env, handle string) {
	r.add("mcp:" + ouID + ":" + env + ":" + handle)
}

func (r *mcpSyncRecorder) RoleAssigneesChanged(context.Context, string, string, string, []thundersvc.AssignmentEntry) {
}
func (r *mcpSyncRecorder) GroupChanged(context.Context, string, string, string) {}
func (r *mcpSyncRecorder) GroupMembersChanged(context.Context, string, string, string, []thundersvc.GroupMember) {
}

func (r *mcpSyncRecorder) BeforeRoleDelete(context.Context, string, string, string) func() {
	return func() {}
}

func (r *mcpSyncRecorder) BeforeGroupDelete(context.Context, string, string, string) func() {
	return func() {}
}

// changeActivatingClient is a CP managed identity client with role change activation.
type changeActivatingClient struct {
	*clientmocks.EnvIdentityClientMock
	activations int
	err         error
}

func (c *changeActivatingClient) ActivateIdentityChange(context.Context) error {
	c.activations++
	return c.err
}

func TestEnsureResourceServersForProxy_TellsTheSyncTriggerWhichServerChanged(t *testing.T) {
	envA, envB := uuid.New(), uuid.New()
	gwUUID := uuid.New()
	ctxPath := "/deepwiki"
	proxy := &models.MCPProxy{
		UUID: uuid.New(), Artifact: &models.Artifact{Handle: "deepwiki"}, Configuration: models.MCPProxyConfig{Context: &ctxPath},
		Endpoints: []models.MCPProxyEndpoint{{
			Configuration: models.MCPEndpointConfig{Security: mcpIdentitySecurity()},
			Environments: []models.MCPProxyEndpointEnvironment{
				{EnvironmentUUID: envA, ArtifactUUID: uuid.New()},
				{EnvironmentUUID: envB, ArtifactUUID: uuid.New()},
			},
		}},
	}
	svc := &MCPProxyService{
		infraManager: stubInfraManager{listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{Name: "env-a", UUID: envA.String()}, {Name: "env-b", UUID: envB.String()}}, nil
		}},
		deploymentRepo: &repomocks.DeploymentRepositoryMock{
			GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) { return []string{gwUUID.String()}, nil },
		},
		gatewayRepo: &repomocks.GatewayRepositoryMock{
			EnvironmentMappingExistsFunc: func(_, _ string) (bool, error) { return true, nil },
			GetByUUIDFunc: func(string) (*models.Gateway, error) {
				return &models.Gateway{UUID: gwUUID, Vhost: "https://gw.example.com"}, nil
			},
		},
		resolver: &clientmocks.EnvThunderResolverMock{
			ResolveIdentityFunc: func(_ context.Context, _, _, envName string) (thundersvc.EnvIdentityClient, error) {
				if envName == "env-b" {
					// An environment that receives its identity by promotion has nothing to ensure.
					return nil, thundersvc.ErrIdentityEnvironmentUnsupported
				}
				return &clientmocks.EnvIdentityClientMock{
					EnsureProxyResourceServerFunc: func(context.Context, string, string, string, []string) (string, error) { return "rs-1", nil },
				}, nil
			},
		},
		logger: discardLogger(),
	}
	trigger := &mcpSyncRecorder{}
	svc.SetAgentIdentitySyncTrigger(trigger)

	svc.EnsureResourceServersForProxy(context.Background(), "ou-1", proxy, []string{"read"})

	assert.Equal(t, []string{"mcp:ou-1:env-a:deepwiki"}, trigger.calls,
		"the environment where the server was registered is told; the promoted one is not")

	// A failed ensure changed nothing, so nothing is reported.
	failing := &clientmocks.EnvIdentityClientMock{
		EnsureProxyResourceServerFunc: func(context.Context, string, string, string, []string) (string, error) { return "", errors.New("boom") },
	}
	svc.resolver = &clientmocks.EnvThunderResolverMock{
		ResolveIdentityFunc: func(context.Context, string, string, string) (thundersvc.EnvIdentityClient, error) {
			return failing, nil
		},
	}
	trigger.calls = nil
	svc.EnsureResourceServersForProxy(context.Background(), "ou-1", proxy, []string{"read"})
	assert.Empty(t, trigger.calls)

	// And a service with no trigger works as before.
	svc.SetAgentIdentitySyncTrigger(nil)
	svc.EnsureResourceServersForProxy(context.Background(), "ou-1", proxy, []string{"read"})
}

// syncingRedeployer is the scope service's proxy service with a sync trigger.
type syncingRedeployer struct {
	*recordingRedeployer
	trigger AgentIdentitySyncTrigger
}

func (r syncingRedeployer) identitySyncTrigger() AgentIdentitySyncTrigger { return r.trigger }

func TestMCPProxyScopeDelete_ActivatesTheRoleSweepAndTellsTheTriggerWhichRolesChanged(t *testing.T) {
	for _, activationFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("activation fails %t", activationFails), func(t *testing.T) {
			envA := uuid.New()
			proxy := &models.MCPProxy{
				UUID: uuid.New(), Artifact: &models.Artifact{Handle: "gh-proxy"},
				Endpoints: []models.MCPProxyEndpoint{identityEnabledEndpoint("a", envA, uuid.New(), true)},
			}
			mock := &clientmocks.EnvIdentityClientMock{
				DeleteProxyResourceServerActionFunc: func(context.Context, string, string) (string, error) { return "rs-1", nil },
				ListRolesFunc: func(_ context.Context, _ string, offset, _ int) ([]thundersvc.ThunderRole, int, error) {
					if offset > 0 {
						return nil, 2, nil
					}
					perm := []thundersvc.RolePermissionRequest{{ResourceServerID: "rs-1", Permissions: []string{"gh-proxy:read"}}}
					return []thundersvc.ThunderRole{
						{ID: "role-1", Permissions: perm},
						{ID: "role-2", Permissions: perm},
						{ID: "role-3", Permissions: []thundersvc.RolePermissionRequest{{ResourceServerID: "rs-1", Permissions: []string{"gh-proxy:write"}}}},
					}, 3, nil
				},
				RemoveRolePermissionsFunc: func(context.Context, string, thundersvc.RolePermissionRequest) error { return nil },
			}
			client := &changeActivatingClient{EnvIdentityClientMock: mock}
			if activationFails {
				client.err = errors.New("gateway unavailable")
			}
			trigger := &mcpSyncRecorder{}
			svc := NewMCPProxyScopeService(
				&repomocks.MCPProxyScopeRepositoryMock{DeleteFunc: func(context.Context, uuid.UUID, string) error { return nil }},
				&repomocks.MCPProxyRepositoryMock{GetByHandleFunc: func(context.Context, string, string) (*models.MCPProxy, error) { return proxy, nil }},
				stubInfraManager{listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
					return []*models.EnvironmentResponse{{Name: "env-a", UUID: envA.String()}}, nil
				}},
				&clientmocks.EnvThunderResolverMock{
					ResolveIdentityFunc: func(context.Context, string, string, string) (thundersvc.EnvIdentityClient, error) {
						return client, nil
					},
				},
				syncingRedeployer{recordingRedeployer: &recordingRedeployer{}, trigger: trigger},
				slog.Default(),
			)

			require.NoError(t, svc.Delete(context.Background(), "org-uuid", "org", "gh-proxy", "read"))

			assert.Equal(t, 1, client.activations, "the role writes of the sweep are made active once")
			if activationFails {
				assert.Empty(t, trigger.calls, "a change the gateway does not run is not promoted")
				return
			}
			assert.Equal(t, []string{"role:org-uuid:env-a:role-1", "role:org-uuid:env-a:role-2"}, trigger.calls,
				"only the roles the sweep changed are reported")
		})
	}
}

func TestFinishRoleSweep_DoesNothingWhenNoRoleWasStrippedAndBoundsTheFanOut(t *testing.T) {
	client := &changeActivatingClient{EnvIdentityClientMock: &clientmocks.EnvIdentityClientMock{}}
	trigger := &mcpSyncRecorder{}

	finishRoleSweep(context.Background(), discardLogger(), client, trigger, "ou", "env", nil)
	assert.Zero(t, client.activations, "nothing to activate when no role changed")
	assert.Empty(t, trigger.calls)

	many := make([]string, maxSyncRolesPerTrigger+10)
	for i := range many {
		many[i] = fmt.Sprintf("role-%d", i)
	}
	finishRoleSweep(context.Background(), discardLogger(), client, trigger, "ou", "env", many)
	assert.Len(t, trigger.calls, maxSyncRolesPerTrigger, "one change syncs at most the cap")

	// Without a trigger or an activator the sweep is as it always was.
	finishRoleSweep(context.Background(), discardLogger(), &clientmocks.EnvIdentityClientMock{}, nil, "ou", "env", many)
}
