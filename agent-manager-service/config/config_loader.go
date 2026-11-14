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
	"math"
	"net"
	"net/url"
	"os"
	"regexp"
	"slices"
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
	// SQLite is the shipped database; a configured DB_HOST without DB_DRIVER still means PostgreSQL, so an install
	// that names its server is never silently moved to a new empty file.
	defaultDriver := DBDriverSQLite
	if strings.TrimSpace(os.Getenv("DB_HOST")) != "" {
		defaultDriver = DBDriverPostgres
	}
	config.Database = DatabaseConfig{
		Driver:     strings.ToLower(strings.TrimSpace(r.readOptionalString("DB_DRIVER", defaultDriver))),
		SQLitePath: strings.TrimSpace(r.readOptionalString("DB_PATH", "data/agent-manager.db")),
	}
	// The server settings are required only when the database is a server.
	readServerString := r.readOptionalString
	if config.Database.Driver == DBDriverPostgres {
		readServerString = func(key, _ string) string { return r.readRequiredString(key) }
	}
	config.POSTGRESQL = POSTGRESQL{
		Host:     readServerString("DB_HOST", ""),
		Port:     int(r.readOptionalInt64("DB_PORT", 5432)),
		User:     readServerString("DB_USER", ""),
		Password: readServerString("DB_PASSWORD", ""),
		DBName:   readServerString("DB_NAME", ""),
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
		// Tracing configuration
		IsTraceContentEnabled: r.readOptionalBool("OTEL_TRACELOOP_TRACE_CONTENT", true),

		// OTLP Exporter configuration: the AI Gateway's /otel API, addressed by
		// its vhost ("<env>-<org>.gateway.localhost") like agent trace exports.
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
	config.DefaultGateway = DefaultGatewayConfig{
		Name:              strings.TrimSpace(r.readOptionalString("DEFAULT_GATEWAY_NAME", "")),
		DisplayName:       r.readOptionalString("DEFAULT_GATEWAY_DISPLAY_NAME", "Default AI Gateway"),
		Vhost:             r.readOptionalString("DEFAULT_GATEWAY_VHOST", ""),
		FunctionalityType: r.readOptionalString("DEFAULT_GATEWAY_TYPE", "BOTH"),
		OrgOUID:           r.readOptionalString("DEFAULT_GATEWAY_ORG_OU_ID", "01900000-0000-7000-8000-000000000001"),
		OrgHandle:         r.readOptionalString("DEFAULT_GATEWAY_ORG_HANDLE", "default"),
		Token:             strings.TrimSpace(r.readOptionalString("DEFAULT_GATEWAY_TOKEN", "")),
		TokenFile:         r.readOptionalString("DEFAULT_GATEWAY_TOKEN_FILE", ""),
	}
	if g := config.DefaultGateway; g.Name != "" && (g.Vhost == "" || (g.Token == "" && g.TokenFile == "")) {
		r.errors = append(r.errors, fmt.Errorf(
			"DEFAULT_GATEWAY_NAME needs DEFAULT_GATEWAY_VHOST and DEFAULT_GATEWAY_TOKEN or DEFAULT_GATEWAY_TOKEN_FILE",
		))
	}
	config.OtelIngest = OtelIngestConfig{UpstreamURL: r.readOptionalString("OTEL_INGEST_UPSTREAM_URL", "")}
	config.A2ACardFetchDialAddr = r.readOptionalString("A2A_CARD_FETCH_DIAL_ADDR", "")
	if addr := config.A2ACardFetchDialAddr; addr != "" {
		if host, port, err := net.SplitHostPort(addr); err != nil {
			r.errors = append(r.errors, fmt.Errorf("A2A_CARD_FETCH_DIAL_ADDR must be host:port: %w", err))
		} else if host == "" || port == "" {
			r.errors = append(r.errors, fmt.Errorf("A2A_CARD_FETCH_DIAL_ADDR must contain a non-empty host and port"))
		}
	}
	config.KeyManagerConfigurations = KeyManagerConfigurations{
		// Comma-separated list of allowed issuers and audiences
		Issuer:   r.readOptionalStringList("KEY_MANAGER_ISSUER", "Agent Management Platform Local"),
		Audience: r.readOptionalStringList("KEY_MANAGER_AUDIENCE", "localhost"),
		JWKSUrl:  r.readOptionalString("KEY_MANAGER_JWKS_URL", ""),
	}
	config.IsOnPremDeployment = r.readOptionalBool("IS_ON_PREM_DEPLOYMENT", true)
	config.ServerPublicURL = r.readOptionalString("SERVER_PUBLIC_URL", "")
	config.GrowthAnalytics = GrowthAnalyticsConfig{
		Enabled:                   r.readOptionalBool("MOESIF_ENABLED", false),
		ConsoleEnabled:            r.readOptionalBool("CONSOLE_ANALYTICS_ENABLED", false),
		MoesifCollectorBaseURL:    r.readOptionalString("MOESIF_COLLECTOR_BASE_URL", ""),
		MoesifCollectorHostHeader: r.readOptionalString("MOESIF_COLLECTOR_HOST_HEADER", ""),
		MoesifApplicationID:       strings.TrimSpace(r.readOptionalString("MOESIF_APPLICATION_ID", "")),
		DeploymentModel:           r.readOptionalString("AMP_DEPLOYMENT_MODEL", "on-prem"),
		Environment:               r.readOptionalString("AMP_ENVIRONMENT", ""),
	}
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

	// Authentication mode. "idp" (the default) validates tokens issued by an
	// external identity provider; "file" authenticates against a YAML user file
	// and mints tokens locally.
	//
	// The default is "idp" so that a process started with no configuration
	// behaves as it always has. File mode refuses to start without a user
	// file — correct for a deployment that means to use it, wrong for every
	// harness and chart that never asked for it. The standalone stack opts in
	// explicitly (AUTH_MODE: ${AUTH_MODE:-file}), so the shipped product still
	// defaults to file-based sign-in.
	config.AuthMode = strings.ToLower(strings.TrimSpace(r.readOptionalString("AUTH_MODE", AuthModeIDP)))
	switch config.AuthMode {
	case AuthModeIDP, AuthModeFile:
	default:
		r.errors = append(r.errors, fmt.Errorf("AUTH_MODE must be %q or %q, got %q", AuthModeIDP, AuthModeFile, config.AuthMode))
	}
	config.FileAuth = FileAuthConfig{
		UsersPath: r.readOptionalString("FILE_AUTH_USERS_PATH", "/etc/amp/users.yaml"),
		Issuer:    r.readOptionalString("FILE_AUTH_ISSUER", "amp-file-auth"),
		Audience:  r.readOptionalString("FILE_AUTH_AUDIENCE", "urn:wso2:amp"),
		TokenTTL:  8 * time.Hour,
	}
	if raw := strings.TrimSpace(r.readOptionalString("FILE_AUTH_TOKEN_TTL", "")); raw != "" {
		ttl, err := time.ParseDuration(raw)
		switch {
		case err != nil:
			r.errors = append(r.errors, fmt.Errorf("FILE_AUTH_TOKEN_TTL must be a Go duration (e.g. 8h): %w", err))
		case ttl <= 0:
			r.errors = append(r.errors, fmt.Errorf("FILE_AUTH_TOKEN_TTL must be positive, got %s", ttl))
		default:
			config.FileAuth.TokenTTL = ttl
		}
	}
	if config.AuthMode == AuthModeFile {
		// Checked here rather than at login time: a deployment that cannot
		// authenticate anyone should fail to start, not fail every request.
		if strings.TrimSpace(config.FileAuth.UsersPath) == "" {
			r.errors = append(r.errors, fmt.Errorf("FILE_AUTH_USERS_PATH is required when AUTH_MODE=%s", AuthModeFile))
		}
		// The API gateway binds a key manager to an issuer, so a login token
		// sharing the agent-token issuer would be accepted as an agent token.
		// This one is a genuine conflict between two operator choices, so it is
		// refused rather than resolved.
		if strings.TrimSpace(config.FileAuth.Issuer) == strings.TrimSpace(config.JWTSigning.Issuer) {
			r.errors = append(r.errors, fmt.Errorf(
				"FILE_AUTH_ISSUER (%q) must differ from JWT_SIGNING_ISSUER: login tokens and agent tokens are told apart by issuer",
				config.FileAuth.Issuer))
		}

		// The next three are invariants, not choices: in file mode this service
		// is the issuer, the audience and the key source for its own login
		// tokens. Making an operator restate that in three more variables only
		// creates three more ways to get a 401 that names nothing.

		// validateIssuer and validateAudience are allowlists that reject when
		// empty, so a token whose own issuer is missing from them is refused.
		if !slices.Contains(config.KeyManagerConfigurations.Issuer, config.FileAuth.Issuer) {
			config.KeyManagerConfigurations.Issuer = append(config.KeyManagerConfigurations.Issuer, config.FileAuth.Issuer)
		}
		if !slices.Contains(config.KeyManagerConfigurations.Audience, config.FileAuth.Audience) {
			config.KeyManagerConfigurations.Audience = append(config.KeyManagerConfigurations.Audience, config.FileAuth.Audience)
		}
		// With no identity provider there is nothing else to validate against,
		// so default to this service's own JWKS. An explicit KEY_MANAGER_JWKS_URL
		// still wins, which is what a mixed deployment would set.
		if strings.TrimSpace(config.KeyManagerConfigurations.JWKSUrl) == "" {
			config.KeyManagerConfigurations.JWKSUrl = fmt.Sprintf("http://localhost:%d/auth/external/jwks.json", config.ServerPort)
		}
	}

	// GitHub configuration for repository API access
	config.GitHub = GitHubConfig{
		Token: r.readOptionalString("GITHUB_TOKEN", ""),
	}

	config.EvalJobsUsePublicGatewayURL = r.readOptionalBool("EVAL_JOBS_USE_PUBLIC_GATEWAY_URL", false)

	// Gateway manifest cache backend. "memory" (default) is process-local and only
	// safe for a single replica; HA deployments must set this to "redis" so every
	// replica reads and writes the same per-gateway cached manifests.
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
	config.OrgNamespace = r.readOptionalString("AMP_ORG_NAMESPACE", "default")

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
		Provider: r.readOptionalString("SECRET_MANAGER_PROVIDER", "unsupported"),
	}

	// Thunder admin API configuration for provisioning per-org OAuth apps
	config.Thunder = ThunderConfig{
		BaseURL:                  r.readOptionalString("THUNDER_BASE_URL", ""),
		ClientID:                 r.readOptionalString("THUNDER_CLIENT_ID", ""),
		ClientSecret:             r.readOptionalString("THUNDER_CLIENT_SECRET", ""),
		ResolveToHost:            r.readOptionalString("THUNDER_RESOLVE_TO_HOST", ""),
		UseDefaultResourceServer: r.readOptionalBool("AMP_USE_THUNDER_DEFAULT_RESOURCE_SERVER", false),
		SystemResourceIdentifier: r.readOptionalString("AMP_THUNDER_SYSTEM_RESOURCE_IDENTIFIER", ""),
	}
	config.IdentityManagementMode = r.readOptionalString("IDENTITY_MANAGEMENT_MODE", "LEGACY_DIRECT")
	validateIdentityManagementMode(config, r)
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
	validateThunderConfig(config, r)

	config.TLSConfig = TLSConfig{
		EnableTLS: r.readOptionalBool("TLS_ENABLED", false),
	}

	config.RootOUHandle = r.readOptionalString("ROOT_OU_HANDLE", "admin")

	// Cross-organization platform routes. Deliberately no default: an unset
	// PLATFORM_ADMIN_OU_ID leaves the routes registered but refusing every
	// caller, which is the safe end of the failure mode. Warned about rather
	// than fatal so open-source and local-dev deployments — which have no
	// platform-admin org at all — still start.
	config.PlatformAdminOUID = strings.TrimSpace(r.readOptionalString("PLATFORM_ADMIN_OU_ID", ""))
	if config.PlatformAdminOUID == "" {
		slog.Warn("PLATFORM_ADMIN_OU_ID is not set; cross-organization platform routes will deny every request")
	}
	config.DeploymentTimeout = DeploymentTimeoutConfig{
		SweepIntervalSeconds: int(r.readOptionalInt64("DEPLOYMENT_TIMEOUT_SWEEP_INTERVAL_SECONDS", 60)),
		TimeoutSeconds:       int(r.readOptionalInt64("DEPLOYMENT_TIMEOUT_SECONDS", 300)),
	}
	validateDeploymentTimeoutConfig(config, r)
	config.Reconcile = ReconcileConfig{
		RenderIntervalSeconds: int(r.readOptionalInt64("RECONCILE_RENDER_INTERVAL_SECONDS", 5)),
		PlaceIntervalSeconds:  int(r.readOptionalInt64("RECONCILE_PLACE_INTERVAL_SECONDS", 600)),
		DriftIntervalSeconds:  int(r.readOptionalInt64("RECONCILE_DRIFT_INTERVAL_SECONDS", 300)),
		RenderLeaseSeconds:    int(r.readOptionalInt64("RECONCILE_RENDER_LEASE_SECONDS", 120)),
		RenderAttemptBudget:   int(r.readOptionalInt64("RECONCILE_RENDER_ATTEMPT_BUDGET", 30)),
	}
	validateReconcileConfig(config, r)
	config.GatewayFailureThresholdSeconds = int(r.readOptionalInt64("GATEWAY_FAILURE_THRESHOLD_SECONDS", 300))
	if config.GatewayFailureThresholdSeconds <= 0 {
		r.errors = append(r.errors, fmt.Errorf(
			"GATEWAY_FAILURE_THRESHOLD_SECONDS must be greater than 0, got %d", config.GatewayFailureThresholdSeconds,
		))
	}
	// 604800 = 7 days. Checked against the staleness threshold rather than only
	// on its own: co-dependent values are validated together, and an upper bound
	// at or below the lower one leaves an empty window that reports every fleet
	// healthy — a failure mode with no symptom.
	config.GatewayFailureMaxAgeSeconds = int(r.readOptionalInt64("GATEWAY_FAILURE_MAX_AGE_SECONDS", 604800))
	if config.GatewayFailureMaxAgeSeconds <= config.GatewayFailureThresholdSeconds {
		r.errors = append(r.errors, fmt.Errorf(
			"GATEWAY_FAILURE_MAX_AGE_SECONDS (%d) must be greater than GATEWAY_FAILURE_THRESHOLD_SECONDS (%d), "+
				"or no gateway can ever be reported as failed",
			config.GatewayFailureMaxAgeSeconds, config.GatewayFailureThresholdSeconds,
		))
	}
	// Both ends are rejected rather than clamped, because either would make the
	// health verdict a constant: at 0 an empty fleet already reports unhealthy,
	// and above 100 no fleet ever can.
	//
	// NaN is checked explicitly because it slips through a range test: every
	// comparison against NaN is false, so `<= 0` and `> 100` both say "in
	// range". ParseFloat accepts the literal "NaN", and the verdict downstream
	// is `percentage < threshold`, also false — so an operator typo would boot
	// a service that reports the fleet unhealthy on every single request.
	// +Inf/-Inf need no special case; the range test already catches them.
	config.GatewayFailurePercentageThreshold = r.readOptionalFloat64("GATEWAY_FAILURE_PERCENTAGE_THRESHOLD", 10)
	if math.IsNaN(config.GatewayFailurePercentageThreshold) ||
		config.GatewayFailurePercentageThreshold <= 0 ||
		config.GatewayFailurePercentageThreshold > 100 {
		r.errors = append(r.errors, fmt.Errorf(
			"GATEWAY_FAILURE_PERCENTAGE_THRESHOLD must be greater than 0 and at most 100, got %v",
			config.GatewayFailurePercentageThreshold,
		))
	}

	// Audit defaults to on. A deployment that wants no audit trail has to say
	// so explicitly, rather than acquiring one by omission.
	config.Audit = AuditConfig{
		Enabled:         r.readOptionalBool("AUDIT_ENABLED", true),
		BufferSize:      int(r.readOptionalInt64("AUDIT_BUFFER_SIZE", 4096)),
		BatchSize:       int(r.readOptionalInt64("AUDIT_BATCH_SIZE", 200)),
		FlushIntervalMs: int(r.readOptionalInt64("AUDIT_FLUSH_INTERVAL_MS", 1000)),
	}
	r.errors = append(r.errors, validateAuditConfig(config.Audit)...)

	// Events default to off, so an install without a bus behaves as before.
	config.Events = EventsConfig{
		Mode:                     r.readOptionalString("EVENTS_MODE", ""),
		NATSURL:                  r.readOptionalString("EVENTS_NATS_URL", ""),
		NATSToken:                r.readOptionalString("EVENTS_NATS_TOKEN", ""),
		AllowPrivateDestinations: r.readOptionalBool("EVENTS_ALLOW_PRIVATE_DESTINATIONS", false),
		DeliveryMaxAttempts:      int(r.readOptionalInt64("EVENTS_DELIVERY_MAX_ATTEMPTS", 6)),
		DeliveryRetentionDays:    int(r.readOptionalInt64("EVENTS_DELIVERY_RETENTION_DAYS", 14)),
	}
	if config.Events.Mode == "" {
		config.Events.Mode = EventsModeOff
		if config.Events.NATSURL != "" {
			config.Events.Mode = EventsModeNATS
		}
	}
	r.errors = append(r.errors, validateEventsConfig(config.Events)...)

	// Resource limits for agent resource configurations (operator-controlled ceilings)
	config.PerAgentResourceLimits = ResourceLimitsConfig{
		MaxReplicas: int(r.readOptionalInt64("RESOURCE_MAX_REPLICAS", 10)),
		MaxCPU:      r.readOptionalString("RESOURCE_MAX_CPU", "500m"),
		MaxMemory:   r.readOptionalString("RESOURCE_MAX_MEMORY", "1Gi"),
	}

	// File mount size caps. The defaults are 1 MB (decimal) rather than 1 MiB so
	// a mount at the per-file cap still leaves room for the ConfigMap/Secret's
	// own metadata under Kubernetes' 1 MiB object limit.
	config.FileMountLimits = FileMountLimitsConfig{
		MaxFileBytes:  int(r.readOptionalInt64("FILE_MOUNT_MAX_FILE_BYTES", 1000000)),
		MaxTotalBytes: int(r.readOptionalInt64("FILE_MOUNT_MAX_TOTAL_BYTES", 1000000)),
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
	validateGrowthAnalyticsConfig(config)
	validateResourceLimitsConfig(config, r)
	validateFileMountLimitsConfig(config, r)
	validateDatabaseConfig(config, r)
	validatePostgresTLSConfig(config, r)
	validateSecretManagerConfig(config, r)
	validateAgentWorkloadCORSConfig(agentWorkloadConfig, r)
	validateGatewayManifestCacheConfig(config, r)

	r.logAndExitIfErrorsFound()

	slog.Info("configReader: configs loaded")
}

