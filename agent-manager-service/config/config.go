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

// Config holds all configuration for the application
type Config struct {
	PackageVersion      string
	ServerHost          string
	ServerPort          int
	AuthHeader          string
	AutoMaxProcsEnabled bool
	LogLevel            string
	POSTGRESQL          POSTGRESQL
	// HTTP Server timeout configurations
	ReadTimeoutSeconds  int
	WriteTimeoutSeconds int
	IdleTimeoutSeconds  int
	MaxHeaderBytes      int
	// Database operation timeout configuration
	DbOperationTimeoutSeconds int
	HealthCheckTimeoutSeconds int

	// CORSAllowedOrigin is the single allowed origin for CORS; use "*" to allow all
	CORSAllowedOrigin string

	// OpenTelemetry configuration
	OTEL OTELConfig

	// Observer service configuration (agent-manager-observer: build logs,
	// trace tools in MCP, and the console/CLI discovery endpoint)
	Observer ObserverConfig

	// Instrumentation url for MCP
	InstrumentationURL string

	IsLocalDevEnv bool

	// Default Chat API configuration
	DefaultChatAPI     DefaultChatAPIConfig
	DefaultGatewayPort int

	// JWT Signing configuration for agent API tokens
	JWTSigning JWTSigningConfig

	KeyManagerConfigurations KeyManagerConfigurations
	IsOnPremDeployment       bool
	ServerPublicURL          string

	// ThunderHostBaseDomain is the domain suffix env-Thunder's developer-facing
	// hostnames are built from: "<handle>.<ThunderHostBaseDomain>".
	// Default "amp.localhost" matches local dev (k3d + the *.amp.localhost wildcard
	// cert). VM/production deployments set this to their own base domain (e.g. a
	// sslip.io address) — see deployments/vm/lib-vm.sh, which sets the identical
	// value when provisioning env-Thunder so the Go-reported URLs and the actually
	// deployed Thunder instance's own self-configured issuer never diverge.
	ThunderHostBaseDomain string

	// ThunderAskSecret, when set, is the shared secret Caddy's on-demand-TLS ask
	// call presents (header X-Thunder-Ask-Secret) to /internal/thunder-ask so
	// that call can be told apart from the public internet, which reaches the
	// same path through the api host's own catch-all route — see
	// api/thunder_ask_routes.go. Empty by default: a deployment that hasn't set
	// this (e.g. one upgrading without regenerating its Caddyfile) keeps today's
	// single shared rate limit rather than breaking.
	ThunderAskSecret string

	// OAuthAuthorizationServers is the list of OAuth 2.0 authorization server URLs
	// advertised in the RFC 9728 protected resource metadata document. Each entry
	// MUST be an absolute http/https URL (validated at config load). Required for
	// the /.well-known/oauth-protected-resource endpoint to serve.
	OAuthAuthorizationServers []string

	// OAuthScopesSupported is the list of OAuth 2.0 scopes supported by this resource.
	// Advertised in the RFC 9728 protected resource metadata document.
	OAuthScopesSupported []string

	// IDP OAuth2 client credentials for service-to-service auth
	IDP IDPConfig

	// GitHub configuration for repository API access
	GitHub GitHubConfig

	// OpenChoreo API configuration
	OpenChoreo OpenChoreoConfig

	// Internal Server configuration (for WebSocket and gateway internal APIs)
	InternalServer InternalServerConfig

	// WebSocket configuration
	WebSocket WebSocketConfig

	// EncryptionKey is a hex-encoded 32-byte key used for AES-256-GCM encryption
	// of secrets at rest (e.g., LLM provider API keys in monitor configs).
	EncryptionKey string `json:"-"`

	// Secret Manager configuration
	SecretManager SecretManagerConfig

	// OpenBao KV store configuration (data plane - for deployment secrets)
	OpenBao OpenBaoConfig

	// WorkflowPlaneOpenBao KV store configuration (workflow plane - for git secrets)
	WorkflowPlaneOpenBao OpenBaoConfig

	// Thunder admin API configuration for provisioning OAuth apps
	Thunder ThunderConfig

	// RBACEnabled enables scope-based authorization on every API route.
	// When false (default), all authenticated requests are allowed regardless of token scopes.
	// Flip to true after roles are assigned to users in Thunder.
	RBACEnabled bool

	// RootOUHandle identifies the root/admin Organization Unit in Thunder.
	// Client-credentials tokens issued to this OU are allowed to access any
	// org's gateway-registration route (see RequireOrgMatchAllowRootOU /
	// RequirePermissionAllowRootOU in middleware/authorization.go), since
	// system clients always carry the root OU rather than a specific tenant's OU.
	RootOUHandle string

	// TLS Configurations
	TLSConfig TLSConfig

	// PerAgentResourceLimits defines the operator-configured maximum values for agent resource configs
	PerAgentResourceLimits ResourceLimitsConfig

	// Audit configures the audit trail.
	Audit AuditConfig

	// GatewayManifestCache configures where the gateway-reported policy manifest
	// cache lives. The default in-memory backend is process-local and therefore
	// inconsistent across replicas — set Backend to "redis" in HA deployments.
	GatewayManifestCache GatewayManifestCacheConfig
}

