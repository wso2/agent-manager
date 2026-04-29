package agent

import (
	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/cmdutil"
)

func NewAgentCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agents in a project",
	}
	cmd.AddCommand(NewListCmd(f))
	cmd.AddCommand(NewGetCmd(f))
	cmd.AddCommand(NewDeleteCmd(f))
	return cmd
}

// firstNonNil returns the first non-nil ErrorResponse, used to pick whichever
// of the typed error variants oapi-codegen populated for a given response.
func firstNonNil(errs ...*amsvc.ErrorResponse) *amsvc.ErrorResponse {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
