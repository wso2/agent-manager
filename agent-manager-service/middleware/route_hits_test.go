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
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
)

// routeHitLines registers a core and a module route with the route-hit log
// set to on, calls each once plus an unregistered path, and returns the log
// lines written.
func routeHitLines(t *testing.T, on bool) []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	SetRouteHitLog(on)
	t.Cleanup(func() {
		slog.SetDefault(prev)
		SetRouteHitLog(false)
	})

	mux := http.NewServeMux()
	rr := NewInternalRouteRegistrar(mux, nil)
	rr.HandleFuncWithValidation("GET /ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mr := NewInternalRouteRegistrar(mux, nil)
	mr.SetModule("widgets")
	mr.HandleModuleInternalRoute("POST /m/widgets/items", ModuleRoutePolicy{
		Audit: audit.RoutePolicy{Action: "widgets:item.create"},
	}, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})

	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/ping?token=secret", nil),
		httptest.NewRequest(http.MethodPost, "/m/widgets/items", strings.NewReader(`{"secret":"x"}`)),
		httptest.NewRequest(http.MethodGet, "/nowhere", nil),
	} {
		mux.ServeHTTP(httptest.NewRecorder(), req)
	}

	if strings.Contains(buf.String(), "secret") {
		t.Errorf("route-hit log carries the query or body: %s", buf.String())
	}
	var lines []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("log line %q: %v", l, err)
		}
		lines = append(lines, m)
	}
	return lines
}

func TestRouteHitLogOn(t *testing.T) {
	lines := routeHitLines(t, true)

	type entry struct {
		msg, pattern, module string
		status               float64
	}
	want := []entry{
		{"route registered", "GET /ping", "core", 0},
		{"route registered", "POST /m/widgets/items", "widgets", 0},
		{"route hit", "GET /ping", "core", http.StatusNoContent},
		{"route hit", "POST /m/widgets/items", "widgets", http.StatusConflict},
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d log lines, want %d: %v", len(lines), len(want), lines)
	}
	for i, w := range want {
		l := lines[i]
		status, _ := l["status"].(float64)
		got := entry{l["msg"].(string), l["pattern"].(string), l["module"].(string), status}
		if got != w {
			t.Errorf("line %d = %+v, want %+v", i, got, w)
		}
	}
}

func TestRouteHitLogOff(t *testing.T) {
	if lines := routeHitLines(t, false); len(lines) != 0 {
		t.Errorf("route-hit log off wrote %d lines: %v", len(lines), lines)
	}
}

// A route wrapped by the route-hit log still upgrades to a WebSocket: the
// upgrader type-asserts http.Hijacker on the writer it is given.
func TestRouteHitLogKeepsWebSocketUpgrade(t *testing.T) {
	// The hit is logged after the hijacked handler returns, on its own
	// goroutine, so the log is read under a lock and waited for.
	var buf lockedBuffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	SetRouteHitLog(true)
	t.Cleanup(func() {
		slog.SetDefault(prev)
		SetRouteHitLog(false)
	})

	mux := http.NewServeMux()
	rr := NewInternalRouteRegistrar(mux, nil)
	var upgrader websocket.Upgrader
	rr.HandleFuncWithValidation("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.WriteMessage(websocket.TextMessage, []byte("hello"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatalf("upgrade through the route-hit log failed: %v", err)
	}
	defer func() { _ = conn.Close() }()
	defer func() { _ = resp.Body.Close() }()
	_, msg, err := conn.ReadMessage()
	if err != nil || string(msg) != "hello" {
		t.Fatalf("read %q, %v; want hello", msg, err)
	}
	for deadline := time.Now().Add(2 * time.Second); !strings.Contains(buf.String(), `"status":101`); {
		if time.Now().After(deadline) {
			t.Fatalf("route hit not logged as 101: %s", buf.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
