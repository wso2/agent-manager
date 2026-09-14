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
	"fmt"
	"strings"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
)

const (
	provisioningInternal = string(amsvc.ProvisioningTypeInternal)
	provisioningExternal = string(amsvc.ProvisioningTypeExternal)

	buildTypeBuildpack = string(amsvc.Buildpack)
	buildTypeDocker    = string(amsvc.Docker)

	// CLI-only: no generated enum for these values.
	subTypeChatAPI   = "chat-api"
	subTypeCustomAPI = "custom-api"
	// subTypeA2A is an agent that speaks the A2A protocol. It declares a port
	// like a custom-api agent but carries no OpenAPI document: it serves its own
	// agent card, and the platform stores no copy.
	subTypeA2A = "a2a-agent"

	agentTypeInternal = "agent-api"
	agentTypeExternal = "external-agent-api"

	// The server exempts only Ballerina from languageVersion/runCommand.
	langBallerina = "ballerina"

	interfaceTypeHTTP = "HTTP"
)

// Build converts flag input into the API request type. It is total: it never
// hard-fails. Inputs that cannot be represented in the request — or that the
// builder would otherwise silently drop — are returned as flag-phrased
// violations; everything else passes through and is judged by validateRequest.
func Build(opts *CreateOptions) (amsvc.CreateAgentRequest, []string) {
	var v []string

	agentType := opts.Type
	if agentType == "" {
		switch opts.Provisioning {
		case provisioningExternal:
			agentType = agentTypeExternal
		default:
			agentType = agentTypeInternal
		}
	}

	req := amsvc.CreateAgentRequest{
		Name:        opts.Name,
		DisplayName: opts.DisplayName,
		AgentType: &amsvc.AgentType{
			Type: agentType,
		},
		Provisioning: buildProvisioning(opts),
	}

	if opts.Description != "" {
		req.Description = &opts.Description
	}
	if opts.SubType != "" {
		req.AgentType.SubType = &opts.SubType
	}

	switch opts.Provisioning {
	case provisioningExternal:
		return req, append(v, droppedExternalFlags(opts)...)
	case provisioningInternal, "":
	default:
		// Unknown provisioning passes through for validateRequest to report;
		// no internal sections can be built for it.
		return req, v
	}

	b, bv := buildBuild(opts)
	v = append(v, bv...)
	req.Build = b
	req.InputInterface = buildInterface(opts)
	cfg, cv := buildConfig(opts)
	v = append(v, cv...)
	req.Configurations = cfg
	req.ModelConfig = buildModelConfig(opts)

	return req, append(v, droppedInternalFlags(opts)...)
}

func ensureLeadingSlash(s string) string {
	if s != "" && !strings.HasPrefix(s, "/") {
		return "/" + s
	}
	return s
}

func buildProvisioning(opts *CreateOptions) amsvc.Provisioning {
	switch opts.Provisioning {
	case provisioningExternal:
		return amsvc.Provisioning{Type: amsvc.ProvisioningTypeExternal}
	case provisioningInternal, "":
		repo := &amsvc.RepositoryConfig{
			Url:     opts.RepoURL,
			Branch:  opts.RepoBranch,
			AppPath: ensureLeadingSlash(opts.RepoPath),
		}
		if opts.RepoSecret != "" {
			repo.SecretRef = &opts.RepoSecret
		}
		return amsvc.Provisioning{
			Type:       amsvc.ProvisioningTypeInternal,
			Repository: repo,
		}
	default:
		return amsvc.Provisioning{Type: amsvc.ProvisioningType(opts.Provisioning)}
	}
}

