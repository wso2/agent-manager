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
package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

// firstRunEnv points the data directory at a fresh temp dir with nothing set.
func firstRunEnv(t *testing.T, authMode string) string {
	t.Helper()
	data := t.TempDir()
	for _, k := range []string{
		"AMP_SECRETS_DIR", "ENCRYPTION_KEY", "JWT_SIGNING_PRIVATE_KEY_PATH", "JWT_SIGNING_PUBLIC_KEYS_CONFIG",
		"FILE_AUTH_USERS_PATH", "AMP_ADMIN_PASSWORD", "IDP_CLIENT_SECRET", "AMP_API_CLIENT_SECRET",
		"DB_DRIVER", "DB_HOST", "AMP_FILE_OWNER", "AMP_CONFIG",
	} {
		t.Setenv(k, "")
		require.NoError(t, os.Unsetenv(k))
	}
	t.Setenv("AMP_DATA_DIR", data)
	t.Setenv("AUTH_MODE", authMode)
	t.Chdir(t.TempDir())
	require.NoError(t, config.LoadUnchecked(""))
	return data
}

func noData(context.Context) (bool, error)  { return false, nil }
func hasData(context.Context) (bool, error) { return true, nil }

func TestFirstRunCreatesKeyAndJWT(t *testing.T) {
	data := firstRunEnv(t, "idp")
	require.NoError(t, firstRun(context.Background(), noData))

	key, err := os.ReadFile(filepath.Join(data, "secrets", "encryption-key"))
	require.NoError(t, err)
	assert.Regexp(t, "^[0-9a-f]{64}$", string(key))
	priv := filepath.Join(data, "secrets", "jwt", "private.pem")
	assert.FileExists(t, priv)
	assert.FileExists(t, filepath.Join(data, "secrets", "jwt", "public.pem"))
	var keys config.PublicKeysConfig
	raw, err := os.ReadFile(filepath.Join(data, "secrets", "jwt", "public-keys-config.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &keys))
	require.Len(t, keys.Keys, 1)
	assert.Equal(t, "key-1", keys.Keys[0].Kid)
	assert.Equal(t, "public.pem", keys.Keys[0].PublicKeyPath)

	before, _ := os.ReadFile(priv)
	require.NoError(t, firstRun(context.Background(), hasData))
	after, _ := os.ReadFile(priv)
	assert.Equal(t, before, after, "kept")
	again, _ := os.ReadFile(filepath.Join(data, "secrets", "encryption-key"))
	assert.Equal(t, key, again, "kept, even once the database has data")
}

func TestFirstRunRefusesKeyForExistingData(t *testing.T) {
	data := firstRunEnv(t, "idp")
	err := firstRun(context.Background(), hasData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ENCRYPTION_KEY")
	assert.Contains(t, err.Error(), filepath.Join(data, "secrets", "encryption-key"))
	assert.NoFileExists(t, filepath.Join(data, "secrets", "encryption-key"))
}

func TestFirstRunKeepsSetKey(t *testing.T) {
	data := firstRunEnv(t, "idp")
	t.Setenv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	require.NoError(t, firstRun(context.Background(), hasData))
	assert.NoFileExists(t, filepath.Join(data, "secrets", "encryption-key"))
}

func TestFirstRunHalfJWTPair(t *testing.T) {
	data := firstRunEnv(t, "idp")
	dir := filepath.Join(data, "secrets", "jwt")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "private.pem"), []byte("x"), 0o600))
	err := firstRun(context.Background(), noData)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "public-keys-config.json")
}

func TestFirstRunWritesFileAuthUsers(t *testing.T) {
	data := firstRunEnv(t, "file")
	require.NoError(t, firstRun(context.Background(), noData))
	users, err := os.ReadFile(filepath.Join(data, "fileauth", "users.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(users), "admin")
}
