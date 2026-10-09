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

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/signals"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// runDispatcher runs the "dispatcher" mode: the webhook dispatcher, consuming
// events from NATS, and the internal webhook API the "api" instance stores
// endpoints through. It uses its own database (DB_* point at it) and needs
// none of the API's dependencies.
func runDispatcher(cfg *config.Config, opts Options) {
	if opts.Migrate {
		if err := dbmigrations.MigrateDispatcher(); err != nil {
			slog.Error("error occurred while migrating the dispatcher database", "error", err)
			os.Exit(1)
		}
	}
	if !opts.Server {
		return
	}

	logger := slog.Default()
	encryptionKey, err := wiring.ProvideEncryptionKey(*cfg)
	if err != nil {
		slog.Error("failed to load encryption key", "error", err)
		os.Exit(1)
	}
	bus, err := events.Connect(cfg.Events.NATSURL, "agent-manager-webhook-dispatcher", logger)
	if err != nil {
		slog.Error("failed to connect to NATS", "error", err)
		os.Exit(1)
	}

	repo := repositories.NewWebhookRepository(db.GetDB())
	sender := services.NewWebhookSender(*cfg)
	store := services.NewLocalWebhookStore(repo, sender, encryptionKey, logger)
	dispatcher := services.NewWebhookDispatcherService(bus, repo, sender, encryptionKey, *cfg, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/internal/", services.NewWebhookInternalAPI(store, cfg.Webhooks.DispatcherAPIKey, logger))
	server := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.Webhooks.DispatcherPort),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		// A test send waits on the endpoint.
		WriteTimeout: 30 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			setupCtx, setupCancel := context.WithTimeout(ctx, 10*time.Second)
			err := bus.Setup(setupCtx)
			setupCancel()
			if err == nil {
				break
			}
			slog.Warn("Event bus not ready; retrying", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
		if err := dispatcher.Start(ctx); err != nil {
			slog.Error("failed to start webhook dispatcher", "error", err)
			os.Exit(1)
		}
	}()

	stopCh := signals.SetupSignalHandler()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-stopCh
		slog.Info("Shutdown signal received, stopping the webhook dispatcher...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("dispatcher API forced shutdown", "error", err)
		}
		cancel()
		dispatcher.Stop()
		bus.Close()
	}()

	slog.Info("Webhook dispatcher is running", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("dispatcher API failed", "error", err)
		os.Exit(1)
	}
	// ListenAndServe returns as soon as shutdown begins; wait for in-flight
	// deliveries to finish and the bus to close.
	<-stopped
}
