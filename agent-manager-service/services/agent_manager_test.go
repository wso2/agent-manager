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

package services

import (
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/instrumentation"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func TestValidateInstrumentationVersion_UsesCatalog(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.11"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	if err := s.validateInstrumentationVersion("0.2.1"); err != nil {
		t.Errorf("0.2.1 should be valid: %v", err)
	}
	err := s.validateInstrumentationVersion("9.9.9")
	if err == nil {
		t.Fatal("9.9.9 should be invalid")
	}
	if !strings.Contains(err.Error(), "9.9.9") {
		t.Errorf("error %q should mention 9.9.9", err)
	}
}

func TestValidatePythonInstrumentationPair(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.10", "3.11"}, ImageRepository: "x"},
			{Version: "0.4.0", PythonVersions: []string{"3.12", "3.13"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	if err := s.validatePythonInstrumentationPair("3.11", "0.2.1"); err != nil {
		t.Errorf("3.11 + 0.2.1 should be valid: %v", err)
	}
	err := s.validatePythonInstrumentationPair("3.13", "0.2.1")
	if err == nil {
		t.Fatal("3.13 + 0.2.1 should be invalid")
	}
	if !strings.Contains(err.Error(), "3.13") || !strings.Contains(err.Error(), "0.2.1") {
		t.Errorf("error %q should mention both python and instrumentation versions", err)
	}
}

func TestValidateEffectivePair_FallsBackToDefault(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.10", "3.11"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	// nil requested version means "use platform default", which is 0.2.1.
	if err := s.validateEffectivePythonInstrumentationPair("3.11", nil); err != nil {
		t.Errorf("3.11 + default(0.2.1) should be valid: %v", err)
	}
	err := s.validateEffectivePythonInstrumentationPair("3.13", nil)
	if err == nil {
		t.Fatal("3.13 + default(0.2.1) should be invalid")
	}
	if !strings.Contains(err.Error(), "3.13") || !strings.Contains(err.Error(), "0.2.1") {
		t.Errorf("error %q should name the resolved default version, not just nil", err)
	}
}

func TestBuildpackPythonVersion_Normalises(t *testing.T) {
	mk := func(lang string, version *string) *spec.Build {
		b := spec.BuildpackBuildAsBuild(&spec.BuildpackBuild{
			Buildpack: spec.BuildpackConfig{
				Language:        lang,
				LanguageVersion: version,
			},
		})
		return &b
	}
	strPtr := func(s string) *string { return &s }

	cases := []struct {
		name string
		in   *spec.Build
		want string
	}{
		{"bare minor", mk("python", strPtr("3.11")), "3.11"},
		{"with patch", mk("python", strPtr("3.11.4")), "3.11"},
		{"with x", mk("python", strPtr("3.11.x")), "3.11"},
		{"leading whitespace", mk("python", strPtr("  3.11  ")), "3.11"},
		{"whitespace only", mk("python", strPtr("   ")), ""},
		{"empty", mk("python", strPtr("")), ""},
		{"capital P language", mk("Python", strPtr("3.11")), ""},
		{"non python language", mk("nodejs", strPtr("20")), ""},
		{"single component", mk("python", strPtr("3")), ""},
		{"nil version", mk("python", nil), ""},
		{"nil build", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildpackPythonVersion(tc.in)
			if got != tc.want {
				t.Errorf("buildpackPythonVersion = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateEffectivePair_NoPythonIsNoOp(t *testing.T) {
	instrumentation.SetCatalog(instrumentation.NewForTest(
		[]instrumentation.Version{
			{Version: "0.2.1", PythonVersions: []string{"3.11"}, ImageRepository: "x"},
		},
		"0.2.1",
	))
	s := &agentManagerService{}

	// Empty python means the agent isn't a python-buildpack build.
	if err := s.validateEffectivePythonInstrumentationPair("", nil); err != nil {
		t.Errorf("empty python should be a no-op: %v", err)
	}
}
