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
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestEventDestinationRejectsStaleWrites(t *testing.T) {
	for _, first := range []string{"rotation", "settings"} {
		t.Run(first+" wins", func(t *testing.T) {
			ctx := context.Background()
			repo := NewEventRepository(db.GetDB())
			d := &models.EventDestination{
				ID: uuid.New(), OUID: newOrg(t), Name: "webhook",
				Kind:            models.DestinationKindWebhook,
				Config:          json.RawMessage(`{"url":"https://old.example.com"}`),
				SecretEncrypted: []byte("old-secret"),
				Availability:    models.DestinationAvailableAll, Projects: []string{},
			}
			require.NoError(t, repo.CreateDestination(ctx, d))
			t.Cleanup(func() { _, err := repo.DeleteDestination(ctx, d.OUID, d.ID); assert.NoError(t, err) })
			current, err := repo.GetDestination(ctx, d.OUID, d.ID)
			require.NoError(t, err)
			stale, err := repo.GetDestination(ctx, d.OUID, d.ID)
			require.NoError(t, err)
			if first == "rotation" {
				current.SecretEncrypted = []byte("new-secret")
				stale.Config = json.RawMessage(`{"url":"https://new.example.com"}`)
			} else {
				current.Config = json.RawMessage(`{"url":"https://new.example.com"}`)
				current.Availability = models.DestinationAvailableOrg
				stale.SecretEncrypted = []byte("new-secret")
			}
			require.NoError(t, repo.UpdateDestination(ctx, current, nil))
			checked := false
			err = repo.UpdateDestination(ctx, stale, func([]models.EventSubscription) error { checked = true; return nil })
			require.ErrorIs(t, err, utils.ErrConflict)
			assert.False(t, checked, "stale input is rejected before availability checks")
			saved, err := repo.GetDestination(ctx, d.OUID, d.ID)
			require.NoError(t, err)
			assert.Equal(t, current.SecretEncrypted, saved.SecretEncrypted)
			assert.JSONEq(t, string(current.Config), string(saved.Config))
			assert.Equal(t, current.Availability, saved.Availability)
			assert.True(t, current.UpdatedAt.Equal(saved.UpdatedAt), "returned revision matches the stored one")

			// An edit based on the latest value succeeds without reverting
			// the secret or settings written by the first request.
			saved.Description = "fresh edit"
			require.NoError(t, repo.UpdateDestination(ctx, saved, nil))
			fresh, err := repo.GetDestination(ctx, d.OUID, d.ID)
			require.NoError(t, err)
			assert.Equal(t, "fresh edit", fresh.Description)
			assert.Equal(t, current.SecretEncrypted, fresh.SecretEncrypted)
			assert.JSONEq(t, string(current.Config), string(fresh.Config))
		})
	}
}

