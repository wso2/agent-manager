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
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func str(s string) *string { return &s }

func TestPricePerMillionIsTheExactDecimal(t *testing.T) {
	cases := []struct {
		name string
		in   string // a JSON number, as the file writes it
		want *string
	}{
		{"scientific", "2.75e-06", str("2.75")},
		{"three decimals", "1.375e-06", str("1.375")},
		{"fractions of a cent", "7.5e-08", str("0.075")},
		{"plain decimal", "0.0000025", str("2.50")},
		{"a whole price", "0.00001", str("10.00")},
		{"an integer zero", "0", str("0.00")},
		{"a float zero", "0.0", str("0.00")},
		{"very small, still exact", "1e-30", str("0.000000000000000000000001")},
		{"too small to be exact in 24 decimals", "1e-31", nil},
		{"very large, at the cap", "1000", str("1000000000.00")},
		{"above the cap", "1001", nil},
		{"negative", "-1e-06", nil},
		{"huge exponent", "1e400", nil},
		{"tiny exponent", "1e-400", nil},
		{"long digits", "0.12345678901234567890123456789012345678901", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, pricePerMillion(json.Number(c.in)))
		})
	}
}

func TestPricePerMillionRefusesWhatIsNotANumber(t *testing.T) {
	for _, v := range []any{nil, "2.75e-06", true, []any{1}, map[string]any{"a": 1}, 2.5, int64(3)} {
		assert.Nil(t, pricePerMillion(v), "%#v", v)
	}
}

func TestProviderIsTrimmedTextOfUpTo64Characters(t *testing.T) {
	assert.Equal(t, str("openai"), providerName("  openai "))
	assert.Equal(t, str(strings.Repeat("é", 64)), providerName(strings.Repeat("é", 64)))
	assert.Nil(t, providerName(strings.Repeat("é", 65)), "too long is dropped, not cut")
	assert.Nil(t, providerName("   "))
	assert.Nil(t, providerName("open\nai"))
	assert.Nil(t, providerName(42))
	assert.Nil(t, providerName(nil))
}

func TestModelNameRules(t *testing.T) {
	assert.True(t, validModelName("azure/eu/gpt-4o-2024-08-06"))
	assert.True(t, validModelName(strings.Repeat("é", 255)))
	assert.False(t, validModelName(strings.Repeat("é", 256)))
	assert.False(t, validModelName(""))
	assert.False(t, validModelName("gpt\t4"))
	assert.False(t, validModelName("a\x00b"))
	assert.False(t, validModelName("\xff\xfe"))
}

func TestSanitizeKeepsOnlyTheStoredFields(t *testing.T) {
	body := `{"gpt-4o": {"provider": "openai", "input_cost_per_token": 2.5e-06, "output_cost_per_token": 1e-05,
		"cache_read_input_token_cost": 1.25e-06, "max_input_tokens": 128000, "max_output_tokens": 16384,
		"mode": "chat", "supports_vision": true, "notes": "<script>alert(1)</script>"}}`
	got, err := SanitizePricing([]byte(body))
	require.NoError(t, err)
	require.Len(t, got.Rows, 1)
	row := got.Rows[0]
	assert.Equal(t, "gpt-4o", row.Model)
	assert.Equal(t, str("openai"), row.Provider)
	assert.Equal(t, str("2.50"), row.InputPerMillion)
	assert.Equal(t, str("10.00"), row.OutputPerMillion)
	assert.Equal(t, str("1.25"), row.CachedInputPerMillion)
	assert.Equal(t, 1, got.Sent)
	assert.Zero(t, got.Skipped)
}

func TestSanitizeEmptiesABadFieldNotTheEntry(t *testing.T) {
	body := `{"m": {"provider": 7, "input_cost_per_token": "cheap", "output_cost_per_token": -1,
		"cache_read_input_token_cost": null, "max_input_tokens": 1.5, "max_output_tokens": "big"}}`
	got, err := SanitizePricing([]byte(body))
	require.NoError(t, err)
	require.Len(t, got.Rows, 1)
	row := got.Rows[0]
	assert.Nil(t, row.Provider)
	assert.Nil(t, row.InputPerMillion)
	assert.Nil(t, row.OutputPerMillion)
	assert.Nil(t, row.CachedInputPerMillion)
}

