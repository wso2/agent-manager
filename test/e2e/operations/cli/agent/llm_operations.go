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

// amctl agent llm commands as thin, assertion-backed operations over the amctl
// harness. Lives in package cliagent alongside agent_operations.go and reuses
// its DeleteResult. Decode structs mirror amsvc.AgentModelConfigResponse /
// AgentModelConfigListItem (only the fields specs assert on).
package cliagent

import (
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework/amctl"
)

// AgentLLMConfig is the data shape of `agent llm set`/`get` (AgentModelConfigResponse).
type AgentLLMConfig struct {
	Name        string                        `json:"name"`
	Type        string                        `json:"type"`
	Description string                        `json:"description"`
	EnvMappings map[string]AgentLLMEnvMapping `json:"envMappings"`
}

// AgentLLMEnvMapping is one entry of AgentModelConfigResponse.EnvMappings.
// Configuration is server-managed and may be nil for an undeployed provider.
type AgentLLMEnvMapping struct {
	Configuration *AgentLLMProviderRef `json:"configuration,omitempty"`
}

// AgentLLMProviderRef is the subset of ProviderConfig we assert on.
type AgentLLMProviderRef struct {
	ProviderName string `json:"providerName"`
}

// AgentLLMConfigList is the data shape of `agent llm list --json` (CLI's ListResult).
type AgentLLMConfigList struct {
	Configs []AgentLLMConfig `json:"configs"`
}

// SetAgentLLMParams are the inputs to `agent llm set`.
type SetAgentLLMParams struct {
	Name        string
	Env         string
	Provider    string
	URLEnv      string
	APIKeyEnv   string
	Description string
}

// SetAgentLLM runs `agent llm set <agent>` (create or update) and returns the config.
func SetAgentLLM(g Gomega, h *amctl.Harness, org, project, agent string, p SetAgentLLMParams) AgentLLMConfig {
	args := []string{"agent", "llm", "set", agent, "--name", p.Name, "--provider", p.Provider}
	if p.URLEnv != "" {
		args = append(args, "--url-env", p.URLEnv)
	}
	if p.APIKeyEnv != "" {
		args = append(args, "--apikey-env", p.APIKeyEnv)
	}
	if p.Description != "" {
		args = append(args, "--description", p.Description)
	}
	args = append(args, "--org", org, "--project", project, "--json")
	return amctl.DecodeData[AgentLLMConfig](g, h.Run(args...))
}

// GetAgentLLM runs `agent llm get <agent> --name` and returns the config.
func GetAgentLLM(g Gomega, h *amctl.Harness, org, project, agent, name string) AgentLLMConfig {
	return amctl.DecodeData[AgentLLMConfig](g, h.Run(
		"agent", "llm", "get", agent, "--name", name,
		"--org", org, "--project", project, "--json"))
}

// GetAgentLLMExpectError runs `agent llm get` expecting a non-zero exit (e.g. the
// config no longer exists) and returns the error envelope — the deletion check.
func GetAgentLLMExpectError(g Gomega, h *amctl.Harness, org, project, agent, name string) amctl.EnvelopeError {
	return h.Run("agent", "llm", "get", agent, "--name", name,
		"--org", org, "--project", project, "--json").ExpectError(g)
}

// ListAgentLLM runs `agent llm list <agent>` and returns the (type=llm) configs.
func ListAgentLLM(g Gomega, h *amctl.Harness, org, project, agent string) AgentLLMConfigList {
	return amctl.DecodeData[AgentLLMConfigList](g, h.Run(
		"agent", "llm", "list", agent,
		"--org", org, "--project", project, "--json"))
}

// UnsetAgentLLM runs `agent llm unset <agent> --name --yes` (whole-config
// delete) and returns the delete result (reusing the package's DeleteResult).
// A whole-config unset (no --env) prompts for confirmation, so --yes is
// required under the non-terminal harness.
func UnsetAgentLLM(g Gomega, h *amctl.Harness, org, project, agent, name string) DeleteResult {
	return amctl.DecodeData[DeleteResult](g, h.Run(
		"agent", "llm", "unset", agent, "--name", name,
		"--org", org, "--project", project, "--yes", "--json"))
}

// Names returns the config names in the list, for membership assertions.
func (l AgentLLMConfigList) Names() []string {
	names := make([]string, 0, len(l.Configs))
	for _, c := range l.Configs {
		names = append(names, c.Name)
	}
	return names
}
