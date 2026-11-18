# AMP Security Tests

Security tests for Agent Manager authentication, authorization, role mappings,
observability access, AgentID credentials, and deployed-agent isolation.

## Prerequisites

Start the local deployment and prepare the E2E configuration:

```bash
make setup
cp test/e2e/.env.example test/e2e/.env
```

The defaults in `.env.example` match the local quick-start deployment. Override
them when testing another environment.

## Commands

Run all static and live security tests:

```bash
make security-test
```

Run only the static authorization checks:

```bash
make security-test-static
```

Run all live suites against an existing deployment:

```bash
make security-test-live
```

Run one live suite or one matching spec:

```bash
make security-test-live SUITE=authz
make security-test-live FOCUS="scope matrix"
```

The live run writes its JUnit report to `test/e2e/security-report.xml`.

## Suites

| Suite | Coverage |
|---|---|
| `authz` | Required API scopes and positive authorization controls |
| `tokens` | Missing, malformed, forged, tampered, and oversized tokens |
| `observability` | Scope enforcement on trace, log, metric, and build-log routes |
| `roles` | Predefined roles and custom-role permission changes |
| `agentid` | AgentID credential creation, rotation, revocation, and non-disclosure |
| `runtime` | Container hardening, Kubernetes API isolation, and AgentID-scoped MCP access |

## Environment overrides

Role tests require Thunder administrative credentials. Use
secret-backed values outside the local quick-start environment:

```bash
THUNDER_ADMIN_URL=https://thunder.example.com \
THUNDER_SYSTEM_CLIENT_ID=... \
THUNDER_SYSTEM_CLIENT_SECRET=... \
make security-test-live SUITE=roles
```

Set `AGENT_IDP_TOKEN_URL` when the AgentID suite cannot discover the environment
Thunder token endpoint automatically:

```bash
AGENT_IDP_TOKEN_URL=https://thunder.example.com/oauth2/token \
make security-test-live SUITE=agentid
```

The runtime suite builds its probe from a remotely cloneable repository. When
testing an unmerged branch, push it and provide the repository and branch:

```bash
SECURITY_PROBE_REPOSITORY_URL=https://github.com/<owner>/agent-manager \
SECURITY_PROBE_REPOSITORY_BRANCH=<branch> \
make security-test-live SUITE=runtime
```

## Adding tests

Keep API calls in `test/e2e/operations/` and assertions in the relevant suite.
Build resource names with framework constants such as `E2EProjectPrefix` and
`E2EAgentPrefix` so stale-resource cleanup can identify them. Delete created
resources with `DeferCleanup`, or in the final spec of an ordered lifecycle.
Never print access tokens, client secrets, or API keys.
