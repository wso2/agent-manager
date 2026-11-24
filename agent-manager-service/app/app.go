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
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/coreext"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/notifyv1"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/server"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/signals"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"

	"go.uber.org/automaxprocs/maxprocs"
	"gorm.io/gorm"
)

// The migration job usually finishes within a minute of the pod starting; the
// timeout only stops a wait that can never succeed from hanging forever.
const (
	migrationWaitTimeout  = 10 * time.Minute
	migrationWaitInterval = 2 * time.Second
)

// Options holds the configuration options for running the application.
// These are typically parsed from command-line flags in main.
type Options struct {
	// DefaultModules are installed at boot when no modules list is declared,
	// unless they were installed before (cmd/amp: ai-gateway).
	DefaultModules []string
	// APIMiddleware wraps the authenticated API mux before dispatch.
	APIMiddleware func(*http.ServeMux) http.Handler
	// StartBackground starts deployment-specific workers before the scheduler.
	// The returned cleanup runs during shutdown.
	StartBackground func(context.Context, *wiring.AppParams) (func(), error)

	// Server indicates whether to start the HTTP server
	Server bool
	// Migrate indicates whether to run database migrations before starting
	Migrate bool
	// WaitForMigrations blocks until another process has applied this build's
	// migrations, for an init container that runs ahead of the server
	WaitForMigrations bool
	// ExtraAPIRoutes registers additional routes onto the authenticated /api/v1 sub-mux.
	// Use this to inject deployment-specific routes without modifying the core handler.
	ExtraAPIRoutes func(mux *http.ServeMux, params *wiring.AppParams)
	// EnvThunderEndpointResolver optionally separates environment-Thunder
	// management and OAuth endpoints. nil preserves on-premises behavior. Run
	// automatically applies it to endpoint-aware provisioning implementations.
	EnvThunderEndpointResolver thundersvc.EnvThunderEndpointResolver
	// BackgroundContextDecorator optionally marks contexts created for trusted
	// background workers. Request contexts never pass through this hook.
	BackgroundContextDecorator func(context.Context) context.Context
	// AgentThunderProvisioning is the deployment-specific AgentID provisioning
	// implementation. A factory because the open-source impl is DB-backed and the
	// DB is initialized inside Run. nil disables provisioning (identity endpoints
	// report it unavailable, reconciler not started, no secret backend required).
	// secretMgmtClient is built by Run itself (see the call site) and handed in —
	// the same secret backend seam LLM/MCP/publisher secrets use (for per-agent
	// credentials). encryptionKey is the platform ENCRYPTION_KEY, used to decrypt
	// the env-Thunder system-client credential from AMS's own Postgres — that one
	// is not read back from a key vault.
	AgentThunderProvisioning func(db *gorm.DB, secretMgmtClient secretmanagersvc.SecretManagementClient, encryptionKey []byte) services.AgentThunderProvisioningService
	// OrgOverrideResolver lets a deployment's control plane act on a tenant's
	// behalf on the few routes it drives (gateway listing, environment Thunder
	// registration, gateway identity providers), typically by trusting an
	// X-Impersonate-Org header from its own service token. nil (the open-source
	// default) ignores any such header: the token is the sole source of org
	// identity. See middleware.OrgOverrideResolver.
	OrgOverrideResolver middleware.OrgOverrideResolver
}

