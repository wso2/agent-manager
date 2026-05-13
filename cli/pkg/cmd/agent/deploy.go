// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
// Licensed under the Apache License, Version 2.0.

package agent

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/prompter"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

// envConflict describes a single key where the CLI's --env value differs from
// what's currently configured on the agent.
type envConflict struct {
	Key              string
	CurrentValue     string
	NewValue         string
	CurrentSensitive bool
}

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

// findLowestEnvironment returns the entry environment of the deployment
// pipeline — the SourceEnvironmentRef that does not appear anywhere as a
// target. Mirrors the server-side selection rule; replicated client-side so we
// can fetch GetAgentConfigurations for that env before the deploy call.
func findLowestEnvironment(paths []gen.PromotionPath) string {
	targets := make(map[string]struct{})
	for _, p := range paths {
		for _, t := range p.TargetEnvironmentRefs {
			targets[t.Name] = struct{}{}
		}
	}
	for _, p := range paths {
		if _, isTarget := targets[p.SourceEnvironmentRef]; !isTarget {
			return p.SourceEnvironmentRef
		}
	}
	return ""
}

func mergeEnv(current []gen.ConfigurationItem, cli map[string]string) ([]gen.EnvironmentVariable, []envConflict) {
	final := make([]gen.EnvironmentVariable, 0, len(current)+len(cli))
	conflicts := make([]envConflict, 0, len(current)+len(cli))
	seen := make(map[string]struct{}, len(current))

	for _, c := range current {
		seen[c.Key] = struct{}{}
		isSensitive := c.IsSensitive != nil && *c.IsSensitive
		newVal, hasNew := cli[c.Key]
		switch {
		case !hasNew:
			val := c.Value
			ev := gen.EnvironmentVariable{Key: c.Key, Value: &val}
			if isSensitive {
				ev.IsSensitive = boolPtrLocal(true)
				if c.SecretRef != nil {
					ev.SecretRef = c.SecretRef
				}
				ev.Value = nil
			}
			final = append(final, ev)
		case isSensitive:
			v := newVal
			final = append(final, gen.EnvironmentVariable{Key: c.Key, Value: &v})
			conflicts = append(conflicts, envConflict{
				Key: c.Key, CurrentValue: "", NewValue: newVal, CurrentSensitive: true,
			})
		case newVal == c.Value:
			v := newVal
			final = append(final, gen.EnvironmentVariable{Key: c.Key, Value: &v})
		default:
			v := newVal
			final = append(final, gen.EnvironmentVariable{Key: c.Key, Value: &v})
			conflicts = append(conflicts, envConflict{
				Key: c.Key, CurrentValue: c.Value, NewValue: newVal, CurrentSensitive: false,
			})
		}
	}

	addedKeys := make([]string, 0, len(cli))
	for k := range cli {
		if _, ok := seen[k]; !ok {
			addedKeys = append(addedKeys, k)
		}
	}
	sort.Strings(addedKeys)
	for _, k := range addedKeys {
		v := cli[k]
		final = append(final, gen.EnvironmentVariable{Key: k, Value: &v})
	}

	return final, conflicts
}

