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

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func TestPopulateConfigurationResponseReportsInheritedCardCORS(t *testing.T) {
	resp := &spec.ConfigurationResponse{}
	PopulateConfigurationResponseFromAgentConfig(resp, &models.AgentConfig{CORSEnabled: true, CORSAllowOrigins: []string{"*"}})
	require.NotNil(t, resp.AgentCardCorsConfig)
	assert.True(t, resp.AgentCardCorsConfig.GetInherit())
	assert.True(t, resp.AgentCardCorsConfig.GetEnabled())
	assert.Equal(t, []string{"*"}, resp.AgentCardCorsConfig.AllowOrigin)
	assert.Equal(t, []string{"Content-Type"}, resp.AgentCardCorsConfig.AllowHeaders)
}

// An external agent created when A2A was a subtype reads back like any other external agent.
func TestConvertToAgentResponse_ExternalNeverSurfacesSubType(t *testing.T) {
	got := ConvertToAgentResponse(&models.AgentResponse{
		Provisioning: models.Provisioning{Type: string(ExternalAgent)},
		Type:         models.AgentType{Type: "external-agent-api", SubType: "a2a-agent"},
	})
	assert.Nil(t, got.AgentType.SubType)
}

func TestConvertToAgentResponse_ExternalOmitsEmptySubType(t *testing.T) {
	got := ConvertToAgentResponse(&models.AgentResponse{
		Provisioning: models.Provisioning{Type: string(ExternalAgent)},
		Type:         models.AgentType{Type: "external-agent-api"},
	})
	assert.Nil(t, got.AgentType.SubType)
}
