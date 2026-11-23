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
package aigateway

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconfig"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwinit"
)

// TestEverySettingIsDocumented: the manifest's Settings are exactly the keys
// the module reads, so the configuration reference stays complete.
func TestEverySettingIsDocumented(t *testing.T) {
	read := map[string]bool{}
	get := func(key string) string { read[key] = true; return "" }
	_, _ = gwconfig.Load(get)
	_, _ = gwinit.LoadGatewayConfig(get)

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
