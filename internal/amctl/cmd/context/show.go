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

package context

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/amctl/clierr"
	"github.com/wso2/agent-manager/internal/amctl/cmdutil"
	"github.com/wso2/agent-manager/internal/amctl/config"
	"github.com/wso2/agent-manager/internal/amctl/iostreams"
	"github.com/wso2/agent-manager/internal/amctl/render"
)

type ShowOptions struct {
	IO     *iostreams.IOStreams
	Config func() (*config.Config, error)
}

type ShowResult struct {
	URL string `json:"url"`
	Org string `json:"org,omitempty"`
}

func NewShowCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &ShowOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}
	return &cobra.Command{
		Use:   "show",
		Short: "Show the current context",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(opts)
		},
	}
}

func runShow(o *ShowOptions) error {
	scope := render.Scope{}

	cfg, err := o.Config()
	if err != nil {
		return render.Error(o.IO, scope, clierr.Newf(clierr.ConfigNotLoaded, "%v", err))
	}

	if cfg.CurrentInstance == "" {
		return render.Error(o.IO, scope, clierr.New(clierr.NoInstance, "no instance configured"))
	}

	inst, ok := cfg.Instances[cfg.CurrentInstance]
	if !ok {
		return render.Error(o.IO, scope, clierr.Newf(clierr.NoInstance, "current instance %q not found in config", cfg.CurrentInstance))
	}

	scope.Instance = cfg.CurrentInstance
	scope.Org = inst.CurrentOrg

	if o.IO.JSON {
		return render.JSONSuccess(o.IO, scope, ShowResult{URL: inst.URL, Org: inst.CurrentOrg})
	}

	w := o.IO.Out
	cs := o.IO.ColorScheme()
	fmt.Fprintf(w, "instance:  %s\n", cs.Bold(cfg.CurrentInstance))
	fmt.Fprintf(w, "url:       %s\n", inst.URL)
	if inst.CurrentOrg != "" {
		fmt.Fprintf(w, "org:       %s\n", cs.Cyan(inst.CurrentOrg))
	}
	return nil
}
