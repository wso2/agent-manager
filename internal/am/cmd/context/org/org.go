package org

import (
	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/am/cmdutil"
)

func NewOrgCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "org",
		Short: "Manage the active organization",
	}
	cmd.AddCommand(NewListCmd(f))
	cmd.AddCommand(NewUseCmd(f))
	return cmd
}
