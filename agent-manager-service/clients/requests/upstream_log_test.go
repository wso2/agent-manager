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

package requests

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
)

// The context logger already supplies these on every record while a request is
// being served. An outbound-call record that sets any of them again puts two
// different values under one key — the inbound method and the upstream method,
// for instance — and whichever the log pipeline keeps is a coin toss.
var inheritedKeys = []string{"method", "path", "correlation_id"}

func TestUpstreamRecordDoesNotShadowInheritedKeys(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})).With(
		slog.String("method", "GET"),
		slog.String("path", "/agents/checkout-bot"),
		slog.String("correlation_id", "trace-me-123"),
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	// An inbound GET driving an outbound POST: the case where a shared `method`
	// key is not just duplicated but wrong.
	ctx := logger.WithLogger(context.Background(), base)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstream.URL, strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := NewRetryableHTTPClient(upstream.Client()).Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if buf.Len() == 0 {
		t.Fatal("outbound call produced no log records")
	}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		for _, key := range inheritedKeys {
			if n := strings.Count(line, `"`+key+`"`); n > 1 {
				t.Errorf("key %q appears %d times in one record: %s", key, n, line)
			}
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("record is not JSON: %v", err)
		}
		if rec["method"] != "GET" {
			t.Errorf("inbound method was overwritten: method = %v, want GET", rec["method"])
		}
		if rec["upstream_method"] != http.MethodPost {
			t.Errorf("upstream_method = %v, want POST", rec["upstream_method"])
		}
	}
}
