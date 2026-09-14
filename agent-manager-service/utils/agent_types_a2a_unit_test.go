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

// An a2a-agent serves its own agent card and has no OpenAPI document, so the
// custom-api schema requirement must not reach it.
func TestValidateInputInterfaceA2ANeedsNoSchema(t *testing.T) {
	subType := string(AgentSubTypeA2A)
	port := int32(9099)
	err := validateInputInterface(
		spec.AgentType{Type: string(AgentTypeAPI), SubType: &subType},
		&spec.InputInterface{Type: "HTTP", Port: &port},
	)
	assert.NoError(t, err)
}
