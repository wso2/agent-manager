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

package cmdutil

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

// ResolveOrgProject returns (org, project) using this fallback chain:
//  1. --org / --project flags
//  2. linked project for the current working directory (or its ancestor)
//  3. current instance's current_org (org only)
//
// requireOrg/requireProject promote a missing value to a clierr.CLIError.
func (f *Factory) ResolveOrgProject(cmd *cobra.Command, requireOrg, requireProject bool) (org, project string, err error) {
	org, _ = cmd.Flags().GetString("org")
	project, _ = cmd.Flags().GetString("project")

	cfg, _ := f.Config()
	if cfg != nil && (org == "" || project == "") {
		if wd, wdErr := os.Getwd(); wdErr == nil {
			if _, lp := cfg.GetLinkedProject(wd); lp != nil {
				if org == "" {
					org = lp.Org
				}
				if project == "" {
					project = lp.Project
				}
			}
		}
	}

	if cfg != nil && org == "" {
		if inst, ierr := cfg.Current(); ierr == nil {
			org = inst.CurrentOrg
		}
	}
	if requireOrg && org == "" {
		return "", "", clierr.New(clierr.NoOrg, "no organization (set --org, run `amctl link`, or run `amctl login`)")
	}
	if requireProject && project == "" {
		return "", "", clierr.New(clierr.NoProject, "no project (set --project or run `amctl link`)")
	}
	return org, project, nil
}

// Scope builds a render envelope scope from the factory's config and the
// resolved org/project values.
func (f *Factory) Scope(org, project string) render.Scope {
	instance := ""
	if cfg, err := f.Config(); err == nil && cfg != nil {
		instance = cfg.CurrentInstance
	}
	return render.Scope{
		Instance: instance,
		Org:      org,
		Project:  project,
	}
}
