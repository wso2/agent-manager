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
