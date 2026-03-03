// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestHandler creates a handler with a nil controller.
// This is valid for testing parameter validation since validation errors
// are returned before the controller is invoked.
func newTestHandler() *Handler {
	return NewHandler(nil)
}

func TestGetTraceOverviews_MissingComponentUid(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/traces?environmentUid=env-1", nil)
	w := httptest.NewRecorder()

	h.GetTraceOverviews(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Message != "componentUid is required" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestGetTraceOverviews_MissingEnvironmentUid(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/traces?componentUid=comp-1", nil)
	w := httptest.NewRecorder()

	h.GetTraceOverviews(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Message != "environmentUid is required" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestGetTraceOverviews_MissingStartTime(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/traces?componentUid=comp-1&environmentUid=env-1", nil)
	w := httptest.NewRecorder()

	h.GetTraceOverviews(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Message != "startTime is required" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestGetTraceOverviews_MissingEndTime(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/traces?componentUid=comp-1&environmentUid=env-1&startTime=2025-01-01T00:00:00Z", nil)
	w := httptest.NewRecorder()

	h.GetTraceOverviews(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Message != "endTime is required" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestGetTraceOverviews_InvalidLimit(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/traces?componentUid=comp-1&environmentUid=env-1&startTime=2025-01-01T00:00:00Z&endTime=2025-01-02T00:00:00Z&limit=-1", nil)
	w := httptest.NewRecorder()

	h.GetTraceOverviews(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Message != "limit must be a positive integer" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestGetTraceOverviews_InvalidLimitNonNumeric(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/traces?componentUid=comp-1&environmentUid=env-1&startTime=2025-01-01T00:00:00Z&endTime=2025-01-02T00:00:00Z&limit=abc", nil)
	w := httptest.NewRecorder()

	h.GetTraceOverviews(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestGetTraceOverviews_InvalidOffset(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/traces?componentUid=comp-1&environmentUid=env-1&startTime=2025-01-01T00:00:00Z&endTime=2025-01-02T00:00:00Z&offset=-5", nil)
	w := httptest.NewRecorder()

	h.GetTraceOverviews(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Message != "offset must be a non-negative integer" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestGetTraceOverviews_InvalidSortOrder(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/traces?componentUid=comp-1&environmentUid=env-1&startTime=2025-01-01T00:00:00Z&endTime=2025-01-02T00:00:00Z&sortOrder=invalid", nil)
	w := httptest.NewRecorder()

	h.GetTraceOverviews(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Message != "sortOrder must be 'asc' or 'desc'" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestGetTraceById_MissingTraceId(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/trace?componentUid=comp-1&environmentUid=env-1", nil)
	w := httptest.NewRecorder()

	h.GetTraceById(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Message != "traceId is required" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestGetTraceById_MissingComponentUid(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/trace?traceId=trace-1&environmentUid=env-1", nil)
	w := httptest.NewRecorder()

	h.GetTraceById(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Message != "componentUid is required" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestGetTraceById_MissingEnvironmentUid(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/trace?traceId=trace-1&componentUid=comp-1", nil)
	w := httptest.NewRecorder()

	h.GetTraceById(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Message != "environmentUid is required" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestGetTraceById_InvalidSortOrder(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/trace?traceId=trace-1&componentUid=comp-1&environmentUid=env-1&sortOrder=wrong", nil)
	w := httptest.NewRecorder()

	h.GetTraceById(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestGetTraceById_InvalidLimit(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/trace?traceId=trace-1&componentUid=comp-1&environmentUid=env-1&limit=0", nil)
	w := httptest.NewRecorder()

	h.GetTraceById(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestExportTraces_MissingComponentUid(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/traces/export?environmentUid=env-1", nil)
	w := httptest.NewRecorder()

	h.ExportTraces(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Message != "componentUid is required" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}

func TestExportTraces_MissingEnvironmentUid(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/traces/export?componentUid=comp-1", nil)
	w := httptest.NewRecorder()

	h.ExportTraces(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestExportTraces_InvalidLimit(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/traces/export?componentUid=comp-1&environmentUid=env-1&limit=abc", nil)
	w := httptest.NewRecorder()

	h.ExportTraces(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestExportTraces_InvalidOffset(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/traces/export?componentUid=comp-1&environmentUid=env-1&offset=-1", nil)
	w := httptest.NewRecorder()

	h.ExportTraces(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestExportTraces_InvalidSortOrder(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest("GET", "/api/v1/traces/export?componentUid=comp-1&environmentUid=env-1&sortOrder=random", nil)
	w := httptest.NewRecorder()

	h.ExportTraces(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestWriteJSON(t *testing.T) {
	h := newTestHandler()
	w := httptest.NewRecorder()
	data := map[string]string{"status": "ok"}

	h.writeJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("unexpected response: %v", resp)
	}
}

func TestWriteError(t *testing.T) {
	h := newTestHandler()
	w := httptest.NewRecorder()

	h.writeError(w, http.StatusBadRequest, "something went wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
	var resp ErrorResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != "error" {
		t.Errorf("expected error field 'error', got %q", resp.Error)
	}
	if resp.Message != "something went wrong" {
		t.Errorf("unexpected message: %q", resp.Message)
	}
}
