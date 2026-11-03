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

package reconcile

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// Fake kinds. fakeKind is gateway-addressed; fakeEnvKind adds per-environment records kept in
// reconcile_fake_records, a table only these tests use.

const (
	fakeGatewayKind = "FakeGateway"
	fakeEnvKindName = "FakeEnv"
	fakeAgentKind   = "FakeAgent"
)

type fakeKind struct {
	name   string
	inline bool

	mu        sync.Mutex
	render    func(ctx context.Context, ref models.ArtifactRef) (Rendered, error)
	renders   int
	afterLive func(ctx context.Context, tx *gorm.DB, p models.Placement) error
	lives     []uuid.UUID
	drifted   func(p models.Placement) bool
}

func newFakeKind(name string, inline bool) *fakeKind {
	return &fakeKind{name: name, inline: inline}
}

func (k *fakeKind) Name() string        { return k.name }
func (k *fakeKind) RendersInline() bool { return k.inline }

func (k *fakeKind) Render(ctx context.Context, ref models.ArtifactRef) (Rendered, error) {
	k.mu.Lock()
	k.renders++
	render := k.render
	k.mu.Unlock()
	if render != nil {
		return render(ctx, ref)
	}
	return Rendered{Name: "fake", Content: []byte("kind: " + ref.Kind + "\n")}, nil
}

func (k *fakeKind) renderCount() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.renders
}

func (k *fakeKind) setRender(render func(ctx context.Context, ref models.ArtifactRef) (Rendered, error)) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.render = render
}

func (k *fakeKind) GatewayArtifactID(ref models.ArtifactRef) string { return ref.ArtifactUUID.String() }

type fakePayload struct {
	ID           string    `json:"id"`
	DeploymentID string    `json:"deploymentId"`
	PerformedAt  time.Time `json:"performedAt"`
}

func (k *fakeKind) DeployEvent(ref EventRef) eventhub.Event {
	return eventhub.NewGatewayEvent("fake.deployed", "CREATE", ref.ArtifactUUID.String(),
		fakePayload{ID: ref.ArtifactUUID.String(), DeploymentID: ref.DeploymentID, PerformedAt: ref.PerformedAt})
}

func (k *fakeKind) UndeployEvent(ref EventRef) eventhub.Event {
	return eventhub.NewGatewayEvent("fake.undeployed", "DELETE", ref.ArtifactUUID.String(),
		fakePayload{ID: ref.ArtifactUUID.String(), DeploymentID: ref.DeploymentID, PerformedAt: ref.PerformedAt})
}

func (k *fakeKind) DeleteEvent(ref models.ArtifactRef) eventhub.Event {
	return eventhub.NewGatewayEvent("fake.deleted", "DELETE", ref.ArtifactUUID.String(), fakePayload{ID: ref.ArtifactUUID.String()})
}

func (k *fakeKind) AfterLive(ctx context.Context, tx *gorm.DB, p models.Placement, deploymentID uuid.UUID) error {
	k.mu.Lock()
	k.lives = append(k.lives, deploymentID)
	afterLive := k.afterLive
	k.mu.Unlock()
	if afterLive != nil {
		return afterLive(ctx, tx, p)
	}
	return nil
}

func (k *fakeKind) Drifted(_ context.Context, p models.Placement) (string, bool, error) {
	k.mu.Lock()
	drifted := k.drifted
	k.mu.Unlock()
	if drifted != nil && drifted(p) {
		return "the endpoint moved", true, nil
	}
	return "", false, nil
}

type fakeEnvKind struct {
	*fakeKind
	targets func(record Record) (uuid.UUID, bool, error)
	roles   []string
}

func newFakeEnvKind(inline bool) *fakeEnvKind {
	k := &fakeEnvKind{fakeKind: newFakeKind(fakeEnvKindName, inline), roles: models.EgressGatewayRoles}
	// Render re-checks the record, as every environment-addressed kind must.
	k.render = func(ctx context.Context, ref models.ArtifactRef) (Rendered, error) {
		if !fakeRecordExistsIn(db.DB(ctx), ref.ArtifactUUID) {
			return Rendered{}, ErrRecordGone
		}
		return Rendered{Name: "fake", Content: []byte("kind: " + ref.Kind + "\n")}, nil
	}
	return k
}

func (k *fakeEnvKind) UnplacedRecords(ctx context.Context, ouID string, env *uuid.UUID, afterKey string, limit int) ([]Record, error) {
	condition, args := UnplacedCondition(
		RecordColumns{OUID: "r.ou_id", ArtifactUUID: "r.artifact_uuid", EnvironmentUUID: "r.environment_uuid"}, k.roles, ouID, env)
	var rows []struct {
		RecordKey        string     `gorm:"column:record_key"`
		OUID             string     `gorm:"column:ou_id"`
		ArtifactUUID     uuid.UUID  `gorm:"column:artifact_uuid"`
		Handle           string     `gorm:"column:handle"`
		EnvironmentUUID  uuid.UUID  `gorm:"column:environment_uuid"`
		RequestedGateway *uuid.UUID `gorm:"column:requested_gateway"`
	}
	err := db.GetDB().WithContext(ctx).Raw(`SELECT r.* FROM reconcile_fake_records r
		WHERE `+condition+` AND r.record_key > ? ORDER BY r.record_key LIMIT ?`,
		append(args, afterKey, limit)...).Scan(&rows).Error
	records := make([]Record, 0, len(rows))
	for _, row := range rows {
		records = append(records, Record{
			ArtifactRef:      models.ArtifactRef{OUID: row.OUID, ArtifactUUID: row.ArtifactUUID, Kind: fakeEnvKindName, Handle: row.Handle},
			EnvironmentUUID:  row.EnvironmentUUID,
			RequestedGateway: row.RequestedGateway,
			Key:              row.RecordKey,
		})
	}
	return records, err
}

func (k *fakeEnvKind) PlacedOn(ctx context.Context, tx *gorm.DB, env, gateway uuid.UUID) (bool, error) {
	condition, args := PlacedCondition(
		RecordColumns{OUID: "r.ou_id", ArtifactUUID: "r.artifact_uuid", EnvironmentUUID: "r.environment_uuid"}, env, gateway)
	var placed bool
	err := tx.WithContext(ctx).Raw(`SELECT EXISTS (SELECT 1 FROM reconcile_fake_records r WHERE `+condition+`)`,
		args...).Scan(&placed).Error
	return placed, err
}

func (k *fakeEnvKind) Targets(ctx context.Context, record Record) (uuid.UUID, bool, error) {
	if k.targets != nil {
		return k.targets(record)
	}
	var gateways []uuid.UUID
	err := db.GetDB().WithContext(ctx).Raw(`SELECT m.gateway_uuid FROM gateway_environment_mappings m
		INNER JOIN gateways g ON g.uuid = m.gateway_uuid AND g.deleted_at IS NULL
		WHERE m.environment_uuid = ? ORDER BY m.gateway_uuid LIMIT 1`, record.EnvironmentUUID).Scan(&gateways).Error
	if err != nil || len(gateways) == 0 {
		return uuid.Nil, false, err
	}
	return gateways[0], true, nil
}

var fakeRecordsTable sync.Once

func ensureFakeRecordsTable(t *testing.T) {
	t.Helper()
	fakeRecordsTable.Do(func() {
		require.NoError(t, db.GetDB().Exec(`CREATE TABLE IF NOT EXISTS reconcile_fake_records (
			record_key TEXT PRIMARY KEY,
			ou_id VARCHAR(255) NOT NULL,
			artifact_uuid UUID NOT NULL REFERENCES artifacts(uuid) ON DELETE CASCADE,
			handle TEXT NOT NULL,
			environment_uuid UUID NOT NULL,
			requested_gateway UUID)`).Error)
	})
}

