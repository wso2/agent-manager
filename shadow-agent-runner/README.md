# Shadow Agent Runner

`amp-shadow-runner` is the source code scan of Shadow Agent Discovery in WSO2 Agent Manager. It finds AI agents in GitHub and Azure DevOps repositories and publishes the confirmed agents to Agent Manager.

It runs on the customer's machine, as the `docker run` command that the Agent Manager console builds for a source code platform. When complete, the command will ask for the Git token only for private repositories, and for the LLM key only with AI review. It will then fetch the repositories, scan them, delete the code and publish only the confirmed agents. Agent Manager never stores the Git token and never pulls the code.

The package has two parts:

- `amp_shadow_runner`: the command line.
- `amp_shadow_runner.engine`: the scan engine, a pure library that parses a local source tree with tree-sitter. It never opens a network connection or starts a process (`tests/engine/test_purity.py` enforces this).

> Scanning is not implemented yet. `scan` checks its options, `AMP_URL` and `AMP_SCAN_TOKEN`, then exits with status 1 and "not implemented yet". `upload` is a stub, and only `version` works in full.

## Command line

```bash
amp-shadow-runner --version
amp-shadow-runner version --json
amp-shadow-runner scan --scan-id <id> --github-org <org> --all-repos
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
| `--ai-review` | Have an LLM review the agents the rules find. |
| `--judge` | Have a second LLM check the AI review. Needs `--ai-review`. |

`--repos` takes at most 5000 names, given as `name` or `<org>/name`. `AMP_URL`, `--github-api-url` and `--azure-devops-url` accept `http://` or `https://` addresses.

| Exit code | Meaning |
|---|---|
| 0 | Success |
| 1 | Runtime error |
| 2 | Configuration or usage error |
| 3 | Authentication error |
| 4 | Agent Manager requires a newer runner |
| 5 | Partial failure |

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
