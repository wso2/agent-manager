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
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/db/sqlitedialect"
	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/tests/modules/testmod"
)

// mountedModule activates m on a scratch database and registers its routes
// through the registrars the server uses, returning the audit ledgers of its
// public routes and of its fixed internal routes.
func mountedModule(t *testing.T, m modkit.Module) (*modhost.Host, []audit.RouteMeta, []audit.RouteMeta) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() {
		rbac.SetCurrent(rbac.NewRegistry())
		events.SetModuleTypes(nil)
		audit.SetModuleActions(nil)
	})
	dsn := "file:" + filepath.Join(t.TempDir(), "amp.db") + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate&_time_format=sqlite"
	g, err := gorm.Open(sqlitedialect.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	require.NoError(t, dbmigrations.MigrateBaseline(g, "sqlite"))
	host := modhost.NewWith(g, "sqlite", slog.New(slog.DiscardHandler), []modkit.Module{m})
	// Every activation (the trial one included) mounts afresh, as the host
	// does; the ledgers keep each pattern's last registration.
	public, internal := map[string]audit.RouteMeta{}, map[string]audit.RouteMeta{}
	host.Attach(modhost.Deps{
		Mount: func(mux *http.ServeMux, r modhost.Route) {
			rr := middleware.NewRouteRegistrar(mux, nil, nil)
			mountModuleRoute(rr, r)
			for _, meta := range rr.Routes() {
				public[meta.Pattern] = meta
			}
		},
		MountInternal: func(mux *http.ServeMux, r modhost.Route) {
			rr := middleware.NewInternalRouteRegistrar(mux, nil)
			rr.HandleModuleInternalRoute(r.Pattern(), modulePolicy(r), r.Handler)
			for _, meta := range rr.Routes() {
				internal[meta.Pattern] = meta
			}
		},
	})
	_, err = host.Migrate(ctx)
	require.NoError(t, err)
	require.NoError(t, host.Boot(ctx))
	_, err = host.Install(ctx, m.Manifest().Name, false)
	require.NoError(t, err)
	require.Equal(t, []string{m.Manifest().Name}, host.Active())
	values := func(m map[string]audit.RouteMeta) []audit.RouteMeta {
		out := make([]audit.RouteMeta, 0, len(m))
		for _, v := range m {
			out = append(out, v)
		}
		return out
	}
	return host, values(public), values(internal)
}

// TestModuleRoutesAreAudited holds module routes to the core's audit
// invariant: every route that changes state is recorded under an action, and
// that action is one of the module's permissions or declared audit actions.
func TestModuleRoutesAreAudited(t *testing.T) {
	m := testmod.Module()
	mf := m.Manifest()
	_, public, internal := mountedModule(t, m)
	declared := map[audit.Action]bool{}
	for _, p := range mf.Permissions {
		declared[audit.Action(mf.PermissionName(p.Name))] = true
	}
	for _, a := range mf.AuditActions {
		declared[audit.Action(mf.Name+":"+a.Name)] = true
	}
	require.NotEmpty(t, public)
	require.NotEmpty(t, internal)
	for _, meta := range append(slices.Clone(public), internal...) {
		if meta.Method != http.MethodGet {
			assert.True(t, meta.Audited, "%s changes state but is not audited", meta.Pattern)
		}
		if meta.Audited {
			assert.True(t, declared[meta.Action], "%s is audited as %q, which the manifest does not declare", meta.Pattern, meta.Action)
		}
	}
	byPattern := map[string]audit.RouteMeta{}
	for _, meta := range public {
		byPattern[meta.Pattern] = meta
	}
	archive := byPattern["POST /m/testmod/widgets/{name}/archive"]
	assert.Equal(t, audit.Action("testmod:widget-archive"), archive.Action, "Audit names the action instead of the permission")
	read := byPattern["GET /m/testmod/secrets/{name}"]
	assert.True(t, read.Audited, "Audit on a read records it")
	assert.Equal(t, audit.ClassCredential, read.Action.Class(), "the manifest classifies its actions")
	assert.False(t, byPattern["GET /m/testmod/tags"].Audited, "other reads are not recorded")
}

// moduleSpec is the part of a module's OpenAPI document the invariant reads.
type moduleSpec struct {
	Info struct {
		Module string `yaml:"x-amp-module"`
	} `yaml:"info"`
	Servers []struct {
		URL string `yaml:"url"`
	} `yaml:"servers"`
	Paths map[string]map[string]any `yaml:"paths"`
}

// checkModuleSpec checks that a module's own OpenAPI document describes
// exactly the routes it mounts with Handle, relative to /m/<name>.
func checkModuleSpec(t *testing.T, m modkit.Module, routes []modhost.Route) {
	t.Helper()
	name := m.Manifest().Name
	d, ok := m.(modkit.APIDescriber)
	require.True(t, ok, "%s mounts routes but has no OpenAPI document", name)
	var spec moduleSpec
	require.NoError(t, yaml.Unmarshal(d.OpenAPI(), &spec))
	assert.Equal(t, name, spec.Info.Module, "info.x-amp-module")
	require.Len(t, spec.Servers, 1)
	assert.Equal(t, "/m/"+name, spec.Servers[0].URL)

	var documented, mounted []string
	for path, item := range spec.Paths {
		for method := range item {
			switch method {
			case "get", "put", "post", "delete", "patch":
				documented = append(documented, strings.ToUpper(method)+" "+path)
			}
		}
	}
	for _, r := range routes {
		mounted = append(mounted, r.Method+" "+r.Path)
	}
	sort.Strings(documented)
	sort.Strings(mounted)
	assert.Equal(t, mounted, documented, "%s's OpenAPI document must describe exactly the routes it mounts", name)
}

func TestModuleSpecMatchesRoutes(t *testing.T) {
	m := testmod.Module()
	host, _, _ := mountedModule(t, m)
	checkModuleSpec(t, m, host.Routes(testmod.Name))
}

// TestCoreSpecHasNoModulePaths keeps module APIs out of the core's document:
// each module describes its routes in its own.
func TestCoreSpecHasNoModulePaths(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "docs", "api_v1_openapi.yaml"))
	require.NoError(t, err)
	var spec struct {
		Info struct {
			Module string `yaml:"x-amp-module"`
		} `yaml:"info"`
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &spec))
	require.NotEmpty(t, spec.Paths)
	assert.Contains(t, []string{"", "core"}, spec.Info.Module)
	for path, item := range spec.Paths {
		assert.False(t, path == "/m" || strings.HasPrefix(path, "/m/"), "the core spec documents %s, a module path", path)
		for method, op := range item {
			var ext struct {
				Module string `yaml:"x-amp-module"`
			}
			if op.Kind == yaml.MappingNode && op.Decode(&ext) == nil {
				assert.Contains(t, []string{"", "core"}, ext.Module, "%s %s belongs to module %s", method, path, ext.Module)
			}
		}
	}
}
