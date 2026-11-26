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

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unset(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		t.Setenv(k, "")
		require.NoError(t, os.Unsetenv(k))
	}
}

func TestDataDirDefaults(t *testing.T) {
	reloadAfter(t)
	unset(t, "AMP_DATA_DIR", "AMP_SECRETS_DIR", "DB_PATH", "INTERNAL_SERVER_CERT_DIR")
	require.NoError(t, loadWith(t, "log_level: INFO\n"))
	assert.Equal(t, "data", DataDir())
	assert.Equal(t, filepath.Join("data", "secrets"), SecretsDir())
	assert.Equal(t, filepath.Join("data", "agent-manager.db"), GetConfig().Database.SQLitePath)
	assert.Equal(t, filepath.Join("data", "certs"), GetConfig().InternalServer.CertDir)

	require.NoError(t, loadWith(t, "amp: {data_dir: /var/lib/amp}\n"))
	assert.Equal(t, "/var/lib/amp", DataDir())
	assert.Equal(t, "/var/lib/amp/secrets", SecretsDir())
	assert.Equal(t, "/var/lib/amp/agent-manager.db", GetConfig().Database.SQLitePath)

	t.Setenv("AMP_SECRETS_DIR", "/run/amp-secrets")
	assert.Equal(t, "/run/amp-secrets", SecretsDir())
}

func TestAliasFallback(t *testing.T) {
	reloadAfter(t)
	unset(t, "SERVER_PUBLIC_URL", "AMP_PUBLIC_URL", "THUNDER_BASE_URL", "THUNDER_PUBLIC_URL")
	t.Setenv("THUNDER_CLIENT_SECRET", "s")

	t.Setenv("AMP_PUBLIC_URL", "http://old.example")
	require.NoError(t, loadWith(t, "log_level: INFO\n"))
	assert.Equal(t, "http://old.example", Value("SERVER_PUBLIC_URL"), "the old name still works")

	t.Setenv("SERVER_PUBLIC_URL", "http://new.example")
	assert.Equal(t, "http://new.example", Value("SERVER_PUBLIC_URL"), "the canonical name wins")

	unset(t, "AMP_PUBLIC_URL", "SERVER_PUBLIC_URL")
	require.NoError(t, loadWith(t, "thunder: {public_url: http://thunder.example}\n"))
	assert.Equal(t, "http://thunder.example", Value("THUNDER_BASE_URL"), "an old name in amp.yaml works")
	fileMu.Lock()
	defer fileMu.Unlock()
	assert.Empty(t, unknownKeys(map[string]string{"THUNDER_PUBLIC_URL": "x"}), "an old name is not a typo")
}

func TestDerivedDefaults(t *testing.T) {
	reloadAfter(t)
	unset(t, "KEY_MANAGER_JWKS_URL", "KEY_MANAGER_ISSUER", "KEY_MANAGER_AUDIENCE", "IDP_TOKEN_URL",
		"IDP_CLIENT_ID", "THUNDER_CLIENT_ID", "OAUTH_AUTHORIZATION_SERVERS", "THUNDER_RESOLVE_TO_HOST",
		"SERVER_PUBLIC_URL", "THUNDER_BASE_URL", "PLATFORM_ADMIN_OU_ID", "AUTH_MODE",
		"FILE_AUTH_USERS_PATH", "SERVER_PORT")
	t.Setenv("THUNDER_CLIENT_SECRET", "s")

	require.NoError(t, loadWith(t, `
server: {port: 9000, public_url: "http://localhost:9000"}
thunder: {base_url: "http://thunder.amp.localhost:8080", resolve_to_host: "thunder:8080"}
auth: {mode: file}
file_auth: {users_path: /tmp/users.yaml}
`))
	c := GetConfig()
	assert.Equal(t, "http://127.0.0.1:9000/auth/external/jwks.json", c.KeyManagerConfigurations.JWKSUrl)
	assert.Equal(t, []string{"Agent Management Platform Local", "http://thunder.amp.localhost:8080", "amp-file-auth"}, c.KeyManagerConfigurations.Issuer)
	assert.Equal(t, []string{
		"urn:wso2:amp", "localhost", "amp-api-client", "amp-console-client", "amctl", "am-mcp",
		"http://localhost:9000/mcp",
	}, c.KeyManagerConfigurations.Audience)
	assert.Equal(t, "http://thunder:8080/oauth2/token", c.IDP.TokenURL)
	assert.Equal(t, "amp-api-client", c.IDP.ClientID)
	assert.Equal(t, "amp-system-client", c.Thunder.ClientID)
	assert.Equal(t, []string{"http://thunder.amp.localhost:8080"}, c.OAuthAuthorizationServers)
	assert.Equal(t, DefaultTrialOrgOUID, c.PlatformAdminOUID, "file sign-in's organization is the platform one")

	require.NoError(t, loadWith(t, `
server: {port: 9000, public_url: "http://localhost:9000"}
thunder: {base_url: "http://thunder.amp.localhost:8080", resolve_to_host: "thunder:8080"}
auth: {mode: idp}
`))
	assert.Equal(t, "http://thunder:8080/oauth2/jwks", GetConfig().KeyManagerConfigurations.JWKSUrl)

	t.Setenv("KEY_MANAGER_JWKS_URL", "http://explicit/jwks")
	require.NoError(t, loadWith(t, "auth: {mode: idp}\n"))
	assert.Equal(t, "http://explicit/jwks", GetConfig().KeyManagerConfigurations.JWKSUrl)
}

