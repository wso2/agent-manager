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

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A flush before any header commits 200; a later WriteHeader can't change
// what the client received, so the recorder keeps 200.
func TestResponseRecorderFlushCommitsStatus(t *testing.T) {
	under := httptest.NewRecorder()
	rec := newResponseRecorder(under)

	rec.Flush()
	rec.WriteHeader(http.StatusInternalServerError)

	if under.Code != http.StatusOK {
		t.Fatalf("client got %d; want 200", under.Code)
	}
	if got := rec.Status(); got != http.StatusOK {
		t.Errorf("recorded %d; want 200", got)
	}
}

// A status written before the flush is kept.
func TestResponseRecorderFlushKeepsWrittenStatus(t *testing.T) {
	rec := newResponseRecorder(httptest.NewRecorder())

	rec.WriteHeader(http.StatusAccepted)
	rec.Flush()

	if got := rec.Status(); got != http.StatusAccepted {
		t.Errorf("recorded %d; want 202", got)
	}
}
