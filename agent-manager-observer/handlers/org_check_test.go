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

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/middleware"
	"github.com/wso2/agent-manager/agent-manager-observer/store"
	"github.com/wso2/agent-manager/agent-manager-observer/store/off"
)

// orgParams is baseParams for organization org.
func orgParams(org string) string {
	return "organization=" + org + "&project=myproject&agent=myagent&environment=dev" +
		"&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z"
}

// withClaims returns r carrying a token for ouHandle; empty means no org claim.
func withClaims(r *http.Request, ouHandle string) *http.Request {
	return r.WithContext(middleware.ContextWithTokenClaims(r.Context(),
		&middleware.TokenClaims{Sub: "user-1", OuHandle: ouHandle}))
}

func TestTraceRoutes_OrgCheck(t *testing.T) {
	// The off store answers 503, so 503 means the request passed the org check.
	h := NewHandler(controllers.NewTracingController(off.Store{}), nil)
	routes := []struct {
		name string
		path func(org string) string
		call func(http.ResponseWriter, *http.Request)
	}{
		{"list", func(o string) string { return "/api/v1/traces?" + orgParams(o) }, h.GetTraceOverviews},
		{"export", func(o string) string { return "/api/v1/traces/export?" + orgParams(o) }, h.ExportTraces},
		{"spans", func(o string) string { return "/api/v1/traces/trace-1/spans?" + orgParams(o) }, h.GetTraceSpans},
		{"span detail", func(o string) string { return "/api/v1/traces/trace-1/spans/span-1?organization=" + o }, h.GetSpanDetail},
	}
	cases := []struct {
		name       string
		tokenOrg   string
		requestOrg string
		want       int
	}{
		{"user token for xyz asks for abc", "xyz", "abc", http.StatusForbidden},
		{"user token for abc asks for abc", "abc", "abc", http.StatusServiceUnavailable},
		{"token without an org claim", "", "abc", http.StatusServiceUnavailable},
	}
	for _, rt := range routes {
		for _, tc := range cases {
			t.Run(rt.name+"/"+tc.name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				rt.call(rec, withClaims(httptest.NewRequest(http.MethodGet, rt.path(tc.requestOrg), nil), tc.tokenOrg))
				assertStatus(t, rec, tc.want)
			})
		}
	}
}

func TestGetSpanDetail_NoOrganizationKeepsUnscopedLookup(t *testing.T) {
	fake := &spanStore{orgs: map[string]bool{"": true}}
	h := NewHandler(controllers.NewTracingController(fake), nil)

	rec := httptest.NewRecorder()
	h.GetSpanDetail(rec, withClaims(httptest.NewRequest(http.MethodGet, "/api/v1/traces/trace-1/spans/span-1", nil), "xyz"))

	assertStatus(t, rec, http.StatusOK)
	if fake.gotOrg != "" {
		t.Errorf("scope org = %q, want empty", fake.gotOrg)
	}
}

func TestGetSpanDetail_SpanOutsideOrgIs404(t *testing.T) {
	fake := &spanStore{orgs: map[string]bool{"abc": true}}
	h := NewHandler(controllers.NewTracingController(fake), nil)

	for org, want := range map[string]int{"abc": http.StatusOK, "xyz": http.StatusNotFound} {
		rec := httptest.NewRecorder()
		h.GetSpanDetail(rec, withClaims(httptest.NewRequest(http.MethodGet,
			"/api/v1/traces/trace-1/spans/span-1?organization="+org, nil), org))
		assertStatus(t, rec, want)
		if fake.gotOrg != org {
			t.Errorf("scope org = %q, want %q", fake.gotOrg, org)
		}
	}
}

// spanStore serves span-1 of trace-1 to the orgs it holds, and ErrNotFound to the rest.
type spanStore struct {
	orgs   map[string]bool
	gotOrg string
}

func (s *spanStore) QueryTraces(context.Context, store.Scope, store.TraceQuery) (*store.TracePage, error) {
	return nil, store.ErrNotFound
}

func (s *spanStore) QueryTraceSpans(context.Context, store.Scope, string, store.SpanQuery) (*store.SpanPage, error) {
	return nil, store.ErrNotFound
}

func (s *spanStore) GetSpanDetails(_ context.Context, sc store.Scope, traceID, spanID string) (*store.SpanDetails, error) {
	s.gotOrg = sc.Names.Org
	if !s.orgs[sc.Names.Org] || traceID != "trace-1" || spanID != "span-1" {
		return nil, store.ErrNotFound
	}
	now := time.Now()
	return &store.SpanDetails{SpanID: spanID, SpanName: "chat", StartTime: now, EndTime: now}, nil
}
