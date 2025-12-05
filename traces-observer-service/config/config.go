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
)

// Config holds all configuration for the tracing service
type Config struct {
	Server     ServerConfig
	OpenSearch OpenSearchConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port int
}

// OpenSearchConfig holds OpenSearch connection configuration
type OpenSearchConfig struct {
	Address  string
	Username string
	Password string
	Index    string
}

// Load loads configuration from environment variables with defaults
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: getEnvAsInt("TRACES_OBSERVER_PORT", 9098),
		},
		OpenSearch: OpenSearchConfig{
			Address:  getEnv("OPENSEARCH_ADDRESS", "http://localhost:9200"),
			Username: getEnv("OPENSEARCH_USERNAME", ""),
			Password: getEnv("OPENSEARCH_PASSWORD", ""),
			Index:    getEnv("OPENSEARCH_TRACE_INDEX", "custom-otel-span-index"),
		},
	}

	// Validate
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.OpenSearch.Username == "" || c.OpenSearch.Password == "" {
		return fmt.Errorf("opensearch username and password are required")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if c.OpenSearch.Address == "" {
		return fmt.Errorf("opensearch address is required")
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
