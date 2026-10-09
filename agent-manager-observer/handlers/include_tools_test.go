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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// include=tools sets Tools, alone or beside models; an unknown value is an error.
func TestParseInclude_Tools(t *testing.T) {
	tests := []struct {
		raw     []string
		want    controllers.Include
		wantErr bool
	}{
		{raw: []string{"tools"}, want: controllers.Include{Tools: true}},
		{raw: []string{"models,tools"}, want: controllers.Include{Models: true, Tools: true}},
		{raw: []string{"tools", "models"}, want: controllers.Include{Models: true, Tools: true}},
		{raw: []string{"tools,toolz"}, wantErr: true},
		{raw: []string{"Tools"}, wantErr: true},
	}
	for _, tt := range tests {
		got, err := parseInclude(tt.raw)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseInclude(%q) error = %v, wantErr %t", tt.raw, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("parseInclude(%q) = %+v, want %+v", tt.raw, got, tt.want)
		}
	}
}

// include=tools reads the span list without attributes; with models it is the attribute list.
func TestGetTraceOverviews_IncludeTools(t *testing.T) {
	tests := []struct {
		query     string
		wantAttrs bool
	}{
		{query: "&include=tools", wantAttrs: false},
		{query: "&include=models,tools", wantAttrs: true},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			fake := &fakeObserverClient{traces: []observer.TraceInfo{{TraceID: "trace-1", RootSpanID: "root", SpanCount: 2}}}
			h := NewHandler(controllers.NewTracingController(fake), nil)

			r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?"+baseParams()+tt.query, nil)
			rec := httptest.NewRecorder()
			h.GetTraceOverviews(rec, r)

			assertStatus(t, rec, http.StatusOK)
			if fake.lastSpansReq.Limit == nil {
				t.Fatal("expected the trace's spans to be listed upstream, got no QueryTraceSpans call")
			}
			if fake.lastSpansReq.IncludeAttributes != tt.wantAttrs {
				t.Errorf("IncludeAttributes = %t, want %t", fake.lastSpansReq.IncludeAttributes, tt.wantAttrs)
			}
		})
	}
}
