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

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// createdBySystem matches the literal the REST controllers record for
// org-level LLM providers and MCP proxies.
const createdBySystem = "system"

type LLMProviderHandler struct {
	templateSvc   *services.LLMProviderTemplateService
	providerSvc   *services.LLMProviderService
	deploymentSvc *services.LLMProviderDeploymentService
}

func NewLLMProviderHandler(
	templateSvc *services.LLMProviderTemplateService,
	providerSvc *services.LLMProviderService,
	deploymentSvc *services.LLMProviderDeploymentService,
) *LLMProviderHandler {
	return &LLMProviderHandler{templateSvc: templateSvc, providerSvc: providerSvc, deploymentSvc: deploymentSvc}
}

func (h *LLMProviderHandler) ListLLMProviderTemplates(_ context.Context, ouID string, limit int, offset int) ([]*models.LLMProviderTemplate, int, error) {
	return h.templateSvc.List(ouID, limit, offset)
}

func (h *LLMProviderHandler) ListLLMProviders(_ context.Context, ouID string, limit int, offset int) ([]*models.LLMProvider, int, error) {
	return h.providerSvc.List(ouID, limit, offset)
}

func (h *LLMProviderHandler) ListLLMGuardrails(ctx context.Context, ouID string, providerID string) (*models.LLMPolicyAvailabilityResponse, error) {
	return h.providerSvc.ListAvailableLLMPolicies(ctx, ouID, providerID)
}

// CreateLLMProvider mirrors the REST controller: with gateways it creates and
// deploys in one step, otherwise it only creates.
func (h *LLMProviderHandler) CreateLLMProvider(ctx context.Context, ouID string, req *spec.CreateLLMProviderRequest) (*models.LLMProvider, []services.DeploymentResult, error) {
	provider := utils.ConvertSpecToModelLLMProvider(req, ouID)
	if len(req.Gateways) == 0 {
		created, err := h.providerSvc.Create(ctx, ouID, createdBySystem, provider)
		return created, nil, err
	}
	resp, err := h.providerSvc.CreateAndDeploy(ctx, ouID, createdBySystem, provider, req.Gateways, h.deploymentSvc)
	if err != nil {
		return nil, nil, err
	}
	return resp.Provider, resp.Deployments, nil
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
