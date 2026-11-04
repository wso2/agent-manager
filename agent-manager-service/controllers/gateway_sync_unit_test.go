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

package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcceptsTarGzip(t *testing.T) {
	for accept, want := range map[string]bool{
		"":                                    true,
		"application/x-tar+gzip":              true,
		"application/x-tar+gzip, */*":         true,
		"*/*":                                 true,
		"application/*":                       true,
		"application/json, */*;q=0.1":         true,
		"application/x-tar+gzip; q=0.5":       true,
		"APPLICATION/X-TAR+GZIP":              true,
		"application/json":                    false,
		"application/json, text/plain":        false,
		"application/x-tar+gzip;q=0":          false,
		"application/x-tar+gzip;q=0, */*;q=0": false,
	} {
		assert.Equal(t, want, acceptsTarGzip(accept), "Accept: %q", accept)
	}
}

func idsBody(field string, n int) string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%d", i)
	}
	body, _ := json.Marshal(map[string][]string{field: ids})
	return string(body)
}

func TestDecodeSyncIDsCapsTheList(t *testing.T) {
	for _, c := range []struct {
		name  string
		field string
		ids   func(syncIDsRequest) []string
	}{
		{"fetch-batch", "deploymentIds", syncIDsRequest.deploymentIDs},
		{"artifacts/exists", "artifactIds", syncIDsRequest.artifactIDs},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			got, ok := decodeSyncIDs(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(idsBody(c.field, maxSyncIDs))), c.ids)
			require.True(t, ok)
			assert.Len(t, got, maxSyncIDs)

			w = httptest.NewRecorder()
			_, ok = decodeSyncIDs(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(idsBody(c.field, maxSyncIDs+1))), c.ids)
			assert.False(t, ok)
			assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
		})
	}
}

func TestDecodeSyncIDsRejectsABadBody(t *testing.T) {
	w := httptest.NewRecorder()
	_, ok := decodeSyncIDs(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{")), syncIDsRequest.deploymentIDs)
	assert.False(t, ok)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
