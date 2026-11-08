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

// Package app is the observer startup shared by open-source and cloud deployments.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/clickhouse"
	"github.com/wso2/agent-manager/agent-manager-observer/config"
	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/handlers"
	"github.com/wso2/agent-manager/agent-manager-observer/mcp"
	"github.com/wso2/agent-manager/agent-manager-observer/middleware"
	"github.com/wso2/agent-manager/agent-manager-observer/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearchreader"
	"github.com/wso2/agent-manager/agent-manager-observer/rbac"
	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

// Options holds deployment-specific hooks for Run; the zero value is the OSS default.
type Options struct{}

// Run serves the observer API until SIGINT or SIGTERM, then shuts down gracefully.
func Run(cfg *config.Config, tokenProvider observer.TokenProvider, _ Options) error {
	setupLogger(cfg)

	slog.Info("Starting agent-manager-observer", "port", cfg.Server.Port)

	handler, cleanup, err := newHandler(cfg, tokenProvider)

	if err != nil {
		return err
	}

	defer cleanup()

	// Create server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start server in a goroutine
	serveErr := make(chan error, 1)
	go func() {
		slog.Info("Server listening", "port", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)
	select {
	case err := <-serveErr:
		return fmt.Errorf("server failed: %w", err)
	case <-quit:
	}

	slog.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	slog.Info("Server exited")
	return nil
}

func newTraceReader(cfg *config.Config, openChoreo observer.Client) (observer.TraceReader, error) {
	switch cfg.Traces.Backend {
	case "", config.TraceBackendOpenChoreo:
		slog.Info("trace backend selected", "backend", config.TraceBackendOpenChoreo)
		return openChoreo, nil
	case config.TraceBackendClickHouse:
		db, err := clickhouse.Open(cfg.Traces)
		if err != nil {
			return nil, fmt.Errorf("failed to open clickhouse: %w", err)
		}

		resolver, err := clickhouse.NewStaticResolverFromJSON(cfg.Traces.ScopeUIDs)

		if err != nil {
			_ = db.Close()
			return nil, err
		}

		reader, err := clickhouse.NewTraceReader(db, clickhouse.Options{
			Table: cfg.Traces.Table, Layout: cfg.Traces.Layout,
			DefaultNamespace: cfg.Observer.DefaultNamespace, Resolver: resolver,
		})

		if err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to create clickhouse trace reader: %w", err)
		}
		slog.Info("trace backend selected", "backend", config.TraceBackendClickHouse)
		return reader, nil
	case config.TraceBackendOpenSearch:
		resolver, err := scope.NewStaticResolverFromJSON(cfg.Traces.ScopeUIDs)
		if err != nil {
			return nil, err
		}
		reader, err := opensearchreader.NewTraceReader(opensearchreader.Options{
			Addr: cfg.Traces.OSAddr, Username: cfg.Traces.OSUser, Password: cfg.Traces.OSPassword,
			IndexPattern: cfg.Traces.OSIndex, InsecureTLS: cfg.Traces.OSInsecureTLS,
			DefaultNamespace: cfg.Observer.DefaultNamespace, Resolver: resolver,
		})
		if err != nil {
			return nil, fmt.Errorf("create opensearch trace reader: %w", err)
		}
		slog.Info("trace backend selected", "backend", config.TraceBackendOpenSearch)
		return reader, nil
	default:
		return nil, fmt.Errorf("unknown TRACE_BACKEND %q", cfg.Traces.Backend)
	}
}

