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
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	// Set required env vars
	_ = os.Setenv("OPENSEARCH_USERNAME", "admin")
	_ = os.Setenv("OPENSEARCH_PASSWORD", "secret")
	defer func() { _ = os.Unsetenv("OPENSEARCH_USERNAME") }()
	defer func() { _ = os.Unsetenv("OPENSEARCH_PASSWORD") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		t.Errorf("expected valid port (1-65535), got %d", cfg.Server.Port)
	}
	if cfg.OpenSearch.Address != "https://localhost:9200" {
		t.Errorf("expected default address, got %q", cfg.OpenSearch.Address)
	}
	if cfg.OpenSearch.DefaultSpanQueryLimit != 1000 {
		t.Errorf("expected default span query limit 1000, got %d", cfg.OpenSearch.DefaultSpanQueryLimit)
	}
	if cfg.LogLevel != "INFO" {
		t.Errorf("expected default log level INFO, got %q", cfg.LogLevel)
	}
}

func TestLoad_CustomPort(t *testing.T) {
	_ = os.Setenv("OPENSEARCH_USERNAME", "admin")
	_ = os.Setenv("OPENSEARCH_PASSWORD", "secret")
	_ = os.Setenv("TRACES_OBSERVER_PORT", "8080")
	defer func() { _ = os.Unsetenv("OPENSEARCH_USERNAME") }()
	defer func() { _ = os.Unsetenv("OPENSEARCH_PASSWORD") }()
	defer func() { _ = os.Unsetenv("TRACES_OBSERVER_PORT") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoad_MissingCredentials(t *testing.T) {
	// Ensure env vars are unset
	_ = os.Unsetenv("OPENSEARCH_USERNAME")
	_ = os.Unsetenv("OPENSEARCH_PASSWORD")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing credentials, got nil")
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	_ = os.Setenv("OPENSEARCH_USERNAME", "admin")
	_ = os.Setenv("OPENSEARCH_PASSWORD", "secret")
	_ = os.Setenv("TRACES_OBSERVER_PORT", "0")
	defer func() { _ = os.Unsetenv("OPENSEARCH_USERNAME") }()
	defer func() { _ = os.Unsetenv("OPENSEARCH_PASSWORD") }()
	defer func() { _ = os.Unsetenv("TRACES_OBSERVER_PORT") }()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid port, got nil")
	}
}

func TestLoad_PortTooHigh(t *testing.T) {
	_ = os.Setenv("OPENSEARCH_USERNAME", "admin")
	_ = os.Setenv("OPENSEARCH_PASSWORD", "secret")
	_ = os.Setenv("TRACES_OBSERVER_PORT", "70000")
	defer func() { _ = os.Unsetenv("OPENSEARCH_USERNAME") }()
	defer func() { _ = os.Unsetenv("OPENSEARCH_PASSWORD") }()
	defer func() { _ = os.Unsetenv("TRACES_OBSERVER_PORT") }()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for port > 65535, got nil")
	}
}

func TestGetEnv(t *testing.T) {
	_ = os.Setenv("TEST_CONFIG_VAR", "hello")
	defer func() { _ = os.Unsetenv("TEST_CONFIG_VAR") }()

	if got := getEnv("TEST_CONFIG_VAR", "default"); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}

	if got := getEnv("NONEXISTENT_CONFIG_VAR", "default"); got != "default" {
		t.Errorf("expected 'default', got %q", got)
	}
}

func TestGetEnvAsInt(t *testing.T) {
	_ = os.Setenv("TEST_INT_VAR", "42")
	defer func() { _ = os.Unsetenv("TEST_INT_VAR") }()

	if got := getEnvAsInt("TEST_INT_VAR", 10); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}

	// Invalid int string falls back to default
	_ = os.Setenv("TEST_INT_VAR", "not-a-number")
	if got := getEnvAsInt("TEST_INT_VAR", 10); got != 10 {
		t.Errorf("expected default 10, got %d", got)
	}

	// Unset var falls back to default
	if got := getEnvAsInt("NONEXISTENT_INT_VAR", 99); got != 99 {
		t.Errorf("expected default 99, got %d", got)
	}
}
