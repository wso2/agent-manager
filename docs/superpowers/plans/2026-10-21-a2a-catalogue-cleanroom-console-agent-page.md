# A2A Catalogue Console Agent Page (Part C) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the catalogue's placeholder agent page with a page that shows one environment's A2A card in detail, compares it with another environment on the backend fingerprint's own terms, and opens a Connect drawer with endpoints, security, a personal test key and runnable quick-start snippets.

**Architecture:** Card parsing, canonicalisation and diffing move into `@agent-management-platform/shared-component` (with a skill item component), where the existing configure-agent panel and overview summary are switched onto them. The api-client gains the connect-details read and an on-demand test-key mutation. Inside the `@agent-management-platform/a2a-agents` workspace, three pure modules (card state, endpoint resolution, snippet builders) carry the rules; a per-environment details area, keyed by environment, composes the card details, compare view and Connect drawer; the page shell owns the entry query, header and chips.

**Tech Stack:** React 19, TypeScript 5.9, react-router-dom 6.30, TanStack Query 5.90, Oxygen UI 0.15 (`@wso2/oxygen-ui`, icons from `@wso2/oxygen-ui-icons-react` which re-exports lucide), Vitest 3 + Testing Library, pnpm workspaces, `a2a-sdk` 1.2 / `@a2a-js/sdk` 1.3 (snippet targets only).

**Spec:** `docs/superpowers/specs/2026-10-21-a2a-catalogue-cleanroom-spec.md` — **Part C** (Console: agent page). Part A §3.3–3.4 and §6 fix the connect contract; Part A §3.2 the single-entry contract. **Prerequisite plan (executed first):** `docs/superpowers/plans/2026-10-21-a2a-catalogue-cleanroom-console-list.md` (Part B) — it creates the types module (`A2ACatalogAgent`, `A2AConnectResponse`, …), `apis/a2a-catalog.ts` / `hooks/a2a-catalog.ts` with `useGetA2ACatalogEntry`, the exported `agentEnvironmentResourcePath` builder, the `@agent-management-platform/a2a-agents` workspace with `routes.ts`, `useReplaceSearchParams`, `useEnvironmentLabel`, the placeholder `src/A2AAgentPage.tsx` and the nested route `view/:projectName/:agentName`. This plan consumes those names exactly and never re-creates them.

## Global Constraints

- Run every command from `/home/lyuda/agent-manager-worktrees/a2a-catalogue-cleanroom` (console commands from its `console/` directory). Clean room: never read any other checkout, worktree, stash or branch other than this branch and `upstream/main`; never search for a prior implementation.
- Console conventions: `console/AGENTS.md` and `console/.ai/oxygen-ui/AGENTS.md`. UI imports only from `@wso2/oxygen-ui` / `@wso2/oxygen-ui-icons-react` (never `@mui/material`); theme tokens in `sx` (`color: "text.secondary"`, `p: 2`), no hard-coded colours or px; no `label` prop on `TextField` (the views `TextInput` `label` prop is the static-label form and is allowed).
- Two-file API pattern: fetch function in `libs/api-client/src/apis/<resource>.ts`, hook in `libs/api-client/src/hooks/<resource>.ts` through `useApiQuery` / `useApiMutation` from `./react-query-notifications`; both exported from `apis/index.ts` and `hooks/index.ts`. Query keys are tuples `[domain, params, …]`. Auth token only from `useAuthHooks().getToken`.
- Routing: `<Route path>` from `relativeRouteMap`; links from `absoluteRouteMap` + `generatePath`; never a hard-coded route string in product code.
- Dependent packages resolve workspace libraries from `dist/`: after changing a library run `pnpm --filter <library> run build` before testing or building its consumers. Package tests: `pnpm --filter <package> test`; lint: `pnpm --filter <package> lint`.
- Every new source file carries the WSO2 Apache-2.0 header used across the repo (copy it from a sibling file; year `2026`).
- Contract strings match Part A character for character: path `…/environments/{envID}/a2a-connect`; security modes `none|apiKey|oauth|unmanaged|unknown`; publication statuses `pending|published|failed|none`; card statuses `pending|fetched|failed`; kinds `platform|external`.
- Snippet universal rules (spec §10.1) apply to every builder: values only as escaped literals (Python = JSON string; TypeScript = JSON string with U+2028 → ` ` and U+2029 → ` `; curl = `'…'` with `'` → `'\''`), no input value inside a comment, constant endpoint comment `The card routes JSON-RPC calls to this endpoint.`, unmanaged comment `The platform does not manage this agent's security; add whatever auth its agent card declares.`, placeholders `<YOUR_API_KEY>` and `<ACCESS_TOKEN>`, no top-level `await` in TypeScript.
- Commits: imperative mood, capitalised first word, no trailing period, no `feat:` prefix, no Co-Authored-By; every message ends with the trailer line `Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q`. Stage explicit paths. Never push.

## Review Focus

1. `?env=` naming an environment the entry does not list (stale bookmark) — the page must not show an empty "no card" state for a phantom environment; it falls back to the lead environment without rewriting the URL. (Test added to Task 11.)
2. Opening Connect while the selected card is `absent` or still loading — the drawer must still open and show the gateway endpoint with no declared interfaces and `Security isn't managed…: nothing.` / the mode copy, never throw on a missing card body. (Test added to Task 11.)
3. `security.mode: "oauth"` with no `oauth` object (a malformed or older backend) — the security section must render `Authorization`, `Issuers:` and `Audiences: any` instead of throwing on `oauth.headerName`. (Test added to Task 10.)
4. Skills without a `name` or with duplicate `id`/`name` values — the skills list and the compare view must render each item without a crash or key collision. (Test added to Task 9.)
5. An entry whose `environments` array is empty (`leadEnvironment: ""`) — the page shows the header only; no chips, no Connect button, no card request, no crash. (Test added to Task 11.)

---

### Task 1: Shared A2A card parse, canonicalise and diff utilities

**Files:**
- Create: `console/workspaces/libs/shared-component/src/utils/a2aCard.ts`
- Create: `console/workspaces/libs/shared-component/src/utils/a2aCardDiff.ts`
- Modify: `console/workspaces/libs/shared-component/src/index.ts` (add `export * from './utils/a2aCard';` and `export * from './utils/a2aCardDiff';` after the `./utils/url` line)
- Test: `console/workspaces/libs/shared-component/src/utils/a2aCard.test.ts`, `src/utils/a2aCardDiff.test.ts`

**Interfaces:**
- Consumes: `validateEndpointUrl` from `./url`.
- Produces (exported from `@agent-management-platform/shared-component`):

```ts
// utils/a2aCard.ts
export interface ParsedA2AInterface { url: string; protocolBinding: string }
export interface ParsedA2ASkill {
  id: string; name: string; description: string;
  tags: string[]; examples: string[]; inputModes: string[]; outputModes: string[];
}
export interface A2ACardCapabilities { streaming: boolean; pushNotifications: boolean; extendedAgentCard: boolean }
export interface ParsedA2ACard {
  name: string; description: string; version: string;
  providerOrganization: string; providerUrl: string;
  interfaces: ParsedA2AInterface[]; skills: ParsedA2ASkill[];
  capabilities: A2ACardCapabilities; securitySchemes: string[];
}
export const EMPTY_A2A_CARD: ParsedA2ACard;                 // the all-empty record
export function parseA2ACard(card: unknown): ParsedA2ACard;  // spec §8.1 rules; never throws

// utils/a2aCardDiff.ts
export interface A2ACardDiff { fields: string[]; skills: { added: string[]; removed: string[]; changed: string[] } }
export function canonicalJson(value: unknown): string | undefined;      // JSON of canonical(value): keys sorted at every depth; undefined for undefined
export function withoutInterfaceUrls(card: Record<string, unknown>): Record<string, unknown>;  // copy; drops `url` from object items of an array `supportedInterfaces`
export function diffA2ACards(a: unknown, b: unknown): A2ACardDiff;       // spec §8.2; non-objects read as {}
export function a2aCardsIdentical(diff: A2ACardDiff): boolean;          // all four lists empty
```

- [ ] **Step 1: Write the failing parse tests** `a2aCard.test.ts` (spec §13.4 scenarios 38–39):

```ts
const full = { name: "Orders", description: "d", version: "1.0",
  provider: { organization: "Acme", url: "https://acme.example" },
  supportedInterfaces: [{ url: "https://x/rpc", protocolBinding: "JSONRPC" }, { protocolBinding: "HTTP+JSON" }, { url: "https://x/rest", protocolBinding: 7 }],
  skills: [{ id: "s1", name: "order-status", description: "sd", tags: ["orders"], examples: ["Where is order 7?"], inputModes: ["text/plain"], outputModes: ["application/json"] }, "junk"],
  capabilities: { streaming: true, pushNotifications: "true", extendedAgentCard: 1 },
  securitySchemes: { bearer: {}, apiKey: {} } };
it("reads every field of a full card", () => expect(parseA2ACard(full)).toEqual({
  name: "Orders", description: "d", version: "1.0", providerOrganization: "Acme", providerUrl: "https://acme.example",
  interfaces: [{ url: "https://x/rpc", protocolBinding: "JSONRPC" }, { url: "https://x/rest", protocolBinding: "" }],
  skills: [{ id: "s1", name: "order-status", description: "sd", tags: ["orders"], examples: ["Where is order 7?"], inputModes: ["text/plain"], outputModes: ["application/json"] }],
  capabilities: { streaming: true, pushNotifications: false, extendedAgentCard: false },
  securitySchemes: ["bearer", "apiKey"] }));
it.each([undefined, null, "card", [1]])("reads %j as the empty card", (v) => expect(parseA2ACard(v)).toEqual(EMPTY_A2A_CARD));
it("is defensive", () => {
  expect(parseA2ACard({ skills: "x", capabilities: null })).toMatchObject({ skills: [], capabilities: { streaming: false, pushNotifications: false, extendedAgentCard: false } });
  expect(parseA2ACard({ provider: { organization: "Acme", url: "javascript:alert(1)" } })).toMatchObject({ providerOrganization: "Acme", providerUrl: "" });
  expect(parseA2ACard({ skills: [{ name: "a", tags: ["t", 3, null] }] }).skills[0]).toMatchObject({ id: "", tags: ["t"], examples: [] });
});
```

- [ ] **Step 2: Write the failing diff tests** `a2aCardDiff.test.ts` (scenarios 40–43):

