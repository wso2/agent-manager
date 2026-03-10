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
	"regexp"
	"strings"

	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/catalog"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// EvaluatorManagerService defines the interface for evaluator catalog and custom evaluator operations
type EvaluatorManagerService interface {
	// Catalog operations (built-in + custom merged)
	ListEvaluators(ctx context.Context, orgName string, filters EvaluatorFilters) ([]*models.EvaluatorResponse, int32, error)
	GetEvaluator(ctx context.Context, orgName string, identifier string) (*models.EvaluatorResponse, error)
	GetLLMProvider(ctx context.Context, name string) (*catalog.LLMProviderEntry, error)

	// Custom evaluator CRUD
	CreateCustomEvaluator(ctx context.Context, orgName string, req *models.CreateCustomEvaluatorRequest) (*models.EvaluatorResponse, error)
	GetCustomEvaluator(ctx context.Context, orgName string, identifier string) (*models.EvaluatorResponse, error)
	UpdateCustomEvaluator(ctx context.Context, orgName string, identifier string, req *models.UpdateCustomEvaluatorRequest) (*models.EvaluatorResponse, error)
	DeleteCustomEvaluator(ctx context.Context, orgName string, identifier string) error

	// Resolve custom evaluators for monitor execution
	ResolveCustomEvaluators(ctx context.Context, orgName string, identifiers []string) ([]models.CustomEvaluator, error)
}

// EvaluatorFilters contains filtering options for listing evaluators
type EvaluatorFilters struct {
	Limit    int32
	Offset   int32
	Tags     []string
	Search   string
	Provider string
	Source   string // "all", "builtin", "custom"
}

type evaluatorManagerService struct {
	logger   *slog.Logger
	custRepo repositories.CustomEvaluatorRepository
}

// NewEvaluatorManagerService creates a new evaluator manager service instance
func NewEvaluatorManagerService(logger *slog.Logger, custRepo repositories.CustomEvaluatorRepository) EvaluatorManagerService {
	return &evaluatorManagerService{
		logger:   logger,
		custRepo: custRepo,
	}
}

// ListEvaluators retrieves evaluators from both the built-in catalog and custom evaluator DB.
// Results are merged, filtered, and paginated.
func (s *evaluatorManagerService) ListEvaluators(ctx context.Context, orgName string, filters EvaluatorFilters) ([]*models.EvaluatorResponse, int32, error) {
	s.logger.Info("Listing evaluators", "orgName", orgName, "filters", filters)

	var merged []*models.EvaluatorResponse

	// Include custom evaluators first (unless source=builtin)
	if filters.Source != "builtin" {
		customFilters := repositories.CustomEvaluatorFilters{
			Search: filters.Search,
			Tags:   filters.Tags,
		}
		// Map provider filter to type for custom evaluators
		if filters.Provider == models.CustomProviderCode {
			customFilters.Type = models.CustomEvaluatorTypeCode
		} else if filters.Provider == models.CustomProviderLLMJudge {
			customFilters.Type = models.CustomEvaluatorTypeLLMJudge
		}

		customs, _, err := s.custRepo.List(orgName, customFilters)
		if err != nil {
			s.logger.Error("Failed to list custom evaluators", "error", err)
			return nil, 0, fmt.Errorf("failed to list custom evaluators: %w", err)
		}
		for i := range customs {
			merged = append(merged, customs[i].ToEvaluatorResponse())
		}
	}

	// Include built-in evaluators after custom ones (unless source=custom)
	if filters.Source != "custom" {
		builtins := catalog.List(filters.Tags, filters.Provider, filters.Search)
		for _, e := range builtins {
			merged = append(merged, catalogEntryToResponse(e))
		}
	}

	total := int32(len(merged))

	// Apply pagination
	offset := int(filters.Offset)
	limit := int(filters.Limit)
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = len(merged)
	}

	if offset >= len(merged) {
		return []*models.EvaluatorResponse{}, total, nil
	}

	end := offset + limit
	if end > len(merged) {
		end = len(merged)
	}
	page := merged[offset:end]

	s.logger.Info("Listed evaluators successfully", "count", len(page), "total", total)
	return page, total, nil
}

