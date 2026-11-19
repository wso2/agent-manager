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

// Anti-drift: the embedded template must round-trip through the exact
// file-mode path (strict decode, envelope check, build-union strictness)
// into the generated request type. If an OpenAPI regen renames or removes a
// field the template mentions, this test fails — keeping the template in
// lockstep with agent-manager-service/docs/api_v1_openapi.yaml via the
// cli-codegen-check workflow.
func TestAgentTemplate_RoundTrip(t *testing.T) {
	path := writeManifest(t, agentTemplate)
	req, err := loadManifest(path)
	if err != nil {
		t.Fatalf("template does not round-trip through file mode: %v", err)
	}

	if req.Name != "<agent-name>" {
		t.Errorf("Name = %q, want placeholder <agent-name>", req.Name)
	}
	if req.Provisioning.Repository == nil || req.Provisioning.Repository.Url == "" {
		t.Errorf("repository not decoded: %+v", req.Provisioning.Repository)
	}
	bp, err := req.Build.AsBuildpackBuild()
	if err != nil {
		t.Fatalf("build union: %v", err)
	}
	if bp.Buildpack.Language == "" {
		t.Error("buildpack.language placeholder missing")
	}
}

// The active (uncommented) template content must be structurally valid:
// a user filling in the placeholders gets zero client-side violations.
func TestAgentTemplate_StructurallyValid(t *testing.T) {
	path := writeManifest(t, agentTemplate)
	req, err := loadManifest(path)
	if err != nil {
		t.Fatalf("load template: %v", err)
	}
	if v := validateRequest(req); len(v) != 0 {
		t.Errorf("template has violations: %v", v)
	}
}

// The commented-out variants must document every reachable top-level spec
// section so manifest authors discover them without reading the API spec.
func TestAgentTemplate_DocumentsVariants(t *testing.T) {
	for _, want := range []string{
		"apiVersion: agent-manager.wso2.com/v1alpha1",
		"kind: Agent",
		"agentKind:",
		"dockerfilePath:",
		"external",
		"secretRef:",
		"ai-gateway:",
		"modelConfig:",
		"basePath:",
	} {
		if !strings.Contains(agentTemplate, want) {
			t.Errorf("template missing %q", want)
		}
	}
}
