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

package gateway

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
	MakeScope    func(org, proj string) render.Scope

	Org     string
	Scope   render.Scope
	Gateway string
}

func NewGetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &GetOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.Scope,
	}
	cmd := &cobra.Command{
		Use:   "get <gateway>",
		Short: "Get details of a gateway by name or UUID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, _, err := opts.ResolveScope(cmd, true, false)
			scope := opts.MakeScope(org, "")
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Scope = org, scope
			opts.Gateway = args[0]
			return runGet(cmd.Context(), opts)
		},
	}
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return cmdutil.CompleteGateways(cmd, f), cobra.ShellCompDirectiveNoFileComp
	}
	return cmd
}

func runGet(ctx context.Context, o *GetOptions) error {
	if err := cmdutil.ValidatePathParam("gateway", o.Gateway); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	resp, err := client.GetGatewayWithResponse(ctx, o.Org, o.Gateway)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse,
			cmdutil.FirstNonNil(resp.JSON400, resp.JSON401, resp.JSON404, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON200)
	}

	g := resp.JSON200
	w := o.IO.Out
	cs := o.IO.ColorScheme()
	fmt.Fprintf(w, "name:          %s\n", cs.Bold(g.Name))
	fmt.Fprintf(w, "display name:  %s\n", g.DisplayName)
	fmt.Fprintf(w, "uuid:          %s\n", g.Uuid)
	fmt.Fprintf(w, "type:          %s\n", string(g.GatewayType))
	fmt.Fprintf(w, "status:        %s\n", string(g.Status))
	fmt.Fprintf(w, "vhost:         %s\n", g.Vhost)
	if g.Region != nil && *g.Region != "" {
		fmt.Fprintf(w, "region:        %s\n", *g.Region)
	}
	if g.RuntimeUrl != nil && *g.RuntimeUrl != "" {
		fmt.Fprintf(w, "runtime url:   %s\n", *g.RuntimeUrl)
	}
	fmt.Fprintf(w, "critical:      %t\n", g.IsCritical)
	fmt.Fprintf(w, "org:           %s\n", o.Org)
	fmt.Fprintf(w, "created:       %s\n", cs.Gray(g.CreatedAt.Format("2006-01-02T15:04:05Z07:00")))
	return nil
}
