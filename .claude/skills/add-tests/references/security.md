# Permissions and live security tests

## A new permission or scope

The static invariants (`agent-manager-service/api/*_invariant_test.go`, `rbac/*_test.go`, run by `make test-unit`) fail until the permission is wired everywhere. In the same change:

1. Declare it in `rbac/` (`resource:verb`) and register the route with an authz registrar.
2. Grant it to at least one predefined role.
3. Advertise its scope in `agent-manager-service/ampinit/templates/thunder/bootstrap/50-amp-api-client.yaml` and `60-amp-resource-server.yaml`.
4. Add the scope to `ampScopes` in `test/e2e/framework/auth.go`, so the e2e client requests it.

Check with `make security-test-static` at the repository root. An authorization invariant you add must be named `TestSecurityInvariant…` or that target never runs it.

## Live security specs (`test/e2e/security/`)

- Live suites sit outside `tests/` on purpose; `make e2e-test` doesn't run them.
- Scope matrix: add a `guardedRoute{Method, Path, Scopes}` to `guardedRoutes` in `security/authz/scope_matrix_test.go` for a route that deletes, disables or archives something, or changes roles, credentials or membership. Requests must have no side effects: name absent resources (`absentName`) and send no body on POST/PUT.
- Roles and role policies: `security/roles/`. Tokens: `security/tokens/`. Agent identity: `security/agentid/`.

## Run

```bash
make security-test-static                   # no stack
make up && make security-test-live SUITE=authz
make security-test-live FOCUS="scope matrix"
```

CI runs the live suites after the e2e suites in `.github/workflows/e2e.yaml`.
