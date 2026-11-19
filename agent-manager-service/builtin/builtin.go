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

// Package builtin is how features linked into the binary that do not run on
// modkit.Runtime yet plug into the server: the AI Gateway and the legacy
// remnants. A bundle registers from its module's init and is built once at
// startup from the core's services. It adds routes to the core's routers,
// with the core's authentication, authorization and audit, and runs its
// workers in the server's lifecycle, so the service behaves as it did when
// this code was wired by the core directly. The core imports no bundle; a
// binary that links none serves the core alone.
package builtin

import (
	"context"
	"net/http"
	"sort"
	"sync"

	"github.com/wso2/agent-manager/agent-manager-service/mcp/tools"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// Bundle is a feature set built on the core.
type Bundle struct {
	// Name is the module the bundle belongs to.
	Name string
	// Build creates the bundle's services from the core's. It runs once,
	// before the module host boots, so the module's Activate can provide
	// what was built.
	Build func(ctx context.Context, core *wiring.AppParams) (*Parts, error)
}

// Parts is what a built bundle adds to the server. Every field is optional.
type Parts struct {
	// Routes registers routes on the authenticated /api/v1 router.
	Routes func(rr *middleware.RouteRegistrar)
	// InternalRoutes registers routes on the internal /api/internal/v1
	// router, which has no JWT authentication.
	InternalRoutes func(rr *middleware.RouteRegistrar, mux *http.ServeMux)
	// RootRoutes registers routes at the server root, outside /api/v1; they
	// authenticate themselves.
	RootRoutes func(mux *http.ServeMux, core *wiring.AppParams)
	// MCPTools adds the bundle's toolsets to the core's MCP server at /mcp.
	MCPTools func(ts *tools.Toolsets)
	// EnvironmentReaders are permissions that also let a caller read the
	// organization's environments, because the bundle's pages list them.
	EnvironmentReaders []rbac.Permission
	// Init runs once right after the module host boots, before the
	// deployment's background workers and the bundle's own workers start;
	// an error stops the server. ctx carries no audit recorder.
	Init func(ctx context.Context) error
	// Start starts the bundle's workers, after the module host and before
	// the servers. ctx is a background context with the audit recorder.
	Start func(ctx context.Context) error
	// Stop stops them, after the servers have drained.
	Stop func(ctx context.Context)
}

var (
	mu      sync.Mutex
	bundles = map[string]Bundle{}
)

// Register adds a bundle. It panics on a second bundle of the same name.
func Register(b Bundle) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := bundles[b.Name]; ok {
		panic("builtin: bundle " + b.Name + " registered twice")
	}
	bundles[b.Name] = b
}

// Registered returns the registered bundles, by name.
func Registered() []Bundle {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Bundle, 0, len(bundles))
	for _, b := range bundles {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Built is the parts of every registered bundle, built in name order.
type Built []*Parts

// Build builds every registered bundle.
func Build(ctx context.Context, core *wiring.AppParams) (Built, error) {
	var out Built
	for _, b := range Registered() {
		p, err := b.Build(ctx, core)
		if err != nil {
			return nil, &BuildError{Bundle: b.Name, Err: err}
		}
		if p != nil {
			out = append(out, p)
		}
	}
	return out, nil
}

// BuildError reports the bundle that failed to build.
type BuildError struct {
	Bundle string
	Err    error
}

func (e *BuildError) Error() string { return "building " + e.Bundle + ": " + e.Err.Error() }
func (e *BuildError) Unwrap() error { return e.Err }

// Routes registers every bundle's API routes.
func (b Built) Routes(rr *middleware.RouteRegistrar) {
	for _, p := range b {
		if p.Routes != nil {
			p.Routes(rr)
		}
	}
}

// InternalRoutes registers every bundle's internal routes.
func (b Built) InternalRoutes(rr *middleware.RouteRegistrar, mux *http.ServeMux) {
	for _, p := range b {
		if p.InternalRoutes != nil {
			p.InternalRoutes(rr, mux)
		}
	}
}

// RootRoutes registers every bundle's root routes.
func (b Built) RootRoutes(mux *http.ServeMux, core *wiring.AppParams) {
	for _, p := range b {
		if p.RootRoutes != nil {
			p.RootRoutes(mux, core)
		}
	}
}

// MCPTools is every bundle's MCP toolsets, to pass to mcp.RegisterRoute.
func (b Built) MCPTools() []func(*tools.Toolsets) {
	var out []func(*tools.Toolsets)
	for _, p := range b {
		if p.MCPTools != nil {
			out = append(out, p.MCPTools)
		}
	}
	return out
}

// EnvironmentReaders is every bundle's environment readers.
func (b Built) EnvironmentReaders() []rbac.Permission {
	var out []rbac.Permission
	for _, p := range b {
		out = append(out, p.EnvironmentReaders...)
	}
	return out
}

// Init runs every bundle's Init, stopping at the first error.
func (b Built) Init(ctx context.Context) error {
	for _, p := range b {
		if p.Init == nil {
			continue
		}
		if err := p.Init(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Start starts every bundle's workers; on an error, the ones already started
// are stopped.
func (b Built) Start(ctx context.Context) error {
	for i, p := range b {
		if p.Start == nil {
			continue
		}
		if err := p.Start(ctx); err != nil {
			b[:i].Stop(ctx)
			return err
		}
	}
	return nil
}

// Stop stops every bundle's workers, last started first.
func (b Built) Stop(ctx context.Context) {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i].Stop != nil {
			b[i].Stop(ctx)
		}
	}
}
