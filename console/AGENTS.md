# console — agent guide

React 19 + TypeScript web UI. **pnpm workspaces monorepo**, built with **Vite**. Server state via **TanStack Query**; UI from **Oxygen UI** (WSO2's MUI 7 layer). Every package is scoped `@agent-management-platform/*` (the app shell `web-ui` is the one unscoped exception).

For the Oxygen UI component library itself (theming, which components exist, import rules), see **`.ai/oxygen-ui/AGENTS.md`** — always import UI from `@wso2/oxygen-ui`, never `@mui/material` directly.

## Monorepo layout

| Area | Package(s) | Role |
|---|---|---|
| `apps/web-ui/` | `web-ui` | App entry, Vite config, runtime `config.js`, router mount |
| `workspaces/core-ui/` | `@…/am-core-ui` | Provider stack, routing, layout, page registry |
| `workspaces/libs/api-client/` | `@…/api-client` | **All** API fetch fns + TanStack Query hooks |
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

1. **Add the fetch fn** in `libs/api-client/src/apis/<resource>.ts`.
2. **Add the hook** in `libs/api-client/src/hooks/<resource>.ts`.
3. **Export** both from the package's `apis/index.ts` / `hooks/index.ts`.
4. **Consume the hook** in a page under `workspaces/ui-modules/<module>/pages/<feature>/`.
5. **Register the page** — add a route (and nav item) contribution to the feature package's `src/plugin.ts`. A new package also goes in its module's `src/index.ts` plugin list. See "UI modules and plugins" below.

### The two-file API pattern (mandatory)

Never call `fetch`/TanStack Query directly from a component. Split into a pure fetch fn and a hook.

**`apis/<resource>.ts`** — pure fetch, takes `params` (path), `query` (search), `getToken`:
```ts
import { httpGET, SERVICE_BASE } from "../utils";

export async function listAgents(params, query, getToken) {
  const { orgName = "default", projName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/agents`,
    { searchParams: /* stringified query */, token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}
```
- HTTP helpers are `httpGET/POST/PUT/PATCH/DELETE` from `../utils` — they read `globalConfig.apiBaseUrl` and set the `Authorization: Bearer` header. Do not hand-roll `fetch`.
- `SERVICE_BASE = '/api/v1'`; the observability service uses `OBS_SERVICE_BASE = '/api'` (`utils/utils.ts`).

**`hooks/<resource>.ts`** — wrap with `useApiQuery` / `useApiMutation` (not raw `useQuery`/`useMutation`):
```ts
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiQuery, useApiMutation } from "./react-query-notifications";

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
- **`useApiQuery` / `useApiMutation`** (in `hooks/react-query-notifications.ts`) wrap TanStack Query and auto-fire success/error snackbars. `useApiMutation` takes `action: { verb, target }` to render the message.
- **`queryKey` is a tuple** — `[domain, params, query]` for detail, `[domain]` for the collection. Invalidation is **prefix-based**: `invalidateQueries({ queryKey: ["agents"] })` clears every `["agents", …]`.
- **Auth token** always comes from `useAuthHooks().getToken` and is passed into the fetch fn — never read a token another way.

## Routing

Routes live in `core-ui/src/Route/`. Use the generated maps in `@…/types`, never hardcoded strings:
- `relativeRouteMap.…path` for `<Route path=…>`.
- `absoluteRouteMap.…path` with `generatePath(path, { orgId, projectId, agentId })` for links/navigation.

Feature pages are not wired in `Route.tsx`: they come from plugin manifests (below). The shell keeps only login, the guards (`OrgGuard`, `ProjectGuard`, `AgentGuard`, which validate the entity exists before rendering the outlet), back-compat redirects, the Module manager and the 404s.

## UI modules and plugins

The console is split into six **UI modules** under `workspaces/ui-modules/`, each a product area:

| Module | Pages |
|---|---|
| `acp-core` | overview, level-resource-listing, add-new-project, add-new-agent, settings (+ events), identities |
| `agent-runtime` | configure-agent, build, deploy, test, deployment-pipelines |
| `ai-gateway` | gateways, llm-providers, mcp-proxies, agent-security (credentials) |
| `ai-observability` | traces, logs, metrics, eval |
| `agent-identity` | env-thunders (Agent ID, groups, roles) |
| `agent-portal` | agent-kind (catalog, publish) |

- **Page plugin** (`pages/<feature>/src/plugin.ts`, exported as `<feature>/plugin`): a `PluginManifest` with `route`, `nav-item` and `panel` contributions. It must not import page components; `load` uses `import()` so the page stays in its own chunk. Types are in `@…/types` (`libs/types/src/plugins`).
- **Activation**: `useActivation` on the plugin or on a contribution is a React hook (route params, `useRouteAgent()`, feature flags, access). Return `undefined` while loading.
- **Nav slots**: `section` names a slot defined in `core-ui/src/Layouts/OxygenLayout/navigationItems.tsx` (`NAV_LAYOUT`).
- **Module** (`<module>/src/index.ts`): a `UIModuleManifest` listing its page plugins; `core-ui/src/plugins/builtinModules.ts` registers modules.
- **Install and setup**: a module may set `core: true` (always installed, cannot be turned off as a whole) and `installation` (Helm `chart`, ordered `steps`, and a `useDetect` hook that reports installed / not-installed, e.g. from `/api/v1/config`). A module's pages only run while it is installed. The Marketplace (user menu → `/org/:orgId/ui-modules`) lists modules as cards with Install / Uninstall; each module has a details page at `/org/:orgId/ui-modules/<id>` (what it adds, backend setup steps, the module's optional `setup` component). Install and uninstall there are a front-end mock for now.
- **Turning off**: runtime config `uiModules.disabled` (env `DISABLED_UI_MODULES`, rendered into `config.js`) takes module ids (`ai-observability`) or `<module>/<plugin>` (`ai-observability/logs`). The Marketplace changes the install state in memory for the current tab.
- **No imports across modules.** A page may import from its own module and from `libs/`. Code two modules need goes to `libs/shared-component` (for example the LLM provider form, the catalog kind listing, MCP ACL helpers), or the owning module adds it as a `panel` at a mount point.

## Auth (runtime-selected)

`@…/auth`'s `index.ts` picks the implementation at **module-load time** from `globalConfig.disableAuth`:
- `false` → Asgardeo OAuth2 (`AsgardeoProvider`, real tokens/scopes).
- `true` → no-auth stub (`getToken` → empty string, always authenticated) for local dev.

`globalConfig` = `window.__RUNTIME_CONFIG__`, injected by `apps/web-ui/public/config.js` **before React mounts**. It is filled from `config.template.js`: in dev, the Vite server serves it from `apps/web-ui/.env.development` plus the process env (`vite.config.ts`, `runtimeConfig`); in production, the image's `40-config-setup.sh` renders it with envsubst. To change auth mode you set `DISABLE_AUTH` in that config and reload — it cannot flip at runtime.

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
- [ ] New page exports `metaData` and is routed.
