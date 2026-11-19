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
	"fmt"
	"io/fs"
	"log/slog"
	"sync/atomic"
	"time"

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
)

// Name is the module's name. It is also the resource of its permissions.
const Name = gwperm.Module

type module struct{}

func init() { modkit.Register(module{}) }

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
	{ID: "ai-gateway/agent-security", Permission: gwperm.AgentRead},
	{ID: "ai-gateway/configure-agent", Permission: gwperm.AgentRead},
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

// OnInstall restores the drift scan's lease row, which the module's
// migration seeds once: a purge empties the table and runs no migration.
func (module) OnInstall(ctx context.Context, rt modkit.Runtime) error {
	return rt.DB().WithContext(ctx).Exec(`INSERT INTO mod_ai_gateway_locks (name)
		SELECT 'drift_scan' WHERE NOT EXISTS (SELECT 1 FROM mod_ai_gateway_locks WHERE name = 'drift_scan')`).Error
}

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

// current is the gateway of the active module. Activate builds it the first
// time and reuses it on every later activation while the module stays active,
// so the gateway's connections and workers outlive another module's change;
// the lifecycle worker tears it down when the module stops.
var current atomic.Pointer[gwwire.Gateway]

func (module) Activate(ctx context.Context, rt modkit.Runtime) error {
	gw := current.Load()
	if gw == nil {
		built, err := build(ctx, rt)
		if err != nil {
			return err
		}
		if !current.CompareAndSwap(nil, built) {
			built.Close()
		}
		gw = current.Load()
	}
	gw.IdentityDirectory.Use(func() (identityv1.Directory, bool) {
		return modkit.Lookup[identityv1.Directory](rt, identityv1.ID)
	})
	gw.Permissions.Use(rt.Can)
	gw.Secrets.Use(rt.Secrets())
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
	rt.Go("gateway", func(ctx context.Context) error { return run(ctx, gw) })
	return nil
}

// Current is the gateway of the active module, or nil.
func Current() *gwwire.Gateway { return current.Load() }

// SetCurrent sets the gateway Activate mounts instead of building one, for
// tests.
func SetCurrent(gw *gwwire.Gateway) { current.Store(gw) }

// build builds the gateway on the module's runtime and registers the gateway
// named in its settings, before its routes are mounted so no gateway can
// connect first. The core's default organization exists by then.
func build(ctx context.Context, rt modkit.Runtime) (*gwwire.Gateway, error) {
	settings, err := gwconfig.Load(rt.Config)
	if err != nil {
		return nil, fmt.Errorf("ai-gateway settings: %w", err)
	}
	// The manifest cache and the agent CORS defaults are package-level, selected
	// before anything reads them (see gwsvc.SetGatewayManifestCacheBackend).
	gwsvc.SetGatewayManifestCacheBackend(gwwire.ProvideGatewayManifestCacheBackend(settings))
	gwsvc.SetAgentCORSDefaults(settings.AgentCORS)
	gw, err := gwwire.Initialize(rt.DB(), rt.Logger(), rt.Core(), settings)
	if err != nil {
		return nil, err
	}
	gw.LLMTemplateStore.Load(resources.BuiltInLLMProviderTemplates)
	seedCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := gw.DefaultGatewaySeeder.Seed(seedCtx); err != nil {
		gw.Close()
		return nil, fmt.Errorf("failed to register the default gateway: %w", err)
	}
	return gw, nil
}

// run runs the gateway's workers until ctx ends, when the module is disabled
// or the server stops (after its servers have drained), then closes the
// gateway's connections; the next activation builds a new one.
func run(ctx context.Context, gw *gwwire.Gateway) error {
	workers := []struct {
		name  string
		start func(context.Context) error
		stop  func() error
	}{
		{"A2A card reconciler", gw.A2ACardReconciler.Start, gw.A2ACardReconciler.Stop},
		{"deployment timeout sweeper", gw.DeploymentTimeoutSweeper.Start, gw.DeploymentTimeoutSweeper.Stop},
		{"reconcile worker", gw.ReconcileWorker.Start, gw.ReconcileWorker.Stop},
	}
	for i, w := range workers {
		if err := w.start(ctx); err != nil {
			for _, started := range workers[:i] {
				_ = started.stop()
			}
			return fmt.Errorf("failed to start %s: %w", w.name, err)
		}
	}
	<-ctx.Done()
	for _, w := range workers {
		if err := w.stop(); err != nil {
			slog.Error("error stopping "+w.name, "error", err)
		}
	}
	gw.Close()
	current.CompareAndSwap(gw, nil)
	return nil
}
