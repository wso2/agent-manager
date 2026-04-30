package main

import (
	"os"

	"github.com/wso2/agent-manager/internal/amctl/amcmd"
)

func main() {
	os.Exit(amcmd.Main())
}
