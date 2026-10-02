# agent-manager-observer — agent guide

Small Go service that serves the console's trace/observability API. It is **not** an OpenSearch client — it is an adapter that calls an upstream **Observer service** over HTTP, then enriches and reshapes the responses. Built on the **stdlib `net/http`** (no Gin), structured logging with **`slog`**, and only one third-party dep: `github.com/golang-jwt/jwt/v5`.

## Request flow

```text
HTTP → RequestLogger → CORS → mux ┬→ /health                             (no auth)
                                   ├→ /.well-known/oauth-protected-resource (no auth)
                                   ├→ JWTAuth → /api/v1/*  → handlers/ → controllers/ → observer/ (HTTP client) → Observer service
                                   │                                                 ↘ opensearch/ (span parsing / enrichment)
                                   └→ JWTAuth → /mcp, /mcp/ → mcp/tools/ → controllers/ (same as above, no HTTP hop)
```

`RequestLogger` and `CORS` wrap the whole server. `JWTAuth` wraps both the `apiMux` mounted at `/api/v1/*` and the `am-obs-mcp` streamable-HTTP MCP server mounted at `/mcp` and `/mcp/` — both on the root mux (`app/app.go`). `/health` and the well-known route are registered on the bare mux and are **unauthenticated**. Unlike `/api/v1/logs`, `/api/v1/build-logs` and `/api/v1/metrics`, `/mcp` is **not** wrapped by `middleware.RejectPublisherAudience` — publisher-audience tokens may call it.

- **`handlers/`** — parse/validate the request, extract path params, call the controller, write the response. Client-facing errors are generic (`"Failed to retrieve …"`); real detail is logged server-side.
- **`controllers/`** — orchestration + enrichment. Fetches trace overviews, then fans out to fetch span details and aggregates input/output/token usage.
- **`observer/`** — the typed HTTP client to the upstream Observer (`QueryTraces`, `QueryTraceSpans`, `GetSpanDetails`), plus auth token management and response→`opensearch.Span` conversion.
- **`opensearch/`** — pure span-parsing logic. Extracts input/output from spans; branches by vendor (CrewAI via `crewai.*` attributes vs LangChain/Traceloop via `traceloop.entity.*`).
- **`config/`** — env-var config loading + startup validation.
- **`mcp/`** — the `am-obs-mcp` streamable-HTTP MCP server (`mcp/setup.go`) and its seven tools (`mcp/tools/`): `get_runtime_logs`, `get_build_logs`, `get_metrics`, `list_traces`, `get_traces`, `get_trace_details`, `get_span_details`. Tool handlers call `controllers.TracingController`/`controllers.ObservabilityController` directly — no HTTP hop, no claims parsing. Every tool takes an explicit, required `organization` input except `get_span_details` (its controller call is scoped by trace/span ID alone).

## File map

| Need | Location |
|---|---|
| Entry point (client-credentials token source) | `main.go` |
| Server + route table, shared with cloud via `app.Run` | `app/app.go` |
| HTTP handlers, path parsing | `handlers/handlers.go` |
| Orchestration + enrichment | `controllers/controller.go` |
| Observer client (interface + impl) | `observer/client.go`, `observer/types.go` |
| Observer auth (token cache + retry) | `observer/auth.go` |
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
| `KEY_MANAGER_JWKS_URL` | `cfg.Auth.JWKSUrl` | JWKS endpoint for JWT validation |
| `KEY_MANAGER_ISSUER` | `cfg.Auth.Issuer` (list) | accepted token issuers (comma-separated) |
| `KEY_MANAGER_AUDIENCE` | `cfg.Auth.Audience` (list) | accepted audiences (comma-separated) |
| `IS_LOCAL_DEV_ENV` | `cfg.Auth.IsLocalDevEnv` | `true` skips JWT signature validation (checks expiry only) |
| `LOG_LEVEL` | `cfg.LogLevel` | DEBUG/INFO/WARN/ERROR |

The four `cfg.Observer.*` fields are required together once `OPENCHOREO_OBSERVER_URL` is set; the `cfg.Auth.*` fields are required unless `IsLocalDevEnv` is true.

Co-dependent values are checked together — a partially-set Observer or Auth config fails at startup, not first request. `getEnvAsList` parses comma-separated issuer/audience lists.

## Auth

`middleware.JWTAuth` validates `Authorization: Bearer` against JWKS (RSA), caching keys in-memory with a 1-hour TTL and force-refreshing on `kid` mismatch. It checks issuer, audience, and a publisher-audience regex (`amp-publisher-[a-zA-Z0-9][a-zA-Z0-9._-]*`). In `IS_LOCAL_DEV_ENV=true` mode signature checks are skipped.

The **observer client** holds its own OAuth2 client-credentials token (cached with a 30s expiry buffer). On a `401` it invalidates the token and retries once, falling back from Basic-auth to POST-form auth for Keycloak compatibility.

### ⚠️ Known gap: no caller-org authorization

`middleware.JWTAuth` only validates the token itself (signature/issuer/audience). It does **not** cross-check the token's org against the requested organization. The handlers read the target org straight from the query string (`organization := query.Get("organization")` in `handlers/handlers.go`) into `controllers.TraceQueryParams.Organization`, which becomes the OpenSearch `Namespace`. **Any valid token can therefore query any organization's traces** — there is no tenant isolation in the request path today.

The intended rule (not yet enforced): the caller's org identity from the JWT must match (or be authorized for) the `organization` query param before the trace query runs. If you add multi-tenant enforcement, do it in the handlers right after reading `organization`, before constructing `TraceQueryParams` — reject with `403` on mismatch. Until then, treat this service as trusting its network boundary for tenant isolation.

## Engineering rules (as practiced here)

- **Errors** — wrap with a `pkg.Func:` prefix and `%w`: `fmt.Errorf("observer.QueryTraces: %w", err)`.
- **Context** — every I/O call takes `context.Context` first and propagates it. **Never pass `nil` as the context** — always pass `r.Context()` or a context derived from it (`context.WithCancel`/`WithTimeout`); a `nil` context panics downstream.
- **Logging** — `slog` (JSON). Use the request-scoped logger via `logger.WithLogger`/`GetLogger(ctx)`. The request logger currently attaches `method`, `path`, `remote_addr`, `status`, `duration`. Per the platform rule, request-scoped logs should carry correlation context — when you log inside a trace query, add the identifiers you have (`organization`, trace/span IDs, request ID) so entries are traceable. Upstream partial failures are logged as warnings, not fatal.
- **Concurrency in enrichment** — the controller uses a **two-tier semaphore** (outer: max 10 concurrent traces; inner: max 50 concurrent span fetches). Do not collapse to one pool — it prevents deadlock. A filtered list checks the root-only filters (`status`, `conversationId`, via `matchesRootFilters`) right after the root fetch, so a rejected trace costs one call and skips the rest of the cascade. A `model` filter is checked as soon as the span list gives the models (`matchesModel`), before the child and leaf fetches; traces over the 100-span threshold have no models, so it rejects them after the root fetch. Enrichment short-circuits once fields are filled, skips leaf aggregation above 100 spans, and caps leaf fetches at 50 (`TokenUsage.Partial=true` when truncated). With `include=models` (or a `model` filter) the span list is fetched with inline attributes below the 100-span threshold; the chain and leaf steps then read their spans from it instead of calling `GetSpanDetails`, and, unless a root filter is set, the root comes from it too, so a trace costs one span-list call. The leaf cap still applies to that aggregation, so a row's tokens match with the flag on or off. The export path uses `context.WithCancel` + `atomic.Bool` for fail-fast.

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
