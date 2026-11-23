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
)

// clearSecrets unsets every generated secret and its older names.
func clearSecrets(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ENCRYPTION_KEY", "IDP_CLIENT_SECRET", "AMP_API_CLIENT_SECRET", "AMP_CONSOLE_CLIENT_SECRET",
		"THUNDER_CLIENT_SECRET", "AMP_SYSTEM_CLIENT_SECRET", "AMP_ADMIN_PASSWORD",
		"MODULE_AI_GATEWAY_ADMIN_PASSWORD", "DB_PASSWORD", "DB_DRIVER", "DB_HOST", "AMP_SHARED_SECRETS_DIR",
	} {
		t.Setenv(k, "")
		require.NoError(t, os.Unsetenv(k))
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func TestEnsureSecretsGeneratesOnce(t *testing.T) {
	clearSecrets(t)
	dir := t.TempDir()
	require.NoError(t, EnsureSecrets(dir, nil))
	first := map[string]string{}
	for _, name := range []string{"amp-api-client-secret", "amp-console-client-secret", "amp-system-client-secret", "admin-password", "gateway-admin-password"} {
		p := filepath.Join(dir, name)
		info, err := os.Stat(p)
		require.NoError(t, err, name)
		assert.NotEmpty(t, read(t, p), name)
		first[name] = read(t, p)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), name)
	}
	assert.Len(t, first["admin-password"], 24)
	assert.NoFileExists(t, filepath.Join(dir, "encryption-key"), "amp serve generates the encryption key")
	assert.NoFileExists(t, filepath.Join(dir, "db-password"), "no PostgreSQL configured")

	require.NoError(t, EnsureSecrets(dir, nil))
	for name, v := range first {
		assert.Equal(t, v, read(t, filepath.Join(dir, name)), "kept: %s", name)
	}
}

func TestEnsureSecretsSkipsSetValues(t *testing.T) {
	clearSecrets(t)
	t.Setenv("IDP_CLIENT_SECRET", "mine")
	dir := t.TempDir()
	require.NoError(t, EnsureSecrets(dir, nil))
	assert.NoFileExists(t, filepath.Join(dir, "amp-api-client-secret"))
}

func TestEnsureSecretsWritesSharedSetValues(t *testing.T) {
	clearSecrets(t)
	dir := t.TempDir()
	t.Setenv("AMP_ADMIN_PASSWORD", "mine")
	require.NoError(t, EnsureSecrets(dir, nil))
	assert.Equal(t, "mine", read(t, filepath.Join(dir, "admin-password")))

	t.Setenv("AMP_ADMIN_PASSWORD", "other")
	require.NoError(t, EnsureSecrets(dir, nil))
	assert.Equal(t, "other", read(t, filepath.Join(dir, "admin-password")))

	// Written as the server reads it, so the two sides agree.
	t.Setenv("AMP_ADMIN_PASSWORD", " spaced ")
	require.NoError(t, EnsureSecrets(dir, nil))
	got, err := os.ReadFile(filepath.Join(dir, "admin-password"))
	require.NoError(t, err)
	assert.Equal(t, " spaced ", string(got))
}

// A secret file that exists but cannot be read is an error, not replaced.
func TestEnsureSecretsKeepsUnreadableFile(t *testing.T) {
	clearSecrets(t)
	if os.Geteuid() == 0 {
		t.Skip("root reads any file")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "amp-api-client-secret")
	require.NoError(t, os.WriteFile(path, []byte("kept"), 0o000))
	err := EnsureSecrets(dir, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IDP_CLIENT_SECRET")
	require.NoError(t, os.Chmod(path, 0o600))
	assert.Equal(t, "kept", read(t, path))
}

func TestEnsureSecretsKeepsStaleFile(t *testing.T) {
	clearSecrets(t)
	dir := t.TempDir()
	require.NoError(t, EnsureSecrets(dir, nil))
	before := read(t, filepath.Join(dir, "amp-api-client-secret"))
	t.Setenv("IDP_CLIENT_SECRET", "mine")
	require.NoError(t, EnsureSecrets(dir, nil))
	assert.Equal(t, before, read(t, filepath.Join(dir, "amp-api-client-secret")))
}

func TestDBPasswordOnlyForPostgres(t *testing.T) {
	clearSecrets(t)
	dir := t.TempDir()
	t.Setenv("DB_DRIVER", "sqlite")
	require.NoError(t, EnsureSecrets(dir, nil))
	assert.NoFileExists(t, filepath.Join(dir, "db-password"))

	t.Setenv("DB_DRIVER", "postgres")
	require.NoError(t, EnsureSecrets(dir, nil))
	assert.Len(t, read(t, filepath.Join(dir, "db-password")), 24)
}

// TestSharedSecretsCopied: only the secrets other containers read are copied
// to AMP_SHARED_SECRETS_DIR, the one directory those containers mount.
func TestSharedSecretsCopied(t *testing.T) {
	clearSecrets(t)
	dir, shared := t.TempDir(), t.TempDir()
	t.Setenv("AMP_SHARED_SECRETS_DIR", shared)
	t.Setenv("DB_DRIVER", "postgres")
	require.NoError(t, EnsureSecrets(dir, nil))

	entries, err := os.ReadDir(shared)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"admin-password", "db-password"}, names)
	assert.Equal(t, read(t, filepath.Join(dir, "admin-password")), read(t, filepath.Join(shared, "admin-password")))
	info, err := os.Stat(filepath.Join(shared, "db-password"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())

	t.Setenv("AMP_ADMIN_PASSWORD", "mine")
	require.NoError(t, EnsureSecrets(dir, nil))
	assert.Equal(t, "mine", read(t, filepath.Join(shared, "admin-password")))
}
