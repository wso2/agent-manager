# Audit logging

Agent Manager records who did what, to which resource, with what outcome. This document covers what is recorded, what is deliberately not, and — most importantly — **what the deployment must provide for the trail to be usable as audit evidence**.

## What gets recorded

Every state-changing API call, every authorization denial, and every rejected token *attempts* one record. Records are written to **stdout as structured JSON** tagged `log_type=audit`, for the cluster log pipeline to collect.

"Attempts" rather than "produces", because two paths deliberately emit less than one record per event: authentication and internal-surface denials are rate-limited per source (see [Denials](#denials)), and ordinary buffered records are dropped rather than allowed to block a request when the buffer fills. Both are counted and both surface in the trail — a suppressed count rides on the next emitted record, and drops are reported as `system:audit-dropped`. Nothing is silently discarded, but the count is a floor, not an exact tally. The one path with no such allowance is `audit.Begin`, which refuses the operation rather than losing the record (see [Reliability](#reliability)).

Coverage comes from two tiers:

| Tier | Where | What it gives |
|---|---|---|
| Coverage | A middleware installed once in `middleware.RouteRegistrar`, and `addTool` on the MCP surface | Every registered route and tool: actor, org, action, outcome, source. Cannot be forgotten for a new endpoint. |
| Semantic | Explicit `audit.Record` / `audit.Begin` calls | The domain effect: which entity, which permissions were granted, which environment. |

Four surfaces are covered, and the `surface` field on every record says which one produced it:

| Surface | What it covers | Actor |
|---|---|---|
| `api` | The authenticated REST API | The token subject |
| `mcp` | MCP tool invocations | The token subject; `details.tool` names the tool |
| `internal` | The gateway-facing internal server (no JWT, `api-key` header) | The gateway |
| `system` | Reconcilers and schedulers, plus startup posture | `system:<component>`, with `onBehalfOf` when a user requested the work earlier |
| `publisher` | The evaluation job publishing monitor scores | The `amp-publisher-*` audience |

When a semantic event describes a successful request, the coverage tier stands down so the trail carries one record rather than two. On failure the coverage record is always kept — a request rejected before it reached the service emits nothing semantic, and that rejection is exactly what must not go unrecorded.

### Which routes and tools

- **Every non-GET route.** A test (`api/audit_coverage_test.go`) fails the build if a mutating route is not audited, so this cannot drift.
- **Every MCP tool that changes state.** `addTool` requires an `audit.Action` alongside its permissions and panics without one, so a new mutating tool cannot ship unattributed — the same registration-time guarantee the route registrar gives REST. Read-only tools declare a read action and are not recorded.
- **Reads only when they disclose credentials or security configuration** — API-key listings, git secrets, gateway tokens, role assignments, identity-provider configuration. Auditing every GET would multiply volume several-fold for little forensic gain.
- One documented exemption: `POST /orgs/{orgName}/utils/generate-name`, which suggests a name and persists nothing.

### Record shape

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-08-05T08:13:37Z",
    "action": "git-secret:create",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "alice@example.com",
    "actorOuId": "ou-9f2",
    "actorTokenId": "jti-7c1e",
    "authMethod": "jwt-bearer",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-…",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/git-secrets",
    "ouId": "ou-9f2",
    "resourceType": "git-secret",
    "resourceId": "gs-7",
    "resourceName": "github-deploy-key",
    "outcome": "success",
    "statusCode": 201,
    "requiredPermission": "amp:git-secret:create",
    "rbacEnforced": true
  }
}
```

`schemaVersion` is the parser contract. Renaming or removing a field requires bumping it.

### Action taxonomy

Actions read as `<resource>:<verb>`. Most are the route's `rbac.Permission` verbatim; where a permission does not describe the effect, `audit/policy.go` maps the route explicitly. The three recurring cases:

- One permission gating several operations — every API-key route is gated by `*:api-key-manage`, so create, rotate and revoke would otherwise be indistinguishable.
- A permission naming a different resource than the operation acts on — `publish-kind` is gated by `agent-kind:create` but acts on an agent.
- A permission narrower than the effect — `deployments/state` is gated by `agent:suspend` but also resumes.

`requestPath` always holds the **route pattern**, never the raw URL. Queries on it are exact, which is what makes deriving the human-readable action safe.

`actionClass` is one of `authn`, `authz`, `credential`, `identity`, `deployment`, `config`, `read`, `system`. `severity` runs 1 (info) to 4 (critical); credential and privilege changes are always 4.

### Severity 4 — the events worth alerting on

`authz:root-ou-bypass`, `system:rbac-disabled`, `system:audit-dropped`, and everything in `actionClass: credential` or `identity` with a mutating verb — notably `role:grant-permission`, `role:assign`, `user:create`, `user:delete`, `api-key:*`, `git-secret:*`, `agent-identity:*`, `gateway-token:*`, `service-account:configure`.

## What is never recorded

Two properties are **structural** rather than filtered, which is what makes the coverage tier safe to run on every route:

1. **Request and response bodies are never read.** The secrets that flow through this API — git credentials, client secrets, upstream auth values, user-creation password attributes, and the API keys and tokens returned once on creation — are out of reach of a record, not merely redacted out of one.
2. **`requestPath` is the route pattern.** Path and query-string leakage is eliminated by construction.

On top of that, anything a caller attaches by hand passes an **allow-list keyed by action** (`audit/schema.go`). A key nobody declared is dropped and reported under `_droppedKeys`. This is deliberately not a deny-list: a deny-list fails on the field nobody thought of.

Where a secret must be referenced, `audit.SecretRef` stores a SHA-256 prefix plus the last four characters — enough to correlate "the key that was rotated" with "the key later used", never enough to use.

**URLs are reduced to scheme, host and path.** A URL is the one declared value that can hold a credential in its own syntax — RFC 3986 userinfo (`https://user:pass@idp.example/jwks`) or a token in the query. A detail declared `KindURL` has its userinfo, query and fragment removed before the record is written, and a `[redacted-components]` marker says something was there. This is enforced at redaction, not at the emit site, so it cannot be forgotten by the next caller; a test fails if a URL-valued detail is declared as anything else.

