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

package dbmigrations

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMigration055DestinationConstraints(t *testing.T) {
	g := scratchDatabase(t)
	require.NoError(t, migrateTo(g, 54))
	require.NoError(t, migrateTo(g, latestVersion))
	destination := uuid.New()
	require.NoError(t, g.Exec("INSERT INTO event_destinations (id, ou_id, name, kind) VALUES (?, 'org-a', 'Webhook', 'webhook')", destination).Error)
	require.Error(t, g.Exec("INSERT INTO event_destinations (ou_id, name, kind) VALUES ('org-a', 'webhook', 'webhook')").Error)
	require.NoError(t, g.Exec("INSERT INTO event_destinations (ou_id, name, kind) VALUES ('org-b', 'Webhook', 'webhook')").Error)
	subscription := uuid.New()
	require.NoError(t, g.Exec("INSERT INTO event_subscriptions (id, ou_id, scope, name, destination_id) VALUES (?, 'org-a', 'org', 'Alerts', ?)", subscription, destination).Error)
	require.Error(t, g.Exec("DELETE FROM event_destinations WHERE id = ?", destination).Error)
	require.NoError(t, g.Exec("DELETE FROM event_subscriptions WHERE id = ?", subscription).Error)
	require.NoError(t, g.Exec("DELETE FROM event_destinations WHERE id = ?", destination).Error)
	require.NoError(t, migrateTo(g, 55), "migration must run only once")
	var legacyTables int64
	require.NoError(t, g.Raw("SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('monitor_watches', 'notifications', 'alert_channels')").Scan(&legacyTables).Error)
	require.Zero(t, legacyTables)
}
