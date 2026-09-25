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
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestThunderConfigResolvedSystemResourceIdentifier(t *testing.T) {
	tests := []struct {
		name string
		cfg  ThunderConfig
		want string
	}{
		{
			name: "derives mcp identifier by default",
			cfg:  ThunderConfig{BaseURL: "https://idp.example.com"},
			want: "https://idp.example.com/mcp",
		},
		{
			name: "trims trailing slash before deriving identifier",
			cfg:  ThunderConfig{BaseURL: "https://idp.example.com/"},
			want: "https://idp.example.com/mcp",
		},
		{
			name: "uses explicit identifier",
			cfg: ThunderConfig{
				BaseURL:                  "https://idp.example.com",
				SystemResourceIdentifier: "urn:example:thunder-system",
			},
			want: "urn:example:thunder-system",
		},
		{
			name: "omits resource for Thunder default",
			cfg: ThunderConfig{
				BaseURL:                  "https://idp.example.com",
				UseDefaultResourceServer: true,
			},
			want: "",
		},
		{
			name: "disabled Thunder has no identifier",
			cfg:  ThunderConfig{},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.ResolvedSystemResourceIdentifier(); got != tc.want {
				t.Errorf("ResolvedSystemResourceIdentifier() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateThunderConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfg         ThunderConfig
		wantErrors  int
		errContains string
	}{
		{
			name:       "derived system resource is valid",
			cfg:        ThunderConfig{BaseURL: "https://idp.example.com"},
			wantErrors: 0,
		},
		{
			name: "configured default resource server is valid",
			cfg: ThunderConfig{
				BaseURL:                  "https://idp.example.com",
				UseDefaultResourceServer: true,
			},
			wantErrors: 0,
		},
		{
			name: "absolute URN override is valid",
			cfg: ThunderConfig{
				BaseURL:                  "https://idp.example.com",
				SystemResourceIdentifier: "urn:example:thunder-system",
			},
			wantErrors: 0,
		},
		{
			name: "default and explicit identifier conflict",
			cfg: ThunderConfig{
				BaseURL:                  "https://idp.example.com",
				UseDefaultResourceServer: true,
				SystemResourceIdentifier: "https://idp.example.com/mcp",
			},
			wantErrors:  1,
			errContains: "cannot be true",
		},
		{
			name: "default requires Thunder base URL",
			cfg: ThunderConfig{
				UseDefaultResourceServer: true,
			},
			wantErrors:  1,
			errContains: "require THUNDER_BASE_URL",
		},
		{
			name: "relative identifier is rejected",
			cfg: ThunderConfig{
				BaseURL:                  "https://idp.example.com",
				SystemResourceIdentifier: "/mcp",
			},
			wantErrors:  1,
			errContains: "absolute URI",
		},
		{
			name: "identifier fragment is rejected",
			cfg: ThunderConfig{
				BaseURL:                  "https://idp.example.com",
				SystemResourceIdentifier: "https://idp.example.com/mcp#fragment",
			},
			wantErrors:  1,
			errContains: "must not contain a fragment",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Thunder: tc.cfg}
			r := &configReader{}
			validateThunderConfig(cfg, r)

			if len(r.errors) != tc.wantErrors {
				t.Fatalf("expected %d errors, got %d: %v", tc.wantErrors, len(r.errors), r.errors)
			}
			if tc.errContains == "" {
				return
			}
			for _, err := range r.errors {
				if strings.Contains(err.Error(), tc.errContains) {
					return
				}
			}
			t.Errorf("expected an error containing %q, got %v", tc.errContains, r.errors)
		})
	}
}

func TestLoadEnvs_ThunderResourceServerSelection(t *testing.T) {
	requiredEnv := map[string]string{
		"OPEN_CHOREO_BASE_URL":  "http://localhost/api/v1",
		"DB_HOST":               "localhost",
		"DB_USER":               "unit",
		"DB_PASSWORD":           "unit",
		"DB_NAME":               "unit",
		"THUNDER_BASE_URL":      "https://idp.example.com",
		"THUNDER_CLIENT_ID":     "system-client",
		"THUNDER_CLIENT_SECRET": "system-secret",
	}

	setBaseEnv := func(t *testing.T) {
		t.Helper()
		for key, value := range requiredEnv {
			t.Setenv(key, value)
		}
		t.Setenv("AMP_USE_THUNDER_DEFAULT_RESOURCE_SERVER", "")
		t.Setenv("AMP_THUNDER_SYSTEM_RESOURCE_IDENTIFIER", "")
	}

	t.Run("defaults to derived mcp resource", func(t *testing.T) {
		setBaseEnv(t)
		loadEnvs()

		if got := config.Thunder.ResolvedSystemResourceIdentifier(); got != "https://idp.example.com/mcp" {
			t.Errorf("resolved system resource = %q, want derived /mcp resource", got)
		}
	})

	t.Run("AMP flag selects Thunder default resource server", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("AMP_USE_THUNDER_DEFAULT_RESOURCE_SERVER", "true")
		loadEnvs()

		if got := config.Thunder.ResolvedSystemResourceIdentifier(); got != "" {
			t.Errorf("resolved system resource = %q, want empty", got)
		}
	})

	t.Run("AMP resource identifier overrides derived resource", func(t *testing.T) {
		setBaseEnv(t)
		t.Setenv("AMP_THUNDER_SYSTEM_RESOURCE_IDENTIFIER", "urn:example:thunder-system")
		loadEnvs()

		if got := config.Thunder.ResolvedSystemResourceIdentifier(); got != "urn:example:thunder-system" {
			t.Errorf("resolved system resource = %q, want custom URN", got)
		}
	})
}