// GatewayManifestCacheConfig selects and configures the backend for the
// gateway-manifest cache (see services.GatewayManifestCacheBackend).
type GatewayManifestCacheConfig struct {
	// Backend is "memory" (default, single-replica only) or "redis" (required for
	// HA — a per-replica in-memory cache would leave replicas disagreeing on which
	// policies gateways report, since each only sees the manifest pushes routed to it).
	Backend string
	Redis   GatewayManifestCacheRedisConfig
}

// GatewayManifestCacheRedisConfig configures the Redis backend. Only read/validated
// when GatewayManifestCacheConfig.Backend == "redis".
type GatewayManifestCacheRedisConfig struct {
	Host       string
	Port       int
	Password   string `json:"-"`
	DB         int
	TLSEnabled bool
}

// AuditConfig controls the audit trail.
//
// Records are written to stdout as structured JSON and collected by the
// platform's log pipeline. Retention and immutability are therefore properties
// of that pipeline, not of this service — see docs/audit-logging.md, which
// documents the retention the deployment must provide.
type AuditConfig struct {
	// Enabled turns audit recording on. Disabling it leaves the platform with
	// no record of who changed what, so it should only be off for local
	// development.
	Enabled bool

	// BufferSize bounds queued events. When the buffer is full, events are
	// dropped and counted rather than blocking the request that produced them:
	// a slow sink must not become an outage.
	BufferSize int

	// BatchSize is the maximum number of events written to the sink at once.
	BatchSize int

	// FlushIntervalMs bounds how long an event waits before being written, so a
	// quiet service still emits promptly.
	FlushIntervalMs int
}

type TLSConfig struct {
	// EnableTLS indicates whether TLS is enabled for the server
	EnableTLS bool
}

// SecretManagerConfig holds secret manager client configuration
type SecretManagerConfig struct {
	// Provider is the secret store provider name (e.g., "openchoreo")
	Provider string
	// TargetPlaneKind is the kind of the plane hosting secret data
	// (e.g. "ClusterDataPlane")
	TargetPlaneKind string
	// TargetPlaneName is the name of the plane hosting secret data
	TargetPlaneName string
	// RefreshInterval is how often SecretReference CRs should refresh from the
	// secret store (default: "1h")
	RefreshInterval string
	// AgentIdentityRefreshInterval is the AgentID SecretReference's refresh
	// cadence (default: "15s"), kept separate from RefreshInterval above so
	// AgentID's fast rotation-to-pod requirement never depends on it.
	AgentIdentityRefreshInterval string
}

// OpenBaoConfig holds OpenBao KV store configuration.
// Only KV v2 secrets engine is supported.
type OpenBaoConfig struct {
	// URL is the OpenBao server URL (e.g., http://openbao.openbao.svc:8200)
	URL string
	// Token is the authentication token
	Token string `json:"-"`
	// Path is the KV secrets engine mount path (default: "secret")
	Path string
}

