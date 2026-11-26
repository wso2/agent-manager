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

package cmdutil

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

// recordedRequest is what the fake platform saw.
type recordedRequest struct {
	Path, Auth, Accept string
}

// fakePlatform serves the API (/api/v1, /m/ai-gateway), a token endpoint and
// the discovery documents on one host.
type fakePlatform struct {
	*httptest.Server
	mu        sync.Mutex
	requests  []recordedRequest
	tokenForm []url.Values
	// tokenResp is the JSON body (status 200) the token endpoint returns;
	// tokenStatus != 0 turns it into an OAuth error instead.
	tokenResp   map[string]any
	tokenStatus int
	configBody  string
}

func newFakePlatform(t *testing.T) *fakePlatform {
	t.Helper()
	p := &fakePlatform{
		tokenResp:  map[string]any{"access_token": "fresh", "refresh_token": "rotated", "token_type": "Bearer", "expires_in": 3600},
		configBody: `{}`,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resource": p.URL, "authorization_servers": []string{p.URL}, "scopes_supported": []string{"amp:a", "amp:b"},
		})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"authorization_endpoint": p.URL + "/oauth2/authorize", "token_endpoint": p.URL + "/oauth2/token",
		})
	})
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		p.mu.Lock()
		p.tokenForm = append(p.tokenForm, r.PostForm)
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if p.tokenStatus != 0 {
			w.WriteHeader(p.tokenStatus)
			_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"nope"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(p.tokenResp)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.requests = append(p.requests, recordedRequest{r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Accept")})
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/config":
			_, _ = w.Write([]byte(p.configBody))
		case "/api/v1/orgs":
			_, _ = w.Write([]byte(`{"organizations":[{"name":"acme","createdAt":"2026-01-01T00:00:00Z"}],"limit":20,"offset":0,"total":1}`))
		case "/m/ai-gateway/llm-providers":
			_, _ = w.Write([]byte(`{"list":[{"metadata":{"name":"zeta"}},{"metadata":{"name":"alpha"}}]}`))
		case "/m/ai-gateway/llm-provider-templates":
			_, _ = w.Write([]byte(`{"list":[{"metadata":{"name":"openai"}},{"metadata":{"name":"anthropic"}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	p.Server = httptest.NewServer(mux)
	t.Cleanup(p.Close)
	return p
}

func (p *fakePlatform) lastRequest(t *testing.T) recordedRequest {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests) == 0 {
		t.Fatal("platform saw no API request")
	}
	return p.requests[len(p.requests)-1]
}

func (p *fakePlatform) tokenRequests() []url.Values {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]url.Values(nil), p.tokenForm...)
}

// factoryFor builds a real Factory (NewFactory) over a config holding a
// single instance "work" pointed at the fake platform.
func factoryFor(t *testing.T, p *fakePlatform, auth config.AuthConfig) (*Factory, *config.Config) {
	t.Helper()
	cfg := &config.Config{
		Path:            filepath.Join(t.TempDir(), "config.yaml"),
		CurrentInstance: "work",
		Instances: map[string]config.Instance{"work": {
			URL: p.URL + "/", TokenURL: p.URL + "/oauth2/token", Auth: auth,
		}},
	}
	io, _, _, _ := iostreams.Test()
	return NewFactory(cfg, io), cfg
}

func TestNewFactory_SignsRequestsWithBearerToken(t *testing.T) {
	p := newFakePlatform(t)
	f, _ := factoryFor(t, p, config.AuthConfig{AccessToken: "static-token"})

	client, err := f.AgentManager(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.ListOrganizationsWithResponse(context.Background(), nil)
	if err != nil || resp.JSON200 == nil {
		t.Fatalf("err=%v resp=%v", err, resp)
	}
	got := p.lastRequest(t)
	if got.Path != "/api/v1/orgs" {
		t.Errorf("path = %q (instance URL trailing slash must be trimmed)", got.Path)
	}
	if got.Auth != "Bearer static-token" || got.Accept != "application/json" {
		t.Errorf("auth/accept = %q / %q", got.Auth, got.Accept)
	}
	if len(p.tokenRequests()) != 0 {
		t.Error("a non-expiring token must not be refreshed")
	}
}

func TestNewFactory_AIGatewayUsesModulePath(t *testing.T) {
	p := newFakePlatform(t)
	f, _ := factoryFor(t, p, config.AuthConfig{AccessToken: "tok"})

	client, err := f.AIGateway(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.ListLLMProvidersWithResponse(context.Background(), nil)
	if err != nil || resp.JSON200 == nil {
		t.Fatalf("err=%v resp=%v", err, resp)
	}
	if got := p.lastRequest(t); got.Path != "/m/ai-gateway/llm-providers" || got.Auth != "Bearer tok" {
		t.Errorf("request = %+v", got)
	}
	if ModuleBase("x") != "/m/x" {
		t.Errorf("ModuleBase = %q", ModuleBase("x"))
	}
}

func TestNewFactory_NoInstanceAndConfigErrors(t *testing.T) {
	io, _, _, _ := iostreams.Test()
	f := NewFactory(&config.Config{Instances: map[string]config.Instance{}}, io)
	for name, call := range map[string]func() error{
		"agent manager": func() error { _, err := f.AgentManager(context.Background()); return err },
		"ai gateway":    func() error { _, err := f.AIGateway(context.Background()); return err },
		"token":         func() error { _, err := f.Token(context.Background()); return err },
		"observer":      func() error { _, err := f.Observer(context.Background()); return err },
	} {
		t.Run(name, func(t *testing.T) {
			var ce clierr.CLIError
			if err := call(); !errors.As(err, &ce) || ce.Code != clierr.NoInstance {
				t.Fatalf("err = %v, want %s", err, clierr.NoInstance)
			}
		})
	}

	f.Config = func() (*config.Config, error) { return nil, errors.New("disk on fire") }
	_, err := f.Token(context.Background())
	var ce clierr.CLIError
	if !errors.As(err, &ce) || ce.Code != clierr.ConfigNotLoaded || !strings.Contains(ce.Message, "disk on fire") {
		t.Fatalf("err = %v", err)
	}
	if _, err := f.AgentManager(context.Background()); !errors.As(err, &ce) || ce.Code != clierr.ConfigNotLoaded {
		t.Fatalf("agent manager err = %v", err)
	}
}

func TestToken_ExpiredClientCredentials_RefreshesAndPersists(t *testing.T) {
	p := newFakePlatform(t)
	f, cfg := factoryFor(t, p, config.AuthConfig{
		GrantType: "client_credentials", ClientID: "svc", ClientSecret: "s",
		AccessToken: "stale", ExpiresAt: time.Now().Add(-time.Minute),
		Scopes: []string{"amp:x"}, Resource: "urn:wso2:amp",
	})

	tok, err := f.Token(context.Background())
	if err != nil || tok != "fresh" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	form := p.tokenRequests()[0]
	if form.Get("grant_type") != "client_credentials" || form.Get("resource") != "urn:wso2:amp" || form.Get("scope") != "amp:x" {
		t.Errorf("refresh request = %v", form)
	}

	// The new token is saved so the next process does not refresh again.
	saved, err := config.Load(cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	got := saved.Instances["work"].Auth
	if got.AccessToken != "fresh" || got.ExpiresAt.Before(time.Now().Add(50*time.Minute)) {
		t.Errorf("persisted auth = %+v", got)
	}
	if got.ClientSecret != "s" {
		t.Error("client secret must survive a refresh")
	}
}

func TestToken_ExpiredWithoutStoredScopesRediscoversThem(t *testing.T) {
	p := newFakePlatform(t)
	f, _ := factoryFor(t, p, config.AuthConfig{
		GrantType: "client_credentials", ClientID: "svc", ClientSecret: "s",
		AccessToken: "stale", ExpiresAt: time.Now().Add(-time.Minute),
	})
	if _, err := f.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := p.tokenRequests()[0].Get("scope"); got != "amp:a amp:b" {
		t.Errorf("scope = %q, want the discovered scopes", got)
	}
}

func TestToken_TokenInsideRefreshBufferIsRefreshed(t *testing.T) {
	p := newFakePlatform(t)
	f, _ := factoryFor(t, p, config.AuthConfig{
		GrantType: "client_credentials", ClientID: "svc", ClientSecret: "s",
		AccessToken: "about-to-die", ExpiresAt: time.Now().Add(refreshBuffer - time.Minute), Scopes: []string{"x"},
	})
	if tok, err := f.Token(context.Background()); err != nil || tok != "fresh" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}

	// Comfortably valid: no round trip.
	f2, _ := factoryFor(t, p, config.AuthConfig{
		GrantType: "client_credentials", AccessToken: "still-good", ExpiresAt: time.Now().Add(time.Hour),
	})
	n := len(p.tokenRequests())
	if tok, err := f2.Token(context.Background()); err != nil || tok != "still-good" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	if len(p.tokenRequests()) != n {
		t.Error("valid token triggered a refresh")
	}
}

func TestToken_ClientCredentialsRefreshFailures(t *testing.T) {
	p := newFakePlatform(t)
	expired := func(a config.AuthConfig) *Factory {
		a.GrantType, a.AccessToken, a.ExpiresAt = "client_credentials", "stale", time.Now().Add(-time.Hour)
		f, _ := factoryFor(t, p, a)
		return f
	}

	t.Run("missing credentials", func(t *testing.T) {
		_, err := expired(config.AuthConfig{ClientID: "svc"}).Token(context.Background())
		var ce clierr.CLIError
		if !errors.As(err, &ce) || ce.Code != clierr.AuthRefreshFailed || !strings.Contains(ce.Message, "missing credentials") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("revoked client", func(t *testing.T) {
		p.tokenStatus = http.StatusUnauthorized
		defer func() { p.tokenStatus = 0 }()
		_, err := expired(config.AuthConfig{ClientID: "svc", ClientSecret: "s", Scopes: []string{"x"}}).Token(context.Background())
		var ce clierr.CLIError
		if !errors.As(err, &ce) || ce.Code != clierr.AuthRefreshFailed ||
			!strings.Contains(ce.Message, "client_credentials refresh") || !strings.Contains(ce.Message, "invalid_client") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("config cannot be saved", func(t *testing.T) {
		f, cfg := factoryFor(t, p, config.AuthConfig{
			GrantType: "client_credentials", ClientID: "svc", ClientSecret: "s",
			AccessToken: "stale", ExpiresAt: time.Now().Add(-time.Hour), Scopes: []string{"x"},
		})
		cfg.Path = t.TempDir() // a directory: writing a file there fails
		_, err := f.Token(context.Background())
		var ce clierr.CLIError
		if !errors.As(err, &ce) || ce.Code != clierr.AuthRefreshFailed || !strings.Contains(ce.Message, "save refreshed config") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestToken_RefreshTokenGrant_PersistsRotatedToken(t *testing.T) {
	p := newFakePlatform(t)
	f, cfg := factoryFor(t, p, config.AuthConfig{
		GrantType: "authorization_code", ClientID: "amctl", AccessToken: "stale",
		RefreshToken: "old-refresh", ExpiresAt: time.Now().Add(-time.Hour),
	})
	tok, err := f.Token(context.Background())
	if err != nil || tok != "fresh" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	form := p.tokenRequests()[0]
	if form.Get("grant_type") != "refresh_token" || form.Get("refresh_token") != "old-refresh" || form.Get("client_id") != "amctl" {
		t.Errorf("form = %v", form)
	}
	if got := cfg.Instances["work"].Auth.RefreshToken; got != "rotated" {
		t.Errorf("refresh token = %q, want the rotated one", got)
	}

	// IdP that does not rotate: the old refresh token is kept.
	p.tokenResp = map[string]any{"access_token": "again", "token_type": "Bearer", "expires_in": 3600}
	inst := cfg.Instances["work"]
	inst.Auth.ExpiresAt = time.Now().Add(-time.Hour)
	cfg.Instances["work"] = inst
	if tok, err := f.Token(context.Background()); err != nil || tok != "again" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
	if got := cfg.Instances["work"].Auth.RefreshToken; got != "rotated" {
		t.Errorf("refresh token dropped when IdP omitted it: %q", got)
	}
}

func TestObserver_DiscoversURLFromServerConfig(t *testing.T) {
	p := newFakePlatform(t)
	p.configBody = `{"observerBaseUrl":"http://host.docker.internal:9098/"}`
	f, _ := factoryFor(t, p, config.AuthConfig{AccessToken: "tok"})

	obs, err := f.Observer(context.Background())
	if err != nil || obs == nil {
		t.Fatalf("obs=%v err=%v", obs, err)
	}
	// Second call reuses the discovery result instead of asking again.
	n := len(p.requests)
	if _, err := f.Observer(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(p.requests) != n {
		t.Errorf("observer URL rediscovered: %d -> %d requests", n, len(p.requests))
	}
	if f.observerURL != "http://localhost:9098/" {
		t.Errorf("observerURL = %q, want docker host rewritten", f.observerURL)
	}
}

func TestObserver_DiscoveryFailures(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantCode string
	}{
		{"server has no trace service", `{}`, clierr.FeatureUnavailable},
		{"unparseable observer field", `{"observerBaseUrl": 5}`, clierr.ServerInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newFakePlatform(t)
			p.configBody = tc.body
			f, _ := factoryFor(t, p, config.AuthConfig{AccessToken: "tok"})
			_, err := f.Observer(context.Background())
			var ce clierr.CLIError
			if !errors.As(err, &ce) || ce.Code != tc.wantCode {
				t.Fatalf("err = %v, want %s", err, tc.wantCode)
			}
		})
	}

	t.Run("config endpoint rejects the token", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
		defer srv.Close()
		io, _, _, _ := iostreams.Test()
		f := NewFactory(&config.Config{CurrentInstance: "w", Instances: map[string]config.Instance{
			"w": {URL: srv.URL, Auth: config.AuthConfig{AccessToken: "t"}}}}, io)
		_, err := f.Observer(context.Background())
		var ce clierr.CLIError
		if !errors.As(err, &ce) || ce.Code != clierr.Unauthorized || ce.Status != 401 {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("server unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close()
		io, _, _, _ := iostreams.Test()
		f := NewFactory(&config.Config{CurrentInstance: "w", Instances: map[string]config.Instance{
			"w": {URL: srv.URL, Auth: config.AuthConfig{AccessToken: "t"}}}}, io)
		_, err := f.Observer(context.Background())
		var ce clierr.CLIError
		if !errors.As(err, &ce) || ce.Code != clierr.Transport {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestRewriteDockerHost(t *testing.T) {
	for in, want := range map[string]string{
		"http://host.docker.internal:9098":      "http://localhost:9098",
		"https://host.docker.internal/obs":      "https://localhost/obs",
		"http://observer.example.com:9098":      "http://observer.example.com:9098",
		"://bad url":                            "://bad url",
		"host.docker.internal:9098":             "host.docker.internal:9098", // no scheme/host: left alone
		"http://host.docker.internal.evil.com/": "http://host.docker.internal.evil.com/",
	} {
		if got := rewriteDockerHost(in); got != want {
			t.Errorf("rewriteDockerHost(%q) = %q, want %q", in, got, want)
		}
	}
}
