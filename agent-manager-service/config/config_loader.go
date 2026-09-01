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
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"k8s.io/apimachinery/pkg/api/resource"
)

var (
	config              *Config
	agentWorkloadConfig *AgentWorkload
)

func GetConfig() *Config {
	return config
}

func GetAgentWorkloadConfig() *AgentWorkload {
	return agentWorkloadConfig
}

func init() {
	loadEnvs()
}

func loadEnvs() {
	config = &Config{}
	agentWorkloadConfig = &AgentWorkload{}

	envFilePath := os.Getenv("ENV_FILE_PATH")
	if envFilePath != "" {
		err := godotenv.Load(envFilePath)
		if err != nil {
			panic(err)
		}
	}

	r := &configReader{}
	config.ServerHost = r.readOptionalString("SERVER_HOST", "")
	config.ServerPort = int(r.readOptionalInt64("SERVER_PORT", 8080))
	config.AuthHeader = r.readOptionalString("AUTH_HEADER", "Authorization")
	config.AutoMaxProcsEnabled = r.readOptionalBool("AUTO_MAX_PROCS_ENABLED", true)
	config.CORSAllowedOrigin = r.readOptionalString("CORS_ALLOWED_ORIGIN", "http://localhost:3000")

	agentWorkloadConfig.CORS = CORSConfig{
		AllowOrigin:      r.readOptionalString("AGENT_WORKLOAD_CORS_ALLOWED_ORIGIN", "*"),
		AllowMethods:     r.readOptionalString("AGENT_WORKLOAD_CORS_ALLOWED_METHODS", "GET,POST,PUT,DELETE,PATCH,OPTIONS"),
		AllowHeaders:     r.readOptionalString("AGENT_WORKLOAD_CORS_ALLOWED_HEADERS", "authorization,Content-Type,Origin,X-API-Key"),
		AllowCredentials: r.readOptionalBool("AGENT_WORKLOAD_CORS_ALLOW_CREDENTIALS", false),
	}

	// Logging configuration
	config.LogLevel = r.readOptionalString("LOG_LEVEL", "INFO")

	// read database configs
	config.POSTGRESQL = POSTGRESQL{
		Host:     r.readRequiredString("DB_HOST"),
		Port:     int(r.readOptionalInt64("DB_PORT", 5432)),
		User:     r.readRequiredString("DB_USER"),
		Password: r.readRequiredString("DB_PASSWORD"),
		DBName:   r.readRequiredString("DB_NAME"),
		// Left empty by default so the connection string stays byte-identical to
		// pre-DB_SSL_MODE builds: pgx applies its libpq-compatible "prefer"
		// default and still honours PGSSLMODE/PGSSLROOTCERT from the environment.
		SSLMode:     strings.TrimSpace(r.readOptionalString("DB_SSL_MODE", "")),
		SSLRootCert: strings.TrimSpace(r.readOptionalString("DB_SSL_ROOT_CERT", "")),
	}
	config.POSTGRESQL.DbConfigs = DbConfigs{
		// gorm configs
		SkipDefaultTransaction:    r.readOptionalBool("GORM_SKIP_DEFAULT_TRANSACTION", true),
		SlowThresholdMilliseconds: r.readOptionalInt64("GORM_SLOW_THRESHOLD_MILLISECONDS", 200),

		// sql.DB configs
		MaxIdleCount:       r.readNullableInt64("DB_MAX_IDLE_COUNT"),
		MaxOpenCount:       r.readNullableInt64("DB_MAX_OPEN_COUNT"),
		MaxIdleTimeSeconds: r.readNullableInt64("DB_MAX_IDLE_TIME_SECONDS"),
		MaxLifetimeSeconds: r.readNullableInt64("DB_MAX_LIFETIME_SECONDS"),
	}
	// HTTP Server timeout configurations
	config.ReadTimeoutSeconds = int(r.readOptionalInt64("HTTP_READ_TIMEOUT_SECONDS", 10))
	config.WriteTimeoutSeconds = int(r.readOptionalInt64("HTTP_WRITE_TIMEOUT_SECONDS", 90))
	config.IdleTimeoutSeconds = int(r.readOptionalInt64("HTTP_IDLE_TIMEOUT_SECONDS", 60))
	config.MaxHeaderBytes = int(r.readOptionalInt64("HTTP_MAX_HEADER_BYTES", 65536)) // 1024 * 64

	// Database operation timeout configuration
	config.DbOperationTimeoutSeconds = int(r.readOptionalInt64("DB_OPERATION_TIMEOUT_SECONDS", 10))
	config.HealthCheckTimeoutSeconds = int(r.readOptionalInt64("HEALTH_CHECK_TIMEOUT_SECONDS", 5))

	config.DefaultChatAPI = DefaultChatAPIConfig{
		DefaultHTTPPort: int32(r.readOptionalInt64("DEFAULT_CHAT_API_HTTP_PORT", 8000)),
		DefaultBasePath: r.readOptionalString("DEFAULT_CHAT_API_BASE_PATH", "/"),
	}

	// OpenTelemetry configuration
	// Use Version from ldflags or environment variable override
	config.PackageVersion = r.readOptionalString("AMP_VERSION", Version)

	config.OTEL = OTELConfig{
		SDKVolumeName: r.readOptionalString("OTEL_SDK_VOLUME_NAME", "otel-tracing-sdk-volume"),
		SDKMountPath:  r.readOptionalString("OTEL_SDK_MOUNT_PATH", "/otel-tracing-sdk"),

		DefaultInstrumentationVersion: r.readOptionalString("OTEL_DEFAULT_INSTRUMENTATION_VERSION", "0.4.1"),

		InstrumentationExtensionPath: r.readOptionalString("INSTRUMENTATION_EXTENSION_PATH", "/etc/amp/instrumentation-extension.yaml"),

		// Tracing configuration
		IsTraceContentEnabled: r.readOptionalBool("OTEL_TRACELOOP_TRACE_CONTENT", true),

		// OTLP Exporter configuration. Routed through kgateway by hostname
		// ("<env>-<org>.gateway.localhost") like agent trace exports, so the
		// endpoint stays independent of the gateway runtime's namespace.
		ExporterEndpoint: r.readOptionalString("OTEL_EXPORTER_OTLP_ENDPOINT", "http://default-default.gateway.localhost:19080/otel"),
	}

	config.IsLocalDevEnv = r.readOptionalBool("IS_LOCAL_DEV_ENV", false)

	// Observer service configuration. URL is used server-side (in-cluster) for
	// monitor-run log fetches. PublicURL is handed to out-of-cluster clients
	// (console, CLI) via GET /api/v1/config. It has NO internal-URL fallback:
	// empty means "observer not configured" and clients surface that loudly.
	publicURLDefault := ""
	if config.IsLocalDevEnv {
		publicURLDefault = "http://localhost:9098"
	}
	config.Observer = ObserverConfig{
		URL:       r.readOptionalString("AM_OBSERVER_URL", "http://localhost:9098"),
		PublicURL: r.readOptionalString("AM_OBSERVER_PUBLIC_URL", publicURLDefault),
	}

	config.InstrumentationURL = r.readOptionalString("INSTRUMENTATION_URL", "http://default-default.gateway.localhost:19080/otel")
	config.DefaultGatewayPort = int(r.readOptionalInt64("DEFAULT_GATEWAY_PORT", 19080))
	config.KeyManagerConfigurations = KeyManagerConfigurations{
		// Comma-separated list of allowed issuers and audiences
		Issuer:   r.readOptionalStringList("KEY_MANAGER_ISSUER", "Agent Management Platform Local"),
		Audience: r.readOptionalStringList("KEY_MANAGER_AUDIENCE", "localhost"),
		JWKSUrl:  r.readOptionalString("KEY_MANAGER_JWKS_URL", ""),
	}
	config.IsOnPremDeployment = r.readOptionalBool("IS_ON_PREM_DEPLOYMENT", true)
	config.ServerPublicURL = r.readOptionalString("SERVER_PUBLIC_URL", "")
	config.ThunderHostBaseDomain = r.readOptionalString("IDP_HOST_BASE_DOMAIN", "amp.localhost")
	config.ThunderAskSecret = r.readOptionalString("THUNDER_ASK_SECRET", "")
	config.OAuthAuthorizationServers = r.readOptionalStringList("OAUTH_AUTHORIZATION_SERVERS", "")
	config.OAuthScopesSupported = r.readOptionalStringList("OAUTH_SCOPES_SUPPORTED", "")

	// IDP OAuth2 client credentials for service-to-service auth
	config.IDP = IDPConfig{
		TokenURL:     r.readOptionalString("IDP_TOKEN_URL", "http://thunder.amp.localhost:8080/oauth2/token"),
		ClientID:     r.readOptionalString("IDP_CLIENT_ID", "amp-api-client"),
		ClientSecret: r.readOptionalString("IDP_CLIENT_SECRET", "amp-api-client-secret"),
	}

	// JWT Signing configuration for agent API tokens
	config.JWTSigning = JWTSigningConfig{
		PrivateKeyPath:        r.readOptionalString("JWT_SIGNING_PRIVATE_KEY_PATH", "keys/private.pem"),
		PublicKeysConfigPath:  r.readOptionalString("JWT_SIGNING_PUBLIC_KEYS_CONFIG", "keys/public-keys-config.json"),
		ActiveKeyID:           r.readOptionalString("JWT_SIGNING_ACTIVE_KEY_ID", "key-1"),
		DefaultExpiryDuration: r.readOptionalString("JWT_SIGNING_DEFAULT_EXPIRY", "2160h"), // 90 days default
		Issuer:                r.readOptionalString("JWT_SIGNING_ISSUER", "agent-manager-service"),
		DefaultEnvironment:    r.readOptionalString("JWT_SIGNING_DEFAULT_ENVIRONMENT", "default"),
	}

	// GitHub configuration for repository API access
	config.GitHub = GitHubConfig{
		Token: r.readOptionalString("GITHUB_TOKEN", ""),
	}

	// Gateway manifest cache backend. "memory" (default) is process-local and only
	// safe for a single replica; HA deployments must set this to "redis" so every
	// replica reads and writes the same cached manifest.
	config.GatewayManifestCache = GatewayManifestCacheConfig{
		Backend: r.readOptionalString("GATEWAY_MANIFEST_CACHE_BACKEND", "memory"),
		Redis: GatewayManifestCacheRedisConfig{
			Host:       r.readOptionalString("GATEWAY_MANIFEST_CACHE_REDIS_HOST", ""),
			Port:       int(r.readOptionalInt64("GATEWAY_MANIFEST_CACHE_REDIS_PORT", 6379)),
			Password:   r.readOptionalString("GATEWAY_MANIFEST_CACHE_REDIS_PASSWORD", ""),
			DB:         int(r.readOptionalInt64("GATEWAY_MANIFEST_CACHE_REDIS_DB", 0)),
			TLSEnabled: r.readOptionalBool("GATEWAY_MANIFEST_CACHE_REDIS_TLS_ENABLED", false),
		},
	}
	config.OpenChoreo = OpenChoreoConfig{
		BaseURL:          r.readRequiredString("OPEN_CHOREO_BASE_URL"),
		DefaultNamespace: r.readOptionalString("OPEN_CHOREO_DEFAULT_NAMESPACE", "default"),
		SystemLabelKeyPrefixes: r.readOptionalStringList(
			"OPEN_CHOREO_SYSTEM_LABEL_KEY_PREFIXES", "openchoreo.dev/",
		),
	}

	// Internal Server configuration (for WebSocket and gateway internal APIs)
	config.InternalServer = InternalServerConfig{
		Host:                r.readOptionalString("INTERNAL_SERVER_HOST", ""),
		Port:                int(r.readOptionalInt64("INTERNAL_SERVER_PORT", 9243)),
		TLSEnabled:          r.readOptionalBool("INTERNAL_SERVER_TLS_ENABLED", true),
		CertDir:             r.readOptionalString("INTERNAL_SERVER_CERT_DIR", "./data/certs"),
		ReadTimeoutSeconds:  int(r.readOptionalInt64("INTERNAL_SERVER_READ_TIMEOUT_SECONDS", 10)),
		WriteTimeoutSeconds: int(r.readOptionalInt64("INTERNAL_SERVER_WRITE_TIMEOUT_SECONDS", 90)),
		IdleTimeoutSeconds:  int(r.readOptionalInt64("INTERNAL_SERVER_IDLE_TIMEOUT_SECONDS", 60)),
		MaxHeaderBytes:      int(r.readOptionalInt64("INTERNAL_SERVER_MAX_HEADER_BYTES", 65536)),
	}

	// WebSocket configuration
	config.WebSocket = WebSocketConfig{
		MaxConnections:    int(r.readOptionalInt64("WEBSOCKET_MAX_CONNECTIONS", 1000)),
		ConnectionTimeout: int(r.readOptionalInt64("WEBSOCKET_CONNECTION_TIMEOUT", 30)),
		RateLimitPerMin:   int(r.readOptionalInt64("WEBSOCKET_RATE_LIMIT_PER_MIN", 10)),
	}

	config.SecretManager = SecretManagerConfig{
		Provider:                     r.readOptionalString("SECRET_MANAGER_PROVIDER", "openchoreo"),
		TargetPlaneKind:              r.readOptionalString("SECRET_MANAGER_TARGET_PLANE_KIND", "ClusterDataPlane"),
		TargetPlaneName:              r.readOptionalString("SECRET_MANAGER_TARGET_PLANE_NAME", "default"),
		RefreshInterval:              r.readOptionalString("SECRET_MANAGER_REFRESH_INTERVAL", "1h"),
		AgentIdentityRefreshInterval: r.readOptionalString("AGENT_IDENTITY_REFRESH_INTERVAL", "15s"),
	}

	// OpenBao KV store configuration (data plane - for deployment secrets)
	config.OpenBao = OpenBaoConfig{
		URL:   r.readOptionalString("OPENBAO_URL", "http://localhost:8200"),
		Token: r.readOptionalString("OPENBAO_TOKEN", ""),
		Path:  r.readOptionalString("OPENBAO_PATH", "secret"),
	}

	// Workflow plane OpenBao KV store configuration (for git secrets)
	config.WorkflowPlaneOpenBao = OpenBaoConfig{
		URL:   r.readOptionalString("WORKFLOW_PLANE_OPENBAO_URL", "http://localhost:8200"),
		Token: r.readOptionalString("WORKFLOW_PLANE_OPENBAO_TOKEN", ""),
	}

	// Thunder admin API configuration for provisioning per-org OAuth apps
	config.Thunder = ThunderConfig{
		BaseURL:       r.readOptionalString("THUNDER_BASE_URL", ""),
		ClientID:      r.readOptionalString("THUNDER_CLIENT_ID", ""),
		ClientSecret:  r.readOptionalString("THUNDER_CLIENT_SECRET", ""),
		ResolveToHost: r.readOptionalString("THUNDER_RESOLVE_TO_HOST", ""),
	}
	if config.Thunder.BaseURL != "" && (config.Thunder.ClientID == "" || config.Thunder.ClientSecret == "") {
		r.errors = append(r.errors, fmt.Errorf("THUNDER_BASE_URL is set but THUNDER_CLIENT_ID and/or THUNDER_CLIENT_SECRET are missing"))
	}
	if config.Thunder.ResolveToHost != "" {
		if config.Thunder.BaseURL == "" {
			r.errors = append(r.errors, fmt.Errorf("THUNDER_RESOLVE_TO_HOST requires THUNDER_BASE_URL"))
		}
		if host, port, err := net.SplitHostPort(config.Thunder.ResolveToHost); err != nil {
			r.errors = append(r.errors, fmt.Errorf("THUNDER_RESOLVE_TO_HOST must be host:port: %w", err))
		} else if host == "" || port == "" {
			r.errors = append(r.errors, fmt.Errorf("THUNDER_RESOLVE_TO_HOST must contain a non-empty host and port"))
		}
	}

	config.TLSConfig = TLSConfig{
		EnableTLS: r.readOptionalBool("TLS_ENABLED", false),
	}

	config.RBACEnabled = r.readOptionalBool("RBAC_ENABLED", false)
	config.RootOUHandle = r.readOptionalString("ROOT_OU_HANDLE", "admin")

	// Audit defaults to on. A deployment that wants no audit trail has to say
	// so explicitly, rather than acquiring one by omission.
	config.Audit = AuditConfig{
		Enabled:         r.readOptionalBool("AUDIT_ENABLED", true),
		BufferSize:      int(r.readOptionalInt64("AUDIT_BUFFER_SIZE", 4096)),
		BatchSize:       int(r.readOptionalInt64("AUDIT_BATCH_SIZE", 200)),
		FlushIntervalMs: int(r.readOptionalInt64("AUDIT_FLUSH_INTERVAL_MS", 1000)),
	}
	r.errors = append(r.errors, validateAuditConfig(config.Audit)...)

	// Resource limits for agent resource configurations (operator-controlled ceilings)
	config.PerAgentResourceLimits = ResourceLimitsConfig{
		MaxReplicas: int(r.readOptionalInt64("RESOURCE_MAX_REPLICAS", 10)),
		MaxCPU:      r.readOptionalString("RESOURCE_MAX_CPU", "500m"),
		MaxMemory:   r.readOptionalString("RESOURCE_MAX_MEMORY", "1Gi"),
	}

	// Encryption key for secrets at rest (hex-encoded 32-byte AES-256 key)
	// Encryption key for secrets at rest (hex-encoded 32-byte AES-256 key).
	// Validated at runtime in wiring.ProvideEncryptionKey() so that
	// non-server commands (e.g. --migrate) can run without the key.
	config.EncryptionKey = r.readOptionalString("ENCRYPTION_KEY", "")

	// Validate HTTP server configurations
	validateHTTPServerConfigs(config, r)

	// Validate Internal server configurations
	validateInternalServerConfigs(config, r)

	validateOAuthAuthorizationServers(config, r)
	validateServerPublicURL(config, r)
	validateInstrumentationURL(config, r)
	validateObserverURLs(config, r)
	validateResourceLimitsConfig(config, r)
	validatePostgresTLSConfig(config, r)
	validateSecretManagerConfig(config, r)
	validateAgentWorkloadCORSConfig(agentWorkloadConfig, r)
	validateGatewayManifestCacheConfig(config, r)

	r.logAndExitIfErrorsFound()

	slog.Info("configReader: configs loaded")
}

