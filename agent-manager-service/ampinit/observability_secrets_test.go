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

package ampinit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

const storeURLKey = "MODULE_OBSERVABILITY_CLICKHOUSE_URL"

// storeDirs are the directories the observability store's passwords go to.
type storeDirs struct{ secrets, clickhouse, api, migrate, collector, shared string }

// observabilityEnv clears every generated secret and the store's settings,
// and gives each of the store's directory settings a fresh directory.
func observabilityEnv(t *testing.T) storeDirs {
	t.Helper()
	clearSecrets(t)
	for _, k := range []string{
		storeURLKey, "MODULE_OBSERVABILITY_CLICKHOUSE_PASSWORD", "MODULE_OBSERVABILITY_CLICKHOUSE_CLEANER_PASSWORD",
		"MODULE_OBSERVABILITY_CLICKHOUSE_MIGRATOR_PASSWORD", "MODULE_OBSERVABILITY_CLICKHOUSE_INGEST_PASSWORD",
		"MODULE_OBSERVABILITY_CLICKHOUSE_PASSWORD_FILE", "AMP_CONFIG",
	} {
		t.Setenv(k, "")
		require.NoError(t, os.Unsetenv(k))
	}
	d := storeDirs{
		secrets: t.TempDir(), clickhouse: t.TempDir(), api: t.TempDir(),
		migrate: t.TempDir(), collector: t.TempDir(), shared: t.TempDir(),
	}
	t.Setenv(ClickHouseSecretsDir, d.clickhouse)
	t.Setenv(ClickHouseAPISecretsDir, d.api)
	t.Setenv(ClickHouseMigrateSecretsDir, d.migrate)
	t.Setenv(ClickHouseCollectorSecretsDir, d.collector)
	t.Setenv("AMP_SHARED_SECRETS_DIR", d.shared)
	return d
}

// names lists a directory's file names.
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	out := []string{}
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Mode().Perm()
}

var storePasswords = []string{"delete-password", "read-password", "setup-password", "write-password"}

func TestObservabilitySecretsSkippedWithoutStore(t *testing.T) {
	d := observabilityEnv(t)
	require.NoError(t, EnsureSecrets(d.secrets, nil))
	for _, dir := range []string{d.clickhouse, d.api, d.migrate, d.collector} {
		assert.Empty(t, names(t, dir), dir)
	}
	for _, n := range storePasswords {
		assert.NoFileExists(t, filepath.Join(d.secrets, n))
	}
}

// Without its directory (amp serve, which reads only its own copies) none is
// written, so the server's secrets directory never holds them.
func TestObservabilitySecretsSkippedWithoutDir(t *testing.T) {
	d := observabilityEnv(t)
	t.Setenv(storeURLKey, "http://clickhouse:8123")
	t.Setenv(ClickHouseSecretsDir, "")
	require.NoError(t, EnsureSecrets(d.secrets, nil))
	for _, dir := range []string{d.api, d.migrate, d.collector} {
		assert.Empty(t, names(t, dir), dir)
	}
	for _, n := range storePasswords {
		assert.NoFileExists(t, filepath.Join(d.secrets, n))
	}
}

func TestObservabilitySecretsGeneratedAndKept(t *testing.T) {
	d := observabilityEnv(t)
	t.Setenv(storeURLKey, "http://clickhouse:8123")
	require.NoError(t, EnsureSecrets(d.secrets, nil))

	assert.Equal(t, storePasswords, names(t, d.clickhouse))
	first := map[string]string{}
	for _, n := range storePasswords {
		p := filepath.Join(d.clickhouse, n)
		first[n] = read(t, p)
		assert.Len(t, first[n], 24, n)
		assert.Equal(t, os.FileMode(0o600), mode(t, p), n)
		assert.NoFileExists(t, filepath.Join(d.secrets, n), "not in the server's secrets directory")
	}

	// Each reader's directory holds its own passwords only.
	assert.Equal(t, []string{"delete-password", "read-password"}, names(t, d.api))
	assert.Equal(t, []string{"setup-password"}, names(t, d.migrate))
	assert.Equal(t, []string{"write-password"}, names(t, d.collector))
	for dir, files := range map[string][]string{d.api: names(t, d.api), d.migrate: names(t, d.migrate), d.collector: names(t, d.collector)} {
		for _, n := range files {
			assert.Equal(t, first[n], read(t, filepath.Join(dir, n)), n)
			assert.Equal(t, os.FileMode(0o644), mode(t, filepath.Join(dir, n)), n)
		}
	}
	assert.Equal(t, []string{"admin-password"}, names(t, d.shared), "none goes to the shared directory")

	require.NoError(t, EnsureSecrets(d.secrets, nil))
	for _, n := range storePasswords {
		assert.Equal(t, first[n], read(t, filepath.Join(d.clickhouse, n)), "kept: %s", n)
	}
}

