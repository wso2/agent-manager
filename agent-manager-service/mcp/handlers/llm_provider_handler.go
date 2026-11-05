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
package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

type LLMProviderHandler struct {
	providerSvc   *services.LLMProviderService
	deploymentSvc *services.LLMProviderDeploymentService
}

func NewLLMProviderHandler(
	providerSvc *services.LLMProviderService,
	deploymentSvc *services.LLMProviderDeploymentService,
) *LLMProviderHandler {
	return &LLMProviderHandler{providerSvc: providerSvc, deploymentSvc: deploymentSvc}
}

func (h *LLMProviderHandler) ListLLMProviders(_ context.Context, ouID string, limit int, offset int) ([]*models.LLMProvider, int, error) {
	return h.providerSvc.List(ouID, limit, offset)
}

func (h *LLMProviderHandler) ListLLMGuardrails(ctx context.Context, ouID string, providerID string) (*models.LLMPolicyAvailabilityResponse, error) {
	return h.providerSvc.ListAvailableLLMPolicies(ctx, ouID, providerID)
}

// GetLLMProvider returns the provider and its gateway deployments, the two
// reads the console's provider page makes.
func (h *LLMProviderHandler) GetLLMProvider(_ context.Context, ouID string, providerID string) (*models.LLMProvider, []*models.Deployment, error) {
	provider, err := h.providerSvc.Get(providerID, ouID)
	if err != nil {
		return nil, nil, err
	}
	deployments, err := h.deploymentSvc.GetLLMProviderDeployments(providerID, ouID, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	return provider, deployments, nil
}

// UpdateLLMProviderGuardrails sets the provider's guardrails to change(its
// current ones) and redeploys it to every gateway it is currently deployed to,
// which is what makes the change reach live traffic. This is the console's
// guardrail save: it sends the provider back with its current gateway list, and
// UpdateAndSync redeploys to each gateway already in that list.
//
// The provider is read once, so the guardrails change is computed from the same
// record that is saved, and the stored provider is saved back as read: the
// upstream credential stays the encrypted reference it already is and nothing
// else changes. An error from change is returned as is, before anything is saved.
func (h *LLMProviderHandler) UpdateLLMProviderGuardrails(
	ctx context.Context, ouID string, providerID string,
	change func([]models.LLMPolicy) ([]models.LLMPolicy, error),
) (*models.LLMProvider, []services.DeploymentResult, error) {
	provider, err := h.providerSvc.Get(providerID, ouID)
	if err != nil {
		return nil, nil, err
	}
	if provider == nil {
		return nil, nil, errors.New("provider service returned an empty provider")
	}
	guardrails, err := change(provider.Configuration.Policies)
	if err != nil {
		return nil, nil, err
	}
	provider.Configuration.Policies = guardrails
	// Get resolves a handle or a UUID, but the update path keys on the UUID
	// alone, so use the one the lookup resolved to from here on.
	providerID = provider.UUID.String()

	// The gateways the provider is placed on, read the way UpdateAndSync reads
	// them: it undeploys every placed gateway missing from the list, so sending
	// this list back unchanged redeploys to each and undeploys none, including a
	// gateway still mid-rollout. With nothing placed it just saves the provider.
	gateways, err := h.providerSvc.GetProviderGatewayMapping(ctx, provider.UUID, ouID, h.deploymentSvc)
	if err != nil {
		return nil, nil, err
	}
	resp, err := h.providerSvc.UpdateAndSync(ctx, providerID, ouID, provider, gateways, h.deploymentSvc)
	if errors.Is(err, services.ErrAllGatewayDeploymentsFailed) {
		// The guardrails were saved before the redeploys failed: say so, rather than read as a save that failed.
		return nil, nil, fmt.Errorf("guardrails saved, but redeploying to every gateway failed, so the gateways "+
			"the provider is deployed to still enforce the previous guardrails: %w", err)
	}
	if err != nil {
		return nil, nil, err
	}
	return resp.Provider, resp.Deployments, nil
}
