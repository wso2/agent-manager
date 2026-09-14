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

// validExternalOpts returns a CreateOptions for external that passes validation.
func validExternalOpts() *CreateOptions {
	return &CreateOptions{
		Name:         "my-agent",
		DisplayName:  "My Agent",
		Provisioning: "external",
	}
}

func mustPrepareErr(t *testing.T, opts *CreateOptions) error {
	t.Helper()
	_, err := prepare(opts)
	if err == nil {
		t.Fatal("expected error")
	}
	return err
}

func TestPrepare_ValidBuildpack(t *testing.T) {
	if _, err := prepare(validBuildpackOpts()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepare_ValidDocker(t *testing.T) {
	if _, err := prepare(validDockerOpts()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepare_ValidExternal(t *testing.T) {
	if _, err := prepare(validExternalOpts()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// The aggregated error header distinguishes spec problems from flag-usage
// problems and is shared by flag mode and file mode.
func TestPrepare_HeaderIsInvalidAgentSpec(t *testing.T) {
	err := mustPrepareErr(t, &CreateOptions{Provisioning: "internal", Port: 8000})
	if !strings.Contains(err.Error(), "invalid agent spec") {
		t.Errorf("error %q missing header 'invalid agent spec'", err.Error())
	}
}

func TestPrepare_MissingRequiredFlags(t *testing.T) {
	opts := &CreateOptions{
		Provisioning: "internal",
		Port:         8000,
	}
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "spec.name is required")
	assertContains(t, details, "spec.displayName is required")
	assertContains(t, details, "spec.provisioning.repository.url is required")
	assertContains(t, details, "spec.provisioning.repository.branch is required")
	assertContains(t, details, "spec.provisioning.repository.appPath is required")
	assertContains(t, details, "spec.build is required for internal provisioning")
	assertContains(t, details, "spec.agentType.subType is required for internal provisioning")
}

func TestPrepare_ExternalRejectsInternalOnlyFlags(t *testing.T) {
	opts := validExternalOpts()
	opts.SubType = "custom-api"
	opts.RepoURL = "https://x"
	opts.RepoBranch = "main"
	opts.RepoPath = "/"
	opts.RepoSecret = "gh-token"
	opts.BuildType = "buildpack"
	opts.Language = "python"
	opts.LanguageVersion = "3.11"
	opts.RunCommand = "python main.py"
	opts.Dockerfile = "Dockerfile"
	opts.PortSet = true
	opts.Port = 9000
	opts.BasePath = "/api"
	opts.OpenAPISpec = "/openapi.yaml"
	opts.Env = []string{"FOO=bar"}
	opts.EnvSecret = []string{"BAZ=quux"}
	opts.EnvFromSecret = []string{"QUUX=secret-name"}
	opts.DisableAutoInstrumentation = true
	opts.LLMProvider = "openai"

	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	for _, msg := range []string{
		"--subtype is not allowed for external provisioning",
		"--repo-url is not allowed for external provisioning",
		"--repo-branch is not allowed for external provisioning",
		"--repo-path is not allowed for external provisioning",
		"--repo-secret is not allowed for external provisioning",
		"--build-type is not allowed for external provisioning",
		"--language is not allowed for external provisioning",
		"--language-version is not allowed for external provisioning",
		"--run-command is not allowed for external provisioning",
		"--dockerfile is not allowed for external provisioning",
		"--port is not allowed for external provisioning",
		"--base-path is not allowed for external provisioning",
		"--openapi-spec is not allowed for external provisioning",
		"--env is not allowed for external provisioning",
		"--env-secret is not allowed for external provisioning",
		"--env-from-secret is not allowed for external provisioning",
		"--no-auto-instrumentation is not allowed for external provisioning",
		"--llm-provider is not allowed for external provisioning",
	} {
		assertContains(t, details, msg)
	}
}

func TestPrepare_UnknownProvisioning(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Provisioning = "magic"
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, `spec.provisioning.type must be "internal" or "external", got "magic"`)
}

func TestPrepare_BuildpackRequiresLanguage(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Language = ""
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "spec.build.buildpack.language is required")
}

// The server requires languageVersion and runCommand for every buildpack
// language except Ballerina — flag mode must report both locally.
func TestPrepare_BuildpackRequiresVersionAndRunCommand(t *testing.T) {
	opts := validBuildpackOpts()
	opts.LanguageVersion = ""
	opts.RunCommand = ""
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, `spec.build.buildpack.languageVersion is required for language "go"`)
	assertContains(t, details, `spec.build.buildpack.runCommand is required for language "go"`)
}

func TestPrepare_BallerinaNeedsNoVersionOrRunCommand(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Language = "ballerina"
	opts.LanguageVersion = ""
	opts.RunCommand = ""
	if _, err := prepare(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepare_BuildpackRejectsDockerfile(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Dockerfile = "Dockerfile"
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--build-type=buildpack conflicts with --dockerfile")
}

func TestPrepare_DockerRequiresDockerfile(t *testing.T) {
	opts := validDockerOpts()
	opts.Dockerfile = ""
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "spec.build.docker.dockerfilePath is required")
}

// --dockerfile is normalized like --repo-path: the server requires a leading
// slash, so the builder prepends one instead of round-tripping a 400.
func TestPrepare_DockerfileWithoutSlashNormalized(t *testing.T) {
	opts := validDockerOpts()
	opts.Dockerfile = "build/Dockerfile"
	req, err := prepare(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	d, err := req.Build.AsDockerBuild()
	if err != nil {
		t.Fatalf("build union: %v", err)
	}
	if d.Docker.DockerfilePath != "/build/Dockerfile" {
		t.Errorf("DockerfilePath = %q, want /build/Dockerfile", d.Docker.DockerfilePath)
	}
}

func TestPrepare_DockerRejectsBuildpackFlags(t *testing.T) {
	opts := validDockerOpts()
	opts.Language = "go"
	opts.LanguageVersion = "1.22"
	opts.RunCommand = "go run ."
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--build-type=docker conflicts with --language")
	assertContains(t, details, "--build-type=docker conflicts with --language-version")
	assertContains(t, details, "--build-type=docker conflicts with --run-command")
}

func TestPrepare_UnknownBuildType(t *testing.T) {
	opts := validBuildpackOpts()
	opts.BuildType = "nix"
	opts.Language = ""
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, `spec.build.type must be "buildpack" or "docker", got "nix"`)
}

func TestPrepare_MissingBuildType(t *testing.T) {
	opts := validBuildpackOpts()
	opts.BuildType = ""
	opts.Language = ""
	opts.LanguageVersion = ""
	opts.RunCommand = ""
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "spec.build is required for internal provisioning")
}

func TestPrepare_ChatAPIRejectsInterfaceFlags(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = "chat-api"
	opts.PortSet = true
	opts.Port = 9000
	opts.BasePath = "/v1"
	opts.OpenAPISpec = "spec.yaml"
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--port is not allowed for subtype chat-api")
	assertContains(t, details, "--base-path is not allowed for subtype chat-api")
	assertContains(t, details, "--openapi-spec is not allowed for subtype chat-api")
}

func TestPrepare_ChatAPIPortSetTo8000Rejected(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = "chat-api"
	opts.PortSet = true
	opts.Port = 8000
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--port is not allowed for subtype chat-api")
}

func TestPrepare_ChatAPIPortUnsetAllowed(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = "chat-api"
	opts.PortSet = false
	if _, err := prepare(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Name format (including '/') is server-owned; the client only requires presence.
func TestPrepare_NameWithSlashAccepted(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Name = "weird/name"
	if _, err := prepare(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepare_RepoPathWithoutSlashPasses(t *testing.T) {
	opts := validBuildpackOpts()
	opts.RepoPath = "src/app"
	if _, err := prepare(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepare_PortRange(t *testing.T) {
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
			_, err := prepare(opts)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// Missing '=' cannot survive conversion (splitEnv would smuggle the whole
// entry through as a key), so it is reported flag-phrased at the conversion
// layer. Key format and duplicates survive into the request and are reported
// as field paths.
func TestPrepare_EnvMissingSeparator(t *testing.T) {
	tests := []struct {
		flag    string
		mutate  func(*CreateOptions)
		wantSub string
	}{
		{"--env", func(o *CreateOptions) { o.Env = []string{"NOEQUALS"} }, `--env "NOEQUALS": missing '=' separator`},
		{"--env-secret", func(o *CreateOptions) { o.EnvSecret = []string{"NOSEP"} }, `--env-secret "NOSEP": missing '=' separator`},
		{"--env-from-secret", func(o *CreateOptions) { o.EnvFromSecret = []string{"NOSEP"} }, `--env-from-secret "NOSEP": missing '=' separator`},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			opts := validBuildpackOpts()
			tt.mutate(opts)
			err := mustPrepareErr(t, opts)
			details := mustFlagDetails(t, err)
			assertContains(t, details, tt.wantSub)
		})
	}
}

func TestPrepare_EnvInvalidKey(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Env = []string{"1BAD=val"}
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, `spec.configurations.env: invalid key "1BAD"`)
}

func TestPrepare_DuplicateEnvKey(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Env = []string{"FOO=bar"}
	opts.EnvSecret = []string{"FOO=secret"}
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, `spec.configurations.env: duplicate key "FOO"`)
}

func TestPrepare_DuplicateEnvKeyAcrossThreeFlags(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Env = []string{"DB_HOST=localhost"}
	opts.EnvSecret = []string{"API_KEY=secret"}
	opts.EnvFromSecret = []string{"DB_HOST=my-secret"}
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, `spec.configurations.env: duplicate key "DB_HOST"`)
}

func TestPrepare_ValidEnv(t *testing.T) {
	opts := validBuildpackOpts()
	opts.Env = []string{"FOO=bar", "BAZ=qux"}
	opts.EnvSecret = []string{"SECRET_KEY=hunter2"}
	opts.EnvFromSecret = []string{"DB_PASS=my-secret"}
	if _, err := prepare(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepare_LLMEnvFlagsRequireProvider(t *testing.T) {
	opts := validBuildpackOpts()
	opts.LLMURLEnv = "MY_URL"
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--llm-url-env/--llm-api-key-env require --llm-provider")
}

func TestPrepare_LLMProviderValid(t *testing.T) {
	opts := validBuildpackOpts()
	opts.LLMProvider = "openai"
	opts.LLMURLEnv = "MY_URL"
	opts.LLMAPIKeyEnv = "MY_KEY"
	if _, err := prepare(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepare_ExternalRejectsLLMEnvFlags(t *testing.T) {
	opts := validExternalOpts()
	opts.LLMURLEnv = "MY_URL"
	opts.LLMAPIKeyEnv = "MY_KEY"
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--llm-url-env is not allowed for external provisioning")
	assertContains(t, details, "--llm-api-key-env is not allowed for external provisioning")
}

func TestPrepare_CustomAPIRequiresBasePathAndOpenAPISpec(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = "custom-api"
	opts.BasePath = ""
	opts.OpenAPISpec = ""
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "spec.inputInterface.basePath is required for subtype custom-api")
	assertContains(t, details, "spec.inputInterface.schema.path is required for subtype custom-api")
}

func TestPrepare_CustomAPIValid(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = "custom-api"
	opts.BasePath = "/v1"
	opts.OpenAPISpec = "openapi.yaml"
	if _, err := prepare(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepare_UnknownSubType(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = "grpc"
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, `spec.agentType.subType must be "chat-api", "custom-api" or "a2a-agent", got "grpc"`)
}

func TestPrepare_CustomAPIWithoutSlashPasses(t *testing.T) {
	opts := validBuildpackOpts()
	opts.SubType = "custom-api"
	opts.BasePath = "v1"
	opts.OpenAPISpec = "spec.yaml"
	if _, err := prepare(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Conversion-layer and request-layer violations surface in one aggregated error.
func TestPrepare_MergesConversionAndRequestViolations(t *testing.T) {
	opts := validBuildpackOpts()
	opts.DisplayName = ""           // request layer
	opts.Env = []string{"NOEQUALS"} // conversion layer
	err := mustPrepareErr(t, opts)
	details := mustFlagDetails(t, err)
	assertContains(t, details, "spec.displayName is required")
	assertContains(t, details, `--env "NOEQUALS": missing '=' separator`)
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
