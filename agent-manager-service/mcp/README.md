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
| `list_projects` | Paginated list of projects within the caller's organization |
| `create_project` | Create a new project |
| `list_project_agent_pairs` | All `(project, agent)` pairs across the org with optional substring filters |

### Agents

| Tool | Purpose |
| --- | --- |
| `list_agents` | Paginated list of agents within a project |
| `create_external_agent` | Register an externally-hosted agent. Returns the agent identity, an API token (scoped to the optional `environment`; defaults to the org's only environment, required when several exist), and step-by-step instrumentation instructions for Python or Ballerina runtimes |
| `create_internal_agent_python` | Create a platform-managed Python agent: source repo, branch, app path, optional config schema, env vars. Triggers the initial build automatically |

### Builds

Internal agents only — external agents are never built by the platform.

| Tool | Purpose |
| --- | --- |
| `list_builds` | Builds for an agent, with status, image id, and timestamps |
| `get_build_details` | Detailed view of one build — steps, durations, commit, build parameters |
| `build_agent` | Trigger a fresh build from a specific commit (defaults to latest). Returns immediately; poll `get_build_details` for completion |

### Deployments

| Tool | Purpose |
| --- | --- |
| `list_deployments` | An agent's deployments across all environments, keyed by env name, with state and image |
| `deploy_agent` | Deploy a built image to the lowest environment in the pipeline. Accepts runtime env vars (plain values, sensitive flags, or references to existing secrets) and an `enable_auto_instrumentation` toggle |
| `update_deployment_state` | Transition a deployment in a specific environment — `redeploy` (active rollout) or `undeploy` (suspend) |

### Environments

| Tool | Purpose |
| --- | --- |
| `list_environments` | Paginated list of the org's environments (name, display name, production flag). Discovers valid names for tools that take an `environment` argument |

## Configuration

### docker-compose (local dev)

The relevant env vars on `agent-manager-service` are already set:

```yaml
- SERVER_PUBLIC_URL=http://localhost:9000
- OAUTH_AUTHORIZATION_SERVERS=http://thunder.amp.localhost:8080
- KEY_MANAGER_ISSUER=Agent Management Platform Local,http://thunder.amp.localhost:8080
- KEY_MANAGER_AUDIENCE=localhost,amp-publisher-*,amp-api-client,amp-console-client,am-cli,http://localhost:9000/mcp
```

### Helm

The same values live in `wso2-agent-manager/values.yaml` under
`keyManager.audience` and `serverPublicURL`. The OAuth client itself is
registered by `wso2-amp-thunder-extension/templates/amp-thunder-bootstrap.yaml`
(script `59-am-mcp-client.sh`).

## Adding a new tool

1. **Define an input struct** in the relevant file under `mcp/tools/`
   (e.g., `builds.go`). Use snake_case JSON tags for keys.
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
   implement it in the corresponding `mcp/handlers/*_handler.go` (which
   delegates to the existing service-layer interface).
7. **Add a test spec** in the matching `*_specs_test.go`, including the
   `permissions` field — the registration tests fail without it.
8. **Add a license header** matching `agent-manager-service/.github/copyright_header.tmpl`.

After saving, the dev-mode service hot-reloads. Refresh the MCP Server
connection (reconnect) to refresh the cached tool list.
