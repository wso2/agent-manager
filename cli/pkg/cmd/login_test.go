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

package cmd

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/auth"
	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

type loginHarness struct {
	opts   *LoginOptions
	cfg    *config.Config
	got    *auth.LoginOptions
	stderr func() string
}

func newLoginHarness(t *testing.T, instances map[string]config.Instance, current string) *loginHarness {
	t.Helper()
	ios, _, _, errOut := iostreams.Test()
	cfg := &config.Config{
		Path:            filepath.Join(t.TempDir(), "config.yaml"),
		CurrentInstance: current,
		Instances:       instances,
	}
	h := &loginHarness{cfg: cfg, stderr: errOut.String}
	h.opts = &LoginOptions{
		IO:     ios,
		Config: func() (*config.Config, error) { return cfg, nil },
		Authenticate: func(_ context.Context, o auth.LoginOptions) (*config.Instance, error) {
			h.got = &o
			return &config.Instance{URL: o.URL, Auth: config.AuthConfig{ClientID: o.ClientID, ClientSecret: o.ClientSecret}}, nil
		},
		AgentManager: func(context.Context) (*amsvc.ClientWithResponses, error) {
			return nil, errors.New("offline")
		},
	}
	return h
}

func TestLogin_WithoutURLReusesCurrentInstance(t *testing.T) {
	h := newLoginHarness(t, map[string]config.Instance{
		"default": {URL: "https://other.example.com"},
		"work":    {URL: "https://amp.example.com", AuthServer: "https://idp.example.com"},
	}, "work")

	if err := runLogin(context.Background(), h.opts); err != nil {
		t.Fatalf("runLogin: %v\n%s", err, h.stderr())
	}

	if h.got.URL != "https://amp.example.com" {
		t.Errorf("URL = %q, want the current instance's URL", h.got.URL)
	}
	if h.got.AuthServer != "https://idp.example.com" {
		t.Errorf("AuthServer = %q, want the stored auth server", h.got.AuthServer)
	}
	if h.cfg.Instances["work"].AuthServer != "https://idp.example.com" {
		t.Errorf("stored AuthServer lost on re-login")
	}
	if h.cfg.Instances["default"].URL != "https://other.example.com" {
		t.Errorf("default instance was modified")
	}
}

func TestLogin_WithoutURLReusesClientCredentials(t *testing.T) {
	h := newLoginHarness(t, map[string]config.Instance{
		"ci": {URL: "https://amp.example.com", Auth: config.AuthConfig{
			GrantType: "client_credentials", ClientID: "bot", ClientSecret: "s3cret",
		}},
	}, "ci")

	if err := runLogin(context.Background(), h.opts); err != nil {
		t.Fatalf("runLogin: %v\n%s", err, h.stderr())
	}

	if h.got.ClientID != "bot" || h.got.ClientSecret != "s3cret" {
		t.Errorf("client = %q/%q, want stored client credentials", h.got.ClientID, h.got.ClientSecret)
	}
}

func TestLogin_WithoutURLKeepsCustomClientID(t *testing.T) {
	h := newLoginHarness(t, map[string]config.Instance{
		"work": {URL: "https://amp.example.com", Auth: config.AuthConfig{
			GrantType: "authorization_code", ClientID: "custom-cli",
		}},
	}, "work")

	if err := runLogin(context.Background(), h.opts); err != nil {
		t.Fatalf("runLogin: %v\n%s", err, h.stderr())
	}

	if h.got.ClientID != "custom-cli" || h.got.ClientSecret != "" {
		t.Errorf("client = %q/%q, want custom-cli with no secret", h.got.ClientID, h.got.ClientSecret)
	}
}

func TestLogin_WithURLStillDefaultsToDefaultInstance(t *testing.T) {
	h := newLoginHarness(t, map[string]config.Instance{
		"work": {URL: "https://amp.example.com"},
	}, "work")
	h.opts.URL = "https://new.example.com"

	if err := runLogin(context.Background(), h.opts); err != nil {
		t.Fatalf("runLogin: %v\n%s", err, h.stderr())
	}

	if h.cfg.Instances["default"].URL != "https://new.example.com" {
		t.Errorf("default instance URL = %q", h.cfg.Instances["default"].URL)
	}
	if h.cfg.Instances["work"].URL != "https://amp.example.com" {
		t.Errorf("work instance was overwritten")
	}
}

func TestLogin_WithoutURLRequiresKnownInstance(t *testing.T) {
	tests := []struct {
		name      string
		instances map[string]config.Instance
		current   string
		flagName  string
		wantErr   string
	}{
		{
			name:    "first login",
			wantErr: "--url is required for the first login",
		},
		{
			name:      "unknown named instance",
			instances: map[string]config.Instance{"work": {URL: "https://amp.example.com"}},
			current:   "work",
			flagName:  "staging",
			wantErr:   "--url is required to log in to new instance 'staging'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newLoginHarness(t, tt.instances, tt.current)
			h.opts.Name = tt.flagName

			if err := runLogin(context.Background(), h.opts); err == nil {
				t.Fatal("runLogin succeeded, want an error")
			}
			if h.got != nil {
				t.Errorf("Authenticate was called")
			}
			if !strings.Contains(h.stderr(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", h.stderr(), tt.wantErr)
			}
		})
	}
}
