//go:build wireinject

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

package wiring

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/wire"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/fileauth"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations/notify"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations/webhook"
)

// Provider sets
var configProviderSet = wire.NewSet(
	ProvideConfigFromPtr,
	ProvideEncryptionKey,
	ProvideSecretRefKeys,
	ProvideSecretVaults,
	repositories.NewSecretStore,
	repositories.NewSecretJobRunner,
)

var clientProviderSet = wire.NewSet(
	repositories.NewPlatformRepo,
	ProvideSecretManagementClient,
	ProvideIdentityClient,
	ProvideOrgResolver,
	ProvideOUHandleLookup,
	thundersvc.NewProber,
	// EnvThunderResolver is wired for the agent-identity passthrough
	// endpoints. AgentID provisioning itself is injected via
	// app.Options.AgentThunderProvisioning, built with its own reader — see app.Run.
	services.NewPlatformThunderSecretReader,
	services.NewPlatformThunderURLReader,
	ProvideEnvThunderResolverWithEndpoints,
)

var serviceProviderSet = wire.NewSet(
	services.NewAgentManagerService,
	services.NewInfraResourceManager,
	services.NewAgentTokenManagerService,
	fileauth.NewService,
	ProvideThunderConfig,
	// Provisioning service is injected (see InitializeAppParams); only the
	// reconciler that consumes it is wired here.
	services.NewAgentThunderReconcilerService,
	services.NewSecretJobWorker,
	services.NewEnvironmentService,
	ProvideEventBus,
	ProvideEventPublisher,
	repositories.NewEventOutboxRepository,
	ProvideDestinationDrivers,
	services.NewEventDestinationService,
	services.NewEventSubscriptionService,
	wire.Bind(new(services.EventsCleaner), new(services.EventSubscriptionService)),
	wire.Bind(new(services.DestinationResolver), new(services.EventDestinationService)),
	services.NewEventDispatcherService,
)

var controllerProviderSet = wire.NewSet(
	controllers.NewAgentController,
	controllers.NewInfraResourceController,
	controllers.NewAgentTokenController,
	controllers.NewEnvironmentController,
	controllers.NewIdentityController,
	controllers.NewEventsController,
)

var testClientProviderSet = wire.NewSet(
	ProvideTestPlatformRepository,
	ProvideTestSecretManagementClient,
	ProvideIdentityClient,
	ProvideOrgResolver,
	ProvideTestOUHandleLookup,
	thundersvc.NewProber,
	services.NewPlatformThunderSecretReader,
	services.NewPlatformThunderURLReader,
	ProvideEnvThunderResolver,
)

// thunderProvisioningTestSet builds the provisioning service for
// the test wiring only; production injects it via InitializeAppParams.
var thunderProvisioningTestSet = wire.NewSet(
	services.NewAgentThunderProvisioningService,
)

// ProvideLogger provides the configured slog.Logger instance
func ProvideLogger() *slog.Logger {
	return slog.Default()
}

// ProvideAuditRecorder builds the audit recorder.
//
// Records go to stdout as structured JSON, where the platform's log pipeline
// already collects them. That also gives the trail a copy this service cannot
// rewrite, since there is no write path from here back into the log store.
func ProvideAuditRecorder(cfg config.Config, logger *slog.Logger) audit.Recorder {
	if !cfg.Audit.Enabled {
		logger.Warn("audit logging is disabled; no record of privileged operations will be kept",
			"setting", "AUDIT_ENABLED")
		return audit.NewNoopRecorder()
	}
	return audit.NewRecorder(audit.NewStdoutSink(), logger, audit.Config{
		BufferSize:    cfg.Audit.BufferSize,
		BatchSize:     cfg.Audit.BatchSize,
		FlushInterval: time.Duration(cfg.Audit.FlushIntervalMs) * time.Millisecond,
	})
}

