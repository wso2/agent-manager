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

package build

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
)

type GetOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj string) render.Scope

	Org       string
	Proj      string
	Scope     render.Scope
	AgentName string
	BuildName string
}

func NewGetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &GetOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.Scope,
	}

	cmd := &cobra.Command{
		Use:   "get <agent> <build-name>",
		Short: "Show details of a build",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.ResolveScope(cmd, true, true)
			scope := opts.MakeScope(org, proj)
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Proj, opts.Scope = org, proj, scope
			opts.AgentName = args[0]
			opts.BuildName = args[1]
			return runGet(cmd.Context(), opts)
		},
	}
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		switch len(args) {
		case 0:
			return cmdutil.CompleteBuildableAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
		case 1:
			return cmdutil.CompleteBuilds(cmd, f, args[0]), cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return cmd
}

func runGet(ctx context.Context, o *GetOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if err := cmdutil.ValidatePathParam("build name", o.BuildName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if err := cmdutil.ValidateBuildable(ctx, client, o.Org, o.Proj, o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	resp, err := client.GetBuildWithResponse(ctx, o.Org, o.Proj, o.AgentName, o.BuildName)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON404, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON200)
	}

	b := resp.JSON200
	cs := o.IO.ColorScheme()

	status := "-"
	if b.Status != nil {
		status = string(*b.Status)
	}

	duration := "-"
	if b.DurationSeconds != nil {
		duration = formatDuration(time.Duration(*b.DurationSeconds) * time.Second)
	}

	fmt.Fprintf(o.IO.Out, "build:     %s\n", cs.Bold(b.BuildName))
	fmt.Fprintf(o.IO.Out, "agent:     %s\n", b.AgentName)
	fmt.Fprintf(o.IO.Out, "status:    %s\n", status)
	fmt.Fprintf(o.IO.Out, "duration:  %s\n", duration)
	fmt.Fprintf(o.IO.Out, "started:   %s\n", cs.Gray(b.StartedAt.Format("2006-01-02T15:04:05Z07:00")))
	if b.EndedAt != nil {
		fmt.Fprintf(o.IO.Out, "ended:     %s\n", cs.Gray(b.EndedAt.Format("2006-01-02T15:04:05Z07:00")))
	}

	if b.Steps != nil && len(*b.Steps) > 0 {
		fmt.Fprintf(o.IO.Out, "\nsteps:\n")
		for _, step := range *b.Steps {
			fmt.Fprintf(o.IO.Out, "  %s %s\n", stepIcon(step.Status), step.Message)
		}
	}

	return nil
}

func formatDuration(d time.Duration) string {
	d = d.Truncate(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m == 0 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%dm%02ds", m, s)
}

func stepIcon(status amsvc.BuildStepStatus) string {
	switch status {
	case amsvc.BuildStepStatusSucceeded:
		return "✓"
	case amsvc.BuildStepStatusFailed:
		return "✗"
	case amsvc.BuildStepStatusRunning:
		return "●"
	default:
		return "○"
	}
}
