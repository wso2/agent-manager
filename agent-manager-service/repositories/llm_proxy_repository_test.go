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

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
)

func TestIsAgentOrMonitorProxy(t *testing.T) {
	repo := NewLLMProxyRepo(db.GetDB())
	ctx := context.Background()
	ouID := newOrg(t)

	agentProxy, agentProxyUUID := seedLLMProxy(t, ouID)
	mapLLMProxyToAgent(t, ouID, agentProxyUUID)
	userProxy, _ := seedLLMProxy(t, ouID)

	tests := []struct {
		name   string
		handle string
		ouID   string
		want   bool
	}{
		{name: "agent LLM config proxy", handle: agentProxy, ouID: ouID, want: true},
		{name: "user proxy", handle: userProxy, ouID: ouID, want: false},
		{name: "unknown handle", handle: "no-such-proxy", ouID: ouID, want: false},
		{name: "agent proxy handle in another org", handle: agentProxy, ouID: newOrg(t), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.IsAgentOrMonitorProxy(ctx, tt.handle, tt.ouID)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
