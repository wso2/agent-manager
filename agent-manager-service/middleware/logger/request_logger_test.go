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

package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

// captureLogger returns a logger writing JSON into buf.
func captureLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func record(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("log line is not JSON: %v (%q)", err, buf.String())
	}
	return rec
}

// Enrich is how middleware attach a field once, at the point it becomes known,
// instead of every downstream call site repeating it.
func TestEnrichAddsFieldsForDownstreamCallers(t *testing.T) {
	var buf bytes.Buffer
	ctx := WithLogger(context.Background(), captureLogger(&buf))

	ctx = Enrich(ctx, slog.String("correlation_id", "trace-me-123"))
	ctx = Enrich(ctx, slog.String("ou_id", "ou-1"))

	GetLogger(ctx).Info("downstream")

	rec := record(t, &buf)
	if rec["correlation_id"] != "trace-me-123" {
		t.Errorf("correlation_id = %v, want trace-me-123", rec["correlation_id"])
	}
	if rec["ou_id"] != "ou-1" {
		t.Errorf("ou_id = %v, want ou-1", rec["ou_id"])
	}
}

// Enriching must not mutate the logger the outer scope still holds.
func TestEnrichDoesNotAffectParentContext(t *testing.T) {
	var buf bytes.Buffer
	parent := WithLogger(context.Background(), captureLogger(&buf))

	_ = Enrich(parent, slog.String("ou_id", "ou-1"))
	GetLogger(parent).Info("outer")

	if _, ok := record(t, &buf)["ou_id"]; ok {
		t.Error("parent context logger picked up an enrichment made on a child")
	}
}

func TestEnrichWithNoAttrsReturnsSameContext(t *testing.T) {
	ctx := context.Background()
	if Enrich(ctx) != ctx {
		t.Error("Enrich with no attributes should return the context unchanged")
	}
}

// Everything outside a request — startup, workers, tests — must still get a
// usable logger rather than a nil dereference.
func TestGetLoggerFallsBackToDefault(t *testing.T) {
	if GetLogger(context.Background()) == nil {
		t.Fatal("GetLogger returned nil for a context with no logger")
	}
}
