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

package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

const defaultEnvironment = "default"

type LinkOptions struct {
	IO           *iostreams.IOStreams
	Config       func() (*config.Config, error)
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(string, string) render.Scope

	Agent string
}

type LinkResult struct {
	Dir         string `json:"dir"`
	Org         string `json:"org"`
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Agent       string `json:"agent,omitempty"`
}

func NewLinkCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &LinkOptions{
		IO:           f.IOStreams,
		Config:       f.Config,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.Scope,
	}
	cmd := &cobra.Command{
		Use:   "link",
		Short: "Link the current directory to an org, project, and optional agent",
		Long: `Link associates the current working directory with a specific org, project,
and optionally an agent. Linked directories always use the "default"
environment. Once linked, commands run from this directory (or any
subdirectory) use this context automatically.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, project, err := opts.ResolveScope(cmd, true, true)
			scope := opts.MakeScope(org, project)
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			return runLink(cmd.Context(), opts, org, project, scope)
		},
	}
	cmdutil.EnableProjectOverride(cmd, f)
	cmd.Flags().StringVar(&opts.Agent, "agent", "", "Agent name (optional)")

	_ = cmd.RegisterFlagCompletionFunc("agent", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return cmdutil.CompleteAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func runLink(ctx context.Context, o *LinkOptions, org, project string, scope render.Scope) error {
	wd, err := os.Getwd()
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.Internal, "get working directory: %v", err))
	}

	cfg, err := o.Config()
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigNotLoaded, "%v", err))
	}

	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, scope, err)
	}

	if err := cmdutil.ValidatePathParam("project", project); err != nil {
		return render.Error(o.IO, scope, err)
	}
	projResp, err := client.GetProjectWithResponse(ctx, org, project)
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if projResp.JSON200 == nil {
		return render.Error(o.IO, scope, cmdutil.ErrorFromServer(projResp.HTTPResponse, cmdutil.FirstNonNil(projResp.JSON404, projResp.JSON500)))
	}

	envResp, err := client.GetEnvironmentWithResponse(ctx, org, defaultEnvironment)
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if envResp.JSON200 == nil {
		return render.Error(o.IO, scope, cmdutil.ErrorFromServer(envResp.HTTPResponse, cmdutil.FirstNonNil(envResp.JSON404, envResp.JSON400, envResp.JSON500)))
	}
	scope.Environment = defaultEnvironment

	if o.Agent != "" {
		if err := cmdutil.ValidatePathParam("agent", o.Agent); err != nil {
			return render.Error(o.IO, scope, err)
		}
		agentResp, err := client.GetAgentWithResponse(ctx, org, project, o.Agent)
		if err != nil {
			return render.Error(o.IO, scope, clierr.Newf(clierr.Transport, "%v", err))
		}
		if agentResp.JSON200 == nil {
			return render.Error(o.IO, scope, cmdutil.ErrorFromServer(agentResp.HTTPResponse, cmdutil.FirstNonNil(agentResp.JSON404, agentResp.JSON500)))
		}
		scope.Agent = o.Agent
	}

	cfg.LinkProject(wd, config.LinkedProject{
		Org:         org,
		Project:     project,
		Environment: defaultEnvironment,
		Agent:       o.Agent,
	})
	if err := cfg.Save(); err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigSaveFailed, "save config: %v", err))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, scope, LinkResult{
			Dir:         wd,
			Org:         org,
			Project:     project,
			Environment: defaultEnvironment,
			Agent:       o.Agent,
		})
	}

	cs := o.IO.StderrColorScheme()
	fmt.Fprintf(o.IO.ErrOut, "%s Linked %s to %s/%s (env: %s)", cs.SuccessIcon(), cs.Bold(wd), cs.Cyan(org), cs.Bold(project), cs.Green(defaultEnvironment))
	if o.Agent != "" {
		fmt.Fprintf(o.IO.ErrOut, " (agent: %s)", cs.Yellow(o.Agent))
	}
	fmt.Fprintln(o.IO.ErrOut)
	return nil
}
