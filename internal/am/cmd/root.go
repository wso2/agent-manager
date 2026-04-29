package cmd

import (
	"github.com/spf13/cobra"
)

func NewRootCmd() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "am",
		Short: "Interact with Agent Manager via CLI",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			// Do Stuff Here
		},
	}

	cmd.AddCommand(NewLoginCmd())

	return cmd, nil
}