func TestSanitizeSkipsUnusableEntries(t *testing.T) {
	long := strings.Repeat("x", 256)
	body := fmt.Sprintf(`{"ok": {}, "  spaced  ": {"provider": "p"}, "spaced": {"provider": "dup"}, "": {}, " ": {},
		"%s": {}, "tab\there": {}, "null": null, "number": 5, "text": "x", "list": [1], "bad\u0000key": {}}`, long)
	got, err := SanitizePricing([]byte(body))
	require.NoError(t, err)
	names := make([]string, 0, len(got.Rows))
	for _, r := range got.Rows {
		names = append(names, r.Model)
	}
	// "  spaced  " and "spaced" are one model once trimmed; the first in key order is kept.
	assert.Equal(t, []string{"ok", "spaced"}, names)
	assert.Equal(t, 12, got.Sent)
	assert.Equal(t, 10, got.Skipped)
	for _, r := range got.Rows {
		if r.Model == "spaced" {
			assert.Equal(t, str("p"), r.Provider, "the entry whose key sorts first wins")
		}
	}
}

func TestSanitizeRefusesWhatIsNotAUsableFile(t *testing.T) {
	many := make(map[string]any, pricingMaxModels+1)
	for i := 0; i <= pricingMaxModels; i++ {
		many[fmt.Sprintf("model-%d", i)] = map[string]any{}
	}
	manyJSON, err := json.Marshal(many)
	require.NoError(t, err)
	for name, body := range map[string]string{
		"an array":            `[{"a": {}}]`,
		"text":                `"prices"`,
		"a number":            `5`,
		"null":                `null`,
		"not json":            `{"a": `,
		"empty":               ``,
		"an empty object":     `{}`,
		"no usable entry":     `{"a": 1, "b": null, "": {}}`,
		"more than the limit": string(manyJSON),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := SanitizePricing([]byte(body))
			var refused *PricingError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, "INVALID_PRICING", refused.Code())
		})
	}
}

func TestSanitizeAcceptsTheLimit(t *testing.T) {
	many := make(map[string]any, pricingMaxModels)
	for i := 0; i < pricingMaxModels; i++ {
		many[fmt.Sprintf("model-%d", i)] = map[string]any{"provider": "p"}
	}
	body, err := json.Marshal(many)
	require.NoError(t, err)
	got, err := SanitizePricing(body)
	require.NoError(t, err)
	assert.Len(t, got.Rows, pricingMaxModels)
	for i := 1; i < len(got.Rows); i++ {
		assert.Less(t, got.Rows[i-1].Model, got.Rows[i].Model, "rows come sorted by model")
	}
}

// TestSanitizeTheRealPriceFile runs the sanitizer over the gateway's own price file when
// PRICING_FIXTURE names it (api-platform: gateway/configs/llm-pricing/model_prices.json).
func TestSanitizeTheRealPriceFile(t *testing.T) {
	path := os.Getenv("PRICING_FIXTURE")
	if path == "" {
		t.Skip("PRICING_FIXTURE is not set")
	}
	body, err := os.ReadFile(path) //nolint:gosec // a developer-supplied fixture
	require.NoError(t, err)
	got, err := SanitizePricing(body)
	require.NoError(t, err)
	t.Logf("sent=%d stored=%d skipped=%d", got.Sent, len(got.Rows), got.Skipped)
	assert.Equal(t, got.Sent, len(got.Rows)+got.Skipped)
	assert.Greater(t, len(got.Rows), 800)
	priced := 0
	for _, r := range got.Rows {
		if r.InputPerMillion != nil {
			priced++
		}
	}
	assert.Greater(t, priced, 700)
}
