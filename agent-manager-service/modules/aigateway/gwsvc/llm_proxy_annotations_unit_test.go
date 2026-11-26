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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
)

func TestAgentProxyAnnotations(t *testing.T) {
	assert.Equal(t, map[string]string{
		"gateway.api-platform.wso2.com/project-id": "api-platform",
		"gateway.api-platform.wso2.com/agent-id":   "hotel-booking",
	}, agentProxyAnnotations("api-platform", "hotel-booking"))
	assert.Nil(t, agentProxyAnnotations("", "hotel-booking"), "no project: nothing to name")
	assert.Nil(t, agentProxyAnnotations("api-platform", ""), "no agent: nothing to name")
}

func TestRenderLLMProxy_SetsTheAnnotationsOnItsMetadata(t *testing.T) {
	proxy := &gwmodels.LLMProxy{Configuration: gwmodels.LLMProxyConfig{Name: "hotel-booking-proxy"}}
	provider := &gwmodels.LLMProvider{Artifact: &gwmodels.Artifact{Handle: "openai"}}

	read := func(content string) map[string]any {
		var doc map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(content), &doc))
		return doc["metadata"].(map[string]any)
	}

	content, err := renderLLMProxy(proxy, provider, agentProxyAnnotations("api-platform", "hotel-booking"))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"gateway.api-platform.wso2.com/project-id": "api-platform",
		"gateway.api-platform.wso2.com/agent-id":   "hotel-booking",
	}, read(content)["annotations"])

	content, err = renderLLMProxy(proxy, provider, nil)
	require.NoError(t, err)
	assert.NotContains(t, read(content), "annotations", "a proxy no single agent uses carries none")
}

func TestAgentAnnotations_UsesTheStoredAnnotations(t *testing.T) {
	stored := agentProxyAnnotations("api-platform", "hotel-booking")
	mappings := &gwrepomocks.EnvAgentModelMappingRepositoryMock{}
	got, err := agentAnnotations(context.Background(), mappings, &gwmodels.LLMProxy{Configuration: gwmodels.LLMProxyConfig{Annotations: stored}})
	require.NoError(t, err)
	assert.Equal(t, stored, got, "stored annotations are used without a lookup")
	assert.Empty(t, mappings.ListAgentConsumersByProxyUUIDsCalls())
}

func TestAgentAnnotations_AHiddenProxyWithoutThemNamesItsOneAgent(t *testing.T) {
	proxy := &gwmodels.LLMProxy{UUID: uuid.New()}
	consumers := []gwrepo.AgentConsumer{{ProjectName: "api-platform", AgentID: "hotel-booking"}}
	mappings := &gwrepomocks.EnvAgentModelMappingRepositoryMock{
		ListAgentConsumersByProxyUUIDsFunc: func(context.Context, []uuid.UUID) ([]gwrepo.AgentConsumer, error) { return consumers, nil },
	}
	got, err := agentAnnotations(context.Background(), mappings, proxy)
	require.NoError(t, err)
	assert.Equal(t, agentProxyAnnotations("api-platform", "hotel-booking"), got)

	consumers = append(consumers, gwrepo.AgentConsumer{ProjectName: "api-platform", AgentID: "billing"})
	got, err = agentAnnotations(context.Background(), mappings, proxy)
	require.NoError(t, err)
	assert.Nil(t, got, "a proxy several agents use names none")
}
