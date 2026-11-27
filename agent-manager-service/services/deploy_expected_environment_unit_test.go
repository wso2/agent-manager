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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestDeployAgent_ExpectedEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		refused bool
	}{
		{"omitted", `{"imageId":"` + ownBuildImage + `"}`, false},
		{"empty", `{"imageId":"` + ownBuildImage + `","expectedEnvironment":""}`, false},
		{"the pipeline's first environment", `{"imageId":"` + ownBuildImage + `","expectedEnvironment":"dev"}`, false},
		{"another environment", `{"imageId":"` + ownBuildImage + `","expectedEnvironment":"staging"}`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req spec.DeployAgentRequest
			require.NoError(t, json.Unmarshal([]byte(tc.body), &req))
			f := newOwnershipFixture("")
			_, err := f.svc.DeployAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &req)
			if tc.refused {
				require.ErrorIs(t, err, utils.ErrDeployEnvironmentMismatch)
				assert.False(t, *f.bindingReached, "a refused deploy must not reach any write")
				return
			}
			assert.NotErrorIs(t, err, utils.ErrDeployEnvironmentMismatch)
			assert.True(t, *f.bindingReached)
		})
	}
}
