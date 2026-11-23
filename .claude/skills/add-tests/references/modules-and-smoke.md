# Modules and smoke tests

Background: `docs/amp-lite/modules.md` → Testing.

## A module

- **Unit tests** for the module's own logic. The host lifecycle is already covered in `internal/modhost`.
- **Integration test** in the style of `TestModuleContract` (`agent-manager-service/tests/module_contract_test.go`): routes 404 before install and after disable, 403/200 by permission, hooks, events after commit, capabilities. Run on both databases.
- **Invariant registrations** (all in `agent-manager-service/api/`): blank-import the module in `route_authz_invariant_test.go` and `permission_owner_invariant_test.go`; add its route package to `routePackages` and its permission package to `modulePermissionPackages`; add `Test<Module>RoutesAreAudited` and `Test<Module>SpecMatchesRoutes` to `module_routes_invariant_test.go`. If it calls `rt.Track`, add its packages to `routeDirs` in `middleware/growthanalytics/taxonomy_test.go` and its features to `taxonomy.yaml`.

## Smoke (`deployments/standalone/scripts/smoke.sh`)

- The script lists which routes each distribution serves. A new module route needs a line, for `amp` (served) and for `amp-core` / `amp-off` (not served).
- It signs in as the admin, so the stack must use file sign-in (the default).

```bash
make up && make smoke                       # full stack
make up-core && make smoke-core             # core only
(cd deployments/standalone && docker compose exec amp-api /go/bin/amp module disable ai-gateway)
deployments/standalone/scripts/smoke.sh amp-off
```

CI (`standalone-smoke.yaml`) runs all three on PRs to `amp-2.x` that touch `agent-manager-service/**` or `deployments/standalone/**`, without the console.
