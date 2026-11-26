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
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

const (
	// PricingMaxBodyBytes bounds a pushed price file; the gateway's own is about 0.3 MB.
	PricingMaxBodyBytes = 2 << 20
	// pricingMaxModels bounds the models of one push.
	pricingMaxModels = 5000

	pricingMaxModelLength    = 255
	pricingMaxProviderLength = 64
	// pricingMaxNumberText bounds a number's text, and pricingMaxExponent its exponent,
	// before it is read as an exact decimal: a huge exponent must not become a huge number.
	pricingMaxNumberText = 40
	pricingMaxExponent   = 100
	// pricingMaxPerToken bounds a price per token: a catalog's are far below a dollar.
	pricingMaxPerToken = 1000.0
	// pricingMaxPriceLength is the width of a stored price.
	pricingMaxPriceLength = 32
	// pricingMaxDecimals is how many decimals a stored price may need to be exact.
	pricingMaxDecimals = 24
)

// PricingError is a pushed price file the control plane refuses.
type PricingError struct{ Message string }

func (e *PricingError) Error() string { return e.Message }

// Code is the error code the push route answers with.
func (e *PricingError) Code() string { return gwerr.ErrCodeInvalidPricing }

func invalidPricing(format string, args ...any) error {
	return &PricingError{Message: fmt.Sprintf(format, args...)}
}

// SanitizedPricing is what a pushed price file keeps: the stored rows (sorted by model),
// how many entries the file listed and how many were skipped.
type SanitizedPricing struct {
	Rows          []gwmodels.ModelPrice
	Sent, Skipped int
}

// SanitizePricing reads a gateway's price file (a JSON object, model name to entry) into
// the rows the catalog stores. Only the model, its provider, the prices of input, output
// and cached input, and the two token limits are kept; everything else in an entry is
// dropped. A bad value empties that one field; an entry that is not an object, or whose
// key is bad, is skipped. The whole file is refused when it is not a JSON object, lists no
// usable entry, or lists more than pricingMaxModels.
func SanitizePricing(body []byte) (*SanitizedPricing, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber() // a price stays as the file wrote it, not as a binary float
	var entries map[string]json.RawMessage
	if err := dec.Decode(&entries); err != nil || entries == nil {
		return nil, invalidPricing("The price file must be a JSON object of models.")
	}
	if len(entries) == 0 {
		return nil, invalidPricing("The price file lists no models.")
	}
	if len(entries) > pricingMaxModels {
		return nil, invalidPricing("The price file lists %d models; at most %d.", len(entries), pricingMaxModels)
	}
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := &SanitizedPricing{Sent: len(entries), Rows: make([]gwmodels.ModelPrice, 0, len(entries))}
	seen := make(map[string]bool, len(entries))
	for _, key := range keys {
		model := strings.TrimSpace(key)
		if !validModelName(model) || seen[model] {
			out.Skipped++
			continue
		}
		var entry map[string]any
		d := json.NewDecoder(bytes.NewReader(entries[key]))
		d.UseNumber()
		if err := d.Decode(&entry); err != nil || entry == nil {
			out.Skipped++
			continue
		}
		seen[model] = true
		out.Rows = append(out.Rows, gwmodels.ModelPrice{
			Model:                 model,
			Provider:              providerName(entry["provider"]),
			InputPerMillion:       pricePerMillion(entry["input_cost_per_token"]),
			OutputPerMillion:      pricePerMillion(entry["output_cost_per_token"]),
			CachedInputPerMillion: pricePerMillion(entry["cache_read_input_token_cost"]),
		})
	}
	if len(out.Rows) == 0 {
		return nil, invalidPricing("The price file has no usable model.")
	}
	sort.Slice(out.Rows, func(i, j int) bool { return out.Rows[i].Model < out.Rows[j].Model })
	return out, nil
}

// validModelName is a trimmed name of 1 to 255 characters without control characters.
func validModelName(s string) bool {
	n := utf8.RuneCountInString(s)
	if n == 0 || n > pricingMaxModelLength || !utf8.ValidString(s) {
		return false
	}
	return !strings.ContainsFunc(s, unicode.IsControl)
}

// providerName is a provider of up to 64 characters, trimmed; nil when absent, blank,
// too long or not text.
func providerName(v any) *string {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > pricingMaxProviderLength || !utf8.ValidString(s) ||
		strings.ContainsFunc(s, unicode.IsControl) {
		return nil
	}
	return &s
}

// pricePerMillion is a per-token cost (a JSON number, 0 or more) as USD per 1 million tokens,
// the exact decimal with at least 2 decimals: 2.75e-06 is "2.75", 1.375e-06 is "1.375", 0 is
// "0.00". It is nil when the value is not a usable number, or has no exact decimal within the
// stored width: a price is never rounded.
func pricePerMillion(v any) *string {
	r, ok := plainNumber(v)
	if !ok || r.Sign() < 0 {
		return nil
	}
	if f, _ := r.Float64(); f > pricingMaxPerToken {
		return nil
	}
	perMillion := new(big.Rat).Mul(r, big.NewRat(1_000_000, 1))
	for decimals := 2; decimals <= pricingMaxDecimals; decimals++ {
		text := perMillion.FloatString(decimals)
		if back, ok := new(big.Rat).SetString(text); ok && back.Cmp(perMillion) == 0 {
			if len(text) > pricingMaxPriceLength {
				return nil
			}
			return &text
		}
	}
	return nil
}

// plainNumber reads a JSON number as an exact decimal. A string, bool or object is not
// one; a number of more than pricingMaxNumberText characters or with an exponent beyond
// pricingMaxExponent is refused before it is expanded.
func plainNumber(v any) (*big.Rat, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return nil, false
	}
	text := n.String()
	if len(text) > pricingMaxNumberText {
		return nil, false
	}
	if i := strings.IndexAny(text, "eE"); i >= 0 {
		exp, err := strconv.Atoi(strings.TrimPrefix(text[i+1:], "+"))
		if err != nil || exp > pricingMaxExponent || exp < -pricingMaxExponent {
			return nil, false
		}
	}
	r, ok := new(big.Rat).SetString(text)
	return r, ok
}
