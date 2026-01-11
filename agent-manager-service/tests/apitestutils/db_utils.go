// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package apitestutils

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

func CreateAgent(t *testing.T, agentID uuid.UUID, orgName string, projectName string, agentName string, provisioningType string) models.Agent {
	agent := &models.Agent{
		ID:               agentID,
		ProvisioningType: provisioningType,
		ProjectName:      projectName,
		OrgName:          orgName,
		Name:             agentName,
		DisplayName:      agentName,
	}
	err := db.DB(context.Background()).Create(agent).Error
	require.NoError(t, err)
	str, _ := json.MarshalIndent(agent, "", "  ")
	t.Logf("Created Agent: %s", str)
	return *agent
}