// validPostgresSSLModes is the set of libpq sslmode values pgx accepts. Kept as
// an explicit allowlist so a typo fails config load with a clear message instead
// of surfacing as an opaque driver parse error at first connect.
var validPostgresSSLModes = map[string]struct{}{
	"disable":     {},
	"allow":       {},
	"prefer":      {},
	"require":     {},
	"verify-ca":   {},
	"verify-full": {},
}

// validateSecretManagerConfig fails config load on a malformed or nonpositive
// AGENT_IDENTITY_REFRESH_INTERVAL, instead of silently falling back to
// agentIdentityInjectionService's own default at first use (see
// secretSyncWaitDuration's doc comment).
func validateSecretManagerConfig(cfg *Config, r *configReader) {
	d, err := time.ParseDuration(cfg.SecretManager.AgentIdentityRefreshInterval)
	switch {
	case err != nil:
		r.errors = append(r.errors, fmt.Errorf(
			"AGENT_IDENTITY_REFRESH_INTERVAL %q is not a valid duration: %w", cfg.SecretManager.AgentIdentityRefreshInterval, err,
		))
	case d <= 0:
		r.errors = append(r.errors, fmt.Errorf(
			"AGENT_IDENTITY_REFRESH_INTERVAL must be a positive duration, got %q", cfg.SecretManager.AgentIdentityRefreshInterval,
		))
	}
}

