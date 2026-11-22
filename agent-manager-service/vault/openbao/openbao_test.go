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

package openbao

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/vault"
)

func newTestVault(t *testing.T, handler http.HandlerFunc) (*Vault, *[]string) {
	t.Helper()
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	v, err := New(server.URL, "token", "")
	require.NoError(t, err)
	return v, &calls
}

func testRef() vault.Ref {
	return vault.Ref{OperationID: uuid.New(), OrgID: uuid.NewString(), SecretID: uuid.New(), OwnerKind: "test"}
}

// The API's policy grants create but not update on data paths, so OpenBao refuses the placeholder write with 403
// once the staged value exists. Abandon must still destroy that value.
func TestAbandonDestroysAValueTheWriterStoredFirst(t *testing.T) {
	for _, refused := range []int{http.StatusBadRequest, http.StatusForbidden} {
		v, calls := newTestVault(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				w.WriteHeader(refused)
			}
		})
		ref := testRef()
		path, err := v.Reserve(ref)
		require.NoError(t, err)
		require.NoError(t, v.Abandon(t.Context(), ref, path), "placeholder answered %d", refused)
		assert.Equal(t, []string{"POST /v1/secret/data/" + path, "PUT /v1/secret/destroy/" + path}, *calls)
	}
}

func TestAbandonFailsWhenTheTokenCannotDestroy(t *testing.T) {
	v, _ := newTestVault(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) })
	ref := testRef()
	path, err := v.Reserve(ref)
	require.NoError(t, err)
	require.ErrorContains(t, v.Abandon(t.Context(), ref, path), "HTTP 403")
}

// kvServer imitates a mount. With v2 it answers reads as KV v2 does, with version metadata; otherwise it returns the
// stored write body as KV v1 does. A missing key answers 404 with an empty error list in both.
func kvServer(t *testing.T, v2 bool) http.HandlerFunc {
	stored := map[string]json.RawMessage{}
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			if _, exists := stored[r.URL.Path]; exists {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			stored[r.URL.Path] = body
		case http.MethodGet:
			body, ok := stored[r.URL.Path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errors":[]}`))
				return
			}
			if !v2 {
				_, _ = fmt.Fprintf(w, `{"data":%s}`, body)
				return
			}
			var req writeRequest
			require.NoError(t, json.Unmarshal(body, &req))
			data, err := json.Marshal(req.Data)
			require.NoError(t, err)
			_, _ = fmt.Fprintf(w, `{"data":{"data":%s,"metadata":{"version":1}}}`, data)
		}
	}
}

func TestCheckAcceptsOnlyKVv2(t *testing.T) {
	v, calls := newTestVault(t, kvServer(t, true))
	require.NoError(t, v.Check(t.Context()), "first check creates the probe")
	path := "/v1/secret/data/amp/healthz"
	assert.Equal(t, []string{"GET " + path, "POST " + path, "GET " + path}, *calls)
	require.NoError(t, v.Check(t.Context()), "later checks read the existing probe")
	assert.Len(t, *calls, 4)

	v1, _ := newTestVault(t, kvServer(t, false))
	require.ErrorContains(t, v1.Check(t.Context()), "not KV v2")
	require.ErrorContains(t, v1.Check(t.Context()), "not KV v2", "a KV v1 probe left by an earlier check")
}

func TestCheckFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"missing mount": {http.StatusNotFound, `{"errors":["no handler for route \"nope/data/amp/healthz\". route entry not found."]}`, "mount does not exist"},
		"404 no body":   {http.StatusNotFound, ``, "mount does not exist"},
		"forbidden":     {http.StatusForbidden, `{"errors":["permission denied"]}`, "HTTP 403"},
		"unavailable":   {http.StatusServiceUnavailable, ``, "HTTP 503"},
		"other value":   {http.StatusOK, `{"data":{"data":{"value":"b3RoZXI="},"metadata":{"version":1}}}`, "not KV v2"},
	} {
		v, _ := newTestVault(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		})
		assert.ErrorContains(t, v.Check(t.Context()), tc.want, name)
	}
}

// A value is refused before it reaches OpenBao unless its read response is sure to fit the decoder's limit.
func TestSealRefusesValuesItCouldNotReadBack(t *testing.T) {
	stored := map[string][]byte{}
	v, _ := newTestVault(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var req writeRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			stored[r.URL.Path] = req.Data.Value
		case http.MethodGet:
			var res readResponse
			res.Data.Data.Value = stored[r.URL.Path]
			require.NoError(t, json.NewEncoder(w).Encode(res))
		}
	})
	value := bytes.Repeat([]byte{0xff}, MaxValueBytes)
	ref := testRef()
	sealed, err := v.Seal(t.Context(), ref, value, nil)
	require.NoError(t, err)
	got, err := v.Open(t.Context(), ref, sealed)
	require.NoError(t, err, "the largest accepted value reads back")
	assert.Equal(t, value, got)

	stored = map[string][]byte{}
	_, err = v.Seal(t.Context(), testRef(), append(value, 0), nil)
	require.ErrorContains(t, err, "more than")
	assert.Empty(t, stored, "an oversized value never reaches OpenBao")
}

func TestNewTransportRules(t *testing.T) {
	for _, tt := range []struct {
		addr  string
		opts  []Option
		allow bool
	}{
		{addr: "https://openbao.example.com:8200", allow: true},
		{addr: "http://127.0.0.1:8200", allow: true},
		{addr: "http://localhost:8200", allow: true},
		{addr: "http://[::1]:8200", allow: true},
		{addr: "http://openbao.example.com:8200"},
		{addr: "http://172.17.0.2:8200"},
		{addr: "http://172.17.0.2:8200", opts: []Option{AllowInsecureHTTP()}, allow: true},
		{addr: "http://host.docker.internal:8200", opts: []Option{AllowInsecureHTTP()}, allow: true},
	} {
		_, err := New(tt.addr, "token", "", tt.opts...)
		if tt.allow {
			assert.NoError(t, err, tt.addr)
		} else {
			assert.ErrorContains(t, err, "must use HTTPS", tt.addr)
		}
	}
}

// An HTTPS server is reached with certificate verification on: an untrusted
// certificate fails, and redirects are not followed.
func TestHTTPSKeepsVerificationAndRefusesRedirects(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example/", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(server.Close)
	v, err := New(server.URL, "token", "")
	require.NoError(t, err)
	err = v.Check(context.Background())
	require.Error(t, err, "the test server's certificate is not trusted")

	v.client.Transport = server.Client().Transport
	err = v.Check(context.Background())
	require.Error(t, err, "a redirect is an error, not followed")
}
