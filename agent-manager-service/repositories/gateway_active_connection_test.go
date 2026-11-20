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

// The 2026-11-20 cloud dev failure: after a network blip a gateway's old connection on one
// replica finished closing after the gateway had already reconnected to the other replica. The
// close's unconditional is_active=false landed last and left a connected gateway INACTIVE.
func TestGatewayActiveStatus_StaleCloseAfterReconnectKeepsGatewayActive(t *testing.T) {
	gdb := db.GetDB()
	repo := NewGatewayRepo(gdb)
	gatewayID := insertInactiveGateway(t, gdb)

	oldActivatedAt, err := repo.MarkConnected(gatewayID)
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)
	newActivatedAt, err := repo.MarkConnected(gatewayID) // the reconnect wins the race
	require.NoError(t, err)

	marked, err := repo.MarkDisconnected(gatewayID, oldActivatedAt) // the old connection's late close
	require.NoError(t, err)
	assert.False(t, marked, "a stale close must not mark a reconnected gateway inactive")
	assert.True(t, gatewayIsActive(t, gdb, gatewayID))

	marked, err = repo.MarkDisconnected(gatewayID, newActivatedAt)
	require.NoError(t, err)
	assert.True(t, marked, "the close of the current connection marks the gateway inactive")
	assert.False(t, gatewayIsActive(t, gdb, gatewayID))
}

// The other order: the old connection's close is handled before the reconnect.
func TestGatewayActiveStatus_CloseBeforeReconnectEndsActive(t *testing.T) {
	gdb := db.GetDB()
	repo := NewGatewayRepo(gdb)
	gatewayID := insertInactiveGateway(t, gdb)

	activatedAt, err := repo.MarkConnected(gatewayID)
	require.NoError(t, err)
	marked, err := repo.MarkDisconnected(gatewayID, activatedAt)
	require.NoError(t, err)
	assert.True(t, marked)
	assert.False(t, gatewayIsActive(t, gdb, gatewayID))

	_, err = repo.MarkConnected(gatewayID)
	require.NoError(t, err)
	assert.True(t, gatewayIsActive(t, gdb, gatewayID))
}

// The documented trade-off: an edit moves updated_at, so the next real close of that connection
// leaves the gateway marked active until it reconnects.
func TestGatewayActiveStatus_EditWhileConnectedKeepsGatewayActiveOnClose(t *testing.T) {
	gdb := db.GetDB()
	repo := NewGatewayRepo(gdb)
	gatewayID := insertInactiveGateway(t, gdb)

	activatedAt, err := repo.MarkConnected(gatewayID)
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)
	// UpdateGateway stamps updated_at with the app clock, like MarkConnected; so does this edit.
	require.NoError(t, gdb.Exec(`UPDATE gateways SET description = 'edited', updated_at = ? WHERE uuid = ?`, time.Now(), gatewayID).Error)

	marked, err := repo.MarkDisconnected(gatewayID, activatedAt)
	require.NoError(t, err)
	assert.False(t, marked)
	assert.True(t, gatewayIsActive(t, gdb, gatewayID))
}

func TestGatewayActiveStatus_MarkConnectedOnMissingGatewayIsNotFound(t *testing.T) {
	repo := NewGatewayRepo(db.GetDB())
	_, err := repo.MarkConnected(uuid.New().String())
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}