func TestEventDeliveryClaimAndFinishedOutcomes(t *testing.T) {
	ctx := context.Background()
	repo := NewEventRepository(db.GetDB())
	d := &models.EventDestination{
		ID: uuid.New(), OUID: newOrg(t), Name: "webhook",
		Kind: models.DestinationKindWebhook, Config: json.RawMessage(`{"url":"https://example.com"}`),
		Availability: models.DestinationAvailableAll, Projects: []string{},
	}
	require.NoError(t, repo.CreateDestination(ctx, d))
	t.Cleanup(func() { _, err := repo.DeleteDestination(ctx, d.OUID, d.ID); assert.NoError(t, err) })
	queued := &models.EventDelivery{
		OUID: d.OUID, DestinationID: d.ID, SubscriptionIDs: []string{}, EventID: "evt-" + uuid.NewString(),
		EventType: "agent.deployed", Status: models.EventDeliveryPending,
	}
	require.NoError(t, repo.RecordQueuedDelivery(ctx, queued))
	delivery, err := repo.GetDelivery(ctx, d.ID, queued.EventID)
	require.NoError(t, err)

	claimed, err := repo.ClaimDelivery(ctx, delivery.ID, time.Minute)
	require.NoError(t, err)
	assert.True(t, claimed)
	claimed, err = repo.ClaimDelivery(ctx, delivery.ID, time.Minute)
	require.NoError(t, err)
	assert.False(t, claimed, "a second worker cannot claim a delivery being sent")

	// A failed attempt releases the claim for the next retry.
	failed := *delivery
	failed.ID, failed.Attempts, failed.LastError = uuid.Nil, 1, "down"
	require.NoError(t, repo.RecordDelivery(ctx, &failed))
	claimed, err = repo.ClaimDelivery(ctx, delivery.ID, time.Minute)
	require.NoError(t, err)
	assert.True(t, claimed)

	done := *delivery
	done.ID, done.Status, done.Attempts = uuid.Nil, models.EventDeliveryDelivered, 2
	require.NoError(t, repo.RecordDelivery(ctx, &done))
	claimed, err = repo.ClaimDelivery(ctx, delivery.ID, time.Minute)
	require.NoError(t, err)
	assert.False(t, claimed, "a finished delivery is not claimed")

	// A late worker cannot turn it back into pending.
	late := *delivery
	late.ID, late.Status, late.Attempts, late.LastError = uuid.Nil, models.EventDeliveryPending, 3, "timeout"
	require.NoError(t, repo.RecordDelivery(ctx, &late))
	saved, err := repo.GetDelivery(ctx, d.ID, queued.EventID)
	require.NoError(t, err)
	assert.Equal(t, models.EventDeliveryDelivered, saved.Status)
	assert.Equal(t, 2, saved.Attempts)
}

func TestEventDeliveryFailStaleDeliveries(t *testing.T) {
	ctx := context.Background()
	gdb := db.GetDB()
	repo := NewEventRepository(gdb)
	d := &models.EventDestination{
		ID: uuid.New(), OUID: newOrg(t), Name: "webhook",
		Kind: models.DestinationKindWebhook, Config: json.RawMessage(`{"url":"https://example.com"}`),
		Availability: models.DestinationAvailableAll, Projects: []string{},
	}
	require.NoError(t, repo.CreateDestination(ctx, d))
	t.Cleanup(func() { _, err := repo.DeleteDestination(ctx, d.OUID, d.ID); assert.NoError(t, err) })
	old := time.Now().Add(-7 * time.Hour)
	queue := func(status string, at time.Time, claimedUntil *time.Time) string {
		row := &models.EventDelivery{
			OUID: d.OUID, DestinationID: d.ID, SubscriptionIDs: []string{}, EventID: "evt-" + uuid.NewString(),
			EventType: "agent.deployed", Status: status, CreatedAt: at,
		}
		require.NoError(t, repo.RecordQueuedDelivery(ctx, row))
		require.NoError(t, gdb.Model(&models.EventDelivery{}).Where("event_id = ?", row.EventID).
			Updates(map[string]any{"updated_at": at, "claimed_until": claimedUntil}).Error)
		return row.EventID
	}
	inFuture := time.Now().Add(time.Minute)
	lost := queue(models.EventDeliveryPending, old, nil)
	fresh := queue(models.EventDeliveryPending, time.Now(), nil)
	sending := queue(models.EventDeliveryPending, old, &inFuture)
	delivered := queue(models.EventDeliveryDelivered, old, nil)

	n, err := repo.FailStaleDeliveries(ctx, time.Now().Add(-6*time.Hour), "job lost")
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	for id, want := range map[string]string{
		lost: models.EventDeliveryFailed, fresh: models.EventDeliveryPending,
		sending: models.EventDeliveryPending, delivered: models.EventDeliveryDelivered,
	} {
		saved, err := repo.GetDelivery(ctx, d.ID, id)
		require.NoError(t, err)
		assert.Equal(t, want, saved.Status, id)
		if want == models.EventDeliveryFailed {
			assert.Equal(t, "job lost", saved.LastError)
		}
	}
}
