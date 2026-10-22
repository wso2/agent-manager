# A2A Agent Catalogue — Clean-Room Specification

**Status:** binding specification for a clean-room implementation on branch `a2a-catalogue-cleanroom` (based on `upstream/main`).

**How this document came to be:** three analysts studied a prior, separate implementation of this feature and wrote down its externally observable behaviour, contracts and rules, with a set of agreed design fixes already applied. The engineers building from this document have never seen that implementation and must not look for it. Everything the feature must do is here; everything it builds on already exists on this branch. Where this document is silent, use the repository's conventions and your judgement, and record the choice.

**Three parts, one feature:**

- **Part A — Backend** (`agent-manager-service/`, `cli/`): the catalogue list, single-entry and connect-details HTTP operations, the stored card projection, published transports, environment ordering, search.
- **Part B — Console: catalogue list** (`console/`): the organisation-level "A2A Agents" page, filters, pagination, the shared filter-bar components in `libs/views` and their reuse by the traces page, routing/navigation, types and api-client hooks for the list and entry operations.
- **Part C — Console: agent page** (`console/`): the single-agent page, card states, compare, the connect drawer, quick-start snippets, test keys, shared-component card utilities, and the connect/test-key hooks.

The console parts consume exactly the JSON contract Part A defines. Field names, enum values, paths and query parameters are shared contracts and must match across parts character for character.

## Global constraints (apply to every part)

- **Go conventions:** follow `agent-manager-service/AGENTS.md` (sentinel errors matched with `errors.Is`, everything else wrapped with `fmt.Errorf("...: %w", err)`, never fall back to a default on a non-not-found error). Unit tests carry no build tag; repository/integration tests carry `//go:build integration`.
- **Go commands (run from `agent-manager-service/`):** `make test-run RUN=<pattern> PKG=./services/` for one test, `make test-unit` for the unit tier, `make test-integration` for the integration tier (an isolated Postgres database; `.env.test` is already present in this worktree), `make mocks PKG=./repositories/` (or `make codegen`) to regenerate moq mocks, `make wire` after touching `wiring/`, `make fmt` and `make lint` before committing. Pinned tools live in `<repo-root>/.tools/bin` and are on PATH inside every make recipe.
- **OpenAPI first:** any API change is made in `agent-manager-service/docs/api_v1_openapi.yaml` first, then `make spec` (from `agent-manager-service/`; uses Docker) regenerates `agent-manager-service/spec/`, and `make amctl-gen-client` (from the repo root) regenerates `cli/pkg/clients/amsvc/gen/`. Generated files are committed with the change that caused them. Use the repo's `/add-api-resource` skill workflow where available.
- **Migrations:** numbered after `048_create_a2a_proxies.go` and appended to `db_migrations/migration_list.go`; idempotent (`IF NOT EXISTS`).
- **Console conventions:** follow `console/AGENTS.md` and `console/.ai/oxygen-ui/AGENTS.md`. API calls use the two-file pattern (`libs/api-client/src/apis/` + `libs/api-client/src/hooks/`). Package manager is pnpm (`pnpm --filter <package> test`, `pnpm --filter <package> lint`). Dependent packages resolve workspace libraries from their built `dist/`, so after changing a library's exports run `pnpm --filter <library-package> run build` before testing its consumers.
- **Commits:** imperative mood, capitalised first word, no trailing period, no `feat:`-style prefix, no Co-Authored-By trailers; every commit message ends with the trailer line `Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q`. Stage explicit paths. Never push.
- **Clean room:** do not read any other checkout or worktree of this repository (anything under `/home/lyuda/agent-manager` or `/home/lyuda/agent-manager-worktrees/` other than this worktree), any stash, or any branch other than this one and `upstream/main`.

---


# PART A — BACKEND

# A2A agent catalogue — backend specification (agent-manager-service + amctl client)

Clean-room specification of the backend half of the A2A agent catalogue feature: the
org-wide catalogue list, the single catalogue entry, and the per-environment "connect
details" API, together with the card projection stored at fetch time, the recorded
published transports, and the regenerated amctl client. It describes the TARGET
behaviour: the feature as it exists on the branch with Tasks 1–5 of the structural-fixes
plan applied (Tasks 6–11 are console-only; Task 12 is out of scope).

Everything named with a file path or identifier below already exists on `upstream/main`
unless the text says it is new. New files, types, functions, constants and database
columns are described by role only; the builders choose the names.

---

## 1. Purpose and user stories

The platform already fetches and stores every A2A agent's public agent card per
environment (`a2a_agent_cards`), publishes platform and proxied-external A2A agents to
the environment's ingress gateway (`a2a_publications`), and lets an external agent be
fronted by the gateway (`a2a_proxies`). What is missing is a way to see all of an
organisation's A2A agents in one place and to know how to call one.

- As a developer in an organisation, I want to list every A2A agent in the organisation
  (across projects and environments) with each agent's name, description, version and
  skills, search it by text, and filter it by project, environment, kind and card state,
  so that I can discover agents to call.
- As a developer, I want to open one agent and see its per-environment card state, in
  deployment-pipeline order, with a flag on each environment whose card differs from the
  reference environment's card.
- As a developer, I want, for one agent in one environment, the exact endpoint to call
  (card URL and JSON-RPC URL), what security the gateway enforces (none / API key / OAuth
  with which issuers), and whether the gateway publication succeeded, so I can connect.
- As an operator, I want the catalogue to stay correct and fast: card text is indexed
  once at fetch time, not re-parsed per request; OpenChoreo trouble fails a request
  loudly rather than silently reordering environments.

---

## 2. Scope of backend changes (overview)

| Area | Change |
|---|---|
| OpenAPI (`agent-manager-service/docs/api_v1_openapi.yaml`) | Three new GET operations and eight new schemas (section 3). |
| Generated types (`agent-manager-service/spec/`) | Regenerated with `make spec`. |
| amctl client (`cli/pkg/clients/amsvc/gen/`) | Regenerated with `make amctl-gen-client` at the repo root (section 10). |
| Routes (`agent-manager-service/api/`) | A new route-registration file for the three routes, wired from `registerAPIRoutes` in `api/app.go`; the new controller constructor added to `controllerConstructors` in `api/audit_coverage_test.go`. |
| Controller / service / repository | One new controller, one new service, one new read-only repository (catalogue reads), all registered in `wiring/wire.go` provider sets and `wiring/params.go` `AppParams` (section 9). |
| Card storage | Two new columns on `a2a_agent_cards` holding a card projection (fingerprint + search text) written at fetch time (section 5); migration 049. |
| Publication storage | One new nullable JSONB column on `a2a_publications` recording the transports actually published (section 6.4); migration 050. |
| Existing code modified | `services/a2a_card_reconciler.go`, `repositories/a2a_agent_card_repository.go` (+ its mock), `models/a2a_agent_card.go`, `models/a2a_publication.go`, `repositories/a2a_publication_repository.go` (+ mock), `services/a2a_publication_reconciler.go`, `services/a2a_agent_deployment.go`, `services/a2a_publisher.go`, `services/a2a_proxy_service.go`, `clients/openchoreosvc/client/deployments.go`, `clients/openchoreosvc/client/components.go`, `utils/errors.go`, `db_migrations/migration_list.go` (section 11). |

---

## 3. OpenAPI contract

All three operations are tagged `Agent Card`, like the existing agent-card operations.
All paths are under the existing org-scoped prefix; `orgName` is a path parameter on
each. Every operation responds `401` with `ErrorResponse` when unauthenticated and
`500` with `ErrorResponse` on an internal error. `ErrorResponse` is the existing schema.

### 3.1 `GET /orgs/{orgName}/a2a-agents` — operationId `listA2AAgents`

Summary: "List the organization's A2A agent catalogue".
Description (requirement text): "Returns one entry per A2A agent in the organization,
with the per-environment state of its agent card. Results are ordered by project and
agent name."

Query parameters:

| Name | Type | Required | Default | Constraints | Description (requirement text) |
|---|---|---|---|---|---|
| `q` | string | no | — | `maxLength: 255` (counted in Unicode code points after trimming surrounding whitespace) | "Case-insensitive substring match over agent names, card names and descriptions, and skill names, descriptions, tags and examples." (This is the Task 2 wording; it replaces the branch's earlier "Free-text search over agent names, descriptions and skills".) |
| `kind` | string enum `platform`, `external`, `proxied` | no | — | | "Restrict to platform or external agents, or to proxied agents" |
| `environment` | string | no | — | | "Restrict to agents deployed in this environment (name)" |
| `project` | string | no | — | | "Restrict to this project (name)" |
| `status` | string enum `pending`, `fetched`, `failed` | no | — | | "Restrict to agents with a card in this fetch state" |
| `limit` | integer (int32) | no | 50 | minimum 1, maximum 100 | "Maximum number of agents to return" |
| `offset` | integer (int32) | no | 0 | minimum 0 | "Number of agents to skip" |

Responses:

| Code | Body | Meaning |
|---|---|---|
| 200 | `A2ACatalogListResponse` | "A page of the A2A agent catalogue" |
| 400 | `ErrorResponse` | "Invalid query parameter" |
| 401 / 500 | `ErrorResponse` | as above |

Exact 400 behaviour (controller-level validation, before the service is called; the first
failing check in this order wins):

1. `limit` present and not an integer, or `< 1`, or `> 100` → 400, message
   `Invalid limit parameter: must be between 1 and 100`.
2. `offset` present and not an integer, or `< 0` → 400, message
   `Invalid offset parameter: must be 0 or greater`.
3. `q` trimmed of leading/trailing whitespace longer than 255 code points → 400, message
   `Invalid q parameter: must be at most 255 characters`.
4. `kind` present and not one of the three enum values → 400, message
   `Invalid kind parameter: must be platform, external or proxied`.
5. `status` present and not one of the three enum values → 400, message
   `Invalid status parameter: must be pending, fetched or failed`.

An absent or empty `limit`/`offset` takes the default (50 / 0, i.e. the existing
`utils.DefaultLimit`, `utils.MinLimit`, `utils.MaxLimit`, `utils.DefaultOffset`,
`utils.MinOffset` constants). An empty `q` after trimming means "no search". `project`
and `environment` are passed through verbatim with no validation. Unknown query
parameters are ignored. The 400 body is written with the existing
`utils.WriteErrorResponse`.

### 3.2 `GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/a2a-catalog-entry` — operationId `getA2ACatalogEntry`

Summary: "Get one agent's A2A catalogue entry".
Description: "Returns the agent's entry in the organization's A2A agent catalogue, with
the per-environment state of its agent card."

Path parameters: `orgName` ("Organization name/handle"), `projName` ("Project name"),
`agentName` ("Agent name"), all strings, all required.

Responses:

| Code | Body | Meaning |
|---|---|---|
| 200 | `A2ACatalogAgent` | "The agent's catalogue entry" |
| 404 | `ErrorResponse` | "The agent is not in the A2A catalogue" — returned when the agent has no card row in any environment; response message `The agent is not in the A2A catalogue`. |
| 401 / 500 | `ErrorResponse` | as above |

### 3.3 `GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/a2a-connect` — operationId `getA2AConnect`

This path reuses the existing `agentCardPathParams` YAML anchor (the same four path
parameters as the existing `.../environments/{envID}/agent-card` operations; `envID` is
the environment name).

Summary: "Get what a client needs to connect to an A2A agent in an environment".
Description: "Returns the endpoint, security requirements and gateway publication state a
client needs to call the agent in the environment."

Responses:

| Code | Body | Meaning |
|---|---|---|
| 200 | `A2AConnectResponse` | "Connection details" |
| 404 | `ErrorResponse` | "Agent or environment not found, or the agent is not an A2A agent" |
| 401 / 500 | `ErrorResponse` | as above |

Actual error mapping (section 8.3) is done by the existing agent-card error writer, so a
missing card row yields 404 `Agent card not found`, an unknown agent 404 `Agent not
found`, and an unknown environment 404 `Environment not found`.

### 3.4 Schemas

All schemas are plain `type: object`. "Required" means the key is always present in the
JSON (never omitted); nullable fields are never emitted as `null` — absent instead.

**`A2ACatalogListResponse`** — required `agents`, `total`, `limit`, `offset`.

| Field | Type | Meaning |
|---|---|---|
| `agents` | array of `A2ACatalogAgent` | The page. Always an array, `[]` when empty. |
| `total` | integer | Number of agents (not rows) matching the filters, before paging. |
| `limit` | integer | The limit applied. |
| `offset` | integer | The offset applied. |

**`A2ACatalogAgent`** — description: "name, description, version and skills come from the
lead environment's card; name falls back to agentName when no card has been fetched."
Required: `projectName`, `agentName`, `kind`, `name`, `description`, `version`, `skills`,
`leadEnvironment`, `environments`.

| Field | Type | Meaning |
|---|---|---|
| `projectName` | string | Project the agent belongs to. |
| `agentName` | string | Agent (component) name. |
| `kind` | string enum `platform`, `external` | The card source of the agent (the existing `A2AAgentCardSource` values). Note the list filter also accepts `proxied`, but an entry's `kind` is never `proxied`. |
| `name` | string | Display name: the reference environment's card `name`, else `agentName`. |
| `description` | string | Reference card's `description`, `""` when none. |
| `version` | string | Reference card's `version`, `""` when none. |
| `skills` | array of string | Names of the reference card's skills, in card order; `[]` when none (never `null`). |
| `leadEnvironment` | string | Description: "The first environment in pipeline order with a fetched card; the summary and cardDiffers are relative to it." (Task 3 wording; the branch had no description.) |
| `environments` | array of `A2ACatalogEnvironment` | Description: "Environments in pipeline order, lead first". Note: after Task 3 the lead is not necessarily first in this array (see section 4.6); the description is kept as the branch wrote it and flagged in section 13. |

**`A2ACatalogEnvironment`** — required `name`, `status`, `proxied`, `cardDiffers`, `matched`.

| Field | Type | Meaning |
|---|---|---|
| `name` | string | Environment name. |
| `status` | string enum `pending`, `fetched`, `failed` | The card row's fetch status (existing `A2AAgentCardStatus`). |
| `fetchedAt` | string, `date-time`, optional | The card row's fetched time; omitted when the row has none. |
| `proxied` | boolean | True when an `a2a_proxies` row exists for this agent and environment (by environment UUID). |
| `cardDiffers` | boolean | True when this environment's stored card fingerprint is non-empty, the reference environment's is non-empty, and they differ (section 4.7). |
| `matched` | boolean | Description: "True only when q was given and this environment's card matched it". Semantics in section 4.4. |

**`A2AConnectResponse`** — required `kind`, `proxied`, `isProduction`, `gatewayUrl`,
`cardUrl`, `jsonRpcUrl`, `security`, `publication`.

| Field | Type | Meaning |
|---|---|---|
| `kind` | string enum `platform`, `external` | Card source. |
| `proxied` | boolean | External agent fronted by the gateway in this environment. Always `false` for platform agents. |
| `isProduction` | boolean | The environment's production flag from OpenChoreo. |
| `gatewayUrl` | string | Description: "Where the environment's gateway serves the agent. Empty for a direct external agent or while no gateway is mapped. This is the endpoint clients must use for platform and proxied agents." |
| `cardUrl` | string | Description: "Where clients fetch the agent card: the gateway's copy when the gateway fronts the agent, otherwise the card's own URL. Empty when neither is known." |
| `jsonRpcUrl` | string | Description: "The JSON-RPC endpoint clients should call: the gateway's when it fronts the agent, otherwise the card's declared JSON-RPC interface. Empty when there is none." |
| `security` | `A2AConnectSecurity` | |
| `publication` | `A2AConnectPublication` | |

**`A2AConnectSecurity`** — required `mode`.

| Field | Type | Meaning |
|---|---|---|
| `mode` | string enum `none`, `apiKey`, `oauth`, `unmanaged`, `unknown` | Section 6.6. |
| `apiKeyHeader` | string, optional | Description: "X-API-Key when mode is apiKey". Present only when non-empty (i.e. only for `apiKey`). |
| `oauth` | `A2AConnectOAuth`, optional | Present only when `mode` is `oauth`. |

**`A2AConnectOAuth`** — required `issuers`, `audiences`, `headerName`, `authHeaderPrefix`.

| Field | Type | Meaning |
|---|---|---|
| `issuers` | array of `A2AConnectIssuer` | One per configured issuer name, in configured order; `[]` when none. |
| `audiences` | array of string | Configured audiences; `[]` when none (never `null`). |
| `headerName` | string | Header carrying the token (default `Authorization`). |
| `authHeaderPrefix` | string | Scheme prefix (default `Bearer`). |

**`A2AConnectIssuer`** — required `name`, `issuer`.

| Field | Type | Meaning |
|---|---|---|
| `name` | string | The identity-provider name saved in the agent config. |
| `issuer` | string | Description: "Empty when the name no longer resolves" — the issuer URL of the environment's identity provider with that name, `""` when no provider of that name is mapped to the environment. |

**`A2AConnectPublication`** — required `status`, `lastError`.

| Field | Type | Meaning |
|---|---|---|
| `status` | string enum `pending`, `published`, `failed`, `none` | Description: "none means a direct external agent, where the gateway isn't involved". The first three are the existing `A2APublicationStatus` values. |
| `lastError` | string | The publication row's last error, or a synthetic message when no row exists (section 6.5); `""` otherwise. |

---

## 4. Catalogue data model and semantics

### 4.1 Source of truth

The catalogue is a read-only view over `a2a_agent_cards` (one row per agent per
environment, created by the existing card reconciler/publication flow) joined with
`a2a_proxies` (existence per agent-environment, by `environment_uuid`). No new write
path is introduced for the catalogue itself. All reads are scoped by `ou_id` taken from
the caller's token (`middleware.OUIDFromRequest`), never from the path.

### 4.2 Grouping: one entry per agent

An entry is keyed by `(project_name, agent_name)` within the organisation. Every card row
of that agent (one per environment) belongs to the same entry. An agent with the same
name in two projects is two entries.

### 4.3 Pagination and total

- Paging is by agents (entries), not by rows: `limit` and `offset` count distinct
  `(project, agent)` keys.
- Order of entries: `project_name` ascending, then `agent_name` ascending (plain string
  collation of the database). This is the only ordering; there is no sort parameter.
- `total` is the number of distinct agents satisfying the filters, independent of
  `limit`/`offset`. It must be correct even when `offset` is past the last page (the page
  is `[]` but `total` is still the true count).
- The repository returns every environment row of each agent on the page (so the service
  can build the per-environment array), plus the total.
- Implementation guidance (not a contract): one grouped query with a window count
  yields the page keys and the total in a single scan; when that yields no rows (offset
  past the end) a separate count query supplies the total; a second query loads all rows
  for the selected keys with their `proxied` and `matched` flags. The row-loading query
  orders by `project_name`, `agent_name`, `environment_name`.
- A limit of 0 at the repository boundary means "no limit"; the controller never passes
  0 (it validates `1..100`).

### 4.4 Search (`q`) — after Task 2

- Semantics: case-insensitive substring match (`ILIKE '%…%'`) of the trimmed query
  against **either** the row's `agent_name` **or** the row's stored search text
  (section 5.3). Nothing else is searched; the card JSON is never parsed at query time.
- Wildcard escaping: the user's `%`, `_` and `\` are literal. Escape them with the
  existing `escapeLikePattern` helper in `repositories/custom_evaluator_repository.go`
  (backslash-escapes `\`, `%`, `_`) and use `ESCAPE '\'` on the predicate. A query of
  `%` matches only text containing a literal percent sign; a query of `_` matches only a
  literal underscore.
- Row vs agent: the search predicate is evaluated per card row. An agent is on the page
  when at least one of its rows matches (together with the other filters, section 4.5).
  All of that agent's rows are then returned, each carrying `matched`.
- `matched` flag: for each returned row, `matched` is the same predicate evaluated for
  that row (`agent_name` or search text contains the query), coerced to `false` when the
  predicate is NULL. When `q` is absent/empty, `matched` is `false` on every row. Because
  `agent_name` is part of the predicate, every row of an agent found by its agent name is
  `matched` (including rows whose card is NULL/pending). A row found only through another
  environment's card text is `matched: false`.
- Regression #4: an agent whose card is NULL in every environment (first fetch pending,
  or failed) must be found by `q` equal to any substring of its agent name, and the row
  is `matched: true`.
- Regression #15: the query never matches JSON syntax. `q=[`, `q="`, `q=", "`, `q=null`
  must not match an agent merely because its skills have tag/example arrays; and an
  example containing a double quote or backslash must be found by its literal text (e.g.
  `q=say "hi"` finds the example `say "hi"`).
- Known limitation: a `q` containing invalid UTF-8 reaches Postgres and fails the
  request with 500 (left as is by the plan).

### 4.5 Filters

All filter conditions are combined with AND and evaluated **per card row**; an agent is
on the page when at least one of its rows satisfies every condition together. All rows of
such an agent are returned (the row-level filters do not prune the environments array).

| Parameter | Row condition |
|---|---|
| `project` | `project_name` equals the value exactly (case-sensitive). |
| `environment` | `environment_name` equals the value exactly. |
| `status` | the row's card `status` equals the value. |
| `kind=platform` | the row's card `source` is `platform`. |
| `kind=external` | the row's card `source` is `external`. |
| `kind=proxied` | an `a2a_proxies` row exists with the same `ou_id`, `project_name`, `agent_name` and `environment_uuid`. (Only external agents have proxies, so this implies external.) |
| `q` | section 4.4. |

Each filter is single-valued (no multi-value / comma lists). There is no server-side cap
on the number of projects; the "50-project filter cap" noted by the review is a console
limitation (its project picker lists the first 50 projects) and is out of scope here.

Example: `environment=prod&status=fetched` returns agents having a `prod` row whose
status is `fetched`; `kind=proxied&environment=dev` returns agents proxied in `dev`
specifically.

### 4.6 Environment ordering within an entry — after Task 5

For each project on the page, the service reads the project's deployment pipeline through
the existing OpenChoreo client method `GetProjectDeploymentPipeline(ctx, ouID, project)`
and derives the environment order from its promotion paths with the client's existing
helper (currently unexported `buildEnvironmentOrder` in
`clients/openchoreosvc/client/deployments.go`; export it so the service can call it, and
update its two internal callers). That helper walks the promotion paths in order, emitting
each path's source environment then its target environments, skipping names already seen;
an empty path list yields an empty order.

Sorting rule for an entry's rows (stable sort):
1. Rows whose environment is in the pipeline order come first, in pipeline position.
2. Rows whose environment is not in the order come after them, sorted by environment
   name ascending.
3. When the order is nil/empty (pipeline unavailable), every row is sorted by environment
   name ascending.

Pipeline lookup rules (Task 5; regression #11):
- Per request, the pipeline is fetched **at most once per project**, concurrently for the
  distinct projects on the page, with at most **8** lookups in flight at a time.
- The whole fan-out runs under one shared deadline of **5 seconds** (a context timeout
  wrapping the request context). The first lookup that fails with a non-ignorable error
  cancels the remaining lookups. The 5-second value is a package constant used as the
  default; the service holds it as a field so a unit test can shorten it (e.g. to 50 ms).
- The single-entry endpoint applies the same 5-second deadline to its one pipeline lookup.
- Errors that are **allowed to fall back** to name order (returning "no order" and
  logging at Info level): `utils.ErrProjectNotFound`, `utils.ErrDeploymentPipelineNotFound`
  and the generic `utils.ErrNotFound` (all compared with `errors.Is`). A nil pipeline
  returned with a nil error is also treated as "no order" and is never passed to the
  order helper. (The OpenChoreo client returns `ErrProjectNotFound` when the project's
  GET is 404, `ErrDeploymentPipelineNotFound` when the project has no pipeline
  reference, and `ErrNotFound` when the pipeline GET is 404.)
- Every other error (5xx, timeout, 401/403, network, context deadline) **fails the
  request**: the service returns the error wrapped as
  `order a2a catalogue environments: <cause>` (using `%w`), and the controller answers
  500 through `handleCommonErrors` with the operation's fallback message. The error
  must not be mapped to the not-in-catalogue 404 and must not be swallowed.
- Pipeline lookups are not cached across requests (each request reads afresh). Caching
  across requests is explicitly out of scope.
- The branch's current behaviour (every error → name order, no deadline) is the bug and
  must not be reproduced.

### 4.7 Reference ("lead") environment, summary and `cardDiffers` — after Task 3

Given an entry's rows sorted as in 4.6:

- **Reference row** = the first row (in that order) whose card body is non-NULL; if no row
  has a card body, the first row.
- `leadEnvironment` = the reference row's environment name.
- Summary fields `name`, `description`, `version`, `skills` are read from the reference
  row's card (section 4.8). `name` falls back to `agentName` when the reference row has
  no card or its card has no non-empty string `name`.
- `kind`, `projectName`, `agentName` come from the entry key / the rows (all rows of an
  agent share them).
- For every row: `cardDiffers = referenceFingerprint != "" AND rowFingerprint != "" AND
  rowFingerprint != referenceFingerprint`, where the fingerprints are the **stored**
  values on the rows (section 5.2). The service never recomputes a fingerprint from the
  card JSON at request time. The reference row itself therefore always has
  `cardDiffers: false`.
- Upgrade window: rows fetched before migration 049 have an empty stored fingerprint
  until their one-time refetch completes; an empty fingerprint on either side never flags
  a difference.
- Regression #13: dev (no card) → staging (skills `[one]`) → prod (skills `[one, two]`)
  must give `leadEnvironment: staging`, `skills: ["one"]`, prod `cardDiffers: true`,
  staging and dev `false`.
- The `environments` array keeps the pipeline order of 4.6 (it is not rotated to put the
  reference first).

Per-row output fields: `name` = environment name; `status`, `fetchedAt` from the row;
`proxied` and `matched` from the repository flags.

### 4.8 Card summary rules

Reading a stored card body for the summary never fails:
- If the body is not a JSON object (invalid JSON, `null`, array, wrong types), every
  summary field is its zero value (`""`, `[]`).
- `name`, `description`, `version`: the top-level string value of that key; a missing or
  non-string value is `""`.
- `skills`: if the top-level `skills` is a JSON array, the array's elements that are
  objects with a non-empty string `name`, in order, mapped to that name. Non-object
  elements and skills without a usable name are skipped. Otherwise `[]`.
- The controller must serialise `skills` as `[]`, never `null`.

---

## 5. Card projection stored at fetch time (Task 1 — already on the branch head)

### 5.1 What is persisted

Two new columns on `a2a_agent_cards`:

| Purpose | Type | Nullability | Default |
|---|---|---|---|
| Card fingerprint (hex SHA-256) | `VARCHAR(64)` | NOT NULL | `''` |
| Search text | `TEXT` | NOT NULL | `''` |

Both are mapped on the existing `models.A2AAgentCard` struct as string fields. A small
value type (a pair of strings: fingerprint, search text) travels from the service layer
to the repository.

### 5.2 Fingerprint canonicalisation rule

Given the fetched card bytes:
1. Decode the JSON preserving number precision exactly as the existing `a2aCardHash`
   in `services/a2a_card_reconciler.go` does (a decoder with `UseNumber`, so large or
   high-precision numbers are not rounded through float64).
2. If the decoded value is **not** a JSON object (array, scalar, `null`), the
   fingerprint is the existing `a2aCardHash` of the raw bytes (which itself canonicalises
   key order/whitespace).
3. If it is an object: for every element of the top-level `supportedInterfaces` array
   that is itself an object, remove its `url` key. Every other key, at every depth, is
   kept — including `protocolBinding`, `protocolVersion`, skill `id`, `inputModes`,
   `outputModes`, `security`, tags, examples, and the order of array elements.
4. Re-serialise with canonical key order at every depth and no insignificant whitespace
   (Go's `encoding/json` map marshalling sorts keys), then hash with `a2aCardHash`
   (SHA-256, lowercase hex, 64 characters).
5. Invalid JSON is an error (the fetch attempt fails; see 5.4). A nil/absent card yields
   an empty projection (both strings `""`) and no error.

Consequences that tests must pin: nested key order does not change the fingerprint;
changing only an interface `url` does not; changing an interface's `protocolBinding`
does; changing a skill's `inputModes` does; swapping two skills does.

### 5.3 Search text rule

Search text is the newline-joined (`"\n"`) list of these string values from the card, in
this order, with empty strings dropped:
1. top-level `name`;
2. top-level `description`;
3. for each element of `skills` (only when `skills` is an array whose elements can all be
   read as objects; otherwise no skill text at all): the skill's `name`, then
   `description`, then each string element of `tags`, then each string element of
   `examples`.

Only string values are included; numbers, booleans, objects, arrays and nulls are
skipped. The text contains the decoded values verbatim (so `say "hi" \ now` appears with
its quote and backslash) and never contains JSON syntax (`[`, `{`, `\"`). A card that is
not a JSON object yields `""`.

### 5.4 When the projection is written

- The card reconciler (`services/a2a_card_reconciler.go`, in the per-row fetch step)
  computes the projection right after it computes the card hash and passes it to the
  repository's `MarkFetched`. A projection error is recorded as an attempt failure in the
  same way a hash error is (same retry/backoff path), and nothing is stored.
- `A2AAgentCardRepository.MarkFetched` gains one trailing parameter (the projection). It
  **always** writes both projection columns, even when the card hash is unchanged and the
  `card`/`card_hash` columns are therefore left untouched (existing conditional
  behaviour). The existing superseded guard (updated_at moved → superseded error) and
  the "source_url only for platform rows" rule are unchanged.
- The repository mock in `repositories/repomocks/` is regenerated; the reconciler unit
  tests and card repository integration tests that call `MarkFetched` take the extra
  argument.

### 5.5 When the projection is reset

Every path that nulls the `card` column resets both projection columns to `''`:
- `ClearCard` (used when switching an external agent between direct and proxied, and
  when a proxied origin changes) — the update map adds both columns set to `''`.
- The upsert assignment list that clears the card when an external row's source URL
  changes (`newSourceURLAssignments` / `keepUnlessChanged` in
  `repositories/a2a_agent_card_repository.go`) adds the same two conditional
  assignments: reset to `''` when the source URL (or source) changed, keep otherwise.
- An upsert with the same source URL keeps both values.

### 5.6 Migration 049 (one-time refetch)

In one transaction:
1. `ALTER TABLE a2a_agent_cards ADD COLUMN IF NOT EXISTS <fingerprint> VARCHAR(64) NOT
   NULL DEFAULT ''`, `ADD COLUMN IF NOT EXISTS <search text> TEXT NOT NULL DEFAULT ''`
   (idempotent).
2. `UPDATE a2a_agent_cards SET status = 'pending', attempt_count = 0, next_attempt_at =
   NOW() WHERE status = 'fetched' AND card IS NOT NULL` — every live fetched card is
   requeued once so the reconciler stores its projection. Rows left alone: pending rows,
   failed rows, and parked rows (`next_attempt_at IS NULL` with a non-fetched status).
   The requeued rows keep their `card`, `card_hash` and `fetched_at` (the existing
   "preserve the last good card" behaviour), so the catalogue keeps serving their summary
   during the window; only the projection is empty until the refetch completes.

Registration: the migration struct's ID is 49, appended to the `migrations` slice in
`db_migrations/migration_list.go`, and `latestVersion` is bumped. Follow the shape of
`db_migrations/048_create_a2a_proxies.go` (a `migration{ID, Migrate}` value running SQL
through the package's `runSQL` helper inside `db.Transaction`).

Upgrade-window behaviours (must hold between migration 049 and the refetch):
- Search still finds those agents by agent name (4.4).
- `cardDiffers` stays `false` for every environment of such agents rather than flagging
  everything (4.7).

---

## 6. Connect details semantics

### 6.1 Inputs and resolution order

For `GET .../environments/{envID}/a2a-connect` (org from token; `projName`, `agentName`,
`envID` from the path, read with the existing `agentCardPath` helper in
`controllers/a2a_agent_card_controller.go`):

1. Load the card row through the existing `A2AAgentCardService.GetA2AAgentCard(ctx, ouID,
   project, agent, envName)`. Its errors are returned unchanged (card not found → 404,
   agent not found → 404, other → 500 via the card error writer).
2. Load the environment through the OpenChoreo client's `GetEnvironment(ctx, ouID,
   envName)`; translate its generic not-found into `utils.ErrEnvironmentNotFound` with
   the existing `translateEnvironmentError`, wrap any other error as
   `failed to read environment <name>: %w`. `isProduction` is the environment's
   `IsProduction`.
3. `kind` = the card row's `source`.
4. If `kind` is `external`: determine `proxied` by looking up the proxy row with
   `A2AProxyRepository.GetByEnvUUID(ctx, ouID, project, agent, card.EnvironmentUUID)`;
   `repositories.ErrA2AProxyNotFound` → not proxied; any other error → wrapped
   `failed to read the A2A proxy: %w` and the request fails. If **not proxied** (direct):
   `security.mode = unmanaged`, `publication.status = none`, `lastError = ""`,
   `gatewayUrl = ""`, URLs from 6.3 "no gateway" branch, and return — the agent config,
   gateway and publication are **not** read.
5. Otherwise (platform, or proxied external): read the gateway status (6.4/6.5) for the
   publication source `platform` (platform card) or `external` (external card):
   `gatewayUrl`, `publication.status`, `publication.lastError`, and the recorded
   transports.
6. URLs from 6.3.
7. Security from 6.6.

### 6.2 Gateway URL

`gatewayUrl` is the existing `A2APublisher.GatewayURL(ouID, envUUID, agentName)`:
the first ingress gateway mapped to the environment (`findIngressGateway`), rendered by
`a2aGatewayURL` as `<scheme>://<vhost>/<agentName>` (vhost given a scheme by
`ensureURLScheme`; context is `/` + trimmed agent name). It is `""` while no ingress
gateway is mapped; that is not an error.

### 6.3 Card URL and JSON-RPC URL rules (Task 4; regression #3)

Let `base` = `gatewayUrl` with trailing slashes trimmed, `cardPath` = the existing
`a2aAgentCardPath` constant (`/.well-known/agent-card.json`), and `declared` = the list of
`url` values of the stored card's `supportedInterfaces` entries whose `protocolBinding`
is `JSONRPC` and whose `url` is non-empty, in card order (read leniently: an empty,
absent or unparseable card declares nothing).

| Situation | `cardUrl` | `jsonRpcUrl` |
|---|---|---|
| **No gateway** (`gatewayUrl == ""`: direct external agent, or platform/proxied agent while no gateway is mapped) | the card row's stored source URL (`SourceURL`; for a platform row this is the last fetched URL, possibly `""`) | `declared[0]` if any, else `""` |
| **Gateway and recorded transports non-nil** (publication recorded after migration 050) | `base + cardPath` | `base + pathPrefix` of the transport whose `protocolBinding` is `JSONRPC`; `""` when no such transport was published. An empty `pathPrefix` (origin serving JSON-RPC at its root) gives exactly `base`, never "no JSON-RPC". |
| **Gateway and transports nil** (publication recorded before migration 050, or no publication row) | `base + cardPath` | the first `declared` URL that equals `base` or starts with `base + "/"`; if none, `base + "/rpc"` (the existing `a2aPathPrefixJSONRPC` constant) |

"Empty means none" contract: a `jsonRpcUrl` of `""` tells clients there is no JSON-RPC
endpoint; the console shows no quick start in that case. The backend must never emit a
guessed `/rpc` when it has recorded that no JSON-RPC transport was published (e.g. an
HTTP+JSON-only origin).

Note: a gateway-fetched card of a proxied external agent already declares the gateway's
own JSON-RPC URL (the gateway rewrites interface URLs), which is why the nil-transports
rule prefers a declared URL under the gateway base.

### 6.4 Published transports — what they are and where they come from

"Transports" are the gateway route bindings the publication reconciler emits in the
Agent resource it publishes: a list of `{protocolBinding, pathPrefix}` pairs (the
existing `A2ATransport` type in `services/a2a_agent_deployment.go`; `pathPrefix` is
omitted from JSON when empty). Today they are:
- for a **platform** agent: the resolved upstream carries nil transports, and the YAML
  builder (`buildA2AAgentDeploymentYAML`) substitutes the platform defaults
  `[{JSONRPC, "/rpc"}, {HTTP+JSON, "/rest"}]`;
- for a **proxied external** agent: derived from the origin card at every publish by
  `deriveA2AProxyRoutes` in `services/a2a_proxy_routes.go` — one transport per offered
  binding among `JSONRPC` and `HTTP+JSON`, with `pathPrefix` = the interface URL's path
  below the origin upstream (`""` when the interface is at the upstream root); a binding
  the origin does not offer is omitted; a card offering neither is not proxyable.

Required change (Task 4): the publication reconciler must record the **effective**
transports — the upstream's transports, or the platform defaults when the upstream
declares none — on the publication row when it marks it published:
- Extract the "defaults when nil" substitution from `buildA2AAgentDeploymentYAML`
  (`services/a2a_agent_deployment.go`, the block that assigns the two defaults) into one
  shared helper so the YAML builder and the reconciler use a single definition of the
  defaults.
- The publish attempt's result (`a2aPublishResult` in
  `services/a2a_publication_reconciler.go`, currently upstream URL + gateway ID) also
  carries the effective transports; `publishOne` marshals them to JSON and passes them to
  the repository.
- `A2APublicationRepository.MarkPublished(ctx, read, upstreamURL)` gains a trailing
  JSON parameter and stores it in the new column alongside `published_upstream_url`.
  Regenerate its mock; update the reconciler tests' `MarkPublishedFunc` mocks and the
  repository integration tests' `MarkPublished` calls.
- `models.A2APublication` gains a raw-JSON field for the new column (nil when NULL).

Storage: one new column on `a2a_publications`, type `JSONB`, **nullable, no default**.
NULL means "recorded before transports were tracked". Migration 050 is a single
`ALTER TABLE a2a_publications ADD COLUMN IF NOT EXISTS <transports> JSONB`, ID 50,
appended to the migration list, `latestVersion` = 50.

### 6.5 Gateway status (publication status and warnings)

Add to `A2APublisher` (`services/a2a_publisher.go`) one read helper that, for an agent in
an environment and a publication source, returns a small status value holding four
things: the gateway URL (6.2, `""` when no gateway is mapped); a publication status (an
`A2APublicationStatus`); a last-error string; and (Task 4) the recorded transports as a
`[]A2ATransport`, nil when the row's stored transports are NULL.

Rules:
- Read the gateway URL first; its error fails the call.
- Read the publication row with the existing `Publication(...)`
  (`A2APublicationRepository.Get`).
  - `repositories.ErrA2APublicationNotFound` → status `failed` and last error = a
    source-specific message (a missing row never resolves on its own, so it is reported
    as a failure that tells the user what re-queues it):
    - source `external`: `no gateway publication is queued for this environment; refresh to publish it again`
    - source `platform`: `no gateway publication is queued for this environment; redeploy the agent or save its deploy settings to publish it again`
  - any other error → wrapped `failed to read the A2A publication: %w`, call fails.
  - otherwise status and last error are copied from the row.
- (Task 4) When the row's stored transports are non-NULL, unmarshal them into
  `[]A2ATransport`; a value that fails to unmarshal is an **error** (wrapped with `%w`),
  not "unrecorded". When NULL, the transports are nil.

The connect response's `publication` is `{status: <status>, lastError: <last error>}`;
for a direct external agent it is `{status: "none", lastError: ""}` without reading
anything.

Required change to `services/a2a_proxy_service.go`: `GetA2AProxy` currently reads
`GatewayURL` and `Publication` itself and holds the external "no publication is queued"
message as a private constant. It must instead call the shared publisher helper with
source `external` and copy its gateway URL, status and last error into
`models.A2AProxyStatus`.
Externally its behaviour is unchanged (same statuses, same message); the private constant
moves to the publisher as the external entry of the per-source message table.

Test-key issuance is **not** part of the connect contract; the console issues test keys
through the existing agent API-key endpoints.

### 6.6 Security modes

Read the environment's agent config with `AgentConfigRepository.Get(ctx, ouID, project,
agent, envName)` (only reached for platform and proxied agents):

| `mode` | When |
|---|---|
| `unmanaged` | Direct (unproxied) external agent: the platform does not manage its security. Decided before any config read. |
| `unknown` | The config read returns `repositories.ErrAgentConfigNotFound` (no row for the agent/environment). The platform's effective default for a gateway-fronted agent is API-key security, but the backend does not guess; it reports `unknown`. |
| `oauth` | Config has OAuth security enabled (`EnableOAuthSecurity`). Checked **before** API key, so a config with both flags reports `oauth`. |
| `apiKey` | Config has API-key security enabled (`EnableApiKeySecurity`) and OAuth not enabled. `apiKeyHeader` = `X-API-Key`. |
| `none` | Config exists with neither enabled. |

Any other config-read error → wrapped `failed to read the agent config: %w`, request
fails (never masked as not-found or as `unknown`).

The `X-API-Key` header value must come from one named constant placed next to the
existing API-key policy builder (`APIKeyAuthPolicy` in
`clients/openchoreosvc/client/components.go`), and that builder must read the same
constant, so the header the gateway policy enforces and the header the connect API
reports cannot drift.

### 6.7 OAuth details and issuer resolution

When `mode` is `oauth`:
- Build the OAuth view with the existing `oauthConfigFromAgentConfig(cfg)`
  (`services/agent_manager.go`): `audiences` = config audiences; `headerName` = config
  header name or `Authorization` when empty; `authHeaderPrefix` = config prefix or
  `Bearer` when empty.
- Resolve issuers: list the environment's identity providers with
  `GatewayRepository.ListIdentityProvidersByEnvironment(envUUID.String())` (returns the
  providers of the gateways mapped to the environment, deduplicated by name, excluding the
  reserved internal provider). Any error → wrapped `failed to list identity providers:
  %w`, request fails. For each issuer **name** in the config's `OAuthIssuers`, in order,
  emit `{name, issuer}` where `issuer` is the matching provider's `Issuer` URL or `""`
  when no provider has that name.
- The response's `oauth.issuers` is this resolved list (not the raw names);
  `oauth.audiences` is `[]` when nil.
- `oauth` is omitted from the response for every other mode; `apiKeyHeader` is omitted
  unless non-empty.

---

## 7. Catalogue service orchestration (list and single entry)

List:
1. Call the catalogue repository's list operation with the filter; a repository error is
   returned wrapped (`list a2a catalogue agents: %w`) → 500.
2. Group rows by `(project, agent)` preserving first-seen order (which is the
   repository's project/agent order).
3. Fetch pipeline orders for the distinct projects (4.6). A non-ignorable error fails the
   request.
4. For each group: sort by the project's order, derive the reference row, summary and
   per-environment flags (4.7, 4.8).
5. Return `{entries, total, limit, offset}` where `limit`/`offset` echo the filter.

Single entry:
1. Call the repository's "rows of one agent" operation (no filters, `matched` always
   false). A repository error is returned wrapped (`get a2a catalogue agent: %w`) → 500,
   and must not be mapped to not-found.
2. Zero rows → the new "agent is not in the A2A catalogue" sentinel → 404.
3. Fetch the project's pipeline order (same rules and 5-second deadline) and build the
   entry as in the list.

---

## 8. Error handling conventions and mappings

### 8.1 Binding rules (from `agent-manager-service/AGENTS.md` on upstream/main)

- Map the specific sentinel with `errors.Is` (e.g. `gorm.ErrRecordNotFound` → the
  specific `utils.ErrXxxNotFound`); wrap everything else with
  `fmt.Errorf("...: %w", err)`.
- Never flatten an unexpected error into not-found; never silently fall back to a default
  on a non-not-found error. Compare sentinels with `errors.Is`, never `==` or string
  matching.
- Every I/O method takes `context.Context` first and propagates it.
- Org scoping: `ou_id` always comes from the caller's token (`middleware.OUIDFromRequest`
  / resolved org), never from the path or body; every org-scoped query carries the
  `ou_id` predicate. Tenant isolation is DB-only.
- Controllers do HTTP only (parse/validate, map results to status codes, translate
  sentinels); services hold the logic and depend on repository/client interfaces;
  repositories do persistence only.
- Don't re-fetch a resource already loaded earlier in the request path.
- Never hold a lock across I/O; serialise expensive side effects per key.
- Logging: Debug for hot paths, Info for rare events, Error for destructive operations;
  include correlation context (org, resource, environment).
- Lint: CI runs `golangci-lint run --config .github/linters/.golangci.yaml ./...` on
  test files too (`nilnil`, `goheader`, `errorlint`, `nilerr`, `nolintlint`,
  `exhaustruct` for production code). Every `.go` file carries the Apache header.

### 8.2 Sentinels the feature introduces (by meaning)

| Meaning | Where declared | HTTP mapping |
|---|---|---|
| "agent is not in the A2A catalogue" (no card row in any environment) | one new `errors.New` value in `utils/errors.go`, next to `ErrAgentCardNotFound` | 404, body message `The agent is not in the A2A catalogue` (single-entry endpoint only) |

No other new sentinels. Everything else reuses upstream sentinels.

### 8.3 Status mapping per operation

List:
- 400: section 3.1 validation.
- 500: repository error, non-ignorable pipeline error (incl. deadline exceeded), via
  `handleCommonErrors(w, err, "Failed to list A2A agents")`.

Single entry:
- 404: the new sentinel, message `The agent is not in the A2A catalogue`.
- 500: anything else via `handleCommonErrors(w, err, "Failed to get the A2A catalogue entry")`.

Connect — all errors go through the existing `writeAgentCardError(w, r, err, "Failed to
get the A2A connect details")`, which yields:
- 404 `Agent card not found` for `utils.ErrAgentCardNotFound` (no card row; the card
  service first confirms the agent exists, so an unknown agent is 404 `Agent not found`
  through `handleCommonErrors`);
- 404 `Agent is not deployed in this environment` for
  `utils.ErrAgentNotDeployedInEnvironment` (not produced by this flow today, kept for
  parity);
- 404 `Environment not found` for `utils.ErrEnvironmentNotFound` (OpenChoreo 404 on the
  environment);
- 400 for the card-service 400 sentinels (not produced by this read);
- 500 otherwise (proxy, publication, gateway, config, identity-provider or OpenChoreo
  failures, unparseable stored transports).

Logging: the card error writer already logs each failure at Warn with the path.

### 8.4 Audit

All three routes are GET and are not in `sensitiveReadPaths` (`audit/policy.go`), so
they are not audited; the route file's comment should say so. No `actionOverrides` entry
and no semantic audit event is needed. The new controller constructor must be appended to
the `controllerConstructors` list in `api/audit_coverage_test.go` so the coverage test
sees its routes.

---

## 9. Route registration, authorization, wiring

- Register in a new file under `agent-manager-service/api/` with a function shaped like
  `registerA2AAgentCardRoutes` (`api/a2a_agent_card_routes.go`), called from
  `registerAPIRoutes` in `api/app.go` right after the A2A proxy routes. Each route uses
  `rr.HandleFuncWithValidationAndAuthz(pattern, rbac.AgentRead, handler)`:
  - `GET /orgs/{orgName}/a2a-agents`
  - `GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/a2a-catalog-entry`
  - `GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/a2a-connect`
- Permission: the existing `rbac.AgentRead` (`agent:read`) on all three. No new
  permission; no change to `rbac/predefined_roles.go` (every predefined role already
  grants `agent:read`). Path-param validation and `RequireOrgMatch` come from the
  registrar (`{orgName}` present on every pattern). Path values are read with the
  existing `utils.PathParamProjName`, `utils.PathParamAgentName`, `utils.PathParamEnvID`
  constants.
- Wiring (`wiring/wire.go`, regenerated `wiring/wire_gen.go` via `make wire` /
  `make codegen`):
  - repository provider set: a provider function constructing the catalogue repository
    from `*gorm.DB` (same shape as `ProvideA2AAgentCardRepository`);
  - service provider set: the catalogue service constructor, which receives the
    catalogue repository, the `A2AAgentCardService`, `A2AProxyRepository`, the
    `*A2APublisher`, `AgentConfigRepository`, `GatewayRepository`, the OpenChoreo client
    and a `*slog.Logger`;
  - controller provider set: the catalogue controller constructor (service interface in);
  - `wiring/params.go` `AppParams`: a new controller field, populated in both
    `InitializeAppParamsWithEnvThunderEndpoints` and `InitializeTestAppParamsWithClientMocks`.
- Interfaces: the service and repository are interfaces; the repository interface carries
  a `//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out
  repomocks/<file>_mock.go . <Iface>:<Iface>Mock` directive (copy an existing one) and its
  mock is generated into `repositories/repomocks/`. The controller unit test may use a
  hand-written stub of the service interface (in-package service interfaces have no
  generated mock).

---

## 10. Code generation and the amctl client

| Step | Command | Notes |
|---|---|---|
| Regenerate `agent-manager-service/spec/` | `make spec` in `agent-manager-service/` | `scripts/gen_client.sh`: openapi-generator-cli 7.0.0 (Docker locally, JAR in CI), `-g go --package-name spec --additional-properties enumClassPrefix=true`; the whole `spec/` directory is deleted and rebuilt, then `scripts/fmt.sh`. Never hand-edit `spec/`. |
| Regenerate mocks + wire | `make codegen` (or `make mocks PKG=./repositories/...` for mocks only, `make wire` for DI only) | Requires `moq` **v0.5.3** on PATH (`make tools` at repo root); `make mocks` fails fast on another version. |
| Regenerate the amctl client | `make amctl-gen-client` at the **repo root** | Runs `oapi-codegen` (v2.6.0 from `make tools` / `make tools-codegen`, pinned in `tools.mk`) twice against `agent-manager-service/docs/api_v1_openapi.yaml` with `cli/pkg/clients/amsvc/gen/oapi-codegen.yaml` (models → `types.gen.go`) and `cli/pkg/clients/amsvc/gen/oapi-codegen-client.yaml` (client → `client.gen.go`, response type suffix `Resp`). CI (`.github/workflows/cli-codegen-check.yaml`) regenerates and fails on drift, then builds and tests `cli/`. |
| Drift check for the service | `make codegenfmt-check` in `agent-manager-service/` | wire + spec + codegen + fmt + `go mod tidy` must leave no `.go` diff. |

What the amctl regeneration must produce: only the two generated files change. The
generated client gains request functions and `…WithResponse` variants for the three
operationIds (`ListA2AAgents` with a params struct holding `Q`, `Kind`, `Environment`,
`Project`, `Status`, `Limit`, `Offset`; `GetA2ACatalogEntry`; `GetA2AConnect`), and the
types file gains the eight schemas plus the enum types oapi-codegen derives for `kind`,
`status`, `mode` and the list params' `kind`/`status`. No amctl command uses them on this
branch; `cli/` must still build and its tests pass.

---

## 11. Required changes to existing upstream/main code (by behaviour)

| File | Change |
|---|---|
| `models/a2a_agent_card.go` | Two string fields for the projection columns; a two-string value type for the projection. |
| `repositories/a2a_agent_card_repository.go` | `MarkFetched` takes the projection and always writes it; `ClearCard` and the source-URL-change assignments reset it (5.4, 5.5). Regenerate `repomocks/a2a_agent_card_repository_mock.go`. |
| `services/a2a_card_reconciler.go` | Compute the projection after the hash; failure = attempt failure; pass it to `MarkFetched`. |
| `services/a2a_card_reconciler_unit_test.go`, `repositories/a2a_agent_card_repository_test.go` | Existing `MarkFetched` mocks/calls take the new argument. |
| `db_migrations/migration_list.go` | Append migrations 049 and 050; `latestVersion` 50. |
| `models/a2a_publication.go` | Raw-JSON field for the published transports (nil = NULL). |
| `repositories/a2a_publication_repository.go` | `MarkPublished` takes the transports JSON and writes the new column. Regenerate its mock; update `a2a_publication_repository_test.go` and `a2a_publication_reconciler_test.go`. |
| `services/a2a_agent_deployment.go` | Extract the "nil transports → platform defaults" rule into one shared helper used by the YAML builder. |
| `services/a2a_publication_reconciler.go` | The publish result carries the effective transports (via that helper); `publishOne` marshals and records them. |
| `services/a2a_publisher.go` | New gateway-status read helper (URL, status, lastError, transports) with the per-source missing-publication messages (6.5). |
| `services/a2a_proxy_service.go` | `GetA2AProxy` uses the shared helper; the private "no publication is queued" constant is removed (moved). |
| `clients/openchoreosvc/client/deployments.go` | Export the promotion-path ordering helper (currently `buildEnvironmentOrder`); update `GetDeployments` and `FindFirstEnvironment` to the exported name. |
| `clients/openchoreosvc/client/components.go` | Named constant for the `X-API-Key` header used by `APIKeyAuthPolicy` and by the connect security view. |
| `utils/errors.go` | One new not-in-catalogue sentinel (8.2). |
| `api/app.go`, `api/audit_coverage_test.go`, `wiring/wire.go`, `wiring/params.go`, `wiring/wire_gen.go` | Registration (section 9). |
| `docs/api_v1_openapi.yaml` | Section 3, including the Task 2 `q` description and the Task 3 `leadEnvironment` description; then `make spec` and `make amctl-gen-client`. |

---

## 12. Testing

### 12.1 Tiers and commands (as the repo defines them)

| Tier | Build tag | DB | Command |
|---|---|---|---|
| Unit (services, controllers) | none (omit the `//go:build` line) | no | `make test-unit`; single test: `make test-run RUN=<pattern> PKG=./services/` |
| Integration (repositories, migrations) | `//go:build integration` on line 1 | isolated Postgres | `make test-integration` |

Rules: never unit-test the repository layer; service unit tests inject `moq` mocks
(`repomocks.<Iface>Mock`, `clientmocks.OpenChoreoClientMock`; an unconfigured `<Method>Func`
panics); use `assert.ErrorIs` / `assert.NotErrorIs` for sentinels and explicitly check
that real errors are not masked as not-found; reuse `strPtr` and `discardLogger` helpers
rather than redeclaring; `go build -tags=integration ./...` must compile; the CI lint
config applies to test files; `go test ./api/ -run TestEveryMutatingRouteIsAudited`
must pass. Repository tests cannot import `services`, so they write literal projection
text/fingerprints the service would produce.

### 12.2 Scenarios — card projection (unit, services)

1. Given two cards that differ only in nested key order, When each is projected, Then
   fingerprints are equal and non-empty.
2. Given cards differing only in `supportedInterfaces[0].url`, Then fingerprints are
   equal; Given cards differing in `supportedInterfaces[0].protocolBinding`, Then they
   differ.
3. Given a card where one skill's `inputModes` changes, or two skills are swapped, Then
   the fingerprint changes.
4. Given `{"name":"Orders","description":"d","skills":[{"name":"s","description":"sd","tags":["refunds"],"examples":["say \"hi\" \\ now"]}]}`,
   Then the search text contains `Orders`, `refunds` and `say "hi" \ now` verbatim and
   contains none of `[`, `{`, `\"`.
5. Given a nil card, Then the projection is empty and there is no error.
6. Given invalid JSON, Then projection returns an error.
7. Given a numeric value with more precision than float64, When projected twice, Then the
   fingerprint is stable and differs from a card with the rounded value.

### 12.3 Scenarios — card summary (unit, services)

8. Given `{"name":"Order Support","description":"Refunds","version":"2.1.0","skills":[{"name":"issue-refund"},{"name":"order-status"}]}`,
   Then name/description/version are read and skills are `[issue-refund, order-status]`.
9. Given nil, `null`, wrong types (`{"name":5,"skills":"x"}`) or invalid JSON, Then every
   summary field is zero; Given `{"skills":[1,{"name":""},{"name":"ok"}]}`, Then skills
   are `[ok]`.

### 12.4 Scenarios — card reconciler and repository (projection)

10. (unit) Given a fetched card whose projection fails, When the reconciler stores it,
    Then an attempt failure is recorded and `MarkFetched` is not called.
11. (integration) Given a row marked fetched with hash H and projection P1, When it is
    re-enqueued, re-claimed and marked fetched with the same hash H and projection P2,
    Then reading the row gives P2's fingerprint and search text (the card body stays as
    before because the hash did not change).
