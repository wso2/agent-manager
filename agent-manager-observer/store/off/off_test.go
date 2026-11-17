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

package off

import (
	"context"
	"errors"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-observer/store"
)

func TestEveryCallIsDisabled(t *testing.T) {
	ctx := context.Background()
	sc := store.Scope{Names: store.Names{Org: "acme"}}
	var s Store
	if _, err := s.QueryTraces(ctx, sc, store.TraceQuery{}); !errors.Is(err, store.ErrDisabled) {
		t.Errorf("QueryTraces error = %v, want ErrDisabled", err)
	}
	if _, err := s.QueryTraceSpans(ctx, sc, "t1", store.SpanQuery{}); !errors.Is(err, store.ErrDisabled) {
		t.Errorf("QueryTraceSpans error = %v, want ErrDisabled", err)
	}
	if _, err := s.GetSpanDetails(ctx, sc, "t1", "s1"); !errors.Is(err, store.ErrDisabled) {
		t.Errorf("GetSpanDetails error = %v, want ErrDisabled", err)
	}
}
