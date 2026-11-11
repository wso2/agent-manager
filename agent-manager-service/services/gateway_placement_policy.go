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

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// gatewayConsumer is who uses an artifact on a gateway. The zero value is the most restrictive.
type gatewayConsumer uint8

const (
	platformAgentConsumer gatewayConsumer = iota // runs
	externalAgentConsumer
	monitorConsumer
)

// placementPolicy is which gateways a consumer may use and how AMS hands it credentials.
type placementPolicy struct{ consumer gatewayConsumer }

var (
	platformAgentPolicy = placementPolicy{platformAgentConsumer}
	externalAgentPolicy = placementPolicy{externalAgentConsumer}
	monitorPolicy       = placementPolicy{monitorConsumer}
)

func placementPolicyFor(agent *models.AgentResponse) placementPolicy {
	if isExternalProvisioned(agent) {
		return externalAgentPolicy
	}
	return platformAgentPolicy
}

// external is true when AMS hands the agent its credentials instead of injecting them into an OpenChoreo workload.
func (p placementPolicy) external() bool { return p.consumer == externalAgentConsumer }

func (p placementPolicy) allowsSelfHosted() bool { return p.consumer == externalAgentConsumer }

// providerHosting is the hosting the consumer's LLM provider placement may have.
func (p placementPolicy) providerHosting() anchorHosting {
	if p.allowsSelfHosted() {
		return managedFirst
	}
	return managedOnly
}

// apiKeyPurpose is the purpose of the API key auto-generated for the consumer's LLM proxy. External agents
// manage this key themselves (view the masked key, regenerate or delete it from the console), so it is
// user-managed. The platform injects the key into everything else, so it stays console-managed.
func (p placementPolicy) apiKeyPurpose() int {
	if p.external() {
		return models.APIKeyPurposeUserManaged
	}
	return models.APIKeyPurposeConsoleManaged
}

// hostingRule is a set of rules a gateway breaks. Each site keeps its own precedence and message.
type hostingRule uint8

const (
	ruleManagedConsumer hostingRule = 1 << iota // a platform-hosted agent or monitor would use it
	ruleAgentIdentity                           // Agent Identity cannot be validated there
)

func (r hostingRule) has(x hostingRule) bool { return r&x != 0 }

// consumerRefuses is true when gw is self-hosted and the consumer may only use managed gateways.
func (p placementPolicy) consumerRefuses(gw *models.Gateway) bool {
	return gw != nil && !gw.IsManaged() && !p.allowsSelfHosted()
}

// identityRefuses is true when gw is self-hosted and security uses Agent Identity, which self-hosted gateways
// cannot validate.
func identityRefuses(gw *models.Gateway, security *models.SecurityConfig) bool {
	return gw != nil && !gw.IsManaged() && mcpIdentityEnabled(security)
}

// refusedBy is the rules gw breaks for an agent or monitor binding secured by security.
func (p placementPolicy) refusedBy(gw *models.Gateway, security *models.SecurityConfig) hostingRule {
	var r hostingRule
	if p.consumerRefuses(gw) {
		r |= ruleManagedConsumer
	}
	if identityRefuses(gw, security) {
		r |= ruleAgentIdentity
	}
	return r
}

// gatewayNeedsPublicUpstream is true when the platform runs gw, so an upstream it calls must not resolve to a
// private address (SSRF guard).
func gatewayNeedsPublicUpstream(gw *models.Gateway) bool { return gw.IsManaged() }

// ownGateway is the gateway the consumer's own LLM or MCP artifact is placed on in envUUID, whatever the placement's
// hosting, refused when the consumer may not use it rather than resolved again: another gateway would be one the
// artifact isn't on.
func (p placementPolicy) ownGateway(
	repo repositories.GatewayRepository, ouID string, envUUID uuid.UUID, placed []string,
) (*models.Gateway, error) {
	gateway, err := resolveEgressGatewayForArtifact(repo, ouID, envUUID, placed, nil)
	if err != nil {
		return nil, err
	}
	if p.consumerRefuses(gateway) {
		return nil, fmt.Errorf("%w: gateway %s (%s) in environment %s",
			errSelfHostedAnchor, gateway.Name, gateway.UUID, envUUID)
	}
	return gateway, nil
}

// providerAnchor is the gateway in envUUID among placed that the consumer's LLM provider placement may use, or nil.
func (p placementPolicy) providerAnchor(
	repo repositories.GatewayRepository, envUUID uuid.UUID, placed []string,
) (*models.Gateway, error) {
	return placedAnchor(repo, envUUID, placed, p.providerHosting())
}

// providerGateway is the gateway the LLM provider is placed on in envUUID that the consumer may use, or the
// environment's single managed egress gateway when there is none. requireProvider accepts the same anchors, so a
// refused anchor never falls through to inference and lands the proxy where the provider isn't.
func (p placementPolicy) providerGateway(
	ctx context.Context, deployments *LLMProxyDeploymentService, gateways repositories.GatewayRepository,
	ouID, providerUUIDStr string, envUUID uuid.UUID,
) (*models.Gateway, error) {
	var deployed []string
	if providerUUID, err := uuid.Parse(providerUUIDStr); err == nil {
		deployed, err = deployments.PlacedGateways(ctx, ouID, providerUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to list deployed gateways for provider %s: %w", providerUUIDStr, err)
		}
	}
	anchor, err := p.providerAnchor(gateways, envUUID, deployed)
	if err != nil {
		return nil, err
	}
	return resolveEgressGatewayFromAnchor(gateways, ouID, envUUID, anchor, nil)
}

// requireProvider is nil when the LLM provider is placed on a gateway mapped to envUUID that the consumer may use.
func (p placementPolicy) requireProvider(
	ctx context.Context, deployments *LLMProxyDeploymentService, gateways repositories.GatewayRepository,
	ouID string, providerUUID, envUUID uuid.UUID,
) error {
	placed, err := deployments.PlacedGateways(ctx, ouID, providerUUID)
	if err != nil {
		return fmt.Errorf("failed to list deployed gateways for provider %s: %w", providerUUID, err)
	}
	anchor, err := p.providerAnchor(gateways, envUUID, placed)
	if err != nil {
		return err
	}
	if anchor != nil {
		return nil
	}
	// Callers name the provider and environment; the message is returned to the user as is.
	if p.allowsSelfHosted() {
		return fmt.Errorf("%w: not deployed to a gateway in this environment; %s",
			utils.ErrInvalidInput, deployProviderHint)
	}
	return fmt.Errorf("%w: not deployed to a managed gateway in this environment (%s can only use LLM providers deployed to managed gateways); %s",
		utils.ErrInvalidInput, p.consumerNoun(), deployProviderHint)
}

const deployProviderHint = "deploy it from the LLM provider's Deployment tab"

// consumerNoun names the consumers the policy covers, for messages.
func (p placementPolicy) consumerNoun() string {
	switch p.consumer {
	case monitorConsumer:
		return "monitors"
	case externalAgentConsumer:
		return "external agents"
	default:
		return "platform-hosted agents"
	}
}