12. (integration) Given a row with a non-empty projection, When `ClearCard` runs, Then
    both projection values are `""`.
13. (integration) Given an external row with a non-empty projection, When it is
    re-enqueued with the same source URL, Then the projection is kept; When re-enqueued
    with a different source URL, Then both values are `""` (and the card is NULL).
14. (integration, migration) Given fetched rows with cards, a failed row, a pending row
    and a parked row (pending, `next_attempt_at` NULL), When migration 049 runs, Then only
    the fetched rows with a card become pending with attempt count 0 and a due
    `next_attempt_at`, keeping their card body; all others are unchanged; running the
    migration twice does not fail.

### 12.5 Scenarios — catalogue repository (integration)

15. Given agent `a` in dev and prod, and agents `b`, `c` in dev, When listing with limit
    2, Then total is 3 and the rows are `a/dev`, `a/prod`, `b/dev`; with limit 2 offset
    2, total is 3 and rows are `c/dev`.
16. Given one agent, When listing with limit 2 offset 4, Then rows are empty and total is 1.
17. Given `orders` in two projects, When listing, Then total is 2.
18. Given agent `orders` external in dev and prod with a proxy in prod, and `orders`
    platform in another project, When reading the single agent for the first project,
    Then exactly its two rows come back in environment-name order with `proxied` false,
    true; reading an unknown agent gives zero rows and no error.
