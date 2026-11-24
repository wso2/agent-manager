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

package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

// fakeIdP serves discovery documents and a token endpoint, recording the
// form values of every token request.
type fakeIdP struct {
	*httptest.Server
	mu         sync.Mutex
	tokenForms []url.Values
	tokenUser  string
	tokenPass  string
	// tokenHandler overrides the default successful token response.
	tokenHandler func(w http.ResponseWriter, r *http.Request)
}

func newFakeIdP(t *testing.T, scopes []string) *fakeIdP {
	t.Helper()
	idp := &fakeIdP{}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resource":              idp.URL,
			"authorization_servers": []string{idp.URL},
			"scopes_supported":      scopes,
		})
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"authorization_endpoint": idp.URL + "/oauth2/authorize",
			"token_endpoint":         idp.URL + "/oauth2/token",
		})
	})
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		idp.mu.Lock()
		idp.tokenForms = append(idp.tokenForms, r.PostForm)
		idp.tokenUser, idp.tokenPass, _ = r.BasicAuth()
		h := idp.tokenHandler
		idp.mu.Unlock()
		if h != nil {
			h(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-1",
			"refresh_token": "refresh-1",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	})
	idp.Server = httptest.NewServer(mux)
	t.Cleanup(idp.Close)
	return idp
}

func (i *fakeIdP) forms() []url.Values {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]url.Values(nil), i.tokenForms...)
}

