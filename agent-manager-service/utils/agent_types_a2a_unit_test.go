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

	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func TestIsA2AAgentSubType(t *testing.T) {
	assert.True(t, IsA2AAgentSubType("a2a-agent"))
	assert.False(t, IsA2AAgentSubType("chat-api"))
	assert.False(t, IsA2AAgentSubType("custom-api"))
	assert.False(t, IsA2AAgentSubType(""))
}

func TestValidateAgentTypeAcceptsA2A(t *testing.T) {
	subType := string(AgentSubTypeA2A)
	err := validateAgentType(spec.AgentType{Type: string(AgentTypeAPI), SubType: &subType})
	assert.NoError(t, err)
}

func externalAgentRequest(subType string) spec.CreateAgentRequest {
	return spec.CreateAgentRequest{
		Name:         "trip-planner",
		DisplayName:  "Trip Planner",
		Provisioning: spec.Provisioning{Type: string(ExternalAgent)},
		AgentType:    &spec.AgentType{Type: string(AgentTypeExternalAPI), SubType: &subType},
	}
}

// The service ignores an external subtype, so none is worth rejecting.
func TestValidateAgentCreatePayloadKeepsExternalSubTypesFreeForm(t *testing.T) {
	for _, subType := range []string{"a2a-agent", "A2A-Agent", "custom-api", "My Custom Agent!"} {
		assert.NoError(t, ValidateAgentCreatePayload(externalAgentRequest(subType)), subType)
	}
}

func TestCreateRejectsHealthChecksForExternalAgents(t *testing.T) {
	tcp := "TCP"
	request := externalAgentRequest("")
	require.NoError(t, ValidateAgentCreatePayload(request))
	request.HealthChecks = &spec.HealthChecks{Readiness: &spec.HealthCheck{Type: &tcp}}
	assert.ErrorContains(t, ValidateAgentCreatePayload(request), "only allowed for internal agents built from a repository")
}
