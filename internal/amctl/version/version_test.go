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

package version

import (
	"testing"
)

func TestStringDev(t *testing.T) {
	Version = "dev"
	Commit = "none"
	Date = "unknown"

	got := String()
	want := "amctl version dev (commit none, built unknown)"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestStringRelease(t *testing.T) {
	Version = "0.1.0"
	Commit = "abc1234"
	Date = "2026-04-30T12:00:00Z"

	got := String()
	want := "amctl version 0.1.0 (commit abc1234, built 2026-04-30T12:00:00Z)"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
