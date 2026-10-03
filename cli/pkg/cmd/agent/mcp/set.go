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

package mcp

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmd/agent/modelconfig"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

type SetOptions struct {
	IO           *iostreams.IOStreams
	Client       func(context.Context) (*amsvc.ClientWithResponses, error)
	ResolveScope func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope    func(org, proj, agent string) render.Scope
	ResolveAgent func([]string) (string, []string, error)

	Org       string
	Proj      string
	Scope     render.Scope
	AgentName string

	Name        string
	Env         string
	Proxy       string
	URLEnv      string
	APIKeyEnv   string
	Description string
}

func NewSetCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &SetOptions{
		IO:           f.IOStreams,
		Client:       f.AgentManager,
		ResolveScope: f.ResolveOrgProject,
		MakeScope:    f.AgentScope,
		ResolveAgent: f.ResolveAgent,
	}

	cmd := &cobra.Command{
		Use:   "set [agent] --name <config> --env <env> --proxy <handle>",
		Short: "Bind an MCP proxy to an agent's environment",
		Long: "Create or update an agent's MCP proxy configuration for a single " +
			"environment. Other environments on the same configuration are preserved.",
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
			return runSet(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.Name, "name", "", "Name of the MCP configuration (required)")
	cmd.Flags().StringVar(&opts.Env, "env", "", "Environment to bind the proxy in (required)")
	cmd.Flags().StringVar(&opts.Proxy, "proxy", "", "Handle of a configured MCP proxy (required)")
	cmd.Flags().StringVar(&opts.URLEnv, "url-env", "", "Env var name for the proxy URL (default: auto-generated)")
	cmd.Flags().StringVar(&opts.APIKeyEnv, "apikey-env", "", "Env var name for the proxy API key (default: auto-generated)")
	cmd.Flags().StringVar(&opts.Description, "description", "", "Optional description for the configuration")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("env")
	_ = cmd.MarkFlagRequired("proxy")
	return cmd
}

func runSet(ctx context.Context, o *SetOptions) error {
	if err := cmdutil.ValidatePathParam("agent name", o.AgentName); err != nil {
		return render.Error(o.IO, o.Scope, err)
	}
	if o.URLEnv != "" {
		if err := cmdutil.ValidateEnvVarName("--url-env", o.URLEnv); err != nil {
			return render.Error(o.IO, o.Scope, err)
		}
	}
	if o.APIKeyEnv != "" {
		if err := cmdutil.ValidateEnvVarName("--apikey-env", o.APIKeyEnv); err != nil {
			return render.Error(o.IO, o.Scope, err)
		}
	}
	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	id, found, err := findMCPConfigByName(ctx, client, o.Org, o.Proj, o.AgentName, o.Name)
	if err != nil {
		return render.Error(o.IO, o.Scope, err)
	}

	mapping := amsvc.EnvModelConfigRequest{ProxyName: modelconfig.StringPtr(o.Proxy)}
	envVars := modelconfig.BuildEnvVars(o.URLEnv, o.APIKeyEnv)

	var result *amsvc.AgentModelConfigResponse
	var verb string
	if !found {
		body := amsvc.CreateAgentModelConfigRequest{
			Name:                 o.Name,
			Type:                 amsvc.CreateAgentModelConfigRequestTypeMcp,
			EnvMappings:          map[string]amsvc.EnvModelConfigRequest{o.Env: mapping},
			EnvironmentVariables: envVars,
			Description:          modelconfig.OptionalString(o.Description),
		}
		resp, err := client.CreateAgentMCPConfigWithResponse(ctx, o.Org, o.Proj, o.AgentName, body)
		if err != nil {
			return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
		}
		if resp.JSON201 == nil {
			return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse,
				cmdutil.FirstNonNil(resp.JSON400, resp.JSON401, resp.JSON404, resp.JSON409, resp.JSON500)))
		}
		result, verb = resp.JSON201, "created"
	} else {
		// Read-merge-write: PUT replaces the entire envMappings set, so we must
		// re-send every existing environment alongside the one we are changing.
		current, err := client.GetAgentMCPConfigWithResponse(ctx, o.Org, o.Proj, o.AgentName, id)
		if err != nil {
			return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
		}
		if current.JSON200 == nil {
			return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(current.HTTPResponse,
				cmdutil.FirstNonNil(current.JSON400, current.JSON404, current.JSON500)))
		}
		merged := modelconfig.MergeExistingEnvMappings(current.JSON200, toEnvModelConfigRequest)
		merged[o.Env] = mapping

		body := amsvc.UpdateAgentModelConfigRequest{EnvMappings: &merged}
		if envVars != nil {
			body.EnvironmentVariables = envVars
		}
		if d := modelconfig.OptionalString(o.Description); d != nil {
			body.Description = d
		}
		resp, err := client.UpdateAgentMCPConfigWithResponse(ctx, o.Org, o.Proj, o.AgentName, id, body)
		if err != nil {
			return render.Error(o.IO, o.Scope, clierr.Newf(clierr.Transport, "%v", err))
		}
		if resp.JSON200 == nil {
			return render.Error(o.IO, o.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse,
				cmdutil.FirstNonNil(resp.JSON400, resp.JSON404, resp.JSON500)))
		}
		result, verb = resp.JSON200, "updated"
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, o.Scope, result)
	}
	cs := o.IO.StderrColorScheme()
	fmt.Fprintf(o.IO.ErrOut, "%s %s MCP config %q: bound proxy %q to environment %q\n",
		cs.SuccessIcon(), verb, o.Name, o.Proxy, o.Env)
	return nil
}
