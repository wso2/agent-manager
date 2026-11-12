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

package memory_test

import (
	"context"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-observer/store"
	"github.com/wso2/agent-manager/agent-manager-observer/store/memory"
	"github.com/wso2/agent-manager/agent-manager-observer/store/storetest"
)

func newStore(_ *testing.T, seeds []storetest.Seed) *memory.Store {
	m := memory.New()
	for _, s := range seeds {
		m.Add(s.Scope, s.Trace, s.Spans...)
	}
	return m
}

func TestSuite(t *testing.T) {
	storetest.Run(t, func(t *testing.T, seeds []storetest.Seed) store.TraceStore {
		return newStore(t, seeds)
	})
}

func TestEmptyOrgRunsNoQuery(t *testing.T) {
	m := memory.New()
	m.Add(store.Scope{OrgID: "org-a", Names: store.Names{Org: "acme"}}, store.Trace{TraceID: "trace-a"})
	noOrg := store.Scope{OrgID: "org-a"}

	if _, err := m.QueryTraces(context.Background(), noOrg, store.TraceQuery{}); err == nil {
		t.Fatal("QueryTraces with no org: want error")
	}
	if _, err := m.QueryTraceSpans(context.Background(), noOrg, "trace-a", store.SpanQuery{}); err == nil {
		t.Fatal("QueryTraceSpans with no org: want error")
	}
	if n := m.Queries(); n != 0 {
		t.Errorf("Queries() = %d, want 0", n)
	}
}
