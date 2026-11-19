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
package opensearch

import "testing"

func TestCapTraceIO(t *testing.T) {
	long := make([]interface{}, MaxTraceIOItems+5)
	if got := CapTraceIO(long).([]interface{}); len(got) != MaxTraceIOItems {
		t.Fatalf("[]interface{}: got %d items, want %d", len(got), MaxTraceIOItems)
	}
	msgs := make([]PromptMessage, MaxTraceIOItems+1)
	if got := CapTraceIO(msgs).([]PromptMessage); len(got) != MaxTraceIOItems {
		t.Fatalf("[]PromptMessage: got %d items, want %d", len(got), MaxTraceIOItems)
	}
	short := []string{"a", "b"}
	if got := CapTraceIO(short).([]string); len(got) != 2 {
		t.Fatalf("short slice changed: %v", got)
	}
	if got := CapTraceIO("text"); got != "text" {
		t.Fatalf("string changed: %v", got)
	}
	if got := CapTraceIO(nil); got != nil {
		t.Fatalf("nil changed: %v", got)
	}
}