```ts
const skill = (name: string, extra = {}) => ({ id: name, name, description: "d", tags: [], ...extra });
it("reports added, removed and changed skills with no fields", () => {
  expect(diffA2ACards({ skills: [skill("order-status")] }, { skills: [skill("order-status"), skill("issue-refund")] }))
    .toEqual({ fields: [], skills: { added: ["issue-refund"], removed: [], changed: [] } });
  expect(diffA2ACards({ skills: [skill("order-status"), skill("issue-refund")] }, { skills: [skill("order-status", { description: "other" })] }))
    .toEqual({ fields: [], skills: { added: [], removed: ["issue-refund"], changed: ["order-status"] } });
});
it("ignores key order in fields", () => expect(diffA2ACards({ version: "1", capabilities: { a: 1, b: 2 } }, { capabilities: { b: 2, a: 1 }, version: "2" }).fields).toEqual(["version"]));
it("ignores interface URLs but not bindings", () => {
  expect(diffA2ACards({ supportedInterfaces: [{ url: "a", protocolBinding: "JSONRPC" }] }, { supportedInterfaces: [{ url: "b", protocolBinding: "JSONRPC" }] }).fields).toEqual([]);
  expect(diffA2ACards({ supportedInterfaces: [{ url: "a", protocolBinding: "JSONRPC" }] }, { supportedInterfaces: [{ url: "a", protocolBinding: "HTTP+JSON" }] }).fields).toEqual(["supportedInterfaces"]);
});
it("a skill's inputModes change marks it changed", () => expect(diffA2ACards({ skills: [skill("a", { inputModes: ["text/plain"] })] }, { skills: [skill("a", { inputModes: ["text/html"] })] }).skills.changed).toEqual(["a"]));
it("reordered skills report skills as a differing field", () => expect(diffA2ACards({ skills: [skill("a"), skill("b")] }, { skills: [skill("b"), skill("a")] })).toEqual({ fields: ["skills"], skills: { added: [], removed: [], changed: [] } }));
it("duplicate skill names still differ", () => expect(diffA2ACards({ skills: [skill("a"), skill("a")] }, { skills: [skill("a")] }).fields).toContain("skills"));
it("nested key order is not a difference", () => expect(a2aCardsIdentical(diffA2ACards({ a: { x: 1, y: [{ p: 1, q: 2 }] }, skills: [skill("a")] }, { skills: [skill("a")], a: { y: [{ q: 2, p: 1 }], x: 1 } }))).toBe(true));
it("a key missing on one side differs from any value", () => expect(diffA2ACards({ version: "" }, {}).fields).toEqual(["version"]));
it("keeps fields sorted when skills is appended", () => expect(diffA2ACards({ version: "1", skills: [skill("a"), skill("b")] }, { version: "2", skills: [skill("b"), skill("a")] }).fields).toEqual(["skills", "version"]));
it("treats a non-array skills side as removed skills", () => expect(diffA2ACards({ skills: [skill("a")] }, { skills: "x" })).toEqual({ fields: [], skills: { added: [], removed: ["a"], changed: [] } }));
// Property: all-empty ⇔ canonical forms without interface URLs are equal
const pairs = [/* every (a, b) pair used above, plus (full, full) and ({}, {}) */];
it.each(pairs)("all-empty iff canonical forms match (%#)", (a, b) => {
  const same = canonicalJson(withoutInterfaceUrls(a)) === canonicalJson(withoutInterfaceUrls(b));
  expect(a2aCardsIdentical(diffA2ACards(a, b))).toBe(same);
});
```

- [ ] **Step 3: Run them to verify they fail**

Run: `cd console && pnpm --filter @agent-management-platform/shared-component test -- src/utils/a2aCard`
Expected: FAIL — modules `./a2aCard` / `./a2aCardDiff` not found.

- [ ] **Step 4: Implement `parseA2ACard`** with private readers `asObject`, `asString` (string else `""`), `stringsIn` (array → string items), `objectsIn` (array → object items, arrays excluded) and the §8.1 rules; `providerUrl` keeps the value only when `typeof url === "string" && validateEndpointUrl(url) === null`; interfaces with `url === ""` are dropped; `securitySchemes` = `Object.keys` of an object, else `[]`.

- [ ] **Step 5: Implement `a2aCardDiff.ts`.** `canonical(v)`: arrays map element-wise, plain objects rebuild with `Object.keys(v).sort()`, scalars unchanged; `canonicalJson = (v) => JSON.stringify(canonical(v))`. `diffA2ACards`: `A = withoutInterfaceUrls(asObject(a))`, same for `B`; `fields` = sorted union of top-level keys minus `skills` where `canonicalJson(A[k]) !== canonicalJson(B[k])`; skill indexes = `Map<name, object>` over the object items of each side's `skills` array (name = `asString(skill.name)`, last duplicate wins, insertion order kept); `added` / `removed` by key presence, `changed` where `canonicalJson` of the two whole objects differs; fallback: when all three are empty and `canonicalJson(A.skills) !== canonicalJson(B.skills)` push `"skills"` into `fields` and re-sort.

- [ ] **Step 6: Run tests, lint, build the library**

Run: `cd console && pnpm --filter @agent-management-platform/shared-component test -- src/utils/a2aCard && pnpm --filter @agent-management-platform/shared-component lint && pnpm --filter @agent-management-platform/shared-component run build`
Expected: all pass; `dist/utils/a2aCard.d.ts` and `dist/utils/a2aCardDiff.d.ts` exist.

- [ ] **Step 7: Commit**

