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
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the secret management service
type Config struct {
	Server     ServerConfig
	KVProvider KVProviderConfig
	Auth       AuthConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host string
	Port int
}

// KVProviderConfig holds KV store provider configuration
type KVProviderConfig struct {
	Type      string // "openbao", "hashicorp-vault", etc.
	Address   string
	Token     string `json:"-"` // Exclude from logs
	Namespace string
	MountPath string
	TLS       TLSConfig
}

// TLSConfig holds TLS configuration for KV provider
type TLSConfig struct {
	CACertPath     string
	ClientCertPath string
	ClientKeyPath  string
	Insecure       bool
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Enabled  bool
	Issuer   []string
	Audience []string
	JWKSUrl  string
}

var cfg *Config

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if it exists
	_ = godotenv.Load()

	cfg = &Config{
		Server: ServerConfig{
			Host: getEnvOrDefault("SERVER_HOST", "0.0.0.0"),
			Port: getEnvAsIntOrDefault("SERVER_PORT", 9099),
		},
		KVProvider: KVProviderConfig{
			Type:      getEnvOrDefault("KV_PROVIDER_TYPE", "openbao"),
			Address:   getEnvOrDefault("KV_PROVIDER_ADDRESS", "http://localhost:8200"),
			Token:     os.Getenv("KV_PROVIDER_TOKEN"),
			Namespace: getEnvOrDefault("KV_PROVIDER_NAMESPACE", ""),
			MountPath: getEnvOrDefault("KV_PROVIDER_MOUNT_PATH", "secret"),
			TLS: TLSConfig{
				CACertPath:     os.Getenv("KV_PROVIDER_TLS_CA_CERT"),
				ClientCertPath: os.Getenv("KV_PROVIDER_TLS_CLIENT_CERT"),
				ClientKeyPath:  os.Getenv("KV_PROVIDER_TLS_CLIENT_KEY"),
				Insecure:       getEnvAsBoolOrDefault("KV_PROVIDER_TLS_INSECURE", false),
			},
		},
		Auth: AuthConfig{
			Enabled:  getEnvAsBoolOrDefault("AUTH_ENABLED", true),
			Issuer:   getEnvAsStringSlice("AUTH_ISSUER"),
			Audience: getEnvAsStringSlice("AUTH_AUDIENCE"),
			JWKSUrl:  os.Getenv("AUTH_JWKS_URL"),
		},
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// GetConfig returns the loaded configuration
func GetConfig() *Config {
	return cfg
}

func validate(cfg *Config) error {
	if cfg.KVProvider.Address == "" {
		return fmt.Errorf("KV_PROVIDER_ADDRESS is required")
	}
	if cfg.KVProvider.Token == "" {
		return fmt.Errorf("KV_PROVIDER_TOKEN is required")
	}
	return nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvAsStringSlice(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}
	// Simple comma-separated parsing
	var result []string
	for _, v := range splitAndTrim(value, ",") {
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}

func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range splitString(s, sep) {
		trimmed := trimSpace(part)
		result = append(result, trimmed)
	}
	return result
}

func splitString(s, sep string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
