---
name: add-feature-telemetry
description: Add growth-analytics (feature taxonomy) tracking for a new or changed Agent Manager feature. Use when a feature is implemented and its usage should be measured — wrapping a route in growthanalytics.Track, adding a dimension, registering a new console action (amp.console.*) the UI should publish, or when TestTaxonomyDeclaresTrackedEvents / TestTaxonomyDeclaresConsoleActions / TestTaxonomyFeaturesHaveEmitters fails. Covers taxonomy.yaml, the server Track wrapper, the console action allowlist in console.go, and the console's ConsoleAction/useTrack side.
---

# Add feature telemetry (growth analytics / feature taxonomy)

Two halves feed Moesif, kept deliberately separate:

| Half | What it measures | Emitted by | Lands in Moesif as |
|---|---|---|---|
| **API (server)** | A mutating endpoint succeeded/failed | `growthanalytics.Track(...)` in `agent-manager-service/api/*_routes.go` | Event, `source: "api"` |
| **Console** | What the user did in the UI that the API **cannot see** (page views, abandoned dialogs, copied snippets, empty states) | `useTrack()` in the console → `POST /telemetry/console-actions` → allowlist in `console.go` | Action, `source: "console"` |

**Single source of truth:** `agent-manager-service/middleware/growthanalytics/taxonomy.yaml`. It describes what the code *actually emits*. Codes and enum values are already in production data — **change the YAML to match the code, never rename a live code or enum value.** Retire with `status: deprecated` + `availability.deprecated_in`.

Read first: the header comments of `taxonomy.yaml`, `track.go` and `console.go`.

## Step 0 — decide what to track

- New mutating endpoint (POST/PUT/PATCH/DELETE) that represents feature adoption → **server Track** (Steps 1–3).
- New UI interaction with no request behind it (a new page, a wizard, a dialog that can be abandoned, a copy button) → first check whether an existing generic console action already covers it (`page-view`, `tab-switch`, `dialog-opened`, `form-abandoned`, `copy-snippet`, `empty-state`, …) with a new **dimension value**. Only add a new `amp.console.*` action if none fits (Steps 4–6).
- **Never** add a console action that duplicates a successful mutation the API already tracks (`TestConsoleTaxonomyIsDisjointFromEndpointCodes`). The console adds the *denominator*, not the success.
- Read-only GETs are not tracked server-side.

## Step 1 — declare the feature in `taxonomy.yaml`

Codes are `amp.<category>.<feature>[.<variant>]`; the second segment **must** equal `category`, and `category` must exist under `categories:`. Copy a neighbouring feature block:

```yaml
  - code: "amp.agent-development.create-widget"
    name: "Widget Creation"
    description: "Create a widget for an agent."
    category: "agent-development"
    tier: "core"            # core | advanced | ...  (match neighbours)
    status: "stable"        # or beta / preview
    availability:
      introduced_in: "<next release, e.g. 2026.11.0>"
    tracking:
      event_name: "amp.agent-development.create-widget"   # == code
      event_category: "adoption"                          # adoption | engagement | ...
      platforms: ["web", "api"]
      dimensions:
        - name: "action"
          type: "enum"
          values: ["created-widget"]
          required: true
      aggregations:
        - type: "count"
          label: "Widgets Created"
```

Rules enforced by `taxonomy_test.go` (`TestTaxonomyConformsToSkeleton`):
- every dimension a call site sends is declared; every `enum` value sent is in `values`; every `required` dimension is sent;
- codes are lowercase dot-separated kebab segments, dimension names snake_case (enum values are kebab-case by convention);
- `introduced_in` is a version like `2026.11.0` (match the newest neighbour);
- per-request metadata (`platform`, `source`, `growth_action`, `product_version`, `deployment_model`, `environment`) is added automatically — do **not** declare it per feature.

If the feature exists but you are not wiring an emitter yet, add its code to `notEmittedFeatures` in `taxonomy_test.go` and put a `# Not emitted yet` comment in its tracking block.

## Step 2 — wrap the route with `Track`

In the matching `api/<resource>_routes.go`, wrap the handler (keep the existing authz wrapper outside):

```go
rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/.../widgets", rbac.PermWidgetCreate,
    growthanalytics.Track("amp.agent-development.create-widget", actionDims("created-widget"), ctrl.CreateWidget))
```

- Static dimensions: pass a **literal map or the existing helpers** (`actionDims`, `updateTargetDims`, …). The test parses call sites from source; dimension values computed at runtime are invisible to it.
- Dynamic values known only inside the handler: `growthanalytics.SetDimension(ctx, "creation_method", "from-kind")` from the controller/service. The key must still be declared in the YAML; for enums, list every value it can take.
- `outcome` (success/failure from the status code) is added by Track — don't set it.
- Never put identifiers, names, free text, secrets or PII in dimensions. Bounded enums, booleans and small counts only.

## Step 3 — run the tests

```bash
cd agent-manager-service && go test ./middleware/growthanalytics/... ./api/...
```

## Step 4 — register a console action on the server (only if Step 0 said so)

In `middleware/growthanalytics/console.go`, add to `consoleActions` under the right group, listing **exactly** the dimension keys it may carry:

```go
"amp.console.widgets.preview-opened": dims("widget_type", "entry_point"),
```

Anything not in this map is dropped server-side (202, counted as `dropped`, logged). This is intentional: **the service release with the allowlist entry must ship before (or with) the console release that emits it.**

Then add a matching feature block in `taxonomy.yaml` (code `amp.console.<group>.<action>`, `category: "console"`, `platforms: ["web"]`). `TestTaxonomyDeclaresConsoleActions` requires the YAML dimension set to equal the `dims(...)` set exactly.

## Step 5 — publish it from the console

1. Add the name to the `ConsoleAction` const in `console/workspaces/libs/api-client/src/hooks/telemetry.ts` (same string as the Go key).
2. Emit it with the existing hooks — never call `reportConsoleActions` directly:
   ```tsx
   const track = useTrack();
   track(ConsoleAction.WidgetPreviewOpened, { widget_type: type, entry_point: "list" });
   ```
   Purpose-built helpers already exist: `usePageViewTracking(path, entityScope)`, `useDialogAnalytics(...)`, `useValidationErrorTracking(entity)`, `useSessionAnalytics(...)`. Prefer them.
3. Dimension values are scalar (`string | number | boolean`), ≤256 chars, and must not contain names, IDs, query strings, secrets or snippet contents (report *which* snippet, never its value).
4. Telemetry is best-effort: never show an error, never retry, never block UI on it. Tracking is a no-op when `isConsoleAnalyticsEnabled()` is false.

## Step 6 — verify

```bash
cd agent-manager-service && go test ./middleware/growthanalytics/...
cd console/workspaces/libs/api-client && npm run lint && npm test
```

Optionally run the console against a local service with growth analytics enabled and confirm the `/telemetry/console-actions` response shows `accepted` > 0 and `dropped: 0` for your action.

## Checklist

- [ ] Feature block in `taxonomy.yaml`, code segment == category, `introduced_in` set
- [ ] Server: `Track` wrapper with literal dims / `SetDimension` for runtime values
- [ ] Console (if any): `consoleActions` entry + YAML block + `ConsoleAction` const + `useTrack` call
- [ ] No PII / identifiers / free text in dimensions
- [ ] No duplicate of an API success as a console action
- [ ] `go test ./middleware/growthanalytics/...` passes
- [ ] If the feature also needs a subscribable platform event, use `/add-platform-event`; if it touches credentials/permissions/deletion, `/add-audit-event`
