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

package modhost_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
)

// subscriber is a module that handles agent.created with fn.
func subscriber(name string, fn modkit.EventHandler) *testModule {
	return &testModule{manifest: manifest(name, nil, nil), activate: func(rt modkit.Runtime) error {
		rt.Subscribe(modkit.EventAgentCreated, fn)
		return nil
	}}
}

// newOrg inserts an organization for events to belong to.
func newOrg(t *testing.T, g *gorm.DB) string {
	t.Helper()
	id := uuid.Must(uuid.NewV7()).String()
	now := dbx.Now()
	require.NoError(t, g.Exec(`INSERT INTO organizations (id, handle, display_name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, "o-"+id[len(id)-12:], "Org", now, now).Error)
	return id
}

func emit(t *testing.T, h *modhost.Host, org string) {
	t.Helper()
	require.NoError(t, h.Begin(context.Background(), modkit.ResourceAgent).Emit(context.Background(), nil,
		modkit.Event{Type: modkit.EventAgentCreated, OrgID: org, Subject: "a1", Data: []byte(`{}`)}))
}

func TestEventDeliveryDoesNotWaitForASlowModule(t *testing.T) {
	eachDatabase(t, eventDeliveryDoesNotWaitForASlowModule)
}

func eventDeliveryDoesNotWaitForASlowModule(t *testing.T, d database) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := make(chan struct{})
	slowStarted := make(chan struct{}, 4)
	fast := make(chan string, 4)
	slow := subscriber("t-slow", func(ctx context.Context, _ modkit.Event) error {
		slowStarted <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return ctx.Err()
	})
	quick := subscriber("t-fast", func(_ context.Context, e modkit.Event) error {
		fast <- e.Subject
		return nil
	})
	g := d.open(t)
	h := bootWith(t, ctx, d, g, []modkit.Module{slow, quick}, func(h *modhost.Host) { h.EventDeliveryTimeout = 20 * time.Second })
	installAll(t, h, "t-slow", "t-fast")
	org := newOrg(t, g)

	emit(t, h, org)
	<-slowStarted
	emit(t, h, org)
	for range 2 {
		select {
		case <-fast:
		case <-time.After(5 * time.Second):
			t.Fatal("the fast module waited for the slow one")
		}
	}
	close(release)
}

func TestShutdownDoesNotCountAsAFailedAttempt(t *testing.T) {
	d := databases(t)[0]
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{}, 1)
	m := subscriber("t-held", func(ctx context.Context, _ modkit.Event) error {
		started <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	})
	g := d.open(t)
	h := bootWith(t, ctx, d, g, []modkit.Module{m}, nil)
	installAll(t, h, "t-held")
	emit(t, h, newOrg(t, g))
	<-started
	cancel()
	waitFor(t, "the delivery to be given back", func() bool {
		return count(t, g, `SELECT count(*) FROM module_event_deliveries WHERE module = 't-held' AND claimed_by IS NULL`) == 1
	})
	assert.EqualValues(t, 0, count(t, g, `SELECT attempts FROM module_event_deliveries WHERE module = 't-held'`))
}

func TestEventRetentionAndUninstallPurge(t *testing.T) {
	eachDatabase(t, eventRetentionAndUninstallPurge)
}

func eventRetentionAndUninstallPurge(t *testing.T, d database) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	never := func(context.Context, modkit.Event) error { return context.Canceled }
	g := d.open(t)
	h := bootWith(t, ctx, d, g, []modkit.Module{subscriber("t-keep", never), subscriber("t-gone", never)}, nil)
	installAll(t, h, "t-keep", "t-gone")
	org := newOrg(t, g)
	old := dbx.Now().Add(-modhost.EventRetention - time.Hour)
	for i, module := range []string{"t-keep", "t-keep", "t-gone"} {
		id := uuid.Must(uuid.NewV7()).String()
		created := dbx.Now()
		var failed any
		if i == 0 {
			created = old // expired, and failed long ago
			failed = old
		}
		// Together, so the drainer's sweep never sees the event without its
		// delivery. Not due, so the drainer leaves them alone.
		require.NoError(t, g.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`INSERT INTO module_events (id, org_id, type, source, data, created_at) VALUES (?, ?, ?, 'core', '{}', ?)`,
				id, org, modkit.EventAgentCreated, created).Error; err != nil {
				return err
			}
			return tx.Exec(`INSERT INTO module_event_deliveries (event_id, module, attempts, available_at, failed_at) VALUES (?, ?, 0, ?, ?)`,
				id, module, dbx.Now().Add(time.Hour), failed).Error
		}))
	}

	h.ExpireEvents(ctx, "")
	assert.EqualValues(t, 1, count(t, g, `SELECT count(*) FROM module_event_deliveries WHERE module = 't-keep'`), "the expired delivery is gone")
	assert.EqualValues(t, 2, count(t, g, `SELECT count(*) FROM module_events`), "and so is its event")

	require.NoError(t, h.Uninstall(ctx, "t-gone", false))
	assert.Zero(t, count(t, g, `SELECT count(*) FROM module_event_deliveries WHERE module = 't-gone'`), "an uninstalled module's deliveries are dropped")
	assert.EqualValues(t, 1, count(t, g, `SELECT count(*) FROM module_events`))
}

func TestModulesCannotTakeCoreResourceNames(t *testing.T) {
	for _, name := range []string{"agent", "org", "amp", "git-secret"} {
		assert.Panics(t, func() {
			modhost.NewWith(nil, "sqlite", quiet, []modkit.Module{&testModule{manifest: manifest(name, nil, nil)}})
		}, name)
	}
	// The AI Gateway's permissions are its own, so no core resource is
	// named after it.
	assert.NotPanics(t, func() {
		modhost.NewWith(nil, "sqlite", quiet, []modkit.Module{&testModule{manifest: manifest("ai-gateway", nil, nil)}})
	})
	builtin := manifest("agent", nil, nil)
	builtin.Builtin = true
	assert.NotPanics(t, func() {
		modhost.NewWith(nil, "sqlite", quiet, []modkit.Module{&testModule{manifest: builtin}})
	}, "a built-in module is part of the core")
}

// bootWith migrates and boots a host for mods on g, after configure.
func bootWith(t *testing.T, ctx context.Context, d database, g *gorm.DB, mods []modkit.Module, configure func(*modhost.Host)) *modhost.Host {
	t.Helper()
	require.NoError(t, dbmigrations.MigrateBaseline(g, d.driver))
	h := modhost.NewWith(g, d.driver, quiet, mods)
	h.PollInterval = 20 * time.Millisecond
	if configure != nil {
		configure(h)
	}
	_, err := h.Migrate(ctx)
	require.NoError(t, err)
	require.NoError(t, h.Boot(ctx))
	return h
}
