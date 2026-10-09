# agent-manager-observer — agent guide

Small Go service that serves the console's trace/observability API. It is **not** an OpenSearch client — it is an adapter that calls an upstream **Observer service** over HTTP, then enriches and reshapes the responses. Built on the **stdlib `net/http`** (no Gin), structured logging with **`slog`**, and only one third-party dep: `github.com/golang-jwt/jwt/v5`.

## Request flow

```text
HTTP → RequestLogger → CORS → mux ┬→ /health                             (no auth)
                                   ├→ /.well-known/oauth-protected-resource (no auth)
                                   ├→ JWTAuth → /api/v1/*  → handlers/ → controllers/ → observer/ (HTTP client) → Observer service
                                   │                                                 ↘ opensearch/ (span parsing / enrichment)
                                   │                                                 ↘ agentmanager/ (score filters only) → agent-manager-service
                                   └→ JWTAuth → /mcp, /mcp/ → mcp/tools/ → controllers/ (same as above, no HTTP hop)
```

`RequestLogger` and `CORS` wrap the whole server. `JWTAuth` wraps both the `apiMux` mounted at `/api/v1/*` and the `am-obs-mcp` streamable-HTTP MCP server mounted at `/mcp` and `/mcp/` — both on the root mux (`app/app.go`). `/health` and the well-known route are registered on the bare mux and are **unauthenticated**. Unlike `/api/v1/logs`, `/api/v1/build-logs` and `/api/v1/metrics`, `/mcp` is **not** wrapped by `middleware.RejectPublisherAudience` — publisher-audience tokens may call it.

- **`handlers/`** — parse/validate the request, extract path params, call the controller, write the response. Client-facing errors are generic (`"Failed to retrieve …"`); real detail is logged server-side.
- **`controllers/`** — orchestration + enrichment. Fetches trace overviews, then fans out to fetch span details and aggregates input/output/token usage.
- **`observer/`** — the typed HTTP client to the upstream Observer (`QueryTraces`, `QueryTraceSpans`, `GetSpanDetails`), plus auth token management and response→`opensearch.Span` conversion.
- **`agentmanager/`** — the optional HTTP client to agent-manager-service (`TraceScores`). Only score filters use it, with the caller's bearer token.
- **`opensearch/`** — pure span-parsing logic. Extracts input/output from spans; branches by vendor (CrewAI via `crewai.*` attributes vs LangChain/Traceloop via `traceloop.entity.*`).
- **`config/`** — env-var config loading + startup validation.
- **`mcp/`** — the `am-obs-mcp` streamable-HTTP MCP server (`mcp/setup.go`) and its seven tools (`mcp/tools/`): `get_runtime_logs`, `get_build_logs`, `get_metrics`, `list_traces`, `get_traces`, `get_trace_details`, `get_span_details`. Tool handlers call `controllers.TracingController`/`controllers.ObservabilityController` directly — no HTTP hop, no claims parsing. Every tool takes an explicit, required `organization` input except `get_span_details` (its controller call is scoped by trace/span ID alone). `list_traces` and `get_traces` take the list filters as snake_case inputs (`status`, `min_duration_ms`, `min_tokens`, `min_span_count`, `model`, `conversation_id`), checked by the same `controllers` validators as the HTTP handler; `list_traces` also takes `include_models` and `cursor`.

## File map

| Need | Location |
|---|---|
| Entry point (client-credentials token source) | `main.go` |
| Server + route table, shared with cloud via `app.Run` | `app/app.go` |
| HTTP handlers, path parsing | `handlers/handlers.go` |
| Orchestration + enrichment | `controllers/controller.go` |
| Observer client (interface + impl) | `observer/client.go`, `observer/types.go` |
| Observer auth (token cache + retry) | `observer/auth.go` |
| agent-manager-service score client | `agentmanager/client.go` |
| Score filter lookup (`ScoreClient`, `filterByScore`) | `controllers/trace_scores.go` |
| Response conversion | `observer/convert.go` |
| Span parsing / vendor branches | `opensearch/process.go`, `opensearch/crewai_process.go` |
| JWT / CORS / request logging | `middleware/` |
| Config + validation | `config/config.go` |
| MCP server + route mount | `mcp/setup.go` |
| MCP tool definitions/handlers | `mcp/tools/{tools,observability,traces}.go` |

## Routing

Plain `http.ServeMux` in `app/app.go`. Dynamic segments (`/api/v1/traces/{traceId}/spans/{spanId}`) are parsed **by hand** with `strings.CutPrefix`/`Index` in the handlers — `pathSegment()` rejects any segment containing `/` (path-traversal guard). When you add a nested route, extend that manual dispatch; there is no path-param router.

