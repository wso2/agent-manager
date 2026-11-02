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

package eventhub

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
)

// newTestBackend returns a backend with prepared statements and no background loops; tests poll by hand.
func newTestBackend(t *testing.T) *SQLBackend {
	t.Helper()
	backend, err := NewSQLBackend(db.GetDB(), slog.New(slog.NewTextHandler(io.Discard, nil)), DefaultSQLBackendConfig())
	require.NoError(t, err)
	require.NoError(t, backend.prepareStatements())
	t.Cleanup(backend.closeStatements)
	return backend
}

// newTestGateway registers a fresh gateway id with the backend and removes its state (and events) afterwards.
func newTestGateway(t *testing.T, backend *SQLBackend) string {
	t.Helper()
	gatewayID := "gw-" + uuid.NewString()
	require.NoError(t, backend.RegisterGateway(gatewayID))
	t.Cleanup(func() {
		db.GetDB().Exec(`DELETE FROM eventhub_gateway_states WHERE gateway_id = ?`, gatewayID)
	})
	return gatewayID
}

func testEvent(entityID string) Event {
	return Event{EventType: "llmprovider.deployed", Action: "CREATE", EntityID: entityID}
}

// drain returns the entity ids of every event waiting on ch.
func drain(ch <-chan Event) []string {
	var ids []string
	for {
		select {
		case evt := <-ch:
			ids = append(ids, evt.EntityID)
		default:
			return ids
		}
	}
}

func eventSeqs(t *testing.T, gatewayID string) []int64 {
	t.Helper()
	var seqs []int64
	require.NoError(t, db.GetDB().Raw(`SELECT seq FROM eventhub_events WHERE gateway_id = ? ORDER BY seq`, gatewayID).
		Scan(&seqs).Error)
	return seqs
}

// Every event takes the next number in its gateway's sequence, under the gateway-state row lock.
func TestPublishEventNumbersEventsPerGateway(t *testing.T) {
	backend := newTestBackend(t)
	gatewayID := "seq-" + uuid.NewString()
	t.Cleanup(func() {
		db.GetDB().Exec(`DELETE FROM eventhub_gateway_states WHERE gateway_id = ?`, gatewayID)
	})

	for range 2 {
		require.NoError(t, backend.PublishEvent(context.Background(), gatewayID, testEvent("e")))
	}

	assert.Equal(t, []int64{1, 2}, eventSeqs(t, gatewayID))
	var stateSeq int64
	require.NoError(t, db.GetDB().Raw(`SELECT seq FROM eventhub_gateway_states WHERE gateway_id = ?`, gatewayID).
		Scan(&stateSeq).Error)
	assert.EqualValues(t, 2, stateSeq)
}

// The outbox: an event is only as durable as the caller's transaction.
func TestPublishEventTxIsNeverDeliveredWhenTheTransactionRollsBack(t *testing.T) {
	backend := newTestBackend(t)
	gatewayID := newTestGateway(t, backend)
	ch, err := backend.Subscribe(gatewayID)
	require.NoError(t, err)

	rolledBack := db.GetDB().Transaction(func(tx *gorm.DB) error {
		require.NoError(t, backend.PublishEventTx(context.Background(), tx, gatewayID, testEvent("rolled-back")))
		return errors.New("roll back")
	})
	require.Error(t, rolledBack)
	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		return backend.PublishEventTx(context.Background(), tx, gatewayID, testEvent("committed"))
	}))

	backend.pollGateways()
	assert.Equal(t, []string{"committed"}, drain(ch))
}

