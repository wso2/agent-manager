# A2A Catalogue Console List (Part B) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every organisation member an "A2A Agents" page that lists one tile per A2A agent across all projects, searchable and filterable through the URL, paged 24 at a time, on top of shared filter-bar components that the traces page is rewired onto.

**Architecture:** The catalogue HTTP contract becomes TypeScript types in `@agent-management-platform/types`, and the list/entry reads become two-file api-client operations (`apis/a2a-catalog.ts` + `hooks/a2a-catalog.ts`). Four stateless filter-bar building blocks move into `@agent-management-platform/views`; the traces page and the new catalogue page both compose them. The page is a new workspace `@agent-management-platform/a2a-agents` whose organisation-level component nests the list route, the agent-page route (a placeholder that Part C replaces) and a redirect, registered in core-ui's lazy page registry, router and navigation.

**Tech Stack:** React 19, TypeScript 5.9, react-router-dom 6.30, TanStack Query 5.90 (`keepPreviousData`), Oxygen UI 0.15 (`@wso2/oxygen-ui`, re-exports MUI 7), Vitest 3 + Testing Library, pnpm workspaces.

**Spec:** `docs/superpowers/specs/2026-10-21-a2a-catalogue-cleanroom-spec.md` — **Part B** (Console: catalogue list). Part A §3 (HTTP contract) fixes the field names; Part C §5.1 and §11 fix what the agent page will import from this plan.

## Global Constraints

- Run every command from `/home/lyuda/agent-manager-worktrees/a2a-catalogue-cleanroom` (console commands from its `console/` directory). Clean room: never read any other checkout, worktree, stash or branch other than this branch and `upstream/main`.
- Console conventions: `console/AGENTS.md` and `console/.ai/oxygen-ui/AGENTS.md`. UI imports only from `@wso2/oxygen-ui` / `@wso2/oxygen-ui-icons-react` (never `@mui/material`); theme tokens in `sx`; no `label` prop on `TextField`.
- Two-file API pattern: fetch function in `libs/api-client/src/apis/<resource>.ts` taking `(params, query, getToken)` with `httpGET` from `../utils` and `SERVICE_BASE` (`/api/v1`); hook in `libs/api-client/src/hooks/<resource>.ts` through `useApiQuery` from `./react-query-notifications`; both exported from `apis/index.ts` and `hooks/index.ts`. Query keys are tuples `[domain, params, query]`.
- Routing: `<Route path>` from `relativeRouteMap`, links from `absoluteRouteMap` + `generatePath`; never hardcode a path string in product code. `generated-route.map.ts` is produced by `pnpm --filter @agent-management-platform/types run generate-route`, never hand-edited.
- Dependent packages resolve workspace libraries from `dist/`: after changing a library run `pnpm --filter <library> run build` before testing or building its consumers. Package tests: `pnpm --filter <package> test`; lint: `pnpm --filter <package> lint`.
- Every new source file carries the WSO2 Apache-2.0 header used across the repo (copy it from a sibling file; year `2026`).
- Field names, enum values, paths and query parameters match Part A character for character: paths `/orgs/{orgName}/a2a-agents`, `/orgs/{orgName}/projects/{projName}/agents/{agentName}/a2a-catalog-entry`, `…/environments/{envID}/a2a-connect`; query `q`, `kind`, `environment`, `project`, `status`, `limit`, `offset`; enums `platform|external` (kind), `platform|external|proxied` (kind filter), `pending|fetched|failed` (status), `none|apiKey|oauth|unmanaged|unknown` (security mode), `pending|published|failed|none` (publication).
- Page size **24**; list `staleTime` **30 000 ms**; search debounce **300 ms**; panel offset **6 px**; panel width **640** (catalogue) / **760** (traces).
- Commits: imperative mood, capitalised first word, no trailing period, no `feat:` prefix, no Co-Authored-By; every message ends with the trailer line `Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q`. Stage explicit paths. Never push.

## Review Focus

1. Whitespace-only search text (`"   "`) — must behave as an empty search: no `q` written, no tile emphasised, request without `q`. (Test added to Task 8.)
2. Removing a filter chip while the panel is open — the open draft must lose that value too, so pressing Filter afterwards does not re-apply it. (Test added to Task 8.)
3. Changing page while a kind/filter is applied — the paginator write must keep every other param (`kind=external&page=1`). (Test added to Task 8.)
4. An entry whose `environments` array is empty or whose `leadEnvironment` names no environment — the tile renders title, chip and project with no caption and no environment row, and never throws. (Test added to Task 7.)
5. Garbage `page` values that `parseFloat` would half-accept (`"1abc"`, `""`, `" 2 "`) — parsing uses `Number()` semantics: `"1abc"` → 0, `""` → 0, `" 2 "` → 2. (Test added to Task 6.)

---

### Task 1: Catalogue types and route-map entries in `@agent-management-platform/types`

**Files:**
- Create: `console/workspaces/libs/types/src/api/a2a-catalog.ts`
- Modify: `console/workspaces/libs/types/src/api/index.ts` (add `export * from './a2a-catalog';` after the `./a2a-proxy` line)
- Modify: `console/workspaces/libs/types/src/routes/routes.map.ts` (new `a2aAgents` node under `org.children`, placed after `catalog`)
- Regenerate: `console/workspaces/libs/types/src/routes/generated-route.map.ts`

**Interfaces:**
- Consumes: `AgentCardSource`, `AgentCardStatus`, `AgentCardPathParams` from `./agent-card`; `AgentPathParams`, `OrgPathParams`, `ListQuery`, `PaginationMeta` from `./common`.
- Produces (all exported from `@agent-management-platform/types`; Part C imports the connect types from here and must not redefine them):

```ts
export type A2ACatalogKindFilter = AgentCardSource | 'proxied';
export interface A2ACatalogEnvironment {
  name: string;
  status: AgentCardStatus;
  /** ISO date-time; omitted when the card row was never fetched. */
  fetchedAt?: string;
  proxied: boolean;
  cardDiffers: boolean;
  /** True only when q was given and this environment's card matched it. */
  matched: boolean;
}
export interface A2ACatalogAgent {
  projectName: string;
  agentName: string;
  kind: AgentCardSource;
  name: string;
  description: string;
  version: string;
  skills: string[];
  leadEnvironment: string;
  environments: A2ACatalogEnvironment[];
}
export interface A2ACatalogListResponse extends PaginationMeta { agents: A2ACatalogAgent[]; }
export interface A2ACatalogListQuery extends ListQuery {
  q?: string;
  kind?: A2ACatalogKindFilter;
  environment?: string;
  project?: string;
  status?: AgentCardStatus;
}
export type A2ACatalogListPathParams = OrgPathParams;      // { orgName: string | undefined }
export type A2ACatalogEntryPathParams = AgentPathParams;   // { orgName, projName, agentName }
export type A2AConnectPathParams = AgentCardPathParams;    // { orgName, projName, agentName, envId }
export type A2AConnectSecurityMode = 'none' | 'apiKey' | 'oauth' | 'unmanaged' | 'unknown';
export interface A2AConnectIssuer { name: string; /** Empty when the name no longer resolves. */ issuer: string; }
export interface A2AConnectOAuth { issuers: A2AConnectIssuer[]; audiences: string[]; headerName: string; authHeaderPrefix: string; }
export interface A2AConnectSecurity { mode: A2AConnectSecurityMode; /** X-API-Key; present only when mode is apiKey. */ apiKeyHeader?: string; /** Present only when mode is oauth. */ oauth?: A2AConnectOAuth; }
export type A2AConnectPublicationStatus = 'pending' | 'published' | 'failed' | 'none';
export interface A2AConnectPublication { /** none means a direct external agent, where the gateway isn't involved. */ status: A2AConnectPublicationStatus; lastError: string; }
export interface A2AConnectResponse {
  kind: AgentCardSource;
  proxied: boolean;
  isProduction: boolean;
  /** Empty for a direct external agent or while no gateway is mapped. */
  gatewayUrl: string;
  /** The gateway's copy when the gateway fronts the agent, otherwise the card's own URL; empty when neither is known. */
  cardUrl: string;
  /** Empty when there is none. */
  jsonRpcUrl: string;
  security: A2AConnectSecurity;
  publication: A2AConnectPublication;
}
```

