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

package rbac

import "testing"

func TestPermissionScope(t *testing.T) {
	cases := []struct {
		perm Permission
		want string
	}{
		{TraceRead, "amp:observability:trace-read"},
		{LogRead, "amp:observability:log-read"},
		{BuildLogRead, "amp:observability:build-log-read"},
		{MonitorLogRead, "amp:observability:monitor-log-read"},
		{MetricRead, "amp:observability:metric-read"},
	}
	for _, tc := range cases {
		if got := tc.perm.Scope(); got != tc.want {
			t.Errorf("Scope() = %q, want %q", got, tc.want)
		}
	}
}
