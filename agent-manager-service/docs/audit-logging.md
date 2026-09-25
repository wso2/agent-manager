# Audit logging

Agent Manager records who did what, to which resource, with what outcome. This document covers what is recorded, what is deliberately not, and — most importantly — **what the deployment must provide for the trail to be usable as audit evidence**.

## What gets recorded

Every state-changing API call, every authorization denial, and every rejected token *attempts* one record. Records are written to **stdout as structured JSON** tagged `log_type=audit`, for the cluster log pipeline to collect.

"Attempts" rather than "produces", because two paths deliberately emit less than one record per event: authentication and internal-surface denials are rate-limited per source (see [Denials](#denials)), and ordinary buffered records are dropped rather than allowed to block a request when the buffer fills. The two end up in different places. Suppressed JWT authentication failures are reported in the trail: their count rides on the next emitted `authn:failure` record as `suppressedCount` (repeats on the internal surface are suppressed without a count). Buffer drops are counted and logged only to the application log (`audit buffer full; event dropped`, with a running `droppedTotal`) — they do **not** reach the audit trail, because `system:audit-dropped` is registered but nothing emits it (see [Not yet covered](#not-yet-covered)). Either way, the record count is a floor, not an exact tally. The one path with no such allowance is `audit.Begin`, which refuses the operation rather than losing the record (see [Reliability](#reliability)).

Coverage comes from two tiers:

| Tier | Where | What it gives |
|---|---|---|
| Coverage | A middleware installed once in `middleware.RouteRegistrar`, and `addTool` on the MCP surface | Every registered route and tool: actor, org, action, outcome, source. Cannot be forgotten for a new endpoint. |
| Semantic | Explicit `audit.Record` / `audit.Begin` calls | The domain effect: which entity, which permissions were granted, which environment. |

Five surfaces are covered, and the `surface` field on every record says which one produced it:

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
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
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
    "orgHandle": "acme",
    "resourceType": "git-secret",
    "resourceId": "gs-7",
    "resourceName": "github-deploy-key",
    "outcome": "success",
    "statusCode": 201,
    "requiredPermission": "amp:git-secret:create"
  }
}
```

`schemaVersion` is the parser contract. The JSON keys are the wire contract (`audit/event.go`): renaming or removing one is a breaking change that requires bumping it.

### Field reference

Every record carries these fields where they apply; empty fields are omitted.

| Field | Holds |
|---|---|
| `eventId` · `occurredAt` · `schemaVersion` | Record identity and time. On a `Begin` outcome record, `details.attemptEventId` links back to its intent record. |
| `action` | What happened, as `resource:verb` — see [Action reference](#action-reference). |
| `actionClass` | Coarse grouping for filtering and alerting. |
| `severity` | `1` info · `2` notice · `3` warning · `4` critical. |
| `outcome` | `success`, `failure`, `deny`, or `unknown` for an intent record whose outcome never arrived. |
| `actorType` | `user` · `service` · `agent` · `gateway` · `system` · `anonymous`. |
| `actorId` | Who did it. For a user, the token subject — a Thunder user id, not an email or username (resolve it with `GET /api/v1/orgs/{orgName}/identities/users/{userId}`). For other actors: a gateway id, a publisher audience, or `system:<component>`. |
| `actorOuId` | The actor's organization unit, from the token. |
| `actorTokenId` | The token's `jti` — the join key to Thunder's login records (see [The authentication gap](#the-authentication-gap)). |
| `actorDisplay` | Reserved for a human-readable name. No emit site populates it today, so records carry the id alone. |
| `onBehalfOf` | Delegation — a reconciler applying what a user asked for earlier. Also a user id. |
| `authMethod` | How the caller authenticated, e.g. `jwt-bearer`. |
| `surface` | Entry point: `api` · `mcp` · `internal` · `system` · `publisher`. |
| `sourceIp` | The leftmost `X-Forwarded-For` hop, falling back to `X-Real-IP` and then the peer address (`utils.ClientIP`). |
| `userAgent` | Distinguishes the console from `amctl` from a script. |
| `correlationId` | Ties the record to the request logs; matches the `x-correlation-id` response header. |
| `requestMethod` · `requestPath` | HTTP verb and the **route pattern**, never the raw URL. The one exception is `authn:failure` on the REST edge: the token is rejected before route matching, so the record carries the URL path — without its query string. |
| `ouId` · `orgHandle` | Tenant, taken from the token — never from the URL path. |
| `resourceType` · `resourceId` · `resourceName` | What was acted on. Names are recorded deliberately, including secret names. |
| `projectName` · `environment` | Scope. For deployments, `environment` is what separates a sandbox push from a production one. |
| `statusCode` · `durationMs` | HTTP status and request duration, on coverage-tier records. |
| `errorCode` · `errorMessage` | Failure detail. `errorMessage` passes through the secret-shaped masker, since upstream errors can echo whole response bodies. |
| `requiredPermission` | The scope that gated the call, recorded on allow and deny alike. |
| `details` | Per-action fields. Only keys the action declares survive; the rest are dropped and listed under `_droppedKeys`. |

Every action may also carry the shared detail keys from `audit/schema.go`: `envelope` (a coverage-tier record), `attemptEventId` (a `Begin` outcome record), `tool` (MCP surface), `suppressedCount` (rate-limited `authn:failure`) and `_droppedKeys`. `pathParams` and `repeatCount` are declared but not set by any emit site today.

### Action taxonomy

Actions read as `<resource>:<verb>`. Most are the route's `rbac.Permission` verbatim; where a permission does not describe the effect, `audit/policy.go` maps the route explicitly. The three recurring cases:

- One permission gating several operations — every API-key route is gated by `*:api-key-manage`, so create, rotate and revoke would otherwise be indistinguishable.
- A permission naming a different resource than the operation acts on — `publish-kind` is gated by `agent-kind:create` but acts on an agent.
- A permission narrower than the effect — `deployments/state` is gated by `agent:suspend` but also resumes.

`requestPath` holds the **route pattern** on every record a route produced (see the one `authn:failure` exception above). Queries on it are exact, which is what makes deriving the human-readable action safe.

`actionClass` is one of `authn`, `authz`, `credential`, `identity`, `deployment`, `config`, `read`, `system`. `severity` runs 1 (info) to 4 (critical); credential and privilege changes are always 4.

### Severity 4 — the events worth alerting on

`authz:root-ou-bypass`, and everything in `actionClass: credential` or `identity` with a mutating verb — notably `role:*`, `group:*`, `user:*`, `api-key:create` / `:rotate` / `:revoke`, `git-secret:*`, `agent-identity:*`, `agent-token:*`, `gateway-token:*`, `service-account:*`, and `gateway:set-identity-provider` / `:remove-identity-provider` (changing which issuer a gateway trusts changes who can mint tokens it accepts). `api-key:issue-test` is the deliberate exception: console Try-It keys are short-lived, so they rank as a notice.

## What is never recorded

Two properties are **structural** rather than filtered, which is what makes the coverage tier safe to run on every route:

1. **Request and response bodies are never read.** The secrets that flow through this API — git credentials, client secrets, upstream auth values, user-creation password attributes, and the API keys and tokens returned once on creation — are out of reach of a record, not merely redacted out of one.
2. **`requestPath` is the route pattern for matched requests.** Once a route has matched, path and query-string leakage is eliminated by construction. Edge `authn:failure` records precede route matching and carry the request's `URL.Path` after generic log sanitization, so concrete path segments (org, project, agent names) may remain — but the query string is never read.

On top of that, anything a caller attaches by hand passes an **allow-list keyed by action** (`audit/schema.go`). A key nobody declared is dropped and reported under `_droppedKeys`. This is deliberately not a deny-list: a deny-list fails on the field nobody thought of.

Where a secret must be referenced, `audit.SecretRef` stores a SHA-256 prefix plus the last four characters — enough to correlate "the key that was rotated" with "the key later used", never enough to use.

**URLs are reduced to scheme, host and path.** A URL is the one declared value that can hold a credential in its own syntax — RFC 3986 userinfo (`https://user:pass@idp.example/jwks`) or a token in the query. A detail declared `KindURL` has its userinfo, query and fragment removed before the record is written, and a `[redacted-components]` marker says something was there. This is enforced at redaction, not at the emit site, so it cannot be forgotten by the next caller; a test fails if a URL-valued detail is declared as anything else.

