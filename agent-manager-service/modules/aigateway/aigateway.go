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

// Package gateway registers the AI Gateway as a built-in module: gateways,
// LLM providers and proxies, MCP proxies, A2A, agent API keys and agent
// identities, with their routes, workers, agent hooks and the capabilities
// llm.proxy/v1 and mcp.proxy/v1. The module is
// always active. Its code is built on the core's services through package
// builtin; the core imports none of it, so a binary that does not link this
// package (cmd/amp-core) has no gateway.
package aigateway

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/builtin"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/llmv1"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/mcpv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwapi"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconfig"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwinit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmcp"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwwire"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/resources"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// Name is the module's name. It is also the resource of its permissions.
const Name = gwperm.Module

type module struct{}

func init() {
	modkit.Register(module{})
	builtin.Register(builtin.Bundle{Name: Name, Build: Build})
	// The environment tier says where an action lands, never what it is.
	audit.RegisterAxisPermissions(gwperm.DeployNonProduction, gwperm.DeployProduction)
	// The scopes of the tools the gateway's toolsets add to /mcp.
}

func (module) Manifest() modkit.Manifest {
	perms := make([]modkit.Permission, 0, len(gwperm.Actions))
	for _, a := range gwperm.Actions {
		perms = append(perms, modkit.Permission{Name: a.Name, DisplayName: a.DisplayName, Description: a.Description})
	}
	return modkit.Manifest{
		Name:         Name,
		DisplayName:  "AI Gateway",
		Version:      "1.0.0",
		Description:  "AI Gateway: gateways, LLM providers and proxies, MCP proxies, A2A and agent identities",
		CoreAPI:      modkit.CoreAPIVersion,
		Builtin:      true,
		Provides:     []string{llmv1.ID, mcpv1.ID},
		Requires:     []string{identityv1.ID},
		Permissions:  perms,
		RoleGrants:   gwperm.RoleGrants,
		Events:       gwaudit.Events,
		AuditActions: gwaudit.Actions,
		UI:           &ui,
	}
}

// ui turns on the console's AI Gateway pages and its agent identity page.
var ui = modkit.UI{Pages: []modkit.Page{
	{ID: "ai-gateway/gateways", Permission: "gateways.read"},
	{ID: "ai-gateway/llm-providers", Permission: "llm-providers.read"},
	{ID: "ai-gateway/mcp-proxies", Permission: "mcp-servers.read"},
	{ID: "ai-gateway/agent-security", Permission: string(rbac.AgentRead)},
	{ID: "ai-gateway/configure-agent", Permission: string(rbac.AgentRead)},
	{ID: "agent-identity/agent-identity", Permission: "agent-identities.read"},
}}

//go:embed api/openapi.yaml
var openAPI []byte

// OpenAPI is the module's API document, at /m/ai-gateway.
func (module) OpenAPI() []byte { return openAPI }

//go:embed migrations
var migrations embed.FS

// Migrations creates the module's tables (mod_ai_gateway_*), rendered from
// schema/ by make schema.
func (module) Migrations() fs.FS {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}
func (module) UI() fs.FS { return nil }

// Init writes the gateway's config.toml, encryption key, listener certificate
// and registration token at "amp init ai-gateway DIR".
func (module) Init(_ context.Context, req modkit.InitRequest) error {
	cfg, err := gwinit.LoadGatewayConfig(req.Config)
	if err != nil {
		return err
	}
	cfg.CoreOwner = req.CoreOwner
	return gwinit.WriteGateway(cfg, req.Dir)
}

// built is the gateway built at startup; Activate provides its capabilities.
var built atomic.Pointer[gwwire.Gateway]

// errNotBuilt is returned by Activate in a process that did not build the
// gateway (it is built when the server starts).
var errNotBuilt = errors.New("the gateway is built when the server starts")

func (module) Activate(_ context.Context, rt modkit.Runtime) error {
	gw := built.Load()
	if gw == nil {
		return errNotBuilt
	}
	gw.IdentityDirectory.Use(func() (identityv1.Directory, bool) {
		return modkit.Lookup[identityv1.Directory](rt, identityv1.ID)
	})
	rt.Provide(llmv1.ID, gw.AgentGateway.LLM())
	rt.Provide(mcpv1.ID, gw.AgentGateway.MCP())
	registerHooks(rt, gw.AgentGateway)
	gwapi.RegisterRoutes(gwapi.NewModuleRouter(rt), gw)
	gwapi.RegisterInternalRoutes(gwapi.NewModuleInternalRouter(rt), gw)
	(&gwmcp.Tools{
		Deployments:  gwmcp.NewDeploymentHandler(gw.AgentGateway),
		LLMProviders: gwmcp.NewLLMProviderHandler(gw.LLMProviderService, gw.LLMProviderDeploymentService, gw.PlatformGatewayService),
		MCPProxies:   gwmcp.NewMCPProxyHandler(gw.MCPProxyService),
		AgentConfigs: gwmcp.NewAgentConfigHandler(gw.AgentConfigurationService),
		Core:         rt.Core(),
		Can:          rt.Can,
	}).Register(rt)
	return nil
}

// Current is the gateway built at startup, or nil.
func Current() *gwwire.Gateway { return built.Load() }

// SetCurrent sets the gateway Activate mounts, for a process that builds it
// without Build (tests).
func SetCurrent(gw *gwwire.Gateway) { built.Store(gw) }

