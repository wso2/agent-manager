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
	"errors"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
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
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/custom_evaluator_repository_mock.go . CustomEvaluatorRepository:CustomEvaluatorRepositoryMock
type CustomEvaluatorRepository interface {
	WithTx(tx *gorm.DB) CustomEvaluatorRepository
	RunInTransaction(fn func(txRepo CustomEvaluatorRepository) error) error

	Create(evaluator *models.CustomEvaluator) error
	GetByIdentifier(ouID, identifier string) (*models.CustomEvaluator, error)
	List(ouID string, filters CustomEvaluatorFilters) ([]models.CustomEvaluator, int64, error)
	Update(evaluator *models.CustomEvaluator) error
	SoftDelete(evaluator *models.CustomEvaluator) error
	GetByIdentifiers(ouID string, identifiers []string) ([]models.CustomEvaluator, error)
}

// ErrCustomEvaluatorsUnsupported means a custom evaluator was saved; Lite offers only the built-in evaluators.
var ErrCustomEvaluatorsUnsupported = errors.New("custom evaluators are not supported")

// CustomEvaluatorRepo stores no custom evaluators: Lite offers only the built-in ones, so reads find nothing.
type CustomEvaluatorRepo struct{}

// NewCustomEvaluatorRepo creates a new custom evaluator repository
func NewCustomEvaluatorRepo(*gorm.DB) CustomEvaluatorRepository {
	return &CustomEvaluatorRepo{}
}

// WithTx returns the repository; it holds no connection.
func (r *CustomEvaluatorRepo) WithTx(*gorm.DB) CustomEvaluatorRepository { return r }

// RunInTransaction runs fn; nothing it can do needs a transaction.
func (r *CustomEvaluatorRepo) RunInTransaction(fn func(txRepo CustomEvaluatorRepository) error) error {
	return fn(r)
}

func (r *CustomEvaluatorRepo) Create(*models.CustomEvaluator) error {
	return ErrCustomEvaluatorsUnsupported
}

func (r *CustomEvaluatorRepo) GetByIdentifier(string, string) (*models.CustomEvaluator, error) {
	return nil, gorm.ErrRecordNotFound
}

func (r *CustomEvaluatorRepo) List(string, CustomEvaluatorFilters) ([]models.CustomEvaluator, int64, error) {
	return []models.CustomEvaluator{}, 0, nil
}

func (r *CustomEvaluatorRepo) Update(*models.CustomEvaluator) error {
	return ErrCustomEvaluatorsUnsupported
}

func (r *CustomEvaluatorRepo) SoftDelete(*models.CustomEvaluator) error {
	return ErrCustomEvaluatorsUnsupported
}

func (r *CustomEvaluatorRepo) GetByIdentifiers(string, []string) ([]models.CustomEvaluator, error) {
	return []models.CustomEvaluator{}, nil
}
