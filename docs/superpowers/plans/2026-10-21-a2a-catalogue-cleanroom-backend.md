# A2A Agent Catalogue — Backend (Part A) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the org-wide A2A agent catalogue (list, single entry) and the per-environment connect-details operation to agent-manager-service, backed by a card projection stored at fetch time and the transports recorded at publish time, and regenerate the amctl client.

**Architecture:** Three new GET operations flow spec-first through `docs/api_v1_openapi.yaml` → `spec/` → a new controller → a new `A2ACatalogService` → a new read-only `A2ACatalogRepository` over `a2a_agent_cards` ⋈ `a2a_proxies`. Two migrations add the card projection columns (fingerprint + search text, written by the card reconciler) and the published-transports column (written by the publication reconciler). Environment order per entry comes from the project's OpenChoreo deployment pipeline, fetched once per project per request under a shared deadline. Connect details reuse the card service, the publisher (new gateway-status helper), the proxy/config/gateway repositories and the OpenChoreo client.

**Tech Stack:** Go 1.26, GORM/Postgres, google/wire, matryer/moq v0.5.3, openapi-generator 7.0.0 (Docker) for `spec/`, oapi-codegen v2.6.0 for `cli/`, testify, `golang.org/x/sync/errgroup`.

**Spec:** `docs/superpowers/specs/2026-10-21-a2a-catalogue-cleanroom-spec.md` — Part A (Backend). Section numbers below (§3, §4.6, …) refer to Part A.

## Global Constraints

- Work only in this worktree (`/home/lyuda/agent-manager-worktrees/a2a-catalogue-cleanroom`, branch `a2a-catalogue-cleanroom`); never read another checkout, worktree, stash, or any branch other than this one and `upstream/main`.
- Go conventions per `agent-manager-service/AGENTS.md`: sentinels matched with `errors.Is`, everything else wrapped `fmt.Errorf("...: %w", err)`; never flatten an unexpected error into not-found; never fall back to a default on a non-not-found error; every I/O method takes `context.Context` first; `ou_id` always from the token (`middleware.OUIDFromRequest`), never the path.
- Unit tests: `package <pkg>`, **no** build tag. Repository/migration tests: `//go:build integration` on line 1. Never unit-test the repository layer. Reuse `strPtr`, `discardLogger`, `testLogger`, `newPublisherMocks` instead of redeclaring. Every `.go` file carries the Apache header (copy from a neighbour).
- Go commands run from `agent-manager-service/`: `make test-run RUN=<pattern> PKG=./services/`, `make test-unit`, `make test-integration` (`.env.test` is present), `make mocks PKG=./repositories/` (moq **v0.5.3**; `make mocks` checks the module version via `go version -m`), `make wire` after touching `wiring/`, `make spec` (Docker), `make fmt`, `make lint`, CI lint `golangci-lint run --config .github/linters/.golangci.yaml ./...` (lints test files too: `nilnil`, `goheader`, `errorlint`, `nilerr`, `nolintlint`, `exhaustruct` for production code). Pinned tools live in `<repo-root>/.tools/bin` and are on PATH inside every make recipe.
- `make amctl-gen-client` runs from the **repo root**; `cd cli && go build ./... && go test ./...` must pass afterwards.
- OpenAPI first: YAML change → `make spec` → `make amctl-gen-client`; generated files are committed with the change that caused them; never hand-edit `spec/` or `cli/pkg/clients/amsvc/gen/`.
- Migrations: numbered after `048_create_a2a_proxies.go`, appended to `db_migrations/migration_list.go`, idempotent (`IF NOT EXISTS`); 049 and 050; `latestVersion` ends at 50.
- Commits: imperative mood, capitalised first word, no trailing period, no `feat:` prefix, no Co-Authored-By; every message ends with the trailer line `Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q`. Stage explicit paths. Never push.
- Contract strings are shared with the console (Parts B/C): paths, query parameter names, JSON field names, enum values and the exact 400/404 messages in §3 must match character for character.

## Review Focus

Inputs the spec implies but its scenarios do not exercise; each is pinned by a test in the task named.

1. `q` that is only whitespace (`q=%20%20`) must mean "no search": the service receives `Query == ""` and every row's `matched` is `false`. (Task 12)
2. With `q` or other filters and an `offset` past the last page, `total` must count only the agents that match the filters, not every agent in the org. (Task 7)
3. A pipeline naming environments the agent has no rows in (dev→staging→prod, rows in dev and prod only) must give `[dev, prod]`, and a row set with no overlap with the pipeline falls back to name order. (Task 9)
4. A platform agent in an environment with no ingress gateway mapped: `gatewayUrl` is `""`, `cardUrl` is the row's last fetched `SourceURL`, `jsonRpcUrl` is the first declared JSON-RPC URL, and `publication`/`security` are still read from the rows. (Task 11)
5. A stored card whose `supportedInterfaces` has a JSON-RPC entry with a non-string `url`, or is not an array, must declare nothing rather than fail the connect request. (Task 11)

---

## File map

| Path | Role |
|---|---|
| `agent-manager-service/docs/api_v1_openapi.yaml` | Three operations + eight schemas (Task 1) |
| `agent-manager-service/spec/` (generated), `cli/pkg/clients/amsvc/gen/{types,client}.gen.go` (generated) | Task 1 |
| `agent-manager-service/db_migrations/049_add_a2a_card_projection.go`, `050_add_a2a_publication_transports.go`, `migration_list.go` | Task 2 |
| `agent-manager-service/models/a2a_agent_card.go`, `models/a2a_publication.go` | New columns, `A2ACardProjection` (Tasks 2, 5) |
| `agent-manager-service/tests/a2a_catalog_migrations_test.go` | Migration scenarios 14, 59 (Task 2) |
| `agent-manager-service/services/a2a_card_projection.go` (+ `_unit_test.go`) | Fingerprint + search text (Task 3) |
| `agent-manager-service/repositories/a2a_agent_card_repository.go` (+ mock, test), `services/a2a_card_reconciler.go` (+ test) | Projection write/reset (Task 4) |
| `agent-manager-service/services/a2a_agent_deployment.go`, `a2a_publication_reconciler.go`, `repositories/a2a_publication_repository.go` (+ mock, tests) | Published transports (Task 5) |
| `agent-manager-service/services/a2a_publisher.go`, `a2a_proxy_service.go` (+ tests) | Gateway status helper (Task 6) |
| `agent-manager-service/models/a2a_catalog.go`, `repositories/a2a_catalog_repository.go` (+ mock, `_test.go`) | Catalogue reads (Task 7) |
| `agent-manager-service/services/a2a_card_summary.go` (+ test) | Summary fields (Task 8) |
| `agent-manager-service/clients/openchoreosvc/client/deployments.go`, `services/a2a_catalog_order.go` (+ test) | Pipeline order (Task 9) |
| `agent-manager-service/utils/errors.go`, `services/a2a_catalog_service.go` (+ test) | List / entry orchestration (Task 10) |
| `agent-manager-service/clients/openchoreosvc/client/components.go`, `models/a2a_connect.go`, `services/a2a_connect.go` (+ test) | Connect details (Task 11) |
| `agent-manager-service/controllers/a2a_catalog_controller.go` (+ test), `api/a2a_catalog_routes.go`, `api/app.go`, `api/audit_coverage_test.go`, `wiring/wire.go`, `wiring/params.go`, `wiring/wire_gen.go` | HTTP surface (Task 12) |

---

### Task 1: API contract and generated clients

**Files:**
- Modify: `agent-manager-service/docs/api_v1_openapi.yaml` (paths: insert after the `.../a2a-proxy/refresh` path block, before `.../agents/{agentName}/monitors`; schemas: insert after `A2AProxyUpstreamAuth`)
- Regenerate: `agent-manager-service/spec/` (whole directory), `cli/pkg/clients/amsvc/gen/types.gen.go`, `cli/pkg/clients/amsvc/gen/client.gen.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `spec.A2ACatalogListResponse{Agents []A2ACatalogAgent; Total, Limit, Offset int32}`, `spec.A2ACatalogAgent{ProjectName, AgentName, Kind, Name, Description, Version string; Skills []string; LeadEnvironment string; Environments []A2ACatalogEnvironment}`, `spec.A2ACatalogEnvironment{Name, Status string; FetchedAt *time.Time; Proxied, CardDiffers, Matched bool}`, `spec.A2AConnectResponse{Kind string; Proxied, IsProduction bool; GatewayUrl, CardUrl, JsonRpcUrl string; Security A2AConnectSecurity; Publication A2AConnectPublication}`, `spec.A2AConnectSecurity{Mode string; ApiKeyHeader *string; Oauth *A2AConnectOAuth}`, `spec.A2AConnectOAuth{Issuers []A2AConnectIssuer; Audiences []string; HeaderName, AuthHeaderPrefix string}`, `spec.A2AConnectIssuer{Name, Issuer string}`, `spec.A2AConnectPublication{Status, LastError string}`, with `New<Type>(required...)` constructors and `Set<Field>` setters for optionals (same shape as `spec.AgentCardResponse`). Confirm the exact generated field names in `spec/model_a2_a_*.go` after `make spec` and use those in Task 12.

- [ ] **Step 1: Add the three operations to the YAML**

Place them after the `/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/a2a-proxy/refresh` block (the `agentCardPathParams` anchor is defined earlier on the `agent-card` path, so `*agentCardPathParams` resolves). Every operation: `tags: [Agent Card]`, responses `401` and `500` with `ErrorResponse`, summaries/descriptions/operationIds verbatim from §3.1–§3.3.

```yaml
  /orgs/{orgName}/a2a-agents:
    get:
      tags:
        - Agent Card
      summary: List the organization's A2A agent catalogue
      description: Returns one entry per A2A agent in the organization, with the per-environment state of its agent card. Results are ordered by project and agent name.
      operationId: listA2AAgents
      parameters:
        - name: orgName
          in: path
          required: true
          description: Organization name/handle
          schema:
            type: string
        - name: q
          in: query
          required: false
          description: Case-insensitive substring match over agent names, card names and descriptions, and skill names, descriptions, tags and examples.
          schema:
            type: string
            maxLength: 255
        # kind (enum platform|external|proxied), environment, project, status (enum pending|fetched|failed),
        # limit (int32, default 50, min 1, max 100), offset (int32, default 0, min 0) — descriptions from §3.1
      responses:
        "200": { description: A page of the A2A agent catalogue, content: { application/json: { schema: { $ref: "#/components/schemas/A2ACatalogListResponse" } } } }
        "400": { description: Invalid query parameter, ... ErrorResponse }
        "401": ...
        "500": ...
