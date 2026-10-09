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

package agentmanager

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/middleware"
)

var (
	testStart = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	testEnd   = time.Date(2026, 9, 1, 12, 0, 0, 500, time.UTC)
)

// withToken is a context carrying the caller's bearer token.
func withToken(t *testing.T) context.Context {
	return middleware.ContextWithBearerToken(t.Context(), "caller-token")
}

// serve starts a server that answers every request with status and body.
func serve(t *testing.T, status int, body string, inspect func(r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if inspect != nil {
			inspect(r)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The request names the agent in the path, carries the caller's token, the
// window, the trace IDs and the evaluator, and the response maps to scores.
func TestTraceScores_Request(t *testing.T) {
	var got *http.Request
	srv := serve(t, http.StatusOK,
		`{"traces":[{"traceId":"a","score":0.4,"totalCount":2,"skippedCount":0},{"traceId":"b","score":null,"totalCount":1,"skippedCount":1}],"totalCount":2}`,
		func(r *http.Request) { got = r.Clone(context.Background()) })

	scores, err := NewClient(srv.URL+"/").TraceScores(withToken(t), "org 1", "proj", "agent/x", testStart, testEnd, []string{"a", "b", "c"}, "Answer Relevance")
	if err != nil {
		t.Fatalf("TraceScores: %v", err)
	}

	if want := "/api/v1/orgs/org%201/projects/proj/agents/agent%2Fx/scores"; got.URL.EscapedPath() != want {
		t.Errorf("path = %s, want %s", got.URL.EscapedPath(), want)
	}
	if auth := got.Header.Get("Authorization"); auth != "Bearer caller-token" {
		t.Errorf("Authorization = %q, want the caller's token", auth)
	}
	q := got.URL.Query()
	for key, want := range map[string]string{
		"startTime": "2026-09-01T00:00:00Z",
		"endTime":   "2026-09-01T12:00:00.0000005Z",
		"traceIds":  "a,b,c",
		"evaluator": "Answer Relevance",
	} {
		if q.Get(key) != want {
			t.Errorf("%s = %q, want %q", key, q.Get(key), want)
		}
	}
	if q.Has("limit") || q.Has("offset") {
		t.Errorf("query %s sets limit or offset; the default limit covers every ID", got.URL.RawQuery)
	}
	if len(scores) != 2 || scores["a"] == nil || *scores["a"] != 0.4 || scores["b"] != nil {
		t.Errorf("scores = %v, want a=0.4, b=nil, c absent", scores)
	}
	if _, ok := scores["c"]; ok {
		t.Error("an unscored trace is present")
	}
}

// Without an evaluator the query has no evaluator param.
func TestTraceScores_NoEvaluator(t *testing.T) {
	var query string
	srv := serve(t, http.StatusOK, `{"traces":[],"totalCount":0}`, func(r *http.Request) { query = r.URL.RawQuery })

	if _, err := NewClient(srv.URL).TraceScores(withToken(t), "o", "p", "a", testStart, testEnd, []string{"a"}, ""); err != nil {
		t.Fatalf("TraceScores: %v", err)
	}
	if q, _ := url.ParseQuery(query); q.Has("evaluator") {
		t.Errorf("query %s has an evaluator", query)
	}
}

// 401 and 403 are ErrForbidden; other statuses and bad bodies are plain errors.
func TestTraceScores_Errors(t *testing.T) {
	tests := []struct {
		name          string
		status        int
		body          string
		wantForbidden bool
	}{
		{name: "401", status: http.StatusUnauthorized, wantForbidden: true},
		{name: "403", status: http.StatusForbidden, wantForbidden: true},
		{name: "400", status: http.StatusBadRequest},
		{name: "500", status: http.StatusInternalServerError},
		{name: "503", status: http.StatusServiceUnavailable},
		{name: "bad JSON", status: http.StatusOK, body: `{"traces":[`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := serve(t, tt.status, tt.body, nil)
			_, err := NewClient(srv.URL).TraceScores(withToken(t), "o", "p", "a", testStart, testEnd, []string{"a"}, "")
			if err == nil {
				t.Fatal("TraceScores succeeded, want an error")
			}
			if got := errors.Is(err, ErrForbidden); got != tt.wantForbidden {
				t.Errorf("errors.Is(err, ErrForbidden) = %v, want %v (err: %v)", got, tt.wantForbidden, err)
			}
		})
	}
}

// A request with no caller token never reaches the service.
func TestTraceScores_NoTokenMakesNoCall(t *testing.T) {
	var calls atomic.Int32
	srv := serve(t, http.StatusOK, `{"traces":[]}`, func(*http.Request) { calls.Add(1) })

	_, err := NewClient(srv.URL).TraceScores(t.Context(), "o", "p", "a", testStart, testEnd, []string{"a"}, "")
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("err = %v, want ErrForbidden", err)
	}
	if calls.Load() != 0 {
		t.Errorf("service called %d times, want 0", calls.Load())
	}
}

// A service that doesn't answer fails the lookup at the timeout.
func TestTraceScores_Timeout(t *testing.T) {
	if got := NewClient("http://unused").httpClient.Timeout; got != scoreLookupTimeout {
		t.Errorf("NewClient timeout = %s, want scoreLookupTimeout (%s)", got, scoreLookupTimeout)
	}
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	const timeout = 100 * time.Millisecond
	began := time.Now()
	_, err := newClient(srv.URL, timeout).TraceScores(withToken(t), "o", "p", "a", testStart, testEnd, []string{"a"}, "")
	elapsed := time.Since(began)

	if err == nil || errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if elapsed < timeout || elapsed > timeout+time.Second {
		t.Errorf("lookup took %s, want about %s", elapsed, timeout)
	}
}

// A cancelled context ends the lookup with context.Canceled.
func TestTraceScores_Cancelled(t *testing.T) {
	srv := serve(t, http.StatusOK, `{"traces":[]}`, nil)
	ctx, cancel := context.WithCancel(withToken(t))
	cancel()

	_, err := NewClient(srv.URL).TraceScores(ctx, "o", "p", "a", testStart, testEnd, []string{"a"}, "")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// Sequential lookups, with any status, reuse one connection.
func TestTraceScores_ReusesConnection(t *testing.T) {
	var conns, status atomic.Int32
	status.Store(http.StatusOK)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(status.Load()))
		_, _ = w.Write([]byte(`{"traces":[{"traceId":"a","score":0.5}],"totalCount":1}`))
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL)
	for i := range 10 {
		if i == 5 {
			status.Store(http.StatusInternalServerError)
		}
		_, _ = c.TraceScores(withToken(t), "o", "p", "a", testStart, testEnd, []string{"a"}, "")
	}
	if got := conns.Load(); got != 1 {
		t.Errorf("opened %d connections, want 1", got)
	}
}
