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

package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/instrumentation"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func TestAgentBuildOptions_ResponseShape(t *testing.T) {
	cat := instrumentation.NewForTest([]instrumentation.Version{
		{Version: "0.2.1", PythonVersions: []string{"3.10", "3.11"}, ImageRepository: "x"},
		{Version: "0.4.0", PythonVersions: []string{"3.11", "3.12"}, ImageRepository: "x"},
	}, "0.2.1")
	c := NewAgentBuildOptionsController(cat, []string{"3.10", "3.11", "3.12"}, "3.11")

	req := httptest.NewRequest(http.MethodGet, "/orgs/org1/agent-build-options", nil)
	w := httptest.NewRecorder()
	c.GetAgentBuildOptions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got spec.AgentBuildOptionsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v; body=%s", err, w.Body.String())
	}
	if got.Python.DefaultVersion != "3.11" {
		t.Errorf("python.defaultVersion = %q, want 3.11", got.Python.DefaultVersion)
	}
	if len(got.Python.SupportedVersions) != 3 {
		t.Errorf("python.supportedVersions len = %d, want 3", len(got.Python.SupportedVersions))
	}
	if got.Instrumentation.DefaultVersion != "0.2.1" {
		t.Errorf("instrumentation.defaultVersion = %q, want 0.2.1", got.Instrumentation.DefaultVersion)
	}
	if len(got.Instrumentation.Versions) != 2 {
		t.Fatalf("instrumentation.versions len = %d, want 2", len(got.Instrumentation.Versions))
	}
	// Sorted newest-first.
	if got.Instrumentation.Versions[0].Version != "0.4.0" || got.Instrumentation.Versions[1].Version != "0.2.1" {
		t.Errorf("versions = %v, want [0.4.0, 0.2.1]", got.Instrumentation.Versions)
	}
}

func TestCompareVersionsDesc(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.4.0", "0.2.1", true},
		{"0.2.1", "0.4.0", false},
		{"0.10.0", "0.2.1", true},  // the case lexicographic sort gets wrong
		{"0.2.1", "0.10.0", false}, // the case lexicographic sort gets wrong
		{"1.0.0", "0.99.99", true},
		{"0.10.0", "0.10.0", false}, // equal -> not less
		{"0.10.1", "0.10.0", true},
		{"0.2", "0.2.0", false}, // shorter version with same components doesn't sort before
	}
	for _, tc := range cases {
		got := compareVersionsDesc(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("compareVersionsDesc(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