func buildBuild(opts *CreateOptions) (*amsvc.Build, []string) {
	var b amsvc.Build
	switch opts.BuildType {
	case buildTypeBuildpack:
		bp := amsvc.BuildpackBuild{
			Type: amsvc.Buildpack,
			Buildpack: amsvc.BuildpackConfig{
				Language: strings.ToLower(opts.Language),
			},
		}
		if opts.LanguageVersion != "" {
			bp.Buildpack.LanguageVersion = &opts.LanguageVersion
		}
		if opts.RunCommand != "" {
			bp.Buildpack.RunCommand = &opts.RunCommand
		}
		if err := b.FromBuildpackBuild(bp); err != nil {
			return nil, []string{fmt.Sprintf("--build-type=buildpack: %v", err)}
		}
	case buildTypeDocker:
		d := amsvc.DockerBuild{
			Type: amsvc.Docker,
			Docker: amsvc.DockerConfig{
				DockerfilePath: ensureLeadingSlash(opts.Dockerfile),
			},
		}
		if err := b.FromDockerBuild(d); err != nil {
			return nil, []string{fmt.Sprintf("--build-type=docker: %v", err)}
		}
	case "":
		return nil, nil
	default:
		// Unknown build type survives as the raw union discriminator so
		// validateRequest reports it alongside everything else.
		raw, err := json.Marshal(map[string]string{"type": opts.BuildType})
		if err != nil {
			return nil, []string{fmt.Sprintf("--build-type: %v", err)}
		}
		if err := b.UnmarshalJSON(raw); err != nil {
			return nil, []string{fmt.Sprintf("--build-type: %v", err)}
		}
	}
	return &b, nil
}

func buildInterface(opts *CreateOptions) *amsvc.InputInterface {
	port := opts.Port
	iface := &amsvc.InputInterface{
		Type: interfaceTypeHTTP,
		Port: &port,
	}
	// chat-api's port and paths are fixed by the platform. custom-api and
	// a2a-agent both carry their own; only custom-api adds a schema.
	if opts.SubType == subTypeChatAPI {
		return iface
	}
	if opts.BasePath != "" {
		bp := ensureLeadingSlash(opts.BasePath)
		iface.BasePath = &bp
	}
	if opts.OpenAPISpec != "" {
		spec := ensureLeadingSlash(opts.OpenAPISpec)
		iface.Schema = &amsvc.InputInterfaceSchema{Path: &spec}
	}
	return iface
}

func buildConfig(opts *CreateOptions) (*amsvc.Configurations, []string) {
	var envs []amsvc.EnvironmentVariable
	var v []string

	appendEnvs := func(entries []string, flag string, mk func(k, val string) amsvc.EnvironmentVariable) {
		for _, entry := range entries {
			k, val, ok := strings.Cut(entry, "=")
			if !ok {
				v = append(v, fmt.Sprintf("%s %q: missing '=' separator", flag, entry))
				continue
			}
			envs = append(envs, mk(k, val))
		}
	}
	appendEnvs(opts.Env, "--env", func(k, val string) amsvc.EnvironmentVariable {
		return amsvc.EnvironmentVariable{Key: k, Value: &val}
	})
	appendEnvs(opts.EnvSecret, "--env-secret", func(k, val string) amsvc.EnvironmentVariable {
		tr := true
		return amsvc.EnvironmentVariable{Key: k, Value: &val, IsSensitive: &tr}
	})
	appendEnvs(opts.EnvFromSecret, "--env-from-secret", func(k, val string) amsvc.EnvironmentVariable {
		return amsvc.EnvironmentVariable{Key: k, SecretRef: &val}
	})

	hasEnv := len(envs) > 0
	hasInstr := opts.DisableAutoInstrumentation
	if !hasEnv && !hasInstr {
		return nil, v
	}

	cfg := &amsvc.Configurations{}
	if hasEnv {
		cfg.Env = &envs
	}
	if hasInstr {
		f := false
		cfg.EnableAutoInstrumentation = &f
	}
	return cfg, v
}

