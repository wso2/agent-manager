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
package configdocs

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/wso2/agent-manager/agent-manager-service/ampinit" // registers amp init's settings
	"github.com/wso2/agent-manager/agent-manager-service/config"
	_ "github.com/wso2/agent-manager/agent-manager-service/modules/all"
	_ "github.com/wso2/agent-manager/agent-manager-service/vault/openbao" // registers the OpenBao vault's settings
)

// TestEverySettingIsDocumented: config.Settings documents exactly the
// settings the server and amp init read.
func TestEverySettingIsDocumented(t *testing.T) {
	t.Setenv(config.FileEnv, "")
	t.Chdir(t.TempDir())
	require.NoError(t, config.LoadUnchecked(""))
	known := config.KnownKeys()

	documented := map[string]bool{}
	for _, s := range config.Settings {
		assert.False(t, documented[s.Key], "documented twice: %s", s.Key)
		documented[s.Key] = true
		assert.NotEmpty(t, s.Description, s.Key)
		assert.NotEmpty(t, s.Group, s.Key)
	}
	var missing, stale []string
	for _, k := range known {
		if !documented[k] && !strings.HasPrefix(k, config.ModulePrefix) && !config.IsOldName(k) {
			missing = append(missing, k)
		}
	}
	for k := range documented {
		if !slices.Contains(known, k) && !strings.HasPrefix(k, config.ConsolePrefix) {
			stale = append(stale, k)
		}
	}
	slices.Sort(stale)
	assert.Empty(t, missing, "settings read but not in config.Settings")
	assert.Empty(t, stale, "config.Settings rows no setting reads")
}

// TestReferenceIsCurrent: docs/amp-lite/configuration.md is what Render
// writes; run `make config-docs` after changing a setting.
func TestReferenceIsCurrent(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "amp-lite", "configuration.md"))
	require.NoError(t, err)
	assert.Equal(t, string(want), Render(), "run make config-docs")
}
