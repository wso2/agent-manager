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

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func TestValidateHealthChecks(t *testing.T) {
	stringOf := func(s string) *string { return &s }
	numberOf := func(n int32) *int32 { return &n }

	tests := []struct {
		name         string
		healthChecks *spec.HealthChecks
		wantErr      bool
	}{
		{name: "not sent", healthChecks: nil},
		{name: "empty, meaning back to the defaults", healthChecks: &spec.HealthChecks{}},
		{name: "readiness HTTP with a path", healthChecks: &spec.HealthChecks{
			Readiness: &spec.HealthCheck{Type: stringOf("HTTP"), Path: stringOf("/health")},
		}},
		{name: "every field set to its lowest value", healthChecks: &spec.HealthChecks{
			Liveness: &spec.HealthCheck{
				Type: stringOf("TCP"), Port: numberOf(1), InitialDelaySeconds: numberOf(0),
				PeriodSeconds: numberOf(1), TimeoutSeconds: numberOf(1), FailureThreshold: numberOf(1),
			},
		}},
		{name: "a check switched off", healthChecks: &spec.HealthChecks{
			Startup: &spec.HealthCheck{Enabled: new(bool)},
		}},
		{name: "unknown type", wantErr: true, healthChecks: &spec.HealthChecks{
			Readiness: &spec.HealthCheck{Type: stringOf("GRPC")},
		}},
		{name: "path on a TCP check", wantErr: true, healthChecks: &spec.HealthChecks{
			Readiness: &spec.HealthCheck{Type: stringOf("TCP"), Path: stringOf("/health")},
		}},
		{name: "path on a check whose type defaults to TCP", wantErr: true, healthChecks: &spec.HealthChecks{
			Readiness: &spec.HealthCheck{Path: stringOf("/health")},
		}},
		{name: "path without a leading slash", wantErr: true, healthChecks: &spec.HealthChecks{
			Readiness: &spec.HealthCheck{Type: stringOf("HTTP"), Path: stringOf("health")},
		}},
		{name: "path leaving its directory", wantErr: true, healthChecks: &spec.HealthChecks{
			Readiness: &spec.HealthCheck{Type: stringOf("HTTP"), Path: stringOf("/a/../b")},
		}},
		{name: "port 0", wantErr: true, healthChecks: &spec.HealthChecks{
			Startup: &spec.HealthCheck{Port: numberOf(0)},
		}},
		{name: "port above range", wantErr: true, healthChecks: &spec.HealthChecks{
			Startup: &spec.HealthCheck{Port: numberOf(70000)},
		}},
		{name: "negative initial delay", wantErr: true, healthChecks: &spec.HealthChecks{
			Liveness: &spec.HealthCheck{InitialDelaySeconds: numberOf(-1)},
		}},
		{name: "interval 0", wantErr: true, healthChecks: &spec.HealthChecks{
			Liveness: &spec.HealthCheck{PeriodSeconds: numberOf(0)},
		}},
		{name: "timeout 0", wantErr: true, healthChecks: &spec.HealthChecks{
			Liveness: &spec.HealthCheck{TimeoutSeconds: numberOf(0)},
		}},
		{name: "failures allowed 0", wantErr: true, healthChecks: &spec.HealthChecks{
			Liveness: &spec.HealthCheck{FailureThreshold: numberOf(0)},
		}},
		// Startup may take at most an hour: initial delay + interval × failures allowed.
		{name: "startup taking exactly an hour", healthChecks: &spec.HealthChecks{
			Startup: &spec.HealthCheck{InitialDelaySeconds: numberOf(0), FailureThreshold: numberOf(720)},
		}},
		{name: "startup over an hour with the default delay and interval", wantErr: true, healthChecks: &spec.HealthChecks{
			Startup: &spec.HealthCheck{FailureThreshold: numberOf(720)},
		}},
		{name: "startup failures allowed large enough to overflow", wantErr: true, healthChecks: &spec.HealthChecks{
			Startup: &spec.HealthCheck{FailureThreshold: numberOf(2147483647)},
		}},
		// Readiness and liveness run as long as the agent does, so they have no total.
		{name: "readiness with many failures allowed", healthChecks: &spec.HealthChecks{
			Readiness: &spec.HealthCheck{FailureThreshold: numberOf(100000)},
		}},
		{name: "interval over an hour", wantErr: true, healthChecks: &spec.HealthChecks{
			Readiness: &spec.HealthCheck{PeriodSeconds: numberOf(3601)},
		}},
		{name: "initial delay over an hour", wantErr: true, healthChecks: &spec.HealthChecks{
			Liveness: &spec.HealthCheck{InitialDelaySeconds: numberOf(3601)},
		}},
		{name: "timeout over an hour", wantErr: true, healthChecks: &spec.HealthChecks{
			Liveness: &spec.HealthCheck{PeriodSeconds: numberOf(3600), TimeoutSeconds: numberOf(3601)},
		}},
		{name: "timeout longer than the default interval", wantErr: true, healthChecks: &spec.HealthChecks{
			Readiness: &spec.HealthCheck{TimeoutSeconds: numberOf(10)},
		}},
		{name: "timeout equal to the interval", healthChecks: &spec.HealthChecks{
			Readiness: &spec.HealthCheck{TimeoutSeconds: numberOf(5), PeriodSeconds: numberOf(5)},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHealthChecks(tt.healthChecks)
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.NotNil(t, IsValidationError(err), "should be reported to the caller as a validation error")
		})
	}
}

// Both requests that save health checks must reject bad ones before anything is saved.
// Each request is valid without the health checks, so the error can only come from them.
func TestPayloadValidatorsCheckHealthChecks(t *testing.T) {
	badType := "GRPC"
	badHealthChecks := &spec.HealthChecks{Readiness: &spec.HealthCheck{Type: &badType}}

	createRequest := spec.CreateAgentRequest{
		Name:         "support-bot",
		DisplayName:  "Support Bot",
		Provisioning: spec.Provisioning{Type: string(ExternalAgent)},
		AgentType:    &spec.AgentType{Type: string(AgentTypeExternalAPI)},
	}
	require.NoError(t, ValidateAgentCreatePayload(createRequest))
	createRequest.HealthChecks = badHealthChecks
	createErr := ValidateAgentCreatePayload(createRequest)
	require.Error(t, createErr)
	assert.Contains(t, createErr.Error(), "TCP or HTTP")

	subType := string(AgentSubTypeA2A)
	port := int32(9099)
	updateRequest := spec.UpdateAgentBuildParametersRequest{
		Provisioning: spec.Provisioning{
			Type: string(InternalAgent),
			Repository: &spec.RepositoryConfig{
				Url:     "https://github.com/wso2/agent-manager",
				Branch:  "main",
				AppPath: "/",
			},
		},
		AgentType: spec.AgentType{Type: string(AgentTypeAPI), SubType: &subType},
		Build: spec.DockerBuildAsBuild(&spec.DockerBuild{
			Type:   "docker",
			Docker: spec.DockerConfig{DockerfilePath: "/Dockerfile"},
		}),
		InputInterface: spec.InputInterface{Type: "HTTP", Port: &port},
	}
	require.NoError(t, ValidateAgentBuildParametersUpdatePayload(updateRequest))
	updateRequest.HealthChecks = badHealthChecks
	updateErr := ValidateAgentBuildParametersUpdatePayload(updateRequest)
	require.Error(t, updateErr)
	assert.Contains(t, updateErr.Error(), "TCP or HTTP")
}
