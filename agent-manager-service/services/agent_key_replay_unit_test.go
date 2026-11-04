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
package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

type keyReplayFixture struct {
	hub       *recordingEventHub
	events    *GatewayEventsService
	placement models.Placement
	now       time.Time
	tx        *gorm.DB
	keys      *repomocks.APIKeyRepositoryMock
}

func newKeyReplayFixture(t *testing.T) *keyReplayFixture {
	t.Helper()
	f := &keyReplayFixture{
		hub: &recordingEventHub{},
		placement: models.Placement{
			ArtifactRef: models.ArtifactRef{OUID: "ou-1", ArtifactUUID: uuid.New(), Kind: models.KindRestAPI},
			GatewayUUID: uuid.New(),
		},
		now: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC),
		tx:  &gorm.DB{},
	}
	f.events = NewGatewayEventsService(f.hub)
	f.storing()
	return f
}

// storing makes the repository return stored for this fixture's artifact, in this fixture's transaction.
func (f *keyReplayFixture) storing(stored ...models.StoredAPIKey) {
	f.keys = &repomocks.APIKeyRepositoryMock{
		ListActiveByArtifactTxFunc: func(_ context.Context, tx *gorm.DB, artifact uuid.UUID, _ time.Time) ([]models.StoredAPIKey, error) {
			if tx != f.tx || artifact != f.placement.ArtifactUUID {
				return nil, nil
			}
			return stored, nil
		},
	}
}

func (f *keyReplayFixture) replay() error {
	return replayAgentKeys(context.Background(), f.tx, f.keys, f.events, f.placement, f.now)
}

func (f *keyReplayFixture) storedKey(name, hash string) models.StoredAPIKey {
	return models.StoredAPIKey{
		UUID: uuid.New(), Name: name, DisplayName: name + " key", ArtifactUUID: f.placement.ArtifactUUID,
		APIKeyHash: hash, MaskedAPIKey: "****" + hash[:4], Status: "active",
		CreatedAt: f.now.Add(-time.Hour), UpdatedAt: f.now.Add(-time.Hour),
	}
}

func createdPayload(t *testing.T, evt eventhub.Event) models.APIKeyCreatedEvent {
	t.Helper()
	var envelope struct {
		Payload models.APIKeyCreatedEvent `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(evt.EventData), &envelope))
	return envelope.Payload
}

func TestReplayAgentKeysWritesOneEventPerActiveKey(t *testing.T) {
	f := newKeyReplayFixture(t)
	first, second := f.storedKey("first", "aaaa1111"), f.storedKey("second", "bbbb2222")
	f.storing(first, second)

	require.NoError(t, f.replay())

	require.Len(t, f.hub.published, 2)
	for i, key := range []models.StoredAPIKey{first, second} {
		evt := f.hub.published[i]
		assert.Equal(t, "apikey.created", string(evt.EventType))
		assert.Equal(t, key.UUID.String(), evt.EntityID)
		assert.Equal(t, f.placement.GatewayUUID.String(), evt.GatewayID)
		assert.Same(t, f.tx, f.hub.txs[i])
		payload := createdPayload(t, evt)
		assert.Equal(t, f.placement.ArtifactUUID.String(), payload.APIID)
		assert.Equal(t, key.Name, payload.Name)
		assert.Equal(t, `{"sha256":"`+key.APIKeyHash+`"}`, payload.ApiKeyHashes)
		assert.Equal(t, `["*"]`, payload.Operations)
		assert.Nil(t, payload.ExpiresAt)
	}
}

func TestReplayAgentKeysNoKeys(t *testing.T) {
	f := newKeyReplayFixture(t)

	require.NoError(t, f.replay())

	assert.Empty(t, f.hub.published)
}

func TestReplayAgentKeysTwiceWritesTwice(t *testing.T) {
	f := newKeyReplayFixture(t)
	f.storing(f.storedKey("only", "cccc3333"))

	require.NoError(t, f.replay())
	require.NoError(t, f.replay())

	require.Len(t, f.hub.published, 2)
	assert.Empty(t, f.hub.published[0].EventID, "the hub generates the event id")
}

func TestReplayAgentKeysAsksForKeysUsableAtNow(t *testing.T) {
	f := newKeyReplayFixture(t)
	var askedAt time.Time
	f.keys.ListActiveByArtifactTxFunc = func(_ context.Context, _ *gorm.DB, _ uuid.UUID, now time.Time) ([]models.StoredAPIKey, error) {
		askedAt = now
		return nil, nil
	}

	require.NoError(t, f.replay())

	assert.Equal(t, f.now, askedAt)
}

func TestApiKeyCreatedEventCarriesExpiry(t *testing.T) {
	f := newKeyReplayFixture(t)
	key := f.storedKey("expiring", "dddd4444")
	expiresAt := f.now.Add(24 * time.Hour)
	key.ExpiresAt = &expiresAt

	event := apiKeyCreatedEventOf(key)

	require.NotNil(t, event.ExpiresAt)
	assert.Equal(t, "2026-11-01T12:00:00Z", *event.ExpiresAt)
}
