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

// InstallOptions holds the resolved inputs for the install command.
type InstallOptions struct {
	IO      *iostreams.IOStreams
	HomeDir string
	DestDir string
	// FetchFS returns the source fs.FS of available skills. Defaults to
	// skills.Remote against the canonical GitHub tarball; tests override.
	FetchFS func(ctx context.Context) (fs.FS, error)
}

// NewInstallCmd builds the `amctl skills install` command.
func NewInstallCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &InstallOptions{
		IO: f.IOStreams,
		FetchFS: func(ctx context.Context) (fs.FS, error) {
			return skills.Remote(ctx, f.HTTPClient())
		},
	}
	return &cobra.Command{
		Use:   "install",
		Short: "Install AI assistant skills to disk",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return render.Error(opts.IO, render.Scope{},
					clierr.Newf(clierr.SkillInstallFailed, "resolve home dir: %v", err))
			}
			opts.HomeDir = home
			opts.DestDir = filepath.Join(home, skills.DefaultDestRel)
			return runInstall(cmd.Context(), opts)
		},
	}
}

func runInstall(ctx context.Context, opts *InstallOptions) error {
	scope := render.Scope{}

	if err := os.MkdirAll(opts.DestDir, 0o755); err != nil {
		return render.Error(opts.IO, scope,
			clierr.Newf(clierr.SkillInstallFailed, "create dest dir: %v", err))
	}

	fsys, err := opts.FetchFS(ctx)
	if err != nil {
		return render.Error(opts.IO, scope,
			clierr.Newf(clierr.SkillInstallFailed, "fetch remote skills: %v", err))
	}

	toolDirs := skills.DetectToolDirs(opts.HomeDir)

	result, err := skills.Install(ctx, fsys, opts.DestDir, toolDirs)
	if err != nil {
		return render.Error(opts.IO, scope,
			clierr.Newf(clierr.SkillInstallFailed, "%v", err))
	}

	if opts.IO.JSON {
		return render.JSONSuccess(opts.IO, scope, result)
	}

	cs := opts.IO.StderrColorScheme()
	w := opts.IO.ErrOut

	fmt.Fprintln(w, "Installing skills...")
	fmt.Fprintln(w)

	for _, s := range result.Skills {
		fmt.Fprintf(w, "  %s Extracted %s to %s\n", cs.SuccessIcon(), s.Name, s.Path)
	}

	if len(toolDirs) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Detecting tool directories...")
		fmt.Fprintln(w)
		for _, td := range toolDirs {
			fmt.Fprintf(w, "  %s Found %s\n", cs.SuccessIcon(), td)
		}
	}

	if len(result.Links) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Linking skills...")
		fmt.Fprintln(w)
		for _, l := range result.Links {
			fmt.Fprintf(w, "  %s Linked %s → %s\n", cs.SuccessIcon(), l.Skill, l.LinkPath)
		}
	}

	totalLocations := len(result.Skills) + len(result.Links)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Installed %d skill to %d locations.\n", len(result.Skills), totalLocations)

	return nil
}
