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

package llm

import (
	"context"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	openapi_types "github.com/oapi-codegen/runtime/types"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmd/agent/modelconfig"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/prompter"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

type UnsetOptions struct {
	IO           *iostreams.IOStreams
	Prompter     prompter.Prompter
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj, agent string) render.Scope
	ResolveAgent func([]string) (string, []string, error)

	Org       string
	Proj      string
	Scope     render.Scope
	AgentName string

	Name string
	Env  string
	Yes  bool
}

// UnsetResult is the JSON envelope payload for a full-config delete.
type UnsetResult struct {
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
}

func NewUnsetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &UnsetOptions{
		IO:           f.IOStreams,
		Prompter:     f.Prompter,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.AgentScope,
		ResolveAgent: f.ResolveAgent,
	}

	cmd := &cobra.Command{
		Use:   "unset [agent] --name <config> [--env <env>]",
		Short: "Remove an LLM provider binding from an agent",
		Long: "Without --env, deletes the whole LLM configuration. With --env, " +
			"removes only that environment's binding and preserves the rest.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, proj, err := opts.ResolveScope(cmd, true, true)
			if err != nil {
				return render.Error(opts.IO, opts.MakeScope(org, proj, ""), err)
			}
			agent, _, agentErr := opts.ResolveAgent(args)
			scope := opts.MakeScope(org, proj, agent)
			if agentErr != nil {
				return render.Error(opts.IO, scope, agentErr)
			}
			opts.Org, opts.Proj, opts.Scope = org, proj, scope
			opts.AgentName = agent
			return runUnset(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Name, "name", "", "Name of the LLM configuration (required)")
	cmd.Flags().StringVar(&opts.Env, "env", "", "Environment binding to remove (default: remove the whole configuration)")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Skip confirmation prompt")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func runUnset(ctx context.Context, o *UnsetOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if o.Env == "" && !o.Yes {
		if !o.IO.CanPrompt() {
			return render.Error(o.IO, o.Scope, clierr.New(clierr.ConfirmationRequired, "deletion requires --yes when stdin is not a terminal"))
		}
		if err := o.Prompter.ConfirmDeletion(o.Name); err != nil {
			return render.Error(o.IO, o.Scope, clierr.Newf(clierr.ConfirmationRequired, "%v", err))
		}
	}

	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	id, found, err := findLLMConfigByName(ctx, client, o.Org, o.Proj, o.AgentName, o.Name)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if !found {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.NotFound, "no LLM configuration named %q for agent %q", o.Name, o.AgentName))
	}

	if o.Env == "" {
		return runUnsetDelete(ctx, o, client, id)
	}
	return runUnsetEnv(ctx, o, client, id)
}

func runUnsetDelete(ctx context.Context, o *UnsetOptions, client *amsvc.ClientWithResponses, id openapi_types.UUID) error {
	resp, err := client.DeleteAgentModelConfigWithResponse(ctx, o.Org, o.Proj, o.AgentName, id)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.HTTPResponse == nil || resp.HTTPResponse.StatusCode != http.StatusNoContent {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse,
			cmdutil.FirstNonNil(resp.JSON400, resp.JSON404, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, UnsetResult{Name: o.Name, Deleted: true})
	}
	cs := o.IO.StderrColorScheme()
	fmt.Fprintf(o.IO.ErrOut, "%s deleted LLM config %q\n", cs.SuccessIcon(), o.Name)
	return nil
}

func runUnsetEnv(ctx context.Context, o *UnsetOptions, client *amsvc.ClientWithResponses, id openapi_types.UUID) error {
	current, err := client.GetAgentModelConfigWithResponse(ctx, o.Org, o.Proj, o.AgentName, id)
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if current.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(current.HTTPResponse,
			cmdutil.FirstNonNil(current.JSON400, current.JSON404, current.JSON500)))
	}

	merged := modelconfig.MergeExistingEnvMappings(current.JSON200, toEnvModelConfigRequest)
	if _, ok := merged[o.Env]; !ok {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.NotFound, "environment %q is not bound on LLM config %q", o.Env, o.Name))
	}
	if len(merged) == 1 {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.InvalidFlag,
			"environment %q is the only binding on LLM config %q; drop --env to delete the whole configuration", o.Env, o.Name))
	}
	delete(merged, o.Env)

	resp, err := client.UpdateAgentModelConfigWithResponse(ctx, o.Org, o.Proj, o.AgentName, id,
		amsvc.UpdateAgentModelConfigRequest{EnvMappings: &merged})
	if err != nil {
		return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON200 == nil {
		return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse,
			cmdutil.FirstNonNil(resp.JSON400, resp.JSON404, resp.JSON500)))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, resp.JSON200)
	}
	cs := o.IO.StderrColorScheme()
	fmt.Fprintf(o.IO.ErrOut, "%s removed environment %q from LLM config %q\n", cs.SuccessIcon(), o.Env, o.Name)
	return nil
}
