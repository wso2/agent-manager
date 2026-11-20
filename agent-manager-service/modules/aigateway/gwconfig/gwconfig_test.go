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
package gwconfig

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func from(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestDefaults(t *testing.T) {
	s, err := Load(from(nil))
	require.NoError(t, err)
	assert.Equal(t, "memory", s.ManifestCache.Backend)
	assert.Equal(t, 5*time.Second, s.Reconcile.RenderInterval)
	assert.Equal(t, 30, s.Reconcile.RenderAttemptBudget)
	assert.Equal(t, 300*time.Second, s.DeploymentTimeout.Timeout)
	assert.Equal(t, 1000, s.WebSocket.MaxConnections)
	assert.Equal(t, "*", s.AgentCORS.AllowOrigin)
	assert.Empty(t, s.DefaultGateway.Name, "no default gateway unless named")
}

func TestInvalidSettings(t *testing.T) {
	cases := map[string]struct {
		values map[string]string
		want   string
	}{
		"unknown cache backend":       {map[string]string{"MANIFEST_CACHE_BACKEND": "memcached"}, "MODULE_AI_GATEWAY_MANIFEST_CACHE_BACKEND"},
		"redis without a host":        {map[string]string{"MANIFEST_CACHE_BACKEND": "redis"}, "MANIFEST_CACHE_REDIS_HOST is required"},
		"zero sweep interval":         {map[string]string{"DEPLOYMENT_TIMEOUT_SWEEP_INTERVAL_SECONDS": "0"}, "DEPLOYMENT_TIMEOUT_SWEEP_INTERVAL_SECONDS must be greater than 0"},
		"zero render budget":          {map[string]string{"RECONCILE_RENDER_ATTEMPT_BUDGET": "0"}, "RECONCILE_RENDER_ATTEMPT_BUDGET"},
		"not a number":                {map[string]string{"WEBSOCKET_MAX_CONNECTIONS": "many"}, "must be an integer"},
		"empty failure window":        {map[string]string{"FAILURE_MAX_AGE_SECONDS": "300"}, "FAILURE_MAX_AGE_SECONDS"},
		"NaN failure percentage":      {map[string]string{"FAILURE_PERCENTAGE_THRESHOLD": "NaN"}, "FAILURE_PERCENTAGE_THRESHOLD"},
		"credentials for any origin":  {map[string]string{"AGENT_CORS_ALLOW_CREDENTIALS": "true"}, "AGENT_CORS_ALLOW_CREDENTIALS"},
		"default gateway with no url": {map[string]string{"DEFAULT_GATEWAY_NAME": "default"}, "DEFAULT_GATEWAY_VHOST"},
		"dial address without a port": {map[string]string{"A2A_CARD_FETCH_DIAL_ADDR": "gateway-runtime"}, "A2A_CARD_FETCH_DIAL_ADDR"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(from(tc.values))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
