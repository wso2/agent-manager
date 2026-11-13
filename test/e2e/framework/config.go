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
	"strings"
	"time"
)

// Config holds all configuration for the e2e test suite.
type Config struct {
	AMPBaseURL                string
	ObserverBaseURL           string
	IDPTokenURL               string
	IDPClientID               string
	IDPClientSecret           string
	ThunderAdminURL           string
	ThunderSystemResource     string
	ThunderSystemClientID     string
	ThunderSystemClientSecret string
	AgentIDTokenURL           string
	SecurityProbeRepository   string
	SecurityProbeBranch       string
	SecurityProbeAppPath      string
	DefaultOrg                string
	DefaultProject            string
	DefaultEnv                string
	ReadinessTimeout          time.Duration
	TavilyAPIKey              string
	OpenAIAPIKey              string
	// A2ACardTestUpstreamHosts are the hosts the platform's A2A card URL guards
	// let through. Non-empty enables the external A2A specs.
	A2ACardTestUpstreamHosts []string
}

// LoadConfig reads configuration from environment variables with sensible defaults
// matching the quick-start install.sh deployment.
func LoadConfig() *Config {
	defaultOrg := envOrDefault("DEFAULT_ORG", "default")
	defaultEnv := envOrDefault("DEFAULT_ENV", "default")
	thunderAdminURL := envOrDefault("THUNDER_ADMIN_URL", "http://thunder.amp.localhost:8080")

	return &Config{
		AMPBaseURL:                envOrDefault("AMP_API_BASE_URL", "http://api.amp.localhost:8080"),
		ObserverBaseURL:           envOrDefault("AM_OBSERVER_BASE_URL", "http://traces.amp.localhost:11080"),
		IDPTokenURL:               envOrDefault("IDP_TOKEN_URL", "http://thunder.amp.localhost:8080/oauth2/token"),
		IDPClientID:               envOrDefault("IDP_CLIENT_ID", "amp-api-client"),
		IDPClientSecret:           envOrDefault("IDP_CLIENT_SECRET", "amp-api-client-secret"),
		ThunderAdminURL:           thunderAdminURL,
		ThunderSystemResource:     envOrDefault("THUNDER_SYSTEM_RESOURCE", strings.TrimRight(thunderAdminURL, "/")+"/mcp"),
		ThunderSystemClientID:     envOrDefault("THUNDER_SYSTEM_CLIENT_ID", "amp-system-client"),
		ThunderSystemClientSecret: envOrDefault("THUNDER_SYSTEM_CLIENT_SECRET", "amp-system-client-secret"),
		// Environment Thunder hosts use registered opaque handles, not a derivable
		// org/environment hostname. The agentid suite discovers the URL through
		// Agent Manager unless a shared/cloud target supplies an explicit override.
		AgentIDTokenURL:          envOrDefault("AGENT_IDP_TOKEN_URL", ""),
		SecurityProbeRepository:  envOrDefault("SECURITY_PROBE_REPOSITORY_URL", "https://github.com/wso2/agent-manager"),
		SecurityProbeBranch:      envOrDefault("SECURITY_PROBE_REPOSITORY_BRANCH", "main"),
		SecurityProbeAppPath:     envOrDefault("SECURITY_PROBE_APP_PATH", "/test/e2e/fixtures/security-probe-agent"),
		DefaultOrg:               defaultOrg,
		DefaultProject:           envOrDefault("DEFAULT_PROJECT", "default"),
		DefaultEnv:               defaultEnv,
		ReadinessTimeout:         envDurationOrDefault("READINESS_TIMEOUT", 5*time.Minute),
		TavilyAPIKey:             envOrDefault("TAVILY_API_KEY", ""),
		OpenAIAPIKey:             envOrDefault("OPENAI_API_KEY", ""),
		A2ACardTestUpstreamHosts: splitHostList(os.Getenv("A2A_CARD_TEST_UPSTREAM_HOSTS")),
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

func splitHostList(raw string) []string {
	var hosts []string
	for _, h := range strings.Split(raw, ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}