- Route map (relative node, key names are the contract Part C uses):

```ts
a2aAgents: {
  path: 'a2a-agents',
  index: true,
  children: {
    view: { path: 'view/:projectName/:agentName', index: true, children: {} },
  },
},
```

  Generated: `absoluteRouteMap.children.org.children.a2aAgents.path === "/org/:orgId/a2a-agents"`, `.wildPath === "/org/:orgId/a2a-agents/*"`, `.children.view.path === "/org/:orgId/a2a-agents/view/:projectName/:agentName"`, `.children.view.wildPath === ".../view/:projectName/:agentName/*"`; relative: `relativeRouteMap.children.org.children.a2aAgents.path === "a2a-agents"`, `.children.view.path === "view/:projectName/:agentName"`.

- [ ] **Step 1: Write `src/api/a2a-catalog.ts`** with the types above (doc comments as shown) and add its export to `src/api/index.ts`.

- [ ] **Step 2: Add the `a2aAgents` node** to `rootRouteMap.children.org.children` in `routes.map.ts`, directly after the `catalog` node.

- [ ] **Step 3: Regenerate and verify the route map**

Run: `cd console && pnpm --filter @agent-management-platform/types run generate-route && grep -n '"/org/:orgId/a2a-agents' workspaces/libs/types/src/routes/generated-route.map.ts`
Expected: four lines — `path` and `wildPath` for `/org/:orgId/a2a-agents` and for `/org/:orgId/a2a-agents/view/:projectName/:agentName`, nested under a key `"a2aAgents"` → `"view"`. Nothing else in the file changed (`git diff --stat` shows only the new block).

- [ ] **Step 4: Build and lint the library**

Run: `cd console && pnpm --filter @agent-management-platform/types run build && pnpm --filter @agent-management-platform/types lint`
Expected: both exit 0; `dist/api/a2a-catalog.d.ts` exists.

- [ ] **Step 5: Commit**

```bash
git add console/workspaces/libs/types/src/api/a2a-catalog.ts console/workspaces/libs/types/src/api/index.ts console/workspaces/libs/types/src/routes/routes.map.ts console/workspaces/libs/types/src/routes/generated-route.map.ts
git commit -m "Add the A2A catalogue API types and routes to the console type library

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 2: One per-environment path builder shared by the agent-card and A2A-proxy APIs

**Files:**
- Modify: `console/workspaces/libs/api-client/src/apis/agent-card.ts` (the private `agentCardPath` becomes an exported, resource-parameterised builder)
- Modify: `console/workspaces/libs/api-client/src/apis/a2a-proxy.ts` (delete `a2aProxyPath` and the `encodeRequired`/`SERVICE_BASE` imports; use the shared builder)
- Test: `console/workspaces/libs/api-client/src/apis/agent-card.test.ts` (new)

**Interfaces:**
- Produces: `export function agentEnvironmentResourcePath(params: AgentCardPathParams, resource: string): string` in `apis/agent-card.ts` (re-exported through `apis/index.ts`'s existing `export * from './agent-card'`). Returns `${SERVICE_BASE}/orgs/${org}/projects/${proj}/agents/${agent}/environments/${env}/${resource}` with each param through `encodeRequired(value, label)` using labels `orgName`, `projName`, `agentName`, `envId`. Part C's connect fetch calls it with `"a2a-connect"`.
- Unchanged exports: `getAgentCard`, `refreshAgentCard`, `setAgentCardSource`, `deleteAgentCardSource`, `getA2AProxy`, `setA2AProxy`, `deleteA2AProxy`, `refreshA2AProxy` (same URLs, same errors).

- [ ] **Step 1: Write the failing test** `src/apis/agent-card.test.ts`

```ts
vi.mock("../utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../utils")>()),
  httpGET: vi.fn(async () => ({ ok: true, json: async () => ({}) })),
}));
import { httpGET, SERVICE_BASE } from "../utils";
import { agentEnvironmentResourcePath } from "./agent-card";
import { getA2AProxy } from "./a2a-proxy";

const params = { orgName: "acme", projName: "p 1", agentName: "a/b", envId: "dev" };

