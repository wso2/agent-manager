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

package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

const protobufType = "application/x-protobuf"

var ingestWindow = Window{StartNs: 0, EndNs: time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano(), Limit: 10, Desc: true}

func newIngestFixture(t *testing.T, maxBytes int64) (*SpanStore, http.Handler) {
	t.Helper()
	store, err := OpenSpanStore(filepath.Join(t.TempDir(), "traces.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, NewIngestHandler(store, maxBytes)
}

func buildRequest(t *testing.T, configure func(span ptrace.Span, res pcommon.Resource)) []byte {
	t.Helper()
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	span.SetSpanID(pcommon.SpanID{1, 2, 3, 4, 5, 6, 7, 8})
	span.SetName("root")
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Unix(1000, 0)))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Unix(1001, 0)))
	if configure != nil {
		configure(span, rs.Resource())
	}
	body, err := ptraceotlp.NewExportRequestFromTraces(traces).MarshalProto()
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func gzipped(t *testing.T, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func post(h http.Handler, body []byte, contentType, encoding string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func storedSpan(t *testing.T, store *SpanStore, sc Scope) (total int) {
	t.Helper()
	_, total, err := store.QueryTraces(context.Background(), sc, ingestWindow)
	if err != nil {
		t.Fatal(err)
	}
	return total
}

var scopeHeaders = map[string]string{
	"x-user-component":   "comp-1",
	"x-user-environment": "env-1",
	"x-user-project":     "proj-1",
	"x-user-namespace":   "ns-1",
}

func TestIngestStoresSpansWithHeaderScope(t *testing.T) {
	store, h := newIngestFixture(t, DefaultMaxIngestBytes)
	rec := post(h, buildRequest(t, nil), protobufType, "", scopeHeaders)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	want, _ := ptraceotlp.NewExportResponse().MarshalProto()
	if !bytes.Equal(rec.Body.Bytes(), want) {
		t.Fatalf("unexpected response body %q", rec.Body.Bytes())
	}
	full := Scope{Namespace: "ns-1", ProjectUID: "proj-1", ComponentUID: "comp-1", EnvironmentUID: "env-1"}
	if got := storedSpan(t, store, full); got != 1 {
		t.Fatalf("scoped total = %d, want 1", got)
	}
	if got := storedSpan(t, store, Scope{ComponentUID: "other"}); got != 0 {
		t.Fatalf("other scope total = %d, want 0", got)
	}
	d, err := store.SpanDetails(context.Background(), "0102030405060708090a0b0c0d0e0f10", "0102030405060708")
	if err != nil {
		t.Fatal(err)
	}
	if d.ResourceAttributes["openchoreo.dev/component-uid"] != "comp-1" {
		t.Fatalf("resource attributes = %v", d.ResourceAttributes)
	}
}

func TestIngestGzip(t *testing.T) {
	store, h := newIngestFixture(t, DefaultMaxIngestBytes)
	rec := post(h, gzipped(t, buildRequest(t, nil)), protobufType, "gzip", scopeHeaders)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if got := storedSpan(t, store, Scope{ComponentUID: "comp-1"}); got != 1 {
		t.Fatalf("total = %d, want 1", got)
	}
}

func TestIngestMapsKindStatusAndAttrTypes(t *testing.T) {
	store, h := newIngestFixture(t, DefaultMaxIngestBytes)
	body := buildRequest(t, func(span ptrace.Span, res pcommon.Resource) {
		span.SetKind(ptrace.SpanKindClient)
		span.Status().SetCode(ptrace.StatusCodeError)
		span.Status().SetMessage("boom")
		span.Attributes().PutInt("n", 12)
		res.Attributes().PutInt("r", 7)
		res.Attributes().PutEmptyMap("m").PutStr("h", "<a&b>")
	})
	if rec := post(h, body, protobufType, "", nil); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	d, err := store.SpanDetails(context.Background(), "0102030405060708090a0b0c0d0e0f10", "0102030405060708")
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != "Client" {
		t.Errorf("kind = %q", d.Kind)
	}
	if d.Status.Code != "error" || d.Status.Message != "boom" {
		t.Errorf("status = %+v", d.Status)
	}
	if v, ok := d.Attributes["n"].(float64); !ok || v != 12 {
		t.Errorf("attribute n = %#v", d.Attributes["n"])
	}
	if d.ResourceAttributes["r"] != "7" {
		t.Errorf("resource r = %#v", d.ResourceAttributes["r"])
	}
	if m, _ := d.ResourceAttributes["m"].(string); m != `{"h":"<a&b>"}` {
		t.Errorf("resource m = %#v", d.ResourceAttributes["m"])
	}
}

func TestIngestWithoutScopeHeaders(t *testing.T) {
	store, h := newIngestFixture(t, DefaultMaxIngestBytes)
	if rec := post(h, buildRequest(t, nil), protobufType, "", nil); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	d, err := store.SpanDetails(context.Background(), "0102030405060708090a0b0c0d0e0f10", "0102030405060708")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"openchoreo.dev/component-uid", "openchoreo.dev/environment-uid", "openchoreo.dev/project-uid", "openchoreo.dev/namespace"} {
		if _, ok := d.ResourceAttributes[key]; ok {
			t.Errorf("unexpected resource attribute %s", key)
		}
	}
	if got := storedSpan(t, store, Scope{ComponentUID: "comp-1"}); got != 0 {
		t.Errorf("scoped total = %d, want 0", got)
	}
	if got := storedSpan(t, store, Scope{}); got != 1 {
		t.Errorf("unscoped total = %d, want 1", got)
	}
}

func TestIngestRejectsGarbage(t *testing.T) {
	_, h := newIngestFixture(t, DefaultMaxIngestBytes)
	if rec := post(h, []byte("not a protobuf \xff\xff"), protobufType, "", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("garbage status = %d, want 400", rec.Code)
	}
	if rec := post(h, []byte("not gzip"), protobufType, "gzip", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad gzip status = %d, want 400", rec.Code)
	}
	if rec := post(h, buildRequest(t, nil), protobufType, "", nil); rec.Code != http.StatusOK {
		t.Fatalf("valid status = %d, want 200", rec.Code)
	}
}

func TestIngestRejectsOversize(t *testing.T) {
	_, h := newIngestFixture(t, 1024)
	if rec := post(h, bytes.Repeat([]byte{0}, 4096), protobufType, "", nil); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("raw oversize status = %d, want 413", rec.Code)
	}
}

