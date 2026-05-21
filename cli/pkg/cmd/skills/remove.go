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

package skills

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
	"github.com/wso2/agent-manager/cli/pkg/skills"
)

// RemoveOptions holds the resolved inputs for the remove command.
type RemoveOptions struct {
	IO       *iostreams.IOStreams
	DestDir  string
	ToolDirs []string
}

// NewRemoveCmd builds the `amctl skills remove` command.
func NewRemoveCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &RemoveOptions{
		IO: f.IOStreams,
	}
	return &cobra.Command{
		Use:   "remove",
		Short: "Remove installed AI assistant skills",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			destDir, toolDirs, err := skills.ResolveLocations()
			if err != nil {
				return render.Error(opts.IO, render.Scope{},
					clierr.Newf(clierr.SkillRemoveFailed, "resolve home dir: %v", err))
			}
			opts.DestDir = destDir
			opts.ToolDirs = toolDirs
			return runRemove(cmd.Context(), opts)
		},
	}
}

func runRemove(ctx context.Context, opts *RemoveOptions) error {
	scope := render.Scope{}

	result, err := skills.Remove(opts.DestDir, opts.ToolDirs)
	if err != nil {
		return render.Error(opts.IO, scope,
			clierr.Newf(clierr.SkillRemoveFailed, "%v", err))
	}

	if opts.IO.JSON {
		return render.JSONSuccess(opts.IO, scope, result)
	}

	cs := opts.IO.StderrColorScheme()
	w := opts.IO.ErrOut

	if len(result.RemovedSkills) == 0 && len(result.RemovedLinks) == 0 {
		fmt.Fprintln(w, "No skills installed.")
		return nil
	}

	fmt.Fprintln(w, "Removing skills...")
	fmt.Fprintln(w)

	for _, link := range result.RemovedLinks {
		fmt.Fprintf(w, "  %s Removed link %s\n", cs.SuccessIcon(), link)
	}
	for _, name := range result.RemovedSkills {
		fmt.Fprintf(w, "  %s Removed %s\n", cs.SuccessIcon(), name)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Removed %s and %s.\n",
		plural(len(result.RemovedSkills), "skill", "skills"),
		plural(len(result.RemovedLinks), "link", "links"),
	)

	return nil
}