it("encodes every segment and appends the resource", () => {
  expect(agentEnvironmentResourcePath(params, "a2a-connect"))
    .toBe(`${SERVICE_BASE}/orgs/acme/projects/p%201/agents/a%2Fb/environments/dev/a2a-connect`);
});
it("names the missing parameter", () => {
  expect(() => agentEnvironmentResourcePath({ ...params, envId: "" }, "agent-card"))
    .toThrow("Missing required parameter: envId");
});
it("keeps the A2A proxy URL", async () => {
  await getA2AProxy(params, async () => "t");
  expect(httpGET).toHaveBeenCalledWith(
    `${SERVICE_BASE}/orgs/acme/projects/p%201/agents/a%2Fb/environments/dev/a2a-proxy`,
    { token: "t" },
  );
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd console && pnpm --filter @agent-management-platform/api-client test -- src/apis/agent-card.test.ts`
Expected: FAIL — `agentEnvironmentResourcePath` is not exported.

- [ ] **Step 3: Implement the builder** in `apis/agent-card.ts`; keep a one-line private `agentCardPath = (p) => agentEnvironmentResourcePath(p, "agent-card")`. In `apis/a2a-proxy.ts` replace every `a2aProxyPath(params)` with `agentEnvironmentResourcePath(params, "a2a-proxy")` (import from `./agent-card`) and drop the imports it no longer needs.

- [ ] **Step 4: Run the api-client tests and lint**

Run: `cd console && pnpm --filter @agent-management-platform/api-client test && pnpm --filter @agent-management-platform/api-client lint`
Expected: all green, including the existing `hooks/a2a-proxy.test.ts` and `hooks/agent-card-mutations.test.ts`; lint reports no unused imports.

- [ ] **Step 5: Commit**

```bash
git add console/workspaces/libs/api-client/src/apis/agent-card.ts console/workspaces/libs/api-client/src/apis/a2a-proxy.ts console/workspaces/libs/api-client/src/apis/agent-card.test.ts
git commit -m "Share one per-environment path builder between the agent-card and A2A proxy APIs

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 3: Catalogue list and entry operations in the api-client

**Files:**
- Create: `console/workspaces/libs/api-client/src/apis/a2a-catalog.ts`
- Create: `console/workspaces/libs/api-client/src/hooks/a2a-catalog.ts`
- Modify: `console/workspaces/libs/api-client/src/apis/index.ts`, `src/hooks/index.ts` (add `export * from './a2a-catalog';` after the `./a2a-proxy` line in each)
- Test: `console/workspaces/libs/api-client/src/hooks/a2a-catalog.test.ts`

**Interfaces:**
- Consumes: Task 1 types; `httpGET`, `SERVICE_BASE`, `encodeRequired`, `nullIfNotFound` from `../utils`; `useApiQuery` from `./react-query-notifications`; `useAuthHooks` from `@agent-management-platform/auth`; `keepPreviousData` from `@tanstack/react-query`.
- Produces (exported from `@agent-management-platform/api-client`):

```ts
// apis/a2a-catalog.ts
export async function listA2ACatalog(params: A2ACatalogListPathParams, query: A2ACatalogListQuery, getToken?: () => Promise<string>): Promise<A2ACatalogListResponse>;
export async function getA2ACatalogEntry(params: A2ACatalogEntryPathParams, getToken?: () => Promise<string>): Promise<A2ACatalogAgent>; // 404 propagates as an error with status 404

// hooks/a2a-catalog.ts
export const A2A_CATALOG_STALE_MS = 30_000;
export const a2aCatalogListQueryKey = (params: A2ACatalogListPathParams, query: A2ACatalogListQuery) => ["a2a-agents", params, query] as const;
export const a2aCatalogEntryQueryKey = (params: A2ACatalogEntryPathParams) => ["a2a-catalog-entry", params] as const;
export function useListA2ACatalog(params: A2ACatalogListPathParams, query: A2ACatalogListQuery): UseQueryResult<A2ACatalogListResponse>;
export function useGetA2ACatalogEntry(params: A2ACatalogEntryPathParams): UseQueryResult<A2ACatalogAgent | null>;
```

  `useListA2ACatalog`: `enabled: !!params.orgName`, `placeholderData: keepPreviousData`, `staleTime: A2A_CATALOG_STALE_MS`, not silent, default retry. `useGetA2ACatalogEntry`: `enabled` when all three params are non-empty, `queryFn: () => nullIfNotFound(getA2ACatalogEntry(params, getToken))`, `staleTime: A2A_CATALOG_STALE_MS`, `retry: false`, `silent: true`.
- Reserved for Part C (do **not** add here): `getA2AConnect` in the same `apis/a2a-catalog.ts`, `useGetA2AConnect` (key `["a2a-connect", params]`) in the same `hooks/a2a-catalog.ts`, and the on-demand test-key mutation in `hooks/agent-api-keys.ts`.

- [ ] **Step 1: Write the failing hook tests** `src/hooks/a2a-catalog.test.ts`, using the probe/`QueryClientProvider` mount helper from `hooks/a2a-proxy.test.ts` and these mocks: `../utils` partially mocked with `httpGET: vi.fn(async () => ({ ok: true, json: async () => ({}) }))`; `@agent-management-platform/auth` → `getToken: async () => "t"`; `@agent-management-platform/views` → `useSnackBar: () => ({ pushSnackBar })` with one shared `pushSnackBar = vi.fn()`; `./telemetry` as in the sibling test.

```ts
it("sends only the defined, non-empty query values as strings", async () => {
  await mount(() => useListA2ACatalog({ orgName: "acme" }, { q: "refund", kind: undefined, limit: 24, offset: 0 }));
  await settle();
  expect(httpGET).toHaveBeenCalledWith(`${SERVICE_BASE}/orgs/acme/a2a-agents`,
    { searchParams: { q: "refund", limit: "24", offset: "0" }, token: "t" });
});
it("forwards the four filters", async () => {  // searchParams exactly { kind: "proxied", environment: "dev", project: "p", status: "failed" }
});
it("passes no searchParams when the query is empty", async () => {  // useListA2ACatalog({ orgName: "acme" }, {}) → second arg is { searchParams: undefined, token: "t" }
});
it("does not request without an organisation", async () => {  // orgName: "" → httpGET not called
});
it("resolves a missing entry to null without an error", async () => {
  vi.mocked(httpGET).mockRejectedValueOnce(Object.assign(new Error("nf"), { status: 404 }));
  const { qc } = await mount(() => useGetA2ACatalogEntry(entry));   // entry = { orgName: "acme", projName: "p1", agentName: "orders" }
  await settle();
  expect(httpGET).toHaveBeenCalledWith(`${SERVICE_BASE}/orgs/acme/projects/p1/agents/orders/a2a-catalog-entry`, { token: "t" });
  expect(qc.getQueryData(a2aCatalogEntryQueryKey(entry))).toBeNull();
  expect(qc.getQueryState(a2aCatalogEntryQueryKey(entry))?.status).toBe("success");
});
it("reports other entry failures silently", async () => {  // status 500 → query state "error", pushSnackBar not called
});
it("does not request an entry with a missing segment", async () => {  // projName: "" → httpGET not called
});
```

  `settle()` is `act(async () => { await new Promise((r) => setTimeout(r, 20)); })`.

- [ ] **Step 2: Run them to verify they fail**

Run: `cd console && pnpm --filter @agent-management-platform/api-client test -- src/hooks/a2a-catalog.test.ts`
Expected: FAIL — module `./a2a-catalog` not found.

- [ ] **Step 3: Implement `apis/a2a-catalog.ts`.** A private `catalogSearchParams(query: A2ACatalogListQuery): Record<string, string> | undefined` drops entries whose value is `undefined` or `""`, stringifies the rest, and returns `undefined` when nothing remains; `listA2ACatalog` calls `httpGET(`${SERVICE_BASE}/orgs/${encodeRequired(params.orgName, "orgName")}/a2a-agents`, { searchParams, token })`; `getA2ACatalogEntry` encodes the three segments with labels `orgName`, `projName`, `agentName`.

- [ ] **Step 4: Implement `hooks/a2a-catalog.ts`** per the Interfaces block and add both barrel exports.

- [ ] **Step 5: Run tests, lint, build**

Run: `cd console && pnpm --filter @agent-management-platform/api-client test && pnpm --filter @agent-management-platform/api-client lint && pnpm --filter @agent-management-platform/api-client run build`
Expected: all pass; `dist/hooks/a2a-catalog.d.ts` exists.

- [ ] **Step 6: Commit**

```bash
git add console/workspaces/libs/api-client/src/apis/a2a-catalog.ts console/workspaces/libs/api-client/src/apis/index.ts console/workspaces/libs/api-client/src/hooks/a2a-catalog.ts console/workspaces/libs/api-client/src/hooks/index.ts console/workspaces/libs/api-client/src/hooks/a2a-catalog.test.ts
git commit -m "Add the A2A catalogue list and entry hooks to the api-client

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 4: Shared filter bar, toggle, chip and panel in `@agent-management-platform/views`

**Files:**
- Replace: `console/workspaces/libs/views/vitest.config.ts` (today it holds only the licence header, so `vitest run` aborts with `config must export or return an object`; make it identical to `console/workspaces/pages/agent-security/vitest.config.ts`)
- Replace: `console/workspaces/libs/views/setupTests.tsx` with the sibling's one line `import '@testing-library/jest-dom';` (the current file imports a `./src/theme` module that does not exist; nothing else references its exports)
- Create: `console/workspaces/libs/views/src/component/FilterBar/FilterBar.tsx`, `FilterToggle.tsx`, `FilterChip.tsx`, `FilterPanel.tsx`, `index.ts`
- Modify: `console/workspaces/libs/views/src/component/index.ts` (add `export * from './FilterBar';`)
- Test: `console/workspaces/libs/views/src/component/FilterBar/FilterBar.test.tsx`

**Interfaces:**
- Consumes: Oxygen `Box`, `Stack`, `ToggleButtonGroup`, `ToggleButton`, `ButtonGroup`, `Button`, `Popper`, `Grow`, `Paper`, `IconButton`, `Typography`; icon `X` from `@wso2/oxygen-ui-icons-react`.
- Produces (exported from `@agent-management-platform/views`):

```ts
export interface FilterBarProps { actions: ReactNode; children?: ReactNode }
export const FilterBar: React.FC<FilterBarProps>;

export interface FilterToggleOption { value: string; label: string }
export interface FilterToggleProps { label: string; value?: string; options: FilterToggleOption[]; onChange: (value?: string) => void }
export const FilterToggle: React.FC<FilterToggleProps>;

export interface FilterChipProps { name: string; value: string; label?: string; onOpen: () => void; onRemove: () => void }
export const FilterChip: React.FC<FilterChipProps>;

export interface FilterPanelProps {
  anchorEl: HTMLElement; id: string; title: string; width: number; applyDisabled?: boolean;
  onReset: () => void; onApply: () => void; onClose: () => void; children: ReactNode;
}
export const FilterPanel: React.FC<FilterPanelProps>;
```

  Behaviour is spec Part B §5.1–§5.4 exactly; the panel's CSS custom property is `--filter-panel-max-height`; the close button's label is `"Close " + title.toLowerCase()`.

- [ ] **Step 1: Repair the views test runner** (replace `vitest.config.ts` and `setupTests.tsx` as listed) and confirm it starts:

Run: `cd console && pnpm --filter @agent-management-platform/views test`
Expected: vitest starts and reports `No test files found` (exit code 1 for that reason only — no config error).

- [ ] **Step 2: Write the failing tests** `FilterBar.test.tsx` (render inside `<ThemeProvider theme={createTheme()}>` from `@wso2/oxygen-ui`, as `pages/traces/src/Traces.test.tsx` does):

```ts
describe("FilterToggle", () => {
  // label "Kind", options Platform/External/Proxied
  it("renders All pressed when no value is set", ...);          // getByRole("group", { name: "Kind" }); buttons All, Platform, External, Proxied; All has aria-pressed="true"
  it("emits the option value", ...);                             // click External → onChange("external")
  it("emits undefined for All", ...);                            // value="external", click All → onChange(undefined)
  it("ignores a click on the pressed button", ...);              // value="external", click External → onChange not called
});
describe("FilterChip", () => {
  it("shows name and value and names its remove button", ...);  // name "Card status", value "Failed", label "Card status: Failed" → getByTitle("Card status: Failed") contains both texts; getByRole("button", { name: "Remove Card status: Failed" })
  it("calls onOpen and onRemove", ...);
  it("defaults the label to name and value", ...);              // no label → title "Card status Failed"
});
describe("FilterPanel", () => {
  // anchorEl = a <button> appended to document.body; id "x"; title "Advanced Filters"; width 640
  it("is a focused aside with heading, close, Reset and Filter", ...);
  // getByRole("complementary", { name: "Advanced Filters" }) has id "x" and toHaveFocus();
  // getByRole("heading", { level: 2, name: "Advanced Filters" }); button "Close advanced filters" → onClose;
  // button "Reset" → onReset; button "Filter" → onApply
  it("disables Filter when applyDisabled", ...);
  it("closes on Escape anywhere", ...);                           // fireEvent.keyDown(document, { key: "Escape" }) → onClose once
});
describe("FilterBar", () => {
  it("renders actions before children", ...);                   // actions <span>act</span>, children <span>child</span>: compareDocumentPosition → actions precede
});
```

- [ ] **Step 3: Run them to verify they fail**

Run: `cd console && pnpm --filter @agent-management-platform/views test -- src/component/FilterBar`
Expected: FAIL — module `./FilterBar` not found.

- [ ] **Step 4: Implement the four components** per spec §5.1–§5.4 (one file each, `index.ts` re-exporting all four with their prop types) and export the folder from `component/index.ts`.

- [ ] **Step 5: Run tests, lint, build**

Run: `cd console && pnpm --filter @agent-management-platform/views test && pnpm --filter @agent-management-platform/views lint && pnpm --filter @agent-management-platform/views run build`
Expected: all pass; `dist/component/FilterBar/index.d.ts` exists.

- [ ] **Step 6: Commit**

```bash
git add console/workspaces/libs/views/vitest.config.ts console/workspaces/libs/views/setupTests.tsx console/workspaces/libs/views/src/component/FilterBar console/workspaces/libs/views/src/component/index.ts
git commit -m "Add shared filter bar, toggle, chip and panel components to the views library

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 5: Rewire the traces filter bar and drawer onto the shared components

**Files:**
- Modify: `console/workspaces/pages/traces/src/subComponents/TraceFilterBar.tsx`
- Modify: `console/workspaces/pages/traces/src/subComponents/TraceFiltersDrawer.tsx`
- Test (unchanged, must pass): `console/workspaces/pages/traces/src/Traces.test.tsx`

**Interfaces:**
- Consumes: `FilterBar`, `FilterToggle`, `FilterChip`, `FilterPanel` from `@agent-management-platform/views` (Task 4; the traces package already depends on views).
- Produces: unchanged — `TraceFilterBar` (`TraceFilterBarProps`) and `TraceFiltersDrawer` (`TraceFiltersDrawerProps`, `TRACE_FILTERS_DRAWER_ID = "trace-filters-drawer"`).

- [ ] **Step 1: Run the traces tests before touching anything** to record the baseline

Run: `cd console && pnpm --filter @agent-management-platform/traces test`
Expected: PASS (all).

- [ ] **Step 2: Rewrite `TraceFilterBar.tsx`**: delete the local `StatusToggle`, `FilterChip` and the layout `Box`/`Stack`; render `<FilterBar actions={<>{onTraceSearch && <TraceIdSearch …/>}<Button …>Advanced Filters</Button></>}>` with children `<FilterToggle label="Status" value={filters.status} options={[{ value: "error", label: "Errors" }, { value: "ok", label: "Success" }]} onChange={(status) => onChange({ ...filters, status: status === "error" || status === "ok" ? status : undefined })} />`, the chips as `<FilterChip name={chip.name} value={chip.value} label={chip.label} onOpen={openDrawer} onRemove={() => remove(chip)} />`, and the `Clear all` button under the same `chips.length + (filters.status ? 1 : 0) >= 2` condition. The Filters button, draft, open/remove/clearAll/apply logic and `{children}` / drawer rendering stay as they are.

- [ ] **Step 3: Rewrite `TraceFiltersDrawer.tsx`**: keep `TRACE_FILTERS_DRAWER_ID`, `valid`, `submit`, `sectionProps` and the three section components; render `<FilterPanel anchorEl={anchorEl} id={TRACE_FILTERS_DRAWER_ID} title="Advanced Filters" width={760} applyDisabled={!valid} onReset={() => onDraftChange(EMPTY_DRAFT)} onApply={submit} onClose={onClose}>` whose children are the two-column `Stack` (direction `{ xs: "column", sm: "row" }`, spacing 3, vertical `Divider`) without the old scroll/padding `sx`. Delete the Popper/Grow/Paper/IconButton/Typography/CloseIcon/`useCallback`/`useEffect` imports and the `PANEL_MODIFIERS` block.

- [ ] **Step 4: Run the traces tests and lint**

Run: `cd console && pnpm --filter @agent-management-platform/traces test && pnpm --filter @agent-management-platform/traces lint`
Expected: the same tests pass as in Step 1; lint clean (no unused imports).

- [ ] **Step 5: Commit**

```bash
git add console/workspaces/pages/traces/src/subComponents/TraceFilterBar.tsx console/workspaces/pages/traces/src/subComponents/TraceFiltersDrawer.tsx
git commit -m "Rewire the traces filter bar onto the shared filter components

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 6: Scaffold the `a2a-agents` page workspace with its URL, label and query helpers

**Files:**
- Create: `console/workspaces/pages/a2a-agents/package.json`, `vite.config.ts`, `vitest.config.ts`, `tsconfig.json`, `tsconfig.lib.json`, `eslint.config.js`, `postcss.config.cjs`, `setupTests.tsx` — each copied from `console/workspaces/pages/agent-security/` and adjusted as below
- Create: `console/workspaces/pages/a2a-agents/src/routes.ts`
- Create: `console/workspaces/pages/a2a-agents/src/utils/useReplaceSearchParams.ts`, `src/utils/useEnvironmentLabel.ts`
- Create: `console/workspaces/pages/a2a-agents/src/catalogue/catalogueQuery.ts`
- Create: `console/workspaces/pages/a2a-agents/src/index.ts` (temporary: re-exports `routes.ts` only, so the package builds; Task 9 writes the final barrel)
- Test: `src/catalogue/catalogueQuery.test.ts`, `src/utils/useReplaceSearchParams.test.tsx`, `src/utils/useEnvironmentLabel.test.tsx`
- Modify: `console/pnpm-lock.yaml` (via `pnpm install`)

**Interfaces:**
- Consumes: `absoluteRouteMap` (Task 1), `A2ACatalogListQuery` from `@agent-management-platform/types`; `useListEnvironments` from `@agent-management-platform/api-client`; `useSearchParams`, `generatePath` from `react-router-dom`.
- Produces — package `@agent-management-platform/a2a-agents` at `console/workspaces/pages/a2a-agents/` (Part C adds its files inside this package and imports these relatively):

```ts
// src/routes.ts
export const a2aAgentsRoute = absoluteRouteMap.children.org.children.a2aAgents;   // .path, .wildPath
export const a2aAgentViewRoute = a2aAgentsRoute.children.view;                    // .path, .wildPath
export interface A2AAgentRouteParams { orgId: string; projectName: string; agentName: string }
export const a2aAgentsListPath = (orgId: string): string => generatePath(a2aAgentsRoute.path, { orgId });
export const a2aAgentViewPath = (params: A2AAgentRouteParams): string => generatePath(a2aAgentViewRoute.path, params);

// src/utils/useReplaceSearchParams.ts
export type SearchParamsMutation = (params: URLSearchParams) => void;
/** [current params, updater]; the updater applies the mutation to a copy of the latest params and replaces history. */
export function useReplaceSearchParams(): [URLSearchParams, (mutate: SearchParamsMutation) => void];

// src/utils/useEnvironmentLabel.ts
export type EnvironmentLabel = (environmentName: string) => string;
export function useEnvironmentLabel(orgName: string): EnvironmentLabel;   // displayName when non-empty, else name; unknown names map to themselves

// src/catalogue/catalogueQuery.ts
export const PAGE_SIZE = 24;
export type AdvancedFilterKey = "environment" | "project" | "status";
export const ADVANCED_FILTER_KEYS: readonly AdvancedFilterKey[] = ["environment", "project", "status"];
export interface CatalogueFilters { kind: string; environment: string; project: string; status: string }
export const EMPTY_FILTERS: CatalogueFilters;
export function readCatalogueFilters(params: URLSearchParams): CatalogueFilters;      // params.get(key) ?? "" for each
export function parsePageParam(raw: string | null): number;                            // Number(); NaN/negative → 0; fraction truncated
export function lastPageIndex(total: number): number;                                  // max(0, ceil(total / PAGE_SIZE) - 1)
export function isFiltering(filters: CatalogueFilters, search: string): boolean;       // any filter non-empty or search non-empty
export function catalogueListQuery(filters: CatalogueFilters, search: string, page: number): A2ACatalogListQuery;
export function setOrDelete(params: URLSearchParams, key: string, value: string): void; // delete when value === ""
export function writePage(params: URLSearchParams, page: number): void;                 // setOrDelete(params, "page", page > 0 ? String(page) : "")
```

- [ ] **Step 1: Create the workspace files.** `package.json`: `name` `@agent-management-platform/a2a-agents`, `description` `A2A Agents catalogue page for the organization level`, scripts identical to the sibling, dependencies `react-dom`, `react-router-dom`, `@mui/material`, `@emotion/react`, `@emotion/styled`, `@wso2/oxygen-ui`, `@wso2/oxygen-ui-icons-react` at the sibling's versions plus `workspace:*` on `@agent-management-platform/views`, `/types`, `/shared-component`, `/api-client` (omit the sibling's `clsx`, `date-fns`, `zod`, `@mui/icons-material`); devDependencies and peerDependencies copied verbatim from the sibling. `tsconfig.lib.json` path alias `@a2a-agents/*`. The other config files are verbatim copies. `setupTests.tsx` is `import '@testing-library/jest-dom';`.

- [ ] **Step 2: Link the new workspace**

Run: `cd console && pnpm install`
Expected: exit 0; `pnpm-lock.yaml` gains an importer `workspaces/pages/a2a-agents`; `workspaces/pages/a2a-agents/node_modules/@agent-management-platform/views` is a symlink.

- [ ] **Step 3: Write the failing tests**

`src/catalogue/catalogueQuery.test.ts`:
```ts
it.each([["-1", 0], ["1.5", 1], ["abc", 0], [null, 0], ["1abc", 0], ["", 0], [" 2 ", 2], ["3", 3]])
  ("parsePageParam(%j) → %i", (raw, expected) => expect(parsePageParam(raw)).toBe(expected));
it.each([[0, 0], [20, 0], [24, 0], [25, 1], [50, 2]])("lastPageIndex(%i) → %i", ...);
it("builds the request with undefined for empty values", () => {
  expect(catalogueListQuery(EMPTY_FILTERS, "", 2))
    .toEqual({ kind: undefined, environment: undefined, project: undefined, status: undefined, q: undefined, limit: 24, offset: 48 });
  expect(catalogueListQuery({ ...EMPTY_FILTERS, kind: "proxied" }, "refund", 0))
    .toEqual(expect.objectContaining({ kind: "proxied", q: "refund", limit: 24, offset: 0 }));
});
it("reads the four filters raw", () => expect(readCatalogueFilters(new URLSearchParams("kind=bogus&status=failed"))).toEqual({ kind: "bogus", environment: "", project: "", status: "failed" }));
it("isFiltering is true for a filter or a search", ...);   // (EMPTY_FILTERS, "") false; (…status "failed", "") true; (EMPTY_FILTERS, "x") true
it("writePage omits page 0", () => { /* writePage(p, 0) deletes "page"; writePage(p, 2) sets "2" */ });
```

`src/utils/useReplaceSearchParams.test.tsx` (MemoryRouter at `/list?a=1`, a probe component rendering `useLocation().search` and exposing the updater and `useNavigate`):
```ts
it("replaces instead of pushing and composes writes in one tick", () => {
  act(() => { update((p) => p.set("b", "2")); update((p) => p.set("c", "3")); });
  expect(search()).toBe("?a=1&b=2&c=3");
  act(() => navigate(-1));                 // nothing to go back to when history was replaced
  expect(search()).toBe("?a=1&b=2&c=3");
});
```

`src/utils/useEnvironmentLabel.test.tsx` (mock `@agent-management-platform/api-client` → `useListEnvironments: vi.fn()`):
```ts
it("prefers a non-empty display name", () => { /* data [{name:"prod",displayName:"Production"},{name:"blank",displayName:""}] → label("prod")==="Production", label("blank")==="blank", label("dev")==="dev" */ });
it("returns names unchanged while the list is missing", () => { /* data undefined → label("prod")==="prod" */ });
```

- [ ] **Step 4: Run them to verify they fail**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test`
Expected: FAIL — modules not found.

