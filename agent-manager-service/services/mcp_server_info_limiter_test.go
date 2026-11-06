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

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestConcurrencyLimiter_PerKeyLimit(t *testing.T) {
	l := newConcurrencyLimiter(10, 2)
	r1, ok := l.tryAcquire("org1")
	require.True(t, ok)
	_, ok = l.tryAcquire("org1")
	require.True(t, ok)
	_, ok = l.tryAcquire("org1")
	assert.False(t, ok, "third slot for the same key must be refused")
	_, ok = l.tryAcquire("org2")
	assert.True(t, ok, "another key is not affected")

	r1()
	r1() // a second release must not free another slot
	_, ok = l.tryAcquire("org1")
	assert.True(t, ok)
	_, ok = l.tryAcquire("org1")
	assert.False(t, ok)
}

func TestConcurrencyLimiter_TotalLimit(t *testing.T) {
	l := newConcurrencyLimiter(2, 2)
	_, ok := l.tryAcquire("org1")
	require.True(t, ok)
	r2, ok := l.tryAcquire("org2")
	require.True(t, ok)
	_, ok = l.tryAcquire("org3")
	assert.False(t, ok, "pod-wide limit must hold across orgs")
	r2()
	_, ok = l.tryAcquire("org3")
	assert.True(t, ok)
	assert.Equal(t, 0, l.perKey["org2"])
}

func TestFetchServerInfo_RefusedWhenUserOverLimit(t *testing.T) {
	orgUUID, userID := "org-limit-test", "user-1"
	var releases []func()
	for range mcpServerInfoMaxInFlightPerUser {
		release, ok := mcpServerInfoLimiter.tryAcquire(mcpServerInfoLimiterKey(orgUUID, userID))
		require.True(t, ok)
		releases = append(releases, release)
	}
	t.Cleanup(func() {
		for _, release := range releases {
			release()
		}
	})

	url := "https://example.com/mcp"
	_, err := (&MCPProxyService{}).FetchServerInfo(context.Background(), orgUUID, userID, &models.MCPServerInfoFetchRequest{URL: &url})
	assert.ErrorIs(t, err, utils.ErrTooManyRequests)
}

func TestMCPServerInfoLimiterKey_SeparatesUsersAndOrgs(t *testing.T) {
	assert.NotEqual(t, mcpServerInfoLimiterKey("org1", "user1"), mcpServerInfoLimiterKey("org1", "user2"))
	assert.NotEqual(t, mcpServerInfoLimiterKey("org1", "user1"), mcpServerInfoLimiterKey("org2", "user1"))
}
