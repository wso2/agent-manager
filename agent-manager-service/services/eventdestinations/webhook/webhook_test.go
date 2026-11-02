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

package webhook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// specVectorKey is the base64 key of the Standard Webhooks specification's
// published test vector; it is not a credential.
const specVectorKey = "MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

func TestSign_StandardWebhooksVector(t *testing.T) {
	sig, err := Sign(SecretPrefix+specVectorKey, "msg_p5jXN8AQM9LWM0D4loKWxJek",
		time.Unix(1614265330, 0), []byte(`{"test": 2432232314}`))
	require.NoError(t, err)
	assert.Equal(t, "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE=", sig)
}

func TestNormalizeConfig(t *testing.T) {
	d := New(false)
	ctx := context.Background()

	_, err := d.NormalizeConfig(ctx, json.RawMessage(`{"url":"http://hooks.example.com/x"}`))
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "plain http is refused")

	_, err = d.NormalizeConfig(ctx, json.RawMessage(`{"url":"https://127.0.0.1/x"}`))
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "a loopback address is refused")

	_, err = d.NormalizeConfig(ctx, json.RawMessage(`[]`))
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "config must be an object")

	local := New(true)
	cfg, err := local.NormalizeConfig(ctx, json.RawMessage(`{"url":"  http://localhost:8787/hook ", "extra": 1}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"url":"http://localhost:8787/hook"}`, string(cfg), "trimmed, unknown fields dropped")
}

func TestRedactConfigAndOrigin(t *testing.T) {
	d := New(false)
	cfg := json.RawMessage(`{"url":"https://hooks.slack.example/services/T0/B0/secret?token=x"}`)
	assert.JSONEq(t, `{"url":"https://hooks.slack.example/…"}`, string(d.RedactConfig(cfg)))
	assert.Equal(t, "https://hooks.slack.example", d.Origin(cfg))
	assert.JSONEq(t, `{"url":"https://hooks.example.com"}`, string(d.RedactConfig(json.RawMessage(`{"url":"https://hooks.example.com"}`))))
}

func TestNewSecret(t *testing.T) {
	secret, ok, err := New(false).NewSecret()
	require.NoError(t, err)
	assert.True(t, ok)
	key, err := base64.StdEncoding.DecodeString(secret[len(SecretPrefix):])
	require.NoError(t, err)
	assert.Len(t, key, secretBytes)
}

func TestDeliver_SignsAndClassifiesResponses(t *testing.T) {
	secret := SecretPrefix + base64.StdEncoding.EncodeToString([]byte("test-signing-key-for-unit-tests!"))
	body := []byte(`{"id":"evt_1"}`)
	status := http.StatusOK
	var got *http.Request
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
	}))
	defer srv.Close()

	d := New(true)
	d.now = func() time.Time { return time.Unix(1700000000, 0) }
	dest := eventdestinations.Destination{Config: json.RawMessage(`{"url":"` + srv.URL + `"}`), Secret: secret}
	msg := eventdestinations.Message{ID: "evt_1", Type: events.TypePrefix + "agent.deployed", Body: body}

	res := d.Deliver(context.Background(), dest, msg)
	require.NoError(t, res.Err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, body, gotBody)
	assert.Equal(t, events.MediaType, got.Header.Get("Content-Type"))
	assert.Equal(t, "evt_1", got.Header.Get(HeaderID))
	assert.Equal(t, strconv.FormatInt(1700000000, 10), got.Header.Get(HeaderTimestamp))
	assert.Equal(t, msg.Type, got.Header.Get(HeaderEventType))
	want, err := Sign(secret, "evt_1", time.Unix(1700000000, 0), body)
	require.NoError(t, err)
	assert.Equal(t, want, got.Header.Get(HeaderSignature))

	for code, retryable := range map[int]bool{500: true, 503: true, 429: true, 408: true, 400: false, 404: false, 410: false} {
		status = code
		res = d.Deliver(context.Background(), dest, msg)
		assert.Error(t, res.Err, "HTTP %d", code)
		assert.Equal(t, code, res.StatusCode)
		assert.Equal(t, retryable, res.Retryable, "HTTP %d", code)
	}

	srv.Close()
	res = d.Deliver(context.Background(), dest, msg)
	assert.Error(t, res.Err)
	assert.True(t, res.Retryable, "a network error is retried")
	var urlErr interface{ URL() string }
	assert.False(t, errors.As(res.Err, &urlErr))
	assert.NotContains(t, res.Err.Error(), srv.URL, "the URL is not echoed into the delivery log")
}
