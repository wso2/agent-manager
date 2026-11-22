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
	"encoding/json"
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

func TestCheck(t *testing.T) {
	for name, tc := range map[string]struct {
		status  int
		body    string
		healthy bool
	}{
		"missing key":   {http.StatusNotFound, `{"errors":[]}`, true},
		"present key":   {http.StatusOK, `{"data":{}}`, true},
		"missing mount": {http.StatusNotFound, `{"errors":["no handler for route \"nope/data/amp/healthz\". route entry not found."]}`, false},
		"404 no body":   {http.StatusNotFound, ``, false},
		"forbidden":     {http.StatusForbidden, `{"errors":["permission denied"]}`, false},
		"unavailable":   {http.StatusServiceUnavailable, ``, false},
	} {
		v, calls := newTestVault(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		})
		err := v.Check(t.Context())
		assert.Equal(t, tc.healthy, err == nil, "%s: %v", name, err)
		assert.Equal(t, []string{"GET /v1/secret/data/amp/healthz"}, *calls)
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
