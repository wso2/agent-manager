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
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/api"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/server"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"

	"go.uber.org/automaxprocs/maxprocs"

	dbmigrations "github.com/wso2/ai-agent-management-platform/agent-manager-service/db_migrations"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/signals"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/wiring"
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
	logger := slog.New(handler)
	slog.SetDefault(logger)

	slog.Info("Logger configured",
		"level", level.String())
}

func main() {
	cfg := config.GetConfig()

	setupLogger(cfg)

	if config.GetConfig().AutoMaxProcsEnabled {
		if _, err := maxprocs.Set(maxprocs.Logger(func(format string, args ...interface{}) {
			// Convert printf-style format string to plain message for structured logging
			slog.Info(fmt.Sprintf(format, args...))
		})); err != nil {
			slog.Error("Failed to set maxprocs", "error", err)
			os.Exit(1)
		}
	}
	serverFlag := flag.Bool("server", true, "start the http Server")
	migrateFlag := flag.Bool("migrate", false, "migrate the database")

	flag.Parse()

	if *migrateFlag {
		if err := dbmigrations.Migrate(); err != nil {
			slog.Error("error occurred while migrating", "error", err)
			os.Exit(1)
		}
	}

	if !*serverFlag {
		return
	}
	// Get the raw DB instance without context - repositories will add context per-operation
	db := db.GetDB()
	dependencies, err := wiring.InitializeAppParams(cfg, db)
	if err != nil {
		slog.Error("failed to initialize app dependencies", "error", err)
		os.Exit(1)
	}

	// Start monitor scheduler with background context
	schedulerCtx, schedulerCancel := context.WithCancel(context.Background())
	if err := dependencies.MonitorScheduler.Start(schedulerCtx); err != nil {
		slog.Error("failed to start monitor scheduler", "error", err)
		os.Exit(1)
	}

	// Load and seed LLM provider templates for all organizations
	if err := loadAndSeedLLMTemplates(cfg, dependencies); err != nil {
		slog.Warn("Failed to load and seed LLM provider templates", "error", err)
		// Don't exit on error - templates can be created manually
	}

	// Create main API server handler
	handler := api.MakeHTTPHandler(dependencies)
	mainServer := &http.Server{
		Addr:           fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort),
		Handler:        handler,
		ReadTimeout:    time.Duration(cfg.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:   time.Duration(cfg.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:    time.Duration(cfg.IdleTimeoutSeconds) * time.Second,
		MaxHeaderBytes: cfg.MaxHeaderBytes,
	}

	// Create internal HTTPS server for WebSocket and gateway internal APIs
	internalHandler := api.MakeInternalHTTPHandler(dependencies)
	internalServer := server.NewInternalServer(&cfg.InternalServer, internalHandler)

	stopCh := signals.SetupSignalHandler()

	// Setup graceful shutdown
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		<-stopCh
		slog.Info("Shutdown signal received, stopping services...")
		// Stop scheduler first
		schedulerCancel()
		if err := dependencies.MonitorScheduler.Stop(); err != nil {
			slog.Error("error stopping monitor scheduler", "error", err)
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Shutdown WebSocket manager
		if dependencies.WebSocketManager != nil {
			slog.Info("Shutting down WebSocket manager")
			dependencies.WebSocketManager.Shutdown()
		}

		// Shutdown main server
		mainCtx, mainCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer mainCancel()
		if err := mainServer.Shutdown(mainCtx); err != nil {
			slog.Error("Main server forced shutdown after timeout", "error", err)
		}

		// Shutdown internal server
		internalCtx, internalCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer internalCancel()
		if err := internalServer.Shutdown(internalCtx); err != nil {
			slog.Error("Internal server forced shutdown after timeout", "error", err)
		}
		wg.Done()
	}()

	// Start internal server in a goroutine
	go func() {
		slog.Info("Internal HTTPS server is running",
			"address", fmt.Sprintf("https://localhost:%d", cfg.InternalServer.Port),
			"maxWebSocketConnections", cfg.WebSocket.MaxConnections,
			"heartbeatTimeout", fmt.Sprintf("%ds", cfg.WebSocket.ConnectionTimeout),
			"rateLimitPerMin", cfg.WebSocket.RateLimitPerMin)
		if err := internalServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Failed to start internal server", "error", err)
			os.Exit(1)
		}
	}()

	// Start main server (blocking)
	slog.Info("Main API server is running", "address", mainServer.Addr)
	if err := mainServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("Failed to start main server", "error", err)
		os.Exit(1)
	}

	// Wait for graceful shutdown to complete
	wg.Wait()
	slog.Info("All servers shut down successfully")
}

// loadAndSeedLLMTemplates loads template files and seeds them for all organizations
func loadAndSeedLLMTemplates(cfg *config.Config, dependencies *wiring.AppParams) error {
	// Load default templates from directory
	templatePath := strings.TrimSpace(cfg.LLMTemplateDefinitionsPath)
	defaultTemplates, err := utils.LoadLLMProviderTemplatesFromDirectory(templatePath)
	if err != nil {
		cleanPath := filepath.Clean(templatePath)
		fallbackPath := ""
		if cleanPath != "" && cleanPath != "." && cleanPath != "src" && !filepath.IsAbs(cleanPath) && !strings.HasPrefix(cleanPath, "src"+string(os.PathSeparator)) {
			fallbackPath = filepath.Join("src", cleanPath)
		}
		if fallbackPath != "" {
			if templates, fallbackErr := utils.LoadLLMProviderTemplatesFromDirectory(fallbackPath); fallbackErr == nil {
				defaultTemplates = templates
				templatePath = fallbackPath
				err = nil
			} else {
				slog.Warn("Failed to load default LLM provider templates from fallback path", "path", fallbackPath, "error", fallbackErr)
			}
		}
		if err != nil {
			return fmt.Errorf("failed to load LLM provider templates from %s: %w", templatePath, err)
		}
	}

	if len(defaultTemplates) == 0 {
		slog.Info("No LLM provider templates found to seed")
		return nil
	}

	slog.Info("Loaded LLM provider templates", "count", len(defaultTemplates), "path", templatePath)

	// Set templates in the seeder
	dependencies.LLMTemplateSeeder.SetTemplates(defaultTemplates)

	// Seed templates for all organizations
	const pageSize = 200
	offset := 0
	seededCount := 0

	for {
		orgs, err := dependencies.OrganizationRepository.ListOrganizations(pageSize, offset)
		if err != nil {
			return fmt.Errorf("failed to list organizations for LLM template seeding: %w", err)
		}
		if len(orgs) == 0 {
			break
		}

		for _, org := range orgs {
			if org == nil || org.UUID == uuid.Nil {
				continue
			}
			if err := dependencies.LLMTemplateSeeder.SeedForOrg(org.UUID); err != nil {
				slog.Warn("Failed to seed LLM templates for organization", "orgUUID", org.UUID, "error", err)
			} else {
				seededCount++
			}
		}
		offset += pageSize
	}

	slog.Info("Seeded LLM provider templates", "templateCount", len(defaultTemplates), "organizationCount", seededCount)
	return nil
}
