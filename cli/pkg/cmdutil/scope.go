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
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

// linkedProject returns the linked project for the current working directory.
func (f *Factory) linkedProject() *config.LinkedProject {
	cfg, _ := f.Config()
	if cfg == nil {
		return nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil
	}
	_, lp := cfg.GetLinkedProject(wd)
	return lp
}

// ResolveOrgProject returns (org, project) using this fallback chain:
//  1. --org / --project flags
//  2. linked project for the current working directory (or its ancestor)
//  3. current instance's current_org (org only)
//
// requireOrg/requireProject promote a missing value to a clierr.CLIError.
func (f *Factory) ResolveOrgProject(cmd *cobra.Command, requireOrg, requireProject bool) (org, project string, err error) {
	org, _ = cmd.Flags().GetString("org")
	project, _ = cmd.Flags().GetString("project")

	if org == "" || project == "" {
		if lp := f.linkedProject(); lp != nil {
			if org == "" {
				org = lp.Org
			}
			if project == "" {
				project = lp.Project
			}
		}
	}

	if org == "" {
		cfg, _ := f.Config()
		if cfg != nil {
			if inst, ierr := cfg.Current(); ierr == nil {
				org = inst.CurrentOrg
			}
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

// ResolveAgent returns the agent name using this fallback chain:
//  1. args[0] if present and non-empty
//  2. linked project's Agent for the current working directory (or ancestor)
//
// remaining holds the args that were not consumed as the agent name.
func (f *Factory) ResolveAgent(args []string) (agent string, remaining []string, err error) {
	if len(args) > 0 && args[0] != "" {
		return args[0], args[1:], nil
	}

	if lp := f.linkedProject(); lp != nil && lp.Agent != "" {
		return lp.Agent, args, nil
	}
	return "", nil, clierr.New(clierr.NoAgent, "agent is required")
}

// AgentScope builds a render envelope scope that includes the resolved agent.
func (f *Factory) AgentScope(org, project, agent string) render.Scope {
	s := f.Scope(org, project)
	s.Agent = agent
	return s
}

// ResolveEnvironment returns the environment using this fallback chain:
//  1. --env flag
//  2. linked project's Environment for the current working directory
func (f *Factory) ResolveEnvironment(cmd *cobra.Command) (string, error) {
	if env, _ := cmd.Flags().GetString("env"); env != "" {
		return env, nil
	}
	if cfg, _ := f.Config(); cfg != nil {
		if wd, err := os.Getwd(); err == nil {
			if _, lp := cfg.GetLinkedProject(wd); lp != nil && lp.Environment != "" {
				return lp.Environment, nil
			}
		}
	}
	return "", clierr.New(clierr.NoEnvironment, "no environment (set --env or run `amctl context link`)")
}

// EnvScope builds a render envelope scope that includes org, project, agent, and environment.
func (f *Factory) EnvScope(org, project, agent, env string) render.Scope {
	s := f.AgentScope(org, project, agent)
	s.Environment = env
	return s
}

// AddEnvFlag registers the standard --env flag on cmd.
func AddEnvFlag(cmd *cobra.Command) {
	cmd.Flags().String("env", "", "Environment name (defaults to linked project's environment)")
}