**The free-form attribute summary never records attribute values.** The user-creation API takes a free-form attribute map that is known to carry passwords. For `user:create`, `username` is copied explicitly into `details.username` and used as the resource id and name; the rest of the attribute map is represented only by its key names (`attributeKeys`), a count (`attributeCount`), and a flag when a key looks credential-shaped (`containsSensitiveKey`) — see `audit.AttributeKeySummary`.

**Authentication failures carry no token material** — only a classified reason (see [The authentication gap](#the-authentication-gap)).

Resource **names** are recorded deliberately, including secret names. A name is not a credential, and "which git secret was deleted" is where an investigation starts.

## Looking things up

Records are newline-delimited JSON on stdout. These work with `jq` against a collected log file and translate directly to a SIEM query.

Who triggered this build?

```bash
jq 'select(.event.action=="agent:build" and .event.details.buildName=="build-42")
   | {who: .event.actorId, when: .event.occurredAt, agent: .event.resourceName}'
```

Everything one person did (`actorId` is a Thunder user id — look it up first if you only have a name):

```bash
jq 'select(.event.actorId=="8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147")
   | "\(.event.occurredAt) \(.event.action) \(.event.resourceName // "")"'
```

Turning an actor id into a person — records carry the id only; the name lives in Thunder:

```
GET /api/v1/orgs/{orgName}/identities/users/{actorId}   # → attributes.username, attributes.email
```

Every privilege change, and exactly what was granted:

```bash
jq 'select(.event.action|startswith("role:"))
   | {who: .event.actorId, role: .event.resourceName, scopes: .event.details.permissions}'
```

Anything critical that was refused:

```bash
jq 'select(.event.severity==4 and .event.outcome=="deny")'
```

Operations that started but never finished (an orphan means the process died mid-operation — see [Reliability](#reliability)):

```bash
jq 'select(.event.outcome=="unknown")'
```

Everything in one request, across audit and application logs:

```bash
jq 'select(.correlation_id=="c0ffee-…" or .event.correlationId=="c0ffee-…")'
```

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

Authorization is always enforced: every audited route carries the `requiredPermission` that was checked, so a record names the decision that was made rather than the one that might have been.

`system:startup` is recorded when the service starts, which bounds any gap in the trail to a restart — a reader can tell "nothing happened" apart from "the service was not running".

## Reliability

| Path | Behaviour on failure |
|---|---|
| Ordinary records (`audit.Record`) | Asynchronous, buffered. On overflow, dropped and counted — never blocks a request. A slow sink must not become an outage. |
| Security-critical operations (`audit.Begin`) | Synchronous. If the record cannot be written the operation is **refused**, because an untraceable privileged change is worse than a failed one. |

Drops are counted and logged to the application log, at most once every 30 seconds, with the running total. A trail that silently loses records is worse than one that admits it — which is why emitting `system:audit-dropped` into the trail itself is listed under [Not yet covered](#not-yet-covered).

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
| Agent OAuth identity | `agent-identity:provision` / `:regenerate-secret` / `:revoke-secret` / `:retry-provisioning` | Fail-closed |
| Agent OAuth identity | `system:agent-identity-provisioned` / `system:agent-identity-exhausted` (reconciler, system actor) | Fail-open |
| Env identity credential | `service-account:configure` / `:remove` | Fail-closed |
| Env Thunder URL | `thunder-url:set` / `:delete` | Fail-closed |
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

## Action reference

The explicitly registered actions — the class and severity that drive alerting, and the detail keys each is **permitted** to carry beyond the shared ones. A key not listed is dropped before the record is written. A permitted key is not necessarily set: the [sample records](#sample-records) below show what each emit site actually writes. Coverage-tier actions derived from a route's permission and not listed here are classified by `classify` in `audit/actions.go`.

The registry is the source of truth (`audit/actions.go`, `audit/actions_domain.go`); `audit.RegisteredActions()` enumerates it.

| Action | Class | Severity | Detail keys |
|---|---|---|---|
| `agent-config:delete` | config | 2 notice | `agentName`, `configName`, `configType`, `environmentCount` |
| `agent-config:update` | config | 1 info | `agentName`, `configName`, `configType`, `updatedFields` |
| `agent-identity:provision` | credential | 4 critical | `agentName`, `alreadyExisted`, `clientId`, `environment` |
| `agent-identity:regenerate-secret` | credential | 4 critical | `agentName`, `clientId`, `environment` |
| `agent-identity:retry-provisioning` | credential | 4 critical | `agentName`, `environment` |
| `agent-identity:revoke-secret` | credential | 4 critical | `agentName`, `clientId`, `environment` |
| `agent-token:mint` | credential | 4 critical | `agentName`, `expiresIn` |
| `agent-token:regenerate-tracing` | credential | 4 critical | `agentName` |
| `agent:build` | deployment | 2 notice | `agentName`, `buildName`, `commitId` |
| `agent:change-deployment-state` | deployment | 3 warning | `agentName`, `environment`, `toState` |
| `agent:create` | config | 2 notice | `agentName`, `agentType` |
| `agent:delete` | config | 3 warning | `agentName` |
| `agent:deploy` | deployment | 2 notice | `agentName`, `environment`, `imageId`, `isProduction` |
| `agent:promote` | deployment | 3 warning | `agentName`, `environment`, `isProduction`, `sourceEnv`, `targetEnv` |
| `agent:read` | read | 1 info | — |
| `api-key:create` | credential | 4 critical | `expiresAt`, `gatewayConnected`, `gatewayCount`, `keyName`, `ownerName`, `ownerType` |
| `api-key:issue-test` | credential | 2 notice | `expiresAt`, `keyName`, `ownerName`, `ownerType`, `rotated` |
| `api-key:revoke` | credential | 4 critical | `gatewayCount`, `keyName`, `ownerName`, `ownerType` |
| `api-key:rotate` | credential | 4 critical | `expiresAt`, `gatewayConnected`, `gatewayCount`, `keyName`, `ownerName`, `ownerType` |
| `api-key:sync` | read | 2 notice | `keyCount` |
| `authn:failure` | authn | 3 warning | `authHeader`, `reason` |
| `authz:deny` | authz | 3 warning | `grantedScopes`, `missingScope`, `reason` |
| `authz:root-ou-bypass` | authz | 4 critical | `rootOUBypass` |
| `environment:read` | read | 1 info | — |
| `gateway-token:revoke` | credential | 4 critical | `gatewayName`, `tokenId` |
| `gateway-token:rotate` | credential | 4 critical | `expiresAt`, `gatewayName`, `tokenId` |
| `gateway:assign-environment` | config | 2 notice | `environment`, `gatewayName`, `gatewayType`, `vhost` |
| `gateway:create` | config | 2 notice | `environment`, `gatewayName`, `gatewayType`, `vhost` |
| `gateway:delete` | config | 3 warning | `environment`, `gatewayName`, `gatewayType`, `vhost` |
| `gateway:push-manifest` | config | 1 info | `policyCount` |
| `gateway:remove-identity-provider` | credential | 4 critical | `gatewayName`, `identityProviderName`, `issuer`, `jwksUri`, `skipTlsVerify` |
| `gateway:set-identity-provider` | credential | 4 critical | `gatewayName`, `identityProviderName`, `issuer`, `jwksUri`, `skipTlsVerify` |
| `gateway:unassign-environment` | config | 2 notice | `environment`, `gatewayName`, `gatewayType`, `vhost` |
| `gateway:update` | config | 2 notice | `environment`, `gatewayName`, `gatewayType`, `vhost` |
| `git-secret:create` | credential | 4 critical | `secretType`, `username` |
| `git-secret:delete` | credential | 4 critical | — |
| `group:add-member` | identity | 4 critical | `groupName`, `memberCount`, `memberTypes`, `members` |
| `group:remove-member` | identity | 4 critical | `groupName`, `memberCount`, `memberTypes`, `members` |
| `monitor-score:publish` | config | 1 info | `monitorId`, `runId`, `scoreCount` |
| `monitor:create` | config | 1 info | `agentName`, `evaluators`, `monitorName`, `monitorType` |
| `monitor:delete` | config | 2 notice | `agentName`, `evaluators`, `monitorName`, `monitorType` |
| `monitor:rerun` | config | 2 notice | `agentName`, `monitorName`, `runId` |
| `monitor:run-failed` | system | 3 warning | `agentName`, `monitorName`, `reason`, `runId` |
| `monitor:start` | config | 2 notice | `agentName`, `evaluators`, `monitorName`, `monitorType` |
| `monitor:stop` | config | 2 notice | `agentName`, `evaluators`, `monitorName`, `monitorType` |
| `monitor:update` | config | 1 info | `agentName`, `evaluators`, `monitorName`, `monitorType` |
| `project:create` | config | 2 notice | `projectName` |
| `project:delete` | config | 3 warning | `projectName` |
| `project:read` | read | 1 info | — |
| `role:assign` | identity | 4 critical | `assigneeCount`, `assigneeTypes`, `assignees`, `roleName` |
| `role:grant-permission` | identity | 4 critical | `permissionCount`, `permissions`, `resourceServerId`, `roleName` |
| `role:revoke-permission` | identity | 4 critical | `permissionCount`, `permissions`, `resourceServerId`, `roleName` |
| `role:unassign` | identity | 4 critical | `assigneeCount`, `assigneeTypes`, `assignees`, `roleName` |
| `service-account:configure` | credential | 4 critical | `clientId`, `environment` |
| `service-account:remove` | credential | 4 critical | `environment` |
| `system:agent-identity-exhausted` | credential | 3 warning | `agentName`, `environment`, `reason` |
| `system:agent-identity-provisioned` | credential | 2 notice | `agentName`, `clientId`, `environment` |
| `system:audit-dropped` | system | 4 critical | `droppedTotal`, `sink` |
| `system:startup` | system | 2 notice | `instance`, `sinks`, `version` |
| `user:create` | identity | 4 critical | `attributeCount`, `attributeKeys`, `containsSensitiveKey`, `email`, `groups`, `userType`, `username` |
| `user:delete` | identity | 4 critical | `username` |
| `user:invite` | identity | 4 critical | `attributeCount`, `attributeKeys`, `containsSensitiveKey`, `email`, `groups`, `userType`, `username` |
| `user:update` | identity | 4 critical | `attributeCount`, `attributeKeys`, `containsSensitiveKey`, `email`, `groups`, `userType`, `username` |

Read actions (`agent:read`, `project:read`, `environment:read`) are declared so every MCP tool can name what it does; they are classified as reads and not recorded.

### Sample records

One record per action, produced by the audit emitter on `main` — `audit.Begin`/`Complete`, `Record`, `RecordSync`, `RecordAncillary`, the coverage envelope and `AuthFailureRecorder` — called with the same options, route pattern and permission as the production emit site named beside it, then serialized by the stdout sink's JSON handler. Each was checked for: class and severity matching the registry, no `_droppedKeys`, and for `Begin` actions an intent record with outcome `unknown` whose `eventId` the outcome record carries as `attemptEventId`. Only the outcome record is shown. Identifiers, names and timestamps are illustrative; every key and the shape of every value is real.

Not shown: the three read actions, which are never recorded, and `system:audit-dropped`, which nothing emits yet.

Where two emit sites share an action, the sample shows the first listed; the others differ as noted.

<details>
<summary><code>agent-config:update</code> — Record · `services/agent_configuration_service.go:3534`</summary>

Model configs only; MCP-config updates produce the coverage envelope alone.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "agent-config:update",
    "actionClass": "config",
    "severity": 1,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "PUT",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "agent-config",
    "resourceId": "cfg-2d7a",
    "resourceName": "gpt-4o-primary",
    "projectName": "payments",
    "outcome": "success",
    "requiredPermission": "amp:agent:update",
    "details": {
      "agentName": "checkout-agent",
      "configName": "gpt-4o-primary",
      "updatedFields": [
        "description",
        "envMappings"
      ]
    }
  }
}
```

</details>

<details>
<summary><code>agent-config:delete</code> — Record · `services/agent_configuration_service.go:3820`</summary>

Model configs only; MCP-config deletes produce the coverage envelope alone.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "agent-config:delete",
    "actionClass": "config",
    "severity": 2,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "DELETE",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "agent-config",
    "resourceId": "cfg-2d7a",
    "resourceName": "gpt-4o-primary",
    "projectName": "payments",
    "outcome": "success",
    "requiredPermission": "amp:agent:delete",
    "details": {
      "agentName": "checkout-agent",
      "configName": "gpt-4o-primary",
      "environmentCount": 3
    }
  }
}
```

</details>

<details>
<summary><code>agent-identity:provision</code> — Begin + Complete · `controllers/agent_controller.go:1263`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "agent-identity:provision",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "PUT",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/identities",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "agent-identity",
    "resourceId": "checkout-agent",
    "resourceName": "checkout-agent",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "requiredPermission": "amp:agent:update",
    "details": {
      "agentName": "checkout-agent",
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "environment": "production"
    }
  }
}
```

</details>

<details>
<summary><code>agent-identity:regenerate-secret</code> — Begin + Complete · `controllers/agent_controller.go:1156`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "agent-identity:regenerate-secret",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/identities",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "agent-identity",
    "resourceId": "checkout-agent",
    "resourceName": "checkout-agent",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "requiredPermission": "amp:agent:update",
    "details": {
      "agentName": "checkout-agent",
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "environment": "production"
    }
  }
}
```

</details>

<details>
<summary><code>agent-identity:retry-provisioning</code> — Begin + Complete · `controllers/agent_controller.go:1329`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "agent-identity:retry-provisioning",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/identities/retry",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "agent-identity",
    "resourceId": "checkout-agent",
    "resourceName": "checkout-agent",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "requiredPermission": "amp:agent:update",
    "details": {
      "agentName": "checkout-agent",
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "environment": "production"
    }
  }
}
```

</details>

<details>
<summary><code>agent-identity:revoke-secret</code> — Begin + Complete · `controllers/agent_controller.go:1208`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "agent-identity:revoke-secret",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "DELETE",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/identities",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "agent-identity",
    "resourceId": "checkout-agent",
    "resourceName": "checkout-agent",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "requiredPermission": "amp:agent:update",
    "details": {
      "agentName": "checkout-agent",
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "environment": "production"
    }
  }
}
```