func TestLogin_ClientCredentials_Discovery(t *testing.T) {
	idp := newFakeIdP(t, []string{"amp:read", "amp:write"})
	io, _, _, _ := iostreams.Test()

	inst, err := Login(context.Background(), LoginOptions{
		URL: idp.URL, ClientID: "svc", ClientSecret: "s3cret", Resource: "urn:wso2:amp", IO: io,
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if inst.Auth.GrantType != "client_credentials" {
		t.Errorf("grant = %q", inst.Auth.GrantType)
	}
	if inst.Auth.AccessToken != "access-1" || inst.Auth.RefreshToken != "refresh-1" {
		t.Errorf("tokens = %q / %q", inst.Auth.AccessToken, inst.Auth.RefreshToken)
	}
	if inst.TokenURL != idp.URL+"/oauth2/token" {
		t.Errorf("TokenURL = %q", inst.TokenURL)
	}
	if got := strings.Join(inst.Auth.Scopes, " "); got != "amp:read amp:write" {
		t.Errorf("scopes come from discovery, got %q", got)
	}
	if inst.Auth.Resource != "urn:wso2:amp" {
		t.Errorf("resource not persisted: %q", inst.Auth.Resource)
	}
	if time.Until(inst.Auth.ExpiresAt) < 55*time.Minute {
		t.Errorf("ExpiresAt = %v, want ~1h out", inst.Auth.ExpiresAt)
	}

	forms := idp.forms()
	if len(forms) != 1 {
		t.Fatalf("token requests = %d", len(forms))
	}
	f := forms[0]
	if f.Get("grant_type") != "client_credentials" {
		t.Errorf("grant_type = %q", f.Get("grant_type"))
	}
	if f.Get("resource") != "urn:wso2:amp" {
		t.Errorf("RFC 8707 resource indicator not sent: %q", f.Get("resource"))
	}
	if f.Get("scope") != "amp:read amp:write" {
		t.Errorf("scope = %q", f.Get("scope"))
	}
}

func TestLogin_ClientCredentials_OmitsResourceWhenEmpty(t *testing.T) {
	idp := newFakeIdP(t, nil)
	io, _, _, _ := iostreams.Test()

	// AuthServer set: discovery is skipped and /oauth2/token is derived.
	inst, err := Login(context.Background(), LoginOptions{
		URL: "http://unused.invalid", AuthServer: idp.URL + "/", ClientID: "svc", ClientSecret: "x", IO: io,
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if inst.TokenURL != idp.URL+"/oauth2/token" {
		t.Errorf("TokenURL = %q", inst.TokenURL)
	}
	if _, ok := idp.forms()[0]["resource"]; ok {
		t.Error("resource param sent although none configured")
	}
}

func TestLogin_ClientCredentials_Rejected(t *testing.T) {
	idp := newFakeIdP(t, nil)
	idp.tokenHandler = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"bad secret"}`))
	}
	io, _, _, _ := iostreams.Test()

	inst, err := Login(context.Background(), LoginOptions{
		URL: idp.URL, ClientID: "svc", ClientSecret: "wrong", IO: io,
	})
	if err == nil {
		t.Fatalf("expected error, got %+v", inst)
	}
	for _, want := range []string{"client_credentials token exchange", "invalid_client", "bad secret"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	if inst != nil {
		t.Error("no instance should be returned on failure")
	}
}

func TestLogin_DiscoveryFailure(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	io, _, _, _ := iostreams.Test()

	for name, secret := range map[string]string{"client_credentials": "s", "pkce": ""} {
		t.Run(name, func(t *testing.T) {
			_, err := Login(context.Background(), LoginOptions{URL: srv.URL, ClientSecret: secret, IO: io,
				OpenBrowser: func(string) error { t.Fatal("browser must not open"); return nil }})
			if err == nil || !strings.Contains(err.Error(), "protected-resource metadata") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestClientCredentialsResourceParams(t *testing.T) {
	if got := ClientCredentialsResourceParams(""); got != nil {
		t.Errorf("empty resource => nil, got %v", got)
	}
	if got := ClientCredentialsResourceParams("urn:x").Get("resource"); got != "urn:x" {
		t.Errorf("got %q", got)
	}
}

// browserDriver is an OpenBrowser stand-in that plays the user: it records
// the authorization URL and then calls back with the supplied query.
func browserDriver(t *testing.T, authURL *url.URL, callback func(redirect, state string) url.Values) func(string) error {
	t.Helper()
	return func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		*authURL = *u
		q := u.Query()
		go func() {
			cb := q.Get("redirect_uri")
			resp, err := http.Get(cb + "?" + callback(cb, q.Get("state")).Encode())
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
		return nil
	}
}

func TestLogin_PKCE_Success(t *testing.T) {
	idp := newFakeIdP(t, []string{"amp:read"})
	io, _, _, errOut := iostreams.Test()

	var authURL url.URL
	open := browserDriver(t, &authURL, func(_, state string) url.Values {
		return url.Values{"code": {"the-code"}, "state": {state}}
	})

	inst, err := Login(context.Background(), LoginOptions{
		URL: idp.URL, Resource: "urn:wso2:amp", IO: io, OpenBrowser: open,
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if inst.Auth.GrantType != "authorization_code" || inst.Auth.ClientID != "amctl" {
		t.Errorf("grant/client = %q/%q", inst.Auth.GrantType, inst.Auth.ClientID)
	}
	if inst.AuthorizationURL != idp.URL+"/oauth2/authorize" {
		t.Errorf("AuthorizationURL = %q", inst.AuthorizationURL)
	}
	if inst.Auth.AccessToken != "access-1" || inst.Auth.RefreshToken != "refresh-1" {
		t.Errorf("tokens = %q/%q", inst.Auth.AccessToken, inst.Auth.RefreshToken)
	}

	aq := authURL.Query()
	if aq.Get("code_challenge_method") != "S256" || aq.Get("code_challenge") == "" {
		t.Errorf("PKCE challenge missing from auth URL: %v", aq)
	}
	if aq.Get("resource") != "urn:wso2:amp" {
		t.Errorf("resource missing from auth URL: %v", aq)
	}
	if aq.Get("response_type") != "code" || aq.Get("scope") != "amp:read" {
		t.Errorf("auth URL = %v", aq)
	}
	if !strings.HasPrefix(aq.Get("redirect_uri"), "http://127.0.0.1:") {
		t.Errorf("redirect_uri = %q", aq.Get("redirect_uri"))
	}

	forms := idp.forms()
	if len(forms) != 1 {
		t.Fatalf("token requests = %d", len(forms))
	}
	if forms[0].Get("code") != "the-code" || forms[0].Get("grant_type") != "authorization_code" {
		t.Errorf("exchange form = %v", forms[0])
	}
	if forms[0].Get("code_verifier") == "" {
		t.Error("code_verifier not sent on exchange")
	}
	if forms[0].Get("client_id") != "amctl" {
		t.Errorf("public client id must be in params: %v", forms[0])
	}
	if !strings.Contains(errOut.String(), "Browser opened") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestLogin_PKCE_AuthServerOverrideAndCustomClient(t *testing.T) {
	idp := newFakeIdP(t, nil)
	io, _, _, _ := iostreams.Test()
	var authURL url.URL
	open := browserDriver(t, &authURL, func(_, state string) url.Values {
		return url.Values{"code": {"c"}, "state": {state}}
	})

	inst, err := Login(context.Background(), LoginOptions{
		URL: "http://unused.invalid", AuthServer: idp.URL, ClientID: "my-client", IO: io, OpenBrowser: open,
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if inst.Auth.ClientID != "my-client" || authURL.Query().Get("client_id") != "my-client" {
		t.Errorf("client id not honoured: %q / %v", inst.Auth.ClientID, authURL.Query())
	}
	if authURL.Path != "/oauth2/authorize" {
		t.Errorf("auth path = %q", authURL.Path)
	}
	if _, ok := authURL.Query()["resource"]; ok {
		t.Error("resource param present although none configured")
	}
}

func TestLogin_PKCE_CallbackFailures(t *testing.T) {
	cases := []struct {
		name     string
		query    func(state string) url.Values
		wantErr  string
		wantPage string
	}{
		{"state mismatch", func(string) url.Values { return url.Values{"code": {"c"}, "state": {"forged"}} },
			"state mismatch", "State mismatch"},
		{"idp error", func(s string) url.Values {
			return url.Values{"state": {s}, "error": {"access_denied"}, "error_description": {"user said <no>"}}
		}, "access_denied: user said <no>", "user said &lt;no&gt;"},
		{"missing code", func(s string) url.Values { return url.Values{"state": {s}} },
			"no authorization code", "No authorization code"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idp := newFakeIdP(t, nil)
			io, _, _, _ := iostreams.Test()
			pageCh := make(chan string, 1)
			statusCh := make(chan int, 1)
			open := func(raw string) error {
				u, _ := url.Parse(raw)
				go func() {
					cb := u.Query().Get("redirect_uri")
					resp, err := http.Get(cb + "?" + tc.query(u.Query().Get("state")).Encode())
					if err != nil {
						pageCh <- err.Error()
						return
					}
					defer resp.Body.Close()
					buf := new(bytes.Buffer)
					_, _ = buf.ReadFrom(resp.Body)
					statusCh <- resp.StatusCode
					pageCh <- buf.String()
				}()
				return nil
			}

			_, err := Login(context.Background(), LoginOptions{URL: idp.URL, IO: io, OpenBrowser: open})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), "authorization code exchange") {
				t.Errorf("error not wrapped: %v", err)
			}
			if len(idp.forms()) != 0 {
				t.Error("token endpoint must not be called after a failed callback")
			}
			if got := <-statusCh; got != http.StatusBadRequest {
				t.Errorf("callback status = %d", got)
			}
			if page := <-pageCh; !strings.Contains(page, tc.wantPage) || !strings.Contains(page, "Login failed") {
				t.Errorf("error page missing %q", tc.wantPage)
			}
		})
	}
}

func TestLogin_PKCE_TokenExchangeRejected(t *testing.T) {
	idp := newFakeIdP(t, nil)
	idp.tokenHandler = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"code expired"}`))
	}
	io, _, _, _ := iostreams.Test()
	var authURL url.URL
	open := browserDriver(t, &authURL, func(_, s string) url.Values { return url.Values{"code": {"old"}, "state": {s}} })

	_, err := Login(context.Background(), LoginOptions{URL: idp.URL, IO: io, OpenBrowser: open})
	if err == nil || !strings.Contains(err.Error(), "token exchange") || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("err = %v", err)
	}
}