func newHandler(cfg *config.Config, tokenProvider observer.TokenProvider) (http.Handler, func(), error) {
	// Setup routes
	mux := http.NewServeMux()

	// Health check - no authentication required
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"healthy","timestamp":"%s"}`, time.Now().Format(time.RFC3339))
	})

	// OAuth 2.0 protected resource metadata (RFC 9728) - no authentication
	// required; consumed by MCP clients to discover the authorization server.
	handlers.RegisterWellKnownRoutes(mux, cfg.Auth)

	// Authenticated API routes
	apiMux := http.NewServeMux()

	// v1 routes — observer-backed
	observerClient := observer.NewClient(cfg.Observer.BaseURL, tokenProvider, cfg.Observer.DefaultNamespace)
	traceReader, err := newTraceReader(cfg, observerClient)

	if err != nil {
		return nil, nil, fmt.Errorf("select trace backend: %w", err)
	}
	cleanup := func() {}
	if closer, ok := traceReader.(io.Closer); ok {
		cleanup = func() {
			if err := closer.Close(); err != nil {
				slog.Error("failed to close trace reader", "error", err)
			}
		}
	}

	controller := controllers.NewTracingController(traceReader)
	controller.SetBatching(cfg.Traces.Batching)
	
	if cfg.Traces.Batching && !controller.CanBatch() {
		slog.Warn("TRACE_BATCHING is on, but this trace backend cannot batch; using per-trace calls")
	}
	slog.Info("trace batching", "enabled", cfg.Traces.Batching && controller.CanBatch())

	obsController := controllers.NewObservabilityController(observerClient)
	handler := handlers.NewHandler(controller, obsController)

	requireTrace := middleware.RequirePermission(rbac.TraceRead)
	requireLog := middleware.RequirePermission(rbac.LogRead)
	requireBuildLog := middleware.RequirePermission(rbac.BuildLogRead)
	requireMetric := middleware.RequirePermission(rbac.MetricRead)

	apiMux.Handle("/api/v1/traces", requireTrace(http.HandlerFunc(handler.GetTraceOverviews)))
	apiMux.Handle("/api/v1/traces/export", requireTrace(http.HandlerFunc(handler.ExportTraces)))
	apiMux.Handle("/api/v1/traces/", requireTrace(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route /api/v1/traces/{traceId}/spans and /api/v1/traces/{traceId}/spans/{spanId}
		if isSpanDetailPath(r.URL.Path) {
			handler.GetSpanDetail(w, r)
		} else {
			handler.GetTraceSpans(w, r)
		}
	})))

	// Data routes carry per-route scope requirements. Publisher-audience
	// tokens are confined to trace-read by RequirePermission (replacing the
	// former RejectPublisherAudience guard).
	apiMux.Handle("/api/v1/logs", requireLog(http.HandlerFunc(handler.GetLogs)))
	apiMux.Handle("/api/v1/build-logs", requireBuildLog(http.HandlerFunc(handler.GetBuildLogs)))
	apiMux.Handle("/api/v1/metrics", requireMetric(http.HandlerFunc(handler.GetMetrics)))

	slog.Info("v1 observer-backed routes registered", "observerBaseURL", cfg.Observer.BaseURL)

	// Apply JWT auth middleware to API routes
	authenticatedHandler := middleware.JWTAuth(cfg.Auth)(apiMux)
	mux.Handle("/api/v1/", authenticatedHandler)

	// am-obs-mcp: streamable-HTTP MCP server on the root mux (not under
	// /api/v1/). Behind the same JWTAuth middleware, with per-tool guards
	// applying the same scope policy as the REST routes: each tool requires
	// its amp:observability:* scope, and publisher-audience tokens are confined
	// to their implicit trace-read permission.
	mcp.RegisterRoute(mux, mcp.Dependencies{
		Tracing:       controller,
		Observability: obsController,
	}, middleware.JWTAuthForMCP(cfg.Auth))
	slog.Info("am-obs-mcp registered", "path", "/mcp")

	// Apply middleware: Request Logger -> CORS
	corsConfig := middleware.DefaultCORSConfig()
	corsHandler := middleware.CORS(corsConfig)(mux)
	loggerHandler := logger.RequestLogger()(corsHandler)

	return loggerHandler, cleanup, nil
}

func setupLogger(cfg *config.Config) {
	var level slog.Level
	switch cfg.LogLevel {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo // default to INFO
	}

	// Create handler options
	opts := &slog.HandlerOptions{
		Level: level,
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	slogger := slog.New(handler)
	slog.SetDefault(slogger)

	slog.Info("Logger configured",
		"level", level.String())
}

// isSpanDetailPath returns true for /api/v1/traces/{traceId}/spans/{spanId}
// (i.e. the path has a non-empty segment after "/spans/").
func isSpanDetailPath(path string) bool {
	const spansSlash = "/spans/"
	idx := strings.LastIndex(path, spansSlash)
	if idx < 0 {
		return false
	}
	tail := path[idx+len(spansSlash):]
	return tail != ""
}
