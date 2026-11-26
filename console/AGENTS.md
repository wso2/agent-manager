# console — agent guide

React 19 + TypeScript web UI. **pnpm workspaces monorepo**, built with **Vite**. Server state via **TanStack Query**; UI from **Oxygen UI** (WSO2's MUI 7 layer). Every package is scoped `@agent-management-platform/*` (the app shell `web-ui` is the one unscoped exception).

For the Oxygen UI component library itself (theming, which components exist, import rules), see **`.ai/oxygen-ui/AGENTS.md`** — always import UI from `@wso2/oxygen-ui`, never `@mui/material` directly.

## Monorepo layout

| Area | Package(s) | Role |
|---|---|---|
| `apps/web-ui/` | `web-ui` | App entry, Vite config, runtime `config.js`, router mount |
| `workspaces/core-ui/` | `@…/am-core-ui` | Provider stack, routing, layout, page registry |
| `workspaces/libs/api-client/` | `@…/api-client` | **All** API fetch fns + TanStack Query hooks, grouped by API topic into `src/<module>/` |
| `workspaces/libs/auth/` | `@…/auth` | Auth-mode switch (Asgardeo vs no-auth) |
| `workspaces/libs/types/` | `@…/types` | Shared types, `globalConfig`, route maps |
| `workspaces/libs/views/` | `@…/views` | UI primitives (PageLayout, FormElements, SnackBar) |
| `workspaces/libs/shared-component/` | `@…/shared-component` | Business components (ConfirmationDialog, PolicyListSection) |
| `workspaces/ui-modules/<module>/` | `@…/ui-module-<module>` | A UI module (product area): exports its `UIModuleManifest` listing its page plugins |
| `workspaces/ui-modules/<module>/pages/<feature>/` | `@…/<feature>` | One package per feature page; exports a `PluginManifest` from `<feature>/plugin` |

`pnpm-workspace.yaml` is the authoritative project list. Vite (dev) resolves each `@agent-management-platform/*` import to the package's **`src/`** for hot reload — no separate TS build step in dev. This holds for core-ui's own build too, so `make dev` needs no prior `make build`. core-ui is Emotion-styled and emits no stylesheet, so nothing consumes a `dist/index.css`.

## Commands

```bash
make install          # pnpm install (only after package.json / dep changes)
make dev              # web-ui dev server → http://localhost:3000 (no build needed)
make build            # pnpm build (pnpm -r run build, all packages)
make build-webapp     # pnpm build:core-ui (core-ui and its dependencies)
make clean            # remove dist/ everywhere
```

Per-package (from inside the package dir):
```bash
pnpm run dev             # watch/build this package
pnpm run build           # build → dist/
pnpm run lint            # eslint
pnpm run lint:fix        # eslint --fix
pnpm run test            # vitest run
pnpm run test-watch      # vitest watch
```

## Golden path: add an API-backed feature

1. **Pick the module** by what the API call is about, not by which page calls it: `libs/api-client/src/<module>/`, one of `acp-core` (orgs, projects, agents, identities, events, config), `agent-runtime` (environments, and an agent's deploy settings, configurations, LLM and MCP configs, agent card and A2A proxy), `ai-gateway` (gateways, LLM providers, MCP proxies, guardrails, API keys), `ai-observability` (traces, logs, metrics, monitors, evaluators), `agent-identity`. Environment calls live in `agent-runtime` even though many pages use them.
2. **Add the fetch fn** in `libs/api-client/src/<module>/apis/<resource>.ts`.
3. **Add the hook** in `libs/api-client/src/<module>/hooks/<resource>.ts`, and export both from `<module>/index.ts`. Pages keep importing everything from `@agent-management-platform/api-client`.
4. **Gate the hook** if the module is not `acp-core`: AND its `enabled` with `useUIModulePresent("<module>")`, so it stays idle while the module is off.
5. **Consume the hook** in a page under `workspaces/ui-modules/<module>/pages/<feature>/`, in the same module that owns the API. UI that another module's page needs is contributed, not imported (see "Pluggable structure" below).
6. **Register the page** — add a route (and nav item) contribution to the feature package's `src/plugin.ts`. A new package also goes in its module's `src/index.ts` plugin list, and its page id in the backend module's manifest (`UI.Pages`, see below). See "Recipes" below.

### The two-file API pattern (mandatory)

Never call `fetch`/TanStack Query directly from a component. Split into a pure fetch fn and a hook.

**`apis/<resource>.ts`** — pure fetch, takes `params` (path), `query` (search), `getToken`:
```ts
import { httpGET, SERVICE_BASE } from "../../shared/utils";

export async function listAgentDocuments(params, query, getToken) {
  const { projName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${SERVICE_BASE}/agents`, {
    searchParams: { ...query, project: projName } /* stringified */,
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}
```
- HTTP helpers are `httpGET/POST/PUT/PATCH/DELETE` from `shared/utils` — they read `globalConfig.apiBaseUrl` and set the `Authorization: Bearer` header. Do not hand-roll `fetch`.
- `SERVICE_BASE = '/api/v1'` (`utils/utils.ts`). Module APIs use `moduleBase('<module>')`, which is `/m/<module>`; `GATEWAY_BASE` is `moduleBase('ai-gateway')`. Trace calls go to the observability module at `moduleBase('observability')`. Other observer calls (logs, metrics) go through `httpGETObserver`, which needs a trace service URL set with `setObserverBaseUrl`; nothing sets it while the stack runs no trace service.

**`hooks/<resource>.ts`** — wrap with `useApiQuery` / `useApiMutation` (not raw `useQuery`/`useMutation`):
```ts
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiQuery, useApiMutation } from "../../shared/react-query-notifications";

export function useListAgents(params, query) {
  const { getToken } = useAuthHooks();
  return useApiQuery({
    queryKey: ["agents", params, query],
    queryFn: () => listAgents(params, query, getToken),
    enabled: !!params.orgName && !!params.projName,
  });
}

export function useCreateAgent() {
  const { getToken } = useAuthHooks();
  const qc = useQueryClient();
  return useApiMutation({
    action: { verb: "create", target: "agent" },        // → auto snackbar "Agent created successfully"
    mutationFn: ({ params, body }) => createAgent(params, body, getToken),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["agents"] }),
  });
}
```

Rules:
- **`useApiQuery` / `useApiMutation`** (in `shared/react-query-notifications.ts`) wrap TanStack Query and auto-fire success/error snackbars. `useApiMutation` takes `action: { verb, target }` to render the message.
- **`queryKey` is a tuple** — `[domain, params, query]` for detail, `[domain]` for the collection. Invalidation is **prefix-based**: `invalidateQueries({ queryKey: ["agents"] })` clears every `["agents", …]`.
- **Auth token** always comes from `useAuthHooks().getToken` and is passed into the fetch fn — never read a token another way.

## Routing

Routes live in `core-ui/src/Route/`. Use the generated maps in `@…/types`, never hardcoded strings:
- `relativeRouteMap.…path` for `<Route path=…>`.
- `absoluteRouteMap.…path` with `generatePath(path, { orgId, projectId, agentId })` for links/navigation.

Feature pages are not wired in `Route.tsx`: they come from plugin manifests (below). The shell keeps only login, the guards (`OrgGuard`, `ProjectGuard`, `AgentGuard`, which validate the entity exists before rendering the outlet), back-compat redirects, the Module manager and the 404s.

## UI modules and plugins

The console is split into four **UI modules** under `workspaces/ui-modules/`, each a product area:

| Module | Pages |
|---|---|
| `acp-core` | overview, level-resource-listing, add-new-project, add-new-agent (the agent-type picker only), external-agent (Externally-Hosted agent type and its register form), settings (+ events), identities |
| `ai-gateway` | gateways, llm-providers, mcp-proxies, agent-security (credentials), configure-agent (an agent's LLM, MCP and A2A configuration; serves external agents too) |
| `ai-observability` | traces, logs, metrics, eval |
| `agent-identity` | env-thunders (Agent ID, groups, roles) |

### Pluggable structure: the rules

Any module except `acp-core` can be turned off or not installed. The console must work, with no dead UI and no failed requests, for every combination. Every change follows these rules:

1. **The owning module holds the code.** UI and API hooks for a resource live in the module that owns that resource (table above, and the api-client module list in the golden path), whichever page shows it.
2. **No imports across modules.** A page package imports from its own module and from `libs/` only, never from another `ui-modules/<module>` package.
3. **Contribute, do not hard-code.** A host page (an overview, "Add a New Agent", the nav) never names another module's cards, sections, agent types or menu items. The owning module adds them as contributions in its `plugin.ts`, and the host renders whatever is active.
4. **`acp-core` knows no other module.** It defines mount points and renders contributions. It does not call another module's hooks or check a module by name.
5. **A module that is off makes no network calls.** Its api-client query hooks are gated with `useUIModulePresent("<module>")`. Not every module is done yet: check "Gating status" below before assuming a hook is gated, and gate any hook you touch.
6. **Outside code hides, never assumes.** UI outside a module that depends on it checks `useIsUIModuleInstalled("<module>")` (views), which is `undefined` while detection runs: treat that as loading, show "off" only on `false`. It uses the module's gated hooks, never its raw API functions.
7. **Gate with `useActivation`, not by rendering null.** A contribution that should not appear is deactivated, so hosts can lay out and count what is really there.
8. **`plugin.ts` stays light.** The shell loads it eagerly: it imports types, route maps, icons and small hooks only. Page components are reached through `import()` in `load`.

### Where does it go?

| You are adding | Put it in | How it shows up |
|---|---|---|
| A new page | `ui-modules/<owner>/pages/<feature>/` | `route` (+ `nav-item`) contribution |
| A way to create a kind of agent | the module that can create it | `agent-type` contribution + its form as a project-level `route` |
| A section on an agent or project overview | the module that owns the data | `panel` at a `PLUGIN_MOUNT_POINTS` slot |
| A resource card on the organization overview | the module that owns the resource | `resource-summary` contribution |
| A component or helper two modules need | `libs/shared-component` (business) or `libs/views` (primitives) | plain import |
| A place where other modules can add UI | the host page; name it in `PLUGIN_MOUNT_POINTS` (`libs/types/src/plugins`) and its props in `libs/shared-component` | host reads panels with `useExternalComponentModulesByMountPoint` |
| API calls | `libs/api-client/src/<owner>/` | hooks, gated per rule 5 |

If a change needs `if (module X is installed)` inside another module's page, stop: either X should contribute that UI, or the code belongs in `libs/`. The remaining case, a control inside a page that only works with X (for example an OAuth option that needs agent-identity), uses rule 6.

### Recipes

**New page package** (copy a sibling such as `acp-core/pages/external-agent` or `ai-gateway/pages/gateways`):
1. Create `ui-modules/<module>/pages/<feature>/` with `package.json` exporting `.` and `./plugin` (the `pnpm-workspace.yaml` globs already cover it; run `make install`).
2. Write `src/plugin.ts` (a `PluginManifest`; its `id` is the page id) and export page components from `src/index.ts`.
3. Add the package as a dependency of the module and list its plugin in `<module>/src/index.ts`, importing from `<feature>/plugin` only.
4. Add `{ID: "<module>/<plugin id>", Permission: ...}` to the `UI.Pages` of the backend module that serves it (`agent-manager-service/modules/<name>`), or to `api.CoreUI` for a core page (not for `acp-core`, which is always on).
5. Update the module table above.

**New agent type**: contribute `agent-type` (title, description, `ctaLabel`, light `illustration`, `order`, `route`) and the create form as a project-level `route` in the same plugin. Wrap the form in `RequireAgentCreateAccess`, build on `libs/shared-component/src/agentCreation`, and take the back link from `useAgentPickerBack()`.

**New overview section**: write the section component in the owning module with the props from `libs/shared-component/src/overview/panelProps.ts`, and contribute it as a `panel` with `mountPoint`, `order` and, for a half-row card, `width: "half"`. Put the conditions (agent type, access) in `useActivation`.

**New organization resource card**: contribute `resource-summary` with a `useSummary` hook that returns the count, action label and link from one cached list query.

### Reference

- **Page plugin** (`pages/<feature>/src/plugin.ts`, exported as `<feature>/plugin`): a `PluginManifest` with `route`, `nav-item`, `panel`, `agent-type` and `resource-summary` contributions. It must not import page components; `load` uses `import()` so the page stays in its own chunk. Types are in `@…/types` (`libs/types/src/plugins`).
- **Activation**: `useActivation` on the plugin or on a contribution is a React hook (route params, `useRouteAgent()`, feature flags, access). Return `undefined` while loading.
- **Nav slots**: `section` names a slot defined in `core-ui/src/Layouts/OxygenLayout/navigationItems.tsx` (`NAV_LAYOUT`).
- **Module** (`<module>/src/index.ts`): a `UIModuleManifest` listing its page plugins; `core-ui/src/plugins/builtinModules.ts` registers modules.
- **Install and setup**: a module may set `core: true` (always installed, cannot be turned off as a whole) and `installation` (Helm `chart`, ordered `steps`, and a `useDetect` hook that reports installed / not-installed, e.g. from `/api/v1/config`). A module's pages only run while it is installed. The AI module manager (user menu → `/org/:orgId/ui-modules`) lists modules as cards with Install / Uninstall; each module has a details page at `/org/:orgId/ui-modules/<id>` (what it adds, backend setup steps, the module's optional `setup` component). Install and uninstall there are a front-end mock for now, so the AI module manager and details pages exist only when runtime config `uiModules.marketplace` is on (env `ENABLE_UI_MARKETPLACE=true`; on in `.env.development`, off by default).
- **Turning on and off**: the server decides. After sign-in the console reads `GET /api/v1/ui-manifest`, which lists every page (`"<module>/<plugin id>"`, e.g. `ai-observability/traces`; the plugin's `id`, which can differ from its folder, e.g. `agent-identity/agent-identity` for `env-thunders`) the core and the active backend modules offer, filtered by the caller's permissions. A non-core UI module with no listed page is off, and so is every unlisted page of it; `acp-core` is always on. Until the manifest loads, non-core pages are pending (deep links wait, nav hides). When you add a page plugin to a non-core module, list it in the owning backend module's manifest (`modkit.UI.Pages`) or in `api.CoreUI`. If the manifest cannot be read the console shows the core pages only and reports the error. A deployment turns on more with `CONSOLE_UI_MODULES_ENABLE` (`uiModules.enable` in config.js: module ids or `<module>/<page>`, e.g. `ai-observability`). The manifest is cached per user and session. The AI module manager changes the install state in memory for the current tab, and cannot switch on a module or page the manifest leaves out.
- **Environments**: the console has no deployment pipelines; pages list the organization's environments with `useOrgEnvironments` / `useOrgEnvironmentsState` (shared-component) and leave `deploymentPipeline` out of project requests. Environments always come from the environments API, by real name and UUID: never hard-code `default`. An environment picker hides itself when `onlyEnvironmentName(environments)` (shared-component) returns a name, and the form derives its value as `chosen || onlyEnvironmentName(environments)` instead of copying it into state.
- **Agent types**: the "Add a New Agent" page (acp-core) defines no agent types. A module that can create a kind of agent contributes an `agent-type` (card title, description, CTA label, a light `illustration` component, `order`, and the `route` of its create form) plus that form as a project-level `route`; wrap the form in `RequireAgentCreateAccess`. The page reads active types with `useAgentTypes()` (views) and renders panels at `PLUGIN_MOUNT_POINTS.addNewAgentOptions` below the cards. What the create forms share (schema, payload builder, `CreateButtons`, `AgentHandleField`, `useAgentWizardReturn`) lives in `libs/shared-component/src/agentCreation`. `wizardSteps` (default `[id]`) are the create wizard's telemetry steps the choice records. With a single active type and no panels, the page redirects straight to that type's form (`useAgentTypePicker().soleAgentType`), so a type's first form page takes its back link from `useAgentPickerBack()`, never a hard-coded link to the picker.
- **Overview sections**: acp-core's overviews show no other module's data themselves. A module adds a section as a `panel` at `PLUGIN_MOUNT_POINTS.agentOverviewEnvironment` (per-environment sections; `order`, and `width: "half"` panels pair up) or `projectOverviewSide`; the overview renders them with `OverviewPanels` and the props in `libs/shared-component/src/overview/panelProps.ts`. A section that should not appear (e.g. the endpoint interface for an external agent) is gated with `useActivation`, not by rendering null, so its half-row partner widens. The organization overview's resource cards are `resource-summary` contributions: card data plus a `useSummary` hook (count, action label and link), run in its own component.
- **A module's network calls stop with the module**: a module's query hooks in api-client AND their `enabled` with `useUIModulePresent("<module>")` (`libs/api-client/src/shared/useUIModulePresent.ts`), so no page, in that module or another, calls its APIs while the UI manifest leaves it out or it is not installed. Code outside the module that uses its data also hides that UI with `useIsUIModuleInstalled("<module>")`, and never calls the module's API functions directly (they bypass the gate). A feature flag alone is not enough: the UI manifest can leave a module out while its flag is on.
- **Gating status** (update this list when you gate more hooks):

  | Module | Query hooks gated | Still pending |
  |---|---|---|
  | `agent-identity` | all (roles, groups, thunder instances, an agent's identity) | none |
  | `ai-gateway` | all except `useTestAgentAPIKey`, which no page uses | Outside UI still shown with the module off (rule 6): ai-observability's evaluator and monitor LLM provider pickers (`EvaluatorLlmProviderSection`, `MonitorLLMProviderDrawer`, `SelectPresetMonitors`). These now load nothing instead of calling the API, but should be hidden. |
  | `agent-runtime` | none | deployments, configurations, deploy settings, model and MCP configs, agent card, A2A proxy (all served by the AI Gateway module). Environments stay ungated on purpose: every module needs them. |
  | `ai-observability` | none | traces, metrics, runtime logs, monitors, monitor alerts, evaluators |
- **No imports across modules.** A page may import from its own module and from `libs/`. Code two modules need goes to `libs/shared-component` (for example the LLM provider form, MCP ACL helpers), or the owning module adds it as a `panel` at a mount point.

## Auth (runtime-selected)

`@…/auth`'s `index.ts` picks the implementation at **module-load time** from `globalConfig.disableAuth`:
- `false` → Asgardeo OAuth2 (`AsgardeoProvider`, real tokens/scopes).
- `true` → no-auth stub (`getToken` → empty string, always authenticated) for local dev.

`globalConfig` = `window.__RUNTIME_CONFIG__`, injected by `apps/web-ui/public/config.js` **before React mounts**. It is filled from `config.template.js`: in dev, the Vite server serves it from `apps/web-ui/.env.development` plus the process env (`vite.config.ts`, `runtimeConfig`); in production, agent-manager-service renders it (the console is embedded in its binary; each `$NAME` comes from `CONSOLE_<NAME>` or a default the service derives). To change auth mode you set `DISABLE_AUTH` in that config and reload — it cannot flip at runtime.

## Conventions & gotchas

- **UI/UX rules** (page actions, create/edit pages vs drawers, listing and empty states, form grid, labels) are in [`UX_GUIDELINES.md`](UX_GUIDELINES.md). Read it before building or changing a page.

- **UI imports** from `@wso2/oxygen-ui` (+ `@wso2/oxygen-ui-icons-react`), not `@mui/material`. Theme is `AcrylicOrangeTheme` via `OxygenUIThemeProvider`. Use theme tokens in `sx` (`color: "text.primary"`, `p: 2`).
- **No floating labels on `TextField`** — don't pass `label` to `TextField` (it produces MUI's floating/shrinking label). Use a static label instead: `FormControl` + `FormLabel` above the field (see `libs/views/src/component/FormElements/TextInput.tsx`), or a `Typography` caption for read-only display cards.
- **Forms are manual** — `useState` + inline validation; no react-hook-form/Formik. Zod appears in `core-ui` only.
- **Server state = TanStack Query**; local/UI state = `useState`. No Redux/global store.
- **Config is load-time** — anything under `globalConfig` (auth mode, API base URL, RBAC flag) is fixed once the page boots.
- **Lint** is ESLint **flat config** (`eslint.config.js` per package; no `.eslintrc`). **Tests** are Vitest (jsdom, `globals: true`).
- **Don't hand-edit `dist/`** — it's generated by `pnpm run build`.

## Done checklist

- [ ] `pnpm run lint` clean in every package you touched.
- [ ] `pnpm run build` succeeds for the changed package(s) (`make build` for cross-package changes).
- [ ] New API code follows the two-file pattern and is exported from the barrel.
- [ ] New page is registered through its `plugin.ts`, listed in its module's `src/index.ts` and in the owning backend module's `UI.Pages`.
- [ ] No import from another `ui-modules/<module>` package; shared code is in `libs/`.
- [ ] UI shown on another module's page is a contribution (`panel`, `agent-type`, `resource-summary`), not hard-coded in the host.
- [ ] Query hooks of a non-core module are gated with `useUIModulePresent`; outside callers check `useIsUIModuleInstalled` and treat `undefined` as loading.
- [ ] `plugin.ts` imports no page components.
- [ ] With the module left out of the UI manifest (e.g. in `amp-core`), its UI is gone and the network tab shows no calls to its APIs.
