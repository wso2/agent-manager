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

package llm

import (
	"context"

	"github.com/spf13/cobra"

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
	"github.com/wso2/agent-manager/cli/pkg/tableprinter"
)

type ListOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*aigateway.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj, agent string) render.Scope
	ResolveAgent func([]string) (string, []string, error)

	Org       string
	Proj      string
	Scope     render.Scope
	AgentName string
}

// ListResult is the JSON envelope payload for `llm list`.
type ListResult struct {
	Configs []aigateway.AgentModelConfigListItem `json:"configs"`
}

func NewListCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:           f.IOStreams,
		Client:       f.AIGateway,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.AgentScope,
		ResolveAgent: f.ResolveAgent,
	}

	cmd := &cobra.Command{
		Use:   "list [agent]",
		Short: "List LLM provider configurations for an agent",
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
			return runList(cmd.Context(), opts)
		},
	}
	return cmd
}

func runList(ctx context.Context, o *ListOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	resp, err := client.ListAgentModelConfigsWithResponse(ctx, o.Org, o.Proj, o.AgentName, &aigateway.ListAgentModelConfigsParams{})
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, resp.JSON500))
	}

	configs := make([]aigateway.AgentModelConfigListItem, 0, len(resp.JSON200.Configs))
	for _, c := range resp.JSON200.Configs {
		if c.Type == configType {
			configs = append(configs, c)
		}
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, ListResult{Configs: configs})
	}

	tp := tableprinter.New(o.IO, "name", "description")
	cs := o.IO.ColorScheme()
	for _, c := range configs {
		tp.AddField(c.Name, tableprinter.WithColor(cs.Bold))
		desc := "-"
		if c.Description != nil && *c.Description != "" {
			desc = *c.Description
		}
		tp.AddField(desc)
		tp.EndRow()
	}
	return tp.Render()
}
