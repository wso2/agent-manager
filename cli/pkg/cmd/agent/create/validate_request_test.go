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

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
)

func intPtr(i int) *int { return &i }

func validInternalReq(t *testing.T) amsvc.CreateAgentRequest {
	t.Helper()
	subType := "chat-api"
	var b amsvc.Build
	if err := b.FromBuildpackBuild(amsvc.BuildpackBuild{
		Type: amsvc.Buildpack,
		Buildpack: amsvc.BuildpackConfig{
			Language:        "python",
			LanguageVersion: strPtr("3.12"),
			RunCommand:      strPtr("python main.py"),
		},
	}); err != nil {
		t.Fatalf("build union: %v", err)
	}
	return amsvc.CreateAgentRequest{
		Name:        "my-agent",
		DisplayName: "My Agent",
		AgentType:   &amsvc.AgentType{Type: "agent-api", SubType: &subType},
		Provisioning: amsvc.Provisioning{
			Type: amsvc.ProvisioningTypeInternal,
			Repository: &amsvc.RepositoryConfig{
				Url:     "https://github.com/example/repo",
				Branch:  "main",
				AppPath: "/app",
			},
		},
		Build: &b,
		InputInterface: &amsvc.InputInterface{
			Type: "HTTP",
			Port: intPtr(8000),
		},
	}
}

func validExternalReq() amsvc.CreateAgentRequest {
	return amsvc.CreateAgentRequest{
		Name:         "my-agent",
		DisplayName:  "My Agent",
		AgentType:    &amsvc.AgentType{Type: "external-agent-api"},
		Provisioning: amsvc.Provisioning{Type: amsvc.ProvisioningTypeExternal},
	}
}

func assertViolation(t *testing.T, violations []string, want string) {
	t.Helper()
	for _, v := range violations {
		if strings.Contains(v, want) {
			return
		}
	}
	t.Errorf("violations %v do not contain %q", violations, want)
}

func assertNoViolations(t *testing.T, violations []string) {
	t.Helper()
	if len(violations) != 0 {
		t.Errorf("expected no violations, got %v", violations)
	}
}

func TestValidateRequest_ValidInternal(t *testing.T) {
	assertNoViolations(t, validateRequest(validInternalReq(t)))
}

func TestValidateRequest_ValidExternal(t *testing.T) {
	assertNoViolations(t, validateRequest(validExternalReq()))
}

func TestValidateRequest_ValidAgentKind(t *testing.T) {
	req := validExternalReq()
	req.AgentType = nil
	req.Provisioning = amsvc.Provisioning{
		Type: amsvc.ProvisioningTypeInternal,
		AgentKind: &struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{Name: "rag-bot", Version: "1.2.0"},
	}
	assertNoViolations(t, validateRequest(req))
}

func TestValidateRequest_RequiredCore(t *testing.T) {
	v := validateRequest(amsvc.CreateAgentRequest{})
	assertViolation(t, v, "spec.name is required")
	assertViolation(t, v, "spec.displayName is required")
	assertViolation(t, v, "spec.provisioning.type is required")
}

func TestValidateRequest_UnknownProvisioningType(t *testing.T) {
	req := validInternalReq(t)
	req.Provisioning.Type = "cloud"
	v := validateRequest(req)
	assertViolation(t, v, `spec.provisioning.type must be "internal" or "external", got "cloud"`)
}

func TestValidateRequest_AgentTypeMismatch(t *testing.T) {
	req := validInternalReq(t)
	req.AgentType.Type = "external-agent-api"
	v := validateRequest(req)
	assertViolation(t, v, `spec.agentType.type must be "agent-api" for internal provisioning, got "external-agent-api"`)

	ext := validExternalReq()
	ext.AgentType.Type = "agent-api"
	v = validateRequest(ext)
	assertViolation(t, v, `spec.agentType.type must be "external-agent-api" for external provisioning, got "agent-api"`)
}

func TestValidateRequest_InternalRequiresRepositoryOrAgentKind(t *testing.T) {
	req := validInternalReq(t)
	req.Provisioning.Repository = nil
	v := validateRequest(req)
	assertViolation(t, v, "spec.provisioning requires exactly one of repository or agentKind for internal provisioning")
}

