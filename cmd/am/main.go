package main

import (
	"github.com/wso2/agent-manager/internal/am/amcmd"
)

// Keep slim becuase main isn't importable elsewhere
func main() {
	amcmd.Main()
}
