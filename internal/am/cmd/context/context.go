package context

import (
	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/am/cmd/context/instance"
	"github.com/wso2/agent-manager/internal/am/cmd/context/org"
	"github.com/wso2/agent-manager/internal/am/cmdutil"
)

func NewContextCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "View and manage CLI context (instances, organizations)",
	}
	cmd.AddCommand(NewShowCmd(f))
	cmd.AddCommand(instance.NewInstanceCmd(f))
	cmd.AddCommand(org.NewOrgCmd(f))
	return cmd
}
