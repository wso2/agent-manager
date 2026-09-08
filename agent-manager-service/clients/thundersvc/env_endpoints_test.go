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

package thundersvc

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type fixedEndpointResolver struct {
	endpoints EnvThunderEndpoints
	err       error
}

func (r fixedEndpointResolver) ResolveEndpoints(context.Context, string, string, string, string, bool) (EnvThunderEndpoints, error) {
	return r.endpoints, r.err
}

func TestResolveEnvThunderTokenEndpointUsesDeploymentResolver(t *testing.T) {
	t.Parallel()
	readURL := func(context.Context, string, string) (string, bool, error) {
		return "https://public.example.com", true, nil
	}
	got, err := ResolveEnvThunderTokenEndpoint(context.Background(), fixedEndpointResolver{endpoints: EnvThunderEndpoints{TokenURL: "https://public.example.com/oauth2/token"}}, readURL, "ou-id", "org", "customer-test")
	require.NoError(t, err)
	require.Equal(t, "https://public.example.com/oauth2/token", got)
}

func TestResolveEnvThunderTokenEndpointPreservesNotProvisioned(t *testing.T) {
	t.Parallel()
	readURL := func(context.Context, string, string) (string, bool, error) { return "", false, nil }
	_, err := ResolveEnvThunderTokenEndpoint(context.Background(), nil, readURL, "ou-id", "org", "dev-eu")
	require.ErrorIs(t, err, ErrThunderNotProvisioned)
}

func TestResolveEnvThunderTokenEndpointRejectsEmptyResolvedURL(t *testing.T) {
	t.Parallel()
	readURL := func(context.Context, string, string) (string, bool, error) {
		return "https://public.example.com", true, nil
	}
	_, err := ResolveEnvThunderTokenEndpoint(context.Background(), fixedEndpointResolver{}, readURL, "ou-id", "org", "dev-eu")
	require.ErrorContains(t, err, "token URL is empty")
}

func TestDefaultEnvThunderEndpointResolverPreservesOnPremisesSemantics(t *testing.T) {
	t.Parallel()
	resolver := defaultEnvThunderEndpointResolver{resolveBaseURL: func(context.Context, string, string, string, bool) (string, string, bool) {
		return "https://issuer.example.com", "10.0.0.10:8090", true
	}}
	endpoints, err := resolver.ResolveEndpoints(context.Background(), "ou-id", "org", "dev-eu", "https://issuer.example.com", false)
	require.NoError(t, err)
	require.Equal(t, "https://issuer.example.com", endpoints.ManagementURL)
	require.Equal(t, "10.0.0.10:8090", endpoints.ManagementResolveToHost)
	require.True(t, endpoints.ManagementURLTrusted)
	require.Equal(t, "https://issuer.example.com/oauth2/token", endpoints.TokenURL)
	require.Equal(t, "10.0.0.10:8090", endpoints.TokenResolveToHost)
	require.True(t, endpoints.TokenURLTrusted)
	require.Equal(t, "system", endpoints.SystemTokenScope)
	require.Equal(t, "https://issuer.example.com/mcp", endpoints.SystemTokenResource)
}

func TestEnvThunderClientUsesSeparateTokenAndManagementEndpoints(t *testing.T) {
	t.Parallel()
	tokenCalled := false
	managementCalled := false
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalled = true
		require.Equal(t, "/oauth2/token", r.URL.Path)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "tenant_instance:system", r.Form.Get("scope"))
		require.Empty(t, r.Form.Get("resource"))
		_, _ = io.WriteString(w, `{"access_token":"system-token","expires_in":300}`)
	}))
	defer tokenServer.Close()
	managementServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		managementCalled = true
		require.Equal(t, "/organization-units/tree/default", r.URL.Path)
		require.Equal(t, "Bearer system-token", r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, `{"id":"default-ou"}`)
	}))
	defer managementServer.Close()

	client, err := NewEnvThunderClientWithEndpoints(EnvThunderEndpoints{
		ManagementURL:        managementServer.URL,
		ManagementURLTrusted: true,
		TokenURL:             tokenServer.URL + "/oauth2/token",
		TokenURLTrusted:      true,
		SystemTokenScope:     "tenant_instance:system",
	}, "client-id", "client-secret")
	require.NoError(t, err)
	ouID, err := client.GetDefaultOUID(context.Background())
	require.NoError(t, err)
	require.Equal(t, "default-ou", ouID)
	require.True(t, tokenCalled)
	require.True(t, managementCalled)
}

func TestEnvThunderClientProtectsUntrustedTokenEndpointByDefault(t *testing.T) {
	t.Parallel()
	var tokenCalled atomic.Bool
	tokenServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		tokenCalled.Store(true)
	}))
	defer tokenServer.Close()

	client, err := NewEnvThunderClientWithEndpoints(EnvThunderEndpoints{
		ManagementURL:        "https://management.example.com",
		ManagementURLTrusted: true,
		TokenURL:             tokenServer.URL + "/oauth2/token",
		SystemTokenScope:     "tenant_instance:system",
	}, "client-id", "client-secret")
	require.NoError(t, err)
	_, err = client.GetDefaultOUID(context.Background())
	require.Error(t, err)
	require.False(t, tokenCalled.Load(), "SSRF guard must reject loopback before credentials are sent")
}

func TestEnvThunderClientProtectsUntrustedManagementEndpointByDefault(t *testing.T) {
	t.Parallel()
	var managementCalled atomic.Bool
	managementServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		managementCalled.Store(true)
	}))
	defer managementServer.Close()
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"access_token":"system-token","expires_in":300}`)
	}))
	defer tokenServer.Close()

	client, err := NewEnvThunderClientWithEndpoints(EnvThunderEndpoints{
		ManagementURL:    managementServer.URL,
		TokenURL:         tokenServer.URL + "/oauth2/token",
		TokenURLTrusted:  true,
		SystemTokenScope: "tenant_instance:system",
	}, "client-id", "client-secret")
	require.NoError(t, err)
	_, err = client.GetDefaultOUID(context.Background())
	require.Error(t, err)
	require.False(t, managementCalled.Load(), "SSRF guard must reject loopback management URL")
}

func TestEnvThunderClientRejectsUnsafeEndpointShapes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		managementURL string
		tokenURL      string
	}{
		{name: "relative management URL", managementURL: "/thunder", tokenURL: "https://token.example.com/oauth2/token"},
		{name: "non HTTP token URL", managementURL: "https://management.example.com", tokenURL: "file:///oauth2/token"},
		{name: "credentials in token URL", managementURL: "https://user:YOUR_PASSWORD_HERE@management.example.com", tokenURL: "https://token.example.com/oauth2/token"},
		{name: "fragment in token URL", managementURL: "https://management.example.com", tokenURL: "https://token.example.com/oauth2/token#fragment"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewEnvThunderClientWithEndpoints(EnvThunderEndpoints{
				ManagementURL:    test.managementURL,
				TokenURL:         test.tokenURL,
				SystemTokenScope: "tenant_instance:system",
			}, "client-id", "client-secret")
			require.Error(t, err)
		})
	}
}
