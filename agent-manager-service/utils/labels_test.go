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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateLabels(t *testing.T) {
	longKey := strings.Repeat("a", 63)

	valid := []map[string]string{
		nil,
		{},
		{"env": "prod"},
		{"team.name": "ml-platform", "version_tag": "v1.2.3"},
		{"a": ""},          // empty value allowed
		{longKey: longKey}, // 63 chars is the inclusive max
		{"0start": "end9"}, // digits are valid at the boundaries
		{"a.b-c_d": "x.y-z_w~"[:7]},
	}
	for _, labels := range valid {
		assert.NoError(t, ValidateLabels(labels), "labels %v should be valid", labels)
	}

	invalid := []map[string]string{
		{"": "value"},        // empty key
		{longKey + "a": "v"}, // key too long
		{"k": longKey + "a"}, // value too long
		{"-start": "v"},      // key starts non-alphanumeric
		{"end-": "v"},        // key ends non-alphanumeric
		{"k": "-v"},          // value starts non-alphanumeric
		{"has:colon": "v"},   // ':' reserved for the filter syntax
		{"has space": "v"},   // whitespace
		{"k": "has space"},   // whitespace in value
	}
	for _, labels := range invalid {
		assert.Error(t, ValidateLabels(labels), "labels %v should be invalid", labels)
	}

	t.Run("rejects more than the max label count", func(t *testing.T) {
		labels := make(map[string]string, MaxLabelsPerResource+1)
		for i := 0; i <= MaxLabelsPerResource; i++ {
			labels["key"+strings.Repeat("x", i)] = "v"
		}
		assert.Error(t, ValidateLabels(labels))
	})
}

func TestParseLabelSelectors(t *testing.T) {
	t.Run("returns an empty map for no params", func(t *testing.T) {
		selectors, err := ParseLabelSelectors(nil)
		require.NoError(t, err)
		assert.Empty(t, selectors)
	})

	t.Run("parses repeated key:value entries", func(t *testing.T) {
		selectors, err := ParseLabelSelectors([]string{"env:prod", "team:ml"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"env": "prod", "team": "ml"}, selectors)
	})

	t.Run("splits on the first colon only", func(t *testing.T) {
		// Values cannot legally contain ':' either, so this must fail
		// validation — but the split itself must not produce key "a:b".
		_, err := ParseLabelSelectors([]string{"a:b:c"})
		assert.Error(t, err)
	})

	t.Run("allows an empty value selector", func(t *testing.T) {
		selectors, err := ParseLabelSelectors([]string{"env:"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"env": ""}, selectors)
	})

	t.Run("rejects an entry without a colon", func(t *testing.T) {
		_, err := ParseLabelSelectors([]string{"noseparator"})
		assert.Error(t, err)
	})

	t.Run("rejects an invalid key", func(t *testing.T) {
		_, err := ParseLabelSelectors([]string{"-bad:v"})
		assert.Error(t, err)
	})

	t.Run("allows the same key repeated with an identical value", func(t *testing.T) {
		selectors, err := ParseLabelSelectors([]string{"env:prod", "env:prod"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"env": "prod"}, selectors)
	})

	t.Run("rejects the same key repeated with conflicting values", func(t *testing.T) {
		_, err := ParseLabelSelectors([]string{"team:ml", "team:web"})
		assert.Error(t, err)
	})

	t.Run("accepts up to the maximum number of selectors", func(t *testing.T) {
		params := make([]string, MaxLabelsPerResource)
		for i := range params {
			params[i] = fmt.Sprintf("k%d:v", i)
		}
		selectors, err := ParseLabelSelectors(params)
		require.NoError(t, err)
		assert.Len(t, selectors, MaxLabelsPerResource)
	})

	t.Run("rejects more raw selectors than the maximum, even when they deduplicate", func(t *testing.T) {
		params := make([]string, MaxLabelsPerResource+1)
		for i := range params {
			params[i] = "env:prod"
		}
		_, err := ParseLabelSelectors(params)
		assert.Error(t, err)
	})
}

func TestLabelsMatch(t *testing.T) {
	have := map[string]string{"env": "prod", "team": "ml"}

	assert.True(t, LabelsMatch(have, nil))
	assert.True(t, LabelsMatch(have, map[string]string{}))
	assert.True(t, LabelsMatch(have, map[string]string{"env": "prod"}))
	assert.True(t, LabelsMatch(have, map[string]string{"env": "prod", "team": "ml"}))
	assert.False(t, LabelsMatch(have, map[string]string{"env": "dev"}))
	assert.False(t, LabelsMatch(have, map[string]string{"missing": "x"}))
	assert.False(t, LabelsMatch(nil, map[string]string{"env": "prod"}))
}
