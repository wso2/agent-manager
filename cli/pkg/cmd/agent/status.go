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
	"time"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
	"github.com/wso2/agent-manager/cli/pkg/tableprinter"
)

type StatusOptions struct {
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

type StatusResult struct {
	Agent        string        `json:"agent"`
	Status       string        `json:"status"`
	LastDeployed time.Time     `json:"lastDeployed"`
	Endpoints    []EndpointRef `json:"endpoints"`
}

type EndpointRef struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Visibility string `json:"visibility"`
}

// notDeployed is the status reported when the agent has no deployment.
const notDeployed = "not-deployed"

func NewStatusCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &StatusOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.AgentScope,
		ResolveAgent: f.ResolveAgent,
	}
	cmd := &cobra.Command{
		Use:   "status [agent]",
		Short: "Show deployment status",
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
			return runStatus(cmd.Context(), opts)
		},
	}
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return cmdutil.CompleteAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
	}
	return cmd
}

func runStatus(ctx context.Context, o *StatusOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	resp, err := client.ListAgentDeploymentsWithResponse(ctx, o.Org, o.Proj, o.AgentName)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON404, resp.JSON500)))
	}

	d, ok := (*resp.JSON200)[cmdutil.DefaultEnvironment]
	if !ok {
		fmt.Fprintln(o.IO.ErrOut, "No deployment found for this agent.")
		result := StatusResult{Agent: o.AgentName, Status: notDeployed, Endpoints: []EndpointRef{}}
		if o.IO.JSON {
			return render.JSONSuccess(o.IO, o.Scope, result)
		}
		return nil
	}
	result := buildStatusResult(o.AgentName, d)
	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, result)
	}
	return renderStatusTable(o.IO, result)
}

func buildStatusResult(agent string, d amsvc.DeploymentDetailsResponse) StatusResult {
	out := StatusResult{
		Agent:     agent,
		Status:    d.Status,
		Endpoints: make([]EndpointRef, 0, len(d.Endpoints)),
	}
	// lastDeployed is omitted for an agent that has not deployed yet; zero time renders as "-".
	if d.LastDeployed != nil {
		out.LastDeployed = *d.LastDeployed
	}
	for _, ep := range d.Endpoints {
		out.Endpoints = append(out.Endpoints, EndpointRef{
			Name:       ep.Name,
			URL:        ep.Url,
			Visibility: string(ep.Visibility),
		})
	}
	return out
}

func renderStatusTable(io *iostreams.IOStreams, result StatusResult) error {
	tp := tableprinter.New(io, "status", "last deployed", "endpoints")
	cs := io.ColorScheme()
	if color := statusColorFunc(cs, result.Status); color != nil {
		tp.AddField(result.Status, tableprinter.WithColor(color))
	} else {
		tp.AddField(result.Status)
	}
	tp.AddField(formatLastDeployed(result.LastDeployed), tableprinter.WithColor(cs.Gray))
	tp.AddField(summarizeEndpoints(result.Endpoints))
	tp.EndRow()
	return tp.Render()
}

func statusColorFunc(cs *iostreams.ColorScheme, status string) func(string) string {
	switch status {
	case "active":
		return cs.Green
	case "in-progress":
		return cs.Yellow
	case "failed":
		return cs.Red
	case "not-deployed", "suspended":
		return cs.Gray
	default:
		return nil
	}
}

func formatLastDeployed(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}

func summarizeEndpoints(endpoints []EndpointRef) string {
	if len(endpoints) == 0 {
		return "-"
	}
	first := endpoints[0].URL
	if len(endpoints) == 1 {
		return first
	}
	return fmt.Sprintf("%s (+%d)", first, len(endpoints)-1)
}