</details>

<details>
<summary><code>agent-token:mint</code> — RecordSync · `services/agent_token_manager.go:364`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "agent-token:mint",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/token",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "agent",
    "resourceId": "5b1e7c2a-9d4f-4a3b-8e6c-0f2d1a9b7c34",
    "resourceName": "checkout-agent",
    "projectName": "payments",
    "environment": "development",
    "outcome": "success",
    "requiredPermission": "amp:agent:token-manage",
    "details": {
      "agentName": "checkout-agent",
      "expiresIn": 3600
    }
  }
}
```

</details>

<details>
<summary><code>agent-token:regenerate-tracing</code> — Begin + Complete · `controllers/agent_controller.go:692`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "agent-token:regenerate-tracing",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/tracing-token/regenerate",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "agent",
    "resourceId": "checkout-agent",
    "resourceName": "checkout-agent",
    "projectName": "payments",
    "environment": "development",
    "outcome": "success",
    "requiredPermission": "amp:agent:token-manage",
    "details": {
      "agentName": "checkout-agent",
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35"
    }
  }
}
```

</details>

<details>
<summary><code>agent:build</code> — Record · `services/agent_manager.go:2938`</summary>

Builds through MCP (`build_agent`) additionally produce a bare wrapper record carrying only `tool`.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "agent:build",
    "actionClass": "deployment",
    "severity": 2,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/builds",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "agent",
    "resourceId": "5b1e7c2a-9d4f-4a3b-8e6c-0f2d1a9b7c34",
    "resourceName": "checkout-agent",
    "projectName": "payments",
    "outcome": "success",
    "requiredPermission": "amp:agent:build",
    "details": {
      "agentName": "checkout-agent",
      "buildName": "checkout-agent-build-42",
      "commitId": "9f3c2e1"
    }
  }
}
```

</details>

<details>
<summary><code>agent:change-deployment-state</code> — Begin + Complete · `controllers/agent_controller.go:887`</summary>

Changes through MCP (`update_deployment_state`) produce a bare wrapper record carrying only `tool`.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "agent:change-deployment-state",
    "actionClass": "deployment",
    "severity": 3,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments/state",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "agent",
    "resourceId": "checkout-agent",
    "resourceName": "checkout-agent",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "requiredPermission": "amp:agent:suspend amp:agent:env-non-production",
    "details": {
      "agentName": "checkout-agent",
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "environment": "production",
      "toState": "Undeploy"
    }
  }
}
```

