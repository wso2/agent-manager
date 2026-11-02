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
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
)

// scratchTimeZone differs from UTC so the performed_at backfill is checked against the session zone, not UTC.
const scratchTimeZone = "Asia/Colombo"

// scratchDatabase creates an empty database next to the test database and drops it when the test ends.
func scratchDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	admin := db.DB(context.Background())
	name := "migration051_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, admin.Exec(`CREATE DATABASE `+name).Error)
	require.NoError(t, admin.Exec(`ALTER DATABASE `+name+` SET timezone TO '`+scratchTimeZone+`'`).Error)

	scratch, err := gorm.Open(postgres.Open(scratchDSN(name)), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := scratch.DB(); err == nil {
			_ = sqlDB.Close()
		}
		require.NoError(t, admin.Exec(`DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`).Error)
	})
	return scratch
}

func scratchDSN(dbName string) string {
	cfg := config.GetConfig().POSTGRESQL
	params := url.Values{}
	if cfg.SSLMode != "" {
		params.Set("sslmode", cfg.SSLMode)
	}
	if cfg.SSLRootCert != "" {
		params.Set("sslrootcert", cfg.SSLRootCert)
	}
	return (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:     "/" + dbName,
		RawQuery: params.Encode(),
	}).String()
}

func exec(t *testing.T, g *gorm.DB, sql string, args ...any) {
	t.Helper()
	require.NoError(t, g.Exec(sql, args...).Error, sql)
}

// seed050 is a database at 050 holding every row shape migration 051 has to carry over.
type seed050 struct {
	gateway uuid.UUID

	deployedArtifact, undeployedArtifact uuid.UUID
	archivedDeployment, liveDeployment   uuid.UUID
	undeployedDeployment                 uuid.UUID
	statusUpdatedAt                      string

	pendingNoCard, failedPlatform, external, artifactGone, dupOlder, dupNewer uuid.UUID
	pendingNoCardArtifact, failedPlatformArtifact, dupArtifact                uuid.UUID
	externalArtifact, published, publishedArtifact                            uuid.UUID
	failedPlatformAgentUUID, externalAgentUUID                                uuid.UUID
	failedPlatformCard, externalCard, orphanCard                              uuid.UUID
}

const seedOU = "ou-051"