`/mcp` and `/mcp/` are registered on the same root mux via `mcp.RegisterRoute(mux, deps, middleware.JWTAuth(cfg.Auth))` — a streamable-HTTP MCP server (`github.com/modelcontextprotocol/go-sdk`), not a REST route, so it isn't in `docs/openapi.yaml`. Tool input schemas are auto-inferred from the Go input structs (`jsonschema` struct tags): a field is schema-required unless it has `,omitempty`.

Middleware wraps in this order (outer→inner): `RequestLogger → CORS → JWTAuth → handler`. Keep it — CORS must see the request before auth rejects it, and the logger must wrap both.

## Commands

```bash
make run          # hot-reload via air (go install github.com/air-verse/air@latest)
make build        # go build -o agent-manager-observer .
make test         # bash scripts/run_tests.sh
make test-cover   # coverage + HTML report
make fmt          # gofmt -w .
make lint         # golangci-lint run ./...
make mock-traces  # generate OTLP traces via telemetrygen (for local testing)
```

`.air.toml` watches `.go` and `.env` files, so editing `.env` reloads the app.

## Config

`config.Load()` reads env vars with defaults and validates at startup (`config/config.go`):

| Var | Config field (`config.Load()` result) | Purpose |
|---|---|---|
| `AM_OBSERVER_PORT` | `cfg.Server.Port` | listen port (default 9098) |
| `OPENCHOREO_OBSERVER_URL` | `cfg.Observer.BaseURL` | upstream Observer base URL |
| `IDP_TOKEN_URL` | `cfg.Observer.TokenURL` | Observer OAuth2 token endpoint |
| `IDP_CLIENT_ID` | `cfg.Observer.ClientID` | Observer OAuth2 client id |
| `IDP_CLIENT_SECRET` | `cfg.Observer.ClientSecret` | Observer OAuth2 client secret |
| `OBSERVER_DEFAULT_NAMESPACE` | `cfg.Observer.DefaultNamespace` | namespace all trace queries are scoped to (default `default`) |
| `AGENT_MANAGER_SERVICE_URL` | `cfg.AgentManager.BaseURL` | optional agent-manager-service base URL for score filters; unset, they return 503 |
| `KEY_MANAGER_JWKS_URL` | `cfg.Auth.JWKSUrl` | JWKS endpoint for JWT validation |
| `KEY_MANAGER_ISSUER` | `cfg.Auth.Issuer` (list) | accepted token issuers (comma-separated) |
| `KEY_MANAGER_AUDIENCE` | `cfg.Auth.Audience` (list) | accepted audiences (comma-separated) |
| `IS_LOCAL_DEV_ENV` | `cfg.Auth.IsLocalDevEnv` | `true` skips JWT signature validation (checks expiry only) |
| `LOG_LEVEL` | `cfg.LogLevel` | DEBUG/INFO/WARN/ERROR |

The four `cfg.Observer.*` fields are required together once `OPENCHOREO_OBSERVER_URL` is set; the `cfg.Auth.*` fields are required unless `IsLocalDevEnv` is true. `AGENT_MANAGER_SERVICE_URL` is optional, but when set it must be an absolute http(s) URL. The Helm chart sets it from `amObserver.agentManagerService.url`, which defaults to the in-cluster `amp-api` service.

Co-dependent values are checked together — a partially-set Observer or Auth config fails at startup, not first request. `getEnvAsList` parses comma-separated issuer/audience lists.

## Auth

`middleware.JWTAuth` validates `Authorization: Bearer` against JWKS (RSA), caching keys in-memory with a 1-hour TTL and force-refreshing on `kid` mismatch. It checks issuer, audience, and a publisher-audience regex (`amp-publisher-[a-zA-Z0-9][a-zA-Z0-9._-]*`). In `IS_LOCAL_DEV_ENV=true` mode signature checks are skipped.

The **observer client** holds its own OAuth2 client-credentials token (cached with a 30s expiry buffer). On a `401` it invalidates the token and retries once, falling back from Basic-auth to POST-form auth for Keycloak compatibility.

The **agent-manager-service client** has no credentials of its own. It forwards the caller's bearer token (`middleware.BearerTokenFromContext`), so the service checks `amp:monitor:score-read` and scopes scores to the token's org. MCP tools take no score filters, since their tokens can't be forwarded.

### ⚠️ Known gap: no caller-org authorization

`middleware.JWTAuth` only validates the token itself (signature/issuer/audience). It does **not** cross-check the token's org against the requested organization. The handlers read the target org straight from the query string (`organization := query.Get("organization")` in `handlers/handlers.go`) into `controllers.TraceQueryParams.Organization`, which becomes the OpenSearch `Namespace`. **Any valid token can therefore query any organization's traces** — there is no tenant isolation in the request path today.

