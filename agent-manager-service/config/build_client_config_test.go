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
	"slices"
	"strings"
	"testing"
)

func TestValidateBuildClientConfig(t *testing.T) {
	validIDP := IDPConfig{
		TokenURL:     "https://idp.example.com/oauth2/token",
		ClientID:     "amp-api-client",
		ClientSecret: "secret",
	}
	enabled := BuildClientConfig{Enabled: true, Workflows: []string{"amp-docker"}}
	tests := []struct {
		name        string
		cfg         Config
		wantErrors  int
		errContains string
	}{
		{
			name:       "disabled needs nothing",
			cfg:        Config{BuildClient: BuildClientConfig{Enabled: false}},
			wantErrors: 0,
		},
		{
			name:       "enabled with an identity provider client and workflows is valid",
			cfg:        Config{IDP: validIDP, BuildClient: enabled},
			wantErrors: 0,
		},
		{
			name:       "token URL with a path prefix is valid",
			cfg:        Config{IDP: IDPConfig{TokenURL: "https://idp.example.com/tenant/oauth2/token", ClientID: "c", ClientSecret: "s"}, BuildClient: enabled},
			wantErrors: 0,
		},
		{
			name:        "enabled requires a token URL ending in /oauth2/token",
			cfg:         Config{IDP: IDPConfig{TokenURL: "https://idp.example.com/token", ClientID: "c", ClientSecret: "s"}, BuildClient: enabled},
			wantErrors:  1,
			errContains: "IDP_TOKEN_URL",
		},
		{
			name:        "enabled requires an absolute token URL",
			cfg:         Config{IDP: IDPConfig{TokenURL: "/oauth2/token", ClientID: "c", ClientSecret: "s"}, BuildClient: enabled},
			wantErrors:  1,
			errContains: "IDP_TOKEN_URL",
		},
		{
			name:        "enabled requires a client ID",
			cfg:         Config{IDP: IDPConfig{TokenURL: validIDP.TokenURL, ClientID: " ", ClientSecret: "s"}, BuildClient: enabled},
			wantErrors:  1,
			errContains: "IDP_CLIENT_ID and IDP_CLIENT_SECRET",
		},
		{
			name:        "enabled requires a client secret",
			cfg:         Config{IDP: IDPConfig{TokenURL: validIDP.TokenURL, ClientID: "c"}, BuildClient: enabled},
			wantErrors:  1,
			errContains: "IDP_CLIENT_ID and IDP_CLIENT_SECRET",
		},
		{
			name:        "enabled requires a workflow",
			cfg:         Config{IDP: validIDP, BuildClient: BuildClientConfig{Enabled: true}},
			wantErrors:  1,
			errContains: "AMP_BUILD_CLIENT_WORKFLOWS",
		},
		{
			name:       "Thunder is no longer required",
			cfg:        Config{IDP: validIDP, Thunder: ThunderConfig{}, BuildClient: enabled},
			wantErrors: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			r := &configReader{}
			validateBuildClientConfig(&cfg, r)

			if len(r.errors) != tc.wantErrors {
				t.Fatalf("expected %d errors, got %d: %v", tc.wantErrors, len(r.errors), r.errors)
			}
			if tc.errContains == "" {
				return
			}
			for _, err := range r.errors {
				if strings.Contains(err.Error(), tc.errContains) {
					return
				}
			}
			t.Errorf("expected an error containing %q, got %v", tc.errContains, r.errors)
		})
	}
}

func TestOrganizationTokenURL(t *testing.T) {
	const org = "6f1e2c3d-0000-4000-8000-000000000001"
	tests := []struct {
		name     string
		tokenURL string
		ouID     string
		want     string
		wantErr  string
	}{
		{
			name:     "inserts the organization before /oauth2/token",
			tokenURL: "https://platform-idp.example.com/oauth2/token",
			ouID:     org,
			want:     "https://platform-idp.example.com/ou/" + org + "/oauth2/token",
		},
		{
			name:     "keeps a path prefix and port",
			tokenURL: "http://thunder.amp.localhost:8080/tenant/oauth2/token",
			ouID:     org,
			want:     "http://thunder.amp.localhost:8080/tenant/ou/" + org + "/oauth2/token",
		},
		{
			name:     "trims surrounding whitespace from the token URL",
			tokenURL: "  https://idp.example.com/oauth2/token  ",
			ouID:     org,
			want:     "https://idp.example.com/ou/" + org + "/oauth2/token",
		},
		{name: "requires an organization", tokenURL: "https://idp.example.com/oauth2/token", ouID: "", wantErr: "invalid organization ID"},
		{name: "refuses a path separator in the organization", tokenURL: "https://idp.example.com/oauth2/token", ouID: "a/b", wantErr: "invalid organization ID"},
		{name: "refuses a parent segment as the organization", tokenURL: "https://idp.example.com/oauth2/token", ouID: "..", wantErr: "invalid organization ID"},
		{name: "refuses characters that need escaping", tokenURL: "https://idp.example.com/oauth2/token", ouID: "a%2fb", wantErr: "invalid organization ID"},
		{name: "refuses a relative URL", tokenURL: "/oauth2/token", ouID: org, wantErr: "absolute http(s) URL"},
		{name: "refuses a non-http scheme", tokenURL: "ftp://idp.example.com/oauth2/token", ouID: org, wantErr: "absolute http(s) URL"},
		{name: "refuses another token path", tokenURL: "https://idp.example.com/token", ouID: org, wantErr: "must end with /oauth2/token"},
		{name: "refuses a query", tokenURL: "https://idp.example.com/oauth2/token?x=1", ouID: org, wantErr: "query or fragment"},
		{name: "refuses an organization's token endpoint", tokenURL: "https://idp.example.com/ou/other/oauth2/token", ouID: org, wantErr: "not an organization's"},
		{name: "refuses a path ending in /ou", tokenURL: "https://idp.example.com/ou/oauth2/token", ouID: org, wantErr: "not an organization's"},
		{
			name:     "a path segment merely starting with ou is fine",
			tokenURL: "https://idp.example.com/outer/oauth2/token",
			ouID:     org,
			want:     "https://idp.example.com/outer/ou/" + org + "/oauth2/token",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := OrganizationTokenURL(tc.tokenURL, tc.ouID)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("OrganizationTokenURL() error = %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("OrganizationTokenURL() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("OrganizationTokenURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoadEnvs_BuildClientDefaults(t *testing.T) {
	// loadEnvs replaces the package-level config; put it back afterwards so later
	// tests in this package do not see the minimal one built here.
	prevConfig, prevWorkload := config, agentWorkloadConfig
	t.Cleanup(func() { config, agentWorkloadConfig = prevConfig, prevWorkload })

	for k, v := range map[string]string{
		"OPEN_CHOREO_BASE_URL": "http://localhost/api/v1",
		"DB_HOST":              "localhost",
		"DB_USER":              "unit",
		"DB_PASSWORD":          "unit",
		"DB_NAME":              "unit",
	} {
		t.Setenv(k, v)
	}
	t.Setenv("AMP_BUILD_CLIENT_ENABLED", "")
	t.Setenv("AMP_BUILD_CLIENT_WORKFLOWS", "")
	loadEnvs()

	if config.BuildClient.Enabled {
		t.Errorf("BuildClient.Enabled = true, want false by default")
	}
	want := []string{"amp-docker", "amp-ballerina-buildpack", "amp-google-cloud-buildpacks"}
	if !slices.Equal(config.BuildClient.Workflows, want) {
		t.Errorf("BuildClient.Workflows = %v, want %v", config.BuildClient.Workflows, want)
	}
}
