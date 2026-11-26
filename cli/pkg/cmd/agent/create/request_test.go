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
	"encoding/json"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
)

func TestBuildProvisioning(t *testing.T) {
	opts := &CreateOptions{
		RepoURL:    "https://github.com/example/repo",
		RepoBranch: "main",
		RepoPath:   "/app",
	}
	p := buildProvisioning(opts)
	if p.Type != "internal" {
		t.Errorf("Type = %q, want internal", p.Type)
	}
	if p.Repository == nil {
		t.Fatal("Repository is nil")
	}
	if p.Repository.Url != opts.RepoURL {
		t.Errorf("Url = %q, want %q", p.Repository.Url, opts.RepoURL)
	}
	if p.Repository.Branch != opts.RepoBranch {
		t.Errorf("Branch = %q, want %q", p.Repository.Branch, opts.RepoBranch)
	}
	if p.Repository.AppPath != opts.RepoPath {
		t.Errorf("AppPath = %q, want %q", p.Repository.AppPath, opts.RepoPath)
	}
	if p.Repository.SecretRef != nil {
		t.Errorf("SecretRef = %v, want nil", p.Repository.SecretRef)
	}
}

func TestBuildProvisioning_WithSecret(t *testing.T) {
	opts := &CreateOptions{
		RepoURL:    "https://github.com/example/repo",
		RepoBranch: "main",
		RepoPath:   "/",
		RepoSecret: "gh-token",
	}
	p := buildProvisioning(opts)
	if p.Repository.SecretRef == nil {
		t.Fatal("SecretRef is nil")
	}
	if *p.Repository.SecretRef != "gh-token" {
		t.Errorf("SecretRef = %q, want gh-token", *p.Repository.SecretRef)
	}
}

func TestBuildProvisioning_PrefixesSlash(t *testing.T) {
	opts := &CreateOptions{
		RepoURL:    "https://github.com/example/repo",
		RepoBranch: "main",
		RepoPath:   "src/app",
	}
	p := buildProvisioning(opts)
	if p.Repository.AppPath != "/src/app" {
		t.Errorf("AppPath = %q, want %q", p.Repository.AppPath, "/src/app")
	}
}

func TestBuildInterface_PrefixesSlash(t *testing.T) {
	opts := &CreateOptions{
		SubType:     "custom-api",
		Port:        8000,
		BasePath:    "v1",
		OpenAPISpec: "spec.yaml",
	}
	iface := buildInterface(opts)
	if iface.BasePath == nil || *iface.BasePath != "/v1" {
		t.Errorf("BasePath = %v, want /v1", iface.BasePath)
	}
	if iface.Schema == nil || iface.Schema.Path == nil || *iface.Schema.Path != "/spec.yaml" {
		t.Errorf("Schema.Path = %v, want /spec.yaml", iface.Schema)
	}
}

