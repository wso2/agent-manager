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

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// An a2a-agent must still declare exactly one workload endpoint: the agent-api
// ComponentType validates "size(workload.endpoints) > 0" and renders the
// component's Service and HTTPRoutes from it. It carries no OpenAPI schema.
func TestBuildEndpointsForA2AAgent(t *testing.T) {
	port := int32(9099)
	req := CreateComponentRequest{
		Name: "trip-planner",
		AgentType: AgentTypeConfig{
			Type:    string(utils.AgentTypeAPI),
			SubType: string(utils.AgentSubTypeA2A),
		},
		InputInterface: &InputInterfaceConfig{
			Type:     string(utils.InputInterfaceTypeHTTP),
			Port:     port,
			BasePath: "/",
		},
	}

	endpoints, err := buildEndpoints(req)
	require.NoError(t, err)
	require.Len(t, endpoints, 1)

	ep := endpoints[0]
	assert.Equal(t, "trip-planner-endpoint", ep["name"])
	assert.Equal(t, port, ep["port"])
	assert.Equal(t, "/", ep["basePath"])
	assert.NotContains(t, ep, "schemaType")
	assert.NotContains(t, ep, "schemaFilePath")
	assert.NotContains(t, ep, "schemaContent")
}
