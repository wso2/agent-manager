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

package mcp

import (
	"context"

	"github.com/spf13/cobra"

	openapi_types "github.com/oapi-codegen/runtime/types"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmd/agent/modelconfig"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
)

// NewMCPCmd is the `agent mcp` command group for managing an agent's MCP
// proxy configurations per environment, after the agent is created.
func NewMCPCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Manage an agent's MCP proxy configurations",
	}
	cmd.AddCommand(NewListCmd(f))
	cmd.AddCommand(NewGetCmd(f))
	cmd.AddCommand(NewSetCmd(f))
	cmd.AddCommand(NewUnsetCmd(f))
	return cmd
}

// configType is the model-config Type value this command group manages. The
// mcp-configs endpoint shares its response shape with model-configs across
// config types (llm/mcp/other), so every read filters to this value.
const configType = "mcp"

// proxyNameFromConfig extracts the MCP proxy handle from a GET-response env
// mapping. The server mirrors the proxy handle into providerName and the
// proxy*-named fields for MCP configs; this prefers the proxy-specific fields
// and falls back to providerName.
func proxyNameFromConfig(c *amsvc.ProviderConfig) string {
	if c == nil {
		return ""
	}
	if c.ProxyName != nil && *c.ProxyName != "" {
		return *c.ProxyName
	}
	if c.McpProxyName != nil && *c.McpProxyName != "" {
		return *c.McpProxyName
	}
	return c.ProviderName
}

// toEnvModelConfigRequest translates a GET-response env mapping into the shape
// the PUT/POST request body expects. Server-managed fields (URL, status, auth
// info, proxy UUID) are dropped; only the proxy handle and policies survive.
func toEnvModelConfigRequest(m amsvc.EnvProviderConfigMappings) amsvc.EnvModelConfigRequest {
	req := amsvc.EnvModelConfigRequest{}
	if m.Configuration != nil {
		req.ProxyName = modelconfig.StringPtr(proxyNameFromConfig(m.Configuration))
		req.Configuration = amsvc.EnvProviderConfiguration{Policies: m.Configuration.Policies}
	}
	return req
}

// findMCPConfigByName resolves a config name to its UUID via List, considering
// only mcp-typed configs. found is false when no mcp config matches the name.
func findMCPConfigByName(ctx context.Context, client *amsvc.ClientWithResponses, org, proj, agent, name string) (openapi_types.UUID, bool, error) {
	resp, err := client.ListAgentMCPConfigsWithResponse(ctx, org, proj, agent, &amsvc.ListAgentMCPConfigsParams{})
	if err != nil {
		return openapi_types.UUID{}, false, clierr.Newf(clierr.Transport, "%v", err)
	}
	if resp.JSON200 == nil {
		return openapi_types.UUID{}, false, cmdutil.ErrorFromServer(resp.HTTPResponse, resp.JSON500)
	}
	for _, c := range resp.JSON200.Configs {
		if c.Type == configType && c.Name == name {
			return c.Uuid, true, nil
		}
	}
	return openapi_types.UUID{}, false, nil
}