// Capabilities returns what the module provides, for a process that builds the
// gateway without a module host (tests).
func Capabilities(gw *gwwire.Gateway) map[string]any {
	return map[string]any{
		llmv1.ID: gw.AgentGateway.LLM(),
		mcpv1.ID: gw.AgentGateway.MCP(),
	}
}

// Build builds the gateway on the core's services and returns what it adds to
// the server.
func Build(_ context.Context, core *wiring.AppParams) (*builtin.Parts, error) {
	gw, err := Wire(core)
	if err != nil {
		return nil, err
	}
	built.Store(gw)
	return Parts(gw), nil
}

// Wire builds the gateway's services without starting anything.
func Wire(core *wiring.AppParams) (*gwwire.Gateway, error) {
	settings, err := gwconfig.Load(func(key string) string {
		return config.Value(config.ModuleConfigKey(gwperm.Module, key))
	})
	if err != nil {
		return nil, fmt.Errorf("ai-gateway settings: %w", err)
	}
	// The manifest cache and the agent CORS defaults are package-level, selected
	// before anything reads them (see gwsvc.SetGatewayManifestCacheBackend).
	gwsvc.SetGatewayManifestCacheBackend(gwwire.ProvideGatewayManifestCacheBackend(settings))
	gwsvc.SetAgentCORSDefaults(settings.AgentCORS)
	opts := currentOptions()
	return gwwire.Initialize(core, opts.GatewayConfigApplier, settings)
}

// Options are what a distribution injects into the gateway. The zero value
// is the open-source behaviour, which cmd/amp uses.
type Options struct {
	// GatewayConfigApplier patches the gateway runtime config when an
	// identity provider changes, keeping the gateway and the AMS mirror in
	// sync. nil preserves the script-driven behaviour; cloud deployments
	// inject an implementation. See gwsvc.GatewayConfigApplier.
	GatewayConfigApplier gwsvc.GatewayConfigApplier
}

var options atomic.Pointer[Options]

// Configure sets the gateway's options. A distribution's main calls it
// before the server starts; the gateway reads them when it is built.
func Configure(o Options) { options.Store(&o) }

func currentOptions() Options {
	if o := options.Load(); o != nil {
		return *o
	}
	return Options{}
}

// Parts is what the built gateway adds to the server.
func Parts(gw *gwwire.Gateway) *builtin.Parts {
	w := &workers{gw: gw}
	return &builtin.Parts{
		Init:  w.seed,
		Start: w.start,
		Stop:  w.stop,
	}
}

type workers struct {
	gw     *gwwire.Gateway
	cancel []context.CancelFunc
}

// seed registers the gateway named in configuration before any gateway can
// connect.
func (w *workers) seed(ctx context.Context) error {
	seedCtx, seedCancel := context.WithTimeout(ctx, time.Minute)
	defer seedCancel()
	if err := w.gw.DefaultGatewaySeeder.Seed(seedCtx); err != nil {
		return fmt.Errorf("failed to register the default gateway: %w", err)
	}
	return nil
}

func (w *workers) start(ctx context.Context) error {
	// Started unconditionally: it is a no-op for an org with no A2A agents.
	for _, s := range []struct {
		name  string
		start func(context.Context) error
	}{
		{"A2A card reconciler", w.gw.A2ACardReconciler.Start},
		{"deployment timeout sweeper", w.gw.DeploymentTimeoutSweeper.Start},
		{"reconcile worker", w.gw.ReconcileWorker.Start},
	} {
		c, cancel := context.WithCancel(ctx)
		w.cancel = append(w.cancel, cancel)
		if err := s.start(c); err != nil {
			return fmt.Errorf("failed to start %s: %w", s.name, err)
		}
	}

	// Load built-in LLM provider templates into memory.
	if templates := resources.BuiltInLLMProviderTemplates; len(templates) == 0 {
		slog.Warn("No built-in LLM templates defined")
	} else {
		w.gw.LLMTemplateStore.Load(templates)
		slog.Info("Loaded built-in LLM provider templates into memory", "count", len(templates))
	}
	return nil
}

func (w *workers) stop(ctx context.Context) {
	for _, cancel := range w.cancel {
		cancel()
	}
	if err := w.gw.A2ACardReconciler.Stop(); err != nil {
		slog.Error("error stopping A2A card reconciler", "error", err)
	}
	if err := w.gw.DeploymentTimeoutSweeper.Stop(); err != nil {
		slog.Error("error stopping deployment timeout sweeper", "error", err)
	}
	if err := w.gw.ReconcileWorker.Stop(); err != nil {
		slog.Error("error stopping reconcile worker", "error", err)
	}

	// Gateway WebSockets are hijacked connections, which server Shutdown
	// neither waits for nor closes; the WebSocket manager closes them here.
	// Shut down in a goroutine since it blocks.
	wsDone := make(chan struct{})
	if w.gw.WebSocketManager != nil {
		go func() {
			slog.Info("Shutting down WebSocket manager")
			w.gw.WebSocketManager.Shutdown()
			close(wsDone)
		}()
	} else {
		close(wsDone)
	}
	select {
	case <-wsDone:
		slog.Info("WebSocket manager shutdown complete")
	case <-ctx.Done():
		slog.Warn("WebSocket manager shutdown timed out")
	}

	// Close EventHub after the servers drain, so events published by
	// draining requests are still polled and delivered and the gateway
	// registration prepared statement stays open, and after the WebSocket
	// manager, so in-flight events are not dropped.
	if w.gw.EventHub != nil {
		if err := w.gw.EventHub.Close(); err != nil {
			slog.Error("error closing EventHub", "error", err)
		}
	}
}