```

`.../agents/{agentName}/a2a-catalog-entry` (own three path parameters per §3.2; 200 `A2ACatalogAgent`, 404 "The agent is not in the A2A catalogue") and `.../environments/{envID}/a2a-connect` (`parameters: *agentCardPathParams`; 200 `A2AConnectResponse`, 404 "Agent or environment not found, or the agent is not an A2A agent"). Use the expanded multi-line response style of the neighbouring `a2a-proxy` operations, not the flow style shown above.

- [ ] **Step 2: Add the eight schemas**

After `A2AProxyUpstreamAuth` in `components.schemas`, as `type: object` with `required` lists and property types/descriptions exactly as the §3.4 tables: `A2ACatalogListResponse`, `A2ACatalogAgent` (description "name, description, version and skills come from the lead environment's card; name falls back to agentName when no card has been fetched."; `leadEnvironment` description "The first environment in pipeline order with a fetched card; the summary and cardDiffers are relative to it."; `environments` description "Environments in pipeline order, lead first"), `A2ACatalogEnvironment` (`fetchedAt` `format: date-time`, not required), `A2AConnectResponse`, `A2AConnectSecurity` (`apiKeyHeader`, `oauth` not required), `A2AConnectOAuth`, `A2AConnectIssuer`, `A2AConnectPublication` (`status` enum `[pending, published, failed, none]`, description "none means a direct external agent, where the gateway isn't involved").

- [ ] **Step 3: Regenerate and build**

Run (from `agent-manager-service/`): `make spec && go build ./... && go vet ./spec/`
Expected: `spec/model_a2_a_catalog_list_response.go`, `..._catalog_agent.go`, `..._catalog_environment.go`, `..._connect_response.go`, `..._connect_security.go`, `..._connect_o_auth.go`, `..._connect_issuer.go`, `..._connect_publication.go` exist; `spec/api_agent_card.go` gains the three operations; build passes.

Run (from repo root): `make amctl-gen-client && cd cli && go build ./... && go test ./...`
Expected: only `cli/pkg/clients/amsvc/gen/types.gen.go` and `client.gen.go` change (`git status --short cli/`); `types.gen.go` contains `type A2ACatalogAgent struct`, `type ListA2AAgentsParams struct` with `Q`, `Kind`, `Environment`, `Project`, `Status`, `Limit`, `Offset`; `client.gen.go` contains `ListA2AAgentsWithResponse`, `GetA2ACatalogEntryWithResponse`, `GetA2AConnectWithResponse`; cli tests pass.

- [ ] **Step 4: Verify the service still passes its unit tier**

Run: `make test-unit`
Expected: PASS (no handler uses the new types yet).

- [ ] **Step 5: Commit**

```bash
git add agent-manager-service/docs/api_v1_openapi.yaml agent-manager-service/spec cli/pkg/clients/amsvc/gen/types.gen.go cli/pkg/clients/amsvc/gen/client.gen.go
git commit -m "Add the A2A catalogue and connect operations to the API contract" -m "Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 2: Migrations 049 and 050 and the new model columns

**Files:**
- Create: `agent-manager-service/db_migrations/049_add_a2a_card_projection.go`, `agent-manager-service/db_migrations/050_add_a2a_publication_transports.go`
- Modify: `agent-manager-service/db_migrations/migration_list.go` (`latestVersion = 50`, append `migration049`, `migration050`)
- Modify: `agent-manager-service/models/a2a_agent_card.go`, `agent-manager-service/models/a2a_publication.go`
- Test: `agent-manager-service/tests/a2a_catalog_migrations_test.go` (`//go:build integration`, `package tests`)

**Interfaces:**
- Consumes: `db_migrations` conventions (`migration{ID, Migrate}`, `runSQL(tx, ddl...)`, `db.Transaction`), `tests/gateway_runtime_url_migration_test.go` pattern (rolled-back tx via `db.DB(ctx).Begin()`).
- Produces: columns `a2a_agent_cards.card_fingerprint VARCHAR(64) NOT NULL DEFAULT ''`, `a2a_agent_cards.card_search_text TEXT NOT NULL DEFAULT ''`, `a2a_publications.published_transports JSONB NULL`; exported `dbmigrations.AddA2ACardProjectionColumns(tx *gorm.DB) error`, `dbmigrations.RequeueFetchedA2ACards(tx *gorm.DB) error`, `dbmigrations.AddA2APublicationTransportsColumn(tx *gorm.DB) error`; model fields `models.A2AAgentCard.CardFingerprint string` (`gorm:"column:card_fingerprint;not null;default:''"`), `models.A2AAgentCard.CardSearchText string` (`gorm:"column:card_search_text;not null;default:''"`), `models.A2APublication.PublishedTransports json.RawMessage` (`gorm:"column:published_transports;type:jsonb"`, nil when NULL); value type `models.A2ACardProjection{Fingerprint, SearchText string}`.

- [ ] **Step 1: Write the failing migration tests**

```go
// catalogMigrationTx is runtimeURLTestTx's shape: a transaction rolled back at test end.
// seedCard inserts an a2a_agent_cards row via tx.Create(&models.A2AAgentCard{...}) with the given status,
// card body (nil or []byte(`{"name":"x"}`)) and nextAttemptAt (nil = parked), unique ou_id per test; returns its ID.

func TestMigration049RequeuesOnlyFetchedCards(t *testing.T) {
	tx := catalogMigrationTx(t)
	now := time.Now()
	fetched := seedCard(t, tx, "fetched", []byte(`{"name":"x"}`), nil)
	failed := seedCard(t, tx, "failed", []byte(`{"name":"x"}`), nil)
	pending := seedCard(t, tx, "pending", nil, &now)
	parked := seedCard(t, tx, "pending", nil, nil)
	require.NoError(t, dbmigrations.RequeueFetchedA2ACards(tx))
	row := cardRow(t, tx, fetched)
	assert.Equal(t, "pending", string(row.Status)); assert.Equal(t, 0, row.AttemptCount)
	require.NotNil(t, row.NextAttemptAt); assert.False(t, row.NextAttemptAt.After(time.Now()))
	assert.JSONEq(t, `{"name":"x"}`, string(row.Card))
	for _, id := range []uuid.UUID{failed, pending, parked} { /* status, attempt_count, next_attempt_at unchanged */ }
}

func TestMigration049And050AreIdempotent(t *testing.T) {
	tx := catalogMigrationTx(t)
	require.NoError(t, dbmigrations.AddA2ACardProjectionColumns(tx))
	require.NoError(t, dbmigrations.AddA2ACardProjectionColumns(tx))
	require.NoError(t, dbmigrations.AddA2APublicationTransportsColumn(tx))
	require.NoError(t, dbmigrations.AddA2APublicationTransportsColumn(tx))
}

func TestMigration050LeavesExistingTransportsNull(t *testing.T) {
	// seed an a2a_publications row with tx.Create(&models.A2APublication{...}); read it back; assert PublishedTransports == nil
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go build -tags=integration ./...`
Expected: compile error — `dbmigrations.RequeueFetchedA2ACards` undefined, `CardFingerprint`/`PublishedTransports` undefined.

- [ ] **Step 3: Implement the migrations and model fields**

`049_add_a2a_card_projection.go`: `var migration049 = migration{ID: 49, Migrate: func(db) error { return db.Transaction(func(tx) error { AddA2ACardProjectionColumns(tx); RequeueFetchedA2ACards(tx) }) }}`. `AddA2ACardProjectionColumns` runs `ALTER TABLE a2a_agent_cards ADD COLUMN IF NOT EXISTS card_fingerprint VARCHAR(64) NOT NULL DEFAULT ''` and `... ADD COLUMN IF NOT EXISTS card_search_text TEXT NOT NULL DEFAULT ''`. `RequeueFetchedA2ACards` runs `UPDATE a2a_agent_cards SET status = 'pending', attempt_count = 0, next_attempt_at = NOW() WHERE status = 'fetched' AND card IS NOT NULL` (§5.6).

`050_add_a2a_publication_transports.go`: `var migration050 = migration{ID: 50, ...}` running `AddA2APublicationTransportsColumn`: `ALTER TABLE a2a_publications ADD COLUMN IF NOT EXISTS published_transports JSONB` (nullable, no default).

Model fields as in Interfaces; add `A2ACardProjection` to `models/a2a_agent_card.go` with a doc comment saying it is what the reconciler stores alongside the card.

- [ ] **Step 4: Run the integration tier**

Run: `make test-integration`
Expected: PASS, including the three new tests (the harness runs `go run . -migrate` first, so 049/050 apply to the test database).

- [ ] **Step 5: Commit**

