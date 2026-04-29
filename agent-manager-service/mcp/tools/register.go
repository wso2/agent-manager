package tools

import gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

func (tools *Toolsets) Register(server *gomcp.Server) {

	if tools == nil {
		return
	}

	if tools.ProjectToolset != nil {
		tools.registerProjectTools(server)
	}
}