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
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// serveLogged runs a handler through WithRequestLog with a logger writing into
// a buffer, and returns the records it emitted.
func serveLogged(t *testing.T, handler http.HandlerFunc) (records []map[string]any) {
	t.Helper()

	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	pattern := "POST /orgs/{orgName}/projects"
	meta := audit.NewRouteMeta(pattern, audit.ExtractPathParams(pattern), []rbac.Permission{rbac.ProjectCreate})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/orgs/acme/projects", nil)
	r = r.WithContext(logger.WithLogger(r.Context(), log))

	// A panicking handler must still produce a record. The result is assembled
	// in the deferred function because the panic unwinds past the return.
	defer func() {
		_ = recover()
		records = parseRecords(t, buf.String())
	}()
	WithRequestLog(meta)(handler)(w, r)

	return nil
}

func parseRecords(t *testing.T, out string) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %v (%q)", err, line)
		}
		records = append(records, rec)
	}
	return records
}

// completionRecord returns the single log_type=request record, failing when
// there is not exactly one: a request that logs its outcome twice is as hard to
// read as one that never logs it.
func completionRecord(t *testing.T, records []map[string]any) map[string]any {
	t.Helper()
	var found []map[string]any
	for _, r := range records {
		if r["log_type"] == "request" {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		t.Fatalf("got %d completion records, want 1", len(found))
	}
	return found[0]
}

func TestRequestLogRecordsSuccess(t *testing.T) {
	records := serveLogged(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	rec := completionRecord(t, records)
	if rec["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", rec["level"])
	}
	if rec["status"] != float64(http.StatusCreated) {
		t.Errorf("status = %v, want 201", rec["status"])
	}
	if _, ok := rec["duration_ms"]; !ok {
		t.Error("record has no duration_ms")
	}
}

// A 4xx is the caller being told no, not a failure of this service. Logging it
// at Error is what made ERROR useless as an alert signal.
func TestRequestLogClientErrorIsWarn(t *testing.T) {
	records := serveLogged(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	rec := completionRecord(t, records)
	if rec["level"] != "WARN" {
		t.Errorf("level = %v, want WARN for a 400", rec["level"])
	}
}

func TestRequestLogServerErrorIsError(t *testing.T) {
	records := serveLogged(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	rec := completionRecord(t, records)
	if rec["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR for a 500", rec["level"])
	}
}

// A handler that panics is exactly the case where the record matters most, and
// exactly the case a naive "log after next()" implementation loses.
func TestRequestLogRecordsPanicAs500(t *testing.T) {
	records := serveLogged(t, func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})

	rec := completionRecord(t, records)
	if rec["status"] != float64(http.StatusInternalServerError) {
		t.Errorf("status = %v, want 500 after a panic", rec["status"])
	}
	if rec["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", rec["level"])
	}
}

// The handler's own lines must be attributable to the operation, which is the
// whole point of enriching the context logger rather than logging out here.
func TestRequestLogEnrichesHandlerLogger(t *testing.T) {
	records := serveLogged(t, func(_ http.ResponseWriter, r *http.Request) {
		logger.GetLogger(r.Context()).Info("handler ran")
	})

	for _, rec := range records {
		if rec["msg"] == "handler ran" {
			if rec["action"] != "project:create" {
				t.Errorf("handler record action = %v, want project:create", rec["action"])
			}
			return
		}
	}
	t.Fatal("handler record not found")
}

// The endpoint pattern was dropped: `path` carries the concrete URL and nothing
// aggregates by pattern. Asserted so it does not creep back as a default.
func TestRequestLogOmitsRoute(t *testing.T) {
	records := serveLogged(t, func(http.ResponseWriter, *http.Request) {})

	for _, rec := range records {
		if _, ok := rec["route"]; ok {
			t.Errorf("record carries route: %v", rec)
		}
	}
}

// setRequestIdentity is how org resolution — which runs inside this wrapper —
// gets the org onto a record emitted outside it.
func TestRequestLogCarriesIdentitySetByInnerMiddleware(t *testing.T) {
	records := serveLogged(t, func(_ http.ResponseWriter, r *http.Request) {
		setRequestIdentity(r.Context(), "ou-123")
	})

	rec := completionRecord(t, records)
	if rec["ou_id"] != "ou-123" {
		t.Errorf("ou_id = %v, want ou-123", rec["ou_id"])
	}
}

