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
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const testGatewayEnvUUID = "0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f"

// Joining project and agent with a hyphen made "a-b"+"c" and "a"+"b-c" the same resource.
func TestAgentGatewayNameUnambiguous(t *testing.T) {
	assert.NotEqual(t,
		agentGatewayName("a-b", "c", testGatewayEnvUUID),
		agentGatewayName("a", "b-c", testGatewayEnvUUID))
}

func TestAgentGatewayNameStable(t *testing.T) {
	name := agentGatewayName("checkout", "trip-planner", testGatewayEnvUUID)
	assert.Equal(t, name, agentGatewayName("checkout", "trip-planner", testGatewayEnvUUID))
	assert.NotEqual(t, name, agentGatewayName("checkout", "trip-planner", "0192f4c1-9a7d-7c3e-b4f2-000000000000"))
	assert.NotEqual(t, name, agentGatewayName("billing", "trip-planner", testGatewayEnvUUID))
}

func TestAgentGatewayNameGatewayRules(t *testing.T) {
	shape := regexp.MustCompile(`^[a-z0-9-]+-[0-9a-f]{16}$`)
	for _, agentName := range []string{
		"trip-planner",
		strings.Repeat("a", utils.MaxResourceNameLength),
		"abcdefghi-jklmn", // the 10th character is the one the prefix cut lands on
	} {
		t.Run(agentName, func(t *testing.T) {
			name := agentGatewayName("checkout", agentName, testGatewayEnvUUID)
			assert.Regexp(t, shape, name)
			assert.NotContains(t, name, "--")
		})
	}
}
