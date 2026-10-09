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

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// healthChecksInEffect is what a "dev" environment runs: the default startup
// window of 10s + 5s x 60, and readiness on.
func healthChecksInEffect() *client.HealthChecks {
	on := true
	return &client.HealthChecks{
		Startup: &client.HealthCheck{Enabled: &on, ProbeTimings: client.ProbeTimings{
			InitialDelaySeconds: int32Ptr(10), PeriodSeconds: int32Ptr(5),
			TimeoutSeconds: int32Ptr(1), FailureThreshold: int32Ptr(60),
		}},
		Readiness: &client.HealthCheck{Enabled: &on, ProbeTimings: client.ProbeTimings{
			InitialDelaySeconds: int32Ptr(0), PeriodSeconds: int32Ptr(5),
			TimeoutSeconds: int32Ptr(1), FailureThreshold: int32Ptr(6),
		}},
	}
}

// healthCheckConfigClient is an OpenChoreo client for UpdateAgentConfigurations on
// a platform-hosted agent in a non-production environment. The binding write is
// left unset, so a test that must not reach it panics if it does.
func healthCheckConfigClient(inEffect func() (*client.HealthChecks, error)) *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(utils.InternalAgent)}}, nil
		},
		GetEnvironmentFunc: nonProductionEnvStub(),
		GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
			return nil, nil
		},
		GetEnvHealthChecksFunc: func(context.Context, string, string, string) (*client.HealthChecks, error) {
			return inEffect()
		},
	}
}

// healthCheckTestService is an agent service on oc, with no AgentID env vars to inject.
func healthCheckTestService(oc *clientmocks.OpenChoreoClientMock) *agentManagerService {
	injector := &agentIdentityInjectorStub{
		EnvVarsForEnvironmentFunc: func(context.Context, string, string, string, string) ([]client.EnvVar, error) {
			return nil, nil
		},
	}
	return &agentManagerService{ocClient: oc, agentIdentityInjection: injector, logger: discardLogger()}
}

// updateWaitTimes sends timings as the "dev" environment's wait times.
func updateWaitTimes(t *testing.T, s *agentManagerService, timings *spec.AgentHealthCheckTimings) error {
	t.Helper()
	return s.UpdateAgentConfigurations(tierGrantedCtx(t), "acme", "proj1", "my-agent",
		&spec.UpdateAgentConfigurationsRequest{EnvironmentName: "dev", Probes: timings})
}

func TestUpdateAgentConfigurations_SavesHealthCheckWaitTimes(t *testing.T) {
	oc := healthCheckConfigClient(func() (*client.HealthChecks, error) { return healthChecksInEffect(), nil })
	var saved *client.HealthCheckTimings
	var savedEnv []client.EnvVar
	var savedFiles []client.FileVar
	oc.ReplaceReleaseBindingWorkloadOverridesFunc = func(_ context.Context, _, _, _ string, env []client.EnvVar, files []client.FileVar, probes *client.HealthCheckTimings) error {
		saved, savedEnv, savedFiles = probes, env, files
		return nil
	}

	err := updateWaitTimes(t, healthCheckTestService(oc), &spec.AgentHealthCheckTimings{
		Startup: &spec.AgentProbeTimings{FailureThreshold: int32Ptr(80)},
	})

	require.NoError(t, err)
	require.Len(t, oc.ReplaceReleaseBindingWorkloadOverridesCalls(), 1)
	require.NotNil(t, saved)
	require.NotNil(t, saved.Startup)
	assert.Equal(t, int32(80), *saved.Startup.FailureThreshold)
	assert.Nil(t, saved.Startup.PeriodSeconds, "only the wait times sent are passed on")
	assert.Nil(t, saved.Readiness)
	assert.Nil(t, savedEnv, "env vars not sent are left as they are")
	assert.Nil(t, savedFiles, "file mounts not sent are left as they are")
}

