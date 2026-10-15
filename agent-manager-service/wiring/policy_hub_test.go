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

package wiring

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

func TestProvidePolicyHubClient_EmptyBaseURLIsDisabled(t *testing.T) {
	client, err := ProvidePolicyHubClient(config.Config{})

	require.NoError(t, err)
	got, err := client.ListPolicies(context.Background())
	require.NoError(t, err, "a disabled hub must not fail listings")
	assert.Empty(t, got)
}

func TestProvidePolicyHubClient_RejectsInvalidCacheTTL(t *testing.T) {
	for _, ttl := range []string{"soon", "0s", "-5m"} {
		t.Run(ttl, func(t *testing.T) {
			_, err := ProvidePolicyHubClient(config.Config{PolicyHub: config.PolicyHubConfig{
				BaseURL:  "https://hub.example.com/v1.0",
				CacheTTL: ttl,
			}})

			require.Error(t, err)
		})
	}
}

func TestProvidePolicyHubClient_BuildsForBothCacheBackends(t *testing.T) {
	// Neither backend connects at construction (the Redis client dials lazily), so
	// this checks the wiring path without a hub or a Redis server.
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			client, err := ProvidePolicyHubClient(config.Config{
				PolicyHub: config.PolicyHubConfig{BaseURL: "https://hub.example.com/v1.0", CacheTTL: "10m"},
				GatewayManifestCache: config.GatewayManifestCacheConfig{
					Backend: backend,
					Redis:   config.GatewayManifestCacheRedisConfig{Host: "redis.invalid", Port: 6379},
				},
			})

			require.NoError(t, err)
			assert.NotNil(t, client)
		})
	}
}
