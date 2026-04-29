package cmd

import (
	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/am/cmdutil"
)

func NewRootCmd(f *cmdutil.Factory) (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:           "am",
		Short:         "Interact with Agent Manager via CLI",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.PersistentFlags().String("org", "", "Override the active organization for this command")
	cmd.PersistentFlags().String("project", "", "Project to operate on (required for project-scoped commands)")

	cmd.AddCommand(NewLoginCmd(f))

	return cmd, nil
}
