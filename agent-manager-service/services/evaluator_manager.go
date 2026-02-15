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

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// EvaluatorManagerService defines the interface for evaluator catalog operations
type EvaluatorManagerService interface {
	ListEvaluators(ctx context.Context, orgID *uuid.UUID, filters EvaluatorFilters) ([]*models.EvaluatorResponse, int32, error)
	GetEvaluator(ctx context.Context, orgID *uuid.UUID, identifier string) (*models.EvaluatorResponse, error)
}

// EvaluatorFilters contains filtering options for listing evaluators
type EvaluatorFilters struct {
	Limit    int32
	Offset   int32
	Tags     []string
	Search   string
	Provider string
}

type evaluatorManagerService struct {
	logger *slog.Logger
}

// NewEvaluatorManagerService creates a new evaluator manager service instance
func NewEvaluatorManagerService(logger *slog.Logger) EvaluatorManagerService {
	return &evaluatorManagerService{
		logger: logger,
	}
}

// ListEvaluators retrieves evaluators from the catalog
// Returns both builtin evaluators (org_id IS NULL) and organization-specific evaluators
func (s *evaluatorManagerService) ListEvaluators(ctx context.Context, orgID *uuid.UUID, filters EvaluatorFilters) ([]*models.EvaluatorResponse, int32, error) {
	s.logger.Info("Listing evaluators", "orgID", orgID, "filters", filters)

	dbConn := db.DB(ctx)
	var evaluators []models.Evaluator

	// Build query: Return builtins (org_id IS NULL) + org-specific (org_id = orgID)
	query := dbConn.Model(&models.Evaluator{})

	// Filter by org: include builtins (org_id IS NULL) or org-specific
	if orgID != nil {
		query = query.Where("org_id IS NULL OR org_id = ?", orgID)
	} else {
		// If no org specified, only return builtins
		query = query.Where("org_id IS NULL")
	}

	// Filter by tags if provided (ANY match using JSONB contains operator)
	if len(filters.Tags) > 0 {
		for _, tag := range filters.Tags {
			query = query.Where("tags @> jsonb_build_array(?::text)", tag)
		}
	}

	// Filter by provider if provided
	if filters.Provider != "" {
		query = query.Where("provider = ?", filters.Provider)
	}

	// Full-text search on display_name and description (uses idx_evaluator_search GIN index)
	if filters.Search != "" {
		query = query.Where(
			"to_tsvector('english', display_name || ' ' || description) @@ plainto_tsquery('english', ?)",
			filters.Search,
		)
	}

	// Get total count before pagination
	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		s.logger.Error("Failed to count evaluators", "error", err)
		return nil, 0, fmt.Errorf("failed to count evaluators: %w", err)
	}

	// Apply pagination and ordering
	query = query.Order("is_builtin DESC, display_name ASC").
		Limit(int(filters.Limit)).
		Offset(int(filters.Offset))

	// Execute query
	if err := query.Find(&evaluators).Error; err != nil {
		s.logger.Error("Failed to list evaluators", "error", err)
		return nil, 0, fmt.Errorf("failed to list evaluators: %w", err)
	}

	// Convert to response DTOs
	responses := make([]*models.EvaluatorResponse, len(evaluators))
	for i, evaluator := range evaluators {
		responses[i] = evaluator.ToResponse()
	}

	s.logger.Info("Listed evaluators successfully", "count", len(responses), "total", totalCount)
	return responses, int32(totalCount), nil
}

// GetEvaluator retrieves a single evaluator by identifier
// Searches in both builtins and org-specific evaluators
func (s *evaluatorManagerService) GetEvaluator(ctx context.Context, orgID *uuid.UUID, identifier string) (*models.EvaluatorResponse, error) {
	s.logger.Info("Getting evaluator", "identifier", identifier, "orgID", orgID)

	dbConn := db.DB(ctx)
	var evaluator models.Evaluator

	// Build query: Try org-specific first, fallback to builtin
	query := dbConn.Model(&models.Evaluator{}).Where("identifier = ?", identifier)

	if orgID != nil {
		// Prioritize org-specific over builtin
		query = query.Where("org_id IS NULL OR org_id = ?", orgID).
			Order("org_id DESC NULLS LAST") // org-specific first
	} else {
		// Only return builtins if no org specified
		query = query.Where("org_id IS NULL")
	}

	// Execute query
	if err := query.First(&evaluator).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.logger.Warn("Evaluator not found", "identifier", identifier)
			return nil, utils.ErrEvaluatorNotFound
		}
		s.logger.Error("Failed to get evaluator", "identifier", identifier, "error", err)
		return nil, fmt.Errorf("failed to get evaluator: %w", err)
	}

	s.logger.Info("Retrieved evaluator successfully", "identifier", identifier)
	return evaluator.ToResponse(), nil
}