```bash
git add console/workspaces/libs/shared-component/src/utils/a2aCard.ts console/workspaces/libs/shared-component/src/utils/a2aCard.test.ts console/workspaces/libs/shared-component/src/utils/a2aCardDiff.ts console/workspaces/libs/shared-component/src/utils/a2aCardDiff.test.ts console/workspaces/libs/shared-component/src/index.ts
git commit -m "Add shared A2A card parse, canonicalise and diff utilities to the console

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 2: Shared skill item component

**Files:**
- Create: `console/workspaces/libs/shared-component/src/components/A2ASkillItem.tsx`
- Modify: `console/workspaces/libs/shared-component/src/components/index.ts` (add `export * from "./A2ASkillItem";` after the `./AgentTypeChips` line)
- Test: `console/workspaces/libs/shared-component/src/components/A2ASkillItem.test.tsx`

**Interfaces:**
- Consumes: `ParsedA2ASkill` (Task 1); Oxygen `Box`, `Chip`, `Stack`, `Typography`.
- Produces: `export interface A2ASkillItemProps { skill: ParsedA2ASkill }`, `export function A2ASkillItem({ skill }: A2ASkillItemProps): JSX.Element` — spec §8.3 exactly (bordered box `p: 1.5`, `border: 1`, `borderColor: "divider"`, `borderRadius: 1`, `Stack spacing={0.5}`; captions `Input` / `Output`; examples as `e.g. {example}`; chips keyed `${index}-${value}`).

- [ ] **Step 1: Write the failing test** (render inside Oxygen `ThemeProvider theme={createTheme()}` as `CodeBlock.test.tsx` does):

```ts
const skill = { id: "s1", name: "order-status", description: "Where an order is", tags: ["orders", "orders"], examples: ["Where is order 7?"], inputModes: ["text/plain"], outputModes: ["application/json"] };
it("renders name, description, tags, modes and examples", () => {
  render(<A2ASkillItem skill={skill} />);
  expect(screen.getByText("order-status")).toBeInTheDocument();
  expect(screen.getByText("Where an order is")).toBeInTheDocument();
  expect(screen.getAllByText("orders")).toHaveLength(2);      // duplicate tags both render
  expect(screen.getByText("Input")).toBeInTheDocument(); expect(screen.getByText("text/plain")).toBeInTheDocument();
  expect(screen.getByText("Output")).toBeInTheDocument(); expect(screen.getByText("application/json")).toBeInTheDocument();
  expect(screen.getByText("e.g. Where is order 7?")).toBeInTheDocument();
});
it("omits empty parts", () => {
  render(<A2ASkillItem skill={{ ...skill, description: "", tags: [], examples: [], inputModes: [], outputModes: [] }} />);
  expect(screen.queryByText("Input")).not.toBeInTheDocument();
  expect(screen.queryByText("Output")).not.toBeInTheDocument();
  expect(screen.queryByText(/^e\.g\./)).not.toBeInTheDocument();
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd console && pnpm --filter @agent-management-platform/shared-component test -- src/components/A2ASkillItem`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the component** per the Interfaces block and export it.

- [ ] **Step 4: Run tests, lint, build**

Run: `cd console && pnpm --filter @agent-management-platform/shared-component test && pnpm --filter @agent-management-platform/shared-component lint && pnpm --filter @agent-management-platform/shared-component run build`
Expected: all pass; `dist/components/A2ASkillItem.d.ts` exists.

- [ ] **Step 5: Commit**

```bash
git add console/workspaces/libs/shared-component/src/components/A2ASkillItem.tsx console/workspaces/libs/shared-component/src/components/A2ASkillItem.test.tsx console/workspaces/libs/shared-component/src/components/index.ts
git commit -m "Add the shared A2A skill item component

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 3: Switch the configure-agent panel and the overview summary onto the shared utilities

**Files:**
- Modify: `console/workspaces/pages/configure-agent/src/Configure/subComponents/A2AAgentCardPanel.tsx` (delete the private `Json`, `isObject`, `asString`, `objectsIn`, `stringsIn` helpers and the inline skill box)
- Modify: `console/workspaces/pages/overview/src/AgentOverview/EnvAgentCardSummary.tsx` (delete `Json`, `isObject`, `asString`, `objectsIn`, `summarizeCard`)
- Test: `console/workspaces/pages/configure-agent/src/Configure/subComponents/A2AAgentCardPanel.test.tsx`, `console/workspaces/pages/overview/src/AgentOverview/EnvAgentCardSummary.test.tsx` (existing tests unchanged; assertions added)

**Interfaces:**
- Consumes: `parseA2ACard`, `A2ASkillItem`, `normalizeVersion` from `@agent-management-platform/shared-component` (Tasks 1–2).
- Produces: unchanged exports `A2AAgentCardPanel`, `EnvAgentCardSummary`; behaviour per spec §8.4 (the panel's skill items now also show modes and examples — the one accepted visual change).

- [ ] **Step 1: Record the baseline**

Run: `cd console && pnpm --filter @agent-management-platform/configure-agent test -- A2AAgentCardPanel && pnpm --filter @agent-management-platform/overview test -- EnvAgentCardSummary`
Expected: PASS (all).

- [ ] **Step 2: Add the failing assertions.** Panel test, in `"shows the fetched card's summary and skills"`: give the card a second interface `{ protocolBinding: "HTTP+JSON" }` (no URL) and the skill `inputModes: ["text/plain"], examples: ["Plan Paris"]`; assert `screen.getAllByRole("button", { name: "Copy URL" })` has length 1, `screen.queryByText("HTTP+JSON")` is null, `screen.getByText("text/plain")` and `screen.getByText("e.g. Plan Paris")` exist, and `screen.getByText("v1.2.0")` exists. Add `it("normalises a v-prefixed version", …)` with `version: "v2.0"` → `screen.getByText("v2.0")` once. Summary test, in `"shows only the provider, interfaces and skill names"`: add a skill `{ description: "no name" }` and assert the Skills row still has exactly the chips `plan` and `book`; add `it("lists a binding-less interface without a chip", …)` with `supportedInterfaces: [{ url: "https://gw/a/rest" }]` → `getByDisplayValue("https://gw/a/rest")` and no `Chip` text `JSONRPC`.

- [ ] **Step 3: Run them to verify the new assertions fail**

Run: the Step 1 command.
Expected: FAIL only on the new assertions (`text/plain`, `e.g. Plan Paris`).

- [ ] **Step 4: Rewrite the panel's card reading.** `const parsed = parseA2ACard(card?.card);` then: header `h6` `{parsed.name}`; version caption `{normalizeVersion(parsed.version)}` only when `parsed.version !== ""`; description when non-empty; Interfaces rows over `parsed.interfaces` (chip only when `protocolBinding !== ""`, keyed `${i}-${url}`); Skills as `<A2ASkillItem key={`${i}-${skill.id || skill.name}`} skill={skill} />` inside `Stack spacing={1.5}`. Keep every other prop, string and branch as it is (status chips, fetch-failed alert, refetch, source editing, proxy section, `View JSON`/`Hide JSON`, Remove).

- [ ] **Step 5: Rewrite the summary's `CardSummary`.** `const { providerOrganization: organization, providerUrl, interfaces, skills } = parseA2ACard(card); const skillNames = skills.map((s) => s.name).filter((n) => n !== "");` and keep the rest of the JSX, with `providerUrl !== ""` and `binding !== ""` as the conditions. The outer component's `card` guard stays `isObject(data?.card)`-equivalent: pass `data?.card` through and render `CardSummary` only when `data?.card` is a non-null object (keep a one-line local predicate).

- [ ] **Step 6: Run both packages' tests and lint**

Run: `cd console && pnpm --filter @agent-management-platform/configure-agent test && pnpm --filter @agent-management-platform/configure-agent lint && pnpm --filter @agent-management-platform/overview test && pnpm --filter @agent-management-platform/overview lint`
Expected: PASS; lint reports no unused imports.

- [ ] **Step 7: Commit**

```bash
git add console/workspaces/pages/configure-agent/src/Configure/subComponents/A2AAgentCardPanel.tsx console/workspaces/pages/configure-agent/src/Configure/subComponents/A2AAgentCardPanel.test.tsx console/workspaces/pages/overview/src/AgentOverview/EnvAgentCardSummary.tsx console/workspaces/pages/overview/src/AgentOverview/EnvAgentCardSummary.test.tsx
git commit -m "Switch the agent card panel and overview summary onto the shared A2A card utilities

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 4: Connect-details fetch + hook and the on-demand test-key mutation

**Files:**
- Modify: `console/workspaces/libs/api-client/src/apis/a2a-catalog.ts` (append `getA2AConnect`)
- Modify: `console/workspaces/libs/api-client/src/hooks/a2a-catalog.ts` (append `a2aConnectQueryKey`, `useGetA2AConnect`)
- Modify: `console/workspaces/libs/api-client/src/hooks/agent-api-keys.ts` (append `useIssueTestAgentAPIKey` after `useTestAgentAPIKey`)
- Test: `console/workspaces/libs/api-client/src/hooks/a2a-catalog.test.ts` (append a `describe("useGetA2AConnect")`), `console/workspaces/libs/api-client/src/hooks/agent-api-keys.test.ts` (new)

**Interfaces:**
- Consumes: `agentEnvironmentResourcePath` from `../apis/agent-card` (Part B Task 2); `A2AConnectPathParams`, `A2AConnectResponse`, `IssueTestAgentAPIKeyPathParams`, `IssueTestAgentAPIKeyResponse` from `@agent-management-platform/types`; `issueTestAgentAPIKey` from `../apis/agent-api-keys`; `httpGET` from `../utils`.
- Produces (exported through the existing barrel lines):

```ts
// apis/a2a-catalog.ts
export async function getA2AConnect(params: A2AConnectPathParams, getToken?: () => Promise<string>): Promise<A2AConnectResponse>;
// GET agentEnvironmentResourcePath(params, "a2a-connect") with { token }; returns res.json()

// hooks/a2a-catalog.ts
export const a2aConnectQueryKey = (params: A2AConnectPathParams) => ["a2a-connect", params] as const;
export function useGetA2AConnect(params: A2AConnectPathParams): UseQueryResult<A2AConnectResponse>;
// enabled when orgName, projName, agentName and envId are all non-empty; app defaults for staleTime, retry and snackbar (not silent)

// hooks/agent-api-keys.ts
export function useIssueTestAgentAPIKey(): UseMutationResult<IssueTestAgentAPIKeyResponse, unknown, IssueTestAgentAPIKeyPathParams>;
// useApiMutation({ action: { verb: "create", target: "test API key" }, showSuccess: false, mutationFn: (params) => issueTestAgentAPIKey(params, getToken) }); no invalidation
```

- [ ] **Step 1: Write the failing connect tests** in `hooks/a2a-catalog.test.ts` (same harness and mocks as the file already has):

```ts
describe("useGetA2AConnect", () => {
  const connect = { orgName: "acme", projName: "p 1", agentName: "a/b", envId: "dev" };
  it("encodes every segment", async () => {
    await mount(() => useGetA2AConnect(connect)); await settle();
    expect(httpGET).toHaveBeenCalledWith(`${SERVICE_BASE}/orgs/acme/projects/p%201/agents/a%2Fb/environments/dev/a2a-connect`, { token: "t" });
  });
  it("does not request without an environment", async () => {
    await mount(() => useGetA2AConnect({ ...connect, envId: "" })); await settle();
    expect(httpGET).not.toHaveBeenCalled();
  });
  it("caches under the a2a-connect key", async () => {
    vi.mocked(httpGET).mockResolvedValueOnce({ ok: true, json: async () => ({ kind: "platform" }) } as Response);
    const { qc } = await mount(() => useGetA2AConnect(connect)); await settle();
    expect(qc.getQueryData(a2aConnectQueryKey(connect))).toEqual({ kind: "platform" });
  });
});
```

- [ ] **Step 2: Write the failing mutation test** `hooks/agent-api-keys.test.ts` (copy the `mount`/act harness from `a2a-proxy.test.ts`; mocks: `@agent-management-platform/auth`, `./telemetry` as there; `@agent-management-platform/views` → `useSnackBar: () => ({ pushSnackBar })` with a shared `const pushSnackBar = vi.fn()`; `../apis/agent-api-keys` → `issueTestAgentAPIKey: vi.fn()`):

```ts
const params = { orgName: "acme", projName: "p1", agentName: "orders", envId: "dev" };
it("issues on demand through the existing fetch and shows no success snackbar", async () => {
  vi.mocked(issueTestAgentAPIKey).mockResolvedValueOnce({ status: "ok", apiKey: "k1", expiresAt: "2026-10-21T10:10:00Z" });
  const { result } = await mount(useIssueTestAgentAPIKey);
  await act(async () => { await result.current.mutateAsync(params); });
  expect(issueTestAgentAPIKey).toHaveBeenCalledWith(params, expect.any(Function));
  expect(result.current.data?.apiKey).toBe("k1");
  expect(pushSnackBar).not.toHaveBeenCalled();
});
it("shows the standard error snackbar on failure", async () => {
  vi.mocked(issueTestAgentAPIKey).mockRejectedValueOnce(Object.assign(new Error("HTTP error! status: 500"), { status: 500 }));
  const { result } = await mount(useIssueTestAgentAPIKey);
  await act(async () => { await result.current.mutateAsync(params).catch(() => undefined); });
  expect(pushSnackBar).toHaveBeenCalledWith({ message: "Failed to submit test API key", type: "error" });
});
```

- [ ] **Step 3: Run them to verify they fail**

Run: `cd console && pnpm --filter @agent-management-platform/api-client test -- src/hooks/a2a-catalog.test.ts src/hooks/agent-api-keys.test.ts`
Expected: FAIL — `useGetA2AConnect` / `useIssueTestAgentAPIKey` are not exported.

- [ ] **Step 4: Implement the three functions** per the Interfaces block.

- [ ] **Step 5: Run tests, lint, build**

Run: `cd console && pnpm --filter @agent-management-platform/api-client test && pnpm --filter @agent-management-platform/api-client lint && pnpm --filter @agent-management-platform/api-client run build`
Expected: all pass; `dist/hooks/a2a-catalog.d.ts` declares `useGetA2AConnect`, `dist/hooks/agent-api-keys.d.ts` declares `useIssueTestAgentAPIKey`.

- [ ] **Step 6: Commit**

```bash
git add console/workspaces/libs/api-client/src/apis/a2a-catalog.ts console/workspaces/libs/api-client/src/hooks/a2a-catalog.ts console/workspaces/libs/api-client/src/hooks/a2a-catalog.test.ts console/workspaces/libs/api-client/src/hooks/agent-api-keys.ts console/workspaces/libs/api-client/src/hooks/agent-api-keys.test.ts
git commit -m "Add the A2A connect-details hook and the on-demand test-key mutation to the api-client

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 5: Point the security page's deployment-settings button at the registered deployment route

**Files:**
- Modify: `console/workspaces/pages/agent-security/src/Security.Component.tsx` (the `Go to Deployment Settings` button, currently `generatePath(agentRoutes.children.environment.children.deploy.path, { orgId, projectId, agentId, envId })`)
- Test: `console/workspaces/pages/agent-security/src/Security.Component.test.tsx` (`"points a deployed agent with security off to its deployment settings"`)

**Interfaces:**
- Consumes: `absoluteRouteMap.children.org.children.projects.children.agents.children.deployment.path` (`/org/:orgId/project/:projectId/agents/:agentId/deployment`, registered in core-ui).
- Produces: unchanged `SecurityComponent`; the button's `to` is `generatePath(agentRoutes.children.deployment.path, { orgId, projectId, agentId })`. The render guard (`orgId && projectId && agentId && envId`) stays as it is.

- [ ] **Step 1: Add the failing assertion** to the existing test: `expect(screen.getByRole("link", { name: "Go to Deployment Settings" })).toHaveAttribute("href", "/org/o/project/p/agents/a/deployment");`

- [ ] **Step 2: Run it to verify it fails**

Run: `cd console && pnpm --filter @agent-management-platform/agent-security test -- Security.Component`
Expected: FAIL — href is `/org/o/project/p/agents/a/environment/dev/deploy`.

- [ ] **Step 3: Change the `to`** per the Interfaces block.

- [ ] **Step 4: Run tests and lint**

Run: `cd console && pnpm --filter @agent-management-platform/agent-security test && pnpm --filter @agent-management-platform/agent-security lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add console/workspaces/pages/agent-security/src/Security.Component.tsx console/workspaces/pages/agent-security/src/Security.Component.test.tsx
git commit -m "Point the security page's deployment-settings button at the registered deployment route

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 6: Card state helper

**Files:**
- Create: `console/workspaces/pages/a2a-agents/src/agent/cardState.ts`
- Test: `console/workspaces/pages/a2a-agents/src/agent/cardState.test.ts`

**Interfaces:**
- Consumes: `AgentCardResponse` from `@agent-management-platform/types`.
- Produces:

```ts
export interface CardQueryResult { data: AgentCardResponse | null | undefined; isLoading: boolean; isError: boolean }
export type CardState =
  | { state: "ready"; response: AgentCardResponse; card: Record<string, unknown> }
  | { state: "noBody"; response: AgentCardResponse }
  | { state: "absent" }
  | { state: "error" }
  | { state: "loading" };
export function cardState(query: CardQueryResult): CardState;   // spec §6, checked in order 1–5
```

- [ ] **Step 1: Write the failing test** (scenario 18):

```ts
const ready = { source: "platform", status: "fetched", sourceUrl: "", lastError: "", card: { name: "Orders" } } as AgentCardResponse;
const pending = { source: "platform", status: "pending", sourceUrl: "", lastError: "" } as AgentCardResponse;
it.each([
  [{ data: ready, isLoading: false, isError: false }, "ready"],
  [{ data: ready, isLoading: false, isError: true }, "ready"],
  [{ data: pending, isLoading: false, isError: false }, "noBody"],
  [{ data: { ...pending, card: null as unknown as undefined }, isLoading: false, isError: false }, "noBody"],
  [{ data: null, isLoading: false, isError: false }, "absent"],
  [{ data: undefined, isLoading: false, isError: true }, "error"],
  [{ data: undefined, isLoading: true, isError: false }, "loading"],
  [{ data: undefined, isLoading: false, isError: false }, "loading"],
])("maps %j to %s", (query, state) => expect(cardState(query).state).toBe(state));
it("carries the body when ready", () => expect(cardState({ data: ready, isLoading: false, isError: false })).toEqual({ state: "ready", response: ready, card: { name: "Orders" } }));
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test -- src/agent/cardState`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `cardState`** (a non-null, non-array object `data.card` is "present").

- [ ] **Step 4: Run tests and lint**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test -- src/agent/cardState && pnpm --filter @agent-management-platform/a2a-agents lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add console/workspaces/pages/a2a-agents/src/agent/cardState.ts console/workspaces/pages/a2a-agents/src/agent/cardState.test.ts
git commit -m "Add the A2A agent page card-state helper

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 7: Connect endpoint resolver

**Files:**
- Create: `console/workspaces/pages/a2a-agents/src/agent/connectEndpoints.ts`
- Test: `console/workspaces/pages/a2a-agents/src/agent/connectEndpoints.test.ts`

**Interfaces:**
- Consumes: `A2AConnectResponse` from `@agent-management-platform/types`; `ParsedA2AInterface` from `@agent-management-platform/shared-component`.
- Produces:

```ts
export const GATEWAY_ENDPOINT_LABEL = "Gateway endpoint";
export const JSON_RPC_ENDPOINT_LABEL = "JSON-RPC";
export const DEFAULT_ENDPOINT_LABEL = "Endpoint";
export interface ConnectEndpoint { label: string; url: string }
export interface ConnectEndpoints { endpoints: ConnectEndpoint[]; declared: ParsedA2AInterface[]; cardUrl: string; snippetEndpoint: string }
export function resolveConnectEndpoints(
  connect: Pick<A2AConnectResponse, "gatewayUrl" | "cardUrl" | "jsonRpcUrl">,
  interfaces: ParsedA2AInterface[],
): ConnectEndpoints;   // spec §9.5: gateway branch when gatewayUrl !== "", else direct branch; snippetEndpoint is always jsonRpcUrl
```

- [ ] **Step 1: Write the failing tests** (scenarios 57–62, 65):

```ts
const jsonrpc = { url: "http://localhost:9099/rpc", protocolBinding: "JSONRPC" };
const rest = { url: "https://origin.example/rest", protocolBinding: "HTTP+JSON" };
it("gateway case", () => expect(resolveConnectEndpoints({ gatewayUrl: "https://gw.example/orders", cardUrl: "https://gw.example/orders/.well-known/agent-card.json", jsonRpcUrl: "https://gw.example/orders/rpc" }, [jsonrpc]))
  .toEqual({ endpoints: [{ label: "Gateway endpoint", url: "https://gw.example/orders" }, { label: "JSON-RPC", url: "https://gw.example/orders/rpc" }], declared: [jsonrpc], cardUrl: "https://gw.example/orders/.well-known/agent-card.json", snippetEndpoint: "https://gw.example/orders/rpc" }));
it("gateway case without JSON-RPC", () => { const r = resolveConnectEndpoints({ gatewayUrl: "https://gw", cardUrl: "", jsonRpcUrl: "" }, [jsonrpc]); expect(r.endpoints).toEqual([{ label: "Gateway endpoint", url: "https://gw" }]); expect(r.snippetEndpoint).toBe(""); });
it("direct case", () => expect(resolveConnectEndpoints({ gatewayUrl: "", cardUrl: "https://origin.example/card", jsonRpcUrl: "https://origin.example/rpc" }, [rest, { url: "https://origin.example/rpc", protocolBinding: "JSONRPC" }]))
  .toEqual({ endpoints: [{ label: "HTTP+JSON", url: "https://origin.example/rest" }, { label: "JSONRPC", url: "https://origin.example/rpc" }], declared: [], cardUrl: "https://origin.example/card", snippetEndpoint: "https://origin.example/rpc" }));
it("never falls back to a non-JSON-RPC interface", () => expect(resolveConnectEndpoints({ gatewayUrl: "", cardUrl: "", jsonRpcUrl: "" }, [rest]).snippetEndpoint).toBe(""));
it("direct case with no interfaces", () => expect(resolveConnectEndpoints({ gatewayUrl: "", cardUrl: "", jsonRpcUrl: "" }, [])).toEqual({ endpoints: [], declared: [], cardUrl: "", snippetEndpoint: "" }));
it("labels a binding-less interface Endpoint", () => expect(resolveConnectEndpoints({ gatewayUrl: "", cardUrl: "", jsonRpcUrl: "" }, [{ url: "https://o/x", protocolBinding: "" }]).endpoints[0].label).toBe("Endpoint"));
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test -- src/agent/connectEndpoints`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `resolveConnectEndpoints`** per the Interfaces block.

- [ ] **Step 4: Run tests and lint**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test -- src/agent/connectEndpoints && pnpm --filter @agent-management-platform/a2a-agents lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add console/workspaces/pages/a2a-agents/src/agent/connectEndpoints.ts console/workspaces/pages/a2a-agents/src/agent/connectEndpoints.test.ts
git commit -m "Add the A2A connect endpoint resolver

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 8: Quick-start snippet builders (Python, TypeScript, curl)

**Files:**
- Create: `console/workspaces/pages/a2a-agents/src/agent/quickStartSnippets.ts`
- Test: `console/workspaces/pages/a2a-agents/src/agent/quickStartSnippets.test.ts`

**Interfaces:**
- Consumes: nothing from the codebase.
- Produces:

```ts
export type SnippetLanguage = "python" | "typescript" | "curl";
export const SNIPPET_LANGUAGES: readonly SnippetLanguage[] = ["python", "typescript", "curl"];
export const SNIPPET_LABELS: Record<SnippetLanguage, string> = { python: "Python", typescript: "TypeScript", curl: "curl" };
export const SNIPPET_HIGHLIGHT: Record<SnippetLanguage, string> = { python: "python", typescript: "typescript", curl: "bash" };
export const ENDPOINT_COMMENT = "The card routes JSON-RPC calls to this endpoint.";
export const UNMANAGED_COMMENT = "The platform does not manage this agent's security; add whatever auth its agent card declares.";
export const API_KEY_PLACEHOLDER = "<YOUR_API_KEY>";
export const ACCESS_TOKEN_PLACEHOLDER = "<ACCESS_TOKEN>";
export interface SnippetAuth { header: string; value: string }
export interface SnippetInputs { cardUrl: string; endpointUrl: string; auth: SnippetAuth | null; authUnmanaged?: boolean }
export function buildQuickStartSnippet(language: SnippetLanguage, inputs: SnippetInputs): string;
```

  Templates: spec §10.2 (Python), §10.3 (TypeScript), §10.4 (curl) **verbatim**, with `«x»` replaced by the language's literal of `x` and the bracketed lines emitted only under their conditions. Private literal helpers: `pythonLiteral = (v) => JSON.stringify(v)`; `typescriptLiteral = (v) => JSON.stringify(v).replace(/ /g, "\\u2028").replace(/ /g, "\\u2029")`; `shellLiteral = (v) => `'${v.replace(/'/g, "'\\''")}'``. The unmanaged comment is emitted only when `inputs.authUnmanaged === true && inputs.auth === null`.

- [ ] **Step 1: Write the failing tests** (scenarios 66–75; `langs = SNIPPET_LANGUAGES`, `base = { cardUrl: "https://gw/orders/.well-known/agent-card.json", endpointUrl: "https://gw/orders/rpc", auth: null }`, `withKey = { ...base, auth: { header: "X-API-Key", value: "k1" } }`):

```ts
it.each(langs)("%s names the card URL and endpoint", (l) => { const s = buildQuickStartSnippet(l, base); expect(s).toContain(base.cardUrl); expect(s).toContain(base.endpointUrl); });
it.each(langs)("%s sends the auth header when given and none otherwise", (l) => {
  const withAuth = buildQuickStartSnippet(l, withKey); expect(withAuth).toContain("X-API-Key"); expect(withAuth).toContain("k1");
  const without = buildQuickStartSnippet(l, base); expect(without).not.toContain("X-API-Key"); expect(without).not.toContain("Authorization");
});
it.each(langs)("%s emits the unmanaged note only with the flag and no auth", (l) => {
  expect(buildQuickStartSnippet(l, { ...base, authUnmanaged: true })).toContain(UNMANAGED_COMMENT);
  expect(buildQuickStartSnippet(l, base)).not.toContain("does not manage this agent's security");
  expect(buildQuickStartSnippet(l, { ...withKey, authUnmanaged: true })).not.toContain("does not manage this agent's security");
});
it("curl posts a v1 JSON-RPC SendMessage", () => { const s = buildQuickStartSnippet("curl", base); expect(s).toContain(`-X POST '${base.endpointUrl}'`); expect(s).toContain('"method": "SendMessage"'); expect(s).toContain('"role": "ROLE_USER"'); expect(s).toContain("A2A-Version: 1.0"); });
it("curl shell-quotes a single quote", () => expect(buildQuickStartSnippet("curl", { ...base, auth: { header: "X-API-Key", value: "it's" } })).toContain(`'X-API-Key: it'\\''s'`));
it.each(["python", "typescript"] as const)("%s escapes quotes", (l) => expect(buildQuickStartSnippet(l, { ...base, auth: { header: "X-API-Key", value: 'a"b' } })).toContain('"a\\"b"'));
it.each(langs)("%s never puts a card value in a comment", (l) => {
  const lines = buildQuickStartSnippet(l, { ...base, endpointUrl: "https://x/rpc evil()//", authUnmanaged: true }).split("\n");
  for (const line of lines.filter((x) => x.startsWith("#") || x.startsWith("//"))) { expect(line).not.toContain("x/rpc"); expect(line).not.toContain("evil"); }
});
it("typescript escapes line separators", () => { const s = buildQuickStartSnippet("typescript", { ...base, endpointUrl: "https://x/ y z" }); expect(s).not.toMatch(/[  ]/); expect(s).toContain("\\u2028"); expect(s).toContain("\\u2029"); });
it("typescript has no top-level await and ends with the main footer", () => { const s = buildQuickStartSnippet("typescript", base); expect(s).not.toMatch(/^\S.*\bawait\b/m); expect(s.trimEnd().endsWith("main().catch((err) => {\n  console.error(err);\n  process.exitCode = 1;\n});")).toBe(true); });
it("emits the constant endpoint comment before the endpoint variable", () => {
  expect(buildQuickStartSnippet("python", base)).toContain(`# ${ENDPOINT_COMMENT}\nJSONRPC_URL = "${base.endpointUrl}"`);
  expect(buildQuickStartSnippet("typescript", base)).toContain(`// ${ENDPOINT_COMMENT}\nconst jsonRpcUrl = "${base.endpointUrl}";`);
});
it("builds with an empty card URL", () => expect(buildQuickStartSnippet("python", { ...base, cardUrl: "" })).toContain('CARD_URL = ""'));
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test -- src/agent/quickStartSnippets`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the builders** as one `switch` over the language with one private function per language that joins template lines with `"\n"` (the spec templates are the body; conditional lines are pushed only under their condition).

- [ ] **Step 4: Run tests and lint**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test -- src/agent/quickStartSnippets && pnpm --filter @agent-management-platform/a2a-agents lint`
Expected: PASS.

- [ ] **Step 5: Manual verification of the TypeScript snippet (scenario 76)**

Run (outside the repo): `dir=$(mktemp -d) && cd "$dir" && npm init -y >/dev/null && npm install @a2a-js/sdk tsx` — then write the output of `buildQuickStartSnippet("typescript", { cardUrl: "https://127.0.0.1:9/card", endpointUrl: "https://127.0.0.1:9/rpc", auth: { header: "X-API-Key", value: "<YOUR_API_KEY>" } })` to `client.ts` (e.g. by a one-off `node -e` that imports the built function, or by pasting the snippet from a vitest `console.log`) and run `npx tsx client.ts`; then `npm ls @a2a-js/sdk`.
Expected: no "Top-level await is currently not supported with the cjs output format" and no TS1378; the process fails only on the network request to the card URL (`ECONNREFUSED` or a fetch error). If the installed SDK rejects the request literal's fields, adjust the TypeScript template's field defaults (spec §10.3's note) and re-run Steps 4–5. Record the SDK version printed by `npm ls` in the commit body below. Delete `$dir`.

- [ ] **Step 6: Commit**

```bash
git add console/workspaces/pages/a2a-agents/src/agent/quickStartSnippets.ts console/workspaces/pages/a2a-agents/src/agent/quickStartSnippets.test.ts
git commit -m "Add the A2A quick-start snippet builders

The TypeScript snippet ran under npx tsx client.ts against @a2a-js/sdk <version>, failing only on the network call.

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 9: Card details and compare components

**Files:**
- Create: `console/workspaces/pages/a2a-agents/src/agent/A2ACardDetails.tsx`
- Create: `console/workspaces/pages/a2a-agents/src/agent/A2ACardCompare.tsx`
- Test: `console/workspaces/pages/a2a-agents/src/agent/A2ACardDetails.test.tsx`, `src/agent/A2ACardCompare.test.tsx`

**Interfaces:**
- Consumes: `CardState`, `cardState` (Task 6); `parseA2ACard`, `diffA2ACards`, `a2aCardsIdentical`, `A2ASkillItem`, `CodeBlock`, `formatRelativeTime` from `@agent-management-platform/shared-component`; `useGetAgentCard` from `@agent-management-platform/api-client`; `EnvironmentLabel` from `../utils/useEnvironmentLabel` (Part B); `AgentPathParams` from `@agent-management-platform/types`; Oxygen `Box`, `Button`, `FormControl`, `MenuItem`, `Select`, `Stack`, `Typography`.
- Produces:

```ts
// A2ACardDetails.tsx — the selected environment's card, spec §7.1–§7.3, §7.5
export const CARD_JSON_FIELD_ID = "a2a-agent-card-json";
export interface A2ACardDetailsProps { state: CardState; compare?: ReactNode }  // compare renders between Capabilities and the raw-JSON toggle, ready state only
export function A2ACardDetails(props: A2ACardDetailsProps): JSX.Element;

// A2ACardCompare.tsx — spec §7.4; owns the selection (useState) and the compare query
export const COMPARE_SELECT_LABEL = "Compare with";
export interface A2ACardCompareProps {
  params: AgentPathParams;            // orgName, projName, agentName
  baseCard: Record<string, unknown>;  // the selected environment's card body
  otherEnvironments: string[];        // entry order, selected environment excluded
  environmentLabel: EnvironmentLabel;
}
export function A2ACardCompare(props: A2ACardCompareProps): JSX.Element | null;  // null when otherEnvironments is empty
```

  Copy pinned here: `Loading card…`, `No agent card is configured for this environment.`, `Couldn't load the agent card.`, `Fetched {relative}`, `Card pending`, `Card fetch failed`, `Skills`, `No skills declared.`, `Capabilities`, `✓ streaming` / `✗ streaming`, `✓ pushNotifications`, `✓ extendedAgentCard`, `View raw JSON` / `Hide raw JSON`; compare: option `None` (value `""`), `Loading…`, `Couldn't load the card to compare.`, `No fetched card to compare.`, `Added: {name}`, `Removed: {name}`, `Changed: {name}`, `Also differs in: {fields joined ", "}`, `The cards are identical.`. The compare query is `useGetAgentCard({ ...params, envId: selected }, { enabled: selected !== "", poll: false })`. Skill items are keyed `${index}-${skill.id || skill.name}`. The select is `FormControl size="small" sx={{ maxWidth: 240 }}` with a `Typography` of id `a2a-compare-label` holding `Compare with` and `Select labelId="a2a-compare-label"` (the `ViewMCPServer` pattern).

- [ ] **Step 1: Write the failing details tests** `A2ACardDetails.test.tsx` (render inside Oxygen `ThemeProvider`; `vi.mock("@agent-management-platform/api-client", () => ({ useTrack: () => ({ track: vi.fn() }), ConsoleAction: {} }))` so the real `CodeBlock` mounts; a `ready` state built from `{ status: "fetched", fetchedAt: <2 min ago ISO>, card: { name: "Orders", description: "desc", skills: [{ id: "s1", name: "order-status", tags: ["orders"] }], capabilities: { streaming: true } } }`; scenarios 19–28 plus Review Focus 4):

```ts
it("shows the loading copy without claiming absence", ...);      // state loading → "Loading card…" present, /No agent card is configured/ absent
it("shows the absent copy", ...);                                 // absent → "No agent card is configured for this environment."
it("shows the error copy, not the absent copy", ...);             // error → "Couldn't load the agent card."; absent copy absent
it("renders only the status block for a pending card without body", ...); // noBody pending → "Card pending"; none of "No skills declared.", "✗ streaming", "Compare with"
it("shows the last error of a failed card", ...);                 // noBody failed lastError "connection refused" → "Card fetch failed" + "connection refused"
it("captions a fetched card relatively", ...);                    // "Fetched 2 minutes ago"
it("renders skills and capabilities", ...);                       // "order-status", "✓ streaming", "✗ pushNotifications", "✗ extendedAgentCard", "desc"
it("says when no skills are declared", ...);                      // card.skills: "x" → "No skills declared."
it("toggles the raw JSON", ...);                                  // no JSON before; click "View raw JSON" → text /order-status/ in a <pre>, button "Hide raw JSON"
it("renders the compare slot only when ready", ...);              // compare={<div>cmp</div>}: present for ready, absent for noBody
it("renders unnamed and duplicate skills without crashing", ...); // skills [{}, {}, { id: "x", name: "a" }, { id: "x", name: "a" }] → render does not throw; getAllByText("a") has length 2 (Review Focus 4)
it("captions an invalid fetchedAt as a dash", ...);               // fetchedAt "nope" → "Fetched -"
```

- [ ] **Step 2: Write the failing compare tests** `A2ACardCompare.test.tsx` (mock `@agent-management-platform/api-client` → `useGetAgentCard: vi.fn()`; `label = (n) => n === "prod" ? "Production" : n`; base card dev `{ skills: [skill("order-status")] }`; scenarios 29–37):

```ts
it("offers the other environments and none when alone", ...);     // combobox name /Compare with/, options "None", "Production" after mouseDown; otherEnvironments [] → container empty
it("queries the compare environment only once selected", ...);    // before: last call envId "" with { enabled: false, poll: false }; select Production → call with envId "prod", { enabled: true, poll: false }
it("lists added skills", ...);                                     // prod [order-status, issue-refund] → "Added: issue-refund"
it("lists removed and changed skills", ...);                       // dev [order-status, issue-refund], prod [order-status (other description)] → "Removed: issue-refund", "Changed: order-status"
it("lists other differing fields", ...);                           // only version differs → "Also differs in: version"; no /^Added:/
it("says identical cards are identical", ...);                     // key order + interface URLs differ only → "The cards are identical."
it("does not compare a card without a body", ...);                 // prod { status: "pending" } → "No fetched card to compare."; no "Added:"
it("reports a compare error and loading", ...);                    // isError no data → "Couldn't load the card to compare."; isLoading → "Loading…"
it("agrees with the fingerprint", ...);                            // inputModes change → "Changed: a"; reorder → "Also differs in: skills"; [a, a] vs [a] → "Also differs in: skills"; never "identical"
```

- [ ] **Step 3: Run them to verify they fail**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test -- src/agent/A2ACard`
Expected: FAIL — modules not found.

- [ ] **Step 4: Implement `A2ACardDetails`** per spec §7 (status block, then for `ready`: `parseA2ACard(state.card)` → description, `Skills` heading `subtitle2` secondary, skill items in `Stack spacing={1}`, `Capabilities` lines, `{compare}`, raw JSON `Button variant="text" size="small"` + `CodeBlock code={JSON.stringify(state.card, null, 2)} language="json" fieldId={CARD_JSON_FIELD_ID}`).

- [ ] **Step 5: Implement `A2ACardCompare`** per the Interfaces block; the result block maps `cardState(compareQuery)` to the copy table and, when ready, renders `diffA2ACards(baseCard, compareState.card)` lines in order added → removed → changed → fields → identical.

- [ ] **Step 6: Run tests and lint**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test && pnpm --filter @agent-management-platform/a2a-agents lint`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add console/workspaces/pages/a2a-agents/src/agent/A2ACardDetails.tsx console/workspaces/pages/a2a-agents/src/agent/A2ACardDetails.test.tsx console/workspaces/pages/a2a-agents/src/agent/A2ACardCompare.tsx console/workspaces/pages/a2a-agents/src/agent/A2ACardCompare.test.tsx
git commit -m "Add the A2A agent page card details and compare components

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 10: Connect drawer content — endpoints, security, test-key issuer, quick start

**Files:**
- Create: `console/workspaces/pages/a2a-agents/src/agent/manageSecurityHref.ts`
- Create: `console/workspaces/pages/a2a-agents/src/agent/A2AConnectSection.tsx`
- Create: `console/workspaces/pages/a2a-agents/src/agent/A2AConnectEndpoints.tsx`
- Create: `console/workspaces/pages/a2a-agents/src/agent/A2AConnectSecurity.tsx`
- Create: `console/workspaces/pages/a2a-agents/src/agent/A2ATestKeyIssuer.tsx`
- Create: `console/workspaces/pages/a2a-agents/src/agent/A2AQuickStart.tsx`
- Test: `console/workspaces/pages/a2a-agents/src/agent/A2AConnectSection.test.tsx`, `src/agent/A2ATestKeyIssuer.test.tsx`

**Interfaces:**
- Consumes: `resolveConnectEndpoints`, `ConnectEndpoints` (Task 7); `buildQuickStartSnippet`, `SnippetInputs`, `SNIPPET_*`, placeholders (Task 8); `useGetA2AConnect`, `useIssueTestAgentAPIKey` (Task 4); `ParsedA2ACard`, `EMPTY_A2A_CARD`, `CodeBlock`, `formatRelativeTime`, `monospaceInputSx` from `@agent-management-platform/shared-component`; `TextInput`, `buildConfigureTabHref` from `@agent-management-platform/views`; `absoluteRouteMap`, `A2ACatalogAgent`, `A2AConnectResponse`, `A2AConnectSecurity as A2AConnectSecurityModel`, `AgentCardSource`, `IssueTestAgentAPIKeyPathParams`, `IssueTestAgentAPIKeyResponse` from `@agent-management-platform/types`; Oxygen `Alert`, `Box`, `Button`, `CircularProgress`, `Stack`, `ToggleButton`, `ToggleButtonGroup`, `Typography`; router `Link`, `generatePath`.
- Produces:

```ts
// manageSecurityHref.ts — spec §9.3
export function manageSecurityHref(kind: AgentCardSource, orgId: string, projectName: string, agentName: string, environment: string): string;
// external → buildConfigureTabHref(orgId, projectName, agentName, "a2a", environment)
// platform → generatePath(absoluteRouteMap.children.org.children.projects.children.agents.children.deployment.path, { orgId, projectId: projectName, agentId: agentName })

// A2AConnectSection.tsx — the drawer body; owns the connect query and the issued key
export interface A2AConnectSectionProps { orgId: string; entry: A2ACatalogAgent; environment: string; card: ParsedA2ACard }
export function A2AConnectSection(props: A2AConnectSectionProps): JSX.Element;

// A2AConnectEndpoints.tsx — spec §9.5 rendering
export interface A2AConnectEndpointsProps { resolved: ConnectEndpoints }
export function A2AConnectEndpoints(props: A2AConnectEndpointsProps): JSX.Element;

// A2AConnectSecurity.tsx — spec §9.6 copy per mode; renders the issuer for apiKey && !isProduction
export interface A2AConnectSecurityProps {
  security: A2AConnectSecurityModel; isProduction: boolean; securitySchemes: string[];
  keyParams: IssueTestAgentAPIKeyPathParams; onKeyChange: (key: string | null) => void;
}
export function A2AConnectSecurity(props: A2AConnectSecurityProps): JSX.Element;

// A2ATestKeyIssuer.tsx — spec §9.7; owns `issued: IssueTestAgentAPIKeyResponse | null` (set from mutate's onSuccess) and reports issued?.apiKey ?? null through onKeyChange in an effect
export interface A2ATestKeyIssuerProps { params: IssueTestAgentAPIKeyPathParams; onKeyChange: (key: string | null) => void }
export function A2ATestKeyIssuer(props: A2ATestKeyIssuerProps): JSX.Element;

// A2AQuickStart.tsx — spec §9.8; owns the language (useState, default "python")
export const QUICK_START_FIELD_ID = "a2a-quick-start";
export interface A2AQuickStartProps { inputs: SnippetInputs }  // inputs.endpointUrl === "" → the no-JSON-RPC message, no code block
export function A2AQuickStart(props: A2AQuickStartProps): JSX.Element;
```

  The section derives `envParams = { orgName: orgId, projName: entry.projectName, agentName: entry.agentName, envId: environment }` once and uses it both for `useGetA2AConnect(envParams)` and as the issuer's `keyParams`. Section layout (spec §9.2): `Manage security` is `Button size="small" variant="text" component={Link} to={manageSecurityHref(entry.kind, …)}` right-aligned on top; states loading → `CircularProgress size={16}`; error && !data → `Couldn't load connection details for this environment.` (`color: "error"`); data → `[error && <Alert severity="warning">Couldn't refresh; showing the last loaded details.</Alert>]`, publication warning (§9.4: `The gateway may not yet enforce these settings.` + ` ${lastError}` when non-empty, for `pending`/`failed` only), `Endpoint`, `Security`, `Quick start` titles as `subtitle2`. Snippet inputs: `cardUrl: resolved.cardUrl`, `endpointUrl: resolved.snippetEndpoint`, `auth` by mode (`apiKey` → `{ header: security.apiKeyHeader ?? "", value: issuedKey ?? API_KEY_PLACEHOLDER }`; `oauth` → `{ header: oauth?.headerName || "Authorization", value: prefix ? `${prefix} ${ACCESS_TOKEN_PLACEHOLDER}` : ACCESS_TOKEN_PLACEHOLDER }` where `prefix = oauth?.authHeaderPrefix ?? ""`; else `null`), `authUnmanaged: security.mode === "unmanaged"`. The security section reads `oauth` through `security.oauth ?? { issuers: [], audiences: [], headerName: "", authHeaderPrefix: "" }` (Review Focus 3). Issuer copy: `Issue test key`, `Issuing…`, `Couldn't issue a test key. Try again.`, `Retry`, `Rotate`, field label `Test API key`, copy tooltip `Copy API key`, caption `Your personal test key for this environment. It expires {formatRelativeTime(expiresAt)}.`, gateway alert text as in spec §9.7.

- [ ] **Step 1: Write the failing issuer tests** `A2ATestKeyIssuer.test.tsx` (mock `@agent-management-platform/api-client` → `useIssueTestAgentAPIKey: vi.fn()`; a `mockMutation({ isPending, isError })` helper returning `{ mutate, isPending, isError, isIdle }` where `mutate = vi.fn((_, opts) => opts?.onSuccess?.(response))` for success and `opts?.onError?.(new Error("x"))` for failure; `response = { status: "ok", apiKey: "k1", expiresAt: <10 min ahead ISO> }`; `params = { orgName: "acme", projName: "p1", agentName: "orders", envId: "dev" }`; scenarios 77–82, 85):

```ts
it("issues on click only and reports the key", ...);        // no mutate on mount; click "Issue test key" → mutate called once with (params, expect.anything()); onKeyChange last called with "k1"; getByDisplayValue("k1"); label "Test API key"; /It expires in 10 minutes\./
it("reads Issuing… while the first issue is pending", ...);  // isPending true, never issued → button "Issuing…" disabled
it("warns when the gateway is disconnected", ...);          // response gatewayConnected false → the alert text; true/absent → no alert
it("rotates and replaces the key", ...);                     // issue k1; mutate now resolves k2; click "Rotate" → mutate called twice; getByDisplayValue("k2"); onKeyChange last "k2"
it("keeps the previous key when a rotate fails", ...);       // issue k1; rerender with isError true and mutate failing; click "Rotate" → "k1" still displayed; error text shown; button still "Rotate"; onKeyChange never called with null after k1
it("offers Retry after a failed first issue", ...);          // mutate fails, isError true → error text, button "Retry", no "Test API key" field, onKeyChange last null
```

- [ ] **Step 2: Write the failing section tests** `A2AConnectSection.test.tsx` (mocks: `@agent-management-platform/api-client` → `useGetA2AConnect: vi.fn(), useIssueTestAgentAPIKey: vi.fn(() => idleMutation), useTrack: () => ({ track: vi.fn() }), ConsoleAction: {}`; `@agent-management-platform/shared-component` partially mocked with `CodeBlock: ({ code }) => <pre data-testid="code-block">{code}</pre>` (so snippet text is one node) — everything else from `importOriginal`; render inside `MemoryRouter` + Oxygen `ThemeProvider`; `entry = { projectName: "p1", agentName: "orders", kind: "platform", … }`; `connect(overrides)` builds `{ kind: "platform", proxied: false, isProduction: false, gatewayUrl: "https://gw.example/orders", cardUrl: "https://gw.example/orders/.well-known/agent-card.json", jsonRpcUrl: "https://gw.example/orders/rpc", security: { mode: "apiKey", apiKeyHeader: "X-API-Key" }, publication: { status: "published", lastError: "" }, ...overrides }`; `card = parseA2ACard({ supportedInterfaces: [{ url: "http://localhost:9099/rpc", protocolBinding: "JSONRPC" }], securitySchemes: { bearer: {} } })`; scenarios 44–54, 56, 63–65, plus Review Focus 3):

```ts
it("shows a spinner while loading", ...);                                  // isLoading, no data → role "progressbar"; no "Endpoint" text
it("shows the error copy without data", ...);                              // "Couldn't load connection details for this environment."
it("keeps the loaded body and key on a failed refetch", ...);              // data + key issued (mutate onSuccess k1); rerender with isError true → endpoints, "k1" in field and in code-block, warning "Couldn't refresh; showing the last loaded details."
it("describes API-key security outside production", ...);                 // "API key required in header", code "X-API-Key", button "Issue test key", mutate not called, code-block contains "<YOUR_API_KEY>"
it("says production keys come from the owners", ...);                     // isProduction true → the sentence; no "Issue test key"
it("describes OAuth", ...);                                                // issuers [corp-idp https://idp/x, gone-idp ""], audiences ["orders-api"], Authorization/Bearer → all §9.6 strings incl. "gone-idp — not found", "Audiences: orders-api"; no issuer; code-block contains "Authorization" and "Bearer <ACCESS_TOKEN>"; audiences [] → "Audiences: any"
it("survives OAuth without an oauth object", ...);                         // security { mode: "oauth" } → "Authorization", "Issuers:", "Audiences: any" (Review Focus 3)
it("describes no, unknown and unmanaged security", ...);                   // none → "No authentication required." and no "X-API-Key" in code; unknown → "Security settings couldn't be determined for this environment." and no "does not manage this agent's security" in code; unmanaged direct (gatewayUrl "", jsonRpcUrl "http://localhost:9099/rpc", publication none) → "Security isn't managed by the platform. The card declares: bearer.", no "Gateway endpoint", first textbox value "http://localhost:9099/rpc", code contains the unmanaged note; card with no schemes → "…declares: nothing."
it("links Manage security per kind", ...);                                 // platform → href "/org/acme/project/p1/agents/orders/deployment"; external + env dev → "/org/acme/project/p1/agents/orders/configure?tab=a2a&env=dev"
it("warns about pending or failed publication", ...);                      // failed + "gateway rejected the policy" → both texts; pending + "" → sentence only; published / none → no alert
it("addresses a proxied agent at the gateway", ...);                       // proxied true, gatewayUrl "https://gw/agents/x", jsonRpcUrl ".../rpc", origin interface → first textbox = gateway URL; "Declared by the card" + origin text; select curl → code contains "https://gw/agents/x" and not the origin URL
it("replaces the quick start when there is no JSON-RPC endpoint", ...);    // jsonRpcUrl "" → "This agent declares no JSON-RPC endpoint, so there is no quick start."; no code-block
it("says when a direct card declares no endpoints", ...);                  // gatewayUrl "", jsonRpcUrl "", card = EMPTY_A2A_CARD → "The card declares no endpoints yet."; no textbox
it("shows the Agent card row only with a card URL", ...);                  // label "Agent card" present / absent
it("switches snippet languages", ...);                                     // toggle buttons Python/TypeScript/curl; Python pressed by default; click curl → code starts with "# Fetch the agent card"; clicking the pressed button keeps it
```

- [ ] **Step 3: Run them to verify they fail**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test -- src/agent/A2AConnect src/agent/A2ATestKeyIssuer`
Expected: FAIL — modules not found.

- [ ] **Step 4: Implement `manageSecurityHref.ts`, `A2ATestKeyIssuer.tsx`, `A2AQuickStart.tsx`, `A2AConnectEndpoints.tsx`, `A2AConnectSecurity.tsx`, `A2AConnectSection.tsx`** per the Interfaces block. Endpoint rows and the key field are `TextInput` with `label`, `value`, `copyable`, `size="small"`, `fullWidth`, `slotProps={{ input: { readOnly: true } }}` and `sx={monospaceInputSx}`; the key field passes `copyTooltipText="Copy API key"`. The language toggle is `ToggleButtonGroup exclusive size="small"` whose `onChange` ignores `null`. The warning `Alert` in the section sits in a fixed slot (`{isError && data ? <Alert…/> : null}`) so the tree below it keeps its position across the error flip.

- [ ] **Step 5: Run tests and lint**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test && pnpm --filter @agent-management-platform/a2a-agents lint`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add console/workspaces/pages/a2a-agents/src/agent/manageSecurityHref.ts console/workspaces/pages/a2a-agents/src/agent/A2AConnectSection.tsx console/workspaces/pages/a2a-agents/src/agent/A2AConnectSection.test.tsx console/workspaces/pages/a2a-agents/src/agent/A2AConnectEndpoints.tsx console/workspaces/pages/a2a-agents/src/agent/A2AConnectSecurity.tsx console/workspaces/pages/a2a-agents/src/agent/A2ATestKeyIssuer.tsx console/workspaces/pages/a2a-agents/src/agent/A2ATestKeyIssuer.test.tsx console/workspaces/pages/a2a-agents/src/agent/A2AQuickStart.tsx
git commit -m "Add the A2A connect drawer content with its security summary, test-key issuer and quick start

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

### Task 11: The agent page — shell, header, chips, per-environment details and the Connect drawer

**Files:**
- Create: `console/workspaces/pages/a2a-agents/src/agent/A2AAgentDetails.tsx`
- Replace: `console/workspaces/pages/a2a-agents/src/A2AAgentPage.tsx` (the Part B placeholder; keep the `A2AAgentPage` named export)
- Test: `console/workspaces/pages/a2a-agents/src/A2AAgentPage.test.tsx`
- Verify unchanged: `console/workspaces/pages/a2a-agents/src/A2AAgents.Organization.test.tsx` (its `"renders the agent page route"` test reads `getByTestId("a2a-agent-page")` — keep that test id on the page's root `Box`)

**Interfaces:**
- Consumes: `useGetA2ACatalogEntry`, `useGetAgentCard` from `@agent-management-platform/api-client`; `a2aAgentsListPath` (`../routes`), `useReplaceSearchParams` (`../utils/useReplaceSearchParams`), `useEnvironmentLabel`, `EnvironmentLabel` (`../utils/useEnvironmentLabel`) — all Part B Task 6; `cardState`, `CardState` (Task 6); `A2ACardDetails`, `A2ACardCompare` (Task 9); `A2AConnectSection` (Task 10); `parseA2ACard`, `EMPTY_A2A_CARD`, `normalizeVersion` from `@agent-management-platform/shared-component`; `PageLayout`, `DrawerWrapper`, `DrawerHeader`, `DrawerContent` from `@agent-management-platform/views`; `A2ACatalogAgent` from `@agent-management-platform/types`; Oxygen `Alert`, `Box`, `Button`, `Chip`, `Stack`, `Typography`; icon `Plug` from `@wso2/oxygen-ui-icons-react`.
- Produces:

```ts
// agent/A2AAgentDetails.tsx — everything that resets per environment; the page renders it with key={environment}
export interface A2AAgentDetailsProps {
  orgId: string; entry: A2ACatalogAgent; environment: string; environmentLabel: EnvironmentLabel;
  state: CardState;        // the selected environment's card state (queried by the page, which also needs it for the header)
  chips: ReactNode;        // rendered left of the Connect button on one row
}
export function A2AAgentDetails(props: A2AAgentDetailsProps): JSX.Element;
// Row: chips (wrapping) | Button variant="contained" size="small" startIcon={<Plug size={16} />} "Connect".
// Then <A2ACardDetails state compare={<A2ACardCompare …/>} /> (compare only when state is ready; otherEnvironments = entry.environments names minus `environment`).
// Drawer: not rendered until the first click (`mounted` state), then <DrawerWrapper open keepMounted maxWidth={700} onClose={close}> with
// <DrawerHeader icon={<Plug size={20} />} title={`Connect · ${environmentLabel(environment)}`} onClose={close} /> and
// <DrawerContent><A2AConnectSection orgId entry environment card={state.state === "ready" ? parseA2ACard(state.card) : EMPTY_A2A_CARD} /></DrawerContent>.

// src/A2AAgentPage.tsx
export const A2A_AGENT_PAGE_TEST_ID = "a2a-agent-page";
export const A2AAgentPage: React.FC;
```

  Page rules (spec §5): params from `useParams()`; `entryQuery = useGetA2ACatalogEntry({ orgName: orgId, projName: projectName, agentName })`; `[params, replaceParams] = useReplaceSearchParams()`; `requested = params.get("env") ?? ""`; `environment = entry && entry.environments.some((e) => e.name === requested) ? requested : entry?.leadEnvironment ?? ""` (Review Focus 1); `cardQuery = useGetAgentCard({ orgName: orgId, projName: projectName, agentName, envId: environment }, { enabled: !!entry && environment !== "" })`; `state = cardState(cardQuery)`. Header: title = ready card `name` (non-empty) else `entry.name`, while loading the `agentName` param; version = ready card `version` else `entry.version`, chip `Chip size="small" variant="outlined" label={normalizeVersion(version)}` only when non-empty; kind chip `Chip size="small" label={cardQuery.data?.source ?? entry.kind}`; both in `titleTail` once the entry has loaded; `description` = `projectName` + ` · ${providerOrganization}` when the ready card's provider organization is non-empty; `PageLayout avatar={null} backHref={a2aAgentsListPath(orgId)} backLabel="Back to A2A Agents" isLoading={entryQuery.isLoading && !entry}`. Body by entry state: `null` → `This agent was not found in the catalogue.` (`color: "text.secondary"`); error without data → `Alert severity="error"` `Failed to load the agent. Please try again.`; data → fixed-slot `{entryQuery.isError ? <Alert severity="warning">Couldn't refresh; showing the last loaded details.</Alert> : null}` then, when `environment !== ""`, `<A2AAgentDetails key={environment} … chips={…} />` (Review Focus 5: no details when the entry lists no environments). Chips: one `Chip` per `entry.environments` in order, `label={environmentLabel(name)}`, selected → `color="primary" variant="filled"`, others `variant="outlined"`, `onClick={() => replaceParams((p) => p.set("env", name))}`.

- [ ] **Step 1: Write the failing page tests** `A2AAgentPage.test.tsx`. Harness: `vi.mock("@agent-management-platform/api-client", () => ({ useGetA2ACatalogEntry: vi.fn(), useGetAgentCard: vi.fn(), useGetA2AConnect: vi.fn(), useIssueTestAgentAPIKey: vi.fn(), useListEnvironments: vi.fn(() => ({ data: [{ name: "dev", displayName: "Development" }, { name: "prod", displayName: "Production", isProduction: true }, { name: "staging" }] })), useTrack: () => ({ track: vi.fn() }), ConsoleAction: {} }))`; `@agent-management-platform/shared-component` partially mocked with the `CodeBlock` stand-in from Task 10; render `<ThemeProvider theme={createTheme()}><MemoryRouter initialEntries={[url]}><Routes><Route path="/org/:orgId/a2a-agents/view/:projectName/:agentName" element={<><A2AAgentPage /><LocationProbe /></>} /></Routes></MemoryRouter></ThemeProvider>` with `url` defaulting to `/org/acme/a2a-agents/view/p1/orders`; `LocationProbe` renders `useLocation().search` into `data-testid="location"`. Entry: `{ projectName: "p1", agentName: "orders", kind: "platform", name: "Orders", description: "", version: "1.0", skills: [], leadEnvironment: "dev", environments: [dev fetched, prod fetched, staging pending] }`. Card hook: `mockImplementation((p) => cards[p.envId] ?? loadingCard)` where `cards.dev` is ready with skill `order-status` and `cards.prod` ready with `issue-refund`. Connect hook: `mockImplementation((p) => ({ data: connectFor(p.envId), isLoading: false, isError: false }))` with `prod` `isProduction: true`. Mutation: the Task 10 `mutate` stand-in resolving `k1`.

```ts
describe("shell", () => {
  it("reads the agent from the URL", ...);                       // entry hook called with { orgName: "acme", projName: "p1", agentName: "orders" }; heading "Orders"
  it("shows no claims while loading", ...);                      // { data: undefined, isLoading: true } → no "platform", "external", "order-status", "Loading card…"
  it("says when the agent is not in the catalogue", ...);        // data null → "This agent was not found in the catalogue."
  it("shows the entry error without data", ...);                 // { data: undefined, isError: true } → "Failed to load the agent. Please try again."; nothing else
  it("keeps the page and the issued key through a failed entry refetch", ...); // open Connect, click "Issue test key", "k1" in field and code-block; rerender with { data, isError: true } → still "k1" twice, "order-status", warning text; no "Failed to load the agent"
  it("opens on the lead environment", ...);                      // the "Development" chip has class "MuiChip-colorPrimary" and "Production" does not; card hook last called with envId "dev", { enabled: true }
  it("opens on the URL's environment and switches from the chips", ...); // url ?env=prod&foo=1 → "issue-refund"; click "Development" → location "?env=dev&foo=1"; "issue-refund" gone, "order-status" shown
  it("falls back to the lead for an unknown env", ...);          // ?env=ghost → "order-status" shown, "Development" selected, location still "?env=ghost" (Review Focus 1)
  it("labels chips with display names", ...);                    // "Development", "Production", "staging"
  it("takes title and version from the ready card", ...);        // dev card name "Orders Agent" version "2.1" → heading "Orders Agent", chip "v2.1"; dev noBody → "Orders", "v1.0"
  it("labels the kind from the card response", ...);             // dev source "external" → chip "external"; cards loading → "platform"
  it("describes the project and provider", ...);                 // provider.organization "Acme" → "p1 · Acme"; none → "p1"
  it("links back to the catalogue", ...);                        // link "Back to A2A Agents" href "/org/acme/a2a-agents"
  it("waits for the entry before requesting the card", ...);     // entry loading → card hook called with expect.objectContaining({ enabled: false })
  it("renders only the header for an entry without environments", ...); // environments [], leadEnvironment "" → heading "Orders", no "Connect", no chips, card hook enabled false (Review Focus 5)
});
describe("connect drawer", () => {
  it("mounts the connect section on click only", ...);          // useGetA2AConnect not called; click "Connect" → heading "Connect · Development"; connect hook called with envId "dev"
  it("keeps the drawer content mounted after closing", ...);     // issue k1; press Escape on the drawer (fireEvent.keyDown(screen.getByRole("presentation"), { key: "Escape" })) → drawer hidden but getByDisplayValue("k1") still in the DOM; click "Connect" → visible again with "k1"
  it("works while the selected card is absent", ...);            // dev card null → click Connect → "Gateway endpoint" row, no crash, "Security isn't managed" absent for apiKey (Review Focus 2)
  it("remounts the details on an environment change", ...);     // select Production in Compare with; open drawer; click "staging" → no combobox selection (getByRole combobox shows "None" after re-render), no dialog/presentation in the DOM
  it("resets the issued key on an environment change", ...);    // issue k1 in dev; click "staging"; open Connect → "Issue test key", no "k1", mutate called once in total; click "Development", open Connect → "Issue test key", no "k1"
  it("drops the key for a production environment", ...);        // issue k1; click "Production"; open Connect → code-block contains "<YOUR_API_KEY>"; no "Issue test key"; back to dev → no "k1"
});
describe("keyboard", () => {
  it("Esc closes only the compare menu", ...);                   // mouseDown combobox → listbox; keyDown Escape on the listbox → listbox gone; location unchanged; heading still "Orders"
  it("Esc closes only the drawer", ...);                         // open; Escape → drawer hidden; location unchanged
});
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test -- src/A2AAgentPage`
Expected: FAIL — the placeholder renders neither the entry hook call nor the heading.

- [ ] **Step 3: Implement `A2AAgentDetails.tsx` and rewrite `A2AAgentPage.tsx`** per the Interfaces block and page rules.

- [ ] **Step 4: Run the package tests, lint and build**

Run: `cd console && pnpm --filter @agent-management-platform/a2a-agents test && pnpm --filter @agent-management-platform/a2a-agents lint && pnpm --filter @agent-management-platform/a2a-agents run build`
Expected: PASS (including the Part B `A2AAgents.Organization.test.tsx`); `dist/index.js` exists.

- [ ] **Step 5: Build core-ui against the new page**

Run: `cd console && pnpm --filter @agent-management-platform/am-core-ui run build`
Expected: exit 0.

- [ ] **Step 6: Manual check (skip if no dev backend is reachable and say so in the task report)**: `cd console && make dev`, open an agent from `http://localhost:3000/org/<org>/a2a-agents`: the chips switch environments with `env=` replacing the URL, Connect opens the drawer with the endpoint rows, the security copy and the three snippet tabs; `Issue test key` fills the key into the snippet; browser back returns to the list. Stop the server.

- [ ] **Step 7: Commit**

```bash
git add console/workspaces/pages/a2a-agents/src/A2AAgentPage.tsx console/workspaces/pages/a2a-agents/src/A2AAgentPage.test.tsx console/workspaces/pages/a2a-agents/src/agent/A2AAgentDetails.tsx
git commit -m "Replace the A2A agent page placeholder with the agent page

Claude-Session: https://claude.ai/code/session_01E9PcFjLbL5S9wj6qUmSh3Q"
```

---

## Decisions recorded (spec ambiguities this plan resolves)

1. **Unknown `env` query value** (not in the spec): the page shows the lead environment and leaves the URL untouched; the next chip click rewrites `env`.
2. **Entry with no environments**: header only — no chips, no Connect, no card request.
3. **`Quick start` title with no JSON-RPC endpoint (A-6)**: the `Quick start` `subtitle2` title stays; the message replaces the toggle and code block beneath it.
4. **Compare select label (A-7)**: a `Typography` with an id referenced by `Select labelId` (the `ViewMCPServer` pattern); the accessible name therefore starts with `Compare with`.
5. **Header title with a ready card whose `name` is empty (A-2)**: falls back to the entry's name; the version is still the card's.
6. **Issued key ownership**: `A2ATestKeyIssuer` owns the last successful response (survives a failed rotate); `A2AConnectSection` mirrors only the key string for the snippet through `onKeyChange`.
7. **Drawer close on Esc / backdrop / icon**: all go through one `close` handler; the drawer is created on first open and kept mounted (`keepMounted`) until the environment changes.
8. **Fixed warning slots**: the two "Couldn't refresh" alerts render in always-present slots so a refetch error never changes the position (and thus the identity) of the subtree that holds the issued key.
9. **`fetchedAt` missing or invalid on a `fetched` card** renders `Fetched -` (the `formatRelativeTime` fallback) rather than inventing new copy.
10. **OAuth without an `oauth` object** is read through an all-empty default so the section renders the defaults instead of throwing.
11. **Manual TypeScript verification** records the `@a2a-js/sdk` version in the Task 8 commit body.
12. **Query key for connect details** is the spec's `["a2a-connect", params]`; the test-key mutation performs no invalidation (A-10 kept).
