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

// readinessHTTP is what the create wizard sends for an HTTP GET readiness check:
// the type and path, nothing else.
func readinessHTTP(path string) *HealthChecks {
	checkType := "http"
	return &HealthChecks{Readiness: &HealthCheck{Type: &checkType, Path: &path}}
}

func sourceAgentRequest(checks *HealthChecks) CreateComponentRequest {
	return CreateComponentRequest{
		Name:             "agent-1",
		DisplayName:      "Agent 1",
		ProvisioningType: "internal",
		AgentType:        AgentTypeConfig{Type: "agent-api", SubType: "a2a-agent"},
		Build:            &BuildConfig{Type: "buildpack", Buildpack: &BuildpackConfig{Language: "python", LanguageVersion: "3.11"}},
		InputInterface:   &InputInterfaceConfig{Type: "HTTP", Port: 9099},
		HealthChecks:     checks,
	}
}

// Only the fields sent are written, so every other field renders from the
// ComponentType's defaults.
func TestBuildInternalAgentFromSourceComponentRequestBody_WritesOnlyTheHealthChecksSent(t *testing.T) {
	body, err := buildInternalAgentFromSourceComponentRequestBody("ns", "proj",
		sourceAgentRequest(readinessHTTP("/.well-known/agent-card.json")))

	require.NoError(t, err)
	require.NotNil(t, body.Spec.Parameters)
	assert.Equal(t,
		m{"readiness": m{"type": "http", "path": "/.well-known/agent-card.json"}},
		(*body.Spec.Parameters)[probesKey])
}

// An agent created without health checks has no parameters.probes, so it gets the
// ComponentType's defaults, the same as agents created before they were configurable.
func TestBuildInternalAgentFromSourceComponentRequestBody_NoHealthChecks(t *testing.T) {
	body, err := buildInternalAgentFromSourceComponentRequestBody("ns", "proj", sourceAgentRequest(nil))

	require.NoError(t, err)
	require.NotNil(t, body.Spec.Parameters)
	assert.NotContains(t, *body.Spec.Parameters, probesKey)
}

// componentWithProbes is a Component whose agent has liveness on over HTTP.
const componentWithProbes = `{
  "metadata": {"name": "agent-1"},
  "spec": {
    "componentType": {"name": "internal-agent/agent-api"},
    "owner": {"projectName": "proj"},
    "parameters": {
      "exposed": true, "port": 8000,
      "probes": {"liveness": {"enabled": true, "type": "http", "path": "/live"}}
    },
    "workflow": {"parameters": {}}
  }
}`

// putParametersOf runs UpdateComponentBuildParameters against a Component with
// stored probes and returns the parameters it writes back.
func putParametersOf(t *testing.T, checks *HealthChecks) m {
	t.Helper()
	var putParameters m
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			var sent struct {
				Spec struct {
					Parameters m `json:"parameters"`
				} `json:"spec"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
			putParameters = sent.Spec.Parameters
		}
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(componentWithProbes))
		require.NoError(t, err)
	}))

	err := client.UpdateComponentBuildParameters(context.Background(), "acme", "proj", "agent-1",
		UpdateComponentBuildParametersRequest{
			AgentType:      AgentTypeConfig{Type: "agent-api", SubType: "chat-api"},
			InputInterface: &InputInterfaceConfig{Type: "HTTP"},
			HealthChecks:   checks,
		})
	require.NoError(t, err)
	require.NotNil(t, putParameters, "expected the update to PUT the component back")
	return putParameters
}

// The health checks sent replace the stored ones as a whole: liveness, not sent,
// goes back to the default.
func TestUpdateComponentBuildParameters_ReplacesTheHealthChecks(t *testing.T) {
	params := putParametersOf(t, readinessHTTP("/ready"))

	assert.Equal(t, m{"readiness": m{"type": "http", "path": "/ready"}}, params[probesKey])
}

// A build-parameters request without health checks keeps the stored ones.
func TestUpdateComponentBuildParameters_KeepsTheHealthChecksWhenNoneSent(t *testing.T) {
	params := putParametersOf(t, nil)

	assert.Equal(t,
		m{"liveness": m{"enabled": true, "type": "http", "path": "/live"}},
		params[probesKey])
}
