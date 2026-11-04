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

package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// MCP keeps its own words for the shared status.
func TestMCPDeploymentStatusOf(t *testing.T) {
	for status, want := range map[DeploymentStatus]string{
		DeploymentStatusPending:     MCPDeploymentStatusDeploying,
		DeploymentStatusDeploying:   MCPDeploymentStatusDeploying,
		DeploymentStatusDeployed:    MCPDeploymentStatusDeployed,
		DeploymentStatusUndeploying: MCPDeploymentStatusUndeploying,
		DeploymentStatusUndeployed:  MCPDeploymentStatusUndeployed,
		DeploymentStatusFailed:      MCPDeploymentStatusFailed,
		DeploymentStatusArchived:    MCPDeploymentStatusUndeployed,
	} {
		assert.Equal(t, want, MCPDeploymentStatusOf(UserStatus{Status: status, Reason: "any"}), "status %s", status)
	}
}

// What an MCP binding shows comes from the derived user status, not the raw row.
func TestMCPDeploymentStatusOfARow(t *testing.T) {
	due := time.Now()
	for name, tc := range map[string]struct {
		row  DeploymentStatusRecord
		want string
	}{
		"a PENDING row":         {DeploymentStatusRecord{Status: DeploymentStatusPending, NextAttemptAt: &due}, MCPDeploymentStatusDeploying},
		"a waiting redeploy":    {DeploymentStatusRecord{Status: DeploymentStatusDeployed, NextAttemptAt: &due}, MCPDeploymentStatusDeploying},
		"a render given up":     {DeploymentStatusRecord{Status: DeploymentStatusPending, RenderReason: "bad"}, MCPDeploymentStatusFailed},
		"a failed deploy":       {DeploymentStatusRecord{Status: DeploymentStatusFailed}, MCPDeploymentStatusFailed},
		"an acked deploy":       {DeploymentStatusRecord{Status: DeploymentStatusDeployed}, MCPDeploymentStatusDeployed},
		"an undeployed binding": {DeploymentStatusRecord{Status: DeploymentStatusUndeployed}, MCPDeploymentStatusUndeployed},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, MCPDeploymentStatusOf(UserStatusOf(tc.row)))
		})
	}
}