// OpenChoreoConfig holds OpenChoreo API configuration
type OpenChoreoConfig struct {
	// BaseURL is the OpenChoreo API base URL
	BaseURL string
	// DefaultNamespace is the OpenChoreo namespace (organization) all API
	// calls are scoped to. The deployment runs single-namespace.
	DefaultNamespace string
	// SystemLabelKeyPrefixes lists component label-key prefixes that are
	// reserved for internal use and never surfaced as user labels in agent
	// API responses.
	SystemLabelKeyPrefixes []string
}

// GitHubConfig holds GitHub API configuration
type GitHubConfig struct {
	// Token is a GitHub Personal Access Token for API authentication (optional but recommended)
	// Without a token, rate limit is 60 requests/hour; with token, 5000 requests/hour
	Token string `json:"-"`
}

type IDPConfig struct {
	TokenURL     string
	ClientID     string
	ClientSecret string `json:"-"`
}

type KeyManagerConfigurations struct {
	Issuer   []string
	Audience []string
	JWKSUrl  string
}

type AgentWorkload struct {
	CORS CORSConfig
}

type CORSConfig struct {
	AllowOrigin      string
	AllowMethods     string
	AllowHeaders     string
	AllowCredentials bool
}

// OTELConfig holds all OpenTelemetry related configuration
type OTELConfig struct {
	// Instrumentation configuration
	SDKVolumeName string
	SDKMountPath  string

	// DefaultInstrumentationVersion is the AMP instrumentation version used for an
	// agent that has not selected one; it resolves to the pre-built
	// amp-python-instrumentation-provider:<version>-python<X.Y> init-container image.
	// Validated at app startup against the assembled instrumentation catalog.
	DefaultInstrumentationVersion string

	// InstrumentationExtensionPath is the on-disk YAML file holding
	// operator-supplied catalog extension entries; consumed by
	// instrumentation.Load. An empty value or missing file is treated as
	// no extension (baseline-only catalog).
	InstrumentationExtensionPath string

	// Tracing configuration
	IsTraceContentEnabled bool

	// OTLP Exporter configuration
	ExporterEndpoint string
}

type ObserverConfig struct {
	// URL is the observer service URL the agent-manager-service itself
	// uses (server-side, in-cluster) for monitor-run log fetches and trace
	// data queries.
	URL string
	// PublicURL is the externally reachable observer URL handed to
	// out-of-cluster clients (console, CLI) via the GET /api/v1/config endpoint.
	// It has NO fallback to URL: empty means "observer not configured" and
	// clients surface that loudly.
	PublicURL string
}

type POSTGRESQL struct {
	Host     string
	Port     int
	User     string
	DBName   string
	Password string `json:"-"`
	// SSLMode is the libpq/pgx "sslmode" connection parameter
	// (disable|allow|prefer|require|verify-ca|verify-full). Empty means the
	// parameter is omitted from the connection string, which leaves the driver
	// default ("prefer") — and any PGSSLMODE in the environment — in effect.
	SSLMode string
	// SSLRootCert is the libpq/pgx "sslrootcert" connection parameter: a path to
	// a PEM CA bundle, or the literal "system" to use the image trust store.
	// Empty omits the parameter, so verification falls back to the system pool.
	SSLRootCert string
	DbConfigs
}

type DbConfigs struct {
	// gorm configs
	SlowThresholdMilliseconds int64
	SkipDefaultTransaction    bool

	// go sql configs
	MaxIdleCount       *int64 // zero means defaultMaxIdleConns (2); negative means 0
	MaxOpenCount       *int64 // <= 0 means unlimited
	MaxLifetimeSeconds *int64 // maximum amount of time a connection may be reused
	MaxIdleTimeSeconds *int64
}

type DefaultChatAPIConfig struct {
	DefaultHTTPPort int32
	DefaultBasePath string
}

