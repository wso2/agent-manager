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
)

func TestAttributesToAny(t *testing.T) {
	got := attributesToAny(map[string]string{
		"http.status_code":           "500",
		"gen_ai.system":              "openai",
		"gen_ai.embedding.dimension": "oops",
		"gen_ai.usage.input_tokens":  "420",
		"llm.is_streaming":           "true",
	})
	if got["http.status_code"] != float64(500) {
		t.Errorf("status code should be a number, got %v", got["http.status_code"])
	}
	if got["gen_ai.system"] != "openai" {
		t.Error("text values must stay text")
	}
	if got["gen_ai.embedding.dimension"] != "oops" {
		t.Error("a value that is not a number must stay text")
	}
	if got["gen_ai.usage.input_tokens"] != float64(420) {
		t.Errorf("input tokens should be a number, got %v", got["gen_ai.usage.input_tokens"])
	}
	if got["llm.is_streaming"] != true {
		t.Errorf("is_streaming should be a boolean, got %v", got["llm.is_streaming"])
	}
	if attributesToAny(nil) == nil {
		t.Error("a nil map should give an empty map, not nil")
	}
}
