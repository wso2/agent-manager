package cmdutil

import (
	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/am/render"
)

// ResolveOrgProject extracts --org / --project from cobra flags and falls back to
// the active instance's current_org. requireOrg/requireProject decide whether
// missing values should produce a render.CLIError.
func (f *Factory) ResolveOrgProject(cmd *cobra.Command, requireOrg, requireProject bool) (org, project string, err error) {
	org, _ = cmd.Flags().GetString("org")
	project, _ = cmd.Flags().GetString("project")

	if org == "" {
		if cfg, cerr := f.Config(); cerr == nil {
			if inst, ierr := cfg.Current(); ierr == nil {
				org = inst.CurrentOrg
			}
		}
	}
	if requireOrg && org == "" {
		return "", "", render.NewError(render.CodeNoOrg, "no organization (set --org or run `am login` to capture current_org)")
	}
	if requireProject && project == "" {
		return "", "", render.NewError(render.CodeNoProject, "--project is required")
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
