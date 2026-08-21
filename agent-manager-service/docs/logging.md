# Application logging

Agent Manager writes application logs to **stdout as structured JSON** (`log/slog`), alongside the audit trail documented in [audit-logging.md](audit-logging.md). This document covers the record shape, the field vocabulary, and the levels — so that a log query written for one part of the service works for the rest of it.

The design goal is a single question being answerable: *what happened while serving this request?* Every record emitted on a request path carries the same `correlation_id`, so filtering on it reconstructs the request in order, across the controller, service, repository and upstream-client layers.

## Configuration

| Setting | Values | Default |
|---|---|---|
| `LOG_LEVEL` | `DEBUG`, `INFO`, `WARN`, `ERROR` (case-insensitive) | `INFO` |

The level is resolved once at startup (`app.setupLogger`) and logged back with the raw configured value, so a typo is visible in the first few lines rather than as silently missing output later.

Records carry `source` as a single `"package/file.go:line"` string. `slog`'s default shape is a three-field object repeating the fully-qualified function name and the absolute build path, which costs ~150 bytes on every record and differs between a container and a local run; `app.compactSource` collapses it to the part you would type into an editor. To drop it entirely, remove `AddSource` from the handler options.

**Audit records ignore `LOG_LEVEL` by design.** The audit sink builds its own handler at a fixed level (`audit/sink.go`) so raising the level cannot erase the trail.

## Streams

Records that belong to a specific stream carry a `log_type` naming it. One field, one term query, one index per stream. **An ordinary application log line has no `log_type`** — absence identifies the application stream. (`slog` appends attributes rather than replacing them, so stamping a default on the request logger would put the key in every stream record twice and make the term query ambiguous.)

| `log_type` | Emitted by | Content |
|---|---|---|
| *(absent)* | Everything on a request path | The default application log |
| `request` | `middleware.WithRequestLog` | One completion record per API request |
| `upstream` | `clients/requests` | Outbound HTTP calls to OpenChoreo, Thunder, the secret manager, git providers. Its own request fields are prefixed (`upstream_method`, `upstream_host`, `upstream_url`) so they do not collide with the inbound ones inherited from the context logger. |
| `audit` | `audit/sink.go` | The audit trail — see [audit-logging.md](audit-logging.md) |
| `gorm` | `db/db.go` | GORM's own warnings, including slow queries |
| `connPool` | `db/connpool` | Connection acquisition and retry |
| `err_response` | `middleware.RecovererOnPanic` | Panics, with stack. A panic produces **two** records sharing a correlation ID: this one carries the stack, and the `request` record carries `status: 500`. |

## Record shape

```json
{"time":"2026-08-19T09:14:22.104Z","level":"INFO","source":"middleware/request_log.go:129",
 "msg":"request completed","log_type":"request","correlation_id":"7f3c…","method":"POST",
 "path":"/orgs/acme/projects/p1/agents","action":"agent.create",
 "status":201,"duration_ms":142,"bytes":318,"ou_id":"…"}
```

Fields are attached once, at the point they become known, and inherited by everything downstream:

| Attached by | Fields |
|---|---|
| `middleware.AddCorrelationID` | `correlation_id` — taken from the `x-correlation-id` request header when the caller supplies one, generated otherwise, and echoed back on the response |
| `logger.RequestLogger` | `method`, `path` |
| `middleware.WithRequestLog` | `action`, on audited routes |
| `middleware.RequireOrgMatch` | `ou_id`, `org_handle` |

Handlers and services therefore do **not** repeat these — a call site adds only what is specific to it. This is a rule, not a convention: `slog` appends attributes rather than replacing them, so a call site that sets an inherited key puts two values under it and the reader cannot tell which is which. If a record genuinely needs its own version of one — an outbound call has a method too — prefix it.

## Levels

| Level | Means | Examples |
|---|---|---|
| `Error` | The service failed and someone should look at it | upstream 5xx, database error, panic, corrupt state |
| `Warn` | Handled, expected, but worth seeing — including everything the caller got a 4xx for | invalid body, permission denied, not found, retry attempt |
| `Info` | A rare, meaningful state change; the request completion record | resource created or deleted, startup, resolved configuration |
| `Debug` | Hot-path detail | per-item loop progress, cache hits, individual SQL operations |

Two rules keep those levels meaningful:

**A rejected request is not an incident.** A malformed body is a `Warn` and a 400, not an `Error`. `ERROR` at `LOG_LEVEL=INFO` should mean the service did something wrong.

**Log once, at the boundary.** Errors are wrapped with `%w` all the way up, so the controller's single `log.Error(..., "error", err)` already prints the whole chain. A service that logs and then returns the same error logs it at `Warn` — it adds context, it does not report a second fault. One failure produces one `ERROR` line.

## Field vocabulary

Keys are **snake_case**, and one concept has one name. When adding a field, reuse the existing name:

| Concept | Key |
|---|---|
| Request correlation | `correlation_id` |
| Org (Thunder OU) | `ou_id`, `org_handle` |
| Error | `error` (never `err`) |
| Elapsed time | `duration_ms` (never a `time.Duration`) |
| HTTP outcome | `status` |
| Entity identifiers | `agent_name`, `project_name`, `env_id`, `env_name`, `provider_id`, `proxy_id`, `gateway_id`, `deployment_id` |

`sloglint` enforces the casing and the key/value pairing in CI; the concept-to-key mapping is a convention this table records.

**The caller's token subject is not logged.** Who did it is the audit trail's job (see [audit-logging.md](audit-logging.md)), which records the actor under the retention and access controls that come with being evidence. Application logs identify the *org* (`ou_id`) and the request (`correlation_id`), which is what debugging needs. A `user_id` field elsewhere in the logs is the user an admin operation is acting *on*, not the caller.

Untrusted values — anything from a request path, body, header or an upstream response — go through `utils.SanitizeForLog` (strips CR/LF and control characters, preventing forged log entries) and `utils.TruncateForLog` before being logged.

## How to trace one request

```bash
# Ask for a correlation ID you choose, so you can find it again:
curl -H 'x-correlation-id: trace-me-123' -H "Authorization: Bearer $TOKEN" \
  "$BASE_URL/orgs/acme/projects/p1/agents"

# Then read the request back, in order, across every layer:
kubectl logs deploy/agent-manager-service | jq -c 'select(.correlation_id == "trace-me-123")'

# Or just the outcomes:
kubectl logs deploy/agent-manager-service | jq -c 'select(.log_type == "request" and .status >= 500)'
```

## Writing a log call

```go
func (s *AgentService) Deploy(ctx context.Context, agentID string) error {
	log := logger.GetLogger(ctx)          // carries correlation_id, method, path, org
	...
	if err != nil {
		log.Warn("failed to deploy agent", "agent_id", agentID, "error", err)
		return fmt.Errorf("deploy agent: %w", err)   // the controller logs the ERROR
	}
	log.Info("agent deployed", "agent_id", agentID)
	return nil
}
```

Use the package-level `slog` functions only where there is genuinely no request: startup, shutdown, migrations, and long-lived background components (`app/`, `db/`, `server/`). CI enforces this — see the `forbidigo` rule in `.github/linters/.golangci.yaml`.

Background work started from a request should carry the context forward with `context.WithoutCancel(ctx)` rather than `context.Background()`, so the correlation ID survives into the cleanup path.
