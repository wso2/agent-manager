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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
)

// goldenMCPProxy is an MCP proxy with one endpoint bound to one environment, exercising api-key security, backend
// auth, a user policy and a scope.
func goldenMCPProxy() (*models.MCPProxy, []models.MCPProxyScope) {
	enabled := true
	ctxPath, vhost := "/weather", "mcp.example.com"
	authType, authHeader, ref := "api-key", "X-Backend-Key", "mcp-backend-key"
	proxy := &models.MCPProxy{
		UUID:    uuid.MustParse("8b3e4c74-2f90-4e3c-9c50-2a7d8b9cadbe"),
		Handle:  "weather",
		Version: "v1.0",
		Configuration: models.MCPProxyConfig{
			Name: "Weather", Version: "v1.0", Context: &ctxPath, Vhost: &vhost, SpecVersion: "2025-06-18",
		},
		Endpoints: []models.MCPProxyEndpoint{{
			UUID:   uuid.MustParse("9c4f5d85-3a01-4f4d-8d61-3b8e9cadbecf"),
			Handle: "primary",
			Configuration: models.MCPEndpointConfig{
				Upstream: &models.UpstreamEndpoint{
					URL:  "https://weather.example.com/mcp",
					Auth: &models.UpstreamAuth{Type: &authType, Header: &authHeader, SecretRef: &ref},
				},
				Policies: []models.MCPPolicy{{Name: "mcp-rate-limit", Version: "v1.0.0", Params: map[string]any{"limit": 10}}},
				Security: &models.SecurityConfig{
					Enabled: &enabled,
					APIKey:  &models.APIKeySecurity{Enabled: &enabled, Key: "X-API-Key", In: "header"},
				},
			},
			Environments: []models.MCPProxyEndpointEnvironment{{
				EnvironmentUUID: uuid.MustParse("ad506e96-4b12-4a5e-9e72-4c9fadbecfd0"),
				ArtifactUUID:    uuid.MustParse("be617fa7-5c23-4b6f-8f83-5dafbecfd0e1"),
			}},
		}},
	}
	scopes := []models.MCPProxyScope{{Action: "read", Tools: []string{"get_forecast"}}}
	return proxy, scopes
}

func TestMCPProxyKindRendersTodaysYAML(t *testing.T) {
	proxy, scopes := goldenMCPProxy()
	endpoint := &proxy.Endpoints[0]
	kind := &MCPProxyKind{load: func(context.Context, models.ArtifactRef) (*mcpArtifact, error) {
		return &mcpArtifact{
			deployable: buildMCPProxyEnvArtifact(proxy, endpoint, &endpoint.Environments[0]), proxyHandle: proxy.Handle,
			scopes: scopes,
		}, nil
	}}

	rendered, err := kind.Render(context.Background(), models.ArtifactRef{ArtifactUUID: endpoint.Environments[0].ArtifactUUID})

	require.NoError(t, err)
	assert.Equal(t, readGolden(t, "mcp_proxy.yaml"), string(rendered.Content))
	assert.Regexp(t, `^weather-primary-ad506e964b124a5e9e724c9fadbecfd0-[0-9a-f]{8}$`, rendered.Name)
}

func TestMCPProxyKindEvents(t *testing.T) {
	kind := &MCPProxyKind{}
	ref := models.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New(), Kind: models.KindMCPMapping, Handle: "h"}
	p := models.Placement{ArtifactRef: ref, GatewayUUID: uuid.New()}
	at := models.NewPerformedAt(time.Now())

	assert.Equal(t, ref.ArtifactUUID.String(), kind.GatewayArtifactID(ref))

	deploy := kind.DeployEvent(reconcile.EventRef{Placement: p, DeploymentID: "d-1", PerformedAt: at})
	assert.Equal(t, "mcpproxy.deployed", string(deploy.EventType))
	assert.Equal(t, "CREATE", deploy.Action)
	assert.Equal(t, map[string]any{
		"proxyId": ref.ArtifactUUID.String(), "deploymentId": "d-1", "performedAt": at.Format(time.RFC3339Nano),
	}, eventPayload(t, deploy.EventData))

	undeploy := kind.UndeployEvent(reconcile.EventRef{Placement: p, PerformedAt: at})
	assert.Equal(t, "mcpproxy.undeployed", string(undeploy.EventType))
	assert.Equal(t, map[string]any{
		"proxyId": ref.ArtifactUUID.String(), "deploymentId": "", "performedAt": at.Format(time.RFC3339Nano),
	}, eventPayload(t, undeploy.EventData))

	deleted := kind.DeleteEvent(ref)
	assert.Equal(t, "mcpproxy.deleted", string(deleted.EventType))
	assert.Equal(t, "DELETE", deleted.Action)
	assert.Equal(t, map[string]any{"proxyId": ref.ArtifactUUID.String()}, eventPayload(t, deleted.EventData))
}
