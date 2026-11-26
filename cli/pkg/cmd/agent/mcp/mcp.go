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

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
)

// NewMCPCmd is the `agent mcp` command group for managing an agent's MCP
// proxy configurations, after the agent is created.
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

// findMCPConfigByName resolves a config name to its UUID via List, considering
// only mcp-typed configs. found is false when no mcp config matches the name.
func findMCPConfigByName(ctx context.Context, client *aigateway.ClientWithResponses, org, proj, agent, name string) (openapi_types.UUID, bool, error) {
	resp, err := client.ListAgentMCPConfigsWithResponse(ctx, agent, &aigateway.ListAgentMCPConfigsParams{Project: proj})
	if err != nil {
		return openapi_types.UUID{}, false, clierr.Newf(clierr.Transport, "%v", err)
	}
	if resp.JSON200 == nil {
		return openapi_types.UUID{}, false, cmdutil.ErrorFromServer(resp.HTTPResponse, resp.JSON500)
	}
	for _, c := range resp.JSON200.List {
		if c.Metadata.Name == name && c.Metadata.Uid != nil {
			return *c.Metadata.Uid, true, nil
		}
	}
	return openapi_types.UUID{}, false, nil
}