func buildModelConfig(opts *CreateOptions) *[]amsvc.ModelConfigRequest {
	if opts.LLMProvider == "" {
		return nil
	}
	mc := amsvc.ModelConfigRequest{ProviderName: opts.LLMProvider}
	var evs []amsvc.EnvironmentVariableConfig
	if opts.LLMURLEnv != "" {
		evs = append(evs, amsvc.EnvironmentVariableConfig{Key: "url", Name: opts.LLMURLEnv})
	}
	if opts.LLMAPIKeyEnv != "" {
		evs = append(evs, amsvc.EnvironmentVariableConfig{Key: "apikey", Name: opts.LLMAPIKeyEnv})
	}
	if len(evs) > 0 {
		mc.EnvironmentVariables = &evs
	}
	return &[]amsvc.ModelConfigRequest{mc}
}

// droppedInternalFlags reports internal-mode flag input the builder dropped:
// chat-api never carries base-path/openapi-spec (and the port is fixed), the
// build union holds exactly one variant's fields, and LLM env names without a
// provider never reach modelConfig.
func droppedInternalFlags(opts *CreateOptions) []string {
	var v []string

	if opts.SubType == subTypeChatAPI {
		if opts.PortSet {
			v = append(v, "--port is not allowed for subtype chat-api")
		}
		if opts.BasePath != "" {
			v = append(v, "--base-path is not allowed for subtype chat-api")
		}
		if opts.OpenAPISpec != "" {
			v = append(v, "--openapi-spec is not allowed for subtype chat-api")
		}
	}

	if opts.SubType == subTypeA2A && opts.OpenAPISpec != "" {
		v = append(v, "--openapi-spec is not allowed for subtype a2a-agent")
	}

	switch opts.BuildType {
	case buildTypeBuildpack:
		if opts.Dockerfile != "" {
			v = append(v, "--build-type=buildpack conflicts with --dockerfile")
		}
	case buildTypeDocker:
		if opts.Language != "" {
			v = append(v, "--build-type=docker conflicts with --language")
		}
		if opts.LanguageVersion != "" {
			v = append(v, "--build-type=docker conflicts with --language-version")
		}
		if opts.RunCommand != "" {
			v = append(v, "--build-type=docker conflicts with --run-command")
		}
	}

	if (opts.LLMURLEnv != "" || opts.LLMAPIKeyEnv != "") && opts.LLMProvider == "" {
		v = append(v, "--llm-url-env/--llm-api-key-env require --llm-provider")
	}

	return v
}

// droppedExternalFlags reports internal-only flag input that the builder
// drops entirely for external provisioning.
func droppedExternalFlags(opts *CreateOptions) []string {
	var v []string
	disallow := func(flag string, set bool) {
		if set {
			v = append(v, flag+" is not allowed for external provisioning")
		}
	}

	disallow("--subtype", opts.SubType != "")
	disallow("--repo-url", opts.RepoURL != "")
	disallow("--repo-branch", opts.RepoBranch != "")
	disallow("--repo-path", opts.RepoPath != "")
	disallow("--repo-secret", opts.RepoSecret != "")
	disallow("--build-type", opts.BuildType != "")
	disallow("--language", opts.Language != "")
	disallow("--language-version", opts.LanguageVersion != "")
	disallow("--run-command", opts.RunCommand != "")
	disallow("--dockerfile", opts.Dockerfile != "")
	// PortSet (not Port != 0) — the --port flag defaults to 8000.
	disallow("--port", opts.PortSet)
	disallow("--base-path", opts.BasePath != "")
	disallow("--openapi-spec", opts.OpenAPISpec != "")
	disallow("--env", len(opts.Env) > 0)
	disallow("--env-secret", len(opts.EnvSecret) > 0)
	disallow("--env-from-secret", len(opts.EnvFromSecret) > 0)
	disallow("--no-auto-instrumentation", opts.DisableAutoInstrumentation)
	disallow("--llm-provider", opts.LLMProvider != "")
	disallow("--llm-url-env", opts.LLMURLEnv != "")
	disallow("--llm-api-key-env", opts.LLMAPIKeyEnv != "")

	return v
}