// validGatewayManifestCacheBackends is the set of backends services.ProvideGatewayManifestCacheBackend
// (wiring) knows how to construct. Kept as an explicit allowlist so a typo fails config
// load with a clear message instead of silently falling back to "memory" in an HA
// deployment that meant to opt into "redis".
var validGatewayManifestCacheBackends = map[string]bool{
	"memory": true,
	"redis":  true,
}

func validateGatewayManifestCacheConfig(cfg *Config, r *configReader) {
	backend := strings.TrimSpace(cfg.GatewayManifestCache.Backend)
	if !validGatewayManifestCacheBackends[backend] {
		r.errors = append(r.errors, fmt.Errorf(
			"GATEWAY_MANIFEST_CACHE_BACKEND %q is not valid (must be \"memory\" or \"redis\")", cfg.GatewayManifestCache.Backend,
		))
		return
	}
	if backend != "redis" {
		return
	}
	// Only the host is mandatory: port/DB/TLS all have workable zero-value defaults,
	// and an empty password is a legitimate (if unusual) Redis deployment.
	if strings.TrimSpace(cfg.GatewayManifestCache.Redis.Host) == "" {
		r.errors = append(r.errors, fmt.Errorf(
			"GATEWAY_MANIFEST_CACHE_REDIS_HOST is required when GATEWAY_MANIFEST_CACHE_BACKEND=redis",
		))
	}
}

