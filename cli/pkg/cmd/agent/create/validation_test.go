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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
)

// validBuildpackOpts returns a CreateOptions that passes all validation.
func validBuildpackOpts() *CreateOptions {
	return &CreateOptions{
		Name:            "my-agent",
		DisplayName:     "My Agent",
		Provisioning:    "internal",
		RepoURL:         "https://github.com/example/repo",
		RepoBranch:      "main",
		RepoPath:        "/",
		BuildType:       "buildpack",
		Language:        "go",
		LanguageVersion: "1.22",
		RunCommand:      "go run .",
		SubType:         "chat-api",
		Port:            8000,
	}
}

// validDockerOpts returns a CreateOptions for docker that passes all validation.
func validDockerOpts() *CreateOptions {
	return &CreateOptions{
		Name:         "my-agent",
		DisplayName:  "My Agent",
		Provisioning: "internal",
		RepoURL:      "https://github.com/example/repo",
		RepoBranch:   "main",
		RepoPath:     "/",
		BuildType:    "docker",
		Dockerfile:   "Dockerfile",
		SubType:      "chat-api",
		Port:         8000,
	}
}

func TestValidate_ValidBuildpack(t *testing.T) {
	if err := validate(validBuildpackOpts()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_ValidDocker(t *testing.T) {
	if err := validate(validDockerOpts()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_MissingRequiredFlags(t *testing.T) {
	opts := &CreateOptions{
		Provisioning: "internal",
		Port:         8000,
	}
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "name argument is required")
	assertContains(t, details, "--display-name is required")
	assertContains(t, details, "--repo-url is required for internal provisioning")
	assertContains(t, details, "--repo-branch is required for internal provisioning")
	assertContains(t, details, "--repo-path is required for internal provisioning")
	assertContains(t, details, "--build-type is required for internal provisioning")
	assertContains(t, details, "--subtype is required")
}

func TestValidate_ExternalProvisioning(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Provisioning = "external"
	err := validate(opts)
	if err == nil {
		t.Fatal("expected error for external provisioning")
	}
	var ce clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if ce.Message != "external provisioning is not yet supported by amctl agent create" {
		t.Errorf("message = %q", ce.Message)
	}
	if _, ok := ce.AdditionalData["details"]; ok {
		t.Error("external error should not have aggregate details")
	}
}

func TestValidate_UnknownProvisioning(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Provisioning = "magic"
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, `--provisioning must be "internal" or "external", got "magic"`)
}

func TestValidate_BuildpackRequiresLanguage(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Language = ""
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--build-type=buildpack requires --language")
}

func TestValidate_BuildpackRequiresLanguageVersion(t *testing.T) {
	opts := validBuildpackOpts()
	opts.LanguageVersion = ""
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--build-type=buildpack requires --language-version")
}

func TestValidate_BuildpackRequiresRunCommand(t *testing.T) {
	opts := validBuildpackOpts()
	opts.RunCommand = ""
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--build-type=buildpack requires --run-command")
}

func TestValidate_BuildpackRequiresBothLanguageVersionAndRunCommand(t *testing.T) {
	opts := validBuildpackOpts()
	opts.LanguageVersion = ""
	opts.RunCommand = ""
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--build-type=buildpack requires --language-version")
	assertContains(t, details, "--build-type=buildpack requires --run-command")
}

func TestValidate_BuildpackRejectsDockerfile(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Dockerfile = "Dockerfile"
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--build-type=buildpack conflicts with --dockerfile")
}

func TestValidate_DockerRequiresDockerfile(t *testing.T) {
	opts := validDockerOpts()
	opts.Dockerfile = ""
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--build-type=docker requires --dockerfile")
}

func TestValidate_DockerRejectsBuildpackFlags(t *testing.T) {
	opts := validDockerOpts()
	opts.Language = "go"
	opts.LanguageVersion = "1.22"
	opts.RunCommand = "go run ."
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--build-type=docker conflicts with --language")
	assertContains(t, details, "--build-type=docker conflicts with --language-version")
	assertContains(t, details, "--build-type=docker conflicts with --run-command")
}

func TestValidate_UnknownBuildType(t *testing.T) {
	opts := validBuildpackOpts()
	opts.BuildType = "nix"
	opts.Language = ""
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, `--build-type must be "buildpack" or "docker", got "nix"`)
}

func TestValidate_ChatAPIRejectsInterfaceFlags(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = "chat-api"
	opts.PortSet = true
	opts.Port = 9000
	opts.BasePath = "/v1"
	opts.OpenAPISpec = "spec.yaml"
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--port is not allowed for subtype chat-api")
	assertContains(t, details, "--base-path is not allowed for subtype chat-api")
	assertContains(t, details, "--openapi-spec is not allowed for subtype chat-api")
}

func TestValidate_ChatAPIPortSetTo8000Rejected(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = "chat-api"
	opts.PortSet = true
	opts.Port = 8000
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--port is not allowed for subtype chat-api")
}

func TestValidate_ChatAPIPortUnsetAllowed(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = "chat-api"
	opts.PortSet = false
	if err := validate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_NameWithSlash(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Name = "bad/name"
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "name must not contain '/'")
}

func TestValidate_RepoPathWithoutSlashPasses(t *testing.T) {
	opts := validBuildpackOpts()
	opts.RepoPath = "src/app"
	if err := validate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_PortRange(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{"zero", 0, true},
		{"one", 1, false},
		{"8000", 8000, false},
		{"65535", 65535, false},
		{"65536", 65536, true},
		{"negative", -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := validBuildpackOpts()
			opts.SubType = "custom-api"
			opts.BasePath = "/v1"
			opts.OpenAPISpec = "/spec.yaml"
			opts.Port = tt.port
			err := validate(opts)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidate_EnvKeyFormat(t *testing.T) {
	tests := []struct {
		name    string
		env     []string
		wantSub string
	}{
		{"missing separator", []string{"NOEQUALS"}, `--env "NOEQUALS": missing '=' separator`},
		{"bad key start", []string{"1BAD=val"}, `--env "1BAD=val": invalid key "1BAD"`},
		{"hyphen in key", []string{"NO-DASH=val"}, `--env "NO-DASH=val": invalid key "NO-DASH"`},
		{"empty key", []string{"=val"}, `--env "=val": invalid key ""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := validBuildpackOpts()
			opts.Env = tt.env
			err := validate(opts)
			details := mustFlagDetails(t, err)
			assertContains(t, details, tt.wantSub)
		})
	}
}

func TestValidate_EnvSecretKeyFormat(t *testing.T) {
	opts := validBuildpackOpts()
	opts.EnvSecret = []string{"BAD KEY=val"}
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, `--env-secret "BAD KEY=val": invalid key "BAD KEY"`)
}

func TestValidate_EnvFromSecretKeyFormat(t *testing.T) {
	opts := validBuildpackOpts()
	opts.EnvFromSecret = []string{"NOSEP"}
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, `--env-from-secret "NOSEP": missing '=' separator`)
}

func TestValidate_DuplicateEnvKey(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Env = []string{"FOO=bar"}
	opts.EnvSecret = []string{"FOO=secret"}
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, `duplicate env key "FOO"`)
}

func TestValidate_DuplicateEnvKeyAcrossThreeFlags(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Env = []string{"DB_HOST=localhost"}
	opts.EnvSecret = []string{"API_KEY=secret"}
	opts.EnvFromSecret = []string{"DB_HOST=my-secret"}
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, `duplicate env key "DB_HOST"`)
}

func TestValidate_ValidEnv(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Env = []string{"FOO=bar", "BAZ=qux"}
	opts.EnvSecret = []string{"SECRET_KEY=hunter2"}
	opts.EnvFromSecret = []string{"DB_PASS=my-secret"}
	if err := validate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_ModelConfigFileMissing(t *testing.T) {
	opts := validBuildpackOpts()
	opts.ModelConfigFile = "/nonexistent/model-config.yaml"
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--model-config-file: ")
}

func TestValidate_ModelConfigFileInvalid(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.yaml")
	if err := os.WriteFile(path, []byte("not: a: list: ["), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	opts := validBuildpackOpts()
	opts.ModelConfigFile = path
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--model-config-file: ")
}

func TestValidate_ModelConfigFileValid(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "model.yaml")
	content := `- envMappings:
    dev:
      providerName: openai
      configuration:
        policies: []
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	opts := validBuildpackOpts()
	opts.ModelConfigFile = path
	if err := validate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_CustomAPIRequiresBasePathAndOpenAPISpec(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = "custom-api"
	opts.BasePath = ""
	opts.OpenAPISpec = ""
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--subtype=custom-api requires --base-path")
	assertContains(t, details, "--subtype=custom-api requires --openapi-spec")
}

func TestValidate_CustomAPIValid(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = "custom-api"
	opts.BasePath = "/v1"
	opts.OpenAPISpec = "openapi.yaml"
	if err := validate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_UnknownSubType(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = "grpc"
	err := validate(opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, `--subtype must be "chat-api" or "custom-api", got "grpc"`)
}

func TestValidate_CustomAPIWithoutSlashPasses(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = "custom-api"
	opts.BasePath = "v1"
	opts.OpenAPISpec = "spec.yaml"
	if err := validate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- helpers ---

func mustFlagDetails(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	var fe *cmdutil.FlagError
	if !errors.As(err, &fe) {
		t.Fatalf("expected *cmdutil.FlagError, got %T: %v", err, err)
	}
	var ce clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected clierr.CLIError, got %T", err)
	}
	details, ok := ce.AdditionalData["details"].([]string)
	if !ok {
		t.Fatalf("details type = %T, want []string", ce.AdditionalData["details"])
	}
	return details
}

func assertContains(t *testing.T, details []string, want string) {
	t.Helper()
	for _, d := range details {
		if strings.Contains(d, want) {
			return
		}
	}
	t.Errorf("details %v does not contain %q", details, want)
}
