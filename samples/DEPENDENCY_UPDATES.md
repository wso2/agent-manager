# Sample dependency updates

[Dependabot configuration](../.github/dependabot.yml) checks sample dependencies
weekly on Sunday at 09:00 Asia/Colombo (UTC+05:30). GitHub schedules the jobs;
execution can be delayed.

| Ecosystem | Coverage |
| --- | --- |
| pip | All nine sample `requirements.txt` directories |
| npm | Insurance Support web manifest and lock file |
| NuGet | .NET agent project package references |
| Docker | .NET agent and Insurance Support agent/MCP server Dockerfiles, including `Dockerfile.ecs` |

All sample version updates share the `sample-dependencies` multi-ecosystem
group. Python, npm, NuGet, and Docker updates across all covered directories
are proposed together in one PR, including major, minor, and patch versions.
There are no separate version-update groups or unmatched dependency patterns.

Security updates are handled separately by GitHub, with wildcard security groups
for Python, npm, and NuGet. They can create additional PRs independently of the
weekly version-update group. This configuration does not impose a global
one-PR limit on automatic security updates. Docker receives version updates only;
Dependabot does not support Docker security-update PRs or scan container contents.

Grouping does not guarantee a compatible dependency set or perform code and
configuration migrations. A failing sample check can block the combined PR;
review release notes and run all applicable smoke tests before merging.

## Enable after merging

Dependabot reads the configuration from the repository's default branch (`main`).
A repository administrator must confirm these features are enabled under
**Settings → Advanced Security** (organization policy may manage them):

- Dependency graph
- Dependabot alerts
- Dependabot security updates

The YAML configures security-update grouping; it does not enable these settings.
Once enabled, GitHub opens security-update PRs when it can resolve a fixed version.
These do not wait for the weekly version-update schedule.

Review PRs from `dependabot[bot]`, check release notes and compatibility, and merge
after applicable CI checks and required reviews pass. This configuration does
not enable automatic merging or add sample test coverage. Existing workflows
run according to their path filters. Dependabot-triggered workflows do not
receive ordinary Actions secrets; checks needing credentials may require
separate Dependabot secrets or a workflow adjustment.

View update jobs under **Insights → Dependency graph → Dependabot**, and
outstanding vulnerabilities under **Security → Dependabot alerts**. The initial
check may raise PRs for dependencies that are already outdated.

## Google Chat notifications

The existing `.github/workflows/pr-open-notify.yaml` announces ready PRs,
including Dependabot PRs, immediately to the existing `GCHAT_PR_WEBHOOK_URL`.
The combined sample dependency PR uses the same message with its linked number,
title, author, and base branch. Opening or reopening a ready PR sends a notice;
a draft is announced when marked ready for review.

There is no weekly digest workflow, scheduled Chat report, or separate bot
webhook. Notification delivery requires the existing repository secret. Because
`pull_request_target` uses the base branch workflow, the restored notification
behavior takes effect after this change is merged.

Existing per-sample Dependabot PRs are not closed or merged by this change.
After merging, check the Dependabot update job and retire superseded PRs once
the new combined PR is available.

## Coverage limits and upkeep

- Declare Python dependencies with an explicit version constraint. Dependabot
  leaves unversioned requirements unchanged for routine version updates. The
  A2A, CrewAI, Customer Support, and Manual Instrumentation samples pin their
  direct requirements so updates are proposed for review rather than silently
  adopted by the next install.
- `leave-assistant-agent` uses Ballerina, which Dependabot does not support.
  Maintain its distribution and dependencies manually; regenerate
  `Dependencies.toml` with Ballerina rather than editing it by hand.
- Python dependencies installed only in code, documentation, or Dockerfile `RUN`
  instructions are outside Python manifest coverage. The Insurance Support
  agent's inline `amp-instrumentation` and `requests` installs still need manual
  maintenance. Docker updates maintain `FROM` image references.
- Requirements files without resolved lock files do not provide complete
  monitoring of every installed transitive dependency. Continue Mend and
  installed-dependency/container scans. Some fixes require upgrading a parent
  package or changing a manifest.
- Add new sample manifest directories to the corresponding ecosystem entry in
  `.github/dependabot.yml`.

References: [configuration options](https://docs.github.com/en/code-security/reference/supply-chain-security/dependabot-options-reference),
[security updates](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/secure-your-dependencies/configure-security-updates),
and [supported ecosystems](https://docs.github.com/en/code-security/reference/supply-chain-security/supported-ecosystems-and-repositories).
