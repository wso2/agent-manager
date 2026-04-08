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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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
	db            *gorm.DB
	providerRepo  repositories.LLMProviderRepository
	templateRepo  repositories.LLMProviderTemplateRepository
	templateStore *LLMTemplateStore
	proxyRepo     repositories.LLMProxyRepository
	artifactRepo  repositories.ArtifactRepository
	secretClient  secretmanagersvc.SecretManagementClient
	gatewayRepo   repositories.GatewayRepository
}

// NewLLMProviderService creates a new LLM provider service
func NewLLMProviderService(
	db *gorm.DB,
	providerRepo repositories.LLMProviderRepository,
	templateRepo repositories.LLMProviderTemplateRepository,
	templateStore *LLMTemplateStore,
	proxyRepo repositories.LLMProxyRepository,
	artifactRepo repositories.ArtifactRepository,
	secretClient secretmanagersvc.SecretManagementClient,
	gatewayRepo repositories.GatewayRepository,
) *LLMProviderService {
	return &LLMProviderService{
		db:            db,
		providerRepo:  providerRepo,
		templateRepo:  templateRepo,
		templateStore: templateStore,
		proxyRepo:     proxyRepo,
		artifactRepo:  artifactRepo,
		secretClient:  secretClient,
		gatewayRepo:   gatewayRepo,
	}
}

// resolveProvider looks up a provider by UUID or handle.
func (s *LLMProviderService) resolveProvider(identifier, orgName string) (*models.LLMProvider, error) {
	if _, err := uuid.Parse(identifier); err == nil {
		return s.providerRepo.GetByUUID(identifier, orgName)
	}
	return s.providerRepo.GetByHandle(identifier, orgName)
}

