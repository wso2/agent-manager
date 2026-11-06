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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func TestMCPGatewayContext(t *testing.T) {
	ctx := "/weather"
	project := "p1"
	projectUUID := "p1-uuid"

	org := &models.MCPProxy{Configuration: models.MCPProxyConfig{Context: &ctx}}
	assert.Equal(t, "/weather", mcpGatewayContext(org, "p1"), "an org proxy's context is unchanged")

	proj := &models.MCPProxy{
		ProjectUUID: &projectUUID, ProjectName: &project,
		Configuration: models.MCPProxyConfig{Context: &ctx},
	}
	assert.Equal(t, "/p1/weather", mcpGatewayContext(proj, "p1"))
	assert.Equal(t, "/p1/weather", mcpGatewayContext(proj, ""), "falls back to the proxy's own project")

	root := "/"
	proj.Configuration.Context = &root
	assert.Equal(t, "/p1", mcpGatewayContext(proj, "p1"))
}
