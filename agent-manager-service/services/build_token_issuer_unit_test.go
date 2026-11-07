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

package services

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
)

const (
	btOrg          = "6f1e2c3d-0000-4000-8000-000000000001"
	btOtherOrg     = "6f1e2c3d-0000-4000-8000-000000000002"
	btClientID     = "AGENT_MANAGER_SYSTEM_APP"
	btClientSecret = "system-secret"
	btSecretRef    = "cred-amp-build-token-abc123"
)

type btBackgroundKey struct{}

// btToken returns a JWT with the given claims, as the identity provider issues it.
func btToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-signing-key"))
	require.NoError(t, err)
	return token
}

func btClaims(ouID, clientID string, expiresIn time.Duration) jwt.MapClaims {
	return jwt.MapClaims{
		"ouId":      ouID,
		"client_id": clientID,
		"aud":       clientID,
		"exp":       time.Now().Add(expiresIn).Unix(),
	}
}

// btIdP is a fake identity provider token endpoint. It records the request it
// received and answers with status and body.
type btIdP struct {
	mu        sync.Mutex
	path      string
	authz     string
	grantType string
	form      map[string][]string
	calls     int
	status    int
	body      string
}

func (f *btIdP) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.calls++
		f.path = r.URL.Path
		f.authz = r.Header.Get("Authorization")
		f.grantType = r.PostForm.Get("grant_type")
		f.form = r.PostForm
		status, body := f.status, f.body
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// btStore records what the issuer stored.
type btStore struct {
	ctx      context.Context
	location secretmanagersvc.SecretLocation
	data     map[string]string
	calls    int
}

func (s *btStore) mock(ref string, err error) *clientmocks.SecretManagementClientMock {
	return &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(ctx context.Context, location secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
			s.calls++
			s.ctx, s.location, s.data = ctx, location, data
			return ref, err
		},
	}
}

func newTestBuildTokenIssuer(srv *httptest.Server, store *clientmocks.SecretManagementClientMock) *buildTokenIssuer {
	return newBuildTokenIssuer(srv.URL+"/oauth2/token", btClientID, btClientSecret, srv.Client(), store, discardLogger())
}

func TestNewBuildTokenIssuer_DisabledIsNil(t *testing.T) {
	cfg := config.Config{BuildClient: config.BuildClientConfig{Enabled: false}}
	assert.Nil(t, NewBuildTokenIssuer(cfg, discardLogger(), &clientmocks.SecretManagementClientMock{}))
}

func TestNewBuildTokenIssuer_EnabledUsesTheServiceClient(t *testing.T) {
	cfg := config.Config{
		IDP:         config.IDPConfig{TokenURL: "https://idp.example.com/oauth2/token", ClientID: btClientID, ClientSecret: btClientSecret},
		BuildClient: config.BuildClientConfig{Enabled: true, Workflows: []string{"amp-docker"}},
	}
	issuer := NewBuildTokenIssuer(cfg, discardLogger(), &clientmocks.SecretManagementClientMock{})
	require.NotNil(t, issuer)
	impl, ok := issuer.(*buildTokenIssuer)
	require.True(t, ok)
	assert.Equal(t, "https://idp.example.com/oauth2/token", impl.idpTokenURL)
	assert.Equal(t, btClientID, impl.clientID)
	assert.Equal(t, btClientSecret, impl.clientSecret)
	_, isSetter := issuer.(BuildTokenIssuerBackgroundContextSetter)
	assert.True(t, isSetter, "app.Run must be able to install the background context")
}

func TestBuildTokenIssuer_IssuesAndStoresAnOrganizationToken(t *testing.T) {
	token := btToken(t, btClaims(btOrg, btClientID, time.Hour))
	idp := &btIdP{status: http.StatusOK, body: `{"access_token":"` + token + `","token_type":"Bearer","expires_in":3600}`}
	srv := idp.server(t)
	store := &btStore{}
	issuer := newTestBuildTokenIssuer(srv, store.mock(btSecretRef, nil))
	bg := context.WithValue(context.Background(), btBackgroundKey{}, "background")
	issuer.SetBackgroundContext(bg)

	// The request carries the user's token; the build token must not be stored with it.
	reqCtx := jwtassertion.ContextWithJWT(auditableCtx(t), "user-token")
	got, err := issuer.IssueBuildToken(reqCtx, btOrg)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, btSecretRef, got.SecretRefName)

	// The organization token endpoint, with the service's own client.
	assert.Equal(t, 1, idp.calls)
	assert.Equal(t, "/ou/"+btOrg+"/oauth2/token", idp.path)
	assert.Equal(t, "client_credentials", idp.grantType)
	assert.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte(btClientID+":"+btClientSecret)), idp.authz)
	assert.NotContains(t, idp.form, "scope", "no scope is requested, so the audience is the client ID")
	assert.NotContains(t, idp.form, "resource", "no resource is requested, so the audience is the client ID")
	assert.NotContains(t, idp.form, "client_secret", "the secret travels only in the Authorization header")

	// Stored under the organization, as the only entry, with the service's credential.
	require.Equal(t, 1, store.calls)
	assert.Equal(t, secretmanagersvc.SecretLocation{OrgName: btOrg, EntityName: "amp-build-token"}, store.location)
	assert.Equal(t, map[string]string{"access-token": token}, store.data)
	require.NotNil(t, store.ctx)
	assert.Empty(t, jwtassertion.GetJWTFromContext(store.ctx), "the user's token must not be used to store the build token")
	assert.Equal(t, "background", store.ctx.Value(btBackgroundKey{}))
	org, ok := orgctx.GetResolvedOrg(store.ctx)
	require.True(t, ok)
	assert.Equal(t, btOrg, org.OUID)
	_, hasDeadline := store.ctx.Deadline()
	assert.True(t, hasDeadline, "the secret store call is bounded")
}