</details>

<details>
<summary><code>agent:create</code> — Record · `mcp/tools/helpers.go:195`</summary>

MCP only (`create_external_agent`, `create_internal_agent_python`). REST agent creation is covered by the coverage envelope alone.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "agent:create",
    "actionClass": "config",
    "severity": 2,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "mcp",
    "sourceIp": "203.0.113.77",
    "userAgent": "claude-code/2.1",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "outcome": "success",
    "details": {
      "tool": "create_external_agent"
    }
  }
}
```

</details>

<details>
<summary><code>agent:delete</code> — Begin + Complete · `services/agent_manager.go:2646`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "agent:delete",
    "actionClass": "config",
    "severity": 3,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "DELETE",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "agent",
    "resourceId": "checkout-agent",
    "resourceName": "checkout-agent",
    "projectName": "payments",
    "outcome": "success",
    "requiredPermission": "amp:agent:delete",
    "details": {
      "agentName": "checkout-agent",
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35"
    }
  }
}
```

</details>

<details>
<summary><code>agent:deploy</code> — Begin + Complete · `services/agent_manager.go:3281`</summary>

Deploys through MCP (`deploy_agent`) additionally produce a bare wrapper record carrying only `tool`.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "agent:deploy",
    "actionClass": "deployment",
    "severity": 2,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "agent",
    "resourceId": "5b1e7c2a-9d4f-4a3b-8e6c-0f2d1a9b7c34",
    "resourceName": "checkout-agent",
    "projectName": "payments",
    "environment": "development",
    "outcome": "success",
    "requiredPermission": "amp:agent:env-non-production",
    "details": {
      "agentName": "checkout-agent",
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "environment": "development",
      "imageId": "registry.example/checkout-agent:9f3c2e1",
      "isProduction": false
    }
  }
}
```

</details>

<details>
<summary><code>agent:promote</code> — Begin + Complete · `services/agent_manager.go:4496`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "agent:promote",
    "actionClass": "deployment",
    "severity": 3,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/promote",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "agent",
    "resourceId": "5b1e7c2a-9d4f-4a3b-8e6c-0f2d1a9b7c34",
    "resourceName": "checkout-agent",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "requiredPermission": "amp:agent:env-non-production",
    "details": {
      "agentName": "checkout-agent",
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "environment": "production",
      "isProduction": true,
      "sourceEnv": "staging",
      "targetEnv": "production"
    }
  }
}
```

