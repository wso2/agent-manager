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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "s3cret-shared-with-the-sender"

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"event_id":"e1"}`)
	now := time.Unix(1_800_000_000, 0)
	good := Sign(testSecret, body, now)

	assert.NoError(t, verifySignature(testSecret, good, body, 5*time.Minute, now))
	assert.NoError(t, verifySignature(testSecret, good, body, 5*time.Minute, now.Add(4*time.Minute)))
	assert.ErrorIs(t, verifySignature(testSecret, good, body, 5*time.Minute, now.Add(6*time.Minute)),
		errTimestampOutOfTolerance, "a stale signature is a possible replay")
	assert.ErrorIs(t, verifySignature(testSecret, good, body, 5*time.Minute, now.Add(-6*time.Minute)),
		errTimestampOutOfTolerance, "so is a future-dated one")
	assert.ErrorIs(t, verifySignature("another-secret", good, body, 5*time.Minute, now), errSignatureInvalid)
	assert.ErrorIs(t, verifySignature(testSecret, good, []byte(`{"event_id":"e2"}`), 5*time.Minute, now),
		errSignatureInvalid, "the signature covers the body")
	assert.ErrorIs(t, verifySignature(testSecret, "", body, 5*time.Minute, now), errSignatureMissing)
	assert.ErrorIs(t, verifySignature(testSecret, "t=abc,v1=00", body, 5*time.Minute, now), errSignatureInvalid)
	assert.ErrorIs(t, verifySignature(testSecret, "v1=00", body, 5*time.Minute, now), errSignatureInvalid)
}

func TestSecretHash(t *testing.T) {
	h := SecretHash(testSecret)
	assert.Len(t, h, 64, "hex SHA3-256")
	assert.Equal(t, h, SecretHash(testSecret))
	assert.NotEqual(t, h, SecretHash(testSecret+"x"))
	assert.Equal(t, strings.ToLower(h), h)
}

func TestAuthenticate(t *testing.T) {
	now := time.Now()
	r := &webhookReceiver{cfg: webhookConfig{Secret: testSecret, SignatureTolerance: time.Minute}, now: func() time.Time { return now }}
	withHash := []byte(`{"event_id":"e1","secret_hash":"` + SecretHash(testSecret) + `"}`)
	assert.NoError(t, r.authenticate(Sign(testSecret, withHash, now), withHash))

	otherHash := []byte(`{"event_id":"e1","secret_hash":"` + SecretHash("other") + `"}`)
	assert.ErrorIs(t, r.authenticate(Sign(testSecret, otherHash, now), otherHash), errSignatureInvalid,
		"a delivery naming a secret this receiver doesn't hold is refused")

	noHash := []byte(`{"event_id":"e1"}`)
	assert.ErrorIs(t, r.authenticate(Sign(testSecret, noHash, now), noHash), errSignatureInvalid,
		"without secret_hash, only when legacy deliveries are allowed")
	r.cfg.LegacySharedSecret = true
	assert.NoError(t, r.authenticate(Sign(testSecret, noHash, now), noHash))
}

func TestEnvelope(t *testing.T) {
	env, err := decodeEnvelope([]byte(`{"event_id":"e1","event_type":"subscription_plan.created","org":{"ref_id":"acme"},"occurred_at":"2026-11-18T10:00:00Z"}`))
	require.NoError(t, err)
	require.NoError(t, env.validate())
	assert.Equal(t, "acme", env.OrgID, "org.ref_id is the organization handle")
	assert.Equal(t, time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC), env.occurredAt())

	legacy, err := decodeEnvelope([]byte(`{"event_id":"e1","event_type":"x","org_id":"acme"}`))
	require.NoError(t, err)
	assert.Equal(t, "acme", legacy.OrgID, "the flat org_id is still read")

	for name, body := range map[string]string{
		"no event id":   `{"event_type":"x","org":{"ref_id":"acme"}}`,
		"no event type": `{"event_id":"e1","org":{"ref_id":"acme"}}`,
		"no org":        `{"event_id":"e1","event_type":"x"}`,
	} {
		env, err := decodeEnvelope([]byte(body))
		require.NoError(t, err, name)
		assert.ErrorIs(t, env.validate(), errInvalidEnvelope, name)
	}
	_, err = decodeEnvelope([]byte(`not json`))
	assert.ErrorIs(t, err, errInvalidEnvelope)
}

func TestRequestWindowsFrom(t *testing.T) {
	got := requestWindowsFrom([]webhookLimit{
		{LimitType: "REQUEST_COUNT", LimitCount: 100, TimeUnit: "minute", TimeAmount: 0},
		{LimitType: "REQUEST_COUNT", LimitCount: -1, TimeUnit: "HOUR"},                 // unlimited
		{LimitType: "BANDWIDTH", LimitCount: 5, TimeUnit: "HOUR"},                      // unsupported
		{LimitType: "REQUEST_COUNT", LimitCount: 7, TimeUnit: "WEEK"},                  // unknown unit
		{LimitType: "REQUEST_COUNT", LimitCount: 9, TimeUnit: "MINUTE", TimeAmount: 1}, // same window length
		{LimitType: "request_count", LimitCount: 1000, TimeUnit: "DAY", TimeAmount: 1},
	})
	assert.Equal(t, []Window{{Count: 100, TimeAmount: 1, TimeUnit: "MINUTE"}, {Count: 1000, TimeAmount: 1, TimeUnit: "DAY"}}, got)
}

func TestWebhookConditions(t *testing.T) {
	allow := Condition{
		ID: ConditionIDRequests, Type: ConditionTypeRequestCount,
		Params: json.RawMessage(`{"limits":[{"count":5,"timeAmount":1,"timeUnit":"MINUTE"}],"onExceed":"allow"}`),
	}
	custom := Condition{ID: "region", Type: ConditionTypeCustom, Params: json.RawMessage(`{"a":1}`)}
	current, err := normalizeConditions([]Condition{allow, custom})
	require.NoError(t, err)

	t.Run("conditions in the event replace all", func(t *testing.T) {
		got, err := webhookConditions(current, &[]Condition{custom}, nil)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "region", got[0].ID)
	})
	t.Run("limits from an older sender replace only the request-count condition, keeping onExceed", func(t *testing.T) {
		got, err := webhookConditions(current, nil, []Window{{Count: 50, TimeAmount: 1, TimeUnit: "HOUR"}})
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.JSONEq(t, `{"limits":[{"count":50,"timeAmount":1,"timeUnit":"HOUR"}],"onExceed":"allow"}`, string(got[0].Params))
		assert.Equal(t, "region", got[1].ID)
	})
	t.Run("no limits remove the request-count condition", func(t *testing.T) {
		got, err := webhookConditions(current, nil, nil)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "region", got[0].ID)
	})
}

func TestWebhookConfigFrom(t *testing.T) {
	settings := map[string]string{}
	get := func(k string) string { return settings[k] }
	cfg := webhookConfigFrom(get, slog.New(slog.DiscardHandler))
	assert.False(t, cfg.Enabled, "off without a secret")
	assert.Equal(t, DefaultWebhookSignatureHeader, cfg.SignatureHeader)
	assert.Equal(t, 5*time.Minute, cfg.SignatureTolerance)
	assert.EqualValues(t, 1<<20, cfg.MaxBodyBytes)

	settings["webhook_secret"] = testSecret
	settings["webhook_signature_tolerance"] = "90s"
	settings["webhook_max_body_size"] = "2048"
	cfg = webhookConfigFrom(get, slog.New(slog.DiscardHandler))
	assert.True(t, cfg.Enabled, "on once a secret is set")
	assert.Equal(t, 90*time.Second, cfg.SignatureTolerance)
	assert.EqualValues(t, 2048, cfg.MaxBodyBytes)

	settings["webhook_enabled"] = "false"
	assert.False(t, webhookConfigFrom(get, slog.New(slog.DiscardHandler)).Enabled, "and can be switched off")
}

func TestReceiverRejectsAlike(t *testing.T) {
	r := &webhookReceiver{cfg: webhookConfig{
		Enabled: true, Secret: testSecret, SignatureHeader: DefaultWebhookSignatureHeader,
		SignatureTolerance: time.Minute, MaxBodyBytes: 1 << 20,
	}, log: slog.New(slog.DiscardHandler), now: time.Now}
	for name, header := range map[string]string{"no signature": "", "bad signature": "t=1,v1=00"} {
		req := httptest.NewRequest(http.MethodPost, WebhookRoutePath, strings.NewReader(`{"event_id":"e1"}`))
		if header != "" {
			req.Header.Set(DefaultWebhookSignatureHeader, header)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, name)
		assert.JSONEq(t, `{"status":"error","code":"UNAUTHORIZED","message":"Invalid or expired credentials."}`, rec.Body.String(), name)
	}
	assert.True(t, isClientFault(errSignatureInvalid))
	assert.False(t, isClientFault(errors.New("database is down")))
}
