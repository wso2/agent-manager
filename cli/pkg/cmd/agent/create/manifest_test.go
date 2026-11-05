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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

const internalManifest = `
apiVersion: agent-manager.wso2.com/v1alpha1
kind: Agent
spec:
  name: my-agent
  displayName: My Agent
  agentType:
    type: agent-api
    subType: chat-api
  provisioning:
    type: internal
    repository:
      url: https://github.com/example/repo
      branch: main
      appPath: /app
  build:
    type: buildpack
    buildpack:
      language: python
      languageVersion: "3.12"
      runCommand: python main.py
  inputInterface:
    type: HTTP
`

func TestLoadManifest_Internal(t *testing.T) {
	path := writeManifest(t, internalManifest)
	req, err := loadManifest(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Name != "my-agent" || req.DisplayName != "My Agent" {
		t.Errorf("name/displayName = %q/%q", req.Name, req.DisplayName)
	}
	if req.Provisioning.Repository == nil || req.Provisioning.Repository.Url != "https://github.com/example/repo" {
		t.Errorf("repository = %+v", req.Provisioning.Repository)
	}
	bp, err := req.Build.AsBuildpackBuild()
	if err != nil {
		t.Fatalf("build union: %v", err)
	}
	if bp.Buildpack.Language != "python" {
		t.Errorf("language = %q", bp.Buildpack.Language)
	}
}

func TestLoadManifest_AgentKind(t *testing.T) {
	path := writeManifest(t, `
apiVersion: agent-manager.wso2.com/v1alpha1
kind: Agent
spec:
  name: prebuilt
  displayName: Prebuilt
  provisioning:
    type: internal
    agentKind:
      name: rag-bot
      version: 1.2.0
`)
	req, err := loadManifest(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Provisioning.AgentKind == nil || req.Provisioning.AgentKind.Name != "rag-bot" || req.Provisioning.AgentKind.Version != "1.2.0" {
		t.Errorf("agentKind = %+v", req.Provisioning.AgentKind)
	}
}

func TestLoadManifest_WrongAPIVersion(t *testing.T) {
	path := writeManifest(t, `
apiVersion: nope/v1
kind: Agent
spec:
  name: x
`)
	_, err := loadManifest(path)
	if err == nil || !strings.Contains(err.Error(), "apiVersion") {
		t.Fatalf("expected apiVersion error, got %v", err)
	}
}

func TestLoadManifest_WrongKind(t *testing.T) {
	path := writeManifest(t, `
apiVersion: agent-manager.wso2.com/v1alpha1
kind: Project
spec:
  name: x
`)
	_, err := loadManifest(path)
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("expected kind error, got %v", err)
	}
}

func TestLoadManifest_UnknownSpecField(t *testing.T) {
	path := writeManifest(t, `
apiVersion: agent-manager.wso2.com/v1alpha1
kind: Agent
spec:
  name: x
  displayNam: typo
`)
	_, err := loadManifest(path)
	if err == nil || !strings.Contains(err.Error(), "displayNam") {
		t.Fatalf("expected unknown-field error naming the field, got %v", err)
	}
}

// The Build union stores raw bytes, so top-level DisallowUnknownFields cannot
// see inside it — loadManifest must strict-decode the concrete variant.
func TestLoadManifest_UnknownFieldInsideBuildUnion(t *testing.T) {
	path := writeManifest(t, `
apiVersion: agent-manager.wso2.com/v1alpha1
kind: Agent
spec:
  name: x
  displayName: X
  provisioning:
    type: internal
    repository:
      url: https://github.com/example/repo
      branch: main
      appPath: /
  build:
    type: buildpack
    buildpack:
      language: go
      runCmd: typo
`)
	_, err := loadManifest(path)
	if err == nil || !strings.Contains(err.Error(), "runCmd") {
		t.Fatalf("expected unknown-field error inside build union, got %v", err)
	}
}

// An unknown build.type is not a decode error — it falls through to
// validateRequest so it lands in the aggregated report.
func TestLoadManifest_UnknownBuildTypePassesDecode(t *testing.T) {
	path := writeManifest(t, `
apiVersion: agent-manager.wso2.com/v1alpha1
kind: Agent
spec:
  name: x
  displayName: X
  provisioning:
    type: internal
    repository:
      url: https://github.com/example/repo
      branch: main
      appPath: /
  build:
    type: nix
`)
	if _, err := loadManifest(path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadManifest_FileNotFound(t *testing.T) {
	_, err := loadManifest(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// A manifest can set the agent's health checks: the field is part of the generated request
// type, so the strict decoder accepts it and carries it to the API.
func TestLoadManifest_HealthChecks(t *testing.T) {
	path := writeManifest(t, internalManifest+`
  healthChecks:
    readiness:
      type: HTTP
      path: /health
      periodSeconds: 10
    liveness:
      type: TCP
`)
	req, err := loadManifest(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.HealthChecks == nil || req.HealthChecks.Readiness == nil || req.HealthChecks.Liveness == nil {
		t.Fatalf("healthChecks = %+v", req.HealthChecks)
	}
	readiness := req.HealthChecks.Readiness
	if readiness.Type == nil || *readiness.Type != "HTTP" || readiness.Path == nil || *readiness.Path != "/health" {
		t.Errorf("readiness = %+v", readiness)
	}
	if readiness.PeriodSeconds == nil || *readiness.PeriodSeconds != 10 {
		t.Errorf("readiness.periodSeconds = %v", readiness.PeriodSeconds)
	}
}
