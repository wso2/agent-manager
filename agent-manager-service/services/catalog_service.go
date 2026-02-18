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

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
)

// CatalogService defines the interface for catalog operations
type CatalogService interface {
	ListCatalog(ctx context.Context, orgUUID string, kind string, limit, offset int) ([]models.CatalogEntry, int64, error)
}

type catalogService struct {
	logger      *slog.Logger
	catalogRepo repositories.CatalogRepository
}

// NewCatalogService creates a new catalog service
func NewCatalogService(logger *slog.Logger, catalogRepo repositories.CatalogRepository) CatalogService {
	return &catalogService{
		logger:      logger,
		catalogRepo: catalogRepo,
	}
}

// ListCatalog retrieves catalog entries filtered by kind and organization
func (s *catalogService) ListCatalog(ctx context.Context, orgUUID string, kind string, limit, offset int) ([]models.CatalogEntry, int64, error) {
	s.logger.Info("Listing catalog entries",
		"orgUUID", orgUUID,
		"kind", kind,
		"limit", limit,
		"offset", offset)

	// Validate orgUUID
	if _, err := uuid.Parse(orgUUID); err != nil {
		s.logger.Error("Invalid organization UUID", "orgUUID", orgUUID, "error", err)
		return nil, 0, fmt.Errorf("invalid organization UUID: %w", err)
	}

	var entries []models.CatalogEntry
	var total int64
	var err error

	// Query based on kind filter
	if kind == "" {
		// No kind filter - return all catalog entries
		entries, total, err = s.catalogRepo.ListAll(orgUUID, limit, offset)
	} else {
		// Filter by specific kind
		entries, total, err = s.catalogRepo.ListByKind(orgUUID, kind, limit, offset)
	}

	if err != nil {
		s.logger.Error("Failed to list catalog entries",
			"orgUUID", orgUUID,
			"kind", kind,
			"error", err)
		return nil, 0, fmt.Errorf("failed to list catalog entries: %w", err)
	}

	s.logger.Info("Successfully listed catalog entries",
		"count", len(entries),
		"total", total)

	return entries, total, nil
}
