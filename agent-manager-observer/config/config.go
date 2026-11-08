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
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	TraceBackendOpenChoreo = "openchoreo"
	TraceBackendClickHouse = "clickhouse"
	TraceBackendOpenSearch = "opensearch"

	TraceLayoutExporter = "exporter"
	TraceLayoutTenant   = "tenant"
)

// Config holds all configuration for agent-manager-observer
type Config struct {
	Server   ServerConfig
	Observer ObserverConfig
	Traces   TraceReaderConfig
	LogLevel string
	Auth     AuthConfig
}

// ObserverConfig holds configuration for the observer service HTTP client
type ObserverConfig struct {
	BaseURL      string
	TokenURL     string
	ClientID     string
	ClientSecret string
	// DefaultNamespace scopes all trace queries to a single namespace.
	DefaultNamespace string
}

type TraceReaderConfig struct {
	Backend         string
	Addr            string
	Database        string
	User            string
	Password        string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	DialTimeout     time.Duration
	Table           string
	Layout          string
	ScopeUIDs       string

	OSAddr        string
	OSUser        string
	OSPassword    string
	OSIndex       string
	OSInsecureTLS bool

	Batching bool
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
		Observer: ObserverConfig{
			BaseURL:          getEnv("OPENCHOREO_OBSERVER_URL", ""),
			TokenURL:         getEnv("IDP_TOKEN_URL", ""),
			ClientID:         getEnv("IDP_CLIENT_ID", ""),
			ClientSecret:     getEnv("IDP_CLIENT_SECRET", ""),
			DefaultNamespace: getEnv("OBSERVER_DEFAULT_NAMESPACE", "default"),
		},
		Traces: TraceReaderConfig{
			Backend:  getEnv("TRACE_BACKEND", TraceBackendOpenChoreo),
			Addr:     getEnv("CLICKHOUSE_ADDR", ""),
			Database: getEnv("CLICKHOUSE_DATABASE", "otel"),
			User:     getEnv("CLICKHOUSE_USER", "default"),
			Password: getEnv("CLICKHOUSE_PASSWORD", ""),

			MaxOpenConns:    getEnvAsInt("CLICKHOUSE_MAX_OPEN_CONNS", 10),
			MaxIdleConns:    getEnvAsInt("CLICKHOUSE_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvAsDuration("CLICKHOUSE_CONN_MAX_LIFETIME", time.Hour),
			DialTimeout:     getEnvAsDuration("CLICKHOUSE_DIAL_TIMEOUT", 5*time.Second),

			Table:  getEnv("CLICKHOUSE_TRACES_TABLE", "otel_traces"),
			Layout: getEnv("CLICKHOUSE_TRACES_LAYOUT", TraceLayoutExporter),

			OSAddr:        getEnv("OPENSEARCH_ADDR", ""),
			OSUser:        getEnv("OPENSEARCH_USERNAME", ""),
			OSPassword:    getEnv("OPENSEARCH_PASSWORD", ""),
			OSInsecureTLS: getEnvAsBool("OPENSEARCH_INSECURE_TLS", false),
			OSIndex:       getEnv("OPENSEARCH_TRACES_INDEX", "otel-traces-*"),

			ScopeUIDs: getEnv("TRACE_SCOPE_UIDS", getEnv("CLICKHOUSE_SCOPE_UIDS", "")),

			Batching: getEnvAsBool("TRACE_BATCHING", false),
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
	if err := c.Observer.validate(); err != nil {
		return err
	}
	if err := c.Traces.validate(c.Auth.IsLocalDevEnv); err != nil {
		return err
	}
	return nil
}

func (o *ObserverConfig) validate() error {
	if strings.TrimSpace(o.BaseURL) == "" {
		return fmt.Errorf("OPENCHOREO_OBSERVER_URL is required")
	}
	return nil
}

// ValidateClientCredentials checks the IDP settings needed by observer.NewAuthProvider.
func (o ObserverConfig) ValidateClientCredentials() error {
	if strings.TrimSpace(o.TokenURL) == "" {
		return fmt.Errorf("IDP_TOKEN_URL is required")
	}
	if strings.TrimSpace(o.ClientID) == "" {
		return fmt.Errorf("IDP_CLIENT_ID is required")
	}
	if strings.TrimSpace(o.ClientSecret) == "" {
		return fmt.Errorf("IDP_CLIENT_SECRET is required")
	}
	return nil
}

func (t TraceReaderConfig) validate(isLocalDev bool) error {
	switch t.Backend {
	case TraceBackendOpenChoreo:
		return nil
	case TraceBackendClickHouse:
		if strings.TrimSpace(t.Addr) == "" {
			return fmt.Errorf("CLICKHOUSE_ADDR is required when TRACE_BACKEND is %q", TraceBackendClickHouse)
		}
		if strings.TrimSpace(t.User) == "" {
			return fmt.Errorf("CLICKHOUSE_USER is required when TRACE_BACKEND is %q", TraceBackendClickHouse)
		}
		if t.Password == "" && !isLocalDev {
			return fmt.Errorf("CLICKHOUSE_PASSWORD is required when TRACE_BACKEND is %q and IS_LOCAL_DEV_ENV is false", TraceBackendClickHouse)
		}
		if t.MaxOpenConns <= 0 || t.MaxIdleConns < 0 || t.DialTimeout <= 0 || t.ConnMaxLifetime <= 0 {
			return fmt.Errorf("CLICKHOUSE_* pool and timeout settings must be positive")
		}
		if strings.TrimSpace(t.Table) == "" {
			return fmt.Errorf("CLICKHOUSE_TRACES_TABLE is required when TRACE_BACKEND is %q", TraceBackendClickHouse)
		}
		if t.Layout != TraceLayoutExporter && t.Layout != TraceLayoutTenant {
			return fmt.Errorf("CLICKHOUSE_TRACES_LAYOUT must be either 'exporter' or 'tenant' when TRACE_BACKEND is %q", TraceBackendClickHouse)
		}
		if len(t.ScopeUIDs) == 0 {
			return fmt.Errorf("CLICKHOUSE_SCOPE_UIDS is required when TRACE_BACKEND is %q", TraceBackendClickHouse)
		}
		if !json.Valid([]byte(t.ScopeUIDs)) {
			return fmt.Errorf("CLICKHOUSE_SCOPE_UIDS must be valid JSON")
		}
		return nil
	case TraceBackendOpenSearch:
		if strings.TrimSpace(t.OSAddr) == "" {
			return fmt.Errorf("OPENSEARCH_ADDR is required when TRACE_BACKEND is %q", TraceBackendOpenSearch)
		}
		if strings.TrimSpace(t.OSUser) == "" {
			return fmt.Errorf("OPENSEARCH_USERNAME is required when TRACE_BACKEND is %q", TraceBackendOpenSearch)
		}
		if t.OSPassword == "" && !isLocalDev {
			return fmt.Errorf("OPENSEARCH_PASSWORD is required when TRACE_BACKEND is %q and IS_LOCAL_DEV_ENV is false", TraceBackendOpenSearch)
		}
		if strings.TrimSpace(t.OSIndex) == "" {
			return fmt.Errorf("OPENSEARCH_TRACES_INDEX is required when TRACE_BACKEND is %q", TraceBackendOpenSearch)
		}
		if t.OSInsecureTLS && !isLocalDev {
			return fmt.Errorf("OPENSEARCH_INSECURE_TLS is only allowed when IS_LOCAL_DEV_ENV is true")
		}
		if !json.Valid([]byte(t.ScopeUIDs)) {
			return fmt.Errorf("TRACE_SCOPE_UIDS must be valid JSON")
		}
		return nil
	default:
		return fmt.Errorf("unknown TRACE_BACKEND %q (use %q or %q)",
			t.Backend, TraceBackendOpenChoreo, TraceBackendClickHouse)
	}
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

func getEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if durVal, err := time.ParseDuration(value); err == nil {
			return durVal
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
