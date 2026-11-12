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
	"testing"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

func TestListOptions(t *testing.T) {
	ten, zero, big := 10, 0, 5000
	asc, bad := "asc", "sideways"

	if l, d, err := listOptions(observer.TracesQueryRequest{}); err != nil || l != 100 || !d {
		t.Errorf("defaults: got %d %v %v", l, d, err)
	}
	if l, d, err := listOptions(observer.TracesQueryRequest{Limit: &ten, SortOrder: &asc}); err != nil || l != 10 || d {
		t.Errorf("asc 10: got %d %v %v", l, d, err)
	}
	if _, _, err := listOptions(observer.TracesQueryRequest{Limit: &zero}); err == nil {
		t.Error("limit 0 must fail")
	}
	if _, _, err := listOptions(observer.TracesQueryRequest{Limit: &big}); err == nil {
		t.Error("limit 5000 must fail")
	}
	if _, _, err := listOptions(observer.TracesQueryRequest{SortOrder: &bad}); err == nil {
		t.Error("a bad sort order must fail")
	}
}

func TestSpanListOptions(t *testing.T) {
	ten, zero, big := 10, 0, 20000
	asc, bad := "asc", "sideways"

	if l, d, err := spanListOptions(observer.TracesQueryRequest{}); err != nil || l != 1000 || d {
		t.Errorf("defaults: got %d %v %v", l, d, err)
	}
	if l, d, err := spanListOptions(observer.TracesQueryRequest{Limit: &ten, SortOrder: &asc}); err != nil || l != 10 || d {
		t.Errorf("asc 10: got %d %v %v", l, d, err)
	}
	for _, req := range []observer.TracesQueryRequest{{Limit: &big}, {Limit: &zero}, {SortOrder: &bad}} {
		if _, _, err := spanListOptions(req); err == nil {
			t.Errorf("expected an error for %+v", req)
		}
	}
}

func TestNewTraceReader(t *testing.T) {
	res := scope.StaticResolver{}

	r, err := NewTraceReader(nil, Options{Table: "otel_traces", Layout: config.TraceLayoutExporter, DefaultNamespace: "default", Resolver: res})
	if err != nil || r.cols != exporterTable {
		t.Fatalf("exporter layout: err %v", err)
	}
	r, err = NewTraceReader(nil, Options{Table: "otel_traces_tenant", Layout: config.TraceLayoutTenant, DefaultNamespace: "default", Resolver: res})
	if err != nil || r.cols != tenantTable {
		t.Fatalf("tenant layout: err %v", err)
	}

	bad := []Options{
		{Table: "otel_traces", Layout: "sideways", DefaultNamespace: "default", Resolver: res},                             // unknown layout
		{Table: "otel_traces; DROP TABLE x", Layout: config.TraceLayoutTenant, DefaultNamespace: "default", Resolver: res}, // unsafe table name
		{Table: "otel_traces", Layout: config.TraceLayoutTenant, DefaultNamespace: "default"},                              // no resolver
		{Table: "otel_traces", Layout: config.TraceLayoutTenant, Resolver: res},                                            // no namespace
	}
	for _, o := range bad {
		if _, err := NewTraceReader(nil, o); err == nil {
			t.Errorf("expected an error for %+v", o)
		}
	}
}