func TestLogin_PKCE_BrowserOpenFailurePrintsURL(t *testing.T) {
	idp := newFakeIdP(t, nil)
	io, _, _, errOut := iostreams.Test()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_, err := Login(ctx, LoginOptions{URL: idp.URL, IO: io,
		OpenBrowser: func(string) error { return http.ErrNotSupported }})
	if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("waiting for a callback that never comes must end with the context error, got %v", err)
	}
	out := errOut.String()
	if !strings.Contains(out, "Could not open browser") || !strings.Contains(out, idp.URL+"/oauth2/authorize?") {
		t.Errorf("fallback URL not printed: %q", out)
	}
}

func TestAuthCodePKCE_ListenerBusy(t *testing.T) {
	cfg := &oauth2.Config{}
	io, _, _, _ := iostreams.Test()
	// Occupy the fixed callback port; skip if something else already has it.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", defaultCallbackPort))
	if err != nil {
		t.Skipf("callback port unavailable: %v", err)
	}
	defer ln.Close()

	_, err = authCodePKCE(context.Background(), cfg, io, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "start callback listener") {
		t.Fatalf("err = %v", err)
	}
}

func TestAuthCodePKCE_PromptsBeforeOpeningBrowser(t *testing.T) {
	idp := newFakeIdP(t, nil)
	io, in, _, errOut := iostreams.Test()
	io.SetTerminal(true, true, true)
	in.WriteString("\n")

	opened := false
	cfg := &oauth2.Config{ClientID: "c", Endpoint: oauth2.Endpoint{
		AuthURL: idp.URL + "/oauth2/authorize", TokenURL: idp.URL + "/oauth2/token"}}
	var authURL url.URL
	drive := browserDriver(t, &authURL, func(_, s string) url.Values { return url.Values{"code": {"c"}, "state": {s}} })
	tok, err := authCodePKCE(context.Background(), cfg, io, func(u string) error { opened = true; return drive(u) })
	if err != nil || tok.AccessToken != "access-1" || !opened {
		t.Fatalf("tok=%v err=%v opened=%v", tok, err, opened)
	}
	if !strings.Contains(errOut.String(), "Press Enter to open the browser") {
		t.Errorf("no prompt: %q", errOut.String())
	}
}

