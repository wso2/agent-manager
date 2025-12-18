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

// Config holds all configuration for the application
type Config struct {
	ServerHost          string
	ServerPort          int
	AuthHeader          string
	AutoMaxProcsEnabled bool
	LogLevel            string
	POSTGRESQL          POSTGRESQL
	KubeConfig          string
	// HTTP Server timeout configurations
	ReadTimeoutSeconds  int
	WriteTimeoutSeconds int
	IdleTimeoutSeconds  int
	MaxHeaderBytes      int
	// Database operation timeout configuration
	DbOperationTimeoutSeconds int
	HealthCheckTimeoutSeconds int

	APIKeyHeader string
	APIKeyValue  string
	// CORSAllowedOrigin is the single allowed origin for CORS; use "*" to allow all
	CORSAllowedOrigin string

	// OpenTelemetry configuration
	OTEL OTELConfig

	// Observer service configuration (for build logs, etc.)
	Observer ObserverConfig

	// Trace Observer service configuration (for distributed tracing)
	TraceObserver TraceObserverConfig

	IsLocalDevEnv bool

	// Default Chat API configuration
	DefaultChatAPI     DefaultChatAPIConfig
	DefaultGatewayPort int
}

// OTELConfig holds all OpenTelemetry related configuration
type OTELConfig struct {
	// Instrumentation configuration
	OTELInstrumentationImage
	SDKVolumeName string
	SDKMountPath  string

	// Tracing configuration
	IsTraceContentEnabled bool

	// OTLP Exporter configuration
	ExporterEndpoint string
}

type OTELInstrumentationImage struct {
	Python310 string
	Python311 string
	Python312 string
	Python313 string
}

type ObserverConfig struct {
	// Observer service URL
	URL      string
	Username string
	Password string `json:"-"`
}

type TraceObserverConfig struct {
	// Trace Observer service URL
	URL string
}

type POSTGRESQL struct {
	Host     string
	Port     int
	User     string
	DBName   string
	Password string `json:"-"`
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