```bash
git add agent-manager-service/db_migrations/049_add_a2a_card_projection.go agent-manager-service/db_migrations/050_add_a2a_publication_transports.go agent-manager-service/db_migrations/migration_list.go agent-manager-service/models/a2a_agent_card.go agent-manager-service/models/a2a_publication.go agent-manager-service/tests/a2a_catalog_migrations_test.go
git commit -m "Add the A2A card projection and published-transport columns" -m "Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 3: Card projection (fingerprint and search text)

**Files:**
- Create: `agent-manager-service/services/a2a_card_projection.go`
- Test: `agent-manager-service/services/a2a_card_projection_unit_test.go`

**Interfaces:**
- Consumes: `a2aCardHash(json.RawMessage) (string, error)` in `services/a2a_card_reconciler.go`; `models.A2ACardProjection` (Task 2).
- Produces: `func a2aCardProjection(card json.RawMessage) (models.A2ACardProjection, error)`.

- [ ] **Step 1: Write the failing tests (scenarios 1–7)**

```go
func TestA2ACardProjectionIgnoresNestedKeyOrder(t *testing.T)            // {"a":{"x":1,"y":2},"name":"n"} vs {"name":"n","a":{"y":2,"x":1}} → equal, non-empty, len 64
func TestA2ACardProjectionIgnoresInterfaceURLButNotBinding(t *testing.T) // url differs → equal; protocolBinding differs → differ
func TestA2ACardProjectionTracksSkillChanges(t *testing.T)              // inputModes change → differ; two skills swapped → differ
func TestA2ACardProjectionSearchTextIsPlainText(t *testing.T) {
	p, err := a2aCardProjection(json.RawMessage(`{"name":"Orders","description":"d","skills":[{"name":"s","description":"sd","tags":["refunds"],"examples":["say \"hi\" \\ now"]}]}`))
	require.NoError(t, err)
	assert.Equal(t, "Orders\nd\ns\nsd\nrefunds\nsay \"hi\" \\ now", p.SearchText)
	for _, bad := range []string{"[", "{", `\"`} { assert.NotContains(t, p.SearchText, bad) }
}
func TestA2ACardProjectionOfNilIsEmpty(t *testing.T)      // nil → {"", ""}, nil error
func TestA2ACardProjectionRejectsInvalidJSON(t *testing.T) // `{` → error
func TestA2ACardProjectionKeepsNumberPrecision(t *testing.T) // {"v":12345678901234567890123} twice → same; vs {"v":1.2345678901234568e+22} → differ
func TestA2ACardProjectionOfANonObjectUsesTheRawHash(t *testing.T) // `[1,2]` → fingerprint == a2aCardHash(`[1,2]`), SearchText ""
```

- [ ] **Step 2: Run to verify failure**

Run: `make test-run RUN=TestA2ACardProjection PKG=./services/`
Expected: FAIL — `a2aCardProjection` undefined.

- [ ] **Step 3: Implement `a2aCardProjection` in `services/a2a_card_projection.go`**

Decode with `json.NewDecoder(bytes.NewReader(card))` + `UseNumber()` into `interface{}`; `len(card) == 0` → zero value, nil error; decode error → `fmt.Errorf("failed to project agent card: %w", err)`. Not a `map[string]interface{}` → `{Fingerprint: a2aCardHash(card), SearchText: ""}`. Object: deep-copy-free approach is fine — for each `supportedInterfaces` element that is a `map[string]interface{}`, `delete(entry, "url")`; `json.Marshal` the object (sorted keys at every depth), SHA-256 → lowercase hex (reuse `a2aCardHash` on the marshalled bytes). Search text per §5.3: `name`, `description`, then per skill (only when `skills` is `[]interface{}` whose every element is a `map[string]interface{}`) `name`, `description`, each string in `tags`, each string in `examples`; join non-empty values with `"\n"`. Only `string` values are taken (never `json.Number`, bools, nested values).

- [ ] **Step 4: Run to verify pass**

Run: `make test-run RUN=TestA2ACardProjection PKG=./services/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent-manager-service/services/a2a_card_projection.go agent-manager-service/services/a2a_card_projection_unit_test.go
git commit -m "Project A2A cards into a fingerprint and search text" -m "Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 4: Store the projection on fetch, reset it on clear

**Files:**
- Modify: `agent-manager-service/repositories/a2a_agent_card_repository.go` (`MarkFetched` signature + body, `ClearCard`, `newSourceURLAssignments`)
- Regenerate: `agent-manager-service/repositories/repomocks/a2a_agent_card_repository_mock.go`
- Modify: `agent-manager-service/services/a2a_card_reconciler.go` (`fetchOne`)
- Modify tests: `agent-manager-service/services/a2a_card_reconciler_unit_test.go` (all `MarkFetchedFunc` literals gain the trailing `models.A2ACardProjection` parameter), `agent-manager-service/repositories/a2a_agent_card_repository_test.go` (all `MarkFetched` calls gain a trailing projection argument)

**Interfaces:**
- Consumes: `a2aCardProjection` (Task 3); `models.A2ACardProjection`, `CardFingerprint`, `CardSearchText` (Task 2).
- Produces: `A2AAgentCardRepository.MarkFetched(ctx context.Context, read models.A2AAgentCard, card json.RawMessage, cardHash, fetchedURL, releaseName string, projection models.A2ACardProjection) error` — always writes `card_fingerprint` and `card_search_text`.

- [ ] **Step 1: Write the failing tests (scenarios 10–13)**

