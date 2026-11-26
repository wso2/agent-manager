//go:build integration

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
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc/providers/unsupported"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// A run that only migrates, or only waits for another process's migrations,
// returns without starting anything once the schema is current.
func TestRunWithoutServerReturnsOnceTheSchemaIsCurrent(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(unsupported.NewProvider(), Options{Migrate: true})
		Run(unsupported.NewProvider(), Options{WaitForMigrations: true})
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Minute):
		t.Fatal("Run did not return")
	}
	require.NoError(t, waitForModuleSchema(context.Background(), modhost.New(db.GetDB(), config.GetConfig().Database.Driver, slog.Default())))
}

// The server boots from configuration alone: it creates the configured
// organization, serves health and the API on the configured ports, starts the
// deployment's background workers with an audit-capable context, and on a
// shutdown signal drains and stops them before returning.
//
// The signal handler can be installed once per process, so this is the only
// test in the package that starts the server.
func TestRunServesAndShutsDownOnSignal(t *testing.T) {
	cfg := config.GetConfig()
	// The server's workers act on every org in the database. On PostgreSQL
	// the packages share one database in parallel, so they would place and
	// render other packages' gateway rows mid-test; SQLite runs them serially.
	if cfg.Database.Driver != "sqlite" {
		t.Skip("boots every background worker; runs on SQLite, where packages run serially")
	}
	saved := *cfg
	t.Cleanup(func() { *cfg = saved })

	port := freeTCPPort(t)
	ouID := uuid.Must(uuid.NewV7()).String()
	handle := "boot-" + ouID[len(ouID)-8:]
	cfg.ServerHost = "127.0.0.1"
	cfg.ServerPort = port
	cfg.InternalServer.Host = "127.0.0.1"
	cfg.InternalServer.Port = freeTCPPort(t)
	cfg.InternalServer.TLSEnabled = false
	cfg.DefaultOrg = config.DefaultOrgConfig{OUID: ouID, Handle: handle}
	// The trial organization is set up at boot only with file sign-in, which needs a user file.
	cfg.AuthMode = config.AuthModeFile
	cfg.FileAuth.UsersPath = filepath.Join(t.TempDir(), "users.yaml")
	require.NoError(t, os.WriteFile(cfg.FileAuth.UsersPath, []byte("organization: {handle: "+handle+", uuid: "+ouID+"}\n"+
		"users:\n  - {username: admin, passwordHash: \"$2a$04$abcdefghijklmnopqrstuuJ2s0ZQnWUoK3qz5bH4n0Ql6rU7lXy2W\", roles: ["+rbac.RoleAdmin+"]}\n"), 0o600))
	cfg.LogLevel = "WARN"
	t.Cleanup(func() {
		g := db.GetDB()
		g.Where("org_id = ?", ouID).Delete(&models.Environment{})
	})

	var (
		startedWithRecorder atomic.Bool
		decorated           atomic.Bool
		stopped             atomic.Bool
		extraRoutes         atomic.Bool
	)
	type decoratedKey struct{}
	opts := Options{
		Server:  true,
		Migrate: true,
		BackgroundContextDecorator: func(ctx context.Context) context.Context {
			return context.WithValue(ctx, decoratedKey{}, true)
		},
		StartBackground: func(ctx context.Context, deps *wiring.AppParams) (func(), error) {
			// A worker's audit emits must reach the real recorder, not the
			// "not installed" fallback that drops them.
			startedWithRecorder.Store(audit.FromContext(ctx) == deps.AuditRecorder)
			decorated.Store(ctx.Value(decoratedKey{}) == true)
			return func() { stopped.Store(true) }, nil
		},
		ExtraAPIRoutes:           func(*http.ServeMux, *wiring.AppParams) { extraRoutes.Store(true) },
		AgentThunderProvisioning: services.DBBackedAgentThunderProvisioning(),
	}

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		Run(unsupported.NewProvider(), opts)
	}()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	require.Eventually(t, func() bool {
		resp, err := http.Get(base + "/healthz")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 90*time.Second, 200*time.Millisecond, "the server never became healthy")

	resp, err := http.Get(base + "/api/v1/orgs")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "the API is served behind authentication")

	internal, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", cfg.InternalServer.Port))
	if err == nil {
		_ = internal.Body.Close()
	}
	assert.NoError(t, err, "the internal server listens on its own port")

	var envs int64
	require.NoError(t, db.GetDB().Model(&models.Environment{}).Where("org_id = ?", ouID).Count(&envs).Error)
	assert.Equal(t, int64(1), envs, "the configured organization gets its default environment at boot")

	assert.True(t, startedWithRecorder.Load())
	assert.True(t, decorated.Load())
	assert.True(t, extraRoutes.Load())
	assert.False(t, stopped.Load())

	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGINT))
	select {
	case <-returned:
	case <-time.After(60 * time.Second):
		t.Fatal("Run did not return after the shutdown signal")
	}
	assert.True(t, stopped.Load(), "the background workers are stopped on shutdown")
	_, err = http.Get(base + "/healthz")
	assert.Error(t, err, "the server no longer listens")
}

// The declared modules list is applied to the modules compiled in; naming one
// that is not is refused. A host with no modules keeps the shared database's
// module state untouched.
func TestReconcileModulesAppliesTheDeclaredList(t *testing.T) {
	host := modhost.NewWith(db.GetDB(), config.GetConfig().Database.Driver, slog.Default(), nil)
	ctx := context.Background()

	t.Setenv(config.ModulesEnv, "none")
	assert.NoError(t, ReconcileModules(ctx, host, nil), "nothing to change")

	t.Setenv(config.ModulesEnv, "no-such-module")
	assert.Error(t, ReconcileModules(ctx, host, nil), "a module this binary does not have is refused")

	t.Setenv(config.ModulesEnv, "Bad Name")
	assert.Error(t, ReconcileModules(ctx, host, nil), "an invalid list is refused before anything changes")

	t.Setenv(config.ModulesEnv, "")
	assert.NoError(t, ReconcileModules(ctx, host, nil), "no list and no defaults installs nothing")
}
