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

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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

func TestMergeEnvProbeTimings(t *testing.T) {
	configs := m{"restartedAt": "t0", probesKey: m{"startup": m{"failureThreshold": 80}}}
	rb := &gen.ReleaseBinding{Spec: &gen.ReleaseBindingSpec{ComponentTypeEnvironmentConfigs: &configs}}

	mergeEnvProbeTimings(rb, m{"startup": m{"periodSeconds": 7}, "liveness": m{"timeoutSeconds": 2}})

	assert.Equal(t, m{
		"startup":  m{"failureThreshold": 80, "periodSeconds": 7},
		"liveness": m{"timeoutSeconds": 2},
	}, (*rb.Spec.ComponentTypeEnvironmentConfigs)[probesKey], "only the wait times sent change")
	assert.Equal(t, "t0", (*rb.Spec.ComponentTypeEnvironmentConfigs)["restartedAt"], "other settings are kept")
}

func TestMergeEnvProbeTimings_NothingSent(t *testing.T) {
	rb := &gen.ReleaseBinding{Spec: &gen.ReleaseBindingSpec{}}

	mergeEnvProbeTimings(rb, nil)
	mergeEnvProbeTimings(rb, m{})

	assert.Nil(t, rb.Spec.ComponentTypeEnvironmentConfigs)
}

func TestSetEnvProbes(t *testing.T) {
	sourceProbes := m{"startup": m{"failureThreshold": 80}}

	t.Run("copies the source's wait times and keeps the target's other settings", func(t *testing.T) {
		configs := m{"resources": m{"requests": m{"cpu": "30m"}}, probesKey: m{"readiness": m{"periodSeconds": 9}}}
		spec := &gen.ReleaseBindingSpec{ComponentTypeEnvironmentConfigs: &configs}

		setEnvProbes(spec, sourceProbes)

		assert.Equal(t, sourceProbes, (*spec.ComponentTypeEnvironmentConfigs)[probesKey])
		assert.Equal(t, m{"requests": m{"cpu": "30m"}}, (*spec.ComponentTypeEnvironmentConfigs)["resources"])
	})

	t.Run("removes the target's wait times when the source has none", func(t *testing.T) {
		configs := m{probesKey: m{"readiness": m{"periodSeconds": 9}}}
		spec := &gen.ReleaseBindingSpec{ComponentTypeEnvironmentConfigs: &configs}

		setEnvProbes(spec, nil)

		assert.NotContains(t, *spec.ComponentTypeEnvironmentConfigs, probesKey)
	})

	t.Run("creates the settings for a new binding", func(t *testing.T) {
		spec := &gen.ReleaseBindingSpec{}

		setEnvProbes(spec, sourceProbes)

		require.NotNil(t, spec.ComponentTypeEnvironmentConfigs)
		assert.Equal(t, sourceProbes, (*spec.ComponentTypeEnvironmentConfigs)[probesKey])
	})

	t.Run("leaves a new binding without settings when the source has none", func(t *testing.T) {
		spec := &gen.ReleaseBindingSpec{}

		setEnvProbes(spec, nil)

		assert.Nil(t, spec.ComponentTypeEnvironmentConfigs)
	})
}

// healthCheckAPI is a stub OpenChoreo API with one agent, my-agent:
//   - now: liveness off and startup failures allowed 80 (its current build configuration),
//   - in "dev": running release my-agent-r1, deployed with liveness on, plus the
//     environment's own readiness interval of 10.
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
					"environment":                     bindingEnvironment,
					"releaseName":                     "my-agent-r1",
					"owner":                           m{"componentName": "my-agent", "projectName": "proj"},
					"componentTypeEnvironmentConfigs": m{probesKey: m{"readiness": m{"periodSeconds": 10}}},
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
	assert.Equal(t, int32(10), *checks.Readiness.PeriodSeconds, "the environment's own wait time applies")
	assert.Equal(t, int32(6), *checks.Readiness.FailureThreshold, "unset values fall back to the defaults")
}

func TestGetEnvHealthChecks_BuildTimeWithoutEnvironment(t *testing.T) {
	c := newTestClient(t, healthCheckAPI(t, "dev"))

	checks, err := c.GetEnvHealthChecks(context.Background(), "acme", "my-agent", "")

	require.NoError(t, err)
	require.NotNil(t, checks)
	assert.False(t, *checks.Liveness.Enabled, "without an environment, the agent's current settings apply")
	assert.Equal(t, int32(80), *checks.Startup.FailureThreshold)
	assert.Equal(t, int32(5), *checks.Readiness.PeriodSeconds, "no environment's wait times apply")
}

func TestGetEnvHealthChecks_NothingDeployedInTheEnvironment(t *testing.T) {
	c := newTestClient(t, healthCheckAPI(t, "dev"))

	checks, err := c.GetEnvHealthChecks(context.Background(), "acme", "my-agent", "prod")

	require.NoError(t, err)
	assert.Nil(t, checks)
}

// concurrentWaitTimeAPI is healthCheckAPI whose "dev" binding, when read for the update,
// already holds a startup interval of 6 that someone else saved. writes records each
// binding the update sends.
func concurrentWaitTimeAPI(t *testing.T, writes *[]gen.ReleaseBinding) http.Handler {
	t.Helper()
	base := healthCheckAPI(t, "dev")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces/default/releasebindings/my-agent-dev" {
			base.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodPut {
			var sent gen.ReleaseBinding
			require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
			*writes = append(*writes, sent)
			require.NoError(t, json.NewEncoder(w).Encode(sent))
			return
		}
		require.NoError(t, json.NewEncoder(w).Encode(m{
			"metadata": m{"name": "my-agent-dev"},
			"spec": m{
				"environment":                     "dev",
				"releaseName":                     "my-agent-r1",
				"owner":                           m{"componentName": "my-agent", "projectName": "proj"},
				"componentTypeEnvironmentConfigs": m{probesKey: m{"startup": m{"periodSeconds": 6}}},
			},
		}))
	})
}

func TestReplaceReleaseBindingWorkloadOverrides_ChecksTheStartupWindowOnTheSavedState(t *testing.T) {
	failures := func(v int32) *HealthCheckTimings {
		return &HealthCheckTimings{Startup: &ProbeTimings{FailureThreshold: &v}}
	}

	t.Run("refuses wait times that only exceed the limit combined with a concurrent change", func(t *testing.T) {
		var writes []gen.ReleaseBinding
		c := newTestClient(t, concurrentWaitTimeAPI(t, &writes))

		// 10s + 700 x 6s (the interval saved concurrently) = 4210s.
		err := c.ReplaceReleaseBindingWorkloadOverrides(context.Background(), "acme", "my-agent", "dev", nil, nil, failures(700))

		require.Error(t, err)
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.Contains(t, err.Error(), "4210 seconds")
		assert.Empty(t, writes, "nothing is saved")
	})

	t.Run("saves wait times that stay within the limit combined with a concurrent change", func(t *testing.T) {
		var writes []gen.ReleaseBinding
		c := newTestClient(t, concurrentWaitTimeAPI(t, &writes))

		// 10s + 80 x 6s = 490s.
		err := c.ReplaceReleaseBindingWorkloadOverrides(context.Background(), "acme", "my-agent", "dev", nil, nil, failures(80))

		require.NoError(t, err)
		require.Len(t, writes, 1)
		saved := (*writes[0].Spec.ComponentTypeEnvironmentConfigs)[probesKey]
		assert.Equal(t, m{"startup": m{"periodSeconds": float64(6), "failureThreshold": float64(80)}}, saved,
			"the concurrent change is kept alongside ours")
	})
}