func validatePostgresTLSConfig(cfg *Config, r *configReader) {
	// loadEnvs already trims, so in the real flow this value is exactly what
	// makeConnString puts in the DSN. Trimmed again here so the check holds for
	// callers that build a Config directly rather than from the environment.
	mode := strings.TrimSpace(cfg.POSTGRESQL.SSLMode)
	if mode == "" {
		return
	}
	if _, ok := validPostgresSSLModes[mode]; !ok {
		r.errors = append(r.errors, fmt.Errorf(
			"DB_SSL_MODE %q is not a valid PostgreSQL sslmode (disable, allow, prefer, require, verify-ca, verify-full)", mode,
		))
	}
}

func validateHTTPServerConfigs(cfg *Config, r *configReader) {
	if cfg.ServerPort < 1 || cfg.ServerPort > 65535 {
		r.errors = append(r.errors, fmt.Errorf("SERVER_PORT must be between 1 and 65535, got %d", cfg.ServerPort))
	}
	if cfg.ReadTimeoutSeconds <= 0 {
		r.errors = append(r.errors, fmt.Errorf("HTTP_READ_TIMEOUT_SECONDS must be greater than 0, got %d", cfg.ReadTimeoutSeconds))
	}
	if cfg.WriteTimeoutSeconds <= 0 {
		r.errors = append(r.errors, fmt.Errorf("HTTP_WRITE_TIMEOUT_SECONDS must be greater than 0, got %d", cfg.WriteTimeoutSeconds))
	}
	if cfg.ReadTimeoutSeconds >= cfg.WriteTimeoutSeconds {
		r.errors = append(r.errors, fmt.Errorf("HTTP_READ_TIMEOUT_SECONDS (%d) must be < HTTP_WRITE_TIMEOUT_SECONDS (%d)",
			cfg.ReadTimeoutSeconds, cfg.WriteTimeoutSeconds))
	}
	if cfg.IdleTimeoutSeconds <= 0 {
		r.errors = append(r.errors, fmt.Errorf("HTTP_IDLE_TIMEOUT_SECONDS must be greater than 0, got %d", cfg.IdleTimeoutSeconds))
	}
	if cfg.MaxHeaderBytes < 1024 || cfg.MaxHeaderBytes > 1048576 { // 1KB to 1MB
		r.errors = append(r.errors, fmt.Errorf("HTTP_MAX_HEADER_BYTES must be between 1024 and 1048576, got %d", cfg.MaxHeaderBytes))
	}
}

