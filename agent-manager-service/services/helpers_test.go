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

package services

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
)

// discardLogger returns a slog.Logger that drops all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func strPtr(s string) *string { return &s }

// capturingSink keeps every event written so a test can assert on the record
// itself rather than on the call that produced it.
type capturingSink struct {
	mu     sync.Mutex
	events []audit.Event
}

func (c *capturingSink) Name() string { return "capturing" }

func (c *capturingSink) Write(_ context.Context, events []audit.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, events...)
	return nil
}

func (c *capturingSink) captured() []audit.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.events)
}

// capturingAuditCtx returns a context whose recorder writes into the returned
// sink, plus the flush that has to run before the events can be read.
func capturingAuditCtx(t *testing.T) (context.Context, *capturingSink, func()) {
	t.Helper()
	sink := &capturingSink{}
	rec := audit.NewRecorder(sink, discardLogger(), audit.Config{BatchSize: 1})
	ctx := audit.WithRecorder(context.Background(), rec)
	return ctx, sink, func() {
		require.NoError(t, rec.Close(context.Background()))
	}
}

func findEvent(events []audit.Event, action audit.Action) (audit.Event, bool) {
	for _, e := range events {
		if e.Action == action {
			return e, true
		}
	}
	return audit.Event{}, false
}
