# Sample dependency updates

[Dependabot configuration](../.github/dependabot.yml) checks sample dependencies
weekly on Monday at 09:00 Asia/Kolkata (UTC+05:30). GitHub schedules the jobs;
execution can be delayed.

| Ecosystem | Coverage |
| --- | --- |
| pip | All nine sample `requirements.txt` directories |
| npm | Insurance Support web manifest and lock file |
| NuGet | .NET agent project package references |
| Docker | .NET agent and Insurance Support agent/MCP server Dockerfiles, including `Dockerfile.ecs` |

Minor and patch updates are grouped by ecosystem. Major updates remain eligible
and are raised separately for review. Security fixes have separate groups with
no version-level restriction. Docker receives version updates only; Dependabot
does not support Docker security-update PRs or scan container contents.

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

## Coverage limits and upkeep

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
