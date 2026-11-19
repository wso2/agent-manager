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
	"strings"
	"testing"
)

func TestValidateShadowRunnerConfig(t *testing.T) {
	tests := []struct {
		name        string
		runner      ShadowRunnerConfig
		audience    []string
		errContains string
	}{
		{name: "an image and no resource is valid", runner: ShadowRunnerConfig{Image: "img"}},
		{
			name:     "a resource the audience admits is valid",
			runner:   ShadowRunnerConfig{Image: "img", Resource: "urn:wso2:cloud"},
			audience: []string{"urn:wso2:cloud"},
		},
		{
			name:        "a relative resource is refused",
			runner:      ShadowRunnerConfig{Image: "img", Resource: "cloud"},
			audience:    []string{"cloud"},
			errContains: "absolute URI",
		},
		{
			name:        "a resource the audience does not admit is refused",
			runner:      ShadowRunnerConfig{Image: "img", Resource: "urn:wso2:cloud"},
			audience:    []string{"amp-api-client"},
			errContains: "KEY_MANAGER_AUDIENCE",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{ShadowRunner: tc.runner, KeyManagerConfigurations: KeyManagerConfigurations{Audience: tc.audience}}
			r := &configReader{}
			validateShadowRunnerConfig(&cfg, r)
			if tc.errContains == "" {
				if len(r.errors) != 0 {
					t.Fatalf("expected no errors, got %v", r.errors)
				}
				return
			}
			if len(r.errors) != 1 || !strings.Contains(r.errors[0].Error(), tc.errContains) {
				t.Fatalf("expected one error containing %q, got %v", tc.errContains, r.errors)
			}
		})
	}
}

func TestShadowRunnerImage(t *testing.T) {
	for in, want := range map[string]string{
		"":                       defaultShadowRunnerImage,
		"   ":                    defaultShadowRunnerImage,
		" registry.example/r:1 ": "registry.example/r:1",
	} {
		if got := shadowRunnerImage(in); got != want {
			t.Errorf("shadowRunnerImage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadEnvs_ShadowRunnerEnabled(t *testing.T) {
	// loadEnvs replaces the package-level config; put it back afterwards.
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
	t.Setenv("AMP_SHADOW_RUNNER_ENABLED", "")
	loadEnvs()
	if config.ShadowRunner.Enabled {
		t.Errorf("ShadowRunner.Enabled = true, want false by default")
	}

	t.Setenv("AMP_SHADOW_RUNNER_ENABLED", "true")
	loadEnvs()
	if !config.ShadowRunner.Enabled {
		t.Errorf("ShadowRunner.Enabled = false, want true with AMP_SHADOW_RUNNER_ENABLED=true")
	}
}
