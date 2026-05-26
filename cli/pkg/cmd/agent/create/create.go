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

package create

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clients/traceobssvc"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

type CreateOptions struct {
	IO            *iostreams.IOStreams
	Client        func(context.Context) (*amsvc.ClientWithResponses, error)
	TraceObserver func(context.Context) (*traceobssvc.Client, error)
	ResolveScope  func(*cobra.Command, bool, bool) (string, string, error)
	MakeScope     func(string, string) render.Scope

	Org   string
	Proj  string
	Scope render.Scope

	Name        string
	DisplayName string
	Description string
	Type        string
	SubType     string

	Provisioning string

	RepoURL    string
	RepoBranch string
	RepoPath   string
	RepoSecret string

	BuildType       string
	Language        string
	LanguageVersion string
	RunCommand      string
	Dockerfile      string

	Port        int
	PortSet     bool
	BasePath    string
	OpenAPISpec string

	DisableAutoInstrumentation bool
	Env                        []string
	EnvSecret                  []string
	EnvFromSecret              []string
	ModelConfigFile            string

	modelConfig *[]amsvc.ModelConfigRequest
}

func NewCreateCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &CreateOptions{
		IO:            f.IOStreams,
		Client:        f.AgentManager,
		TraceObserver: f.TraceObserver,
		ResolveScope:  f.ResolveOrgProject,
		MakeScope:     f.Scope,
	}

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an agent",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Name = args[0]
			}
			opts.PortSet = cmd.Flags().Changed("port")

			if err := validate(opts); err != nil {
				return render.Error(opts.IO, render.Scope{}, err)
			}

			org, proj, err := opts.ResolveScope(cmd, true, true)
			scope := opts.MakeScope(org, proj)
			if err != nil {
				return render.Error(opts.IO, scope, err)
			}
			opts.Org, opts.Proj, opts.Scope = org, proj, scope

			return runCreate(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.DisplayName, "display-name", "", "Human-readable name (required)")
	cmd.Flags().StringVar(&opts.Type, "type", "", "Agent type (auto-derived from --provisioning)")
	cmd.Flags().StringVar(&opts.Description, "description", "", "Agent description")
	cmd.Flags().StringVar(&opts.SubType, "subtype", "", "Agent sub-type: chat-api or custom-api")

	_ = cmd.Flags().MarkHidden("type")

	cmd.Flags().StringVar(&opts.Provisioning, "provisioning", provisioningInternal, "Provisioning type: internal or external")
	cmd.Flags().StringVar(&opts.RepoURL, "repo-url", "", "Repository URL")
	cmd.Flags().StringVar(&opts.RepoBranch, "repo-branch", "", "Repository branch")
	cmd.Flags().StringVar(&opts.RepoPath, "repo-path", "", "Application path within the repository")
	cmd.Flags().StringVar(&opts.RepoSecret, "repo-secret", "", "Secret reference for private repositories")
	cmd.Flags().StringVar(&opts.BuildType, "build-type", "", "Build type: buildpack or docker")
	cmd.Flags().StringVar(&opts.Language, "language", "", "Language for buildpack builds")
	cmd.Flags().StringVar(&opts.LanguageVersion, "language-version", "", "Language version for buildpack builds")
	cmd.Flags().StringVar(&opts.RunCommand, "run-command", "", "Run command for buildpack builds")
	cmd.Flags().StringVar(&opts.Dockerfile, "dockerfile", "", "Dockerfile path for docker builds")
	cmd.Flags().IntVar(&opts.Port, "port", 8000, "Service port (1..65535) (custom-api only; chat-api uses a fixed port)")
	cmd.Flags().StringVar(&opts.BasePath, "base-path", "", "Base path for the service")
	cmd.Flags().StringVar(&opts.OpenAPISpec, "openapi-spec", "", "Path to OpenAPI schema within the repo")
	cmd.Flags().BoolVar(&opts.DisableAutoInstrumentation, "no-auto-instrumentation", false, "Disable automatic instrumentation")
	cmd.Flags().StringSliceVar(&opts.Env, "env", nil, "Environment variable as KEY=VALUE (repeatable)")
	cmd.Flags().StringSliceVar(&opts.EnvSecret, "env-secret", nil, "Sensitive env variable as KEY=VALUE (repeatable)")
	cmd.Flags().StringSliceVar(&opts.EnvFromSecret, "env-from-secret", nil, "Env variable from secret as KEY=SECRETNAME (repeatable)")
	cmd.Flags().StringVar(&opts.ModelConfigFile, "model-config-file", "", "Path to model config YAML/JSON file")

	_ = cmd.RegisterFlagCompletionFunc("provisioning", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{provisioningInternal, provisioningExternal}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("build-type", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{buildTypeBuildpack, buildTypeDocker}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("subtype", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{subTypeChatAPI, subTypeCustomAPI}, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func runCreate(ctx context.Context, opts *CreateOptions) error {
	if opts.ModelConfigFile != "" {
		mc, err := loadModelConfig(opts.ModelConfigFile)
		if err != nil {
			return render.Error(opts.IO, opts.Scope, clierr.Newf(clierr.InvalidFlag, "--model-config-file: %s", err))
		}
		opts.modelConfig = mc
	}

	req, err := Build(opts)
	if err != nil {
		return render.Error(opts.IO, opts.Scope, err)
	}

	client, err := opts.Client(ctx)
	if err != nil {
		return render.Error(opts.IO, opts.Scope, err)
	}

	resp, err := client.CreateAgentWithResponse(ctx, opts.Org, opts.Proj, req)
	if err != nil {
		return render.Error(opts.IO, opts.Scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.JSON202 == nil {
		return render.Error(opts.IO, opts.Scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON400, resp.JSON409, resp.JSON500)))
	}

	if !opts.IO.JSON {
		printAgentSummary(opts.IO, resp.JSON202)
	}

	if opts.Provisioning != provisioningExternal {
		if opts.IO.JSON {
			return render.JSONSuccess(opts.IO, opts.Scope, resp.JSON202)
		}
		return nil
	}

	// The agent is already created server-side, so a post-create failure (token
	// mint, trace-observer discovery) is a warning, not an error — otherwise the
	// user retries and hits 409.
	if err := runExternalPostCreate(ctx, opts, resp.JSON202, client); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "warning: agent created but failed to generate token: %v\n", err)
		if opts.IO.JSON {
			return render.JSONSuccess(opts.IO, opts.Scope, map[string]any{"agent": resp.JSON202})
		}
	}
	return nil
}

func printAgentSummary(io *iostreams.IOStreams, a *amsvc.AgentResponse) {
	cs := io.StderrColorScheme()
	fmt.Fprintf(io.ErrOut, "%s Created agent %s\n\n", cs.SuccessIcon(), a.Name)
	fmt.Fprintf(io.ErrOut, "  Name:         %s\n", a.Name)
	fmt.Fprintf(io.ErrOut, "  Display Name: %s\n", a.DisplayName)
	fmt.Fprintf(io.ErrOut, "  Type:         %s\n", a.AgentType.Type)
	if a.AgentType.SubType != nil {
		fmt.Fprintf(io.ErrOut, "  Sub-Type:     %s\n", *a.AgentType.SubType)
	}
	fmt.Fprintf(io.ErrOut, "  Provisioning: %s\n", a.Provisioning.Type)
}
