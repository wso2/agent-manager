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
	"fmt"

	"github.com/spf13/cobra"

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
	"github.com/wso2/agent-manager/cli/pkg/tableprinter"
)

type GetOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*aigateway.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj, agent string) render.Scope
	ResolveAgent func([]string) (string, []string, error)

	Org       string
	Proj      string
	Scope     render.Scope
	AgentName string
	Name      string
}

func NewGetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &GetOptions{
		IO:           f.IOStreams,
		Client:       f.AIGateway,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.AgentScope,
		ResolveAgent: f.ResolveAgent,
	}

	cmd := &cobra.Command{
		Use:   "get [agent] --name <config>",
		Short: "Show an agent's MCP proxy configuration",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.ResolveScope(cmd, true, true)
			if err != nil {
				return render.Error(opts.IO, opts.MakeScope(org, proj, ""), err)
			}
			agent, _, agentErr := opts.ResolveAgent(args)
			scope := opts.MakeScope(org, proj, agent)
			if agentErr != nil {
				return render.Error(opts.IO, scope, agentErr)
			}
			opts.Org, opts.Proj, opts.Scope = org, proj, scope
			opts.AgentName = agent
			return runGet(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Name, "name", "", "Name of the MCP configuration (required)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func runGet(ctx context.Context, o *GetOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	id, found, err := findMCPConfigByName(ctx, client, o.Org, o.Proj, o.AgentName, o.Name)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if !found {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.NotFound, "no MCP configuration named %q for agent %q", o.Name, o.AgentName))
	}

	resp, err := client.GetAgentMCPConfigWithResponse(ctx, o.AgentName, id, &aigateway.GetAgentMCPConfigParams{Project: o.Proj})
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON400, resp.JSON404, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON200)
	}

	return printConfig(o.IO, resp.JSON200)
}

func printConfig(io *iostreams.IOStreams, c *aigateway.AgentMcpConfigDocument) error {
	cs := io.ColorScheme()
	desc := "-"
	if c.Spec.Description != nil && *c.Spec.Description != "" {
		desc = *c.Spec.Description
	}
	fmt.Fprintf(io.Out, "name:         %s\n", cs.Bold(c.Metadata.Name))
	fmt.Fprintf(io.Out, "description:  %s\n", desc)

	url := "-"
	if c.Status != nil && c.Status.Url != nil && *c.Status.Url != "" {
		url = *c.Status.Url
	}
	fmt.Fprintf(io.Out, "\nbinding:\n")
	tp := tableprinter.New(io, "proxy", "url")
	tp.AddField(c.Spec.ProxyName)
	tp.AddField(url, tableprinter.WithColor(cs.Gray))
	tp.EndRow()
	if err := tp.Render(); err != nil {
		return err
	}

	if c.Spec.EnvironmentVariables != nil && len(*c.Spec.EnvironmentVariables) > 0 {
		fmt.Fprintf(io.Out, "\nenvironment variables:\n")
		for _, ev := range *c.Spec.EnvironmentVariables {
			fmt.Fprintf(io.Out, "  %-8s %s\n", ev.Key, ev.Name)
		}
	}

	return nil
}