func seedAt050(t *testing.T, g *gorm.DB) seed050 {
	t.Helper()
	s := seed050{
		gateway:                 uuid.New(),
		deployedArtifact:        uuid.New(),
		undeployedArtifact:      uuid.New(),
		archivedDeployment:      uuid.New(),
		liveDeployment:          uuid.New(),
		undeployedDeployment:    uuid.New(),
		statusUpdatedAt:         "2026-10-01 12:34:56.789123",
		pendingNoCard:           uuid.New(),
		failedPlatform:          uuid.New(),
		external:                uuid.New(),
		artifactGone:            uuid.New(),
		dupOlder:                uuid.New(),
		dupNewer:                uuid.New(),
		pendingNoCardArtifact:   uuid.New(),
		failedPlatformArtifact:  uuid.New(),
		dupArtifact:             uuid.New(),
		failedPlatformAgentUUID: uuid.New(),
		externalAgentUUID:       uuid.New(),
		failedPlatformCard:      uuid.New(),
		externalCard:            uuid.New(),
		orphanCard:              uuid.New(),
		externalArtifact:        uuid.New(),
		published:               uuid.New(),
		publishedArtifact:       uuid.New(),
	}

	exec(t, g, `INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id)
		VALUES (?, 'gw', 'gw', 'gw.local', 'both', ?)`, s.gateway, seedOU)

	artifact := func(id uuid.UUID, handle, kind string) {
		exec(t, g, `INSERT INTO artifacts (uuid, handle, name, version, kind, ou_id) VALUES (?, ?, ?, 'v1', ?, ?)`,
			id, handle, handle, kind, seedOU)
	}
	artifact(s.deployedArtifact, "provider-a", "LlmProvider")
	artifact(s.undeployedArtifact, "provider-b", "LlmProvider")
	for id, handle := range map[uuid.UUID]string{
		s.pendingNoCardArtifact:  "proj/pending/env",
		s.failedPlatformArtifact: "proj/failed/env",
		s.dupArtifact:            "proj/dup/env",
		s.externalArtifact:       "proj/external/env",
		s.publishedArtifact:      "proj/published/env",
	} {
		artifact(id, handle, "Agent")
	}

	deployment := func(id, artifactID uuid.UUID, createdAt string) {
		exec(t, g, `INSERT INTO deployments (deployment_id, name, artifact_uuid, gateway_uuid, content, ou_id, created_at)
			VALUES (?, 'd', ?, ?, 'x', ?, ?)`, id, artifactID, s.gateway, seedOU, createdAt)
	}
	deployment(s.archivedDeployment, s.deployedArtifact, "2026-09-01 00:00:00")
	deployment(s.liveDeployment, s.deployedArtifact, "2026-09-02 00:00:00")
	deployment(s.undeployedDeployment, s.undeployedArtifact, "2026-09-03 00:00:00")
	exec(t, g, `INSERT INTO deployment_status (artifact_uuid, gateway_uuid, deployment_id, status, updated_at, ou_id)
		VALUES (?, ?, ?, 'DEPLOYED', ?, ?), (?, ?, ?, 'UNDEPLOYED', ?, ?)`,
		s.deployedArtifact, s.gateway, s.liveDeployment, s.statusUpdatedAt, seedOU,
		s.undeployedArtifact, s.gateway, s.undeployedDeployment, s.statusUpdatedAt, seedOU)
	// Agents already deployed whose publication then went pending or failed: a redeploy is owed.
	for _, agentArtifact := range []uuid.UUID{s.failedPlatformArtifact, s.externalArtifact, s.publishedArtifact} {
		agentDeployment := uuid.New()
		deployment(agentDeployment, agentArtifact, "2026-09-04 00:00:00")
		exec(t, g, `INSERT INTO deployment_status (artifact_uuid, gateway_uuid, deployment_id, status, updated_at, ou_id)
			VALUES (?, ?, ?, 'DEPLOYED', ?, ?)`, agentArtifact, s.gateway, agentDeployment, s.statusUpdatedAt, seedOU)
	}

	exec(t, g, `INSERT INTO association_mappings (artifact_uuid, resource_uuid, association_type, ou_id)
		VALUES (?, ?, 'gateway', ?)`, s.deployedArtifact, s.gateway, seedOU)

	publication := func(id uuid.UUID, agent, envName string, envUUID, artifactID uuid.UUID, agentUUID *uuid.UUID,
		source, status, updatedAt string,
	) {
		exec(t, g, `INSERT INTO a2a_publications
			(id, ou_id, project_name, agent_name, environment_name, environment_uuid, artifact_uuid, agent_uuid,
			 source, status, attempt_count, last_error, published_upstream_url, created_at, updated_at)
			VALUES (?, ?, 'proj', ?, ?, ?, ?, ?, ?, ?, 3, 'boom', 'http://up', ?, ?)`,
			id, seedOU, agent, envName, envUUID, artifactID, agentUUID, source, status, updatedAt, updatedAt)
	}
	failedEnv, externalEnv := uuid.New(), uuid.New()
	publication(s.pendingNoCard, "pending", "dev", uuid.New(), s.pendingNoCardArtifact, nil,
		"platform", "pending", "2026-10-01 00:00:00+00")
	publication(s.failedPlatform, "failed", "dev", failedEnv, s.failedPlatformArtifact, &s.failedPlatformAgentUUID,
		"platform", "failed", "2026-10-01 00:00:00+00")
	publication(s.external, "external", "dev", externalEnv, s.externalArtifact, &s.externalAgentUUID,
		"external", "pending", "2026-10-01 00:00:00+00")
	publication(s.published, "published", "dev", uuid.New(), s.publishedArtifact, nil,
		"platform", "published", "2026-10-01 00:00:00+00")
	publication(s.artifactGone, "gone", "dev", uuid.New(), uuid.New(), nil,
		"platform", "published", "2026-10-01 00:00:00+00")
	// A same-agent pair left by an environment recreated under a new name; both point at one artifact.
	publication(s.dupOlder, "dup", "old-dev", uuid.New(), s.dupArtifact, nil,
		"platform", "published", "2026-10-01 00:00:00+00")
	publication(s.dupNewer, "dup", "dev", uuid.New(), s.dupArtifact, nil,
		"platform", "pending", "2026-10-02 00:00:00+00")

	card := func(id uuid.UUID, agent string, envUUID uuid.UUID, source string) {
		exec(t, g, `INSERT INTO a2a_agent_cards (id, ou_id, project_name, agent_name, environment_name, environment_uuid, source)
			VALUES (?, ?, 'proj', ?, 'dev', ?, ?)`, id, seedOU, agent, envUUID, source)
	}
	card(s.failedPlatformCard, "failed", failedEnv, "platform")
	card(s.externalCard, "external", externalEnv, "external")
	card(s.orphanCard, "nobody", uuid.New(), "platform")

	gwState := func(id string) {
		exec(t, g, `INSERT INTO eventhub_gateway_states (gateway_id, version_id) VALUES (?, 'v')`, id)
	}
	gwState("gw-1")
	gwState("gw-2")
	gwState("gw-idle")
	event := func(eventID, gatewayID, processedAt string) {
		exec(t, g, `INSERT INTO eventhub_events
			(event_id, gateway_id, processed_timestamp, originated_timestamp, entity_type, action, entity_id, event_data)
			VALUES (?, ?, ?, ?, 'llmprovider', 'CREATE', 'e', '{}')`, eventID, gatewayID, processedAt, processedAt)
	}
	// Inserted out of order; equal timestamps break their tie on event_id.
	event("e-3", "gw-1", "2026-10-01 00:00:03")
	event("e-1", "gw-1", "2026-10-01 00:00:01")
	event("e-2b", "gw-1", "2026-10-01 00:00:02")
	event("e-2a", "gw-1", "2026-10-01 00:00:02")
	event("f-2", "gw-2", "2026-10-01 00:00:02")
	event("f-1", "gw-2", "2026-10-01 00:00:05")
	return s
}

