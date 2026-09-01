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
	"context"
	"sync"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
)

type postureSink struct {
	mu     sync.Mutex
	events []audit.Event
}

func (s *postureSink) Name() string { return "posture" }

func (s *postureSink) Write(_ context.Context, events []audit.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, events...)
	return nil
}

func (s *postureSink) find(action audit.Action) (audit.Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.events {
		if e.Action == action {
			return e, true
		}
	}
	return audit.Event{}, false
}

func capturePosture(t *testing.T) *postureSink {
	t.Helper()
	sink := &postureSink{}
	rec := audit.NewRecorder(sink, nil, audit.Config{BatchSize: 1})
	recordStartupPosture(rec)
	if err := rec.Close(context.Background()); err != nil {
		t.Fatalf("close recorder: %v", err)
	}
	return sink
}

// TestStartupRecordIsWritten keeps the trail's opening bookend: without it a
// reader cannot tell "nothing happened" apart from "the service was not
// running", and any gap in the record is unbounded.
func TestStartupRecordIsWritten(t *testing.T) {
	sink := capturePosture(t)

	if _, ok := sink.find(audit.ActionSystemStartup); !ok {
		t.Fatal("no system:startup record")
	}
}

// TestStartupPostureWithoutARecorderIsANoop keeps startup from depending on the
// trail being wired: the service must still boot when it is not.
func TestStartupPostureWithoutARecorderIsANoop(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("recordStartupPosture panicked with no recorder: %v", r)
		}
	}()
	recordStartupPosture(nil)
}
