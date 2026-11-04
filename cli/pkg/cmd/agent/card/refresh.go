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

package card

import (
	"context"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

// RefreshOptions holds the dependencies and resolved inputs for card refresh.
type RefreshOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	ResolveAgent func([]string) (string, []string, error)
	ResolveEnv   func(*cobra.Command) (string, error)
	MakeScope    func(org, proj, agent, env string) render.Scope

	Org       string
	Proj      string
	AgentName string
	Env       string
	Scope     render.Scope
}

// NewRefreshCmd returns the "agent card refresh" command.
func NewRefreshCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &RefreshOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		ResolveAgent: f.ResolveAgent,
		ResolveEnv:   f.ResolveEnvironment,
		MakeScope:    f.EnvScope,
	}

	cmd := &cobra.Command{
		Use:   "refresh [agent]",
		Short: "Re-fetch an agent's card in an environment",
		Long: "Queue a re-fetch of the agent's A2A card in one environment. " +
			"For a proxied environment this also republishes the gateway routes.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.ResolveScope(cmd, true, true)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			agentName, _, err := opts.ResolveAgent(args)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			env, err := opts.ResolveEnv(cmd)
			if err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}
			opts.Org, opts.Proj, opts.AgentName, opts.Env = org, proj, agentName, env
			opts.Scope = opts.MakeScope(org, proj, agentName, env)
			return runRefresh(cmd.Context(), opts)
		},
	}
	cmdutil.AddEnvFlag(cmd)
	cmd.ValidArgsFunction = completeAgentArg(f)
	return cmd
}

func runRefresh(ctx context.Context, o *RefreshOptions) error {
	if err := validateTarget(o.AgentName, o.Env); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	resp, err := client.RefreshAgentCardWithResponse(ctx, o.Org, o.Proj, o.AgentName, o.Env)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.StatusCode() != http.StatusAccepted {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse,
			cmdutil.FirstNonNil(resp.JSON400, resp.JSON403, resp.JSON404, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, map[string]any{"environment": o.Env, "queued": true})
	}
	cs := o.IO.StderrColorScheme()
	fmt.Fprintf(o.IO.ErrOut, "%s Card refresh queued for %s\n", cs.SuccessIcon(), o.Env)
	fmt.Fprintf(o.IO.ErrOut, "  Check it with: amctl agent card get %s --env %s\n", o.AgentName, o.Env)
	return nil
}