// The caller's token subject belongs in the audit trail, not the application
// log. Asserted so it cannot drift back in as a convenience field.
func TestRequestLogOmitsTokenSubject(t *testing.T) {
	records := serveLogged(t, func(_ http.ResponseWriter, r *http.Request) {
		setRequestIdentity(r.Context(), "ou-123")
	})

	for _, rec := range records {
		if _, ok := rec["user_id"]; ok {
			t.Errorf("record carries user_id: %v", rec)
		}
	}
}

// TestChainStitchesRecordsByCorrelationID is the end-to-end claim this work
// exists to make true: every record produced while serving one request can be
// selected with a single correlation_id term, whichever layer emitted it.
func TestChainStitchesRecordsByCorrelationID(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)

	mux := http.NewServeMux()
	rr := NewRouteRegistrar(mux, nil, &captureRecorder{})
	// A read route with no {orgName} segment: no authz and no org middleware
	// run, so what is under test is the logging chain alone.
	rr.HandleFuncWithValidation("GET /widgets/{widgetID}", func(_ http.ResponseWriter, r *http.Request) {
		// Stands in for the controller, service and repository layers, all of
		// which now reach their logger the same way.
		logger.GetLogger(r.Context()).Info("service did work")
	})

	// The same wrapper order as api/app.go, outermost last.
	var handler http.Handler = mux
	handler = logger.RequestLogger()(handler)
	handler = AddCorrelationID()(handler)
	handler = RecovererOnPanic()(handler)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/widgets/w-1", nil)
	r.Header.Set(CorrelationIDHeader, "trace-me-123")
	handler.ServeHTTP(w, r)

	if got := w.Header().Get(CorrelationIDHeader); got != "trace-me-123" {
		t.Errorf("response correlation header = %q, want the one the caller sent", got)
	}

	records := parseRecords(t, buf.String())
	if len(records) < 2 {
		t.Fatalf("got %d records, want the handler's line and the completion record", len(records))
	}
	for _, rec := range records {
		if rec["correlation_id"] != "trace-me-123" {
			t.Errorf("record %q has correlation_id %v, want trace-me-123", rec["msg"], rec["correlation_id"])
		}
	}
	if rec := completionRecord(t, records); rec["status"] != float64(http.StatusOK) {
		t.Errorf("status = %v, want 200", rec["status"])
	}
}

// TestCompletionRecordHasNoDuplicateKeys guards a bug this middleware shipped
// with: slog appends attributes rather than replacing them, so a key set on the
// base request logger *and* on the completion record appeared twice in one JSON
// object. Decoding into a map hides it — the raw bytes are the only witness.
func TestCompletionRecordHasNoDuplicateKeys(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	mux := http.NewServeMux()
	rr := NewRouteRegistrar(mux, nil, &captureRecorder{})
	rr.HandleFuncWithValidation("GET /widgets/{widgetID}", func(http.ResponseWriter, *http.Request) {})

	var handler http.Handler = mux
	handler = logger.RequestLogger()(handler)
	handler = AddCorrelationID()(handler)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/widgets/w-1", nil))

	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		for _, key := range []string{`"log_type"`, `"correlation_id"`, `"status"`, `"method"`, `"path"`} {
			if n := strings.Count(line, key); n > 1 {
				t.Errorf("key %s appears %d times in one record: %s", key, n, line)
			}
		}
	}
}

// An unaudited route has no action, and an empty one is noise on every record
// the request produces.
func TestUnauditedRouteOmitsAction(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	mux := http.NewServeMux()
	rr := NewRouteRegistrar(mux, nil, &captureRecorder{})
	rr.HandleFuncWithValidation("GET /widgets/{widgetID}", func(http.ResponseWriter, *http.Request) {})
	mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/widgets/w-1", nil))

	if strings.Contains(buf.String(), `"action":""`) {
		t.Errorf("record carries an empty action: %s", buf.String())
	}
}
