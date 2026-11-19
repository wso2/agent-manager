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

package llmprovider

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

// defaultListLimit is the page size requested by `list`, matching `project list`.
const defaultListLimit = 50

type ListOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*aigateway.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj string) render.Scope

	Org   string
	Scope render.Scope
}

func NewListCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:           f.IOStreams,
		Client:       f.AIGateway,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.Scope,
	}
	return &cobra.Command{
		Use:   "list",
		Short: "List LLM providers in an organization",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, _, err := opts.ResolveScope(cmd, true, false)
			scope := opts.MakeScope(org, "")
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Scope = org, scope
			return runList(cmd.Context(), opts)
		},
	}
}

func runList(ctx context.Context, o *ListOptions) error {
	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	// Request a 50-item page to match `project list`; --limit/--offset paging
	// is not exposed yet.
	limit := int32(defaultListLimit)
	resp, err := client.ListLLMProvidersWithResponse(ctx, o.Org, &aigateway.ListLLMProvidersParams{Limit: &limit})
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON400, resp.JSON401, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON200)
	}

	if len(resp.JSON200.Providers) == 0 {
		return render.EmptyList(o.IO, fmt.Sprintf("No LLM providers found in organization %q.", o.Org))
	}

	tp := tableprinter.New(o.IO, "id", "name", "template", "status", "created")
	cs := o.IO.ColorScheme()
	for _, p := range resp.JSON200.Providers {
		tp.AddField(p.Id, tableprinter.WithColor(cs.Bold))
		tp.AddField(p.Name)
		tp.AddField(p.Template)
		tp.AddField(string(p.Status))
		created := ""
		if p.CreatedAt != nil {
			created = p.CreatedAt.Format("2006-01-02")
		}
		tp.AddField(created, tableprinter.WithColor(cs.Gray))
		tp.EndRow()
	}
	return tp.Render()
}