The intended rule (not yet enforced): the caller's org identity from the JWT must match (or be authorized for) the `organization` query param before the trace query runs. If you add multi-tenant enforcement, do it in the handlers right after reading `organization`, before constructing `TraceQueryParams` — reject with `403` on mismatch. Until then, treat this service as trusting its network boundary for tenant isolation.

## Engineering rules (as practiced here)

- **Errors** — wrap with a `pkg.Func:` prefix and `%w`: `fmt.Errorf("observer.QueryTraces: %w", err)`.
- **Context** — every I/O call takes `context.Context` first and propagates it. **Never pass `nil` as the context** — always pass `r.Context()` or a context derived from it (`context.WithCancel`/`WithTimeout`); a `nil` context panics downstream.
- **Logging** — `slog` (JSON). Use the request-scoped logger via `logger.WithLogger`/`GetLogger(ctx)`. The request logger currently attaches `method`, `path`, `remote_addr`, `status`, `duration`. Per the platform rule, request-scoped logs should carry correlation context — when you log inside a trace query, add the identifiers you have (`organization`, trace/span IDs, request ID) so entries are traceable. Upstream partial failures are logged as warnings, not fatal.
- **Concurrency in enrichment** — the controller uses a **two-tier semaphore** (outer: max 10 concurrent traces; inner: max 50 concurrent span fetches). Do not collapse to one pool — it prevents deadlock. See [Trace enrichment and filtering](#trace-enrichment-and-filtering).

## Trace enrichment and filtering

All in `controllers/controller.go`.

### Enrichment

- Short-circuits once the fields are filled.
- Skips leaf aggregation above **100 spans**.
- Caps leaf fetches at **50**; sets `TokenUsage.Partial=true` when it hits the cap.
- With `include=models` (or a `model` filter), below 100 spans the span list is fetched with inline attributes:
  - Chain and leaf steps read their spans from it instead of calling `GetSpanDetails`.
  - The root comes from it too, unless a root filter is set — so a trace costs one span-list call.
  - The leaf cap still applies, so a row's tokens match with the flag on or off.
- With `include=tools`, at most **200 spans** (`maxToolListSpans`), `tools` and `failedTools` come from the span list (`toolsFromSpanList`):
  - It reuses the list the cascade reads. A root-complete trace fetches it without attributes, so it costs one call; other traces cost none.
  - It reads only span names (`ToolNameFromSpanName`: `execute_tool {name}`, `{name}.tool`) and the OTel status, never attributes.
  - Limitation: spans named only after the tool (OpenInference, Logfire) aren't recognised, and a bare `execute_tool` names no tool.
- With `include=mcpServers`, at most 200 spans, `mcpServers` come from the trace's `initialize.mcp` spans (`readMCPServers`):
  - They're found by name in the same list. `handshakesToFetch` keeps one per tool: each is keyed by its parent span's tool name (`ToolNameFromSpanName`, from the same attribute-free list), and one whose parent isn't in the list or names no tool is its own key. A client that opens a session per tool call puts a handshake under every call, and calls to one tool reach one server.
  - Each kept handshake costs one `GetSpanDetails`, for the first **5** keys in start order (`maxMCPHandshakesPerTrace`), fetched after the tool and model checks and before the child and leaf steps.
  - A filtered walk also counts its handshake fetches across traces; see the per-request cap below. An unfiltered page doesn't walk, so it's bounded only per trace: up to `limit` × 5.
  - The server is `serverInfo.name` (else `title`) from the span's `traceloop.entity.output` (`MCPServerFromHandshake`); the rest of that output is skipped.
  - Limitation: only Traceloop's MCP instrumentation emits the span, and only when the agent opens a session inside the trace. An agent that holds one session for its lifetime reports no servers in later traces.

### Filtered list

Filters run as early as possible, so a rejected trace stays cheap:

- **`status`** — `matchesStatus`, right after the root fetch. A rejected trace costs one call.
- **`conversationId`** — the ID comes from the root, then the child and leaf spans (`ExtractConversationID`). Without the filter only spans enrichment reads anyway are checked, so it costs no extra call. With it, a root with another ID rejects the trace after one call. A root without one, even a complete one, leaves the trace open: another ID on the child rejects it before the leaf fetches, and a trace with no ID is read in full and left out. A failed read the ID depends on (the span list, the child, or a leaf ahead of the first ID) lists the trace as failed, not as having no ID.
- **`model`** — `matchesModel`, as soon as the span list gives the models, before the child and leaf fetches. Traces over 100 spans have no models, so they're rejected after the root fetch.
- **`tool`, `toolError`** — `matchesTools` on the tools from the span list's names and OTel statuses, never attributes. Both imply `include=tools`, and with both set, a failed tool must match `tool`.
  - Traces over `maxToolListSpans` (200) have no tools, so `matchesSummary` rejects them before any fetch.
  - With no `status` or `conversationId` filter, the span list is fetched before the root, without attributes unless `listSpansFirst` already wants them. A rejected trace costs one list call and no root fetch; only matches fetch the root and run the cascade.
  - With `status` or `conversationId`, the root comes first, then the list.
  - A failed list fetch lists the trace as failed.
- **`mcpServer`** — `matchesMCPServer` on `mcpServers`; implies `include=mcpServers`. It shares the tool filters' span cap and list-before-root order, and its handshake fetches come last, before the child and leaf steps. A trace with no handshake span is rejected after the list, at no extra call. A failed handshake fetch lists the trace as failed.
- **`minScore`, `maxScore`, `evaluator`** — `matchesScore` on the trace's mean non-skipped score (with `evaluator`, that evaluator's), read from agent-manager-service.
  - `filterByScore` makes one `TraceScores` call per chunk of up to 50 summary survivors, after `filterTraceInfos` and before `enrichTraces`, so an out-of-range trace costs no upstream call. It counts as examined, not as read or failed. A request makes at most 10 lookups, each bounded by 3 s (`scoreLookupTimeout`).
  - A trace with no score never matches.
  - Without a score client the walk fails before its first fetch (`ErrScoresNotConfigured`, 503). A failed lookup fails the request (`ErrScoreLookup`, 502; 403 when the service rejects the token), with no retry and no partial page, unless the budget cut it: then the walk stops as truncated.
  - Requests without a score filter never call the client; the cost baseline (`trace_cost_baseline_test.go`) asserts zero lookups for them.