</details>

<details>
<summary><code>api-key:create</code> — Begin + Complete · `services/agent_apikey_service.go:189`</summary>

Agent key. Model-config and MCP-config keys (`controllers/agent_configuration_controller.go`) use `ownerType` `model-config` / `mcp-config` and carry no `gatewayCount`; LLM-provider and LLM-proxy keys carry no project or environment.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "api-key:create",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "api-key",
    "resourceId": "art-71c2",
    "resourceName": "ci-key",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "requiredPermission": "amp:agent:api-key-manage",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "gatewayConnected": true,
      "gatewayCount": 2,
      "keyName": "ci-key",
      "ownerName": "checkout-agent",
      "ownerType": "agent"
    }
  }
}
```

</details>

<details>
<summary><code>api-key:issue-test</code> — Record · `services/agent_apikey_service.go:365`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "api-key:issue-test",
    "actionClass": "credential",
    "severity": 2,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/test",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "api-key",
    "resourceId": "art-71c2",
    "resourceName": "test-key-8c1f04b2",
    "projectName": "payments",
    "environment": "development",
    "outcome": "success",
    "requiredPermission": "amp:agent:api-key-manage",
    "details": {
      "expiresAt": "2026-09-25T09:13:37Z",
      "keyName": "test-key-8c1f04b2",
      "ownerName": "checkout-agent",
      "ownerType": "agent",
      "rotated": false
    }
  }
}
```

</details>

<details>
<summary><code>api-key:revoke</code> — Begin + Complete · `services/agent_apikey_service.go:234`</summary>

Agent key; the other owners differ as for `api-key:create`.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "api-key:revoke",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "DELETE",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/{keyName}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "api-key",
    "resourceId": "art-71c2",
    "resourceName": "ci-key",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "requiredPermission": "amp:agent:api-key-manage",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "gatewayCount": 2,
      "keyName": "ci-key",
      "ownerName": "checkout-agent",
      "ownerType": "agent"
    }
  }
}
```

</details>

<details>
<summary><code>api-key:rotate</code> — Begin + Complete · `services/agent_apikey_service.go:269`</summary>

Agent key; the other owners differ as for `api-key:create`.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "api-key:rotate",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "PUT",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/{keyName}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "api-key",
    "resourceId": "art-71c2",
    "resourceName": "ci-key",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "requiredPermission": "amp:agent:api-key-manage",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "gatewayConnected": true,
      "gatewayCount": 2,
      "keyName": "ci-key",
      "ownerName": "checkout-agent",
      "ownerType": "agent"
    }
  }
}
```

</details>

<details>
<summary><code>api-key:sync</code> — coverage envelope · `middleware/audit_route.go:139 (coverage envelope)`</summary>

No semantic emit: the coverage envelope for the gateway bulk-sync routes, coalesced to one record per gateway per window.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "durationMs": 42,
    "action": "api-key:sync",
    "actionClass": "read",
    "severity": 2,
    "actorType": "gateway",
    "actorId": "gw-3c9e",
    "authMethod": "api-key",
    "surface": "internal",
    "sourceIp": "203.0.113.77",
    "userAgent": "api-platform-gateway/1.0",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "GET",
    "requestPath": "/llm-providers/api-keys",
    "outcome": "success",
    "statusCode": 200,
    "details": {
      "envelope": true
    }
  }
}
```

</details>

<details>
<summary><code>authn:failure</code> — Record · `audit/authn.go:110`</summary>

REST edge. The internal gateway server records its own (`controllers/gateway_internal_audit.go`) with surface `internal`, actor `gateway` and auth method `api-key`.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "authn:failure",
    "actionClass": "authn",
    "severity": 3,
    "actorType": "anonymous",
    "authMethod": "jwt-bearer",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "curl/8.4.0",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "GET",
    "requestPath": "/api/v1/orgs/acme/projects/payments/agents",
    "outcome": "deny",
    "statusCode": 401,
    "details": {
      "authHeader": true,
      "reason": "expired"
    }
  }
}
```

</details>

<details>
<summary><code>authz:deny</code> — Record · `middleware/authorization.go:51`</summary>

REST scope denial. MCP denials (`mcp/tools/authz.go`) carry `tool` and no `grantedScopes`; environment-tier denials (`requireEnvTier`) carry reason `missing-environment-tier-scope`.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "authz:deny",
    "actionClass": "authz",
    "severity": 3,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/identities/roles/{roleID}/permissions/add",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "outcome": "deny",
    "statusCode": 403,
    "requiredPermission": "amp:role:update",
    "details": {
      "grantedScopes": 11,
      "missingScope": "amp:role:update",
      "reason": "missing-scope"
    }
  }
}
```

</details>

<details>
<summary><code>authz:root-ou-bypass</code> — RecordAncillary · `middleware/authorization.go:178`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "authz:root-ou-bypass",
    "actionClass": "authz",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/git-secrets",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "outcome": "success",
    "requiredPermission": "amp:git-secret:create",
    "details": {
      "rootOUBypass": true
    }
  }
}
```

</details>

<details>
<summary><code>gateway-token:revoke</code> — Begin + Complete · `controllers/gateway_controller.go:642`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "gateway-token:revoke",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "DELETE",
    "requestPath": "/orgs/{orgName}/gateways/{gatewayID}/tokens/{tokenID}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "outcome": "success",
    "requiredPermission": "amp:gateway:token-manage",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "tokenId": "tok-1f8b"
    }
  }
}
```

</details>

<details>
<summary><code>gateway-token:rotate</code> — Begin + Complete · `controllers/gateway_controller.go:602`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "gateway-token:rotate",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/gateways/{gatewayID}/tokens",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "outcome": "success",
    "requiredPermission": "amp:gateway:token-manage",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "tokenId": "tok-4a2d"
    }
  }
}
```

</details>