func migratedFrom050(t *testing.T) (*gorm.DB, seed050) {
	t.Helper()
	g := scratchDatabase(t)
	require.NoError(t, migrateTo(g, 50))
	s := seedAt050(t, g)
	require.NoError(t, migrateTo(g, 51))
	return g, s
}

func TestMigration051_DeploymentStatus(t *testing.T) {
	g, s := migratedFrom050(t)

	type statusRow struct {
		Status        string
		StatusDesired string
		StatusReason  string
		PerformedAt   time.Time
		AttemptCount  int
		RenderReason  string
		NextAttemptAt *time.Time
	}
	read := func(artifactID uuid.UUID) statusRow {
		var row statusRow
		require.NoError(t, g.Raw(`SELECT status, status_desired, status_reason, performed_at, attempt_count,
			render_reason, next_attempt_at FROM deployment_status WHERE artifact_uuid = ?`, artifactID).Scan(&row).Error)
		return row
	}
	zone, err := time.LoadLocation(scratchTimeZone)
	require.NoError(t, err)
	wantPerformedAt := time.Date(2026, 10, 1, 12, 34, 56, 789_000_000, zone)

	t.Run("settled rows keep their status as the desired one, with performed_at in milliseconds", func(t *testing.T) {
		deployed, undeployed := read(s.deployedArtifact), read(s.undeployedArtifact)
		assert.Equal(t, "DEPLOYED", deployed.Status)
		assert.Equal(t, "DEPLOYED", deployed.StatusDesired)
		assert.Equal(t, "UNDEPLOYED", undeployed.Status)
		assert.Equal(t, "UNDEPLOYED", undeployed.StatusDesired)
		for _, row := range []statusRow{deployed, undeployed} {
			assert.True(t, wantPerformedAt.Equal(row.PerformedAt), "performed_at %s, want %s", row.PerformedAt, wantPerformedAt)
			assert.Empty(t, row.StatusReason)
			assert.Empty(t, row.RenderReason)
			assert.Zero(t, row.AttemptCount)
			assert.Nil(t, row.NextAttemptAt)
		}
	})

	t.Run("a deployed agent whose publication was pending or failed is due for a redeploy", func(t *testing.T) {
		for _, artifactID := range []uuid.UUID{s.failedPlatformArtifact, s.externalArtifact} {
			row := read(artifactID)
			assert.Equal(t, "DEPLOYED", row.Status)
			require.NotNil(t, row.NextAttemptAt)
			assert.False(t, row.NextAttemptAt.After(time.Now()))
			assert.Zero(t, row.AttemptCount)
			assert.Empty(t, row.RenderReason)
		}
		assert.Nil(t, read(s.publishedArtifact).NextAttemptAt, "a published agent owes nothing")
	})

	t.Run("the archived deployment survives with no status row", func(t *testing.T) {
		var count int64
		require.NoError(t, g.Raw(`SELECT count(*) FROM deployments d WHERE d.deployment_id = ?
			AND NOT EXISTS (SELECT 1 FROM deployment_status s WHERE s.deployment_id = d.deployment_id)`,
			s.archivedDeployment).Scan(&count).Error)
		assert.EqualValues(t, 1, count)
	})

	insertStatus := func(t *testing.T, status, desired string, deploymentID *uuid.UUID) error {
		t.Helper()
		artifactID := uuid.New()
		exec(t, g, `INSERT INTO artifacts (uuid, handle, name, version, kind, ou_id) VALUES (?, ?, ?, 'v1', 'LlmProvider', ?)`,
			artifactID, artifactID.String(), artifactID.String(), seedOU)
		if deploymentID != nil {
			*deploymentID = uuid.New()
			exec(t, g, `INSERT INTO deployments (deployment_id, name, artifact_uuid, gateway_uuid, content, ou_id)
				VALUES (?, 'd', ?, ?, 'x', ?)`, *deploymentID, artifactID, s.gateway, seedOU)
		}
		return g.Exec(`INSERT INTO deployment_status
			(artifact_uuid, gateway_uuid, deployment_id, status, status_desired, performed_at, ou_id)
			VALUES (?, ?, ?, ?, ?, NOW(), ?)`, artifactID, s.gateway, deploymentID, status, desired, seedOU).Error
	}
	withID := func() *uuid.UUID { return &uuid.UUID{} }

	t.Run("every one of the six states is accepted with an agreeing desired state", func(t *testing.T) {
		for _, c := range []struct {
			status, desired string
			id              *uuid.UUID
		}{
			{"PENDING", "DEPLOYED", nil},
			{"DEPLOYING", "DEPLOYED", withID()},
			{"DEPLOYED", "DEPLOYED", withID()},
			{"UNDEPLOYING", "UNDEPLOYED", withID()},
			{"UNDEPLOYED", "UNDEPLOYED", nil},
			{"UNDEPLOYED", "UNDEPLOYED", withID()},
			{"FAILED", "DEPLOYED", withID()},
			{"FAILED", "UNDEPLOYED", withID()},
		} {
			assert.NoError(t, insertStatus(t, c.status, c.desired, c.id), "%s/%s", c.status, c.desired)
		}
	})

	t.Run("the status CHECKs reject bad rows", func(t *testing.T) {
		for _, c := range []struct {
			name, status, desired string
			id                    *uuid.UUID
		}{
			{"unknown status", "ARCHIVED", "DEPLOYED", withID()},
			{"unknown desired", "DEPLOYED", "DEPLOYING", withID()},
			{"deploying towards undeployed", "DEPLOYING", "UNDEPLOYED", withID()},
			{"deployed towards undeployed", "DEPLOYED", "UNDEPLOYED", withID()},
			{"pending towards undeployed", "PENDING", "UNDEPLOYED", nil},
			{"undeploying towards deployed", "UNDEPLOYING", "DEPLOYED", withID()},
			{"undeployed towards deployed", "UNDEPLOYED", "DEPLOYED", withID()},
			{"deploying with no deployment", "DEPLOYING", "DEPLOYED", nil},
			{"deployed with no deployment", "DEPLOYED", "DEPLOYED", nil},
			{"failed with no deployment", "FAILED", "DEPLOYED", nil},
		} {
			assert.Error(t, insertStatus(t, c.status, c.desired, c.id), c.name)
		}
	})

	t.Run("status_desired defaults to DEPLOYED, so an insert from an older instance still passes the CHECK", func(t *testing.T) {
		var desired string
		require.NoError(t, g.Raw(`SELECT column_default FROM information_schema.columns
			WHERE table_name = 'deployment_status' AND column_name = 'status_desired'`).Scan(&desired).Error)
		assert.Contains(t, desired, "'DEPLOYED'")
	})

	t.Run("indexes for the render scan and per-gateway reads exist", func(t *testing.T) {
		assertIndex(t, g, "deployment_status", "next_attempt_at", "WHERE (next_attempt_at IS NOT NULL)")
		assertIndex(t, g, "deployment_status", "(gateway_uuid, ou_id)", "")
	})
}