// main's cursor was (processed_timestamp, event_id), stamped before the gateway lock: an event stamped earlier but
// committed later (a long transaction, or a replica whose clock is behind) was skipped for ever. The seq cursor
// orders events by commit.
func TestPollerDeliversEventsCommittedInTheOppositeOrderToTheirStamps(t *testing.T) {
	backend := newTestBackend(t)
	gatewayID := newTestGateway(t, backend)
	ch, err := backend.Subscribe(gatewayID)
	require.NoError(t, err)
	behind := newTestBackend(t)
	behind.clock = func() time.Time { return time.Now().Add(-time.Minute) }

	require.NoError(t, backend.PublishEvent(context.Background(), gatewayID, testEvent("first")))
	backend.pollGateways()
	require.Equal(t, []string{"first"}, drain(ch))

	require.NoError(t, behind.PublishEvent(context.Background(), gatewayID, testEvent("stamped-earlier")))
	backend.pollGateways()
	assert.Equal(t, []string{"stamped-earlier"}, drain(ch))
}

// Two transactions publish to one gateway at once: the second waits on the gateway lock, and both are delivered in
// commit order.
func TestConcurrentPublishesToOneGatewayAreBothDelivered(t *testing.T) {
	backend := newTestBackend(t)
	gatewayID := newTestGateway(t, backend)
	ch, err := backend.Subscribe(gatewayID)
	require.NoError(t, err)

	firstLocked := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		assert.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
			if err := backend.PublishEventTx(context.Background(), tx, gatewayID, testEvent("a")); err != nil {
				return err
			}
			close(firstLocked)
			backend.pollGateways() // the uncommitted event isn't visible yet
			time.Sleep(100 * time.Millisecond)
			return nil
		}))
	}()
	go func() {
		defer wg.Done()
		<-firstLocked
		assert.NoError(t, backend.PublishEvent(context.Background(), gatewayID, testEvent("b")))
	}()
	wg.Wait()

	backend.pollGateways()
	assert.Equal(t, []string{"a", "b"}, drain(ch))
	assert.Equal(t, []int64{1, 2}, eventSeqs(t, gatewayID))
}

// A re-published event id is skipped, and the caller's transaction carries on.
func TestPublishEventTxSkipsADuplicateEventWithoutAbortingTheTransaction(t *testing.T) {
	backend := newTestBackend(t)
	gatewayID := newTestGateway(t, backend)
	event := testEvent("dup")
	event.EventID = uuid.NewString()

	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		require.NoError(t, backend.PublishEventTx(context.Background(), tx, gatewayID, event))
		require.NoError(t, backend.PublishEventTx(context.Background(), tx, gatewayID, event))
		return tx.Exec(`SELECT 1`).Error
	}))

	var count int64
	require.NoError(t, db.GetDB().Raw(`SELECT count(*) FROM eventhub_events WHERE event_id = ?`, event.EventID).
		Scan(&count).Error)
	assert.EqualValues(t, 1, count)
}

// A gateway with no state row yet gets one on its first publish.
func TestPublishEventTxCreatesTheGatewayState(t *testing.T) {
	backend := newTestBackend(t)
	gatewayID := "unregistered-" + uuid.NewString()
	t.Cleanup(func() {
		db.GetDB().Exec(`DELETE FROM eventhub_gateway_states WHERE gateway_id = ?`, gatewayID)
	})

	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		return backend.PublishEventTx(context.Background(), tx, gatewayID, testEvent("e"))
	}))
	assert.Equal(t, []int64{1}, eventSeqs(t, gatewayID))
}

// Transactions that each publish to the same gateways, locking them in gateway_id order, never deadlock.
func TestPublishesLockedInGatewayOrderDontDeadlock(t *testing.T) {
	backend := newTestBackend(t)
	gateways := []string{newTestGateway(t, backend), newTestGateway(t, backend)}
	if gateways[0] > gateways[1] {
		gateways[0], gateways[1] = gateways[1], gateways[0]
	}

	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
				for _, gatewayID := range gateways {
					if err := backend.PublishEventTx(context.Background(), tx, gatewayID, testEvent("e")); err != nil {
						return err
					}
					time.Sleep(time.Duration(i) * 10 * time.Millisecond)
				}
				return nil
			}))
		}()
	}
	wg.Wait()

	for _, gatewayID := range gateways {
		assert.Equal(t, []int64{1, 2, 3, 4}, eventSeqs(t, gatewayID))
	}
}