func validateOAuthAuthorizationServers(cfg *Config, r *configReader) {
	for _, raw := range cfg.OAuthAuthorizationServers {
		u, err := url.Parse(raw)
		if err != nil {
			r.errors = append(r.errors, fmt.Errorf("OAUTH_AUTHORIZATION_SERVERS entry %q is not a valid URL: %w", raw, err))
			continue
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			r.errors = append(r.errors, fmt.Errorf("OAUTH_AUTHORIZATION_SERVERS entry %q must use http or https scheme", raw))
		}
		if u.Host == "" {
			r.errors = append(r.errors, fmt.Errorf("OAUTH_AUTHORIZATION_SERVERS entry %q must have a non-empty host", raw))
		}
	}
}

func validateServerPublicURL(cfg *Config, r *configReader) {
	if cfg.ServerPublicURL == "" {
		return
	}
	u, err := url.Parse(cfg.ServerPublicURL)
	if err != nil {
		r.errors = append(r.errors, fmt.Errorf("SERVER_PUBLIC_URL %q is not a valid URL: %w", cfg.ServerPublicURL, err))
		return
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		r.errors = append(r.errors, fmt.Errorf("SERVER_PUBLIC_URL %q must use http or https scheme", cfg.ServerPublicURL))
	}
	if u.Host == "" {
		r.errors = append(r.errors, fmt.Errorf("SERVER_PUBLIC_URL %q must have a non-empty host", cfg.ServerPublicURL))
	}
}

