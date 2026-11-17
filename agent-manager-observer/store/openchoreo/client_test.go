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
package openchoreo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient returns a Reader for srv that gets its token from a stub token server.
func newTestClient(t *testing.T, srv *httptest.Server) Reader {
	t.Helper()
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"t","expires_in":3600}`))
	}))
	t.Cleanup(tokenSrv.Close)

	return NewClient(srv.URL, NewAuthProvider(tokenSrv.URL, "client-id", "client-secret"), "default")
}

func TestClientQueryTraceSpans_EscapesTraceID(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spans": []}`))
	}))
	defer srv.Close()

	if _, err := newTestClient(t, srv).QueryTraceSpans(context.Background(), "../../etc/passwd", TracesQueryRequest{}); err != nil {
		t.Fatalf("QueryTraceSpans returned error: %v", err)
	}

	const want = "/api/v1alpha1/traces/..%2F..%2Fetc%2Fpasswd/spans/query"
	if gotPath != want {
		t.Errorf("expected escaped path %q, got %q", want, gotPath)
	}
}

func TestClientGetSpanDetails_EscapesTraceAndSpanID(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	if _, err := newTestClient(t, srv).GetSpanDetails(context.Background(), "trace/../escape", "span?evil=1"); err != nil {
		t.Fatalf("GetSpanDetails returned error: %v", err)
	}

	const want = "/api/v1alpha1/traces/trace%2F..%2Fescape/spans/span%3Fevil=1"
	if gotPath != want {
		t.Errorf("expected escaped path %q, got %q", want, gotPath)
	}
}

type stubTokenProvider struct {
	token       string
	invalidated int
}

func (s *stubTokenProvider) GetToken(context.Context) (string, error) { return s.token, nil }
func (s *stubTokenProvider) InvalidateToken()                         { s.invalidated++ }

func TestClient_RefreshesTokenOn401(t *testing.T) {
	var gotAuth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		if len(gotAuth) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spans": []}`))
	}))
	defer srv.Close()

	tp := &stubTokenProvider{token: "caller-jwt"}
	if _, err := NewClient(srv.URL, tp, "default").QueryTraceSpans(context.Background(), "trace-1", TracesQueryRequest{}); err != nil {
		t.Fatalf("QueryTraceSpans returned error: %v", err)
	}
	if len(gotAuth) != 2 {
		t.Fatalf("expected 2 calls (401 then retry), got %d", len(gotAuth))
	}
	for i, h := range gotAuth {
		if h != "Bearer caller-jwt" {
			t.Errorf("call %d: Authorization = %q, want %q", i+1, h, "Bearer caller-jwt")
		}
	}
	if tp.invalidated != 1 {
		t.Errorf("InvalidateToken calls = %d, want 1", tp.invalidated)
	}
}