func TestValidateOAuthAuthorizationServers(t *testing.T) {
	tests := []struct {
		name        string
		servers     []string
		wantErrors  int
		errContains string
	}{
		{
			name:       "empty list is allowed at config-load time",
			servers:    nil,
			wantErrors: 0,
		},
		{
			name:       "valid https URL",
			servers:    []string{"https://idp.example.com"},
			wantErrors: 0,
		},
		{
			name:       "valid http URL (dev)",
			servers:    []string{"http://thunder.amp.localhost:8080"},
			wantErrors: 0,
		},
		{
			name:       "multiple valid URLs",
			servers:    []string{"https://idp1.example.com", "https://idp2.example.com/path"},
			wantErrors: 0,
		},
		{
			name:        "non-http(s) scheme rejected",
			servers:     []string{"ftp://idp.example.com"},
			wantErrors:  1,
			errContains: "must use http or https",
		},
		{
			name:        "missing host rejected",
			servers:     []string{"https://"},
			wantErrors:  1,
			errContains: "must have a non-empty host",
		},
		{
			name:        "non-URL string rejected",
			servers:     []string{"Agent Management Platform Local"},
			wantErrors:  2, // missing scheme + missing host
			errContains: "OAUTH_AUTHORIZATION_SERVERS",
		},
		{
			name:        "one good one bad accumulates only the bad",
			servers:     []string{"https://idp.example.com", "ftp://nope.example.com"},
			wantErrors:  1,
			errContains: "ftp://nope.example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{OAuthAuthorizationServers: tc.servers}
			r := &configReader{}
			validateOAuthAuthorizationServers(cfg, r)

			if len(r.errors) != tc.wantErrors {
				t.Fatalf("expected %d errors, got %d: %v", tc.wantErrors, len(r.errors), r.errors)
			}
			if tc.errContains != "" {
				found := false
				for _, e := range r.errors {
					if strings.Contains(e.Error(), tc.errContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected an error containing %q, got %v", tc.errContains, r.errors)
				}
			}
		})
	}
}

func TestValidateServerPublicURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantErrors  int
		errContains string
	}{
		{
			name:       "empty is allowed",
			url:        "",
			wantErrors: 0,
		},
		{
			name:       "valid https URL",
			url:        "https://api.example.com",
			wantErrors: 0,
		},
		{
			name:       "valid http URL with port",
			url:        "http://localhost:8080",
			wantErrors: 0,
		},
		{
			name:       "valid https URL with path",
			url:        "https://api.example.com/v1",
			wantErrors: 0,
		},
		{
			name:        "non-http(s) scheme rejected",
			url:         "ftp://api.example.com",
			wantErrors:  1,
			errContains: "must use http or https",
		},
		{
			name:        "missing host rejected",
			url:         "https://",
			wantErrors:  1,
			errContains: "must have a non-empty host",
		},
		{
			name:        "bare string rejected",
			url:         "not-a-url",
			wantErrors:  2,
			errContains: "SERVER_PUBLIC_URL",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{ServerPublicURL: tc.url}
			r := &configReader{}
			validateServerPublicURL(cfg, r)

			if len(r.errors) != tc.wantErrors {
				t.Fatalf("expected %d errors, got %d: %v", tc.wantErrors, len(r.errors), r.errors)
			}
			if tc.errContains != "" {
				found := false
				for _, e := range r.errors {
					if strings.Contains(e.Error(), tc.errContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected an error containing %q, got %v", tc.errContains, r.errors)
				}
			}
		})
	}
}

func TestValidateObserverURLs(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		publicURL   string
		wantErrors  int
		errContains string
	}{
		{
			name:       "valid in-cluster and public URLs",
			url:        "http://amp-observer.openchoreo-observability-plane.svc.cluster.local:9098",
			publicURL:  "https://observer.example.com",
			wantErrors: 0,
		},
		{
			name:       "valid http URL with port",
			url:        "http://localhost:9098",
			publicURL:  "http://localhost:9098",
			wantErrors: 0,
		},
		{
			name:        "malformed in-cluster URL rejected",
			url:         "not-a-url",
			publicURL:   "https://observer.example.com",
			wantErrors:  2, // missing scheme + missing host
			errContains: "AM_OBSERVER_URL",
		},
		{
			name:        "malformed public URL rejected",
			url:         "http://localhost:9098",
			publicURL:   "not-a-url",
			wantErrors:  2, // missing scheme + missing host
			errContains: "AM_OBSERVER_PUBLIC_URL",
		},
		{
			name:        "non-http(s) scheme rejected",
			url:         "ftp://observer.example.com",
			publicURL:   "https://observer.example.com",
			wantErrors:  1,
			errContains: "must use http or https",
		},
		{
			name:        "missing host rejected",
			url:         "http://localhost:9098",
			publicURL:   "https://",
			wantErrors:  1,
			errContains: "must have a non-empty host",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Observer: ObserverConfig{URL: tc.url, PublicURL: tc.publicURL}}
			r := &configReader{}
			validateObserverURLs(cfg, r)

			if len(r.errors) != tc.wantErrors {
				t.Fatalf("expected %d errors, got %d: %v", tc.wantErrors, len(r.errors), r.errors)
			}
			if tc.errContains != "" {
				found := false
				for _, e := range r.errors {
					if strings.Contains(e.Error(), tc.errContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected an error containing %q, got %v", tc.errContains, r.errors)
				}
			}
		})
	}
}

func TestValidatePostgresTLSConfig(t *testing.T) {
	tests := []struct {
		name        string
		sslMode     string
		wantErrors  int
		errContains string
	}{
		{
			name:       "empty is allowed and leaves the driver default in place",
			sslMode:    "",
			wantErrors: 0,
		},
		{
			name:       "whitespace-only is treated as unset",
			sslMode:    "   ",
			wantErrors: 0,
		},
		{
			name:       "require accepted",
			sslMode:    "require",
			wantErrors: 0,
		},
		{
			name:       "disable accepted",
			sslMode:    "disable",
			wantErrors: 0,
		},
		{
			name:       "verify-full accepted",
			sslMode:    "verify-full",
			wantErrors: 0,
		},
		{
			name:        "typo rejected",
			sslMode:     "requrie",
			wantErrors:  1,
			errContains: "DB_SSL_MODE",
		},
		{
			name:        "uppercase rejected because libpq is case sensitive",
			sslMode:     "REQUIRE",
			wantErrors:  1,
			errContains: "is not a valid PostgreSQL sslmode",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{POSTGRESQL: POSTGRESQL{SSLMode: tc.sslMode}}
			r := &configReader{}
			validatePostgresTLSConfig(cfg, r)

			if len(r.errors) != tc.wantErrors {
				t.Fatalf("expected %d errors, got %d: %v", tc.wantErrors, len(r.errors), r.errors)
			}
			if tc.errContains != "" {
				found := false
				for _, e := range r.errors {
					if strings.Contains(e.Error(), tc.errContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected an error containing %q, got %v", tc.errContains, r.errors)
				}
			}
		})
	}
}