19. (#4) Given `orders-bot` with a NULL card (pending) and empty search text, When
    listing with `q=orders`, Then the agent is returned and its row is `matched: true`.
20. Given dev text without `refund` and prod text with `refund` for agent `a`, and
    another agent without it, When listing with `q=refund`, Then total 1, both rows of
    `a` are returned, dev `matched` false and prod `matched` true.
21. (#15) Given a row whose search text came from a card with tags/examples, When listing
    with `q=[` or `q="`, Then it is not returned; Given an example `say "hi"` in the
    text, When `q=say "hi"`, Then it is returned.
22. Given search text `100% uptime bot` and `Ledger`, When `q=%`, Then only the uptime
    agent is returned; When `q=_`, Then nothing.
23. Given a pending agent (NULL card, empty search text) and `q=x`, Then it is not
    returned; without `q`, it is returned with `matched` false.
24. Given external `ext` in dev and prod with a proxy in prod, When `kind=proxied`, Then
    total 1 and both rows return with `proxied` false/true; When `kind=platform`, Then
    total 0; When `environment=prod`, Then total 1 and both rows are returned.
25. Given rows in two organisations, When listing as one, Then only its rows and count
    are returned.
26. Given `status=fetched` and `environment=prod`, Then an agent whose prod row is
    pending but dev row fetched is not returned (conditions are per row).

### 12.6 Scenarios — catalogue service (unit)

27. Given rows prod, dev, staging for one agent and a dev→staging→prod pipeline, When
    listing, Then environments are `[dev, staging, prod]`, lead is `dev`, and name /
    description / version / skills come from dev's card, kind is `platform`.
28. Given three rows with identical bodies except interface URLs and prod having an extra
    skill, with the stored fingerprints set accordingly, Then only prod `cardDiffers`.
29. (#13) Given dev with no card, staging skills `[one]`, prod `[one, two]` (stored
    fingerprints differ between staging and prod), Then lead is `staging`, skills `[one]`,
    prod differs, staging and dev do not.
30. Given no row has a card, Then lead is the first row, name is the agent name,
    description empty, skills empty, nothing differs.
31. Given two rows with identical bodies but different stored fingerprints, Then the
    second is flagged (proves the stored value is used, not a recomputation).
32. Given a row whose stored fingerprint is empty (upgrade window) next to a row with one,
    Then neither is flagged.
33. Given environments `zeta`, `alpha`, `prod`, `dev` and a dev→prod pipeline, Then the
    order is `[dev, prod, alpha, zeta]`.
34. Given the pipeline lookup returns `utils.ErrDeploymentPipelineNotFound` (or
    `ErrProjectNotFound`, or `ErrNotFound`), Then the request succeeds and environments
    are in name order.
35. Given the pipeline lookup returns a nil pipeline and nil error, Then name order.
36. (#11) Given the lookup returns a generic error (e.g. "503"), Then listing returns an
    error wrapping it, not the not-in-catalogue sentinel; the same for the single entry.
37. (#11) Given the lookup blocks until its context is done and returns `ctx.Err()`, with
    the service's timeout set to 50 ms, Then listing returns an error matching
    `context.DeadlineExceeded` within the timeout plus one second.
38. Given agents in projects p1 (two agents) and p2, Then the pipeline is fetched exactly
    once per project in one request; Given two consecutive requests, Then it is fetched
    again (no cross-request cache); Given a failing-with-not-found lookup and two agents in
    one project, Then it is fetched once.
39. Given the repository marks a row proxied and matched and reports total 42 with filter
    limit 10 offset 20, Then the page echoes total 42, limit 10, offset 20 and the
    environment carries `proxied` and `matched` true.
40. Given the repository fails, Then the error is returned wrapped (`errors.Is` finds
    the cause) and, for the single entry, is not the not-in-catalogue sentinel.
41. Given the single-entry read returns zero rows, Then the not-in-catalogue sentinel.
42. Given rows for the agent in two projects, When reading one project's entry, Then only
    that project's rows are used and ordered by its pipeline.

### 12.7 Scenarios — connect (unit, service)

43. Given a platform card, an environment, a mapped gateway `gw.example.com`, a published
    publication and a config with API-key security, Then kind `platform`, proxied false,
    `gatewayUrl` = the gateway URL for the agent, security `{apiKey, X-API-Key}`,
    publication `{published, ""}`.
44. Given a failed publication with last error "boom", Then publication `{failed, boom}`.
45. Given OAuth config with issuers `[thunder, gone]` and an environment provider
    `thunder → https://idp/x`, Then mode `oauth`, issuers
    `[{thunder, https://idp/x}, {gone, ""}]`, header `Authorization`, prefix `Bearer`.
46. Given a config with neither security flag, Then mode `none`.
47. Given a missing config row, Then mode `unknown` (and no error).
48. Given an external card with no proxy row, Then kind `external`, proxied false,
    `gatewayUrl` "", mode `unmanaged`, publication `{none, ""}`, and neither the config
    nor the publication nor the gateway is read.
49. Given an external card with a proxy row and API-key config, Then proxied true,
    `gatewayUrl` set, publication from the row, mode `apiKey`.
50. Given a platform card with no publication row, Then publication `{failed, "no gateway
    publication is queued for this environment; redeploy the agent or save its deploy
    settings to publish it again"}`; Given an external proxied card with no row, Then the
    message ends "refresh to publish it again".
51. Given the environment is production, Then `isProduction` true.
52. Given the card service returns card-not-found, Then that sentinel is returned;
    Given the environment lookup returns `utils.ErrNotFound`, Then
    `utils.ErrEnvironmentNotFound`.
53. Given a database error from the config read, proxy read, publication read or
    identity-provider listing, Then the error is returned (`errors.Is` the cause), the
    result is nil, and it is not card-not-found.
54. (#3) URL table, with gateway base `https://gw/agent`:
    - recorded transports `[{JSONRPC, ""}]` → `jsonRpcUrl` `https://gw/agent`;
    - recorded `[{JSONRPC, "/a2a/jsonrpc"}]` → `https://gw/agent/a2a/jsonrpc`;
    - recorded `[{HTTP+JSON, "/rest"}]` → `""`;
    - nil transports and a platform card declaring an internal JSON-RPC URL
      (`http://weather.internal:8080/`) → `https://gw/agent/rpc`;
    - nil transports and a proxied card declaring `https://gw/agent/v1/jsonrpc` →
      `https://gw/agent/v1/jsonrpc`;
    - no gateway (direct) and a card declaring `https://agent.example/rpc` → `cardUrl` =
      the stored source URL, `jsonRpcUrl` `https://agent.example/rpc`;
    - no gateway and an unreadable card → `jsonRpcUrl` "", `cardUrl` = source URL.
    In every gateway case `cardUrl` is `https://gw/agent/.well-known/agent-card.json`.
55. Given a publication row whose stored transports JSON is unparseable, Then the gateway
    status read fails with a wrapped error (not treated as nil transports).

### 12.8 Scenarios — publication reconciler and repository (transports)

56. (unit) Given a platform publish (nil upstream transports), When published, Then the
    repository is told the platform defaults `[{JSONRPC,/rpc},{HTTP+JSON,/rest}]` as JSON.
57. (unit) Given a proxied external publish whose origin offers only HTTP+JSON at
    `/rest`, Then `[{HTTP+JSON,/rest}]` is recorded; an origin serving JSON-RPC at its
    root records `[{JSONRPC}]` with no path prefix.
58. (integration) Given `MarkPublished` with a transports JSON, Then `Get` returns the
    same JSON; Given a row published before the column existed, Then the field is nil.
59. (integration, migration) Running migration 050 twice does not fail; existing rows
    have NULL transports.

### 12.9 Scenarios — proxy service

60. Given a proxied environment with no publication row, When `GetA2AProxy` is read,
    Then status `failed` with the external "refresh to publish it again" message
    (behaviour unchanged from upstream/main).

### 12.10 Scenarios — controller (unit)

61. Given `q=%20refund%20&kind=proxied&environment=dev&project=p1&status=fetched&limit=24&offset=48`,
    Then the service receives org from the token, query `refund` (trimmed), project
    `p1`, environment `dev`, kind proxied, status fetched, limit 24, offset 48; the JSON
    echoes `total`, `limit`, `offset`, and the environment's `cardDiffers`, `matched`,
    `proxied`, `status` and `fetchedAt`.
62. Given no query parameters, Then limit 50 and offset 0 reach the service; an empty page
    serialises as `{"agents":[],"total":0,"limit":0,"offset":0}` when the service returns
    zeros.
63. Given each of `limit=0`, `limit=101`, `limit=x`, `offset=-1`, `kind=bogus`,
    `status=bogus`, `q=<256 chars>`, Then 400 with the matching message and the service
    is not called.
64. Given a connect result for a proxied external agent with API-key security, Then the
    JSON is exactly `{"kind":"external","proxied":true,"isProduction":false,"gatewayUrl":…,"cardUrl":…,"jsonRpcUrl":…,"security":{"mode":"apiKey","apiKeyHeader":"X-API-Key"},"publication":{"status":"published","lastError":""}}`
    (no `oauth` key).
65. Given an OAuth result with audiences `[aud]`, header `Authorization`, prefix
    `Bearer ` and one resolved issuer, Then `security` is
    `{"mode":"oauth","oauth":{"issuers":[{"name":"thunder","issuer":"https://idp.example.com"}],"audiences":["aud"],"headerName":"Authorization","authHeaderPrefix":"Bearer "}}`
    (no `apiKeyHeader`), with empty `gatewayUrl`/`cardUrl`/`jsonRpcUrl` strings present.
66. Given the service returns card-not-found, Then 404.
67. Given a single-entry result with a pending environment and no card, Then the JSON has
    `"description":""`, `"version":""`, `"skills":[]`, and the environment object has no
    `fetchedAt`; the path values project/agent and the token org reach the service.
68. Given the not-in-catalogue sentinel, Then 404.
69. (api) The audit coverage test still passes with the new controller listed; every
    route is registered through the authz registrar with `agent:read`.

---

## 13. Known limitations / out of scope

- No trigram index on the search text (needs `pg_trgm`, which no migration enables);
  search is a sequential `ILIKE` scan.
- Pipeline order is not cached across requests.
- The connect operation lives in the catalogue service (not split into its own service).
- A `q` with invalid UTF-8 returns 500.
- Filters are single-valued; the console's project picker lists only the first 50
  projects (console limitation).
- `environments` keeps pipeline order even when the reference environment is not first;
  the schema description still reads "lead first".
- `kind` on an entry is only `platform`/`external`; `proxied` is a filter, surfaced per
  environment as `proxied`.
- The `A2ACatalogAgent` summary comes from one environment only; other environments'
  names/versions are not exposed by the catalogue (the per-environment card endpoint
  already exists for that).
- PR splitting (Task 12) is out of scope.

---

## 14. Dependencies on upstream/main (what the feature needs from each)

| Upstream item | Needed for |
|---|---|
| `repositories.A2AAgentCardRepository` / `a2a_agent_cards` table, `models.A2AAgentCard`, statuses/sources | Catalogue rows; projection columns; `MarkFetched`, `ClearCard`, source-URL upsert. |
| `services.A2AAgentCardService.GetA2AAgentCard` | Connect step 1 (and its 404 semantics via `getAgent`). |
| Card reconciler (`services/a2a_card_reconciler.go`, `a2aCardHash`) and fetcher (`a2aAgentCardPath`, card validation requiring `name`, non-empty `supportedInterfaces[].url`, `skills` array) | Projection at fetch time; card URL suffix. |
| `repositories.A2APublicationRepository`, `models.A2APublication`, statuses, `ErrA2APublicationNotFound`, publication reconciler (`publishOne`, `a2aPublishResult`, `resolvedUpstream`, `deriveA2AProxyRoutes`) | Publication status; recorded transports. |
| `services.A2APublisher` (`GatewayURL`, `Publication`, `findIngressGateway`, `a2aGatewayURL`) | Gateway URL and status helper. |
| `services/a2a_agent_deployment.go` (`A2ATransport`, `a2aTransportJSONRPC`, `a2aTransportHTTPJSON`, `a2aPathPrefixJSONRPC`, `a2aPathPrefixHTTPRPC`, `buildA2AAgentDeploymentYAML`) | Transport defaults and the `/rpc` fallback. |
| `repositories.A2AProxyRepository.GetByEnvUUID`, `ErrA2AProxyNotFound`, `a2a_proxies` table | `proxied` flag (SQL EXISTS) and connect step 4. |
| `services/a2a_proxy_service.go` `GetA2AProxy` | Switch to the shared status helper. |
| `repositories.AgentConfigRepository.Get`, `ErrAgentConfigNotFound`, `models.AgentConfig` (`EnableApiKeySecurity`, `EnableOAuthSecurity`, `OAuthIssuers`, `OAuthAudiences`, `OAuthHeaderName`, `OAuthAuthHeaderPrefix`), `oauthConfigFromAgentConfig`, `models.OAuthConfig` | Security modes and OAuth view. |
| `repositories.GatewayRepository.ListIdentityProvidersByEnvironment`, `models.GatewayIdentityProvider` (`Name`, `Issuer`) | Issuer resolution. |
| `clients/openchoreosvc/client` `OpenChoreoClient.GetProjectDeploymentPipeline`, `GetEnvironment`, `buildEnvironmentOrder` (to export), `models.DeploymentPipelineResponse.PromotionPaths`, `models.EnvironmentResponse.IsProduction`, sentinels `utils.ErrProjectNotFound`, `utils.ErrDeploymentPipelineNotFound`, `utils.ErrNotFound` | Environment ordering, production flag. |
| `services.translateEnvironmentError` | Environment 404. |
| `controllers` helpers `handleCommonErrors`, `writeAgentCardError`, `agentCardPath`; `utils.WriteErrorResponse`, `utils.WriteSuccessResponse`; `utils.DefaultLimit`/`MinLimit`/`MaxLimit`/`DefaultOffset`/`MinOffset`; `utils.PathParam*` | Controller. |
| `repositories.escapeLikePattern` | Search escaping. |
| `middleware.RouteRegistrar.HandleFuncWithValidationAndAuthz`, `rbac.AgentRead`, `middleware.OUIDFromRequest` | Routes and authz. |
| `wiring/wire.go` provider sets, `wiring/params.go`, `api/app.go`, `api/audit_coverage_test.go` | Registration. |
| `golang.org/x/sync/errgroup` (vendored) | Bounded concurrent pipeline lookups with shared cancellation. |
| `db_migrations` conventions (`migration{ID, Migrate}`, `runSQL`, `migration_list.go`) | Migrations 049/050. |

---

## 15. Ambiguities resolved

1. **`environments` description vs. Task 3.** After Task 3 the reference environment can
   be a later environment while the array stays in pipeline order; I kept the array in
   pipeline order (the plan says "Environments keeps pipeline order") and left the
   schema description as the branch wrote it ("lead first"), listing it as a limitation.
2. **OpenAPI schema names.** I treated the component schema names
   (`A2ACatalogListResponse`, `A2ACatalogAgent`, `A2ACatalogEnvironment`,
   `A2AConnectResponse`, `A2AConnectSecurity`, `A2AConnectOAuth`, `A2AConnectIssuer`,
   `A2AConnectPublication`) as part of the contract because generated Go and console
   types derive from them.
3. **Platform agent with no mapped gateway.** The branch routes this through the
   "no gateway" URL branch (card URL = last fetched URL, JSON-RPC = first declared);
   kept as is since the plan says "Without a gateway, it does what it does today".
4. **Status code for pipeline failures.** The plan says the request fails; the
   controller's generic path yields 500. Specified 500.
5. **`unknown` vs. API-key default.** The backend reports `unknown` when the config row is
   missing; the console (Task 6) stops interpreting it as unmanaged. No backend defaulting.
6. **OAuth precedence.** When both security flags are set (the platform validates them
   as mutually exclusive on save), the connect view reports `oauth`; specified as the
   branch does.
7. **Task 1 state.** The branch head already contains Task 1 (migration 049, projection,
   repository changes); the only uncommitted change is an unrelated `CLAUDE.md` link.
   The spec treats Task 1 as part of the target.
8. **Row-level filters.** The repository evaluates filters per row and returns all rows
   of a matching agent; specified explicitly (the OpenAPI text "Restrict to agents
   deployed in this environment" is read this way).
9. **Pipeline lookup concurrency.** 8 in flight at once, from the branch constant; the
   plan does not change it.
10. **Migration 049 requeue vs. parked rows.** Only `status = 'fetched' AND card IS NOT
    NULL` rows are requeued, per the plan and the branch migration.


---

# PART B — CONSOLE: CATALOGUE LIST

# Clean-room specification — Console: organisation-level "A2A Agents" catalogue list

Scope of this document: the console side of the A2A catalogue **list** experience and the plumbing it needs — the new page workspace's list route, tiles, search, filters, pagination, states; the shared filter bar / filter panel extracted into `console/workspaces/libs/views` and the traces page's re-wiring onto it; route, navigation and page-export registration; workspace package conventions; the `@agent-management-platform/types` additions; the api-client fetch functions and hooks; the environment display-name labelling utility; and the URL search-param replacement helper.

Out of scope (owned by the agent-page analyst): the agent detail page, card details, compare view, connect drawer, quick-start snippets and the test-key issuer. This document only states the navigation hand-off to that page (section 4.9).

Target behaviour = the branch as it stands **with Tasks 1–11 of the structural-fixes plan applied**. For this document the relevant plan tasks are Task 11 (page parsing and clamping) on the console side, and Tasks 2 and 3 on the backend side insofar as they change what the list displays (what `matched`, `leadEnvironment` and `cardDiffers` mean). Items the plan's "Not in this plan" section left out keep the branch's behaviour and are listed under Known limitations (section 11).

---

## 1. Purpose and user stories

The platform already stores, per agent and per environment, a fetched copy of the agent's A2A agent card. The backend (other spec) exposes an organisation-wide catalogue that summarises those cards. This console feature gives every organisation member one place to see **every A2A-capable agent across all projects of the organisation**, find one by free text or by kind / environment / project / card status, and jump to its own page.

- As an organisation member, I want to open "A2A Agents" from the organisation navigation and see one tile per agent (per project), so that I can discover agents that other teams expose over A2A.
- As an organisation member, I want to type a few letters and have the list narrow to agents whose name, description or skills match, with the matching tiles visibly emphasised, so that I can find an agent without knowing its project.
- As an organisation member, I want to restrict the list to platform / external / proxied agents, to one environment, to one project, or to agents whose card is pending / fetched / failed, with the filters reflected in the URL so I can share or bookmark the view.
- As an organisation member, I want to page through a large catalogue 24 tiles at a time, and never be stranded on an empty page when the catalogue shrinks.
- As a console engineer, I want the filter bar and filter panel that the traces page introduced to be a shared component in the views library, so the catalogue and traces look and behave identically and future list pages can reuse it, with the traces page keeping its exact behaviour.

---

## 2. Binding conventions (from the repository's own guides)

These are quoted so the clean-room team applies the same rules. Sources: `console/AGENTS.md`, `console/.ai/oxygen-ui/AGENTS.md`, `.claude/skills/add-console-api-feature/SKILL.md` (project copy; the user-level copy at `~/.claude/skills/add-console-api-feature/SKILL.md` was not present).

1. **Two-file API pattern (mandatory).** Every backend call is a pure fetch function in `console/workspaces/libs/api-client/src/apis/<resource>.ts` taking `(params, query, getToken)` and using the `httpGET/POST/PUT/PATCH/DELETE` helpers from `../utils` with `SERVICE_BASE` (`/api/v1`), plus a hook in `console/workspaces/libs/api-client/src/hooks/<resource>.ts` wrapping it with `useApiQuery` / `useApiMutation` from `./react-query-notifications`, never raw `useQuery`/`useMutation`. The token comes from `useAuthHooks().getToken` and is passed into the fetch function. Both files are exported from the barrels `apis/index.ts` and `hooks/index.ts`.
2. **Query keys are tuples**: `[domain, params, query]` for a parameterised read, `[domain]` for a collection; invalidation is prefix-based.
3. **Routing**: `<Route path>` uses `relativeRouteMap.…path`; links use `absoluteRouteMap.…path` with `generatePath(path, {...})`. Never hardcode path strings. The route map lives in `console/workspaces/libs/types/src/routes/routes.map.ts`; the derived `generated-route.map.ts` is produced by `pnpm run generate-route` inside `console/workspaces/libs/types` (script `scripts/generate-route.ts`), never hand-edited.
4. **Pages** are one package per feature under `console/workspaces/pages/<feature>/`, scoped `@agent-management-platform/<feature>`, discovered via an exported `metaData` (`PageMetadata` from `@agent-management-platform/types`: `title`, `description`, `icon`, `path`, `component`, optional `levels`), wired in `core-ui` (`src/pages/index.tsx` lazy export + `src/Route/Route.tsx` route + `src/Layouts/OxygenLayout/navigationItems.tsx` nav item). `pnpm-workspace.yaml` already globs `workspaces/pages/*`, so a new folder is a workspace automatically; `core-ui/package.json` must list the new package as a `workspace:*` dependency and `pnpm install` must be re-run so `pnpm-lock.yaml` records it.
5. **UI imports** come from `@wso2/oxygen-ui` and `@wso2/oxygen-ui-icons-react`, never `@mui/material`. Use theme tokens in `sx` (`color: "text.secondary"`, `p: 2`). No floating `label` on `TextField`.
6. **Forms are manual** (`useState` + inline validation); server state is TanStack Query; local UI state is `useState`. Tests are Vitest (jsdom, `globals: true`); lint is ESLint flat config (`eslint.config.js` per package).
7. **Done checklist**: `pnpm run lint` and `pnpm run build` clean in every touched package; new API code follows the two-file pattern and is exported from the barrels; the page exports `metaData` and is routed.

Note for the controller: the skill file mentions `rushx` commands; the repository actually uses pnpm (`pnpm run lint|build|test` per package, `make build` from `console/`). The plan's console test command is `pnpm --filter @agent-management-platform/<feature> test` from `console/`.

---

## 3. Things that already exist on upstream/main and what the feature needs from them

| Existing thing (path / identifier) | What the feature uses it for |
|---|---|
| `console/workspaces/libs/shared-component/src/components/ResourceListShell/ResourceListShell.tsx` (`ResourceListShell`) | The list page's frame: toolbar slot, mutually exclusive `error` → `isLoading` → `isEmpty` → `isSearchEmpty` → children rendering, loading placeholder override, empty-state and search-empty-state rendering via `ListingTable.EmptyState`, and analytics (`ConsoleAction.Search` on a 1200 ms debounce with `scope = resourceName`, `ConsoleAction.EmptyState` once per state). The catalogue passes `resourceName="a2a-agents"`, its own `toolbar`, a `loadingPlaceholder`, `searchValue`/`onSearchChange` (so search analytics fire), and the two empty states. The shell's own default toolbar/search bar is **not** used because `toolbar` is supplied. |
| `console/workspaces/libs/views/src/component/PageLayout/PageLayout.tsx` (`PageLayout`) | Page header with `title`; the list page renders no header icon/avatar (`avatar={null}`; the branch uses the deprecated `disableIcon` prop, which is equivalent). The agent page uses `backHref`/`backLabel`/`isLoading`. |
| `console/workspaces/libs/api-client/src/hooks/deployments.ts` (`useListEnvironments({ orgName })` → `Environment[]`, each with `name`, optional `displayName`, `isProduction`, …) | Environment filter options and environment display-name labelling. |
| `console/workspaces/libs/api-client/src/hooks/projects.ts` (`useListProjects({ orgName }, query?)` → `{ projects: [...] }`, each with `name`, `displayName`) | Project filter options. Called **without** a query, so the backend's default first page is returned (see Known limitations). |
| `console/workspaces/libs/api-client/src/hooks/react-query-notifications.ts` (`useApiQuery`, `useApiMutation`) | Query wrappers. `useApiQuery` accepts every TanStack `useQuery` option plus `silent?: boolean`; when a query errors and is not silent it shows one error snackbar reading `Failed to fetch <Title Case of the key's first segment>` (segments split on `-`, `_`, whitespace; first letter of each upper-cased, rest unchanged), suppressed for HTTP 404; auth failures (401/expired session) are handled even when silent. `useApiMutation` takes `action: { verb, target }` and `showSuccess`. |
| `console/workspaces/libs/api-client/src/ClientProvider.tsx` | App-wide query defaults the hooks inherit: `refetchOnWindowFocus: true`, `staleTime: 8000` ms, retry up to 3 times except for HTTP 4xx other than 408 (never retried). |
| `console/workspaces/libs/api-client/src/utils/utils.ts` (`SERVICE_BASE`, `encodeRequired(value, label)`, `httpGET(path, { searchParams?, token?, … })`, `nullIfNotFound(promise)`, `isNotFoundError`) | URL building (`encodeRequired` URI-encodes and throws `Missing required parameter: <label>` on empty), GET with optional `Record<string,string>` search params, 404 → `null` for the entry read. |
| `console/workspaces/libs/api-client/src/apis/agent-card.ts` and `apis/a2a-proxy.ts` (existing per-environment card/proxy fetchers) | Modified by this feature to share one path builder (section 8.3). |
| `console/workspaces/libs/api-client/src/hooks/a2a-query-keys.ts` (`agentCardQueryKey`, `a2aProxyQueryKey`, `invalidateA2AQueries`) | The existing per-environment key convention: `["a2a", orgName, projName, agentName, envId]` prefix with a trailing `"card"` / `"proxy"` segment, invalidated together. See section 8.5 for how the catalogue hooks relate to it. |
| `console/workspaces/libs/api-client/src/apis/agent-api-keys.ts` (`issueTestAgentAPIKey(params, getToken)` → `IssueTestAgentAPIKeyResponse`) | Wrapped by a new on-demand mutation hook (section 8.4) that the agent page's test-key issuer uses. |
| `console/workspaces/libs/types/src/api/common.ts` (`ListQuery { limit?, offset? }`, `PaginationMeta { total, limit, offset }`) | Base types for the catalogue query and list response. |
| `console/workspaces/libs/types/src/api/agent-card.ts` (`AgentCardPathParams { orgName, projName, agentName, envId }`) | Path params of the per-environment connect read. |
| `console/workspaces/libs/types/src/page-metadata/index.ts` (`PageMetadata`) | Page export shape. |
| `console/workspaces/libs/types/src/routes/routes.map.ts`, `generated-route.map.ts`, `scripts/generate-route.ts`, `types.ts` (`AppRoute { path, wildPath?, index?, children }`) | Route registration and generation. |
| `console/workspaces/core-ui/src/pages/index.tsx`, `src/Route/Route.tsx`, `src/Layouts/OxygenLayout/navigationItems.tsx`, `core-ui/package.json` | Page export, route and nav registration; package dependency. The `Network` icon from `@wso2/oxygen-ui-icons-react` is already imported in `navigationItems.tsx`. |
| `console/workspaces/pages/eval/src/EvalEvaluators.Organization.tsx` | The visual precedent for the tile grid: a CSS grid with `repeat(auto-fill, minmax(260px, 1fr))` on `xs` and `minmax(300px, 1fr)` from `md`, each tile a `Form.CardButton` wrapped in a router `Link`. |
| `@wso2/oxygen-ui` 0.15.0: `Form.CardButton` (props include `selected`, `disabled`, `onClick`, Card props), `Form.CardHeader`, `Form.CardContent`, `SearchBar`, `TablePagination`, `Select`, `MenuItem`, `ToggleButtonGroup`, `ToggleButton`, `ButtonGroup`, `Button`, `Chip`, `Tooltip`, `Skeleton`, `Popper`, `Grow`, `Paper`, `IconButton`, `Stack`, `Box`, `Typography`, `Divider`; icons `Network`, `Bot`, `Search`, `ListFilter`, `X` from `@wso2/oxygen-ui-icons-react` | All UI primitives. The views library already depends on both Oxygen packages at these versions. |
| `console/workspaces/pages/agent-security/` | The sibling page workspace whose build/test configuration the new workspace mirrors (section 7). |
| `console/workspaces/pages/traces/src/traceFilters.ts`, `subComponents/TraceFilterControls.tsx`, `subComponents/TraceIdSearch.tsx`, `Traces.test.tsx` | Unchanged by this feature; the traces filter bar and drawer are rewired onto the shared components and `Traces.test.tsx` must pass unchanged (section 6). |

---

## 4. The catalogue list page — externally observable behaviour

### 4.1 Routes and page skeleton

| Item | Value |
|---|---|
| List route (absolute) | `/org/:orgId/a2a-agents` |
| Wild path (nav active match) | `/org/:orgId/a2a-agents/*` |
| Agent page route (hand-off) | `/org/:orgId/a2a-agents/view/:projectName/:agentName` (see 4.9) |
| Any other sub-path under the feature | Client-side redirect (history **replace**) to the list route for the same `orgId`. |
| Route map entry | A new child under the organisation node of `rootRouteMap` with relative path `a2a-agents`, `index: true`, and one child with relative path `view/:projectName/:agentName`, `index: true`, no further children. The generated map must then contain `path: "/org/:orgId/a2a-agents"`, `wildPath: "/org/:orgId/a2a-agents/*"`, and the child `path: "/org/:orgId/a2a-agents/view/:projectName/:agentName"`, `wildPath: ".../*"`. The key names under `children` are the builder's choice. |
| `Route.tsx` registration | Inside the organisation subtree (same level as the gateways / settings / deployment-pipelines / environments / llm-providers / mcp-proxies org routes), a `<Route path={<relative org a2a path> + "/*"} element={<lazy org-level component />} />`. The branch places it between the llm-providers org route and the mcp-proxies org route; order is not behaviourally significant. |
| `pages/index.tsx` registration | A `lazy(() => import("@agent-management-platform/<feature>").then(m => ({ default: m.metaData.levels!.organization })))` export, following the existing pattern for the other org-level pages. |
| Page `metaData` | `title: "A2A Agents"`, `description: "Organization-wide catalogue of A2A agents"`, `icon: Network` (from `@wso2/oxygen-ui-icons-react`), `path: "/a2a-agents"`, `component` = the organisation-level component, `levels: { organization: <same component> }`. The package's default export is that component too. |
| Organisation-level component | Renders nested `Routes`: index → `PageLayout` titled **"A2A Agents"** with no avatar/icon, containing the catalogue list for `orgId` (read from `useParams`, defaulting to `""`); `view/:projectName/:agentName` → the agent page; `*` → `Navigate replace` to the list route. |

### 4.2 Navigation item

In `navigationItems.tsx`, in the **organisation-level** item list (the branch taken when `orgId` is set and no project is selected), insert an item **after "Agent Catalog" and before the conditional "Settings" item**:

- `label: "A2A Agents"`, `type: "item"`, `icon: <Network size={20} />`
- `href`: `generatePath(<absolute list path>, { orgId })`
- `isActive`: `!!matchPath(<absolute wild path>, pathname)` — so the item stays highlighted on the agent page too.

### 4.3 Layout of the list

Top to bottom inside the page layout:

1. **Toolbar** (the `ResourceListShell` `toolbar` slot) = the catalogue filter bar (4.5): left-aligned kind toggle, active-filter chips and "Clear all"; right-floated search box and "Advanced Filters" button.
2. **Body** (`ResourceListShell` states, evaluated in this order): error → loading → empty → no-match → grid.
3. **Grid**: a CSS grid, gap 2 theme units, columns `repeat(auto-fill, minmax(260px, 1fr))` below `md` and `repeat(auto-fill, minmax(300px, 1fr))` from `md`. One tile per catalogue entry, React key `"<projectName>/<agentName>"` so the same agent name in two projects yields two tiles.
4. **Pagination** (below the grid, only when `total > 24`): Oxygen `TablePagination` rendered as a `div`, `rowsPerPageOptions={[]}` (no page-size chooser), `count = total`, `rowsPerPage = 24`, `page` = the current 0-based page, `onPageChange` → write the page (4.7). Default MUI displayed-rows text (e.g. "1–24 of 57").

There is **no** side panel, drawer, selection state or list/grid toggle on the list page any more (earlier branch commits had a collapsible list with a card side panel; the final design opens agents on their own page). Nothing on the list page is "selected" except the search-match emphasis below.

### 4.4 Tile contents

Each tile is a router `Link` (no text decoration) to the agent page URL (4.9) wrapping an Oxygen `Form.CardButton` with `selected` = the match emphasis flag, full width/height, `minHeight: 200`, left-aligned text, column flex, content top-aligned. Builders should give each tile and each environment status dot a stable test id (names are theirs).

Header (`Form.CardHeader`, content allowed to shrink, overflow hidden):

| Element | Rule / copy |
|---|---|
| Title | `name` from the entry, falling back to `agentName` when `name` is empty. `h6`, single line with ellipsis, and a tooltip (placement top) showing the full title. |
| Second line, left to right | (a) a small outlined primary `Chip` whose label is the raw `kind` value (`platform` or `external`, lowercase as returned); (b) a vertical divider; (c) the `projectName` in caption style, secondary colour, single line; (d) **only when the lead environment's card is not fetched**: a caption in warning colour reading `card pending` when the lead environment's `status` is `pending`, or `card failed` when it is `failed`. The lead environment is the entry in `environments` whose `name === leadEnvironment`. (After plan Task 3 the backend's lead is the first environment in pipeline order **with a fetched card**, else the first environment; so this caption appears only when no environment of the agent has a fetched card.) |

Body (`Form.CardContent`, column flex, `justifyContent: space-between`, grows, gap 1, bottom padding fixed at 2 so hover does not shift the layout):

| Element | Rule / copy |
|---|---|
| Description | `description`, caption style, secondary colour, clamped to 2 lines (webkit line clamp). May be empty. |
| Skills row (only when `skills.length > 0`) | The first **2** skills as small default `Chip`s (non-shrinking), then, if there are more, a caption `+<n> more` where `n = skills.length - 2`, with a tooltip (placement top) listing **all** skills joined by `", "`. Row does not wrap; overflow hidden. |
| Environments row (wraps, gap 1.5) | One status group per entry in `environments`, in the order returned (pipeline order, lead first). |

Environment status group (row, spacing 0.5, non-shrinking):

| Element | Rule / copy |
|---|---|
| Status dot | 8×8 circle; background `success.main` for `fetched`, `warning.main` for `pending`, `error.main` for `failed`; native `title` attribute = the raw status string. |
| Label | caption, secondary colour: the environment's **display name** via the labelling utility (section 9), i.e. `displayName` when non-empty else `name`. |
| `proxied` chip | small default `Chip` labelled `proxied`, only when `proxied === true`. |
| `card differs` chip | small **warning** `Chip` labelled `card differs`, only when `cardDiffers === true`. (After plan Task 3 the flag is relative to the lead environment's stored fingerprint and is never set during the fingerprint upgrade window.) |

Small chips: height 20, font size 0.7rem, label horizontal padding 0.75.

Search-match emphasis: the tile's `selected` flag is `true` iff the **debounced, trimmed** search text is non-empty **and** at least one of the entry's environments has `matched === true`. With an empty search no tile is emphasised, whatever `matched` says.

### 4.5 Toolbar: search, kind toggle, advanced filters

Built on the shared filter bar (section 5). Layout: the shared bar's `actions` slot holds, in order, the search box and the "Advanced Filters" button; its children hold the kind toggle, then one chip per active advanced filter, then (conditionally) "Clear all".

**Search box**: Oxygen `SearchBar`, size small, placeholder **`Search A2A agents`**, width 300 px (max 100%). Controlled by the page's local search text state. Typing does not fetch immediately (4.6).

**Kind toggle**: the shared toggle with accessible name **`Kind`** and buttons **`All`**, **`Platform`** (`platform`), **`External`** (`external`), **`Proxied`** (`proxied`). Selecting applies immediately: the URL's `kind` param is set (or removed for All) and `page` is removed. The kind is **not** part of the advanced-filter draft.

**"Advanced Filters" button**: outlined, `color="inherit"`, start icon `ListFilter` size 16, text **`Advanced Filters`** (no text transform, border colour divider), `aria-expanded` = whether the panel is open, `aria-controls` = the panel's element id. Click toggles: when the panel is closed, copy the currently applied values into the draft and open it; when open, close it (draft discarded).

**Advanced filter panel**: the shared panel (5.4) anchored to that button, title **`Advanced Filters`**, width **640** px, apply never disabled. Body: a `Stack` that is a column below `sm` and a row from `sm`, spacing 2, containing three labelled selects in this order, each `flex: 1`, `minWidth: 180`, a `body2` secondary-colour label above a small, full-width, `displayEmpty` `Select` whose accessible name is the label (via `labelId`):

| Label | Options (value → label) | Source |
|---|---|---|
| `Environment` | `""` → `Any`; then one per organisation environment: `name` → `displayName ?? name` (note: `??`, so an empty-string display name renders an empty option label — kept as-is, see Known limitations) | `useListEnvironments({ orgName })` |
| `Project` | `""` → `Any`; then one per project: `name` → `displayName || name` | `useListProjects({ orgName })` (no query) |
| `Card status` | `""` → `Any`; `fetched` → `Fetched`; `pending` → `Pending`; `failed` → `Failed` | static |

Panel buttons: **`Reset`** sets the draft's three advanced values to `""` (kind untouched); **`Filter`** applies the draft — writes `environment`, `project`, `status` from the draft and keeps the currently applied `kind` (the toggle may have changed while the panel was open) — then closes the panel. Escape anywhere, or the panel's close button (aria-label `Close advanced filters`), closes without applying. Changing a select while the panel is open never fetches.

**Active-filter chips**: one shared chip per advanced filter whose applied value is non-empty, in the order Environment, Project, Card status. Chip `name` = the filter label, `value` = the option label for the applied value, or the raw value when it is not among the options (e.g. a URL value for an environment that no longer exists); the chip's full `label` (its `title` and the remove button's accessible name) is **`<name>: <value>`**, e.g. `Card status: Failed`, remove button accessible name `Remove Card status: Failed`. Clicking the chip body opens the panel on the applied values (if already open, keeps the current draft); clicking its × removes just that filter (URL and draft) and deletes `page`. The kind filter gets **no** chip (the toggle shows it).

**"Clear all"**: small text button labelled **`Clear all`**, shown only when `(number of active advanced filters) + (1 if kind is set) >= 2`. Clicking resets kind and all three advanced filters at once (one URL write), resets the draft, deletes `page`. The panel, if open, stays open.

### 4.6 URL query-string state and the request

All list state lives in the URL's query string of the list route. Every write uses the replacement helper (section 10): the current params are copied, mutated, and written with **history replace** (no new history entry).

| Key | Values | Written by | Notes |
|---|---|---|---|
| `q` | trimmed search text | debounced search effect | removed when empty |
| `kind` | `platform` \| `external` \| `proxied` | kind toggle, Clear all | removed when All |
| `environment` | environment `name` | panel Filter, chip ×, Clear all | removed when empty |
| `project` | project `name` | panel Filter, chip ×, Clear all | removed when empty |
| `status` | `pending` \| `fetched` \| `failed` | panel Filter, chip ×, Clear all | removed when empty |
| `page` | 0-based page index as a decimal string | pagination, clamp rule | removed when 0; **deleted** by any search/filter write |

Reading: the four filter values are read raw from the URL (`params.get(key) ?? ""`), without validation; an unknown value (e.g. `kind=bogus`) is sent to the API as-is, and the backend's 400 surfaces as the error state. The page is parsed by the rule in 4.7.

**Search debounce**: the search box state is initialised from the URL's `q` on mount. The value the page acts on is the **trimmed** box text debounced by **300 ms** (a timer restarted on every change). When the debounced value settles and differs from the URL's `q`, the URL is updated (set or delete `q`, delete `page`). The request uses the debounced value directly (not re-read from the URL). Consequence: when `q` changes by other means while the list is mounted, the box text wins — the debounced writer re-applies it to the URL (Known limitations).

**The request** (the list hook, section 8.2) is issued with `params = { orgName }` and a query object whose fields are: each of `kind`, `environment`, `project`, `status` when non-empty (else undefined), `q` = the debounced search when non-empty (else undefined), `limit = 24`, `offset = page * 24`. The fetch function drops undefined and empty values and stringifies the rest (so `limit: "24"`, `offset: "0"`).

"Is filtering" = any of the four filter values is non-empty **or** the debounced search is non-empty. It decides which empty state shows (4.8).

### 4.7 Pagination rules (plan Task 11, closes finding #14)

- Page size is **24**. The visible page index is 0-based; the URL stores it under `page` and omits it for page 0.
- **Parsing** the `page` param: the result is a non-negative integer. `null` (absent) → 0; a string that does not parse as a finite number → 0; a negative number → 0; a fractional number is **truncated** toward zero (`"1.5"` → 1). Examples from the plan: `"-1"` → 0, `"1.5"` → 1, `"abc"` → 0, `null` → 0. The offset sent is therefore always a non-negative multiple of 24.
- **Clamp to the last page**: whenever a list response arrives with `agents.length === 0`, `total > 0` and the current page `> 0`, the page is replaced with `ceil(total / 24) - 1` (the param is deleted when that is 0), which triggers a new request for the last non-empty page. The regression this closes: with `?page=3`, a response `{ agents: [], total: 20 }` must lead to the next request using `offset: 0` (the param removed). A response with `total === 0` never clamps (the empty states apply).
- Changing a page via the paginator writes `page` (or deletes it for 0) and keeps every other param.
- Any search or filter write deletes `page`, so a new query always starts from the first page.

### 4.8 States and exact copy

Evaluated in this order by the shell:

| State | Condition | What is shown (exact copy) |
|---|---|---|
| Error | the list query has an error (and the hook reports it — note `placeholderData` keeps old data visible only while fetching; once the query errors with no fresh data the shell shows the error) | The shell's error alert: the error's `message` when it is an `Error` instance, otherwise **`Failed to load. Please try again.`**. In addition, because the list hook is not silent, the query wrapper shows one error snackbar reading **`Failed to fetch A2a Agents`** (derived from the hook's query-key domain `a2a-agents`; 404s do not snackbar). |
| Loading | the list query's `isLoading` is true (first load, no data yet) | The grid layout filled with **4** rounded `Skeleton`s of height 200. Later page/filter changes do not show this state because the previous page is kept as placeholder data (see 8.2). |
| Empty | `total === 0` and not filtering | Illustration `Bot` icon size 64; title **`No A2A agents yet`**; description **`Agents that expose an A2A card show up here once deployed.`** No action button. |
| No match | `total === 0` and filtering | Illustration `Search` icon size 64; title **`No A2A agents match your search`**; description **`Try a different keyword or clear the filters.`** |
| Grid | otherwise | Tiles (4.4) and, when `total > 24`, the paginator. |

`total` is `data.total` or 0 while there is no data. The environment-label and filter-option lookups (`useListEnvironments`, `useListProjects`) are best-effort decorations: while they load or if they fail, labels fall back to raw names and option lists are empty; they never block the grid.

### 4.9 Navigation hand-off to the agent page (owned by the other analyst)

- Tile link target: `generatePath("/org/:orgId/a2a-agents/view/:projectName/:agentName", { orgId: <organisation name>, projectName: entry.projectName, agentName: entry.agentName })`. Example: `/org/acme/a2a-agents/view/p1/orders`.
- The agent page reads `orgId`, `projectName`, `agentName` from the route params and an optional `env` query param (the environment name to show; defaults to the entry's `leadEnvironment`). The list never writes `env`.
- The agent page's back link goes to the list route for the same `orgId`, labelled `Back to A2A Agents`; it does **not** restore the list's query-string state (search/filters/page are lost on the way back; the 30 s stale time means the previous unfiltered page is served from cache).
- The agent page reads its entry through the catalogue-entry hook (8.2) and uses the same URL-replacement helper (section 10) and environment labelling (section 9).

### 4.10 Keyboard and accessibility summary

- Each tile is a link: Tab focuses it, Enter opens the agent page.
- Kind toggle: a toggle-button group with accessible name `Kind`; the pressed button has `aria-pressed="true"`; the `All` button is pressed when no kind is set.
- Advanced Filters button: `aria-expanded` reflects the panel; `aria-controls` names the panel element.
- Panel: an `aside` landmark (`role="complementary"`) with accessible name `Advanced Filters`, focused on open (without scrolling), heading level 2 with the title, close button accessible name `Close advanced filters`; Escape anywhere closes it (an open select menu consumes its own Escape first, so a second Escape closes the panel).
- Each select's accessible name is its label (`Environment`, `Project`, `Card status`); options are `role="option"` with the labels above.
- Chips: body button has `title` = full label; × button accessible name `Remove <full label>`.

---

## 5. Shared filter bar and filter panel (views library)

Extract four small building blocks into `console/workspaces/libs/views/src/component/` (a new sub-folder; export it from `component/index.ts` so it is importable from `@agent-management-platform/views`). They carry **no page-specific state**: the page owns the applied values, the draft and the open flag. Identifier names are the builder's choice; the branch's names are deliberately not given here.

### 5.1 The bar (layout only)

Props: `actions: ReactNode` (fixed on the right of the first line: search, the Filters button), `children: ReactNode` (toggles, chips, Clear all, inline on the left).

Behaviour/layout: a `flow-root` block with bottom margin 1; every direct child is `inline-flex`, vertically centred (`verticalAlign: middle`), with right and bottom margins of 1. The `actions` are rendered first inside a horizontal, wrapping `Stack` (spacing 1, `useFlexGap`, items centred) that **floats right**, `maxWidth: 100%`, `minWidth: 0`, right margin 0 and left margin 1 (overriding the child rule with a higher-specificity selector). Children that do not fit on the first line wrap below across the full width, so the actions never move.

### 5.2 The toggle ("All or one option; applies straight away")

Props: `label: string` (the group's accessible name), `value?: string` (selected option; `undefined` selects All), `options: { value: string; label: string }[]`, `onChange(value?: string)`.

Behaviour: an exclusive, small `ToggleButtonGroup` with `aria-label = label`, whose value is `value ?? "all"` (an internal sentinel). Buttons: **`All`** first, then one per option with the option's label. On change: a `null` next value (the user clicked the already-pressed button) is ignored; the sentinel maps to `onChange(undefined)`; anything else to `onChange(next)`. Buttons have horizontal padding 1.75 and no text transform.

### 5.3 The chip ("the filter's name and value, which open the panel, and an × that removes it")

Props: `name: string`, `value: string`, `label?: string` (the whole chip, for its `title` and the × label; defaults to `"<name> <value>"`), `onOpen()`, `onRemove()`.

Behaviour: a small, outlined, `color="inherit"` `ButtonGroup`, `maxWidth: 320`, buttons' border colour `divider`. First button: `title = label`, `onClick = onOpen`, no text transform, `minWidth: 0`; contents: the `name` in `text.secondary`, `nowrap`; a space; the `value` in weight 600, left margin 0.75, single line with ellipsis. Second button: `aria-label = "Remove <label>"`, `onClick = onRemove`, `minWidth: 0`, horizontal padding 0.75, containing the `X` icon at size 14.

### 5.4 The panel ("panel under the Filters button that edits a draft; Filter applies it, Escape discards it")

Props: `anchorEl: HTMLElement` (the Filters button), `id: string` (for the button's `aria-controls`), `title: string` (heading and accessible name), `width: number`, `applyDisabled?: boolean` (default false; blocks Filter, e.g. while a value is invalid), `onReset()`, `onApply()`, `onClose()`, `children` (the fields; scrolls only if the window is short).

Behaviour:
- Rendered through a `Popper` that is always `open` while mounted (the page mounts it only when open and when the anchor exists), `placement="bottom-end"`, with a `Grow` transition whose transform origin is `right bottom` when the resolved placement starts with `top`, else `right top`; `role="presentation"` on the popper (the paper inside is the landmark); z-index = the theme's `mobileStepper` z-index.
- Popper modifiers: an offset of **6 px** below the anchor; and a custom `beforeWrite` modifier that, on every Popper update (including resize and scroll), sets a CSS custom property on the popper element to `window.innerHeight − anchor bottom − 6 − 16` px; the paper's `maxHeight` reads that property, so the panel never extends past the window bottom (16 px margin) and its body scrolls instead.
- The `Paper`: rendered as `aside`, `aria-label = title`, `id = id`, `tabIndex = -1`, focused on mount via a callback ref with `preventScroll: true` (callback ref because the portal mounts after the component), elevation 8, `width = width`, `maxWidth: calc(100vw - 32px)`, column flex, border 1 in `divider`, radius 1.5, `overflow: hidden`, no focus outline, opaque `background.default` with background image and backdrop filter disabled (so the list underneath does not show through).
- Header row (padding left 2.5, right 1.5, vertical 1.5, bottom border `divider`): `Typography variant="h6" component="h2"` with the title, and a small `IconButton` with `aria-label = "Close " + title.toLowerCase()` (so `Close advanced filters` for both pages) containing `X` at size 18, calling `onClose`.
- Body: a `Stack` with `flex: 1`, `minHeight: 0`, `overflowY: auto`, horizontal padding 2.5, vertical padding 2, containing `children`.
- Footer row (horizontal padding 2.5, vertical 1.5, top border): a text button **`Reset`** → `onReset`; a contained button **`Filter`** → `onApply`, `disabled = applyDisabled`.
- A document-level `keydown` listener for the lifetime of the panel: `Escape` → `onClose()`. (Oxygen/MUI menus stop propagation of their own Escape, so an open select menu closes first.)

### 5.5 Division of responsibility (what the page must do)

The shared pieces are stateless. A page that uses them must: hold the applied filters (URL), a draft (local state), and an open flag; open the panel by copying applied → draft (unless already open); apply by writing the draft in **one** URL update and closing; discard by closing without writing; remove a chip by clearing that key in both the URL and the draft; "Clear all" by clearing everything in one write and resetting the draft; show "Clear all" only when at least two filters (toggle included) are active; and keep the toggle, chips and Clear all live even while the panel is open.

---

## 6. Traces page re-wiring onto the shared components (no behaviour change)

Files modified: `console/workspaces/pages/traces/src/subComponents/TraceFilterBar.tsx` and `TraceFiltersDrawer.tsx`. Everything else on the traces page (`traceFilters.ts`, `TraceFilterControls.tsx`, `TraceIdSearch.tsx`, `TracesView.tsx`, `Traces.Component.tsx`, `Traces.test.tsx`) stays as on upstream/main, and **`Traces.test.tsx` must pass unchanged**.

### 6.1 What upstream's `TraceFilterBar` does today (contract to preserve)

Props: `filters: TraceFilters`, `onChange(filters)`, `open: boolean`, `onOpenChange(open)`, `onTraceSearch?(traceId)` (the trace-ID field shows only when set), `children?` (the list rendered below the bar).

- Local state: a draft (`TraceFilterDraft`, initialised from `draftFrom(filters)`) and the Filters button element (needed as the panel anchor).
- Chips = `traceFilterChips(filters)` **minus** the `status` chip (the toggle shows status).
- Opening: if already open do nothing; else `draft = draftFrom(filters)` and `onOpenChange(true)`.
- Removing a chip: delete the chip's key **and** its `alsoClears` keys from a copy of `filters`, call `onChange`, and apply `draftWithout(draft, keys)` to the draft.
- Clear all: `onChange({})` and `draft = EMPTY_DRAFT`.
- Apply: `onChange(appliedFilters(draft, filters.status))` and `onOpenChange(false)`.
- Rendered layout (now provided by the shared bar, 5.1): right-floated actions = the trace-ID search (when `onTraceSearch`) followed by the **`Advanced Filters`** button (outlined, inherit colour, `ListFilter` 16 start icon, no text transform, divider border, `aria-expanded = open`, `aria-controls = "trace-filters-drawer"`, click → close if open else open); left = the status toggle, the chips, and **`Clear all`** (small text button) when `chips.length + (filters.status ? 1 : 0) >= 2`.
- Status toggle (now the shared toggle, 5.2) with accessible name **`Status`**, buttons **`All`**, **`Errors`** (`error`), **`Success`** (`ok`); change → `onChange({ ...filters, status })` where status is `undefined` for All. Upstream coerced any value other than `error`/`ok` to `undefined`; with the shared toggle only those two option values can be emitted, so behaviour is identical.
- Chips use the shared chip (5.3) with `name = chip.name`, `value = chip.value`, `label = chip.label`, `onOpen` = open, `onRemove` = remove.
- `children` render after the bar; the drawer renders only when `open && filtersButton`.

### 6.2 What upstream's `TraceFiltersDrawer` does today (contract to preserve)

Exports the constant id `"trace-filters-drawer"` (used by the button's `aria-controls`). Props: `anchorEl`, `draft`, `onDraftChange`, `onApply`, `onClose`.

- `valid = isDraftValid(draft)`; `submit()` calls `onApply` only when valid; the section fields get `{ draft, onDraftChange, onSubmit: submit }` so Enter in a text field applies (unless invalid).
- Now rendered with the shared panel (5.4): `id = "trace-filters-drawer"`, `title = "Advanced Filters"`, `width = 760`, `applyDisabled = !valid`, `onReset = () => onDraftChange(EMPTY_DRAFT)`, `onApply = submit`, `onClose`.
- Children (unchanged content): two columns — a `Stack` that is a column below `sm` and a row from `sm`, spacing 3, with a vertical `Divider` between columns; left column (`flex: 1`, `minWidth: 0`, spacing 2.5, horizontal dividers between sections): the **Performance** section (threshold fields for `minDurationMs`, `minTokens`, `minSpanCount`) then the **Conversation** section (monospace `Conversation ID` text field, disabled and blanked while the "missing" box is checked; checkbox **`Only traces without a conversation ID`** that sets `missing` to `["conversationId"]` or `[]`); right column: the **Model and tools** section (`Model` placeholder `e.g. gpt-4o`, `Tool called` placeholder `e.g. search_web`, checkbox **`Only traces where a tool call failed`** bound to `toolError`, `MCP server` placeholder `e.g. github`). Section titles and controls come from `TraceFilterControls.tsx` unchanged.
- Only structural difference versus upstream: upstream made the two-column `Stack` itself the scrolling body (flex 1, min-height 0, overflow-y auto, padding 2.5/2); with the shared panel the panel's body wrapper scrolls and the two-column stack sits inside it. Visually equivalent; no test depends on it. The CSS custom property name used for the max-height is internal and may change.

### 6.3 Traces behaviours pinned by `Traces.test.tsx` (must keep passing)

Given/When/Then, each corresponding to an existing test:

- Given `?timeRange=1h&status=error&minDurationMs=5000&model=gpt-4o&minTokens=abc`, when the page renders, then the request carries `{ status: "error", minDurationMs: 5000, model: "gpt-4o" }`, the `Errors` toggle button is pressed, no `Status: Error` chip exists, chips titled `Latency ≥ 5s` and `Model: gpt-4o` exist, and no `Tokens ≥` chip exists.
- Given the drawer open, when `10s` latency and `5k` tokens pills are picked and `Filter` clicked, then the URL has `minDurationMs=10000`, `minTokens=5000`, keeps `timeRange` and `limit`, and the request matches.
- Given the drawer open, when `gpt-4o` is typed into `Model`, then the URL is unchanged until Enter is pressed in that field, after which `model=gpt-4o` is set and the drawer is closed; typing ` conv-1 ` into `Conversation ID` and clicking `Filter` writes `conversationId=conv-1` (trimmed).
- Given the drawer open, then `Model`, `Tool called`, `MCP server`, `Conversation ID` fields each have `maxLength="256"`.
- Given `?status=error&minSpanCount=20&model=gpt-4o`, when `Remove Steps ≥ 20` is clicked, then only `minSpanCount` is removed; when `Clear all` is clicked, then `status` and `model` are removed, `timeRange` kept, and the request filters are `{}`.
- Given a single active filter (either `?status=ok` or `?model=gpt-4o`), then no `Clear all` button is shown.
- Given `?selectedTrace=t-err` (the trace still matches), when `Errors` is clicked, then `selectedTrace` is kept; given `?selectedTrace=t-ok`, when `Errors` is clicked, then `status=error` is written and `selectedTrace` removed (page logic, unchanged).
- Given `?timeRange=1h`, then `All` is pressed; clicking `Errors` writes `status=error`; `Success` writes `status=ok`; `All` removes it, leaving exactly `timeRange=1h`.
- Given the drawer open (`aria-expanded="true"` on the button), when `5s` is picked and `gpt-4o` typed, then nothing is written; when `Filter` is clicked, then exactly **one** URL update occurs carrying both values, the drawer closes, `aria-expanded` is `false`, and both chips appear.
- Given `?minDurationMs=5000`, when the drawer is opened and `Filter` clicked without changes, then **no** URL update occurs and the drawer closes.
- Given the drawer open with a picked pill, when `Close advanced filters` is clicked, then the drawer closes, the URL is unchanged, and reopening shows `Any` pressed; given typed `Model` text, when Escape is pressed in the field, then the drawer closes, nothing is written, and reopening shows the field empty.
- Given `?status=error&minDurationMs=5000&model=gpt-4o&toolError=true`, when the drawer's `Reset` is clicked, then the pills, field and checkbox clear but the URL is unchanged until `Filter`, after which the URL is exactly `timeRange=1h&status=error` (status survives Reset).
- Given chips present, when the `Latency ≥ 5s` chip body is clicked, then the drawer opens with `5s` pressed and `Model` holding `gpt-4o`; when `Remove Model: gpt-4o` is clicked, then `model` is removed, `minDurationMs` kept, and the drawer stays closed.
- Given the drawer open with `5k` picked, when `Errors`, then `Remove Latency ≥ 5s`, then `Clear all` are clicked, then each applies immediately (URL updated each time), the draft reflects removals (`Any` pressed for latency, `5k` still pressed for tokens until Clear all, `Model` still `gpt-4o` until Clear all), and the drawer **remains open** after Clear all.
- Given every filter set, when `Clear all` is clicked, then the URL is exactly `timeRange=1h`, `All` is pressed and no `Remove …` button exists.
- Given `?minDurationMs=2500`, then a chip `Latency ≥ 2500ms` exists and the latency pills read `Any, 1s, 2500ms, 5s, 10s, 30s, Custom` with `2500ms` pressed.
- Given a custom latency, then `2.5` writes `minDurationMs=2500`; given invalid values (`-1`, `5k`, `1e3`), then the field is `aria-invalid` and `Filter` is disabled; Enter with an invalid value leaves the drawer open and the URL unchanged.
- Given `?selectedTrace=t-err` (a trace open), when `Advanced Filters` is clicked, then no drawer appears (page logic).
- Given the drawer open with a pill picked, when a trace ID is submitted in `Go to trace ID` or a row is clicked, then the drawer closes, its draft is discarded and `selectedTrace` is set (page logic).

The test helpers locate the drawer as `role="complementary"` named `Advanced Filters`, the status group as `role="group"` named `Status`, the Filters button by name `Advanced Filters`, chips by `title`, the close button by name `Close advanced filters`, and the apply button by name `Filter` — the shared components must preserve these roles and names exactly.

---

## 7. New page workspace: package and configuration conventions

Create `console/workspaces/pages/<feature>/` (the branch uses the URL segment as the folder and package name, giving `@agent-management-platform/a2a-agents`; the plan's test command assumes that name). Mirror the **agent-security** page workspace (`console/workspaces/pages/agent-security/`), whose `vitest.config.ts` and `setupTests.tsx` the branch copied verbatim:

| File | Requirement |
|---|---|
| `package.json` | `name` `@agent-management-platform/<feature>`, `version` `1.0.0`, `type` `module`, `types`/`exports` pointing at `dist/index.d.ts` and `dist/index.js`, `files: ["/dist"]`, `license` `Apache-2.0`, `description` `A2A Agents catalogue page for the organization level`, `keywords` `ai, agents, page, react`. Scripts identical to the sibling: `format`, `lint` (`eslint --config ./eslint.config.js`), `lint:fix`, `test` (`vitest run`), `test-watch`, `test:ui`, `clean`, `copy-assets`, `build` (`clean` → `tsc -project tsconfig.lib.json` → `copy-assets`), `dev`. Dependencies: `react-dom 19.2.3`, `react-router-dom 6.30.6`, `@mui/material 7.0.0`, `@emotion/react 11.13.5`, `@emotion/styled 11.13.5`, `@wso2/oxygen-ui 0.15.0`, `@wso2/oxygen-ui-icons-react 0.13.1`, and `workspace:*` on `@agent-management-platform/views`, `/types`, `/shared-component`, `/api-client`. Dev dependencies: `@types/jest 29.5.14`, `@testing-library/jest-dom 6.6.3`, `@testing-library/react 16.3.0`, `jsdom 26.1.0`, `vitest 3.1.3`, `vite 6.3.5`, `@vitejs/plugin-react 5.0.4`, `vite-plugin-dts 4.5.4`, `typescript 5.9.3`, `eslint 9.36.0`, `@eslint/eslintrc 3.3.1`, `@agent-management-platform/eslint-config workspace:*`, `@types/node 24.6.0`, `@types/react 19.2.14`, `@types/react-dom 19.1.9`, `react 19.2.3`. Peer dependency `react 19.2.3`. (Versions are those pinned across the monorepo; match the sibling.) |
| `vite.config.ts` | Same as the sibling: library build with `peerDependencies` externalised, `cssCodeSplit: false`, no sourcemap, CSS modules with `[local]_[hash:base64:5]` and camelCase; `test: { globals: true, environment: "jsdom", setupFiles: "./setupTests" }`; plugins `react()` and `dts({ include: ["src"], exclude: ["**/*.stories.tsx", "**/*.test.tsx"] })`. |
| `vitest.config.ts` | Identical to the sibling: `react()` plugin; `test.globals true`, `environment jsdom`, `setupFiles ./setupTests.tsx`, and `server.deps.inline: ["@wso2/oxygen-ui", "@mui/x-data-grid"]` (Oxygen's dist imports prismjs sub-paths without extensions and x-data-grid imports raw CSS, which Node's ESM loader rejects). |
| `tsconfig.json` | Same compiler options as the sibling (ES2020, bundler resolution, strict, `jsx: react-jsx`, `declaration`, `types: ["jest", "node"]`, `outDir dist`); `include` `src`, `.storybook`, `vite.config.ts`, `setupTests.tsx`; `exclude` `src/**/__docs__`, `src/**/__test__`, `node_modules`, `dist`, `scripts`. |
| `tsconfig.lib.json` | Extends `tsconfig.json`; `rootDir src`, `declarationDir dist`, a `paths` alias `@<feature>/*` → `./src/*`; excludes tests, stories and specs from the library build. |
| `eslint.config.js` | Flat config spreading `@agent-management-platform/eslint-config`, scoped to `**/*.{ts,tsx,js,jsx,mjs,cjs}`, ignoring `dist`, `node_modules`, `coverage`, `.storybook`, `storybook-static` and `*.config.{js,cjs}`. |
| `postcss.config.cjs` | `postcss-simple-vars` plugin only (as the sibling). |
| `setupTests.tsx` | Imports `@testing-library/jest-dom`. |
| `src/index.ts` | Exports `metaData` (4.1), a named export of the organisation component, and the component as default. |
| Source layout | The branch keeps the list, tile and filter bar as sub-components under `src/`, and the two utilities (section 9, 10) under a `utils` folder; file names are the builder's choice. |

All files carry the WSO2 Apache-2.0 header used across the repo.

Registration elsewhere: `core-ui/package.json` gains `"@agent-management-platform/<feature>": "workspace:*"` among the page dependencies; run `pnpm install` from `console/` so the lockfile records the new importer.

---

## 8. Types, fetch functions and hooks

### 8.1 `@agent-management-platform/types` additions

A new module under `console/workspaces/libs/types/src/api/` exported from `api/index.ts`. Field names and types are the OpenAPI contract; TypeScript type names are the builder's choice (mirroring the OpenAPI schema names is the repository's habit).

| OpenAPI schema | TypeScript shape |
|---|---|
| kind enum | `'platform' \| 'external'` |
| `kind` **filter** (query param, superset of kind) | `'platform' \| 'external' \| 'proxied'` |
| `A2ACatalogEnvironment` | `{ name: string; status: 'pending' \| 'fetched' \| 'failed'; fetchedAt?: string; proxied: boolean; cardDiffers: boolean; matched: boolean }` (`fetchedAt` is the only optional field; ISO date-time string) |
| `A2ACatalogAgent` | `{ projectName: string; agentName: string; kind: <kind>; name: string; description: string; version: string; skills: string[]; leadEnvironment: string; environments: <environment>[] }` |
| `A2ACatalogListResponse` | `PaginationMeta & { agents: <agent>[] }` i.e. `agents`, `total: number`, `limit: number`, `offset: number` |
| list query | `ListQuery & { q?: string; kind?: <kind filter>; environment?: string; project?: string; status?: 'pending' \| 'fetched' \| 'failed' }` |
| security mode enum | `'none' \| 'apiKey' \| 'oauth' \| 'unmanaged' \| 'unknown'` |
| `A2AConnectOAuth` | `{ issuers: { name: string; issuer: string }[]; audiences: string[]; headerName: string; authHeaderPrefix: string }` |
| `A2AConnectResponse` | `{ kind: <kind>; proxied: boolean; isProduction: boolean; gatewayUrl: string; cardUrl: string; jsonRpcUrl: string; security: { mode: <security mode>; apiKeyHeader?: string; oauth?: <oauth> }; publication: { status: 'pending' \| 'published' \| 'failed' \| 'none'; lastError: string } }` |

Doc-comment the semantics the OpenAPI gives: `cardUrl` is the gateway's copy when the gateway fronts the agent, otherwise the card's own URL, empty when neither is known; `jsonRpcUrl` is empty when there is none; `apiKeyHeader` is set when `mode` is `apiKey`; publication `none` means a direct external agent.

### 8.2 Fetch functions and hooks (two-file pattern)

New `apis/<resource>.ts` and `hooks/<resource>.ts` in the api-client, exported from both barrels.

**Fetch functions** (all GET, `SERVICE_BASE` = `/api/v1`, bearer token from `getToken` when provided):

| Operation | URL | Params / query handling | Returns |
|---|---|---|---|
| List catalogue | `GET /api/v1/orgs/{orgName}/a2a-agents` | `orgName` through `encodeRequired` (throws `Missing required parameter: orgName` when empty). Query object → search params: drop entries whose value is `undefined` or `""`, stringify the rest (`limit: 24` → `"24"`); pass **no** `searchParams` at all when nothing remains. | the list response JSON |
| Get one entry | `GET /api/v1/orgs/{orgName}/projects/{projName}/agents/{agentName}/a2a-catalog-entry` | each segment through `encodeRequired` | the entry JSON; 404 propagates as an error with `status 404` (the hook maps it) |
| Get connect details | `GET /api/v1/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envId}/a2a-connect` | built with the shared per-environment path builder (8.3), so each segment is `encodeRequired` | the connect JSON |

**Hooks** (all through `useApiQuery`; token from `useAuthHooks().getToken`):

| Hook | Query key | `enabled` | Options | Why |
|---|---|---|---|---|
| List catalogue (`params: { orgName }`, `query`) | `["a2a-agents", params, query]` | `!!orgName` | `placeholderData: keepPreviousData` (TanStack 5); `staleTime` **30 000 ms**; default retry (client policy); **not** silent | The previous page stays on screen while the next loads (no skeleton flash on paging/filtering); a back-navigation within 30 s reuses the page because each read summarises every card on the page. The key domain `a2a-agents` fixes the automatic error snackbar copy `Failed to fetch A2a Agents`. |
| Get one entry (`params: { orgName, projName, agentName }`) | `["a2a-catalog-entry", params]` | all three non-empty | `queryFn` wraps the fetch in `nullIfNotFound` so a 404 resolves to `null` (data type `entry \| null`); `staleTime` 30 000 ms; `retry: false`; `silent: true` (the page shows its own failure state) | Used by the agent page. |
| Get connect details (`params: AgentCardPathParams`) | `["a2a-connect", params]` | all four non-empty, `envId` included | client defaults (8 s stale, retry policy), not silent | Used by the agent page's connect drawer. |

No mutation invalidates these keys; they refresh by stale time and window focus.

### 8.3 Changes to existing api-client files

- `apis/agent-card.ts`: extract and **export** a path builder for an agent's per-environment resources: given `AgentCardPathParams` and a resource segment, it returns `${SERVICE_BASE}/orgs/{org}/projects/{proj}/agents/{agent}/environments/{env}/{resource}` with each of the four params passed through `encodeRequired` (labels `orgName`, `projName`, `agentName`, `envId`). The existing agent-card URL becomes that builder with `"agent-card"`.
- `apis/a2a-proxy.ts`: delete its private copy of the same URL construction and use the shared builder with `"a2a-proxy"`. Resulting URLs, encoding and thrown errors are unchanged; the existing `hooks/a2a-proxy.test.ts` must keep passing. (The `SERVICE_BASE`/`encodeRequired` imports it no longer needs are removed.)
- The new connect fetch uses the builder with `"a2a-connect"`.

### 8.4 New mutation hook in `hooks/agent-api-keys.ts` (plumbing for the agent page)

Add a hook that issues (or rotates) the caller's test API key **on demand**, for pages that issue only on request rather than keeping one fresh: `useApiMutation` with `action: { verb: "create", target: "test API key" }`, `showSuccess: false` (the key itself is shown where it was requested), `mutationFn` = the existing `issueTestAgentAPIKey(params, getToken)` taking the path params as variables. No cache invalidation. The existing polling-style test-key hook in that file is untouched.

### 8.5 Relation to the existing per-environment key convention

`hooks/a2a-query-keys.ts` keys the card and proxy reads under `["a2a", org, proj, agent, env, …]` so one invalidation refetches both. The catalogue hooks above use their own domains and are **not** covered by that invalidation: after a proxy toggle or card refetch, the catalogue list and connect details refresh only by stale time (30 s / 8 s) or window focus. This matches the branch. (Recommendation recorded in section 12: keying the connect read under the per-environment prefix so `invalidateA2AQueries` covers it is a one-line improvement the controller may choose.)

### 8.6 Hook test scenarios (api-client)

Test setup on the branch: mount the hook in a probe component under a fresh `QueryClientProvider`, mock `../utils`'s `httpGET` to resolve `{ ok: true, json: async () => ({}) }`, mock the auth hook to return a token, mock the views snackbar and the telemetry module.

- Given `orgName = "acme"` and query `{ q: "refund", limit: 24, offset: 0 }`, when the list hook mounts, then `httpGET` is called with `${SERVICE_BASE}/orgs/acme/a2a-agents` and `searchParams` exactly `{ q: "refund", limit: "24", offset: "0" }` (no undefined keys).
- Given query `{ kind: "proxied", environment: "dev", project: "p", status: "failed" }`, when the list hook mounts, then `searchParams` is exactly those four stringified values.
- Given params `{ orgName: "acme", projName: "p 1", agentName: "a/b", envId: "dev" }`, when the connect hook mounts, then the URL is `${SERVICE_BASE}/orgs/acme/projects/p%201/agents/a%2Fb/environments/dev/a2a-connect`.
- Given the same params with `envId: ""`, when the connect hook mounts, then `httpGET` is never called.
- (Implied by `nullIfNotFound`, worth a test) Given the entry fetch rejects with an error whose `status` is 404, when the entry hook settles, then `data` is `null` and no error is reported; given it rejects with status 500, then the hook is in error and no snackbar is shown (silent).

---

## 9. Environment display-name labelling

A small hook in the page workspace: given `orgName`, it calls `useListEnvironments({ orgName })` and returns a function `(environmentName) => label` where the label is the environment's `displayName` when that is a non-empty string, else its `name`; names not in the list (or any name while the list is loading / failed) map to themselves. Built each render from the current data (no memoisation required). Used by the list (passed down to tiles) and by the agent page for its environment chips. Note the filter panel's Environment options use a slightly different rule (`displayName ?? name`), kept as-is (Known limitations).

Test pinned on the branch: given environments `[{ name: "prod", displayName: "Production" }]` and tiles with environments `dev` and `prod`, when the grid renders, then `Production` is shown, the raw text `prod` is not, and `dev` appears once per tile that has it.

---

## 10. URL search-param replacement helper

A hook in the page workspace wrapping `useSearchParams` from react-router: it returns the current `URLSearchParams` and an updater taking a mutation callback. The updater composes concurrent writes on the latest params: it keeps a pending copy of the params (seeded from the URL and re-seeded whenever the URL changes by other means), applies the callback to that copy, and calls `setSearchParams` with the result and `{ replace: true }` so **no history entry** is pushed. (react-router-dom 6.30.x hands the functional `setSearchParams` form the params of the last *render*, so two writes in one tick do not compose through it; the pending-copy design is what makes same-tick writes compose.) Every list write (search, kind, filters, page, clamp) and the agent page's `env` write go through it; nothing on these pages pushes history. Composition matters for the clamp rule and the debounced search, which may write in the same tick as a user action.

---

## 10a. Data semantics the list relies on (what each field means to the console)

The console derives nothing from card JSON on the list page; every displayed value comes straight from the list response. The rules below are the backend contract (other spec) restated from the consumer's side so the clean-room console team knows what to expect, including during the upgrade window after the backend's projection migration.

| Field | Meaning for the list | Edge cases |
|---|---|---|
| `agents` order | Ordered by project name, then agent name (backend rule). The grid renders in response order; it never re-sorts. | — |
| `total` | Count of agents matching the search and filters across all pages. Drives the paginator, the clamp rule and the empty states. | `total` can be smaller than `offset` after the catalogue shrinks → clamp (4.7). |
| `name`, `description`, `version`, `skills` | From the **lead** environment's card; `name` falls back to `agentName` when no card has been fetched; the others are then empty / `[]`. | The tile shows `description` even when empty (an empty caption) and hides the skills row when `skills` is empty. `version` is not shown on the tile (the agent page shows it). |
| `leadEnvironment` | After plan Task 3: the first environment in pipeline order with a fetched card, else the first environment. | The tile's `card pending`/`card failed` caption keys off this environment's `status`. |
| `environments[].status` | `pending` = not yet fetched (first fetch, or re-queued), `fetched` = card body stored, `failed` = last fetch failed. | A `pending` row can exist for an environment whose card was fetched before but cleared (proxy toggle, source URL change). |
| `environments[].cardDiffers` | After Task 3: `true` when the environment's stored fingerprint and the lead's are both non-empty and differ. | During the upgrade window (rows refetched once after the migration) fingerprints may be empty, so the chip is **absent** rather than wrongly present. |
| `environments[].matched` | After Task 2: `true` only when `q` was given and the row matched on the agent name **or** the stored search text (card name/description, skill names/descriptions/tags/examples; case-insensitive substring, JSON syntax never matches). | An agent with a `pending` card is found by its agent name and its rows are `matched`. The tile emphasis additionally requires the console's own debounced search to be non-empty, guarding against a stale `matched` from a kept placeholder page. |
| `environments[].proxied` | The environment serves the agent through the platform gateway's A2A proxy. | Independent of `kind` (an `external` agent may be proxied; the `proxied` **filter** value selects these). |
| `environments[].fetchedAt` | Not displayed on the list. | — |
| `kind` | `platform` (built and deployed on the platform) or `external` (registered card). Shown raw on the tile's chip. | The `kind` filter accepts a third value `proxied` that is not a `kind`. |

Query-parameter limits the console must respect (OpenAPI): `q` max length 255 (the console does not truncate; longer input yields a 400 → error state, see Known limitations), `limit` 1–100 (console uses 24), `offset` ≥ 0 (guaranteed by the page parser).

Caching semantics: a list page is fresh for 30 s; within that window, revisiting the same URL (same key: org + query object) renders from cache without a request. The key includes `limit` and `offset`, so each page is cached separately. A window-focus after 30 s refetches in the background while the cached page stays visible.

## 10b. Required changes to existing upstream/main code (summary, by behaviour)

| File on upstream/main | Change |
|---|---|
| `console/workspaces/libs/types/src/routes/routes.map.ts` | Add the org child route node with relative path `a2a-agents` (index) and its child `view/:projectName/:agentName` (index). |
| `console/workspaces/libs/types/src/routes/generated-route.map.ts` | Regenerate (`pnpm run generate-route` in `libs/types`). |
| `console/workspaces/libs/types/src/api/index.ts` | Export the new catalogue/connect types module. |
| `console/workspaces/libs/api-client/src/apis/agent-card.ts` | Export a shared per-environment resource path builder; build the agent-card URL with it. |
| `console/workspaces/libs/api-client/src/apis/a2a-proxy.ts` | Use the shared builder; delete the duplicated URL construction. |
| `console/workspaces/libs/api-client/src/apis/index.ts`, `hooks/index.ts` | Export the new catalogue fetch functions and hooks. |
| `console/workspaces/libs/api-client/src/hooks/agent-api-keys.ts` | Add the on-demand test-key mutation hook (8.4). |
| `console/workspaces/libs/views/src/component/index.ts` | Export the new shared filter bar / toggle / chip / panel module. |
| `console/workspaces/pages/traces/src/subComponents/TraceFilterBar.tsx`, `TraceFiltersDrawer.tsx` | Rewire onto the shared components with no behaviour change (section 6). |
| `console/workspaces/core-ui/package.json` | Add the new page package as a `workspace:*` dependency. |
| `console/workspaces/core-ui/src/pages/index.tsx` | Add the lazy organisation-level export. |
| `console/workspaces/core-ui/src/Route/Route.tsx` | Register the wildcard org route. |
| `console/workspaces/core-ui/src/Layouts/OxygenLayout/navigationItems.tsx` | Add the org-level nav item. |
| `console/pnpm-lock.yaml` | Updated by `pnpm install` (new importer and its dependencies). |
| `console/workspaces/libs/shared-component/*` | Changes here (card parsing utility, a skill list item component, their barrel exports) belong to the agent-page analyst; the list page does not use them. |

## 10c. All user-visible strings on the list page (one table)

| Where | String |
|---|---|
| Nav item, page title, page `metaData.title` | `A2A Agents` |
| Page `metaData.description` | `Organization-wide catalogue of A2A agents` |
| Search placeholder | `Search A2A agents` |
| Filters button, panel title, panel accessible name | `Advanced Filters` |
| Panel close button accessible name | `Close advanced filters` |
| Panel footer buttons | `Reset`, `Filter` |
| Kind toggle accessible name / buttons | `Kind` / `All`, `Platform`, `External`, `Proxied` |
| Select labels | `Environment`, `Project`, `Card status` |
| Select empty option | `Any` |
| Card status options | `Fetched`, `Pending`, `Failed` |
| Chip label pattern / remove button | `<Label>: <Value>` / `Remove <Label>: <Value>` |
| Clear-all button | `Clear all` |
| Tile captions | `card pending`, `card failed`, `+<n> more` |
| Tile chips | raw `kind` (`platform` / `external`), `proxied`, `card differs` |
| Status dot hover text | raw status (`fetched` / `pending` / `failed`) |
| Empty state | `No A2A agents yet` / `Agents that expose an A2A card show up here once deployed.` |
| No-match state | `No A2A agents match your search` / `Try a different keyword or clear the filters.` |
| Error alert fallback (shell) | `Failed to load. Please try again.` |
| Error snackbar (query wrapper) | `Failed to fetch A2a Agents` |
| Agent page back link (hand-off) | `Back to A2A Agents` |

---

## 11. Known limitations (kept deliberately; plan "Not in this plan")

1. **Search box wins over outside `q` changes.** The box is seeded from `q` once on mount and the request follows the (debounced) box text, not the URL. If `q` changes by another route while the list is mounted (a programmatic or pasted change; the list itself never pushes history), the debounced writer notices the difference and writes the box text back to the URL, so the box, URL and request stay consistent at the cost of undoing the outside change. Editing the URL and reloading works because the mount re-seeds.
2. **Brief false empty state.** Because the previous response is kept as placeholder data, when a write flips "is filtering" (e.g. clearing the last filter or the search) while the previous response had `total === 0`, the "No A2A agents yet" state can flash until the new response arrives; the reverse flash ("No A2A agents match your search") can occur when the first filter is applied to a previously empty catalogue. Also, with an empty `orgName` the query is disabled and (TanStack 5 reports `isLoading === false` for a disabled query with no data) the empty state shows rather than a skeleton.
3. **Project filter cap.** The Project select lists only the backend's default first page of projects (`GET /orgs/{orgName}/projects` without `limit`; the OpenAPI default is 10, maximum 50 — the plan calls this "the 50-project filter cap"). Organisations with more projects cannot pick the rest from the panel; a `project=<name>` URL value still works and its chip shows the raw name.
4. **Blank environment labels.** The Environment select labels options with `displayName ?? name`, so an environment whose `displayName` is present but empty renders an empty option label (the tile labelling rule in section 9 does not have this gap).
5. **Unvalidated URL filter values** are forwarded to the API; a bad `kind`/`status` yields the backend's 400 and therefore the error state rather than being ignored. Likewise the search box has no `maxLength`; a search longer than the API's 255-character limit yields a 400 and the error state.
6. **List state is not restored** when returning from the agent page (back link goes to the bare list URL).
7. **Catalogue queries are not invalidated** by card/proxy mutations (section 8.5).
8. **Trigram index / pipeline-order caching** on the backend are out of scope; they do not change console behaviour.
9. The uncommitted root `CLAUDE.md` change on the branch (an "Oxygen UI" section linking `.claude/oxygen-ui/CLAUDE.md`) is unrelated repo housekeeping and not part of the feature; if adopted it should point at `console/.ai/oxygen-ui/AGENTS.md` (plan note).

---

## 12. Test scenarios (Given / When / Then)

Test harness used on the branch for the grid: render inside a `MemoryRouter` at `/?q=old`; mock the api-client module so the list hook, `useListEnvironments` (→ `{ data: [] }`) and `useListProjects` (→ `{ data: { projects: [] } }`) are controllable; mock `ResourceListShell` with a stub that renders `toolbar` and then either `emptyState.title`, `searchEmptyState.title` or `children` by its flags; use fake timers for the debounce. The default mocked response is two entries both named `orders` (projects `p1` and `p2`), the first with environments `dev` and `prod` (`prod.cardDiffers = true`), the second with `dev` (`proxied = true`), `total: 2`.

### 12.1 Grid / tiles

- Given two entries with the same `agentName` in different projects, when the grid renders, then exactly two tiles are rendered.
- Given an environment with `cardDiffers: true` and another with `proxied: true`, when the grid renders, then the texts `card differs` and `proxied` are present.
- Given environments `[{ name: "prod", displayName: "Production" }]`, when the grid renders, then `Production` is shown, `prod` is not, and `dev` appears twice (once per tile).
- Given the default entries, when the grid renders, then the first tile's link contains the text `p1` and has `href="/org/acme/a2a-agents/view/p1/orders"`, and the second has `href="/org/acme/a2a-agents/view/p2/orders"`.
- Given an entry whose `name` is empty, when its tile renders, then the title is its `agentName`.
- Given an entry with 5 skills, when its tile renders, then the first two skills are chips and a caption `+3 more` is shown whose tooltip lists all five skills comma-separated; given 2 skills, then no `+… more`; given 0 skills, then no skills row.
- Given the lead environment's status is `pending`, then the caption `card pending` is shown in the header; given `failed`, then `card failed`; given `fetched`, then neither.
- Given a non-empty debounced search and an entry with any `matched: true` environment, then that tile is rendered selected/outlined and tiles with no match are not; given an empty search, then no tile is emphasised even if `matched` is true.
- Given `total: 2`, then no paginator is rendered; given `total: 25`, then a paginator with count 25 and 24 rows per page is rendered; when its next-page control is used, then the URL gains `page=1` and the next request uses `offset: 24`.
- Given `isLoading: true` and no data, then four skeletons render in place of the grid; given `error` set, then the shell's error state renders.
- Given `total: 0` and no search/filters, then `No A2A agents yet` is shown; given `total: 0` with any filter or search, then `No A2A agents match your search` is shown.

### 12.2 Page parsing and clamp (regression #14, plan Task 11)

- Given URL `page` values `"-1"`, `"1.5"`, `"abc"` and absent, when the page is parsed, then the results are `0`, `1`, `0`, `0` respectively, and the request offsets are `0`, `24`, `0`, `0`.
- Given `?page=3` and a response `{ agents: [], total: 20 }`, when it arrives, then the URL's `page` param is removed (replace, not push) and the next request uses `offset: 0`.
- Given `?page=5` and a response `{ agents: [], total: 50 }`, then `page` becomes `2` and the next request uses `offset: 48`.
- Given `?page=2` and a response `{ agents: [], total: 0 }`, then no clamp write occurs and the "no match"/empty state shows per 4.8.
- Given `?page=0` or no page and `{ agents: [], total: 20 }` (inconsistent server data), then no clamp write occurs (page is already 0).

### 12.3 Search (including "drive search through the real SearchBar")

- Given the grid rendered with the real toolbar (the shell mock must render the `toolbar` slot and must **not** supply its own search input — commit note: an earlier mock supplied its own input so the only search box users see was never exercised), when the user types `refund` into the input with placeholder `Search A2A agents`, then the list hook has **not** been called with `q: "refund"`; when 300 ms elapse, then the last call is `({ orgName: "acme" }, { …, q: "refund", offset: 0, limit: 24 })`.
- Given the URL holds `q=old` on mount, then the search input's value is `old` and the first request carries `q: "old"`.
- Given the input text `  refund  `, when the debounce settles, then the URL has `q=refund` (trimmed) and `page` has been removed.
- Given the input cleared to empty, when the debounce settles, then `q` is removed from the URL and the request carries no `q`.
- Given `?q=old&page=2`, when the user types a new search and it settles, then `page` is removed from the URL.

### 12.4 Kind toggle and advanced filters (page-level)

- Given the grid, when the `External` toggle button is clicked, then the last request contains `kind: "external"` and `offset: 0`, and the URL has `kind=external` without `page`.
- Given `kind=external` applied, when `All` is clicked, then `kind` is removed from the URL.
- Given the grid, when `Advanced Filters` is clicked, the `Card status` combobox opened and the option `Failed` chosen, then the last request still has `status` undefined; when the panel's `Filter` button is clicked, then the last request contains `status: "failed"` and the panel is closed; when the button `Remove Card status: Failed` is clicked, then `status` is undefined again.
- Given environments `[{ name: "dev", displayName: "Development" }]` and projects `[{ name: "p1", displayName: "Project One" }]`, when the panel opens, then the `Environment` select offers `Any` and `Development` (value `dev`) and the `Project` select offers `Any` and `Project One` (value `p1`); applying both writes `environment=dev&project=p1` and shows chips `Environment: Development` and `Project: Project One`.
- Given `?environment=ghost` where `ghost` is not an organisation environment, then a chip `Environment: ghost` is shown.
- Given one advanced filter applied and no kind, then no `Clear all`; given one advanced filter and a kind, then `Clear all` is shown; when clicked, then kind and all advanced filters are removed in one URL write and the request has none of them.
- Given the panel open with a changed draft, when Escape is pressed (or `Close advanced filters` clicked), then nothing is written and reopening shows the applied values, not the discarded draft.
- Given the panel open, when `Reset` is clicked, then the three selects show `Any` but the URL is unchanged until `Filter`.
- Given the panel open, when the kind toggle is clicked, then the kind applies immediately, the panel stays open, and a subsequent `Filter` keeps the new kind.
- Given `?kind=platform&status=failed&page=1`, when the `Remove Card status: Failed` button is clicked, then the URL is `kind=platform` (page removed too).
- Given the grid, then the `Advanced Filters` button has `aria-expanded="false"` and, after clicking, `"true"`, with `aria-controls` equal to the panel's `id`.

### 12.5 Shared filter bar components (views library)

- Given the toggle with label `Kind`, options Platform/External/Proxied and `value` undefined, then a group named `Kind` renders buttons `All`, `Platform`, `External`, `Proxied` with `All` pressed; when `External` is clicked, then `onChange("external")`; given `value="external"`, when `All` is clicked, then `onChange(undefined)`; when the pressed button is clicked again, then `onChange` is not called.
- Given the chip with `name="Card status"`, `value="Failed"`, `label="Card status: Failed"`, then a button titled `Card status: Failed` showing `Card status` and `Failed`, and a button named `Remove Card status: Failed`; clicking the first calls `onOpen`, the second `onRemove`; given no `label`, then the title is `Card status Failed`.
- Given the panel with `title="Advanced Filters"`, `id="x"`, then an `aside` (role complementary) named `Advanced Filters` with `id="x"` is focused on mount, has an `h2` reading `Advanced Filters`, a button named `Close advanced filters` that calls `onClose`, a button `Reset` calling `onReset`, and a button `Filter` calling `onApply`; given `applyDisabled`, then `Filter` is disabled; when Escape is pressed on the document, then `onClose` is called.
- Given the bar with `actions` and `children`, then the actions render before the children in the DOM, floated right, and the children render inline on the left (layout test optional).

### 12.6 Registration

- Given the generated route map, then it contains the two paths in 4.1 with the correct wild paths.
- Given an organisation route and the real root router, when `/org/acme/a2a-agents` is visited, then the list page renders; when `/org/acme/a2a-agents/anything-else` is visited, then the URL is replaced by `/org/acme/a2a-agents`.
- Given the organisation navigation, then an item labelled `A2A Agents` with the `Network` icon appears after `Agent Catalog`, links to `/org/acme/a2a-agents`, and is active both on the list and on `/org/acme/a2a-agents/view/p1/orders`.

---

## 13. Ambiguities resolved (for the controller to overrule)

1. **Package/folder name.** The clean-room rules forbid branch-created file paths, but the plan's test command names `@agent-management-platform/a2a-agents` and the route segment is `a2a-agents`. I specified the scope as binding and recommended the segment-derived name as the conventional choice.
2. **Query-key domain strings.** Strictly internal, but the first segment determines the user-visible error snackbar (`Failed to fetch A2a Agents`). I specified `a2a-agents` for the list key for copy parity and left the other two domains as the branch has them (`a2a-catalog-entry`, `a2a-connect`) since they are silent / rarely error-visible.
3. **Catalogue keys vs the per-environment invalidation helper.** The branch does not key the connect read under the existing `["a2a", …]` prefix; I kept the branch behaviour (not covered by invalidation) and noted the improvement.
4. **"Side panel/drawer on the list".** Earlier branch commits had a list + side panel; the final branch has none on the list (the drawer is on the agent page). I specified "no side panel on the list".
5. **`card pending` / `card failed` caption after Task 3.** The branch derives it from the lead environment's status; with Task 3's lead rule this means "no environment has a fetched card". I specified the derivation (lead status) rather than the consequence, so it stays correct if the backend changes.
6. **Project filter cap.** The plan says "50-project filter cap"; the OpenAPI default page size for projects is 10 (max 50). I recorded the discrepancy and specified the behaviour as "the backend's default first page".
7. **Environment label rule mismatch** (`||` in the labelling utility vs `??` in the filter options). The plan lists "blank environment labels" as deliberately left out, so I kept both rules as they are and documented the gap.
8. **Error-state coexistence with placeholder data.** The branch passes the hook's `error` to the shell, which takes precedence over children. With `keepPreviousData`, an error on a later page/filter fetch therefore replaces the stale grid with the error alert (the plan's "error replaces content only when there is no data" rule in Task 10 is scoped to the agent page). I kept the list's branch behaviour.
9. **`disableIcon` on `PageLayout`** is deprecated upstream in favour of `avatar={null}`; I specified the behaviour ("no header icon") and noted both are equivalent.
10. **Route order in `Route.tsx` and nav order.** Specified exactly as on the branch (nav: after Agent Catalog, before Settings; route: between llm-providers and mcp-proxies), while noting route order is not behaviourally significant.
11. **Test ids** on the tile and the status dot are branch identifiers; I required "a stable test id" without naming them.

## 14. Could not determine from the sources

- The exact text of the error thrown by `httpGET` on a non-2xx (and therefore the exact message shown in the shell's error alert for a 400 from an invalid URL filter value); the shell's fallback copy is given.
- Whether the analytics `ConsoleAction.Search` scope string `a2a-agents` is consumed by any dashboard that would require this exact value; I kept it as the branch has it.
- No dedicated unit tests exist on the branch for the shared views components, the environment-label hook or the URL-replacement hook (they are covered indirectly through the grid tests and `Traces.test.tsx`); the scenarios in 12.5 are derived from their observable behaviour.


---

# PART C — CONSOLE: AGENT PAGE

# Console specification — the A2A agent page (opened from the catalogue)

Clean-room specification. Target behaviour = the `a2a-agent-catalogue` branch WITH Tasks 1–11 of the structural-fixes plan applied. Where this document says "after the fix", the branch does something else today and the plan's behaviour is the one to build.

Companion specs: the catalogue LIST page (grid, tiles, search, filters, pagination, FilterBar, route and nav registration, the list hook and the shared console types for the catalogue entry) is specified by the catalogue-list analyst; the backend contract (list, catalogue entry, connect endpoint, fingerprint, lead environment) by the backend analyst. This document only references the route this page lives at, the list-side link into it, and the JSON fields it reads.

---

## 1. Purpose and user stories

The catalogue lists every A2A agent in an organization. Clicking a tile opens that agent on its own page, where a developer can:

- **See one environment's agent card in detail** — skills, capabilities, status of the card fetch — and switch between the environments the agent is deployed to.
- **Compare the card across environments** — "is prod running the same card as dev?" — on exactly the same terms as the catalogue's `cardDiffers` flag, so the tile and the page never contradict each other.
- **Connect to the agent** from a drawer: the endpoints to call, what security the gateway enforces, a personal short-lived test key for non-production environments, and ready-to-run quick-start snippets (Python, TypeScript, curl) that already carry the right URLs and auth header.

Who: any console user who can read the organization's agents (same permission as the catalogue list and the per-environment agent-card endpoint). Issuing a test key requires whatever the existing test-key endpoint requires.

---

## 2. Scope of this document

In scope:

1. The agent page route under the catalogue, its header, environment chips, publication warnings, back navigation, keyboard behaviour.
2. The card-details area with its explicit card-state model and the Compare feature.
3. Additions to the shared-component library: A2A card utilities (parse, canonicalise, diff) and a skill item component; the required changes to `A2AAgentCardPanel` (configure-agent) and `EnvAgentCardSummary` (overview) to use them.
4. The Connect drawer: endpoints, security summary, "Manage security" link, test-key issuer, quick-start snippets, copy-to-clipboard, keep-content-on-refetch-error.
5. The api-client/hook additions this page needs that the list spec does not cover: the connect-details hook, the per-environment agent-card API refactor, the test-key issue hook.
6. Test scenarios (Given/When/Then), including every console-side regression from the review findings (#1, #2, #5, #6, #7, #8, #9, #10, #12) and the manual TypeScript-snippet check.

Out of scope: everything on the list page, backend behaviour, PR splitting.

---

## 3. Binding conventions (cite when building)

From `console/AGENTS.md` (binding):

| Rule | What it means for this feature |
|---|---|
| Two-file API pattern is mandatory: a pure fetch function in `libs/api-client/src/apis/<resource>.ts` and a hook in `libs/api-client/src/hooks/<resource>.ts`, both exported from the barrels (`apis/index.ts`, `hooks/index.ts`). Never call `fetch`/TanStack directly from a component. | The connect-details fetch + hook, the test-key mutation hook, the catalogue-entry fetch + hook (shared with the list spec) all follow this. |
| HTTP helpers `httpGET/POST/PUT/PATCH/DELETE` from `../utils`; `SERVICE_BASE = '/api/v1'`; path segments encoded with `encodeRequired`. | All new fetch functions. |
| Hooks wrap `useApiQuery` / `useApiMutation` from `hooks/react-query-notifications.ts`, never raw `useQuery`/`useMutation`. `queryKey` is a tuple `[domain, params, …]`; invalidation is prefix-based. | Query keys in §11. |
| Auth token always from `useAuthHooks().getToken`, passed into the fetch function. | All hooks. |
| UI imports from `@wso2/oxygen-ui` (+ icons from `@wso2/oxygen-ui-icons-react`), never `@mui/material`. Theme tokens in `sx` (`color: "text.secondary"`, `p: 2`), no hard-coded colours/px. | Every component here. |
| No floating labels on `TextField`: use a static label (`FormControl` + `FormLabel`, which the views `TextInput` already does) or a `Typography` caption. | Endpoint rows and the test-key field use the views `TextInput` with its `label` prop. See ambiguity A-7 for the Compare select. |
| Forms are manual (`useState` + inline validation). Server state = TanStack Query, UI state = `useState`. | Compare selection, drawer open state, snippet language, issued key are `useState`. |
| Routing: `relativeRouteMap.…path` for `<Route path>`, `absoluteRouteMap.…path` + `generatePath` for links; never hard-coded route strings. | Back link, tile link, Manage-security link. |
| Lint = ESLint flat config per package; tests = Vitest (jsdom, `globals: true`). Done checklist: `pnpm run lint` and `pnpm run build` clean in every touched package; new page exports `metaData` and is routed. | — |

From `console/.ai/oxygen-ui/AGENTS.md` (binding): import components from `@wso2/oxygen-ui`, icons from `@wso2/oxygen-ui-icons-react`, MUI-X via namespaces, always theme tokens. Oxygen provides `CodeBlock`-style display components; the repo's own `CodeBlock` (shared-component) is the one used here because it carries copy analytics.

---

## 4. Dependencies that already exist on upstream/main

| Upstream identifier (file) | What this page needs from it |
|---|---|
| `useGetAgentCard(params, { enabled?, poll? })` — `console/workspaces/libs/api-client/src/hooks/agent-card.ts` | Per-environment card. Returns `data: AgentCardResponse \| null \| undefined` (null on 404 via `nullIfNotFound`), `retry: false`, `silent: true` (no snackbar), polls while `status === "pending"` unless `poll: false`. Query key from `agentCardQueryKey(params)` = `["a2a", org, proj, agent, env, "card"]`. **Unchanged by this feature.** |
| `getAgentCard` and the path builder in `console/workspaces/libs/api-client/src/apis/agent-card.ts` | The private per-environment path builder becomes an exported helper that takes the four path params and a trailing resource name and returns `/api/v1/orgs/{org}/projects/{proj}/agents/{agent}/environments/{env}/{resource}` with every segment `encodeRequired`-encoded. The card fetch uses it with `agent-card`; the connect fetch with `a2a-connect`; `apis/a2a-proxy.ts` is refactored to use it with `a2a-proxy` (behaviour unchanged). |
| `issueTestAgentAPIKey(params, getToken)` — `apis/agent-api-keys.ts` | Already POSTs `/api/v1/orgs/{org}/projects/{proj}/agents/{agent}/environments/{env}/api-keys/test` with an empty body and returns `IssueTestAgentAPIKeyResponse` (`status`, `message?`, `keyId?`, `apiKey?`, `expiresAt`, `gatewayConnected?`). **Nothing to add to the fetch layer**; only a mutation hook is added (§11.3). |
| `useTestAgentAPIKey` — `hooks/agent-api-keys.ts` | The Try-It flow's query-based issuer (issues on mount, refreshes every 9 min). **Not used here**: this page must issue only on click, so a sibling mutation hook is added next to it. |
| `useApiQuery` / `useApiMutation` — `hooks/react-query-notifications.ts` | Snackbar behaviour: a query with `silent: true` never toasts; a non-silent query toasts `Failed to fetch <Title-cased first key segment>` on error (404 suppressed). A mutation with `action: { verb, target }` toasts `<Target> created successfully` unless `showSuccess: false`, and on error toasts the server message or `Failed to submit <target>`. |
| App query defaults — `libs/api-client/src/ClientProvider.tsx` | `staleTime` 8 000 ms, `refetchOnWindowFocus: true`, retry up to 3 times except on 4xx other than 408. |
| `useListEnvironments({ orgName })` — `hooks/deployments.ts` | Environment list (`Environment.name`, optional `displayName`, `isProduction`) for chip labels. |
| `nullIfNotFound`, `encodeRequired`, `httpGET`, `SERVICE_BASE` — `libs/api-client/src/utils` | As described in conventions. |
| `AgentCardResponse`, `AgentCardPathParams`, `AgentCardStatus` — `libs/types/src/api/agent-card.ts` | `{ source: 'platform'\|'external', status: 'pending'\|'fetched'\|'failed', sourceUrl, lastError, card?: Record<string,unknown>, fetchedAt? }`; path params `{ orgName, projName, agentName, envId }`. |
| `PageLayout` — `libs/views/src/component/PageLayout` | Props used: `title`, `backHref`, `backLabel`, `isLoading` (renders header skeletons and hides children), `titleTail`, `description`. |
| `DrawerWrapper`, `DrawerHeader`, `DrawerContent` — `libs/views/src/component/Drawer` | Right-anchored temporary drawer (`open`, `onClose`, `keepMounted`, `maxWidth`); header with `icon`, `title`, `onClose` (renders a close icon button). |
| `TextInput` — `libs/views/src/component/FormElements/TextInput.tsx` | Static-label text field; `copyable` adds a copy icon button whose tooltip is `copyTooltipText` (default `Copy`) and flips to `Copied!` for 2 s after a successful clipboard write. |
| `buildConfigureTabHref(orgId, projectId, agentId, "a2a", envName?)` — `libs/views/src/utils/configureTabLink.ts` | Yields `/org/:orgId/project/:projectId/agents/:agentId/configure?tab=a2a&env=<env>`. Used for the external agent's Manage-security link. |
| `CodeBlock` — `libs/shared-component/src/components/CodeBlock.tsx` | Syntax-highlighted block with a copy button; props `code`, `language`, `fieldId` (constant per block), optional `analyticsId`. Copy writes `code` to the clipboard, shows "Copied!" for 2 s and tracks `CopySnippet` with `snippet_type = analyticsId ?? fieldId` and `language`. |
| `normalizeVersion`, `formatRelativeTime` — `libs/shared-component/src/utils/format.ts` | `normalizeVersion("1.0")` → `v1.0` (leaves an existing `v`/`V` prefix); `formatRelativeTime(iso)` → date-fns "2 minutes ago" / "in 10 minutes", `-` for an invalid or missing value. |
| `monospaceInputSx` — `libs/shared-component/src/components/AgentIdentityCredentials/AgentIdentityCredentials.tsx` | `sx` that makes a text input monospace. |
| `validateEndpointUrl(value)` — `libs/shared-component/src/utils/url.ts` | Returns `null` only for a parsable http/https URL; used to decide whether a provider URL is safe to link. |
| `absoluteRouteMap` / `relativeRouteMap` — `libs/types/src/routes` | Existing agent routes: `…agents.children.deployment.path` = `/org/:orgId/project/:projectId/agents/:agentId/deployment` (registered in `core-ui/src/Route/Route.tsx`); `…agents.children.environment.children.deploy.path` = `/org/:orgId/project/:projectId/agents/:agentId/environment/:envId/deploy` (**not registered** — a dead link). |
| `A2AAgentCardPanel` — `pages/configure-agent/src/Configure/subComponents/A2AAgentCardPanel.tsx`; `EnvAgentCardSummary` — `pages/overview/src/AgentOverview/EnvAgentCardSummary.tsx` | Both currently hold private, duplicated "read the untrusted card JSON defensively" helpers. They switch to the shared utilities (§8.4). |
| `Security.Component.tsx` — `pages/agent-security/src` (package `@agent-management-platform/agent-security`) | Carries the pre-existing dead `deploy` link (the "Go to Deployment Settings" button); fixed identically (§9.3). |
| Oxygen UI components | `Alert`, `Box`, `Button`, `Chip`, `CircularProgress`, `FormControl`, `InputLabel`, `MenuItem`, `Select`, `Stack`, `ToggleButton`, `ToggleButtonGroup`, `Typography`; icon `Plug` (connect button and drawer header). |

---

## 5. Route and page shell

### 5.1 Route

| Item | Value |
|---|---|
| Absolute path | `/org/:orgId/a2a-agents/view/:projectName/:agentName` |
| Registered as | a child `view/:projectName/:agentName` of the catalogue route `a2a-agents` in the route map (both `routes.map.ts` and the generated map) — registration itself is in the list spec. Inside the catalogue page package, the organization-level component renders nested `<Routes>`: index = the list, `view/:projectName/:agentName` = this page, `*` = redirect (replace) to the list. |
| Path params | `orgId` (organization handle), `projectName`, `agentName` — all used verbatim as the API path params (`orgName`, `projName`, `agentName`). |
| Query string | `env=<environment name>` — the selected environment. Absent ⇒ the entry's `leadEnvironment`. Changing the environment **replaces** the current history entry (no new back-stack entry) and keeps every other query param. |
| Entry from the list | A tile links to the absolute path with no query string, so the page opens on the lead environment. |
| Back navigation | `PageLayout` back link labelled **`Back to A2A Agents`** → `/org/:orgId/a2a-agents` (generated from the route map). Browser back from the page returns to the list with whatever list query string it had (the page never pushes history entries). |

### 5.2 Resolving the agent ("resolve unseen selected agents from the catalogue")

The page never assumes anything from the list's cached page: a deep link to an agent that is not on the current list page, or to a bookmarked URL, must still show the real kind and environments. It therefore always reads the single-agent catalogue entry endpoint:

`GET /api/v1/orgs/{orgName}/projects/{projName}/agents/{agentName}/a2a-catalog-entry` → the same JSON object the list returns per agent (`projectName`, `agentName`, `kind: platform|external`, `name`, `description`, `version`, `skills: string[]`, `leadEnvironment`, `environments: [{ name, status: pending|fetched|failed, fetchedAt?, proxied, cardDiffers, matched }]`); 404 when the agent is not in the catalogue.

Page states driven by that query (hook rules in §11.1):

| State | What renders |
|---|---|
| Loading, no data | `PageLayout` with `isLoading` (title skeleton; title text falls back to the `agentName` path param); no chips, no kind/version, no body. Must not show "platform"/"external" or any card claim yet. |
| 404 (data resolved to `null`) | Body text (secondary colour): **`This agent was not found in the catalogue.`** Header shows the `agentName` path param. |
| Error and **no** cached data | `Alert` severity error: **`Failed to load the agent. Please try again.`** Nothing else in the body. |
| Error **with** cached data (a failed background refetch) — after the fix (#10) | The full page stays exactly as it was (details, chips, drawer, compare selection, issued key) and an inline `Alert` severity warning is shown above the details: **`Couldn't refresh; showing the last loaded details.`** |
| Data | The details (§5.3–5.6, §6–7). |

The card query for the selected environment is enabled only once the entry has loaded (the environment name may come from the entry).

### 5.3 Header

| Element | Rule |
|---|---|
| Title | After the fix (#9's "header mixes environments" note): title and version are taken **together** from one source. When the selected environment's card is in the `ready` state (§6), title = the card body's `name`, version = the card body's `version`. Otherwise title = the entry's `name`, version = the entry's `version`. (If a ready card has an empty `name`, fall back to the entry's `name` for the title only — ambiguity A-2.) While the entry is loading, title = the `agentName` path param. |
| Version chip | Small outlined `Chip` labelled `normalizeVersion(version)` (e.g. `v1.0`); omitted when the version is empty. Rendered only once the entry has loaded. |
| Kind chip | Small filled `Chip` labelled with the card response's `source` when the card response is loaded, else the entry's `kind` — i.e. the literal text `platform` or `external`. Rendered only once the entry has loaded. |
| Description line | Secondary `body2` text: `{projectName}` followed by ` · {provider organization}` when the selected card's parsed provider organization is non-empty (e.g. `p1 · Acme`). |
| Back link | See §5.1. |

### 5.4 Environment selector chips

- One `Chip` per entry in the entry's `environments` array, **in the order given** (pipeline order, lead first).
- Label = the environment's display name from `useListEnvironments({ orgName })` (`displayName`), falling back to the environment `name` when no display name exists or the environment list has not loaded / does not contain it.
- The selected environment's chip is `color="primary"`, `variant="filled"`; the others `variant="outlined"`, default colour.
- Clicking a chip sets `env` in the query string (replace) and switches the whole details area to that environment. The details area is **remounted** on environment change (keyed by environment), which resets: the compare selection to none, the Connect drawer to closed-and-unmounted, and any issued test key (per-environment reset, §9.7).
- The chips and the **Connect** button sit on one row: chips left (wrapping), button right.

### 5.5 Connect button and drawer

- Button: `variant="contained"`, `size="small"`, start icon `Plug`, label **`Connect`**. Always rendered once the entry is loaded.
- Click → opens a right-side `DrawerWrapper` (`maxWidth` 700) with `DrawerHeader` icon `Plug`, title **`Connect · {environment label}`** (e.g. `Connect · Development`), and a `DrawerContent` holding the connect section (§9).
- The drawer's content is **not mounted until the first open** (no connect request, no key issuing before the user asks). Once opened, the drawer stays mounted (`keepMounted`) after closing, so an issued test key, the chosen snippet language and the loaded connect details survive close/re-open within the same environment.
- Closing: the header's close icon button, the backdrop, or Esc (MUI default) — all call the same close handler.

### 5.6 Keyboard behaviour ("Esc stays local")

Esc must only ever act on the innermost open surface and never navigate away from the page:

- Esc while the Compare dropdown (§7.4) is open closes the dropdown only.
- Esc while the Connect drawer is open closes the drawer (content stays mounted, §5.5).
- Esc with focus in any input (the copyable URL fields, the test-key field) does nothing beyond the input's own default.
- Esc never triggers the back navigation to the catalogue. (The page registers no global key handler; see ambiguity A-3.)

### 5.7 The keep-content-on-refetch-error rule (page-wide)

An error replaces content **only when there is no data**. With data and an error, the content stays and an inline warning `Alert` reads **`Couldn't refresh; showing the last loaded details.`** This applies to the entry query (§5.2) and the connect query (§9.2). A failed refetch therefore never unmounts the drawer or the test-key issuer and never resets the snippet back to the placeholder.

---

## 6. Card state model (explicit; closes #8 and #9)

A small pure function in the page package (unit-tested on its own) maps a per-environment card query result `{ data: AgentCardResponse | null | undefined, isLoading, isError }` to exactly one of five states, checked **in this order**:

| Order | Condition | State | Carries |
|---|---|---|---|
| 1 | `data` present and `data.card` is a non-null object | **ready** | the response and the card body |
| 2 | `data` present and `data.card` absent | **noBody** (a pending or failed row with no stored card) | the response |
| 3 | `data === null` | **absent** (404: no card configured for this environment) | — |
| 4 | `isError` | **error** | — |
| 5 | otherwise | **loading** | — |

Invariants: a cached `ready` card stays `ready` through a later refetch error (order 1 beats order 4); a `null` from a 404 is `absent`, not `error`; `isLoading` with no data is `loading`.

The same mapping is applied to the compare environment's query (§7.4).

---

## 7. Card details area

Rendered below the chips row. Everything in this section is the selected environment's card.

### 7.1 Per-state rendering

| State | Renders (exact copy) |
|---|---|
| loading | Secondary `body2`: **`Loading card…`** (with the `…` character). Must not claim the card is missing. |
| absent | Secondary `body2`: **`No agent card is configured for this environment.`** |
| error (after the fix, #9) | **`Couldn't load the agent card.`** — must not render the absent copy. (The branch today renders the absent copy for any non-404 failure; that is the regression.) |
| noBody (after the fix, #8) | **Only** the status block (§7.2). No description, no `Skills` heading, no `No skills declared.`, no capabilities, no compare selector, no raw-JSON toggle. |
| ready | Status block → description → Skills → Capabilities → Compare (if other environments exist) → raw JSON toggle. |

### 7.2 Status block

| `status` | Rendering |
|---|---|
| `fetched` | Caption, secondary colour: **`Fetched {formatRelativeTime(fetchedAt)}`** (e.g. `Fetched 2 minutes ago`). |
| `pending` | `body2` in `warning.main`: **`Card pending`**; below it, when `lastError` is non-empty, a caption in `error.main` with the raw `lastError` text. |
| `failed` | `body2` in `warning.main`: **`Card fetch failed`**; below it the `lastError` caption as above. |

Not shown anywhere on this page: attempt counts, next-attempt times, the `sourceUrl` of the card source (the Connect drawer shows the gateway/card URLs instead). See Known limitations.

### 7.3 Description, skills, capabilities (ready only)

- **Description**: the parsed card `description` as `body2`, omitted when empty.
- **Skills** heading (`subtitle2`, secondary): then either secondary `body2` **`No skills declared.`** when the parsed skills array is empty, or a vertical list (spacing 1) of the shared skill item component (§8.3), one per parsed skill, keyed by `id` falling back to `name`.
- **Capabilities** heading: three `body2` lines in this fixed order, each `"{✓ or ✗} {key}"` with a single space: `✓ streaming` / `✗ streaming`, `✓ pushNotifications` / `✗ pushNotifications`, `✓ extendedAgentCard` / `✗ extendedAgentCard`. `✓` only when the card's `capabilities.<key>` is boolean `true`; any other value (missing, `"true"`, 1, null) renders `✗`. (U+2713 and U+2717.)

Provider organization is shown in the page header (§5.3), not here. Interfaces/transports are shown in the Connect drawer (§9.5), not here. The provider's website link and the documentation URL are **not** shown on this page (Known limitations).

### 7.4 Compare feature

- Rendered only in the **ready** state and only when the entry has at least one environment other than the selected one.
- A small `Select` labelled **`Compare with`** (max width 240): first option **`None`** (value empty string), then one option per other environment in entry order, labelled with the environment display label (§5.4), value = environment name. Default: `None`.
- Selecting an environment starts a card query for that environment using the existing `useGetAgentCard` with `poll: false` and `enabled` only while a compare environment is selected. (No polling for the comparison target.)
- Below the select, when a compare environment is selected, a result block driven by the **compare** card state (§6 applied to the compare query) and the selected card state:

| Compare state | Renders |
|---|---|
| loading | **`Loading…`** |
| error | **`Couldn't load the card to compare.`** |
| absent or noBody | **`No fetched card to compare.`** (must not list every skill of the other card as Added — #8) |
| ready (and the selected card is ready, which it is by construction) | the diff lines below |

Diff output (direction: base = **selected** environment's card body, other = **compare** environment's card body; "Added" means present in the compare card but not in the selected one):

1. One `body2` line **`Added: {skill name}`** per skill in `skills.added`, in order.
2. One line **`Removed: {skill name}`** per `skills.removed`.
3. One line **`Changed: {skill name}`** per `skills.changed`.
4. If `fields` is non-empty, one line **`Also differs in: {fields joined by ", "}`** (fields sorted ascending, e.g. `Also differs in: capabilities, name` or `Also differs in: skills`).
5. If all four lists are empty: a secondary caption **`The cards are identical.`**

The diff is computed by the shared utility of §8.2, so "identical" here ⇔ the backend fingerprint of the two bodies is equal.

### 7.5 Raw JSON

Ready state only: a small text `Button` toggling between **`View raw JSON`** and **`Hide raw JSON`**; when shown, a `CodeBlock` with the card body pretty-printed as JSON (2-space indentation), `language="json"`, with a constant field id for its copy button.

---

## 8. Shared-component library additions (`console/workspaces/libs/shared-component`)

Exported from the library's barrel (`src/index.ts` for the utilities via a new entry under `src/utils/`, `src/components/index.ts` for the component).

### 8.1 Parsed card shape and the parse rules

A pure function reads an **untrusted** card value (anything: object, string, undefined) and returns a fully populated record — a missing or malformed value reads as `""`, `[]` or `false`, never throws:

| Field | Rule |
|---|---|
| `name`, `description`, `version` | the card's top-level string of that name, else `""`. |
| `providerOrganization` | `provider.organization` when `provider` is an object and the value is a string, else `""`. |
| `providerUrl` | `provider.url` only when it is a string **and** `validateEndpointUrl` accepts it (http/https, parsable); otherwise `""`. `javascript:` or malformed URLs are dropped. |
| `interfaces` | from `supportedInterfaces` (array; non-object items ignored): `{ url, protocolBinding }` with each read as string-or-`""`; entries whose `url` is `""` are **dropped** (an interface without a URL is unreachable). A non-string `protocolBinding` reads as `""`. |
| `skills` | from `skills` (array; non-object items ignored): each `{ id, name, description, tags, examples, inputModes, outputModes }` — strings as string-or-`""`, the four lists as arrays keeping only string items (else `[]`). |
| `capabilities` | `{ streaming, pushNotifications, extendedAgentCard }`, each `true` only when the card's `capabilities.<key> === true`. |
| `securitySchemes` | the **keys** of the card's `securitySchemes` object, else `[]`. |

Non-object input (undefined, null, string, array) yields the all-empty record.

### 8.2 Canonicalisation and the diff rules (after Task 8; closes #5)

Goal: the page's Compare view and the catalogue's `cardDiffers` flag must agree. The backend fingerprint is the hash of the card with object keys sorted at every depth and with the `url` removed from every `supportedInterfaces` entry; everything else (skill order, duplicates, every skill field, every top-level field) counts.

Definitions:

- **canonical(v)**: arrays map element-wise; objects are rebuilt with keys sorted ascending and values canonicalised; scalars unchanged. Two values are "canonically equal" when their JSON serialisations after canonicalisation are identical.
- **without interface URLs(card)**: a copy of the card object whose `supportedInterfaces`, when an array, has the `url` key removed from each object item (non-object items untouched); when not an array, left as is.

`diff(a, b)` — inputs are the two raw card bodies (non-objects are treated as `{}`) — returns `{ fields: string[], skills: { added: string[], removed: string[], changed: string[] } }`:

1. Let `A = without interface URLs(a)`, `B = without interface URLs(b)`.
2. **fields** = every top-level key present in `A` or `B`, sorted ascending, **other than** `skills`, whose values are not canonically equal (a key missing on one side counts as differing from any present value).
3. **skills**: index the raw skill objects of `A` and of `B` by their `name` (string, as parsed; the last duplicate wins the index).
   - `added` = names in `B`'s index not in `A`'s (in `B` order of first appearance);
   - `removed` = names in `A`'s index not in `B`'s;
   - `changed` = names in both whose **whole raw skill objects** are not canonically equal (so a change to `id`, `inputModes`, `outputModes`, `security`, `tags`, `examples`, `description` or any other key marks the skill changed — not only description/tags/examples as the branch does today).
4. **the `skills` field fallback**: if `added`, `removed` and `changed` are all empty but the raw `skills` values of `A` and `B` are not canonically equal (reordering, duplicate names, a non-array on one side), append `"skills"` to `fields` (keeping `fields` sorted).

Guarantee (must hold and be tested): `fields`, `added`, `removed`, `changed` are **all empty if and only if** `canonical(A)` equals `canonical(B)` — i.e. iff the backend fingerprints are equal.

Order of returned names in `added`/`removed`/`changed` follows the index insertion order; the page renders them in that order.

### 8.3 Skill item component

A presentational component taking one parsed skill (§8.1 shape) and rendering a bordered box (`p: 1.5`, `border: 1`, `borderColor: "divider"`, `borderRadius: 1`, vertical spacing 0.5):

1. `subtitle2` with the skill `name`.
2. `body2` secondary with the `description` — omitted when empty.
3. A wrapping row of small outlined `Chip`s, one per `tags` entry — the whole row omitted when there are no tags.
4. A wrapping row with caption **`Input`** followed by one small outlined `Chip` per `inputModes` entry — omitted when empty.
5. The same with caption **`Output`** for `outputModes`.
6. One secondary caption per `examples` entry, text **`e.g. {example}`**.

Chips are keyed by index + value so duplicate tag strings do not collide.

### 8.4 Required changes to existing upstream components (behaviour must stay identical)

**`A2AAgentCardPanel` (configure-agent)** — replace its private JSON helpers with the shared parser and the shared skill item. Must keep:

- Header line: `h6` with the card name; a secondary caption with the normalised version (`normalizeVersion`, so `1.2` → `v1.2`, `v1.2` stays) only when the version is non-empty; the description below when non-empty.
- **Interfaces** overline, then per parsed interface a row with a small `Chip` of the `protocolBinding` (when non-empty, min width 96) and a read-only monospace copyable `TextInput` of the URL (tooltip `Copy URL`). Interfaces without a URL are not listed (same as the parser's rule).
- **Skills** overline, then the shared skill item per skill (spacing 1.5). This is a deliberate, accepted visual change on that panel: the item now also shows input/output mode chips and examples where it previously showed only name, description and tags.
- Everything else on the panel (status chips, fetch-failed alert, refetch button, source URL editing, proxy section, JSON toggle `View JSON`/`Hide JSON`, Remove) unchanged.

**`EnvAgentCardSummary` (overview)** — replace its private `summarizeCard` with the shared parser. Must keep:

- Empty-card message **`The agent card lists no provider, interfaces or skills.`** when organization is empty, no interfaces with URL, and no skills with a non-empty name.
- **Provider** overline with the organization and, only when the provider URL passed the http/https check, an icon button (`ExternalLink`, `aria-label="Open provider website"`, `target="_blank"`, `rel="noopener noreferrer"`).
- **Interfaces** block identical to the panel's rows above.
- **Skills** block: small outlined chips with each skill's **name** only (skills with an empty name are skipped).
- Loading spinner / `Register agent card URL` chip for an external agent with no source / `Unable to load the agent card. Try again later.` on error / `Agent card not available yet.` when there is no body — unchanged.

---

## 9. The Connect experience (drawer content)

### 9.1 Data

`GET /api/v1/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/a2a-connect` (hook in §11.2). Response fields read by the console:

| Field | Type | Meaning (from the OpenAPI description) |
|---|---|---|
| `kind` | `platform` \| `external` | |
| `proxied` | boolean | external agent routed through the gateway |
| `isProduction` | boolean | the environment is a production environment |
| `gatewayUrl` | string | where the gateway serves the agent; empty for a direct external agent or while no gateway is mapped |
| `cardUrl` | string | the gateway's card copy when it fronts the agent, else the card's own URL; empty when neither is known |
| `jsonRpcUrl` | string | the JSON-RPC endpoint clients should call; **empty when there is none** (after backend Task 4 this is authoritative — the console must not guess) |
| `security.mode` | `none` \| `apiKey` \| `oauth` \| `unmanaged` \| `unknown` | `unmanaged` = direct external agent; `unknown` = the gateway fronts the agent but its security config could not be determined |
| `security.apiKeyHeader` | string? | header name when mode is `apiKey` (e.g. `X-API-Key`) |
| `security.oauth` | `{ issuers: [{ name, issuer }], audiences: string[], headerName, authHeaderPrefix }`? | when mode is `oauth`; `issuer` is empty when the name no longer resolves |
| `publication.status` | `pending` \| `published` \| `failed` \| `none` | `none` = direct external agent |
| `publication.lastError` | string | |

The drawer also receives the selected environment's card response (for the card's declared interfaces and security scheme names) and the catalogue entry (for `kind`, `projectName`, `agentName`).

### 9.2 Layout and states

Top row, right-aligned: a small text `Button` **`Manage security`** (always rendered, link semantics — §9.3). Below it, the body:

| State | Body |
|---|---|
| loading (no data) | a 16 px `CircularProgress` |
| error and no data | `body2` in error colour: **`Couldn't load connection details for this environment.`** |
| error with data (after the fix, #10) | the loaded body stays; a warning `Alert` **`Couldn't refresh; showing the last loaded details.`** is shown above it |
| data | publication warning (§9.4) → **Endpoint** section → **Security** section → **Quick start** section (or its replacement message) |

Section titles are `subtitle2` text: `Endpoint`, `Security`, `Quick start`.

### 9.3 "Manage security" link target (after Task 11; closes #2)

| `kind` of the entry | Target |
|---|---|
| `external` | `buildConfigureTabHref(orgId, projectName, agentName, "a2a", env)` → `/org/{orgId}/project/{projectName}/agents/{agentName}/configure?tab=a2a&env={env}` |
| `platform` | the agent **deployment** route: `/org/{orgId}/project/{projectName}/agents/{agentName}/deployment` (generated from `absoluteRouteMap…agents.children.deployment.path`). **Not** the per-environment `environment/:envId/deploy` path, which no `<Route>` registers and lands on the Not Found page. |

**Identical fix on upstream/main**: `console/workspaces/pages/agent-security/src/Security.Component.tsx` renders a `Go to Deployment Settings` button (shown when a deployed, gateway-served, non-external agent has security disabled) whose `to` is generated from the same dead `environment.children.deploy` path. Change it to the `deployment` path above (params `orgId`, `projectId`, `agentId` only). Its existing test ("points a deployed agent with security off to its deployment settings") must still pass; add an assertion that the link's `href` ends with `/agents/{agentId}/deployment`.

### 9.4 Publication status warning

When `publication.status` is `pending` or `failed`: a warning `Alert` with text **`The gateway may not yet enforce these settings.`** followed, when `publication.lastError` is non-empty, by a space and the raw `lastError` (e.g. `The gateway may not yet enforce these settings. gateway rejected the policy`). For `published` and `none` nothing is rendered.

### 9.5 Endpoint section

A pure resolver in the page package turns `{ gatewayUrl, cardUrl, jsonRpcUrl }` plus the parsed card interfaces into `{ endpoints: [{label, url}], declared: interfaces[], cardUrl, snippetEndpoint }`:

**Gateway fronts the agent (`gatewayUrl` non-empty):**

- `endpoints` = `[{ label: "Gateway endpoint", url: gatewayUrl }]` followed by `{ label: "JSON-RPC", url: jsonRpcUrl }` **only when `jsonRpcUrl` is non-empty** (after Task 7 — the row is omitted, never shown blank).
- `declared` = the card's parsed interfaces (information only: the card's own URLs are not where clients should call).
- `snippetEndpoint` = `jsonRpcUrl` (may be `""`).

**Direct (`gatewayUrl` empty):**

- `endpoints` = one row per parsed interface, `label` = its `protocolBinding` or **`Endpoint`** when that is empty, `url` = the interface URL.
- `declared` = `[]`.
- `snippetEndpoint` = `jsonRpcUrl` exactly (after Task 7). **No fallback** to the first interface of any binding (#7): an HTTP+JSON-only card yields `""`.

Rendering:

1. If `endpoints` is empty: secondary `body2` **`The card declares no endpoints yet.`**
2. Each endpoint row: a read-only, full-width, small, monospace, copyable `TextInput` whose static label is the row's `label` and value the URL (copy tooltip default `Copy`).
3. When `cardUrl` is non-empty: one more such row labelled **`Agent card`**.
4. When `declared` is non-empty: a secondary caption **`Declared by the card`** followed by one monospace caption line per interface: `{url}` plus ` · {protocolBinding}` when the binding is non-empty (wrapping anywhere).

### 9.6 Security section — copy per mode

Inline code spans are rendered monospace (`<code>` styled with `fontFamily: "monospace"`).

| `security.mode` | Rendering |
|---|---|
| `apiKey` | `body2`: **`API key required in header `** + code span with `apiKeyHeader` (empty string if missing). Then, if `isProduction`: secondary `body2` **`Keys for production are issued by the agent's owners.`**; else the test-key issuer (§9.7). |
| `oauth` | `body2`: **`OAuth access token required in header `** + code span with `oauth.headerName` or **`Authorization`** when empty; then, only when `oauth.authHeaderPrefix` is non-empty, **` with prefix `** + code span with the prefix. Next line secondary: **`Issuers:`**. Then one `body2` line per issuer (indented, `pl: 1`): **`{name} — {issuer}`** with an em dash, where an empty `issuer` renders as **`not found`** (e.g. `gone-idp — not found`). Then secondary: **`Audiences: {audiences joined ", "}`** or **`Audiences: any`** when the list is empty. No key issuer. |
| `none` | `body2`: **`No authentication required.`** |
| `unmanaged` | `body2`: **`Security isn't managed by the platform. The card declares: {security scheme names joined ", "}.`** — with **`nothing`** in place of the list when the parsed card has no `securitySchemes` keys (e.g. `Security isn't managed by the platform. The card declares: bearer.`). |
| `unknown` (and any unrecognised value) | `body2`: **`Security settings couldn't be determined for this environment.`** |

### 9.7 Test key issuer (apiKey, non-production only)

Purpose: a personal, short-lived key for trying the agent; issuing rotates the caller's previous test key on the backend, so it must only ever happen on an explicit click — never on mount, never on refetch.

Uses the test-key **mutation** hook (§11.3) with the four path params `{ orgName, projName, agentName, envId }` of the selected environment. The issuer reports the current key (string or `null`) up to the drawer so the quick-start snippet can substitute it.

States and copy (after Task 10):

| State | Rendering |
|---|---|
| idle (never issued in this mount) | small outlined `Button` **`Issue test key`**. |
| first issue pending | the button reads **`Issuing…`** and is disabled (plus a 16 px spinner). |
| first issue failed | `body2` error colour **`Couldn't issue a test key. Try again.`** and a small `Button` **`Retry`**. |
| key issued | a read-only, full-width, small, monospace, copyable `TextInput` labelled **`Test API key`** with the key as value and copy tooltip **`Copy API key`**; a secondary caption **`Your personal test key for this environment. It expires {formatRelativeTime(expiresAt)}.`** (e.g. `… It expires in 10 minutes.`); when the response's `gatewayConnected === false`, a warning `Alert`: **`The gateway is not connected to the control plane right now. The API key has been stored but will only work once the gateway reconnects.`**; and a small `Button` **`Rotate`** (disabled while a rotate is pending). |
| rotate pending | the previous key stays displayed and stays in the snippet; `Rotate` disabled; spinner shown. |
| rotate failed | the **previous key stays displayed and in the snippet**; the error text `Couldn't issue a test key. Try again.` is shown; the button still reads **`Rotate`** (not `Retry`). |

Rules:

- The last successfully issued key is held in component state that survives a pending or failed rotate; it is only replaced by a newer successful response.
- The key never survives an environment switch: the details area is remounted per environment (§5.4), so switching to another environment and back shows `Issue test key` again and the snippet shows the placeholder. Switching to a production environment also drops it (the issuer is not rendered there).
- A failed background refetch of the entry or of the connect details must **not** unmount the issuer or reset the key (#10; §5.7, §9.2).
- The mutation shows no success snackbar (the key itself is the confirmation); failures show the standard error snackbar from `useApiMutation` (server message or `Failed to submit test API key`).

### 9.8 Quick start

- Rendered only when `snippetEndpoint` (§9.5) is non-empty. Otherwise, in its place (same position, under a `Quick start` title or as the section body — ambiguity A-6), a `body2` message: **`This agent declares no JSON-RPC endpoint, so there is no quick start.`** and **no** code block (Task 7).
- Language toggle: an exclusive small `ToggleButtonGroup` with buttons **`Python`**, **`TypeScript`**, **`curl`** (default **Python**); deselecting all is impossible (a null change is ignored).
- Below it a `CodeBlock` with the snippet for the chosen language, highlight language `python` / `typescript` / `bash` respectively, and a constant field id (so copy analytics have a stable `snippet_type`).
- Snippet inputs derived from the drawer's data:
  - `cardUrl` = resolved `cardUrl`; `endpointUrl` = `snippetEndpoint`;
  - `auth`: mode `apiKey` → `{ header: apiKeyHeader ?? "", value: issuedKey ?? "<YOUR_API_KEY>" }`; mode `oauth` → `{ header: oauth.headerName || "Authorization", value: prefix ? "{prefix} <ACCESS_TOKEN>" : "<ACCESS_TOKEN>" }`; any other mode → no auth;
  - `authUnmanaged` = `security.mode === "unmanaged"` **exactly** (after Task 6; the branch's `mode !== "none"` wrongly labels `unknown` as unmanaged — #12).
- The snippet re-renders whenever the issued key changes, so an issued key replaces `<YOUR_API_KEY>` in all three languages and a dropped key restores the placeholder.

### 9.9 Copy to clipboard

- Every URL row and the test-key field use the views `TextInput` copy button: writes the value, tooltip `Copy` (or `Copy API key` for the key) becomes `Copied!` for 2 s; clipboard failures are silent.
- The snippet and raw-JSON blocks use `CodeBlock`'s copy button: writes the whole code, shows `Copied!` for 2 s, tracks `CopySnippet` with `snippet_type` = the block's constant id and the highlight language. The copied text must be byte-identical to what the block displays (no trailing-whitespace trimming), including the U+2028/U+2029 **escapes** of §10.

---

## 10. Quick-start snippets (exact rules)

A pure builder `(language, { cardUrl, endpointUrl, auth: {header, value} | null, authUnmanaged?: boolean }) → string` in the page package. Written against A2A protocol 1.0, `a2a-sdk` 1.2 (Python) and `@a2a-js/sdk` 1.3 (TypeScript).

### 10.1 Universal rules (after Task 6; closes #1, #6, #12)

1. **Built only from escaped literals.** Every input value (`cardUrl`, `endpointUrl`, `auth.header`, `auth.value`) appears in the output only as a language string literal:
   - Python: the JSON string encoding of the value (double-quoted; `"`, `\`, control characters escaped).
   - TypeScript: the JSON string encoding **plus** every U+2028 replaced by the six characters `\u2028` and every U+2029 by `\u2029` (JavaScript treats the raw characters as line terminators; JSON encoding leaves them raw).
   - curl: single-quoted shell literal where each `'` in the value becomes `'\''` (quote, backslash, quote, quote).
2. **No input value inside a comment, in any language.** Comments carry constant text only.
3. **Constant endpoint comment**: the line before the endpoint variable is exactly `The card routes JSON-RPC calls to this endpoint.` (prefixed `# ` in Python, `// ` in TypeScript). The endpoint itself is emitted as a variable: Python `JSONRPC_URL = <literal>`, TypeScript `const jsonRpcUrl = <literal>;`. curl uses the endpoint as the POST argument. (The SDK snippets resolve the real endpoint from the card; the variable is informational.)
4. **Unmanaged preamble**: when `authUnmanaged` is true **and** there is no `auth`, a comment line with the constant text `The platform does not manage this agent's security; add whatever auth its agent card declares.` is emitted (Python: after the endpoint line; TypeScript: after the endpoint line, before the headers constant; curl: as the very first line). It appears for security mode `unmanaged` only — never for `unknown`, `none`, `apiKey` or `oauth`.
5. **Auth placement**: with `auth`, the header is sent on the card fetch and on the message call: Python `httpx.AsyncClient(headers={<header literal>: <value literal>})`; TypeScript a `headers` record `{ <header literal>: <value literal> }` injected into the card resolver's fetch and passed as `serviceParameters`; curl a `-H '<header>: <value>'` line on both requests. Without `auth`: Python `httpx.AsyncClient()` with no arguments, TypeScript `const headers: Record<string, string> = {};`, curl no `-H` auth lines. The placeholders are literally `<YOUR_API_KEY>` (API key) and `<ACCESS_TOKEN>` (OAuth, optionally prefixed, e.g. `Bearer <ACCESS_TOKEN>`).
6. **TypeScript has no top-level `await`**: all awaiting code sits inside `async function main(): Promise<void> { … }`, and the file ends with `main().catch((err) => { console.error(err); process.exitCode = 1; });`. The snippet must run under `npx tsx client.ts` in a fresh `npm init -y && npm install @a2a-js/sdk tsx` project (CommonJS default) without "Top-level await" / TS1378 errors, failing only on the network call.

### 10.2 Rendered shape — Python

Expected output (`«x»` = the escaped literal of input x; bracketed lines are conditional):

```
# pip install a2a-sdk
import asyncio
import uuid

import httpx
from a2a.client import ClientConfig, create_client
from a2a.types import Message, Part, Role, SendMessageRequest

CARD_URL = «cardUrl»
# The card routes JSON-RPC calls to this endpoint.
JSONRPC_URL = «endpointUrl»
[# The platform does not manage this agent's security; add whatever auth its agent card declares.]

async def main() -> None:
    async with httpx.AsyncClient([headers={«header»: «value»}]) as http:
        client = await create_client(
            CARD_URL, ClientConfig(httpx_client=http), relative_card_path="/"
        )
        message = Message(
            message_id=str(uuid.uuid4()),
            role=Role.ROLE_USER,
            parts=[Part(text="Hello")],
        )
        async for event in client.send_message(SendMessageRequest(message=message)):
            print(event)


asyncio.run(main())
```

### 10.3 Rendered shape — TypeScript

```
// npm install @a2a-js/sdk
import { Role, type SendMessageRequest } from "@a2a-js/sdk";
import {
  ClientFactory,
  ClientFactoryOptions,
  DefaultAgentCardResolver,
} from "@a2a-js/sdk/client";

const cardUrl = «cardUrl»;
// The card routes JSON-RPC calls to this endpoint.
const jsonRpcUrl = «endpointUrl»;
[// The platform does not manage this agent's security; add whatever auth its agent card declares.]
const headers: Record<string, string> = «{} or { «header»: «value» }»;

const fetchWithHeaders: typeof fetch = (input, init) =>
  fetch(input, { ...init, headers: { ...Object.fromEntries(new Headers(init?.headers)), ...headers } });

async function main(): Promise<void> {
  const factory = new ClientFactory(
    ClientFactoryOptions.createFrom(ClientFactoryOptions.default, {
      cardResolver: new DefaultAgentCardResolver({ fetchImpl: fetchWithHeaders }),
    }),
  );
  const client = await factory.createFromUrl(cardUrl, "");

  const request: SendMessageRequest = {
    tenant: "",
    message: {
      messageId: crypto.randomUUID(),
      contextId: "",
      taskId: "",
      role: Role.ROLE_USER,
      parts: [
        { content: { $case: "text", value: "Hello" }, metadata: undefined, filename: "", mediaType: "text/plain" },
      ],
      metadata: undefined,
      extensions: [],
      referenceTaskIds: [],
    },
    configuration: undefined,
    metadata: undefined,
  };
  const result = await client.sendMessage(request, { serviceParameters: headers });
  console.log(JSON.stringify(result, null, 2));
}

main().catch((err) => {
  console.error(err);
  process.exitCode = 1;
});
```

(The request literal mirrors the SDK 1.3 `SendMessageRequest` shape; builders may adjust field defaults to whatever the installed SDK version requires, as long as rules 10.1 hold.)

### 10.4 Rendered shape — curl

```
[# The platform does not manage this agent's security; add whatever auth its agent card declares.]
# Fetch the agent card
curl -s '«cardUrl»'[ \
  -H '«header»: «value»']

# Send a message (JSON-RPC SendMessage)
curl -s -X POST '«endpointUrl»' \
  -H 'Content-Type: application/json' \
  -H 'A2A-Version: 1.0'[ \
  -H '«header»: «value»'] \
  -d '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "SendMessage",
  "params": {
    "message": {
      "messageId": "msg-1",
      "role": "ROLE_USER",
      "parts": [
        {
          "text": "Hello"
        }
      ]
    }
  }
}'
```

Continuation lines are joined with ` \` + newline; the JSON body is pretty-printed with 2-space indentation and shell-quoted as one argument.

---

## 11. api-client / hook additions needed by this page

All in `console/workspaces/libs/api-client/src/{apis,hooks}/` and exported from both barrels. The literal query-key strings below are the branch's; they are not externally observable except through the default error snackbar text, so builders may rename them provided the invariants hold (ambiguity A-8).

### 11.1 Catalogue-entry hook (shared with the list spec; stated here because this page depends on its exact behaviour)

| Aspect | Rule |
|---|---|
| Fetch | `GET /api/v1/orgs/{org}/projects/{proj}/agents/{agent}/a2a-catalog-entry` with `encodeRequired` on each segment; returns the parsed JSON. |
| Hook result type | the catalogue entry, or `null` when the server answers 404 (`nullIfNotFound`). |
| Query key | `["a2a-catalog-entry", { orgName, projName, agentName }]` — a prefix distinct from the list query's key. |
| `enabled` | all three params non-empty |
| `staleTime` | 30 000 ms (same constant as the list: a back-navigation within 30 s reuses the cached read) |
| `retry` | `false` |
| `silent` | `true` — the page renders its own error states (§5.2); no snackbar. |

### 11.2 Connect-details hook

| Aspect | Rule |
|---|---|
| Fetch | `GET …/environments/{env}/a2a-connect` via the shared per-environment path builder (§4, agent-card.ts) with resource `a2a-connect`; returns the parsed JSON. |
| Query key | `["a2a-connect", { orgName, projName, agentName, envId }]` |
| `enabled` | all four params non-empty (no request without an environment) |
| staleTime / retry / silent | app defaults: 8 s stale, retry up to 3 (not on 4xx ≠ 408), **not** silent — a terminal failure toasts the default `Failed to fetch A2a Connect`-style message in addition to the inline error (ambiguity A-9). |

### 11.3 Test-key issue mutation hook (`hooks/agent-api-keys.ts`, next to `useTestAgentAPIKey`)

| Aspect | Rule |
|---|---|
| Built on | the existing fetch function `issueTestAgentAPIKey` (`POST …/environments/{env}/api-keys/test`). |
| Wrapper | `useApiMutation` with `action: { verb: "create", target: "test API key" }` and `showSuccess: false`; `mutationFn` takes the four path params as the variables and calls the fetch with `getToken`. |
| Semantics | each call issues (or rotates) the **caller's** test key for that environment — the backend derives the key name from the JWT subject, so it never touches other users' keys. No query invalidation (the Try-It query's cache is left alone — ambiguity A-10). |
| Consumer contract | `mutate(params)`, `data?.apiKey`, `data?.expiresAt`, `data?.gatewayConnected`, `isPending`, `isError`, `isIdle`. |

### 11.4 Per-environment agent-card API (`apis/agent-card.ts`)

Behaviour unchanged; the private path builder becomes exported and parameterised by resource name (§4) and `apis/a2a-proxy.ts` reuses it. `hooks/agent-card.ts` (`useGetAgentCard`) is **not** modified; this page passes `{ enabled: entryLoaded }` for the selected environment and `{ enabled: compareEnv !== "", poll: false }` for the compare target.

### 11.5 Environment labels

A tiny page-level hook wraps `useListEnvironments({ orgName })` and returns `(name) => displayName || name`; while the list is loading or missing, the raw name is returned.

### 11.6 Query-string updates

A page-level helper over `useSearchParams` applies a mutation to a **copy** of the current params and calls `setSearchParams(next, { replace: true })`, so environment changes never add history entries.

---

## 12. Contract summary consumed by this page

| Call | Method + path | Used for |
|---|---|---|
| Catalogue entry | `GET /api/v1/orgs/{orgName}/projects/{projName}/agents/{agentName}/a2a-catalog-entry` → entry JSON; 404 = not in catalogue | page shell, chips, kind, name/version fallback |
| Agent card (existing) | `GET /api/v1/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/agent-card` → `AgentCardResponse`; 404 = no card configured | card details, compare, header, declared interfaces, card security schemes |
| Connect details | `GET …/environments/{envID}/a2a-connect` → §9.1 | drawer |
| Test key (existing) | `POST …/environments/{envID}/api-keys/test` (empty body) → `IssueTestAgentAPIKeyResponse` | test key issuer |
| Environments (existing) | `GET /api/v1/orgs/{orgName}/environments` via `useListEnvironments` | chip labels |

Authorization: the bearer token from `useAuthHooks().getToken` on every call; the page has no additional RBAC gating of its own.

---

## 13. Test scenarios (Given / When / Then)

Component tests mock the api-client hooks (`vi.mock`), render inside a `MemoryRouter` with the route `/org/:orgId/a2a-agents/view/:projectName/:agentName`, and replace `views`/`shared-component` display pieces with minimal stand-ins that **render their `value`/`code`** (so key and snippet text can be asserted). Pure utilities are unit-tested directly.

### 13.1 Agent view (page shell)

1. **Reads the agent from the URL.** Given the route `/org/acme/a2a-agents/view/p1/orders`; When the page renders; Then the entry hook is called with `{ orgName: "acme", projName: "p1", agentName: "orders" }` and the heading shows the entry's name `Orders`.
2. **Loading shows no claims.** Given the entry query is loading with no data; Then the page shows its loading state and neither `platform`, `external`, any skill, nor any card copy is visible.
3. **Not in catalogue.** Given the entry hook resolves `null`; Then `This agent was not found in the catalogue.` is shown.
4. **Entry failure without data.** Given the entry hook returns `{ data: undefined, error }`; Then `Failed to load the agent. Please try again.` is shown and nothing else.
5. **Regression #10 — failed entry refetch keeps the page and the issued key.** Given the page rendered with data and the drawer open, the user clicked `Issue test key` and the key `test-key-1` is shown and present in the snippet; When the entry hook re-renders returning the same `data` plus an `error`; Then the details, drawer and `test-key-1` (field and snippet) are still shown and the warning `Couldn't refresh; showing the last loaded details.` appears; no `Failed to load the agent` alert.
6. **Opens on the lead environment.** Given an entry with `leadEnvironment: "dev"` and no `env` query; Then the `dev` chip is selected and the card request is for `envId: "dev"`.
7. **Opens on the URL's environment and switches from the chips.** Given `?env=prod` and prod's card lists `issue-refund`; Then `issue-refund` is shown; When the `Development` chip is clicked; Then the URL's `env` becomes `dev` (history replaced, other params kept) and `issue-refund` is gone.
8. **Chip labels use display names.** Given environments `dev` → `Development`, `prod` → `Production`; Then the chips read `Development` and `Production`; Given an environment with no display name; Then its chip reads its raw name.
9. **Regression #9-note — header follows the selected card.** Given the entry's `name: "Orders", version: "1.0"` and the selected (ready) card's `name: "Orders Agent", version: "2.1"`; Then the heading reads `Orders Agent` and the version chip `v2.1`; When the selected card is `noBody`/`absent`/`loading`/`error`; Then the heading reads `Orders` and the chip `v1.0`.
10. **Kind chip source.** Given the card response `source: "external"` for the selected environment; Then the kind chip reads `external`; Given no card response yet and entry `kind: "platform"`; Then it reads `platform`.
11. **Description line.** Given project `p1` and a ready card with `provider.organization: "Acme"`; Then the description reads `p1 · Acme`; without a provider organization it reads `p1`.
12. **Back link.** Then a link labelled `Back to A2A Agents` points at `/org/acme/a2a-agents`.
13. **Card query waits for the entry.** Given the entry is loading; Then the card hook is called with `enabled: false`.
14. **Connect only on click.** Given the page rendered; Then no connect section is mounted and the connect hook has not requested; When `Connect` is clicked; Then a drawer titled `Connect · Development` opens with the connect section.
15. **Drawer content stays mounted after closing.** Given the drawer was opened; When it is closed; Then the connect section is still mounted (hidden) so an issued key survives re-opening.
16. **Environment change remounts details.** Given a compare environment selected and the drawer open; When another chip is clicked; Then the compare select is back to `None` and the drawer is closed and unmounted.
17. **Esc stays local.** Given the Compare dropdown is open; When Esc is pressed; Then only the dropdown closes and the page URL is unchanged. Given the drawer is open; When Esc is pressed; Then the drawer closes and the page URL is unchanged.

### 13.2 Card state model and card details

18. **State mapping (unit).** `{data: readyCard}` → ready; `{data: readyCard, isError: true}` → ready; `{data: {status:"pending", card: undefined}}` → noBody; `{data: null}` → absent; `{data: undefined, isError: true}` → error; `{data: undefined, isLoading: true}` → loading; `{data: undefined, isLoading: false, isError: false}` → loading.
19. **Loading copy.** Given the card hook is loading with no data; Then `Loading card…` is shown and `No agent card is configured` is not.
20. **Absent copy.** Given the card hook resolves `null`; Then `No agent card is configured for this environment.` is shown.
21. **Regression #9 — card error.** Given the card hook returns `isError: true` with no data (a 500/503/403/network failure); Then `Couldn't load the agent card.` is shown and `No agent card is configured` is not.
22. **Regression #8 — pending card with no body.** Given `{ status: "pending", card: undefined }`; Then `Card pending` is shown and neither `No skills declared.` nor `✗ streaming` nor a Compare select is rendered.
23. **Failed card shows the error.** Given `{ status: "failed", lastError: "connection refused", card: undefined }`; Then `Card fetch failed` and `connection refused` are shown.
24. **Fetched caption.** Given `status: "fetched", fetchedAt` two minutes ago; Then `Fetched 2 minutes ago` is shown.
25. **Skills and capabilities.** Given a ready card with skill `order-status` and `capabilities: { streaming: true }`; Then `order-status` is listed, `✓ streaming`, `✗ pushNotifications`, `✗ extendedAgentCard` are shown.
26. **No skills.** Given a ready card whose `skills` is missing or not an array; Then `No skills declared.` is shown.
27. **Skill item content.** Given a skill with description, tags `["orders"]`, inputModes `["text/plain"]`, outputModes `["application/json"]`, examples `["Where is order 7?"]`; Then the item shows the name, the description, a chip `orders`, `Input` + chip `text/plain`, `Output` + chip `application/json`, and `e.g. Where is order 7?`.
28. **Raw JSON.** Given a ready card; Then no JSON block is shown; When `View raw JSON` is clicked; Then a JSON block containing `order-status` appears and the button reads `Hide raw JSON`.

### 13.3 Compare

29. **Selector presence.** Given a ready card and two environments; Then a `Compare with` select with options `None`, `Production` exists; Given only one environment; Then no selector.
30. **Compare query.** When `Production` is selected; Then the card hook is called for `envId: "prod"` with `poll: false`, and with `enabled: false` while `None` is selected.
31. **Added skills.** Given dev `[order-status]`, prod `[order-status, issue-refund]`; When comparing with Production; Then `Added: issue-refund` is shown.
32. **Removed and changed skills.** Given dev `[order-status, issue-refund]`, prod `[order-status (different description)]`; Then `Removed: issue-refund` and `Changed: order-status`.
33. **Other fields.** Given cards differing only in `version`; Then `Also differs in: version` and no skill lines.
34. **Identical.** Given canonically equal cards (different key order, different interface URLs); Then `The cards are identical.`
35. **Regression #8 — compare target without body.** Given the compare environment's card is `{status: "pending", card: undefined}`; Then `No fetched card to compare.` and no `Added:` line.
36. **Regression #9 — compare target error / loading.** Given the compare query errored with no data; Then `Couldn't load the card to compare.`; Given it is loading; Then `Loading…`.
37. **Regression #5 — compare agrees with `cardDiffers`.** Given dev and prod cards that differ only in one skill's `inputModes`; Then `Changed: <skill>`. Given they differ only in skill order; Then `Also differs in: skills`. Given dev `[a, a]` (duplicate names) vs prod `[a]`; Then `Also differs in: skills`. In none of these is `The cards are identical.` shown.

### 13.4 Shared card utilities (unit)

38. **Parse reads every field** of a full v1 card (name, description, version, provider organization and URL, interfaces, skills with all seven fields, capabilities, security scheme keys).
39. **Parse is defensive.** `{ skills: "x", capabilities: null }` → skills `[]`, all capabilities false; `undefined` → empty record; `provider.url: "javascript:alert(1)"` → `providerUrl: ""` but organization kept; interfaces without `url` dropped; a numeric `protocolBinding` reads `""`.
40. **Diff — added/removed/changed** as in 31–32; `fields` empty when only skills differ.
41. **Diff — fields ignore key order** (`{version:"1", capabilities:{a,b}}` vs `{version:"2", capabilities:{b,a}}` → `["version"]`).
42. **Diff — interface URLs ignored, bindings not**: same binding, different URLs → no fields; same URL, different binding → `["supportedInterfaces"]`.
43. **Regression #5 — same terms as the fingerprint**: `"a skill's inputModes change marks it changed"` → `changed == ["a"]`; `"reordered skills report skills as a differing field"` → `fields` contains `"skills"`; `"duplicate skill names still differ"` (`[a, a]` vs `[a]`) → `fields` contains `"skills"`; `"nested key order is not a difference"` → everything empty. Plus the property: for any pair, all-empty ⇔ canonical forms without interface URLs are equal.

### 13.5 Connect section — states, security, manage link, publication

44. **Loading.** Given the connect hook is loading; Then a spinner and no sections.
45. **Error without data.** Given `isError` and no data; Then `Couldn't load connection details for this environment.`
46. **Regression #10 — failed connect refetch inside the drawer.** Given connect data loaded and a key issued; When the connect hook re-renders with the same data plus `isError: true`; Then the endpoints, security, key `test-key-1` and snippet with `test-key-1` remain and the warning `Couldn't refresh; showing the last loaded details.` is shown.
47. **API key, non-production.** Given `mode: "apiKey", apiKeyHeader: "X-API-Key", isProduction: false`; Then `API key required in header` with `X-API-Key`, an `Issue test key` button, no issue request yet, no key value shown, and the snippet contains `<YOUR_API_KEY>`.
48. **API key, production.** Given `isProduction: true`; Then `Keys for production are issued by the agent's owners.` and no `Issue test key` button.
49. **OAuth.** Given issuers `corp-idp → https://idp/x`, `gone-idp → ""`, audiences `["orders-api"]`, header `Authorization`, prefix `Bearer`; Then `OAuth access token required in header Authorization with prefix Bearer`, `Issuers:`, `corp-idp — https://idp/x`, `gone-idp — not found`, `Audiences: orders-api`, no key issuer, and the snippet contains `Authorization` and `Bearer <ACCESS_TOKEN>`. Given empty audiences; Then `Audiences: any`.
50. **None.** Given `mode: "none"`; Then `No authentication required.` and the snippet has no `X-API-Key`.
51. **Unknown.** Given `mode: "unknown"`; Then `Security settings couldn't be determined for this environment.`
52. **Regression #12 — unknown is not unmanaged.** Given `mode: "unknown"`; Then the rendered snippet (every language) does **not** contain `does not manage this agent's security`.
53. **Unmanaged.** Given a direct external agent (`gatewayUrl: ""`, `mode: "unmanaged"`, `publication.status: "none"`) whose card declares security scheme `bearer`; Then `Security isn't managed by the platform. The card declares: bearer.`, no `Gateway endpoint` row, the first endpoint row holds the card's interface URL, the snippet has no `X-API-Key` and does contain the unmanaged note. Given no schemes; Then `…The card declares: nothing.`
54. **Regression #2 — Manage security for a platform agent.** Given entry `kind: "platform"`, org `acme`, project `p1`, agent `orders`; Then the `Manage security` link's href is `/org/acme/project/p1/agents/orders/deployment`. Given `kind: "external"` and env `dev`; Then it is `/org/acme/project/p1/agents/orders/configure?tab=a2a&env=dev`.
55. **Regression #2 — Security.Component.** Given a deployed non-external agent with security disabled; Then the `Go to Deployment Settings` link's href ends with `/agents/{agentId}/deployment` (and renders, not Not Found, under the real router).
56. **Publication warning.** Given `publication: { status: "failed", lastError: "gateway rejected the policy" }`; Then `The gateway may not yet enforce these settings.` and `gateway rejected the policy` are shown. Given `pending` with empty error; Then only the sentence. Given `published` or `none`; Then no warning.

### 13.6 Endpoint resolution

57. **Gateway case.** Given `gatewayUrl: "https://gw.example/orders"`, `jsonRpcUrl: "https://gw.example/orders/rpc"`, card interface `http://localhost:9099/rpc JSONRPC`; Then endpoints are `[Gateway endpoint → gatewayUrl, JSON-RPC → jsonRpcUrl]`, declared = the card interfaces, snippet endpoint = `jsonRpcUrl`.
58. **Gateway case without JSON-RPC.** Given `gatewayUrl` set and `jsonRpcUrl: ""`; Then endpoints are `[Gateway endpoint]` only (no blank `JSON-RPC` row) and snippet endpoint `""`.
59. **Direct case.** Given `gatewayUrl: ""`, `jsonRpcUrl: "https://origin.example/rpc"`, interfaces `[rest HTTP+JSON, rpc JSONRPC]`; Then endpoints `[HTTP+JSON → …/rest, JSONRPC → …/rpc]`, declared `[]`, snippet endpoint `…/rpc`.
60. **Regression #7 — no fallback to a non-JSON-RPC interface.** Given a direct agent with `jsonRpcUrl: ""` and only an `HTTP+JSON` interface; Then snippet endpoint is `""` (the branch's test expecting the REST URL is replaced by this one).
61. **Direct case, no interfaces.** Given `jsonRpcUrl: ""` and no interfaces; Then endpoints `[]`, snippet endpoint `""`, and the drawer shows `The card declares no endpoints yet.`
62. **Label fallback.** Given an interface with empty `protocolBinding`; Then its row is labelled `Endpoint`.
63. **Proxied agent is addressed at the gateway.** Given `proxied: true`, `gatewayUrl: "https://gw/agents/x"`, `jsonRpcUrl: "https://gw/agents/x/rpc"` and a card interface at the origin; Then the first endpoint row is the gateway URL, `Declared by the card` lists the origin, and the curl snippet contains `https://gw/agents/x` and not the origin URL.
64. **Regression #7 — no JSON-RPC endpoint message.** Given `jsonRpcUrl: ""`; Then the drawer shows `This agent declares no JSON-RPC endpoint, so there is no quick start.` and no code block.
65. **Agent card row.** Given `cardUrl` non-empty; Then a row labelled `Agent card` with it; Given empty; Then no such row.

### 13.7 Snippets

66. **Each language names the card URL and the endpoint** as literals (contains both strings).
67. **Each language sends the auth header when given** (`X-API-Key` and `k1` present) and **none without auth** (no `X-API-Key`, no `Authorization`).
68. **Unmanaged note** appears in each language with `authUnmanaged: true` and no auth; does not appear without the flag; does not appear with the flag when auth is present.
69. **curl posts a v1 JSON-RPC SendMessage**: contains `-X POST '<endpoint>'`, `"method": "SendMessage"`, `"role": "ROLE_USER"`, `A2A-Version: 1.0`.
70. **curl shell-quotes a single quote**: value `it's` → `'X-API-Key: it'\''s'`.
71. **Python and TypeScript escape quotes**: value `a"b` → `"a\"b"` in both.
72. **Regression #1 — no card value reaches a comment.** For each language, with `endpointUrl = "https://x/rpc\u2028evil()//"`; Then every output line beginning with `#` or `//` contains neither `x/rpc` nor `evil`.
73. **Regression #1 — TypeScript escapes line separators.** Given an endpoint containing U+2028 and U+2029; Then the TypeScript output contains neither raw character and contains the six-character sequences `\u2028` and `\u2029`.
74. **Regression #6 — no top-level await.** The TypeScript output does not match `/^\S.*\bawait\b/m` (every `await` is indented inside `main`) and ends with the `main().catch(…)` footer.
75. **Constant endpoint comment.** Python output contains the line `# The card routes JSON-RPC calls to this endpoint.` immediately followed by `JSONRPC_URL = "<endpoint>"`; TypeScript contains `// The card routes JSON-RPC calls to this endpoint.` followed by `const jsonRpcUrl = "<endpoint>";`.
76. **Manual verification (plan Task 6 step 5).** In a scratch directory: `npm init -y && npm install @a2a-js/sdk tsx`; paste the generated TypeScript snippet (with placeholder auth) into `client.ts`; run `npx tsx client.ts`. Expected: no "Top-level await is currently not supported with the cjs output format" and no TS1378; the run fails only on the network request to the card URL. Record the SDK version used.

### 13.8 Test key issuer

77. **Issue on click only.** Given an apiKey non-production environment; When `Issue test key` is clicked; Then the mutation is called once with `{ orgName: "acme", projName: "p1", agentName: "orders", envId: "dev" }`, the key is shown in a `Test API key` field, the snippet contains the key, `It expires in 10 minutes.` is shown.
78. **Issuing label.** While the first issue is pending; Then the control reads `Issuing…` and is disabled.
79. **Gateway disconnected warning.** Given the response has `gatewayConnected: false`; Then `The gateway is not connected to the control plane right now. The API key has been stored but will only work once the gateway reconnects.` is shown; with `true` or absent it is not.
80. **Rotate.** Given a key was issued; When `Rotate` is clicked; Then the mutation is called a second time; on success the new key replaces the old one in the field and the snippet.
81. **Regression #10 — failed rotate keeps the previous key.** Given key `k1` issued; When `Rotate` fails; Then `k1` is still shown in the field and the snippet, `Couldn't issue a test key. Try again.` is shown, and the button still reads `Rotate`.
82. **First issue failed.** When the first issue fails; Then the error text and a `Retry` button are shown, no key field, snippet still `<YOUR_API_KEY>`.
83. **Environment change resets the key.** Given `k1` issued in `dev`; When the user switches to `staging`; Then no further mutation is called, no key value is shown, and the snippet does not contain `k1`; When switching back to `dev`; Then `Issue test key` is offered again and `k1` is not shown (no automatic re-issue).
84. **Production switch drops the key.** Given `k1` issued in `dev`; When switching to a production environment; Then the snippet shows `<YOUR_API_KEY>` and no issuer; switching back to `dev` does not bring `k1` back.
85. **No success snackbar** on issue; the standard error snackbar on failure.

### 13.9 Hooks (api-client)

86. **Connect hook path encoding.** Given params `{ orgName: "acme", projName: "p 1", agentName: "a/b", envId: "dev" }`; Then it GETs `/api/v1/orgs/acme/projects/p%201/agents/a%2Fb/environments/dev/a2a-connect`.
87. **Connect hook disabled without an environment.** Given `envId: ""`; Then no request.
88. **Entry hook maps 404 to null, is silent, does not retry, 30 s stale.**
89. **Test-key hook** calls the existing fetch (`POST …/api-keys/test`) with the variables and shows no success snackbar.
90. **a2a-proxy fetches** still hit `…/environments/{env}/a2a-proxy` after the path-builder refactor (existing tests pass).

### 13.10 Existing components after the shared-utility switch

91. **`A2AAgentCardPanel`**: with a fetched card, renders name, `v`-normalised version, description, interface rows with binding chip and copyable URL, and one skill item per skill; interfaces without URL are not listed; `View JSON` toggle still works.
92. **`EnvAgentCardSummary`**: renders `Provider` with the organization and the external-link button only for an http(s) URL; `Interfaces` rows; `Skills` name chips; the empty message when nothing is declared; the loading/register/error/not-available states unchanged.

---

## 14. Known limitations / out of scope (kept as on the branch)

- The drawer's publication warning can be stale for a few seconds after a publication completes (the "drawer's stale pending warning" note — left out by the plan).
- Blank environment labels: an environment whose `displayName` is whitespace shows that whitespace (plan: left out).
- The card details do not show: attempt count / next attempt, the card source URL, the provider website link, the documentation URL, raw interface list (interfaces appear only in the Connect drawer as "Declared by the card" or as direct endpoints).
- The TypeScript/Python `jsonRpcUrl` variables are informational; the SDKs resolve the endpoint from the card.
- The compare target is not polled; a pending compare card will not refresh until re-selected or the page is reloaded.
- No per-user RBAC gating in the page beyond what the endpoints enforce.
- The default error snackbar for the connect query uses the auto-generated title (§11.2).
- Task 12 (PR splitting) and the `CLAUDE.md` Oxygen link are not part of this spec.

---

## 15. Ambiguities resolved (controller may overrule)

- **A-1 — "Esc stays local" and "resolve unseen selected agents".** These phrases come from commits made when the catalogue had a side panel (selection kept in list state). The branch later replaced the panel with this routed page. I mapped them to the current design: "unseen agents" = the page always resolves the agent through the catalogue-entry endpoint rather than the list cache (§5.2); "Esc stays local" = no page-level key handler, Esc acts only on the innermost surface (§5.6).
- **A-2 — Header with a ready card that has an empty `name`.** Plan says title and version come "together" from the ready card. I specified: title falls back to the entry's `name` only when the card's `name` is empty; version is the card's version (possibly empty → no chip).
- **A-3 — Esc handling.** Specified as MUI defaults (Select closes its menu; Drawer calls onClose) with no custom handler, and a scenario that the URL does not change.
- **A-4 — "Issuing…" and "Retry" semantics.** Plan: `Rotate` once a key exists, `Retry` only after a failed first issue, `Issuing…` while the first issue is pending. I specified `Issuing…` as the label of the (disabled) issue button plus a spinner; a pending rotate shows the spinner and disables `Rotate` without relabelling.
- **A-5 — Error copy with cached connect data.** Plan gives the same warning sentence for both the page and the drawer; I reused it verbatim in both places.
- **A-6 — Where the "no JSON-RPC endpoint" message sits.** Plan: "render the message in place of the Quick start block". I specified it in the quick-start position; whether the `Quick start` title stays above it is left to builders.
- **A-7 — Compare select label.** The branch uses an `InputLabel` on a `Select`; `console/AGENTS.md` forbids floating labels on `TextField` only. I kept "a select labelled `Compare with`" (accessible name must be `Compare with`) and leave the label mechanism to the builders.
- **A-8 — Query-key literals.** Given as the branch's values with the invariant (distinct prefixes, params tuple) as the real requirement.
- **A-9 — Connect query snackbar.** The branch's connect hook is not silent, so a terminal failure toasts an auto-titled message in addition to the inline error; I documented rather than changed it.
- **A-10 — Test-key mutation and the Try-It cache.** The branch's mutation does not invalidate `["agent-test-api-key", …]`; a user with the Try-It page open elsewhere would still hold its own cached key for up to 9 minutes while this page rotated it. Kept as on the branch.
- **A-11 — Rendered snippet templates.** §10.2–10.4 show the snippets as the product's *output* (what users copy), including the plan's changes, with inputs as placeholders. They are included because the snippet text is user-visible and must run against the named SDK versions; the request/response field shapes may be adjusted to the installed SDK.
- **A-12 — Compare query stale/retry.** Uses the existing `useGetAgentCard` unchanged (`retry: false`, silent, `poll: false`) — no new hook.
- **A-13 — Per-environment agent-card hook.** The task mentions hook changes; the branch changes only the fetch module (exported path builder). Specified accordingly; `useGetAgentCard` is untouched.

## 16. Could not determine from the sources

- Whether the `Quick start` section title is kept above the "no JSON-RPC endpoint" message (A-6).
- The exact SDK field defaults of `@a2a-js/sdk` 1.3's `SendMessageRequest` beyond what the branch emits; the manual verification (scenario 76) is the check.
- Whether the backend's `a2a-connect` endpoint has permission requirements beyond the agent-card read; the console passes the bearer token and renders the generic error on 403.
