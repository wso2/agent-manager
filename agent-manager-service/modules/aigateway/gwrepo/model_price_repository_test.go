//go:build integration

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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// emptyCatalog clears the global price catalog, now and when the test ends: the table
// belongs to no organization, so a test starts from nothing and leaves nothing.
func emptyCatalog(t *testing.T) ModelPriceRepository {
	t.Helper()
	clear := func() { require.NoError(t, db.GetDB().Exec("DELETE FROM mod_ai_gateway_model_prices").Error) }
	clear()
	t.Cleanup(clear)
	return NewModelPriceRepository(db.GetDB())
}

func s(v string) *string { return &v }

func TestModelPricesInsertMissingKeepsTheFirstOne(t *testing.T) {
	repo := emptyCatalog(t)
	ctx := context.Background()

	added, err := repo.InsertMissing(ctx, []gwmodels.ModelPrice{
		{Model: "gpt-4o", Provider: s("openai"), InputPerMillion: s("2.50")},
		{Model: "gpt-4o-mini", Provider: s("openai"), InputPerMillion: s("0.15")},
		{Model: "dall-e-2"},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, added)

	// A second push: other prices for a stored model, and one new model.
	added, err = repo.InsertMissing(ctx, []gwmodels.ModelPrice{
		{Model: "gpt-4o", Provider: s("azure"), InputPerMillion: s("99.00"), OutputPerMillion: s("1.00")},
		{Model: "gpt-4.1", Provider: s("openai"), InputPerMillion: s("2.00")},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, added, "only the model not stored yet is added")

	rows, total, err := repo.List(ctx, ModelPriceFilter{})
	require.NoError(t, err)
	assert.EqualValues(t, 4, total)
	byModel := map[string]gwmodels.ModelPrice{}
	for _, r := range rows {
		byModel[r.Model] = r
	}
	first := byModel["gpt-4o"]
	assert.Equal(t, s("openai"), first.Provider, "the stored model is left as it was")
	assert.Equal(t, s("2.50"), first.InputPerMillion)
	assert.Nil(t, first.OutputPerMillion, "a price the first file lacked is not filled in later")
	assert.Nil(t, byModel["dall-e-2"].InputPerMillion)
	assert.Equal(t, s("2.00"), byModel["gpt-4.1"].InputPerMillion)
}

func TestModelPricesInsertNothingIsFine(t *testing.T) {
	repo := emptyCatalog(t)
	added, err := repo.InsertMissing(context.Background(), nil)
	require.NoError(t, err)
	assert.Zero(t, added)
	_, total, err := repo.List(context.Background(), ModelPriceFilter{})
	require.NoError(t, err)
	assert.Zero(t, total)
}

func TestModelPricesInsertInBatches(t *testing.T) {
	repo := emptyCatalog(t)
	ctx := context.Background()
	rows := make([]gwmodels.ModelPrice, 0, 841)
	for i := 0; i < 841; i++ {
		rows = append(rows, gwmodels.ModelPrice{Model: fmt.Sprintf("model-%04d", i), Provider: s(fmt.Sprintf("p%d", i%15)), InputPerMillion: s("1.00")})
	}
	added, err := repo.InsertMissing(ctx, rows)
	require.NoError(t, err)
	assert.Equal(t, 841, added, "more than one batch")

	added, err = repo.InsertMissing(ctx, rows)
	require.NoError(t, err)
	assert.Zero(t, added, "the same file again adds nothing")

	_, total, err := repo.List(ctx, ModelPriceFilter{Limit: 1})
	require.NoError(t, err)
	assert.EqualValues(t, 841, total)
	providers, err := repo.Providers(ctx)
	require.NoError(t, err)
	assert.Len(t, providers, 15)
}

func seedSearchable(t *testing.T) ModelPriceRepository {
	t.Helper()
	repo := emptyCatalog(t)
	_, err := repo.InsertMissing(context.Background(), []gwmodels.ModelPrice{
		{Model: "azure/eu/GPT-4o", Provider: s("azure")},
		{Model: "gpt-4o", Provider: s("openai")},
		{Model: "gpt-4o-mini", Provider: s("openai")},
		{Model: "claude-3", Provider: s("anthropic")},
		{Model: "a_b", Provider: s("x")},
		{Model: "aXb", Provider: s("x")},
		{Model: "100%", Provider: s("")},
		{Model: "no-provider"},
	})
	require.NoError(t, err)
	return repo
}

func modelNames(rows []gwmodels.ModelPrice) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Model)
	}
	return out
}

