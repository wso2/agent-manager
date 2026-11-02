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

package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactRemovesCredentialFields(t *testing.T) {
	in := map[string]any{
		"name":         "agent-a",
		"apiKey":       "sk-123",
		"clientSecret": "s3cret",
		"nested": map[string]any{
			"password": "p",
			"items":    []any{map[string]any{"token": "t", "id": "1"}},
		},
		"key":         "raw-key",
		"keyName":     "kept",
		"environment": "dev",
	}
	out := Redact(in).(map[string]any)
	assert.Equal(t, "agent-a", out["name"])
	assert.Equal(t, "dev", out["environment"])
	assert.Equal(t, "kept", out["keyName"], "a name that merely contains 'key' is kept")
	for _, k := range []string{"apiKey", "clientSecret", "key"} {
		assert.NotContains(t, out, k)
	}
	nested := out["nested"].(map[string]any)
	assert.NotContains(t, nested, "password")
	item := nested["items"].([]any)[0].(map[string]any)
	assert.NotContains(t, item, "token")
	assert.Equal(t, "1", item["id"])
}

func TestCatalogTypesHaveUniqueNamesAndValidScopes(t *testing.T) {
	seen := map[string]bool{}
	for _, typ := range Types("") {
		assert.False(t, seen[typ.Name], "duplicate event type %s", typ.Name)
		seen[typ.Name] = true
		assert.True(t, typ.Scope.Valid(), typ.Name)
		assert.NotEmpty(t, typ.Description, typ.Name)
	}
	for _, typ := range Types(ScopeProject) {
		assert.Equal(t, ScopeProject, typ.Scope)
	}
}
