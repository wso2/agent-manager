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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func a2aInput() A2AAgentDeploymentInput {
	return A2AAgentDeploymentInput{
		ArtifactName: "checkout-trip-planner-0192f4c19a7d7c3e",
		DisplayName:  "Trip Planner",
		AgentName:    "trip-planner",
		Vhost:        "agents.example.com",
		UpstreamURL:  "http://trip-planner.dp-default:9099",
		Policies: []map[string]interface{}{
			{"name": "cors", "version": "v1", "params": map[string]interface{}{"allowOrigins": []string{"*"}}},
			{"name": "api-key-auth", "version": "v1"},
		},
	}
}

// The emitted resource is a cross-repo contract with the gateway. A golden file
// is what makes a drift in it visible in a diff rather than at runtime.
func TestGenerateA2AAgentDeploymentYAMLMatchesGolden(t *testing.T) {
	got, err := generateA2AAgentDeploymentYAML(a2aInput())
	require.NoError(t, err)

	goldenPath := filepath.Join("testdata", "a2a_agent_golden.yaml")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0o600))
	}
	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	assert.Equal(t, string(want), got)
}

// Both transports, in this order, at these prefixes. M1 fixes them.
func TestBuildA2AAgentDeploymentYAMLTransports(t *testing.T) {
	got, err := buildA2AAgentDeploymentYAML(a2aInput())
	require.NoError(t, err)

	transports := got.Spec.A2A.OperationConfigs.Transports
	require.Len(t, transports, 2)
	assert.Equal(t, "JSONRPC", transports[0].ProtocolBinding)
	assert.Equal(t, "/rpc", transports[0].PathPrefix)
	assert.Equal(t, "HTTP+JSON", transports[1].ProtocolBinding)
	assert.Equal(t, "/rest", transports[1].PathPrefix)
	assert.Equal(t, "1.0", got.Spec.A2A.ProtocolVersion)
}

// The gateway's default for a passthrough card is to rewrite its URLs, and the
// Agent kind disables its route timeout so streaming operations survive. Both
// are what M1 wants, so neither block is written — emitting them is the only
// way to get them wrong.
func TestGenerateA2AAgentDeploymentYAMLOmitsCardAndResilience(t *testing.T) {
	got, err := generateA2AAgentDeploymentYAML(a2aInput())
	require.NoError(t, err)
	assert.NotContains(t, got, "agentCard")
	assert.NotContains(t, got, "resilience")
}

func TestBuildA2AAgentDeploymentYAMLContextIsAgentName(t *testing.T) {
	got, err := buildA2AAgentDeploymentYAML(a2aInput())
	require.NoError(t, err)
	assert.Equal(t, "/trip-planner", got.Spec.Context)
	assert.Equal(t, "v1.0", got.Spec.Version)
	assert.Equal(t, "Trip Planner", got.Spec.DisplayName)
	assert.Equal(t, "checkout-trip-planner-0192f4c19a7d7c3e", got.Metadata.Name)
}

// An empty upstream would produce an Agent that routes nowhere. Refuse rather
// than publish it — the reconciler retries until the binding reports one.
func TestBuildA2AAgentDeploymentYAMLRefusesEmptyUpstream(t *testing.T) {
	in := a2aInput()
	in.UpstreamURL = "   "
	_, err := buildA2AAgentDeploymentYAML(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upstream")
}

// vhost is optional: an environment whose gateway declares none must not emit
// an empty string, which the gateway would read as a host of "".
func TestBuildA2AAgentDeploymentYAMLOmitsEmptyVhost(t *testing.T) {
	in := a2aInput()
	in.Vhost = ""
	got, err := buildA2AAgentDeploymentYAML(in)
	require.NoError(t, err)
	assert.Nil(t, got.Spec.Vhost)

	out, err := generateA2AAgentDeploymentYAML(in)
	require.NoError(t, err)
	assert.NotContains(t, out, "vhost")
}

// The DB handle uses slashes, which are illegal in a Kubernetes name. The
// gateway resource name is derived with dashes and a compacted environment
// UUID, exactly as mcpProxyEnvArtifactHandle does.
func TestA2AAgentEnvArtifactName(t *testing.T) {
	name := a2aAgentEnvArtifactName("checkout", "trip-planner", "0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f")
	assert.Equal(t, "checkout-trip-planner-0192f4c19a7d7c3eb4f21a2b3c4d5e6f", name)
	assert.NotContains(t, name, "/")
}

// The gateway parses these values verbatim; unmarshalling the emitted document
// back is what proves the yaml tags are what the contract needs.
func TestGenerateA2AAgentDeploymentYAMLRoundTrips(t *testing.T) {
	out, err := generateA2AAgentDeploymentYAML(a2aInput())
	require.NoError(t, err)

	var back A2AAgentDeploymentYAML
	require.NoError(t, yaml.Unmarshal([]byte(out), &back))
	assert.Equal(t, apiVersionA2AAgent, back.ApiVersion)
	assert.Equal(t, kindA2AAgent, back.Kind)
	assert.Equal(t, "http://trip-planner.dp-default:9099", back.Spec.Upstream.URL)
	require.Len(t, back.Spec.A2A.OperationConfigs.Policies, 2)
	assert.Equal(t, "cors", back.Spec.A2A.OperationConfigs.Policies[0]["name"])
}