func TestModelPricesSearchAndProviderFilter(t *testing.T) {
	repo := seedSearchable(t)
	ctx := context.Background()
	list := func(f ModelPriceFilter) ([]string, int64) {
		rows, total, err := repo.List(ctx, f)
		require.NoError(t, err)
		return modelNames(rows), total
	}

	names, total := list(ModelPriceFilter{Search: "GPT-4O"})
	assert.Equal(t, []string{"azure/eu/GPT-4o", "gpt-4o", "gpt-4o-mini"}, names, "any case, by model name")
	assert.EqualValues(t, 3, total)

	names, _ = list(ModelPriceFilter{Search: "gpt", Provider: "openai"})
	assert.Equal(t, []string{"gpt-4o", "gpt-4o-mini"}, names)

	names, _ = list(ModelPriceFilter{Provider: "anthropic"})
	assert.Equal(t, []string{"claude-3"}, names)

	names, total = list(ModelPriceFilter{Search: "nothing like this"})
	assert.Empty(t, names)
	assert.Zero(t, total)

	names, _ = list(ModelPriceFilter{Search: "a_b"})
	assert.Equal(t, []string{"a_b"}, names, "an underscore is a character, not a wildcard")
	names, _ = list(ModelPriceFilter{Search: "%"})
	assert.Equal(t, []string{"100%"}, names, "a percent sign is a character, not a wildcard")
	names, _ = list(ModelPriceFilter{Search: "!"})
	assert.Empty(t, names, "the escape character itself is safe")
	names, _ = list(ModelPriceFilter{Search: "  claude  "})
	assert.Equal(t, []string{"claude-3"}, names, "the search is trimmed")
}

func TestModelPricesPaging(t *testing.T) {
	repo := seedSearchable(t)
	ctx := context.Background()
	all, total, err := repo.List(ctx, ModelPriceFilter{})
	require.NoError(t, err)
	assert.EqualValues(t, 8, total)
	require.Len(t, all, 8)
	for i := 1; i < len(all); i++ {
		assert.Less(t, all[i-1].Model, all[i].Model, "by model name")
	}

	page, total, err := repo.List(ctx, ModelPriceFilter{Limit: 3, Offset: 2})
	require.NoError(t, err)
	assert.EqualValues(t, 8, total, "the total counts the whole filter")
	assert.Equal(t, modelNames(all[2:5]), modelNames(page))

	page, _, err = repo.List(ctx, ModelPriceFilter{Limit: 3, Offset: 7})
	require.NoError(t, err)
	assert.Equal(t, modelNames(all[7:]), modelNames(page), "a short last page")

	page, _, err = repo.List(ctx, ModelPriceFilter{Limit: 3, Offset: 100})
	require.NoError(t, err)
	assert.Empty(t, page)
}

func TestModelPricesProvidersAreTheWholeCatalogSorted(t *testing.T) {
	repo := seedSearchable(t)
	providers, err := repo.Providers(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"anthropic", "azure", "openai", "x"}, providers, "distinct, sorted, no blanks")
}

func TestModelPricesNewestIsWhenTheLastModelWasAdded(t *testing.T) {
	repo := emptyCatalog(t)
	ctx := context.Background()
	newest, err := repo.Newest(ctx)
	require.NoError(t, err)
	assert.Nil(t, newest, "nothing stored yet")

	before := time.Now().UTC().Add(-time.Second)
	_, err = repo.InsertMissing(ctx, []gwmodels.ModelPrice{{Model: "a"}})
	require.NoError(t, err)
	newest, err = repo.Newest(ctx)
	require.NoError(t, err)
	require.NotNil(t, newest)
	assert.True(t, newest.After(before), "%s", newest)
	assert.Equal(t, time.UTC, newest.Location())

	first := *newest
	time.Sleep(5 * time.Millisecond)
	_, err = repo.InsertMissing(ctx, []gwmodels.ModelPrice{{Model: "a"}, {Model: "b"}})
	require.NoError(t, err)
	newest, err = repo.Newest(ctx)
	require.NoError(t, err)
	assert.True(t, newest.After(first), "a later model moves it")
}

func TestModelPricesCanBeListedByProviderWithBlankOnesLast(t *testing.T) {
	repo := seedSearchable(t)
	rows, total, err := repo.List(context.Background(), ModelPriceFilter{ByProvider: true})
	require.NoError(t, err)
	assert.EqualValues(t, 8, total)
	names := modelNames(rows)
	require.Len(t, names, 8)
	assert.Equal(t, []string{"claude-3", "azure/eu/GPT-4o", "gpt-4o", "gpt-4o-mini"}, names[:4], "anthropic, azure, openai")
	assert.ElementsMatch(t, []string{"a_b", "aXb"}, names[4:6], "provider x")
	assert.Equal(t, []string{"100%", "no-provider"}, names[6:], "blank and missing providers last")
}
