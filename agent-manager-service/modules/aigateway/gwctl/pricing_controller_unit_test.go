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

package gwctl

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
)

type fakePricing struct {
	got gwsvc.PricingQuery
	err error
}

func (f *fakePricing) Save(context.Context, []byte) (gwsvc.PricingSaved, error) {
	return gwsvc.PricingSaved{}, nil
}

func (f *fakePricing) Catalog(_ context.Context, q gwsvc.PricingQuery) (*gwspec.ModelPriceList, error) {
	f.got = q
	if f.err != nil {
		return nil, f.err
	}
	return gwspec.NewModelPriceList([]string{"openai"}, 1, 50, 0, []gwspec.ModelPrice{*gwspec.NewModelPrice("gpt-4o")}), nil
}

func getPricing(t *testing.T, svc *fakePricing, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/orgs/acme/pricing"+query, nil)
	w := httptest.NewRecorder()
	NewPricingController(svc).GetPricing(w, req)
	return w
}

func TestGetPricingPassesTheQueryOn(t *testing.T) {
	svc := &fakePricing{}
	w := getPricing(t, svc, "?search=gpt&provider=openai&limit=20&offset=40")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, gwsvc.PricingQuery{Search: "gpt", Provider: "openai", Limit: 20, Offset: 40}, svc.got)
	assert.Contains(t, w.Body.String(), `"model":"gpt-4o"`)
}

func TestGetPricingWithoutAQueryAsksForTheDefaults(t *testing.T) {
	svc := &fakePricing{}
	require.Equal(t, http.StatusOK, getPricing(t, svc, "").Code)
	assert.Equal(t, gwsvc.PricingQuery{}, svc.got)
}

func TestGetPricingRefusesABadPage(t *testing.T) {
	for _, query := range []string{
		"?limit=0", "?limit=201", "?limit=-1", "?limit=ten", "?limit=1.5", "?offset=-1", "?offset=x",
		"?search=" + strings.Repeat("a", 256), "?provider=" + strings.Repeat("a", 65),
	} {
		t.Run(query[:min(len(query), 20)], func(t *testing.T) {
			w := getPricing(t, &fakePricing{}, query)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "VALIDATION_ERROR")
		})
	}
}

func TestGetPricingAcceptsTheLimits(t *testing.T) {
	for _, query := range []string{"?limit=1", "?limit=200", "?offset=0", "?search=" + strings.Repeat("a", 255)} {
		require.Equal(t, http.StatusOK, getPricing(t, &fakePricing{}, query).Code, query[:min(len(query), 20)])
	}
}

func TestGetPricingAnswers500WhenTheCatalogCannotBeRead(t *testing.T) {
	w := getPricing(t, &fakePricing{err: errors.New("database is down")}, "")
	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "database is down", "internal errors stay out of the response")
}
