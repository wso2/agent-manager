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

package gwsvc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/constants"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestResolveResilienceTimeoutSeconds(t *testing.T) {
	tests := []struct {
		name      string
		existing  *gwmodels.AgentConfig
		requested *int32
		withDef   bool
		want      int32
		wantErr   bool
	}{
		{"nothing set, withDefaults=true falls back to default", nil, nil, true, constants.DefaultResilienceTimeoutSeconds, false},
		{"nothing set, withDefaults=false yields zero (no override)", nil, nil, false, 0, false},
		{"existing DB value is used", &gwmodels.AgentConfig{ResilienceTimeoutSeconds: int32Ptr(60)}, nil, false, 60, false},
		{"request overrides existing DB value", &gwmodels.AgentConfig{ResilienceTimeoutSeconds: int32Ptr(60)}, int32Ptr(90), false, 90, false},
		{"out-of-bounds request is rejected", &gwmodels.AgentConfig{ResilienceTimeoutSeconds: int32Ptr(60)}, int32Ptr(10000), false, 0, true},
		{"minimum bound is accepted", nil, int32Ptr(constants.MinResilienceTimeoutSeconds), false, constants.MinResilienceTimeoutSeconds, false},
		{"maximum bound is accepted", nil, int32Ptr(constants.MaxResilienceTimeoutSeconds), false, constants.MaxResilienceTimeoutSeconds, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveResilienceTimeoutSeconds(tc.existing, tc.requested, tc.withDef)
			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, utils.ErrInvalidInput)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func int32Ptr(v int32) *int32 { return &v }
