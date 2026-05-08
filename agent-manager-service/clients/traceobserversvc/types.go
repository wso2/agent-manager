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

package traceobserversvc

// TraceListParams holds query parameters for listing/exporting traces.
type TraceListParams struct {
	Organization string
	Project      string
	Component    string
	Environment  string
	StartTime    string
	EndTime      string
	Limit        int
	SortOrder    string
}

// TraceDetailsParams holds query parameters for fetching a specific trace.
type TraceDetailsParams struct {
	TraceID      string
	Organization string
	Project      string
	Component    string
	Environment  string
	SortOrder    string
	Limit        int
	StartTime    string
	EndTime      string
}

// SpanDetailsParams holds query parameters for fetching a specific span in a trace.
type SpanDetailsParams struct {
	TraceID      string
	SpanID       string
	Organization string
	Project      string
	Component    string
	Environment  string
}