// ProvideSecretManagementClient creates the secret management service client.
// The database and its secret store are handed to the provider for the
// in-house store; a provider with its own backend ignores them.
func ProvideSecretManagementClient(cfg config.Config, db *gorm.DB, secrets *repositories.SecretStore, secretProvider secretmanagersvc.Provider) (secretmanagersvc.SecretManagementClient, error) {
	return secretmanagersvc.NewSecretManagementClientWithConfig(secretmanagersvc.SecretManagementClientConfig{
		StoreConfig: &secretmanagersvc.StoreConfig{Provider: cfg.SecretManager.Provider, DB: db, Secrets: secrets},
		Provider:    secretProvider,
	})
}

var loggerProviderSet = wire.NewSet(
	ProvideLogger,
	ProvideAuditRecorder,
)

var repositoryProviderSet = wire.NewSet(
	ProvideAgentConfigRepository,
	ProvideAgentThunderClientRepository,
	repositories.NewEventRepository,
)

// Test client providers
func ProvideTestPlatformRepository(testClients TestClients) repositories.PlatformRepository {
	return testClients.PlatformRepository
}

func ProvideTestSecretManagementClient(testClients TestClients) secretmanagersvc.SecretManagementClient {
	return testClients.SecretMgmtClient
}

func ProvideAgentConfigRepository(db *gorm.DB) repositories.AgentConfigRepository {
	return repositories.NewAgentConfigRepo(db)
}

func ProvideAgentThunderClientRepository(db *gorm.DB) repositories.AgentThunderClientRepository {
	return repositories.NewAgentThunderClientRepo(db)
}

// ProvideEnvThunderResolver maps the default environment to an authenticated
// client for the platform Thunder, read through the injected readers.
func ProvideEnvThunderResolver(readSystemClient thundersvc.ReadSystemClientFunc, readThunderURL thundersvc.ReadThunderURLFunc) thundersvc.EnvThunderResolver {
	return thundersvc.NewEnvThunderResolver(readSystemClient, readThunderURL)
}

func ProvideEnvThunderResolverWithEndpoints(readSystemClient thundersvc.ReadSystemClientFunc, readThunderURL thundersvc.ReadThunderURLFunc, endpointResolver thundersvc.EnvThunderEndpointResolver) thundersvc.EnvThunderResolver {
	return thundersvc.NewEnvThunderResolverWithEndpoints(readSystemClient, readThunderURL, endpointResolver)
}

// ProvideDestinationDrivers registers the driver of each supported event
// destination kind. Add a kind here once its driver exists.
func ProvideDestinationDrivers(cfg config.Config) *eventdestinations.Registry {
	return eventdestinations.NewRegistry(
		webhook.New(cfg.Events.AllowPrivateDestinations),
		notify.New(cfg.Events.AllowPrivateDestinations),
	)
}

// ProvideEventBus connects to NATS, or starts the embedded in-memory server,
// as EVENTS_MODE says. With events off it returns nil: nothing is published.
func ProvideEventBus(cfg config.Config, logger *slog.Logger) (events.Bus, error) {
	switch cfg.Events.Mode {
	case config.EventsModeEmbedded:
		bus, err := events.StartEmbedded(logger)
		if err != nil {
			return nil, err
		}
		return bus, nil
	case config.EventsModeNATS:
		bus, err := events.Connect(cfg.Events.NATSURL, cfg.Events.NATSToken, "agent-manager-service", logger)
		if err != nil {
			return nil, err
		}
		return bus, nil
	default:
		// A nil interface, not a nil *NATSBus, so callers' nil checks work.
		logger.Info("Events disabled: EVENTS_MODE is off")
		return nil, nil
	}
}

// ProvideEventPublisher returns the publisher routes use. Without a bus,
// events are discarded.
func ProvideEventPublisher(bus events.Bus, outbox events.Outbox, logger *slog.Logger) events.Publisher {
	if bus == nil {
		return events.NoopPublisher{}
	}
	return events.NewAsyncPublisher(bus, outbox, logger)
}

