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
	"net/http"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

func parseEnvFlag(inputs []string) (map[string]string, error) {
	out := make(map[string]string, len(inputs))
	for _, raw := range inputs {
		idx := strings.IndexByte(raw, '=')
		if idx < 0 {
			return nil, fmt.Errorf("invalid --env %q: expected KEY=VALUE", raw)
		}
		key := raw[:idx]
		if key == "" {
			return nil, fmt.Errorf("invalid --env %q: empty key", raw)
		}
		out[key] = raw[idx+1:]
	}
	return out, nil
}

// envVarsFromFlags turns parsed --env values into the deploy body's list,
// sorted by key so the request is deterministic.
func envVarsFromFlags(env map[string]string) []gen.EnvironmentVariable {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]gen.EnvironmentVariable, 0, len(keys))
	for _, k := range keys {
		v := env[k]
		out = append(out, gen.EnvironmentVariable{Key: k, Value: &v})
	}
	return out
}

type DeployOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*gen.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj, agent string) render.Scope
	ResolveAgent func([]string) (string, []string, error)

	Org       string
	Proj      string
	Scope     render.Scope
	AgentName string

	BuildName string
	EnvFlags  []string
}

type DeployResult struct {
	Agent   string `json:"agent"`
	Build   string `json:"build"`
	ImageId string `json:"imageId"`
}

type deployableBuild struct {
	Name    string
	ImageID string
	Status  string
}

func NewDeployCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &DeployOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.AgentScope,
		ResolveAgent: f.ResolveAgent,
	}
	cmd := &cobra.Command{
		Use:   "deploy [agent]",
		Short: "Deploy a built agent image",
		Long: "Deploy a built agent image.\n" +
			"\n" +
			"Env vars supplied via --env are sent with the deployment and stored as\n" +
			"plain text — use the platform UI for secrets.",
		Args: cobra.MaximumNArgs(1),
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
			return runDeploy(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.BuildName, "build-name", "", "Specific build to deploy (default: latest by startedAt)")
	cmd.Flags().StringArrayVar(&opts.EnvFlags, "env", nil, "Set env var as KEY=VALUE (repeatable)")
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return cmdutil.CompleteBuildableAgents(cmd, f), cobra.ShellCompDirectiveNoFileComp
	}
	return cmd
}

func runDeploy(ctx context.Context, o *DeployOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	if err := cmdutil.ValidateBuildable(ctx, client, o.Org, o.Proj, o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	build, err := resolveDeployableBuild(ctx, client, o.Org, o.Proj, o.AgentName, o.BuildName)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	cliEnv, err := parseEnvFlag(o.EnvFlags)
	if err != nil {
		return render.Error(o.IO, o.Scope, cmdutil.FlagErrorf("%v", err))
	}
	finalEnv := envVarsFromFlags(cliEnv)

	body := gen.DeployAgentJSONRequestBody{
		ImageId: build.ImageID,
		Env:     &finalEnv,
	}
	depResp, err := client.DeployAgentWithResponse(ctx, o.Org, o.Proj, o.AgentName, body)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if depResp.HTTPResponse == nil || depResp.HTTPResponse.StatusCode != http.StatusAccepted {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(depResp.HTTPResponse,
			cmdutil.FirstNonNil(depResp.JSON400, depResp.JSON404, depResp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, DeployResult{
			Agent: o.AgentName, Build: build.Name, ImageId: build.ImageID,
		})
	}
	cs := o.IO.StderrColorScheme()
	fmt.Fprintf(o.IO.ErrOut,
		"%s Deploy requested for agent %q (build: %s, image: %s). Run 'amctl agent get %s' to check status.\n",
		cs.SuccessIcon(), o.AgentName, build.Name, build.ImageID, o.AgentName)
	return nil
}

func resolveDeployableBuild(ctx context.Context, client *gen.ClientWithResponses,
	org, proj, agent, buildName string,
) (deployableBuild, error) {
	if buildName != "" {
		b, err := cmdutil.GetBuild(ctx, client, org, proj, agent, buildName)
		if err != nil {
			return deployableBuild{}, err
		}
		status := ""
		if b.Status != nil {
			status = string(*b.Status)
		}
		if !isBuildDeployable(status) || b.ImageId == nil || *b.ImageId == "" {
			return deployableBuild{}, clierr.Newf(clierr.BuildNotDeployable,
				"build %q not deployable: status=%s", b.BuildName, status)
		}
		return deployableBuild{Name: b.BuildName, ImageID: *b.ImageId, Status: status}, nil
	}
	limit := 1
	resp, err := client.GetAgentBuildsWithResponse(ctx, org, proj, agent, &gen.GetAgentBuildsParams{Limit: &limit})
	if err != nil {
		return deployableBuild{}, clierr.Newf(clierr.Transport, "%v", err)
	}
	if resp.JSON200 == nil {
		return deployableBuild{}, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON400, resp.JSON404, resp.JSON500))
	}
	builds := buildsFromListResponse(resp.JSON200)
	if len(builds) == 0 {
		return deployableBuild{}, clierr.Newf(clierr.BuildNotDeployable,
			"no builds found for agent %q; run 'amctl agent build create' first", agent)
	}
	b := builds[0]
	status := ""
	if b.Status != nil {
		status = string(*b.Status)
	}
	if !isBuildDeployable(status) || b.ImageId == nil || *b.ImageId == "" {
		return deployableBuild{}, clierr.Newf(clierr.BuildNotDeployable,
			"build %q not deployable: status=%s", b.BuildName, status)
	}
	return deployableBuild{Name: b.BuildName, ImageID: *b.ImageId, Status: status}, nil
}

// isBuildDeployable returns true for build status strings the server emits
// when an image is available. The OpenAPI spec advertises a
// BuildCompleted/BuildInProgress/BuildTriggered enum, but the server emits the
// build workflow phase verbatim. imageId is populated in both states.
func isBuildDeployable(status string) bool {
	return status == "Completed" || status == "Succeeded"
}

func buildsFromListResponse(r *gen.BuildsListResponse) []gen.BuildResponse {
	if r == nil {
		return nil
	}
	return r.Builds
}
