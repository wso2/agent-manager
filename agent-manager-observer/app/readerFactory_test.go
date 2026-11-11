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

package app

import (
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearchreader"
)

func TestNewTraceReader_OpenChoreo(t *testing.T) {
	client := observer.NewClient("http://unused", requestTokenProvider{}, "default")

	for _, backend := range []string{"", config.TraceBackendOpenChoreo} {
		cfg := &config.Config{
			Observer: config.ObserverConfig{DefaultNamespace: "default"},
			Traces:   config.TraceReaderConfig{Backend: backend},
		}

		r, err := newTraceReader(cfg, client)
		if err != nil {
			t.Fatalf("backend %q: unexpected error: %v", backend, err)
		}
		if r != client {
			t.Errorf("backend %q: expected openChoreo client to be returned directly", backend)
		}
	}
}

func TestNewTraceReader_UnknownBackend(t *testing.T) {
	client := observer.NewClient("http://unused", requestTokenProvider{}, "default")
	cfg := &config.Config{
		Traces: config.TraceReaderConfig{Backend: "cassandra"},
	}

	_, err := newTraceReader(cfg, client)
	if err == nil {
		t.Fatal("expected error for unknown backend, got nil")
	}
	if !strings.Contains(err.Error(), "unknown TRACE_BACKEND") {
		t.Errorf("expected error message to contain 'unknown TRACE_BACKEND', got %q", err.Error())
	}
}

func TestNewTraceReader_OpenSearch_Success(t *testing.T) {
	client := observer.NewClient("http://unused", requestTokenProvider{}, "default")

	cfg := &config.Config{
		Observer: config.ObserverConfig{DefaultNamespace: "default"},
		Traces: config.TraceReaderConfig{
			Backend:      config.TraceBackendOpenSearch,
			OSAddr:       "http://localhost:9200",
			OSUser:       "admin",
			OSPassword:   "admin",
			OSIndex:      "otel-traces-*",
			OSInsecureTLS: true,
			ScopeUIDs:    `{"projects":{"default/default":"proj-1"}}`,
		},
	}

	r, err := newTraceReader(cfg, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := r.(*opensearchreader.TraceReaderImpl); !ok {
		t.Errorf("expected *opensearchreader.TraceReaderImpl, got %T", r)
	}
}

func TestNewTraceReader_OpenSearch_InvalidScopeJSON(t *testing.T) {
	client := observer.NewClient("http://unused", requestTokenProvider{}, "default")

	cfg := &config.Config{
		Observer: config.ObserverConfig{DefaultNamespace: "default"},
		Traces: config.TraceReaderConfig{
			Backend:   config.TraceBackendOpenSearch,
			OSAddr:    "http://localhost:9200",
			OSUser:    "admin",
			OSPassword: "admin",
			OSIndex:   "otel-traces-*",
			ScopeUIDs: "invalid-json",
		},
	}

	_, err := newTraceReader(cfg, client)
	if err == nil {
		t.Fatal("expected error for invalid ScopeUIDs JSON, got nil")
	}
}

func TestNewTraceReader_OpenSearch_InvalidOptions(t *testing.T) {
	client := observer.NewClient("http://unused", requestTokenProvider{}, "default")

	cfg := &config.Config{
		Observer: config.ObserverConfig{DefaultNamespace: "default"},
		Traces: config.TraceReaderConfig{
			Backend:      config.TraceBackendOpenSearch,
			OSAddr:       "invalid-address-without-scheme",
			OSUser:       "admin",
			OSPassword:   "admin",
			OSIndex:      "otel-traces-*",
			ScopeUIDs:    `{}`,
		},
	}

	_, err := newTraceReader(cfg, client)
	if err == nil {
		t.Fatal("expected error for invalid OpenSearch options, got nil")
	}
	if !strings.Contains(err.Error(), "create opensearch trace reader") {
		t.Errorf("expected error message to contain 'create opensearch trace reader', got %q", err.Error())
	}
}

func TestNewTraceReader_ClickHouse_ConnectionFailure(t *testing.T) {
	client := observer.NewClient("http://unused", requestTokenProvider{}, "default")

	cfg := &config.Config{
		Observer: config.ObserverConfig{DefaultNamespace: "default"},
		Traces: config.TraceReaderConfig{
			Backend:      config.TraceBackendClickHouse,
			Addr:         "127.0.0.1:1", // guaranteed unreachable port
			Database:     "otel",
			User:         "default",
			Password:     "",
			DialTimeout:  50 * time.Millisecond,
			Table:        "otel_traces",
			Layout:       config.TraceLayoutExporter,
			ScopeUIDs:    `{}`,
		},
	}

	_, err := newTraceReader(cfg, client)
	if err == nil {
		t.Fatal("expected error for unreachable ClickHouse instance, got nil")
	}
	if !strings.Contains(err.Error(), "failed to open clickhouse") {
		t.Errorf("expected error to contain 'failed to open clickhouse', got %q", err.Error())
	}
}
