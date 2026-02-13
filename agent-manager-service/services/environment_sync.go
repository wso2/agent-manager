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

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
)

// EnvironmentSynchronizer syncs environments from OpenChoreo to the database
type EnvironmentSynchronizer interface {
	SyncEnvironmentsFromOpenChoreo(ctx context.Context) error
}

type environmentSynchronizer struct {
	ocClient client.OpenChoreoClient
	logger   *slog.Logger
}

// NewEnvironmentSyncer creates a new environment syncer
func NewEnvironmentSyncer(
	ocClient client.OpenChoreoClient,
	logger *slog.Logger,
) EnvironmentSynchronizer {
	return &environmentSynchronizer{
		ocClient: ocClient,
		logger:   logger,
	}
}

// SyncEnvironmentsFromOpenChoreo fetches all environments from OpenChoreo and syncs them to the database
// This is called on service startup to ensure the DB is in sync with OpenChoreo
func (s *environmentSynchronizer) SyncEnvironmentsFromOpenChoreo(ctx context.Context) error {
	s.logger.Info("Starting environment sync from OpenChoreo")

	// List all organizations from OpenChoreo
	orgs, err := s.ocClient.ListOrganizations(ctx)
	if err != nil {
		s.logger.Error("Failed to list organizations from OpenChoreo", "error", err)
		return fmt.Errorf("failed to list organizations: %w", err)
	}

	s.logger.Info("Found organizations", "count", len(orgs))

	totalSynced := 0
	totalSkipped := 0
	totalErrors := 0

	// For each organization, fetch all environments and sync them
	for _, org := range orgs {
		synced, skipped, err := s.synchronizeEnvironmentsForOrg(ctx, org.Name)
		if err != nil {
			s.logger.Error("Failed to sync environments for organization", "orgName", org.Name, "error", err)
			totalErrors++
		} else {
			totalSynced += synced
			totalSkipped += skipped
		}
	}

	s.logger.Info("Environment sync completed",
		"totalSynced", totalSynced,
		"totalSkipped", totalSkipped,
		"totalErrors", totalErrors,
	)

	return nil
}

// syncEnvironmentsForOrg syncs all environments for a given organization
func (s *environmentSynchronizer) synchronizeEnvironmentsForOrg(ctx context.Context, orgName string) (synced int, skipped int, err error) {
	s.logger.Debug("Syncing environments for organization", "orgName", orgName)

	// Fetch all environments from OpenChoreo for this organization
	ocEnvs, err := s.ocClient.ListEnvironments(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to list environments from OpenChoreo", "orgName", orgName, "error", err)
		return 0, 0, fmt.Errorf("failed to list environments: %w", err)
	}

	s.logger.Debug("Found environments in OpenChoreo", "orgName", orgName, "count", len(ocEnvs))

	for _, ocEnv := range ocEnvs {
		if syncErr := s.synchronizeEnvironment(ctx, orgName, ocEnv); syncErr != nil {
			if errors.Is(syncErr, gorm.ErrDuplicatedKey) || errors.Is(syncErr, utils.ErrEnvironmentAlreadyExists) {
				skipped++
				s.logger.Debug("Environment already exists in DB", "orgName", orgName, "envName", ocEnv.Name)
			} else {
				s.logger.Error("Failed to sync environment", "orgName", orgName, "envName", ocEnv.Name, "error", syncErr)
			}
		} else {
			synced++
		}
	}

	return synced, skipped, nil
}

// syncEnvironment syncs a single environment from OpenChoreo to the database
func (s *environmentSynchronizer) synchronizeEnvironment(ctx context.Context, orgName string, ocEnv *models.EnvironmentResponse) error {
	// Parse UUID from OpenChoreo
	envUUID, err := uuid.Parse(ocEnv.UUID)
	if err != nil {
		s.logger.Warn("Failed to parse environment UUID from OpenChoreo, generating new one", "envUUID", ocEnv.UUID, "error", err)
		envUUID = uuid.New()
	}

	// Check if environment already exists in DB
	var existing models.Environment
	err = db.DB(ctx).Where("uuid = ?", envUUID).First(&existing).Error
	if err == nil {
		// Environment exists, update it if needed
		existing.DisplayName = ocEnv.DisplayName
		existing.DataplaneRef = ocEnv.DataplaneRef
		existing.DNSPrefix = ocEnv.DNSPrefix
		existing.IsProduction = ocEnv.IsProduction

		if err := db.DB(ctx).Save(&existing).Error; err != nil {
			return fmt.Errorf("failed to update environment: %w", err)
		}
		s.logger.Debug("Environment updated", "orgName", orgName, "envName", ocEnv.Name, "envUUID", envUUID)
		return utils.ErrEnvironmentAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to check existing environment: %w", err)
	}

	// Create new environment in DB
	env := &models.Environment{
		UUID:             envUUID,
		OrganizationName: orgName,
		Name:             ocEnv.Name,
		DisplayName:      ocEnv.DisplayName,
		Description:      "", // OpenChoreo EnvironmentResponse doesn't have description
		DataplaneRef:     ocEnv.DataplaneRef,
		DNSPrefix:        ocEnv.DNSPrefix,
		IsProduction:     ocEnv.IsProduction,
		CreatedAt:        ocEnv.CreatedAt,
		UpdatedAt:        ocEnv.CreatedAt,
	}

	if err := db.DB(ctx).Create(env).Error; err != nil {
		// Handle unique constraint violation
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return utils.ErrEnvironmentAlreadyExists
		}
		return fmt.Errorf("failed to create environment: %w", err)
	}

	s.logger.Info("Environment synced successfully", "orgName", orgName, "envName", ocEnv.Name, "envUUID", envUUID)
	return nil
}
