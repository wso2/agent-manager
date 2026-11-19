# MCP Server for Agent Manager

The Agent Manager exposes a Model Context Protocol (MCP) server at `/mcp` so
MCP-capable assistants (Claude Code, etc.) can read and create platform
resources directly from the developer's workflow — no Console required.

The server speaks MCP over Streamable HTTP and is protected by the existing
JWT middleware, which means every tool call goes through the standard OAuth
2.0 authorization-code + PKCE flow against Thunder. Each tool additionally
declares the `rbac` permission(s) it requires; calls are denied unless the
token carries the matching `amp:*` scopes.

The caller's organization is always derived from the token's claims — tools
take no `org_name`/`org_handle` input, and every operation is scoped to the
org the caller authenticated against.

## Quick start with Claude Code

1. **Make sure Agent Manager is running** — `make dev-up` brings up the
   service on `http://localhost:9000`. MCP sign-in goes through ThunderID,
   so the stack needs `AUTH_MODE=idp`.

2. **Register the MCP server** in Claude Code:

   ```bash
   claude mcp add --transport http agent-manager http://localhost:9000/mcp \
     --client-id am-mcp \
     --callback-port 33418
   ```

   - `--client-id am-mcp` matches the OAuth client registered in Thunder
     (written into Thunder's bootstrap by `amp init thunder`).
   - `--callback-port 33418` pins Claude Code's local OAuth listener to a
     fixed port that matches the redirect URI registered in Thunder.

3. **Trigger a tool call** in any Claude Code session, e.g.
   *"List all projects in the default org"*. The first tool call opens a
   browser to Thunder's login page; subsequent calls reuse the cached token
   until it expires.

That's it — Claude Code now sees the platform's tools alongside its own.

## Available tools

### Projects

| Tool | Purpose |
| --- | --- |
| `list_projects` | Paginated list of projects within the caller's organization |
| `create_project` | Create a new project |
| `list_project_agent_pairs` | All `(project, agent)` pairs across the org with optional substring filters |

### Agents

| Tool | Purpose |
| --- | --- |
| `list_agents` | Paginated list of agents within a project |
| `create_external_agent` | Register an externally-hosted agent. Returns the agent identity, an API token (scoped to the optional `environment`; defaults to the org's only environment, required when several exist) |

### Environments

| Tool | Purpose |
| --- | --- |
| `list_environments` | Paginated list of the org's environments (name, display name, production flag). Discovers valid names for tools that take an `environment` argument |

### AI Gateway (`amp` only)

The `ai-gateway` module adds these tools with `rt.MCPTool` while it is active. `amp-core` serves only the tools above.

| Tool | Purpose |
| --- | --- |
| `list_llm_providers`, `get_llm_provider` | The organization's LLM providers and one provider's details |
| `add_llm_provider_guardrails`, `remove_llm_provider_guardrail` | Add or remove guardrails on all of a provider's routes |
| `list_llm_guardrails` | The guardrails the organization's gateways can enforce |
| `add_llm_provider_to_catalog` | Put a provider back in the catalog so agents can be attached to it |
| `list_mcp_proxies`, `get_mcp_proxy` | MCP proxies that publish external MCP servers through the gateway |
| `attach_llm_provider_to_agent`, `attach_mcp_proxy_to_agent` | Give an agent access to an LLM provider or an MCP proxy's tools |
| `list_deployments` | Where an agent is reached and its gateway deployment status |

## Configuration

### docker-compose (local dev)

The relevant env vars on `amp-api` are already set in `deployments/standalone/docker-compose.yml`:

```yaml
- SERVER_PUBLIC_URL=http://localhost:9000
- OAUTH_AUTHORIZATION_SERVERS=http://thunder.amp.localhost:8080
- KEY_MANAGER_ISSUER=Agent Management Platform Local,http://thunder.amp.localhost:8080
- KEY_MANAGER_AUDIENCE=urn:wso2:amp,localhost,amp-api-client,amp-console-client,amctl,am-mcp,http://localhost:9000/mcp
```

The OAuth client itself comes from `ampinit/templates/thunder/bootstrap/59-am-mcp-client.yaml`, rendered by `amp init thunder`.

## Adding a new tool

The core's tools (projects, agents, environments) live in `mcp/tools/` with handlers in `mcp/handlers/`; the steps below are for those. A module adds its tools with `rt.MCPTool` instead (see [modules.md](../../docs/amp-lite/modules.md#mcp-tools)); the gateway's are in `modules/aigateway/gwmcp/`.

1. **Define an input struct** in the relevant file under `mcp/tools/`
   (e.g., `projects.go`). Use snake_case JSON tags for keys.
2. **Define a typed output struct** in the same file. Avoid
   `map[string]any` — typed structs give MCP clients a stable schema.
3. **Pick the tool's permission(s)** from `rbac/permissions.go` — mirror the
   permission the equivalent REST route declares in `api/*_routes.go`. Every
   tool call is authorized against the caller's token scopes; a tool
   registered without permissions panics at startup, and one registered by
   bypassing `addTool` is denied on every call (fail-closed).
4. **Register the tool** inside the package's `register*Tools` function using
   `addTool(reg, server, tool, handler, perms...)`. Provide a clear
   description; the LLM relies on it to decide when to call the tool.
5. **Implement the handler closure** — validate input, resolve the caller's
   org via `resolveOUID` (never from tool input), call into the toolset
   handler interface, format the output struct, and return via
   `handleToolResult`.
6. **Wire the toolset interface method** in `mcp/tools/types.go` and
   implement it in the corresponding `mcp/handlers/*_handler.go`, or in the
   module's handler package for a module tool (it delegates to the existing
   service-layer interface).
7. **Add a test spec** in the matching `*_specs_test.go`, including the
   `permissions` field — the registration tests fail without it.
8. **Add a license header** matching `agent-manager-service/.github/copyright_header.tmpl`.

After saving, the dev-mode service hot-reloads. Refresh the MCP Server
connection (reconnect) to refresh the cached tool list.
