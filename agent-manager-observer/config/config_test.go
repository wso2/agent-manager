// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
	"os"
	"reflect"
	"testing"
)

func TestLoad_CustomPort(t *testing.T) {
	_ = os.Setenv("OPENCHOREO_OBSERVER_URL", "http://localhost:8085")
	_ = os.Setenv("IDP_TOKEN_URL", "http://localhost:8090/oauth2/token")
	_ = os.Setenv("IDP_CLIENT_ID", "amp-api-client")
	_ = os.Setenv("IDP_CLIENT_SECRET", "amp-api-client-secret")
	_ = os.Setenv("AM_OBSERVER_PORT", "8080")
	_ = os.Setenv("IS_LOCAL_DEV_ENV", "true")
	defer func() { _ = os.Unsetenv("OPENCHOREO_OBSERVER_URL") }()
	defer func() { _ = os.Unsetenv("IDP_TOKEN_URL") }()
	defer func() { _ = os.Unsetenv("IDP_CLIENT_ID") }()
	defer func() { _ = os.Unsetenv("IDP_CLIENT_SECRET") }()
	defer func() { _ = os.Unsetenv("AM_OBSERVER_PORT") }()
	defer func() { _ = os.Unsetenv("IS_LOCAL_DEV_ENV") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoad_MissingObserverConfig(t *testing.T) {
	_ = os.Unsetenv("OPENCHOREO_OBSERVER_URL")
	_ = os.Unsetenv("IDP_TOKEN_URL")
	_ = os.Unsetenv("IDP_CLIENT_ID")
	_ = os.Unsetenv("IDP_CLIENT_SECRET")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing observer config, got nil")
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	_ = os.Setenv("OPENCHOREO_OBSERVER_URL", "http://localhost:8085")
	_ = os.Setenv("IDP_TOKEN_URL", "http://localhost:8090/oauth2/token")
	_ = os.Setenv("IDP_CLIENT_ID", "amp-api-client")
	_ = os.Setenv("IDP_CLIENT_SECRET", "amp-api-client-secret")
	_ = os.Setenv("AM_OBSERVER_PORT", "0")
	_ = os.Setenv("IS_LOCAL_DEV_ENV", "true")
	defer func() { _ = os.Unsetenv("OPENCHOREO_OBSERVER_URL") }()
	defer func() { _ = os.Unsetenv("IDP_TOKEN_URL") }()
	defer func() { _ = os.Unsetenv("IDP_CLIENT_ID") }()
	defer func() { _ = os.Unsetenv("IDP_CLIENT_SECRET") }()
	defer func() { _ = os.Unsetenv("AM_OBSERVER_PORT") }()
	defer func() { _ = os.Unsetenv("IS_LOCAL_DEV_ENV") }()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid port, got nil")
	}
}

func TestLoad_PortTooHigh(t *testing.T) {
	_ = os.Setenv("OPENCHOREO_OBSERVER_URL", "http://localhost:8085")
	_ = os.Setenv("IDP_TOKEN_URL", "http://localhost:8090/oauth2/token")
	_ = os.Setenv("IDP_CLIENT_ID", "amp-api-client")
	_ = os.Setenv("IDP_CLIENT_SECRET", "amp-api-client-secret")
	_ = os.Setenv("AM_OBSERVER_PORT", "70000")
	_ = os.Setenv("IS_LOCAL_DEV_ENV", "true")
	defer func() { _ = os.Unsetenv("OPENCHOREO_OBSERVER_URL") }()
	defer func() { _ = os.Unsetenv("IDP_TOKEN_URL") }()
	defer func() { _ = os.Unsetenv("IDP_CLIENT_ID") }()
	defer func() { _ = os.Unsetenv("IDP_CLIENT_SECRET") }()
	defer func() { _ = os.Unsetenv("AM_OBSERVER_PORT") }()
	defer func() { _ = os.Unsetenv("IS_LOCAL_DEV_ENV") }()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for port > 65535, got nil")
	}
}

