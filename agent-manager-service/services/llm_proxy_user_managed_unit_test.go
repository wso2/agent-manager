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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestRequireUserManaged(t *testing.T) {
	dbErr := errors.New("database unavailable")
	tests := []struct {
		name    string
		owned   bool
		repoErr error
		wantErr error
	}{
		{name: "agent or monitor proxy", owned: true, wantErr: utils.ErrLLMProxyManagedByAMS},
		{name: "user proxy", owned: false},
		{name: "repository error", repoErr: dbErr, wantErr: dbErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &repomocks.LLMProxyRepositoryMock{
				IsAgentOrMonitorProxyFunc: func(_ context.Context, proxyHandle, ouID string) (bool, error) {
					require.Equal(t, "proxy-a", proxyHandle)
					require.Equal(t, "ou-acme", ouID)
					return tt.owned, tt.repoErr
				},
			}
			svc := NewLLMProxyDeploymentService(nil, nil, repo, nil, nil, nil, nil, nil)

			err := svc.RequireUserManaged(context.Background(), "proxy-a", "ou-acme")

			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
			if tt.repoErr != nil {
				assert.NotErrorIs(t, err, utils.ErrLLMProxyManagedByAMS, "a lookup failure must not read as AMS-owned")
			}
		})
	}
}
