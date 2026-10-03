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
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/config"
)

func TestEnsureFreshToken_ZeroExpiryReturnsCachedToken(t *testing.T) {
	f := &Factory{}
	cfg := &config.Config{
		CurrentInstance: "x",
		Instances:       map[string]config.Instance{"x": {}},
	}
	inst := &config.Instance{
		Auth: config.AuthConfig{AccessToken: "cached-token"},
	}

	tok, err := f.ensureFreshToken(context.Background(), cfg, inst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "cached-token" {
		t.Errorf("tok = %q, want cached-token", tok)
	}
}

func expiredSessionConfig(t *testing.T, tokenURL string) (*config.Config, *config.Instance) {
	t.Helper()
	inst := config.Instance{
		URL:      "https://amp.example.com",
		TokenURL: tokenURL,
		Auth: config.AuthConfig{
			GrantType:    "authorization_code",
			ClientID:     "amctl",
			AccessToken:  "stale",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().Add(-time.Hour),
		},
	}
	cfg := &config.Config{
		Path:            filepath.Join(t.TempDir(), "config.yaml"),
		CurrentInstance: "work",
		Instances:       map[string]config.Instance{"work": inst},
	}
	return cfg, &inst
}

func TestEnsureFreshToken_RefreshFailures(t *testing.T) {
	rejecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_grant","error_description":"Invalid refresh token"}`)
	}))
	defer rejecting.Close()
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"server_error","error_description":"boom"}`)
	}))
	defer failing.Close()
	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachable.Close()

	tests := []struct {
		name        string
		tokenURL    string
		wantCode    string
		wantMessage string
		wantCause   string
	}{
		{
			name:        "expired refresh token asks to log in again",
			tokenURL:    rejecting.URL,
			wantCode:    clierr.AuthTokenExpired,
			wantMessage: "Your session for 'work' (https://amp.example.com) has expired. Run 'amctl login' to sign in again.",
			wantCause:   "invalid_grant",
		},
		{
			name:        "authorization server error",
			tokenURL:    failing.URL,
			wantCode:    clierr.AuthRefreshFailed,
			wantMessage: "Could not refresh your session for 'work' (https://amp.example.com). Run 'amctl login' to sign in again.",
			wantCause:   "server_error",
		},
		{
			name:        "authorization server unreachable",
			tokenURL:    unreachable.URL,
			wantCode:    clierr.AuthRefreshFailed,
			wantMessage: "Could not reach the authorization server to refresh your session for 'work' (https://amp.example.com).",
			wantCause:   "connect",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, inst := expiredSessionConfig(t, tt.tokenURL)
			f := &Factory{}

			_, err := f.ensureFreshToken(context.Background(), cfg, inst)

			var cliErr clierr.CLIError
			if !errors.As(err, &cliErr) {
				t.Fatalf("err = %v, want clierr.CLIError", err)
			}
			if cliErr.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", cliErr.Code, tt.wantCode)
			}
			if cliErr.Message != tt.wantMessage {
				t.Errorf("message = %q, want %q", cliErr.Message, tt.wantMessage)
			}
			cause, _ := cliErr.AdditionalData["cause"].(string)
			if !strings.Contains(cause, tt.wantCause) {
				t.Errorf("cause = %q, want it to contain %q", cause, tt.wantCause)
			}
		})
	}
}

func TestEnsureFreshToken_MissingRefreshTokenAsksToLogIn(t *testing.T) {
	cfg, inst := expiredSessionConfig(t, "https://idp.example.com/oauth2/token")
	inst.Auth.RefreshToken = ""
	f := &Factory{}

	_, err := f.ensureFreshToken(context.Background(), cfg, inst)

	var cliErr clierr.CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != clierr.AuthTokenExpired {
		t.Fatalf("err = %v, want %s", err, clierr.AuthTokenExpired)
	}
}