// Run starts the application with the provided providers and options.
// This is the main entry point that both open-source and cloud main.go will call.
// The secretProvider parameter lets each deployment inject its own secret
// management backend.
func Run(secretProvider secretmanagersvc.Provider, opts Options) {
	cfg := config.GetConfig()
	// Installed before the HTTP server starts, so no request sees it change.
	middleware.SetOrgOverrideResolver(opts.OrgOverrideResolver)
	middleware.SetRouteHitLog(cfg.LogRouteHits)

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

	// The modules compiled into this binary. Their tables are migrated with the
	// core's, whether or not they are installed. Built on first use, so a run
	// that does nothing never opens the database.
	var modules *modhost.Host
	moduleHost := func() *modhost.Host {
		if modules == nil {
			modules = modhost.New(db.GetDB(), cfg.Database.Driver, slog.Default())
		}
		return modules
	}

	if opts.Migrate {
		if err := dbmigrations.Migrate(); err != nil {
			slog.Error("error occurred while migrating", "error", err)
			os.Exit(1)
		}
		if _, err := moduleHost().Migrate(context.Background()); err != nil {
			slog.Error("error occurred while migrating modules", "error", err)
			os.Exit(1)
		}
	}

	if opts.WaitForMigrations {
		ctx, cancel := context.WithTimeout(context.Background(), migrationWaitTimeout)
		err := dbmigrations.WaitForLatest(ctx, migrationWaitInterval)
		if err == nil {
			err = waitForModuleSchema(ctx, moduleHost())
		}
		cancel()
		if err != nil {
			slog.Error("database migrations did not complete; check that the migration job runs the same image version as this server",
				"error", err)
			os.Exit(1)
		}
	}

	if !opts.Server {
		return
	}

	// Get the raw DB instance without context - repositories will add context per-operation
	database := db.GetDB()

	// Deployment-specific AgentID provisioning; nil disables it.
	var agentThunderProvisioning services.AgentThunderProvisioningService
	if opts.AgentThunderProvisioning != nil {
		encryptionKey, err := wiring.ProvideEncryptionKey(*cfg)
		if err != nil {
			slog.Error("failed to load encryption key for AgentID provisioning", "error", err)
			os.Exit(1)
		}
		secretRefKeys, err := wiring.ProvideSecretRefKeys(*cfg, encryptionKey)
		if err != nil {
			slog.Error("failed to load retained encryption keys for AgentID provisioning", "error", err)
			os.Exit(1)
		}
		secretVaults, err := wiring.ProvideSecretVaults(*cfg, secretRefKeys)
		if err != nil {
			slog.Error("failed to open the secret vault for AgentID provisioning", "error", err)
			os.Exit(1)
		}
		secretMgmtClientForProvisioning, err := wiring.ProvideSecretManagementClient(*cfg, database, repositories.NewSecretStore(secretVaults), secretProvider)
		if err != nil {
			slog.Error("failed to create secret management client for AgentID provisioning", "error", err)
			os.Exit(1)
		}
		agentThunderProvisioning = opts.AgentThunderProvisioning(database, secretMgmtClientForProvisioning, encryptionKey)
		if setter, ok := agentThunderProvisioning.(services.EnvThunderEndpointResolverSetter); ok {
			setter.SetEnvThunderEndpointResolver(opts.EnvThunderEndpointResolver)
		}
	}

	dependencies, err := wiring.InitializeAppParamsWithEnvThunderEndpoints(cfg, database, secretProvider, agentThunderProvisioning, opts.EnvThunderEndpointResolver)
	if err != nil {
		slog.Error("failed to initialize app dependencies", "error", err)
		os.Exit(1)
	}

	// Load the active modules and follow lifecycle changes made elsewhere. The
	// host gets the core services modules use first, so the snapshot it
	// builds carries their routes. The default organization is created first,
	// so it exists when modules activate.
	modulesCtx, modulesCancel := context.WithCancel(context.Background())
	defer modulesCancel()
	if err := ensureDefaultOrg(audit.WithRecorder(modulesCtx, dependencies.AuditRecorder), database, cfg.DefaultOrg); err != nil {
		slog.Error("failed to create the default organization", "error", err)
		os.Exit(1)
	}
	stopModules, err := StartModules(modulesCtx, moduleHost(), database, dependencies)
	if err != nil {
		slog.Error("failed to start the module host", "error", err)
		os.Exit(1)
	}
	if err := ReconcileModules(modulesCtx, moduleHost(), opts.DefaultModules); err != nil {
		slog.Error("failed to apply the configured modules list", "error", err)
		os.Exit(1)
	}

	// Background workers have no request behind them, so nothing installs a
	// recorder on their contexts the way the HTTP middleware does. Install one
	// here, or their audit emits would reach the "not installed" fallback and
	// be lost — silently, which is the failure mode a context-carried recorder
	// is most prone to.
	backgroundCtx := audit.WithRecorder(context.Background(), dependencies.AuditRecorder)
	if opts.BackgroundContextDecorator != nil {
		backgroundCtx = opts.BackgroundContextDecorator(backgroundCtx)
	}

	stopBackground := func() {}
	if opts.StartBackground != nil {
		var err error
		stopBackground, err = opts.StartBackground(backgroundCtx, dependencies)
		if err != nil {
			slog.Error("failed to start deployment workers", "error", err)
			os.Exit(1)
		}
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

	// Promotes secret values to an external vault and removes replaced ones, once the change needing it commits.
	secretJobWorkerCtx, secretJobWorkerCancel := context.WithCancel(backgroundCtx)
	if err := dependencies.SecretJobWorker.Start(secretJobWorkerCtx); err != nil {
		slog.Error("failed to start secret vault job worker", "error", err)
		os.Exit(1)
	}

	// Start publishing events. The bus is set up in the background, so a NATS
	// server that is still starting does not hold up the API; events raised
	// meanwhile wait in the durable outbox.
	eventsCtx, eventsCancel := context.WithCancel(backgroundCtx)
	eventsStarted := startEvents(eventsCtx, dependencies)

	recordStartupPosture(dependencies.AuditRecorder)

	// Create main API server handler
	handler := api.MakeHTTPHandlerWithModules(dependencies, opts.ExtraAPIRoutes, opts.APIMiddleware, moduleHost())
	mainServer := &http.Server{
		Addr:           fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort),
		Handler:        handler,
		ReadTimeout:    time.Duration(cfg.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:   time.Duration(cfg.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:    time.Duration(cfg.IdleTimeoutSeconds) * time.Second,
		MaxHeaderBytes: cfg.MaxHeaderBytes,
	}

	// Create internal HTTPS server for module workloads and built-in features
	// (the gateway WebSocket and internal APIs)
	internalHandler := api.MakeInternalHTTPHandlerWithModules(dependencies, moduleHost())
	internalServer := server.NewInternalServer(&cfg.InternalServer, internalHandler)

	stopCh := signals.SetupSignalHandler()

	// Setup graceful shutdown
	var wg sync.WaitGroup

	wg.Go(func() {
		<-stopCh
		slog.Info("Shutdown signal received, stopping services...")

		// Single timeout context for the entire shutdown sequence
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		// Drain both servers before stopping anything an in-flight request
		// calls into: the built-in features' workers (the gateway's reconcile
		// worker, EventHub and WebSocket manager), the audit recorder.
		if err := mainServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("Main server forced shutdown after timeout", "error", err)
		}

		if err := internalServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("Internal server forced shutdown after timeout", "error", err)
		}

		stopBackground()
		modulesCancel()
		agentThunderReconcilerCancel()
		if agentThunderProvisioning != nil {
			if err := dependencies.AgentThunderReconciler.Stop(); err != nil {
				slog.Error("error stopping agent thunder provisioning reconciler", "error", err)
			}
		}

		secretJobWorkerCancel()
		if err := dependencies.SecretJobWorker.Stop(); err != nil {
			slog.Error("error stopping secret vault job worker", "error", err)
		}

		eventsCancel()
		<-eventsStarted
		stopEvents(dependencies)
		stopModules()

		// Flush the audit buffer last. The servers have drained and the
		// background workers have stopped, so no further events can be
		// produced and every in-flight request has finished recording.
		// Draining any earlier would discard the audit records of the requests
		// still being served.
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
			"tlsEnabled", cfg.InternalServer.TLSEnabled)
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

// StartModules attaches the core services to the module host, boots it, and
// makes the core's resource code run module hooks. The host drains module
// events and runs module workers itself. The returned function stops the
// workers and detaches the host from the core.
func StartModules(ctx context.Context, host *modhost.Host, database *gorm.DB, deps *wiring.AppParams) (func(), error) {
	attach := modhost.Deps{
		Mount:         api.ModuleMounter(deps),
		MountInternal: api.ModuleInternalMounter(deps),
		Core:          repositories.NewCoreReadAPI(database),
		Secrets:       repositories.NewModuleSecretStore(database, deps.SecretStore),
		InternalURL:   config.GetConfig().InternalServerPublicURL,
		Capabilities: map[string]any{
			identityv1.ID: services.NewIdentityDirectory(deps.EnvThunderResolver, deps.AgentThunderClientRepository),
			notifyv1.ID:   services.NewModuleNotifier(repositories.NewEventRepository(database, nil), repositories.NewEventOutboxRepository, database),
		},
	}
	if signer, ok := deps.AgentTokenManagerService.(services.WorkloadTokenSigner); ok {
		attach.Tokens = signer
	}
	host.Attach(attach)
	if err := host.Boot(ctx); err != nil {
		return func() {}, err
	}
	coreext.Set(host)
	return func() {
		host.StopWorkers()
		coreext.Set(nil)
	}, nil
}

// ReconcileModules applies the declarative modules list (amp.yaml `modules:`
// or AMP_MODULES), if one is configured: listed modules are installed and
// enabled or disabled as listed; others are disabled. With no list, it
// installs the distribution's default modules that were never installed and
// changes nothing else. Every replica runs it at boot; it changes nothing once
// the database matches.
func ReconcileModules(ctx context.Context, host *modhost.Host, defaults []string) error {
	mods, err := config.GetModules()
	if err != nil {
		return err
	}
	if !mods.Declared {
		steps, err := host.InstallDefaults(ctx, defaults)
		for _, s := range steps {
			slog.Info("default module installed", "module", s.Module, "action", s.Action)
		}
		return err
	}
	want := make([]modhost.Desired, 0, len(mods.List))
	for _, m := range mods.List {
		want = append(want, modhost.Desired{Name: m.Name, Enabled: m.Enabled})
	}
	slog.Info("applying the configured modules list", "modules", want)
	steps, err := host.Reconcile(ctx, want)
	for _, s := range steps {
		slog.Info("modules list applied", "module", s.Module, "action", s.Action)
	}
	if err == nil && len(steps) == 0 {
		slog.Info("modules already match the configured list")
	}
	return err
}

// waitForModuleSchema waits until another process has applied the migrations
// of every module compiled into this binary.
func waitForModuleSchema(ctx context.Context, modules *modhost.Host) error {
	for {
		err := modules.CheckSchema(ctx)
		if err == nil {
			return nil
		}
		slog.Info("waiting for module migrations", "reason", err.Error())
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for module migrations: %w (last: %w)", ctx.Err(), err)
		case <-time.After(migrationWaitInterval):
		}
	}
}

// recordStartupPosture writes the audit trail's own opening bookend. The record
// bounds any gap in the trail to a restart: a reader can tell "nothing happened"
// apart from "the service was not running".
func recordStartupPosture(recorder audit.Recorder) {
	if recorder == nil {
		return
	}
	ctx := audit.WithRecorder(context.Background(), recorder)

	audit.RecordAncillary(
		ctx, audit.ActionSystemStartup,
		audit.Actor(audit.ActorSystem, "system:agent-manager-service", ""),
		audit.SurfaceOpt(audit.SurfaceSystem),
		audit.Detail("sinks", []string{"stdout"}),
	)
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

// startEvents sets up the event bus, retrying until it answers, then starts
// the publisher and the event dispatcher. A no-op with events off.
func startEvents(ctx context.Context, deps *wiring.AppParams) <-chan struct{} {
	done := make(chan struct{})
	if deps.EventBus == nil {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		// Setup provisions durable consumers as well as streams. Each step retries
		// independently, so a transient consumer-creation failure cannot disable
		// alerting until the next process restart.
		steps := []struct {
			name  string
			start func(context.Context) error
		}{
			{"event bus", deps.EventBus.Setup},
			{"event dispatcher", deps.EventDispatcher.Start},
		}
		for _, step := range steps {
			for {
				if ctx.Err() != nil {
					return
				}
				// The setup timeout must not become a worker's lifetime context.
				var err error
				if step.name == "event bus" {
					setupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
					err = step.start(setupCtx)
					cancel()
				} else {
					err = step.start(ctx)
				}
				if err == nil {
					break
				}
				slog.Warn("Event component not ready; retrying", "component", step.name, "error", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
			}
		}
		if ctx.Err() == nil {
			if publisher, ok := deps.EventPublisher.(*events.AsyncPublisher); ok {
				publisher.Start()
			}
		}
	}()
	return done
}

// stopEvents stops the event dispatcher, flushes
// queued events and closes the bus.
func stopEvents(deps *wiring.AppParams) {
	if deps.EventBus == nil {
		return
	}
	if err := deps.EventDispatcher.Stop(); err != nil {
		slog.Error("Failed to stop the event dispatcher", "error", err)
	}
	if publisher, ok := deps.EventPublisher.(*events.AsyncPublisher); ok {
		publisher.Stop()
	}
	deps.EventBus.Close()
}
