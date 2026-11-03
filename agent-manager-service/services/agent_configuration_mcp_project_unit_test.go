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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func projectScopedProxy(projectUUID, projectName string) *models.MCPProxy {
	return &models.MCPProxy{
		Artifact:    &models.Artifact{InCatalog: false},
		ProjectUUID: &projectUUID,
		ProjectName: &projectName,
	}
}

func TestMCPProxyBindableFromProject(t *testing.T) {
	t.Run("org proxy in catalog", func(t *testing.T) {
		require.NoError(t, mcpProxyBindableFromProject(&models.MCPProxy{Artifact: &models.Artifact{InCatalog: true}}, "p-1"))
	})
	t.Run("org proxy not in catalog", func(t *testing.T) {
		err := mcpProxyBindableFromProject(&models.MCPProxy{Artifact: &models.Artifact{InCatalog: false}}, "p-1")
		require.ErrorIs(t, err, utils.ErrInvalidInput)
	})
	t.Run("project proxy of the same project, not in catalog", func(t *testing.T) {
		require.NoError(t, mcpProxyBindableFromProject(projectScopedProxy("p-1", "p1"), "p-1"))
	})
	t.Run("project proxy of another project is not found", func(t *testing.T) {
		err := mcpProxyBindableFromProject(projectScopedProxy("p-2", "p2"), "p-1")
		require.ErrorIs(t, err, utils.ErrMCPProxyNotFound)
	})
}

func TestValidateMCPProxiesInCatalog_ProjectScope(t *testing.T) {
	s := &agentConfigurationService{
		mcpProxyRepo: &repomocks.MCPProxyRepositoryMock{
			GetByHandleFunc: func(_ context.Context, handle, _ string) (*models.MCPProxy, error) {
				switch handle {
				case "weather":
					return projectScopedProxy("p-1", "p1"), nil
				case "other":
					return projectScopedProxy("p-2", "p2"), nil
				case "org":
					return &models.MCPProxy{Artifact: &models.Artifact{InCatalog: true}}, nil
				}
				return nil, gorm.ErrRecordNotFound
			},
		},
		ocClient: &clientmocks.OpenChoreoClientMock{
			GetProjectFunc: func(_ context.Context, _, name string) (*models.ProjectResponse, error) {
				return &models.ProjectResponse{Name: name, UUID: "p-1"}, nil
			},
		},
	}
	ctx := context.Background()

	require.NoError(t, s.ValidateMCPProxiesInCatalog(ctx, "org1", "p1", []string{"weather", "org"}))
	require.ErrorIs(t, s.ValidateMCPProxiesInCatalog(ctx, "org1", "p1", []string{"other"}), utils.ErrMCPProxyNotFound)
	require.ErrorIs(t, s.ValidateMCPProxiesInCatalog(ctx, "org1", "p1", []string{"missing"}), utils.ErrMCPProxyNotFound)
}

// agentMCPURL is the URL an agent bound to proxy gets injected, built the way every
// injection path builds it: from the deployed proxy buildAgentMCPConfigProxy derives.
func agentMCPURL(source *models.MCPProxy) string {
	envUUID := uuid.New()
	mapping := &models.EnvAgentMCPMapping{EnvironmentUUID: envUUID, ArtifactUUID: uuid.New(), MCPProxy: source}
	deployed := buildAgentMCPConfigProxy(&models.AgentConfiguration{}, mapping, source, "dev", "org1", "h")
	return buildMCPProxyURL(&models.Gateway{Vhost: "gw.example.com"}, deployed.Configuration)
}

func TestAgentMCPURL_ProjectProxyIncludesProjectPrefix(t *testing.T) {
	ctxPath := "/weather"
	proxy := projectScopedProxy("p-1", "p1")
	proxy.Configuration = models.MCPProxyConfig{Name: "weather", Context: &ctxPath}
	require.Equal(t, "https://gw.example.com/p1/weather/mcp", agentMCPURL(proxy))
	require.Equal(t, "/weather", *proxy.Configuration.Context, "the stored proxy is not mutated")
}

func TestAgentMCPURL_OrgProxyUnchanged(t *testing.T) {
	ctxPath := "/weather"
	proxy := &models.MCPProxy{Artifact: &models.Artifact{InCatalog: true},
		Configuration: models.MCPProxyConfig{Name: "weather", Context: &ctxPath}}
	require.Equal(t, "https://gw.example.com/weather/mcp", agentMCPURL(proxy))
}
