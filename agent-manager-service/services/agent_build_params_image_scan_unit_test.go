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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The image scan setting reaches OpenChoreo as sent, and an omitted one stays nil so the
// saved setting is kept.
func TestUpdateBuildParametersPassesTheImageScanSetting(t *testing.T) {
	failBuild, severity := false, "HIGH"
	for name, sent := range map[string]*spec.ImageScan{
		"sent":    {FailBuild: &failBuild, Severity: &severity},
		"omitted": nil,
	} {
		t.Run(name, func(t *testing.T) {
			svc, ocClient := buildParamsServiceForSubType(string(utils.AgentSubTypeChatAPI))
			req := buildParamsRequestWithSubType(string(utils.AgentSubTypeChatAPI))
			req.ImageScan = sent

			_, err := svc.UpdateAgentBuildParameters(context.Background(), "org-1", "proj-1", "agent-1", req)

			require.NoError(t, err)
			require.Len(t, ocClient.UpdateComponentBuildParametersCalls(), 1)
			got := ocClient.UpdateComponentBuildParametersCalls()[0].Req.ImageScan
			if sent == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, &failBuild, got.FailBuild)
			assert.Equal(t, &severity, got.Severity)
		})
	}
}