func TestValidateRequest_RepositoryAndAgentKindConflict(t *testing.T) {
	req := validInternalReq(t)
	req.Provisioning.AgentKind = &struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}{Name: "rag-bot", Version: "1.0.0"}
	v := validateRequest(req)
	assertViolation(t, v, "spec.provisioning requires exactly one of repository or agentKind for internal provisioning")
}

func TestValidateRequest_RepositoryRequiredFields(t *testing.T) {
	req := validInternalReq(t)
	req.Provisioning.Repository = &amsvc.RepositoryConfig{}
	v := validateRequest(req)
	assertViolation(t, v, "spec.provisioning.repository.url is required")
	assertViolation(t, v, "spec.provisioning.repository.branch is required")
	assertViolation(t, v, "spec.provisioning.repository.appPath is required")
}

func TestValidateRequest_AgentKindRequiredFields(t *testing.T) {
	req := validExternalReq()
	req.AgentType = nil
	req.Provisioning = amsvc.Provisioning{
		Type: amsvc.ProvisioningTypeInternal,
		AgentKind: &struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{},
	}
	v := validateRequest(req)
	assertViolation(t, v, "spec.provisioning.agentKind.name is required")
	assertViolation(t, v, "spec.provisioning.agentKind.version is required")
}

func TestValidateRequest_RepositoryRequiresBuild(t *testing.T) {
	req := validInternalReq(t)
	req.Build = nil
	v := validateRequest(req)
	assertViolation(t, v, "spec.build is required for internal provisioning")
}

func TestValidateRequest_UnknownBuildType(t *testing.T) {
	req := validInternalReq(t)
	var b amsvc.Build
	if err := b.UnmarshalJSON([]byte(`{"type":"nix"}`)); err != nil {
		t.Fatalf("seed union: %v", err)
	}
	req.Build = &b
	v := validateRequest(req)
	assertViolation(t, v, `spec.build.type must be "buildpack" or "docker", got "nix"`)
}

func TestValidateRequest_BuildpackRequiresLanguage(t *testing.T) {
	req := validInternalReq(t)
	var b amsvc.Build
	if err := b.FromBuildpackBuild(amsvc.BuildpackBuild{Type: amsvc.Buildpack}); err != nil {
		t.Fatalf("build union: %v", err)
	}
	req.Build = &b
	v := validateRequest(req)
	assertViolation(t, v, "spec.build.buildpack.language is required")
}

// Ballerina is the only language the server exempts from languageVersion and
// runCommand (utils.validateLanguage / validateBuildConfiguration).
func TestValidateRequest_BallerinaNeedsNoVersionOrRunCommand(t *testing.T) {
	req := validInternalReq(t)
	var b amsvc.Build
	if err := b.FromBuildpackBuild(amsvc.BuildpackBuild{
		Type:      amsvc.Buildpack,
		Buildpack: amsvc.BuildpackConfig{Language: "ballerina"},
	}); err != nil {
		t.Fatalf("build union: %v", err)
	}
	req.Build = &b
	assertNoViolations(t, validateRequest(req))
}

func TestValidateRequest_BallerinaNeedsNoVersionOrRunCommand_CaseInsensitive(t *testing.T) {
	req := validInternalReq(t)
	var b amsvc.Build
	if err := b.FromBuildpackBuild(amsvc.BuildpackBuild{
		Type:      amsvc.Buildpack,
		Buildpack: amsvc.BuildpackConfig{Language: "Ballerina"},
	}); err != nil {
		t.Fatalf("build union: %v", err)
	}
	req.Build = &b
	assertNoViolations(t, validateRequest(req))
}

func TestValidateRequest_BuildpackRequiresVersionAndRunCommand(t *testing.T) {
	req := validInternalReq(t)
	var b amsvc.Build
	if err := b.FromBuildpackBuild(amsvc.BuildpackBuild{
		Type:      amsvc.Buildpack,
		Buildpack: amsvc.BuildpackConfig{Language: "python"},
	}); err != nil {
		t.Fatalf("build union: %v", err)
	}
	req.Build = &b
	v := validateRequest(req)
	assertViolation(t, v, `spec.build.buildpack.languageVersion is required for language "python"`)
	assertViolation(t, v, `spec.build.buildpack.runCommand is required for language "python"`)
}