func validateInstrumentationURL(cfg *Config, r *configReader) {
	if cfg.InstrumentationURL == "" {
		return
	}
	u, err := url.Parse(cfg.InstrumentationURL)
	if err != nil {
		r.errors = append(r.errors, fmt.Errorf("INSTRUMENTATION_URL %q is not a valid URL: %w", cfg.InstrumentationURL, err))
		return
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		r.errors = append(r.errors, fmt.Errorf("INSTRUMENTATION_URL %q must use http or https scheme", cfg.InstrumentationURL))
	}
	if u.Host == "" {
		r.errors = append(r.errors, fmt.Errorf("INSTRUMENTATION_URL %q must have a non-empty host", cfg.InstrumentationURL))
	}
}

func validateObserverURLs(cfg *Config, r *configReader) {
	validate := func(envVar, raw string) {
		if raw == "" {
			return
		}
		u, err := url.Parse(raw)
		if err != nil {
			r.errors = append(r.errors, fmt.Errorf("%s %q is not a valid URL: %w", envVar, raw, err))
			return
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			r.errors = append(r.errors, fmt.Errorf("%s %q must use http or https scheme", envVar, raw))
		}
		if u.Host == "" {
			r.errors = append(r.errors, fmt.Errorf("%s %q must have a non-empty host", envVar, raw))
		}
	}
	validate("AM_OBSERVER_URL", cfg.Observer.URL)
	validate("AM_OBSERVER_PUBLIC_URL", cfg.Observer.PublicURL)
}

