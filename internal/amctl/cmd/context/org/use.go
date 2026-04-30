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

package org

import (
	"context"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/internal/amctl/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/amctl/clierr"
	"github.com/wso2/agent-manager/internal/amctl/cmdutil"
	"github.com/wso2/agent-manager/internal/amctl/config"
	"github.com/wso2/agent-manager/internal/amctl/iostreams"
	"github.com/wso2/agent-manager/internal/amctl/render"
)

type UseOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)
	Client func(context.Context) (*amsvc.ClientWithResponses, error)

	Name string
}

type UseResult struct {
	Org string `json:"org"`
}

func NewUseCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &UseOptions{
		IO:     f.IOStreams,
		Config: f.Config,
		Client: f.AgentManager,
	}
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Set the active organization",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]
			return runUse(cmd.Context(), opts)
		},
	}
}

func runUse(ctx context.Context, o *UseOptions) error {
	scope := render.Scope{}

	cfg, err := o.Config()
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigNotLoaded, "%v", err))
	}
	if cfg.CurrentInstance == "" {
		return render.Error(o.IO, scope, clierr.New(clierr.NoInstance, "no instance configured"))
	}
	scope.Instance = cfg.CurrentInstance

	if err := cmdutil.ValidatePathParam("org name", o.Name); err != nil {
		return render.Error(o.IO, scope, err)
	}

	client, err := o.Client(ctx)
	if err != nil {
		return render.Error(o.IO, scope, err)
	}

	resp, err := client.GetOrganizationWithResponse(ctx, o.Name)
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.Transport, "%v", err))
	}
	if resp.HTTPResponse == nil || resp.HTTPResponse.StatusCode != http.StatusOK {
		return render.Error(o.IO, scope, cmdutil.ErrorFromServer(resp.HTTPResponse, cmdutil.FirstNonNil(resp.JSON404, resp.JSON500)))
	}

	inst := cfg.Instances[cfg.CurrentInstance]
	inst.CurrentOrg = o.Name
	cfg.Instances[cfg.CurrentInstance] = inst

	if err := cfg.Save(); err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigSaveFailed, "save config: %v", err))
	}

	scope.Org = o.Name

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, scope, UseResult{Org: o.Name})
	}

	cs := o.IO.StderrColorScheme()
	fmt.Fprintf(o.IO.ErrOut, "%s Switched to organization %s\n", cs.SuccessIcon(), o.Name)
	return nil
}
