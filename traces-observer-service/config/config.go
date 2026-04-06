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
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all configuration for the tracing service
type Config struct {
	Server   ServerConfig
	Observer ObserverConfig
	LogLevel string
	Auth     AuthConfig
}

// ObserverConfig holds configuration for the observer service HTTP client
type ObserverConfig struct {
	BaseURL      string
	TokenURL     string
	ClientID     string
	ClientSecret string
}

// AuthConfig holds JWT authentication configuration
type AuthConfig struct {
	JWKSUrl       string
	Issuer        []string
	Audience      []string
	IsLocalDevEnv bool
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port int
}

// Load loads configuration from environment variables with defaults
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: getEnvAsInt("TRACES_OBSERVER_PORT", 9098),
		},
		Observer: ObserverConfig{
			BaseURL:      getEnv("OBSERVER_BASE_URL", ""),
			TokenURL:     getEnv("IDP_TOKEN_URL", ""),
			ClientID:     getEnv("IDP_CLIENT_ID", ""),
			ClientSecret: getEnv("IDP_CLIENT_SECRET", ""),
		},
		LogLevel: getEnv("LOG_LEVEL", "INFO"),
		Auth: AuthConfig{
			JWKSUrl:       getEnv("KEY_MANAGER_JWKS_URL", ""),
			Issuer:        getEnvAsList("KEY_MANAGER_ISSUER", "Agent Management Platform Local"),
			Audience:      getEnvAsList("KEY_MANAGER_AUDIENCE", "localhost"),
			IsLocalDevEnv: getEnvAsBool("IS_LOCAL_DEV_ENV", false),
		},
	}

	// Validate
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if err := c.Auth.validate(); err != nil {
		return err
	}
	if err := c.Observer.validate(); err != nil {
		return err
	}
	return nil
}

func (o *ObserverConfig) validate() error {
	if o.BaseURL == "" {
		return nil // observer is optional; v2 endpoints return 503 if unconfigured
	}
	if o.TokenURL == "" {
		return fmt.Errorf("IDP_TOKEN_URL is required when OBSERVER_BASE_URL is set")
	}
	if o.ClientID == "" {
		return fmt.Errorf("IDP_CLIENT_ID is required when OBSERVER_BASE_URL is set")
	}
	if o.ClientSecret == "" {
		return fmt.Errorf("IDP_CLIENT_SECRET is required when OBSERVER_BASE_URL is set")
	}
	return nil
}

func (a *AuthConfig) validate() error {
	if a.IsLocalDevEnv {
		return nil
	}
	if strings.TrimSpace(a.JWKSUrl) == "" {
		return fmt.Errorf("KEY_MANAGER_JWKS_URL is required when IS_LOCAL_DEV_ENV is false")
	}
	if len(a.Issuer) == 0 {
		return fmt.Errorf("KEY_MANAGER_ISSUER must contain at least one non-empty issuer when IS_LOCAL_DEV_ENV is false")
	}
	if len(a.Audience) == 0 {
		return fmt.Errorf("KEY_MANAGER_AUDIENCE must contain at least one non-empty audience when IS_LOCAL_DEV_ENV is false")
	}
	return nil
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1"
	}
	return defaultValue
}

// getEnvAsList reads a comma-separated environment variable into a []string slice.
// Falls back to a single-element slice containing defaultValue when the variable is unset.
func getEnvAsList(key, defaultValue string) []string {
	value := os.Getenv(key)
	if value == "" {
		return []string{defaultValue}
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
