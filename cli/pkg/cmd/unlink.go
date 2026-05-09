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

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

type UnlinkOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)
}

type UnlinkResult struct {
	Dir string `json:"dir"`
}

func NewUnlinkCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &UnlinkOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}
	return &cobra.Command{
		Use:   "unlink",
		Short: "Remove the project link for the current directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUnlink(opts)
		},
	}
}

func runUnlink(o *UnlinkOptions) error {
	scope := render.Scope{}

	cfg, err := o.Config()
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigNotLoaded, "%v", err))
	}
	scope.Instance = cfg.CurrentInstance

	wd, err := os.Getwd()
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.Internal, "get working directory: %v", err))
	}

	linkedDir, lp := cfg.GetLinkedProject(wd)
	if lp == nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.NotLinked, "no linked project in %s", wd))
	}

	cfg.UnlinkProject(linkedDir)
	if err := cfg.Save(); err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigSaveFailed, "save config: %v", err))
	}

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, scope, UnlinkResult{Dir: linkedDir})
	}

	cs := o.IO.StderrColorScheme()
	fmt.Fprintf(o.IO.ErrOut, "%s Unlinked %s\n", cs.SuccessIcon(), cs.Bold(linkedDir))
	return nil
}
