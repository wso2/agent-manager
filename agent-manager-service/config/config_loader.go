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
	"log/slog"
	"os"

	"github.com/joho/godotenv"
)

var config *Config

func GetConfig() *Config {
	return config
}

func init() {
	loadEnvs()
}

func loadEnvs() {
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

	config.CORS = CORSConfig{
		AllowOrigin:  r.readOptionalString("CORS_ALLOWED_ORIGIN", "http://localhost:3000"),
		AllowMethods: r.readOptionalString("CORS_ALLOWED_METHODS", "GET,POST,PUT,DELETE,PATCH,OPTIONS"),
		AllowHeaders: r.readOptionalString("CORS_ALLOWED_HEADERS", "authorization,Content-Type,Origin"),
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
	config.KubeConfig = r.readOptionalString("KUBECONFIG", "")

	// HTTP Server timeout configurations
	config.ReadTimeoutSeconds = int(r.readOptionalInt64("HTTP_READ_TIMEOUT_SECONDS", 10))
	config.WriteTimeoutSeconds = int(r.readOptionalInt64("HTTP_WRITE_TIMEOUT_SECONDS", 30))
	config.IdleTimeoutSeconds = int(r.readOptionalInt64("HTTP_IDLE_TIMEOUT_SECONDS", 60))
	config.MaxHeaderBytes = int(r.readOptionalInt64("HTTP_MAX_HEADER_BYTES", 65536)) // 1024 * 64

	// Database operation timeout configuration
	config.DbOperationTimeoutSeconds = int(r.readOptionalInt64("DB_OPERATION_TIMEOUT_SECONDS", 10))
	config.HealthCheckTimeoutSeconds = int(r.readOptionalInt64("HEALTH_CHECK_TIMEOUT_SECONDS", 5))

	config.DefaultChatAPI = DefaultChatAPIConfig{
		DefaultHTTPPort: int32(r.readOptionalInt64("DEFAULT_CHAT_API_HTTP_PORT", 8000)),
		DefaultBasePath: r.readOptionalString("DEFAULT_CHAT_API_BASE_PATH", "/"),
	}

	config.APIKeyHeader = r.readOptionalString("API_KEY_HEADER", "X-API-KEY")
	config.APIKeyValue = r.readRequiredString("API_KEY_VALUE")

	// OpenTelemetry configuration
	// Use Version from ldflags or environment variable override
	config.PackageVersion = r.readOptionalString("AMP_VERSION", Version)

	config.OTEL = OTELConfig{
		SDKVolumeName: r.readOptionalString("OTEL_SDK_VOLUME_NAME", "otel-tracing-sdk-volume"),
		SDKMountPath:  r.readOptionalString("OTEL_SDK_MOUNT_PATH", "/otel-tracing-sdk"),

		// Tracing configuration
		IsTraceContentEnabled: r.readOptionalBool("OTEL_TRACELOOP_TRACE_CONTENT", true),

		// OTLP Exporter configuration
		ExporterEndpoint: r.readOptionalString("OTEL_EXPORTER_OTLP_ENDPOINT", "http://opentelemetry-collector.openchoreo-observability-plane.svc.cluster.local:4318"),
	}

	// Observer service configuration - temporarily use localhost for agent-manager-service to access observer service
	config.Observer = ObserverConfig{
		URL:      r.readOptionalString("OBSERVER_URL", "http://localhost:8085"),
		Username: r.readOptionalString("OBSERVER_USERNAME", "dummy"),
		Password: r.readOptionalString("OBSERVER_PASSWORD", "dummy"),
	}

	// Trace Observer service configuration - for distributed tracing
	config.TraceObserver = TraceObserverConfig{
		URL: r.readOptionalString("TRACE_OBSERVER_URL", "http://localhost:9098"),
	}

	config.IsLocalDevEnv = r.readOptionalBool("IS_LOCAL_DEV_ENV", false)
	config.DefaultGatewayPort = int(r.readOptionalInt64("DEFAULT_GATEWAY_PORT", 9080))

	// Validate HTTP server configurations
	validateHTTPServerConfigs(config, r)

	r.logAndExitIfErrorsFound()

	slog.Info("configReader: configs loaded")
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
