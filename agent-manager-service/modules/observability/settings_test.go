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

package observability

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

// funcRuntime answers Config with get.
type funcRuntime struct {
	configRuntime
	get func(string) string
}

func (r funcRuntime) Config(key string) string { return r.get(key) }

// TestEverySettingIsDocumented:the manifest's Settings are exactly the keys
// the module reads and initOnly, so the configuration reference stays complete.
func TestEverySettingIsDocumented(t *testing.T) {
	read := map[string]bool{}
	get := func(key string) string {
		read[key] = true
		switch key {
		case "clickhouse.url":
			return "http://clickhouse:8123"
		case "clickhouse.ttl-days":
			return "30"
		}
		return "x"
	}
	_, _, err := openStores(funcRuntime{get: get})
	assert.NoError(t, err)
	for _, apply := range []bool{true, false} {
		_, err := (module{}).ExternalSchema(get, apply)
		assert.NoError(t, err)
	}

	for _, k := range initOnly {
		read[k] = true
	}

	documented := map[string]bool{}
	for _, s := range (module{}).Manifest().Settings {
		documented[s.Key] = true
		assert.NotEmpty(t, s.Description, s.Key)
	}
	var missing, stale []string
	for k := range read {
		if !documented[k] {
			missing = append(missing, k)
		}
	}
	for k := range documented {
		if !read[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	assert.Empty(t, missing, "settings the module reads but does not document")
	assert.Empty(t, stale, "documented settings the module does not read")
}