func TestValidateRequest_DockerRequiresDockerfilePath(t *testing.T) {
	req := validInternalReq(t)
	var b amsvc.Build
	if err := b.FromDockerBuild(amsvc.DockerBuild{Type: amsvc.Docker}); err != nil {
		t.Fatalf("build union: %v", err)
	}
	req.Build = &b
	v := validateRequest(req)
	assertViolation(t, v, "spec.build.docker.dockerfilePath is required")
}

// The server rejects dockerfilePath without a leading / (utils.go) — file mode
// must report it locally instead of after the API call.
func TestValidateRequest_DockerfilePathLeadingSlash(t *testing.T) {
	req := validInternalReq(t)
	var b amsvc.Build
	if err := b.FromDockerBuild(amsvc.DockerBuild{
		Type:   amsvc.Docker,
		Docker: amsvc.DockerConfig{DockerfilePath: "Dockerfile"},
	}); err != nil {
		t.Fatalf("build union: %v", err)
	}
	req.Build = &b
	v := validateRequest(req)
	assertViolation(t, v, `spec.build.docker.dockerfilePath must start with "/"`)
}

func TestValidateRequest_AppPathLeadingSlash(t *testing.T) {
	req := validInternalReq(t)
	req.Provisioning.Repository.AppPath = "app"
	v := validateRequest(req)
	assertViolation(t, v, `spec.provisioning.repository.appPath must start with "/"`)
}

// Internal source agents require agentType server-side (utils.go); only
// agentKind-sourced agents may omit it (enriched from the kind).
func TestValidateRequest_SourceRequiresAgentType(t *testing.T) {
	req := validInternalReq(t)
	req.AgentType = nil
	v := validateRequest(req)
	assertViolation(t, v, "spec.agentType.type is required for internal provisioning")
}

func TestValidateRequest_ExternalRequiresAgentType(t *testing.T) {
	req := validExternalReq()
	req.AgentType = nil
	v := validateRequest(req)
	assertViolation(t, v, "spec.agentType.type is required for external provisioning")
}

// Internal source agents require inputInterface for every subtype — chat-api
// included (utils.validateInputInterface).
func TestValidateRequest_SourceRequiresInputInterface(t *testing.T) {
	req := validInternalReq(t)
	req.InputInterface = nil
	v := validateRequest(req)
	assertViolation(t, v, "spec.inputInterface is required for internal provisioning")
}

func TestValidateRequest_SourceRequiresInterfaceType(t *testing.T) {
	req := validInternalReq(t)
	req.InputInterface.Type = ""
	v := validateRequest(req)
	assertViolation(t, v, `spec.inputInterface.type is required (must be "HTTP")`)
}

func TestValidateRequest_UnknownSubType(t *testing.T) {
	req := validInternalReq(t)
	req.AgentType.SubType = strPtr("grpc")
	v := validateRequest(req)
	assertViolation(t, v, `spec.agentType.subType must be "chat-api", "custom-api" or "a2a-agent", got "grpc"`)
}

func TestValidateRequest_SubTypeRequiredForRepository(t *testing.T) {
	req := validInternalReq(t)
	req.AgentType.SubType = nil
	v := validateRequest(req)
	assertViolation(t, v, "spec.agentType.subType is required for internal provisioning")
}

func TestValidateRequest_CustomAPIRequiresBasePathAndSchema(t *testing.T) {
	req := validInternalReq(t)
	req.AgentType.SubType = strPtr("custom-api")
	v := validateRequest(req)
	assertViolation(t, v, "spec.inputInterface.basePath is required for subtype custom-api")
	assertViolation(t, v, "spec.inputInterface.schema.path is required for subtype custom-api")
}

func TestValidateRequest_CustomAPIRequiresPort(t *testing.T) {
	req := validInternalReq(t)
	req.AgentType.SubType = strPtr("custom-api")
	req.InputInterface.BasePath = strPtr("/v1")
	req.InputInterface.Schema = &amsvc.InputInterfaceSchema{Path: strPtr("/openapi.yaml")}
	req.InputInterface.Port = nil
	v := validateRequest(req)
	assertViolation(t, v, "spec.inputInterface.port is required for subtype custom-api")
}

func TestValidateRequest_CustomAPISchemaPathLeadingSlash(t *testing.T) {
	req := validInternalReq(t)
	req.AgentType.SubType = strPtr("custom-api")
	req.InputInterface.BasePath = strPtr("/v1")
	req.InputInterface.Schema = &amsvc.InputInterfaceSchema{Path: strPtr("openapi.yaml")}
	v := validateRequest(req)
	assertViolation(t, v, `spec.inputInterface.schema.path must start with "/"`)
}

