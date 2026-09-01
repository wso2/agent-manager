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

// Package app provides shared application startup logic that can be used by both
// open-source and cloud deployments. The Run function handles all common setup
// including configuration, logging, database connections, and server lifecycle,
// while accepting an injectable AuthProvider for deployment-specific authentication.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/api"
	"github.com/wso2/agent-manager/agent-manager-service/audit"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
	"github.com/wso2/agent-manager/agent-manager-service/resources"
	"github.com/wso2/agent-manager/agent-manager-service/server"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/signals"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"

	"go.uber.org/automaxprocs/maxprocs"
	"gorm.io/gorm"
)

// Options holds the configuration options for running the application.
// These are typically parsed from command-line flags in main.
type Options struct {
	// Server indicates whether to start the HTTP server
	Server bool
	// Migrate indicates whether to run database migrations before starting
	Migrate bool
	// ExtraAPIRoutes registers additional routes onto the authenticated /api/v1 sub-mux.
	// Use this to inject deployment-specific routes without modifying the core handler.
	ExtraAPIRoutes func(mux *http.ServeMux, params *wiring.AppParams)
	// GatewayConfigApplier patches the gateway runtime config when an identity provider
	// changes, keeping the gateway and the AMS mirror in sync. nil (the open-source
	// default) preserves the script-driven behavior; cloud deployments inject an
	// implementation. See services.GatewayConfigApplier.
	GatewayConfigApplier services.GatewayConfigApplier
	// AgentThunderProvisioning is the deployment-specific AgentID provisioning
	// implementation. A factory because the open-source impl is DB-backed and the
	// DB is initialized inside Run. nil disables provisioning (identity endpoints
	// report it unavailable, reconciler not started, no secret backend required).
	// secretMgmtClient is built by Run itself (see the call site) and handed in —
	// the same secret backend seam LLM/MCP/publisher secrets use (for per-agent
	// credentials). encryptionKey is the platform ENCRYPTION_KEY, used to decrypt
	// the env-Thunder system-client credential from AMS's own Postgres — that one
	// is not read back from a key vault.
	AgentThunderProvisioning func(db *gorm.DB, secretMgmtClient secretmanagersvc.SecretManagementClient, ocClient occlient.OpenChoreoClient, encryptionKey []byte) services.AgentThunderProvisioningService
	// BuildSecretProvisioner provisions the per-run git clone secret before a source
	// build's WorkflowRun is created. nil (the open-source default) is a no-op: private
	// repos are cloned via the PAT-backed git secret the user created (or public repos
	// anonymously). A deployment can inject an implementation that mints a short-lived
	// token from a platform GitHub App. See services.BuildSecretProvisioner.
	BuildSecretProvisioner services.BuildSecretProvisioner
	// RepositoryCommitProvider optionally resolves commit history from a
	// deployment-specific component source binding. nil preserves the standard
	// anonymous/static-token and PAT-backed repository behavior.
	RepositoryCommitProvider services.RepositoryCommitProvider
}