// assertIndex finds an index on table whose definition contains both fragments.
func assertIndex(t *testing.T, g *gorm.DB, table, columns, predicate string) {
	t.Helper()
	var defs []string
	require.NoError(t, g.Raw(`SELECT indexdef FROM pg_indexes WHERE tablename = ?`, table).Scan(&defs).Error)
	for _, def := range defs {
		if strings.Contains(def, columns) && strings.Contains(def, predicate) {
			return
		}
	}
	t.Errorf("no index on %s with %q %q among %v", table, columns, predicate, defs)
}

func TestMigration051_PlatformAgentExposures(t *testing.T) {
	g, s := migratedFrom050(t)

	t.Run("platform rows are kept whatever their status, the rest are deleted", func(t *testing.T) {
		var ids []uuid.UUID
		require.NoError(t, g.Raw(`SELECT id FROM platform_agent_exposures`).Scan(&ids).Error)
		assert.ElementsMatch(t, []uuid.UUID{s.pendingNoCard, s.failedPlatform, s.dupNewer, s.published}, ids)
	})

	t.Run("a2a_publications is renamed and its queue columns are gone", func(t *testing.T) {
		var tables int64
		require.NoError(t, g.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_name = 'a2a_publications'`).
			Scan(&tables).Error)
		assert.Zero(t, tables)

		var columns []string
		require.NoError(t, g.Raw(`SELECT column_name FROM information_schema.columns
			WHERE table_name = 'platform_agent_exposures' ORDER BY column_name`).Scan(&columns).Error)
		assert.ElementsMatch(t, []string{
			"id", "ou_id", "artifact_uuid", "project_name", "agent_name", "agent_uuid",
			"environment_uuid", "environment_name", "protocol", "created_at", "updated_at",
		}, columns)
	})

	t.Run("the kept rows keep their identity and are a2a", func(t *testing.T) {
		var row struct {
			ArtifactUUID uuid.UUID
			AgentUUID    *uuid.UUID
			Protocol     string
		}
		require.NoError(t, g.Raw(`SELECT artifact_uuid, agent_uuid, protocol FROM platform_agent_exposures WHERE id = ?`,
			s.failedPlatform).Scan(&row).Error)
		assert.Equal(t, s.failedPlatformArtifact, row.ArtifactUUID)
		require.NotNil(t, row.AgentUUID)
		assert.Equal(t, s.failedPlatformAgentUUID, *row.AgentUUID)
		assert.Equal(t, "a2a", row.Protocol)
	})

	t.Run("constraints carry the exposure names", func(t *testing.T) {
		var names []string
		require.NoError(t, g.Raw(`SELECT conname FROM pg_constraint
			WHERE conrelid = 'platform_agent_exposures'::regclass AND contype <> 'n'`).Scan(&names).Error)
		assert.ElementsMatch(t, []string{
			"platform_agent_exposures_pkey",
			"uq_platform_agent_exposures_artifact",
			"uq_platform_agent_exposures_agent_env",
			"chk_platform_agent_exposures_protocol",
			"fk_platform_agent_exposures_artifact",
		}, names)
		assertIndex(t, g, "platform_agent_exposures", "(ou_id, environment_uuid)", "")
	})

	t.Run("timestamps are TIMESTAMPTZ", func(t *testing.T) {
		var types []string
		require.NoError(t, g.Raw(`SELECT data_type FROM information_schema.columns
			WHERE table_name = 'platform_agent_exposures' AND column_name IN ('created_at', 'updated_at')`).
			Scan(&types).Error)
		assert.Equal(t, []string{"timestamp with time zone", "timestamp with time zone"}, types)
	})

	t.Run("only a2a is an accepted protocol and one artifact has one exposure", func(t *testing.T) {
		insert := func(artifactID uuid.UUID, agent, protocol string) error {
			return g.Exec(`INSERT INTO platform_agent_exposures
				(ou_id, artifact_uuid, project_name, agent_name, environment_uuid, environment_name, protocol)
				VALUES (?, ?, 'proj', ?, ?, 'dev', ?)`, seedOU, artifactID, agent, uuid.New(), protocol).Error
		}
		assert.Error(t, insert(s.dupArtifact, "another", "a2a"), "artifact already exposed")
		assert.Error(t, insert(s.deployedArtifact, "rest", "rest"), "protocol")
		assert.Error(t, insert(uuid.New(), "nowhere", "a2a"), "artifact must exist")
	})

	t.Run("an exposure goes with its artifact", func(t *testing.T) {
		exec(t, g, `DELETE FROM artifacts WHERE uuid = ?`, s.pendingNoCardArtifact)
		var count int64
		require.NoError(t, g.Raw(`SELECT count(*) FROM platform_agent_exposures WHERE id = ?`, s.pendingNoCard).
			Scan(&count).Error)
		assert.Zero(t, count)
	})

	t.Run("cards get the agent of their publication, external ones included", func(t *testing.T) {
		agentOf := func(cardID uuid.UUID) *uuid.UUID {
			var card struct{ AgentUUID *uuid.UUID }
			require.NoError(t, g.Raw(`SELECT agent_uuid FROM a2a_agent_cards WHERE id = ?`, cardID).Scan(&card).Error)
			return card.AgentUUID
		}
		require.NotNil(t, agentOf(s.failedPlatformCard))
		assert.Equal(t, s.failedPlatformAgentUUID, *agentOf(s.failedPlatformCard))
		require.NotNil(t, agentOf(s.externalCard))
		assert.Equal(t, s.externalAgentUUID, *agentOf(s.externalCard))
		assert.Nil(t, agentOf(s.orphanCard))
	})
}

func TestMigration051_EventHubSequence(t *testing.T) {
	g, _ := migratedFrom050(t)

	seqs := func(gatewayID string) []string {
		var ids []string
		require.NoError(t, g.Raw(`SELECT event_id FROM eventhub_events WHERE gateway_id = ? ORDER BY seq`, gatewayID).
			Scan(&ids).Error)
		return ids
	}
	assert.Equal(t, []string{"e-1", "e-2a", "e-2b", "e-3"}, seqs("gw-1"))
	assert.Equal(t, []string{"f-2", "f-1"}, seqs("gw-2"))

	var numbered []int64
	require.NoError(t, g.Raw(`SELECT seq FROM eventhub_events WHERE gateway_id = 'gw-1' ORDER BY seq`).Scan(&numbered).Error)
	assert.Equal(t, []int64{1, 2, 3, 4}, numbered)

	stateSeq := func(gatewayID string) int64 {
		var seq int64
		require.NoError(t, g.Raw(`SELECT seq FROM eventhub_gateway_states WHERE gateway_id = ?`, gatewayID).Scan(&seq).Error)
		return seq
	}
	assert.EqualValues(t, 4, stateSeq("gw-1"))
	assert.EqualValues(t, 2, stateSeq("gw-2"))
	assert.EqualValues(t, 0, stateSeq("gw-idle"))

	err := g.Exec(`INSERT INTO eventhub_events
		(event_id, gateway_id, originated_timestamp, entity_type, action, entity_id, event_data)
		VALUES ('no-seq', 'gw-1', NOW(), 't', 'a', 'e', '{}')`).Error
	assert.Error(t, err, "seq is NOT NULL")
	assertIndex(t, g, "eventhub_events", "(gateway_id, seq)", "")
}

func TestMigration051_DropsDeadSchema(t *testing.T) {
	g, _ := migratedFrom050(t)

	var tables int64
	require.NoError(t, g.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_name = 'association_mappings'`).
		Scan(&tables).Error)
	assert.Zero(t, tables)

	var orgColumns []string
	require.NoError(t, g.Raw(`SELECT table_name FROM information_schema.columns
		WHERE column_name = 'organization_name' AND table_name IN ('deployments', 'deployment_status', 'artifacts')`).
		Scan(&orgColumns).Error)
	assert.Empty(t, orgColumns)

	var primaryKey string
	require.NoError(t, g.Raw(`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = 'pk_deployment_status'`).
		Scan(&primaryKey).Error)
	assert.Equal(t, "PRIMARY KEY (artifact_uuid, ou_id, gateway_uuid)", primaryKey)
}

func TestMigration051_RunsOnce(t *testing.T) {
	g, _ := migratedFrom050(t)

	require.NoError(t, migrateTo(g, 51), "a second run is a no-op")
	var ids []string
	require.NoError(t, g.Raw(`SELECT id FROM migration_history WHERE id >= '0050' ORDER BY id`).Scan(&ids).Error)
	assert.Equal(t, []string{"0050", "0051"}, ids)
}

func TestMigration051_RefusesToDropAKeyOnOrganizationName(t *testing.T) {
	g := scratchDatabase(t)
	require.NoError(t, migrateTo(g, 50))
	seedAt050(t, g)
	exec(t, g, `CREATE INDEX idx_leftover_org ON deployments (organization_name)`)

	err := migrateTo(g, 51)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "idx_leftover_org")

	var applied []string
	require.NoError(t, g.Raw(`SELECT id FROM migration_history WHERE id >= '0050' ORDER BY id`).Scan(&applied).Error)
	assert.Equal(t, []string{"0050"}, applied)
	var rolledBack int64
	require.NoError(t, g.Raw(`SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'deployment_status' AND column_name = 'status_desired'`).Scan(&rolledBack).Error)
	assert.Zero(t, rolledBack, "the steps before the guard roll back with it")
}
