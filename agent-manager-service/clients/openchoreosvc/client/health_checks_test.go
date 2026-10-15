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

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type m = map[string]interface{}

// checkSchema is one check's schema in agent-api.yaml's parameters.probes, with defaults.
func checkSchema(enabled bool, failureThreshold int) m {
	return m{"type": "object", "properties": m{
		"enabled":             m{"type": "boolean", "default": enabled},
		"type":                m{"type": "string", "default": "tcp"},
		"path":                m{"type": "string", "default": "/health"},
		"port":                m{"type": "integer"},
		"initialDelaySeconds": m{"type": "integer", "default": 10},
		"periodSeconds":       m{"type": "integer", "default": 5},
		"timeoutSeconds":      m{"type": "integer", "default": 1},
		"failureThreshold":    m{"type": "integer", "default": failureThreshold},
	}}
}

// parametersSchema is an agent-api ComponentType's parameters schema with health checks.
func parametersSchema() m {
	return m{"type": "object", "properties": m{
		"routePath": m{"type": "string"},
		probesKey: m{"type": "object", "properties": m{
			"startup":   checkSchema(true, 60),
			"readiness": checkSchema(true, 6),
			"liveness":  checkSchema(false, 3),
		}},
	}}
}

func TestMergeMaps(t *testing.T) {
	base := m{"startup": m{"failureThreshold": 60, "periodSeconds": 5}, "liveness": m{"enabled": false}}
	override := m{"startup": m{"failureThreshold": 80}, "readiness": m{"periodSeconds": 10}}

	merged := mergeMaps(base, override)

	assert.Equal(t, m{
		"startup":   m{"failureThreshold": 80, "periodSeconds": 5},
		"liveness":  m{"enabled": false},
		"readiness": m{"periodSeconds": 10},
	}, merged)
	assert.Equal(t, 60, base["startup"].(m)["failureThreshold"], "the base map must not change")
	assert.Equal(t, m{"failureThreshold": 80}, override["startup"], "the override map must not change")
}

func TestProbeDefaults(t *testing.T) {
	defaults, ok := probeDefaults(parametersSchema())

	require.True(t, ok)
	assert.Equal(t, m{
		"enabled": true, "type": "tcp", "path": "/health",
		"initialDelaySeconds": 10, "periodSeconds": 5, "timeoutSeconds": 1, "failureThreshold": 60,
	}, defaults["startup"], "a field without a default (port) is left out")
	assert.Equal(t, false, defaults["liveness"].(m)["enabled"])
}

func TestProbeDefaults_ComponentTypeWithoutHealthChecks(t *testing.T) {
	_, ok := probeDefaults(m{"type": "object", "properties": m{"routePath": m{"type": "string"}}})
	assert.False(t, ok)

	_, ok = probeDefaults(nil)
	assert.False(t, ok)
}

// healthCheckAPI is a stub OpenChoreo API with one agent, my-agent:
//   - now: liveness off and startup failures allowed 80 (its current build configuration),
//   - in "dev": running release my-agent-r1, deployed with liveness on.
func healthCheckAPI(t *testing.T, bindingEnvironment string) http.Handler {
	t.Helper()
	write := func(w http.ResponseWriter, body interface{}) {
		require.NoError(t, json.NewEncoder(w).Encode(body))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/namespaces/default/components/my-agent":
			write(w, m{
				"metadata": m{"name": "my-agent"},
				"spec": m{
					"componentType": m{"kind": "ComponentType", "name": "deployment/agent-api"},
					"owner":         m{"projectName": "proj"},
					"parameters": m{probesKey: m{
						"liveness": m{"enabled": false},
						"startup":  m{"failureThreshold": 80},
					}},
				},
			})
		case "/api/v1/namespaces/default/componenttypes/agent-api":
			write(w, m{
				"metadata": m{"name": "agent-api"},
				"spec":     m{"workloadType": "proxy", "parameters": m{"openAPIV3Schema": parametersSchema()}},
			})
		case "/api/v1/namespaces/default/releasebindings":
			write(w, m{"items": []m{{
				"metadata": m{"name": "my-agent-" + bindingEnvironment},
				"spec": m{
					"environment": bindingEnvironment,
					"releaseName": "my-agent-r1",
					"owner":       m{"componentName": "my-agent", "projectName": "proj"},
				},
			}}, "pagination": m{}})
		case "/api/v1/namespaces/default/componentreleases/my-agent-r1":
			write(w, m{
				"metadata": m{"name": "my-agent-r1"},
				"spec": m{
					"owner": m{"componentName": "my-agent", "projectName": "proj"},
					"componentType": m{
						"kind": "ComponentType", "name": "agent-api",
						"spec": m{"parameters": m{"openAPIV3Schema": parametersSchema()}},
					},
					"componentProfile": m{"parameters": m{probesKey: m{"liveness": m{"enabled": true}}}},
					"workload":         m{},
				},
			})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func TestGetEnvHealthChecks_EnvironmentShowsTheReleaseItRuns(t *testing.T) {
	c := newTestClient(t, healthCheckAPI(t, "dev"))

	checks, err := c.GetEnvHealthChecks(context.Background(), "acme", "my-agent", "dev")

	require.NoError(t, err)
	require.NotNil(t, checks)
	assert.True(t, *checks.Liveness.Enabled, "dev runs a release deployed with liveness on, whatever the agent has now")
	assert.Equal(t, int32(60), *checks.Startup.FailureThreshold, "the agent's newer startup value is not deployed in dev yet")
	assert.Equal(t, int32(6), *checks.Readiness.FailureThreshold, "unset values fall back to the defaults")
}

func TestGetEnvHealthChecks_CurrentSettingsWithoutEnvironment(t *testing.T) {
	c := newTestClient(t, healthCheckAPI(t, "dev"))

	checks, err := c.GetEnvHealthChecks(context.Background(), "acme", "my-agent", "")

	require.NoError(t, err)
	require.NotNil(t, checks)
	assert.False(t, *checks.Liveness.Enabled, "without an environment, the agent's current settings apply")
	assert.Equal(t, int32(80), *checks.Startup.FailureThreshold)
}

func TestGetEnvHealthChecks_NothingDeployedInTheEnvironment(t *testing.T) {
	c := newTestClient(t, healthCheckAPI(t, "dev"))

	checks, err := c.GetEnvHealthChecks(context.Background(), "acme", "my-agent", "prod")

	require.NoError(t, err)
	assert.Nil(t, checks)
}

func TestGetHealthCheckDefaults(t *testing.T) {
	c := newTestClient(t, healthCheckAPI(t, "dev"))

	checks, err := c.GetHealthCheckDefaults(context.Background(), "acme")

	require.NoError(t, err)
	require.NotNil(t, checks)
	assert.Equal(t, int32(60), *checks.Startup.FailureThreshold, "the ComponentType's default, not my-agent's 80")
	assert.Equal(t, int32(5), *checks.Startup.PeriodSeconds)
	assert.False(t, *checks.Liveness.Enabled)
}
