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

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func chatAgentRequest(language string, iface *InputInterfaceConfig) CreateComponentRequest {
	return CreateComponentRequest{
		Name:           "math-tutor",
		AgentType:      AgentTypeConfig{Type: string(utils.AgentTypeAPI), SubType: string(utils.AgentSubTypeChatAPI)},
		Build:          &BuildConfig{Type: "buildpack", Buildpack: &BuildpackConfig{Language: language}},
		InputInterface: iface,
	}
}

// A Ballerina chat agent is a ballerina/ai ai:Listener: it serves on its own port
// and service path, and its endpoint carries the ai:Listener contract.
func TestBuildEndpoints_BallerinaChatAgentUsesItsPortAndBasePath(t *testing.T) {
	endpoints, err := buildEndpoints(chatAgentRequest(string(utils.LanguageBallerina), &InputInterfaceConfig{
		Type: string(utils.InputInterfaceTypeHTTP), Port: 9191, BasePath: "/math-tutor",
	}))
	require.NoError(t, err)
	require.Len(t, endpoints, 1)
	assert.Equal(t, int32(9191), endpoints[0]["port"])
	assert.Equal(t, "/math-tutor", endpoints[0]["basePath"])
	assert.Contains(t, endpoints[0]["schemaContent"], "ChatReqMessage", "the ai:Listener contract")
}

func TestBuildEndpoints_BallerinaChatAgentDefaultsTo9090(t *testing.T) {
	endpoints, err := buildEndpoints(chatAgentRequest(string(utils.LanguageBallerina), &InputInterfaceConfig{
		Type: string(utils.InputInterfaceTypeHTTP),
	}))
	require.NoError(t, err)
	assert.Equal(t, BallerinaChatAPIDefaultPort, endpoints[0]["port"])
}

// Python chat agents keep the platform's fixed chat contract and port.
func TestBuildEndpoints_PythonChatAgentUnchanged(t *testing.T) {
	endpoints, err := buildEndpoints(chatAgentRequest(string(utils.LanguagePython), &InputInterfaceConfig{
		Type: string(utils.InputInterfaceTypeHTTP),
	}))
	require.NoError(t, err)
	assert.Equal(t, config.GetConfig().DefaultChatAPI.DefaultHTTPPort, endpoints[0]["port"])
	assert.Contains(t, endpoints[0]["schemaContent"], "session_id", "the default chat contract")
}

func TestBuildEndpointsFromInputInterface_ChatAgent(t *testing.T) {
	chat := AgentTypeConfig{Type: string(utils.AgentTypeAPI), SubType: string(utils.AgentSubTypeChatAPI)}
	defaults := config.GetConfig().DefaultChatAPI

	t.Run("Ballerina uses the request's port and base path", func(t *testing.T) {
		eps, err := buildEndpointsFromInputInterface("math-tutor",
			&InputInterfaceConfig{Type: "HTTP", Port: 9191, BasePath: "/math-tutor"}, chat, string(utils.LanguageBallerina))
		require.NoError(t, err)
		assert.Equal(t, int32(9191), eps[0]["port"])
		assert.Equal(t, "/math-tutor", eps[0]["basePath"])
	})
	t.Run("Ballerina without a port defaults to 9090", func(t *testing.T) {
		eps, err := buildEndpointsFromInputInterface("math-tutor",
			&InputInterfaceConfig{Type: "HTTP"}, chat, string(utils.LanguageBallerina))
		require.NoError(t, err)
		assert.Equal(t, BallerinaChatAPIDefaultPort, eps[0]["port"])
		assert.Equal(t, defaults.DefaultBasePath, eps[0]["basePath"])
	})
	t.Run("Python keeps the platform defaults", func(t *testing.T) {
		eps, err := buildEndpointsFromInputInterface("tutor",
			&InputInterfaceConfig{Type: "HTTP"}, chat, string(utils.LanguagePython))
		require.NoError(t, err)
		assert.Equal(t, defaults.DefaultHTTPPort, eps[0]["port"])
		assert.Equal(t, defaults.DefaultBasePath, eps[0]["basePath"])
	})
	t.Run("custom-api keeps its own port", func(t *testing.T) {
		eps, err := buildEndpointsFromInputInterface("svc",
			&InputInterfaceConfig{Type: "HTTP", Port: 8123, BasePath: "/api"},
			AgentTypeConfig{Type: string(utils.AgentTypeAPI), SubType: string(utils.AgentSubTypeCustomAPI)}, string(utils.LanguageBallerina))
		require.NoError(t, err)
		assert.Equal(t, int32(8123), eps[0]["port"])
		assert.Equal(t, "/api", eps[0]["basePath"])
	})
}