func TestValidateSecretManagerConfig(t *testing.T) {
	tests := []struct {
		name        string
		interval    string
		wantErrors  int
		errContains string
	}{
		{
			name:       "documented default accepted",
			interval:   "15s",
			wantErrors: 0,
		},
		{
			name:       "other valid positive duration accepted",
			interval:   "1h",
			wantErrors: 0,
		},
		{
			name:        "malformed duration rejected",
			interval:    "not-a-duration",
			wantErrors:  1,
			errContains: "AGENT_IDENTITY_REFRESH_INTERVAL",
		},
		{
			name:        "zero rejected",
			interval:    "0s",
			wantErrors:  1,
			errContains: "must be a positive duration",
		},
		{
			name:        "negative rejected",
			interval:    "-15s",
			wantErrors:  1,
			errContains: "must be a positive duration",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{SecretManager: SecretManagerConfig{AgentIdentityRefreshInterval: tc.interval}}
			r := &configReader{}
			validateSecretManagerConfig(cfg, r)

			if len(r.errors) != tc.wantErrors {
				t.Fatalf("expected %d errors, got %d: %v", tc.wantErrors, len(r.errors), r.errors)
			}
			if tc.errContains != "" {
				found := false
				for _, e := range r.errors {
					if strings.Contains(e.Error(), tc.errContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected an error containing %q, got %v", tc.errContains, r.errors)
				}
			}
		})
	}
}

func TestValidateGatewayManifestCacheConfig(t *testing.T) {
	tests := []struct {
		name        string
		backend     string
		redisHost   string
		wantErrors  int
		errContains string
	}{
		{
			name:       "default memory backend accepted",
			backend:    "memory",
			wantErrors: 0,
		},
		{
			name:       "redis backend with host accepted",
			backend:    "redis",
			redisHost:  "redis.internal",
			wantErrors: 0,
		},
		{
			name:        "redis backend without host rejected",
			backend:     "redis",
			redisHost:   "",
			wantErrors:  1,
			errContains: "GATEWAY_MANIFEST_CACHE_REDIS_HOST is required",
		},
		{
			name:        "unknown backend rejected",
			backend:     "memcached",
			wantErrors:  1,
			errContains: "GATEWAY_MANIFEST_CACHE_BACKEND",
		},
		{
			name:        "empty backend rejected",
			backend:     "",
			wantErrors:  1,
			errContains: "GATEWAY_MANIFEST_CACHE_BACKEND",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{GatewayManifestCache: GatewayManifestCacheConfig{
				Backend: tc.backend,
				Redis:   GatewayManifestCacheRedisConfig{Host: tc.redisHost},
			}}
			r := &configReader{}
			validateGatewayManifestCacheConfig(cfg, r)

			if len(r.errors) != tc.wantErrors {
				t.Fatalf("expected %d errors, got %d: %v", tc.wantErrors, len(r.errors), r.errors)
			}
			if tc.errContains != "" {
				found := false
				for _, e := range r.errors {
					if strings.Contains(e.Error(), tc.errContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected an error containing %q, got %v", tc.errContains, r.errors)
				}
			}
		})
	}
}

