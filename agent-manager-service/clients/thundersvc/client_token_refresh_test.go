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

package thundersvc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeThunder issues a new token ("tok-1", "tok-2", ...) on every token request
// and only accepts accepted() on the API path. Setting tokenStatus makes the
// token endpoint fail.
type fakeThunder struct {
	server      *httptest.Server
	tokenCalls  atomic.Int32
	apiCalls    atomic.Int32
	tokenStatus atomic.Int32
	accepted    func(issued int32) string
	lastBody    atomic.Value
}

func newFakeThunder(t *testing.T, accepted func(issued int32) string) *fakeThunder {
	t.Helper()
	f := &fakeThunder{accepted: accepted}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, _ *http.Request) {
		n := f.tokenCalls.Add(1)
		if status := int(f.tokenStatus.Load()); status != 0 {
			http.Error(w, "token endpoint down", status)
			return
		}
		// expires_in is deliberately far longer than the real lifetime, as seen
		// when Thunder stops accepting a token before its reported expiry.
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": fmt.Sprintf("tok-%d", n), "expires_in": 43200})
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		f.apiCalls.Add(1)
		body, _ := io.ReadAll(r.Body)
		f.lastBody.Store(string(body))
		if r.Header.Get("Authorization") != "Bearer "+f.accepted(f.tokenCalls.Load()) {
			http.Error(w, `{"code":"AUTH-4010"}`, http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeThunder) client(t *testing.T) *thunderClient {
	t.Helper()
	tc, ok := NewThunderClient(f.server.URL, "cid", "secret").(*thunderClient)
	require.True(t, ok)
	return tc
}

func (f *fakeThunder) call(tc *thunderClient, token string, payload any) ([]byte, error) {
	method := http.MethodGet
	if payload != nil {
		method = http.MethodPost
	}
	return tc.doRequest(context.Background(), method, f.server.URL+"/api", token, payload)
}

func seedCachedToken(tc *thunderClient, token string) {
	tc.cachedToken = token
	tc.tokenExpiry = time.Now().Add(time.Hour)
}

func TestDoRequest_RefreshesRejectedSystemToken(t *testing.T) {
	f := newFakeThunder(t, func(int32) string { return "tok-1" })
	tc := f.client(t)
	seedCachedToken(tc, "stale")

	body, err := f.call(tc, "stale", nil)

	require.NoError(t, err)
	assert.Equal(t, "ok", string(body))
	assert.EqualValues(t, 1, f.tokenCalls.Load(), "exactly one new token is fetched")
	assert.EqualValues(t, 2, f.apiCalls.Load(), "the request is retried once")

	_, err = f.call(tc, "tok-1", nil)
	require.NoError(t, err)
	assert.EqualValues(t, 1, f.tokenCalls.Load(), "the refreshed token is cached for later requests")
}

func TestDoRequest_RetryReplaysRequestBody(t *testing.T) {
	f := newFakeThunder(t, func(int32) string { return "tok-1" })
	tc := f.client(t)
	seedCachedToken(tc, "stale")

	_, err := f.call(tc, "stale", map[string]string{"name": "demo"})

	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"demo"}`, f.lastBody.Load().(string), "the retry must resend the original body")
}

func TestDoRequest_ValidTokenIsNotRefreshed(t *testing.T) {
	f := newFakeThunder(t, func(int32) string { return "good" })
	tc := f.client(t)
	seedCachedToken(tc, "good")

	_, err := f.call(tc, "good", nil)

	require.NoError(t, err)
	assert.EqualValues(t, 0, f.tokenCalls.Load())
	assert.EqualValues(t, 1, f.apiCalls.Load())
}

func TestDoRequest_RetriesOnlyOnce(t *testing.T) {
	f := newFakeThunder(t, func(int32) string { return "never-issued" })
	tc := f.client(t)
	seedCachedToken(tc, "stale")

	_, err := f.call(tc, "stale", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 401")
	assert.EqualValues(t, 2, f.apiCalls.Load(), "one retry, no loop")
	assert.EqualValues(t, 1, f.tokenCalls.Load())
}

func TestDoRequest_ReturnsOriginal401WhenRefreshFails(t *testing.T) {
	f := newFakeThunder(t, func(int32) string { return "tok-1" })
	f.tokenStatus.Store(http.StatusInternalServerError)
	tc := f.client(t)
	seedCachedToken(tc, "stale")

	_, err := f.call(tc, "stale", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 401", "the caller sees the Thunder rejection, not the token error")
	assert.EqualValues(t, 1, f.apiCalls.Load(), "no retry without a new token")
}

func TestDoRequest_DoesNotRetryOtherErrors(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
			}))
			defer server.Close()
			tc, ok := NewThunderClient(server.URL, "cid", "secret").(*thunderClient)
			require.True(t, ok)

			_, err := tc.doRequest(context.Background(), http.MethodGet, server.URL, "tok", nil)

			require.Error(t, err)
			assert.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestDoRequest_ControlPlaneManagedClientIsNotRetried(t *testing.T) {
	f := newFakeThunder(t, func(int32) string { return "tok-1" })
	tc := f.client(t)
	tc.cpManaged = true

	_, err := f.call(tc, cpManagedPlaceholderToken, nil)

	require.Error(t, err)
	assert.EqualValues(t, 1, f.apiCalls.Load())
	assert.EqualValues(t, 0, f.tokenCalls.Load(), "a control-plane client never mints a system token")
}

func TestDoRequest_ConcurrentRejectionsShareOneRefresh(t *testing.T) {
	f := newFakeThunder(t, func(int32) string { return "tok-1" })
	tc := f.client(t)
	seedCachedToken(tc, "stale")

	const callers = 8
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			_, err := f.call(tc, "stale", nil)
			errs <- err
		}()
	}
	for i := 0; i < callers; i++ {
		require.NoError(t, <-errs)
	}
	assert.EqualValues(t, 1, f.tokenCalls.Load(), "callers that were rejected together share one token fetch")
}
