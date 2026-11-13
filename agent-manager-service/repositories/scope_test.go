//go:build integration

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

package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func TestEnsureOrgSeedsDefaultEnvironmentAndPipeline(t *testing.T) {
	gdb := db.GetDB()
	ou := uuid.Must(uuid.NewV7()).String()

	org, err := ensureOrg(context.Background(), gdb, ou)
	require.NoError(t, err)

	var env models.Environment
	require.NoError(t, gdb.Where("org_id = ? AND name = ?", org.ID, models.DefaultEnvironmentName).Take(&env).Error)
	var pipeline models.DeploymentPipeline
	require.NoError(t, gdb.Where("org_id = ? AND name = ?", org.ID, models.DefaultPipelineName).Take(&pipeline).Error)
	var stages []models.DeploymentPipelineEnvironment
	require.NoError(t, gdb.Where("pipeline_id = ?", pipeline.ID).Find(&stages).Error)
	require.Len(t, stages, 1)
	require.Equal(t, env.ID, stages[0].EnvironmentID)
	require.Equal(t, 0, stages[0].Position)

	again, err := ensureOrg(context.Background(), gdb, ou)
	require.NoError(t, err)
	require.Equal(t, org.ID, again.ID)
}
