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

package wiring

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/aesgcm"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/vault"
)

func TestProvideAuthMiddlewareAdvertisesResourceSpecificMetadata(t *testing.T) {
	middleware := ProvideAuthMiddleware(config.Config{
		AuthHeader:      "Authorization",
		ServerPublicURL: "https://api.example.com/",
	})
	handler := middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "REST API", path: "/api/v1/projects", want: "https://api.example.com/.well-known/oauth-protected-resource"},
		{name: "MCP", path: "/mcp", want: "https://api.example.com/.well-known/oauth-protected-resource/mcp"},
		{name: "MCP subpath", path: "/mcp/", want: "https://api.example.com/.well-known/oauth-protected-resource/mcp"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
			challenge := rec.Header().Get("WWW-Authenticate")
			if !strings.Contains(challenge, `resource_metadata="`+tc.want+`"`) {
				t.Fatalf("expected challenge to advertise %q, got %q", tc.want, challenge)
			}
		})
	}
}

func TestSecretVaultRolloutCompatibility(t *testing.T) {
	ctx := t.Context()
	key := bytes.Repeat([]byte{1}, aesgcm.KeySize)
	ref := vault.Ref{OrgID: uuid.NewString(), SecretID: uuid.New()}
	cfg := config.Config{}
	compatible, err := ProvideSecretVaults(cfg, utils.SecretRefKeys{key})
	require.NoError(t, err)
	rotated := []byte("rotated webhook secret")
	sealed, err := compatible.Writer().Seal(ctx, ref, rotated, nil)
	require.NoError(t, err)
	require.Empty(t, sealed.KeyID)
	got, err := aesgcm.Decrypt(sealed.Ciphertext, key)
	require.NoError(t, err, "a rollback must read a credential rotated by the new binary")
	require.Equal(t, rotated, got)

	oldWrite := []byte("written by old binary")
	sealed.Ciphertext, err = aesgcm.Encrypt(oldWrite, key)
	require.NoError(t, err)
	got, err = compatible.Writer().Open(ctx, ref, sealed)
	require.NoError(t, err, "the new binary must read a credential overwritten by an old replica")
	require.Equal(t, oldWrite, got)

	cfg.SecretManager.VaultBindLocalRows = true
	bound, err := ProvideSecretVaults(cfg, utils.SecretRefKeys{bytes.Repeat([]byte{2}, aesgcm.KeySize), key})
	require.NoError(t, err)
	got, err = bound.Writer().Open(ctx, ref, sealed)
	require.NoError(t, err, "old writes remain readable after opting in and rotating the key")
	require.Equal(t, oldWrite, got)
	sealed, err = bound.Writer().Seal(ctx, ref, rotated, nil)
	require.NoError(t, err)
	require.NotEmpty(t, sealed.KeyID)
	_, err = bound.Writer().Open(ctx, vault.Ref{OrgID: ref.OrgID, SecretID: uuid.New()}, sealed)
	require.Error(t, err, "row binding must reject ciphertext opened for a different secret")
}
