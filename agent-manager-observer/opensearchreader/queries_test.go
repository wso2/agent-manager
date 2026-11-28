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
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

func TestListQuery(t *testing.T) {
	w := window{Start: time.Unix(1000, 0), End: time.Unix(2000, 0)}
	b, err := json.Marshal(listQuery(scope.UIDs{Namespace: "default", Component: "abc"}, w, true, 10))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"resource.openchoreo.dev/component-uid":"abc"`,
		`"resource.openchoreo.dev/namespace":"default"`,
		`"min_start_time":"desc"`,
		`"parentSpanId":""`,
		`"size":10`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("query is missing %s", want)
		}
	}
	if strings.Contains(s, "resource.openchoreo.dev/project-uid") {
		t.Error("a project filter appeared although no project was asked for")
	}
}

func TestDetailQueryHasNamespaceGuard(t *testing.T) {
	b, _ := json.Marshal(detailQuery("default", "t1", "s1"))
	s := string(b)
	for _, want := range []string{
		`"resource.openchoreo.dev/namespace":"default"`,
		`"traceId":"t1"`,
		`"spanId":"s1"`,
		`"size":1`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("detail query missing %s", want)
		}
	}
}

func TestSpansQuery(t *testing.T) {
	w := window{Start: time.Unix(1000, 0), End: time.Unix(2000, 0)}

	// With attributes excluded (includeAttrs = false)
	b, err := json.Marshal(spansQuery(scope.UIDs{Namespace: "default", Component: "abc"}, "trace-xyz", w, false, false, 50))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"traceId":"trace-xyz"`,
		`"resource.openchoreo.dev/component-uid":"abc"`,
		`"resource.openchoreo.dev/namespace":"default"`,
		`"excludes":["attributes","resource"]`,
		`"order":"asc"`,
		`"size":50`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("spans query (without attrs) missing %s", want)
		}
	}

	// With attributes included (includeAttrs = true)
	bWithAttrs, _ := json.Marshal(spansQuery(scope.UIDs{Namespace: "default"}, "trace-xyz", w, true, true, 100))
	sWithAttrs := string(bWithAttrs)
	if strings.Contains(sWithAttrs, `"excludes"`) {
		t.Error("attributes should not be excluded when includeAttrs is true")
	}
	if !strings.Contains(sWithAttrs, `"order":"desc"`) {
		t.Error("expected descending order when desc is true")
	}
}

func TestBatchQuery(t *testing.T) {
	w := window{Start: time.Unix(1000, 0), End: time.Unix(2000, 0)}
	b, err := json.Marshal(batchQuery(scope.UIDs{Namespace: "default"}, []string{"t1", "t2"}, w, 1000))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"terms":{"traceId":["t1","t2"]}`,
		`"resource.openchoreo.dev/namespace":"default"`,
		`"size":1000`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("batch query missing %s", want)
		}
	}
}