The walk stops at the 500-trace examine cap or after **20 s** (`listLookBackBudget`), returning `truncated` and a `nextCursor`. The budget doesn't cut the first chunk; only the **25 s** request deadline (`requestTimeout`) does, failing the request. After it, the walk's fetches run under the budget, which cancels those in flight without a warning; the walk then stops before the chunk they belong to.

With `mcpServer` or `include=mcpServers`, the walk also stops before its next chunk once it has fetched **100** MCP handshakes (`maxMCPHandshakesPerRequest`), the same way: `truncated`, a `nextCursor` at the last examined trace, and `handshakeCapReached` in the walk's log line. The first chunk always runs and a chunk in flight finishes, so a request fetches at most 99 + 50 × 5 = 349 handshakes. Once the cap is reached, that chunk's failed traces aren't retried. Export selection uses the same walk. Other requests never fetch a handshake, so the cap never stops them.

A trace that can't be read is retried once, then left out. The list fails, filtered or not, when it could read none of the traces it examined. A trace a filter rejected after reading it counts as read.

### Export

- **No filter** — one `QueryTraces` call.
- **Filtered** — selects traces with `lookBackForMatches`, then fetches full spans only for the matches. Selection has no cursor, the same 500-trace examine cap and 100-handshake cap, and a **10 s** budget that leaves the rest of the 25 s `requestTimeout` for the span fetches. An export still running at `requestTimeout` fails.
- Uses the same filter criteria as the list, but the shorter budget and no paging can cover a different part of history, so it may not select the same traces.

Retried once:

- Selection: a failed root fetch, or a failed span list under a `model` or `minTokens` filter.
- Span phase: a failed `QueryTraceSpans`.

Response flags:

- `truncated` — the search stopped early, or a trace hit the **10 000** span cap.
- `spansTruncated` — the span cap only, so a client can tell the two apart.
- `failedTraceIds` — traces left out because they still failed after the retry, had no root span, or fell short of `minTokens` with leaf spans unread (past the cap or failed).

The export fails only when `ctx` is done or it could read none of the traces it selected.

## Gotchas

- Service **cannot run in isolation** — it needs a reachable Observer (`OPENCHOREO_OBSERVER_URL` + IDP creds). For local work set `IS_LOCAL_DEV_ENV=true`.
- The old README mentions OpenSearch directly; the code does **not** talk to OpenSearch — the `opensearch/` package is only span-parsing types/logic.
- Tests need env vars set at import/run time. Some existing tests (e.g. `config_test.go`) use the manual `os.Setenv` + `defer os.Unsetenv` pattern; **prefer `t.Setenv("KEY", "val")` in new tests** — it sets the var for the test and auto-restores it on cleanup (no `defer` boilerplate, and it guards against parallel misuse).

## Done checklist

- [ ] `make test` passes.
- [ ] `make lint` (`golangci-lint run ./...`) clean.
- [ ] `make fmt` applied.
- [ ] New I/O paths take and propagate `context.Context`; errors wrapped with `%w`.
- [ ] **Authorization** — any new endpoint that reads org-scoped data checks the caller's org against the requested `organization` (see the known-gap note under Auth); don't widen access without addressing it.
- [ ] **Config validation** — new config values are validated in `config.validate()` at startup, with co-dependent values checked together.