Resource **names** are recorded deliberately, including secret names. A name is not a credential, and "which git secret was deleted" is where an investigation starts.

## Deployment requirements

### Retention is not provided by this service

Records go to stdout. **Retention, immutability and access control are properties of your log pipeline, not of Agent Manager.**

The observability plane's default log retention is far shorter than the evidence period most audits require (SOC 2 evidence periods typically run 3–12 months; ISO 27001 A.5.33 requires retention per a defined policy). **Route `log_type=audit` records to a dedicated index or bucket with a retention period that matches your obligations**, ideally write-once storage.

This is the single most important thing to get right. Everything else here is already done for you.

### Tamper evidence

Agent Manager has no write path back into the log store, so the collected copy is one the service itself cannot rewrite. That is a genuine property and it is the basis for treating the pipeline copy as authoritative. It is **not** protection against someone with write access to the log store itself — for that, use write-once storage or forward to a SIEM that platform administrators do not also administer.

### Separation of duties

Anyone who can grant permissions can grant themselves any permission, and that grant is itself audited. No arrangement of in-product RBAC fixes this, because the administrators administer the RBAC. Genuine separation of duties requires shipping these records to a system that Agent Manager administrators do not control. This design enables that; it cannot substitute for it.

## The authentication gap

**Login, logout, failed login, MFA, password change and session revocation are not recorded here, because they do not happen here.** Authentication is performed by WSO2 Thunder; Agent Manager only ever sees an already-minted JWT. There is no login endpoint in this service.

These are precisely the events SOC 2 CC6.1/CC6.6 and ISO 27001 A.8.15 expect. **A complete trail requires collecting Thunder's own audit events as well.** Any claim of completeness based on Agent Manager alone is wrong.

What this service does record, and which makes the two joinable:

- `actorTokenId` — the token's `jti`, on every record.
- `actorId` — the token subject.
- `authn:failure` — a token rejected at this service's edge, with a classified reason (`expired`, `bad-signature`, `bad-issuer`, `bad-audience`, `unknown-kid`, `malformed`, `missing-header`) and the source IP. Note this is "a bad token reached Agent Manager", which is **not** the same as "a login failed".

**To join:** correlate on `actorId` (subject) plus `actorTokenId` (`jti`) within the token's validity window. Thunder's record of issuing that `jti` to that subject links a login session to every action taken with it.

Authentication-failure records are rate-limited to 10 per source IP per minute, with the suppressed count carried on the next emitted record, so a token flood cannot become a storage-volume problem while still leaving the signal visible.