func TestValidateRequest_CustomAPIValid(t *testing.T) {
	req := validInternalReq(t)
	req.AgentType.SubType = strPtr("custom-api")
	req.InputInterface.BasePath = strPtr("/v1")
	req.InputInterface.Schema = &amsvc.InputInterfaceSchema{Path: strPtr("/openapi.yaml")}
	assertNoViolations(t, validateRequest(req))
}

func TestValidateRequest_InterfaceTypeMustBeHTTP(t *testing.T) {
	req := validInternalReq(t)
	req.InputInterface.Type = "grpc"
	v := validateRequest(req)
	assertViolation(t, v, `spec.inputInterface.type must be "HTTP", got "grpc"`)
}

func TestValidateRequest_PortRange(t *testing.T) {
	for _, tt := range []struct {
		port    int
		wantErr bool
	}{{0, true}, {1, false}, {8000, false}, {65535, false}, {65536, true}, {-1, true}} {
		req := validInternalReq(t)
		req.InputInterface.Port = intPtr(tt.port)
		v := validateRequest(req)
		hasPortViolation := false
		for _, s := range v {
			if strings.Contains(s, "spec.inputInterface.port must be 1..65535") {
				hasPortViolation = true
			}
		}
		if hasPortViolation != tt.wantErr {
			t.Errorf("port %d: violation=%v, want %v (all: %v)", tt.port, hasPortViolation, tt.wantErr, v)
		}
	}
}

func TestValidateRequest_EnvKeys(t *testing.T) {
	req := validInternalReq(t)
	req.Configurations = &amsvc.Configurations{
		Env: &[]amsvc.EnvironmentVariable{
			{Key: "GOOD", Value: strPtr("v")},
			{Key: "1BAD", Value: strPtr("v")},
			{Key: "", Value: strPtr("v")},
			{Key: "GOOD", Value: strPtr("dup")},
		},
	}
	v := validateRequest(req)
	assertViolation(t, v, `spec.configurations.env: invalid key "1BAD"`)
	assertViolation(t, v, `spec.configurations.env: invalid key ""`)
	assertViolation(t, v, `spec.configurations.env: duplicate key "GOOD"`)
}

func TestValidateRequest_ModelConfigProviderRequired(t *testing.T) {
	req := validInternalReq(t)
	req.ModelConfig = &[]amsvc.ModelConfigRequest{{}}
	v := validateRequest(req)
	assertViolation(t, v, "spec.modelConfig[0].providerName is required")
}

func TestValidateRequest_ExternalRejectsInternalSections(t *testing.T) {
	req := validInternalReq(t)
	req.Provisioning.Type = amsvc.ProvisioningTypeExternal
	req.AgentType.Type = "external-agent-api"
	req.AgentType.SubType = nil
	req.Configurations = &amsvc.Configurations{}
	req.ModelConfig = &[]amsvc.ModelConfigRequest{{ProviderName: "openai"}}
	v := validateRequest(req)
	assertViolation(t, v, "spec.provisioning.repository is not allowed for external provisioning")
	assertViolation(t, v, "spec.build is not allowed for external provisioning")
	assertViolation(t, v, "spec.inputInterface is not allowed for external provisioning")
	assertViolation(t, v, "spec.configurations is not allowed for external provisioning")
	assertViolation(t, v, "spec.modelConfig is not allowed for external provisioning")
}

func TestValidateRequest_ExternalRejectsAgentKind(t *testing.T) {
	req := validExternalReq()
	req.Provisioning.AgentKind = &struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}{Name: "x", Version: "1"}
	v := validateRequest(req)
	assertViolation(t, v, "spec.provisioning.agentKind is not allowed for external provisioning")
}

// Aggregation: one pass reports across sections.
func TestValidateRequest_Aggregates(t *testing.T) {
	req := amsvc.CreateAgentRequest{
		Provisioning: amsvc.Provisioning{
			Type:       amsvc.ProvisioningTypeInternal,
			Repository: &amsvc.RepositoryConfig{},
		},
	}
	v := validateRequest(req)
	if len(v) < 5 {
		t.Errorf("expected >=5 aggregated violations, got %d: %v", len(v), v)
	}
}
