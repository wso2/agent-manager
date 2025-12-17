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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

func CreateOrganization(t *testing.T, orgID uuid.UUID, userIdpID uuid.UUID, orgName string) models.Organization {
	org := &models.Organization{
		ID:                orgID,
		UserIdpId:         userIdpID,
		OrgName:           orgName,
		OpenChoreoOrgName: orgName,
		CreatedAt:         time.Now(),
	}
	err := db.DB(context.Background()).Create(org).Error
	require.NoError(t, err)
	str, _ := json.MarshalIndent(org, "", "  ")
	t.Logf("Created Organization: %s", str)
	return *org
}

func CreateProject(t *testing.T, projectID uuid.UUID, orgID uuid.UUID, projectName string) models.Project {
	project := &models.Project{
		ID:                projectID,
		OrgID:             orgID,
		Name:              projectName,
		CreatedAt:         time.Now(),
		OpenChoreoProject: projectName,
	}
	err := db.DB(context.Background()).Create(project).Error
	require.NoError(t, err)
	str, _ := json.MarshalIndent(project, "", "  ")
	t.Logf("Created Project: %s", str)
	return *project
}

func CreateAgent(t *testing.T, agentID uuid.UUID, orgID uuid.UUID, projectID uuid.UUID, agentName string, provisioningType string) models.Agent {
	agent := &models.Agent{
		ID:          agentID,
		ProvisioningType:   provisioningType,
		ProjectId:   projectID,
		OrgID:       orgID,
		Name:        agentName,
		DisplayName: agentName,
	}
	err := db.DB(context.Background()).Create(agent).Error
	require.NoError(t, err)
	str, _ := json.MarshalIndent(agent, "", "  ")
	t.Logf("Created Agent: %s", str)
	return *agent
}