- [ ] **Step 5: Implement `routes.ts`, the two hooks and `catalogueQuery.ts`** per the Interfaces block. `useReplaceSearchParams` calls `setSearchParams((prev) => { const next = new URLSearchParams(prev); mutate(next); return next; }, { replace: true })`. `useEnvironmentLabel` builds the lookup from `useListEnvironments({ orgName }).data` on every render. Temporary `src/index.ts`: `export * from './routes';`.

- [ ] **Step 6: Run tests, lint, build**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test && pnpm --filter @agent-management-platform/a2a-agents lint && pnpm --filter @agent-management-platform/a2a-agents run build`
Expected: all pass; `dist/index.js` exists.

- [ ] **Step 7: Commit**

```bash
git add console/pnpm-lock.yaml console/workspaces/pages/a2a-agents
git commit -m "Scaffold the A2A agents page workspace with its URL and label helpers

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 7: The catalogue tile

**Files:**
- Create: `console/workspaces/pages/a2a-agents/src/catalogue/A2AAgentTile.tsx`
- Test: `console/workspaces/pages/a2a-agents/src/catalogue/A2AAgentTile.test.tsx`

**Interfaces:**
- Consumes: `A2ACatalogAgent`, `A2ACatalogEnvironment` types; `a2aAgentViewPath`, `EnvironmentLabel` (Task 6); Oxygen `Form.CardButton`, `Form.CardHeader`, `Form.CardContent`, `Chip`, `Tooltip`, `Typography`, `Stack`, `Box`, `Divider`; router `Link`.
- Produces:

```ts
export interface A2AAgentTileProps { orgId: string; agent: A2ACatalogAgent; emphasised: boolean; environmentLabel: EnvironmentLabel }
export const A2AAgentTile: React.FC<A2AAgentTileProps>;
export const TILE_TEST_ID = "a2a-agent-tile";                              // on the Link; it also carries data-emphasised="true"|"false"
export const envStatusTestId = (name: string) => `a2a-env-status-${name}`;  // on the 8×8 status dot
```

- [ ] **Step 1: Write the failing tests** (render inside `MemoryRouter` + Oxygen `ThemeProvider`; a base entry `{ projectName: "p1", agentName: "orders", kind: "platform", name: "Orders", description: "d", version: "1", skills: [], leadEnvironment: "dev", environments: [dev fetched] }`):

```ts
it("links to the agent page and shows kind and project", ...);   // getByTestId("a2a-agent-tile") href "/org/acme/a2a-agents/view/p1/orders"; texts "platform", "p1", heading "Orders"
it("falls back to the agent name when name is empty", ...);
it("shows two skills and a tooltip with all of them", async ...); // 5 skills → chips s1, s2, caption "+3 more"; hover → tooltip "s1, s2, s3, s4, s5"
it("shows no overflow caption for two skills and no row for none", ...);
it.each([["pending", "card pending"], ["failed", "card failed"]])("captions a %s lead card", ...);
it("shows no caption for a fetched lead card", ...);
it("renders environment chips, dots and labels", ...);           // envs dev(fetched, proxied) + prod(failed, cardDiffers) with environmentLabel = n => n === "prod" ? "Production" : n
   // → texts "proxied", "card differs", "Production"; getByTestId("a2a-env-status-dev") has title "fetched"; prod dot title "failed"
it("marks emphasis through data-emphasised", ...);               // emphasised true → "true"; false → "false"
it("survives an entry with no environments", ...);               // environments: [] and leadEnvironment "" → heading rendered, no caption, no status dots (Review Focus 4)
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test -- src/catalogue/A2AAgentTile`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the tile** per spec §4.4 (header: title `h6` noWrap with top tooltip; second line: small outlined primary `Chip` with raw `kind`, vertical divider, project caption, warning caption for a non-fetched lead; body: 2-line-clamped description, skills row, environments row with status dots `success.main` / `warning.main` / `error.main`, `proxied` and warning `card differs` small chips). Small chips: height 20, font size 0.7rem, label padding-x 0.75. Grid-friendly: `Form.CardButton` `selected={emphasised}`, `width: "100%"`, `height: "100%"`, `minHeight: 200`, column flex.