func TestLoadEnvs_ObserverConfig(t *testing.T) {
	requiredEnv := map[string]string{
		"OPEN_CHOREO_BASE_URL": "http://localhost/api/v1",
		"DB_HOST":              "localhost",
		"DB_USER":              "unit",
		"DB_PASSWORD":          "unit",
		"DB_NAME":              "unit",
	}

	setEnv := func(t *testing.T, vars map[string]string) {
		t.Helper()
		for k, v := range vars {
			t.Setenv(k, v)
		}
	}

	t.Run("defaults: URL set, PublicURL empty", func(t *testing.T) {
		setEnv(t, requiredEnv)
		// Pinned rather than left ambient: .env.test sets IS_LOCAL_DEV_ENV=true, which would
		// otherwise supply the localhost PublicURL default this case exists to rule out.
		t.Setenv("IS_LOCAL_DEV_ENV", "false")
		loadEnvs()

		if got := config.Observer.URL; got != "http://localhost:9098" {
			t.Errorf("Observer.URL = %q, want %q", got, "http://localhost:9098")
		}
		if got := config.Observer.PublicURL; got != "" {
			t.Errorf("Observer.PublicURL = %q, want empty", got)
		}
	})

	t.Run("IS_LOCAL_DEV_ENV=true defaults PublicURL to localhost", func(t *testing.T) {
		setEnv(t, requiredEnv)
		t.Setenv("IS_LOCAL_DEV_ENV", "true")
		loadEnvs()

		if got := config.Observer.PublicURL; got != "http://localhost:9098" {
			t.Errorf("Observer.PublicURL = %q, want %q", got, "http://localhost:9098")
		}
	})

	t.Run("explicit AM_OBSERVER_PUBLIC_URL wins over IS_LOCAL_DEV_ENV default", func(t *testing.T) {
		setEnv(t, requiredEnv)
		t.Setenv("IS_LOCAL_DEV_ENV", "true")
		t.Setenv("AM_OBSERVER_PUBLIC_URL", "https://observer.example.com")
		loadEnvs()

		if got := config.Observer.PublicURL; got != "https://observer.example.com" {
			t.Errorf("Observer.PublicURL = %q, want %q", got, "https://observer.example.com")
		}
	})

	t.Run("explicit AM_OBSERVER_URL wins over default", func(t *testing.T) {
		setEnv(t, requiredEnv)
		t.Setenv("AM_OBSERVER_URL", "http://observer.internal:9099")
		loadEnvs()

		if got := config.Observer.URL; got != "http://observer.internal:9099" {
			t.Errorf("Observer.URL = %q, want %q", got, "http://observer.internal:9099")
		}
	})
}

func TestValidateAuditConfig(t *testing.T) {
	valid := AuditConfig{Enabled: true, BufferSize: 4096, BatchSize: 200, FlushIntervalMs: 1000}

	tests := []struct {
		name        string
		mutate      func(*AuditConfig)
		wantErrors  int
		errContains string
	}{
		{
			name:       "defaults are valid",
			mutate:     func(*AuditConfig) {},
			wantErrors: 0,
		},
		{
			name:        "zero buffer size is rejected rather than defaulted",
			mutate:      func(c *AuditConfig) { c.BufferSize = 0 },
			wantErrors:  1,
			errContains: "AUDIT_BUFFER_SIZE",
		},
		{
			name:        "negative flush interval is rejected",
			mutate:      func(c *AuditConfig) { c.FlushIntervalMs = -1 },
			wantErrors:  1,
			errContains: "AUDIT_FLUSH_INTERVAL_MS",
		},
		{
			// A batch that cannot fill would make every flush wait for the
			// timer, silently disabling the batching the values describe.
			name:        "batch larger than the buffer is rejected",
			mutate:      func(c *AuditConfig) { c.BatchSize = 8192 },
			wantErrors:  1,
			errContains: "must not exceed",
		},
		{
			name:       "each malformed value is reported separately",
			mutate:     func(c *AuditConfig) { c.BufferSize = 0; c.BatchSize = 0; c.FlushIntervalMs = 0 },
			wantErrors: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)

			errs := validateAuditConfig(cfg)
			if len(errs) != tt.wantErrors {
				t.Fatalf("got %d errors %v, want %d", len(errs), errs, tt.wantErrors)
			}
			if tt.errContains != "" && !strings.Contains(errs[0].Error(), tt.errContains) {
				t.Errorf("error %q does not mention %q", errs[0], tt.errContains)
			}
		})
	}
}

