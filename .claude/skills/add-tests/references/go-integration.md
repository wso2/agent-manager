# Go integration tests (agent-manager-service)

Background: `agent-manager-service/AGENTS.md` → Testing.

## Repository, SQL and migration tests

- File next to the code, `//go:build integration` as **line 1**, then the licence header, then the package.
- Get the database with `db.GetDB()`; the runner has already migrated it. Reuse the fixtures in `repositories/fixtures_integration_test.go` (`newOrg`, `ensureAgent`, `seedAgent`, `testSecrets`) instead of declaring new ones — a duplicate name is a compile error.
- Make each test independent: create its own org with `newOrg(t)` and clean up with `t.Cleanup`.
- Assert what only the database can tell you: constraints, uniqueness, cascades, transactions, encrypted columns, per-database queries.
- Core schema: edit `db_migrations/schema/lite.yaml`, run `make schema`, and commit both rendered migrations. `TestBaselinesAreGenerated` fails if they are stale. Never edit a migration that has shipped.

## API-level tests (`agent-manager-service/tests/`)

- `package tests`, `//go:build integration`. Name the file after the operation: `tests/<operation>_test.go`.
- Build the router with `apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{...}, authMiddleware)`; put the mocks you need in `wiring.TestClients` and sign requests with `jwtassertion.NewMockMiddleware(t)`. Copy `tests/create_project_test.go` or `tests/delete_agent_test.go`.
- Module behaviour over the real API: copy `TestModuleContract` (`tests/module_contract_test.go`), which builds a host with `modhost.NewWith(...)`; `tests/modules/testmod` is the example module.

## Run

```bash
cd agent-manager-service
make test-integration                       # SQLite, -p 1
DB_DRIVER=postgres make test-integration    # PostgreSQL from .env.test
```

CI runs both on every PR that touches `agent-manager-service/**`. Don't use `make test`: it runs against the database in `.env`.