func TestAuthCodePKCE_AbortedPrompt(t *testing.T) {
	io, _, _, _ := iostreams.Test() // empty stdin => EOF on the Enter prompt
	io.SetTerminal(true, true, true)
	cfg := &oauth2.Config{}
	_, err := authCodePKCE(context.Background(), cfg, io, func(string) error {
		t.Fatal("browser must not open after an aborted prompt")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("err = %v", err)
	}
}

func TestRandomState_UniqueAndURLSafe(t *testing.T) {
	a, err := randomState()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := randomState()
	if a == b || len(a) < 40 || strings.ContainsAny(a, "+/=") {
		t.Errorf("state not random/url-safe: %q %q", a, b)
	}
}

func TestLoginPages_EscapeMessages(t *testing.T) {
	ok := loginSuccessPage()
	if !strings.Contains(ok, "Login successful") || !strings.Contains(ok, `class="icon success"`) {
		t.Error("success page content")
	}
	bad := loginErrorPage(`<script>alert(1)</script>`)
	if strings.Contains(bad, "<script>alert") || !strings.Contains(bad, "&lt;script&gt;") {
		t.Error("error message must be HTML-escaped")
	}
	if !strings.Contains(bad, `class="icon error"`) {
		t.Error("error page icon")
	}
}
