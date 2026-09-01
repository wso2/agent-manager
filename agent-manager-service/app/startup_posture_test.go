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
	"github.com/wso2/agent-manager/agent-manager-service/config"
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

func capturePosture(t *testing.T, rbacEnabled bool) *postureSink {
	t.Helper()
	sink := &postureSink{}
	rec := audit.NewRecorder(sink, nil, audit.Config{BatchSize: 1})
	recordStartupPosture(&config.Config{RBACEnabled: rbacEnabled}, rec)
	if err := rec.Close(context.Background()); err != nil {
		t.Fatalf("close recorder: %v", err)
	}
	return sink
}

// TestStartupRecordStatesTheRealEnforcementPosture is the regression for a
// record that asserted the opposite of the truth.
//
// rbacEnforced is not a neutral field: false is the claim that no authorization
// check happened, and the design leans on it ("a record with rbacEnforced:false
// shows on its face that no check happened"). Outside a request there is no
// audit source to carry the value, so the startup record serialised the zero
// value — and on an RBAC-enabled deployment the first record in the trail
// contradicted every request record after it.
//
// Found by running the service with RBAC_ENABLED=true: the startup record said
// rbacEnforced:false while the authz:deny records from the same process said
// true.
func TestStartupRecordStatesTheRealEnforcementPosture(t *testing.T) {
	t.Run("enforced", func(t *testing.T) {
		sink := capturePosture(t, true)

		e, ok := sink.find(audit.ActionSystemStartup)
		if !ok {
			t.Fatal("no system:startup record")
		}
		if !e.RBACEnforced {
			t.Error("startup record says rbacEnforced=false while RBAC is enabled; " +
				"it contradicts every request record the same process will write")
		}
		if _, disabled := sink.find(audit.ActionSystemRBACDisabled); disabled {
			t.Error("system:rbac-disabled recorded while RBAC is enabled")
		}
	})

	t.Run("not enforced", func(t *testing.T) {
		sink := capturePosture(t, false)

		e, ok := sink.find(audit.ActionSystemStartup)
		if !ok {
			t.Fatal("no system:startup record")
		}
		if e.RBACEnforced {
			t.Error("startup record says rbacEnforced=true while RBAC is disabled")
		}

		disabled, ok := sink.find(audit.ActionSystemRBACDisabled)
		if !ok {
			t.Fatal("RBAC is off and no system:rbac-disabled record was written; " +
				"the posture would be visible only in a config file")
		}
		if disabled.RBACEnforced {
			t.Error("system:rbac-disabled record claims enforcement is on")
		}
	})
}

// TestStartupPostureWithoutARecorderIsANoop keeps startup from depending on the
// trail being wired: the service must still boot when it is not.
func TestStartupPostureWithoutARecorderIsANoop(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("recordStartupPosture panicked with no recorder: %v", r)
		}
	}()
	recordStartupPosture(&config.Config{RBACEnabled: true}, nil)
}