- [ ] **Step 4: Run tests and lint**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test && pnpm --filter @agent-management-platform/a2a-agents lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add console/workspaces/pages/a2a-agents/src/catalogue/A2AAgentTile.tsx console/workspaces/pages/a2a-agents/src/catalogue/A2AAgentTile.test.tsx
git commit -m "Add the A2A catalogue tile

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 8: The catalogue list — filter bar, URL state, pagination and clamp

**Files:**
- Create: `console/workspaces/pages/a2a-agents/src/catalogue/A2AAgentFilterBar.tsx`
- Create: `console/workspaces/pages/a2a-agents/src/catalogue/A2AAgentCatalogue.tsx`
- Test: `console/workspaces/pages/a2a-agents/src/catalogue/A2AAgentCatalogue.test.tsx`

**Interfaces:**
- Consumes: Task 6 helpers; `A2AAgentTile` (Task 7); `FilterBar`, `FilterToggle`, `FilterChip`, `FilterPanel` (Task 4); `useListA2ACatalog` (Task 3), `useListEnvironments`, `useListProjects` from `@agent-management-platform/api-client`; `ResourceListShell` from `@agent-management-platform/shared-component`; Oxygen `SearchBar`, `Button`, `Select`, `MenuItem`, `Stack`, `Typography`, `Box`, `Skeleton`, `TablePagination`; icons `ListFilter`, `Bot`, `Search`.
- Produces:

```ts
// A2AAgentFilterBar.tsx — owns the draft and the open flag; the catalogue owns the applied (URL) values
export const A2A_FILTER_PANEL_ID = "a2a-agents-filter-panel";
export type AdvancedFilters = Record<AdvancedFilterKey, string>;
export interface A2AAgentFilterBarProps {
  search: string; onSearchChange: (value: string) => void;
  filters: CatalogueFilters;                          // applied
  onKindChange: (kind: string | undefined) => void;   // applies immediately
  onApply: (advanced: AdvancedFilters) => void;       // one write; the catalogue keeps the applied kind
  onRemove: (key: AdvancedFilterKey) => void;
  onClearAll: () => void;
  environments: Environment[]; projects: ProjectResponse[];
}
export const A2AAgentFilterBar: React.FC<A2AAgentFilterBarProps>;

// A2AAgentCatalogue.tsx
export interface A2AAgentCatalogueProps { orgId: string }
export const A2AAgentCatalogue: React.FC<A2AAgentCatalogueProps>;
```

  Copy (spec §10c): search placeholder `Search A2A agents`; toggle `Kind` / `All`, `Platform`, `External`, `Proxied`; button and panel title `Advanced Filters`; selects `Environment`, `Project`, `Card status` with empty option `Any` and status options `Fetched`, `Pending`, `Failed`; chip label `<name>: <value>`; `Clear all`; empty state `No A2A agents yet` / `Agents that expose an A2A card show up here once deployed.`; no-match `No A2A agents match your search` / `Try a different keyword or clear the filters.`; `resourceName="a2a-agents"`.

- [ ] **Step 1: Write the failing tests** `A2AAgentCatalogue.test.tsx`. Harness: `vi.mock("@agent-management-platform/api-client", () => ({ useListA2ACatalog: vi.fn(), useListEnvironments: vi.fn(() => ({ data: [] })), useListProjects: vi.fn(() => ({ data: { projects: [] } })) }))`; `vi.mock("@agent-management-platform/shared-component", () => ({ ResourceListShell: (p) => <div>{p.toolbar}{p.error ? <div>error</div> : p.isLoading ? p.loadingPlaceholder : p.isEmpty ? <div>{p.emptyState.title}</div> : p.isSearchEmpty ? <div>{p.searchEmptyState.title}</div> : p.children}</div> }))`; render `<ThemeProvider theme={createTheme()}><MemoryRouter initialEntries={[`/org/acme/a2a-agents${search}`]}><Routes><Route path="/org/:orgId/a2a-agents" element={<><A2AAgentCatalogue orgId="acme" /><SearchProbe /></>} /></Routes></MemoryRouter></ThemeProvider>`; `lastQuery = () => vi.mocked(useListA2ACatalog).mock.lastCall?.[1]`; default response: two entries both `agentName: "orders"` in `p1` (envs `dev`, `prod` with `prod.cardDiffers = true`) and `p2` (env `dev`, `proxied = true`), `total: 2`; `vi.useFakeTimers()` only inside the search `describe`.

