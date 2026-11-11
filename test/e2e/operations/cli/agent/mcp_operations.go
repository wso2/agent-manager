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

// amctl agent mcp commands as thin, assertion-backed operations over the amctl
// harness. Lives in package cliagent alongside llm_operations.go and reuses its
// DeleteResult. Decode structs mirror amsvc.AgentModelConfigResponse /
// AgentModelConfigListItem (only the fields specs assert on). Unlike an llm
// config, an mcp config references a catalog MCP proxy by handle; the server
// mirrors that handle into proxyName/mcpProxyName/providerName, so
// AgentMCPProxyRef.Handle reads them in that order. Note that `agent mcp unset`
// requires --yes for a whole-config delete (the llm equivalent does not).
package cliagent

import (
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework/amctl"
)

// AgentMCPConfig is the data shape of `agent mcp set`/`get` (AgentModelConfigResponse).
type AgentMCPConfig struct {
	Name        string                        `json:"name"`
	Type        string                        `json:"type"`
	Description string                        `json:"description"`
	EnvMappings map[string]AgentMCPEnvMapping `json:"envMappings"`
}

// AgentMCPEnvMapping is one entry of AgentModelConfigResponse.EnvMappings.
// Configuration is server-managed and may be nil for an undeployed proxy.
type AgentMCPEnvMapping struct {
	Configuration *AgentMCPProxyRef `json:"configuration,omitempty"`
}

// AgentMCPProxyRef is the subset of ProviderConfig we assert on. The server
// mirrors the bound MCP proxy handle into all three fields; Handle prefers the
// proxy-specific names and falls back to providerName.
type AgentMCPProxyRef struct {
	ProxyName    string `json:"proxyName,omitempty"`
	McpProxyName string `json:"mcpProxyName,omitempty"`
	ProviderName string `json:"providerName,omitempty"`
}

// Handle returns the bound MCP proxy handle (proxyName → mcpProxyName → providerName).
func (r AgentMCPProxyRef) Handle() string {
	if r.ProxyName != "" {
		return r.ProxyName
	}
	if r.McpProxyName != "" {
		return r.McpProxyName
	}
	return r.ProviderName
}

// AgentMCPConfigList is the data shape of `agent mcp list --json` (CLI's ListResult).
type AgentMCPConfigList struct {
	Configs []AgentMCPConfig `json:"configs"`
}

// Names returns the config names in the list, for membership assertions.
func (l AgentMCPConfigList) Names() []string {
	names := make([]string, 0, len(l.Configs))
	for _, c := range l.Configs {
		names = append(names, c.Name)
	}
	return names
}

// SetAgentMCPParams are the inputs to `agent mcp set`.
type SetAgentMCPParams struct {
	Name        string
	Env         string
	Proxy       string
	URLEnv      string
	APIKeyEnv   string
	Description string
}

// SetAgentMCP runs `agent mcp set <agent>` (create or update) and returns the config.
func SetAgentMCP(g Gomega, h *amctl.Harness, org, project, agent string, p SetAgentMCPParams) AgentMCPConfig {
	args := []string{"agent", "mcp", "set", agent, "--name", p.Name, "--proxy", p.Proxy}
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
	return amctl.DecodeData[AgentMCPConfig](g, h.Run(args...))
}

// GetAgentMCP runs `agent mcp get <agent> --name` and returns the config.
func GetAgentMCP(g Gomega, h *amctl.Harness, org, project, agent, name string) AgentMCPConfig {
	return amctl.DecodeData[AgentMCPConfig](g, h.Run(
		"agent", "mcp", "get", agent, "--name", name,
		"--org", org, "--project", project, "--json"))
}

// GetAgentMCPExpectError runs `agent mcp get` expecting a non-zero exit (e.g. the
// config no longer exists) and returns the error envelope — the deletion check.
func GetAgentMCPExpectError(g Gomega, h *amctl.Harness, org, project, agent, name string) amctl.EnvelopeError {
	return h.Run("agent", "mcp", "get", agent, "--name", name,
		"--org", org, "--project", project, "--json").ExpectError(g)
}

// ListAgentMCP runs `agent mcp list <agent>` and returns the (type=mcp) configs.
func ListAgentMCP(g Gomega, h *amctl.Harness, org, project, agent string) AgentMCPConfigList {
	return amctl.DecodeData[AgentMCPConfigList](g, h.Run(
		"agent", "mcp", "list", agent,
		"--org", org, "--project", project, "--json"))
}

// UnsetAgentMCP runs `agent mcp unset <agent> --name --yes` (whole-config delete)
// and returns the delete result. --yes is required because the harness runs with
// a non-terminal stdin and the command refuses an unconfirmed whole-config delete.
func UnsetAgentMCP(g Gomega, h *amctl.Harness, org, project, agent, name string) DeleteResult {
	return amctl.DecodeData[DeleteResult](g, h.Run(
		"agent", "mcp", "unset", agent, "--name", name, "--yes",
		"--org", org, "--project", project, "--json"))
}