func fakeRecordExists(artifactUUID uuid.UUID) bool {
	return fakeRecordExistsIn(db.GetDB(), artifactUUID)
}

func fakeRecordExistsIn(gdb *gorm.DB, artifactUUID uuid.UUID) bool {
	var n int64
	gdb.Raw(`SELECT COUNT(*) FROM reconcile_fake_records WHERE artifact_uuid = ?`, artifactUUID).Scan(&n)
	return n > 0
}

// Fixtures.

func testOU() string { return "ou-" + uuid.NewString()[:8] }

// seedGateway registers a gateway in ouID, mapped to envs.
func seedGateway(t *testing.T, ouID string, envs ...uuid.UUID) uuid.UUID {
	t.Helper()
	return seedGatewayWithRole(t, ouID, models.GatewayRoleBoth, envs...)
}

// seedGatewayWithRole is seedGateway for a gateway of the given role.
func seedGatewayWithRole(t *testing.T, ouID, role string, envs ...uuid.UUID) uuid.UUID {
	t.Helper()
	gdb := db.GetDB()
	gatewayID := uuid.New()
	name := "reconcile-gw-" + gatewayID.String()[:8]
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		gatewayID, name, name, "gw.example", role, ouID).Error)
	for _, env := range envs {
		require.NoError(t, gdb.Create(&models.GatewayEnvironmentMapping{GatewayUUID: gatewayID, EnvironmentUUID: env}).Error)
	}
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM eventhub_events WHERE gateway_id = ?`, gatewayID.String())
		gdb.Exec(`DELETE FROM eventhub_gateway_states WHERE gateway_id = ?`, gatewayID.String())
		gdb.Exec(`DELETE FROM deployment_status WHERE gateway_uuid = ?`, gatewayID)
		gdb.Exec(`DELETE FROM deployments WHERE gateway_uuid = ?`, gatewayID)
		gdb.Exec(`DELETE FROM gateway_environment_mappings WHERE gateway_uuid = ?`, gatewayID)
		gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID)
	})
	return gatewayID
}

func seedArtifact(t *testing.T, ouID, kind string) models.ArtifactRef {
	t.Helper()
	gdb := db.GetDB()
	artifactUUID := uuid.New()
	handle := "handle-" + artifactUUID.String()[:8]
	require.NoError(t, repositories.NewArtifactRepo(gdb).Create(gdb, &models.Artifact{
		UUID: artifactUUID, Handle: handle, Name: handle, Version: "v1.0", Kind: kind, OUID: ouID,
	}))
	t.Cleanup(func() { gdb.Exec(`DELETE FROM artifacts WHERE uuid = ?`, artifactUUID) })
	return models.ArtifactRef{OUID: ouID, ArtifactUUID: artifactUUID, Kind: kind, Handle: handle}
}

func seedPlacement(t *testing.T, kind string) models.Placement {
	t.Helper()
	ouID := testOU()
	return models.Placement{ArtifactRef: seedArtifact(t, ouID, kind), GatewayUUID: seedGateway(t, ouID)}
}

// seedRecord saves an environment-addressed record, on tx when given.
func seedRecord(t *testing.T, tx *gorm.DB, ref models.ArtifactRef, env uuid.UUID) Record {
	t.Helper()
	ensureFakeRecordsTable(t)
	if tx == nil {
		tx = db.GetDB()
	}
	key := ref.ArtifactUUID.String()
	require.NoError(t, tx.Exec(`INSERT INTO reconcile_fake_records (record_key, ou_id, artifact_uuid, handle, environment_uuid)
		VALUES (?, ?, ?, ?, ?)`, key, ref.OUID, ref.ArtifactUUID, ref.Handle, env).Error)
	return Record{ArtifactRef: ref, EnvironmentUUID: env, Key: key}
}

func deleteRecord(t *testing.T, artifactUUID uuid.UUID) {
	t.Helper()
	require.NoError(t, db.GetDB().Exec(`DELETE FROM reconcile_fake_records WHERE artifact_uuid = ?`, artifactUUID).Error)
}

const testLease = 2 * time.Minute

func testConfig() Config {
	return Config{
		RenderInterval: time.Hour, PlaceInterval: time.Hour, DriftInterval: time.Hour,
		Lease: testLease, AttemptBudget: 30,
	}
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newCore(t *testing.T, kinds ...Kind) *Core {
	t.Helper()
	hub, err := eventhub.NewSQLBackend(db.GetDB(), testLogger(), eventhub.DefaultSQLBackendConfig())
	require.NoError(t, err)
	return NewCore(db.GetDB(), repositories.NewDeploymentRepo(db.GetDB()), hub, kinds, testConfig(), testLogger())
}

func newWorker(t *testing.T, kinds ...Kind) (*Core, *Worker) {
	t.Helper()
	core := newCore(t, kinds...)
	return core, NewWorker(core, testLogger())
}

// clockAt fixes the core's clock at a moment no other test's row is due at: the worker claims across the whole
// table, and integration packages share the database.
func clockAt(core *Core) time.Time {
	now := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(rand.IntN(1_000_000)) * time.Minute)
	core.now = func() time.Time { return now }
	return now
}

// runOnceFor runs the worker's render pass over the placement's artifact only. The packages' integration tests share
// one database, and a fleet-wide pass on this package's year-2000 clock would claim other packages' rows.
func runOnceFor(t *testing.T, worker *Worker, p models.Placement) {
	t.Helper()
	require.NoError(t, worker.RunOnceFor(context.Background(), p.OUID, p.ArtifactUUID))
}

func advance(core *Core, by time.Duration) time.Time {
	now := core.now().Add(by)
	core.now = func() time.Time { return now }
	return now
}

func dueAt(t *testing.T, p models.Placement, at time.Time) {
	t.Helper()
	require.NoError(t, db.GetDB().Exec(
		`UPDATE deployment_status SET next_attempt_at = ? WHERE artifact_uuid = ? AND ou_id = ? AND gateway_uuid = ?`,
		at, p.ArtifactUUID, p.OUID, p.GatewayUUID).Error)
}

func statusRow(t *testing.T, p models.Placement) *models.DeploymentStatusRecord {
	t.Helper()
	row, err := repositories.NewDeploymentRepo(db.GetDB()).GetStatusRecord(context.Background(), p)
	require.NoError(t, err)
	return row
}

func inTx(t *testing.T, fn func(tx *gorm.DB) error) error {
	t.Helper()
	return db.GetDB().Transaction(fn)
}

type sentEvent struct {
	Type    string
	Payload fakePayload
}

func eventsFor(t *testing.T, gatewayID uuid.UUID) []sentEvent {
	t.Helper()
	var data []string
	require.NoError(t, db.GetDB().Raw(`SELECT event_data FROM eventhub_events WHERE gateway_id = ? ORDER BY seq`,
		gatewayID.String()).Scan(&data).Error)
	events := make([]sentEvent, 0, len(data))
	for _, d := range data {
		var envelope struct {
			Type    string      `json:"type"`
			Payload fakePayload `json:"payload"`
		}
		require.NoError(t, json.Unmarshal([]byte(d), &envelope))
		events = append(events, sentEvent{Type: envelope.Type, Payload: envelope.Payload})
	}
	return events
}

func deploy(t *testing.T, core *Core, p models.Placement) *Applied {
	t.Helper()
	var applied *Applied
	require.NoError(t, inTx(t, func(tx *gorm.DB) error {
		var err error
		applied, err = core.Deploy(context.Background(), tx, p)
		return err
	}))
	return applied
}

func insertPending(t *testing.T, p models.Placement) {
	t.Helper()
	require.NoError(t, inTx(t, func(tx *gorm.DB) error {
		_, err := repositories.NewDeploymentRepo(db.GetDB()).InsertPending(context.Background(), tx, p, "placed")
		return err
	}))
}

func ack(t *testing.T, core *Core, p models.Placement, deploymentID uuid.UUID, status string) bool {
	t.Helper()
	applied, err := repositories.NewDeploymentRepo(db.GetDB()).ApplyAck(context.Background(), p.GatewayUUID.String(),
		models.DeploymentAck{DeploymentID: deploymentID.String(), Action: models.DeploymentAckActionDeploy, Status: status},
		core.AfterLive())
	require.NoError(t, err)
	return applied
}

// Tests.

func TestDeployWritesTheDeploymentAndItsEventLast(t *testing.T) {
	kind := newFakeKind(fakeGatewayKind, true)
	core := newCore(t, kind)
	p := seedPlacement(t, fakeGatewayKind)

	applied := deploy(t, core, p)

	row := statusRow(t, p)
	require.NotNil(t, row)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
	assert.Equal(t, applied.DeploymentID, *row.DeploymentID)
	events := eventsFor(t, p.GatewayUUID)
	require.Len(t, events, 1)
	assert.Equal(t, "fake.deployed", events[0].Type)
	assert.Equal(t, applied.DeploymentID.String(), events[0].Payload.DeploymentID)
	assert.True(t, row.PerformedAt.Equal(events[0].Payload.PerformedAt), "the event carries performed_at exactly")
}

func TestUndeploySendsAnEmptyDeploymentID(t *testing.T) {
	kind := newFakeKind(fakeGatewayKind, true)
	core := newCore(t, kind)
	p := seedPlacement(t, fakeGatewayKind)
	deploy(t, core, p)

	require.NoError(t, inTx(t, func(tx *gorm.DB) error { return core.Undeploy(context.Background(), tx, p) }))

	row := statusRow(t, p)
	assert.Equal(t, models.DeploymentStatusUndeploying, row.Status)
	events := eventsFor(t, p.GatewayUUID)
	require.Len(t, events, 2)
	assert.Equal(t, "fake.undeployed", events[1].Type)
	assert.Empty(t, events[1].Payload.DeploymentID)
	assert.True(t, row.PerformedAt.Equal(events[1].Payload.PerformedAt))
}

func TestUndeployOfAPendingRowSendsNothing(t *testing.T) {
	kind := newFakeKind(fakeAgentKind, false)
	core := newCore(t, kind)
	p := seedPlacement(t, fakeAgentKind)
	applied := deploy(t, core, p)
	assert.True(t, applied.Nudge, "a kind that doesn't render inline nudges the worker")

	require.NoError(t, inTx(t, func(tx *gorm.DB) error { return core.Undeploy(context.Background(), tx, p) }))

	assert.Equal(t, models.DeploymentStatusUndeployed, statusRow(t, p).Status)
	assert.Empty(t, eventsFor(t, p.GatewayUUID))
	assert.Zero(t, kind.renderCount())
}

// The claim token: an ack mid-render doesn't supersede the write-back; a newer request, a restore and a user
// undeploy do.
func TestClaimToken(t *testing.T) {
	setup := func(t *testing.T) (*fakeKind, *Core, models.Placement, uuid.UUID) {
		kind := newFakeKind(fakeGatewayKind, true)
		core := newCore(t, kind)
		p := seedPlacement(t, fakeGatewayKind)
		first := deploy(t, core, p).DeploymentID
		return kind, core, p, first
	}
	markStale := func(t *testing.T, core *Core, p models.Placement) {
		require.NoError(t, core.MarkStale(context.Background(), fakeGatewayKind, p.OUID, p.ArtifactUUID, nil, "inputs changed"))
	}
	during := func(kind *fakeKind, action func()) {
		kind.setRender(func(_ context.Context, ref models.ArtifactRef) (Rendered, error) {
			action()
			return Rendered{Name: "fake", Content: []byte("v2")}, nil
		})
	}

	t.Run("an ack mid-render", func(t *testing.T) {
		kind, core, p, first := setup(t)
		during(kind, func() { assert.True(t, ack(t, core, p, first, models.DeploymentAckStatusSuccess)) })

		markStale(t, core, p)

		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
		assert.NotEqual(t, first, *row.DeploymentID, "the render wrote back")
		assert.Nil(t, row.NextAttemptAt)
		assert.Len(t, eventsFor(t, p.GatewayUUID), 2)
	})
	t.Run("a newer request", func(t *testing.T) {
		kind, core, p, first := setup(t)
		during(kind, func() {
			_, err := repositories.NewDeploymentRepo(db.GetDB()).QueueRender(context.Background(), nil, p.OUID,
				p.ArtifactUUID, nil, "newer request", repositories.QueueScopeAny)
			assert.NoError(t, err)
		})

		markStale(t, core, p)

		row := statusRow(t, p)
		assert.Equal(t, first, *row.DeploymentID, "the superseded render wrote nothing")
		assert.NotNil(t, row.NextAttemptAt, "the newer request is still owed")
		assert.Equal(t, "newer request", row.RenderReason)
		assert.Len(t, eventsFor(t, p.GatewayUUID), 1)
	})
	t.Run("a restore", func(t *testing.T) {
		kind, core, p, first := setup(t)
		second := deploy(t, core, p).DeploymentID
		require.NotEqual(t, first, second)
		during(kind, func() {
			assert.NoError(t, inTx(t, func(tx *gorm.DB) error { return core.Restore(context.Background(), tx, p, first) }))
		})

		markStale(t, core, p)

		row := statusRow(t, p)
		assert.Equal(t, first, *row.DeploymentID, "the restore stands")
		assert.Nil(t, row.NextAttemptAt)
		events := eventsFor(t, p.GatewayUUID)
		require.Len(t, events, 3)
		assert.Equal(t, first.String(), events[2].Payload.DeploymentID)
		assert.True(t, row.PerformedAt.Equal(events[2].Payload.PerformedAt))
	})
	t.Run("a user undeploy", func(t *testing.T) {
		kind, core, p, _ := setup(t)
		during(kind, func() {
			assert.NoError(t, inTx(t, func(tx *gorm.DB) error { return core.Undeploy(context.Background(), tx, p) }))
		})

		markStale(t, core, p)

		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusUndeploying, row.Status)
		assert.Nil(t, row.NextAttemptAt)
		events := eventsFor(t, p.GatewayUUID)
		require.Len(t, events, 2)
		assert.Equal(t, "fake.undeployed", events[1].Type)
	})
}

// A restore over a waiting redeploy clears the queue, so the later render can't overwrite it.
func TestRestoreOverAWaitingRedeploy(t *testing.T) {
	kind := newFakeKind(fakeGatewayKind, true)
	core, worker := newWorker(t, kind)
	p := seedPlacement(t, fakeGatewayKind)
	first := deploy(t, core, p).DeploymentID
	ack(t, core, p, first, models.DeploymentAckStatusSuccess)
	second := deploy(t, core, p).DeploymentID
	ack(t, core, p, second, models.DeploymentAckStatusSuccess)
	kind.setRender(func(context.Context, models.ArtifactRef) (Rendered, error) {
		return Rendered{}, InputNotReady("waiting for the endpoint")
	})
	require.NoError(t, core.MarkStale(context.Background(), fakeGatewayKind, p.OUID, p.ArtifactUUID, nil, "inputs changed"))
	require.NotNil(t, statusRow(t, p).NextAttemptAt, "the redeploy is waiting")

	require.NoError(t, inTx(t, func(tx *gorm.DB) error { return core.Restore(context.Background(), tx, p, first) }))
	kind.setRender(nil)
	clockAt(core)
	advance(core, time.Hour)
	runOnceFor(t, worker, p)

	row := statusRow(t, p)
	assert.Equal(t, first, *row.DeploymentID)
	assert.Nil(t, row.NextAttemptAt)
	claims, err := repositories.NewDeploymentRepo(db.GetDB()).ClaimDueRendersOf(context.Background(), p.OUID,
		p.ArtifactUUID, nil, time.Now().Add(time.Hour), testLease)
	require.NoError(t, err)
	assert.Empty(t, claims, "nothing is owed after the restore")
}

// A redeploy whose input isn't ready leaves the row DEPLOYED with the queue due; the user sees DEPLOYING.
func TestRedeployWaitingForAnInput(t *testing.T) {
	kind := newFakeKind(fakeGatewayKind, true)
	core := newCore(t, kind)
	p := seedPlacement(t, fakeGatewayKind)
	first := deploy(t, core, p).DeploymentID
	ack(t, core, p, first, models.DeploymentAckStatusSuccess)
	kind.setRender(func(context.Context, models.ArtifactRef) (Rendered, error) {
		return Rendered{}, InputNotReady("waiting for the agent's endpoint")
	})

	require.NoError(t, core.MarkStale(context.Background(), fakeGatewayKind, p.OUID, p.ArtifactUUID, nil, "inputs changed"))

	row := statusRow(t, p)
	assert.Equal(t, models.DeploymentStatusDeployed, row.Status, "the gateway still serves the old version")
	assert.Equal(t, first, *row.DeploymentID)
	require.NotNil(t, row.NextAttemptAt)
	assert.Zero(t, row.AttemptCount)
	status, err := core.Status(context.Background(), p)
	require.NoError(t, err)
	assert.Equal(t, models.UserStatus{Status: models.DeploymentStatusDeploying, Reason: "waiting for the agent's endpoint"}, status)
}

// The worker backs a waiting row off from 5 s to the 5 min cap without spending its attempts.
func TestWorkerBacksOffAWaitingRow(t *testing.T) {
	kind := newFakeKind(fakeAgentKind, false)
	kind.setRender(func(context.Context, models.ArtifactRef) (Rendered, error) {
		return Rendered{}, InputNotReady("waiting for the agent's endpoint")
	})
	core, worker := newWorker(t, kind)
	p := seedPlacement(t, fakeAgentKind)
	deploy(t, core, p)
	now := clockAt(core)
	dueAt(t, p, now)

	var delays []time.Duration
	for range 8 {
		runOnceFor(t, worker, p)
		row := statusRow(t, p)
		require.NotNil(t, row.NextAttemptAt)
		assert.Zero(t, row.AttemptCount)
		delay := row.NextAttemptAt.Sub(core.now())
		delays = append(delays, delay)
		advance(core, delay)
	}

	assert.Equal(t, []time.Duration{
		5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 80 * time.Second, 160 * time.Second,
		5 * time.Minute, 5 * time.Minute,
	}, delays)
	assert.Equal(t, models.DeploymentStatusPending, statusRow(t, p).Status)
}

func TestWorkerGivesUpAfterTheAttemptBudget(t *testing.T) {
	kind := newFakeKind(fakeAgentKind, false)
	kind.setRender(func(context.Context, models.ArtifactRef) (Rendered, error) {
		return Rendered{}, errors.New("connection reset")
	})
	core, worker := newWorker(t, kind)
	p := seedPlacement(t, fakeAgentKind)
	deploy(t, core, p)
	clockAt(core)
	dueAt(t, p, core.now())

	for range 30 {
		runOnceFor(t, worker, p)
		advance(core, 10*time.Minute)
	}

	row := statusRow(t, p)
	assert.Nil(t, row.NextAttemptAt, "given up")
	assert.Equal(t, 29, row.AttemptCount)
	assert.Equal(t, 30, kind.renderCount())
	assert.Equal(t, models.UserStatus{Status: models.DeploymentStatusFailed, Reason: "connection reset"}, models.UserStatusOf(*row))
}

// Kill between render and write-back: nothing is half-committed, and the row is claimed again after the lease.
func TestWorkerClaimsAnAbandonedRenderAfterTheLease(t *testing.T) {
	kind := newFakeKind(fakeAgentKind, false)
	core, worker := newWorker(t, kind)
	p := seedPlacement(t, fakeAgentKind)
	deploy(t, core, p)
	now := clockAt(core)
	dueAt(t, p, now)
	killed, err := repositories.NewDeploymentRepo(db.GetDB()).ClaimDueRendersOf(context.Background(), p.OUID, p.ArtifactUUID,
		nil, now, testLease)
	require.NoError(t, err)
	require.Len(t, killed, 1, "a replica claims the row and dies before writing back")

	runOnceFor(t, worker, p)
	assert.Zero(t, kind.renderCount(), "the row is leased")
	row := statusRow(t, p)
	assert.Equal(t, models.DeploymentStatusPending, row.Status)
	assert.Nil(t, row.DeploymentID)

	advance(core, testLease+time.Second)
	runOnceFor(t, worker, p)

	assert.Equal(t, 1, kind.renderCount())
	row = statusRow(t, p)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
	assert.Nil(t, row.NextAttemptAt)
	assert.Len(t, eventsFor(t, p.GatewayUUID), 1)
}

func TestDeployRecord(t *testing.T) {
	t.Run("an inline kind deploys on the environment's gateway", func(t *testing.T) {
		kind := newFakeEnvKind(true)
		core := newCore(t, kind)
		ouID, env := testOU(), uuid.New()
		gateway := seedGateway(t, ouID, env)
		ref := seedArtifact(t, ouID, fakeEnvKindName)

		var applied *Applied
		require.NoError(t, inTx(t, func(tx *gorm.DB) error {
			record := seedRecord(t, tx, ref, env)
			var err error
			applied, err = core.DeployRecord(context.Background(), tx, kind, record)
			return err
		}))

		assert.False(t, applied.NoGateway)
		assert.Equal(t, gateway, applied.Placement.GatewayUUID)
		assert.Equal(t, models.DeploymentStatusDeploying, statusRow(t, applied.Placement).Status)
		assert.Len(t, eventsFor(t, gateway), 1)
	})
	t.Run("a requested gateway is used as it is", func(t *testing.T) {
		kind := newFakeEnvKind(true)
		kind.targets = func(Record) (uuid.UUID, bool, error) { panic("Targets isn't asked") }
		core := newCore(t, kind)
		ouID, env := testOU(), uuid.New()
		seedGateway(t, ouID, env)
		requested := seedGateway(t, ouID, env)
		ref := seedArtifact(t, ouID, fakeEnvKindName)

		require.NoError(t, inTx(t, func(tx *gorm.DB) error {
			record := seedRecord(t, tx, ref, env)
			record.RequestedGateway = &requested
			_, err := core.DeployRecord(context.Background(), tx, kind, record)
			return err
		}))

		assert.NotNil(t, statusRow(t, models.Placement{ArtifactRef: ref, GatewayUUID: requested}))
	})
	t.Run("no gateway: a valid record is rendered once and writes no row", func(t *testing.T) {
		kind := newFakeEnvKind(true)
		core := newCore(t, kind)
		ref := seedArtifact(t, testOU(), fakeEnvKindName)

		var applied *Applied
		require.NoError(t, inTx(t, func(tx *gorm.DB) error {
			record := seedRecord(t, tx, ref, uuid.New())
			var err error
			applied, err = core.DeployRecord(context.Background(), tx, kind, record)
			return err
		}))

		assert.True(t, applied.NoGateway)
		assert.Equal(t, 1, kind.renderCount(), "the validation render")
		assertNoRows(t, ref)
	})
	t.Run("no gateway: a validation error is returned and nothing is written", func(t *testing.T) {
		kind := newFakeEnvKind(true)
		kind.setRender(func(context.Context, models.ArtifactRef) (Rendered, error) {
			return Rendered{}, errors.New("invalid security config")
		})
		core := newCore(t, kind)
		ref := seedArtifact(t, testOU(), fakeEnvKindName)

		err := inTx(t, func(tx *gorm.DB) error {
			record := seedRecord(t, tx, ref, uuid.New())
			_, err := core.DeployRecord(context.Background(), tx, kind, record)
			return err
		})

		assert.ErrorContains(t, err, "invalid security config")
		assert.False(t, fakeRecordExists(ref.ArtifactUUID))
		assertNoRows(t, ref)
	})
	t.Run("a render error rolls back the caller's edited state and sends nothing", func(t *testing.T) {
		kind := newFakeEnvKind(true)
		kind.setRender(func(context.Context, models.ArtifactRef) (Rendered, error) {
			return Rendered{}, errors.New("invalid security config")
		})
		core := newCore(t, kind)
		ouID, env := testOU(), uuid.New()
		gateway := seedGateway(t, ouID, env)
		ref := seedArtifact(t, ouID, fakeEnvKindName)

		err := inTx(t, func(tx *gorm.DB) error {
			record := seedRecord(t, tx, ref, env)
			_, err := core.DeployRecord(context.Background(), tx, kind, record)
			return err
		})

		assert.Error(t, err)
		assert.False(t, fakeRecordExists(ref.ArtifactUUID), "the edited state rolled back")
		assertNoRows(t, ref)
		assert.Empty(t, eventsFor(t, gateway))
	})
	t.Run("a kind that doesn't render inline places a PENDING row and nudges", func(t *testing.T) {
		kind := newFakeEnvKind(false)
		core := newCore(t, kind)
		ouID, env := testOU(), uuid.New()
		gateway := seedGateway(t, ouID, env)
		ref := seedArtifact(t, ouID, fakeEnvKindName)

		var applied *Applied
		require.NoError(t, inTx(t, func(tx *gorm.DB) error {
			var err error
			applied, err = core.DeployRecord(context.Background(), tx, kind, seedRecord(t, tx, ref, env))
			return err
		}))

		assert.True(t, applied.Nudge)
		assert.Zero(t, kind.renderCount())
		row := statusRow(t, models.Placement{ArtifactRef: ref, GatewayUUID: gateway})
		assert.Equal(t, models.DeploymentStatusPending, row.Status)
		assert.NotNil(t, row.NextAttemptAt)
	})
}

func assertNoRows(t *testing.T, ref models.ArtifactRef) {
	t.Helper()
	var rows int64
	require.NoError(t, db.GetDB().Model(&models.DeploymentStatusRecord{}).
		Where("artifact_uuid = ?", ref.ArtifactUUID).Count(&rows).Error)
	assert.Zero(t, rows)
}

// PlaceEnv after a mapping places only that environment, and renders what it places in the same call.
func TestPlaceEnv(t *testing.T) {
	kind := newFakeEnvKind(true)
	_, worker := newWorker(t, kind)
	ouID, env, otherEnv := testOU(), uuid.New(), uuid.New()
	ref := seedArtifact(t, ouID, fakeEnvKindName)
	seedRecord(t, nil, ref, env)
	otherRef := seedArtifact(t, ouID, fakeEnvKindName)
	seedRecord(t, nil, otherRef, otherEnv)
	gateway := seedGateway(t, ouID, env)
	otherGateway := seedGateway(t, ouID, otherEnv)

	require.NoError(t, worker.PlaceEnv(context.Background(), ouID, env))

	row := statusRow(t, models.Placement{ArtifactRef: ref, GatewayUUID: gateway})
	require.NotNil(t, row)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status, "rendered in the same call")
	assert.Len(t, eventsFor(t, gateway), 1)
	assert.Nil(t, statusRow(t, models.Placement{ArtifactRef: otherRef, GatewayUUID: otherGateway}))

	require.NoError(t, worker.PlaceEnv(context.Background(), ouID, env))
	assert.Equal(t, 1, kind.renderCount(), "a placed record isn't placed again")
}

func TestPlaceAll(t *testing.T) {
	kind := newFakeEnvKind(true)
	_, worker := newWorker(t, kind)
	ouID, env := testOU(), uuid.New()
	ref := seedArtifact(t, ouID, fakeEnvKindName)
	seedRecord(t, nil, ref, env)
	gateway := seedGateway(t, ouID, env)

	require.NoError(t, worker.PlaceAll(context.Background()))

	assert.Equal(t, models.DeploymentStatusDeploying, statusRow(t, models.Placement{ArtifactRef: ref, GatewayUUID: gateway}).Status)
}

// The placement pass racing a user undeploy: the record is deleted after the pass read it, so the pass inserts a
// PENDING row; Render's re-check undeploys it with no event, and the next pass doesn't re-place it.
func TestPlacementPassRacingAUserUndeploy(t *testing.T) {
	kind := newFakeEnvKind(true)
	_, worker := newWorker(t, kind)
	ouID, env := testOU(), uuid.New()
	ref := seedArtifact(t, ouID, fakeEnvKindName)
	seedRecord(t, nil, ref, env)
	gateway := seedGateway(t, ouID, env)
	kind.targets = func(record Record) (uuid.UUID, bool, error) {
		deleteRecord(t, record.ArtifactUUID)
		return gateway, true, nil
	}

	require.NoError(t, worker.PlaceEnv(context.Background(), ouID, env))

	p := models.Placement{ArtifactRef: ref, GatewayUUID: gateway}
	row := statusRow(t, p)
	require.NotNil(t, row)
	assert.Equal(t, models.DeploymentStatusUndeployed, row.Status)
	assert.Nil(t, row.DeploymentID)
	assert.Nil(t, row.NextAttemptAt)
	assert.Empty(t, eventsFor(t, gateway))

	kind.targets = nil
	seedRecord(t, nil, ref, env)
	require.NoError(t, worker.PlaceEnv(context.Background(), ouID, env))
	assert.Equal(t, models.DeploymentStatusUndeployed, statusRow(t, p).Status, "an UNDEPLOYED row is a row: not re-placed")
	assert.Equal(t, 1, kind.renderCount())
}

func TestUnplacedRecordsExcludeSoftDeletedGateways(t *testing.T) {
	kind := newFakeEnvKind(true)
	ouID, env := testOU(), uuid.New()
	ref := seedArtifact(t, ouID, fakeEnvKindName)
	seedRecord(t, nil, ref, env)
	gateway := seedGateway(t, ouID, env)

	records, err := kind.UnplacedRecords(context.Background(), ouID, &env, "", 10)
	require.NoError(t, err)
	require.Len(t, records, 1)

	require.NoError(t, repositories.NewGatewayRepo(db.GetDB()).Delete(gateway.String(), ouID))

	records, err = kind.UnplacedRecords(context.Background(), ouID, &env, "", 10)
	require.NoError(t, err)
	assert.Empty(t, records, "the mapping stays after a soft delete, but the gateway doesn't count")
}

func TestUnplacedRecordsNeedAGatewayOfTheKindsRole(t *testing.T) {
	for name, tc := range map[string]struct {
		roles               []string
		wrongRole, kindRole string
	}{
		"egress kind":  {roles: models.EgressGatewayRoles, wrongRole: models.GatewayRoleIngress, kindRole: models.GatewayRoleEgress},
		"ingress kind": {roles: models.IngressGatewayRoles, wrongRole: models.GatewayRoleEgress, kindRole: models.GatewayRoleIngress},
	} {
		t.Run(name, func(t *testing.T) {
			kind := newFakeEnvKind(true)
			kind.roles = tc.roles
			ouID, env := testOU(), uuid.New()
			ref := seedArtifact(t, ouID, fakeEnvKindName)
			seedRecord(t, nil, ref, env)
			seedGatewayWithRole(t, ouID, tc.wrongRole, env)

			records, err := kind.UnplacedRecords(context.Background(), ouID, &env, "", 10)
			require.NoError(t, err)
			assert.Empty(t, records, "an environment with only the other role can't place the kind")

			seedGatewayWithRole(t, ouID, tc.kindRole, env)

			records, err = kind.UnplacedRecords(context.Background(), ouID, &env, "", 10)
			require.NoError(t, err)
			assert.Len(t, records, 1)
		})
	}
}

func TestGatewayDeleteKeepsAUserUndeploy(t *testing.T) {
	for name, sent := range map[string]bool{"undeployed after a deploy": true, "undeployed from PENDING": false} {
		t.Run(name, func(t *testing.T) {
			kind := newFakeEnvKind(true)
			core, worker := newWorker(t, kind)
			ouID, env := testOU(), uuid.New()
			ref := seedArtifact(t, ouID, fakeEnvKindName)
			seedRecord(t, nil, ref, env)
			old := seedGateway(t, ouID, env)
			p := models.Placement{ArtifactRef: ref, GatewayUUID: old}
			repo := repositories.NewDeploymentRepo(db.GetDB())
			if sent {
				deploy(t, core, p)
			} else {
				insertPending(t, p)
			}
			require.NoError(t, inTx(t, func(tx *gorm.DB) error { return core.Undeploy(context.Background(), tx, p) }))
			if sent {
				acked, err := repo.ApplyAck(context.Background(), old.String(), models.DeploymentAck{
					ArtifactID: ref.ArtifactUUID.String(), Action: models.DeploymentAckActionUndeploy, Status: models.DeploymentAckStatusSuccess,
				}, core.AfterLive())
				require.NoError(t, err)
				require.True(t, acked)
			}
			require.Equal(t, models.DeploymentStatusUndeployed, statusRow(t, p).Status)

			require.NoError(t, repositories.NewGatewayRepo(db.GetDB()).Delete(old.String(), ouID))
			fresh := seedGateway(t, ouID, env)
			require.NoError(t, worker.PlaceEnv(context.Background(), ouID, env))

			row := statusRow(t, p)
			require.NotNil(t, row, "the undeploy survives its gateway")
			assert.Equal(t, models.DeploymentStatusUndeployed, row.Status)
			assert.Nil(t, row.DeploymentID)
			assert.Nil(t, statusRow(t, models.Placement{ArtifactRef: ref, GatewayUUID: fresh}), "not re-placed")
			assert.Empty(t, eventsFor(t, fresh))
			sentTo, err := repo.SentGateways(context.Background(), ouID, ref.ArtifactUUID)
			require.NoError(t, err)
			assert.Empty(t, sentTo, "no delete event goes to the dead gateway")
		})
	}
}

func TestGatewayDeleteDropsAnUndeployOwedADeploy(t *testing.T) {
	kind := newFakeEnvKind(true)
	core, worker := newWorker(t, kind)
	ouID, env := testOU(), uuid.New()
	ref := seedArtifact(t, ouID, fakeEnvKindName)
	seedRecord(t, nil, ref, env)
	old := seedGateway(t, ouID, env)
	p := models.Placement{ArtifactRef: ref, GatewayUUID: old}
	insertPending(t, p)
	require.NoError(t, inTx(t, func(tx *gorm.DB) error { return core.Undeploy(context.Background(), tx, p) }))
	_, err := repositories.NewDeploymentRepo(db.GetDB()).QueueRender(context.Background(), nil, ouID, ref.ArtifactUUID, nil, "redeploy", repositories.QueueScopeAny)
	require.NoError(t, err)
	require.NotNil(t, statusRow(t, p).NextAttemptAt, "a deploy is owed on the undeployed row")

	require.NoError(t, repositories.NewGatewayRepo(db.GetDB()).Delete(old.String(), ouID))
	fresh := seedGateway(t, ouID, env)
	require.NoError(t, worker.PlaceEnv(context.Background(), ouID, env))

	assert.Nil(t, statusRow(t, p), "the owed deploy goes with the gateway")
	assert.NotNil(t, statusRow(t, models.Placement{ArtifactRef: ref, GatewayUUID: fresh}), "so the artifact is placed again")
}

func TestGatewayDeleteRemovesPendingRows(t *testing.T) {
	ouID := testOU()
	p := models.Placement{ArtifactRef: seedArtifact(t, ouID, fakeEnvKindName), GatewayUUID: seedGateway(t, ouID)}
	insertPending(t, p)
	require.NotNil(t, statusRow(t, p))

	require.NoError(t, repositories.NewGatewayRepo(db.GetDB()).Delete(p.GatewayUUID.String(), ouID))

	assert.Nil(t, statusRow(t, p))
}

// AfterLive runs exactly once per transition to DEPLOYED, late acks included; a hook error doesn't roll back the ack.
func TestAfterLive(t *testing.T) {
	t.Run("once per transition, late acks included", func(t *testing.T) {
		kind := newFakeKind(fakeGatewayKind, true)
		core := newCore(t, kind)
		p := seedPlacement(t, fakeGatewayKind)
		first := deploy(t, core, p).DeploymentID

		assert.True(t, ack(t, core, p, first, models.DeploymentAckStatusSuccess))
		assert.False(t, ack(t, core, p, first, models.DeploymentAckStatusSuccess))
		second := deploy(t, core, p).DeploymentID
		_, err := repositories.NewDeploymentRepo(db.GetDB()).FailStaleTransitional(context.Background(), time.Now().Add(time.Hour))
		require.NoError(t, err)
		require.Equal(t, models.DeploymentStatusFailed, statusRow(t, p).Status, "timed out")
		assert.True(t, ack(t, core, p, second, models.DeploymentAckStatusSuccess), "the late ack lands")

		assert.Equal(t, []uuid.UUID{first, second}, kind.lives)
		assert.Equal(t, models.DeploymentStatusDeployed, statusRow(t, p).Status)
	})
	t.Run("a hook error doesn't roll back the ack", func(t *testing.T) {
		kind := newFakeEnvKind(true)
		core := newCore(t, kind)
		ouID, env := testOU(), uuid.New()
		gateway := seedGateway(t, ouID, env)
		ref := seedArtifact(t, ouID, fakeEnvKindName)
		hookRef := seedArtifact(t, ouID, fakeEnvKindName)
		kind.afterLive = func(_ context.Context, tx *gorm.DB, _ models.Placement) error {
			seedRecord(t, tx, hookRef, env)
			return errors.New("card fetch couldn't be queued")
		}
		var applied *Applied
		require.NoError(t, inTx(t, func(tx *gorm.DB) error {
			var err error
			applied, err = core.DeployRecord(context.Background(), tx, kind, seedRecord(t, tx, ref, env))
			return err
		}))

		assert.True(t, ack(t, core, applied.Placement, applied.DeploymentID, models.DeploymentAckStatusSuccess))

		assert.Equal(t, models.DeploymentStatusDeployed, statusRow(t, models.Placement{ArtifactRef: ref, GatewayUUID: gateway}).Status)
		assert.False(t, fakeRecordExists(hookRef.ArtifactUUID), "the hook's writes are rolled back")
	})
}

// Drift runs on one replica per interval: of two workers scanning at once, one scans.
func TestDriftRunsOnOneReplica(t *testing.T) {
	kind := newFakeKind("FakeDrift", true)
	core, first := newWorker(t, kind)
	second := NewWorker(newCore(t, kind), testLogger())
	p := seedDeployedDriftPlacement(t, core)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	kind.drifted = func(got models.Placement) bool {
		if got != p {
			return false
		}
		once.Do(func() { close(entered) })
		<-release
		return true
	}

	var firstScanned bool
	var wg sync.WaitGroup
	wg.Go(func() {
		var err error
		firstScanned, err = first.CheckDrift(context.Background())
		assert.NoError(t, err)
	})
	<-entered
	secondScanned, err := second.CheckDrift(context.Background())
	require.NoError(t, err)
	close(release)
	wg.Wait()

	assert.True(t, firstScanned)
	assert.False(t, secondScanned, "the other replica skips the interval")
	row := statusRow(t, p)
	require.NotNil(t, row.NextAttemptAt, "the drifted row is marked for a re-render")
	assert.Equal(t, "the endpoint moved", row.RenderReason)
	assert.Equal(t, models.DeploymentStatusDeployed, row.Status)
}

func seedDeployedDriftPlacement(t *testing.T, core *Core) models.Placement {
	t.Helper()
	p := models.Placement{ArtifactRef: seedArtifact(t, testOU(), "FakeDrift")}
	p.GatewayUUID = seedGateway(t, p.OUID)
	ack(t, core, p, deploy(t, core, p).DeploymentID, models.DeploymentAckStatusSuccess)
	return p
}

func TestDriftScanHoldsNoTransactionOpen(t *testing.T) {
	kind := newFakeKind("FakeDrift", true)
	core, worker := newWorker(t, kind)
	p := seedDeployedDriftPlacement(t, core)
	var states []string
	kind.drifted = func(got models.Placement) bool {
		if got == p {
			db.GetDB().Raw(`SELECT s.state FROM pg_locks l JOIN pg_stat_activity s USING (pid)
				WHERE l.locktype = 'advisory' AND l.classid = ? AND l.objid = hashtext(?)::oid`,
				repositories.DriftScanLockNamespace, repositories.DriftScanLockKey).Scan(&states)
		}
		return false
	}

	_, err := worker.CheckDrift(context.Background())

	require.NoError(t, err)
	require.Len(t, states, 1, "the scan's lock is found in pg_locks while the scan runs")
	assert.Equal(t, "idle", states[0])
}

func TestDriftScanReleasesTheLockOnError(t *testing.T) {
	kind := newFakeKind("FakeDrift", true)
	core, first := newWorker(t, kind)
	second := NewWorker(newCore(t, kind), testLogger())
	p := seedDeployedDriftPlacement(t, core)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	kind.drifted = func(got models.Placement) bool {
		if got == p {
			cancel()
		}
		return true
	}

	scanned, err := first.CheckDrift(ctx)
	require.Error(t, err)
	assert.True(t, scanned)

	kind.drifted = nil
	scanned, err = second.CheckDrift(context.Background())
	require.NoError(t, err)
	assert.True(t, scanned, "the failed scan released the lock")
}

// An unlock that fails ends the pinned connection's session, which releases the lock with it.
func TestDriftUnlockFailureDiscardsTheConnection(t *testing.T) {
	_, worker := newWorker(t)
	var pid int
	err := db.DB(context.Background()).Connection(func(conn *gorm.DB) error {
		require.NoError(t, conn.Raw("SELECT pg_backend_pid()").Scan(&pid).Error)
		require.Error(t, repositories.UnlockDriftScan(conn), "the lock was never taken")
		worker.unlockDriftScan(context.Background(), conn)
		return nil
	})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		var sessions int64
		require.NoError(t, db.DB(context.Background()).Raw("SELECT count(*) FROM pg_stat_activity WHERE pid = ?", pid).Scan(&sessions).Error)
		return sessions == 0
	}, 5*time.Second, 50*time.Millisecond)
}

// MarkStale touches only placed rows.
func TestMarkStaleTouchesOnlyPlacedRows(t *testing.T) {
	kind := newFakeKind(fakeGatewayKind, true)
	core := newCore(t, kind)
	placed := seedPlacement(t, fakeGatewayKind)
	undeployed := placed
	undeployed.GatewayUUID = seedGateway(t, placed.OUID)
	deploy(t, core, placed)
	deploy(t, core, undeployed)
	require.NoError(t, inTx(t, func(tx *gorm.DB) error { return core.Undeploy(context.Background(), tx, undeployed) }))
	rendersBefore := kind.renderCount()

	require.NoError(t, core.MarkStale(context.Background(), fakeGatewayKind, placed.OUID, placed.ArtifactUUID, nil, "provider auth changed"))

	assert.Equal(t, rendersBefore+1, kind.renderCount())
	assert.Len(t, eventsFor(t, placed.GatewayUUID), 2)
	row := statusRow(t, undeployed)
	assert.Equal(t, models.DeploymentStatusUndeploying, row.Status)
	assert.Nil(t, row.NextAttemptAt)
	assert.Len(t, eventsFor(t, undeployed.GatewayUUID), 2, "deploy and undeploy only")
}

// A failed inline render reaches MarkStale's caller; the gateway keeps what it was sent and the row keeps the
// outcome.
func TestMarkStaleReturnsAFailedInlineRender(t *testing.T) {
	kind := newFakeKind(fakeGatewayKind, true)
	core := newCore(t, kind)
	p := seedPlacement(t, fakeGatewayKind)
	first := deploy(t, core, p).DeploymentID
	renderErr := Permanent(errors.New("invalid config"))
	kind.setRender(func(context.Context, models.ArtifactRef) (Rendered, error) { return Rendered{}, renderErr })

	err := core.MarkStale(context.Background(), fakeGatewayKind, p.OUID, p.ArtifactUUID, nil, "inputs changed")

	var failed *RenderFailedError
	require.ErrorAs(t, err, &failed)
	assert.Equal(t, repositories.RenderGiveUp, failed.Outcome)
	assert.ErrorIs(t, err, renderErr)
	row := statusRow(t, p)
	assert.Equal(t, first, *row.DeploymentID, "no deployment was written")
	assert.Equal(t, "invalid config", row.RenderReason)
	assert.Len(t, eventsFor(t, p.GatewayUUID), 1, "only the first deploy's event")
}

var errServesEnvironment = errors.New("the gateway serves the environment")

// unmapInTx removes the gateway's mapping to env in tx as the gateway service does: delete, then check.
func unmapInTx(ctx context.Context, core *Core, tx *gorm.DB, gateway, env uuid.UUID) error {
	if err := tx.Exec(`DELETE FROM gateway_environment_mappings WHERE gateway_uuid = ? AND environment_uuid = ?`,
		gateway, env).Error; err != nil {
		return err
	}
	serves, err := core.ServesEnvironment(ctx, tx, env, gateway)
	if err != nil {
		return err
	}
	if serves {
		return errServesEnvironment
	}
	return nil
}

func notDoneWithin[T any](t *testing.T, ch <-chan T, what string) {
	t.Helper()
	select {
	case got := <-ch:
		t.Fatalf("%s didn't wait: %v", what, got)
	case <-time.After(300 * time.Millisecond):
	}
}

// An unmap racing a placement on the gateway waits for it, then sees its row and is refused.
func TestUnmapWaitsForAPlacementInProgress(t *testing.T) {
	kind := newFakeEnvKind(false)
	core := newCore(t, kind)
	ouID, env := testOU(), uuid.New()
	gateway := seedGateway(t, ouID, env)
	record := seedRecord(t, nil, seedArtifact(t, ouID, fakeEnvKindName), env)
	placing := db.GetDB().Begin()
	defer placing.Rollback()
	_, err := core.DeployRecord(context.Background(), placing, kind, record)
	require.NoError(t, err)

	unmapped := make(chan error, 1)
	go func() {
		unmapped <- db.GetDB().Transaction(func(tx *gorm.DB) error {
			return unmapInTx(context.Background(), core, tx, gateway, env)
		})
	}()
	notDoneWithin(t, unmapped, "the unmap")
	require.NoError(t, placing.Commit().Error)

	assert.ErrorIs(t, <-unmapped, errServesEnvironment)
}

// A placement racing an unmap of the environment's gateway waits for it, then finds no gateway and places nothing.
func TestPlacementWaitsForAnUnmapInProgress(t *testing.T) {
	kind := newFakeEnvKind(false)
	core := newCore(t, kind)
	ouID, env := testOU(), uuid.New()
	gateway := seedGateway(t, ouID, env)
	kind.targets = func(Record) (uuid.UUID, bool, error) { return gateway, true, nil }
	record := seedRecord(t, nil, seedArtifact(t, ouID, fakeEnvKindName), env)
	unmapping := db.GetDB().Begin()
	defer unmapping.Rollback()
	require.NoError(t, unmapInTx(context.Background(), core, unmapping, gateway, env))

	placed := make(chan *Applied, 1)
	go func() {
		var applied *Applied
		assert.NoError(t, inTx(t, func(tx *gorm.DB) error {
			var err error
			applied, err = core.DeployRecord(context.Background(), tx, kind, record)
			return err
		}))
		placed <- applied
	}()
	notDoneWithin(t, placed, "the placement")
	require.NoError(t, unmapping.Commit().Error)

	assert.True(t, (<-placed).NoGateway)
	assert.Nil(t, statusRow(t, models.Placement{ArtifactRef: record.ArtifactRef, GatewayUUID: gateway}))
}

// A record gone before its redeploy renders undeploys the row; that isn't a failure.
func TestMarkStaleOfAGoneRecordIsNotAFailure(t *testing.T) {
	kind := newFakeEnvKind(true)
	core := newCore(t, kind)
	ouID := testOU()
	env := uuid.New()
	gateway := seedGateway(t, ouID, env)
	record := seedRecord(t, nil, seedArtifact(t, ouID, fakeEnvKindName), env)
	require.NoError(t, inTx(t, func(tx *gorm.DB) error {
		_, err := core.DeployRecord(context.Background(), tx, kind, record)
		return err
	}))
	deleteRecord(t, record.ArtifactUUID)

	require.NoError(t, core.MarkStale(context.Background(), fakeEnvKindName, ouID, record.ArtifactUUID, nil, "inputs changed"))

	row := statusRow(t, models.Placement{ArtifactRef: record.ArtifactRef, GatewayUUID: gateway})
	assert.Equal(t, models.DeploymentStatusUndeploying, row.Status)
}

func TestPrepareDeleteSendsToEveryGatewaySentTheArtifactAndEveryActiveGateway(t *testing.T) {
	kind := newFakeKind(fakeGatewayKind, true)
	core := newCore(t, kind)
	p := seedPlacement(t, fakeGatewayKind)
	undeployed := p
	undeployed.GatewayUUID = seedGateway(t, p.OUID)
	pending := p
	pending.GatewayUUID = seedGateway(t, p.OUID)
	require.NoError(t, db.GetDB().Exec(`UPDATE gateways SET is_active = FALSE WHERE uuid IN (?, ?)`,
		undeployed.GatewayUUID, pending.GatewayUUID).Error)
	bystander := seedGateway(t, p.OUID)
	deploy(t, core, p)
	deploy(t, core, undeployed)
	require.NoError(t, inTx(t, func(tx *gorm.DB) error { return core.Undeploy(context.Background(), tx, undeployed) }))
	insertPending(t, pending)

	require.NoError(t, inTx(t, func(tx *gorm.DB) error {
		events, err := core.PrepareDelete(context.Background(), tx, p.ArtifactRef)
		if err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM artifacts WHERE uuid = ?`, p.ArtifactUUID).Error; err != nil {
			return err
		}
		return events.Publish(context.Background(), tx)
	}))

	for _, gateway := range []uuid.UUID{p.GatewayUUID, undeployed.GatewayUUID, bystander} {
		events := eventsFor(t, gateway)
		require.NotEmpty(t, events, "a gateway sent the artifact, or any active gateway of the org, is told")
		assert.Equal(t, "fake.deleted", events[len(events)-1].Type)
	}
	assert.Empty(t, eventsFor(t, pending.GatewayUUID), "an inactive gateway that was never sent the artifact isn't told")
}

