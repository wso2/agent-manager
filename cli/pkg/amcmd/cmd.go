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

package amcmd

import (
	"errors"
	"fmt"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmd"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

// Main loads config, builds the root command, and executes it. Returns the
// process exit code.
func Main() int {
	io := iostreams.System()

	path, err := config.DefaultPath()
	if err != nil {
		_ = render.Error(io, render.Scope{}, clierr.Newf(clierr.ConfigNotLoaded, "%v", err))
		return 1
	}
	cfg, err := config.Load(path)
	if err != nil {
		_ = render.Error(io, render.Scope{}, clierr.Newf(clierr.ConfigNotLoaded, "%v", err))
		return 1
	}

	root, err := cmd.NewRootCmd(cmdutil.NewFactory(cfg, io))
	if err != nil {
		_ = render.Error(io, render.Scope{}, err)
		return 1
	}
	matched, err := root.ExecuteC()
	if err != nil {
		if !render.IsRendered(err) {
			_ = render.Error(io, render.Scope{}, err)
			if !io.JSON {
				fmt.Fprintln(io.ErrOut)
				fmt.Fprint(io.ErrOut, matched.UsageString())
			}
		}
		var fe *cmdutil.FlagError
		if errors.As(err, &fe) {
			return 2
		}
		return 1
	}
	return 0
}
