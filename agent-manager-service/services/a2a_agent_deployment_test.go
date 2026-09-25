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

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func a2aInput() A2AAgentDeploymentInput {
	return A2AAgentDeploymentInput{
		ArtifactName: "checkout-trip-planner-0192f4c19a7d7c3e",
		DisplayName:  "Trip Planner",
		AgentName:    "trip-planner",
		UpstreamURL:  "http://trip-planner.dp-default:9099",
		Policies: []map[string]interface{}{
			client.CORSPolicy([]string{"*"}, []string{"GET", "POST", "OPTIONS"}, []string{"Content-Type", "A2A-Version"}, false),
			client.APIKeyAuthPolicy(),
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

// Without card policies there is nothing to say about the card: the gateway's
// passthrough-with-rewrite default is what every A2A agent wants.
func TestGenerateA2AAgentDeploymentYAMLOmitsCardWithoutPoliciesAndResilience(t *testing.T) {
	got, err := generateA2AAgentDeploymentYAML(a2aInput())
	require.NoError(t, err)
	assert.NotContains(t, got, "agentCard")
	assert.NotContains(t, got, "resilience")
}

// The card block carries only its policy list. Mode, path and rewriteUrls stay
// unset so the gateway's passthrough-with-rewrite default still applies.
func TestGenerateA2AAgentDeploymentYAMLWritesOnlyCardPolicies(t *testing.T) {
	in := a2aInput()
	in.CardPolicies = buildCardPolicies(models.CardCORS{Enabled: true, AllowOrigins: []string{"https://client.example"}, AllowHeaders: []string{"Content-Type"}})
	got, err := generateA2AAgentDeploymentYAML(in)
	require.NoError(t, err)

	goldenPath := filepath.Join("testdata", "a2a_agent_card_cors_golden.yaml")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0o600))
	}
	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	assert.Equal(t, string(want), got)

	for _, forbidden := range []string{"mode:", "rewriteUrls", "signing", "protected", "resilience"} {
		assert.NotContains(t, got, forbidden)
	}
}

func TestBuildCardPolicies(t *testing.T) {
	assert.Nil(t, buildCardPolicies(models.CardCORS{Enabled: false, AllowOrigins: []string{"*"}}), "disabled")
	assert.Nil(t, buildCardPolicies(models.CardCORS{Enabled: true}), "enabled without origins")

	got := buildCardPolicies(models.CardCORS{Enabled: true, AllowOrigins: []string{"*"}, AllowHeaders: []string{"Content-Type"}})
	require.Len(t, got, 1)
	assert.Equal(t, "cors", got[0]["name"])
	params := got[0]["params"].(map[string]interface{})
	assert.Equal(t, []string{"GET", "OPTIONS"}, params["allowedMethods"])
}

// Card CORS is independent of operation CORS: the card can be public while the
// operation routes allow no cross-origin callers at all.
func TestBuildCardPoliciesIndependentOfAgentCORS(t *testing.T) {
	cfg := &models.AgentConfig{CORSEnabled: false}
	cfg.SetCardCORSOverride(&models.CardCORS{Enabled: true, AllowOrigins: []string{"*"}, AllowHeaders: []string{"Content-Type"}})
	assert.Len(t, buildCardPolicies(cfg.EffectiveCardCORS()), 1)
	assert.Empty(t, buildPolicies(resolveAPIConfig(cfg, nil, nil, nil, nil, false)))
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
