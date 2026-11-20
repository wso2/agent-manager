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

package opensearchreader

import (
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

func TestListOptions(t *testing.T) {
	ten, zero, big, huge := 10, 0, 5000, 20000
	asc, desc, bad := "asc", "desc", "sideways"

	// Trace list: 100 and newest first by default.
	if l, d, err := listOptions(observer.TracesQueryRequest{}, defaultLimit, maxLimit, true); err != nil || l != 100 || !d {
		t.Errorf("list defaults: got %d %v %v", l, d, err)
	}
	if l, d, err := listOptions(observer.TracesQueryRequest{Limit: &ten, SortOrder: &asc}, defaultLimit, maxLimit, true); err != nil || l != 10 || d {
		t.Errorf("list asc 10: got %d %v %v", l, d, err)
	}
	// Spans: 1000 and oldest first by default.
	if l, d, err := listOptions(observer.TracesQueryRequest{}, defaultSpanLimit, maxSpanLimit, false); err != nil || l != 1000 || d {
		t.Errorf("span defaults: got %d %v %v", l, d, err)
	}
	if l, d, err := listOptions(observer.TracesQueryRequest{SortOrder: &desc}, defaultSpanLimit, maxSpanLimit, false); err != nil || l != 1000 || !d {
		t.Errorf("span desc: got %d %v %v", l, d, err)
	}
	// 5000 is too many traces for a list (maximum 1000) but fine for the spans of one trace (maximum 10000).
	if _, _, err := listOptions(observer.TracesQueryRequest{Limit: &big}, defaultLimit, maxLimit, true); err == nil {
		t.Error("a list limit of 5000 must fail")
	}
	if l, _, err := listOptions(observer.TracesQueryRequest{Limit: &big}, defaultSpanLimit, maxSpanLimit, false); err != nil || l != 5000 {
		t.Errorf("a span limit of 5000 is allowed: got %d %v", l, err)
	}
	for _, req := range []observer.TracesQueryRequest{{Limit: &huge}, {Limit: &zero}, {SortOrder: &bad}} {
		if _, _, err := listOptions(req, defaultSpanLimit, maxSpanLimit, false); err == nil {
			t.Errorf("expected an error for %+v", req)
		}
	}
}

func TestNewTraceReader(t *testing.T) {
	good := Options{
		Addr: "http://localhost:9200", Username: "admin", Password: "admin",
		IndexPattern: "otel-traces-*", InsecureTLS: true,
		DefaultNamespace: "default", Resolver: scope.StaticResolver{}, Timeout: 5 * time.Second,
	}
	if r, err := NewTraceReader(good); err != nil || r == nil {
		t.Fatalf("good options must work: %v", err)
	}

	bad := []struct {
		name   string
		change func(*Options)
	}{
		{"no address", func(o *Options) { o.Addr = "" }},
		{"address without a scheme", func(o *Options) { o.Addr = "localhost:9200" }},
		{"no username", func(o *Options) { o.Username = "" }},
		{"no password", func(o *Options) { o.Password = "" }},
		{"unsafe index pattern", func(o *Options) { o.IndexPattern = "otel-traces-*; DROP" }},
		{"no namespace", func(o *Options) { o.DefaultNamespace = "" }},
		{"no resolver", func(o *Options) { o.Resolver = nil }},
		{"negative timeout", func(o *Options) { o.Timeout = -time.Second }},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			o := good
			tc.change(&o)
			if _, err := NewTraceReader(o); err == nil {
				t.Error("expected an error")
			}
		})
	}

	// A zero timeout is allowed: it means "use the default".
	o := good
	o.Timeout = 0
	if _, err := NewTraceReader(o); err != nil {
		t.Errorf("a zero timeout should use the default: %v", err)
	}
}
