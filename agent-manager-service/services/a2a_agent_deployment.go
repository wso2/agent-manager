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
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

const (
	apiVersionA2AAgent = "gateway.api-platform.wso2.com/v1"
	kindA2AAgent       = "Agent"

	// a2aProtocolVersion is the single A2A version an M1 agent exposes. The
	// gateway performs no version conversion, so this is a platform constant
	// rather than a per-agent setting.
	a2aProtocolVersion = "1.0"

	// a2aAgentVersion mirrors what the api-configuration trait sets for a REST
	// agent, so A2A and REST agents keep consistent artifact versions.
	a2aAgentVersion = "v1.0"

	// The two bindings the gateway supports, at platform-chosen prefixes. There
	// is no gRPC binding. M1 exposes both and makes neither configurable.
	a2aTransportJSONRPC  = "JSONRPC"
	a2aTransportHTTPJSON = "HTTP+JSON"
	a2aPathPrefixJSONRPC = "/rpc"
	a2aPathPrefixHTTPRPC = "/rest"
)

// A2AAgentDeploymentYAML is the kind: Agent resource agent-manager publishes to
// the gateway. It is a structural sibling of MCPProxyDeploymentYAML.
//
// Two blocks are deliberately absent and must stay absent in M1:
//
//   - agentCard. The gateway's default for a proxied card is passthrough WITH
//     URL rewriting, which is exactly what M1 wants — an un-rewritten card
//     advertises the agent's own address and sends every client past the
//     gateway. Writing the block out would only add a way to get it wrong.
//   - resilience. The gateway disables the Agent route's request timeout by
//     default precisely because A2A streaming operations are long-lived.
//     Writing agent-manager's resilienceTimeoutSeconds here would override that
//     and sever every stream at the timeout.
type A2AAgentDeploymentYAML struct {
	ApiVersion string                 `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                 `yaml:"kind" json:"kind"`
	Metadata   DeploymentMetadata     `yaml:"metadata" json:"metadata"`
	Spec       A2AAgentDeploymentSpec `yaml:"spec" json:"spec"`
}

// A2AAgentDeploymentSpec is the spec section of the Agent resource.
type A2AAgentDeploymentSpec struct {
	DisplayName string      `yaml:"displayName" json:"displayName"`
	Version     string      `yaml:"version" json:"version"`
	Context     string      `yaml:"context" json:"context"`
	Vhost       *string     `yaml:"vhost,omitempty" json:"vhost,omitempty"`
	Upstream    A2AUpstream `yaml:"upstream" json:"upstream"`
	A2A         A2AConfig   `yaml:"a2a" json:"a2a"`
}

// A2AUpstream is the address the gateway dials to reach the agent.
type A2AUpstream struct {
	URL string `yaml:"url" json:"url"`
}

// A2AConfig carries the protocol version and the per-operation configuration.
type A2AConfig struct {
	ProtocolVersion  string              `yaml:"protocolVersion" json:"protocolVersion"`
	OperationConfigs A2AOperationConfigs `yaml:"operationConfigs" json:"operationConfigs"`
}

// A2AOperationConfigs holds the transport bindings and the agent-wide policy
// chain. Per-operation policy overrides exist in the gateway schema but are out
// of scope for M1.
type A2AOperationConfigs struct {
	Transports []A2ATransport           `yaml:"transports" json:"transports"`
	Policies   []map[string]interface{} `yaml:"policies" json:"policies"`
}

// A2ATransport binds one A2A protocol to a gateway-facing path prefix.
type A2ATransport struct {
	ProtocolBinding string `yaml:"protocolBinding" json:"protocolBinding"`
	PathPrefix      string `yaml:"pathPrefix" json:"pathPrefix"`
}

// A2AAgentDeploymentInput is everything the builder needs, already resolved.
// Keeping resolution out of the builder is what lets the emitted contract be
// tested as a pure function against a golden file.
type A2AAgentDeploymentInput struct {
	// ArtifactName is the sanitized per-environment resource name; see
	// a2aAgentEnvArtifactName.
	ArtifactName string
	DisplayName  string
	// AgentName is the component name, which becomes the URL context.
	AgentName string
	// Vhost is the environment's gateway vhost. Empty means "omit".
	Vhost string
	// UpstreamURL comes from the release binding's status. Empty is an error.
	UpstreamURL string
	// Policies is the output of buildPolicies — already in the gateway's
	// {name, version, params} shape, so no translation layer is needed.
	Policies []map[string]interface{}
}

// a2aAgentEnvArtifactName builds the per-environment Kubernetes resource name
// for an agent's Agent resource.
//
// It cannot reuse agentEnvAPIArtifactHandle: that builds "project/agent/envID"
// with slashes, which is fine as a database handle and illegal as a Kubernetes
// name. The shape here follows mcpProxyEnvArtifactHandle — dashes, with the
// environment UUID's hyphens stripped — so the two publication paths name
// resources the same way.
func a2aAgentEnvArtifactName(projectName, agentName, envID string) string {
	suffix := strings.ReplaceAll(strings.TrimSpace(envID), "-", "")
	return fmt.Sprintf("%s-%s-%s", strings.TrimSpace(projectName), strings.TrimSpace(agentName), suffix)
}

// generateA2AAgentDeploymentYAML renders the Agent resource as YAML.
func generateA2AAgentDeploymentYAML(in A2AAgentDeploymentInput) (string, error) {
	deployment, err := buildA2AAgentDeploymentYAML(in)
	if err != nil {
		return "", err
	}
	yamlBytes, err := yaml.Marshal(deployment)
	if err != nil {
		return "", fmt.Errorf("failed to marshal A2A agent deployment YAML: %w", err)
	}
	return string(yamlBytes), nil
}

// buildA2AAgentDeploymentYAML assembles the Agent resource.
//
// An empty upstream is refused rather than emitted: the release binding
// publishes its ServiceURL only once it has reconciled, and an Agent published
// before then would route nowhere while looking healthy. The publication
// reconciler retries instead.
func buildA2AAgentDeploymentYAML(in A2AAgentDeploymentInput) (*A2AAgentDeploymentYAML, error) {
	upstreamURL := strings.TrimSpace(in.UpstreamURL)
	if upstreamURL == "" {
		return nil, fmt.Errorf("refusing to publish agent %q: upstream url is not available yet", in.ArtifactName)
	}

	var vhost *string
	if trimmed := strings.TrimSpace(in.Vhost); trimmed != "" {
		vhost = &trimmed
	}

	// Non-nil so "no authentication and no CORS" marshals to an empty array
	// rather than null, matching what buildPolicies guarantees for the trait.
	policies := in.Policies
	if policies == nil {
		policies = []map[string]interface{}{}
	}

	return &A2AAgentDeploymentYAML{
		ApiVersion: apiVersionA2AAgent,
		Kind:       kindA2AAgent,
		Metadata:   DeploymentMetadata{Name: in.ArtifactName},
		Spec: A2AAgentDeploymentSpec{
			DisplayName: in.DisplayName,
			Version:     a2aAgentVersion,
			// Matches what the api-configuration trait sets for a REST agent
			// (buildAPIConfigurationTraitParameters), so A2A and REST agents
			// keep consistent URL shapes.
			Context:  "/" + strings.TrimPrefix(strings.TrimSpace(in.AgentName), "/"),
			Vhost:    vhost,
			Upstream: A2AUpstream{URL: upstreamURL},
			A2A: A2AConfig{
				ProtocolVersion: a2aProtocolVersion,
				OperationConfigs: A2AOperationConfigs{
					Transports: []A2ATransport{
						{ProtocolBinding: a2aTransportJSONRPC, PathPrefix: a2aPathPrefixJSONRPC},
						{ProtocolBinding: a2aTransportHTTPJSON, PathPrefix: a2aPathPrefixHTTPRPC},
					},
					Policies: policies,
				},
			},
		},
	}, nil
}

// broadcastA2AAgentDeletion tells every gateway that could be holding this
// Agent to drop it.
//
// The recipient set is the union of the gateways the artifact has deployment
// rows for and every active gateway in the org — the same union
// gatewayIDsForDeletion builds for MCP proxies, and for the same reason: a
// gateway left holding a deleted agent keeps routing to a workload that is
// gone, so a redundant delete is much cheaper than a missed one.
//
// Best effort. Deletion of the agent itself has already happened by the time
// this runs; failing it here would leave the caller unable to complete a delete
// it cannot undo.
func broadcastA2AAgentDeletion(
	ctx context.Context,
	events *GatewayEventsService,
	deploymentRepo repositories.DeploymentRepository,
	gatewayRepo repositories.GatewayRepository,
	artifactUUID uuid.UUID,
	ouID string,
	logger *slog.Logger,
) {
	_ = ctx
	if events == nil || artifactUUID == uuid.Nil {
		return
	}

	gatewayIDs := map[string]struct{}{}
	if deploymentRepo != nil {
		deployed, err := deploymentRepo.GetDeployedGatewaysByProvider(artifactUUID, ouID)
		if err != nil {
			logger.Warn("Failed to list deployed gateways for A2A agent deletion",
				"artifactID", artifactUUID, "error", err)
		}
		for _, id := range deployed {
			if strings.TrimSpace(id) != "" {
				gatewayIDs[id] = struct{}{}
			}
		}
	}
	if gatewayRepo != nil {
		active := true
		gateways, err := gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
			OrganizationID: ouID,
			Status:         &active,
		})
		if err != nil {
			logger.Warn("Failed to list active gateways for A2A agent deletion",
				"artifactID", artifactUUID, "error", err)
		}
		for _, gw := range gateways {
			if gw != nil {
				gatewayIDs[gw.UUID.String()] = struct{}{}
			}
		}
	}

	event := &models.AgentDeletionEvent{AgentID: artifactUUID.String()}
	for gatewayID := range gatewayIDs {
		if err := events.BroadcastAgentDeletionEvent(gatewayID, event); err != nil {
			logger.Warn("Failed to broadcast A2A agent deletion event",
				"artifactID", artifactUUID, "gatewayID", gatewayID, "error", err)
		}
	}
}
