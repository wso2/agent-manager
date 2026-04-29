package amcmd

import "github.com/wso2/agent-manager/internal/am/cmd"

// Main entry point and setup for config, auth etc.
func Main() int {
	cmd, err := cmd.NewRootCmd()

	if err != nil {
		return 1
	}
	cmd.Execute()
	return 0
}
