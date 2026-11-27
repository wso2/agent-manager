# Console browser journeys (Playwright)

Browser end-to-end tests for the AMP console, run with Playwright in Chromium against the Docker Compose stack (`deployments/standalone`), where `amp-api` serves the console on the API's port. Each journey asserts what the user ends up seeing, and reads the result back through the API where the UI's word alone would not prove it.

This package has its own `package.json` and lockfile; it is not part of the console's pnpm workspace.

## Journeys

| Spec | Journey |
|---|---|
| `sign-in.spec.ts` | Sign in through the form (and a wrong password refused), then the organization overview |
| `projects.spec.ts` | Create, edit and delete a project |
| `external-agent.spec.ts` | Register an external agent and copy its token from the setup drawer |
| `llm-provider.spec.ts` | Add an organization LLM provider and attach it to an agent; the LLM proxy that creates issues an API key and the provider lists the agent as a consumer |
| `gateways.spec.ts` | Register a self-hosted gateway, issue and revoke a registration token, remove the gateway |
| `members.spec.ts` | Invite a user by email; add a user, assign a role, see both in the member list |
| `subscription-plans.spec.ts` | Create a plan with a tool-call limit; edit its description and switch its status; an MCP server offers it but never a deprecated plan; deprecate it, after which it is read only and its offer stays. Skipped when the AI Hub module isn't installed; the MCP server step is skipped when no active gateway serves the default environment |

The console has no page of its own that creates a project-level LLM proxy; attaching a provider to an agent creates one, so `llm-provider.spec.ts` covers it there.

## Run it

```bash
make up                       # at the repository root: the stack with the console, file sign-in
cd test/ui-e2e
npm ci
npx playwright install chromium
npx playwright test
```

The stack must run with file sign-in (`auth.mode: file`, the default). Settings, all optional:

| Variable | Default | Meaning |
|---|---|---|
| `AMP_BASE_URL` | `http://localhost:$AMP_API_PORT` | Where the console and API are served |
| `AMP_API_PORT` | `9000` | Used when `AMP_BASE_URL` is unset |
| `AMP_ADMIN_PASSWORD` | read from the running `amp-api` container | The admin's password, found the way `deployments/standalone/scripts/smoke.sh` finds it |
| `COMPOSE_PROJECT_NAME` | `amp-lite` | The Compose project whose `amp-api` holds the generated password |
| `AMP_ORG` | `default` | The organization the journeys run in |

`npx playwright test --headed`, `--ui` and `--debug` help while writing a spec. After a failure, `npx playwright show-report` opens the report, with a trace and a screenshot for each failed test.

## Writing a spec

- One page object per page, in `pages/`. Find elements by role and accessible name (`getByRole`, `getByLabel`); no CSS or test-id selectors. A control the browser cannot name is an accessibility bug: give it an `aria-label` in the component.
- Import `test` and `expect` from `support/fixtures.ts`. It starts every test signed in as the admin (global setup signs in through the API and stores the console's session) and gives each worker its own project (`workerProject`) and an API client (`api`) for setting up, cleaning up and reading back.
- Name what you create with `uniqueName()` (prefix `uie2e-`), and remove it in `afterAll` through `api`, so a failed run leaves nothing a rerun trips over.
- Wait on what the user sees (`expect(...).toBeVisible()`, `expect.poll` for API read-back), never on time.
- Specs that sign in through the form import `test` from `@playwright/test` instead, so they start signed out.