<details>
<summary><code>gateway:assign-environment</code> — Record · `controllers/gateway_controller.go:430`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "gateway:assign-environment",
    "actionClass": "config",
    "severity": 2,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/gateways/{gatewayID}/environments/{envID}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "environment": "e7a1c3d2-4b5f-4e6a-9c8d-1f2e3a4b5c6d",
    "outcome": "success",
    "requiredPermission": "amp:gateway:update",
    "details": {
      "environment": "e7a1c3d2-4b5f-4e6a-9c8d-1f2e3a4b5c6d"
    }
  }
}
```

</details>

<details>
<summary><code>gateway:create</code> — Record · `controllers/gateway_controller.go:204`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "gateway:create",
    "actionClass": "config",
    "severity": 2,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/gateways",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "resourceName": "edge-eu",
    "outcome": "success",
    "requiredPermission": "amp:gateway:create",
    "details": {
      "gatewayName": "edge-eu",
      "gatewayType": "egress",
      "vhost": "edge-eu.example.com"
    }
  }
}
```

</details>

<details>
<summary><code>gateway:delete</code> — Begin + Complete · `controllers/gateway_controller.go:387`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "gateway:delete",
    "actionClass": "config",
    "severity": 3,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "DELETE",
    "requestPath": "/orgs/{orgName}/gateways/{gatewayID}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "outcome": "success",
    "requiredPermission": "amp:gateway:delete",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35"
    }
  }
}
```

</details>

<details>
<summary><code>gateway:push-manifest</code> — Record · `controllers/gateway_internal_controller.go:455`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "gateway:push-manifest",
    "actionClass": "config",
    "severity": 1,
    "actorType": "gateway",
    "actorId": "gw-3c9e",
    "authMethod": "api-key",
    "surface": "internal",
    "sourceIp": "203.0.113.77",
    "userAgent": "api-platform-gateway/1.0",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/gateways/{gatewayId}/manifest",
    "ouId": "ou-9f2",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "outcome": "success",
    "details": {
      "policyCount": 4
    }
  }
}
```

</details>

<details>
<summary><code>gateway:remove-identity-provider</code> — Begin + Complete · `controllers/gateway_identity_provider_controller.go:196`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "gateway:remove-identity-provider",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "DELETE",
    "requestPath": "/orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "resourceName": "gw-3c9e",
    "outcome": "success",
    "requiredPermission": "amp:gateway:update",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "identityProviderName": "corp-idp"
    }
  }
}
```

</details>

<details>
<summary><code>gateway:set-identity-provider</code> — Begin + Complete · `controllers/gateway_identity_provider_controller.go:157`</summary>

`jwksUri` was supplied as `https://svc:s3cret@idp.example.com/oauth2/jwks?key=abc`; userinfo and query are stripped at write time.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "gateway:set-identity-provider",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "PUT",
    "requestPath": "/orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "resourceName": "gw-3c9e",
    "outcome": "success",
    "requiredPermission": "amp:gateway:update",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "identityProviderName": "corp-idp",
      "issuer": "https://idp.example.com",
      "jwksUri": "https://idp.example.com/oauth2/jwks[redacted-components]",
      "skipTlsVerify": false
    }
  }
}
```

</details>

<details>
<summary><code>gateway:unassign-environment</code> — Record · `controllers/gateway_controller.go:470`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "gateway:unassign-environment",
    "actionClass": "config",
    "severity": 2,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "DELETE",
    "requestPath": "/orgs/{orgName}/gateways/{gatewayID}/environments/{envID}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "environment": "e7a1c3d2-4b5f-4e6a-9c8d-1f2e3a4b5c6d",
    "outcome": "success",
    "requiredPermission": "amp:gateway:update",
    "details": {
      "environment": "e7a1c3d2-4b5f-4e6a-9c8d-1f2e3a4b5c6d"
    }
  }
}
```

</details>

<details>
<summary><code>gateway:update</code> — Record · `controllers/gateway_controller.go:362`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "gateway:update",
    "actionClass": "config",
    "severity": 2,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "PUT",
    "requestPath": "/orgs/{orgName}/gateways/{gatewayID}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "outcome": "success",
    "requiredPermission": "amp:gateway:update"
  }
}
```

</details>

<details>
<summary><code>git-secret:create</code> — Begin + Complete · `services/git_secret_service.go:79`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "git-secret:create",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/git-secrets",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "git-secret",
    "resourceId": "github-deploy-key",
    "resourceName": "github-deploy-key",
    "outcome": "success",
    "requiredPermission": "amp:git-secret:create",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "secretType": "basic-auth",
      "username": "deploy-bot"
    }
  }
}
```

</details>

<details>
<summary><code>git-secret:delete</code> — Begin + Complete · `services/git_secret_service.go:154`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "git-secret:delete",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "DELETE",
    "requestPath": "/orgs/{orgName}/git-secrets/{secretName}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "git-secret",
    "resourceId": "github-deploy-key",
    "resourceName": "github-deploy-key",
    "outcome": "success",
    "requiredPermission": "amp:git-secret:delete",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35"
    }
  }
}
```

</details>

<details>
<summary><code>group:add-member</code> — Begin + Complete · `controllers/identity_controller.go:716`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "group:add-member",
    "actionClass": "identity",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/identities/groups/{groupID}/members/add",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "group",
    "resourceId": "grp-5e1a",
    "resourceName": "platform-eng",
    "outcome": "success",
    "requiredPermission": "amp:group:update",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "groupName": "platform-eng",
      "memberCount": 1,
      "members": [
        "2b7d9e41-0c3a-4f6e-8d1b-7a9c5e3f2d10"
      ]
    }
  }
}
```

</details>

<details>
<summary><code>group:remove-member</code> — Begin + Complete · `controllers/identity_controller.go:780`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "group:remove-member",
    "actionClass": "identity",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/identities/groups/{groupID}/members/remove",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "group",
    "resourceId": "grp-5e1a",
    "resourceName": "platform-eng",
    "outcome": "success",
    "requiredPermission": "amp:group:update",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "groupName": "platform-eng",
      "memberCount": 1,
      "members": [
        "2b7d9e41-0c3a-4f6e-8d1b-7a9c5e3f2d10"
      ]
    }
  }
}
```

</details>

<details>
<summary><code>monitor-score:publish</code> — Record · `controllers/monitor_scores_publisher_controller.go:89`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "monitor-score:publish",
    "actionClass": "config",
    "severity": 1,
    "actorType": "service",
    "actorId": "amp-publisher-eval",
    "authMethod": "publisher-client",
    "actorTokenId": "jti-51ab",
    "surface": "publisher",
    "sourceIp": "203.0.113.77",
    "userAgent": "amp-evaluation-job/1.0",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/publisher/monitors/{monitorId}/runs/{runId}/scores",
    "resourceType": "monitor-run",
    "resourceId": "run-8d21",
    "resourceName": "run-8d21",
    "outcome": "success",
    "details": {
      "monitorId": "mon-6b3f",
      "runId": "run-8d21",
      "scoreCount": 24
    }
  }
}
```

