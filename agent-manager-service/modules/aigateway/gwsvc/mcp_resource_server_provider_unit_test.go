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
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1/identitymocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
)

func deployedIdentityProxy(envA, envB uuid.UUID) (*gwmodels.MCPProxy, *MCPProxyService) {
	gwUUID := uuid.New()
	ctxPath := "/deepwiki"
	proxy := &gwmodels.MCPProxy{
		UUID:          uuid.New(),
		Artifact:      &gwmodels.Artifact{Handle: "deepwiki", Name: "Deep Wiki"},
		Configuration: gwmodels.MCPProxyConfig{Context: &ctxPath},
		Endpoints: []gwmodels.MCPProxyEndpoint{{
			Configuration: gwmodels.MCPEndpointConfig{Security: mcpIdentitySecurity()},
			Environments: []gwmodels.MCPProxyEndpointEnvironment{
				{EnvironmentUUID: envA, ArtifactUUID: uuid.New()},
				{EnvironmentUUID: envB, ArtifactUUID: uuid.New()},
			},
		}},
	}
	svc := &MCPProxyService{
		infraManager: stubInfraManager{listOrgEnvs: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{Name: "dev", UUID: envA.String()}, {Name: "prod", UUID: envB.String()}}, nil
		}},
		deploymentRepo: &gwrepomocks.DeploymentRepositoryMock{
			PlacedGatewaysFunc: func(_ context.Context, _ string, _ uuid.UUID) ([]string, error) {
				return []string{gwUUID.String()}, nil
			},
		},
		gatewayRepo: &gwrepomocks.GatewayRepositoryMock{
			EnvironmentMappingExistsFunc: func(_, _ string) (bool, error) { return true, nil },
			GetByUUIDFunc: func(_ string) (*gwmodels.Gateway, error) {
				return &gwmodels.Gateway{UUID: gwUUID, Vhost: "https://gw.example.com"}, nil
			},
		},
		// resolver deliberately nil: the legacy per-environment Thunder must not be reached.
		logger: discardLogger(),
	}
	return proxy, svc
}

func TestMCPResourceServerEnsureUsesEachEnvironmentsThunder(t *testing.T) {
	envA, envB := uuid.New(), uuid.New()
	proxy, svc := deployedIdentityProxy(envA, envB)

	var resolved []string
	svc.resolver = &identitymocks.DirectoryMock{
		ForFunc: func(_ context.Context, ouID, envName string) (identityv1.Client, error) {
			require.Equal(t, "ou-1", ouID)
			resolved = append(resolved, envName)
			return &identitymocks.ClientMock{
				EnsureResourceServerFunc: func(_ context.Context, handle, name, identifier string, actions []string) (string, error) {
					require.Equal(t, "deepwiki", handle)
					require.Equal(t, "Deep Wiki", name)
					require.Equal(t, "https://gw.example.com/deepwiki/mcp", identifier)
					require.Equal(t, []string{"read"}, actions)
					return "rs-1", nil
				},
			}, nil
		},
	}

	svc.EnsureResourceServersForProxy(context.Background(), "ou-1", proxy, []string{"read"})

	require.Equal(t, []string{"dev", "prod"}, resolved, "each environment's own Thunder")
}

func TestLegacyMCPResourceServerProvider(t *testing.T) {
	ctx := context.Background()
	request := MCPResourceServerRequest{OrganizationID: "ou-1", ProxyHandle: "deepwiki", Environment: "dev"}

	t.Run("a resolver failure is returned, not swallowed", func(t *testing.T) {
		provider := &legacyMCPResourceServerProvider{resolver: &identitymocks.DirectoryMock{
			ForFunc: func(context.Context, string, string) (identityv1.Client, error) {
				return nil, errors.New("unreachable")
			},
		}}
		require.ErrorContains(t, provider.RemoveMCPResourceServerAction(ctx, request, "read"), "env-Thunder unavailable")
	})

	t.Run("no roles are swept when the resource server was never there", func(t *testing.T) {
		// ListRoles is not stubbed: the mock panics if it is called.
		provider := &legacyMCPResourceServerProvider{logger: discardLogger(), resolver: &identitymocks.DirectoryMock{
			ForFunc: func(context.Context, string, string) (identityv1.Client, error) {
				return &identitymocks.ClientMock{
					RemoveResourceServerActionFunc: func(context.Context, string, string) (string, error) { return "", nil },
				}, nil
			},
		}}
		require.NoError(t, provider.RemoveMCPResourceServerAction(ctx, request, "read"))
	})

	t.Run("a failed server delete skips the role sweep", func(t *testing.T) {
		provider := &legacyMCPResourceServerProvider{logger: discardLogger(), resolver: &identitymocks.DirectoryMock{
			ForFunc: func(context.Context, string, string) (identityv1.Client, error) {
				return &identitymocks.ClientMock{
					DeleteResourceServerFunc: func(context.Context, string) error { return errors.New("boom") },
				}, nil
			},
		}}
		request := request
		request.Actions = []string{"read"}
		require.ErrorContains(t, provider.DeleteMCPResourceServer(ctx, request), "boom")
	})
}
