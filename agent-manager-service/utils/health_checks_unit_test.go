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

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func TestValidateHealthChecks(t *testing.T) {
	stringOf := func(s string) *string { return &s }
	numberOf := func(n int32) *int32 { return &n }
	boolOf := func(b bool) *bool { return &b }

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
		{name: "HTTP check without a path", wantErr: true, healthChecks: &spec.HealthChecks{
			Readiness: &spec.HealthCheck{Type: stringOf("HTTP")},
		}},
		// A check switched off never runs, so it needs no path.
		{name: "switched-off HTTP check without a path", healthChecks: &spec.HealthChecks{
			Liveness: &spec.HealthCheck{Type: stringOf("HTTP"), Enabled: boolOf(false)},
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
		// Readiness and liveness act after interval × failures allowed, at most an hour; the
		// initial delay does not count, as they keep running after it.
		{name: "readiness acting after exactly an hour", healthChecks: &spec.HealthChecks{
			Readiness: &spec.HealthCheck{InitialDelaySeconds: numberOf(600), FailureThreshold: numberOf(720)},
		}},
		{name: "readiness acting after more than an hour", wantErr: true, healthChecks: &spec.HealthChecks{
			Readiness: &spec.HealthCheck{FailureThreshold: numberOf(721)},
		}},
		{name: "liveness failures allowed large enough to overflow", wantErr: true, healthChecks: &spec.HealthChecks{
			Liveness: &spec.HealthCheck{FailureThreshold: numberOf(2147483647)},
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

	subType := string(AgentSubTypeA2A)
	port := int32(9099)
	provisioning := spec.Provisioning{
		Type: string(InternalAgent),
		Repository: &spec.RepositoryConfig{
			Url:     "https://github.com/wso2/agent-manager",
			Branch:  "main",
			AppPath: "/",
		},
	}
	agentType := spec.AgentType{Type: string(AgentTypeAPI), SubType: &subType}
	build := spec.DockerBuildAsBuild(&spec.DockerBuild{
		Type:   "docker",
		Docker: spec.DockerConfig{DockerfilePath: "/Dockerfile"},
	})
	inputInterface := spec.InputInterface{Type: "HTTP", Port: &port}

	createRequest := spec.CreateAgentRequest{
		Name:           "support-bot",
		DisplayName:    "Support Bot",
		Provisioning:   provisioning,
		AgentType:      &agentType,
		Build:          &build,
		InputInterface: &inputInterface,
	}
	require.NoError(t, ValidateAgentCreatePayload(createRequest))
	createRequest.HealthChecks = badHealthChecks
	createErr := ValidateAgentCreatePayload(createRequest)
	require.Error(t, createErr)
	assert.Contains(t, createErr.Error(), "TCP or HTTP")

	updateRequest := spec.UpdateAgentBuildParametersRequest{
		Provisioning:   provisioning,
		AgentType:      agentType,
		Build:          build,
		InputInterface: inputInterface,
	}
	require.NoError(t, ValidateAgentBuildParametersUpdatePayload(updateRequest))
	updateRequest.HealthChecks = badHealthChecks
	updateErr := ValidateAgentBuildParametersUpdatePayload(updateRequest)
	require.Error(t, updateErr)
	assert.Contains(t, updateErr.Error(), "TCP or HTTP")
}

// The platform does not run an external agent, and a catalog agent runs its version's build's
// checks, so health checks sent when creating either are rejected rather than dropped.
func TestCreateRejectsHealthChecksForExternalAndCatalogAgents(t *testing.T) {
	tcp := "TCP"
	healthChecks := &spec.HealthChecks{Readiness: &spec.HealthCheck{Type: &tcp}}

	externalRequest := spec.CreateAgentRequest{
		Name:         "support-bot",
		DisplayName:  "Support Bot",
		Provisioning: spec.Provisioning{Type: string(ExternalAgent)},
		AgentType:    &spec.AgentType{Type: string(AgentTypeExternalAPI)},
	}
	require.NoError(t, ValidateAgentCreatePayload(externalRequest))
	externalRequest.HealthChecks = healthChecks
	assert.ErrorContains(t, ValidateAgentCreatePayload(externalRequest), "only allowed for internal agents built from a repository")

	catalogRequest := spec.CreateAgentRequest{
		Name:        "support-bot",
		DisplayName: "Support Bot",
		Provisioning: spec.Provisioning{
			Type:      string(InternalAgent),
			AgentKind: &spec.ProvisioningAgentKind{Name: "support-kind", Version: "1.0.0"},
		},
		HealthChecks: healthChecks,
	}
	assert.ErrorContains(t, ValidateAgentCreatePayload(catalogRequest), "only allowed for internal agents built from a repository")
}

// A build's own health checks are what it runs wherever it is deployed, so both build
// responses return them; a build that recorded none returns none.
func TestBuildResponsesReturnTheBuildsHealthChecks(t *testing.T) {
	path := "/health"
	httpCheck := "HTTP"
	build := models.BuildResponse{
		Name: "build-1",
		HealthChecks: &models.HealthChecks{
			Readiness: &models.HealthCheck{Type: &httpCheck, Path: &path},
		},
	}
	want := &spec.HealthChecks{Readiness: &spec.HealthCheck{Type: &httpCheck, Path: &path}}

	assert.Equal(t, want, ConvertToBuildResponse(&build).HealthChecks)
	assert.Equal(t, want, ConvertToBuildDetailsResponse(&models.BuildDetailsResponse{BuildResponse: build}).HealthChecks)

	build.HealthChecks = nil
	assert.Nil(t, ConvertToBuildResponse(&build).HealthChecks)
}
