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
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
)

var envKeyRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// contentFlags carry request content. They are owned by flag mode: --template
// takes no input at all, and with --file the manifest is the source of truth.
// Contextual flags (--org, --project, --json) stay allowed in every mode.
var contentFlags = []string{
	"display-name", "description", "provisioning", "subtype", "type",
	"repo-url", "repo-branch", "repo-path", "repo-secret",
	"build-type", "language", "language-version", "run-command", "dockerfile",
	"port", "base-path", "openapi-spec", "no-auto-instrumentation",
	"env", "env-secret", "env-from-secret",
	"llm-provider", "llm-url-env", "llm-api-key-env",
}

// changedContentFlags uses Changed (not zero-value checks) because --port
// defaults to 8000.
func changedContentFlags(cmd *cobra.Command) []string {
	var changed []string
	for _, name := range contentFlags {
		if cmd.Flags().Changed(name) {
			changed = append(changed, name)
		}
	}
	return changed
}

func validateTemplateMode(cmd *cobra.Command, args []string) error {
	var v []string
	if len(args) > 0 {
		v = append(v, "name argument is not allowed with --template")
	}
	if cmd.Flags().Changed("file") {
		v = append(v, "--file is not allowed with --template")
	}
	if cmd.Flags().Changed("json") {
		v = append(v, "--json is not allowed with --template (the template is YAML)")
	}
	for _, name := range changedContentFlags(cmd) {
		v = append(v, "--"+name+" is not allowed with --template")
	}
	if len(v) == 0 {
		return nil
	}
	return cmdutil.FlagErrors(v)
}

func validateFileMode(cmd *cobra.Command, args []string) error {
	var v []string
	if len(args) > 0 {
		v = append(v, "name argument is not allowed with --file (set spec.name in the manifest)")
	}
	for _, name := range changedContentFlags(cmd) {
		v = append(v, "--"+name+" is not allowed with --file (the manifest is the source of truth)")
	}
	if len(v) == 0 {
		return nil
	}
	return cmdutil.FlagErrors(v)
}

// prepare is the flag-mode entry point: convert the flags into the API
// request type, then validate the request. Conversion-layer violations
// (flag input that cannot survive the conversion) and request-layer
// violations are aggregated into a single error.
func prepare(opts *CreateOptions) (amsvc.CreateAgentRequest, error) {
	req, v := Build(opts)
	v = append(v, validateRequest(req)...)
	if len(v) > 0 {
		return req, cmdutil.FlagErrorsHeader("invalid agent spec", v)
	}
	return req, nil
}

// validateRequest is the shared semantic rule set for both flag mode and file
// mode, defined against the request type the server receives. Messages are
// phrased as manifest field paths. Coverage is structural (required fields,
// enum validity, variant shape); server-owned business rules (name format,
// repo URL format, supported languages) are deliberately not mirrored.
func validateRequest(req amsvc.CreateAgentRequest) []string {
	var v []string

	if req.Name == "" {
		v = append(v, "spec.name is required")
	}
	if req.DisplayName == "" {
		v = append(v, "spec.displayName is required")
	}

	switch req.Provisioning.Type {
	case amsvc.ProvisioningTypeInternal:
		v = append(v, internalRequestViolations(req)...)
	case amsvc.ProvisioningTypeExternal:
		v = append(v, externalRequestViolations(req)...)
	case "":
		v = append(v, "spec.provisioning.type is required")
	default:
		v = append(v, fmt.Sprintf("spec.provisioning.type must be %q or %q, got %q", provisioningInternal, provisioningExternal, req.Provisioning.Type))
	}

	return v
}

