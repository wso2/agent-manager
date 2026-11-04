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
			name: "enabled with Thunder and workflows is valid",
			cfg: Config{
				Thunder:     ThunderConfig{BaseURL: "https://idp.example.com"},
				BuildClient: BuildClientConfig{Enabled: true, Workflows: []string{"amp-docker"}},
			},
			wantErrors: 0,
		},
		{
			name: "enabled requires Thunder",
			cfg: Config{
				BuildClient: BuildClientConfig{Enabled: true, Workflows: []string{"amp-docker"}},
			},
			wantErrors:  1,
			errContains: "requires THUNDER_BASE_URL",
		},
		{
			name: "enabled requires a workflow",
			cfg: Config{
				Thunder:     ThunderConfig{BaseURL: "https://idp.example.com"},
				BuildClient: BuildClientConfig{Enabled: true},
			},
			wantErrors:  1,
			errContains: "AMP_BUILD_CLIENT_WORKFLOWS",
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

func TestLoadEnvs_BuildClientDefaults(t *testing.T) {
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
