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

package aihub

import (
	"context"
	"log/slog"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/sqlitedialect"
)

// TestEverySettingIsDocumented: the manifest's Settings are exactly the keys the module reads, so the
// configuration reference stays complete.
func TestEverySettingIsDocumented(t *testing.T) {
	read := map[string]bool{}
	_, _ = webhookConfigFrom(func(key string) string { read[key] = true; return "" })

	documented := map[string]bool{}
	for _, s := range Module().Manifest().Settings {
		documented[s.Key] = true
		assert.NotEmpty(t, s.Description, s.Key)
		assert.False(t, s.Secret, "no setting holds a secret, the organizations' webhook secrets are kept per organization: "+s.Key)
	}
	var missing, stale []string
	for k := range read {
		if !documented[k] {
			missing = append(missing, k)
		}
	}
	for k := range documented {
		if !read[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	assert.Empty(t, missing, "settings the module reads but does not document")
	assert.Empty(t, stale, "documented settings the module does not read")

	// Each documented default is what the module uses when the setting isn't set.
	defaults := map[string]string{}
	for _, s := range Module().Manifest().Settings {
		defaults[s.Key] = s.Default
	}
	fromDefaults, err := webhookConfigFrom(func(key string) string { return defaults[key] })
	require.NoError(t, err)
	unset, err := webhookConfigFrom(func(string) string { return "" })
	require.NoError(t, err)
	assert.Equal(t, unset, fromDefaults)
}

// webhookSecretsDB is a SQLite database with the core's baseline and the module's tables.
func webhookSecretsDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "amp.db") + "?_pragma=foreign_keys(1)&_txlock=immediate&_time_format=sqlite"
	db, err := gorm.Open(sqlitedialect.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, dbmigrations.MigrateBaseline(db, "sqlite"))
	_, err = modhost.NewWith(db, "sqlite", slog.New(slog.DiscardHandler), []modkit.Module{Module()}).Migrate(context.Background())
	require.NoError(t, err)
	return db
}

// A secret deleted between being read and being changed is not found, as if it had been gone already: a rotation
// racing a deletion answers 404, and inserts no version for a secret that no longer exists.
func TestWebhookSecretDeletedMeanwhileIsNotFound(t *testing.T) {
	db := webhookSecretsDB(t)
	s := &webhookSecretService{db: func() *gorm.DB { return db }, now: time.Now}
	org, gone := uuid.NewString(), uuid.NewString()

	_, err := s.nextVersion(db, org, gone, "acme-portal")
	assert.ErrorIs(t, err, errWebhookSecretNotFound)
	assert.ErrorIs(t, s.lock(db, org, gone, "acme-portal"), errWebhookSecretNotFound)

	id := uuid.NewString()
	now := time.Now().UTC()
	require.NoError(t, db.Exec(`INSERT INTO mod_ai_hub_webhook_secrets (`+webhookSecretColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, org, "acme-portal", "Acme portal", nil, 1, nil, now, now).Error)
	next, err := s.nextVersion(db, org, id, "acme-portal")
	require.NoError(t, err)
	assert.Equal(t, 2, next)
	require.NoError(t, s.lock(db, org, id, "acme-portal"))
	assert.ErrorIs(t, s.lock(db, uuid.NewString(), id, "acme-portal"), errWebhookSecretNotFound,
		"another organization's secret is not found either")
}