func TestUpdateAgentConfigurations_RejectsInvalidHealthCheckWaitTimes(t *testing.T) {
	tests := []struct {
		name    string
		timings *spec.AgentHealthCheckTimings
	}{
		{
			name:    "out of range",
			timings: &spec.AgentHealthCheckTimings{Readiness: &spec.AgentProbeTimings{PeriodSeconds: int32Ptr(0)}},
		},
		{
			// 10s (in effect) + 60s x 80 = 4810s, over the one-hour limit.
			name:    "startup window over an hour with the values in effect",
			timings: &spec.AgentHealthCheckTimings{Startup: &spec.AgentProbeTimings{PeriodSeconds: int32Ptr(60), FailureThreshold: int32Ptr(80)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oc := healthCheckConfigClient(func() (*client.HealthChecks, error) { return healthChecksInEffect(), nil })

			err := updateWaitTimes(t, healthCheckTestService(oc), tt.timings)

			require.Error(t, err)
			assert.ErrorIs(t, err, utils.ErrInvalidInput)
			assert.Empty(t, oc.ReplaceReleaseBindingWorkloadOverridesCalls(), "nothing is saved")
		})
	}
}

func TestUpdateAgentConfigurations_EmptyHealthCheckWaitTimesAreNotSaved(t *testing.T) {
	// An empty probes object asks for nothing, so it must not reach the binding
	// write (which would restart the agent).
	oc := healthCheckConfigClient(func() (*client.HealthChecks, error) { return healthChecksInEffect(), nil })

	err := updateWaitTimes(t, healthCheckTestService(oc), &spec.AgentHealthCheckTimings{Startup: &spec.AgentProbeTimings{}})

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "a request with nothing in it is the caller's mistake")
	assert.Empty(t, oc.GetEnvHealthChecksCalls(), "an empty probes object is treated as not sent")
	assert.Empty(t, oc.ReplaceReleaseBindingWorkloadOverridesCalls())
}

func TestUpdateAgentConfigurations_RejectsWaitTimesForAgentWithoutHealthChecks(t *testing.T) {
	// The agent's ComponentType defines no health checks, as for an external agent.
	oc := healthCheckConfigClient(func() (*client.HealthChecks, error) {
		return nil, nil //nolint:nilnil // no health checks, no error
	})

	err := updateWaitTimes(t, healthCheckTestService(oc), &spec.AgentHealthCheckTimings{
		Startup: &spec.AgentProbeTimings{FailureThreshold: int32Ptr(80)},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Empty(t, oc.ReplaceReleaseBindingWorkloadOverridesCalls())
}

func TestUpdateAgentConfigurations_HealthCheckReadFailureIsNotInvalidInput(t *testing.T) {
	boom := errors.New("openchoreo unavailable")
	oc := healthCheckConfigClient(func() (*client.HealthChecks, error) { return nil, boom })

	err := updateWaitTimes(t, healthCheckTestService(oc), &spec.AgentHealthCheckTimings{
		Startup: &spec.AgentProbeTimings{FailureThreshold: int32Ptr(80)},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, utils.ErrInvalidInput, "a failed read must not be reported as the caller's mistake")
	assert.Empty(t, oc.ReplaceReleaseBindingWorkloadOverridesCalls())
}

func TestGetAgentHealthChecks(t *testing.T) {
	t.Run("returns the health checks in effect", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvHealthChecksFunc: func(context.Context, string, string, string) (*client.HealthChecks, error) {
				return healthChecksInEffect(), nil
			},
		}

		checks, err := healthCheckTestService(oc).GetAgentHealthChecks(context.Background(), "acme", "proj1", "my-agent", "dev")

		require.NoError(t, err)
		require.NotNil(t, checks)
		assert.Equal(t, int32(60), *checks.Startup.FailureThreshold)
		assert.Nil(t, checks.Liveness)
	})

	t.Run("an agent without health checks gets none", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvHealthChecksFunc: func(context.Context, string, string, string) (*client.HealthChecks, error) {
				return nil, nil //nolint:nilnil // no health checks, no error
			},
		}

		checks, err := healthCheckTestService(oc).GetAgentHealthChecks(context.Background(), "acme", "proj1", "my-agent", "dev")

		require.NoError(t, err)
		assert.Nil(t, checks)
	})

	t.Run("a missing agent is reported as agent not found", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetEnvHealthChecksFunc: func(context.Context, string, string, string) (*client.HealthChecks, error) {
				return nil, utils.ErrNotFound
			},
		}

		_, err := healthCheckTestService(oc).GetAgentHealthChecks(context.Background(), "acme", "proj1", "my-agent", "")

		assert.ErrorIs(t, err, utils.ErrAgentNotFound)
	})
}
