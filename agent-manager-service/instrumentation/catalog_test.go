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

package instrumentation

import (
	"testing"
)

func TestLoad_BaselineOnly(t *testing.T) {
	c, err := Load("", "0.2.1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Default() != "0.2.1" {
		t.Errorf("Default = %q, want 0.2.1", c.Default())
	}
	all := c.All()
	if len(all) != 1 {
		t.Fatalf("len(All) = %d, want 1", len(all))
	}
	if all[0].Version != "0.2.1" {
		t.Errorf("All[0].Version = %q, want 0.2.1", all[0].Version)
	}
	if all[0].Source != SourceBundled {
		t.Errorf("All[0].Source = %q, want bundled", all[0].Source)
	}
	if !c.Has("0.2.1") {
		t.Error("Has(0.2.1) = false, want true")
	}
	if c.Has("99.0.0") {
		t.Error("Has(99.0.0) = true, want false")
	}
	got, ok := c.Get("0.2.1")
	if !ok || got.ImageRepository != "ghcr.io/wso2/amp-python-instrumentation-provider" {
		t.Errorf("Get(0.2.1) = %+v, ok=%v", got, ok)
	}
}

func TestLoad_DefaultNotInSet(t *testing.T) {
	if _, err := Load("", "9.9.9"); err == nil {
		t.Fatal("Load with bad default returned nil error")
	}
}
