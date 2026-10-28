"""Read-only synthetic insurance MCP server. The gateway owns AgentID authorization."""

import hmac
import os
import sys
from pathlib import Path

from mcp.server.fastmcp import FastMCP
from starlette.responses import JSONResponse

# Local checkout and container both use the agent's shared policy read functions.
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "agent"))
from policy_reads import list_policies, lookup_policy


def create_app():
    key = os.environ.get("INSURANCE_MCP_UPSTREAM_KEY", "")
    if not key.strip():
        raise RuntimeError("INSURANCE_MCP_UPSTREAM_KEY is required")
    mcp = FastMCP(
        "Insurance policy tools",
        host="0.0.0.0",
        stateless_http=True,
        json_response=True,
    )
    mcp.tool()(list_policies)
    mcp.tool()(lookup_policy)
    inner = mcp.streamable_http_app()

    async def app(scope, receive, send):
        if scope["type"] == "http":
            if scope["path"] == "/health" and scope["method"] == "GET":
                await JSONResponse({"status": "ok"})(scope, receive, send)
                return
            headers = dict(scope.get("headers", []))
            supplied = headers.get(b"x-insurance-upstream-key", b"")
            if not hmac.compare_digest(supplied, key.encode()):
                await JSONResponse({"error": "Unauthorized"}, status_code=401)(
                    scope, receive, send
                )
                return
        await inner(scope, receive, send)

    return app


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(create_app(), host="0.0.0.0", port=int(os.environ.get("PORT", "8001")))
