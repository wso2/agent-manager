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
	"os"
	"strings"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"gopkg.in/yaml.v3"
)

const (
	provisioningInternal = string(amsvc.ProvisioningTypeInternal)
	provisioningExternal = "external" // CLI-only: API spec has no ProvisioningTypeExternal yet.

	buildTypeBuildpack = string(amsvc.Buildpack)
	buildTypeDocker    = string(amsvc.Docker)

	// CLI-only: no generated enum for these values.
	subTypeChatAPI   = "chat-api"
	subTypeCustomAPI = "custom-api"

	agentTypeInternal = "agent-api"
	agentTypeExternal = "external-agent-api"

	interfaceTypeHTTP = "HTTP"
)

func Build(opts *CreateOptions) (amsvc.CreateAgentRequest, error) {
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

	b, err := buildBuild(opts)
	if err != nil {
		return amsvc.CreateAgentRequest{}, err
	}
	req.Build = b
	req.InputInterface = buildInterface(opts)
	req.Configurations = buildConfig(opts)
	req.ModelConfig = opts.modelConfig

	return req, nil
}

func ensureLeadingSlash(s string) string {
	if s != "" && !strings.HasPrefix(s, "/") {
		return "/" + s
	}
	return s
}

func buildProvisioning(opts *CreateOptions) amsvc.Provisioning {
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
}

func buildBuild(opts *CreateOptions) (*amsvc.Build, error) {
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
			return nil, fmt.Errorf("buildpack: %w", err)
		}
	case buildTypeDocker:
		d := amsvc.DockerBuild{
			Type: amsvc.Docker,
			Docker: amsvc.DockerConfig{
				DockerfilePath: opts.Dockerfile,
			},
		}
		if err := b.FromDockerBuild(d); err != nil {
			return nil, fmt.Errorf("docker: %w", err)
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
	if opts.SubType == subTypeChatAPI {
		return iface
	}
	if opts.BasePath != "" {
		bp := ensureLeadingSlash(opts.BasePath)
		iface.BasePath = &bp
	}
	if opts.OpenAPISpec != "" {
		spec := ensureLeadingSlash(opts.OpenAPISpec)
		iface.Schema = &amsvc.InputInterfaceSchema{Path: spec}
	}
	return iface
}

func buildConfig(opts *CreateOptions) *amsvc.Configurations {
	var envs []amsvc.EnvironmentVariable

	for _, entry := range opts.Env {
		k, v := splitEnv(entry)
		envs = append(envs, amsvc.EnvironmentVariable{Key: k, Value: &v})
	}
	for _, entry := range opts.EnvSecret {
		k, v := splitEnv(entry)
		tr := true
		envs = append(envs, amsvc.EnvironmentVariable{Key: k, Value: &v, IsSensitive: &tr})
	}
	for _, entry := range opts.EnvFromSecret {
		k, v := splitEnv(entry)
		envs = append(envs, amsvc.EnvironmentVariable{Key: k, SecretRef: &v})
	}

	hasEnv := len(envs) > 0
	hasInstr := opts.DisableAutoInstrumentation
	if !hasEnv && !hasInstr {
		return nil
	}

	cfg := &amsvc.Configurations{}
	if hasEnv {
		cfg.Env = &envs
	}
	if hasInstr {
		f := false
		cfg.EnableAutoInstrumentation = &f
	}
	return cfg
}

func splitEnv(entry string) (string, string) {
	k, v, _ := strings.Cut(entry, "=")
	return k, v
}

func loadModelConfig(path string) (*[]amsvc.ModelConfigRequest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%v", err)
	}
	// yaml.v3 doesn't honour json struct tags, so we unmarshal into a generic
	// value first, re-encode as JSON, then decode using encoding/json which
	// does honour json tags.
	var raw interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse error: %v", err)
	}
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("re-encode error: %v", err)
	}
	var configs []amsvc.ModelConfigRequest
	if err := json.Unmarshal(jsonBytes, &configs); err != nil {
		return nil, fmt.Errorf("parse error: %v", err)
	}
	return &configs, nil
}
