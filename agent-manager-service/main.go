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
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/api"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"

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
	dependencies, err := wiring.InitializeAppParams(cfg)
	if err != nil {
		slog.Error("failed to initialize app dependencies", "error", err)
		os.Exit(1)
	}

	handler := api.MakeHTTPHandler(dependencies)
	server := &http.Server{
		Addr:           fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort),
		Handler:        handler,
		ReadTimeout:    time.Duration(cfg.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:   time.Duration(cfg.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:    time.Duration(cfg.IdleTimeoutSeconds) * time.Second,
		MaxHeaderBytes: cfg.MaxHeaderBytes,
	}

	stopCh := signals.SetupSignalHandler()

	go func() {
		<-stopCh
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			slog.Error("forced shutdown after timeout", "error", err)
		}
	}()

	slog.Info("agent-manager-service is running", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}
}
