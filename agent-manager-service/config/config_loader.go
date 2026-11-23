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
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var config *Config

// pendingErrors are the errors of the settings loaded at package init, from
// the environment alone. The command line loads the configuration file
// (Load) before anything reads the settings, which replaces them; a process
// that reads the settings without loading (a test binary) fails on them then.
var pendingErrors []error

func GetConfig() *Config {
	if pendingErrors != nil {
		(&configReader{errors: pendingErrors}).logAndExitIfErrorsFound()
	}
	return config
}

func init() {
	// The configuration file is not known yet (--config is parsed later), so
	// errors here wait for the first reader; see pendingErrors.
	loadSettings(false)
}

// loadEnvs reads every setting and exits on an invalid one.
func loadEnvs() { loadSettings(true) }

func loadSettings(strict bool) {
	config = &Config{}

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
	config.ConsoleEnabled = r.readOptionalBool("CONSOLE_ENABLED", true)

	// Logging configuration
	config.LogLevel = r.readOptionalString("LOG_LEVEL", "INFO")

	// read database configs
	// SQLite is the shipped database; a configured DB_HOST without DB_DRIVER still means PostgreSQL, so an install
	// that names its server is never silently moved to a new empty file.
	defaultDriver := DBDriverSQLite
	if strings.TrimSpace(Value("DB_HOST")) != "" {
		defaultDriver = DBDriverPostgres
	}
	config.Database = DatabaseConfig{
		Driver:     strings.ToLower(strings.TrimSpace(r.readOptionalString("DB_DRIVER", defaultDriver))),
		SQLitePath: strings.TrimSpace(r.readOptionalString("DB_PATH", filepath.Join(DataDir(), "agent-manager.db"))),
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

	// Use Version from ldflags or environment variable override
	config.PackageVersion = r.readOptionalString("AMP_VERSION", Version)

	config.IsLocalDevEnv = r.readOptionalBool("IS_LOCAL_DEV_ENV", false)

	config.DefaultOrg = DefaultOrgConfig{
		OUID:   strings.TrimSpace(r.readOptionalString("DEFAULT_ORG_OU_ID", "01900000-0000-7000-8000-000000000001")),
		Handle: r.readOptionalString("DEFAULT_ORG_HANDLE", "default"),
	}
	d := derivedDefaults(config.ServerPort)
	config.KeyManagerConfigurations = KeyManagerConfigurations{
		// Comma-separated list of allowed issuers and audiences
		Issuer:   r.readOptionalStringList("KEY_MANAGER_ISSUER", d.issuer),
		Audience: r.readOptionalStringList("KEY_MANAGER_AUDIENCE", d.audience),
		JWKSUrl:  r.readOptionalString("KEY_MANAGER_JWKS_URL", d.jwksURL),
	}
	config.IsOnPremDeployment = r.readOptionalBool("IS_ON_PREM_DEPLOYMENT", true)
	config.ServerPublicURL = r.readOptionalString("SERVER_PUBLIC_URL", "")
	config.InternalServerPublicURL = r.readOptionalString("INTERNAL_SERVER_PUBLIC_URL", "")
	config.GrowthAnalytics = GrowthAnalyticsConfig{
		Enabled:                   r.readOptionalBool("MOESIF_ENABLED", false),
		ConsoleEnabled:            r.readOptionalBool("CONSOLE_ANALYTICS_ENABLED", false),
		MoesifCollectorBaseURL:    r.readOptionalString("MOESIF_COLLECTOR_BASE_URL", ""),
		MoesifCollectorHostHeader: r.readOptionalString("MOESIF_COLLECTOR_HOST_HEADER", ""),
		MoesifApplicationID:       strings.TrimSpace(r.readOptionalString("MOESIF_APPLICATION_ID", "")),
		DeploymentModel:           r.readOptionalString("AMP_DEPLOYMENT_MODEL", "on-prem"),
		Environment:               r.readOptionalString("AMP_ENVIRONMENT", ""),
	}
	config.OAuthAuthorizationServers = r.readOptionalStringList("OAUTH_AUTHORIZATION_SERVERS", d.thunderURL)
	config.OAuthScopesSupported = r.readOptionalStringList("OAUTH_SCOPES_SUPPORTED", "")

	// IDP OAuth2 client credentials for service-to-service auth
	config.IDP = IDPConfig{
		TokenURL:     r.readOptionalString("IDP_TOKEN_URL", d.tokenURL),
		ClientID:     r.readOptionalString("IDP_CLIENT_ID", "amp-api-client"),
		ClientSecret: r.readOptionalString("IDP_CLIENT_SECRET", ""),
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
				config.FileAuth.Issuer,
			))
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

	config.OrgNamespace = r.readOptionalString("AMP_ORG_NAMESPACE", "default")

	// Internal Server configuration (for WebSocket and gateway internal APIs)
	config.InternalServer = InternalServerConfig{
		Host:                r.readOptionalString("INTERNAL_SERVER_HOST", ""),
		Port:                int(r.readOptionalInt64("INTERNAL_SERVER_PORT", 9243)),
		TLSEnabled:          r.readOptionalBool("INTERNAL_SERVER_TLS_ENABLED", true),
		CertDir:             r.readOptionalString("INTERNAL_SERVER_CERT_DIR", filepath.Join(DataDir(), "certs")),
		ReadTimeoutSeconds:  int(r.readOptionalInt64("INTERNAL_SERVER_READ_TIMEOUT_SECONDS", 10)),
		WriteTimeoutSeconds: int(r.readOptionalInt64("INTERNAL_SERVER_WRITE_TIMEOUT_SECONDS", 90)),
		IdleTimeoutSeconds:  int(r.readOptionalInt64("INTERNAL_SERVER_IDLE_TIMEOUT_SECONDS", 60)),
		MaxHeaderBytes:      int(r.readOptionalInt64("INTERNAL_SERVER_MAX_HEADER_BYTES", 65536)),
	}

	config.SecretManager = SecretManagerConfig{
		Provider:           r.readOptionalString("SECRET_MANAGER_PROVIDER", "inhouse"),
		VaultProvider:      r.readOptionalString("SECRET_VAULT_PROVIDER", "local"),
		VaultReaders:       r.readOptionalString("SECRET_VAULT_READERS", ""),
		VaultBindLocalRows: r.readOptionalBool("SECRET_VAULT_BIND_LOCAL_ROWS", false),
	}

	// Thunder admin API configuration for provisioning per-org OAuth apps
	config.Thunder = ThunderConfig{
		BaseURL:                  r.readOptionalString("THUNDER_BASE_URL", ""),
		ClientID:                 r.readOptionalString("THUNDER_CLIENT_ID", "amp-system-client"),
		ClientSecret:             r.readOptionalString("THUNDER_CLIENT_SECRET", ""),
		ResolveToHost:            r.readOptionalString("THUNDER_RESOLVE_TO_HOST", ""),
		UseDefaultResourceServer: r.readOptionalBool("AMP_USE_THUNDER_DEFAULT_RESOURCE_SERVER", false),
		SystemResourceIdentifier: r.readOptionalString("AMP_THUNDER_SYSTEM_RESOURCE_IDENTIFIER", ""),
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
	config.PlatformAdminOUID = strings.TrimSpace(r.readOptionalString("PLATFORM_ADMIN_OU_ID", DefaultPlatformAdminOUID))
	if config.PlatformAdminOUID == "" {
		slog.Warn("PLATFORM_ADMIN_OU_ID is not set; cross-organization platform routes will deny every request")
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

	// Encryption key for secrets at rest (hex-encoded 32-byte AES-256 key).
	// Validated at runtime in wiring.ProvideEncryptionKey() so that
	// non-server commands (e.g. --migrate) can run without the key.
	config.EncryptionKey = r.readOptionalString("ENCRYPTION_KEY", "")
	// Keys ENCRYPTION_KEY replaced; the secret vault still reads with them.
	config.PreviousEncryptionKeys = r.readOptionalString("ENCRYPTION_KEY_PREVIOUS", "")

	// Validate HTTP server configurations
	validateHTTPServerConfigs(config, r)

	// Validate Internal server configurations
	validateInternalServerConfigs(config, r)

	validateOAuthAuthorizationServers(config, r)
	validateServerPublicURL(config, r)
	validatePublicURL("INTERNAL_SERVER_PUBLIC_URL", config.InternalServerPublicURL, r)
	validateGrowthAnalyticsConfig(config)
	validateDatabaseConfig(config, r)
	validatePostgresTLSConfig(config, r)
	validateSecretManagerConfig(config, r)

	pendingErrors = nil
	if !strict {
		if len(r.errors) > 0 {
			pendingErrors = r.errors
		}
		return
	}
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
	validatePublicURL("SERVER_PUBLIC_URL", cfg.ServerPublicURL, r)
}

// validatePublicURL checks an optional http(s) origin setting.
func validatePublicURL(key, value string, r *configReader) {
	if value == "" {
		return
	}
	u, err := url.Parse(value)
	if err != nil {
		r.errors = append(r.errors, fmt.Errorf("%s %q is not a valid URL: %w", key, value, err))
		return
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		r.errors = append(r.errors, fmt.Errorf("%s %q must use http or https scheme", key, value))
	}
	if u.Host == "" {
		r.errors = append(r.errors, fmt.Errorf("%s %q must have a non-empty host", key, value))
	}
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

// DefaultPlatformAdminOUID is the organization a single-organization install
// (file sign-in, amp init's ThunderID bootstrap) uses.
const DefaultPlatformAdminOUID = "01900000-0000-7000-8000-000000000001"

// defaults are the settings worked out from the public URLs and sign-in mode
// when they are not set themselves.
type defaults struct {
	thunderURL, tokenURL, jwksURL, issuer, audience string
}

func derivedDefaults(serverPort int) defaults {
	publicURL := strings.TrimRight(strings.TrimSpace(Value("SERVER_PUBLIC_URL")), "/")
	thunderURL := strings.TrimRight(strings.TrimSpace(Value("THUNDER_BASE_URL")), "/")
	// The server dials ThunderID by THUNDER_RESOLVE_TO_HOST when set (a
	// container name), though its tokens name the public URL.
	thunderDial := thunderURL
	if h := strings.TrimSpace(Value("THUNDER_RESOLVE_TO_HOST")); h != "" && thunderURL != "" {
		if u, err := url.Parse(thunderURL); err == nil {
			thunderDial = u.Scheme + "://" + h
		}
	}
	d := defaults{
		thunderURL: thunderURL,
		tokenURL:   "http://thunder.amp.localhost:8080/oauth2/token",
		issuer:     "Agent Management Platform Local",
	}
	if thunderURL != "" {
		d.tokenURL = thunderDial + "/oauth2/token"
		d.issuer += "," + thunderURL
	}
	if strings.EqualFold(strings.TrimSpace(Value("AUTH_MODE")), AuthModeFile) {
		d.jwksURL = fmt.Sprintf("http://127.0.0.1:%d/auth/external/jwks.json", serverPort)
	} else if thunderURL != "" {
		d.jwksURL = thunderDial + "/oauth2/jwks"
	}
	audience := []string{
		"urn:wso2:amp", "localhost", valueOr("IDP_CLIENT_ID", "amp-api-client"),
		valueOr("AMP_CONSOLE_CLIENT_ID", "amp-console-client"), "amctl", "am-mcp",
	}
	if publicURL != "" {
		audience = append(audience, publicURL+"/mcp")
	}
	d.audience = strings.Join(audience, ",")
	return d
}

func valueOr(key, def string) string {
	if v := strings.TrimSpace(Value(key)); v != "" {
		return v
	}
	return def
}
