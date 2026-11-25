---
name: add-api-resource
description: Add or change a REST API resource in agent-manager-service (the Go control plane). Use when the user asks to add/modify an endpoint, resource, route, or API operation in agent-manager-service — anything that touches the OpenAPI spec, controllers, services, repositories, or RBAC. Enforces the spec-first, codegen, and per-route-authz workflow so generated code and permissions stay consistent.
---

# Add/change an API resource (agent-manager-service)

Spec-first, layered, per-route-authz workflow for the Go control plane. The request/response types and mocks are **generated** — never hand-write them.

**Read first:** `agent-manager-service/AGENTS.md` → "Golden path", "Layering", "Permissions (RBAC)", "Code generation". This skill is the executable checklist; that guide has the *why*.

## Steps (do in order)

1. **Edit the OpenAPI spec** — `agent-manager-service/docs/api_v1_openapi.yaml`. Add/modify the path, operation, and schemas. This is the source of truth for request/response shapes.
2. **`make spec`** (needs Docker) — regenerates `spec/` from the YAML. The whole `spec/` dir is deleted and rebuilt, so **never** hand-edit a struct there; your edits would be lost.
3. **Add permission(s)** in `rbac/permissions.go` — name them `resource:verb` (`:create`/`:read`/`:update`/`:delete`, or `:manage` only where peers already do). Its scope also goes in the ThunderID bootstrap (`ampinit/templates/thunder/bootstrap/50-amp-api-client.yaml`, `60-amp-resource-server.yaml`) and in `ampScopes` in `test/e2e/framework/auth.go`; `make test-unit` fails until all of them agree.
4. **Implement controller → service → repository** (`controllers/` → `services/` → `repositories/`):
   - Controller: HTTP only — parse/validate, map result→status, translate sentinel errors→HTTP.
   - Service: business logic; depend on repo/client **interfaces**, never concrete types.
   - Repository: persistence only (interface + impl). Add an interface if you introduce a new one.
5. **Register the route with authz** in `api/<resource>_routes.go` using `HandleFuncWithValidationAndAuthz(pattern, rbac.<Perm>, ctrl.<Handler>)`. Every route declares its own permission.
6. **Grant the permission to at least one role** in `rbac/predefined_roles.go` (`PredefinedRolePermissions`). An ungranted permission is unreachable.
7. **`make codegen`** — only if you added/changed an interface (regenerates wire DI + mocks; needs `moq` on PATH). A new repo interface needs a `//go:generate moq ...` directive above its declaration — copy an existing one.
8. **Add the tests** — use the `add-tests` skill: a service unit test (`add-service-unit-test`), an API-level integration test, and an e2e or scope-matrix spec where the route changes a user flow or guards something destructive.

## Audit logging

Step 5 audits the route automatically — a mutating route cannot ship unaudited, and `api/audit_coverage_test.go` fails the build if one does. Two cases need you:

- **The build says `cannot derive an action for audited route`**, or the route's `rbac.Permission` does not describe what it does (one permission gating several operations, or naming a different resource). Add one line to `actionOverrides` in `audit/policy.go`.
- **The operation touches credentials, privileges, membership, deployment or deletion.** The envelope record cannot say *what changed*, so add a semantic event — use the **`add-audit-event` skill**.

Never read a request or response body into a record; pass named fields via `audit.Detail`, which takes scalars and `[]string` only.

## Guardrails

- **Multi-tenancy:** the service must validate the caller's org against the **target resource it loads**, not just the path (`RequireOrgMatch` is a first-pass filter, not the enforcement layer).
- **Errors:** map `gorm.ErrRecordNotFound` → the specific `utils.ErrXxxNotFound`; wrap everything else with `%w`. Never flatten an unexpected error into not-found; never silently fall back to a default on a real error. Compare with `errors.Is`.
- **Context:** every I/O method takes `context.Context` first and propagates it.
- **Don't re-fetch** a resource already loaded earlier in the request path — pass it down.

## Done checklist

- [ ] `spec/` regenerated (`make spec`) and committed if the YAML changed.
- [ ] New permission exists in `rbac/permissions.go` **and** is granted in `rbac/predefined_roles.go`.
- [ ] Route registered with an authz registrar (not plain `HandleFuncWithValidation` unless deliberate).
- [ ] `make codegen` run + mocks committed if an interface changed.
- [ ] Tests added per the `add-tests` skill; `make test-unit` passes (includes `TestEveryMutatingRouteIsAudited` and the `TestSecurityInvariant*` checks).
- [ ] Semantic audit event added if the operation is security-critical (`add-audit-event` skill).
- [ ] `go build -tags=integration ./...` compiles.
- [ ] CI lint clean: `golangci-lint run --config .github/linters/.golangci.yaml ./...`
