# MCP Server for Agent Manager

The Agent Manager exposes a Model Context Protocol (MCP) server at `/mcp` so
MCP-capable assistants (Claude Code, etc.) can read and create platform
resources directly from the developer's workflow — no Console required.

The server speaks MCP over Streamable HTTP and is protected by the existing
JWT middleware, which means every tool call goes through the standard OAuth
2.0 authorization-code + PKCE flow against Thunder.

## Quick start with Claude Code

1. **Make sure Agent Manager is running** — `make dev-up` brings up the
   service on `http://localhost:9000`.

2. **Register the MCP server** in Claude Code:

   ```bash
   claude mcp add --transport http agent-manager http://localhost:9000/mcp \
     --client-id am-mcp \
     --callback-port 33418
   ```

   - `--client-id am-mcp` matches the OAuth client registered in Thunder
     (provisioned automatically by the `wso2-amp-thunder-extension` chart).
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
| `list_projects` | Paginated list of projects within an organization |
| `create_project` | Create a new project |
| `list_project_agent_pairs` | All `(project, agent)` pairs across an org with optional substring filters |

### Agents

| Tool | Purpose |
| --- | --- |
| `list_agents` | Paginated list of agents within a project |
| `create_external_agent` | Register an externally-hosted agent. Returns the agent identity, an API token, and step-by-step instrumentation instructions for Python or Ballerina runtimes |
| `create_internal_agent_python` | Create a platform-managed Python agent: source repo, branch, app path, optional config schema, env vars. Triggers the initial build automatically |

### Builds

Internal agents only — external agents are never built by the platform.

| Tool | Purpose |
| --- | --- |
| `list_builds` | Builds for an agent, with status, image id, and timestamps |
| `get_build_details` | Detailed view of one build — steps, durations, commit, build parameters |
| `get_build_logs` | Build-time stdout/stderr from each pipeline stage |
| `build_agent` | Trigger a fresh build from a specific commit (defaults to latest). Returns immediately; poll `get_build_details` for completion |

### Deployments

| Tool | Purpose |
| --- | --- |
| `list_deployments` | An agent's deployments across all environments, keyed by env name, with state and image |
| `deploy_agent` | Deploy a built image to the lowest environment in the pipeline. Accepts runtime env vars (plain values, sensitive flags, or references to existing secrets) and an `enable_auto_instrumentation` toggle |
| `update_deployment_state` | Transition a deployment in a specific environment — `redeploy` (active rollout) or `undeploy` (suspend) |

## Configuration

### docker-compose (local dev)

The relevant env vars on `agent-manager-service` are already set:

```yaml
- SERVER_PUBLIC_URL=http://localhost:9000
- OAUTH_AUTHORIZATION_SERVERS=http://thunder.amp.localhost:8080
- KEY_MANAGER_ISSUER=Agent Management Platform Local,http://thunder.amp.localhost:8080
- KEY_MANAGER_AUDIENCE=localhost,amp-publisher-*,amp-api-client,amp-console-client,am-cli,http://localhost:9000/
```

### Helm

The same values live in `wso2-agent-manager/values.yaml` under
`keyManager.audience` and `serverPublicURL`. The OAuth client itself is
registered by `wso2-amp-thunder-extension/templates/amp-thunder-bootstrap.yaml`
(script `58-am-mcp-client.sh`).

## Adding a new tool

1. **Define an input struct** in the relevant file under `mcp/tools/`
   (e.g., `builds.go`). Use snake_case JSON tags for keys.
2. **Define a typed output struct** in the same file. Avoid
   `map[string]any` — typed structs give MCP clients a stable schema.
3. **Register the tool** inside the package's `register*Tools` function
   using `gomcp.AddTool` + `withToolLogging`. Provide a clear description;
   the LLM relies on it to decide when to call the tool.
4. **Implement the handler closure** — validate input, resolve org name
   via `resolveOrgName`, call into the toolset handler interface, format
   the output struct, and return via `handleToolResult`.
5. **Wire the toolset interface method** in `mcp/tools/types.go` and
   implement it in the corresponding `mcp/handlers/*_handler.go` (which
   delegates to the existing service-layer interface).
6. **Add a license header** matching `agent-manager-service/.github/copyright_header.tmpl`.

After saving, the dev-mode service hot-reloads. Refresh the MCP Server
connection (reconnect) to refresh the cached tool list.
