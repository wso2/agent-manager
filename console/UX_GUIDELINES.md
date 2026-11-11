# Console UI/UX guidelines

Rules for building pages in the Agent Manager console. They apply to people and to coding agents. When a page breaks one of these rules, fix it in the same change if it's small, or note it.

Use Oxygen UI (`@wso2/oxygen-ui`, `@wso2/oxygen-ui-icons-react`) components and theme tokens only. Component docs are in [`.claude/oxygen-ui/`](../.claude/oxygen-ui/).

## 1. Page types and where the primary action goes

| Page | Primary action | Where |
| --- | --- | --- |
| Listing page (agents, projects, gateways, LLM providers…) | **Create** the resource | `PageLayout` `actions` (page header, top right) |
| Resource page (overview / view page) | **Edit** the resource | `PageLayout` `actions` |
| Create / edit page | Save / Create | Form action row at the bottom of the form, left-aligned |

- The primary CTA always sits in the page action position. Don't place it inside a card header, toolbar or table when the page has a header.
- One contained (primary) button per page or card. Other actions are `outlined` or `text`.
- Buttons, menu items and navigation labels use sentence case: "Create project", "Save changes", "Runtime logs". Keep acronyms and product names as written ("Add LLM provider", "Register MCP server").
- **Exception:** a list that sits inside a tabbed card puts its create button inside that card, next to the tabs, not in the page header. The header action must make sense for the whole page.

## 2. Create and edit happen on pages, not drawers

- **Create and edit are full pages** with their own routes (`…/create`, `…/edit/:id`). Don't open a drawer to create or edit a resource.
- **Drawers are only for** a details view, or a small configuration that belongs to the current page (a few fields, no navigation).
- Confirmation of destructive actions uses the confirmation dialog (`useConfirmationDialog`), not a drawer.

Create/edit pages must:
- Return to where they were opened. Pass `state={returnToState(location)}` on the link or `navigate` call, and read the target with `useReturnTo(fallbackPath)` (both from `@agent-management-platform/views`). Edit pages return to the resource's own page.
- Guard unsaved changes with `useUnsavedChangesGuard(isDirty)`. Cancel and a successful save call `allowNavigation(...)` so they don't prompt. Values the form fills in by itself (defaults) are not user changes and must not make the form dirty.

## 3. Listing pages

Build lists with `ResourceListShell` (`@agent-management-platform/shared-component`).

- **No items:** show the empty state with the create action inside it (`emptyState.action`). Hide the search bar and toolbar, and hide the header create button, so there is only one create button on screen. Use `isListEmpty({ isLoading, count, isFiltered })` to decide.
- **Items exist:** show the search bar at the left of a toolbar row above the table or cards, with `width="standard"` (400px), not full width. Sort and view toggles go on the right of the same row. The create button is in the page header.
- **Search with no matches:** show `searchEmptyState`, not the "no items" empty state, and keep the search bar visible.
- **Load error:** show the error only. Never show "No X found / Create one" when the request failed. A failed fetch is not an empty list.
- Loading: skeleton rows/cards (`loadingPlaceholder`), not a spinner.

### Row and card actions

- **Listing cards (`ListingCard`):** delete is an **outlined** button with a label, not an icon button. Make the card clickable with `onClick` and `navigate(...)`. Don't wrap the card in a `<Link>`, because that nests the delete button inside an anchor.
- **Tables (non-card lists):** row actions are **icon buttons** with a `Tooltip` and `aria-label`.
- Show a "Created" or "Last updated" column only for the timestamp you actually have. If the API only returns `createdAt`, the column is "Created".

## 4. Resource (view) pages

- The edit button is the primary action in the page header.
- **Tabbed detail pages keep their card.** When a view page uses tabs, the tab content sits inside a `Form.Section` / card. Don't strip the card from view pages; only plain forms drop it.
- Danger actions (delete) go in a danger zone at the bottom, not in the header.

## 5. Forms

Use `TextInput` from `@agent-management-platform/views` for text fields **and** selects (`select` prop with `MenuItem` children). Don't mix raw `TextField`, `Select` and `FormControl` with `TextInput`, because their heights and labels differ.

`TextInput` takes only its own props. MUI `Select` props such as `displayEmpty` are not accepted and break the CI build (`tsconfig.lib.json`).

### Grid layout

Forms use Oxygen `Grid` with `size={{ ... }}`:

| Field kind | Size |
| --- | --- |
| Short field (name, version, select, number) | `{ xs: 12, md: 6, xl: 4 }` |
| Long field (description, URL, API key, endpoint) | `{ xs: 12, xl: 8 }` |
| Full-width block (resilience/timeout group, editors, tables) | `{ xs: 12 }` |

