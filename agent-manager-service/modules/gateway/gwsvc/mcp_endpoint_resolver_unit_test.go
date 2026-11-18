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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// proxyWithEndpoints builds a source proxy whose endpoints are each bound to one
// environment, so resolveMCPEndpointForEnv has a deterministic graph to walk.
func proxyWithEndpoints(endpoints ...models.MCPProxyEndpoint) *models.MCPProxy {
	return &models.MCPProxy{UUID: uuid.New(), Endpoints: endpoints}
}

func TestResolveMCPEndpointForEnv_ReturnsBoundEndpoint(t *testing.T) {
	envA := uuid.New()
	envB := uuid.New()
	artifactA := uuid.New()
	upstreamA := "https://a.example.com/mcp"

	proxy := proxyWithEndpoints(
		models.MCPProxyEndpoint{
			UUID:   uuid.New(),
			Handle: "primary",
			Configuration: models.MCPEndpointConfig{
				Upstream: &models.UpstreamEndpoint{URL: upstreamA},
			},
			Environments: []models.MCPProxyEndpointEnvironment{
				{EnvironmentUUID: envA, ArtifactUUID: artifactA},
			},
		},
		models.MCPProxyEndpoint{
			UUID:   uuid.New(),
			Handle: "secondary",
			Environments: []models.MCPProxyEndpointEnvironment{
				{EnvironmentUUID: envB, ArtifactUUID: uuid.New()},
			},
		},
	)

	endpoint, ee := resolveMCPEndpointForEnv(proxy, envA.String())
	require.NotNil(t, endpoint)
	require.NotNil(t, ee)
	assert.Equal(t, "primary", endpoint.Handle)
	assert.Equal(t, upstreamA, endpoint.Configuration.Upstream.URL)
	assert.Equal(t, artifactA, ee.ArtifactUUID)
	assert.Equal(t, envA, ee.EnvironmentUUID)
}

func TestResolveMCPEndpointForEnv_NoMatchReturnsNil(t *testing.T) {
	proxy := proxyWithEndpoints(
		models.MCPProxyEndpoint{
			Handle: "primary",
			Environments: []models.MCPProxyEndpointEnvironment{
				{EnvironmentUUID: uuid.New(), ArtifactUUID: uuid.New()},
			},
		},
	)
	endpoint, ee := resolveMCPEndpointForEnv(proxy, uuid.New().String())
	assert.Nil(t, endpoint)
	assert.Nil(t, ee)
}

func TestResolveMCPEndpointForEnv_NilProxyOrEmptyEnv(t *testing.T) {
	endpoint, ee := resolveMCPEndpointForEnv(nil, uuid.New().String())
	assert.Nil(t, endpoint)
	assert.Nil(t, ee)

	proxy := proxyWithEndpoints(models.MCPProxyEndpoint{
		Environments: []models.MCPProxyEndpointEnvironment{{EnvironmentUUID: uuid.New()}},
	})
	endpoint, ee = resolveMCPEndpointForEnv(proxy, "  ")
	assert.Nil(t, endpoint)
	assert.Nil(t, ee)
}

func TestMCPProxyEnvArtifactUUID_FromEndpointBinding(t *testing.T) {
	env := uuid.New()
	artifact := uuid.New()
	proxy := proxyWithEndpoints(models.MCPProxyEndpoint{
		Environments: []models.MCPProxyEndpointEnvironment{
			{EnvironmentUUID: env, ArtifactUUID: artifact},
		},
	})
	assert.Equal(t, artifact, mcpProxyEnvArtifactUUID(proxy, env.String()))
	assert.Equal(t, uuid.Nil, mcpProxyEnvArtifactUUID(proxy, uuid.New().String()))
}

func TestMCPProxySecurityForEnv_FromEndpointBinding(t *testing.T) {
	env := uuid.New()
	proxy := proxyWithEndpoints(models.MCPProxyEndpoint{
		Configuration: models.MCPEndpointConfig{Security: identityEnabledSecurity()},
		Environments: []models.MCPProxyEndpointEnvironment{
			{EnvironmentUUID: env, ArtifactUUID: uuid.New()},
		},
	})
	sec := mcpProxySecurityForEnv(proxy, env.String())
	require.NotNil(t, sec)
	assert.True(t, isBoolTrue(sec.Enabled))
	assert.Nil(t, mcpProxySecurityForEnv(proxy, uuid.New().String()))
}

// buildAgentMCPConfigProxy flattens the endpoint bound to the mapping's environment into
// the flat deployable proxy; with no matching endpoint the upstream stays empty.
func TestBuildAgentMCPConfigProxy_FlattensBoundEndpoint(t *testing.T) {
	env := uuid.New()
	mappingArtifact := uuid.New()
	upstream := "https://tools.example.com/mcp"
	ctxPath := "/shared"

	source := &models.MCPProxy{
		UUID:   uuid.New(),
		Status: models.StatusCreated,
		Configuration: models.MCPProxyConfig{
			Version: "v1.0",
			Context: &ctxPath,
		},
		Endpoints: []models.MCPProxyEndpoint{
			{
				Handle: "primary",
				Configuration: models.MCPEndpointConfig{
					Upstream: &models.UpstreamEndpoint{URL: upstream},
				},
				Environments: []models.MCPProxyEndpointEnvironment{
					{EnvironmentUUID: env, ArtifactUUID: uuid.New()},
				},
			},
		},
	}
	mapping := &models.EnvAgentMCPMapping{
		EnvironmentUUID: env,
		MCPProxyUUID:    source.UUID,
		ArtifactUUID:    mappingArtifact,
	}

	out := buildAgentMCPConfigProxy(&models.AgentConfiguration{}, mapping, source, "dev", "org", "handle")
	require.NotNil(t, out.Configuration.Upstream.Main)
	assert.Equal(t, upstream, out.Configuration.Upstream.Main.URL)
	assert.Equal(t, mappingArtifact, out.UUID)
	assert.Equal(t, "v1.0", out.Version)
}

func TestBuildAgentMCPConfigProxy_NoEndpointLeavesUpstreamEmpty(t *testing.T) {
	source := &models.MCPProxy{UUID: uuid.New(), Configuration: models.MCPProxyConfig{Version: "v1.0"}}
	mapping := &models.EnvAgentMCPMapping{
		EnvironmentUUID: uuid.New(),
		MCPProxyUUID:    source.UUID,
		ArtifactUUID:    uuid.New(),
	}
	out := buildAgentMCPConfigProxy(&models.AgentConfiguration{}, mapping, source, "dev", "org", "handle")
	assert.Nil(t, out.Configuration.Upstream.Main)
}