func validateInternalServerConfigs(cfg *Config, r *configReader) {
	if cfg.InternalServer.Port < 1 || cfg.InternalServer.Port > 65535 {
		r.errors = append(r.errors, fmt.Errorf("INTERNAL_SERVER_PORT must be between 1 and 65535, got %d", cfg.InternalServer.Port))
	}
	if cfg.InternalServer.ReadTimeoutSeconds <= 0 {
		r.errors = append(r.errors, fmt.Errorf("INTERNAL_SERVER_READ_TIMEOUT_SECONDS must be greater than 0, got %d", cfg.InternalServer.ReadTimeoutSeconds))
	}
	if cfg.InternalServer.WriteTimeoutSeconds <= 0 {
		r.errors = append(r.errors, fmt.Errorf("INTERNAL_SERVER_WRITE_TIMEOUT_SECONDS must be greater than 0, got %d", cfg.InternalServer.WriteTimeoutSeconds))
	}
	if cfg.InternalServer.CertDir == "" {
		r.errors = append(r.errors, fmt.Errorf("INTERNAL_SERVER_CERT_DIR must be non-empty"))
	}
}

func validateAgentWorkloadCORSConfig(cfg *AgentWorkload, r *configReader) {
	if cfg.CORS.AllowOrigin == "*" && cfg.CORS.AllowCredentials {
		r.errors = append(r.errors, fmt.Errorf("AGENT_WORKLOAD_CORS_ALLOW_CREDENTIALS cannot be true when AGENT_WORKLOAD_CORS_ALLOWED_ORIGIN is \"*\""))
	}
}