```ts
describe("grid", () => {
  it("renders one tile per project/agent pair", ...);                          // 2 tiles; hrefs /org/acme/a2a-agents/view/p1/orders and /p2/orders; first contains "p1"
  it("shows environment flags and display names", ...);                        // useListEnvironments → [{ name: "prod", displayName: "Production" }]: "card differs", "proxied", "Production" present, text "prod" absent, "dev" twice
  it("emphasises matched tiles only while searching", ...);                     // ?q=x with env matched → data-emphasised "true" on that tile, "false" on the other; no q → all "false"
  it("renders four skeletons while loading", ...);                             // { data: undefined, isLoading: true } → 4 role-less skeletons by data-testid "a2a-tile-skeleton"
  it("renders the error state", ...);                                           // { error: new Error("x") } → text "error"
  it("renders the empty and no-match states", ...);                            // total 0 & no filters → "No A2A agents yet"; ?status=failed → "No A2A agents match your search"
  it("pages only beyond 24", ...);                                              // total 2 → no "Go to next page" button; total 25 → button exists, clicking writes page=1 and lastQuery().offset === 24
  it("keeps other params when paging", ...);                                   // ?kind=external, total 25, next page → params kind=external&page=1 (Review Focus 3)
});
describe("page parsing and clamp", () => {
  it.each([["-1", 0], ["1.5", 24], ["abc", 0], ["", 0]])("?page=%s requests offset %i", ...);
  it("clamps to the last page when the page is empty", ...);                   // ?page=3 + { agents: [], total: 20 } → page removed, lastQuery().offset === 0
  it("clamps to page 2 of 50", ...);                                            // ?page=5 + { agents: [], total: 50 } → page=2, offset 48
  it("never clamps an empty catalogue or page 0", ...);                        // ?page=2 + total 0 → search unchanged; no page + { agents: [], total: 20 } → unchanged
});
describe("search", () => {
  it("debounces typing through the real search box", ...);                     // type "refund" into placeholder "Search A2A agents" → no call with q "refund"; advance 300 ms → lastQuery() toEqual({ kind: undefined, environment: undefined, project: undefined, status: undefined, q: "refund", limit: 24, offset: 0 })
  it("seeds the box from the URL", ...);                                        // ?q=old → input value "old", first query q "old"
  it("trims and drops the page", ...);                                          // ?q=old&page=2, type "  refund  " → q=refund, no page
  it("removes q when cleared or whitespace", ...);                             // clear → no q param, no q in request; "   " → same (Review Focus 1)
});
describe("kind toggle and advanced filters", () => {
  it("applies kind immediately", ...);                                          // click External → kind=external, no page, lastQuery().kind "external"; click All → kind removed
  it("applies status only on Filter and removes it from the chip", ...);       // open panel, mouseDown combobox "Card status", click option "Failed" → status undefined; click "Filter" → status "failed", panel gone; click "Remove Card status: Failed" → undefined
  it("offers environment and project options and chips", ...);                 // env [{ dev, "Development" }], project [{ p1, "Project One" }] → options; apply both → environment=dev&project=p1, chips "Environment: Development", "Project: Project One"
  it("shows a raw chip for an unknown environment", ...);                      // ?environment=ghost → title "Environment: ghost"
  it("shows Clear all for two active filters and clears in one write", ...);   // ?status=failed → no Clear all; ?status=failed&kind=platform → present; click → searchHistory grows by exactly 1 and ends with ""; lastQuery() has no filters
  it("discards the draft on Escape and close", ...);                           // open, pick Failed, Escape → nothing written; reopen → combobox shows "Any"; same via "Close advanced filters"
  it("resets the draft without writing", ...);
  it("keeps the panel open across a kind change and keeps the new kind on Filter", ...);
  it("removes a chip while open and drops it from the draft", ...);            // apply Failed; reopen; click "Remove Card status: Failed" → URL without status; combobox "Card status" shows "Any"; click Filter → status still undefined (Review Focus 2)
  it("removes page with the chip", ...);                                       // ?kind=platform&status=failed&page=1 → remove → "?kind=platform"
  it("wires aria-expanded and aria-controls", ...);                            // "false" then "true"; aria-controls === "a2a-agents-filter-panel" === the aside's id
});
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test -- src/catalogue/A2AAgentCatalogue`
Expected: FAIL — modules not found.

- [ ] **Step 3: Implement `A2AAgentFilterBar.tsx`** per spec §4.5: `FilterBar` with `actions` = `SearchBar` (size small, width 300, `maxWidth: "100%"`) then the `Advanced Filters` `Button` (outlined, `color="inherit"`, `ListFilter` 16, `aria-expanded`, `aria-controls={A2A_FILTER_PANEL_ID}`, ref captured in state for the anchor); children = `FilterToggle label="Kind"`, one `FilterChip` per non-empty advanced filter in order Environment, Project, Card status, then `Clear all` when `activeAdvanced + (kind ? 1 : 0) >= 2`. Local state: `draft: AdvancedFilters`, `open: boolean`. Open copies applied → draft unless already open; Filter calls `onApply(draft)` then closes; chip body opens; chip × calls `onRemove(key)` and clears the draft key; Clear all calls `onClearAll()` and resets the draft, leaving `open` as is. Panel: `FilterPanel` with `id={A2A_FILTER_PANEL_ID}`, title `Advanced Filters`, width 640; body `Stack direction={{ xs: "column", sm: "row" }} spacing={2}` of three labelled small `displayEmpty` `Select`s (`labelId` → a `body2` `text.secondary` `Typography` with that id; `flex: 1`, `minWidth: 180`). Chip value label: environment `displayName ?? name`, project `displayName || name`, status from the static options, else the raw value.

- [ ] **Step 4: Implement `A2AAgentCatalogue.tsx`**: `const [params, replaceParams] = useReplaceSearchParams()`; `filters = readCatalogueFilters(params)`; `page = parsePageParam(params.get("page"))`; `search` state seeded from `params.get("q") ?? ""`; `debouncedSearch` state updated by a 300 ms timer effect on `search.trim()`; an effect that writes `q` (setOrDelete + delete `page`) when `debouncedSearch !== (params.get("q") ?? "")`; `useListA2ACatalog({ orgName: orgId }, catalogueListQuery(filters, debouncedSearch, page))`; clamp effect on `data` (`agents.length === 0 && total > 0 && page > 0` → `writePage(p, lastPageIndex(total))`); `useListEnvironments({ orgName: orgId })`, `useListProjects({ orgName: orgId })`, `useEnvironmentLabel(orgId)`. Writers: kind → `setOrDelete("kind")` + delete `page`; apply → the three keys + delete `page`; remove → delete key + `page`; clear all → delete all four + `page`. Render `ResourceListShell` (`resourceName="a2a-agents"`, `searchValue={search}`, `onSearchChange`, `toolbar`, `error={query.error}`, `isLoading={query.isLoading}`, `isEmpty={total === 0 && !filtering}`, `isSearchEmpty={total === 0 && filtering}`, `loadingPlaceholder` = the grid with 4 `Skeleton variant="rounded" height={200} data-testid="a2a-tile-skeleton"`, the two empty states with `Bot` / `Search` size 64) whose children are the grid (`display: grid`, `gap: 2`, columns `{ xs: "repeat(auto-fill, minmax(260px, 1fr))", md: "repeat(auto-fill, minmax(300px, 1fr))" }`, key `${projectName}/${agentName}`, `emphasised = debouncedSearch !== "" && agent.environments.some((e) => e.matched)`) and, when `total > PAGE_SIZE`, `<TablePagination component="div" rowsPerPageOptions={[]} count={total} rowsPerPage={PAGE_SIZE} page={page} onPageChange={(_, next) => replaceParams((p) => writePage(p, next))} />`.

