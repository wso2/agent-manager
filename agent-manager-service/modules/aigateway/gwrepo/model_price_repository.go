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

package gwrepo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
)

// ModelPriceFilter is a page of the catalog: models whose name contains Search
// (case-insensitive), of one Provider, by model name.
type ModelPriceFilter struct {
	Search, Provider string
	// ByProvider lists providers A to Z (models without one last), then by model name.
	ByProvider    bool
	Limit, Offset int
}

// ModelPriceRepository keeps the global model price catalog.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg gwrepomocks -out gwrepomocks/model_price_repository_mock.go . ModelPriceRepository:ModelPriceRepositoryMock
type ModelPriceRepository interface {
	// InsertMissing stores the models not stored yet and leaves the others as they are,
	// in one transaction. It returns how many it added.
	InsertMissing(ctx context.Context, rows []gwmodels.ModelPrice) (int, error)
	// List returns a page of the catalog and how many models match the filter.
	List(ctx context.Context, f ModelPriceFilter) ([]gwmodels.ModelPrice, int64, error)
	// Providers returns the distinct providers of the whole catalog, sorted.
	Providers(ctx context.Context) ([]string, error)
	// Newest returns when the newest model was added, or nil with an empty catalog.
	Newest(ctx context.Context) (*time.Time, error)
}

type modelPriceRepository struct {
	db *gorm.DB
}

// NewModelPriceRepository creates a ModelPriceRepository.
func NewModelPriceRepository(db *gorm.DB) ModelPriceRepository {
	return &modelPriceRepository{db: db}
}

func (r *modelPriceRepository) InsertMissing(ctx context.Context, rows []gwmodels.ModelPrice) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	now := dbx.Now()
	added := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Most pushes find every model stored: read which are, so those cost no write.
		stored := make(map[string]bool, len(rows))
		names := make([]string, 0, len(rows))
		for _, row := range rows {
			names = append(names, row.Model)
		}
		for _, chunk := range dbx.Chunks(names) {
			var found []string
			if err := tx.Model(&gwmodels.ModelPrice{}).Where("model IN ?", chunk).Pluck("model", &found).Error; err != nil {
				return fmt.Errorf("failed to read stored model prices: %w", err)
			}
			for _, name := range found {
				stored[name] = true
			}
		}
		// A model another push stored meanwhile is not an error: the first one stays.
		for _, row := range rows {
			if stored[row.Model] {
				continue
			}
			row.CreatedAt = now
			inserted, err := dbx.InsertIfAbsent(tx, &row)
			if err != nil {
				return fmt.Errorf("failed to store model price: %w", err)
			}
			if inserted {
				added++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return added, nil
}

// likeEscape is the escape character of a search's LIKE pattern.
const likeEscape = "!"

func (r *modelPriceRepository) filtered(ctx context.Context, f ModelPriceFilter) *gorm.DB {
	q := r.db.WithContext(ctx).Model(&gwmodels.ModelPrice{})
	if search := strings.ToLower(strings.TrimSpace(f.Search)); search != "" {
		escaped := strings.NewReplacer(likeEscape, likeEscape+likeEscape, "%", likeEscape+"%", "_", likeEscape+"_").Replace(search)
		q = q.Where("LOWER(model) LIKE ? ESCAPE '"+likeEscape+"'", "%"+escaped+"%")
	}
	if provider := strings.TrimSpace(f.Provider); provider != "" {
		q = q.Where("provider = ?", provider)
	}
	return q
}

func (r *modelPriceRepository) List(ctx context.Context, f ModelPriceFilter) ([]gwmodels.ModelPrice, int64, error) {
	var total int64
	if err := r.filtered(ctx, f).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count model prices: %w", err)
	}
	var rows []gwmodels.ModelPrice
	q := r.filtered(ctx, f)
	if f.ByProvider {
		q = q.Order("(provider IS NULL OR provider = '') ASC").Order("COALESCE(provider, '') ASC")
	}
	q = q.Order("model ASC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list model prices: %w", err)
	}
	return rows, total, nil
}

func (r *modelPriceRepository) Providers(ctx context.Context) ([]string, error) {
	var out []string
	err := r.db.WithContext(ctx).Model(&gwmodels.ModelPrice{}).
		Where("provider IS NOT NULL AND provider <> ''").Distinct().Order("provider ASC").Pluck("provider", &out).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list model price providers: %w", err)
	}
	return out, nil
}

func (r *modelPriceRepository) Newest(ctx context.Context) (*time.Time, error) {
	var row gwmodels.ModelPrice
	err := r.db.WithContext(ctx).Order("created_at DESC").Limit(1).Take(&row).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the newest model price: %w", err)
	}
	t := row.CreatedAt.UTC()
	return &t, nil
}
