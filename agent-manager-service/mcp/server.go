package mcp

import (
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wso2/agent-manager/agent-manager-service/mcp/tools"
	"net/http"
)

// NewHTTPServer creates a streamable MCP HTTP server wired to the service layer.
func NewHTTPServer(toolsets *tools.Toolsets) http.Handler {
	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "agent-manager",
		Version: "0.1.0",
	}, nil)

	if toolsets != nil {
		toolsets.Register(server)
	}

	return gomcp.NewStreamableHTTPHandler(func(r *http.Request) *gomcp.Server {
		return server
	}, nil)
}
