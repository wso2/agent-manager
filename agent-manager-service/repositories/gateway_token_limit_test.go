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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const testGatewayTokenLimit = 2

// seedGatewayWithActiveToken inserts a gateway holding one active token and returns its UUID.
func seedGatewayWithActiveToken(t *testing.T, gdb *gorm.DB) uuid.UUID {
	t.Helper()
	ouID := "ou-" + uuid.New().String()[:8]
	gatewayID := uuid.New()
	name := "token-gw-" + gatewayID.String()[:8]
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		gatewayID, name, name, "gw.example", models.GatewayRoleIngress, ouID,
	).Error)
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM gateway_tokens WHERE gateway_uuid = ?`, gatewayID)
		gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID)
	})
	require.NoError(t, gdb.Create(newActiveGatewayToken(gatewayID)).Error)
	return gatewayID
}

func newActiveGatewayToken(gatewayID uuid.UUID) *models.GatewayToken {
	return &models.GatewayToken{
		UUID:        uuid.New(),
		GatewayUUID: gatewayID,
		TokenPrefix: uuid.New().String(),
		TokenHash:   "hash",
		Salt:        "salt",
		Status:      "active",
	}
}

func countActiveGatewayTokens(t *testing.T, gdb *gorm.DB, gatewayID uuid.UUID) int64 {
	t.Helper()
	var active int64
	require.NoError(t, gdb.Model(&models.GatewayToken{}).
		Where("gateway_uuid = ? AND status = ?", gatewayID, "active").Count(&active).Error)
	return active
}

// Concurrent rotations of a gateway with one active token used to each count one token, see
// room, and insert, leaving the gateway with more active tokens than the limit. Under the
// per-gateway lock exactly one of them fits.
func TestCreateTokenWithinLimit_HoldsTheLimitUnderConcurrency(t *testing.T) {
	gdb := db.GetDB()
	repo := NewGatewayRepo(gdb)
	gatewayID := seedGatewayWithActiveToken(t, gdb)

	const concurrent = 20
	warmPool(t, gdb, concurrent)
	var (
		start         = make(chan struct{})
		wg            sync.WaitGroup
		mu            sync.Mutex
		created       int
		limitRefusals int
		otherErrs     []error
	)
	for range concurrent {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := repo.CreateTokenWithinLimit(newActiveGatewayToken(gatewayID), testGatewayTokenLimit)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				created++
			case errors.Is(err, utils.ErrGatewayTokenLimitReached):
				limitRefusals++
			default:
				otherErrs = append(otherErrs, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	require.Empty(t, otherErrs)
	assert.Equal(t, 1, created, "exactly one rotation fits next to the existing token")
	assert.Equal(t, concurrent-1, limitRefusals, "every other rotation must be refused at the limit")
	assert.Equal(t, int64(testGatewayTokenLimit), countActiveGatewayTokens(t, gdb, gatewayID),
		"the gateway must end at the limit, never above it")
}

// warmPool opens n connections at once, so the concurrent callers that follow start together
// instead of trickling in while the pool dials.
func warmPool(t *testing.T, gdb *gorm.DB, n int) {
	t.Helper()
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, gdb.Exec("SELECT pg_sleep(0.05)").Error)
		}()
	}
	wg.Wait()
}

// A rotation that has inserted its token but not yet committed must make every other rotation
// of the same gateway wait, and the waiting one must then see the new token and be refused.
// Without the lock the waiting rotation counts only the committed token, sees room, and inserts
// a third active token. The open transaction below stands in for the first rotation, holding
// the same lock CreateTokenWithinLimit takes.
func TestCreateTokenWithinLimit_WaitsForAnInFlightRotation(t *testing.T) {
	gdb := db.GetDB()
	repo := NewGatewayRepo(gdb)
	gatewayID := seedGatewayWithActiveToken(t, gdb)

	inFlight := gdb.Begin()
	require.NoError(t, inFlight.Error)
	committed := false
	t.Cleanup(func() {
		if !committed {
			inFlight.Rollback()
		}
	})
	require.NoError(t, advisoryXactLock(inFlight, gatewayTokenLockNamespace, gatewayID.String()))
	require.NoError(t, inFlight.Create(newActiveGatewayToken(gatewayID)).Error)

	done := make(chan error, 1)
	go func() { done <- repo.CreateTokenWithinLimit(newActiveGatewayToken(gatewayID), testGatewayTokenLimit) }()

	select {
	case err := <-done:
		t.Fatalf("rotation finished (err=%v) while another rotation of the gateway was in flight; it must wait", err)
	case <-time.After(300 * time.Millisecond):
	}

	require.NoError(t, inFlight.Commit().Error)
	committed = true

	select {
	case err := <-done:
		require.ErrorIs(t, err, utils.ErrGatewayTokenLimitReached)
	case <-time.After(10 * time.Second):
		t.Fatal("rotation did not finish after the in-flight rotation committed")
	}
	assert.Equal(t, int64(testGatewayTokenLimit), countActiveGatewayTokens(t, gdb, gatewayID))
}

// Only active tokens count: revoking one frees a slot for the next rotation.
func TestCreateTokenWithinLimit_RevokedTokensDoNotCount(t *testing.T) {
	gdb := db.GetDB()
	repo := NewGatewayRepo(gdb)
	gatewayID := seedGatewayWithActiveToken(t, gdb)

	second := newActiveGatewayToken(gatewayID)
	require.NoError(t, repo.CreateTokenWithinLimit(second, testGatewayTokenLimit))
	require.ErrorIs(t, repo.CreateTokenWithinLimit(newActiveGatewayToken(gatewayID), testGatewayTokenLimit),
		utils.ErrGatewayTokenLimitReached)

	require.NoError(t, repo.RevokeToken(second.UUID.String()))

	require.NoError(t, repo.CreateTokenWithinLimit(newActiveGatewayToken(gatewayID), testGatewayTokenLimit))
	assert.Equal(t, int64(testGatewayTokenLimit), countActiveGatewayTokens(t, gdb, gatewayID))
}
