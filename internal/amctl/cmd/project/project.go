package project

import (
	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/amctl/cmdutil"
)

func NewProjectCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects in an organization",
	}
	cmd.AddCommand(NewListCmd(f))
	cmd.AddCommand(NewGetCmd(f))
	cmd.AddCommand(NewCreateCmd(f))
	cmd.AddCommand(NewDeleteCmd(f))
	return cmd
}
