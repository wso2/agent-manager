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
	"os"
	"path/filepath"
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

func TestBuildBuild_Buildpack(t *testing.T) {
	opts := &CreateOptions{
		BuildType:       "buildpack",
		Language:        "go",
		LanguageVersion: "1.22",
		RunCommand:      "go run .",
	}
	b, err := buildBuild(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	b, err := buildBuild(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bp, _ := b.AsBuildpackBuild()
	if bp.Buildpack.Language != "python" {
		t.Errorf("Language = %q, want %q (should be lowercased)", bp.Buildpack.Language, "python")
	}
}

func TestBuildBuild_BuildpackMinimal(t *testing.T) {
	opts := &CreateOptions{BuildType: "buildpack", Language: "python"}
	b, err := buildBuild(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	b, err := buildBuild(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	disc, _ := b.Discriminator()
	if disc != "docker" {
		t.Fatalf("discriminator = %q, want docker", disc)
	}
	d, _ := b.AsDockerBuild()
	if d.Docker.DockerfilePath != "build/Dockerfile" {
		t.Errorf("DockerfilePath = %q, want build/Dockerfile", d.Docker.DockerfilePath)
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
	if iface.Schema == nil || iface.Schema.Path != "openapi.yaml" {
		t.Errorf("Schema.Path = %v, want openapi.yaml", iface.Schema)
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

func TestBuildConfig_None(t *testing.T) {
	opts := &CreateOptions{}
	cfg := buildConfig(opts)
	if cfg != nil {
		t.Errorf("expected nil, got %+v", cfg)
	}
}

func TestBuildConfig_DisableAutoInstrumentation(t *testing.T) {
	opts := &CreateOptions{DisableAutoInstrumentation: true}
	cfg := buildConfig(opts)
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.EnableAutoInstrumentation == nil || *cfg.EnableAutoInstrumentation != false {
		t.Errorf("EnableAutoInstrumentation = %v, want false", cfg.EnableAutoInstrumentation)
	}
}

func TestBuildConfig_EnvVariables(t *testing.T) {
	opts := &CreateOptions{
		Env:           []string{"FOO=bar", "BAZ=qux"},
		EnvSecret:     []string{"SECRET=hunter2"},
		EnvFromSecret: []string{"DB_PASS=my-secret"},
	}
	cfg := buildConfig(opts)
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Env == nil {
		t.Fatal("Env is nil")
	}
	envs := *cfg.Env
	if len(envs) != 4 {
		t.Fatalf("len(Env) = %d, want 4", len(envs))
	}

	// --env FOO=bar (IsSensitive omitted — server default)
	assertEnvVar(t, envs[0], "FOO", strPtr("bar"), nil, nil)
	// --env BAZ=qux
	assertEnvVar(t, envs[1], "BAZ", strPtr("qux"), nil, nil)
	// --env-secret SECRET=hunter2
	assertEnvVar(t, envs[2], "SECRET", strPtr("hunter2"), nil, boolPtr(true))
	// --env-from-secret DB_PASS=my-secret
	assertEnvVar(t, envs[3], "DB_PASS", nil, strPtr("my-secret"), nil)
}

func TestBuildConfig_EnvAutoInstrumentationOmittedByDefault(t *testing.T) {
	opts := &CreateOptions{Env: []string{"A=1"}}
	cfg := buildConfig(opts)
	if cfg.EnableAutoInstrumentation != nil {
		t.Errorf("EnableAutoInstrumentation = %v, want nil", cfg.EnableAutoInstrumentation)
	}
}

func TestLoadModelConfig(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "model.yaml")
	content := `- envMappings:
    dev:
      providerName: openai
      configuration:
        policies: []
  environmentVariables:
    - key: OPENAI_API_KEY
      name: OpenAI API Key
`
	os.WriteFile(path, []byte(content), 0644)

	configs, err := loadModelConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if configs == nil || len(*configs) != 1 {
		t.Fatalf("len = %v, want 1", configs)
	}
	c := (*configs)[0]
	if _, ok := c.EnvMappings["dev"]; !ok {
		t.Error("missing dev env mapping")
	}
}

func TestLoadModelConfig_JSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "model.json")
	content := `[{"envMappings":{"prod":{"providerName":"anthropic","configuration":{"policies":[]}}}}]`
	os.WriteFile(path, []byte(content), 0644)

	configs, err := loadModelConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*configs) != 1 {
		t.Fatalf("len = %d, want 1", len(*configs))
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
	req, err := Build(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	req, err := Build(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	req, err := Build(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.AgentType.Type != "agent-api" {
		t.Errorf("AgentType.Type = %q, want agent-api", req.AgentType.Type)
	}
	disc, _ := req.Build.Discriminator()
	if disc != "docker" {
		t.Errorf("build discriminator = %q, want docker", disc)
	}
}

func TestBuild_WithModelConfig(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "mc.yaml")
	os.WriteFile(path, []byte(`[{envMappings: {dev: {providerName: test, configuration: {}}}}]`), 0644)

	opts := validBuildpackOpts()
	opts.ModelConfigFile = path
	req, err := Build(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.ModelConfig == nil || len(*req.ModelConfig) != 1 {
		t.Errorf("ModelConfig = %v, want 1 entry", req.ModelConfig)
	}
}

func TestBuild_NoConfigWhenUnset(t *testing.T) {
	opts := validBuildpackOpts()
	req, err := Build(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Configurations != nil {
		t.Errorf("Configurations = %v, want nil", req.Configurations)
	}
}

// --- helpers ---

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

func assertEnvVar(t *testing.T, ev amsvc.EnvironmentVariable, key string, value, secretRef *string, isSensitive *bool) {
	t.Helper()
	if ev.Key != key {
		t.Errorf("Key = %q, want %q", ev.Key, key)
	}
	if (ev.Value == nil) != (value == nil) || (ev.Value != nil && *ev.Value != *value) {
		t.Errorf("Value = %v, want %v", ev.Value, value)
	}
	if (ev.SecretRef == nil) != (secretRef == nil) || (ev.SecretRef != nil && *ev.SecretRef != *secretRef) {
		t.Errorf("SecretRef = %v, want %v", ev.SecretRef, secretRef)
	}
	if (ev.IsSensitive == nil) != (isSensitive == nil) || (ev.IsSensitive != nil && *ev.IsSensitive != *isSensitive) {
		t.Errorf("IsSensitive = %v, want %v", ev.IsSensitive, isSensitive)
	}
}