- At most **two short fields per row**. Put `<FormGridSpacer />` (or `columns={8}`) after a row to force the next field onto a new row at xl.
- Fields that belong together (method and header key, quota, duration and unit) share one row.

### Labels and validation

- **No floating labels.** Don't pass `label` to `TextField` (MUI's floating/shrinking label), including inside an `Autocomplete` `renderInput`. Put a static label above the field: `FormControl` + `FormLabel`, `Form.ElementWrapper label=...`, or the `TextInput` component (`libs/views/src/component/FormElements/TextInput.tsx`).
- **Don't mark required fields with `*`.** Mark optional ones with an "(optional)" suffix in the label instead. Don't use `<FormLabel required>`.
- Every text input has a placeholder showing an example value ("e.g. github-mcp-server", "1.0.0", "/github").
- Show character counters only near the limit (`hideCharacterCount` until close), not on every field.
- Client limits must match the backend. For example, agent names are capped at 25 characters (`MaxResourceNameLength`). Keep each limit in one exported constant shared by the schema and the field.
- Use `useFormValidation` with a zod schema. Don't call setters or validation inside a `setState` updater; updaters must be pure.

### Action row

- The Cancel / Save row is **left-aligned** (`justifyContent: "flex-start"`), including in drawers and wizards.
- Cancel is `outlined`; the submit button is `contained` and says what it does ("Create project", "Save"), not "Submit".

## 6. Empty, error and "not ready" states

Use `StateNotice` (`@agent-management-platform/views`) for any state that blocks a page: OAuth-only security, missing config, load errors.

- Give a title that names the state ("Configure your agent first"), one sentence of explanation, and **one** action that fixes it ("Go to Configure").
- Don't repeat the same action in two places on one page. If two cards are both empty for the same reason, only one carries the button.
- Avoid step indicators ("1 Build, 2 Deploy, 3 Try it") in small cards; they crowd the layout.
- Use `compact` inside cards and `contained` when the notice needs its own panel.
- Errors say what failed and what to do next; they don't apologise.

## 7. Links and routes

- Build every link with `generatePath(absoluteRouteMap...path, params)`. Don't hand-write URLs.
- Use the route the page actually lives at. For example, an agent's traces page is `agents.children.observability.children.traces`. Check `libs/types/src/routes/routes.map.ts` before adding a link.
- Keep navigation client-side with router `Link` / `navigate`. Avoid plain `href` anchors to app pages.

## 8. Layout and responsiveness

- Every page must work from phone width up: no horizontal page scroll, and stack cards on `xs`.
- Cards that sit side by side and show the same kind of content have the same height (`minHeight`, or a flex row with `alignItems: "stretch"`). Footer links in a card ("More projects") are pinned to the bottom (`mt: "auto"` in a flex column).
- Code or script boxes: the scroll area and the copy/icon buttons are separate elements, so the icons don't scroll away and don't cover the text.

## 9. Styling rules

- Use theme tokens (`text.secondary`, `primary.main`, `divider`, `action.hover`…). **No hex colours.**
- Keep the console's own colour layout. When a mockup comes from elsewhere, copy its structure, not its colours.
- Don't restyle Oxygen internals with `[class*="Slot"]` selectors. Those class names exist only in dev builds, so the rule does nothing in production.
- Motion only where it carries meaning (for example, the rotating overview cards). Respect `prefers-reduced-motion`.
- Timestamps go through `formatRelativeTime` from `@agent-management-platform/shared-component`, not `date-fns` directly.

## 10. Code structure

- One page or feature per folder, named after the component: `OrganizationOverview/OrganizationOverview.tsx`, with `subComponents/` for its pieces and `hooks/` for its hooks. Don't leave helper components loose in a package's `src/`.
- File names match the exported component, with correct spelling (`AgentTypeSummary`, not `AgentTypeSummery`).
- Code comments are short (one to three lines) and explain why, not what.
- Shared UI goes in `libs/views` (presentational) or `libs/shared-component` (data-aware). After changing a lib, run `pnpm run build` in it so other packages see the change.

## 11. Before you push

For each package you touched:

```bash
pnpm exec tsc --noEmit -p tsconfig.lib.json   # what CI builds; must be clean
pnpm exec eslint --config ./eslint.config.js src
```

- Check the page in the browser (light and dark mode, a narrow window), including its empty, loading and error states.
- If you removed or renamed a shared prop, search the whole console for callers, including test mocks. A branch merged from `main` may still use the old prop.