// GetEvaluator retrieves a single evaluator by identifier.
// Checks the built-in catalog first, then falls back to custom evaluators.
func (s *evaluatorManagerService) GetEvaluator(ctx context.Context, orgName string, identifier string) (*models.EvaluatorResponse, error) {
	s.logger.Info("Getting evaluator", "identifier", identifier)

	// Check built-in catalog first
	e := catalog.Get(identifier)
	if e != nil {
		s.logger.Info("Retrieved built-in evaluator", "identifier", identifier)
		return catalogEntryToResponse(e), nil
	}

	// Fall back to custom evaluator
	custom, err := s.custRepo.GetByIdentifier(orgName, identifier)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.logger.Warn("Evaluator not found", "identifier", identifier)
			return nil, utils.ErrEvaluatorNotFound
		}
		return nil, fmt.Errorf("failed to get custom evaluator: %w", err)
	}

	s.logger.Info("Retrieved custom evaluator", "identifier", identifier)
	return custom.ToEvaluatorResponse(), nil
}

// GetLLMProvider retrieves a single LLM provider by name from the in-memory catalog.
func (s *evaluatorManagerService) GetLLMProvider(_ context.Context, name string) (*catalog.LLMProviderEntry, error) {
	s.logger.Info("Getting LLM provider", "name", name)

	p := catalog.GetProvider(name)
	if p == nil {
		s.logger.Warn("LLM provider not found", "name", name)
		return nil, fmt.Errorf("LLM provider %q not found in catalog: %w", name, utils.ErrEvaluatorNotFound)
	}

	s.logger.Info("Retrieved LLM provider successfully", "name", name)
	return p, nil
}

// CreateCustomEvaluator creates a new custom evaluator
func (s *evaluatorManagerService) CreateCustomEvaluator(ctx context.Context, orgName string, req *models.CreateCustomEvaluatorRequest) (*models.EvaluatorResponse, error) {
	s.logger.Info("Creating custom evaluator", "orgName", orgName, "displayName", req.DisplayName, "type", req.Type)

	// Generate identifier from display name if not provided
	identifier := req.Identifier
	if identifier == "" {
		identifier = slugify(req.DisplayName)
	}

	// Validate identifier doesn't clash with built-in evaluators
	if catalog.Get(identifier) != nil {
		return nil, utils.ErrCustomEvaluatorIdentifierTaken
	}

	// Validate type-specific constraints
	if req.Type == models.CustomEvaluatorTypeLLMJudge && req.Dependencies != nil {
		return nil, fmt.Errorf("LLM-judge evaluators cannot have dependencies: %w", utils.ErrInvalidInput)
	}

	evaluator := &models.CustomEvaluator{
		OrgName:      orgName,
		Identifier:   identifier,
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		Type:         req.Type,
		Level:        req.Level,
		Source:       req.Source,
		Dependencies: req.Dependencies,
		ConfigSchema: req.ConfigSchema,
		Tags:         req.Tags,
	}

	if evaluator.ConfigSchema == nil {
		evaluator.ConfigSchema = []models.EvaluatorConfigParam{}
	}
	if evaluator.Tags == nil {
		evaluator.Tags = []string{}
	}

	if err := s.custRepo.Create(evaluator); err != nil {
		if strings.Contains(err.Error(), "uq_custom_evaluator_org_identifier") {
			return nil, utils.ErrCustomEvaluatorAlreadyExists
		}
		return nil, fmt.Errorf("failed to create custom evaluator: %w", err)
	}

	s.logger.Info("Created custom evaluator", "identifier", identifier)
	return evaluator.ToEvaluatorResponse(), nil
}

