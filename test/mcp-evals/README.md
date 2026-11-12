# Agent Manager MCP evals

[promptfoo](https://www.promptfoo.dev) evals for the Agent Manager MCP server (`agent-manager-service/mcp/`). The agent is Claude Code through the Agent SDK provider, limited to this server's tools, against a real running server. There are no mocks: every tool call reaches the server you point it at.

This directory holds only what is specific to this server: `suite.json`, `promptfooconfig.yaml` and `tests/`. The sign-in, runner, compare, tools diff, cost cap and safety settings come from `@wso2/eval-kit` (`tools/eval-kit` in `wso2/agent-skills`; see its README). Until that is released upstream, `package.json` installs a test release from a fork.

## Prerequisites

- Node 22.22 or later, then `npm ci` here.
- Claude Code signed in (`claude`, then `/login`). Runs use that login, not an API key; the kit never passes `ANTHROPIC_API_KEY` on.
- A running Agent Manager: `make setup` once, then `make thunder-up` and `make dev-up`. The MCP server is then at http://localhost:9000/mcp. Create an agent or two in the console first: against an empty org, `list-agents` only checks that the reply says there are none.

## How to use

- `npm run doctor`: Node, promptfoo and SDK versions, git-ignores, the sign-in and `tools/list`.
- `npm run eval`: signs in, runs every case 3 times, prints each case's pass count, then compares with the baseline. A case passes when 2 of its 3 runs pass. The first sign-in opens a browser for the `am-mcp` client; later runs reuse or refresh the token in `.state/`. `-- --label <name>` names the run (`latest`, `baseline` and `previous` are reserved), `-- --budget <USD>` caps its cost (default 2).
- `npm run list`, `npm run show -- [ref] [case]`: past runs, and one run's replies, judge reasons and tool calls, redacted.
- `npm run pin -- <ref>`: makes that run the baseline. A ref is an eval id, a label, `latest` or `baseline`.
- `npm run compare -- [A] [B]`: per-case verdict (BETTER, WORSE, NO CHANGE, INCONCLUSIVE or NOT COMPARABLE), by default the baseline against the latest run. Runs against different MCP URLs are NOT COMPARABLE.
- `npm run tools -- --diff`: the tool fields that changed since the baseline. `npm run tools` prints the current `tools/list`.
- `npm run view`: promptfoo's viewer, on 127.0.0.1.
- The edit loop: pin a good run, edit `agent-manager-service/mcp/` (the dev server rebuilds), check `tools -- --diff`, run `eval`, then `compare`. Pin the new run when you keep the change.
- `MCP_URL` and `MCP_AUTH_FROM` point a run at another server; the token comes from `MCP_AUTH_FROM` and goes to `MCP_URL`. To keep an override, put it in `suite.local.json` (git-ignored), e.g. `{"url": "http://localhost:9001/mcp"}`. Tool outputs go to the model and into `results/`, so never point a run at a server that holds customer data. Pass promptfoo flags after `--`, for example `npm run eval -- --filter-pattern list-agents`.
- Exit codes: 0 ok, 1 failing cases or WORSE, 2 usage, 3 sign-in or `tools/list` failed, 4 a run errored, was cut short by the budget (partial) or saw the tools change mid-run (tainted).
- No retries: a rate-limited or failed call is an ERROR row, never a second conversation. promptfoo reads no `.env` file and gets no `ANTHROPIC_API_KEY`.

## Adding a case

Add `tests/<case>.yaml`. Check tool calls first with `calledAnyOf` (the kit's `lib/asserts.js`), then judge with the kit's `lib/judge.js:verdict`, which gets the reply and the tool results and answers PASS, FAIL or UNKNOWN; a judge error is an ERROR row, never a verdict. Grade against the tool results instead of typing expected data into the rubric, so cases keep working when the server's data changes. The agent may call only the read tools in `custom_allowed_tools`. A case that writes needs its own `provider` that also allows the write tools it uses; it changes the server you point at and runs 3 times, so it must not depend on its earlier runs, and you run it with `-j 1`.

`.state/` (tokens), `.promptfoo/` and `results/` (prompts, replies and tool outputs) are git-ignored and owner-only; do not share them. A run starts no call once its budget is spent; calls already running can go over it by up to their own `max_budget_usd` each.
