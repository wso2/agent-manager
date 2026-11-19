# Shadow Agent Runner

`amp-shadow-runner` is the source code scan of Shadow Agent Discovery in WSO2 Agent Manager. It finds AI agents in GitHub and Azure DevOps repositories and publishes the confirmed agents to Agent Manager.

It runs on the customer's machine, as the `docker run` command that the Agent Manager console builds for a source code platform. When complete, the command will ask for the Git token only for private repositories, and for the LLM key only with AI review. It will then fetch the repositories, scan them, delete the code and publish only the confirmed agents. Agent Manager never stores the Git token and never pulls the code.

The package has two parts:

- `amp_shadow_runner`: the command line.
- `amp_shadow_runner.engine`: the scan engine, a pure library that parses a local source tree with tree-sitter. It never opens a network connection or starts a process (`tests/engine/test_purity.py` enforces this).

> `scan` works for GitHub with the rules for Python, TypeScript, JavaScript, C# and Ballerina agents (Ballerina matches are always Not sure). Azure DevOps comes in a later version. With `--ai-review` the command already asks for the LLM settings and has the LLM client, but it does not review yet, so Not sure agents are still not published.

## Command line

```bash
amp-shadow-runner --version
amp-shadow-runner version --json
amp-shadow-runner scan --scan-id <id> --github-org <org> --all-repos
amp-shadow-runner upload --scan-id <id> [/out/report.json]
```

Agent Manager builds the `scan` command, so its options match what the console shows. `AMP_URL` (the Agent Manager address) and `AMP_SCAN_TOKEN` (the scan token, valid for 1 hour) come from the environment. A scan needs at least 10 minutes left on the token.

| Option | Meaning |
|---|---|
| `--scan-id <id>` | The scan that Agent Manager issued. Required. |
| `--github-org <org>` | Scan a GitHub organization or user account. Give this or `--azure-devops-org`. |
| `--all-repos` or `--repos a,b` | GitHub: every repository, or the named ones. |
| `--include-private` | GitHub: also scan private repositories. |
| `--github-api-url <url>` | GitHub: the API address of GitHub Enterprise. The default is `https://api.github.com`. |
| `--azure-devops-org <org>` | Scan an Azure DevOps organization. |
| `--all-projects` or `--projects a,b` | Azure DevOps: every project, or the named ones. |
| `--azure-devops-url <url>` | Azure DevOps: the address of Azure DevOps Server. The default is `https://dev.azure.com`. |
| `--read-ci` | Also read CI pipeline definitions. |
| `--ai-review` | Have an LLM review the agents the rules find. The command asks for the LLM settings (see below). |
| `--judge` | Have a second LLM check the AI review, with the same provider, address and key. Needs `--ai-review`. |
| `--yes` | Scan without asking to confirm the repository list (also `AMP_SCAN_YES=1`). Needed without a terminal. |

`--repos` takes at most 5000 names, given as `name` or `<org>/name`. `AMP_URL`, `--github-api-url` and `--azure-devops-url` accept `http://` or `https://` addresses.

The command asks in the terminal, with hidden input, for the secrets Agent Manager never receives. It asks for a GitHub token only with `--include-private` (a fine-grained token with read-only Contents access), and always for an Azure DevOps personal access token (Code read, and Project and team read). `docker run -it` gives the command the terminal it needs. To skip the question, or to run without a terminal, export `GITHUB_TOKEN` or `AZURE_DEVOPS_TOKEN` in your shell and pass it to the container with `-e NAME`, which copies an exported variable into the container. A GitHub scan of public repositories also uses `GITHUB_TOKEN` when it is passed, for a higher API rate limit, but never asks for it. When the GitHub or Azure DevOps address is plain `http://`, the command warns that the token will be sent to it unencrypted.

With `--ai-review`, the command then asks for the LLM it reviews with. Agent Manager never receives these settings or the key, and nothing is saved.

| Question | Variable | Default |
|---|---|---|
| LLM provider: `openai` (any OpenAI-compatible API) or `anthropic` | `AMP_LLM_PROVIDER` | None |
| LLM address | `AMP_LLM_URL` | At the prompt, `https://api.openai.com/v1` or `https://api.anthropic.com` |
| LLM model | `AMP_LLM_MODEL` | None |
| Judge model, only with `--judge` | `AMP_LLM_JUDGE_MODEL` | The LLM model |
| LLM API key, with hidden input | `AMP_LLM_API_KEY` | None |

