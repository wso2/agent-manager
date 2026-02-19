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

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wso2/ai-agent-management-platform/secret-management-service/config"
	"github.com/wso2/ai-agent-management-platform/secret-management-service/controllers"
	"github.com/wso2/ai-agent-management-platform/secret-management-service/providers"
	"github.com/wso2/ai-agent-management-platform/secret-management-service/services"

	// Register providers
	_ "github.com/wso2/ai-agent-management-platform/secret-management-service/providers/openbao"
)

func main() {
	// Initialize logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Create KV provider
	kvProvider, err := providers.NewProvider(providers.ProviderType(cfg.KVProvider.Type), providers.Config{
		Address:   cfg.KVProvider.Address,
		Token:     cfg.KVProvider.Token,
		Namespace: cfg.KVProvider.Namespace,
		MountPath: cfg.KVProvider.MountPath,
	})
	if err != nil {
		slog.Error("Failed to create KV provider", "error", err)
		os.Exit(1)
	}

	// Health check KV provider
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := kvProvider.HealthCheck(ctx); err != nil {
		slog.Error("KV provider health check failed", "error", err)
		os.Exit(1)
	}
	slog.Info("KV provider connected successfully", "type", cfg.KVProvider.Type)

	// Create services
	secretService := services.NewSecretService(kvProvider, logger)

	// Create controllers
	secretController := controllers.NewSecretController(secretService, logger)

	// Setup HTTP routes
	mux := http.NewServeMux()
	secretController.RegisterRoutes(mux)

	// Health endpoint
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	// Create server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		slog.Info("Starting secret-management-service", "address", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down server...")
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	slog.Info("Server stopped")
}
