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
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
)

func TestPricingSaveSanitizesThenAddsTheMissingModels(t *testing.T) {
	repo := &gwrepomocks.ModelPriceRepositoryMock{
		InsertMissingFunc: func(_ context.Context, rows []gwmodels.ModelPrice) (int, error) { return len(rows) - 1, nil },
	}
	saved, err := NewPricingService(repo).Save(context.Background(),
		[]byte(`{"a": {"input_cost_per_token": 1e-06}, "b": {}, "c": 3}`))
	require.NoError(t, err)
	assert.Equal(t, PricingSaved{Sent: 3, Skipped: 1, Added: 1}, saved)
	require.Len(t, repo.InsertMissingCalls(), 1)
	rows := repo.InsertMissingCalls()[0].Rows
	require.Len(t, rows, 2)
	assert.Equal(t, "a", rows[0].Model)
	assert.Equal(t, str("1.00"), rows[0].InputPerMillion)
}

func TestPricingSaveWritesNothingForARefusedFile(t *testing.T) {
	repo := &gwrepomocks.ModelPriceRepositoryMock{}
	_, err := NewPricingService(repo).Save(context.Background(), []byte(`[]`))
	var refused *PricingError
	require.ErrorAs(t, err, &refused)
	assert.Empty(t, repo.InsertMissingCalls())
}

func TestPricingSavePassesAStorageFailureOn(t *testing.T) {
	boom := errors.New("database is down")
	repo := &gwrepomocks.ModelPriceRepositoryMock{
		InsertMissingFunc: func(context.Context, []gwmodels.ModelPrice) (int, error) { return 0, boom },
	}
	_, err := NewPricingService(repo).Save(context.Background(), []byte(`{"a": {}}`))
	require.ErrorIs(t, err, boom)
	var refused *PricingError
	assert.False(t, errors.As(err, &refused), "a storage failure is not the file's fault")
}

func catalogRepo(rows []gwmodels.ModelPrice, total int64, newest *time.Time) *gwrepomocks.ModelPriceRepositoryMock {
	return &gwrepomocks.ModelPriceRepositoryMock{
		ListFunc: func(context.Context, gwrepo.ModelPriceFilter) ([]gwmodels.ModelPrice, int64, error) {
			return rows, total, nil
		},
		ProvidersFunc: func(context.Context) ([]string, error) { return []string{"azure", "openai"}, nil },
		NewestFunc:    func(context.Context) (*time.Time, error) { return newest, nil },
	}
}

func TestPricingCatalogPagesAndFilters(t *testing.T) {
	repo := catalogRepo(nil, 0, nil)
	svc := NewPricingService(repo)
	for _, c := range []struct {
		name                  string
		in                    PricingQuery
		wantLimit, wantOffset int
	}{
		{"defaults", PricingQuery{}, 50, 0},
		{"a page", PricingQuery{Limit: 20, Offset: 40}, 20, 40},
		{"the cap", PricingQuery{Limit: 5000}, 200, 0},
		{"a negative offset", PricingQuery{Limit: 10, Offset: -5}, 10, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, err := svc.Catalog(context.Background(), c.in)
			require.NoError(t, err)
			assert.EqualValues(t, c.wantLimit, out.Limit)
			assert.EqualValues(t, c.wantOffset, out.Offset)
			calls := repo.ListCalls()
			assert.Equal(t, c.wantLimit, calls[len(calls)-1].F.Limit)
			assert.Equal(t, c.wantOffset, calls[len(calls)-1].F.Offset)
		})
	}
	_, err := svc.Catalog(context.Background(), PricingQuery{Search: "gpt", Provider: "openai"})
	require.NoError(t, err)
	last := repo.ListCalls()[len(repo.ListCalls())-1].F
	assert.Equal(t, "gpt", last.Search)
	assert.Equal(t, "openai", last.Provider)
}

func TestPricingCatalogShowsAMissingPriceAsNull(t *testing.T) {
	newest := time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)
	repo := catalogRepo([]gwmodels.ModelPrice{
		{Model: "gpt-4o", Provider: str("openai"), InputPerMillion: str("2.50"), OutputPerMillion: str("10.00"), CachedInputPerMillion: str("1.25")},
		{Model: "dall-e-2"},
	}, 2, &newest)
	out, err := NewPricingService(repo).Catalog(context.Background(), PricingQuery{})
	require.NoError(t, err)
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, "2026-11-25T08:00:00Z", got["updatedAt"])
	assert.Equal(t, []any{"azure", "openai"}, got["providers"])
	assert.EqualValues(t, 2, got["total"])
	models := got["models"].([]any)
	first := models[0].(map[string]any)
	assert.Equal(t, "2.50", first["inputPer1M"])
	assert.Equal(t, "1.25", first["cachedInputPer1M"])
	second := models[1].(map[string]any)
	for _, k := range []string{"provider", "inputPer1M", "outputPer1M", "cachedInputPer1M"} {
		v, present := second[k]
		assert.True(t, present, "%s is listed, as null", k)
		assert.Nil(t, v, k)
	}
}

func TestPricingCatalogIsEmptyBeforeAnyGatewaySends(t *testing.T) {
	out, err := NewPricingService(catalogRepo(nil, 0, nil)).Catalog(context.Background(), PricingQuery{})
	require.NoError(t, err)
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Nil(t, got["updatedAt"])
	assert.Equal(t, []any{}, got["models"])
}
