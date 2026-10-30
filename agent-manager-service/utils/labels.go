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

package utils

import (
	"fmt"
	"regexp"
	"strings"
)

// MaxLabelsPerResource caps the number of user-defined labels on an agent or kind.
const MaxLabelsPerResource = 10

// Label keys must not contain ':' — it separates key from value in the
// `label=key:value` list-filter query parameter, so keeping it out of keys
// makes splitting on the first ':' unambiguous.
var (
	labelKeyRegex   = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`)
	labelValueRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?)?$`)
)

const maxLabelLength = 63

// ValidateLabels checks the label count and each key/value against the rules
// documented in the API spec: at most MaxLabelsPerResource labels; keys are
// 1-63 chars of [a-zA-Z0-9._-] starting and ending alphanumeric; values follow
// the same rules but may be empty.
func ValidateLabels(labels map[string]string) error {
	if len(labels) > MaxLabelsPerResource {
		return NewValidationErrorf(
			fmt.Sprintf("A resource can have at most %d labels", MaxLabelsPerResource),
			"labels: %d labels provided, maximum is %d", len(labels), MaxLabelsPerResource,
		)
	}
	for key, value := range labels {
		if len(key) == 0 || len(key) > maxLabelLength || !labelKeyRegex.MatchString(key) {
			return NewValidationErrorf(
				"Label keys must be 1-63 characters of letters, digits, '.', '_' or '-', starting and ending with a letter or digit",
				"labels: invalid key %q", key,
			)
		}
		if len(value) > maxLabelLength || !labelValueRegex.MatchString(value) {
			return NewValidationErrorf(
				"Label values must be at most 63 characters of letters, digits, '.', '_' or '-', starting and ending with a letter or digit",
				"labels: invalid value %q for key %q", value, key,
			)
		}
	}
	return nil
}

// ValidateLabelValue checks a single value against the Kubernetes label-value
// rules, for values the platform stores as labels on OpenChoreo resources
// rather than accepting from the user's label map. field names the offending
// input in the detail message.
func ValidateLabelValue(value, field string) error {
	if len(value) > maxLabelLength || !labelValueRegex.MatchString(value) {
		return NewValidationErrorf(
			fmt.Sprintf("%s must be at most %d characters of letters, digits, '.', '_' or '-', starting and ending with a letter or digit",
				field, maxLabelLength),
			"%s: invalid value %q", field, value,
		)
	}
	return nil
}

// ParseLabelSelectors parses repeated `label=key:value` query parameters into
// a label map. Each entry is split on the first ':'; the key and value must
// satisfy the same rules as stored labels. Repeating the same key with
// different values would make the AND-semantics filter impossible to satisfy
// (no label can equal two different values at once), so that's rejected
// rather than silently keeping only the last one. The raw parameter count is
// capped at MaxLabelsPerResource (the spec's maxItems) before deduplication,
// so repeated identical selectors cannot bypass the limit.
func ParseLabelSelectors(params []string) (map[string]string, error) {
	if len(params) > MaxLabelsPerResource {
		return nil, NewValidationErrorf(
			fmt.Sprintf("At most %d label filters can be specified", MaxLabelsPerResource),
			"label: %d filters provided, maximum is %d", len(params), MaxLabelsPerResource,
		)
	}
	selectors := make(map[string]string, len(params))
	for _, param := range params {
		key, value, found := strings.Cut(param, ":")
		if !found {
			return nil, NewValidationErrorf(
				"Label filters must be in key:value format",
				"label: missing ':' separator in %q", param,
			)
		}
		if existing, exists := selectors[key]; exists && existing != value {
			return nil, NewValidationErrorf(
				"Label filter keys cannot be repeated with different values",
				"label: key %q specified with conflicting values %q and %q", key, existing, value,
			)
		}
		selectors[key] = value
	}
	if err := ValidateLabels(selectors); err != nil {
		return nil, err
	}
	return selectors, nil
}

// LabelsMatch reports whether every key/value pair in want is present in have.
// An empty or nil want matches everything.
func LabelsMatch(have, want map[string]string) bool {
	for key, value := range want {
		if haveValue, ok := have[key]; !ok || haveValue != value {
			return false
		}
	}
	return true
}
