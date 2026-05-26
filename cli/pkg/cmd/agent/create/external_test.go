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

package create

import (
	"strings"
	"testing"
)

func TestBuildPythonInstructions_ContainsExportLines(t *testing.T) {
	got := buildPythonInstructions("https://otel.example/v1/traces", "tok-123")
	for _, sub := range []string{
		"pip install amp-instrumentation",
		`export AMP_OTEL_ENDPOINT="https://otel.example/v1/traces"`,
		`export AMP_AGENT_API_KEY="tok-123"`,
		"amp-instrument <your_existing_start_command>",
	} {
		if !strings.Contains(got, sub) {
			t.Errorf("instructions missing %q\n---\n%s", sub, got)
		}
	}
}

func TestOtelIngestEndpoint(t *testing.T) {
	cases := map[string]string{
		"https://opentelemetry.obs.dp.cloud.wso2.com":  "https://opentelemetry.obs.dp.cloud.wso2.com/v1/traces",
		"https://opentelemetry.obs.dp.cloud.wso2.com/": "https://opentelemetry.obs.dp.cloud.wso2.com/v1/traces",
		"http://localhost:22893":                       "http://localhost:22893/v1/traces",
	}
	for in, want := range cases {
		if got := otelIngestEndpoint(in); got != want {
			t.Errorf("otelIngestEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}
