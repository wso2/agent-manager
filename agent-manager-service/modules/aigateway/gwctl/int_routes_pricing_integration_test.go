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

package gwctl_test

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// emptyPriceCatalog clears the global price catalog, now and when the test ends.
func emptyPriceCatalog(t *testing.T) {
	t.Helper()
	clear := func() { require.NoError(t, db.GetDB().Exec("DELETE FROM mod_ai_gateway_model_prices").Error) }
	clear()
	t.Cleanup(clear)
}

// priceFile is a gateway's price file of n models in the shape of the real one: names with
// slashes, 15 providers, prices written as floats, ints and exponents, and every 12th model
// with no price, plus entries the control plane skips.
func priceFile(n int, inputPerToken string) string {
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		name := fmt.Sprintf("model-%04d", i)
		if i%3 == 0 {
			name = fmt.Sprintf("vendor/region/%s", name)
		}
		if i%12 == 11 {
			fmt.Fprintf(&b, `"%s": {"provider": "p%d", "mode": "image_generation"}`, name, i%15)
			continue
		}
		fmt.Fprintf(&b, `"%s": {"provider": "p%d", "input_cost_per_token": %s, "output_cost_per_token": 1e-05, "cache_read_input_token_cost": 0, "max_input_tokens": 128000, "max_output_tokens": 4096, "supports_vision": true}`,
			name, i%15, inputPerToken)
	}
	b.WriteString(`, "": {}, "not-an-entry": 5`)
	b.WriteString("}")
	return b.String()
}

func getPrices(t *testing.T, a *intAPI, query string, opts ...reqOpt) map[string]any {
	t.Helper()
	return expect(t, a.do(http.MethodGet, "/orgs/{org}/pricing"+query, nil, opts...), http.StatusOK)
}

func pricedModel(t *testing.T, body map[string]any, model string) map[string]any {
	t.Helper()
	for _, m := range body["models"].([]any) {
		if m.(map[string]any)["model"] == model {
			return m.(map[string]any)
		}
	}
	t.Fatalf("model %q is not in %v", model, body["models"])
	return nil
}

func TestINTRoutesPricingPushIsStoredAndServed(t *testing.T) {
	emptyPriceCatalog(t)
	a := newINTAPI(t)
	push := "/gateways/" + a.gateway.UUID.String() + "/pricing"

	empty := getPrices(t, a, "")
	assert.Nil(t, empty["updatedAt"])
	assert.Empty(t, empty["models"])

	rec := a.call(http.MethodPost, push, priceFile(841, "2.5e-06"))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Empty(t, rec.Body.String())

	first := getPrices(t, a, "?limit=200")
	assert.EqualValues(t, 841, first["total"], "the two unusable entries are skipped")
	assert.EqualValues(t, 200, first["limit"])
	assert.Len(t, first["models"], 200)
	assert.Len(t, first["providers"], 15)
	assert.NotNil(t, first["updatedAt"])
	last := getPrices(t, a, "?limit=200&offset=800")
	assert.Len(t, last["models"], 41, "the last page is short")

	one := pricedModel(t, getPrices(t, a, "?search=MODEL-0001"), "model-0001")
	assert.Equal(t, "2.50", one["inputPer1M"])
	assert.Equal(t, "10.00", one["outputPer1M"])
	assert.Equal(t, "0.00", one["cachedInputPer1M"], "a price of zero is a price")
	assert.Equal(t, "p1", one["provider"])
	unpriced := pricedModel(t, getPrices(t, a, "?search=model-0011"), "model-0011")
	assert.Nil(t, unpriced["inputPer1M"])
	assert.Contains(t, unpriced, "inputPer1M", "a missing price is listed, as null")

	assert.EqualValues(t, 1, getPrices(t, a, "?search=model-0007")["total"])
	assert.EqualValues(t, 1, getPrices(t, a, "?provider=p3&search=model-0003")["total"])
	assert.EqualValues(t, 57, getPrices(t, a, "?provider=p0")["total"])

	// A second push, with other prices and one more model: the first prices stay.
	again := strings.Replace(priceFile(841, "9e-06"), `"not-an-entry"`, `"extra-model": {"provider": "p0", "input_cost_per_token": 1e-06}, "not-an-entry"`, 1)
	rec = a.call(http.MethodPost, push, again)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	after := getPrices(t, a, "?search=model-0001")
	assert.Equal(t, "2.50", pricedModel(t, after, "model-0001")["inputPer1M"], "the first one wins")
	assert.EqualValues(t, 842, getPrices(t, a, "")["total"], "the new model is added")
	assert.Equal(t, "1.00", pricedModel(t, getPrices(t, a, "?search=extra-model"), "extra-model")["inputPer1M"])
}

