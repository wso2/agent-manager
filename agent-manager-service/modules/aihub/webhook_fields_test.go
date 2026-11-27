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
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A sealed field opens with the webhook secret, as the sender sealed it; anything else is refused.
func TestDecryptField(t *testing.T) {
	iv := []byte("0123456789ab")
	sealed, err := SealField(testSecret, "the-subscription-key", iv)
	require.NoError(t, err)

	got, err := decryptField(testSecret, sealed)
	require.NoError(t, err)
	assert.Equal(t, "the-subscription-key", got)

	_, err = decryptField("another-secret-of-thirty-two-chars!", sealed)
	assert.True(t, errors.Is(err, errInvalidEnvelope), "a field sealed with another secret: %v", err)

	var f encryptedField
	require.NoError(t, json.Unmarshal(sealed, &f))
	f.Tag = "AAAAAAAAAAAAAAAAAAAAAA=="
	tampered, _ := json.Marshal(f)
	_, err = decryptField(testSecret, tampered)
	assert.True(t, errors.Is(err, errInvalidEnvelope), "a tampered field: %v", err)

	_, err = decryptField(testSecret, json.RawMessage(`"plain-key"`))
	assert.True(t, errors.Is(err, errInvalidEnvelope), "a plaintext field is refused")
}

func TestSubscriptionChangeFromAnEvent(t *testing.T) {
	r := &webhookReceiver{cfg: webhookConfig{Secret: testSecret}, log: slog.New(slog.DiscardHandler)}
	token, err := SealField(testSecret, "k-1", []byte("0123456789ab"))
	require.NoError(t, err)
	data := func(apiType string, withToken bool) json.RawMessage {
		d := map[string]any{
			"subscription_id": "s-1", "subscriber_id": "alice", "status": "ACTIVE",
			"subscription_plan": map[string]any{"ref_id": "gold", "name": "Gold"},
			"api":               map[string]any{"ref_id": "docs-mcp", "type": apiType, "name": "Docs", "version": "v1.0"},
		}
		if withToken {
			d["token"] = token
		}
		raw, _ := json.Marshal(d)
		return raw
	}

	ch, err := r.subscriptionChange(&envelope{EventType: EventSubscriptionCreated, Data: data("Mcp", true)})
	require.NoError(t, err)
	assert.Equal(t, "created", ch.Event)
	assert.Equal(t, "s-1", ch.SourceID)
	assert.Equal(t, "docs-mcp", ch.ArtifactHandle)
	assert.Equal(t, "gold", ch.PlanHandle)
	require.NotNil(t, ch.Token)
	assert.Equal(t, "k-1", *ch.Token)

	_, err = r.subscriptionChange(&envelope{EventType: EventSubscriptionCreated, Data: data("RestApi", true)})
	assert.True(t, errors.Is(err, errUnsupportedEvent), "only MCP servers take subscriptions here")

	ch, err = r.subscriptionChange(&envelope{EventType: EventSubscriptionUpdated, Data: data("Mcp", false)})
	require.NoError(t, err)
	assert.Nil(t, ch.Token, "an update carries no token")

	_, err = r.subscriptionChange(&envelope{EventType: EventSubscriptionDeleted, Data: json.RawMessage(`{"api":{}}`)})
	assert.True(t, errors.Is(err, errInvalidEnvelope), "a subscription id is required")
}