func TestEnsureLeadingSlash(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"/", "/"},
		{"/v1", "/v1"},
		{"v1", "/v1"},
		{"src/app", "/src/app"},
	}
	for _, tt := range tests {
		if got := ensureLeadingSlash(tt.in); got != tt.want {
			t.Errorf("ensureLeadingSlash(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildBuild_Buildpack(t *testing.T) {
	opts := &CreateOptions{
		BuildType:       "buildpack",
		Language:        "go",
		LanguageVersion: "1.22",
		RunCommand:      "go run .",
	}
	b, v := buildBuild(opts)
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	disc, err := b.Discriminator()
	if err != nil {
		t.Fatalf("discriminator: %v", err)
	}
	if disc != "buildpack" {
		t.Fatalf("discriminator = %q, want buildpack", disc)
	}
	bp, err := b.AsBuildpackBuild()
	if err != nil {
		t.Fatalf("AsBuildpackBuild: %v", err)
	}
	if bp.Buildpack.Language != "go" {
		t.Errorf("Language = %q, want go", bp.Buildpack.Language)
	}
	if bp.Buildpack.LanguageVersion == nil || *bp.Buildpack.LanguageVersion != "1.22" {
		t.Errorf("LanguageVersion = %v, want 1.22", bp.Buildpack.LanguageVersion)
	}
	if bp.Buildpack.RunCommand == nil || *bp.Buildpack.RunCommand != "go run ." {
		t.Errorf("RunCommand = %v, want 'go run .'", bp.Buildpack.RunCommand)
	}
}

func TestBuildBuild_BuildpackLowercasesLanguage(t *testing.T) {
	opts := &CreateOptions{BuildType: "buildpack", Language: "Python"}
	b, v := buildBuild(opts)
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	bp, _ := b.AsBuildpackBuild()
	if bp.Buildpack.Language != "python" {
		t.Errorf("Language = %q, want %q (should be lowercased)", bp.Buildpack.Language, "python")
	}
}

func TestBuildBuild_BuildpackMinimal(t *testing.T) {
	opts := &CreateOptions{BuildType: "buildpack", Language: "python"}
	b, v := buildBuild(opts)
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	bp, _ := b.AsBuildpackBuild()
	if bp.Buildpack.LanguageVersion != nil {
		t.Errorf("LanguageVersion = %v, want nil", bp.Buildpack.LanguageVersion)
	}
	if bp.Buildpack.RunCommand != nil {
		t.Errorf("RunCommand = %v, want nil", bp.Buildpack.RunCommand)
	}
}

func TestBuildBuild_Docker(t *testing.T) {
	opts := &CreateOptions{BuildType: "docker", Dockerfile: "build/Dockerfile"}
	b, v := buildBuild(opts)
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	disc, _ := b.Discriminator()
	if disc != "docker" {
		t.Fatalf("discriminator = %q, want docker", disc)
	}
	d, _ := b.AsDockerBuild()
	if d.Docker.DockerfilePath != "/build/Dockerfile" {
		t.Errorf("DockerfilePath = %q, want /build/Dockerfile (leading slash normalized)", d.Docker.DockerfilePath)
	}
}

func TestBuildInterface_AgentAPI(t *testing.T) {
	opts := &CreateOptions{
		Port:        9090,
		BasePath:    "/v1",
		OpenAPISpec: "openapi.yaml",
	}
	iface := buildInterface(opts)
	if iface.Type != "HTTP" {
		t.Errorf("Type = %q, want HTTP", iface.Type)
	}
	if iface.Port == nil || *iface.Port != 9090 {
		t.Errorf("Port = %v, want 9090", iface.Port)
	}
	if iface.BasePath == nil || *iface.BasePath != "/v1" {
		t.Errorf("BasePath = %v, want /v1", iface.BasePath)
	}
	if iface.Schema == nil || iface.Schema.Path == nil || *iface.Schema.Path != "/openapi.yaml" {
		t.Errorf("Schema.Path = %v, want /openapi.yaml", iface.Schema)
	}
}

func TestBuildInterface_AgentAPIDefaults(t *testing.T) {
	opts := &CreateOptions{Port: 8000}
	iface := buildInterface(opts)
	if iface.Port == nil || *iface.Port != 8000 {
		t.Errorf("Port = %v, want 8000", iface.Port)
	}
	if iface.BasePath != nil {
		t.Errorf("BasePath = %v, want nil", iface.BasePath)
	}
	if iface.Schema != nil {
		t.Errorf("Schema = %v, want nil", iface.Schema)
	}
}

func TestBuildInterface_ChatAPI(t *testing.T) {
	opts := &CreateOptions{SubType: "chat-api", Port: 8000}
	iface := buildInterface(opts)
	if iface.Type != "HTTP" {
		t.Errorf("Type = %q, want HTTP", iface.Type)
	}
	if iface.Port == nil || *iface.Port != 8000 {
		t.Errorf("Port = %v, want 8000", iface.Port)
	}
	if iface.BasePath != nil {
		t.Errorf("BasePath = %v, want nil", iface.BasePath)
	}
	if iface.Schema != nil {
		t.Errorf("Schema = %v, want nil", iface.Schema)
	}
}

func TestBuild_FullBuildpack(t *testing.T) {
	opts := &CreateOptions{
		Name:         "my-agent",
		DisplayName:  "My Agent",
		Description:  "An agent",
		SubType:      "custom-api",
		Provisioning: "internal",
		RepoURL:      "https://github.com/example/repo",
		RepoBranch:   "main",
		RepoPath:     "/",
		BuildType:    "buildpack",
		Language:     "go",
		Port:         8000,
	}
	req, v := Build(opts)
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	if req.AgentType.Type != "agent-api" {
		t.Errorf("AgentType.Type = %q, want agent-api (derived from internal provisioning)", req.AgentType.Type)
	}
	if req.AgentType.SubType == nil || *req.AgentType.SubType != "custom-api" {
		t.Errorf("SubType = %v, want custom-api", req.AgentType.SubType)
	}
	if req.Name != "my-agent" {
		t.Errorf("Name = %q", req.Name)
	}
	if req.DisplayName != "My Agent" {
		t.Errorf("DisplayName = %q", req.DisplayName)
	}
	if req.Description == nil || *req.Description != "An agent" {
		t.Errorf("Description = %v", req.Description)
	}
	if req.Provisioning.Type != "internal" {
		t.Errorf("Provisioning.Type = %q", req.Provisioning.Type)
	}
	if req.Build == nil {
		t.Fatal("Build is nil")
	}
	if req.InputInterface == nil {
		t.Fatal("InputInterface is nil")
	}

	// round-trip via JSON to verify union serializes
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	json.Unmarshal(raw, &decoded)
	build := decoded["build"].(map[string]any)
	if build["type"] != "buildpack" {
		t.Errorf("build.type = %v", build["type"])
	}
}

func TestBuild_ExplicitTypeOverride(t *testing.T) {
	opts := &CreateOptions{
		Name:         "my-agent",
		DisplayName:  "My Agent",
		Type:         "custom-type",
		Provisioning: "internal",
		RepoURL:      "https://github.com/example/repo",
		RepoBranch:   "main",
		RepoPath:     "/",
		BuildType:    "buildpack",
		Language:     "go",
		Port:         8000,
	}
	req, v := Build(opts)
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	if req.AgentType.Type != "custom-type" {
		t.Errorf("AgentType.Type = %q, want custom-type (explicit override)", req.AgentType.Type)
	}
}

func TestBuild_FullDocker(t *testing.T) {
	opts := &CreateOptions{
		Name:         "docker-agent",
		DisplayName:  "Docker Agent",
		Provisioning: "internal",
		RepoURL:      "https://github.com/example/repo",
		RepoBranch:   "main",
		RepoPath:     "/",
		BuildType:    "docker",
		Dockerfile:   "Dockerfile.prod",
		Port:         8000,
	}
	req, v := Build(opts)
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	if req.AgentType.Type != "agent-api" {
		t.Errorf("AgentType.Type = %q, want agent-api", req.AgentType.Type)
	}
	disc, _ := req.Build.Discriminator()
	if disc != "docker" {
		t.Errorf("build discriminator = %q, want docker", disc)
	}
}

// gatewayModelConfig returns the single modelConfig entry the builder put in
// the request's ai-gateway extension block.
func gatewayModelConfig(t *testing.T, req *amsvc.CreateAgentRequest) map[string]any {
	t.Helper()
	if req.Extensions == nil {
		t.Fatal("Extensions is nil, want an ai-gateway block")
	}
	block, ok := (*req.Extensions)["ai-gateway"]
	if !ok {
		t.Fatalf("Extensions = %v, want an ai-gateway block", *req.Extensions)
	}
	mcs, ok := block["modelConfig"].([]any)
	if !ok || len(mcs) != 1 {
		t.Fatalf("modelConfig = %v, want 1 entry", block["modelConfig"])
	}
	return mcs[0].(map[string]any)
}

func TestBuild_ModelConfig_ProviderOnly(t *testing.T) {
	opts := validBuildpackOpts()
	opts.LLMProvider = "openai"
	req, v := Build(opts)
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	mc := gatewayModelConfig(t, &req)
	if mc["providerName"] != "openai" {
		t.Errorf("providerName = %v, want openai", mc["providerName"])
	}
	if _, ok := mc["environmentVariables"]; ok {
		t.Errorf("environmentVariables = %v, want absent (no overrides)", mc["environmentVariables"])
	}
}

func TestBuild_ModelConfig_WithEnvOverrides(t *testing.T) {
	opts := validBuildpackOpts()
	opts.LLMProvider = "openai"
	opts.LLMURLEnv = "MY_URL"
	opts.LLMAPIKeyEnv = "MY_KEY"
	req, v := Build(opts)
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	evs, ok := gatewayModelConfig(t, &req)["environmentVariables"].([]map[string]string)
	if !ok {
		t.Fatal("environmentVariables missing, want 2 entries")
	}
	got := map[string]string{}
	for _, ev := range evs {
		got[ev["key"]] = ev["name"]
	}
	if got["url"] != "MY_URL" || got["apikey"] != "MY_KEY" {
		t.Errorf("env overrides = %v, want url=MY_URL apikey=MY_KEY", got)
	}
}

func TestBuild_NoProvider_NoExtension(t *testing.T) {
	req, v := Build(validBuildpackOpts())
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	if req.Extensions != nil {
		t.Errorf("Extensions = %v, want nil without --llm-provider", *req.Extensions)
	}
}

func TestBuildProvisioning_External(t *testing.T) {
	opts := &CreateOptions{Provisioning: "external"}
	p := buildProvisioning(opts)
	if p.Type != amsvc.ProvisioningTypeExternal {
		t.Errorf("Type = %q, want %q", p.Type, amsvc.ProvisioningTypeExternal)
	}
	if p.Repository != nil {
		t.Errorf("Repository = %v, want nil", p.Repository)
	}
}

func TestBuild_External(t *testing.T) {
	opts := &CreateOptions{
		Name:         "testing",
		DisplayName:  "Testing",
		Description:  "dssdf",
		Provisioning: "external",
	}
	req, v := Build(opts)
	if len(v) != 0 {
		t.Fatalf("Build returned violations: %v", v)
	}
	if req.Name != "testing" || req.DisplayName != "Testing" {
		t.Errorf("name/displayName mismatch: %+v", req)
	}
	if req.Description == nil || *req.Description != "dssdf" {
		t.Errorf("Description = %v, want \"dssdf\"", req.Description)
	}
	if req.AgentType == nil || req.AgentType.Type != "external-agent-api" {
		t.Errorf("AgentType.Type = %v, want external-agent-api", req.AgentType)
	}
	if req.AgentType.SubType != nil {
		t.Errorf("AgentType.SubType should be nil for external, got %v", *req.AgentType.SubType)
	}
	if req.Provisioning.Type != amsvc.ProvisioningTypeExternal {
		t.Errorf("Provisioning.Type = %q, want external", req.Provisioning.Type)
	}
	if req.Provisioning.Repository != nil {
		t.Errorf("Repository should be nil for external, got %+v", req.Provisioning.Repository)
	}
	if req.Build != nil {
		t.Errorf("Build should be nil for external, got %+v", req.Build)
	}
	if req.InputInterface != nil {
		t.Errorf("InputInterface should be nil for external, got %+v", req.InputInterface)
	}
	if req.Extensions != nil {
		t.Errorf("Extensions should be nil for external, got %+v", *req.Extensions)
	}
}

// An unknown --build-type survives conversion as the raw union discriminator
// so validateRequest reports it as a field-path violation instead of the
// builder hard-failing.
func TestBuildBuild_UnknownTypePassesThrough(t *testing.T) {
	opts := &CreateOptions{BuildType: "nix"}
	b, v := buildBuild(opts)
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	if b == nil {
		t.Fatal("Build is nil, want raw union carrying the unknown type")
	}
	disc, err := b.Discriminator()
	if err != nil {
		t.Fatalf("discriminator: %v", err)
	}
	if disc != "nix" {
		t.Errorf("discriminator = %q, want nix", disc)
	}
}

func TestBuildBuild_EmptyTypeReturnsNil(t *testing.T) {
	b, v := buildBuild(&CreateOptions{})
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	if b != nil {
		t.Errorf("Build = %+v, want nil when --build-type unset", b)
	}
}

// An unknown --provisioning passes through so validateRequest reports it;
// internal-only sections are not built for it.
func TestBuild_UnknownProvisioningPassesThrough(t *testing.T) {
	opts := &CreateOptions{
		Name:         "x",
		DisplayName:  "X",
		Provisioning: "cloud",
	}
	req, v := Build(opts)
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	if string(req.Provisioning.Type) != "cloud" {
		t.Errorf("Provisioning.Type = %q, want cloud (passthrough)", req.Provisioning.Type)
	}
	if req.Provisioning.Repository != nil || req.Build != nil || req.InputInterface != nil {
		t.Errorf("internal sections should not be built for unknown provisioning: %+v", req)
	}
}

// --- helpers ---

func strPtr(s string) *string { return &s }
