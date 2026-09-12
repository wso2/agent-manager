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
| `workspaces/pages/<feature>/` | `@…/<feature>` | One package per feature page (configure-agent, deploy, gateways, llm-providers, …) |

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
4. **Consume the hook** in a page under `workspaces/pages/<feature>/`.
5. **Register the page** — export `metaData` from the feature package's `index.ts`; wire its route in `core-ui`.

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

Feature pages are discovered by their exported `metaData` (`{ title, icon, path, component, … }`). Guards (`OrgGuard`, `ProjectGuard`, `AgentGuard`) validate the entity exists before rendering the outlet.

## Auth (runtime-selected)

`@…/auth`'s `index.ts` picks the implementation at **module-load time** from `globalConfig.disableAuth`:
- `false` → Asgardeo OAuth2 (`AsgardeoProvider`, real tokens/scopes).
- `true` → no-auth stub (`getToken` → empty string, always authenticated) for local dev.

`globalConfig` = `window.__RUNTIME_CONFIG__`, injected by `apps/web-ui/public/config.js` (rendered from `config.template.js` via env substitution) **before React mounts**. To change auth mode you set `DISABLE_AUTH` in that config and reload — it cannot flip at runtime.

## Conventions & gotchas

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
