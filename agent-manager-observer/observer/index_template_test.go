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

package observer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
)

func TestEnsureIndexTemplateRetriesUntilApplied(t *testing.T) {
	var calls atomic.Int32
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/_index_template/amp-otel-traces" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if u, p, ok := r.BasicAuth(); !ok || u != "admin" || p != "pw" {
			t.Errorf("missing basic auth")
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("body is not JSON: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.OpenSearchConfig{
		Address: srv.URL, Username: "admin", Password: "pw",
		TraceIndex: "spans-*", IndexTemplate: "amp-otel-traces",
	}
	if err := EnsureIndexTemplate(context.Background(), cfg, time.Millisecond); err != nil {
		t.Fatalf("EnsureIndexTemplate: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d, want 3", calls.Load())
	}
	patterns, _ := got["index_patterns"].([]any)
	if len(patterns) != 1 || patterns[0] != "spans-*" {
		t.Fatalf("index_patterns = %v, want [spans-*]", got["index_patterns"])
	}
	if _, ok := got["template"]; !ok {
		t.Fatalf("template section missing")
	}
}

func TestEnsureIndexTemplateGivesUpWithContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	cfg := config.OpenSearchConfig{Address: srv.URL, TraceIndex: "otel-traces-*", IndexTemplate: "t"}
	if err := EnsureIndexTemplate(ctx, cfg, 10*time.Millisecond); err == nil {
		t.Fatal("want an error once the context ends")
	}
}

func TestEnsureIndexTemplateDisabled(t *testing.T) {
	cfg := config.OpenSearchConfig{Address: "http://127.0.0.1:1", TraceIndex: "otel-traces-*"}
	if err := EnsureIndexTemplate(context.Background(), cfg, time.Millisecond); err != nil {
		t.Fatalf("disabled template returned %v", err)
	}
}
