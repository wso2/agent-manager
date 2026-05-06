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

package framework

import (
	"os"
	"time"
)

// Config holds all configuration for the e2e test suite.
type Config struct {
	AMPBaseURL       string
	TracesBaseURL    string
	IDPTokenURL      string
	IDPClientID      string
	IDPClientSecret  string
	DefaultOrg       string
	DefaultProject   string
	DefaultEnv       string
	ReadinessTimeout time.Duration
	TavilyAPIKey     string
	OpenAIAPIKey     string
}

// LoadConfig reads configuration from environment variables with sensible defaults
// matching the quick-start install.sh deployment.
func LoadConfig() *Config {
	return &Config{
		AMPBaseURL:       envOrDefault("AMP_API_BASE_URL", "http://localhost:9000"),
		TracesBaseURL:    envOrDefault("TRACES_OBSERVER_BASE_URL", "http://localhost:9098"),
		IDPTokenURL:      envOrDefault("IDP_TOKEN_URL", "http://thunder.amp.localhost:8080/oauth2/token"),
		IDPClientID:      envOrDefault("IDP_CLIENT_ID", "amp-api-client"),
		IDPClientSecret:  envOrDefault("IDP_CLIENT_SECRET", "amp-api-client-secret"),
		DefaultOrg:       envOrDefault("DEFAULT_ORG", "default"),
		DefaultProject:   envOrDefault("DEFAULT_PROJECT", "default"),
		DefaultEnv:       envOrDefault("DEFAULT_ENV", "default"),
		ReadinessTimeout: envDurationOrDefault("READINESS_TIMEOUT", 5*time.Minute),
		TavilyAPIKey:     envOrDefault("TAVILY_API_KEY", ""),
		OpenAIAPIKey:     envOrDefault("OPENAI_API_KEY", ""),
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return fallback
}