func internalRequestViolations(req amsvc.CreateAgentRequest) []string {
	var v []string

	if req.AgentType != nil && req.AgentType.Type != "" && req.AgentType.Type != agentTypeInternal {
		v = append(v, fmt.Sprintf("spec.agentType.type must be %q for internal provisioning, got %q", agentTypeInternal, req.AgentType.Type))
	}

	hasRepo := req.Provisioning.Repository != nil
	hasKind := req.Provisioning.AgentKind != nil
	if hasRepo == hasKind {
		v = append(v, "spec.provisioning requires exactly one of repository or agentKind for internal provisioning")
	}
	if hasRepo {
		repo := req.Provisioning.Repository
		if repo.Url == "" {
			v = append(v, "spec.provisioning.repository.url is required")
		}
		if repo.Branch == "" {
			v = append(v, "spec.provisioning.repository.branch is required")
		}
		if repo.AppPath == "" {
			v = append(v, "spec.provisioning.repository.appPath is required")
		} else if !strings.HasPrefix(repo.AppPath, "/") {
			v = append(v, `spec.provisioning.repository.appPath must start with "/"`)
		}
		if req.Build == nil {
			v = append(v, "spec.build is required for internal provisioning")
		}
		// Only agentKind-sourced agents may omit agentType and inputInterface
		// (the server enriches them from the kind); source agents need both.
		if req.AgentType == nil || req.AgentType.Type == "" {
			v = append(v, fmt.Sprintf("spec.agentType.type is required for internal provisioning (%s)", agentTypeInternal))
		}
		if req.InputInterface == nil {
			v = append(v, "spec.inputInterface is required for internal provisioning")
		} else if req.InputInterface.Type == "" {
			v = append(v, fmt.Sprintf("spec.inputInterface.type is required (must be %q)", interfaceTypeHTTP))
		}
	}
	if hasKind {
		kind := req.Provisioning.AgentKind
		if kind.Name == "" {
			v = append(v, "spec.provisioning.agentKind.name is required")
		}
		if kind.Version == "" {
			v = append(v, "spec.provisioning.agentKind.version is required")
		}
	}

	if req.Build != nil {
		v = append(v, buildViolations(req.Build)...)
	}

	subType := ""
	if req.AgentType != nil && req.AgentType.SubType != nil {
		subType = *req.AgentType.SubType
	}
	switch subType {
	case subTypeChatAPI:
	case subTypeCustomAPI:
		if req.InputInterface == nil || req.InputInterface.BasePath == nil || *req.InputInterface.BasePath == "" {
			v = append(v, "spec.inputInterface.basePath is required for subtype custom-api")
		}
		switch {
		case req.InputInterface == nil || req.InputInterface.Schema == nil || req.InputInterface.Schema.Path == nil || *req.InputInterface.Schema.Path == "":
			v = append(v, "spec.inputInterface.schema.path is required for subtype custom-api")
		case !strings.HasPrefix(*req.InputInterface.Schema.Path, "/"):
			v = append(v, `spec.inputInterface.schema.path must start with "/"`)
		}
		if req.InputInterface != nil && req.InputInterface.Port == nil {
			v = append(v, "spec.inputInterface.port is required for subtype custom-api")
		}
	case subTypeA2A:
		// No schema and no base path: an a2a agent serves its own agent card at a
		// well-known path, and the gateway derives its upstream from the endpoint
		// this port declares.
		if req.InputInterface == nil || req.InputInterface.Port == nil {
			v = append(v, "spec.inputInterface.port is required for subtype a2a-agent")
		}
	case "":
		if hasRepo {
			v = append(v, "spec.agentType.subType is required for internal provisioning (chat-api, custom-api or a2a-agent)")
		}
	default:
		v = append(v, fmt.Sprintf("spec.agentType.subType must be %q, %q or %q, got %q",
			subTypeChatAPI, subTypeCustomAPI, subTypeA2A, subType))
	}

	if iface := req.InputInterface; iface != nil {
		if iface.Type != "" && iface.Type != interfaceTypeHTTP {
			v = append(v, fmt.Sprintf("spec.inputInterface.type must be %q, got %q", interfaceTypeHTTP, iface.Type))
		}
		if iface.Port != nil && (*iface.Port < 1 || *iface.Port > 65535) {
			v = append(v, fmt.Sprintf("spec.inputInterface.port must be 1..65535, got %d", *iface.Port))
		}
	}

	if req.Configurations != nil && req.Configurations.Env != nil {
		v = append(v, envViolations(*req.Configurations.Env)...)
	}

	if req.ModelConfig != nil {
		for i, mc := range *req.ModelConfig {
			if mc.ProviderName == "" {
				v = append(v, fmt.Sprintf("spec.modelConfig[%d].providerName is required", i))
			}
		}
	}

	return v
}