// TestINTRoutesPricingPushOfTheRealPriceFile pushes the gateway's own price file when
// PRICING_FIXTURE names it (api-platform: gateway/configs/llm-pricing/model_prices.json).
func TestINTRoutesPricingPushOfTheRealPriceFile(t *testing.T) {
	path := os.Getenv("PRICING_FIXTURE")
	if path == "" {
		t.Skip("PRICING_FIXTURE is not set")
	}
	body, err := os.ReadFile(path) //nolint:gosec // a developer-supplied fixture
	require.NoError(t, err)
	emptyPriceCatalog(t)
	a := newINTAPI(t)

	rec := a.call(http.MethodPost, "/gateways/"+a.gateway.UUID.String()+"/pricing", string(body))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	got := getPrices(t, a, "?limit=200")
	t.Logf("total=%v providers=%d", got["total"], len(got["providers"].([]any)))
	assert.EqualValues(t, 841, got["total"])
	assert.Len(t, got["providers"], 15)
	// gpt-4o: 2.5e-06 per token is 2.50 per million.
	gpt := pricedModel(t, getPrices(t, a, "?search=gpt-4o&provider=openai&limit=200"), "gpt-4o")
	t.Logf("gpt-4o: %v", gpt)
	assert.Equal(t, "2.50", gpt["inputPer1M"])
}

func TestINTRoutesPricingPushRefusesWhatIsNotAPriceFile(t *testing.T) {
	emptyPriceCatalog(t)
	a := newINTAPI(t)
	push := "/gateways/" + a.gateway.UUID.String() + "/pricing"

	for name, body := range map[string]string{
		"an array":        `[{"a": {}}]`,
		"not json":        `{"a": `,
		"an empty object": `{}`,
		"no usable model": `{"a": 1, "": {}}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := a.call(http.MethodPost, push, body)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Contains(t, rec.Body.String(), "INVALID_PRICING")
		})
	}
	assert.EqualValues(t, 0, getPrices(t, a, "")["total"], "nothing was stored")

	big := `{"a": {"provider": "` + strings.Repeat("x", gwsvc.PricingMaxBodyBytes) + `"}}`
	rec := a.call(http.MethodPost, push, big)
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "too large")
	assert.EqualValues(t, 0, getPrices(t, a, "")["total"])
}

func TestINTRoutesPricingPushIsScopedToTheCallingGateway(t *testing.T) {
	emptyPriceCatalog(t)
	a := newINTAPI(t)

	other := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleBoth).UUID.String()
	rec := a.call(http.MethodPost, "/gateways/"+other+"/pricing", priceFile(20, "1e-06"))
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "does not match gateway ID")

	rec = a.call(http.MethodPost, "/gateways/"+a.gateway.UUID.String()+"/pricing", priceFile(20, "1e-06"), intNoKey)
	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())

	assert.EqualValues(t, 0, getPrices(t, a, "")["total"], "a refused push stores nothing")
}

func TestINTRoutesPricingReadNeedsTheCostScope(t *testing.T) {
	emptyPriceCatalog(t)
	a := newINTAPI(t)

	rec := a.do(http.MethodGet, "/orgs/{org}/pricing", nil, withScopes())
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, "/orgs/{org}/pricing", nil, withScopes(scope(gwperm.CostWrite)))
	require.Equal(t, http.StatusForbidden, rec.Code, "writing costs is not reading them: %s", rec.Body.String())
	expect(t, a.do(http.MethodGet, "/orgs/{org}/pricing", nil, withScopes(scope(gwperm.CostRead))), http.StatusOK)

	for _, query := range []string{"?limit=0", "?limit=201", "?offset=-1", "?limit=x"} {
		rec = a.do(http.MethodGet, "/orgs/{org}/pricing"+query, nil)
		require.Equal(t, http.StatusBadRequest, rec.Code, "%s: %s", query, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "VALIDATION_ERROR")
	}
}