A set variable skips its question. Without a terminal, each one needs its variable, except the judge model: `AMP_LLM_URL` has no default there, so a gateway's key is never sent to the public API by mistake. Before the key is used, the command prints the provider, the address's host and the models, never the key. The address is the API base: requests go to `<address>/chat/completions` for `openai`, and to `<address>/v1/messages` for `anthropic` (or `<address>/messages` when the address already ends in `/v1`). It accepts `http://` or `https://`, with no user name, query or fragment, and the command warns before sending the key over plain `http://`. The key must be ASCII. Requests use temperature 0 and send the answer limit as `max_tokens`, which OpenAI-compatible servers (vLLM, Ollama, LiteLLM and others) accept. OpenAI's reasoning models (the gpt-5 family and the o-series) refuse both, so for them the command sends `max_completion_tokens` and no temperature instead. Azure OpenAI works only through its `/openai/v1` address with a key it accepts as a Bearer token; classic deployments (an `api-key` header and an `api-version` query) do not. A busy or rate-limited LLM (HTTP 429, 5xx, or Anthropic's 529) is tried up to 3 times; a timed-out request is not sent again.

The command lists the repositories it will scan, with those not found (counted as scanned, with no agents) and those it cannot scan shown apart, and asks to confirm. Declining publishes nothing and exits 0; the pending scan in Agent Manager expires on its own after 1 hour. Ctrl-C cancels with exit 1. Each repository is downloaded as an archive into a private temporary folder, scanned, and deleted before the next. Files a repository's `.gitattributes` marks `export-ignore` are not in the archive, so they are not scanned. A repository that fails is reported as failed and the scan goes on. The report is written to `/out/report.json` (the folder the command runs in) before the results are published, once.

The whole scan must finish within the scan token's hour. No repository is started with less than 3 minutes left: the scan stops there, reports that repository and the rest as not scanned, and publishes what it finished. Scan the rest with a new command, for example with `--repos`.

`upload` publishes a saved scan report again, such as after a network failure, with the same `AMP_URL` and `AMP_SCAN_TOKEN`. The report must be for the scan ID given. Each scan accepts one publish within the hour of its token, so a token that has expired, or a scan that is no longer waiting for results, needs a new command from Agent Manager. In the folder that holds `report.json`:

```bash
docker run --rm -it --user "$(id -u):$(id -g)" -v "$PWD:/out" -e AMP_URL=… -e AMP_SCAN_TOKEN=… <image> upload --scan-id <id>
```

Only certain agents are published: those the rules prove, and, with AI review, those it confirms. Agents in test code are never published. A publish over Agent Manager's limits (1000 agents, 5000 names in each repository list, 8 MiB) is refused rather than cut down; evidence past the 50th of an agent is left out.

| Exit code | Meaning |
|---|---|
| 0 | Success |
| 1 | Runtime error, including a publish refused as over the limits or because the scan is no longer waiting for results (HTTP 409) |
| 2 | Configuration or usage error, including a scan Agent Manager does not know for the token's organization (HTTP 404) |
| 3 | Authentication error: Agent Manager refused the scan token |
| 4 | Agent Manager requires a newer runner |
| 5 | Partial failure: the results were published, but some repositories failed, the repository list was incomplete, or the scan stopped early (the Git host refused the token or its rate limit was used up, or the scan token's hour ran out) |

## Development

Python 3.12 is required. From this folder:

```bash
uv venv --python 3.12 .venv && . .venv/bin/activate
make install-dev
make lint format-check type-check test
make help        # every target
```

### Dependencies and licences

Runtime dependencies must be MIT or Apache-2.0, and `tree-sitter-language-pack` must never be added.

1. Edit the exact pins in both `requirements.in` and `[project].dependencies` in `pyproject.toml`.
2. Run `make lock` to regenerate `locks/requirements-linux-py312.txt` (hash-pinned, needs PyPI access and `uv`).
3. Run `make install-dev licenses` to regenerate `licenses/THIRD_PARTY_LICENSES.md`.
4. Run `make check-lock check-deps check-wheels`.

A package whose licence is not MIT or Apache-2.0 can be accepted only after review, by adding its exact `name==version` to `licenses/exceptions.txt`. GPL-family licences cannot be excepted.

### Container image

```bash
make docker-build VERSION=0.0.0-dev
make docker-smoke VERSION=0.0.0-dev
```

The image runs as user 10001 by default, and as the caller's user with `--user`. It is built for a read-only root file system: the only writable paths it needs are `/tmp` (also `HOME`) and the `/out` mount.
