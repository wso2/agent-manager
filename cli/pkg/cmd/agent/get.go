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

package agent

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

type GetOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj, agent string) render.Scope
	ResolveAgent func([]string) (string, []string, error)

	Org       string
	Proj      string
	Scope     render.Scope
	AgentName string
}

func NewGetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &GetOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.AgentScope,
		ResolveAgent: f.ResolveAgent,
	}
	cmd := &cobra.Command{
		Use:   "get [agent]",
		Short: "Show details of an agent",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.ResolveScope(cmd, true, true)
			if err != nil {
				scope := opts.MakeScope(org, proj, "")
				return render.Error(opts.IO, scope, err)
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
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return cmdutil.CompleteAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
	}
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
	resp, err := client.GetAgentWithResponse(ctx, o.AgentName, &amsvc.GetAgentParams{Project: o.Proj})
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON404, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON200)
	}

	a := resp.JSON200
	w := o.IO.Out
	cs := o.IO.ColorScheme()
	description := ""
	if a.Spec.Description != nil {
		description = *a.Spec.Description
	}
	fmt.Fprintf(w, "name:          %s\n", cs.Bold(a.Metadata.Name))
	fmt.Fprintf(w, "display name:  %s\n", a.Spec.DisplayName)
	fmt.Fprintf(w, "description:   %s\n", description)
	typeStr := a.Spec.AgentType.Type
	if a.Spec.AgentType.SubType != nil && *a.Spec.AgentType.SubType != "" {
		typeStr = fmt.Sprintf("%s / %s", a.Spec.AgentType.Type, *a.Spec.AgentType.SubType)
	}
	fmt.Fprintf(w, "type:          %s\n", typeStr)
	status := "-"
	if a.Status != nil && a.Status.State != nil {
		status = *a.Status.State
	}
	fmt.Fprintf(w, "status:        %s\n", status)
	fmt.Fprintf(w, "project:       %s\n", a.Metadata.Project)
	created := "-"
	if a.Metadata.CreationTimestamp != nil {
		created = a.Metadata.CreationTimestamp.Format("2006-01-02T15:04:05Z07:00")
	}
	fmt.Fprintf(w, "created:       %s\n", cs.Gray(created))
	return nil
}