func validateResourceLimitsConfig(cfg *Config, r *configReader) {
	if cfg.PerAgentResourceLimits.MaxReplicas < 1 {
		r.errors = append(r.errors, fmt.Errorf("RESOURCE_MAX_REPLICAS must be at least 1, got %d", cfg.PerAgentResourceLimits.MaxReplicas))
	}
	if _, err := resource.ParseQuantity(cfg.PerAgentResourceLimits.MaxCPU); err != nil {
		r.errors = append(r.errors, fmt.Errorf("RESOURCE_MAX_CPU %q is not a valid Kubernetes resource quantity: %w", cfg.PerAgentResourceLimits.MaxCPU, err))
	}
	if _, err := resource.ParseQuantity(cfg.PerAgentResourceLimits.MaxMemory); err != nil {
		r.errors = append(r.errors, fmt.Errorf("RESOURCE_MAX_MEMORY %q is not a valid Kubernetes resource quantity: %w", cfg.PerAgentResourceLimits.MaxMemory, err))
	}
}

// validateAuditConfig rejects malformed audit tuning at startup.
//
// The recorder silently substitutes a default for any non-positive value, so a
// typo in a deployment would quietly change how much of the trail is buffered
// and how long a record waits, with nothing to show it had happened.
func validateAuditConfig(cfg AuditConfig) []error {
	var errs []error

	for _, field := range []struct {
		name  string
		value int
	}{
		{"AUDIT_BUFFER_SIZE", cfg.BufferSize},
		{"AUDIT_BATCH_SIZE", cfg.BatchSize},
		{"AUDIT_FLUSH_INTERVAL_MS", cfg.FlushIntervalMs},
	} {
		if field.value <= 0 {
			errs = append(errs, fmt.Errorf("%s must be a positive integer, got %d", field.name, field.value))
		}
	}

	// A batch larger than the buffer can never fill, so every flush would wait
	// for the timer instead — the batching would silently stop working.
	if cfg.BatchSize > 0 && cfg.BufferSize > 0 && cfg.BatchSize > cfg.BufferSize {
		errs = append(errs, fmt.Errorf(
			"AUDIT_BATCH_SIZE (%d) must not exceed AUDIT_BUFFER_SIZE (%d)",
			cfg.BatchSize, cfg.BufferSize,
		))
	}
	return errs
}