func TestBuildTokenIssuer_RefusesTokensThatAreNotThisOrganizationsBuildToken(t *testing.T) {
	tests := []struct {
		name    string
		token   func(t *testing.T) string
		wantErr string
	}{
		{
			name:    "token for another organization",
			token:   func(t *testing.T) string { return btToken(t, btClaims(btOtherOrg, btClientID, time.Hour)) },
			wantErr: "issued for organization",
		},
		{
			name: "token naming no organization",
			token: func(t *testing.T) string {
				c := btClaims(btOrg, btClientID, time.Hour)
				delete(c, "ouId")
				return btToken(t, c)
			},
			wantErr: "issued for organization",
		},
		{
			name:    "token issued to another client",
			token:   func(t *testing.T) string { return btToken(t, btClaims(btOrg, "OTHER_APP", time.Hour)) },
			wantErr: "issued to client",
		},
		{
			name: "token without an expiry",
			token: func(t *testing.T) string {
				c := btClaims(btOrg, btClientID, time.Hour)
				delete(c, "exp")
				return btToken(t, c)
			},
			wantErr: "no valid expiry",
		},
		{
			name:    "token that expires too soon for a build",
			token:   func(t *testing.T) string { return btToken(t, btClaims(btOrg, btClientID, 5*time.Minute)) },
			wantErr: "less than the 10m0s a build needs",
		},
		{
			name:    "opaque token",
			token:   func(*testing.T) string { return "opaque-access-token" },
			wantErr: "not a readable JWT",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			token := tc.token(t)
			idp := &btIdP{status: http.StatusOK, body: `{"access_token":"` + token + `"}`}
			store := &btStore{}
			issuer := newTestBuildTokenIssuer(idp.server(t), store.mock(btSecretRef, nil))

			got, err := issuer.IssueBuildToken(auditableCtx(t), btOrg)
			require.Error(t, err)
			assert.Nil(t, got)
			require.ErrorIs(t, err, errBuildToken)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.NotContains(t, err.Error(), token, "the token must never appear in an error")
			assert.Zero(t, store.calls, "a refused token must not be stored")
		})
	}
}

func TestBuildTokenIssuer_Failures(t *testing.T) {
	validToken := func(t *testing.T) string { return btToken(t, btClaims(btOrg, btClientID, time.Hour)) }

	t.Run("identity provider refuses the client", func(t *testing.T) {
		idp := &btIdP{status: http.StatusUnauthorized, body: `{"error":"invalid_client"}`}
		store := &btStore{}
		issuer := newTestBuildTokenIssuer(idp.server(t), store.mock(btSecretRef, nil))
		_, err := issuer.IssueBuildToken(auditableCtx(t), btOrg)
		require.ErrorIs(t, err, errBuildToken)
		assert.Contains(t, err.Error(), "token request failed")
		assert.NotContains(t, err.Error(), btClientSecret)
		assert.Zero(t, store.calls)
	})

	t.Run("identity provider returns no token", func(t *testing.T) {
		idp := &btIdP{status: http.StatusOK, body: `{"token_type":"Bearer"}`}
		store := &btStore{}
		issuer := newTestBuildTokenIssuer(idp.server(t), store.mock(btSecretRef, nil))
		_, err := issuer.IssueBuildToken(auditableCtx(t), btOrg)
		require.ErrorIs(t, err, errBuildToken)
		assert.Contains(t, err.Error(), "no access token")
		assert.Zero(t, store.calls)
	})

	t.Run("secret store fails", func(t *testing.T) {
		token := validToken(t)
		idp := &btIdP{status: http.StatusOK, body: `{"access_token":"` + token + `"}`}
		storeErr := errors.New("secret manager unavailable")
		store := &btStore{}
		issuer := newTestBuildTokenIssuer(idp.server(t), store.mock("", storeErr))
		_, err := issuer.IssueBuildToken(auditableCtx(t), btOrg)
		require.ErrorIs(t, err, errBuildToken)
		require.ErrorIs(t, err, storeErr)
		assert.NotContains(t, err.Error(), token)
	})

	t.Run("secret store returns no SecretReference", func(t *testing.T) {
		idp := &btIdP{status: http.StatusOK, body: `{"access_token":"` + validToken(t) + `"}`}
		store := &btStore{}
		issuer := newTestBuildTokenIssuer(idp.server(t), store.mock("", nil))
		_, err := issuer.IssueBuildToken(auditableCtx(t), btOrg)
		require.ErrorIs(t, err, errBuildToken)
		assert.Contains(t, err.Error(), "no SecretReference")
	})

	t.Run("refuses to issue a token without an audit trail", func(t *testing.T) {
		idp := &btIdP{status: http.StatusOK, body: `{"access_token":"` + validToken(t) + `"}`}
		store := &btStore{}
		issuer := newTestBuildTokenIssuer(idp.server(t), store.mock(btSecretRef, nil))
		_, err := issuer.IssueBuildToken(context.Background(), btOrg)
		require.ErrorIs(t, err, audit.ErrRecorderUnavailable)
		assert.Zero(t, idp.calls, "no token is requested before the intent is recorded")
		assert.Zero(t, store.calls)
	})

	t.Run("refuses an empty organization", func(t *testing.T) {
		idp := &btIdP{status: http.StatusOK}
		store := &btStore{}
		issuer := newTestBuildTokenIssuer(idp.server(t), store.mock(btSecretRef, nil))
		_, err := issuer.IssueBuildToken(auditableCtx(t), " ")
		require.ErrorIs(t, err, errBuildToken)
		assert.Zero(t, idp.calls)
	})

	t.Run("refuses an organization that is not one path segment", func(t *testing.T) {
		idp := &btIdP{status: http.StatusOK}
		store := &btStore{}
		issuer := newTestBuildTokenIssuer(idp.server(t), store.mock(btSecretRef, nil))
		_, err := issuer.IssueBuildToken(auditableCtx(t), "../"+btOtherOrg)
		require.ErrorIs(t, err, errBuildToken)
		assert.Zero(t, idp.calls)
	})
}

