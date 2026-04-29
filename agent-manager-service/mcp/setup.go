package mcp

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/mcp/tools"
)

// Dependencies holds the services needed by MCP toolsets.
// Fields are added as toolsets are introduced in later.
type Dependencies struct {
}

// RegisterRoute builds the MCP HTTP handler, wraps it with the standard middleware chain,
// and registers it on the given mux at /mcp.
func RegisterRoute(mux *http.ServeMux, deps Dependencies, 	authMiddleware func(http.Handler) http.Handler,
) {

	toolsets := &tools.Toolsets{

	}

	handler := NewHTTPServer(toolsets)
	mux.Handle("/mcp", authMiddleware(handler))
	mux.Handle("/mcp/", authMiddleware(handler))
}
