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

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/wso2/ai-agent-management-platform/traces-observer-service/config"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/controllers"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/handlers"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/middleware"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/observer"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/opensearch"
)

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

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Setup structured logging
	setupLogger(cfg)

	slog.Info("Starting tracing service", "port", cfg.Server.Port)

	// Initialize OpenSearch client
	osClient, err := opensearch.NewClient(&cfg.OpenSearch)
	if err != nil {
		slog.Error("Failed to create OpenSearch client", "error", err)
		os.Exit(1)
	}

	// Initialize service
	tracingController := controllers.NewTracingController(osClient)

	// Initialize handlers
	handler := handlers.NewHandler(tracingController)

	// Setup routes
	mux := http.NewServeMux()

	// Health check - no authentication required
	mux.HandleFunc("/health", handler.Health)

	// Authenticated API routes
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/v1/traces", handler.GetTraceOverviews)
	apiMux.HandleFunc("/api/v1/traces/export", handler.ExportTraces)
	apiMux.HandleFunc("/api/v1/trace", handler.GetTraceById)

	// v2 routes — observer-backed; only registered when OBSERVER_BASE_URL is set.
	if cfg.Observer.BaseURL != "" {
		authProvider := observer.NewAuthProvider(
			cfg.Observer.TokenURL,
			cfg.Observer.ClientID,
			cfg.Observer.ClientSecret,
		)
		observerClient := observer.NewClient(cfg.Observer.BaseURL, authProvider)
		v2Controller := controllers.NewV2TracingController(observerClient)
		v2Handler := handlers.NewV2Handler(v2Controller)

		apiMux.HandleFunc("/api/v2/traces", v2Handler.GetTraceOverviews)
		apiMux.HandleFunc("/api/v2/traces/", func(w http.ResponseWriter, r *http.Request) {
			// Route /api/v2/traces/{traceId}/spans and /api/v2/traces/{traceId}/spans/{spanId}
			if isSpanDetailPath(r.URL.Path) {
				v2Handler.GetSpanDetail(w, r)
			} else {
				v2Handler.GetTraceSpans(w, r)
			}
		})

		slog.Info("v2 observer-backed routes registered", "observerBaseURL", cfg.Observer.BaseURL)
	} else {
		// Register stub handlers that return 503 so clients get a clear message.
		unavailable := func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":"observer not configured"}`, http.StatusServiceUnavailable)
		}
		apiMux.HandleFunc("/api/v2/", unavailable)
		slog.Info("v2 routes disabled: OBSERVER_BASE_URL not set")
	}

	// Apply JWT auth middleware to API routes
	authenticatedHandler := middleware.JWTAuth(cfg.Auth)(apiMux)
	mux.Handle("/api/v1/", authenticatedHandler)
	mux.Handle("/api/v2/", authenticatedHandler)

	// Apply middleware: Request Logger -> CORS
	corsConfig := middleware.DefaultCORSConfig()
	corsHandler := middleware.CORS(corsConfig)(mux)
	loggerHandler := logger.RequestLogger()(corsHandler)

	// Create server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      loggerHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		slog.Info("Server listening", "port", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("Server exited")
}

// isSpanDetailPath returns true for /api/v2/traces/{traceId}/spans/{spanId}
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
