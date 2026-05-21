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
	"sort"
	"strings"
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
	Env       string
}

type StatusResult struct {
	Agent        string      `json:"agent"`
	Environments []EnvStatus `json:"environments"`
}

type EnvStatus struct {
	Name         string        `json:"name"`
	DisplayName  string        `json:"displayName,omitempty"`
	Status       string        `json:"status"`
	LastDeployed time.Time     `json:"lastDeployed"`
	Endpoints    []EndpointRef `json:"endpoints"`
}

type EndpointRef struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Visibility string `json:"visibility"`
}

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
		Short: "Show deployment status by environment",
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
	cmd.Flags().StringVar(&opts.Env, "env", "", "Filter by environment name")
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
	deployments, paths, err := fetchStatusData(ctx, client, o.Org, o.Proj, o.AgentName)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	if len(deployments) == 0 {
		fmt.Fprintln(o.IO.ErrOut, "No environments configured for this project's deployment pipeline.")
		result := StatusResult{Agent: o.AgentName, Environments: []EnvStatus{}}
		if o.IO.JSON {
			return render.JSONSuccess(o.IO, o.Scope, result)
		}
		return nil
	}

	ordered := orderEnvironments(paths, deployments)
	ordered, err = filterEnvironments(ordered, deployments, o.Env)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	result := buildStatusResult(o.AgentName, ordered, deployments)
	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, result)
	}
	return renderStatusTable(o.IO, result)
}

func fetchStatusData(ctx context.Context, client *amsvc.ClientWithResponses, org, proj, agent string) (amsvc.DeploymentListResponse, []amsvc.PromotionPath, error) {
	type depResult struct {
		resp *amsvc.ListAgentDeploymentsResp
		err  error
	}
	type pipeResult struct {
		resp *amsvc.GetDeploymentPipelineResp
		err  error
	}

	depCh := make(chan depResult, 1)
	pipeCh := make(chan pipeResult, 1)

	go func() {
		resp, err := client.ListAgentDeploymentsWithResponse(ctx, org, proj, agent)
		depCh <- depResult{resp: resp, err: err}
	}()
	go func() {
		resp, err := client.GetDeploymentPipelineWithResponse(ctx, org, proj)
		pipeCh <- pipeResult{resp: resp, err: err}
	}()

	dep := <-depCh
	pipe := <-pipeCh

	if dep.err != nil {
		return nil, nil, clierr.Newf(clierr.Transport, "%v", dep.err)
	}
	if dep.resp.JSON200 == nil {
		return nil, nil, cmdutil.ErrorFromServer(dep.resp.HTTPResponse, cmdutil.FirstNonNil(dep.resp.JSON404, dep.resp.JSON500))
	}
	if pipe.err != nil {
		return nil, nil, clierr.Newf(clierr.Transport, "%v", pipe.err)
	}
	if pipe.resp.JSON200 == nil {
		return nil, nil, cmdutil.ErrorFromServer(pipe.resp.HTTPResponse, cmdutil.FirstNonNil(pipe.resp.JSON404, pipe.resp.JSON500))
	}
	return *dep.resp.JSON200, pipe.resp.JSON200.PromotionPaths, nil
}

func buildStatusResult(agent string, order []string, deployments amsvc.DeploymentListResponse) StatusResult {
	out := StatusResult{Agent: agent, Environments: make([]EnvStatus, 0, len(order))}
	for _, envName := range order {
		d := deployments[envName]
		row := EnvStatus{
			Name:         envName,
			Status:       d.Status,
			LastDeployed: d.LastDeployed,
			Endpoints:    make([]EndpointRef, 0, len(d.Endpoints)),
		}
		if d.EnvironmentDisplayName != nil {
			row.DisplayName = *d.EnvironmentDisplayName
		}
		for _, ep := range d.Endpoints {
			row.Endpoints = append(row.Endpoints, EndpointRef{
				Name:       ep.Name,
				URL:        ep.Url,
				Visibility: string(ep.Visibility),
			})
		}
		out.Environments = append(out.Environments, row)
	}
	return out
}

func orderEnvironments(paths []amsvc.PromotionPath, deployments amsvc.DeploymentListResponse) []string {
	ordered := make([]string, 0, len(deployments))
	visited := make(map[string]struct{}, len(deployments))
	traversed := make(map[string]struct{}, len(paths))

	bySource := make(map[string]amsvc.PromotionPath, len(paths))
	for _, p := range paths {
		bySource[p.SourceEnvironmentRef] = p
	}

	cur := findLowestEnvironment(paths)
	for cur != "" {
		if _, seen := traversed[cur]; seen {
			break
		}
		traversed[cur] = struct{}{}
		if _, ok := deployments[cur]; ok {
			ordered = append(ordered, cur)
			visited[cur] = struct{}{}
		}
		path, ok := bySource[cur]
		if !ok || len(path.TargetEnvironmentRefs) == 0 {
			break
		}
		cur = path.TargetEnvironmentRefs[0].Name
	}

	leftovers := make([]string, 0)
	for env := range deployments {
		if _, seen := visited[env]; !seen {
			leftovers = append(leftovers, env)
		}
	}
	sort.Strings(leftovers)
	ordered = append(ordered, leftovers...)
	return ordered
}

func filterEnvironments(order []string, deployments amsvc.DeploymentListResponse, selected string) ([]string, error) {
	if selected == "" {
		return order, nil
	}
	if _, ok := deployments[selected]; !ok {
		valid := make([]string, 0, len(deployments))
		for name := range deployments {
			valid = append(valid, name)
		}
		sort.Strings(valid)
		return nil, clierr.Newf(clierr.NotFound, "environment %q is not in deployment results; valid: %s", selected, strings.Join(valid, ", "))
	}
	return []string{selected}, nil
}

func renderStatusTable(io *iostreams.IOStreams, result StatusResult) error {
	tp := tableprinter.New(io, "env", "status", "last deployed", "endpoints")
	cs := io.ColorScheme()
	for _, env := range result.Environments {
		tp.AddField(env.Name, tableprinter.WithColor(cs.Bold))
		if color := statusColorFunc(cs, env.Status); color != nil {
			tp.AddField(env.Status, tableprinter.WithColor(color))
		} else {
			tp.AddField(env.Status)
		}
		tp.AddField(formatLastDeployed(env.LastDeployed), tableprinter.WithColor(cs.Gray))
		tp.AddField(summarizeEndpoints(env.Endpoints))
		tp.EndRow()
	}
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
	return t.Format("2006-01-02T15:04:05Z07:00")
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