func TestIngestRejectsOversizeDecompressed(t *testing.T) {
	_, h := newIngestFixture(t, 1024)
	compressed := gzipped(t, bytes.Repeat([]byte{0}, 4096))
	if len(compressed) >= 1024 {
		t.Fatalf("test setup: compressed body is %d bytes", len(compressed))
	}
	if rec := post(h, compressed, protobufType, "gzip", nil); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}

func TestIngestRejectsJSONAndUnknownEncoding(t *testing.T) {
	_, h := newIngestFixture(t, DefaultMaxIngestBytes)
	body := buildRequest(t, nil)
	if rec := post(h, body, "application/json", "", nil); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("json status = %d, want 415", rec.Code)
	}
	if rec := post(h, body, protobufType, "br", nil); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("encoding status = %d, want 415", rec.Code)
	}
	if rec := post(h, body, "", "", nil); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("missing content-type status = %d, want 415", rec.Code)
	}
}

func TestIngestRoutes(t *testing.T) {
	_, h := newIngestFixture(t, DefaultMaxIngestBytes)
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/v1/traces", nil))
	if get.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want 405", get.Code)
	}
	other := httptest.NewRecorder()
	h.ServeHTTP(other, httptest.NewRequest(http.MethodPost, "/v1/metrics", strings.NewReader("")))
	if other.Code != http.StatusNotFound {
		t.Errorf("path status = %d, want 404", other.Code)
	}
}

func TestSpanStatus(t *testing.T) {
	for code, want := range map[ptrace.StatusCode]string{
		ptrace.StatusCodeOk: "ok", ptrace.StatusCodeError: "error", ptrace.StatusCodeUnset: "unset",
	} {
		if got := spanStatus(code); got != want {
			t.Errorf("spanStatus(%v) = %q, want %q", code, got, want)
		}
	}
}