// The trial organization is file sign-in's; with file sign-in it is also the platform organization, so the trial
// admin keeps the cross-organization routes. With an identity provider the platform organization stays ThunderID's.
func TestPlatformAdminDefaultFollowsAuthMode(t *testing.T) {
	reloadAfter(t)
	unset(t, "PLATFORM_ADMIN_OU_ID", "DEFAULT_ORG_OU_ID", "DEFAULT_ORG_HANDLE", "AUTH_MODE", "FILE_AUTH_USERS_PATH")
	t.Setenv("THUNDER_CLIENT_SECRET", "s")
	base := "server: {port: 9000, public_url: \"http://localhost:9000\"}\n" +
		"thunder: {base_url: \"http://thunder.amp.localhost:8080\"}\n"
	file := base + "auth: {mode: file}\nfile_auth: {users_path: /tmp/users.yaml}\n"

	require.NoError(t, loadWith(t, file))
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", GetConfig().DefaultOrg.OUID)
	assert.Equal(t, "default", GetConfig().DefaultOrg.Handle)
	t.Setenv("DEFAULT_ORG_HANDLE", "  Acme ")
	require.NoError(t, loadWith(t, file))
	assert.Equal(t, "acme", GetConfig().DefaultOrg.Handle, "normalised as amp init writes it into users.yaml")
	unset(t, "DEFAULT_ORG_HANDLE")
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", GetConfig().PlatformAdminOUID)

	t.Setenv("DEFAULT_ORG_OU_ID", "   ")
	require.NoError(t, loadWith(t, file))
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", GetConfig().DefaultOrg.OUID, "blank is unset")
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", GetConfig().PlatformAdminOUID)
	unset(t, "DEFAULT_ORG_OU_ID")

	require.NoError(t, loadWith(t, file+"default: {org_ou_id: 0190a000-0000-7000-8000-0000000000bb}\n"))
	assert.Equal(t, "0190a000-0000-7000-8000-0000000000bb", GetConfig().PlatformAdminOUID, "follows the trial organization")

	require.NoError(t, loadWith(t, base+"auth: {mode: idp}\ndefault: {org_ou_id: 0190a000-0000-7000-8000-0000000000bb}\n"))
	assert.Equal(t, "01900000-0000-7000-8000-000000000001", GetConfig().PlatformAdminOUID, "an identity provider's stays ThunderID's")

	// Set but empty denies every caller the cross-organization routes, in either mode.
	t.Setenv("PLATFORM_ADMIN_OU_ID", "")
	for _, cfg := range []string{file, base + "auth: {mode: idp}\n"} {
		require.NoError(t, loadWith(t, cfg))
		assert.Empty(t, GetConfig().PlatformAdminOUID, "an explicitly empty value stays empty")
	}

	t.Setenv("PLATFORM_ADMIN_OU_ID", "0190a000-0000-7000-8000-0000000000cc")
	for _, cfg := range []string{file, base + "auth: {mode: idp}\n"} {
		require.NoError(t, loadWith(t, cfg))
		assert.Equal(t, "0190a000-0000-7000-8000-0000000000cc", GetConfig().PlatformAdminOUID, "an explicit value wins")
	}
}
