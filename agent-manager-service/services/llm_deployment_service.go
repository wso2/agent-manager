/*
 *  Copyright (c) 2026, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package services

import (
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// LLMProviderDeploymentService handles LLM deployment business logic
type LLMProviderDeploymentService struct {
	deploymentRepo repositories.DeploymentRepository
	providerRepo   repositories.LLMProviderRepository
	templateRepo   repositories.LLMProviderTemplateRepository
	gatewayRepo    repositories.GatewayRepository
}

// NewLLMProviderDeploymentService creates a new LLM deployment service
func NewLLMProviderDeploymentService(
	deploymentRepo repositories.DeploymentRepository,
	providerRepo repositories.LLMProviderRepository,
	templateRepo repositories.LLMProviderTemplateRepository,
	gatewayRepo repositories.GatewayRepository,
) *LLMProviderDeploymentService {
	return &LLMProviderDeploymentService{
		deploymentRepo: deploymentRepo,
		providerRepo:   providerRepo,
		templateRepo:   templateRepo,
		gatewayRepo:    gatewayRepo,
	}
}

// DeployLLMProvider deploys an LLM provider to a gateway
// TODO: Implement using deploymentRepo.CreateWithLimitEnforcement()
func (s *LLMProviderDeploymentService) DeployLLMProvider(providerID string, req *models.DeployAPIRequest, orgID string) (*models.Deployment, error) {
	return nil, utils.ErrNotImplemented
}

// UndeployLLMProvider undeploys an LLM provider from a gateway
// TODO: Implement using deploymentRepo.SetCurrent() with status = Undeployed
func (s *LLMProviderDeploymentService) UndeployLLMProvider(providerID, orgID, gatewayID string, deploymentID string) error {
	return utils.ErrNotImplemented
}

// RestoreLLMProviderDeployment restores a previous deployment
// TODO: Implement using deploymentRepo.SetCurrent() with status = Deployed
func (s *LLMProviderDeploymentService) RestoreLLMProviderDeployment(providerID, orgID, gatewayID string, deploymentID string) (*models.Deployment, error) {
	return nil, utils.ErrNotImplemented
}

// GetLLMProviderDeployments retrieves all deployments for a provider
// TODO: Implement using deploymentRepo.GetDeploymentsWithState()
func (s *LLMProviderDeploymentService) GetLLMProviderDeployments(providerID, orgID, gatewayID string) ([]*models.Deployment, error) {
	return nil, utils.ErrNotImplemented
}

// GetLLMProviderDeployment retrieves a specific deployment
// TODO: Implement using deploymentRepo.GetWithState()
func (s *LLMProviderDeploymentService) GetLLMProviderDeployment(providerID, orgID, deploymentID string) (*models.Deployment, error) {
	return nil, utils.ErrNotImplemented
}

// DeleteLLMProviderDeployment deletes a deployment
// TODO: Implement using deploymentRepo.Delete()
func (s *LLMProviderDeploymentService) DeleteLLMProviderDeployment(providerID, orgID, deploymentID string) error {
	return utils.ErrNotImplemented
}

// UndeployLLMProviderDeployment undeploys a deployment (alias for UndeployLLMProvider)
func (s *LLMProviderDeploymentService) UndeployLLMProviderDeployment(providerID, orgID, gatewayID, deploymentID string) error {
	return s.UndeployLLMProvider(providerID, orgID, gatewayID, deploymentID)
}
