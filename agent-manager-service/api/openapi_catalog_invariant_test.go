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

package api

import (
	"go/ast"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/apicatalog"
	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/modules/gateway"
	"github.com/wso2/agent-manager/agent-manager-service/modules/legacy"
)

// The OpenAPI spec and the route table must describe the same API, and each
// operation must name the module that serves it (x-amp-module), because
// GET /api/v1/openapi.{yaml,json} serves only the operations of the active
// modules. These tests hold the spec to the code in both directions:
//
//   - every route has a spec operation, and every spec operation a route;
//   - an operation's x-amp-module is the module whose package registers it;
//   - an operation is x-amp-not-served exactly when its route answers 501.
//
// Ownership comes from the registration itself: the core's /api/v1 routes are
// the ones registerAPIRoutes adds with no bundle, a module's are the ones its
// builtin.Parts adds. Routes registered straight on the root mux are read from
// the source, and belong to the module of the package that registers them.

// specFile is the API contract, relative to this package.
var specFile = filepath.Join("..", "docs", "api_v1_openapi.yaml")

// routeModuleOfPackage maps each package in routePackages to the module that
// owns the routes it registers.
var routeModuleOfPackage = map[string]string{
	".":                        apicatalog.CoreModule,
	filepath.Join("..", "mcp"): apicatalog.CoreModule,
	filepath.Join("..", "modules", "gateway", "gwapi"):    gateway.Name,
	filepath.Join("..", "modules", "legacy", "legacyapi"): legacy.Name,
}

// unspecifiedRoutes are the registrations that deliberately have no operation
// in the spec, keyed by the pattern as registered. Keep it short: a route a
// client can call belongs in the spec.
var unspecifiedRoutes = map[string]string{
	// Not part of the REST API.
	"GET /healthz":         "liveness probe for the container runtime, not a client API.",
	"POST /auth/login":     "file-mode sign-in form endpoint; it issues the bearer token every spec operation requires.",
	"OPTIONS /auth/login":  "CORS preflight of the sign-in endpoint above.",
	"/mcp":                 "the MCP server (JSON-RPC over HTTP); its tools are described by MCP tools/list, not OpenAPI.",
	"/mcp/":                "the MCP server, as above.",
	"GET /api/docs":        "the HTML API reference that renders this spec.",
	"GET /api/docs/{file}": "the reference viewer's script and licence texts, embedded in the binary.",
	"GET /.well-known/oauth-protected-resource":     "RFC 9728 discovery document, defined by that RFC rather than by this API.",
	"GET /.well-known/oauth-protected-resource/mcp": "RFC 9728 discovery document for the MCP resource.",

	// Mount points of sub-routers, not endpoints.
	"/":        "the console's static files and SPA fallback.",
	"/api/v1/": "mount point of the authenticated API router, whose routes are checked one by one.",
	"/m/":      "mount point of runtime modules' routes (/api/v1/m/<module>/...), which a module declares in its manifest and the module host serves; they are not in this spec.",

	// Routes this version answers with 501 that the spec never described.
	// They are left out of the served spec anyway (see x-amp-not-served).
	"GET /orgs/{orgName}/agent-kinds/{kindName}/agents":        "agent kinds are not served in this version (501).",
	"POST /publisher/monitors/{monitorId}/runs/{runId}/scores": "monitor score publishing is not served in this version (501); it takes a publisher credential, not a user token.",
}

// internalSurfaceFunctions register routes on the internal listener
// (/api/internal/v1, gateway WebSocket), which gateways and module workloads
// call. That listener is not exposed to API clients and is not in this spec.
var internalSurfaceFunctions = map[string]bool{
	"MakeInternalHTTPHandlerWithModules": true,
	"RegisterWebSocketRoutes":            true,
	"RegisterGatewayInternalRoutes":      true,
}

var pathParamPattern = regexp.MustCompile(`\{[^}]+\}`)

// operationKey identifies an operation by method and path shape. Path
// parameter names are ignored: renaming one in the spec changes generated
// code, and the router does not care what it is called.
func operationKey(method, path string) string {
	return strings.ToUpper(method) + " " + pathParamPattern.ReplaceAllString(path, "{}")
}

type specOperation struct {
	path      string
	method    string
	module    string
	notServed bool
}

var httpMethods = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true,
	"options": true, "head": true, "patch": true, "trace": true,
}

