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

package clickhouse

import (
	"context"
	"errors"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
)

// A Scope with no organization is refused before any query: the store
// points at nothing listening, so a query would fail with another error.
func TestNoOrgRunsNoQuery(t *testing.T) {
	s, err := New(Config{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for name, sc := range map[string]store.Scope{
		"no org id":   {Names: store.Names{Org: "acme"}, AgentID: "a"},
		"no org name": {OrgID: "org-a", AgentID: "a"},
		"blank":       {OrgID: " ", Names: store.Names{Org: "acme"}},
	} {
		if _, err := s.QueryTraces(ctx, sc, store.TraceQuery{}); !errors.Is(err, store.ErrMissingOrg) {
			t.Errorf("%s: QueryTraces error = %v, want ErrMissingOrg", name, err)
		}
		if _, err := s.QueryTraceSpans(ctx, sc, "t", store.SpanQuery{}); !errors.Is(err, store.ErrMissingOrg) {
			t.Errorf("%s: QueryTraceSpans error = %v, want ErrMissingOrg", name, err)
		}
		if _, err := s.GetSpanDetails(ctx, sc, "t", "s"); !errors.Is(err, store.ErrMissingOrg) {
			t.Errorf("%s: GetSpanDetails error = %v, want ErrMissingOrg", name, err)
		}
	}
	if err := s.DeleteAgent(ctx, "", "a"); !errors.Is(err, store.ErrMissingOrg) {
		t.Errorf("DeleteAgent with no org = %v, want ErrMissingOrg", err)
	}
	if err := s.DeleteAgent(ctx, "org-a", ""); !errors.Is(err, ErrMissingAgent) {
		t.Errorf("DeleteAgent with no agent = %v, want ErrMissingAgent", err)
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	for _, cfg := range []Config{
		{URL: ""},
		{URL: "tcp://clickhouse:9000"},
		{URL: "http://clickhouse:8123", Database: "otel; DROP TABLE x"},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("New(%+v) succeeded, want an error", cfg)
		}
	}
}
