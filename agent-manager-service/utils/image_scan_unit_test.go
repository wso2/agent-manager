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

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// A build parameters update takes the severities the build workflow knows and rejects
// any other before it is saved.
func TestValidateAgentBuildParametersUpdatePayloadChecksImageScanSeverity(t *testing.T) {
	port := int32(9099)
	subType := string(AgentSubTypeChatAPI)
	request := spec.UpdateAgentBuildParametersRequest{
		Provisioning: spec.Provisioning{
			Type: string(InternalAgent),
			Repository: &spec.RepositoryConfig{
				Url:     "https://github.com/wso2/agent-manager",
				Branch:  "main",
				AppPath: "/",
			},
		},
		AgentType: spec.AgentType{Type: string(AgentTypeAPI), SubType: &subType},
		Build: spec.DockerBuildAsBuild(&spec.DockerBuild{
			Type:   "docker",
			Docker: spec.DockerConfig{DockerfilePath: "/Dockerfile"},
		}),
		InputInterface: spec.InputInterface{Type: "HTTP", Port: &port},
	}
	require.NoError(t, ValidateAgentBuildParametersUpdatePayload(request))

	failBuild := false
	request.ImageScan = &spec.ImageScan{FailBuild: &failBuild}
	require.NoError(t, ValidateAgentBuildParametersUpdatePayload(request), "no severity uses the default")

	for _, severity := range []string{"CRITICAL", "HIGH", "MEDIUM"} {
		request.ImageScan = &spec.ImageScan{Severity: &severity}
		assert.NoError(t, ValidateAgentBuildParametersUpdatePayload(request), severity)
	}

	for _, severity := range []string{"LOW", "high", ""} {
		request.ImageScan = &spec.ImageScan{Severity: &severity}
		err := ValidateAgentBuildParametersUpdatePayload(request)
		require.Error(t, err, severity)
		assert.Contains(t, err.Error(), "imageScan.severity")
	}
}

// The agent response carries the saved setting, and none when none is saved.
func TestConvertToAgentResponseCarriesTheImageScanSetting(t *testing.T) {
	failBuild, severity := false, "MEDIUM"
	got := ConvertToAgentResponse(&models.AgentResponse{
		Provisioning: models.Provisioning{Type: string(InternalAgent)},
		ImageScan:    &models.ImageScan{FailBuild: &failBuild, Severity: &severity},
	})
	require.NotNil(t, got.ImageScan)
	assert.Equal(t, &failBuild, got.ImageScan.FailBuild)
	assert.Equal(t, &severity, got.ImageScan.Severity)

	got = ConvertToAgentResponse(&models.AgentResponse{Provisioning: models.Provisioning{Type: string(InternalAgent)}})
	assert.Nil(t, got.ImageScan)
}