func validateIdentityManagementMode(cfg *Config, r *configReader) {
	switch cfg.IdentityManagementMode {
	case "LEGACY_DIRECT", "CP_MANAGED":
	default:
		r.errors = append(r.errors, fmt.Errorf("IDENTITY_MANAGEMENT_MODE must be LEGACY_DIRECT or CP_MANAGED"))
	}
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
}

type positiveField struct {
	name  string
	value int
}

func requirePositive(r *configReader, fields ...positiveField) {
	for _, field := range fields {
		if field.value <= 0 {
			r.errors = append(r.errors, fmt.Errorf("%s must be greater than 0, got %d", field.name, field.value))
		}
	}
}

func validateDeploymentTimeoutConfig(cfg *Config, r *configReader) {
	requirePositive(
		r,
		positiveField{"DEPLOYMENT_TIMEOUT_SWEEP_INTERVAL_SECONDS", cfg.DeploymentTimeout.SweepIntervalSeconds},
		positiveField{"DEPLOYMENT_TIMEOUT_SECONDS", cfg.DeploymentTimeout.TimeoutSeconds},
	)
}

func validateReconcileConfig(cfg *Config, r *configReader) {
	requirePositive(
		r,
		positiveField{"RECONCILE_RENDER_INTERVAL_SECONDS", cfg.Reconcile.RenderIntervalSeconds},
		positiveField{"RECONCILE_PLACE_INTERVAL_SECONDS", cfg.Reconcile.PlaceIntervalSeconds},
		positiveField{"RECONCILE_DRIFT_INTERVAL_SECONDS", cfg.Reconcile.DriftIntervalSeconds},
		positiveField{"RECONCILE_RENDER_LEASE_SECONDS", cfg.Reconcile.RenderLeaseSeconds},
		positiveField{"RECONCILE_RENDER_ATTEMPT_BUDGET", cfg.Reconcile.RenderAttemptBudget},
	)
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

func validateDatabaseConfig(cfg *Config, r *configReader) {
	switch cfg.Database.Driver {
	case DBDriverPostgres:
	case DBDriverSQLite:
		if cfg.Database.SQLitePath == "" {
			r.errors = append(r.errors, fmt.Errorf("DB_PATH is required when DB_DRIVER=%s", DBDriverSQLite))
		}
	default:
		r.errors = append(r.errors, fmt.Errorf(
			"DB_DRIVER %q is not supported (%s, %s)", cfg.Database.Driver, DBDriverPostgres, DBDriverSQLite,
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

func validateThunderConfig(cfg *Config, r *configReader) {
	thunder := cfg.Thunder
	if thunder.UseDefaultResourceServer && thunder.SystemResourceIdentifier != "" {
		r.errors = append(r.errors, fmt.Errorf(
			"AMP_USE_THUNDER_DEFAULT_RESOURCE_SERVER cannot be true when AMP_THUNDER_SYSTEM_RESOURCE_IDENTIFIER is set",
		))
	}
	if (thunder.UseDefaultResourceServer || thunder.SystemResourceIdentifier != "") && thunder.BaseURL == "" {
		r.errors = append(r.errors, fmt.Errorf(
			"AMP_USE_THUNDER_DEFAULT_RESOURCE_SERVER and AMP_THUNDER_SYSTEM_RESOURCE_IDENTIFIER require THUNDER_BASE_URL",
		))
	}
	if thunder.SystemResourceIdentifier == "" {
		return
	}
	u, err := url.Parse(thunder.SystemResourceIdentifier)
	if err != nil || !u.IsAbs() {
		r.errors = append(r.errors, fmt.Errorf(
			"AMP_THUNDER_SYSTEM_RESOURCE_IDENTIFIER %q must be an absolute URI", thunder.SystemResourceIdentifier,
		))
		return
	}
	if u.Fragment != "" {
		r.errors = append(r.errors, fmt.Errorf(
			"AMP_THUNDER_SYSTEM_RESOURCE_IDENTIFIER %q must not contain a fragment", thunder.SystemResourceIdentifier,
		))
	}
	if (u.Scheme == "http" || u.Scheme == "https") && u.Host == "" {
		r.errors = append(r.errors, fmt.Errorf(
			"AMP_THUNDER_SYSTEM_RESOURCE_IDENTIFIER %q must have a non-empty host", thunder.SystemResourceIdentifier,
		))
	}
}

// organizationTokenPathSuffix is the token endpoint path an organization token
// endpoint is derived from.
const organizationTokenPathSuffix = "/oauth2/token"

// organizationIDPattern is what an organization (organization unit) ID may look
// like in an organization token endpoint path: one path segment of letters,
// digits, '.', '_' and '-' (identity provider OU IDs are UUIDs), so it never needs
// escaping and can never add or climb a path segment.
var organizationIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// OrganizationTokenURL returns the identity provider's token endpoint for one
// organization: tokenURL, an absolute http(s) URL whose path ends in
// /oauth2/token, with /ou/<ouID> inserted before that suffix. A token issued there
// is scoped to that organization (its ouId claim names it).
func OrganizationTokenURL(tokenURL, ouID string) (string, error) {
	if !organizationIDPattern.MatchString(ouID) || ouID == "." || ouID == ".." {
		return "", fmt.Errorf("invalid organization ID %q", ouID)
	}
	parsed, err := url.Parse(strings.TrimSpace(tokenURL))
	if err != nil {
		return "", fmt.Errorf("invalid token URL: %w", err)
	}
	if (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return "", fmt.Errorf("token URL must be an absolute http(s) URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("token URL must not carry a query or fragment")
	}
	prefix, ok := strings.CutSuffix(parsed.Path, organizationTokenPathSuffix)
	if !ok {
		return "", fmt.Errorf("token URL path must end with %s", organizationTokenPathSuffix)
	}
	if strings.Contains(prefix+"/", "/ou/") {
		return "", fmt.Errorf("token URL must be the identity provider's own token endpoint, not an organization's")
	}
	parsed.Path = prefix + "/ou/" + ouID + organizationTokenPathSuffix
	parsed.RawPath = ""
	return parsed.String(), nil
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

// validateGrowthAnalyticsConfig checks the feature-usage telemetry settings.
//
// Unlike every other validator here it never fails config load. Telemetry is
// non-essential: a bad MOESIF_* value must not hold the whole API hostage, so
// a misconfiguration disables only the tracking and the service starts
// normally. Each problem is logged at WARN — loud enough to find, since the
// alternative symptom is events silently never arriving — and the collector
// URL is then cleared, which is what makes middleware/growthanalytics no-op.
//
// Clearing rather than merely warning matters for a malformed URL: leaving it
// in place would let Track install its wrapper and fail per request, turning
// one startup warning into an error line on every tracked call.
//
// MOESIF_COLLECTOR_HOST_HEADER only ever applies to a configured collector
// URL, so setting it alone is a misconfiguration worth naming rather than
// ignoring.
//
// It also reports the resolved on/off state once at startup. Without that
// line, tracking being off is indistinguishable from tracking being broken:
// Track no-ops at route-registration time, so a disabled deployment produces
// no events and no log entries at all, and "why is Moesif empty" can only be
// answered by reading the pod's environment. One INFO line at boot answers it
// from the logs instead.
func validateGrowthAnalyticsConfig(cfg *Config) {
	warned := false
	disable := func(msg string, args ...any) {
		slog.Warn("configReader: "+msg+"; feature-usage tracking is disabled, the service is unaffected", args...)
		cfg.GrowthAnalytics.MoesifCollectorBaseURL = ""
		warned = true
	}

	// With an Application ID the obvious target is Moesif itself, so an
	// unset URL means "send straight to Moesif" rather than "disabled".
	if cfg.GrowthAnalytics.MoesifApplicationID != "" && cfg.GrowthAnalytics.MoesifCollectorBaseURL == "" {
		cfg.GrowthAnalytics.MoesifCollectorBaseURL = defaultMoesifAPIBaseURL
	}

	switch cfg.GrowthAnalytics.MoesifCollectorBaseURL {
	case "":
		if cfg.GrowthAnalytics.MoesifCollectorHostHeader != "" {
			disable("MOESIF_COLLECTOR_HOST_HEADER is set but MOESIF_COLLECTOR_BASE_URL is empty, "+
				"and the host header only applies to a configured collector URL",
				"hostHeader", cfg.GrowthAnalytics.MoesifCollectorHostHeader)
		}
	default:
		u, err := url.Parse(cfg.GrowthAnalytics.MoesifCollectorBaseURL)
		switch {
		case err != nil:
			disable("MOESIF_COLLECTOR_BASE_URL is not a valid URL",
				"url", cfg.GrowthAnalytics.MoesifCollectorBaseURL, "error", err)
		case u.Scheme != "http" && u.Scheme != "https":
			disable("MOESIF_COLLECTOR_BASE_URL must use the http or https scheme",
				"url", cfg.GrowthAnalytics.MoesifCollectorBaseURL, "scheme", u.Scheme)
		case u.Host == "":
			disable("MOESIF_COLLECTOR_BASE_URL must have a non-empty host",
				"url", cfg.GrowthAnalytics.MoesifCollectorBaseURL)
		case cfg.GrowthAnalytics.MoesifApplicationID != "" && u.Scheme != "https" && !isLoopbackHost(u.Hostname()):
			// The Application ID is a credential; never send it in cleartext
			// beyond this machine.
			disable("MOESIF_COLLECTOR_BASE_URL must use https when MOESIF_APPLICATION_ID is set",
				"url", cfg.GrowthAnalytics.MoesifCollectorBaseURL)
		}
	}

	logGrowthAnalyticsState(cfg.GrowthAnalytics, warned)
}

// defaultMoesifAPIBaseURL is Moesif's public collection API, used when an
// Application ID is configured without a collector URL.
const defaultMoesifAPIBaseURL = "https://api.moesif.net"

// isLoopbackHost reports whether host is localhost or a loopback IP, the only
// places an Application ID may be sent over plain http (local proxies, tests).
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// logGrowthAnalyticsState records, once at startup, whether feature-usage
// tracking will actually report anything — the two conditions Track checks
// before it wraps a route. alreadyWarned suppresses the redundant "disabled"
// line when a WARN above has already said so and given the real reason.
func logGrowthAnalyticsState(ga GrowthAnalyticsConfig, alreadyWarned bool) {
	if alreadyWarned {
		return
	}
	switch {
	case !ga.Enabled:
		slog.Info("growthanalytics: feature-usage tracking disabled (MOESIF_ENABLED is false)")
	case ga.MoesifCollectorBaseURL == "":
		slog.Info("growthanalytics: feature-usage tracking disabled (neither MOESIF_APPLICATION_ID nor MOESIF_COLLECTOR_BASE_URL is set)")
	default:
		slog.Info("growthanalytics: feature-usage tracking enabled",
			"collector", ga.MoesifCollectorBaseURL,
			"auth", growthAnalyticsAuthMode(ga),
			"environment", ga.Environment,
			"deploymentModel", ga.DeploymentModel,
			"consoleActions", ga.ConsoleEnabled)
		if !ga.ConsoleEnabled {
			slog.Info("growthanalytics: console action tracking disabled (CONSOLE_ANALYTICS_ENABLED is false); " +
				"endpoint feature-usage events are unaffected")
		}
	}
}

// growthAnalyticsAuthMode names the credential in use without revealing it.
func growthAnalyticsAuthMode(ga GrowthAnalyticsConfig) string {
	if ga.MoesifApplicationID != "" {
		return "moesif-application-id"
	}
	return "caller-jwt"
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

// kubernetesObjectMaxBytes is Kubernetes' size limit for a single ConfigMap or
// Secret. All of an agent's file mounts render into one such object.
const kubernetesObjectMaxBytes = 1 << 20

func validateFileMountLimitsConfig(cfg *Config, r *configReader) {
	limits := cfg.FileMountLimits
	if limits.MaxFileBytes < 1 {
		r.errors = append(r.errors, fmt.Errorf("FILE_MOUNT_MAX_FILE_BYTES must be at least 1, got %d", limits.MaxFileBytes))
	}
	if limits.MaxTotalBytes < 1 {
		r.errors = append(r.errors, fmt.Errorf("FILE_MOUNT_MAX_TOTAL_BYTES must be at least 1, got %d", limits.MaxTotalBytes))
	}
	// A per-file cap above the total could never be reached, which would make
	// the per-file setting look like it works when it does not.
	if limits.MaxFileBytes > limits.MaxTotalBytes {
		r.errors = append(r.errors, fmt.Errorf("FILE_MOUNT_MAX_FILE_BYTES (%d) must not exceed FILE_MOUNT_MAX_TOTAL_BYTES (%d)", limits.MaxFileBytes, limits.MaxTotalBytes))
	}
	// Above this the rendered object is rejected by Kubernetes at deploy time,
	// after the request has already been accepted.
	if limits.MaxTotalBytes > kubernetesObjectMaxBytes {
		r.errors = append(r.errors, fmt.Errorf("FILE_MOUNT_MAX_TOTAL_BYTES must not exceed %d (the Kubernetes ConfigMap/Secret limit), got %d", kubernetesObjectMaxBytes, limits.MaxTotalBytes))
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

// validateEventsConfig checks the event bus settings.
func validateEventsConfig(cfg EventsConfig) []error {
	var errs []error
	switch cfg.Mode {
	case EventsModeOff:
		return nil
	case EventsModeEmbedded:
	case EventsModeNATS:
		if cfg.NATSURL == "" {
			errs = append(errs, fmt.Errorf("EVENTS_NATS_URL is required when EVENTS_MODE is nats"))
		}
	default:
		return []error{fmt.Errorf("EVENTS_MODE must be nats, embedded or off, got %q", cfg.Mode)}
	}
	if cfg.DeliveryMaxAttempts <= 0 {
		errs = append(errs, fmt.Errorf("EVENTS_DELIVERY_MAX_ATTEMPTS must be positive, got %d", cfg.DeliveryMaxAttempts))
	}
	if cfg.DeliveryRetentionDays <= 0 {
		errs = append(errs, fmt.Errorf("EVENTS_DELIVERY_RETENTION_DAYS must be positive, got %d", cfg.DeliveryRetentionDays))
	}
	return errs
}
