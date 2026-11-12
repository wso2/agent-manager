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
	_ = os.Setenv("OPENSEARCH_ADDRESS", "https://localhost:9200")
	_ = os.Setenv("AM_OBSERVER_PORT", "8080")
	_ = os.Setenv("IS_LOCAL_DEV_ENV", "true")
	defer func() { _ = os.Unsetenv("OPENSEARCH_ADDRESS") }()
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

func TestLoad_MissingOpenSearchAddress(t *testing.T) {
	_ = os.Unsetenv("OPENSEARCH_ADDRESS")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing OpenSearch address, got nil")
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	_ = os.Setenv("OPENSEARCH_ADDRESS", "https://localhost:9200")
	_ = os.Setenv("AM_OBSERVER_PORT", "0")
	_ = os.Setenv("IS_LOCAL_DEV_ENV", "true")
	defer func() { _ = os.Unsetenv("OPENSEARCH_ADDRESS") }()
	defer func() { _ = os.Unsetenv("AM_OBSERVER_PORT") }()
	defer func() { _ = os.Unsetenv("IS_LOCAL_DEV_ENV") }()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid port, got nil")
	}
}

func TestLoad_PortTooHigh(t *testing.T) {
	_ = os.Setenv("OPENSEARCH_ADDRESS", "https://localhost:9200")
	_ = os.Setenv("AM_OBSERVER_PORT", "70000")
	_ = os.Setenv("IS_LOCAL_DEV_ENV", "true")
	defer func() { _ = os.Unsetenv("OPENSEARCH_ADDRESS") }()
	defer func() { _ = os.Unsetenv("AM_OBSERVER_PORT") }()
	defer func() { _ = os.Unsetenv("IS_LOCAL_DEV_ENV") }()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for port > 65535, got nil")
	}
}

func TestLoad_MissingJWKSWhenNotLocalDev(t *testing.T) {
	_ = os.Setenv("OPENSEARCH_ADDRESS", "https://localhost:9200")
	_ = os.Unsetenv("KEY_MANAGER_JWKS_URL")
	_ = os.Setenv("IS_LOCAL_DEV_ENV", "false")
	defer func() { _ = os.Unsetenv("OPENSEARCH_ADDRESS") }()
	defer func() { _ = os.Unsetenv("IS_LOCAL_DEV_ENV") }()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when JWKS URL is missing and IS_LOCAL_DEV_ENV is false, got nil")
	}
}

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("OPENSEARCH_ADDRESS", "http://localhost:8085")
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

func TestLoad_OpenSearchDefaults(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.OpenSearch.TraceIndex != "otel-traces-*" {
		t.Errorf("unexpected default TraceIndex: %q", cfg.OpenSearch.TraceIndex)
	}
	if cfg.OpenSearch.InsecureSkipVerify {
		t.Error("expected TLS verification on by default")
	}
}

func TestLoad_TraceBackendDefaultsToOpenSearch(t *testing.T) {
	setRequiredEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Traces.Backend != TraceBackendOpenSearch {
		t.Errorf("Traces.Backend = %q, want %q", cfg.Traces.Backend, TraceBackendOpenSearch)
	}
}

func TestLoad_TraceBackendOpenSearchNeedsAddress(t *testing.T) {
	t.Setenv("OPENSEARCH_ADDRESS", "")
	t.Setenv("IS_LOCAL_DEV_ENV", "true")
	t.Setenv("TRACE_BACKEND", "opensearch")
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for opensearch without OPENSEARCH_ADDRESS")
	}
}

func TestLoad_TraceBackendOffNeedsNoBackendSettings(t *testing.T) {
	t.Setenv("OPENSEARCH_ADDRESS", "")
	t.Setenv("OPENCHOREO_OBSERVER_URL", "")
	t.Setenv("IS_LOCAL_DEV_ENV", "true")
	t.Setenv("TRACE_BACKEND", "off")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Traces.Backend != TraceBackendOff {
		t.Errorf("Traces.Backend = %q, want %q", cfg.Traces.Backend, TraceBackendOff)
	}
}

func TestLoad_TraceBackendOpenChoreo(t *testing.T) {
	settings := map[string]string{
		"OPENCHOREO_OBSERVER_URL": "http://observer:8080",
		"IDP_TOKEN_URL":           "http://idp/oauth2/token",
		"IDP_CLIENT_ID":           "client",
		"IDP_CLIENT_SECRET":       "secret",
	}
	setAll := func(t *testing.T) {
		t.Setenv("OPENSEARCH_ADDRESS", "")
		t.Setenv("IS_LOCAL_DEV_ENV", "true")
		t.Setenv("TRACE_BACKEND", "openchoreo")
		for k, v := range settings {
			t.Setenv(k, v)
		}
	}

	t.Run("needs no OpenSearch", func(t *testing.T) {
		setAll(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Traces.Backend != TraceBackendOpenChoreo || cfg.OpenChoreo.DefaultNamespace != "default" {
			t.Errorf("Traces = %+v, OpenChoreo = %+v", cfg.Traces, cfg.OpenChoreo)
		}
	})
	for name := range settings {
		t.Run("needs "+name, func(t *testing.T) {
			setAll(t)
			t.Setenv(name, "")
			if _, err := Load(); err == nil {
				t.Fatalf("expected an error without %s", name)
			}
		})
	}
}

func TestLoad_TraceBackendUnknown(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("TRACE_BACKEND", "clickhouse")
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for an unknown TRACE_BACKEND")
	}
}