func TestLoad_MissingJWKSWhenNotLocalDev(t *testing.T) {
	_ = os.Setenv("OPENCHOREO_OBSERVER_URL", "http://localhost:8085")
	_ = os.Setenv("IDP_TOKEN_URL", "http://localhost:8090/oauth2/token")
	_ = os.Setenv("IDP_CLIENT_ID", "amp-api-client")
	_ = os.Setenv("IDP_CLIENT_SECRET", "amp-api-client-secret")
	_ = os.Unsetenv("KEY_MANAGER_JWKS_URL")
	_ = os.Setenv("IS_LOCAL_DEV_ENV", "false")
	defer func() { _ = os.Unsetenv("OPENCHOREO_OBSERVER_URL") }()
	defer func() { _ = os.Unsetenv("IDP_TOKEN_URL") }()
	defer func() { _ = os.Unsetenv("IDP_CLIENT_ID") }()
	defer func() { _ = os.Unsetenv("IDP_CLIENT_SECRET") }()
	defer func() { _ = os.Unsetenv("IS_LOCAL_DEV_ENV") }()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when JWKS URL is missing and IS_LOCAL_DEV_ENV is false, got nil")
	}
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("OPENCHOREO_OBSERVER_URL", "http://localhost:8085")
	t.Setenv("IDP_TOKEN_URL", "http://localhost:8090/oauth2/token")
	t.Setenv("IDP_CLIENT_ID", "amp-api-client")
	t.Setenv("IDP_CLIENT_SECRET", "amp-api-client-secret")
	t.Setenv("IS_LOCAL_DEV_ENV", "true")
}

func TestLoad_OAuthDefaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Auth.ServerPublicURL != "" {
		t.Errorf("expected empty ServerPublicURL by default, got %q", cfg.Auth.ServerPublicURL)
	}
	if len(cfg.Auth.AuthorizationServers) != 0 {
		t.Errorf("expected no authorization servers by default, got %v", cfg.Auth.AuthorizationServers)
	}
	if len(cfg.Auth.ScopesSupported) != 0 {
		t.Errorf("expected no scopes by default, got %v", cfg.Auth.ScopesSupported)
	}
}

func TestLoad_OAuthConfigured(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("SERVER_PUBLIC_URL", "http://traces.amp.localhost:8080")
	t.Setenv("OAUTH_AUTHORIZATION_SERVERS", "http://thunder.amp.localhost:8080, http://other.example.com")
	t.Setenv("OAUTH_SCOPES_SUPPORTED", "amp:observability:log-read,amp:observability:trace-read")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Auth.ServerPublicURL != "http://traces.amp.localhost:8080" {
		t.Errorf("unexpected ServerPublicURL: %q", cfg.Auth.ServerPublicURL)
	}
	wantServers := []string{"http://thunder.amp.localhost:8080", "http://other.example.com"}
	if !reflect.DeepEqual(cfg.Auth.AuthorizationServers, wantServers) {
		t.Errorf("expected AuthorizationServers %v, got %v", wantServers, cfg.Auth.AuthorizationServers)
	}
	wantScopes := []string{"amp:observability:log-read", "amp:observability:trace-read"}
	if !reflect.DeepEqual(cfg.Auth.ScopesSupported, wantScopes) {
		t.Errorf("expected ScopesSupported %v, got %v", wantScopes, cfg.Auth.ScopesSupported)
	}
}

func TestLoadRBACEnabled(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("RBAC_ENABLED", "true")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !cfg.Auth.RBACEnabled {
		t.Error("RBACEnabled = false, want true when RBAC_ENABLED=true")
	}
}

func TestLoadRBACEnabledDefaultsFalse(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("RBAC_ENABLED", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Auth.RBACEnabled {
		t.Error("RBACEnabled = true, want default false when RBAC_ENABLED unset")
	}
}
