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

package repositories

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// CustomEvaluatorFilters holds filter/pagination options for listing custom evaluators
type CustomEvaluatorFilters struct {
	Limit  int
	Offset int
	Type   string // "code", "llm_judge", or "" for all
	Level  string // "trace", "agent", "llm", or "" for all
	Search string // search across identifier, display_name, description
	Tags   []string
}

// CustomEvaluatorRepository defines the interface for custom evaluator data access
type CustomEvaluatorRepository interface {
	WithTx(tx *gorm.DB) CustomEvaluatorRepository
	RunInTransaction(fn func(txRepo CustomEvaluatorRepository) error) error

	Create(evaluator *models.CustomEvaluator) error
	GetByIdentifier(orgName, identifier string) (*models.CustomEvaluator, error)
	List(orgName string, filters CustomEvaluatorFilters) ([]models.CustomEvaluator, int64, error)
	Update(evaluator *models.CustomEvaluator) error
	SoftDelete(evaluator *models.CustomEvaluator) error
	GetByIdentifiers(orgName string, identifiers []string) ([]models.CustomEvaluator, error)
}

// CustomEvaluatorRepo implements CustomEvaluatorRepository using GORM
type CustomEvaluatorRepo struct {
	db *gorm.DB
}

// NewCustomEvaluatorRepo creates a new custom evaluator repository
func NewCustomEvaluatorRepo(db *gorm.DB) CustomEvaluatorRepository {
	return &CustomEvaluatorRepo{db: db}
}

// WithTx returns a new repository backed by the given transaction
func (r *CustomEvaluatorRepo) WithTx(tx *gorm.DB) CustomEvaluatorRepository {
	return &CustomEvaluatorRepo{db: tx}
}

// RunInTransaction executes fn within a database transaction
func (r *CustomEvaluatorRepo) RunInTransaction(fn func(txRepo CustomEvaluatorRepository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(r.WithTx(tx))
	})
}

// Create creates a new custom evaluator record
func (r *CustomEvaluatorRepo) Create(evaluator *models.CustomEvaluator) error {
	return r.db.Create(evaluator).Error
}

// GetByIdentifier retrieves a custom evaluator by org and identifier (active records only)
func (r *CustomEvaluatorRepo) GetByIdentifier(orgName, identifier string) (*models.CustomEvaluator, error) {
	var evaluator models.CustomEvaluator
	if err := r.db.Where("org_name = ? AND identifier = ? AND deleted_at IS NULL", orgName, identifier).
		First(&evaluator).Error; err != nil {
		return nil, err
	}
	return &evaluator, nil
}

// List returns paginated custom evaluators with optional filters
func (r *CustomEvaluatorRepo) List(orgName string, filters CustomEvaluatorFilters) ([]models.CustomEvaluator, int64, error) {
	query := r.db.Where("org_name = ? AND deleted_at IS NULL", orgName)

	if filters.Type != "" {
		query = query.Where("type = ?", filters.Type)
	}
	if filters.Level != "" {
		query = query.Where("level = ?", filters.Level)
	}
	if filters.Search != "" {
		escaped := escapeLikePattern(filters.Search)
		search := "%" + escaped + "%"
		query = query.Where("(identifier ILIKE ? OR display_name ILIKE ? OR description ILIKE ?)", search, search, search)
	}
	if len(filters.Tags) > 0 {
		query = query.Where("tags @> ?::jsonb", toJSONArray(filters.Tags))
	}

	// Count total matching records
	var total int64
	if err := query.Model(&models.CustomEvaluator{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Fetch paginated results
	var evaluators []models.CustomEvaluator
	paginatedQuery := query.Order("created_at DESC")
	if filters.Limit > 0 {
		paginatedQuery = paginatedQuery.Limit(filters.Limit).Offset(filters.Offset)
	}
	if err := paginatedQuery.Find(&evaluators).Error; err != nil {
		return nil, 0, err
	}

	return evaluators, total, nil
}

// Update saves all fields of the custom evaluator
func (r *CustomEvaluatorRepo) Update(evaluator *models.CustomEvaluator) error {
	return r.db.Save(evaluator).Error
}

// SoftDelete marks a custom evaluator as deleted
func (r *CustomEvaluatorRepo) SoftDelete(evaluator *models.CustomEvaluator) error {
	now := time.Now()
	evaluator.DeletedAt = &now
	return r.db.Save(evaluator).Error
}

// GetByIdentifiers batch-fetches custom evaluators by their identifiers (active records only)
func (r *CustomEvaluatorRepo) GetByIdentifiers(orgName string, identifiers []string) ([]models.CustomEvaluator, error) {
	if len(identifiers) == 0 {
		return nil, nil
	}
	var evaluators []models.CustomEvaluator
	err := r.db.Where("org_name = ? AND identifier IN ? AND deleted_at IS NULL", orgName, identifiers).
		Find(&evaluators).Error
	return evaluators, err
}

// escapeLikePattern escapes SQL LIKE/ILIKE special characters (%, _, \) in user input
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// toJSONArray converts a string slice to a JSON array string for JSONB containment queries
func toJSONArray(tags []string) string {
	b, err := json.Marshal(tags)
	if err != nil {
		return "[]"
	}
	return string(b)
}
