# CLI, console and Python tests

## amctl (`cli/`)

- `cli/pkg/cmd/<group>/<command>_test.go`, same package as the command.
- Build `*XxxOptions` by hand and call `runX(ctx, opts)`; never go through cobra's `RunE`. Capture output with `iostreams.Test()`. Copy `cli/pkg/cmd/agent/get_test.go` or `delete_test.go`.
- If the service or a module's OpenAPI document changed, run `make amctl-gen-client` and commit the result; `cli-codegen-check.yaml` fails otherwise.
- Run: `cd cli && go test -race ./pkg/cmd/<group>/...`. CI: `amctl-pr-checks.yaml`.

## Console (`console/`)

- Vitest (jsdom). The test sits next to the file: `<name>.test.ts` or `<name>.test.tsx`. Examples: `workspaces/libs/api-client/src/acp-core/hooks/projects.test.ts`, `workspaces/ui-modules/ai-gateway/pages/gateways/src/subComponents/`.
- A package needs a `test` script (`vitest run`) and a `vitest.config.ts`; copy them from `workspaces/libs/shared-component` when adding the first test to a package. Keep `../../../vitest.jsdom-storage.ts` (path relative to the package) in `setupFiles`: on Node 25+ a test that touches `localStorage` fails without it.
- Mock the workspace hooks a page calls (`vi.mock("@agent-management-platform/api-client", …)`) and render it inside a `MemoryRouter` with the route it expects; `AddNewAgent.test.tsx` and `ProjectOverview.test.tsx` under `ui-modules/acp-core/pages/` are examples.
- Tests import other workspace packages from their built output, so build first:

```bash
cd console
pnpm build          # once, and after changing a package another test imports
pnpm test           # every package with tests
pnpm --filter @agent-management-platform/<package> run test
```

CI: `amp-console-package-pr-checks.yaml` (lint, build, test) on PRs that touch `console/**`.

## Python

- `libs/amp-instrumentation`: `tests/`, pytest, ruff and mypy (`libs/amp-instrumentation/AGENTS.md`). CI on PRs that touch the library.
- `libs/amp-evaluation`: `tests/`, pytest; an evaluator → the `add-evaluator` skill. CI is manual only while evaluation is out of AMP Lite.
- Instrumentation matrix: a new framework or provider version is a cell — `test/instrumentation-matrix/RUNBOOK.md`.
- Samples: a local smoke in `test/sample-smoke/<sample>/test_*.py` and, for samples that have one, a platform smoke (`platform_smoke.py`) that needs the stack in ThunderID sign-in mode — `test/sample-smoke/README.md`.