// TestValidateGrowthAnalyticsConfig covers the deliberate asymmetry of this
// validator: it never contributes a config-load error, because telemetry is
// non-essential and must not stop the service from starting. A bad value
// disables tracking (the collector URL is cleared, which is what makes Track
// no-op) and the process carries on.
func TestValidateGrowthAnalyticsConfig(t *testing.T) {
	const vhost = "development-wso2cloud.gateway-internal.openchoreo-data-plane"

	tests := []struct {
		name        string
		baseURL     string
		hostHeader  string
		wantBaseURL string // "" means tracking ends up disabled
	}{
		{
			name:        "both empty is the normal disabled state",
			wantBaseURL: "",
		},
		{
			name:        "in-cluster URL is kept",
			baseURL:     "http://" + vhost + ":8080/moesif-collector",
			wantBaseURL: "http://" + vhost + ":8080/moesif-collector",
		},
		{
			name:        "local-dev port-forward URL with host header is kept",
			baseURL:     "http://localhost:18080/moesif-collector",
			hostHeader:  vhost,
			wantBaseURL: "http://localhost:18080/moesif-collector",
		},
		{
			name:        "host header without a base URL disables tracking",
			hostHeader:  vhost,
			wantBaseURL: "",
		},
		{
			name:        "non-http(s) scheme disables tracking",
			baseURL:     "ftp://collector.example.com",
			wantBaseURL: "",
		},
		{
			name:        "missing host disables tracking",
			baseURL:     "https://",
			wantBaseURL: "",
		},
		{
			name:        "unparseable URL disables tracking",
			baseURL:     "http://[::1",
			wantBaseURL: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{GrowthAnalytics: GrowthAnalyticsConfig{
				MoesifCollectorBaseURL:    tc.baseURL,
				MoesifCollectorHostHeader: tc.hostHeader,
			}}

			validateGrowthAnalyticsConfig(cfg)

			if got := cfg.GrowthAnalytics.MoesifCollectorBaseURL; got != tc.wantBaseURL {
				t.Errorf("MoesifCollectorBaseURL = %q, want %q", got, tc.wantBaseURL)
			}
		})
	}
}

// TestValidateGrowthAnalyticsConfig_NeverFailsConfigLoad is the guarantee the
// rest of the service depends on: no MOESIF_* value, however malformed, may
// stop agent-manager-service from starting.
func TestValidateGrowthAnalyticsConfig_NeverFailsConfigLoad(t *testing.T) {
	for _, bad := range []struct{ baseURL, hostHeader string }{
		{baseURL: "ftp://collector.example.com"},
		{baseURL: "https://"},
		{baseURL: "http://[::1"},
		{baseURL: "not-a-url"},
		{hostHeader: "some-vhost"},
	} {
		cfg := &Config{GrowthAnalytics: GrowthAnalyticsConfig{
			MoesifCollectorBaseURL:    bad.baseURL,
			MoesifCollectorHostHeader: bad.hostHeader,
		}}
		r := &configReader{}

		validateGrowthAnalyticsConfig(cfg)

		if len(r.errors) != 0 {
			t.Errorf("baseURL=%q hostHeader=%q produced config errors %v, want none — "+
				"telemetry misconfiguration must not stop the service", bad.baseURL, bad.hostHeader, r.errors)
		}
		if cfg.GrowthAnalytics.MoesifCollectorBaseURL != "" {
			t.Errorf("baseURL=%q hostHeader=%q left the collector URL set, want tracking disabled",
				bad.baseURL, bad.hostHeader)
		}
	}
}