func loadSpecOperations(t *testing.T) map[string]specOperation {
	t.Helper()
	raw, err := os.ReadFile(specFile)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	ops := map[string]specOperation{}
	for path, item := range doc.Paths {
		for method, node := range item {
			if !httpMethods[method] {
				continue
			}
			var op struct {
				Module    string `yaml:"x-amp-module"`
				NotServed bool   `yaml:"x-amp-not-served"`
			}
			if err := node.Decode(&op); err != nil {
				t.Fatalf("decode %s %s: %v", method, path, err)
			}
			key := operationKey(method, path)
			if prev, dup := ops[key]; dup {
				t.Errorf("spec operations %s %s and %s %s differ only in path parameter names",
					prev.method, prev.path, method, path)
			}
			ops[key] = specOperation{path: path, method: method, module: op.Module, notServed: op.NotServed}
		}
	}
	if len(ops) < 200 {
		t.Fatalf("parsed only %d spec operations; the parser is broken, not the spec", len(ops))
	}
	return ops
}

// registeredRoute is a route and the module that registers it.
type registeredRoute struct {
	pattern string
	module  string
}

// registeredRoutes returns every route a client can reach, by operation key:
// the /api/v1 routes of the core and of each built-in module, registered for
// real, and the routes registered on the root mux, read from the source.
func registeredRoutes(t *testing.T) map[string]registeredRoute {
	t.Helper()
	routes := map[string]registeredRoute{}
	add := func(key string, r registeredRoute) {
		if prev, dup := routes[key]; dup {
			t.Errorf("%s is registered by both %s (%q) and %s (%q)", key, prev.module, prev.pattern, r.module, r.pattern)
		}
		routes[key] = r
	}

	params := stubAppParams(t)
	ledger := func(module string, register func(rr *middleware.RouteRegistrar)) {
		rr := middleware.NewRouteRegistrar(http.NewServeMux(), nil, audit.NewNoopRecorder())
		register(rr)
		if len(rr.Routes()) == 0 {
			t.Fatalf("module %s registered no /api/v1 routes; the walk is broken", module)
		}
		for _, m := range rr.Routes() {
			add(operationKey(m.Method, m.Path), registeredRoute{pattern: m.Pattern, module: module})
		}
	}
	ledger(apicatalog.CoreModule, func(rr *middleware.RouteRegistrar) {
		registerAPIRoutes(rr, params, nil)
		registerUIManifestRoute(rr, nil)
	})
	for name, parts := range stubModuleParts(t, params) {
		ledger(name, parts.Routes)
	}

	for _, r := range rootMuxRoutes(t) {
		add(r.key, registeredRoute{pattern: r.pattern, module: r.module})
	}
	return routes
}

type rootRoute struct {
	key, pattern, module string
}

// rootMuxRoutes reads the routes registered straight on the public root mux
// (mux.Handle / mux.HandleFunc), skipping the internal listener's.
func rootMuxRoutes(t *testing.T) []rootRoute {
	t.Helper()
	var out []rootRoute
	forEachRouteRegistration(t, func(r routeRegistration) {
		if (r.registrar != "Handle" && r.registrar != "HandleFunc") || internalSurfaceFunctions[r.function] {
			return
		}
		method, path := "", r.pattern
		if m, p, ok := strings.Cut(r.pattern, " "); ok {
			method, path = m, p
		}
		if method == "" {
			// A method-less pattern is how a GET endpoint lets its CORS
			// preflight through; the handler enforces GET.
			method = http.MethodGet
		}
		if path != "/api/v1/" {
			path = strings.TrimPrefix(path, "/api/v1")
		}
		out = append(out, rootRoute{key: operationKey(method, path), pattern: r.pattern, module: r.module})
	})
	return out
}

// TestSpecDescribesEveryRoute fails when a route has no spec operation (and
// is not in unspecifiedRoutes), when a spec operation has no route, and when
// an allowlist entry no longer matches a route.
func TestSpecDescribesEveryRoute(t *testing.T) {
	ops := loadSpecOperations(t)
	routes := registeredRoutes(t)

	allowed := map[string]bool{}
	for key, r := range routes {
		if _, ok := ops[key]; ok {
			continue
		}
		if _, ok := unspecifiedRoutes[r.pattern]; ok {
			allowed[r.pattern] = true
			continue
		}
		t.Errorf("%s route %q has no operation in %s.\n\n"+
			"Add the operation with `x-amp-module: %s`, or, if clients must not call it, add the pattern "+
			"to unspecifiedRoutes with the reason.", r.module, r.pattern, specFile, r.module)
	}
	for key, op := range ops {
		if _, ok := routes[key]; !ok {
			t.Errorf("spec operation %s %s has no registered route. Register it or remove it from the spec.",
				strings.ToUpper(op.method), op.path)
		}
	}
	for pattern, reason := range unspecifiedRoutes {
		if !allowed[pattern] {
			t.Errorf("stale unspecifiedRoutes entry %q: no unspecified route has that pattern any more. "+
				"Remove it.\nIt was allowlisted because: %s", pattern, reason)
		}
	}
}