- [ ] **Step 5: Run tests and lint**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test && pnpm --filter @agent-management-platform/a2a-agents lint`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add console/workspaces/pages/a2a-agents/src/catalogue
git commit -m "Add the A2A catalogue list with search, filters and pagination

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 9: Organisation component, agent-page placeholder, page metadata and core-ui registration

**Files:**
- Create: `console/workspaces/pages/a2a-agents/src/A2AAgentPage.tsx` (placeholder; Part C replaces its contents, keeping the export)
- Create: `console/workspaces/pages/a2a-agents/src/A2AAgents.Organization.tsx`
- Replace: `console/workspaces/pages/a2a-agents/src/index.ts` (final barrel)
- Test: `console/workspaces/pages/a2a-agents/src/A2AAgents.Organization.test.tsx`
- Modify: `console/workspaces/core-ui/package.json` (add `"@agent-management-platform/a2a-agents": "workspace:*"` next to the other page packages), `console/pnpm-lock.yaml` (via `pnpm install`)
- Modify: `console/workspaces/core-ui/src/pages/index.tsx` (add `LazyA2AAgentsOrg`)
- Modify: `console/workspaces/core-ui/src/Route/Route.tsx` (new org route between the `llmProviders` and `mcpProxies` wildcard routes)
- Modify: `console/workspaces/core-ui/src/Layouts/OxygenLayout/navigationItems.tsx` (org-level item after "Agent Catalog", before the conditional "Settings")

**Interfaces:**
- Consumes: `A2AAgentCatalogue` (Task 8), `a2aAgentsListPath`, `a2aAgentsRoute` (Task 6); `PageLayout` from `@agent-management-platform/views`; `relativeRouteMap`, `absoluteRouteMap`, `PageMetadata` from `@agent-management-platform/types`; `Network` icon.
- Produces — the contract Part C builds on:
  - Route (registered here, both maps): absolute `/org/:orgId/a2a-agents/view/:projectName/:agentName`; relative child path `view/:projectName/:agentName` rendered inside `A2AAgentsOrganization`'s nested `<Routes>`; params `orgId`, `projectName`, `agentName`; Part C adds the optional `env` query param (the list never writes it).
  - Placeholder `export const A2AAgentPage: React.FC` in `src/A2AAgentPage.tsx`: reads the three params, renders `<PageLayout title={agentName} avatar={null} backHref={a2aAgentsListPath(orgId)} backLabel="Back to A2A Agents">` with an empty `<Box data-testid="a2a-agent-page" />`. Part C rewrites this file's body (and may move the implementation into a sub-folder) but keeps the `A2AAgentPage` export so `A2AAgents.Organization.tsx` needs no change.
  - `export const A2AAgentsOrganization: React.FC` in `src/A2AAgents.Organization.tsx`: `<Routes>`: `index` → `<PageLayout title="A2A Agents" avatar={null}><A2AAgentCatalogue orgId={orgId ?? ""} /></PageLayout>`; `path={relativeRouteMap.children.org.children.a2aAgents.children.view.path}` → `<A2AAgentPage />`; `path="*"` → `<Navigate to={a2aAgentsListPath(orgId ?? "")} replace />`.
  - `src/index.ts`: `export const metaData: PageMetadata = { title: "A2A Agents", description: "Organization-wide catalogue of A2A agents", icon: Network, path: "/a2a-agents", component: A2AAgentsOrganization, levels: { organization: A2AAgentsOrganization } }`; named exports `A2AAgentsOrganization`, `A2AAgentPage`; default export `A2AAgentsOrganization`.
  - core-ui: `export const LazyA2AAgentsOrg = lazy(() => import("@agent-management-platform/a2a-agents").then((m) => ({ default: m.metaData.levels!.organization as ComponentType })))`; `<Route path={relativeRouteMap.children.org.children.a2aAgents.path + "/*"} element={<LazyA2AAgentsOrg />} />`; nav item `{ label: "A2A Agents", type: "item", icon: <Network size={20} />, href: generatePath(absoluteRouteMap.children.org.children.a2aAgents.path, { orgId }), isActive: !!matchPath(absoluteRouteMap.children.org.children.a2aAgents.wildPath, pathname) }`.

- [ ] **Step 1: Write the failing test** `A2AAgents.Organization.test.tsx` (same api-client and shared-component mocks as Task 8, list hook → `{ data: { agents: [], total: 0, limit: 24, offset: 0 }, isLoading: false }`; render `<MemoryRouter initialEntries={[url]}><Routes><Route path="/org/:orgId/a2a-agents/*" element={<><A2AAgentsOrganization /><LocationProbe /></>} /></Routes></MemoryRouter>` inside the Oxygen `ThemeProvider`):

```ts
it("renders the list at the index route", ...);                       // url /org/acme/a2a-agents → heading "A2A Agents" and text "No A2A agents yet"
it("renders the agent page route", ...);                               // url /org/acme/a2a-agents/view/p1/orders → getByTestId("a2a-agent-page"); link "Back to A2A Agents" href "/org/acme/a2a-agents"
it("replaces any other sub-path with the list", ...);                  // url /org/acme/a2a-agents/anything-else → probe pathname "/org/acme/a2a-agents"; history length unchanged (navigate(-1) stays)
it("exports page metadata", ...);                                      // metaData.title "A2A Agents", description "Organization-wide catalogue of A2A agents", path "/a2a-agents", levels.organization === A2AAgentsOrganization
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test -- src/A2AAgents.Organization`
Expected: FAIL — modules not found.

- [ ] **Step 3: Implement `A2AAgentPage.tsx`, `A2AAgents.Organization.tsx` and the final `src/index.ts`** per the Interfaces block.

- [ ] **Step 4: Run the page tests, lint and build**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test && pnpm --filter @agent-management-platform/a2a-agents lint && pnpm --filter @agent-management-platform/a2a-agents run build`
Expected: PASS; `dist/index.d.ts` declares `metaData`.

- [ ] **Step 5: Register in core-ui**: add the dependency to `core-ui/package.json`, run `cd console && pnpm install`, add `LazyA2AAgentsOrg` to `src/pages/index.tsx` (import it in `Route.tsx` next to `LazyMCPProxiesOrg`), the route and the nav item as specified.

- [ ] **Step 6: Verify core-ui**

Run: `cd console && pnpm --filter @agent-management-platform/am-core-ui lint && pnpm --filter @agent-management-platform/am-core-ui run build`
Expected: both exit 0.
Then: `grep -n "a2aAgents" workspaces/core-ui/src/Route/Route.tsx workspaces/core-ui/src/Layouts/OxygenLayout/navigationItems.tsx workspaces/core-ui/src/pages/index.tsx`
Expected: one route, one nav `href` + one `isActive`, one lazy import.

- [ ] **Step 7: Manual check (no automated core-ui tests exist; skip if no dev backend is reachable and say so in the task report)**: `cd console && make dev`, open `http://localhost:3000/org/<org>/a2a-agents`: the nav shows "A2A Agents" after "Agent Catalog" and stays highlighted on `/org/<org>/a2a-agents/view/<p>/<a>`; `/org/<org>/a2a-agents/other` is replaced by the list URL. Stop the server.

- [ ] **Step 8: Commit**

```bash
git add console/pnpm-lock.yaml console/workspaces/pages/a2a-agents/src/A2AAgentPage.tsx console/workspaces/pages/a2a-agents/src/A2AAgents.Organization.tsx console/workspaces/pages/a2a-agents/src/A2AAgents.Organization.test.tsx console/workspaces/pages/a2a-agents/src/index.ts console/workspaces/core-ui/package.json console/workspaces/core-ui/src/pages/index.tsx console/workspaces/core-ui/src/Route/Route.tsx console/workspaces/core-ui/src/Layouts/OxygenLayout/navigationItems.tsx
git commit -m "Register the A2A Agents page in the console routes and navigation

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

## Decisions recorded (spec ambiguities this plan resolves)

1. **Type names** mirror the OpenAPI schema names (`A2ACatalogAgent`, `A2AConnectResponse`, …); the card source/status enums reuse the existing `AgentCardSource` / `AgentCardStatus` rather than duplicating them.
2. **Route-map key names**: `a2aAgents` → `view`.
3. **Package/folder name**: `@agent-management-platform/a2a-agents` at `console/workspaces/pages/a2a-agents/`, as the spec's test command assumes.
4. **Draft and open flag** live in the page's `A2AAgentFilterBar` (page-owned state as spec §5.5 requires; the shared views components stay stateless), while the applied values stay in the URL owned by `A2AAgentCatalogue`.
5. **Tile emphasis** is exposed as `data-emphasised` on the tile link (in addition to `Form.CardButton`'s `selected`) so tests do not depend on Oxygen's internal markup.
6. **`page` parsing uses `Number()`** (so `"1abc"` → 0, `" 2 "` → 2), truncating toward zero and flooring negatives/NaN at 0.
7. **Views test runner repair**: `libs/views/vitest.config.ts` and `setupTests.tsx` are replaced by the sibling's working versions because the current files cannot start vitest at all; nothing else references them.
8. **Placeholder agent page** renders only the Part C back link and title so no copy is invented that Part C would have to retract.
9. **Connect fetch/hook and the test-key mutation** are left to Part C, with their file homes named (`apis/a2a-catalog.ts`, `hooks/a2a-catalog.ts`, `hooks/agent-api-keys.ts`) so Part C appends rather than creating parallel modules.
