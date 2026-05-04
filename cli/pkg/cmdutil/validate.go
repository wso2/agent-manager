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

package cmdutil

import "strings"

// ValidatePathParam checks that value is safe to embed in a URL path segment.
// label describes the parameter for error messages (e.g. "agent name").
func ValidatePathParam(label, value string) error {
	if strings.TrimSpace(value) == "" {
		return FlagErrorf("%s must not be empty", label)
	}
	if strings.Contains(value, "/") {
		return FlagErrorf("%s must not contain '/'", label)
	}
	return nil
}