func TestObservabilitySecretsWriteSetValues(t *testing.T) {
	d := observabilityEnv(t)
	t.Setenv(storeURLKey, "http://clickhouse:8123")
	t.Setenv("MODULE_OBSERVABILITY_CLICKHOUSE_INGEST_PASSWORD", "mine")
	require.NoError(t, EnsureSecrets(d.secrets, nil))
	assert.Equal(t, "mine", read(t, filepath.Join(d.clickhouse, "write-password")))
	assert.Equal(t, "mine", read(t, filepath.Join(d.collector, "write-password")))

	// A changed value replaces the kept one; an empty one counts as unset.
	t.Setenv("MODULE_OBSERVABILITY_CLICKHOUSE_INGEST_PASSWORD", "other")
	require.NoError(t, EnsureSecrets(d.secrets, nil))
	assert.Equal(t, "other", read(t, filepath.Join(d.collector, "write-password")))
	t.Setenv("MODULE_OBSERVABILITY_CLICKHOUSE_INGEST_PASSWORD", "")
	require.NoError(t, EnsureSecrets(d.secrets, nil))
	assert.Equal(t, "other", read(t, filepath.Join(d.collector, "write-password")), "kept, not regenerated")

	// A file the environment names.
	file := filepath.Join(t.TempDir(), "read")
	require.NoError(t, os.WriteFile(file, []byte("from-file\n"), 0o600))
	t.Setenv("MODULE_OBSERVABILITY_CLICKHOUSE_PASSWORD_FILE", file)
	require.NoError(t, EnsureSecrets(d.secrets, nil))
	assert.Equal(t, "from-file", read(t, filepath.Join(d.clickhouse, "read-password")))
	assert.Equal(t, "from-file", read(t, filepath.Join(d.api, "read-password")))
}

func TestObservabilitySecretsFromConfigFile(t *testing.T) {
	d := observabilityEnv(t)
	t.Chdir(t.TempDir())
	setup := filepath.Join(t.TempDir(), "setup-password")
	require.NoError(t, os.WriteFile(setup, []byte("chosen-setup"), 0o600))
	cfg := filepath.Join(t.TempDir(), "amp.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte("module:\n  observability:\n    clickhouse:\n"+
		"      url: http://clickhouse.example:8123\n"+
		"      migrator-password: \"${file:"+setup+"}\"\n"), 0o600))
	require.NoError(t, config.LoadUnchecked(cfg))
	t.Cleanup(func() { _ = config.LoadUnchecked("") })

	require.NoError(t, EnsureSecrets(d.secrets, nil))
	assert.Equal(t, "chosen-setup", read(t, filepath.Join(d.clickhouse, "setup-password")))
	assert.Equal(t, "chosen-setup", read(t, filepath.Join(d.migrate, "setup-password")))
	assert.Len(t, read(t, filepath.Join(d.clickhouse, "read-password")), 24, "the others are generated")
}

// An error names the setting, never the value.
func TestObservabilitySecretsErrorNamesSetting(t *testing.T) {
	d := observabilityEnv(t)
	t.Setenv(storeURLKey, "http://clickhouse:8123")
	t.Setenv("MODULE_OBSERVABILITY_CLICKHOUSE_INGEST_PASSWORD", "never-shown")
	require.NoError(t, os.WriteFile(filepath.Join(d.collector, "blocker"), nil, 0o600))
	t.Setenv(ClickHouseCollectorSecretsDir, filepath.Join(d.collector, "blocker"))
	err := EnsureSecrets(d.secrets, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MODULE_OBSERVABILITY_CLICKHOUSE_INGEST_PASSWORD")
	assert.NotContains(t, err.Error(), "never-shown")
}
