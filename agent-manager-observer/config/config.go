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

// Config holds all configuration for agent-manager-observer
type Config struct {
	Server     ServerConfig
	OpenSearch OpenSearchConfig
	LogLevel   string
	Auth       AuthConfig
}

// OpenSearchConfig holds the connection settings for the trace store.
type OpenSearchConfig struct {
	Address  string
	Username string
	Password string
	// TraceIndex is the index pattern the OTel collector writes spans to.
	TraceIndex         string
	InsecureSkipVerify bool
}

// AuthConfig holds JWT authentication configuration
type AuthConfig struct {
	JWKSUrl       string
	Issuer        []string
	Audience      []string
	IsLocalDevEnv bool

	// ServerPublicURL is the externally reachable base URL of this service.
	// Used as the `resource` identifier in RFC 9728 protected resource
	// metadata and to build the `resource_metadata` parameter of the
	// WWW-Authenticate challenge on 401 responses. Empty by default; the
	// well-known route serves 503 and the challenge omits resource_metadata
	// until this is configured.
	ServerPublicURL string

	// AuthorizationServers is the list of OAuth 2.0 authorization server URLs
	// advertised in the RFC 9728 protected resource metadata document.
	// Required for /.well-known/oauth-protected-resource to serve.
	AuthorizationServers []string

	// ScopesSupported is the list of OAuth 2.0 scopes supported by this
	// resource, advertised in the RFC 9728 protected resource metadata document.
	ScopesSupported []string
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port int
}

// Load loads configuration from environment variables with defaults
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: getEnvAsInt("AM_OBSERVER_PORT", 9098),
		},
		OpenSearch: OpenSearchConfig{
			Address:            getEnv("OPENSEARCH_ADDRESS", ""),
			Username:           getEnv("OPENSEARCH_USERNAME", ""),
			Password:           getEnv("OPENSEARCH_PASSWORD", ""),
			TraceIndex:         getEnv("OPENSEARCH_TRACE_INDEX", "otel-traces-*"),
			InsecureSkipVerify: getEnvAsBool("OPENSEARCH_TLS_INSECURE_SKIP_VERIFY", false),
		},
		LogLevel: getEnv("LOG_LEVEL", "INFO"),
		Auth: AuthConfig{
			JWKSUrl:              getEnv("KEY_MANAGER_JWKS_URL", ""),
			Issuer:               getEnvAsList("KEY_MANAGER_ISSUER", "Agent Management Platform Local"),
			Audience:             getEnvAsList("KEY_MANAGER_AUDIENCE", "localhost"),
			IsLocalDevEnv:        getEnvAsBool("IS_LOCAL_DEV_ENV", false),
			ServerPublicURL:      getEnv("SERVER_PUBLIC_URL", ""),
			AuthorizationServers: getEnvAsOptionalList("OAUTH_AUTHORIZATION_SERVERS"),
			ScopesSupported:      getEnvAsOptionalList("OAUTH_SCOPES_SUPPORTED"),
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
	if err := c.OpenSearch.validate(); err != nil {
		return err
	}
	return nil
}

func (o *OpenSearchConfig) validate() error {
	if strings.TrimSpace(o.Address) == "" {
		return fmt.Errorf("OPENSEARCH_ADDRESS is required")
	}
	if strings.TrimSpace(o.TraceIndex) == "" {
		return fmt.Errorf("OPENSEARCH_TRACE_INDEX must not be empty")
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

// getEnvAsOptionalList reads a comma-separated environment variable into a
// []string slice, trimming whitespace around each entry and dropping empty
// entries. Unlike getEnvAsList, it returns nil (rather than a default value)
// when the variable is unset — used for OAuth discovery fields that are
// legitimately optional (the well-known route 503s when unconfigured instead
// of failing config load).
func getEnvAsOptionalList(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
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
