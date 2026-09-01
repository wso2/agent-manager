# MCP Server for Agent Manager Observer

The observer exposes a Model Context Protocol (MCP) server at `/mcp` so
MCP-capable assistants (Claude Code, etc.) can query traces, logs, and
metrics for deployed agents directly from the developer's workflow.

The server speaks MCP over Streamable HTTP and is protected by the existing
JWT middleware, which means every tool call goes through the standard OAuth
2.0 authorization-code + PKCE flow against Thunder.

## Quick start with Claude Code

1. **Make sure the local AMP stack is running** — the observer is deployed
   into the kind cluster's observability plane and exposed through the
   ingress at `http://traces.amp.localhost:11080`.

2. **Register the MCP server** in Claude Code:

   ```bash
   claude mcp add --transport http agent-manager-obs http://traces.amp.localhost:11080/mcp \
     --client-id am-obs-mcp \
     --callback-port 33419
   ```

   - The URL **must** be the ingress URL, not a direct port like
     `http://localhost:9098/mcp`. During the OAuth flow the MCP client
     validates that the server's advertised resource identifier
     (`SERVER_PUBLIC_URL`, served from
     `/.well-known/oauth-protected-resource/mcp`) matches the URL it was
     configured with; a mismatch fails with
     `Protected resource ... does not match expected ...` before login
     even starts.
   - `--client-id am-obs-mcp` matches the observer's dedicated public PKCE
     OAuth client registered in Thunder (provisioned automatically by the
     `wso2-amp-thunder-extension` chart). It is allowed only the OIDC
     scopes plus the four `amp:observability:*-read` scopes — the
     am-service MCP client (`am-mcp`) cannot be used here.
   - `--callback-port 33419` pins Claude Code's local OAuth listener to a
     fixed port that matches the redirect URI registered in Thunder
     (`am-mcp` uses 33418; this client uses 33419).

3. **Trigger a tool call** in any Claude Code session, e.g.
   *"List recent traces for agent X"*. The first tool call opens a browser
   to Thunder's login page; subsequent calls reuse the cached token until
   it expires.

## Available tools

### Traces

| Tool | Purpose |
| --- | --- |
| `list_traces` | Summary view of recent traces for an agent within a time window |
| `get_traces` | Traces for an agent including full span details within a time window |
| `get_trace_details` | Metadata plus span list for one trace |
| `get_span_details` | Execution details for a single span (LLM call, tool invocation, retriever lookup, …) |

### Logs and metrics

| Tool | Purpose |
| --- | --- |
| `get_runtime_logs` | Runtime logs for an agent, filterable by time window, level, sort order, or text search |
| `get_build_logs` | Step-by-step build output for a specific build of an internal agent |
| `get_metrics` | CPU/memory usage, request and limit metrics for an agent over a time range |

## Configuration

The RFC 9728 protected resource metadata that MCP clients use for OAuth
discovery is controlled by three env vars (see `.env.example`):

```bash
SERVER_PUBLIC_URL=http://traces.amp.localhost:11080
OAUTH_AUTHORIZATION_SERVERS=http://thunder.amp.localhost:8080
OAUTH_SCOPES_SUPPORTED=amp:observability:trace-read,amp:observability:log-read,amp:observability:build-log-read,amp:observability:metric-read
```

When either `SERVER_PUBLIC_URL` or `OAUTH_AUTHORIZATION_SERVERS` is unset,
`GET /.well-known/oauth-protected-resource/mcp` returns 503 and MCP clients cannot
complete OAuth discovery.

In the Helm deployment these come from
`wso2-amp-observability-extension/values.yaml` under `amObserver.publicUrl`
and `amObserver.oauth.*`; `amObserver.oauth.authorizationServers` is empty by
default and falls back to `amObserver.auth.issuer`, so overriding the issuer for
a custom domain moves both. The `am-obs-mcp` OAuth client itself is registered
by `wso2-amp-thunder-extension` (script `64-am-obs-mcp-client.sh`; see
`amObsMcpClient` in its `values.yaml`).

## Adding a new tool

Tool registration lives in `mcp/tools/` (`traces.go`, `observability.go`).
Input schemas are auto-inferred from the Go input structs via `jsonschema`
struct tags — a field is schema-required unless it has `,omitempty`. See
`agent-manager-observer/AGENTS.md` for the routing and middleware details.
