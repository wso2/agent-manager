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
// identities, with their routes, workers and the capabilities the core's agent
// service uses (llm.proxy/v1, mcp.proxy/v1, agent.gateway/v1). The module is
// always active. Its code is built on the core's services through package
// builtin; the core imports none of it, so a binary that does not link this
// package (cmd/amp-core) has no gateway.
package gateway

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/builtin"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/agentgwv1"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/llmv1"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/mcpv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/gateway/gwapi"
	"github.com/wso2/agent-manager/agent-manager-service/modules/gateway/gwinit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/gateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/gateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/gateway/gwwire"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/resources"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// Name is the module's name. It is also the resource of its permissions.
const Name = gwperm.Module

type module struct{}

func init() {
	modkit.Register(module{})
	builtin.Register(builtin.Bundle{Name: Name, Build: Build})
	gwinit.Register()
}

func (module) Manifest() modkit.Manifest {
	perms := make([]modkit.Permission, 0, len(gwperm.Actions))
	for _, a := range gwperm.Actions {
		perms = append(perms, modkit.Permission{Name: a.Name, Description: a.Description})
	}
	return modkit.Manifest{
		Name:        Name,
		Version:     "1.0.0",
		Description: "AI Gateway: gateways, LLM providers and proxies, MCP proxies, A2A and agent identities",
		CoreAPI:     modkit.CoreAPIVersion,
		Builtin:     true,
		Provides:    []string{llmv1.ID, mcpv1.ID, agentgwv1.ID},
		Permissions: perms,
		RoleGrants:  gwperm.RoleGrants,
	}
}

func (module) Migrations() fs.FS { return nil }
func (module) UI() fs.FS         { return nil }

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
	rt.Provide(llmv1.ID, gw.AgentGateway.LLM())
	rt.Provide(mcpv1.ID, gw.AgentGateway.MCP())
	rt.Provide(agentgwv1.ID, gw.AgentGateway)
	return nil
}

// Current is the gateway built at startup, or nil.
func Current() *gwwire.Gateway { return built.Load() }

// Capabilities returns what the module provides, for a process that builds the
// gateway without a module host (tests).
func Capabilities(gw *gwwire.Gateway) map[string]any {
	return map[string]any{
		llmv1.ID:     gw.AgentGateway.LLM(),
		mcpv1.ID:     gw.AgentGateway.MCP(),
		agentgwv1.ID: gw.AgentGateway,
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
	return Parts(gw, core), nil
}

// Wire builds the gateway's services without starting anything.
func Wire(core *wiring.AppParams) (*gwwire.Gateway, error) {
	cfg := core.Config
	// The manifest cache is a package-level backend, selected before anything
	// reads or writes it (see gwsvc.SetGatewayManifestCacheBackend).
	backend, err := gwwire.ProvideGatewayManifestCacheBackend(*cfg)
	if err != nil {
		return nil, fmt.Errorf("gateway manifest cache backend: %w", err)
	}
	gwsvc.SetGatewayManifestCacheBackend(backend)
	opts := currentOptions()
	gwsvc.SetMCPResourceServerProvider(thundersvc.IdentityManagementMode(cfg.IdentityManagementMode), opts.MCPResourceServerProvider)
	return gwwire.Initialize(core, opts.GatewayConfigApplier)
}

// Options are what a distribution injects into the gateway. The zero value
// is the open-source behaviour, which cmd/amp uses.
type Options struct {
	// GatewayConfigApplier patches the gateway runtime config when an
	// identity provider changes, keeping the gateway and the AMS mirror in
	// sync. nil preserves the script-driven behaviour; cloud deployments
	// inject an implementation. See gwsvc.GatewayConfigApplier.
	GatewayConfigApplier gwsvc.GatewayConfigApplier
	// MCPResourceServerProvider registers MCP resource servers through an
	// identity control plane. It is used only when IDENTITY_MANAGEMENT_MODE
	// is CP_MANAGED; in LEGACY_DIRECT mode (the default) MCP resource servers
	// are registered in each environment's own Thunder and this is ignored.
	// CP_MANAGED without it reports MCP resource servers as unavailable and
	// never uses the per-environment Thunder.
	MCPResourceServerProvider gwsvc.MCPResourceServerProvider
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
func Parts(gw *gwwire.Gateway, core *wiring.AppParams) *builtin.Parts {
	w := &workers{gw: gw}
	return &builtin.Parts{
		Routes: func(rr *middleware.RouteRegistrar) { gwapi.RegisterRoutes(rr, gw, core) },
		InternalRoutes: func(rr *middleware.RouteRegistrar, mux *http.ServeMux) {
			gwapi.RegisterInternalRoutes(rr, mux, gw)
		},
		MCPTools: gwapi.MCPTools(gw),
		// The gateway's pages list environments for callers who hold only
		// gateway, LLM provider or LLM proxy access.
		EnvironmentReaders: []rbac.Permission{rbac.LLMProviderRead, rbac.LLMProxyRead, gwperm.Read},
		Init:               w.seed,
		Start:              w.start,
		Stop:               w.stop,
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