// GetCustomEvaluator retrieves a custom evaluator by identifier
func (s *evaluatorManagerService) GetCustomEvaluator(ctx context.Context, orgName string, identifier string) (*models.EvaluatorResponse, error) {
	s.logger.Info("Getting custom evaluator", "orgName", orgName, "identifier", identifier)

	custom, err := s.custRepo.GetByIdentifier(orgName, identifier)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrCustomEvaluatorNotFound
		}
		return nil, fmt.Errorf("failed to get custom evaluator: %w", err)
	}

	return custom.ToEvaluatorResponse(), nil
}

// UpdateCustomEvaluator updates an existing custom evaluator
func (s *evaluatorManagerService) UpdateCustomEvaluator(ctx context.Context, orgName string, identifier string, req *models.UpdateCustomEvaluatorRequest) (*models.EvaluatorResponse, error) {
	s.logger.Info("Updating custom evaluator", "orgName", orgName, "identifier", identifier)

	evaluator, err := s.custRepo.GetByIdentifier(orgName, identifier)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrCustomEvaluatorNotFound
		}
		return nil, fmt.Errorf("failed to get custom evaluator: %w", err)
	}

	// Validate type-specific constraints
	if evaluator.Type == models.CustomEvaluatorTypeLLMJudge && req.Dependencies != nil {
		return nil, fmt.Errorf("LLM-judge evaluators cannot have dependencies: %w", utils.ErrInvalidInput)
	}

	// Apply updates
	if req.DisplayName != nil {
		evaluator.DisplayName = *req.DisplayName
	}
	if req.Description != nil {
		evaluator.Description = *req.Description
	}
	if req.Source != nil {
		evaluator.Source = *req.Source
	}
	if req.Dependencies != nil {
		evaluator.Dependencies = req.Dependencies
	}
	if req.ConfigSchema != nil {
		evaluator.ConfigSchema = *req.ConfigSchema
	}
	if req.Tags != nil {
		evaluator.Tags = *req.Tags
	}

	if err := s.custRepo.Update(evaluator); err != nil {
		return nil, fmt.Errorf("failed to update custom evaluator: %w", err)
	}

	s.logger.Info("Updated custom evaluator", "identifier", identifier)
	return evaluator.ToEvaluatorResponse(), nil
}

// DeleteCustomEvaluator soft-deletes a custom evaluator
func (s *evaluatorManagerService) DeleteCustomEvaluator(ctx context.Context, orgName string, identifier string) error {
	s.logger.Info("Deleting custom evaluator", "orgName", orgName, "identifier", identifier)

	evaluator, err := s.custRepo.GetByIdentifier(orgName, identifier)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrCustomEvaluatorNotFound
		}
		return fmt.Errorf("failed to get custom evaluator: %w", err)
	}

	if err := s.custRepo.SoftDelete(evaluator); err != nil {
		return fmt.Errorf("failed to delete custom evaluator: %w", err)
	}

	s.logger.Info("Deleted custom evaluator", "identifier", identifier)
	return nil
}

// ResolveCustomEvaluators batch-fetches custom evaluators by identifiers for monitor execution
func (s *evaluatorManagerService) ResolveCustomEvaluators(ctx context.Context, orgName string, identifiers []string) ([]models.CustomEvaluator, error) {
	if len(identifiers) == 0 {
		return nil, nil
	}
	return s.custRepo.GetByIdentifiers(orgName, identifiers)
}

// catalogEntryToResponse converts a catalog.Entry to an EvaluatorResponse DTO.
func catalogEntryToResponse(e *catalog.Entry) *models.EvaluatorResponse {
	return &models.EvaluatorResponse{
		ID:           e.ID(),
		Identifier:   e.Identifier,
		DisplayName:  e.DisplayName,
		Description:  e.Description,
		Version:      e.Version,
		Provider:     e.Provider,
		Level:        e.Level,
		Tags:         e.Tags,
		IsBuiltin:    true,
		ConfigSchema: e.ConfigSchema,
	}
}

// slugify converts a display name to a URL-friendly identifier
var slugifyRegex = regexp.MustCompile(`[^a-z0-9-]+`)

func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugifyRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 128 {
		s = s[:128]
	}
	return s
}