</details>

<details>
<summary><code>monitor:create</code> — Record · `services/monitor_manager.go:344`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "monitor:create",
    "actionClass": "config",
    "severity": 1,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "monitor",
    "resourceId": "mon-6b3f",
    "resourceName": "latency-slo",
    "projectName": "payments",
    "outcome": "success",
    "requiredPermission": "amp:monitor:create",
    "details": {
      "agentName": "checkout-agent",
      "monitorName": "latency-slo",
      "monitorType": "scheduled"
    }
  }
}
```

</details>

<details>
<summary><code>monitor:delete</code> — Record · `services/monitor_manager.go:800`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "monitor:delete",
    "actionClass": "config",
    "severity": 2,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "DELETE",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "monitor",
    "resourceId": "mon-6b3f",
    "resourceName": "latency-slo",
    "projectName": "payments",
    "outcome": "success",
    "requiredPermission": "amp:monitor:delete",
    "details": {
      "agentName": "checkout-agent",
      "monitorName": "latency-slo"
    }
  }
}
```

</details>

<details>
<summary><code>monitor:rerun</code> — Record · `services/monitor_manager.go:1064`</summary>

The route derives `monitor:execute`, so a failed request's envelope is labelled `monitor:execute`.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "monitor:rerun",
    "actionClass": "config",
    "severity": 2,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/runs/{runId}/rerun",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "monitor",
    "resourceId": "mon-6b3f",
    "resourceName": "latency-slo",
    "projectName": "payments",
    "outcome": "success",
    "requiredPermission": "amp:monitor:execute",
    "details": {
      "agentName": "checkout-agent",
      "monitorName": "latency-slo",
      "runId": "run-8d21"
    }
  }
}
```

</details>

<details>
<summary><code>monitor:run-failed</code> — Record · `services/monitor_scheduler.go:226`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "monitor:run-failed",
    "actionClass": "system",
    "severity": 3,
    "actorType": "system",
    "actorId": "system:monitor-scheduler",
    "surface": "system",
    "correlationId": "-",
    "ouId": "ou-9f2",
    "resourceType": "monitor",
    "resourceId": "mon-6b3f",
    "resourceName": "latency-slo",
    "outcome": "failure",
    "errorMessage": "evaluation job could not be scheduled",
    "details": {
      "monitorName": "latency-slo",
      "reason": "execute-failed"
    }
  }
}
```

</details>

<details>
<summary><code>monitor:start</code> — Record · `services/monitor_manager.go:931`</summary>

The route derives `monitor:execute`, so a failed request's envelope is labelled `monitor:execute`.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "monitor:start",
    "actionClass": "config",
    "severity": 2,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/start",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "monitor",
    "resourceId": "mon-6b3f",
    "resourceName": "latency-slo",
    "projectName": "payments",
    "outcome": "success",
    "requiredPermission": "amp:monitor:execute",
    "details": {
      "agentName": "checkout-agent",
      "monitorName": "latency-slo"
    }
  }
}
```

</details>

<details>
<summary><code>monitor:stop</code> — Record · `services/monitor_manager.go:875`</summary>

The route derives `monitor:execute`, so a failed request's envelope is labelled `monitor:execute`.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "monitor:stop",
    "actionClass": "config",
    "severity": 2,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/stop",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "monitor",
    "resourceId": "mon-6b3f",
    "resourceName": "latency-slo",
    "projectName": "payments",
    "outcome": "success",
    "requiredPermission": "amp:monitor:execute",
    "details": {
      "agentName": "checkout-agent",
      "monitorName": "latency-slo"
    }
  }
}
```

</details>

<details>
<summary><code>monitor:update</code> — Record · `services/monitor_manager.go:723`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "monitor:update",
    "actionClass": "config",
    "severity": 1,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "PATCH",
    "requestPath": "/orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "monitor",
    "resourceId": "mon-6b3f",
    "resourceName": "latency-slo",
    "projectName": "payments",
    "outcome": "success",
    "requiredPermission": "amp:monitor:update",
    "details": {
      "agentName": "checkout-agent",
      "monitorName": "latency-slo"
    }
  }
}
```

</details>

<details>
<summary><code>project:create</code> — Record · `mcp/tools/helpers.go:195`</summary>

MCP only (`create_project`). REST project creation is covered by the coverage envelope alone.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "project:create",
    "actionClass": "config",
    "severity": 2,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "mcp",
    "sourceIp": "203.0.113.77",
    "userAgent": "claude-code/2.1",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "outcome": "success",
    "details": {
      "tool": "create_project"
    }
  }
}
```

</details>

<details>
<summary><code>project:delete</code> — Begin + Complete · `services/infra_resource_manager.go:298`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "project:delete",
    "actionClass": "config",
    "severity": 3,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "DELETE",
    "requestPath": "/orgs/{orgName}/projects/{projName}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "project",
    "resourceId": "payments",
    "resourceName": "payments",
    "projectName": "payments",
    "outcome": "success",
    "requiredPermission": "amp:project:delete",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "projectName": "payments"
    }
  }
}
```

</details>

<details>
<summary><code>role:assign</code> — Begin + Complete · `controllers/identity_controller.go:1291`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "role:assign",
    "actionClass": "identity",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/identities/roles/{roleID}/assignees/add",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "role",
    "resourceId": "role-9a4c",
    "resourceName": "sre",
    "outcome": "success",
    "requiredPermission": "amp:role:update",
    "details": {
      "assigneeCount": 1,
      "assigneeTypes": [
        "user"
      ],
      "assignees": [
        "2b7d9e41-0c3a-4f6e-8d1b-7a9c5e3f2d10"
      ],
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "roleName": "sre"
    }
  }
}
```

</details>

<details>
<summary><code>role:grant-permission</code> — Begin + Complete · `controllers/identity_controller.go:1176`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "role:grant-permission",
    "actionClass": "identity",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/identities/roles/{roleID}/permissions/add",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "role",
    "resourceId": "role-9a4c",
    "resourceName": "sre",
    "outcome": "success",
    "requiredPermission": "amp:role:update",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "permissionCount": 1,
      "permissions": [
        "amp:agent:env-production"
      ],
      "resourceServerId": "rs-amp",
      "roleName": "sre"
    }
  }
}
```

</details>

<details>
<summary><code>role:revoke-permission</code> — Begin + Complete · `controllers/identity_controller.go:1235`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "role:revoke-permission",
    "actionClass": "identity",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/identities/roles/{roleID}/permissions/remove",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "role",
    "resourceId": "role-9a4c",
    "resourceName": "sre",
    "outcome": "success",
    "requiredPermission": "amp:role:update",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "permissionCount": 1,
      "permissions": [
        "amp:agent:env-production"
      ],
      "resourceServerId": "rs-amp",
      "roleName": "sre"
    }
  }
}
```

