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

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
	"github.com/wso2/agent-manager/cli/pkg/tableprinter"
)

type ListOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj string) render.Scope

	Org    string
	Proj   string
	Scope  render.Scope
	Limit  *int
	Offset *int
}

func NewListCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.Scope,
	}
	var limit, offset int
	var limitSet, offsetSet bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List agents in a project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.ResolveScope(cmd, true, true)
			scope := opts.MakeScope(org, proj)
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			if limitSet && limit < 1 {
				return render.Error(opts.IO, scope, cmdutil.FlagErrorf("--limit must be >= 1"))
			}
			if offsetSet && offset < 0 {
				return render.Error(opts.IO, scope, cmdutil.FlagErrorf("--offset must be >= 0"))
			}
			opts.Org, opts.Proj, opts.Scope = org, proj, scope
			if limitSet {
				v := limit
				opts.Limit = &v
			}
			if offsetSet {
				v := offset
				opts.Offset = &v
			}
			return runList(cmd.Context(), opts)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of results to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "Number of results to skip")
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		limitSet = cmd.Flags().Changed("limit")
		offsetSet = cmd.Flags().Changed("offset")
		return nil
	}
	return cmd
}

func runList(ctx context.Context, o *ListOptions) error {
	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	resp, err := client.ListAgentsWithResponse(ctx, o.Org, o.Proj, &amsvc.ListAgentsParams{
		Limit:  o.Limit,
		Offset: o.Offset,
	})
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON400, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON200)
	}

	tp := tableprinter.New(o.IO, "name", "display name", "status", "created")
	cs := o.IO.ColorScheme()
	for _, a := range resp.JSON200.Agents {
		tp.AddField(a.Name, tableprinter.WithColor(cs.Bold))
		tp.AddField(a.DisplayName)
		status := "-"
		if a.Status != nil {
			status = *a.Status
		}
		tp.AddField(status)
		tp.AddField(a.CreatedAt.Format("2006-01-02"), tableprinter.WithColor(cs.Gray))
		tp.EndRow()
	}
	return tp.Render()
}
