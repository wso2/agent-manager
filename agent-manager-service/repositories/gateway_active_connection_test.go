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

package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func insertInactiveGateway(t *testing.T, gdb *gorm.DB) string {
	t.Helper()
	ouID := "ou-" + uuid.New().String()[:8]
	gatewayID := uuid.New()
	name := "conn-gw-" + gatewayID.String()[:8]
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, FALSE)`,
		gatewayID, name, name, "gw.example", models.GatewayRoleIngress, ouID,
	).Error)
	t.Cleanup(func() { gdb.Exec(`DELETE FROM gateways WHERE ou_id = ?`, ouID) })
	return gatewayID.String()
}

func gatewayIsActive(t *testing.T, gdb *gorm.DB, gatewayID string) bool {
	t.Helper()
	var active bool
	require.NoError(t, gdb.Raw(`SELECT is_active FROM gateways WHERE uuid = ?`, gatewayID).Scan(&active).Error)
	return active
}

// at returns base plus offset, truncated like a real activation time.
func at(base time.Time, offset time.Duration) time.Time {
	return base.Add(offset).Truncate(time.Microsecond)
}

func markDisconnected(t *testing.T, repo GatewayRepository, gatewayID string, activatedAt time.Time) bool {
	t.Helper()
	marked, err := repo.MarkDisconnected(context.Background(), gatewayID, activatedAt)
	require.NoError(t, err)
	return marked
}

// The 2026-11-20 cloud dev failure: after a network blip a gateway's old connection on one
// replica finished closing after the gateway had already reconnected to the other replica. The
// close's unconditional is_active=false landed last and left a connected gateway INACTIVE.
func TestGatewayActiveStatus_StaleCloseAfterReconnectKeepsGatewayActive(t *testing.T) {
	ctx, gdb := context.Background(), db.GetDB()
	repo := NewGatewayRepo(gdb)
	gatewayID := insertInactiveGateway(t, gdb)
	base := time.Now().Add(-time.Minute)
	tA, tB := at(base, 0), at(base, time.Second)

	require.NoError(t, repo.MarkConnected(ctx, gatewayID, tA))
	require.NoError(t, repo.MarkConnected(ctx, gatewayID, tB)) // B reconnects before A's close is handled

	assert.False(t, markDisconnected(t, repo, gatewayID, tA), "a stale close must not mark a reconnected gateway inactive")
	assert.True(t, gatewayIsActive(t, gdb, gatewayID))

	assert.True(t, markDisconnected(t, repo, gatewayID, tB), "the close of the current connection marks the gateway inactive")
	assert.False(t, gatewayIsActive(t, gdb, gatewayID))
}

// A was accepted before B but its connect write (or its whole handler, before the write) stalled
// until after B's write. The activation time is the accept time, so A's write cannot take the row
// from B, and A's close must not deactivate B.
func TestGatewayActiveStatus_StalledOlderConnectCannotStealTheRow(t *testing.T) {
	ctx, gdb := context.Background(), db.GetDB()
	repo := NewGatewayRepo(gdb)
	gatewayID := insertInactiveGateway(t, gdb)
	base := time.Now().Add(-time.Minute)
	tA, tB := at(base, 0), at(base, time.Second)

	require.NoError(t, repo.MarkConnected(ctx, gatewayID, tB)) // B writes first
	require.NoError(t, repo.MarkConnected(ctx, gatewayID, tA)) // A's stalled write lands

	assert.False(t, markDisconnected(t, repo, gatewayID, tA), "A's close must not deactivate B")
	assert.True(t, gatewayIsActive(t, gdb, gatewayID))

	assert.True(t, markDisconnected(t, repo, gatewayID, tB))
	assert.False(t, gatewayIsActive(t, gdb, gatewayID))
}

// B is accepted, then A's close is handled (writing a later updated_at), then B's connect write
// lands. A disconnect's timestamp must not be kept as B's activation, or B's own close could never
// match and the gateway would stay ACTIVE with no connection.
func TestGatewayActiveStatus_DisconnectBetweenAcceptAndWriteDoesNotStickActive(t *testing.T) {
	ctx, gdb := context.Background(), db.GetDB()
	repo := NewGatewayRepo(gdb)
	gatewayID := insertInactiveGateway(t, gdb)
	base := time.Now().Add(-time.Minute)
	tA, tB := at(base, 0), at(base, time.Second)

	require.NoError(t, repo.MarkConnected(ctx, gatewayID, tA))
	// B is accepted at tB; A closes now, after tB, and owns the row, so it marks it inactive.
	assert.True(t, markDisconnected(t, repo, gatewayID, tA))
	require.NoError(t, repo.MarkConnected(ctx, gatewayID, tB)) // B's write lands
	assert.True(t, gatewayIsActive(t, gdb, gatewayID))

	assert.True(t, markDisconnected(t, repo, gatewayID, tB), "B's close must match its own activation")
	assert.False(t, gatewayIsActive(t, gdb, gatewayID))
}

// The other order: the old connection's close is handled before the reconnect.
func TestGatewayActiveStatus_CloseBeforeReconnectEndsActive(t *testing.T) {
	ctx, gdb := context.Background(), db.GetDB()
	repo := NewGatewayRepo(gdb)
	gatewayID := insertInactiveGateway(t, gdb)
	base := time.Now().Add(-time.Minute)
	tA, tB := at(base, 0), at(base, time.Second)

	require.NoError(t, repo.MarkConnected(ctx, gatewayID, tA))
	assert.True(t, markDisconnected(t, repo, gatewayID, tA))
	assert.False(t, gatewayIsActive(t, gdb, gatewayID))

	require.NoError(t, repo.MarkConnected(ctx, gatewayID, tB))
	assert.True(t, gatewayIsActive(t, gdb, gatewayID))
}

// The documented trade-off: an edit moves updated_at, so the next real close of that connection
// leaves the gateway marked active until it reconnects.
func TestGatewayActiveStatus_EditWhileConnectedKeepsGatewayActiveOnClose(t *testing.T) {
	ctx, gdb := context.Background(), db.GetDB()
	repo := NewGatewayRepo(gdb)
	gatewayID := insertInactiveGateway(t, gdb)
	tA := at(time.Now().Add(-time.Minute), 0)

	require.NoError(t, repo.MarkConnected(ctx, gatewayID, tA))
	// UpdateGateway stamps updated_at with the app clock; so does this edit.
	require.NoError(t, gdb.Exec(`UPDATE gateways SET description = 'edited', updated_at = ? WHERE uuid = ?`, time.Now(), gatewayID).Error)

	assert.False(t, markDisconnected(t, repo, gatewayID, tA))
	assert.True(t, gatewayIsActive(t, gdb, gatewayID))
}

func TestGatewayActiveStatus_MarkConnectedOnMissingGatewayIsNotFound(t *testing.T) {
	repo := NewGatewayRepo(db.GetDB())
	err := repo.MarkConnected(context.Background(), uuid.New().String(), time.Now())
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