Unit (`a2a_card_reconciler_unit_test.go`). Scenario 10 says "a card whose projection fails"; `a2aCardHash` and `a2aCardProjection` decode the same bytes the same way, so the only body that fails projection is invalid JSON, which fails the hash first on the same path. Pin that path and the stored value:
```go
func TestCardReconcilerChargesAnAttemptWhenTheCardCannotBeProjected(t *testing.T) {
	h := newCardReconcilerHarness("http://trip-planner.example")
	h.fetcher.FetchFunc = func(context.Context, string, bool) (json.RawMessage, error) { return json.RawMessage(`{`), nil }
	h.svc.fetchOne(context.Background(), pendingCard(models.A2AAgentCardSourcePlatform))
	assert.Empty(t, h.repo.MarkFetchedCalls())
	require.Len(t, h.repo.MarkAttemptFailedCalls(), 1)
}
func TestCardReconcilerStoresTheProjectionWithTheCard(t *testing.T) {
	h := newCardReconcilerHarness("http://trip-planner.example")
	h.svc.fetchOne(context.Background(), pendingCard(models.A2AAgentCardSourcePlatform))
	want, _ := a2aCardProjection(json.RawMessage(validTestCard))
	require.Len(t, h.repo.MarkFetchedCalls(), 1)
	assert.Equal(t, want, h.repo.MarkFetchedCalls()[0].Projection)
	assert.NotEmpty(t, want.Fingerprint)
}
```

Integration (`a2a_agent_card_repository_test.go`):
```go
func TestA2AAgentCardMarkFetchedAlwaysWritesTheProjection(t *testing.T) // mark fetched hash H + P1; Requeue; dueCardFor; MarkFetched same H + P2 → cardRowOf has P2 fingerprint/search text and the original card body
func TestA2AAgentCardClearCardResetsTheProjection(t *testing.T)        // non-empty projection; ClearCard → both ""
func TestA2AAgentCardReenqueueKeepsOrResetsTheProjection(t *testing.T) // external row: reenqueueExternal same URL → kept; different URL → both "" and Card nil
```

- [ ] **Step 2: Verify compilation fails**

Run: `go build ./... && go build -tags=integration ./...`
Expected: FAIL — `MarkFetched` arity.

- [ ] **Step 3: Implement**

Repository: `MarkFetched` adds `"card_fingerprint": projection.Fingerprint, "card_search_text": projection.SearchText` to `updates` unconditionally (outside the `cardHash != read.CardHash` guard). `ClearCard` adds `"card_fingerprint": ""`, `"card_search_text": ""`. `newSourceURLAssignments` appends `keepUnlessChanged("card_fingerprint", "''")` and `keepUnlessChanged("card_search_text", "''")`. Update the interface doc comment. Reconciler `fetchOne`: after `hash` succeeds, `projection, err := a2aCardProjection(fetched.body)`; on error `s.recordAttemptFailure(ctx, row, fetched.releaseName, err)`; pass `projection` to `MarkFetched`.

Run: `make mocks PKG=./repositories/` — expected: only `repomocks/a2a_agent_card_repository_mock.go` changes.

- [ ] **Step 4: Run tests**

Run: `make test-run RUN=TestCardReconciler PKG=./services/ && make test-integration`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent-manager-service/repositories/a2a_agent_card_repository.go agent-manager-service/repositories/repomocks/a2a_agent_card_repository_mock.go agent-manager-service/repositories/a2a_agent_card_repository_test.go agent-manager-service/services/a2a_card_reconciler.go agent-manager-service/services/a2a_card_reconciler_unit_test.go
git commit -m "Store the card projection when an A2A card is fetched" -m "Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 5: Record the published transports

**Files:**
- Modify: `agent-manager-service/services/a2a_agent_deployment.go` (extract the defaults), `agent-manager-service/services/a2a_publication_reconciler.go` (`a2aPublishResult`, `attemptPublish`, `publishOne`), `agent-manager-service/repositories/a2a_publication_repository.go` (`MarkPublished`)
- Regenerate: `agent-manager-service/repositories/repomocks/a2a_publication_repository_mock.go`
- Modify tests: `agent-manager-service/services/a2a_publication_reconciler_test.go` (every `MarkPublishedFunc` literal gains `json.RawMessage`), `agent-manager-service/repositories/a2a_publication_repository_test.go` (every `MarkPublished` call gains a trailing argument), `agent-manager-service/services/a2a_agent_deployment_test.go` if it asserts the defaults

**Interfaces:**
- Consumes: `A2ATransport`, `a2aTransportJSONRPC`, `a2aTransportHTTPJSON`, `a2aPathPrefixJSONRPC`, `a2aPathPrefixHTTPRPC`; `models.A2APublication.PublishedTransports` (Task 2).
- Produces: `func a2aEffectiveTransports(transports []A2ATransport) []A2ATransport` (nil → `[{JSONRPC,"/rpc"},{HTTP+JSON,"/rest"}]`, otherwise unchanged); `a2aPublishResult.transports []A2ATransport`; `A2APublicationRepository.MarkPublished(ctx context.Context, read models.A2APublication, upstreamURL string, transports json.RawMessage) error` storing `published_transports`.

- [ ] **Step 1: Write the failing tests (scenarios 56–58)**

```go
func TestReconcilerRecordsThePlatformTransports(t *testing.T) {
	h := newA2AReconcilerHarness("http://trip-planner.dp-default:9099")
	h.svc.publishOne(context.Background(), pendingPublication())
	marked := h.pubRepo.MarkPublishedCalls(); require.Len(t, marked, 1)
	assert.JSONEq(t, `[{"protocolBinding":"JSONRPC","pathPrefix":"/rpc"},{"protocolBinding":"HTTP+JSON","pathPrefix":"/rest"}]`, string(marked[0].Transports))
}
func TestReconcilerRecordsTheOriginsTransports(t *testing.T) // withExternalOrigin + a card offering only HTTP+JSON at <origin>/rest → `[{"protocolBinding":"HTTP+JSON","pathPrefix":"/rest"}]`; a card with JSONRPC at the origin root → `[{"protocolBinding":"JSONRPC"}]`
```
Integration:
```go
func TestA2APublicationMarkPublishedStoresTheTransports(t *testing.T) // MarkPublished(..., json.RawMessage(`[{"protocolBinding":"JSONRPC","pathPrefix":"/rpc"}]`)) → Get returns JSONEq; MarkPublished(..., nil) → Get returns nil PublishedTransports
```

- [ ] **Step 2: Verify compilation fails**

Run: `go build ./... && go build -tags=integration ./...`
Expected: FAIL — `MarkPublished` arity / `Transports` field.

- [ ] **Step 3: Implement**

`a2aEffectiveTransports` replaces the inline `if transports == nil { ... }` block in `buildA2AAgentDeploymentYAML`. In `attemptPublish`, compute `transports := a2aEffectiveTransports(upstream.transports)` once, pass it as `Transports` to the YAML input and return it in `a2aPublishResult{upstreamURL, gatewayID, transports}`. In `publishOne`, `json.Marshal(published.transports)` (a marshal error is recorded through `recordAttemptFailure` like any other failure) and pass the bytes to `MarkPublished`. Repository: add `"published_transports"` to the update map — the `transports` value when `len(transports) > 0`, otherwise an untyped `nil`, exactly as `ClearCard` writes `"card": nil` for the card column.

Run: `make mocks PKG=./repositories/` — expected: only `a2a_publication_repository_mock.go` changes.

- [ ] **Step 4: Run tests**

Run: `make test-run RUN='TestReconciler|TestA2AAgentDeployment' PKG=./services/ && make test-integration`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent-manager-service/services/a2a_agent_deployment.go agent-manager-service/services/a2a_publication_reconciler.go agent-manager-service/services/a2a_publication_reconciler_test.go agent-manager-service/repositories/a2a_publication_repository.go agent-manager-service/repositories/repomocks/a2a_publication_repository_mock.go agent-manager-service/repositories/a2a_publication_repository_test.go
git commit -m "Record the transports an A2A publication published" -m "Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 6: Gateway-status helper on the publisher

**Files:**
- Modify: `agent-manager-service/services/a2a_publisher.go`, `agent-manager-service/services/a2a_proxy_service.go` (`GetA2AProxy`; delete `a2aProxyNoPublicationMsg`)
- Test: `agent-manager-service/services/a2a_publisher_unit_test.go` (new tests), `agent-manager-service/services/a2a_proxy_service_unit_test.go` (scenario 60 — extend `TestGetA2AProxyWithoutAPublicationReportsFailed` to assert the exact message)

**Interfaces:**
- Consumes: `A2APublisher.GatewayURL`, `A2APublisher.Publication`, `repositories.ErrA2APublicationNotFound`, `A2ATransport`, `models.A2APublication.PublishedTransports`.
- Produces:
  ```go
  // A2AGatewayStatus is what the gateway currently does for an agent in one environment.
  type A2AGatewayStatus struct {
      GatewayURL string                      // "" while no ingress gateway is mapped
      Status     models.A2APublicationStatus
      LastError  string
      Transports []A2ATransport              // nil when the row's stored transports are NULL
  }
  func (p *A2APublisher) GatewayStatus(ctx context.Context, ouID, projectName, agentName string, envUUID uuid.UUID, source models.A2APublicationSource) (A2AGatewayStatus, error)
  var a2aNoPublicationMsg = map[models.A2APublicationSource]string{ external: "no gateway publication is queued for this environment; refresh to publish it again", platform: "no gateway publication is queued for this environment; redeploy the agent or save its deploy settings to publish it again" }
  ```

- [ ] **Step 1: Write the failing tests (scenarios 50 message table, 55, 60)**

```go
func TestGatewayStatusReportsAMissingPublicationPerSource(t *testing.T) // table: external → failed + "…refresh to publish it again"; platform → failed + "…redeploy the agent or save its deploy settings to publish it again"
func TestGatewayStatusCopiesTheRow(t *testing.T)                        // row {published, "", transports `[{"protocolBinding":"JSONRPC","pathPrefix":""}]`} → Status published, Transports [{JSONRPC,""}]; NULL transports → nil
func TestGatewayStatusFailsOnUnparseableTransports(t *testing.T)        // PublishedTransports `{` → err != nil, not ErrA2APublicationNotFound
func TestGatewayStatusFailsWhenTheGatewayLookupFails(t *testing.T)      // gateways.ListWithFiltersFunc returns assert.AnError → errors.Is
func TestGatewayStatusWrapsPublicationReadErrors(t *testing.T)          // pubs.GetFunc returns assert.AnError → errors.Is, message prefix "failed to read the A2A publication"
```
Build the publisher with `newPublisherMocks(t).publisher()`; set `m.gateways.ListWithFiltersFunc` to return `[]*models.Gateway{{Vhost: "agents.example.com", GatewayFunctionalityType: models.GatewayRoleIngress}}` and `m.pubs.GetFunc` per case.

- [ ] **Step 2: Run to verify failure**

Run: `make test-run RUN=TestGatewayStatus PKG=./services/`
Expected: FAIL — `GatewayStatus` undefined.

- [ ] **Step 3: Implement**

`GatewayStatus`: gateway URL first (error fails); then `p.Publication(...)`; `ErrA2APublicationNotFound` → `{GatewayURL, Failed, a2aNoPublicationMsg[source], nil}`; other error → `fmt.Errorf("failed to read the A2A publication: %w", err)`; else copy `Status`, `LastError`; if `len(pub.PublishedTransports) > 0`, `json.Unmarshal` into `[]A2ATransport`, error → `fmt.Errorf("failed to read the published A2A transports: %w", err)`. `GetA2AProxy` replaces its `GatewayURL` + `Publication` reads with one `GatewayStatus(..., models.A2APublicationSourceExternal)` call and copies `GatewayURL`, `Status`, `LastError` into `models.A2AProxyStatus`.

- [ ] **Step 4: Run tests**

Run: `make test-run RUN='TestGatewayStatus|TestGetA2AProxy' PKG=./services/`
Expected: PASS, including the unchanged proxy-service expectations.

- [ ] **Step 5: Commit**

```bash
git add agent-manager-service/services/a2a_publisher.go agent-manager-service/services/a2a_publisher_unit_test.go agent-manager-service/services/a2a_proxy_service.go agent-manager-service/services/a2a_proxy_service_unit_test.go
git commit -m "Read an A2A agent's gateway status through the publisher" -m "Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 7: Catalogue repository

**Files:**
- Create: `agent-manager-service/models/a2a_catalog.go`, `agent-manager-service/repositories/a2a_catalog_repository.go`
- Generate: `agent-manager-service/repositories/repomocks/a2a_catalog_repository_mock.go`
- Test: `agent-manager-service/repositories/a2a_catalog_repository_test.go` (`//go:build integration`)

**Interfaces:**
- Consumes: `escapeLikePattern` (`repositories/custom_evaluator_repository.go`), `models.A2AAgentCard` with `CardSearchText`, `a2a_proxies`; test helpers `newTestCard`, `inEnv`, `envUUIDOf`, `cleanupCard`, `newTestProxy`, `cleanupProxy` from the sibling test files.
- Produces (`models/a2a_catalog.go`):
  ```go
  type A2ACatalogKind string
  const ( A2ACatalogKindPlatform A2ACatalogKind = "platform"; A2ACatalogKindExternal A2ACatalogKind = "external"; A2ACatalogKindProxied A2ACatalogKind = "proxied" )
  // A2ACatalogFilter is one catalogue page request; zero fields mean "no filter"; Limit 0 means no limit.
  type A2ACatalogFilter struct { OUID, Query, Project, Environment string; Kind A2ACatalogKind; Status A2AAgentCardStatus; Limit, Offset int }
  // A2ACatalogRow is one card row with its catalogue flags.
  type A2ACatalogRow struct { A2AAgentCard `gorm:"embedded"`; Proxied bool `gorm:"column:proxied"`; Matched bool `gorm:"column:matched"` }
  ```
  (`repositories/a2a_catalog_repository.go`):
  ```go
  //go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/a2a_catalog_repository_mock.go . A2ACatalogRepository:A2ACatalogRepositoryMock
  type A2ACatalogRepository interface {
      // List returns every environment row of the agents on the page, ordered by project, agent, environment name, and the number of matching agents.
      List(ctx context.Context, filter models.A2ACatalogFilter) ([]models.A2ACatalogRow, int, error)
      // ListForAgent returns the agent's rows in environment-name order, Matched false; no rows is not an error.
      ListForAgent(ctx context.Context, ouID, projectName, agentName string) ([]models.A2ACatalogRow, error)
  }
  func NewA2ACatalogRepository(db *gorm.DB) A2ACatalogRepository
  ```

- [ ] **Step 1: Write the failing tests (scenarios 15–26 + review item 2)**

One `seedCatalogCard(t, repo cardRepo, ou, project, agent, env string, source, status, searchText, card)` helper that `Enqueue`s a card row (then, when `status == fetched`, claims it via `dueCardFor` and `MarkFetched`s with the search text as the projection); per-test unique `ou` via `"ou-" + uuid.New().String()[:8]`.

```go
func TestA2ACatalogListPagesByAgent(t *testing.T)                 // a(dev,prod), b(dev), c(dev); limit 2 → total 3, rows a/dev,a/prod,b/dev; limit 2 offset 2 → total 3, rows c/dev
func TestA2ACatalogListPastTheEndKeepsTheTotal(t *testing.T)      // 1 agent; limit 2 offset 4 → rows empty, total 1
func TestA2ACatalogListPastTheEndCountsOnlyMatches(t *testing.T)  // review #2: agents x (text "refund") and y; q=refund limit 2 offset 4 → total 1
func TestA2ACatalogListSeparatesProjects(t *testing.T)            // orders in p1 and p2 → total 2
func TestA2ACatalogListForAgent(t *testing.T)                     // external orders dev,prod + proxy in prod + platform orders in another project → 2 rows [dev false, prod true]; unknown agent → 0 rows, nil error
func TestA2ACatalogListFindsAPendingAgentByName(t *testing.T)     // orders-bot NULL card, "" text; q=orders → returned, Matched true
func TestA2ACatalogListMarksTheMatchingRows(t *testing.T)         // a: dev text without "refund", prod with; other agent without → total 1, dev false, prod true
func TestA2ACatalogListNeverMatchesJSONSyntax(t *testing.T)       // text "s\nsay \"hi\"" → q=[ and q=" not returned; q=say "hi" returned
func TestA2ACatalogListEscapesWildcards(t *testing.T)             // texts "100% uptime bot", "Ledger" → q=% returns only uptime; q=_ nothing
func TestA2ACatalogListLeavesPendingAgentsOutOfSearch(t *testing.T) // NULL card, "" text: q=x → absent; no q → present, Matched false
func TestA2ACatalogListFiltersByKindAndEnvironment(t *testing.T)  // ext dev,prod + proxy prod: kind=proxied → total 1, rows [false,true]; kind=platform → 0; environment=prod → total 1, both rows
func TestA2ACatalogListIsScopedToTheOrg(t *testing.T)             // rows in two ous → only the caller's rows and count
func TestA2ACatalogListEvaluatesFiltersPerRow(t *testing.T)       // prod pending, dev fetched; status=fetched&environment=prod → absent
```

- [ ] **Step 2: Verify compilation fails**

Run: `go build -tags=integration ./repositories/`
Expected: FAIL — `NewA2ACatalogRepository` undefined.

- [ ] **Step 3: Implement**

Build the row predicate once (`[]string` of SQL fragments + args) from the filter: `c.ou_id = ?`; optional `c.project_name = ?`, `c.environment_name = ?`, `c.status = ?`; kind `platform`/`external` → `c.source = ?`; `proxied` → `EXISTS (SELECT 1 FROM a2a_proxies p WHERE p.ou_id = c.ou_id AND p.project_name = c.project_name AND p.agent_name = c.agent_name AND p.environment_uuid = c.environment_uuid)`; non-empty `Query` → `(c.agent_name ILIKE ? ESCAPE '\' OR c.card_search_text ILIKE ? ESCAPE '\')` with pattern `"%" + escapeLikePattern(q) + "%"` bound twice. Two raw queries:

1. `SELECT COUNT(*) FROM (SELECT 1 FROM a2a_agent_cards c WHERE <pred> GROUP BY c.project_name, c.agent_name) m` → total.
2. `WITH keys AS (SELECT c.project_name, c.agent_name FROM a2a_agent_cards c WHERE <pred> GROUP BY c.project_name, c.agent_name ORDER BY c.project_name, c.agent_name [LIMIT ?] OFFSET ?) SELECT c.*, <proxied EXISTS> AS proxied, <matched> AS matched FROM a2a_agent_cards c JOIN keys k ON k.project_name = c.project_name AND k.agent_name = c.agent_name WHERE c.ou_id = ? ORDER BY c.project_name, c.agent_name, c.environment_name` → rows, where `<matched>` is `COALESCE(<the same ILIKE predicate>, false)` when `Query != ""` and `false` otherwise; omit `LIMIT` when `Limit == 0`.

`ListForAgent`: same `SELECT c.*, <proxied>, false AS matched ... WHERE c.ou_id = ? AND c.project_name = ? AND c.agent_name = ? ORDER BY c.environment_name`. Scan with `r.db.WithContext(ctx).Raw(sql, args...).Scan(&rows)`; return `[]models.A2ACatalogRow{}` (non-nil) when empty. Wrap nothing (the service wraps).

Run: `make mocks PKG=./repositories/` → `repomocks/a2a_catalog_repository_mock.go` created.

- [ ] **Step 4: Run tests**

Run: `make test-integration`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent-manager-service/models/a2a_catalog.go agent-manager-service/repositories/a2a_catalog_repository.go agent-manager-service/repositories/repomocks/a2a_catalog_repository_mock.go agent-manager-service/repositories/a2a_catalog_repository_test.go
git commit -m "Add the A2A catalogue repository" -m "Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 8: Card summary

**Files:**
- Create: `agent-manager-service/services/a2a_card_summary.go`
- Test: `agent-manager-service/services/a2a_card_summary_unit_test.go`

**Interfaces:**
- Produces: `type a2aCardSummary struct { Name, Description, Version string; Skills []string }` and `func a2aCardSummaryOf(card json.RawMessage) a2aCardSummary` — never fails; `Skills` is always non-nil.

- [ ] **Step 1: Write the failing tests (scenarios 8, 9)**

```go
func TestA2ACardSummaryReadsTheTopLevelFields(t *testing.T) // {"name":"Order Support","description":"Refunds","version":"2.1.0","skills":[{"name":"issue-refund"},{"name":"order-status"}]} → all four fields
func TestA2ACardSummaryOfUnusableInputIsZero(t *testing.T)  // table: nil, `null`, `{"name":5,"skills":"x"}`, `{` → {"", "", "", []string{}}
func TestA2ACardSummarySkipsUnusableSkills(t *testing.T)    // {"skills":[1,{"name":""},{"name":"ok"}]} → ["ok"]
```

- [ ] **Step 2: Run to verify failure**

Run: `make test-run RUN=TestA2ACardSummary PKG=./services/`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `a2aCardSummaryOf`**

`json.Unmarshal` into `map[string]json.RawMessage`; on error or empty input return the zero summary with `Skills: []string{}`. Use the existing `jsonString` (`services/a2a_proxy_routes.go`) for `name`, `description`, `version`; `skills` → unmarshal into `[]json.RawMessage`, each element into `map[string]json.RawMessage`, keep non-empty `name` strings.

- [ ] **Step 4: Run to verify pass**

Run: `make test-run RUN=TestA2ACardSummary PKG=./services/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent-manager-service/services/a2a_card_summary.go agent-manager-service/services/a2a_card_summary_unit_test.go
git commit -m "Summarise an A2A card for the catalogue" -m "Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 9: Environment order from the deployment pipeline

**Files:**
- Modify: `agent-manager-service/clients/openchoreosvc/client/deployments.go` (export `buildEnvironmentOrder` → `BuildEnvironmentOrder`; update its two callers in `GetDeployments` and `FindFirstEnvironment`), `agent-manager-service/clients/openchoreosvc/client/deployments_test.go` if it names the helper
- Create: `agent-manager-service/services/a2a_catalog_order.go`
- Test: `agent-manager-service/services/a2a_catalog_order_unit_test.go`

**Interfaces:**
- Consumes: `client.OpenChoreoClient.GetProjectDeploymentPipeline(ctx, ouID, project) (*models.DeploymentPipelineResponse, error)`, `utils.ErrProjectNotFound`, `utils.ErrDeploymentPipelineNotFound`, `utils.ErrNotFound`, `golang.org/x/sync/errgroup` (already a dependency), `models.A2ACatalogRow` (Task 7).
- Produces:
  ```go
  func BuildEnvironmentOrder(promotionPaths []models.PromotionPath) []string   // package client, same body as before
  const a2aCatalogPipelineTimeout = 5 * time.Second
  const a2aCatalogPipelineConcurrency = 8
  // a2aEnvironmentOrders fetches each project's pipeline order at most once, concurrently, under one deadline.
  // Not-found pipelines (ErrProjectNotFound, ErrDeploymentPipelineNotFound, ErrNotFound) and nil pipelines give a nil order; any other error fails as "order a2a catalogue environments: %w".
  func a2aEnvironmentOrders(ctx context.Context, oc client.OpenChoreoClient, logger *slog.Logger, ouID string, projects []string, timeout time.Duration) (map[string][]string, error)
  // sortA2ACatalogRows orders rows by pipeline position, then unknown environments by name; nil order means name order. Stable.
  func sortA2ACatalogRows(rows []models.A2ACatalogRow, order []string)
  ```

- [ ] **Step 1: Write the failing tests (scenarios 33–38, review item 3)**

```go
func TestSortA2ACatalogRowsUsesPipelineThenName(t *testing.T)       // zeta, alpha, prod, dev with [dev, prod] → [dev, prod, alpha, zeta]; nil order → [alpha, dev, prod, zeta]
func TestSortA2ACatalogRowsSkipsPipelineNamesWithoutRows(t *testing.T) // rows dev, prod; order [dev, staging, prod] → [dev, prod]; rows a, b; order [x, y] → [a, b]
func TestA2AEnvironmentOrdersFallBackOnNotFound(t *testing.T)       // table over the three sentinels (wrapped with %w) and (nil, nil) → nil order, nil error
func TestA2AEnvironmentOrdersFailOnOtherErrors(t *testing.T)        // errors.New("503") → err wraps it; errors.Is(err, cause); message has prefix "order a2a catalogue environments: "
func TestA2AEnvironmentOrdersTimeOut(t *testing.T)                  // lookup blocks on <-ctx.Done() and returns ctx.Err(); timeout 50ms → errors.Is(err, context.DeadlineExceeded); elapsed < 50ms + 1s
func TestA2AEnvironmentOrdersFetchEachProjectOnce(t *testing.T)     // projects [p1, p1, p2] → GetProjectDeploymentPipelineCalls() len 2; second call → 2 more
func TestA2AEnvironmentOrdersReadPromotionPaths(t *testing.T)       // dev→staging→prod paths → ["dev","staging","prod"]
```
Use `clientmocks.OpenChoreoClientMock{GetProjectDeploymentPipelineFunc: ...}` and `discardLogger()`.

- [ ] **Step 2: Run to verify failure**

Run: `make test-run RUN='TestSortA2ACatalogRows|TestA2AEnvironmentOrders' PKG=./services/`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

Export the client helper (rename only). `a2aEnvironmentOrders`: dedupe projects preserving order; `ctx, cancel := context.WithTimeout(ctx, timeout)`; `g, gctx := errgroup.WithContext(ctx)`; `g.SetLimit(a2aCatalogPipelineConcurrency)`; results in a pre-sized `[][]string` indexed by project position (no lock needed); per project: call the client with `gctx`; on an ignorable sentinel log at Info (`"A2A catalogue: project has no deployment pipeline; using name order"`, with `ouID`, `project`) and leave nil; on nil pipeline leave nil; else `client.BuildEnvironmentOrder(pipeline.PromotionPaths)`; other error → `return fmt.Errorf("order a2a catalogue environments: %w", err)`. `g.Wait()` error returns as is. `sortA2ACatalogRows`: `sort.SliceStable` with rank = index in order (map name→index) else `len(order)`; ties by `EnvironmentName`.

- [ ] **Step 4: Run tests**

Run: `make test-run RUN='TestSortA2ACatalogRows|TestA2AEnvironmentOrders' PKG=./services/ && go test ./clients/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent-manager-service/clients/openchoreosvc/client/deployments.go agent-manager-service/services/a2a_catalog_order.go agent-manager-service/services/a2a_catalog_order_unit_test.go
git commit -m "Order A2A catalogue environments by the project pipeline" -m "Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```
(Also stage `clients/openchoreosvc/client/deployments_test.go` if it changed.)

---

### Task 10: Catalogue service — list and single entry

**Files:**
- Modify: `agent-manager-service/utils/errors.go` (add `ErrAgentNotInA2ACatalog` next to `ErrAgentCardNotFound`), `agent-manager-service/models/a2a_catalog.go` (entry types)
- Create: `agent-manager-service/services/a2a_catalog_service.go`
- Test: `agent-manager-service/services/a2a_catalog_service_unit_test.go`

**Interfaces:**
- Consumes: `repositories.A2ACatalogRepository` (Task 7), `a2aCardSummaryOf` (Task 8), `a2aEnvironmentOrders`, `sortA2ACatalogRows`, `a2aCatalogPipelineTimeout` (Task 9), `A2AAgentCardService`, `repositories.A2AProxyRepository`, `*A2APublisher`, `repositories.AgentConfigRepository`, `repositories.GatewayRepository`, `client.OpenChoreoClient`, `*slog.Logger` (the last six are stored now and used by Task 11).
- Produces:
  ```go
  // utils
  ErrAgentNotInA2ACatalog = errors.New("agent is not in the A2A catalogue")
  // models/a2a_catalog.go
  type A2ACatalogEnvironment struct { Name string; Status A2AAgentCardStatus; FetchedAt *time.Time; Proxied, CardDiffers, Matched bool }
  type A2ACatalogEntry struct { ProjectName, AgentName string; Kind A2AAgentCardSource; Name, Description, Version string; Skills []string; LeadEnvironment string; Environments []A2ACatalogEnvironment }
  type A2ACatalogPage struct { Entries []A2ACatalogEntry; Total, Limit, Offset int }
  // services/a2a_catalog_service.go
  type A2ACatalogService interface {
      ListA2ACatalog(ctx context.Context, filter models.A2ACatalogFilter) (*models.A2ACatalogPage, error)
      GetA2ACatalogEntry(ctx context.Context, ouID, projectName, agentName string) (*models.A2ACatalogEntry, error)
      GetA2AConnectDetails(ctx context.Context, ouID, projectName, agentName, envName string) (*models.A2AConnectDetails, error) // added in Task 11
  }
  type a2aCatalogService struct { catalogRepo repositories.A2ACatalogRepository; cardService A2AAgentCardService; proxyRepo repositories.A2AProxyRepository; publisher *A2APublisher; agentConfigRepo repositories.AgentConfigRepository; gatewayRepo repositories.GatewayRepository; ocClient client.OpenChoreoClient; logger *slog.Logger; pipelineTimeout time.Duration }
  func NewA2ACatalogService(catalogRepo repositories.A2ACatalogRepository, cardService A2AAgentCardService, proxyRepo repositories.A2AProxyRepository, publisher *A2APublisher, agentConfigRepo repositories.AgentConfigRepository, gatewayRepo repositories.GatewayRepository, ocClient client.OpenChoreoClient, logger *slog.Logger) A2ACatalogService  // pipelineTimeout = a2aCatalogPipelineTimeout
  func buildA2ACatalogEntry(rows []models.A2ACatalogRow, order []string) models.A2ACatalogEntry // rows non-empty, all one agent
  ```
  Until Task 11, declare the interface without `GetA2AConnectDetails`.

- [ ] **Step 1: Write the failing tests (scenarios 27–32, 36–42)**

Harness: `newCatalogHarness(t)` holding `catalog *repomocks.A2ACatalogRepositoryMock`, `oc *clientmocks.OpenChoreoClientMock` (default `GetProjectDeploymentPipelineFunc` returning dev→staging→prod), and `svc *a2aCatalogService` built with `NewA2ACatalogService(catalog, nil, nil, nil, nil, nil, oc, discardLogger()).(*a2aCatalogService)`; a `row(project, agent, env string, card string, fingerprint string)` helper returning `models.A2ACatalogRow`.

```go
func TestListA2ACatalogOrdersEnvironmentsAndSummarisesTheLead(t *testing.T) // rows prod, dev, staging (platform; dev card {"name":"Trip","description":"d","version":"1","skills":[{"name":"plan"}]}) → envs [dev,staging,prod], Lead "dev", Name "Trip", Description "d", Version "1", Skills ["plan"], Kind platform
func TestListA2ACatalogFlagsTheDifferingEnvironment(t *testing.T)            // same bodies except interface URLs, prod extra skill; fingerprints dev==staging!=prod → only prod CardDiffers
func TestListA2ACatalogLeadIsTheFirstEnvironmentWithACard(t *testing.T)      // #13: dev no card, staging [one] fp "a", prod [one,two] fp "b" → Lead staging, Skills [one], prod true, staging/dev false
func TestListA2ACatalogWithoutAnyCard(t *testing.T)                           // Lead = first row, Name = agentName, Description "", Skills []string{} (not nil), nothing differs
func TestListA2ACatalogUsesTheStoredFingerprint(t *testing.T)                 // identical bodies, fingerprints "a","b" → second flagged
func TestListA2ACatalogNeverFlagsAnEmptyFingerprint(t *testing.T)             // "" next to "x" → neither flagged
func TestListA2ACatalogFailsOnPipelineErrors(t *testing.T)                    // oc returns errors.New("503") → err wraps it; assert.NotErrorIs(err, utils.ErrAgentNotInA2ACatalog); same for GetA2ACatalogEntry
func TestListA2ACatalogHonoursTheTimeout(t *testing.T)                        // svc.pipelineTimeout = 50ms, oc blocks on ctx → errors.Is(err, context.DeadlineExceeded) within 50ms+1s
func TestListA2ACatalogFetchesThePipelineOncePerProject(t *testing.T)         // p1 two agents + p2 → 2 calls; second request → 4; not-found + two agents in one project → 1
func TestListA2ACatalogEchoesTheRepositoryFlagsAndPaging(t *testing.T)        // repo: row proxied+matched, total 42; filter limit 10 offset 20 → Total 42, Limit 10, Offset 20, env Proxied && Matched
func TestListA2ACatalogWrapsRepositoryErrors(t *testing.T)                    // repo List → assert.AnError → errors.Is; GetA2ACatalogEntry too, and NotErrorIs ErrAgentNotInA2ACatalog
func TestGetA2ACatalogEntryNotInCatalogue(t *testing.T)                       // ListForAgent → [] → errors.Is(err, utils.ErrAgentNotInA2ACatalog); pipeline not called
func TestGetA2ACatalogEntryUsesOnlyThatProject(t *testing.T)                  // repo returns rows for project "p1" only (as ListForAgent is keyed by project); pipeline fetched for "p1"; order applied
```

- [ ] **Step 2: Run to verify failure**

Run: `make test-run RUN='TestListA2ACatalog|TestGetA2ACatalogEntry' PKG=./services/`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

`ListA2ACatalog`: `rows, total, err := catalogRepo.List` → error `fmt.Errorf("list a2a catalogue agents: %w", err)`; group by `(ProjectName, AgentName)` preserving first-seen order; `orders, err := a2aEnvironmentOrders(ctx, s.ocClient, s.logger, filter.OUID, distinctProjects, s.pipelineTimeout)`; per group `buildA2ACatalogEntry(rows, orders[project])`; return `&models.A2ACatalogPage{Entries (non-nil), Total: total, Limit: filter.Limit, Offset: filter.Offset}`. `GetA2ACatalogEntry`: `ListForAgent` → error `fmt.Errorf("get a2a catalogue agent: %w", err)`; zero rows → `utils.ErrAgentNotInA2ACatalog`; orders for `[]string{projectName}`; build. `buildA2ACatalogEntry`: `sortA2ACatalogRows`; reference = first row with `len(Card) > 0` else `rows[0]`; `summary := a2aCardSummaryOf(ref.Card)`; `Name` falls back to `AgentName` when `summary.Name == ""`; per row `CardDiffers = ref.CardFingerprint != "" && row.CardFingerprint != "" && row.CardFingerprint != ref.CardFingerprint`; `Kind = rows[0].Source`.

- [ ] **Step 4: Run tests**

Run: `make test-run RUN='TestListA2ACatalog|TestGetA2ACatalogEntry' PKG=./services/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent-manager-service/utils/errors.go agent-manager-service/models/a2a_catalog.go agent-manager-service/services/a2a_catalog_service.go agent-manager-service/services/a2a_catalog_service_unit_test.go
git commit -m "Build A2A catalogue entries from stored cards" -m "Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 11: Connect details

**Files:**
- Modify: `agent-manager-service/clients/openchoreosvc/client/components.go` (add `const APIKeyHeaderName = "X-API-Key"`; `APIKeyAuthPolicy` uses it), `agent-manager-service/services/a2a_catalog_service.go` (add `GetA2AConnectDetails` to the interface)
- Create: `agent-manager-service/models/a2a_connect.go`, `agent-manager-service/services/a2a_connect.go`
- Test: `agent-manager-service/services/a2a_connect_unit_test.go`

**Interfaces:**
- Consumes: `A2AAgentCardService.GetA2AAgentCard`, `client.OpenChoreoClient.GetEnvironment`, `translateEnvironmentError`, `repositories.A2AProxyRepository.GetByEnvUUID` + `ErrA2AProxyNotFound`, `A2APublisher.GatewayStatus` (Task 6), `repositories.AgentConfigRepository.Get` + `ErrAgentConfigNotFound`, `oauthConfigFromAgentConfig`, `repositories.GatewayRepository.ListIdentityProvidersByEnvironment(envUUID string)`, `a2aAgentCardPath`, `a2aPathPrefixJSONRPC`, `a2aTransportJSONRPC`, `A2ATransport`, `jsonString`.
- Produces (`models/a2a_connect.go`):
  ```go
  type A2AConnectSecurityMode string
  const ( A2AConnectSecurityNone A2AConnectSecurityMode = "none"; A2AConnectSecurityAPIKey = "apiKey"; A2AConnectSecurityOAuth = "oauth"; A2AConnectSecurityUnmanaged = "unmanaged"; A2AConnectSecurityUnknown = "unknown" )
  // A2AConnectPublicationNone is the connect-only publication status of a direct external agent.
  const A2AConnectPublicationNone = "none"
  type A2AConnectIssuer struct { Name, Issuer string }
  type A2AConnectOAuth struct { Issuers []A2AConnectIssuer; Audiences []string; HeaderName, AuthHeaderPrefix string }
  type A2AConnectSecurity struct { Mode A2AConnectSecurityMode; APIKeyHeader string; OAuth *A2AConnectOAuth }
  type A2AConnectPublication struct { Status string; LastError string } // Status: an A2APublicationStatus value or A2AConnectPublicationNone
  type A2AConnectDetails struct { Kind A2AAgentCardSource; Proxied, IsProduction bool; GatewayURL, CardURL, JSONRPCURL string; Security A2AConnectSecurity; Publication A2AConnectPublication }
  ```
  (`services/a2a_connect.go`):
  ```go
  func (s *a2aCatalogService) GetA2AConnectDetails(ctx context.Context, ouID, projectName, agentName, envName string) (*models.A2AConnectDetails, error)
  func a2aConnectURLs(gatewayURL string, transports []A2ATransport, card *models.A2AAgentCard) (cardURL, jsonRPCURL string) // §6.3 table
  func a2aDeclaredJSONRPCURLs(card json.RawMessage) []string // lenient; nil on any decode problem
  func (s *a2aCatalogService) a2aConnectSecurity(ctx context.Context, ouID, projectName, agentName, envName string, envUUID uuid.UUID) (models.A2AConnectSecurity, error)
  ```

- [ ] **Step 1: Write the failing tests (scenarios 43–54, review items 4–5)**

Harness `newConnectHarness(t)`: `m := newPublisherMocks(t)` (gateway mock returning `[]*models.Gateway{{Vhost: "gw.example.com", GatewayFunctionalityType: models.GatewayRoleIngress}}`), `cards` as a hand-written in-package stub `type connectCardServiceStub struct { A2AAgentCardService; GetFunc func(ctx context.Context, ouID, projectName, agentName, envName string) (*models.A2AAgentCard, error) }` (no generated mock exists for in-package service interfaces), `proxies := m.proxies`, `configs *repomocks.AgentConfigRepositoryMock`, `oc` with `GetEnvironmentFunc` returning `{UUID: envUUID.String(), Name: "dev", IsProduction: false}`; `svc := NewA2ACatalogService(&repomocks.A2ACatalogRepositoryMock{}, cards, m.proxies, m.publisher(), configs, m.gateways, oc, discardLogger())`. Platform card fixture: `{Source: platform, SourceURL: "http://weather.internal:8080/.well-known/agent-card.json", EnvironmentUUID: envUUID, Card: {"supportedInterfaces":[{"protocolBinding":"JSONRPC","url":"http://weather.internal:8080/"}]}}`.

```go
func TestConnectPlatformAgentWithAPIKey(t *testing.T)         // published row, cfg {EnableApiKeySecurity:true} → Kind platform, Proxied false, GatewayURL "https://gw.example.com/weather", Security {apiKey, "X-API-Key", nil}, Publication {published, ""}
func TestConnectReportsAFailedPublication(t *testing.T)       // row {failed, "boom"} → {failed, boom}
func TestConnectResolvesOAuthIssuers(t *testing.T)            // cfg {EnableOAuthSecurity:true, OAuthIssuers:[thunder, gone]}, gateways.ListIdentityProvidersByEnvironmentFunc → [{Name thunder, Issuer https://idp/x}] → mode oauth, Issuers [{thunder,https://idp/x},{gone,""}], HeaderName Authorization, AuthHeaderPrefix Bearer, Audiences []string{}
func TestConnectSecurityNoneAndUnknown(t *testing.T)          // cfg neither flag → none; ErrAgentConfigNotFound → unknown, nil error
func TestConnectDirectExternalAgentReadsNothingElse(t *testing.T) // external card, proxies → ErrA2AProxyNotFound; configs.GetFunc nil, m.pubs.GetFunc nil, m.gateways.ListWithFiltersFunc nil → Kind external, Proxied false, GatewayURL "", mode unmanaged, Publication {none, ""}, CardURL == SourceURL, JSONRPCURL "https://agent.example/rpc" (declared)
func TestConnectProxiedExternalAgent(t *testing.T)            // proxy row present, apiKey cfg → Proxied true, GatewayURL set, publication from row, mode apiKey
func TestConnectMissingPublicationMessages(t *testing.T)      // platform → "...redeploy the agent or save its deploy settings to publish it again"; proxied external → ends "refresh to publish it again"
func TestConnectReportsProduction(t *testing.T)               // IsProduction true
func TestConnectPassesCardAndEnvironmentSentinels(t *testing.T) // cards → utils.ErrAgentCardNotFound → errors.Is; oc.GetEnvironment → utils.ErrNotFound → utils.ErrEnvironmentNotFound
func TestConnectDoesNotMaskDatabaseErrors(t *testing.T)       // table: config read, proxy read, publication read, identity-provider listing each → assert.AnError: errors.Is, result nil, NotErrorIs ErrAgentCardNotFound
func TestA2AConnectURLs(t *testing.T) {                       // #3 table, base "https://gw/agent"
	// [{JSONRPC,""}] → "https://gw/agent"; [{JSONRPC,"/a2a/jsonrpc"}] → ".../a2a/jsonrpc"; [{HTTP+JSON,"/rest"}] → "";
	// nil + platform card declaring http://weather.internal:8080/ → ".../rpc"; nil + proxied card declaring https://gw/agent/v1/jsonrpc → that;
	// no gateway + card declaring https://agent.example/rpc → cardURL = SourceURL, jsonRPC = declared; no gateway + unreadable card → "", SourceURL;
	// every gateway case: cardURL "https://gw/agent/.well-known/agent-card.json"; trailing slash on base is trimmed
}
func TestConnectPlatformAgentWithoutAGateway(t *testing.T)    // review #4: ListWithFiltersFunc → [] → GatewayURL "", CardURL == SourceURL, JSONRPCURL "http://weather.internal:8080/", Publication from row, Security from cfg
func TestA2ADeclaredJSONRPCURLsIsLenient(t *testing.T)        // review #5: {"supportedInterfaces":[{"protocolBinding":"JSONRPC","url":5},{"protocolBinding":"JSONRPC","url":"https://x/rpc"}]} → ["https://x/rpc"]; {"supportedInterfaces":"x"} → nil; nil card → nil
```

- [ ] **Step 2: Run to verify failure**

Run: `make test-run RUN='TestConnect|TestA2AConnectURLs|TestA2ADeclared' PKG=./services/`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

`GetA2AConnectDetails` follows §6.1 in order: card (errors returned unchanged) → environment (`translateEnvironmentError`, others `fmt.Errorf("failed to read environment %s: %w", envName, err)`; `IsProduction`) → `Kind = card.Source` → external: `proxyRepo.GetByEnvUUID(ctx, ouID, projectName, agentName, card.EnvironmentUUID)`; `ErrA2AProxyNotFound` → direct: `Security{Mode: unmanaged}`, `Publication{Status: A2AConnectPublicationNone}`, URLs from `a2aConnectURLs("", nil, card)`, return; other error `fmt.Errorf("failed to read the A2A proxy: %w", err)` → platform/proxied: `status, err := s.publisher.GatewayStatus(ctx, ouID, projectName, agentName, card.EnvironmentUUID, source)` (source external for an external card, platform otherwise); URLs `a2aConnectURLs(status.GatewayURL, status.Transports, card)`; `Publication{string(status.Status), status.LastError}`; security via `a2aConnectSecurity`.

`a2aConnectURLs`: `base := strings.TrimRight(gatewayURL, "/")`; no gateway → `(card.SourceURL, first declared or "")`; transports non-nil → `(base + a2aAgentCardPath, base + pathPrefix of the first JSONRPC transport, or "")`; transports nil → `(base + a2aAgentCardPath, first declared equal to base or with prefix base+"/", else base + a2aPathPrefixJSONRPC)`.

`a2aConnectSecurity`: `agentConfigRepo.Get`; `ErrAgentConfigNotFound` → `unknown`; other error `fmt.Errorf("failed to read the agent config: %w", err)`; `EnableOAuthSecurity` → `oauth` with `oauthConfigFromAgentConfig(cfg)`, issuers from `gatewayRepo.ListIdentityProvidersByEnvironment(envUUID.String())` (error → `fmt.Errorf("failed to list identity providers: %w", err)`), name→issuer map, `Audiences` non-nil; `EnableApiKeySecurity` → `apiKey` with `APIKeyHeader: client.APIKeyHeaderName`; else `none`.

- [ ] **Step 4: Run tests**

Run: `make test-run RUN='TestConnect|TestA2AConnectURLs|TestA2ADeclared' PKG=./services/ && go test ./clients/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add agent-manager-service/clients/openchoreosvc/client/components.go agent-manager-service/models/a2a_connect.go agent-manager-service/services/a2a_catalog_service.go agent-manager-service/services/a2a_connect.go agent-manager-service/services/a2a_connect_unit_test.go
git commit -m "Report what a client needs to connect to an A2A agent" -m "Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 12: Controller, routes, wiring

**Files:**
- Create: `agent-manager-service/controllers/a2a_catalog_controller.go`, `agent-manager-service/api/a2a_catalog_routes.go`
- Modify: `agent-manager-service/api/app.go` (call `registerA2ACatalogRoutes(rr, params.A2ACatalogController)` right after `registerA2AProxyRoutes`), `agent-manager-service/api/audit_coverage_test.go` (append `controllers.NewA2ACatalogController` after `NewA2AProxyController`), `agent-manager-service/wiring/wire.go` (`ProvideA2ACatalogRepository` in the repository set after `ProvideA2AProxyRepository`; `services.NewA2ACatalogService` after `services.NewA2AProxyService`; `controllers.NewA2ACatalogController` after `controllers.NewA2AProxyController`), `agent-manager-service/wiring/params.go` (`A2ACatalogController controllers.A2ACatalogController` after `A2AProxyController`)
- Regenerate: `agent-manager-service/wiring/wire_gen.go` (`make wire`)
- Test: `agent-manager-service/controllers/a2a_catalog_controller_unit_test.go`

**Interfaces:**
- Consumes: `services.A2ACatalogService` (Tasks 10–11), `spec.*` types (Task 1), `utils.WriteSuccessResponse`, `utils.WriteErrorResponse`, `handleCommonErrors`, `writeAgentCardError`, `agentCardPath`, `middleware.OUIDFromRequest`, `utils.PathParamProjName`/`PathParamAgentName`/`PathParamEnvID`, `utils.DefaultLimit`/`MinLimit`/`MaxLimit`/`DefaultOffset`/`MinOffset`, `rbac.AgentRead`, `utils.ErrAgentNotInA2ACatalog`.
- Produces:
  ```go
  type A2ACatalogController interface { ListA2AAgents(w http.ResponseWriter, r *http.Request); GetA2ACatalogEntry(w http.ResponseWriter, r *http.Request); GetA2AConnect(w http.ResponseWriter, r *http.Request) }
  func NewA2ACatalogController(catalogService services.A2ACatalogService) A2ACatalogController
  func registerA2ACatalogRoutes(rr *middleware.RouteRegistrar, ctrl controllers.A2ACatalogController) // comment: three GETs, none in sensitiveReadPaths, so not audited
  func ProvideA2ACatalogRepository(db *gorm.DB) repositories.A2ACatalogRepository
  ```

- [ ] **Step 1: Write the failing tests (scenarios 61–68, review item 1)**

Stub: `catalogServiceStub{ListFunc, EntryFunc, ConnectFunc}` implementing `services.A2ACatalogService`. Requests built like `agentEnvRequest` (set path values, `middleware.WithResolvedOrg(ctx, middleware.ResolvedOrg{OUID: "ou-org"})`).

```go
func TestListA2AAgentsParsesTheQuery(t *testing.T)   // ?q=%20refund%20&kind=proxied&environment=dev&project=p1&status=fetched&limit=24&offset=48 → filter {OUID "ou-org", Query "refund", Project p1, Environment dev, Kind proxied, Status fetched, Limit 24, Offset 48}; service returns total 42/limit 24/offset 48 and one env {proxied, matched, cardDiffers true, status fetched, fetchedAt set} → JSON echoes them, fetchedAt RFC3339
func TestListA2AAgentsDefaults(t *testing.T)         // no params → Limit 50, Offset 0; service returns zero page → body exactly {"agents":[],"total":0,"limit":0,"offset":0}
func TestListA2AAgentsBlankQueryMeansNoSearch(t *testing.T) // review #1: ?q=%20%20 → Query ""
func TestListA2AAgentsRejectsBadParameters(t *testing.T) { // table; service not called
	// limit=0, limit=101, limit=x → "Invalid limit parameter: must be between 1 and 100"
	// offset=-1 → "Invalid offset parameter: must be 0 or greater"
	// q=<256 chars> → "Invalid q parameter: must be at most 255 characters"
	// kind=bogus → "Invalid kind parameter: must be platform, external or proxied"
	// status=bogus → "Invalid status parameter: must be pending, fetched or failed"
	// limit=1 and limit=100 → 200
}
func TestGetA2AConnectSerialisesAPIKeySecurity(t *testing.T) // body JSONEq {"kind":"external","proxied":true,"isProduction":false,"gatewayUrl":"https://gw/agent","cardUrl":"https://gw/agent/.well-known/agent-card.json","jsonRpcUrl":"https://gw/agent/rpc","security":{"mode":"apiKey","apiKeyHeader":"X-API-Key"},"publication":{"status":"published","lastError":""}}
func TestGetA2AConnectSerialisesOAuthSecurity(t *testing.T)  // security {"mode":"oauth","oauth":{"issuers":[{"name":"thunder","issuer":"https://idp.example.com"}],"audiences":["aud"],"headerName":"Authorization","authHeaderPrefix":"Bearer "}}, no apiKeyHeader; "gatewayUrl":"","cardUrl":"","jsonRpcUrl":"" present
func TestGetA2AConnectMapsCardNotFound(t *testing.T)         // utils.ErrAgentCardNotFound → 404
func TestGetA2ACatalogEntrySerialisesAPendingAgent(t *testing.T) // entry with one pending env, no card → "description":"", "version":"", "skills":[], env object lacks "fetchedAt"; service received ("ou-org","checkout","weather")
func TestGetA2ACatalogEntryNotInCatalogue(t *testing.T)     // utils.ErrAgentNotInA2ACatalog → 404, message "The agent is not in the A2A catalogue"
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./controllers/ -run 'A2AAgents|A2ACatalogEntry|A2AConnect'`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement the controller**

`ListA2AAgents`: validate in §3.1 order with a local `parseBoundedInt(raw string, def, min, max int) (int, bool)` (absent/empty → default; non-integer or out of range → false) — `limit` (1..100), `offset` (0..∞); `q := strings.TrimSpace(r.URL.Query().Get("q"))`, `utf8.RuneCountInString(q) > 255` → 400; `kind` ∈ {"", platform, external, proxied}; `status` ∈ {"", pending, fetched, failed}; messages verbatim from §3.1 via `utils.WriteErrorResponse(w, 400, msg)`. Call `ListA2ACatalog(r.Context(), models.A2ACatalogFilter{OUID: middleware.OUIDFromRequest(r), ...})`; error → `handleCommonErrors(w, err, "Failed to list A2A agents")`; map to `spec.A2ACatalogListResponse` with `Agents` non-nil and each agent's `Skills` non-nil (`[]string{}`); use the generated `spec.New…` constructors and `SetFetchedAt` so optionals stay absent. `GetA2ACatalogEntry`: path values via `r.PathValue(utils.PathParamProjName/AgentName)`; `errors.Is(err, utils.ErrAgentNotInA2ACatalog)` → `utils.WriteErrorResponse(w, 404, "The agent is not in the A2A catalogue")`; else `handleCommonErrors(w, err, "Failed to get the A2A catalogue entry")`. `GetA2AConnect`: `agentCardPath(r)`; error → `writeAgentCardError(w, r, err, "Failed to get the A2A connect details")`; map to `spec.A2AConnectResponse` (`SetApiKeyHeader` only when non-empty, `SetOauth` only for oauth; `Audiences`/`Issuers` non-nil).

- [ ] **Step 4: Routes and wiring**

`api/a2a_catalog_routes.go`: three `rr.HandleFuncWithValidationAndAuthz("GET " + pattern, rbac.AgentRead, handler)` for `/orgs/{orgName}/a2a-agents`, `/orgs/{orgName}/projects/{projName}/agents/{agentName}/a2a-catalog-entry`, `/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/a2a-connect`. Wire as listed under Files; `ProvideA2ACatalogRepository` returns `repositories.NewA2ACatalogRepository(db)`.

Run: `make wire && go build ./...`
Expected: `wiring/wire_gen.go` constructs `a2ACatalogController` in both initializers; build passes.

- [ ] **Step 5: Run tests**

Run: `go test ./controllers/ -run 'A2AAgents|A2ACatalogEntry|A2AConnect' && go test ./api/ && make test-unit`
Expected: PASS, including `TestEveryMutatingRouteIsAudited` and the route-authz invariant test.

- [ ] **Step 6: Commit**

```bash
git add agent-manager-service/controllers/a2a_catalog_controller.go agent-manager-service/controllers/a2a_catalog_controller_unit_test.go agent-manager-service/api/a2a_catalog_routes.go agent-manager-service/api/app.go agent-manager-service/api/audit_coverage_test.go agent-manager-service/wiring/wire.go agent-manager-service/wiring/params.go agent-manager-service/wiring/wire_gen.go
git commit -m "Serve the A2A catalogue and connect operations over HTTP" -m "Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 13: Whole-branch verification

**Files:** none new; fix-ups only.

- [ ] **Step 1: Format and lint**

Run (from `agent-manager-service/`): `make fmt && golangci-lint run --config .github/linters/.golangci.yaml ./... && gofmt -l .`
Expected: no findings, no files listed.

- [ ] **Step 2: Generated-code drift**

Run: `make codegenfmt-check && git status --short`
Expected: no `.go` diff (wire, spec, mocks, fmt and `go mod tidy` leave the tree unchanged).

- [ ] **Step 3: All tiers**

Run: `make test-unit && go build -tags=integration ./... && make test-integration`
Expected: PASS.

Run (repo root): `make amctl-gen-client && git status --short cli/ && cd cli && go build ./... && go test ./...`
Expected: no diff under `cli/`; build and tests pass.

- [ ] **Step 4: Commit any fix-ups**

Only if Steps 1–3 changed files: stage those paths explicitly and commit with an imperative message describing the fix plus the `Claude-Session` trailer.
