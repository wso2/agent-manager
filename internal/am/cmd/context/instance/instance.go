package instance

import (
	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/am/cmdutil"
)

func NewInstanceCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "instance",
		Short: "Manage configured instances",
	}
	cmd.AddCommand(NewListCmd(f))
	cmd.AddCommand(NewUseCmd(f))
	cmd.AddCommand(NewRemoveCmd(f))
	return cmd
}
