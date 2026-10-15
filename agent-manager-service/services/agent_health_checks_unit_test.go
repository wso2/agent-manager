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

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
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

// healthCheckTestService is an agent service on oc, with no AgentID env vars to inject.
func healthCheckTestService(oc *clientmocks.OpenChoreoClientMock) *agentManagerService {
	injector := &agentIdentityInjectorStub{
		EnvVarsForEnvironmentFunc: func(context.Context, string, string, string, string) ([]client.EnvVar, error) {
			return nil, nil
		},
	}
	return &agentManagerService{ocClient: oc, agentIdentityInjection: injector, logger: discardLogger()}
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
