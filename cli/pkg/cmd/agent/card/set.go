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

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/cmd/agent/a2acard"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

// SetOptions holds the dependencies and resolved inputs for card set.
type SetOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	ResolveAgent func([]string) (string, []string, error)
	MakeScope    func(org, proj, agent string) render.Scope

	Org       string
	Proj      string
	AgentName string
	Scope     render.Scope

	URL   string
	Proxy bool
}

// NewSetCmd returns the "agent card set" command.
func NewSetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &SetOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		ResolveAgent: f.ResolveAgent,
		MakeScope:    f.AgentScope,
	}

	cmd := &cobra.Command{
		Use:   "set [agent] --url <card-url> [--proxy]",
		Short: "Register an agent's card URL",
		Long: "Register the URL the agent's A2A card is fetched from. " +
			"With --proxy the agent is routed through its gateway. " +
			"Callers of a proxied agent need a gateway API key, created in the console.",
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
			opts.Org, opts.Proj, opts.AgentName = org, proj, agentName
			opts.Scope = opts.MakeScope(org, proj, agentName)
			return runSet(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.URL, "url", "", "URL the agent card is served from (required)")
	cmd.Flags().BoolVar(&opts.Proxy, "proxy", false, "Route the agent through its gateway (URL must end in /.well-known/agent-card.json)")
	_ = cmd.MarkFlagRequired("url")
	cmd.ValidArgsFunction = completeAgentArg(f)
	return cmd
}

func runSet(ctx context.Context, o *SetOptions) error {
	if err := validateTarget(o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if err := a2acard.CheckCardURL("--url", o.URL, o.Proxy); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	target := a2acard.Target{Org: o.Org, Proj: o.Proj, Agent: o.AgentName}
	res, err := a2acard.ApplySource(ctx, client, target, o.URL, o.Proxy)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, res)
	}
	cs := o.IO.StderrColorScheme()
	fmt.Fprintf(o.IO.ErrOut, "%s Registered card URL (%s)\n", cs.SuccessIcon(), res.Routing)
	fmt.Fprintf(o.IO.ErrOut, "  Check it with: amctl agent card get %s\n", o.AgentName)
	return nil
}
