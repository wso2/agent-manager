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

package update

import "testing"

func TestNewer(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{name: "patch bump", current: "1.0.0", latest: "1.0.1", want: true},
		{name: "minor bump", current: "1.0.5", latest: "1.1.0", want: true},
		{name: "major bump", current: "1.9.9", latest: "2.0.0", want: true},
		{name: "numeric, not lexical", current: "1.0.9", latest: "1.0.10", want: true},
		{name: "equal", current: "1.0.0", latest: "1.0.0", want: false},
		{name: "latest is older", current: "1.1.0", latest: "1.0.9", want: false},
		{name: "pre-release before its release", current: "1.0.0-rc3", latest: "1.0.0", want: true},
		{name: "release after its pre-release", current: "1.0.0", latest: "1.0.0-rc3", want: false},
		{name: "v prefix on current", current: "v1.0.0", latest: "1.0.1", want: true},
		{name: "v prefix on latest", current: "1.0.0", latest: "v1.0.1", want: true},
		{name: "dev current", current: "dev", latest: "1.0.1", want: false},
		{name: "dev latest", current: "1.0.0", latest: "dev", want: false},
		{name: "dev both", current: "dev", latest: "dev", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Newer(tt.current, tt.latest); got != tt.want {
				t.Errorf("Newer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}
