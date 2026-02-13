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
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	apiplatformclient "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/apiplatformsvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// OrganizationSynchronizer syncs organizations from OpenChoreo to the database and API Platform
type OrganizationSynchronizer interface {
	SyncOrganizationsFromOpenChoreo(ctx context.Context) error
}

type organizationSynchronizer struct {
	ocClient          client.OpenChoreoClient
	apiPlatformClient apiplatformclient.APIPlatformClient
	logger            *slog.Logger
}

// NewOrganizationSyncer creates a new organization syncer
func NewOrganizationSyncer(
	ocClient client.OpenChoreoClient,
	apiPlatformClient apiplatformclient.APIPlatformClient,
	logger *slog.Logger,
) OrganizationSynchronizer {
	return &organizationSynchronizer{
		ocClient:          ocClient,
		apiPlatformClient: apiPlatformClient,
		logger:            logger,
	}
}

// SyncOrganizationsFromOpenChoreo fetches organizations from OpenChoreo and syncs them to the database and API Platform
// This is called on service startup to ensure both the DB and API Platform are in sync with OpenChoreo
func (s *organizationSynchronizer) SyncOrganizationsFromOpenChoreo(ctx context.Context) error {
	s.logger.Info("Starting organization sync from OpenChoreo")

	// List all organizations from OpenChoreo
	orgs, err := s.ocClient.ListOrganizations(ctx)
	if err != nil {
		s.logger.Error("Failed to list organizations from OpenChoreo", "error", err)
		return fmt.Errorf("failed to list organizations: %w", err)
	}

	s.logger.Info("Found organizations from OpenChoreo", "count", len(orgs))

	totalSynced := 0
	totalSkipped := 0
	totalErrors := 0

	// For each organization, sync to DB and API Platform
	for _, org := range orgs {
		if err := s.synchronizeOrganization(ctx, org); err != nil {
			s.logger.Error("Failed to sync organization", "orgName", org.Name, "error", err)
			totalErrors++
		} else {
			totalSynced++
		}
	}

	s.logger.Info("Organization sync completed",
		"totalSynced", totalSynced,
		"totalSkipped", totalSkipped,
		"totalErrors", totalErrors,
	)

	return nil
}

// synchronizeOrganization syncs a single organization from OpenChoreo to the database and API Platform
func (s *organizationSynchronizer) synchronizeOrganization(ctx context.Context, ocOrg *models.OrganizationResponse) error {
	s.logger.Info("Syncing organization", "orgName", ocOrg.Name)

	// Generate a handle from the organization name (URL-friendly)
	handle := generateHandle(ocOrg.Name)

	// First, sync to local database
	var org models.Organization
	err := db.DB(ctx).Where("name = ?", ocOrg.Name).First(&org).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Organization doesn't exist in DB, create it
		org = models.Organization{
			UUID:      uuid.New(),
			Name:      ocOrg.Name,
			Handle:    handle,
			Region:    "US", // Default region, can be made configurable
			CreatedAt: ocOrg.CreatedAt,
			UpdatedAt: time.Now(),
		}

		if err := db.DB(ctx).Create(&org).Error; err != nil {
			// Handle unique constraint violation
			if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
				s.logger.Debug("Organization already exists in DB", "orgName", ocOrg.Name)
				// Re-fetch to get the existing org
				if err := db.DB(ctx).Where("name = ?", ocOrg.Name).First(&org).Error; err != nil {
					return fmt.Errorf("failed to get existing organization: %w", err)
				}
			} else {
				return fmt.Errorf("failed to create organization in DB: %w", err)
			}
		} else {
			s.logger.Info("Organization created in DB", "orgName", ocOrg.Name, "uuid", org.UUID)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check existing organization in DB: %w", err)
	} else {
		// Organization exists, update if needed
		org.UpdatedAt = time.Now()
		if err := db.DB(ctx).Save(&org).Error; err != nil {
			return fmt.Errorf("failed to update organization in DB: %w", err)
		}
		s.logger.Debug("Organization updated in DB", "orgName", ocOrg.Name)
	}

	// Now, sync to API Platform
	if s.apiPlatformClient != nil {
		s.logger.Info("Syncing orgs with api platform")
		if err := s.ensureOrganizationInAPIPlatform(ctx, &org); err != nil {
			s.logger.Warn("Failed to sync organization to API Platform", "orgName", org.Name, "error", err)
			// Don't fail the entire sync if API Platform sync fails
			// The organization is already in the local DB
		}
	}

	return nil
}

// ensureOrganizationInAPIPlatform ensures the organization exists in API Platform
func (s *organizationSynchronizer) ensureOrganizationInAPIPlatform(ctx context.Context, org *models.Organization) error {
	s.logger.Debug("Checking organization in API Platform", "orgName", org.Name)

	// Try to get the organization from API Platform
	resp, err := s.apiPlatformClient.GetOrganization(ctx)

	if err == nil && resp != nil {
		// Organization exists in API Platform
		s.logger.Debug("Organization already exists in API Platform", "orgName", org.Name)
		return nil
	}

	// Check if it's a 404 (organization doesn't exist)
	if err != nil && strings.Contains(err.Error(), "404") {
		s.logger.Info("Creating organization in API Platform", "orgName", org.Name)

		// Create organization in API Platform
		createReq := apiplatformclient.RegisterOrganizationRequest{
			ID:     org.UUID.String(),
			Name:   org.Name,
			Handle: org.Handle,
			Region: org.Region,
		}

		createResp, err := s.apiPlatformClient.RegisterOrganization(ctx, createReq)
		if err != nil {
			// Check if it's a conflict (organization already exists)
			if strings.Contains(err.Error(), "409") || strings.Contains(err.Error(), "conflict") {
				s.logger.Debug("Organization already exists in API Platform (conflict)", "orgName", org.Name)
				return nil
			}
			return fmt.Errorf("failed to create organization in API Platform: %w", err)
		}

		s.logger.Info("Organization created in API Platform", "orgName", org.Name, "id", createResp.ID)
	} else if err != nil {
		s.logger.Warn("Failed to check organization in API Platform", "orgName", org.Name, "error", err)
		// Don't fail - organization sync can continue
	}

	return nil
}

// generateHandle creates a URL-friendly handle from organization name
func generateHandle(name string) string {
	// Convert to lowercase and replace spaces/special chars with hyphens
	handle := strings.ToLower(name)
	handle = strings.ReplaceAll(handle, " ", "-")
	handle = strings.ReplaceAll(handle, "_", "-")
	// Remove any characters that aren't alphanumeric or hyphens
	var result strings.Builder
	for _, char := range handle {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			result.WriteRune(char)
		}
	}
	return result.String()
}