func buildViolations(b *amsvc.Build) []string {
	disc, err := b.Discriminator()
	if err != nil {
		disc = ""
	}
	switch disc {
	case buildTypeBuildpack:
		bp, err := b.AsBuildpackBuild()
		if err != nil {
			return nil
		}
		cfg := bp.Buildpack
		if cfg.Language == "" {
			return []string{"spec.build.buildpack.language is required"}
		}
		// Ballerina is the only language the server exempts from
		// languageVersion and runCommand.
		var v []string
		if !strings.EqualFold(cfg.Language, langBallerina) {
			if cfg.LanguageVersion == nil || *cfg.LanguageVersion == "" {
				v = append(v, fmt.Sprintf("spec.build.buildpack.languageVersion is required for language %q", cfg.Language))
			}
			if cfg.RunCommand == nil || *cfg.RunCommand == "" {
				v = append(v, fmt.Sprintf("spec.build.buildpack.runCommand is required for language %q", cfg.Language))
			}
		}
		return v
	case buildTypeDocker:
		d, err := b.AsDockerBuild()
		if err != nil {
			return nil
		}
		if d.Docker.DockerfilePath == "" {
			return []string{"spec.build.docker.dockerfilePath is required"}
		}
		if !strings.HasPrefix(d.Docker.DockerfilePath, "/") {
			return []string{`spec.build.docker.dockerfilePath must start with "/"`}
		}
	default:
		return []string{fmt.Sprintf("spec.build.type must be %q or %q, got %q", buildTypeBuildpack, buildTypeDocker, disc)}
	}
	return nil
}

func envViolations(envs []amsvc.EnvironmentVariable) []string {
	var v []string
	seen := map[string]bool{}
	for _, e := range envs {
		if !envKeyRE.MatchString(e.Key) {
			v = append(v, fmt.Sprintf("spec.configurations.env: invalid key %q (must match [A-Za-z_][A-Za-z0-9_]*)", e.Key))
			continue
		}
		if seen[e.Key] {
			v = append(v, fmt.Sprintf("spec.configurations.env: duplicate key %q", e.Key))
			continue
		}
		seen[e.Key] = true
	}
	return v
}

func externalRequestViolations(req amsvc.CreateAgentRequest) []string {
	var v []string

	if req.AgentType == nil || req.AgentType.Type == "" {
		v = append(v, fmt.Sprintf("spec.agentType.type is required for external provisioning (%s)", agentTypeExternal))
	} else if req.AgentType.Type != agentTypeExternal {
		v = append(v, fmt.Sprintf("spec.agentType.type must be %q for external provisioning, got %q", agentTypeExternal, req.AgentType.Type))
	}

	disallow := func(path string, present bool) {
		if present {
			v = append(v, path+" is not allowed for external provisioning")
		}
	}
	disallow("spec.provisioning.repository", req.Provisioning.Repository != nil)
	disallow("spec.provisioning.agentKind", req.Provisioning.AgentKind != nil)
	disallow("spec.build", req.Build != nil)
	disallow("spec.inputInterface", req.InputInterface != nil)
	disallow("spec.configurations", req.Configurations != nil)
	disallow("spec.modelConfig", req.ModelConfig != nil)

	return v
}
