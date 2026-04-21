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

package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newHandler creates a Handler with a nil controller — safe for validation-only tests
// because the controller is never called when a 400 is returned first.
func newHandler() *Handler {
	return &Handler{controller: nil}
}

// baseParams returns a query string with all required parameters present.
func baseParams() string {
	return "namespace=default&project=myproject&component=myagent&environment=dev" +
		"&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z"
}

func assertBadRequest(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// ── GetTraceOverviews ────────────────────────────────────────────────────────

func TestGetTraceOverviews_MissingNamespace(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?project=p&component=c&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceOverviews_MissingProject(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?namespace=default&component=c&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceOverviews_MissingComponent(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?namespace=default&project=p&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceOverviews_MissingEnvironment(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?namespace=default&project=p&component=c&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceOverviews_MissingStartTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?namespace=default&project=p&component=c&environment=e&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceOverviews_MissingEndTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?namespace=default&project=p&component=c&environment=e&startTime=2026-04-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceOverviews_InvalidSortOrder(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?"+baseParams()+"&sortOrder=invalid", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)
	assertBadRequest(t, rec)
}

// ── GetTraceSpans ────────────────────────────────────────────────────────────

func TestGetTraceSpans_MissingNamespace(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/abc123/spans?startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceSpans(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceSpans_MissingStartTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/abc123/spans?namespace=default&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceSpans(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceSpans_MissingEndTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/abc123/spans?namespace=default&startTime=2026-04-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceSpans(rec, r)
	assertBadRequest(t, rec)
}

// ── ExportTraces ─────────────────────────────────────────────────────────────

func TestExportTraces_MissingNamespace(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/export?project=p&component=c&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.ExportTraces(rec, r)
	assertBadRequest(t, rec)
}

func TestExportTraces_MissingProject(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/export?namespace=default&component=c&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.ExportTraces(rec, r)
	assertBadRequest(t, rec)
}

func TestExportTraces_MissingComponent(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/export?namespace=default&project=p&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.ExportTraces(rec, r)
	assertBadRequest(t, rec)
}

func TestExportTraces_MissingEnvironment(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/export?namespace=default&project=p&component=c&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.ExportTraces(rec, r)
	assertBadRequest(t, rec)
}

func TestExportTraces_MissingStartTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/export?namespace=default&project=p&component=c&environment=e&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.ExportTraces(rec, r)
	assertBadRequest(t, rec)
}

func TestExportTraces_MissingEndTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/export?namespace=default&project=p&component=c&environment=e&startTime=2026-04-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	h.ExportTraces(rec, r)
	assertBadRequest(t, rec)
}
