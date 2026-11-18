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

package gwconv

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// An LLM deployment shows the derived status with where its transition is heading and why, not the raw row.
func TestConvertModelToSpecLLMDeploymentResponse(t *testing.T) {
	due := time.Now()
	withRow := func(row gwmodels.DeploymentStatusRecord) *gwmodels.Deployment {
		d := &gwmodels.Deployment{
			DeploymentID: uuid.New(), Name: "p-1a2b3c4d", ArtifactUUID: uuid.New(),
			GatewayUUID: uuid.New(), CreatedAt: due,
		}
		d.SetStatusRow(&row)
		return d
	}
	archived := gwmodels.DeploymentStatusArchived
	for name, tc := range map[string]struct {
		deployment           *gwmodels.Deployment
		status, desired, why string
	}{
		"deploying": {withRow(gwmodels.DeploymentStatusRecord{
			Status: gwmodels.DeploymentStatusDeploying, StatusDesired: gwmodels.DeploymentStatusDeployed,
		}), "DEPLOYING", "DEPLOYED", ""},
		"deployed": {withRow(gwmodels.DeploymentStatusRecord{
			Status: gwmodels.DeploymentStatusDeployed, StatusDesired: gwmodels.DeploymentStatusDeployed,
		}), "DEPLOYED", "DEPLOYED", ""},
		"deployed with a redeploy owed": {withRow(gwmodels.DeploymentStatusRecord{
			Status: gwmodels.DeploymentStatusDeployed, StatusDesired: gwmodels.DeploymentStatusDeployed,
			NextAttemptAt: &due, RenderReason: "the provider changed",
		}), "DEPLOYING", "DEPLOYED", "the provider changed"},
		"deployed with the redeploy given up": {withRow(gwmodels.DeploymentStatusRecord{
			Status: gwmodels.DeploymentStatusDeployed, StatusDesired: gwmodels.DeploymentStatusDeployed,
			RenderReason: "invalid security config",
		}), "DEPLOYED", "DEPLOYED", "invalid security config"},
		"failed deploy": {withRow(gwmodels.DeploymentStatusRecord{
			Status: gwmodels.DeploymentStatusFailed, StatusDesired: gwmodels.DeploymentStatusDeployed,
			StatusReason: "GATEWAY_PROCESSING_ERROR",
		}), "FAILED", "DEPLOYED", "GATEWAY_PROCESSING_ERROR"},
		"timed out deploy": {withRow(gwmodels.DeploymentStatusRecord{
			Status: gwmodels.DeploymentStatusFailed, StatusDesired: gwmodels.DeploymentStatusDeployed,
			StatusReason: "DEPLOYMENT_TIMEOUT",
		}), "FAILED", "DEPLOYED", "DEPLOYMENT_TIMEOUT"},
		"failed undeploy": {withRow(gwmodels.DeploymentStatusRecord{
			Status: gwmodels.DeploymentStatusFailed, StatusDesired: gwmodels.DeploymentStatusUndeployed,
			StatusReason: "GATEWAY_PROCESSING_ERROR",
		}), "FAILED", "UNDEPLOYED", "GATEWAY_PROCESSING_ERROR"},
		"undeploying": {withRow(gwmodels.DeploymentStatusRecord{
			Status: gwmodels.DeploymentStatusUndeploying, StatusDesired: gwmodels.DeploymentStatusUndeployed,
		}), "UNDEPLOYING", "UNDEPLOYED", ""},
		"undeployed": {withRow(gwmodels.DeploymentStatusRecord{
			Status: gwmodels.DeploymentStatusUndeployed, StatusDesired: gwmodels.DeploymentStatusUndeployed,
		}), "UNDEPLOYED", "UNDEPLOYED", ""},
		"archived": {&gwmodels.Deployment{DeploymentID: uuid.New(), Status: &archived}, "ARCHIVED", "", ""},
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
