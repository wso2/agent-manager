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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/catalog"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// EvaluatorManagerService defines the interface for evaluator catalog operations
type EvaluatorManagerService interface {
	ListEvaluators(ctx context.Context, orgID *uuid.UUID, filters EvaluatorFilters) ([]*models.EvaluatorResponse, int32, error)
	GetEvaluator(ctx context.Context, orgID *uuid.UUID, identifier string) (*models.EvaluatorResponse, error)
	GetLLMProvider(ctx context.Context, name string) (*catalog.LLMProviderEntry, error)
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

// ListEvaluators retrieves evaluators from the in-memory catalog.
// orgID is currently unused — only builtin evaluators are supported.
func (s *evaluatorManagerService) ListEvaluators(_ context.Context, _ *uuid.UUID, filters EvaluatorFilters) ([]*models.EvaluatorResponse, int32, error) {
	s.logger.Info("Listing evaluators", "filters", filters)

	all := catalog.List(filters.Tags, filters.Provider, filters.Search)
	total := int32(len(all))

	// Apply pagination
	offset := int(filters.Offset)
	limit := int(filters.Limit)
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}

	if offset >= len(all) {
		return []*models.EvaluatorResponse{}, total, nil
	}

	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	page := all[offset:end]

	responses := make([]*models.EvaluatorResponse, len(page))
	for i, e := range page {
		responses[i] = catalogEntryToResponse(e)
	}

	s.logger.Info("Listed evaluators successfully", "count", len(responses), "total", total)
	return responses, total, nil
}

// GetEvaluator retrieves a single evaluator by identifier from the in-memory catalog.
// orgID is currently unused — only builtin evaluators are supported.
func (s *evaluatorManagerService) GetEvaluator(_ context.Context, _ *uuid.UUID, identifier string) (*models.EvaluatorResponse, error) {
	s.logger.Info("Getting evaluator", "identifier", identifier)

	e := catalog.Get(identifier)
	if e == nil {
		s.logger.Warn("Evaluator not found", "identifier", identifier)
		return nil, utils.ErrEvaluatorNotFound
	}

	s.logger.Info("Retrieved evaluator successfully", "identifier", identifier)
	return catalogEntryToResponse(e), nil
}

// GetLLMProvider retrieves a single LLM provider by name from the in-memory catalog.
func (s *evaluatorManagerService) GetLLMProvider(_ context.Context, name string) (*catalog.LLMProviderEntry, error) {
	s.logger.Info("Getting LLM provider", "name", name)

	p := catalog.GetProvider(name)
	if p == nil {
		s.logger.Warn("LLM provider not found", "name", name)
		return nil, fmt.Errorf("LLM provider %q not found in catalog: %w", name, utils.ErrInvalidInput)
	}

	s.logger.Info("Retrieved LLM provider successfully", "name", name)
	return p, nil
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
