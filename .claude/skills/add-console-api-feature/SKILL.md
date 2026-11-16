---
name: add-console-api-feature
description: Add an API-backed feature to the console (React/TypeScript web UI). Use when the user asks to call a new backend endpoint from the console, add a data-fetching hook, wire a mutation, or build a feature page that talks to agent-manager-service. Enforces the mandatory two-file API pattern (apis/ + hooks/), TanStack Query conventions, and page registration.
---

# Add an API-backed feature (console)

**Read first:** `console/AGENTS.md` → "Golden path", "The two-file API pattern", "Routing". For UI components/theming see `console/.ai/oxygen-ui/AGENTS.md`. This skill is the executable checklist.

## Steps

0. **Module** — pick `workspaces/libs/api-client/src/<module>/` by the topic of the API call (not by which page uses it); see the list in `console/AGENTS.md` → "Golden path". Environment and pipeline calls are `agent-runtime`.
1. **Fetch fn** — `workspaces/libs/api-client/src/<module>/apis/<resource>.ts`. Pure function taking `params` (path), `query` (search), `getToken`. Use the `httpGET/POST/PUT/PATCH/DELETE` helpers from `../../shared/utils` (they read `globalConfig.apiBaseUrl` and set the Bearer header) — never hand-roll `fetch`. Prefix API paths with `SERVICE_BASE` (`/api/v1`), or `OBS_SERVICE_BASE` for the observability service.
2. **Hook** — `workspaces/libs/api-client/src/<module>/hooks/<resource>.ts`. Wrap with `useApiQuery` / `useApiMutation` (from `../../shared/react-query-notifications`), **not** raw `useQuery`/`useMutation` — the wrappers auto-fire snackbars. Get the token from `useAuthHooks().getToken` and pass it into the fetch fn.
3. **Export** both from the module barrel (`<module>/index.ts`); the package root re-exports every module, so pages import from `@agent-management-platform/api-client` as before.
4. **Consume** the hook in a page under `workspaces/pages/<feature>/`.
5. **Register the page** — export `metaData` (`{ title, icon, path, component }`) from the feature package's `index.ts`; wire the route in `core-ui` using the generated route maps.

## Conventions that matter

- **`queryKey` is a tuple:** `[domain, params, query]` for a detail, `[domain]` for a collection. Invalidation is **prefix-based** — `qc.invalidateQueries({ queryKey: ["agents"] })` clears every `["agents", …]`. Invalidate in the mutation's `onSuccess`.
- **`useApiMutation` takes `action: { verb, target }`** → renders "Agent created successfully". Set it.
- **Routing:** use `relativeRouteMap.…path` for `<Route path=…>` and `absoluteRouteMap.…path` + `generatePath(path, {...})` for links. Never hardcode path strings.
- **UI imports from `@wso2/oxygen-ui`**, not `@mui/material`. Forms are manual `useState` + inline validation (no react-hook-form).

## Commands

```bash
# from inside the changed package dir:
rushx lint            # eslint (flat config)
rushx build           # build → dist/
rushx test            # vitest
# from console/:
make build            # rush build (cross-package changes)
make dev              # dev server → http://localhost:3000
```

## Done checklist

- [ ] Fetch fn + hook follow the two-file pattern and are exported from `<module>/index.ts`.
- [ ] Mutation invalidates the right prefix `queryKey` in `onSuccess`.
- [ ] Token comes from `useAuthHooks().getToken`.
- [ ] Page exports `metaData` and is routed via the generated route maps.
- [ ] `rushx lint` + `rushx build` clean for touched packages.