// Run starts the application with the provided providers and options.
// This is the main entry point that both open-source and cloud main.go will call.
// The authProvider parameter allows different deployments to inject their own
// authentication mechanism (e.g., OAuth2 for open-source, workload identity for cloud).
// The secretProvider parameter allows different deployments to inject their own
// secret management backend (e.g., the OpenChoreo secret API for open-source, cloud-specific for cloud).
func Run(authProvider occlient.AuthProvider, secretProvider secretmanagersvc.Provider, opts Options) {
	cfg := config.GetConfig()

	setupLogger(cfg)

	if cfg.AutoMaxProcsEnabled {
		if _, err := maxprocs.Set(maxprocs.Logger(func(format string, args ...any) {
			// Convert printf-style format string to plain message for structured logging
			slog.Info(fmt.Sprintf(format, args...))
		})); err != nil {
			slog.Error("Failed to set maxprocs", "error", err)
			os.Exit(1)
		}
	}

	if opts.Migrate {
		if err := dbmigrations.Migrate(); err != nil {
			slog.Error("error occurred while migrating", "error", err)
			os.Exit(1)
		}
	}

	if !opts.Server {
		return
	}

	// Get the raw DB instance without context - repositories will add context per-operation
	database := db.GetDB()

	// Select the gateway-manifest cache backend before anything reads or writes it —
	// see services.SetGatewayManifestCacheBackend's doc comment for why this is a
	// direct call rather than part of the wire dependency graph below.
	manifestCacheBackend, err := wiring.ProvideGatewayManifestCacheBackend(*cfg)
	if err != nil {
		slog.Error("failed to initialize gateway manifest cache backend", "error", err)
		os.Exit(1)
	}
	services.SetGatewayManifestCacheBackend(manifestCacheBackend)

	// Deployment-specific AgentID provisioning; nil disables it.
	var agentThunderProvisioning services.AgentThunderProvisioningService
	if opts.AgentThunderProvisioning != nil {
		// Built once here (not inside wiring.InitializeAppParams, which builds
		// its own separate instance below for every other secret-backed
		// service) because agentThunderProvisioning must exist before
		// InitializeAppParams runs — see SetWorkloadInjector's doc comment for
		// why. Both instances point at the same secret backend, so this
		// duplication is harmless (occlient/secretmanagersvc clients are
		// stateless wrappers, not connections).
		ocClientForProvisioning, err := wiring.ProvideOCClient(*cfg, authProvider)
		if err != nil {
			slog.Error("failed to create OpenChoreo client for AgentID provisioning", "error", err)
			os.Exit(1)
		}
		secretMgmtClientForProvisioning, err := wiring.ProvideSecretManagementClient(*cfg, secretProvider, ocClientForProvisioning)
		if err != nil {
			slog.Error("failed to create secret management client for AgentID provisioning", "error", err)
			os.Exit(1)
		}
		encryptionKey, err := wiring.ProvideEncryptionKey(*cfg)
		if err != nil {
			slog.Error("failed to load encryption key for AgentID provisioning", "error", err)
			os.Exit(1)
		}
		agentThunderProvisioning = opts.AgentThunderProvisioning(database, secretMgmtClientForProvisioning, ocClientForProvisioning, encryptionKey)
	}

	dependencies, err := wiring.InitializeAppParams(cfg, database, authProvider, secretProvider, opts.GatewayConfigApplier, agentThunderProvisioning, opts.BuildSecretProvisioner)
	if err != nil {
		slog.Error("failed to initialize app dependencies", "error", err)
		os.Exit(1)
	}

	// Backfill the real AgentIdentityInjectionService into agentThunderProvisioning
	// now that it exists (agentThunderProvisioning is built above, before
	// InitializeAppParams constructs the OpenChoreo client this service depends
	// on). Must happen before the reconciler and HTTP server start below — see
	// services.WorkloadInjectorSetter's doc comment for why that ordering makes
	// this race-free. Type-asserted rather than a method on
	// AgentThunderProvisioningService itself: an alternative deployment's
	// provisioning implementation that does no workload injection simply
	// doesn't implement this optional interface.
	if setter, ok := agentThunderProvisioning.(services.WorkloadInjectorSetter); ok {
		setter.SetWorkloadInjector(dependencies.AgentIdentityInjectionService)
	}
	dependencies.RepositoryService.SetCommitProvider(opts.RepositoryCommitProvider)

	// Background workers have no request behind them, so nothing installs a
	// recorder on their contexts the way the HTTP middleware does. Install one
	// here, or their audit emits would reach the "not installed" fallback and
	// be lost — silently, which is the failure mode a context-carried recorder
	// is most prone to.
	backgroundCtx := audit.WithRecorder(context.Background(), dependencies.AuditRecorder)

	// So a rotated agent's deferred pod rollout (see RefreshAfterRotation)
	// stops waiting, or aborts an in-flight roll, once shutdown starts below
	// instead of firing an outbound update afterward.
	agentIdentityRolloutCtx, agentIdentityRolloutCancel := context.WithCancel(backgroundCtx)
	if setter, ok := dependencies.AgentIdentityInjectionService.(services.AgentIdentityShutdownContextSetter); ok {
		setter.SetShutdownContext(agentIdentityRolloutCtx)
	}

	// Start monitor scheduler with background context
	schedulerCtx, schedulerCancel := context.WithCancel(backgroundCtx)
	if err := dependencies.MonitorScheduler.Start(schedulerCtx); err != nil {
		slog.Error("failed to start monitor scheduler", "error", err)
		os.Exit(1)
	}

	// Start the AgentID provisioning retry reconciler, but only when provisioning
	// is enabled — otherwise there is nothing to reconcile.
	agentThunderReconcilerCtx, agentThunderReconcilerCancel := context.WithCancel(backgroundCtx)
	if agentThunderProvisioning != nil {
		if err := dependencies.AgentThunderReconciler.Start(agentThunderReconcilerCtx); err != nil {
			slog.Error("failed to start agent thunder provisioning reconciler", "error", err)
			os.Exit(1)
		}
	}

	// Load built-in LLM provider templates into memory
	if err := loadBuiltInLLMTemplates(dependencies); err != nil {
		slog.Error("Failed to load built-in LLM provider templates", "error", err)
		// Don't exit - templates can still be created via API
	}

	recordStartupPosture(cfg, dependencies.AuditRecorder)

	// Create main API server handler
	handler := api.MakeHTTPHandler(dependencies, opts.ExtraAPIRoutes)
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

	wg.Go(func() {
		<-stopCh
		slog.Info("Shutdown signal received, stopping services...")

		// Abort any AgentID pod rollout still waiting or in flight first —
		// this is a plain context.CancelFunc, so there is no reason to run
		// it after any of the synchronous shutdown steps below, some of
		// which could otherwise delay it long enough for a still-waiting
		// rollout to fire in the meantime.
		agentIdentityRolloutCancel()

		// Single timeout context for the entire shutdown sequence
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		// Stop scheduler first
		schedulerCancel()
		if err := dependencies.MonitorScheduler.Stop(); err != nil {
			slog.Error("error stopping monitor scheduler", "error", err)
		}

		agentThunderReconcilerCancel()
		if agentThunderProvisioning != nil {
			if err := dependencies.AgentThunderReconciler.Stop(); err != nil {
				slog.Error("error stopping agent thunder provisioning reconciler", "error", err)
			}
		}

		// Shutdown WebSocket manager in a goroutine since it blocks
		wsDone := make(chan struct{})
		if dependencies.WebSocketManager != nil {
			go func() {
				slog.Info("Shutting down WebSocket manager")
				dependencies.WebSocketManager.Shutdown()
				close(wsDone)
			}()
		} else {
			close(wsDone)
		}

		// Wait for WebSocket shutdown or timeout
		select {
		case <-wsDone:
			slog.Info("WebSocket manager shutdown complete")
		case <-shutdownCtx.Done():
			slog.Warn("WebSocket manager shutdown timed out")
		}

		// Close EventHub after WebSocket manager so in-flight events are not dropped
		if dependencies.EventHub != nil {
			if err := dependencies.EventHub.Close(); err != nil {
				slog.Error("error closing EventHub", "error", err)
			}
		}

		// Shutdown both servers using the same context
		if err := mainServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("Main server forced shutdown after timeout", "error", err)
		}

		if err := internalServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("Internal server forced shutdown after timeout", "error", err)
		}

		// Flush the audit buffer last. Both servers have stopped, so no further
		// events can be produced and every in-flight request has finished
		// recording. Draining any earlier would discard the audit records of the
		// requests still being served — note this is deliberately not where
		// EventHub.Close sits, which runs before the servers stop.
		if dependencies.AuditRecorder != nil {
			if err := dependencies.AuditRecorder.Close(shutdownCtx); err != nil {
				slog.Error("error flushing audit recorder; some events may be lost", "error", err)
			}
		}
	})

	// Start internal server in a goroutine
	go func() {
		scheme := "https"
		if !cfg.InternalServer.TLSEnabled {
			scheme = "http"
		}
		slog.Info("Internal server is running",
			"address", fmt.Sprintf("%s://localhost:%d", scheme, cfg.InternalServer.Port),
			"tlsEnabled", cfg.InternalServer.TLSEnabled,
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

// recordStartupPosture writes the audit trail's own bookend events.
//
// The startup record bounds any gap in the trail to a restart: a reader can
// tell "nothing happened" apart from "the service was not running".
//
// The RBAC record matters more. Authorization enforcement is off by default, and
// when it is off every permission check returns early. Without an event saying
// so, that gap is visible only in a config file no auditor reads — and every
// request in the trail would imply a check that never happened.
func recordStartupPosture(cfg *config.Config, recorder audit.Recorder) {
	if recorder == nil {
		return
	}
	ctx := audit.WithRecorder(context.Background(), recorder)

	// rbacEnforced is set explicitly here. Outside a request there is no source
	// to carry it, so it would otherwise serialise as false — and false is not a
	// neutral default on this field: it is the assertion that no authorization
	// check happened. On an RBAC-enabled deployment the startup record, which is
	// the first thing a reader of the trail sees, then contradicts every request
	// record that follows it.
	audit.RecordAncillary(
		ctx, audit.ActionSystemStartup,
		audit.Actor(audit.ActorSystem, "system:agent-manager-service", ""),
		audit.SurfaceOpt(audit.SurfaceSystem),
		audit.RBACEnforced(cfg.RBACEnabled),
		audit.Detail("sinks", []string{"stdout"}),
	)

	if !cfg.RBACEnabled {
		audit.RecordAncillary(
			ctx, audit.ActionSystemRBACDisabled,
			audit.Actor(audit.ActorSystem, "system:agent-manager-service", ""),
			audit.SurfaceOpt(audit.SurfaceSystem),
			audit.RBACEnforced(false),
			audit.Detail("reason", "rbac-enabled-false"),
		)
		slog.Warn("RBAC is disabled; every authenticated request is allowed regardless of token scopes",
			"setting", "RBAC_ENABLED")
	}
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
	logger := slog.New(handler)
	slog.SetDefault(logger)

	slog.Info("Logger configured",
		"level", level.String())
}

// loadBuiltInLLMTemplates loads built-in LLM provider templates into in-memory store
func loadBuiltInLLMTemplates(dependencies *wiring.AppParams) error {
	// Get built-in templates from Go structs
	templates := resources.BuiltInLLMProviderTemplates

	if len(templates) == 0 {
		slog.Warn("No built-in LLM templates defined")
		return nil
	}

	// Load into in-memory store
	dependencies.LLMTemplateStore.Load(templates)

	slog.Info("Loaded built-in LLM provider templates into memory", "count", len(templates))
	return nil
}
