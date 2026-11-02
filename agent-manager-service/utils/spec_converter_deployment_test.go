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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// An LLM deployment shows the derived status with where its transition is heading and why, not the raw row.
func TestConvertModelToSpecLLMDeploymentResponse(t *testing.T) {
	due := time.Now()
	withRow := func(row models.DeploymentStatusRecord) *models.Deployment {
		d := &models.Deployment{
			DeploymentID: uuid.New(), Name: "p-1a2b3c4d", ArtifactUUID: uuid.New(),
			GatewayUUID: uuid.New(), CreatedAt: due,
		}
		d.SetStatusRow(&row)
		return d
	}
	archived := models.DeploymentStatusArchived
	for name, tc := range map[string]struct {
		deployment           *models.Deployment
		status, desired, why string
	}{
		"deploying": {withRow(models.DeploymentStatusRecord{
			Status: models.DeploymentStatusDeploying, StatusDesired: models.DeploymentStatusDeployed,
		}), "DEPLOYING", "DEPLOYED", ""},
		"deployed": {withRow(models.DeploymentStatusRecord{
			Status: models.DeploymentStatusDeployed, StatusDesired: models.DeploymentStatusDeployed,
		}), "DEPLOYED", "DEPLOYED", ""},
		"deployed with a redeploy owed": {withRow(models.DeploymentStatusRecord{
			Status: models.DeploymentStatusDeployed, StatusDesired: models.DeploymentStatusDeployed,
			NextAttemptAt: &due, RenderReason: "the provider changed",
		}), "DEPLOYING", "DEPLOYED", "the provider changed"},
		"deployed with the redeploy given up": {withRow(models.DeploymentStatusRecord{
			Status: models.DeploymentStatusDeployed, StatusDesired: models.DeploymentStatusDeployed,
			RenderReason: "invalid security config",
		}), "DEPLOYED", "DEPLOYED", "invalid security config"},
		"failed deploy": {withRow(models.DeploymentStatusRecord{
			Status: models.DeploymentStatusFailed, StatusDesired: models.DeploymentStatusDeployed,
			StatusReason: "GATEWAY_PROCESSING_ERROR",
		}), "FAILED", "DEPLOYED", "GATEWAY_PROCESSING_ERROR"},
		"timed out deploy": {withRow(models.DeploymentStatusRecord{
			Status: models.DeploymentStatusFailed, StatusDesired: models.DeploymentStatusDeployed,
			StatusReason: "DEPLOYMENT_TIMEOUT",
		}), "FAILED", "DEPLOYED", "DEPLOYMENT_TIMEOUT"},
		"failed undeploy": {withRow(models.DeploymentStatusRecord{
			Status: models.DeploymentStatusFailed, StatusDesired: models.DeploymentStatusUndeployed,
			StatusReason: "GATEWAY_PROCESSING_ERROR",
		}), "FAILED", "UNDEPLOYED", "GATEWAY_PROCESSING_ERROR"},
		"undeploying": {withRow(models.DeploymentStatusRecord{
			Status: models.DeploymentStatusUndeploying, StatusDesired: models.DeploymentStatusUndeployed,
		}), "UNDEPLOYING", "UNDEPLOYED", ""},
		"undeployed": {withRow(models.DeploymentStatusRecord{
			Status: models.DeploymentStatusUndeployed, StatusDesired: models.DeploymentStatusUndeployed,
		}), "UNDEPLOYED", "UNDEPLOYED", ""},
		"archived": {&models.Deployment{DeploymentID: uuid.New(), Status: &archived}, "ARCHIVED", "", ""},
	} {
		t.Run(name, func(t *testing.T) {
			got := ConvertModelToSpecLLMDeploymentResponse(tc.deployment)

			assert.Equal(t, tc.deployment.DeploymentID.String(), got.DeploymentId)
			assert.Equal(t, tc.status, got.Status)
			if tc.desired == "" {
				assert.Nil(t, got.StatusDesired)
			} else {
				require.NotNil(t, got.StatusDesired)
				assert.Equal(t, tc.desired, *got.StatusDesired)
			}
			if tc.why == "" {
				assert.Nil(t, got.StatusReason)
			} else {
				require.NotNil(t, got.StatusReason)
				assert.Equal(t, tc.why, *got.StatusReason)
			}
		})
	}
}