func renderConflictTable(io *iostreams.IOStreams, conflicts []envConflict) {
	w := io.ErrOut

	sensitiveKeys := make([]string, 0)
	for _, c := range conflicts {
		if c.CurrentSensitive {
			sensitiveKeys = append(sensitiveKeys, c.Key)
		}
	}
	if len(sensitiveKeys) > 0 {
		fmt.Fprintf(w,
			"Warning: %s is currently stored as a secret. Replacing it via --env will "+
				"store the new value as plain text. Use the platform UI to keep it as a secret.\n\n",
			strings.Join(sensitiveKeys, ", "))
	}

	fmt.Fprintln(w, "The following env vars will be replaced:")
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  KEY\tCURRENT\tNEW")
	for _, c := range conflicts {
		cur := c.CurrentValue
		newV := c.NewValue
		if c.CurrentSensitive {
			cur = "(secret)"
			newV = "***"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", c.Key, cur, newV)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
}

func boolPtrLocal(b bool) *bool { return &b }

type DeployOptions struct {
	IO           *iostreams.IOStreams
	Prompter     prompter.Prompter
	Client       func(context.Context) (*gen.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj, agent string) render.Scope
	ResolveAgent func([]string) (string, []string, error)

	Org       string
	Proj      string
	Scope     render.Scope
	AgentName string

	BuildName string   // wired in Task 5
	EnvFlags  []string // wired in Task 6
	Yes       bool
}

type DeployResult struct {
	Agent             string   `json:"agent"`
	Build             string   `json:"build"`
	ImageId           string   `json:"imageId"`
	TargetEnvironment string   `json:"targetEnvironment"`
	ConflictsResolved []string `json:"conflictsResolved"`
}

type deployableBuild struct {
	Name    string
	ImageID string
	Status  string
}

func NewDeployCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &DeployOptions{
		IO:           f.IOStreams,
		Prompter:     f.Prompter,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.AgentScope,
		ResolveAgent: f.ResolveAgent,
	}
	cmd := &cobra.Command{
		Use:   "deploy [agent]",
		Short: "Deploy a built agent image",
		Long: "Deploy a built agent image to the lowest environment in the deployment pipeline.\n" +
			"\n" +
			"Env vars supplied via --env are merged with the agent's current configuration;\n" +
			"conflicting values require interactive confirmation or --yes. CLI-supplied values\n" +
			"are always stored as plain text in v1 — use the platform UI for secrets.",
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
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Skip the env-conflict confirmation prompt")
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

	pipeResp, err := client.GetDeploymentPipelineWithResponse(ctx, o.Org, o.Proj)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if pipeResp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(pipeResp.HTTPResponse, cmdutil.FirstNonNil(pipeResp.JSON404, pipeResp.JSON500)))
	}
	targetEnv := findLowestEnvironment(pipeResp.JSON200.PromotionPaths)
	if targetEnv == "" {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Internal,
			"deployment pipeline has no entry environment for project %q", o.Proj))
	}

	cfgResp, err := client.GetAgentConfigurationsWithResponse(ctx, o.Org, o.Proj, o.AgentName,
		&gen.GetAgentConfigurationsParams{Environment: targetEnv})
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if cfgResp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(cfgResp.HTTPResponse, cmdutil.FirstNonNil(cfgResp.JSON404, cfgResp.JSON500)))
	}
	currentEnv := configurationItemsFromResponse(cfgResp.JSON200)

	cliEnv, err := parseEnvFlag(o.EnvFlags)
	if err != nil {
		return render.Error(o.IO, o.Scope, cmdutil.FlagErrorf("%v", err))
	}

	finalEnv, conflicts := mergeEnv(currentEnv, cliEnv)

	if len(conflicts) > 0 && !o.Yes {
		if o.IO.JSON {
			return render.Error(o.IO, o.Scope, clierr.New(clierr.ConfirmationRequired,
				"confirmation required; pass --yes when using --json"))
		}
		if !o.IO.CanPrompt() {
			return render.Error(o.IO, o.Scope, clierr.New(clierr.ConfirmationRequired,
				"confirmation required; pass --yes when stdin is not a terminal"))
		}
		renderConflictTable(o.IO, conflicts)
		ok, perr := o.Prompter.Confirm("Proceed with deploy?")
		if perr != nil {
			return render.Error(o.IO, o.Scope, clierr.Newf(clierr.ConfirmationRequired, "%v", perr))
		}
		if !ok {
			return render.Error(o.IO, o.Scope, clierr.New(clierr.ConfirmationRequired, "deploy cancelled"))
		}
	}

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

	conflictKeys := make([]string, 0, len(conflicts))
	for _, c := range conflicts {
		conflictKeys = append(conflictKeys, c.Key)
	}
	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, DeployResult{
			Agent: o.AgentName, Build: build.Name, ImageId: build.ImageID,
			TargetEnvironment: targetEnv, ConflictsResolved: conflictKeys,
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
		// --build-name branch lands in Task 5.
		return deployableBuild{}, clierr.New(clierr.Internal, "--build-name not implemented yet")
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
	if status != "BuildCompleted" || b.ImageId == nil || *b.ImageId == "" {
		return deployableBuild{}, clierr.Newf(clierr.BuildNotDeployable,
			"build %q not deployable: status=%s", b.BuildName, status)
	}
	return deployableBuild{Name: b.BuildName, ImageID: *b.ImageId, Status: status}, nil
}

func buildsFromListResponse(r *gen.BuildsListResponse) []gen.BuildResponse {
	if r == nil {
		return nil
	}
	return r.Builds
}

func configurationItemsFromResponse(r *gen.ConfigurationResponse) []gen.ConfigurationItem {
	if r == nil {
		return nil
	}
	return r.Configurations
}