</details>

<details>
<summary><code>role:unassign</code> — Begin + Complete · `controllers/identity_controller.go:1347`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "role:unassign",
    "actionClass": "identity",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/identities/roles/{roleID}/assignees/remove",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "role",
    "resourceId": "role-9a4c",
    "resourceName": "sre",
    "outcome": "success",
    "requiredPermission": "amp:role:update",
    "details": {
      "assigneeCount": 1,
      "assigneeTypes": [
        "user"
      ],
      "assignees": [
        "2b7d9e41-0c3a-4f6e-8d1b-7a9c5e3f2d10"
      ],
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "roleName": "sre"
    }
  }
}
```

</details>

<details>
<summary><code>service-account:configure</code> — Begin + Complete · `controllers/environment_controller.go:426`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "service-account:configure",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "PUT",
    "requestPath": "/orgs/{orgName}/environments/{envID}/thunder-system-client",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "service-account",
    "resourceId": "production",
    "resourceName": "production",
    "environment": "production",
    "outcome": "success",
    "requiredPermission": "amp:org:manage-service-account",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "clientId": "amp-sys-client-prod",
      "environment": "production"
    }
  }
}
```

</details>

<details>
<summary><code>service-account:remove</code> — Begin + Complete · `controllers/environment_controller.go:462`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "service-account:remove",
    "actionClass": "credential",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "DELETE",
    "requestPath": "/orgs/{orgName}/environments/{envID}/thunder-system-client",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "service-account",
    "resourceId": "production",
    "resourceName": "production",
    "environment": "production",
    "outcome": "success",
    "requiredPermission": "amp:org:manage-service-account",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "environment": "production"
    }
  }
}
```

</details>

<details>
<summary><code>system:agent-identity-exhausted</code> — Record · `services/agent_thunder_provisioning_service.go:876`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "system:agent-identity-exhausted",
    "actionClass": "credential",
    "severity": 3,
    "actorType": "system",
    "actorId": "system:agent-thunder-reconciler",
    "onBehalfOf": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "surface": "system",
    "correlationId": "-",
    "ouId": "ou-9f2",
    "resourceType": "agent-identity",
    "resourceId": "checkout-agent",
    "resourceName": "checkout-agent",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "details": {
      "agentName": "checkout-agent",
      "environment": "production",
      "reason": "retries-exhausted"
    }
  }
}
```

</details>

<details>
<summary><code>system:agent-identity-provisioned</code> — Record · `services/agent_thunder_provisioning_service.go:816`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "system:agent-identity-provisioned",
    "actionClass": "credential",
    "severity": 2,
    "actorType": "system",
    "actorId": "system:agent-thunder-reconciler",
    "onBehalfOf": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "surface": "system",
    "correlationId": "-",
    "ouId": "ou-9f2",
    "resourceType": "agent-identity",
    "resourceId": "checkout-agent",
    "resourceName": "checkout-agent",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "details": {
      "agentName": "checkout-agent",
      "clientId": "agt-checkout-prod",
      "environment": "production"
    }
  }
}
```

</details>

<details>
<summary><code>system:startup</code> — RecordAncillary · `app/app.go:345`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "system:startup",
    "actionClass": "system",
    "severity": 2,
    "actorType": "system",
    "actorId": "system:agent-manager-service",
    "surface": "system",
    "correlationId": "-",
    "outcome": "success",
    "details": {
      "sinks": [
        "stdout"
      ]
    }
  }
}
```

</details>

<details>
<summary><code>user:create</code> — Begin + Complete · `controllers/identity_controller.go:198`</summary>

The request's attribute map held `username`, `email`, `password` and `department`; only the key names reach the record.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "user:create",
    "actionClass": "identity",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/identities/users",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "user",
    "resourceId": "carol",
    "resourceName": "carol",
    "outcome": "success",
    "requiredPermission": "amp:org:invite-member",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "attributeCount": 4,
      "attributeKeys": [
        "department",
        "email",
        "password",
        "username"
      ],
      "containsSensitiveKey": true,
      "userType": "employee",
      "username": "carol"
    }
  }
}
```

</details>

<details>
<summary><code>user:delete</code> — Begin + Complete · `controllers/identity_controller.go:308`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "user:delete",
    "actionClass": "identity",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "DELETE",
    "requestPath": "/orgs/{orgName}/identities/users/{userID}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "user",
    "resourceId": "2b7d9e41-0c3a-4f6e-8d1b-7a9c5e3f2d10",
    "resourceName": "carol",
    "outcome": "success",
    "requiredPermission": "amp:org:remove-member",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "username": "carol"
    }
  }
}
```

</details>

<details>
<summary><code>user:invite</code> — Begin + Complete · `controllers/identity_controller.go:436`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "user:invite",
    "actionClass": "identity",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "POST",
    "requestPath": "/orgs/{orgName}/identities/users/invite",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "user",
    "resourceId": "dave@example.com",
    "resourceName": "dave@example.com",
    "outcome": "success",
    "requiredPermission": "amp:org:invite-member",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "email": "dave@example.com"
    }
  }
}
```

</details>

<details>
<summary><code>user:update</code> — coverage envelope · `middleware/audit_route.go:139 (coverage envelope)`</summary>

No semantic emit: the coverage envelope for the route.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "durationMs": 42,
    "action": "user:update",
    "actionClass": "identity",
    "severity": 4,
    "actorType": "user",
    "actorId": "8c1f04b2-7e39-4a6d-9f5b-2d0ae83c1147",
    "actorOuId": "ou-9f2",
    "authMethod": "jwt-bearer",
    "actorTokenId": "jti-7c1e",
    "surface": "api",
    "sourceIp": "203.0.113.77",
    "userAgent": "amctl/1.2.3",
    "correlationId": "c0ffee-4d2a-9b1e",
    "requestMethod": "PUT",
    "requestPath": "/orgs/{orgName}/identities/users/{userID}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "outcome": "success",
    "statusCode": 200,
    "requiredPermission": "amp:org:invite-member",
    "details": {
      "envelope": true
    }
  }
}
```

</details>

## Not yet covered

Semantic-event coverage is incomplete, not absent, in these places: some events are never emitted, and others are emitted but not registered, so they lack a detail schema. Each is a real gap in coverage today:

- **Thunder's own events** — see the authentication gap above. This is the one gap that cannot be closed from this repository.
- **`system:audit-dropped`** — registered, but the recorder only logs drops to the application log; no record reaches the trail.
- **`thunder-url:set` / `:delete`** — emitted (fail-closed) but not registered, so they are classified by heuristic (`config`, info / notice) and their `handle` and `url` details are dropped into `_droppedKeys`.