// TestLogGrowthAnalyticsState covers the startup line that makes the on/off
// state visible. Tracking being off produces no events and no logs at
// runtime, so this is the only thing that distinguishes "disabled" from
// "broken" without reading the pod's environment.
func TestLogGrowthAnalyticsState(t *testing.T) {
	tests := []struct {
		name          string
		ga            GrowthAnalyticsConfig
		alreadyWarned bool
		wantLevel     slog.Level
		wantContains  string
	}{
		{
			name:         "enabled reports the collector and environment",
			ga:           GrowthAnalyticsConfig{Enabled: true, MoesifCollectorBaseURL: "http://collector:8080/moesif-collector", Environment: "development", DeploymentModel: "saas"},
			wantLevel:    slog.LevelInfo,
			wantContains: "tracking enabled",
		},
		{
			name:         "no collector URL names that reason",
			ga:           GrowthAnalyticsConfig{Enabled: true},
			wantLevel:    slog.LevelInfo,
			wantContains: "MOESIF_COLLECTOR_BASE_URL is unset",
		},
		{
			name:         "kill switch names that reason instead",
			ga:           GrowthAnalyticsConfig{Enabled: false, MoesifCollectorBaseURL: "http://collector:8080/moesif-collector"},
			wantLevel:    slog.LevelInfo,
			wantContains: "MOESIF_ENABLED is false",
		},
		{
			name:          "stays quiet when a WARN already explained why",
			ga:            GrowthAnalyticsConfig{Enabled: true},
			alreadyWarned: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			orig := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(orig) })

			logGrowthAnalyticsState(tc.ga, tc.alreadyWarned)

			got := buf.String()
			if tc.wantContains == "" {
				if got != "" {
					t.Errorf("expected no log output, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.wantContains) {
				t.Errorf("log %q does not contain %q", got, tc.wantContains)
			}
			if !strings.Contains(got, tc.wantLevel.String()) {
				t.Errorf("log %q is not at level %s", got, tc.wantLevel)
			}
		})
	}
}

func TestValidateFileMountLimitsConfig(t *testing.T) {
	tests := []struct {
		name        string
		limits      FileMountLimitsConfig
		wantErrors  int
		errContains string
	}{
		{
			name:       "defaults are valid",
			limits:     FileMountLimitsConfig{MaxFileBytes: 1000000, MaxTotalBytes: 1000000},
			wantErrors: 0,
		},
		{
			name:       "total at the Kubernetes object limit is allowed",
			limits:     FileMountLimitsConfig{MaxFileBytes: 262144, MaxTotalBytes: 1 << 20},
			wantErrors: 0,
		},
		{
			name:        "zero per-file cap rejected",
			limits:      FileMountLimitsConfig{MaxFileBytes: 0, MaxTotalBytes: 1000000},
			wantErrors:  1,
			errContains: "FILE_MOUNT_MAX_FILE_BYTES must be at least 1",
		},
		{
			name:        "negative total rejected",
			limits:      FileMountLimitsConfig{MaxFileBytes: 1, MaxTotalBytes: -1},
			wantErrors:  2, // total < 1, and per-file then exceeds total
			errContains: "FILE_MOUNT_MAX_TOTAL_BYTES must be at least 1",
		},
		{
			name:        "per-file above total rejected",
			limits:      FileMountLimitsConfig{MaxFileBytes: 900000, MaxTotalBytes: 500000},
			wantErrors:  1,
			errContains: "must not exceed FILE_MOUNT_MAX_TOTAL_BYTES",
		},
		{
			name:        "total above the Kubernetes object limit rejected",
			limits:      FileMountLimitsConfig{MaxFileBytes: 1000000, MaxTotalBytes: 2 << 20},
			wantErrors:  1,
			errContains: "Kubernetes ConfigMap/Secret limit",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{FileMountLimits: tc.limits}
			r := &configReader{}
			validateFileMountLimitsConfig(cfg, r)

			if len(r.errors) != tc.wantErrors {
				t.Fatalf("expected %d errors, got %d: %v", tc.wantErrors, len(r.errors), r.errors)
			}
			if tc.errContains != "" {
				found := false
				for _, e := range r.errors {
					if strings.Contains(e.Error(), tc.errContains) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected an error containing %q, got %v", tc.errContains, r.errors)
				}
			}
		})
	}
}
