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
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestHelmDefaultInstrumentationVersionConsistent guards the release
// path between the Helm chart and the embedded baseline. A developer
// who bumps `defaultInstrumentationVersion` in values.yaml without
// regenerating baseline.json (or without first adding the new version
// to .github/release-config.json) ships a chart whose freshly-installed
// pod refuses to start with "default instrumentation version not in
// effective set". That runtime failure surfaces during a customer's
// helm install, not in CI; this test moves the catch to PR time.
//
// The buildpack-python overlap check that the Wire provider runs at
// boot isn't repeated here because including utils would pull in
// config.init() and its env requirements; mismatch there still
// surfaces at helm-install time via validateDefaultCoversBuildpackPython.
func TestHelmDefaultInstrumentationVersionConsistent(t *testing.T) {
	const valuesRel = "../../deployments/helm-charts/wso2-agent-manager/values.yaml"

	abs, err := filepath.Abs(valuesRel)
	if err != nil {
		t.Fatalf("resolve values.yaml path: %v", err)
	}
	raw, err := os.ReadFile(abs)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skipf("values.yaml not found at %s (module checked out without the chart?)", abs)
	}
	if err != nil {
		t.Fatalf("read %s: %v", abs, err)
	}

	// Decode just the keys we need. The chart has many other values;
	// using a narrowly-typed shape keeps the test from breaking on
	// every unrelated chart change.
	var values struct {
		AgentManagerService struct {
			Config struct {
				OTEL struct {
					DefaultInstrumentationVersion string `yaml:"defaultInstrumentationVersion"`
				} `yaml:"otel"`
			} `yaml:"config"`
		} `yaml:"agentManagerService"`
	}
	if err := yaml.Unmarshal(raw, &values); err != nil {
		t.Fatalf("parse %s: %v", abs, err)
	}

	chartDefault := values.AgentManagerService.Config.OTEL.DefaultInstrumentationVersion
	if chartDefault == "" {
		t.Fatal("agentManagerService.config.otel.defaultInstrumentationVersion is empty in values.yaml; the chart will deploy with no platform default")
	}

	baseline, err := decodeBaseline()
	if err != nil {
		t.Fatalf("decode embedded baseline: %v", err)
	}
	for _, v := range baseline {
		if v.Version == chartDefault {
			return
		}
	}

	available := make([]string, 0, len(baseline))
	for _, v := range baseline {
		available = append(available, v.Version)
	}
	t.Fatalf(
		"chart default %q is not in the embedded baseline %v; "+
			"add the entry to .github/release-config.json and run "+
			"`make gen-instrumentation-baseline` before bumping the chart default",
		chartDefault, available,
	)
}
