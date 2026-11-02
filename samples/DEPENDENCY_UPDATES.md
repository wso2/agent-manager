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

Version updates are grouped by sample and ecosystem, including major, minor,
and patch versions. Related libraries can therefore update together rather than
leaving major upgrades in individual PRs. Hotel Booking's two Python directories
share a group; Insurance Support's agent and MCP server share a Python group
and a separate Docker group. Its web dependencies have their own npm group.
Updates for different samples are kept separate.

Security fixes have separate groups for each sample and ecosystem with no
version-level restriction. Docker receives version updates only; Dependabot
does not support Docker security-update PRs or scan container contents.

Grouping does not guarantee a compatible dependency set or perform code and
configuration migrations. Major upgrades still need review and passing smoke
tests. There is no repository-wide limit of one or two PRs: different samples,
ecosystems, and security updates can produce separate PRs.

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

## Google Chat summary

`.github/workflows/dependabot-pr-summary.yaml` sends one weekly digest on Monday
at 09:00 Sri Lanka time (`Asia/Colombo`, UTC+05:30), the day after Sunday's
Dependabot version-update schedule. Its UTC cron is `30 3 * * 1`.
It uses the existing `GCHAT_PR_WEBHOOK_URL`; no separate bot webhook is needed.
Individual Dependabot PR notifications are suppressed in `wso2/agent-manager`.
Forks keep individual notifications because the digest runs only upstream.
Other authors keep their immediate notifications.

The digest lists all open, non-draft Dependabot PRs with linked numbers, titles,
and aggregate check results from both the head and available PR merge commit.
A mixture of successful and skipped/neutral checks is shown as `passed`;
`skipped / neutral` means all reported checks were skipped or neutral with no
commit statuses. Failed, cancelled, and pending checks affect the aggregate.
It does not wait for CI to finish. If there are no eligible PRs, it sends a short "no open PRs" summary. Open PRs appear again in
subsequent weekly digests until closed or merged. New PRs and results after the
snapshot appear in the next digest; security updates can arrive outside Sunday's
schedule and their GitHub alerts remain available immediately.

There is no polling, batch timer, or stored notification state. **Run workflow**
on the default branch sends an additional on-demand digest; manually rerunning a
successful run also sends another message. Failed GitHub reads or Chat delivery
fail the workflow. The report describes observed checks, not branch-protection
approval or mergeability.

The workflow runs only in `wso2/agent-manager` on the default branch, using its
existing GitHub token with read permissions and the existing Chat webhook secret.
It never checks out PR code. GitHub can delay scheduled runs, so 09:00 is the
scheduled time rather than a guaranteed delivery time.

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
- Add new sample manifest directories to the corresponding sample/ecosystem entry in
  `.github/dependabot.yml`.

References: [configuration options](https://docs.github.com/en/code-security/reference/supply-chain-security/dependabot-options-reference),
[security updates](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/secure-your-dependencies/configure-security-updates),
and [supported ecosystems](https://docs.github.com/en/code-security/reference/supply-chain-security/supported-ecosystems-and-repositories).
