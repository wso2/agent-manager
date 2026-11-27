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
	req.Extensions = buildGatewayExtension(opts)

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

// buildGatewayExtension attaches the LLM provider through the AI Gateway
// module's extension block (extensions.ai-gateway.modelConfig).
func buildGatewayExtension(opts *CreateOptions) *amsvc.Extensions {
	if opts.LLMProvider == "" {
		return nil
	}
	mc := map[string]any{"providerName": opts.LLMProvider}
	var evs []map[string]string
	if opts.LLMURLEnv != "" {
		evs = append(evs, map[string]string{"key": "url", "name": opts.LLMURLEnv})
	}
	if opts.LLMAPIKeyEnv != "" {
		evs = append(evs, map[string]string{"key": "apikey", "name": opts.LLMAPIKeyEnv})
	}
	if len(evs) > 0 {
		mc["environmentVariables"] = evs
	}
	return &amsvc.Extensions{"ai-gateway": {"modelConfig": []any{mc}}}
}

// droppedInternalFlags reports internal-mode flag input the builder dropped:
// chat-api never carries base-path/openapi-spec (and the port is fixed), the
// build union holds exactly one variant's fields, and LLM env names without a
// provider never reach the extension block.
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

	if opts.SubType != "" {
		v = append(v, "--subtype is not allowed for external provisioning; use --card-url to register an A2A agent card")
	}
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
	disallow("--llm-provider", opts.LLMProvider != "")
	disallow("--llm-url-env", opts.LLMURLEnv != "")
	disallow("--llm-api-key-env", opts.LLMAPIKeyEnv != "")

	return v
}

// agentDocument wraps a create request in the Agent document the server takes.
func agentDocument(project string, req amsvc.CreateAgentRequest) amsvc.AgentDocument {
	doc := amsvc.AgentDocument{
		ApiVersion: amsvc.AgentDocumentApiVersionAiControlPlaneChoreolabsComv1,
		Kind:       amsvc.AgentDocumentKindAgent,
		Metadata:   amsvc.ProjectScopedDocumentMetadata{Name: req.Name, Project: project},
		Spec: amsvc.AgentSpec{
			DisplayName:  req.DisplayName,
			Description:  req.Description,
			Provisioning: amsvc.AgentProvisioning{Type: amsvc.AgentProvisioningKind(req.Provisioning.Type)},
			Labels:       req.Labels,
			Extensions:   req.Extensions,
		},
	}
	if req.AgentType != nil {
		doc.Spec.AgentType = *req.AgentType
	}
	return doc
}
