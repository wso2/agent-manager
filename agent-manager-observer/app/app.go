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
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/handlers"
	"github.com/wso2/agent-manager/agent-manager-observer/mcp"
	"github.com/wso2/agent-manager/agent-manager-observer/middleware"
	"github.com/wso2/agent-manager/agent-manager-observer/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/rbac"
)

// Options holds deployment-specific hooks for Run; the zero value is the OSS default.
type Options struct {
	// Client replaces the default OpenChoreo-backed observer client.
	Client observer.Client
	// Ingest, when set, is served on IngestAddr alongside the API server.
	Ingest     http.Handler
	IngestAddr string
	// Close releases resources owned by the hooks, after both servers stop.
	Close func() error
}

// Run serves the observer API until SIGINT or SIGTERM, then shuts down gracefully.
func Run(cfg *config.Config, tokenProvider observer.TokenProvider, opts Options) (err error) {
	setupLogger(cfg)
	if opts.Close != nil {
		defer func() {
			if closeErr := opts.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("closing observer store: %w", closeErr))
			}
		}()
	}

	client := opts.Client
	if client == nil {
		client = observer.NewClient(cfg.Observer.BaseURL, tokenProvider, cfg.Observer.DefaultNamespace)
	}

	slog.Info("Starting agent-manager-observer", "port", cfg.Server.Port)

	// Create server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      newHandler(cfg, client),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start server in a goroutine
	serveErr := make(chan error, 2)
	serve := func(s *http.Server) {
		slog.Info("Server listening", "addr", s.Addr)
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}
	go serve(server)

	servers := []*http.Server{server}
	if opts.Ingest != nil {
		ingestServer := &http.Server{
			Addr:        opts.IngestAddr,
			Handler:     opts.Ingest,
			ReadTimeout: 30 * time.Second,
		}
		servers = append(servers, ingestServer)
		go serve(ingestServer)
	}

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

	for _, s := range servers {
		if err := s.Shutdown(ctx); err != nil {
			return fmt.Errorf("server forced to shutdown: %w", err)
		}
	}

	slog.Info("Server exited")
	return nil
}

func newHandler(cfg *config.Config, observerClient observer.Client) http.Handler {
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
	controller := controllers.NewTracingController(observerClient)
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

	return loggerHandler
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
