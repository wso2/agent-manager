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

import "strconv"

type attrType int

const (
	attrNumber attrType = iota + 1
	attrBool
)

// typedAttributes lists attributes whose original type was a number or a boolean.
// ClickHouse stores every value as text, so these are converted back.
var typedAttributes = map[string]attrType{
	"http.status_code":           attrNumber,
	"gen_ai.embedding.dimension": attrNumber,
	"gen_ai.request.temperature": attrNumber,
	"gen_ai.request.top_p":       attrNumber,
	"gen_ai.request.max_tokens":  attrNumber,
	"gen_ai.usage.input_tokens":  attrNumber,
	"gen_ai.usage.output_tokens": attrNumber,
	"gen_ai.usage.total_tokens":  attrNumber,
	"llm.usage.total_tokens":     attrNumber,
	"llm.is_streaming":           attrBool,
}

// attributesToAny turns ClickHouse's text-only map into the mixed-value map the
// rest of the code expects. Values that cannot be converted stay text.
func attributesToAny(in map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		switch typedAttributes[k] {
		case attrNumber:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				out[k] = f
				continue
			}
		case attrBool:
			if b, err := strconv.ParseBool(v); err == nil {
				out[k] = b
				continue
			}
		}
		out[k] = v
	}
	return out
}