// TestSpecOperationsNameTheirModule fails when an operation's x-amp-module is
// missing or names a module other than the one that registers its route.
func TestSpecOperationsNameTheirModule(t *testing.T) {
	ops := loadSpecOperations(t)
	routes := registeredRoutes(t)

	for key, op := range ops {
		where := strings.ToUpper(op.method) + " " + op.path
		if op.module == "" {
			t.Errorf("spec operation %s has no x-amp-module", where)
			continue
		}
		r, ok := routes[key]
		if !ok {
			continue // reported by TestSpecDescribesEveryRoute
		}
		if op.module != r.module {
			t.Errorf("spec operation %s says x-amp-module: %s, but %s registers it (%q)",
				where, op.module, r.module, r.pattern)
		}
	}
}

// TestSpecMarksNotServedOperations fails unless x-amp-not-served is set on
// exactly the operations whose route answers 501 (a notImplemented handler).
func TestSpecMarksNotServedOperations(t *testing.T) {
	ops := loadSpecOperations(t)
	routes := registeredRoutes(t)
	stubs := notImplementedRoutes(t)

	for key, pattern := range stubs {
		if _, ok := routes[key]; !ok {
			t.Errorf("found a 501 registration %q that is not among the registered routes; "+
				"the pattern resolver is out of step with the registrar", pattern)
		}
	}
	for key, op := range ops {
		_, stub := stubs[key]
		where := strings.ToUpper(op.method) + " " + op.path
		switch {
		case stub && !op.notServed:
			t.Errorf("%s answers 501 but its spec operation lacks `x-amp-not-served: true`", where)
		case !stub && op.notServed:
			t.Errorf("%s is served but its spec operation says `x-amp-not-served: true`", where)
		}
	}
}

// notImplementedRoutes returns the routes registered with a notImplemented
// handler, by operation key. Patterns built from local variables and constants and the
// route(method, path) helper are resolved; a 501 registration whose pattern
// cannot be resolved fails the test.
func notImplementedRoutes(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, dir := range routePackages {
		forEachFile(t, dir, func(_ *token.FileSet, file *ast.File) {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				env := map[string]string{}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					switch n := n.(type) {
					case *ast.ValueSpec:
						for i, id := range n.Names {
							if i < len(n.Values) {
								if v, ok := resolveString(n.Values[i], env); ok {
									env[id.Name] = v
								}
							}
						}
					case *ast.AssignStmt:
						if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
							if id, ok := n.Lhs[0].(*ast.Ident); ok {
								if v, ok := resolveString(n.Rhs[0], env); ok {
									env[id.Name] = v
								}
							}
						}
					case *ast.CallExpr:
						sel, ok := n.Fun.(*ast.SelectorExpr)
						if !ok || !isRouteRegistrar(sel.Sel.Name) || !hasNotImplementedArg(n) {
							return true
						}
						pattern, ok := resolveString(n.Args[0], env)
						if !ok {
							t.Errorf("%s: cannot resolve the pattern %s of a 501 route", fn.Name.Name, renderExpr(n.Args[0]))
							return true
						}
						method, path, _ := strings.Cut(pattern, " ")
						out[operationKey(method, path)] = pattern
					}
					return true
				})
			}
		})
	}
	if len(out) < 40 {
		t.Fatalf("found only %d 501 routes; the walk is broken", len(out))
	}
	return out
}

func hasNotImplementedArg(call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		if id, ok := arg.(*ast.Ident); ok && id.Name == "notImplemented" {
			return true
		}
	}
	return false
}

// resolveString evaluates a string expression built from literals, local
// variables in env, + and the route(method, path) helper.
func resolveString(expr ast.Expr, env map[string]string) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.Ident:
		s, ok := env[e.Name]
		return s, ok
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		x, okX := resolveString(e.X, env)
		y, okY := resolveString(e.Y, env)
		return x + y, okX && okY
	case *ast.CallExpr:
		if id, ok := e.Fun.(*ast.Ident); ok && id.Name == "route" && len(e.Args) == 2 {
			m, okM := resolveString(e.Args[0], env)
			p, okP := resolveString(e.Args[1], env)
			return m + " " + p, okM && okP
		}
	}
	return "", false
}
