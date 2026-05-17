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
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
	"github.com/wso2/agent-manager/cli/pkg/skills"
)

// ListOptions holds the resolved inputs for the list command.
type ListOptions struct {
	IO       *iostreams.IOStreams
	DestDir  string
	ToolDirs []string
	// FetchFS returns the source fs.FS of available skills. Defaults to
	// skills.Remote against the canonical GitHub tarball; tests override.
	FetchFS func(ctx context.Context) (fs.FS, error)
}

type listData struct {
	Skills []skills.SkillInfo `json:"skills"`
}

// NewListCmd builds the `amctl skills list` command.
func NewListCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &ListOptions{
		IO: f.IOStreams,
		FetchFS: func(ctx context.Context) (fs.FS, error) {
			return skills.Remote(ctx, f.HTTPClient())
		},
	}
	return &cobra.Command{
		Use:   "list",
		Short: "List available AI assistant skills",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return render.Error(opts.IO, render.Scope{},
					clierr.Newf(clierr.SkillListFailed, "resolve home dir: %v", err))
			}
			opts.DestDir = filepath.Join(home, skills.DefaultDestRel)
			opts.ToolDirs = skills.DetectToolDirs(home)
			return runList(cmd.Context(), opts)
		},
	}
}

func runList(ctx context.Context, opts *ListOptions) error {
	scope := render.Scope{}

	if !opts.IO.JSON {
		fmt.Fprintln(opts.IO.ErrOut, "Fetching skills...")
	}

	fsys, err := opts.FetchFS(ctx)
	if err != nil {
		return render.Error(opts.IO, scope,
			clierr.Newf(clierr.SkillListFailed, "fetch remote skills: %v", err))
	}

	infos, err := skills.List(ctx, fsys, opts.DestDir, opts.ToolDirs)
	if err != nil {
		return render.Error(opts.IO, scope,
			clierr.Newf(clierr.SkillListFailed, "%v", err))
	}

	if opts.IO.JSON {
		return render.JSONSuccess(opts.IO, scope, listData{Skills: infos})
	}

	if len(infos) == 0 {
		fmt.Fprintln(opts.IO.Out, "No skills available.")
		return nil
	}

	w := opts.IO.Out
	cs := opts.IO.ColorScheme()
	for _, info := range infos {
		heading := cs.Bold(info.Name)
		if info.Path == "" {
			heading += " " + cs.Gray("(not installed)")
		}
		fmt.Fprintf(w, "%s  %s\n", heading, cs.Gray(info.Description))
		if info.Path != "" {
			fmt.Fprintf(w, "  Path: %s\n", info.Path)
		}
		for _, link := range info.ActiveLinks {
			fmt.Fprintf(w, "  %s Linked at %s\n", cs.SuccessIcon(), link)
		}
		for _, tool := range info.NativeTools {
			fmt.Fprintf(w, "  %s Active for %s (native, reads %s)\n", cs.SuccessIcon(), tool, info.Path)
		}
		fmt.Fprintln(w)
	}
	return nil
}
