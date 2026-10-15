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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// healthCheckInt32 returns a pointer to v.
func healthCheckInt32(v int32) *int32 { return &v }

func TestValidateHealthChecks(t *testing.T) {
	str := func(v string) *string { return &v }
	tests := []struct {
		name    string
		checks  *spec.AgentHealthChecks
		wantErr string
	}{
		{
			name:   "nothing sent",
			checks: nil,
		},
		{
			name: "HTTP readiness check on a given port",
			checks: &spec.AgentHealthChecks{Readiness: &spec.AgentHealthCheck{
				Enabled: spec.PtrBool(true), Type: str("http"), Path: str("/health"), Port: healthCheckInt32(8000),
			}},
		},
		{
			name:    "unknown check type",
			checks:  &spec.AgentHealthChecks{Readiness: &spec.AgentHealthCheck{Type: str("grpc")}},
			wantErr: "readiness type must be tcp or http",
		},
		{
			name:    "port out of range",
			checks:  &spec.AgentHealthChecks{Liveness: &spec.AgentHealthCheck{Port: healthCheckInt32(70000)}},
			wantErr: "liveness port must be between 1 and 65535",
		},
		{
			name:    "path without a leading slash",
			checks:  &spec.AgentHealthChecks{Readiness: &spec.AgentHealthCheck{Type: str("http"), Path: str("health")}},
			wantErr: "readiness path must start with /",
		},
		{
			name:    "wait time out of range",
			checks:  &spec.AgentHealthChecks{Startup: &spec.AgentHealthCheck{TimeoutSeconds: healthCheckInt32(0)}},
			wantErr: "startup timeoutSeconds must be between 1 and 3600",
		},
		{
			name:    "path with a space",
			checks:  &spec.AgentHealthChecks{Readiness: &spec.AgentHealthCheck{Type: str("http"), Path: str("/he alth")}},
			wantErr: "readiness path must not contain spaces",
		},
		{
			name:    "path too long",
			checks:  &spec.AgentHealthChecks{Liveness: &spec.AgentHealthCheck{Path: str("/" + strings.Repeat("a", 1024))}},
			wantErr: "liveness path must be at most 1024 characters",
		},
		{
			// The startup limit needs the defaults, so the service checks it.
			name:   "startup wait times sent in part",
			checks: &spec.AgentHealthChecks{Startup: &spec.AgentHealthCheck{FailureThreshold: healthCheckInt32(80)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHealthChecks(tt.checks)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateStartupWindow(t *testing.T) {
	tests := []struct {
		name                             string
		delay, period, timeout, failures int32
		wantErr                          string
	}{
		// The defaults: 10s + 60 x 5s = 310s.
		{name: "the defaults", delay: 10, period: 5, timeout: 1, failures: 60},
		// The issue's example: failures allowed raised to 80, 10s + 80 x 5s = 410s.
		{name: "more failures allowed", delay: 10, period: 5, timeout: 1, failures: 80},
		{name: "exactly an hour", delay: 0, period: 10, timeout: 1, failures: 360},
		{
			name: "over an hour", delay: 10, period: 10, timeout: 1, failures: 400,
			wantErr: "startup check would wait up to 4010 seconds",
		},
		{
			// 10s + 60 x 100s (the timeout, longer than the 5s interval) = 6010s.
			name: "over an hour because of a long timeout", delay: 10, period: 5, timeout: 100, failures: 60,
			wantErr: "startup check would wait up to 6010 seconds",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStartupWindow(tt.delay, tt.period, tt.timeout, tt.failures)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