func TestBuildTokenIssuer_WithoutABackgroundContextStoresWithAFreshContext(t *testing.T) {
	// Before app.Run installs the background context (and in tests), the store
	// still gets a context without the request's token.
	token := btToken(t, btClaims(btOrg, btClientID, time.Hour))
	idp := &btIdP{status: http.StatusOK, body: `{"access_token":"` + token + `"}`}
	store := &btStore{}
	issuer := newTestBuildTokenIssuer(idp.server(t), store.mock(btSecretRef, nil))

	_, err := issuer.IssueBuildToken(jwtassertion.ContextWithJWT(auditableCtx(t), "user-token"), btOrg)
	require.NoError(t, err)
	require.NotNil(t, store.ctx)
	assert.Empty(t, jwtassertion.GetJWTFromContext(store.ctx))
}

func TestBuildTokenIssuer_StoreIsCancelledWithTheRequest(t *testing.T) {
	token := btToken(t, btClaims(btOrg, btClientID, time.Hour))
	idp := &btIdP{status: http.StatusOK, body: `{"access_token":"` + token + `"}`}
	reqCtx, cancelReq := context.WithCancel(auditableCtx(t))
	defer cancelReq()
	var storeErr error
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(ctx context.Context, _ secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
			// The request ends while the token is being stored.
			cancelReq()
			select {
			case <-ctx.Done():
				storeErr = ctx.Err()
				return "", ctx.Err()
			case <-time.After(5 * time.Second):
				return "", errors.New("the store was not cancelled with the request")
			}
		},
	}
	issuer := newTestBuildTokenIssuer(idp.server(t), store)

	_, err := issuer.IssueBuildToken(reqCtx, btOrg)
	require.ErrorIs(t, err, errBuildToken)
	require.ErrorIs(t, storeErr, context.Canceled)
}

func TestBuildTokenIssuer_BudgetsStayUnderAGatewayTimeout(t *testing.T) {
	// Issuing runs inside the request that starts a build; a 30s gateway timeout
	// must not cut it short.
	assert.LessOrEqual(t, buildTokenRequestTimeout+buildTokenStoreTimeout, 25*time.Second)
}

func TestAgentManagerBuildClientFor(t *testing.T) {
	t.Run("nil when build tokens are disabled", func(t *testing.T) {
		s := &agentManagerService{}
		assert.Nil(t, s.buildClientFor(btOrg))
	})

	t.Run("issues the token for the build's organization", func(t *testing.T) {
		token := btToken(t, btClaims(btOrg, btClientID, time.Hour))
		idp := &btIdP{status: http.StatusOK, body: `{"access_token":"` + token + `"}`}
		store := &btStore{}
		s := &agentManagerService{buildTokens: newTestBuildTokenIssuer(idp.server(t), store.mock(btSecretRef, nil))}

		source := s.buildClientFor(btOrg)
		require.NotNil(t, source)
		assert.Zero(t, idp.calls, "nothing is issued until the run's workflow takes the token")
		got, err := source(auditableCtx(t))
		require.NoError(t, err)
		assert.Equal(t, btSecretRef, got.SecretRefName)
		assert.True(t, strings.HasSuffix(idp.path, "/ou/"+btOrg+"/oauth2/token"))
	})
}