// Create creates a new LLM provider
func (s *LLMProviderService) Create(ctx context.Context, orgName, createdBy string, provider *models.LLMProvider) (*models.LLMProvider, error) {
	slog.Info("LLMProviderService.Create: starting", "orgName", orgName, "createdBy", createdBy)

	if provider == nil {
		slog.Error("LLMProviderService.Create: provider is nil", "orgName", orgName)
		return nil, utils.ErrInvalidInput
	}

	// Extract handle, name, and version from configuration
	// Note: handle is not in Configuration, so we use name as handle
	name := provider.Configuration.Name
	version := provider.Configuration.Version

	// Use name as handle (artifact identifier)
	handle := provider.Configuration.Handle

	slog.Info("LLMProviderService.Create: extracted configuration", "orgName", orgName, "handle", handle, "name", name, "version", version)

	if handle == "" || name == "" || version == "" {
		slog.Error("LLMProviderService.Create: missing required fields", "orgName", orgName, "handle", handle, "name", name, "version", version)
		return nil, utils.ErrInvalidInput
	}

	// Fail fast if a provider with this handle already exists, before touching KV.
	if _, err := s.providerRepo.GetByHandle(handle, orgName); err == nil {
		slog.Warn("LLMProviderService.Create: provider already exists", "orgName", orgName, "handle", handle)
		return nil, utils.ErrLLMProviderExists
	}

	// Validate template exists
	template := provider.Configuration.Template
	if template == "" {
		slog.Error("LLMProviderService.Create: template not specified", "orgName", orgName, "handle", handle)
		return nil, utils.ErrInvalidInput
	}

	// Set default values
	provider.CreatedBy = createdBy
	provider.Status = llmStatusPending
	if provider.Configuration.Context == nil {
		defaultContext := "/"
		provider.Configuration.Context = &defaultContext
	}

	slog.Info("LLMProviderService.Create: set default values", "orgName", orgName, "handle", handle, "status", provider.Status, "context", *provider.Configuration.Context)

	// Serialize model providers to ModelList
	if len(provider.ModelProviders) > 0 {
		slog.Info("LLMProviderService.Create: serializing model providers", "orgName", orgName, "handle", handle, "count", len(provider.ModelProviders))
		modelListBytes, err := json.Marshal(provider.ModelProviders)
		if err != nil {
			slog.Error("LLMProviderService.Create: failed to serialize model providers", "orgName", orgName, "handle", handle, "error", err)
			return nil, fmt.Errorf("failed to serialize model providers: %w", err)
		}
		provider.ModelList = string(modelListBytes)
	}

	// Validate template exists (check both built-in and user templates)
	slog.Info("LLMProviderService.Create: validating template", "orgName", orgName, "handle", handle, "template", template)
	templateExists := s.templateStore.Exists(template)
	if !templateExists {
		// Check user templates in database
		userTemplateExists, err := s.templateRepo.Exists(template, orgName)
		if err != nil {
			slog.Error("LLMProviderService.Create: failed to validate user template", "orgName", orgName, "handle", handle, "template", template, "error", err)
			return nil, fmt.Errorf("failed to validate template: %w", err)
		}
		if !userTemplateExists {
			slog.Warn("LLMProviderService.Create: template not found", "orgName", orgName, "handle", handle, "template", template)
			return nil, utils.ErrLLMProviderTemplateNotFound
		}
	}

	// Set template handle in provider
	provider.TemplateHandle = template

	// Validate mutual exclusivity of Auth.Value and Auth.SecretRef
	if provider.Configuration.Upstream != nil &&
		provider.Configuration.Upstream.Main != nil &&
		provider.Configuration.Upstream.Main.Auth != nil {
		if err := provider.Configuration.Upstream.Main.Auth.Validate(); err != nil {
			return nil, err
		}
	}

	// Store upstream API key in KV if provided
	var secretLoc *secretmanagersvc.SecretLocation
	if provider.Configuration.Upstream != nil &&
		provider.Configuration.Upstream.Main != nil &&
		provider.Configuration.Upstream.Main.Auth != nil &&
		provider.Configuration.Upstream.Main.Auth.Value != nil {

		if s.secretClient == nil {
			return nil, fmt.Errorf("secret management client is not configured; cannot store upstream API key")
		}

		loc := secretmanagersvc.SecretLocation{
			OrgName:    orgName,
			EntityName: handle,
			SecretKey:  secretmanagersvc.SecretKeyAPIKey,
		}
		kvPath, err := s.secretClient.CreateSecret(
			ctx, loc,
			map[string]string{secretmanagersvc.SecretKeyAPIKey: *provider.Configuration.Upstream.Main.Auth.Value},
		)
		if err != nil {
			slog.Error("LLMProviderService.Create: failed to store upstream key in KV",
				"orgName", orgName, "handle", handle, "error", err)
			return nil, fmt.Errorf("failed to store upstream API key: %w", err)
		}
		secretLoc = &loc

		// Zero out the plaintext value before releasing the reference
		plaintext := provider.Configuration.Upstream.Main.Auth.Value
		if plaintext != nil {
			zeroBytes := make([]byte, len(*plaintext))
			for i := range zeroBytes {
				zeroBytes[i] = 0
			}
			cleared := string(zeroBytes)
			*plaintext = cleared
		}
		// Replace plaintext with KV reference
		provider.Configuration.Upstream.Main.Auth.SecretRef = &kvPath
		provider.Configuration.Upstream.Main.Auth.Value = nil

		slog.Info("LLMProviderService.Create: stored upstream key in KV",
			"orgName", orgName, "handle", handle, "kvPath", kvPath)
	}

	// Create provider in transaction with validation
	slog.Info("LLMProviderService.Create: creating provider in database", "orgName", orgName, "handle", handle, "name", name, "version", version)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Create provider - uniqueness enforced by DB constraint
		return s.providerRepo.Create(tx, provider, handle, name, version, orgName)
	})
	if err != nil {
		// Check for unique constraint violation
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			slog.Warn("LLMProviderService.Create: provider already exists (unique constraint)", "orgName", orgName, "handle", handle)
			return nil, utils.ErrLLMProviderExists
		}
		// Compensating delete: remove the KV secret if DB creation failed
		if secretLoc != nil {
			if delErr := s.secretClient.DeleteSecret(ctx, *secretLoc, secretLoc.SecretRefName()); delErr != nil {
				kvPath, _ := secretLoc.KVPath()
				slog.Error("LLMProviderService.Create: failed to delete orphaned secret after DB failure — manual cleanup required",
					"orgName", orgName, "handle", handle,
					"kvPath", kvPath,
					"action", "DELETE_MANUALLY",
					"error", delErr)
			}
		}
		// Return template not found error directly
		if errors.Is(err, utils.ErrLLMProviderTemplateNotFound) {
			return nil, err
		}
		slog.Error("LLMProviderService.Create: failed to create provider", "orgName", orgName, "handle", handle, "error", err)
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	slog.Info("LLMProviderService.Create: provider created, fetching details", "orgName", orgName, "handle", handle, "uuid", provider.UUID)

	// Fetch created provider by UUID
	created, err := s.providerRepo.GetByUUID(provider.UUID.String(), orgName)
	if err != nil {
		slog.Error("LLMProviderService.Create: failed to fetch created provider", "orgName", orgName, "uuid", provider.UUID, "error", err)
		return nil, fmt.Errorf("failed to fetch created provider: %w", err)
	}

	// Parse model providers from ModelList
	if created.ModelList != "" {
		slog.Info("LLMProviderService.Create: parsing model providers from ModelList", "orgName", orgName, "handle", handle)
		if err := json.Unmarshal([]byte(created.ModelList), &created.ModelProviders); err != nil {
			slog.Error("LLMProviderService.Create: failed to parse model providers", "orgName", orgName, "handle", handle, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	slog.Info("LLMProviderService.Create: completed successfully", "orgName", orgName, "handle", handle, "providerUUID", created.UUID)
	return created, nil
}

// List lists all LLM providers for an organization
func (s *LLMProviderService) List(orgName string, limit, offset int) ([]*models.LLMProvider, int, error) {
	slog.Info("LLMProviderService.List: starting", "orgName", orgName, "limit", limit, "offset", offset)

	providers, err := s.providerRepo.List(orgName, limit, offset)
	if err != nil {
		slog.Error("LLMProviderService.List: failed to list providers", "orgName", orgName, "error", err)
		return nil, 0, fmt.Errorf("failed to list providers: %w", err)
	}

	slog.Info("LLMProviderService.List: providers retrieved from repository", "orgName", orgName, "count", len(providers))

	// Parse model providers for each provider
	for i, p := range providers {
		if p.ModelList != "" {
			if err := json.Unmarshal([]byte(p.ModelList), &p.ModelProviders); err != nil {
				slog.Error("LLMProviderService.List: failed to parse model providers", "orgName", orgName, "providerIndex", i, "providerUUID", p.UUID, "error", err)
				return nil, 0, fmt.Errorf("failed to parse model providers: %w", err)
			}
		}
	}

	totalCount, err := s.providerRepo.Count(orgName)
	if err != nil {
		slog.Error("LLMProviderService.List: failed to count providers", "orgName", orgName, "error", err)
		return nil, 0, fmt.Errorf("failed to count providers: %w", err)
	}

	slog.Info("LLMProviderService.List: completed successfully", "orgName", orgName, "count", len(providers), "total", totalCount)
	return providers, totalCount, nil
}

// Get retrieves an LLM provider by ID
func (s *LLMProviderService) Get(providerID, orgName string) (*models.LLMProvider, error) {
	slog.Info("LLMProviderService.Get: starting", "orgName", orgName, "providerID", providerID)

	if providerID == "" {
		slog.Error("LLMProviderService.Get: providerID is empty", "orgName", orgName)
		return nil, utils.ErrInvalidInput
	}

	provider, err := s.resolveProvider(providerID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderService.Get: provider not found", "orgName", orgName, "providerID", providerID)
			return nil, utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Get: failed to get provider", "orgName", orgName, "providerID", providerID, "error", err)
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		slog.Warn("LLMProviderService.Get: provider not found", "orgName", orgName, "providerID", providerID)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Parse model providers from ModelList
	if provider.ModelList != "" {
		slog.Info("LLMProviderService.Get: parsing model providers", "orgName", orgName, "providerID", providerID, "providerUUID", provider.UUID)
		if err := json.Unmarshal([]byte(provider.ModelList), &provider.ModelProviders); err != nil {
			slog.Error("LLMProviderService.Get: failed to parse model providers", "orgName", orgName, "providerID", providerID, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	slog.Info("LLMProviderService.Get: completed successfully", "orgName", orgName, "providerID", providerID, "providerUUID", provider.UUID)
	return provider, nil
}

// Update updates an existing LLM provider
func (s *LLMProviderService) Update(ctx context.Context, providerID, orgName string, updates *models.LLMProvider) (*models.LLMProvider, error) {
	slog.Info("LLMProviderService.Update: starting", "orgName", orgName, "providerID", providerID)

	if providerID == "" || updates == nil {
		slog.Error("LLMProviderService.Update: invalid input", "orgName", orgName, "providerID", providerID, "updatesIsNil", updates == nil)
		return nil, utils.ErrInvalidInput
	}

	// Validate template exists (check both built-in and user templates)
	template := updates.Configuration.Template
	if template != "" {
		slog.Info("LLMProviderService.Update: validating template", "orgName", orgName, "providerID", providerID, "template", template)
		templateExists := s.templateStore.Exists(template)
		if !templateExists {
			// Check user templates in database
			userTemplateExists, err := s.templateRepo.Exists(template, orgName)
			if err != nil {
				slog.Error("LLMProviderService.Update: failed to validate user template", "orgName", orgName, "providerID", providerID, "template", template, "error", err)
				return nil, fmt.Errorf("failed to validate template: %w", err)
			}
			if !userTemplateExists {
				slog.Warn("LLMProviderService.Update: template not found", "orgName", orgName, "providerID", providerID, "template", template)
				return nil, utils.ErrLLMProviderTemplateNotFound
			}
		}
		// Set template handle in updates
		updates.TemplateHandle = template
	}

	// Serialize model providers to ModelList
	if len(updates.ModelProviders) > 0 {
		slog.Info("LLMProviderService.Update: serializing model providers", "orgName", orgName, "providerID", providerID, "count", len(updates.ModelProviders))
		modelListBytes, err := json.Marshal(updates.ModelProviders)
		if err != nil {
			slog.Error("LLMProviderService.Update: failed to serialize model providers", "orgName", orgName, "providerID", providerID, "error", err)
			return nil, fmt.Errorf("failed to serialize model providers: %w", err)
		}
		updates.ModelList = string(modelListBytes)
	}

	// Validate mutual exclusivity of Auth.Value and Auth.SecretRef
	if updates.Configuration.Upstream != nil &&
		updates.Configuration.Upstream.Main != nil &&
		updates.Configuration.Upstream.Main.Auth != nil {
		if err := updates.Configuration.Upstream.Main.Auth.Validate(); err != nil {
			return nil, err
		}
	}

	// Update upstream API key in KV if a new value is provided
	if updates.Configuration.Upstream != nil &&
		updates.Configuration.Upstream.Main != nil &&
		updates.Configuration.Upstream.Main.Auth != nil &&
		updates.Configuration.Upstream.Main.Auth.Value != nil {

		if s.secretClient == nil {
			return nil, fmt.Errorf("secret management client is not configured; cannot update upstream API key")
		}

		// Get existing provider to find the handle for KV path
		existing, err := s.resolveProvider(providerID, orgName)
		if err != nil {
			return nil, fmt.Errorf("failed to get existing provider: %w", err)
		}
		if existing == nil {
			return nil, utils.ErrLLMProviderNotFound
		}
		providerHandle := existing.Artifact.Handle

		secretLoc := secretmanagersvc.SecretLocation{
			OrgName:    orgName,
			EntityName: providerHandle,
			SecretKey:  secretmanagersvc.SecretKeyAPIKey,
		}
		kvPath, err := s.secretClient.CreateSecret(
			ctx, secretLoc,
			map[string]string{secretmanagersvc.SecretKeyAPIKey: *updates.Configuration.Upstream.Main.Auth.Value},
		)
		if err != nil {
			slog.Error("LLMProviderService.Update: failed to update upstream key in KV",
				"orgName", orgName, "providerID", providerID, "error", err)
			return nil, fmt.Errorf("failed to update upstream API key: %w", err)
		}

		// Zero out the plaintext value before releasing the reference
		plaintext := updates.Configuration.Upstream.Main.Auth.Value
		if plaintext != nil {
			zeroBytes := make([]byte, len(*plaintext))
			for i := range zeroBytes {
				zeroBytes[i] = 0
			}
			cleared := string(zeroBytes)
			*plaintext = cleared
		}
		// Replace plaintext with KV reference
		updates.Configuration.Upstream.Main.Auth.SecretRef = &kvPath
		updates.Configuration.Upstream.Main.Auth.Value = nil
	}

	// Update provider
	slog.Info("LLMProviderService.Update: updating provider in database", "orgName", orgName, "providerID", providerID)
	if err := s.providerRepo.Update(updates, providerID, orgName); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderService.Update: provider not found", "orgName", orgName, "providerID", providerID)
			return nil, utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Update: failed to update provider", "orgName", orgName, "providerID", providerID, "error", err)
		// HIGH-4: KV has the new key but DB update failed — log for manual reconciliation
		slog.Error("STALE_KV_SECRET — DB update failed after KV update; manual reconciliation required",
			"orgName", orgName, "providerID", providerID)
		return nil, fmt.Errorf("failed to update provider: %w", err)
	}

	// Fetch updated provider
	slog.Info("LLMProviderService.Update: fetching updated provider", "orgName", orgName, "providerID", providerID)
	updated, err := s.resolveProvider(providerID, orgName)
	if err != nil {
		slog.Error("LLMProviderService.Update: failed to fetch updated provider", "orgName", orgName, "providerID", providerID, "error", err)
		return nil, fmt.Errorf("failed to fetch updated provider: %w", err)
	}
	if updated == nil {
		slog.Warn("LLMProviderService.Update: updated provider not found", "orgName", orgName, "providerID", providerID)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Parse model providers from ModelList
	if updated.ModelList != "" {
		slog.Info("LLMProviderService.Update: parsing model providers", "orgName", orgName, "providerID", providerID)
		if err := json.Unmarshal([]byte(updated.ModelList), &updated.ModelProviders); err != nil {
			slog.Error("LLMProviderService.Update: failed to parse model providers", "orgName", orgName, "providerID", providerID, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	slog.Info("LLMProviderService.Update: completed successfully", "orgName", orgName, "providerID", providerID, "providerUUID", updated.UUID)
	return updated, nil
}

// Delete deletes an LLM provider after undeploying from all gateways
func (s *LLMProviderService) Delete(ctx context.Context, providerID, orgName string, deploymentService *LLMProviderDeploymentService) error {
	slog.Info("LLMProviderService.Delete: starting", "orgName", orgName, "providerID", providerID)

	if providerID == "" {
		slog.Error("LLMProviderService.Delete: providerID is empty", "orgName", orgName)
		return utils.ErrInvalidInput
	}

	// Verify provider exists
	provider, err := s.resolveProvider(providerID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderService.Delete: provider not found", "orgName", orgName, "providerID", providerID)
			return utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Delete: failed to get provider", "orgName", orgName, "providerID", providerID, "error", err)
		return fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		slog.Warn("LLMProviderService.Delete: provider not found", "orgName", orgName, "providerID", providerID)
		return utils.ErrLLMProviderNotFound
	}

	// Get all deployed gateways for this provider
	providerUUID, err := uuid.Parse(providerID)
	if err != nil {
		slog.Error("LLMProviderService.Delete: invalid provider UUID", "providerID", providerID, "error", err)
		return fmt.Errorf("invalid provider UUID: %w", err)
	}

	gatewayIDs, err := deploymentService.deploymentRepo.GetDeployedGatewaysByProvider(providerUUID, orgName)
	if err != nil {
		slog.Error("LLMProviderService.Delete: failed to get deployed gateways", "orgName", orgName, "providerID", providerID, "error", err)
		return fmt.Errorf("failed to get deployed gateways: %w", err)
	}

	slog.Info("LLMProviderService.Delete: found deployed gateways", "orgName", orgName, "providerID", providerID, "gatewayCount", len(gatewayIDs))

	// Undeploy from all gateways before deleting
	if len(gatewayIDs) > 0 {
		undeploymentErrors := []string{}
		successfulUndeployments := 0

		for _, gatewayID := range gatewayIDs {
			slog.Info("LLMProviderService.Delete: undeploying from gateway", "orgName", orgName, "providerID", providerID, "gatewayID", gatewayID)

			// Get current deployment for this gateway
			deployments, err := deploymentService.GetLLMProviderDeployments(providerID, orgName, &gatewayID, nil)
			if err != nil {
				slog.Error("LLMProviderService.Delete: failed to get deployments for gateway", "orgName", orgName, "providerID", providerID, "gatewayID", gatewayID, "error", err)
				undeploymentErrors = append(undeploymentErrors, fmt.Sprintf("gateway %s: failed to fetch deployments: %v", gatewayID, err))
				continue
			}

			// Find the deployed deployment and undeploy it
			found := false
			for _, deployment := range deployments {
				if deployment.Status != nil && *deployment.Status == models.DeploymentStatusDeployed {
					found = true
					if _, err := deploymentService.UndeployLLMProviderDeployment(providerID, deployment.DeploymentID.String(), gatewayID, orgName); err != nil {
						slog.Error("LLMProviderService.Delete: failed to undeploy from gateway", "orgName", orgName, "providerID", providerID, "gatewayID", gatewayID, "deploymentID", deployment.DeploymentID, "error", err)
						undeploymentErrors = append(undeploymentErrors, fmt.Sprintf("gateway %s: %v", gatewayID, err))
					} else {
						slog.Info("LLMProviderService.Delete: undeployed from gateway successfully", "orgName", orgName, "providerID", providerID, "gatewayID", gatewayID)
						successfulUndeployments++
					}
					break
				}
			}
			if !found {
				slog.Warn("LLMProviderService.Delete: no deployed deployment found for gateway", "orgName", orgName, "providerID", providerID, "gatewayID", gatewayID)
			}
		}

		slog.Info("LLMProviderService.Delete: undeployment results", "orgName", orgName, "providerID", providerID, "successfulUndeployments", successfulUndeployments, "totalGateways", len(gatewayIDs), "errorCount", len(undeploymentErrors))

		// If all undeployments failed, return error
		if len(undeploymentErrors) > 0 && successfulUndeployments == 0 {
			slog.Error("LLMProviderService.Delete: all undeployments failed", "orgName", orgName, "providerID", providerID, "errors", undeploymentErrors)
			return fmt.Errorf("failed to undeploy from all %d gateways: %v", len(gatewayIDs), undeploymentErrors)
		}

		// If some undeployments failed, log warning but continue with deletion
		if len(undeploymentErrors) > 0 {
			slog.Warn("LLMProviderService.Delete: some undeployments failed, continuing with deletion", "orgName", orgName, "providerID", providerID, "errors", undeploymentErrors)
		}
	}

	// Now delete the provider from database (cascade deletes mappings)
	slog.Info("LLMProviderService.Delete: deleting provider from database", "orgName", orgName, "providerID", providerID)
	if err := s.providerRepo.Delete(providerID, orgName); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderService.Delete: provider not found", "orgName", orgName, "providerID", providerID)
			return utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Delete: failed to delete provider", "orgName", orgName, "providerID", providerID, "error", err)
		return fmt.Errorf("failed to delete provider: %w", err)
	}

	// Delete upstream API key from KV only after successful undeploy and DB deletion
	if provider.Configuration.Upstream != nil &&
		provider.Configuration.Upstream.Main != nil &&
		provider.Configuration.Upstream.Main.Auth != nil &&
		provider.Configuration.Upstream.Main.Auth.SecretRef != nil {

		if s.secretClient == nil {
			slog.Warn("LLMProviderService.Delete: secret management client is not configured; skipping KV cleanup",
				"orgName", orgName, "providerID", providerID,
				"kvPath", *provider.Configuration.Upstream.Main.Auth.SecretRef)
		} else {
			// Build location matching how the secret was created (org-level provider secret)
			secretLoc := secretmanagersvc.SecretLocation{
				OrgName:    orgName,
				EntityName: provider.Configuration.Handle,
				SecretKey:  secretmanagersvc.SecretKeyAPIKey,
			}
			if err := s.secretClient.DeleteSecret(ctx, secretLoc, secretLoc.SecretRefName()); err != nil {
				kvPath, _ := secretLoc.KVPath()
				slog.Error("LLMProviderService.Delete: failed to delete upstream key from KV — manual cleanup may be needed",
					"orgName", orgName, "providerID", providerID,
					"kvPath", kvPath, "error", err)
				// Continue — KV cleanup failure is non-fatal after successful DB deletion
			}
		}
	}

	slog.Info("LLMProviderService.Delete: completed successfully", "orgName", orgName, "providerID", providerID)
	return nil
}

// UpdateAndSync updates an LLM provider and syncs its gateway deployments
func (s *LLMProviderService) UpdateAndSync(ctx context.Context, providerID, orgName string, updates *models.LLMProvider, gatewayIDs []string, deploymentService *LLMProviderDeploymentService) (*UpdateAndSyncResponse, error) {
	slog.Info("LLMProviderService.UpdateAndSync: starting", "providerID", providerID, "orgName", orgName, "gatewayCount", len(gatewayIDs))

	// First, update the provider using the existing Update method
	updated, err := s.Update(ctx, providerID, orgName, updates)
	if err != nil {
		slog.Error("LLMProviderService.UpdateAndSync: failed to update provider", "providerID", providerID, "orgName", orgName, "error", err)
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
			slog.Error("LLMProviderService.UpdateAndSync: invalid gateway UUID", "orgName", orgName, "gatewayID", gatewayID, "error", err)
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

	currentGateways, err := deploymentService.deploymentRepo.GetDeployedGatewaysByProvider(providerUUID, orgName)
	if err != nil {
		slog.Error("LLMProviderService.UpdateAndSync: failed to get deployed gateways", "providerID", providerID, "error", err)
		return nil, err
	}

	slog.Info("LLMProviderService.UpdateAndSync: current deployed gateways retrieved", "providerID", providerID, "newCount", len(gatewayUUIDs), "oldCount", len(currentGateways))

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

			if _, err := deploymentService.DeployLLMProvider(providerID, deployReq, orgName); err != nil {
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
			currentDeployment, err := deploymentService.deploymentRepo.GetCurrentByGateway(providerID, gatewayID, orgName)
			if err != nil {
				deploymentResults = append(deploymentResults, DeploymentResult{
					GatewayID: gatewayID,
					Success:   false,
					Error:     err.Error(),
				})
			}

			deployReq := &models.DeployAPIRequest{
				Name: currentDeployment.Name,
				// Use "current" so the deployment YAML is regenerated from the latest provider
				// configuration (including updated policies). Using the old deployment UUID as Base
				// would copy the stale YAML content, missing any policy or config changes.
				Base:      "current",
				GatewayID: gatewayID,
				Metadata: map[string]interface{}{
					"auto_deployed": true,
					"sync_update":   true,
				},
			}

			if _, err := deploymentService.DeployLLMProvider(providerID, deployReq, orgName); err != nil {
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
			deployments, err := deploymentService.GetLLMProviderDeployments(providerID, orgName, &gatewayID, nil)
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
					if _, err := deploymentService.UndeployLLMProviderDeployment(providerID, deployment.DeploymentID.String(), gatewayID, orgName); err != nil {
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
func (s *LLMProviderService) ListProxiesByProvider(providerID, orgName string, limit, offset int) ([]*models.LLMProxy, int, error) {
	if providerID == "" {
		return nil, 0, utils.ErrInvalidInput
	}

	// Get provider to get its UUID
	provider, err := s.resolveProvider(providerID, orgName)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return nil, 0, utils.ErrLLMProviderNotFound
	}

	// List proxies by provider UUID
	proxies, err := s.proxyRepo.ListByProvider(orgName, provider.UUID.String(), limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list proxies by provider: %w", err)
	}

	totalCount, err := s.proxyRepo.CountByProvider(orgName, provider.UUID.String())
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count proxies by provider: %w", err)
	}

	return proxies, totalCount, nil
}

// CreateAndDeploy creates an LLM provider and deploys it to the specified gateways
func (s *LLMProviderService) CreateAndDeploy(ctx context.Context, orgName, createdBy string, provider *models.LLMProvider, gatewayIDs []string, deploymentService *LLMProviderDeploymentService) (*CreateAndDeployResponse, error) {
	slog.Info("LLMProviderService.CreateAndDeploy: starting", "orgName", orgName, "createdBy", createdBy, "gatewayCount", len(gatewayIDs))

	// Validate gateway UUIDs
	deploymentResults := make([]DeploymentResult, 0, len(gatewayIDs))
	validGatewayIDs := make([]string, 0, len(gatewayIDs))

	for _, gatewayID := range gatewayIDs {
		_, err := uuid.Parse(gatewayID)
		if err != nil {
			slog.Error("LLMProviderService.CreateAndDeploy: invalid gateway UUID", "orgName", orgName, "gatewayID", gatewayID, "error", err)
			deploymentResults = append(deploymentResults, DeploymentResult{
				GatewayID: gatewayID,
				Success:   false,
				Error:     fmt.Sprintf("invalid gateway UUID: %v", err),
			})
			continue
		}

		_, err = s.gatewayRepo.GetByUUID(gatewayID)
		if err != nil {
			slog.Error("LLMProviderService.CreateAndDeploy: no gateway found for provided gateway", "orgName", orgName, "gatewayID", gatewayID, "error", err)
			deploymentResults = append(deploymentResults, DeploymentResult{
				GatewayID: gatewayID,
				Success:   false,
				Error:     fmt.Sprintf("Gateway not found: %v", err),
			})
			continue
		}

		validGatewayIDs = append(validGatewayIDs, gatewayID)
	}

	// Return error if ALL gateway IDs are invalid
	if len(gatewayIDs) > 0 && len(validGatewayIDs) == 0 {
		slog.Error("LLMProviderService.CreateAndDeploy: all gateway UUIDs are invalid", "orgName", orgName, "totalRequested", len(gatewayIDs))
		return nil, fmt.Errorf("all %d gateway IDs are invalid", len(gatewayIDs))
	}

	// Create the provider using the existing Create method
	created, err := s.Create(ctx, orgName, createdBy, provider)
	if err != nil {
		slog.Error("LLMProviderService.CreateAndDeploy: failed to create provider", "orgName", orgName, "error", err)
		return nil, err
	}

	slog.Info("LLMProviderService.CreateAndDeploy: provider created successfully", "orgName", orgName, "providerUUID", created.UUID)

	// Deploy to each valid gateway and track results
	successfulDeployments := 0
	for i, gatewayID := range validGatewayIDs {
		slog.Info("LLMProviderService.CreateAndDeploy: deploying to gateway", "orgName", orgName, "providerUUID", created.UUID, "gatewayID", gatewayID, "index", i+1, "total", len(validGatewayIDs))

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
		deployment, err := deploymentService.DeployLLMProvider(created.UUID.String(), deployReq, orgName)
		if err != nil {
			slog.Error("LLMProviderService.CreateAndDeploy: failed to deploy to gateway", "orgName", orgName, "providerUUID", created.UUID, "gatewayID", gatewayID, "error", err)
			deploymentResults = append(deploymentResults, DeploymentResult{
				GatewayID: gatewayID,
				Success:   false,
				Error:     err.Error(),
			})
			continue
		}

		slog.Info("LLMProviderService.CreateAndDeploy: deployed to gateway successfully", "orgName", orgName, "providerUUID", created.UUID, "gatewayID", gatewayID, "deploymentID", deployment.DeploymentID)
		successfulDeployments++
		deploymentResults = append(deploymentResults, DeploymentResult{
			GatewayID: gatewayID,
			Success:   true,
		})
	}

	// Fail if ALL deployments failed (but only if we had valid gateways to deploy to)
	if len(validGatewayIDs) > 0 && successfulDeployments == 0 {
		slog.Error("LLMProviderService.CreateAndDeploy: all deployments failed", "orgName", orgName, "providerUUID", created.UUID, "attempted", len(validGatewayIDs))
		return nil, fmt.Errorf("all %d gateway deployments failed", len(validGatewayIDs))
	}

	slog.Info("LLMProviderService.CreateAndDeploy: completed", "orgName", orgName, "providerUUID", created.UUID, "successfulDeployments", successfulDeployments, "totalAttempted", len(validGatewayIDs))

	return &CreateAndDeployResponse{
		Provider:    created,
		Deployments: deploymentResults,
	}, nil
}

func (s *LLMProviderService) GetProviderGatewayMapping(providerId uuid.UUID, orgName string, deploymentService *LLMProviderDeploymentService) ([]string, error) {
	gws, err := deploymentService.deploymentRepo.GetDeployedGatewaysByProvider(providerId, orgName)
	if err != nil {
		slog.Error("error while fetching deployed gateways for provider", "providerID", providerId.String(), "error", err)
		return nil, err
	}
	return gws, nil
}

// UpdateCatalogStatus updates the catalog visibility status of an LLM provider
func (s *LLMProviderService) UpdateCatalogStatus(providerID, orgName string, inCatalog bool) (*models.LLMProvider, error) {
	slog.Info("LLMProviderService.UpdateCatalogStatus: starting", "providerID", providerID, "orgName", orgName, "inCatalog", inCatalog)

	// Validate UUIDs
	_, err := uuid.Parse(providerID)
	if err != nil {
		slog.Error("LLMProviderService.UpdateCatalogStatus: invalid provider UUID", "providerID", providerID, "error", err)
		return nil, utils.ErrInvalidInput
	}

	// Start transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		slog.Error("LLMProviderService.UpdateCatalogStatus: failed to begin transaction", "error", tx.Error)
		return nil, tx.Error
	}

	// Ensure transaction is rolled back on panic or error
	committed := false
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			slog.Error("LLMProviderService.UpdateCatalogStatus: panic recovered, rolling back", "panic", r)
			panic(r) // Re-panic after rollback
		}
		if !committed {
			tx.Rollback()
		}
	}()

	// Verify provider exists and belongs to org (within transaction)
	// Note: We use the non-transactional repo here since GetByUUID doesn't support tx parameter
	// This is acceptable as the critical update happens within the transaction
	provider, err := s.resolveProvider(providerID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("LLMProviderService.UpdateCatalogStatus: provider not found", "providerID", providerID, "orgName", orgName)
			return nil, utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.UpdateCatalogStatus: failed to get provider", "providerID", providerID, "error", err)
		return nil, err
	}
	if provider == nil {
		slog.Warn("LLMProviderService.UpdateCatalogStatus: provider not found", "providerID", providerID, "orgName", orgName)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Update artifact catalog status within transaction
	err = s.artifactRepo.UpdateCatalogStatus(tx, providerID, orgName, inCatalog)
	if err != nil {
		slog.Error("LLMProviderService.UpdateCatalogStatus: failed to update artifact catalog status", "providerID", providerID, "error", err)
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		slog.Error("LLMProviderService.UpdateCatalogStatus: failed to commit transaction", "error", err)
		return nil, err
	}
	committed = true

	// Update InCatalog field to reflect the committed change
	provider.InCatalog = inCatalog

	slog.Info("LLMProviderService.UpdateCatalogStatus: completed successfully", "providerID", providerID, "inCatalog", inCatalog)
	return provider, nil
}
