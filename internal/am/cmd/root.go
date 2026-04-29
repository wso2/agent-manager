package cmd

import (
	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/am/cmd/agent"
	amcontext "github.com/wso2/agent-manager/internal/am/cmd/context"
	"github.com/wso2/agent-manager/internal/am/cmd/project"
	"github.com/wso2/agent-manager/internal/am/cmdutil"
)

func NewRootCmd(f *cmdutil.Factory) (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:           "am",
		Short:         "Interact with Agent Manager via CLI",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return cmdutil.FlagErrorWrap(err)
	})
	cmd.PersistentFlags().String("org", "", "Override the active organization for this command")

	cmd.AddCommand(NewLoginCmd(f))
	cmd.AddCommand(agent.NewAgentCmd(f))
	cmd.AddCommand(amcontext.NewContextCmd(f))
	cmd.AddCommand(project.NewProjectCmd(f))

	return cmd, nil
}