// JWTSigningConfig holds configuration for JWT token generation
type JWTSigningConfig struct {
	// PrivateKeyPath is the path to the RSA private key file (PEM format)
	PrivateKeyPath string
	// PublicKeysConfigPath is the path to the JSON file containing multiple public keys (required)
	PublicKeysConfigPath string
	// ActiveKeyID is the key ID (kid) to use for signing tokens
	ActiveKeyID string
	// DefaultExpiryDuration is the default token expiry duration (e.g., "8760h" for 1 year)
	DefaultExpiryDuration string
	// Issuer is the issuer claim for the JWT
	Issuer string
	// DefaultEnvironment is the default environment to use for token claims
	DefaultEnvironment string
}

// PublicKeyConfig represents a single public key configuration in the JSON file
type PublicKeyConfig struct {
	Kid           string `json:"kid"`
	Algorithm     string `json:"algorithm"`
	PublicKeyPath string `json:"publicKeyPath"`
	Description   string `json:"description,omitempty"`
	CreatedAt     string `json:"createdAt,omitempty"`
}

// PublicKeysConfig represents the structure of the public keys JSON configuration file
type PublicKeysConfig struct {
	Keys []PublicKeyConfig `json:"keys"`
}

// APIPlatformConfig holds API Platform client configuration
type APIPlatformConfig struct {
	BaseURL string // Base URL for API Platform
	Enable  bool
}

// InternalServerConfig holds configuration for the internal server
// This server hosts WebSocket connections and gateway internal APIs
type InternalServerConfig struct {
	Host       string // Server host (default: "")
	Port       int    // Server port (default: 9243)
	TLSEnabled bool   // Enable TLS (default: true). When false, serves plain HTTP.
	CertDir    string // Directory for TLS certificates (default: "./data/certs")
	// HTTP Server timeout configurations
	ReadTimeoutSeconds  int
	WriteTimeoutSeconds int
	IdleTimeoutSeconds  int
	MaxHeaderBytes      int
}

// ThunderConfig holds Thunder admin API configuration for provisioning OAuth apps
type ThunderConfig struct {
	// BaseURL is the Thunder API base URL (if empty, provisioner uses static defaults).
	// Must be Thunder's public/issuer URL: it also derives the System resource server
	// identifier (RFC 8707) admin API tokens are scoped to — see
	// thundersvc.systemResourceIdentifier. It does NOT need to be directly dialable
	// from this process; see ResolveToHost below.
	BaseURL string
	// ClientID is the OAuth2 client ID of the system app (with Administrator role)
	ClientID string
	// ClientSecret is the OAuth2 client secret of the system app
	ClientSecret string `json:"-"`
	// ResolveToHost, if set, is the host:port this process actually dials for every
	// Thunder request, while requests still carry BaseURL's host as the HTTP Host
	// header and BaseURL still derives the System resource server identifier. Set
	// this when BaseURL isn't directly dialable from here — e.g. agent-manager-service
	// running in-cluster while Thunder's public URL only resolves via the host
	// machine's own DNS/hosts setup (typically the in-cluster Thunder service's own
	// cluster-DNS address). Leave empty when BaseURL is already directly dialable.
	ResolveToHost string
}

// WebSocketConfig holds WebSocket-specific configuration
type WebSocketConfig struct {
	MaxConnections    int // Maximum number of concurrent WebSocket connections (default: 1000)
	ConnectionTimeout int // Connection timeout in seconds (default: 30)
	RateLimitPerMin   int // Rate limit per gateway per minute (default: 10)
}

// ResourceLimitsConfig holds the operator-configured upper bounds for agent resource configs.
// All user-submitted values are validated against these limits and rejected with 400 if exceeded.
type ResourceLimitsConfig struct {
	// MaxReplicas is the maximum replica count (static and autoscaling maxReplicas)
	MaxReplicas int
	// MaxCPU is the maximum CPU value (Kubernetes quantity string) applied to both requests and limits
	MaxCPU string
	// MaxMemory is the maximum memory value (Kubernetes quantity string) applied to both requests and limits
	MaxMemory string
}
