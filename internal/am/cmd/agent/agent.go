package agent

import (
	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/am/cmdutil"
)

func NewAgentCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agents in a project",
	}
	cmd.PersistentFlags().String("project", "", "Project to operate on (required for project-scoped commands)")
	cmd.AddCommand(NewListCmd(f))
	cmd.AddCommand(NewGetCmd(f))
	cmd.AddCommand(NewDeleteCmd(f))
	return cmd
}
