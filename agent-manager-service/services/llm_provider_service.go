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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

const (
	llmStatusPending = "pending"
)

// DeploymentResult captures the outcome of deploying to a single gateway
type DeploymentResult struct {
	GatewayID string `json:"gateway_id"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// CreateAndDeployResponse contains the created provider and deployment results
type CreateAndDeployResponse struct {
	Provider    *models.LLMProvider `json:"provider"`
	Deployments []DeploymentResult  `json:"deployments"`
}

// UpdateAndSyncResponse contains the updated provider and sync results
type UpdateAndSyncResponse struct {
	Provider      *models.LLMProvider `json:"provider"`
	Deployments   []DeploymentResult  `json:"deployments"`   // Results for new gateway deployments
	Undeployments []DeploymentResult  `json:"undeployments"` // Results for removed gateway undeployments
}

// LLMProviderService handles LLM provider business logic
type LLMProviderService struct {
	db           *gorm.DB
	providerRepo repositories.LLMProviderRepository
	templateRepo repositories.LLMProviderTemplateRepository
	proxyRepo    repositories.LLMProxyRepository
	mappingRepo  repositories.LLMProviderGatewayMappingRepository
}

// NewLLMProviderService creates a new LLM provider service
func NewLLMProviderService(
	db *gorm.DB,
	providerRepo repositories.LLMProviderRepository,
	templateRepo repositories.LLMProviderTemplateRepository,
	proxyRepo repositories.LLMProxyRepository,
	mappingRepo repositories.LLMProviderGatewayMappingRepository,
) *LLMProviderService {
	return &LLMProviderService{
		db:           db,
		providerRepo: providerRepo,
		templateRepo: templateRepo,
		proxyRepo:    proxyRepo,
		mappingRepo:  mappingRepo,
	}
}

// Create creates a new LLM provider
func (s *LLMProviderService) Create(orgID, createdBy string, provider *models.LLMProvider) (*models.LLMProvider, error) {
	slog.Info("LLMProviderService.Create: starting", "orgID", orgID, "createdBy", createdBy)

	if provider == nil {
		slog.Error("LLMProviderService.Create: provider is nil", "orgID", orgID)
		return nil, utils.ErrInvalidInput
	}

	// Extract handle, name, and version from configuration
	// Note: handle is not in Configuration, so we use name as handle
	name := provider.Configuration.Name
	version := provider.Configuration.Version

	// Use name as handle (artifact identifier)
	handle := name

	slog.Info("LLMProviderService.Create: extracted configuration", "orgID", orgID, "handle", handle, "name", name, "version", version)

	if handle == "" || name == "" || version == "" {
		slog.Error("LLMProviderService.Create: missing required fields", "orgID", orgID, "handle", handle, "name", name, "version", version)
		return nil, utils.ErrInvalidInput
	}

	// Validate template exists
	template := provider.Configuration.Template
	if template == "" {
		slog.Error("LLMProviderService.Create: template not specified", "orgID", orgID, "handle", handle)
		return nil, utils.ErrInvalidInput
	}

	// Parse organization UUID
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		slog.Error("LLMProviderService.Create: invalid organization UUID", "orgID", orgID, "error", err)
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}

	// Set default values
	provider.CreatedBy = createdBy
	provider.Status = llmStatusPending
	if provider.Configuration.Context == nil {
		defaultContext := "/"
		provider.Configuration.Context = &defaultContext
	}

	slog.Info("LLMProviderService.Create: set default values", "orgID", orgID, "handle", handle, "status", provider.Status, "context", *provider.Configuration.Context)

	// Serialize model providers to ModelList
	if len(provider.ModelProviders) > 0 {
		slog.Info("LLMProviderService.Create: serializing model providers", "orgID", orgID, "handle", handle, "count", len(provider.ModelProviders))
		modelListBytes, err := json.Marshal(provider.ModelProviders)
		if err != nil {
			slog.Error("LLMProviderService.Create: failed to serialize model providers", "orgID", orgID, "handle", handle, "error", err)
			return nil, fmt.Errorf("failed to serialize model providers: %w", err)
		}
		provider.ModelList = string(modelListBytes)
	}

	// Create provider in transaction with validation
	slog.Info("LLMProviderService.Create: creating provider in database", "orgID", orgID, "handle", handle, "name", name, "version", version)
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Validate template exists within transaction
		slog.Info("LLMProviderService.Create: validating template in transaction", "orgID", orgID, "handle", handle, "template", template)
		templateExists, err := s.templateRepo.Exists(template, orgID)
		if err != nil {
			slog.Error("LLMProviderService.Create: failed to validate template", "orgID", orgID, "handle", handle, "template", template, "error", err)
			return fmt.Errorf("failed to validate template: %w", err)
		}
		if !templateExists {
			slog.Warn("LLMProviderService.Create: template not found", "orgID", orgID, "handle", handle, "template", template)
			return utils.ErrLLMProviderTemplateNotFound
		}

		// Create provider - uniqueness enforced by DB constraint
		return s.providerRepo.Create(tx, provider, handle, name, version, orgUUID)
	})
	if err != nil {
		// Check for unique constraint violation
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			slog.Warn("LLMProviderService.Create: provider already exists (unique constraint)", "orgID", orgID, "handle", handle)
			return nil, utils.ErrLLMProviderExists
		}
		// Return template not found error directly
		if errors.Is(err, utils.ErrLLMProviderTemplateNotFound) {
			return nil, err
		}
		slog.Error("LLMProviderService.Create: failed to create provider", "orgID", orgID, "handle", handle, "error", err)
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	slog.Info("LLMProviderService.Create: provider created, fetching details", "orgID", orgID, "handle", handle, "uuid", provider.UUID)

	// Fetch created provider by UUID
	created, err := s.providerRepo.GetByUUID(provider.UUID.String(), orgID)
	if err != nil {
		slog.Error("LLMProviderService.Create: failed to fetch created provider", "orgID", orgID, "uuid", provider.UUID, "error", err)
		return nil, fmt.Errorf("failed to fetch created provider: %w", err)
	}

	// Parse model providers from ModelList
	if created.ModelList != "" {
		slog.Info("LLMProviderService.Create: parsing model providers from ModelList", "orgID", orgID, "handle", handle)
		if err := json.Unmarshal([]byte(created.ModelList), &created.ModelProviders); err != nil {
			slog.Error("LLMProviderService.Create: failed to parse model providers", "orgID", orgID, "handle", handle, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	slog.Info("LLMProviderService.Create: completed successfully", "orgID", orgID, "handle", handle, "providerUUID", created.UUID)
	return created, nil
}

// List lists all LLM providers for an organization
func (s *LLMProviderService) List(orgID string, limit, offset int) ([]*models.LLMProvider, int, error) {
	slog.Info("LLMProviderService.List: starting", "orgID", orgID, "limit", limit, "offset", offset)

	providers, err := s.providerRepo.List(orgID, limit, offset)
	if err != nil {
		slog.Error("LLMProviderService.List: failed to list providers", "orgID", orgID, "error", err)
		return nil, 0, fmt.Errorf("failed to list providers: %w", err)
	}

	slog.Info("LLMProviderService.List: providers retrieved from repository", "orgID", orgID, "count", len(providers))

	// Parse model providers for each provider
	for i, p := range providers {
		if p.ModelList != "" {
			if err := json.Unmarshal([]byte(p.ModelList), &p.ModelProviders); err != nil {
				slog.Error("LLMProviderService.List: failed to parse model providers", "orgID", orgID, "providerIndex", i, "providerUUID", p.UUID, "error", err)
				return nil, 0, fmt.Errorf("failed to parse model providers: %w", err)
			}
		}
	}

	totalCount, err := s.providerRepo.Count(orgID)
	if err != nil {
		slog.Error("LLMProviderService.List: failed to count providers", "orgID", orgID, "error", err)
		return nil, 0, fmt.Errorf("failed to count providers: %w", err)
	}

	slog.Info("LLMProviderService.List: completed successfully", "orgID", orgID, "count", len(providers), "total", totalCount)
	return providers, totalCount, nil
}

// Get retrieves an LLM provider by ID
func (s *LLMProviderService) Get(providerID, orgID string) (*models.LLMProvider, error) {
	slog.Info("LLMProviderService.Get: starting", "orgID", orgID, "providerID", providerID)

	if providerID == "" {
		slog.Error("LLMProviderService.Get: providerID is empty", "orgID", orgID)
		return nil, utils.ErrInvalidInput
	}

	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderService.Get: provider not found", "orgID", orgID, "providerID", providerID)
			return nil, utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Get: failed to get provider", "orgID", orgID, "providerID", providerID, "error", err)
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		slog.Warn("LLMProviderService.Get: provider not found", "orgID", orgID, "providerID", providerID)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Parse model providers from ModelList
	if provider.ModelList != "" {
		slog.Info("LLMProviderService.Get: parsing model providers", "orgID", orgID, "providerID", providerID, "providerUUID", provider.UUID)
		if err := json.Unmarshal([]byte(provider.ModelList), &provider.ModelProviders); err != nil {
			slog.Error("LLMProviderService.Get: failed to parse model providers", "orgID", orgID, "providerID", providerID, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	slog.Info("LLMProviderService.Get: completed successfully", "orgID", orgID, "providerID", providerID, "providerUUID", provider.UUID)
	return provider, nil
}

// Update updates an existing LLM provider
func (s *LLMProviderService) Update(providerID, orgID string, updates *models.LLMProvider) (*models.LLMProvider, error) {
	slog.Info("LLMProviderService.Update: starting", "orgID", orgID, "providerID", providerID)

	if providerID == "" || updates == nil {
		slog.Error("LLMProviderService.Update: invalid input", "orgID", orgID, "providerID", providerID, "updatesIsNil", updates == nil)
		return nil, utils.ErrInvalidInput
	}

	// Validate template exists
	template := updates.Configuration.Template
	if template != "" {
		slog.Info("LLMProviderService.Update: validating template", "orgID", orgID, "providerID", providerID, "template", template)
		templateExists, err := s.templateRepo.Exists(template, orgID)
		if err != nil {
			slog.Error("LLMProviderService.Update: failed to validate template", "orgID", orgID, "providerID", providerID, "template", template, "error", err)
			return nil, fmt.Errorf("failed to validate template: %w", err)
		}
		if !templateExists {
			slog.Warn("LLMProviderService.Update: template not found", "orgID", orgID, "providerID", providerID, "template", template)
			return nil, utils.ErrLLMProviderTemplateNotFound
		}
	}

	// Parse organization UUID
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		slog.Error("LLMProviderService.Update: invalid organization UUID", "orgID", orgID, "providerID", providerID, "error", err)
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}

	// Serialize model providers to ModelList
	if len(updates.ModelProviders) > 0 {
		slog.Info("LLMProviderService.Update: serializing model providers", "orgID", orgID, "providerID", providerID, "count", len(updates.ModelProviders))
		modelListBytes, err := json.Marshal(updates.ModelProviders)
		if err != nil {
			slog.Error("LLMProviderService.Update: failed to serialize model providers", "orgID", orgID, "providerID", providerID, "error", err)
			return nil, fmt.Errorf("failed to serialize model providers: %w", err)
		}
		updates.ModelList = string(modelListBytes)
	}

	// Update provider
	slog.Info("LLMProviderService.Update: updating provider in database", "orgID", orgID, "providerID", providerID)
	if err := s.providerRepo.Update(updates, providerID, orgUUID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderService.Update: provider not found", "orgID", orgID, "providerID", providerID)
			return nil, utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Update: failed to update provider", "orgID", orgID, "providerID", providerID, "error", err)
		return nil, fmt.Errorf("failed to update provider: %w", err)
	}

	// Fetch updated provider
	slog.Info("LLMProviderService.Update: fetching updated provider", "orgID", orgID, "providerID", providerID)
	updated, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		slog.Error("LLMProviderService.Update: failed to fetch updated provider", "orgID", orgID, "providerID", providerID, "error", err)
		return nil, fmt.Errorf("failed to fetch updated provider: %w", err)
	}
	if updated == nil {
		slog.Warn("LLMProviderService.Update: updated provider not found", "orgID", orgID, "providerID", providerID)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Parse model providers from ModelList
	if updated.ModelList != "" {
		slog.Info("LLMProviderService.Update: parsing model providers", "orgID", orgID, "providerID", providerID)
		if err := json.Unmarshal([]byte(updated.ModelList), &updated.ModelProviders); err != nil {
			slog.Error("LLMProviderService.Update: failed to parse model providers", "orgID", orgID, "providerID", providerID, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	slog.Info("LLMProviderService.Update: completed successfully", "orgID", orgID, "providerID", providerID, "providerUUID", updated.UUID)
	return updated, nil
}

// Delete deletes an LLM provider after undeploying from all gateways
func (s *LLMProviderService) Delete(providerID, orgID string, deploymentService *LLMProviderDeploymentService) error {
	slog.Info("LLMProviderService.Delete: starting", "orgID", orgID, "providerID", providerID)

	if providerID == "" {
		slog.Error("LLMProviderService.Delete: providerID is empty", "orgID", orgID)
		return utils.ErrInvalidInput
	}

	// Verify provider exists
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderService.Delete: provider not found", "orgID", orgID, "providerID", providerID)
			return utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Delete: failed to get provider", "orgID", orgID, "providerID", providerID, "error", err)
		return fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		slog.Warn("LLMProviderService.Delete: provider not found", "orgID", orgID, "providerID", providerID)
		return utils.ErrLLMProviderNotFound
	}

	// Get all gateway mappings for this provider
	providerUUID, err := uuid.Parse(providerID)
	if err != nil {
		slog.Error("LLMProviderService.Delete: invalid provider UUID", "providerID", providerID, "error", err)
		return fmt.Errorf("invalid provider UUID: %w", err)
	}

	gatewayIDs, err := s.mappingRepo.GetByProvider(providerUUID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		slog.Error("LLMProviderService.Delete: failed to get gateway mappings", "orgID", orgID, "providerID", providerID, "error", err)
		return fmt.Errorf("failed to get gateway mappings: %w", err)
	}

	slog.Info("LLMProviderService.Delete: found gateway mappings", "orgID", orgID, "providerID", providerID, "gatewayCount", len(gatewayIDs))

	// Undeploy from all gateways before deleting
	if len(gatewayIDs) > 0 {
		undeploymentErrors := []string{}
		successfulUndeployments := 0

		for _, gatewayID := range gatewayIDs {
			slog.Info("LLMProviderService.Delete: undeploying from gateway", "orgID", orgID, "providerID", providerID, "gatewayID", gatewayID)

			// Get current deployment for this gateway
			deployments, err := deploymentService.GetLLMProviderDeployments(providerID, orgID, &gatewayID, nil)
			if err != nil {
				slog.Error("LLMProviderService.Delete: failed to get deployments for gateway", "orgID", orgID, "providerID", providerID, "gatewayID", gatewayID, "error", err)
				undeploymentErrors = append(undeploymentErrors, fmt.Sprintf("gateway %s: failed to fetch deployments: %v", gatewayID, err))
				continue
			}

			// Find the deployed deployment and undeploy it
			found := false
			for _, deployment := range deployments {
				if deployment.Status != nil && *deployment.Status == models.DeploymentStatusDeployed {
					found = true
					if _, err := deploymentService.UndeployLLMProviderDeployment(providerID, deployment.DeploymentID.String(), gatewayID, orgID); err != nil {
						slog.Error("LLMProviderService.Delete: failed to undeploy from gateway", "orgID", orgID, "providerID", providerID, "gatewayID", gatewayID, "deploymentID", deployment.DeploymentID, "error", err)
						undeploymentErrors = append(undeploymentErrors, fmt.Sprintf("gateway %s: %v", gatewayID, err))
					} else {
						slog.Info("LLMProviderService.Delete: undeployed from gateway successfully", "orgID", orgID, "providerID", providerID, "gatewayID", gatewayID)
						successfulUndeployments++
					}
					break
				}
			}
			if !found {
				slog.Warn("LLMProviderService.Delete: no deployed deployment found for gateway", "orgID", orgID, "providerID", providerID, "gatewayID", gatewayID)
			}
		}

		slog.Info("LLMProviderService.Delete: undeployment results", "orgID", orgID, "providerID", providerID, "successfulUndeployments", successfulUndeployments, "totalGateways", len(gatewayIDs), "errorCount", len(undeploymentErrors))

		// If all undeployments failed, return error
		if len(undeploymentErrors) > 0 && successfulUndeployments == 0 {
			slog.Error("LLMProviderService.Delete: all undeployments failed", "orgID", orgID, "providerID", providerID, "errors", undeploymentErrors)
			return fmt.Errorf("failed to undeploy from all %d gateways: %v", len(gatewayIDs), undeploymentErrors)
		}

		// If some undeployments failed, log warning but continue with deletion
		if len(undeploymentErrors) > 0 {
			slog.Warn("LLMProviderService.Delete: some undeployments failed, continuing with deletion", "orgID", orgID, "providerID", providerID, "errors", undeploymentErrors)
		}
	}

	// Now delete the provider from database (cascade deletes mappings)
	slog.Info("LLMProviderService.Delete: deleting provider from database", "orgID", orgID, "providerID", providerID)
	if err := s.providerRepo.Delete(providerID, orgID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderService.Delete: provider not found", "orgID", orgID, "providerID", providerID)
			return utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Delete: failed to delete provider", "orgID", orgID, "providerID", providerID, "error", err)
		return fmt.Errorf("failed to delete provider: %w", err)
	}

	slog.Info("LLMProviderService.Delete: completed successfully", "orgID", orgID, "providerID", providerID)
	return nil
}

// UpdateAndSync updates an LLM provider and syncs its gateway deployments
func (s *LLMProviderService) UpdateAndSync(providerID, orgID string, updates *models.LLMProvider, gatewayIDs []string, deploymentService *LLMProviderDeploymentService) (*UpdateAndSyncResponse, error) {
	slog.Info("LLMProviderService.UpdateAndSync: starting", "providerID", providerID, "orgID", orgID, "gatewayCount", len(gatewayIDs))

	// First, update the provider using the existing Update method
	updated, err := s.Update(providerID, orgID, updates)
	if err != nil {
		slog.Error("LLMProviderService.UpdateAndSync: failed to update provider", "providerID", providerID, "orgID", orgID, "error", err)
		return nil, err
	}

	slog.Info("LLMProviderService.UpdateAndSync: provider updated successfully", "providerID", providerID, "providerUUID", updated.UUID)

	// Parse UUIDs
	providerUUID, err := uuid.Parse(providerID)
	if err != nil {
		slog.Error("LLMProviderService.UpdateAndSync: invalid provider UUID", "providerID", providerID, "error", err)
		return nil, fmt.Errorf("invalid provider UUID: %w", err)
	}

	// Convert gateway IDs to UUIDs and track invalid ones
	gatewayUUIDs := make([]uuid.UUID, 0, len(gatewayIDs))
	invalidGatewayResults := []DeploymentResult{}
	for _, gatewayID := range gatewayIDs {
		gatewayUUID, err := uuid.Parse(gatewayID)
		if err != nil {
			slog.Error("LLMProviderService.UpdateAndSync: invalid gateway UUID", "orgID", orgID, "gatewayID", gatewayID, "error", err)
			invalidGatewayResults = append(invalidGatewayResults, DeploymentResult{
				GatewayID: gatewayID,
				Success:   false,
				Error:     fmt.Sprintf("invalid gateway UUID: %v", err),
			})
			continue
		}
		gatewayUUIDs = append(gatewayUUIDs, gatewayUUID)
	}

	// Return error if ALL gateway IDs are invalid
	if len(gatewayIDs) > 0 && len(gatewayUUIDs) == 0 {
		slog.Error("LLMProviderService.UpdateAndSync: all gateway UUIDs are invalid", "providerID", providerID, "totalRequested", len(gatewayIDs))
		return nil, fmt.Errorf("all %d gateway IDs are invalid", len(gatewayIDs))
	}

	// Get current gateway mappings (returns []string)
	currentGateways, err := s.mappingRepo.GetByProvider(providerUUID)
	if err != nil {
		slog.Error("LLMProviderService.UpdateAndSync: failed to get current mappings", "providerID", providerID, "error", err)
		return nil, err
	}

	// Replace mappings in database
	if err := s.mappingRepo.ReplaceForProvider(providerUUID, gatewayUUIDs); err != nil {
		slog.Error("LLMProviderService.UpdateAndSync: failed to replace gateway mappings", "providerID", providerID, "error", err)
		return nil, err
	}

	slog.Info("LLMProviderService.UpdateAndSync: gateway mappings replaced", "providerID", providerID, "newCount", len(gatewayUUIDs), "oldCount", len(currentGateways))

	// Determine which gateways to add and which to remove
	currentGatewayMap := make(map[string]bool)
	for _, gwID := range currentGateways {
		currentGatewayMap[gwID] = true
	}

	newGatewayMap := make(map[string]bool)
	for _, gw := range gatewayUUIDs {
		newGatewayMap[gw.String()] = true
	}

	// Deploy to newly added gateways and track results
	deploymentResults := make([]DeploymentResult, 0)
	deploymentResults = append(deploymentResults, invalidGatewayResults...)
	deploymentIndex := 1
	successfulDeployments := 0
	attemptedDeployments := 0

	for _, gatewayUUID := range gatewayUUIDs {
		gatewayID := gatewayUUID.String()
		if !currentGatewayMap[gatewayUUID.String()] {
			attemptedDeployments++
			slog.Info("LLMProviderService.UpdateAndSync: deploying to new gateway", "providerID", providerID, "gatewayID", gatewayID)

			deploymentName := fmt.Sprintf("%s-deployment-%d", updated.Configuration.Name, deploymentIndex)
			deployReq := &models.DeployAPIRequest{
				Name:      deploymentName,
				Base:      "current",
				GatewayID: gatewayID,
				Metadata: map[string]interface{}{
					"auto_deployed": true,
					"sync_update":   true,
				},
			}

			if _, err := deploymentService.DeployLLMProvider(providerID, deployReq, orgID); err != nil {
				slog.Error("LLMProviderService.UpdateAndSync: failed to deploy to new gateway", "providerID", providerID, "gatewayID", gatewayID, "error", err)
				deploymentResults = append(deploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   false,
					Error:     err.Error(),
				})
			} else {
				slog.Info("LLMProviderService.UpdateAndSync: deployed to new gateway successfully", "providerID", providerID, "gatewayID", gatewayID)
				successfulDeployments++
				deploymentResults = append(deploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   true,
				})
			}
			deploymentIndex++
		} else {
			attemptedDeployments++
			slog.Info("LLMProviderService.UpdateAndSync: updating the current deployment", "providerID", providerID, "gatewayID", gatewayID)
			currentDeployment, err := deploymentService.deploymentRepo.GetCurrentByGateway(providerID, gatewayID, orgID)
			if err != nil {
				deploymentResults = append(deploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   false,
					Error:     err.Error(),
				})
			}

			deployReq := &models.DeployAPIRequest{
				Name:      currentDeployment.Name,
				Base:      currentDeployment.DeploymentID.String(),
				GatewayID: gatewayID,
				Metadata: map[string]interface{}{
					"auto_deployed": true,
					"sync_update":   true,
				},
			}

			if _, err := deploymentService.DeployLLMProvider(providerID, deployReq, orgID); err != nil {
				slog.Error("LLMProviderService.UpdateAndSync: failed to update deployment in gateway", "providerID", providerID, "gatewayID", gatewayID, "error", err)
				deploymentResults = append(deploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   false,
					Error:     err.Error(),
				})
			} else {
				slog.Info("LLMProviderService.UpdateAndSync: deployed to new gateway successfully", "providerID", providerID, "gatewayID", gatewayID)
				successfulDeployments++
				deploymentResults = append(deploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   true,
				})
			}
			deploymentIndex++
		}
	}

	// Fail if ALL new deployments failed
	if attemptedDeployments > 0 && successfulDeployments == 0 {
		slog.Error("LLMProviderService.UpdateAndSync: all new deployments failed", "providerID", providerID, "attempted", attemptedDeployments)
		return nil, fmt.Errorf("all %d new gateway deployments failed", attemptedDeployments)
	}

	// Undeploy from removed gateways and track results
	undeploymentResults := make([]DeploymentResult, 0)
	attemptedUndeployments := 0
	successfulUndeployments := 0

	for _, gatewayID := range currentGateways {
		if !newGatewayMap[gatewayID] {
			attemptedUndeployments++
			slog.Info("LLMProviderService.UpdateAndSync: undeploying from removed gateway", "providerID", providerID, "gatewayID", gatewayID)

			// Get current deployment for this gateway
			deployments, err := deploymentService.GetLLMProviderDeployments(providerID, orgID, &gatewayID, nil)
			if err != nil {
				slog.Error("LLMProviderService.UpdateAndSync: failed to get deployments for gateway", "providerID", providerID, "gatewayID", gatewayID, "error", err)
				undeploymentResults = append(undeploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   false,
					Error:     fmt.Sprintf("failed to fetch deployments: %v", err),
				})
				continue
			}

			// Find the deployed deployment and undeploy it
			found := false
			for _, deployment := range deployments {
				if deployment.Status != nil && *deployment.Status == models.DeploymentStatusDeployed {
					found = true
					if _, err := deploymentService.UndeployLLMProviderDeployment(providerID, deployment.DeploymentID.String(), gatewayID, orgID); err != nil {
						slog.Error("LLMProviderService.UpdateAndSync: failed to undeploy from removed gateway", "providerID", providerID, "gatewayID", gatewayID, "deploymentID", deployment.DeploymentID, "error", err)
						undeploymentResults = append(undeploymentResults, DeploymentResult{
							GatewayID: gatewayID,
							Success:   false,
							Error:     err.Error(),
						})
					} else {
						slog.Info("LLMProviderService.UpdateAndSync: undeployed from removed gateway successfully", "providerID", providerID, "gatewayID", gatewayID)
						successfulUndeployments++
						undeploymentResults = append(undeploymentResults, DeploymentResult{
							GatewayID: gatewayID,
							Success:   true,
						})
					}
					break
				}
			}
			if !found {
				slog.Warn("LLMProviderService.UpdateAndSync: no deployed deployment found for gateway", "providerID", providerID, "gatewayID", gatewayID)
				undeploymentResults = append(undeploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   false,
					Error:     "no deployed deployment found",
				})
			}
		}
	}

	slog.Info("LLMProviderService.UpdateAndSync: completed",
		"providerID", providerID,
		"newGatewayCount", len(gatewayUUIDs),
		"previousGatewayCount", len(currentGateways),
		"successfulDeployments", successfulDeployments,
		"attemptedDeployments", attemptedDeployments,
		"successfulUndeployments", successfulUndeployments,
		"attemptedUndeployments", attemptedUndeployments)

	return &UpdateAndSyncResponse{
		Provider:      updated,
		Deployments:   deploymentResults,
		Undeployments: undeploymentResults,
	}, nil
}

// ListProxiesByProvider lists all LLM proxies for a provider
func (s *LLMProviderService) ListProxiesByProvider(providerID, orgID string, limit, offset int) ([]*models.LLMProxy, int, error) {
	if providerID == "" {
		return nil, 0, utils.ErrInvalidInput
	}

	// Get provider to get its UUID
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return nil, 0, utils.ErrLLMProviderNotFound
	}

	// List proxies by provider UUID
	proxies, err := s.proxyRepo.ListByProvider(orgID, provider.UUID.String(), limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list proxies by provider: %w", err)
	}

	totalCount, err := s.proxyRepo.CountByProvider(orgID, provider.UUID.String())
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count proxies by provider: %w", err)
	}

	return proxies, totalCount, nil
}

// CreateAndDeploy creates an LLM provider and deploys it to the specified gateways
func (s *LLMProviderService) CreateAndDeploy(orgID, createdBy string, provider *models.LLMProvider, gatewayIDs []string, deploymentService *LLMProviderDeploymentService) (*CreateAndDeployResponse, error) {
	slog.Info("LLMProviderService.CreateAndDeploy: starting", "orgID", orgID, "createdBy", createdBy, "gatewayCount", len(gatewayIDs))

	// First, create the provider using the existing Create method
	created, err := s.Create(orgID, createdBy, provider)
	if err != nil {
		slog.Error("LLMProviderService.CreateAndDeploy: failed to create provider", "orgID", orgID, "error", err)
		return nil, err
	}

	slog.Info("LLMProviderService.CreateAndDeploy: provider created successfully", "orgID", orgID, "providerUUID", created.UUID)

	// Track invalid gateway UUIDs and create mappings for valid ones
	mappings := make([]*models.LLMProviderGatewayMapping, 0, len(gatewayIDs))
	deploymentResults := make([]DeploymentResult, 0, len(gatewayIDs))
	validGatewayIDs := make([]string, 0, len(gatewayIDs))

	for _, gatewayID := range gatewayIDs {
		gatewayUUID, err := uuid.Parse(gatewayID)
		if err != nil {
			slog.Error("LLMProviderService.CreateAndDeploy: invalid gateway UUID", "orgID", orgID, "gatewayID", gatewayID, "error", err)
			deploymentResults = append(deploymentResults, DeploymentResult{
				GatewayID: gatewayID,
				Success:   false,
				Error:     fmt.Sprintf("invalid gateway UUID: %v", err),
			})
			continue
		}
		mappings = append(mappings, &models.LLMProviderGatewayMapping{
			LLMProviderUUID: created.UUID.String(),
			GatewayUUID:     gatewayUUID.String(),
		})
		validGatewayIDs = append(validGatewayIDs, gatewayID)
	}

	// Return error if ALL gateway IDs are invalid
	if len(gatewayIDs) > 0 && len(validGatewayIDs) == 0 {
		slog.Error("LLMProviderService.CreateAndDeploy: all gateway UUIDs are invalid", "orgID", orgID, "totalRequested", len(gatewayIDs))
		return nil, fmt.Errorf("all %d gateway IDs are invalid", len(gatewayIDs))
	}

	// Store mappings in database
	mappingsFailed := false
	if len(mappings) > 0 {
		if err := s.mappingRepo.CreateBatch(mappings); err != nil {
			slog.Error("LLMProviderService.CreateAndDeploy: failed to store gateway mappings", "orgID", orgID, "providerUUID", created.UUID, "error", err)
			// Return error instead of silently continuing - this creates state inconsistency
			return nil, fmt.Errorf("failed to store gateway mappings: %w", err)
		}
		slog.Info("LLMProviderService.CreateAndDeploy: gateway mappings stored", "orgID", orgID, "providerUUID", created.UUID, "mappingCount", len(mappings))
	}

	// Deploy to each valid gateway and track results
	successfulDeployments := 0
	for i, gatewayID := range validGatewayIDs {
		slog.Info("LLMProviderService.CreateAndDeploy: deploying to gateway", "orgID", orgID, "providerUUID", created.UUID, "gatewayID", gatewayID, "index", i+1, "total", len(validGatewayIDs))

		// Generate deployment name: provider-name-gateway-index
		deploymentName := fmt.Sprintf("%s-deployment-%d", created.Configuration.Name, i+1)

		// Create deployment request
		deployReq := &models.DeployAPIRequest{
			Name:      deploymentName,
			Base:      "current", // Use current provider configuration
			GatewayID: gatewayID,
			Metadata: map[string]interface{}{
				"auto_deployed": true,
				"gateway_index": i + 1,
			},
		}

		// Deploy to gateway
		deployment, err := deploymentService.DeployLLMProvider(created.UUID.String(), deployReq, orgID)
		if err != nil {
			slog.Error("LLMProviderService.CreateAndDeploy: failed to deploy to gateway", "orgID", orgID, "providerUUID", created.UUID, "gatewayID", gatewayID, "error", err)
			deploymentResults = append(deploymentResults, DeploymentResult{
				GatewayID: gatewayID,
				Success:   false,
				Error:     err.Error(),
			})
			continue
		}

		slog.Info("LLMProviderService.CreateAndDeploy: deployed to gateway successfully", "orgID", orgID, "providerUUID", created.UUID, "gatewayID", gatewayID, "deploymentID", deployment.DeploymentID)
		successfulDeployments++
		deploymentResults = append(deploymentResults, DeploymentResult{
			GatewayID: gatewayID,
			Success:   true,
		})
	}

	// Fail if ALL deployments failed (but only if we had valid gateways to deploy to)
	if !mappingsFailed && len(validGatewayIDs) > 0 && successfulDeployments == 0 {
		slog.Error("LLMProviderService.CreateAndDeploy: all deployments failed", "orgID", orgID, "providerUUID", created.UUID, "attempted", len(validGatewayIDs))
		return nil, fmt.Errorf("all %d gateway deployments failed", len(validGatewayIDs))
	}

	slog.Info("LLMProviderService.CreateAndDeploy: completed", "orgID", orgID, "providerUUID", created.UUID, "successfulDeployments", successfulDeployments, "totalAttempted", len(validGatewayIDs))

	return &CreateAndDeployResponse{
		Provider:    created,
		Deployments: deploymentResults,
	}, nil
}

func (s *LLMProviderService) GetProviderGatewayMapping(providerId uuid.UUID) ([]string, error) {
	gws, err := s.mappingRepo.GetByProvider(providerId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("no gateway mapping found for provider")
			return make([]string, 0), nil
		}
		slog.Error("error while fetching gateway mapping for provider", providerId.String(), err)
		return nil, err
	}
	return gws, nil
}
