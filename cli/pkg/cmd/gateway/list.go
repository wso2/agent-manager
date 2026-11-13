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
	"math"
	"strings"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
	"github.com/wso2/agent-manager/cli/pkg/tableprinter"
)

// gatewayTypes are the canonical placement roles accepted by --type. The server
// also takes the deprecated REGULAR/AI aliases, which the CLI deliberately does not
// advertise.
var gatewayTypes = []string{"INGRESS", "EGRESS", "BOTH"}

type ListOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj string) render.Scope

	Org   string
	Scope render.Scope

	Type   string
	Limit  *int32
	Offset *int32
}

func NewListCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.Scope,
	}
	var limit, offset int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List gateways in an organization",
		Long: "List gateways in an organization.\n\n" +
			"The uuid column is the value to pass to 'amctl llm-provider create --gateways'.\n" +
			"Only EGRESS and BOTH gateways can host an LLM proxy, and no two of a\n" +
			"provider's gateways may share an environment.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, _, err := opts.ResolveScope(cmd, true, false)
			scope := opts.MakeScope(org, "")
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Scope = org, scope

			// Both bounds are checked before the int32 conversion the wire type needs:
			// on a 64-bit host `--limit 4294967297` truncates to 1, so an unchecked
			// upper bound silently sends a page size nobody asked for.
			if cmd.Flags().Changed("limit") {
				if limit < 1 || limit > math.MaxInt32 {
					return render.Error(opts.IO, scope,
						cmdutil.FlagErrorf("--limit must be between 1 and %d", math.MaxInt32))
				}
				v := int32(limit)
				opts.Limit = &v
			}
			if cmd.Flags().Changed("offset") {
				if offset < 0 || offset > math.MaxInt32 {
					return render.Error(opts.IO, scope,
						cmdutil.FlagErrorf("--offset must be between 0 and %d", math.MaxInt32))
				}
				v := int32(offset)
				opts.Offset = &v
			}
			return runList(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Type, "type", "",
		fmt.Sprintf("Filter by placement role (%s)", strings.Join(gatewayTypes, "|")))
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of results to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "Number of results to skip")
	_ = cmd.RegisterFlagCompletionFunc("type",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return gatewayTypes, cobra.ShellCompDirectiveNoFileComp
		})
	return cmd
}

// normalizeGatewayType upper-cases the --type value and rejects anything outside
// the canonical roles, so a typo fails locally instead of being sent as a filter
// the server may not honour.
func normalizeGatewayType(value string) (amsvc.GatewayTypeInput, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	for _, known := range gatewayTypes {
		if normalized == known {
			return amsvc.GatewayTypeInput(normalized), nil
		}
	}
	return "", cmdutil.FlagErrorf("--type must be one of %s", strings.Join(gatewayTypes, ", "))
}

func runList(ctx context.Context, o *ListOptions) error {
	params := &amsvc.ListGatewaysParams{
		Limit:  o.Limit,
		Offset: o.Offset,
	}
	if o.Type != "" {
		gatewayType, err := normalizeGatewayType(o.Type)
		if err != nil {
			return render.Error(o.IO, o.Scope, err)
		}
		params.Type = &gatewayType
	}
	// Status is deliberately never filtered: the server's candidate set for
	// placement is not liveness-filtered, so an ACTIVE-only view would offer a
	// narrower set than the server actually accepts.

	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	resp, err := client.ListGatewaysWithResponse(ctx, o.Org, params)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse,
			cmdutil.FirstNonNil(resp.JSON400, resp.JSON401, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON200)
	}

	if len(resp.JSON200.Gateways) == 0 {
		return render.EmptyList(o.IO, o.emptyMessage())
	}

	tp := tableprinter.New(o.IO, "name", "uuid", "type", "status", "vhost")
	cs := o.IO.ColorScheme()
	for _, g := range resp.JSON200.Gateways {
		tp.AddField(g.Name, tableprinter.WithColor(cs.Bold))
		tp.AddField(g.Uuid)
		tp.AddField(string(g.GatewayType))
		tp.AddField(string(g.Status))
		tp.AddField(g.Vhost, tableprinter.WithColor(cs.Gray))
		tp.EndRow()
	}
	return tp.Render()
}

// emptyMessage names the active filters, so a filter that matches nothing reads as
// such rather than as an org with no gateways.
func (o *ListOptions) emptyMessage() string {
	filters := []string{}
	if o.Type != "" {
		filters = append(filters, fmt.Sprintf("type %s", strings.ToUpper(o.Type)))
	}
	if len(filters) == 0 {
		return fmt.Sprintf("No gateways found in organization %q.", o.Org)
	}
	return fmt.Sprintf("No gateways in organization %q match %s.",
		o.Org, strings.Join(filters, " and "))
}
