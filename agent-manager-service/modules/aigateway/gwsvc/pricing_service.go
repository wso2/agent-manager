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

package gwsvc

import (
	"context"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

const (
	pricingDefaultLimit = 50
	pricingMaxLimit     = 200
)

// PricingSaved is what a pushed price file did: the entries it listed, the entries it
// skipped as unusable, and the models it added to the catalog.
type PricingSaved struct{ Sent, Skipped, Added int }

// PricingQuery is a page of the catalog: models whose name contains Search, of one Provider.
// A Limit of 0 is the default page size.
type PricingQuery struct {
	Search, Provider string
	// Sort is "model" (the default) or "provider".
	Sort          string
	Limit, Offset int
}

// PricingService keeps the global model price catalog gateways send, and serves it read-only.
type PricingService interface {
	// Save sanitizes a gateway's price file and adds the models not stored yet. It returns
	// a *PricingError for a file it refuses.
	Save(ctx context.Context, body []byte) (PricingSaved, error)
	// Catalog returns a page of the catalog by model name, with every provider and when
	// the newest model was added.
	Catalog(ctx context.Context, q PricingQuery) (*gwspec.ModelPriceList, error)
}

type pricingService struct {
	repo gwrepo.ModelPriceRepository
}

// NewPricingService creates a PricingService.
func NewPricingService(repo gwrepo.ModelPriceRepository) PricingService {
	return &pricingService{repo: repo}
}

func (s *pricingService) Save(ctx context.Context, body []byte) (PricingSaved, error) {
	clean, err := SanitizePricing(body)
	if err != nil {
		return PricingSaved{}, err
	}
	added, err := s.repo.InsertMissing(ctx, clean.Rows)
	if err != nil {
		return PricingSaved{}, err
	}
	return PricingSaved{Sent: clean.Sent, Skipped: clean.Skipped, Added: added}, nil
}

func (s *pricingService) Catalog(ctx context.Context, q PricingQuery) (*gwspec.ModelPriceList, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = pricingDefaultLimit
	}
	limit = min(limit, pricingMaxLimit)
	offset := max(q.Offset, 0)
	rows, total, err := s.repo.List(ctx, gwrepo.ModelPriceFilter{Search: q.Search, Provider: q.Provider, ByProvider: q.Sort == "provider", Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	providers, err := s.repo.Providers(ctx)
	if err != nil {
		return nil, err
	}
	newest, err := s.repo.Newest(ctx)
	if err != nil {
		return nil, err
	}
	models := make([]gwspec.ModelPrice, 0, len(rows))
	for _, r := range rows {
		models = append(models, modelPriceResponse(r))
	}
	out := gwspec.NewModelPriceList(append([]string{}, providers...), int32(total), int32(limit), int32(offset), models) //nolint:gosec // paging bounds
	out.UpdatedAt = *gwspec.NewNullableTime(newest)
	return out, nil
}

// modelPriceResponse is a stored model in the API's shape: a price the file lacked is null.
func modelPriceResponse(r gwmodels.ModelPrice) gwspec.ModelPrice {
	m := gwspec.NewModelPrice(r.Model)
	m.Provider = *gwspec.NewNullableString(r.Provider)
	m.InputPer1M = *gwspec.NewNullableString(r.InputPerMillion)
	m.OutputPer1M = *gwspec.NewNullableString(r.OutputPerMillion)
	m.CachedInputPer1M = *gwspec.NewNullableString(r.CachedInputPerMillion)
	return *m
}
