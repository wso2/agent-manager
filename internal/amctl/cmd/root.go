package cmd

import (
	"github.com/spf13/cobra"

	"github.com/wso2/agent-manager/internal/amctl/cmd/agent"
	amcontext "github.com/wso2/agent-manager/internal/amctl/cmd/context"
	"github.com/wso2/agent-manager/internal/amctl/cmd/project"
	"github.com/wso2/agent-manager/internal/amctl/cmdutil"
)

func NewRootCmd(f *cmdutil.Factory) (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:           "amctl",
		Short:         "Interact with Agent Manager via CLI",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return cmdutil.FlagErrorWrap(err)
	})
	cmd.PersistentFlags().String("org", "", "Override the active organization for this command")
	cmd.PersistentFlags().BoolVar(&f.IOStreams.JSON, "json", false, "Output as JSON envelopes")

	cmd.AddCommand(NewLoginCmd(f))
	cmd.AddCommand(agent.NewAgentCmd(f))
	cmd.AddCommand(amcontext.NewContextCmd(f))
	cmd.AddCommand(project.NewProjectCmd(f))

	return cmd, nil
}