## Denials

Refusals are recorded on every surface, because a denied attempt is often more interesting than a successful one:

- **REST** — all five deny sites in `middleware/authorization.go`, with the specific missing scope and the caller's scope count.
- **MCP** — insufficient scope, an unknown tool (a probe), and an organization mismatch (a token driving a tool against a different org than its session — the clearest attack signal this surface produces).
- **Internal** — a missing or invalid gateway `api-key`, and a valid key presented for a *different* gateway, which is recorded separately because a valid credential used out of scope is a stronger signal than an invalid one. These are rate-limited per source, since gateways poll continuously and one with a stale key would otherwise emit forever.
- **Edge** — rejected tokens, with a classified reason and no token material.

## Enforcement posture is part of the trail

`RBAC_ENABLED` defaults to `false` in code, and when it is off **every permission check returns early**. The Helm chart sets it to `true`, so a chart-based install enforces authorization — but a bare binary run does not.

Rather than leave that gap visible only in a config file, the trail documents it:

- `system:rbac-disabled` is recorded at startup when authorization is off.
- Every record carries `rbacEnforced`, alongside the `requiredPermission` that *would* have applied.

A record with `rbacEnforced: false` shows on its face that no check happened. Without this, every record would imply an authorization decision that never occurred.

`system:startup` is recorded when the service starts, which bounds any gap in the trail to a restart — a reader can tell "nothing happened" apart from "the service was not running".

## Reliability

| Path | Behaviour on failure |
|---|---|
| Ordinary records (`audit.Record`) | Asynchronous, buffered. On overflow, dropped and counted — never blocks a request. A slow sink must not become an outage. |
| Security-critical operations (`audit.Begin`) | Synchronous. If the record cannot be written the operation is **refused**, because an untraceable privileged change is worse than a failed one. |

Drops are counted, logged, and reported as `system:audit-dropped`. A trail that silently loses records is worse than one that admits it.

**The honest limit of "fail-closed" here:** a successful write means the record reached the process's output, not that it reached durable storage. This catches the common failure — the process is running but its sink is broken — but it is not the atomic "change and record commit together" guarantee a same-database write would give. `audit.Begin` therefore writes an *intent* record before an external mutation and an *outcome* record after. A record left at `outcome: "unknown"` means the process died mid-operation; that orphan is deliberate forensic signal, not a defect.

**Where the intent record sits, and what that costs.** `audit.Begin` is placed immediately before the *commit point* — the irreversible external call — not at the top of the operation. In the multi-step lifecycle operations (`DeleteAgent`, `DeployAgent`, `PromoteAgent`) that means some local preparation has already run when the intent is written: secret references cleaned up, the Component CR updated, a target-environment config row upserted. If `Begin` fails there, the operation is refused *after* that preparation, and only the coverage-tier envelope records the attempt.

This is a deliberate trade, not an oversight. Beginning at the top of those operations would mean every intermediate return path — and there are many, since each validates against OpenChoreo as it goes — has to resolve the attempt or leave a false `outcome: "unknown"` orphan. Orphans are the signal that the process died mid-operation; manufacturing them on ordinary validation failures would devalue the one thing they mean. The narrow placement keeps the orphan window at "we called OpenChoreo and did not learn the result", which is exactly the condition worth investigating.

The gap is real and bounded: the preparation that can precede an intent record is confined to this service's own state, and every one of those steps sits behind a route the coverage tier already recorded. What cannot happen unrecorded is the external mutation itself.

The buffer is flushed on shutdown **after both HTTP servers stop**, so in-flight requests finish recording first.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `AUDIT_ENABLED` | `true` | Turn recording on. Off means no record of privileged operations. |
| `AUDIT_BUFFER_SIZE` | `4096` | Queued events before dropping. |
| `AUDIT_BATCH_SIZE` | `200` | Events written per sink call. |
| `AUDIT_FLUSH_INTERVAL_MS` | `1000` | Maximum time an event waits before being written. |

Helm: `agentManagerService.config.audit.*`.

## Adding a semantic event

Services and controllers emit through package-level functions that take only a context, so no constructor changes and no new dependencies are needed. Outside a request the calls are no-ops, which keeps unit tests free of audit wiring.