// A user undeploy supersedes a deploy owed on a row that is already undeployed: nothing is rendered or sent.
func TestUndeployCancelsADeployOwedOnAnUndeployedRow(t *testing.T) {
	kind := newFakeKind(fakeGatewayKind, true)
	core, worker := newWorker(t, kind)
	p := seedPlacement(t, fakeGatewayKind)
	deploy(t, core, p)
	require.NoError(t, inTx(t, func(tx *gorm.DB) error { return core.Undeploy(context.Background(), tx, p) }))
	_, err := repositories.NewDeploymentRepo(db.GetDB()).QueueRender(context.Background(), nil, p.OUID, p.ArtifactUUID,
		nil, "redeploy", repositories.QueueScopeAny)
	require.NoError(t, err)
	eventsBefore := len(eventsFor(t, p.GatewayUUID))
	rendersBefore := kind.renderCount()

	require.NoError(t, inTx(t, func(tx *gorm.DB) error { return core.Undeploy(context.Background(), tx, p) }))
	clockAt(core)
	runOnceFor(t, worker, p)
	claims, err := repositories.NewDeploymentRepo(db.GetDB()).ClaimDueRendersOf(context.Background(), p.OUID,
		p.ArtifactUUID, nil, time.Now().Add(time.Hour), testLease)
	require.NoError(t, err)
	assert.Empty(t, claims, "nothing is owed after the undeploy")

	row := statusRow(t, p)
	assert.Equal(t, models.DeploymentStatusUndeployed, row.StatusDesired)
	assert.Equal(t, models.DeploymentStatusUndeploying, row.Status)
	assert.Nil(t, row.NextAttemptAt)
	assert.Equal(t, rendersBefore, kind.renderCount())
	assert.Len(t, eventsFor(t, p.GatewayUUID), eventsBefore)
}