func ProvideThunderConfig(cfg config.Config) config.ThunderConfig {
	return cfg.Thunder
}

// ProvideIdentityClient creates a Thunder identity client using the Thunder system app credentials.
func ProvideIdentityClient(cfg config.ThunderConfig) thundersvc.IdentityClient {
	return thundersvc.NewIdentityClientWithSystemResource(
		cfg.BaseURL,
		cfg.ClientID,
		cfg.ClientSecret,
		cfg.ResolveToHost,
		cfg.ResolvedSystemResourceIdentifier(),
	)
}

// ProvideOUHandleLookup maps an OU ID to its handle through the platform
// Thunder, for the org override on the project MCP routes. It is deliberately
// uncached: a handle can be renamed, and the lookup runs only on impersonated
// requests. A missing OU is reported as middleware.ErrOverrideOrgNotFound.
func ProvideOUHandleLookup(client thundersvc.IdentityClient) middleware.OUHandleLookup {
	return func(ctx context.Context, ouID string) (string, error) {
		handle, err := client.GetOUHandleByID(ctx, ouID)
		if thundersvc.IsNotFound(err) {
			return "", fmt.Errorf("%w: %w", middleware.ErrOverrideOrgNotFound, err)
		}
		return handle, err
	}
}

// ProvideTestOUHandleLookup uses the test's lookup when it supplies one, and
// the Thunder-backed lookup otherwise.
func ProvideTestOUHandleLookup(testClients TestClients, client thundersvc.IdentityClient) middleware.OUHandleLookup {
	if testClients.OUHandleLookup != nil {
		return testClients.OUHandleLookup
	}
	return ProvideOUHandleLookup(client)
}

// ProvideOrgResolver creates the org resolver backed by Thunder, with a per-org OU ID cache.
func ProvideOrgResolver(client thundersvc.IdentityClient) middleware.OrgResolver {
	return middleware.NewOrgResolver(client)
}

// InitializeAppParams builds the core's services.
func InitializeAppParams(cfg *config.Config, db *gorm.DB, secretProvider secretmanagersvc.Provider, agentThunderProvisioning services.AgentThunderProvisioningService) (*AppParams, error) {
	return InitializeAppParamsWithEnvThunderEndpoints(cfg, db, secretProvider, agentThunderProvisioning, nil)
}

// InitializeAppParamsWithEnvThunderEndpoints is InitializeAppParams with an
// optional separation of environment-Thunder management and OAuth endpoints.
func InitializeAppParamsWithEnvThunderEndpoints(cfg *config.Config, db *gorm.DB, secretProvider secretmanagersvc.Provider, agentThunderProvisioning services.AgentThunderProvisioningService, endpointResolver thundersvc.EnvThunderEndpointResolver) (*AppParams, error) {
	wire.Build(
		configProviderSet,
		clientProviderSet,
		loggerProviderSet,
		repositoryProviderSet,
		serviceProviderSet,
		controllerProviderSet,
		ProvideAuthMiddleware,
		ProvideJWTSigningConfig,
		wire.Struct(new(AppParams), "*"),
	)
	return &AppParams{}, nil
}

// InitializeTestAppParamsWithClientMocks builds the core's services with
// mocked clients.
func InitializeTestAppParamsWithClientMocks(
	cfg *config.Config,
	db *gorm.DB,
	authMiddleware jwtassertion.Middleware,
	testClients TestClients,
) (*AppParams, error) {
	wire.Build(
		testClientProviderSet,
		thunderProvisioningTestSet,
		loggerProviderSet,
		repositoryProviderSet,
		serviceProviderSet,
		controllerProviderSet,
		configProviderSet,
		ProvideJWTSigningConfig,
		wire.Struct(new(AppParams), "*"),
	)
	return &AppParams{}, nil
}