```go
// Ordinary: asynchronous, fail-open.
audit.Record(ctx, audit.Action("agent:deploy"),
    audit.ResourceNamed("agent", agent.ID, agentName),
    audit.Project(projName),
    audit.Environment(envName),
    audit.Result(err),
)

// Security-critical: refuse the operation if the record cannot be written.
attempt, err := audit.Begin(ctx, audit.Action("git-secret:create"),
    audit.ResourceNamed("git-secret", req.Name, req.Name))
if err != nil {
    return nil, err // do NOT perform the operation
}
result, err := s.client.CreateGitSecret(ctx, ouID, ocReq)
attempt.Complete(ctx, err)
```

Domain actions are declared in `audit/actions_domain.go`, which registers each one's class, severity and permitted detail keys together so the three cannot drift apart. A test fails if a registered action has no detail schema, so the decision cannot be skipped.

**Use the same action constant the coverage tier derives for the route.** `TestDomainActionsMatchRouteDerivedActions` fails the build if a semantic emit and its route disagree — otherwise "who deployed agent X" returns half the answer depending on which tier recorded it.

`audit.Detail` accepts only scalars and string slices. Refusing structs and maps is what keeps request payloads structurally unable to reach a record.

### Testing a service that emits

Operations that must not happen unrecorded refuse to proceed when no recorder is installed, so a bare `context.Background()` makes them fail by design. Tests exercising those paths use `auditableCtx(t)` (`services/audit_testing_test.go`), which installs a discarding recorder. To assert the refusal itself, pass a bare context and expect `audit.ErrRecorderUnavailable`.

## Semantic events

The operations below emit a record describing what actually changed, not just that the route was called. Everything else is covered by the coverage tier alone.

| Area | Actions | Mode |
|---|---|---|
| API keys | `api-key:create` / `:rotate` / `:revoke` (agent, LLM provider, LLM proxy) | Fail-closed |
| API keys | `api-key:issue-test` (console Try-It) | Fail-open |
| Git secrets | `git-secret:create` / `:delete` | Fail-closed |
| Gateway tokens | `gateway-token:rotate` / `:revoke` | Fail-closed |
| Agent tokens | `agent-token:mint`, `agent-token:regenerate-tracing` | Fail-closed |
| Agent OAuth identity | `agent-identity:provision` / `:regenerate-secret` / `:revoke-secret` | Fail-closed |
| Env identity credential | `service-account:configure` / `:remove` | Fail-closed |
| Privilege | `role:grant-permission` / `:revoke-permission` / `role:assign` / `:unassign` | Fail-closed |
| Membership | `group:add-member` / `:remove-member` | Fail-closed |
| Users | `user:invite` / `:create` / `:delete` | Fail-closed |
| Agent lifecycle | `agent:deploy`, `agent:promote`, `agent:change-deployment-state`, `agent:delete`, `project:delete` | Fail-closed |
| Agent lifecycle | `agent:build` | Fail-open |
| Gateways | `gateway:delete`, `gateway:set-identity-provider`, `gateway:remove-identity-provider` | Fail-closed |
| Gateways | `gateway:create` / `:update` / `:assign-environment` / `:unassign-environment` | Fail-open |
| Internal | `gateway:push-manifest`, `api-key:sync` (bulk sync, coalesced per gateway) | Fail-open |
| Agent config | `agent-config:update` / `:delete` | Fail-open |
| Per-config API keys | `api-key:create` / `:rotate` / `:revoke` (model-config, mcp-config) | Fail-closed |
| Monitors | `monitor:create` / `:update` / `:delete` / `:start` / `:stop` / `:rerun` | Fail-open |
| Monitors | `monitor:run-failed` (scheduler, system actor) | Fail-open |

Two of these carry detail worth calling out:

- **`agent:deploy` and `agent:promote` both record the target environment and `isProduction`.** The route declares only the tier floor whatever the agent actually lands on, so the declared permission cannot tell a sandbox push from a production one — only the record can. Both flags come free: the environment-tier check (`requireEnvTier`) has already resolved the environment by the time the record is opened. `agent:promote` records both ends of the move as well.
- **`role:grant-permission` records the granted scopes in full.** Scope strings are identifiers, not secrets, and "alice granted SRE deploy-production" is the question the event exists to answer.

## Not yet covered

These surfaces install the recorder but do not yet emit semantic events; their absence is a real gap in coverage today:

- **Thunder's own events** — see the authentication gap above. This is the one gap that cannot be closed from this repository, and it is the only remaining hole in coverage.
