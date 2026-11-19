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

import "reflect"

// MaxTraceIOItems is the most elements a trace-level input or output array
// carries in an API response. Root span I/O is often a chat message list, and
// longer lists are cut to their first MaxTraceIOItems elements. Keep in step
// with maxItems on FullTrace and TraceOverview input/output in docs/openapi.yaml.
const MaxTraceIOItems = 1000

// CapTraceIO returns v with any top-level array cut to MaxTraceIOItems
// elements. Strings, objects and nil pass through unchanged.
func CapTraceIO(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice || rv.Len() <= MaxTraceIOItems {
		return v
	}
	return rv.Slice(0, MaxTraceIOItems).Interface()
}
