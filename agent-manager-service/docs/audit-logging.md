# Audit logging

Agent Manager records who did what, to which resource, with what outcome. This document covers what is recorded, what is deliberately not, and — most importantly — **what the deployment must provide for the trail to be usable as audit evidence**.

## What gets recorded

Every state-changing API call, every authorization denial, and every rejected token *attempts* one record. Records are written to **stdout as structured JSON** tagged `log_type=audit`, for a log pipeline to collect from the `amp-api` container.

"Attempts" rather than "produces", because two paths deliberately emit less than one record per event: authentication and internal-surface denials are rate-limited per source (see [Denials](#denials)), and ordinary buffered records are dropped rather than allowed to block a request when the buffer fills. The two end up in different places. Suppressed JWT authentication failures are reported in the trail: their count rides on the next emitted `authn:failure` record as `suppressedCount` (repeats on the internal surface are suppressed without a count). Buffer drops are counted and logged only to the application log (`audit buffer full; event dropped`, with a running `droppedTotal`) — they do **not** reach the audit trail, because `system:audit-dropped` is registered but nothing emits it (see [Not yet covered](#not-yet-covered)). Either way, the record count is a floor, not an exact tally. The one path with no such allowance is `audit.Begin`, which refuses the operation rather than losing the record (see [Reliability](#reliability)).

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

When a semantic event describes a successful request, the coverage tier stands down so the trail carries one record rather than two. On failure the coverage record is always kept — a request rejected before it reached the service emits nothing semantic, and that rejection is exactly what must not go unrecorded.

### Which routes and tools

- **Every non-GET route.** A test (`api/audit_coverage_test.go`) fails the build if a mutating route is not audited, so this cannot drift.
- **Every MCP tool that changes state.** `addTool` requires an `audit.Action` alongside its permissions and panics without one, so a new mutating tool cannot ship unattributed — the same registration-time guarantee the route registrar gives REST. Read-only tools declare a read action and are not recorded.
- **Reads only when they disclose credentials or security configuration** — API-key listings, gateway tokens, agent identities, role assignments, identity-provider configuration. Auditing every GET would multiply volume several-fold for little forensic gain.
- One documented exemption: `POST /orgs/{orgName}/utils/generate-name`, which suggests a name and persists nothing.
- **Module routes and tools follow the same rules.** A module's mutating route (`/m/<module>/...`) is recorded under its permission, or under an action its manifest declares and the route names with `modkit.Audit`; the manifest gives that action its class and severity, and `modkit.Audit` on a read records the read. `api/module_routes_invariant_test.go` checks it for the test module and the AI Gateway.

### Record shape

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-08-05T08:13:37Z",
    "action": "ai-gateway:api-keys.create",
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
    "requestPath": "/m/ai-gateway/agents/{agentName}/api-keys",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "api-key",
    "resourceId": "5b1e7c2a-9d4f-4a3b-8e6c-0f2d1a9b7c34",
    "resourceName": "ci-key",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "statusCode": 201,
    "durationMs": 412,
    "requiredPermission": "amp:ai-gateway:agent-api-keys.manage",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "envelope": true,
      "gatewayConnected": true,
      "gatewayCount": 1,
      "keyName": "ci-key",
      "ownerName": "checkout-agent",
      "ownerType": "agent"
    }
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
| `actorId` | Who did it. For a user, the token subject — a Thunder user id, not an email or username (resolve it with `GET /api/v1/orgs/{orgName}/identities/users/{userId}`). For other actors: a gateway id or `system:<component>`. |
| `actorOuId` | The actor's organization unit, from the token. |
| `actorTokenId` | The token's `jti` — the join key to Thunder's login records (see [The authentication gap](#the-authentication-gap)). |
| `actorDisplay` | Reserved for a human-readable name. No emit site populates it today, so records carry the id alone. |
| `onBehalfOf` | Delegation — a reconciler applying what a user asked for earlier. Also a user id. |
| `authMethod` | How the caller authenticated, e.g. `jwt-bearer`. |
| `surface` | Entry point: `api` · `mcp` · `internal` · `system`. |
| `sourceIp` | The leftmost `X-Forwarded-For` hop, falling back to `X-Real-IP` and then the peer address (`utils.ClientIP`). |
| `userAgent` | Distinguishes the console from `amctl` from a script. |
| `correlationId` | Ties the record to the request logs; matches the `x-correlation-id` response header. |
| `requestMethod` · `requestPath` | HTTP verb and the **route pattern**, never the raw URL. The one exception is `authn:failure` on the REST edge: the token is rejected before route matching, so the record carries the URL path — without its query string. |
| `ouId` · `orgHandle` | Tenant, taken from the token — never from the URL path. |
| `resourceType` · `resourceId` · `resourceName` | What was acted on. Names are recorded deliberately, including secret names. |
| `projectName` · `environment` | Scope. On an environment-scoped operation, such as an agent API key, `environment` is what separates a sandbox change from a production one. |
| `statusCode` · `durationMs` | HTTP status and request duration, on coverage-tier records. |
| `errorCode` · `errorMessage` | Failure detail. `errorMessage` passes through the secret-shaped masker, since upstream errors can echo whole response bodies. |
| `requiredPermission` | The scope that gated the call, recorded on allow and deny alike. |
| `details` | Per-action fields. Only keys the action declares survive; the rest are dropped and listed under `_droppedKeys`. |

Every action may also carry the shared detail keys from `audit/schema.go`: `envelope` (a coverage-tier record), `attemptEventId` (a `Begin` outcome record), `tool` (MCP surface), `suppressedCount` (rate-limited `authn:failure`), `reason` and `missingScope` (a request a module refused for a scope only it can check, such as the AI Gateway's environment tier) and `_droppedKeys`. `pathParams` and `repeatCount` are declared but not set by any emit site today.

### Action taxonomy

Actions read as `<resource>:<verb>`. Most are the route's `rbac.Permission` verbatim; where a permission does not describe the effect, `audit/policy.go` maps the route explicitly. The three recurring cases:

- One permission gating several operations — every API-key route is gated by `*:api-key-manage`, so create, rotate and revoke would otherwise be indistinguishable.
- A permission naming a different resource than the operation acts on — the user routes are gated by `org:invite-member` / `org:remove-member` but act on a user, so they record `user:invite`, `user:create`, `user:update` and `user:delete`.
- A permission broader than the effect — event destinations and subscriptions are gated by the permission of the scope they belong to (`events:manage` for the organization's), which says nothing about whether one was created, rotated or deleted.

A module's actions carry its name: `<module>:<name>`, where the name is the module's permission or a declared action, often `<resource>.<verb>`. The AI Gateway's read `ai-gateway:<resource>.<verb>` (`ai-gateway:api-keys.rotate`, `ai-gateway:gateways.set-identity-provider`); its routes declare them, and the manifest declares their detail keys (`modules/aigateway/gwaudit`). A module writes no records of its own: its handlers name what they acted on and add declared details to the route's record (`modkit.AuditResource`, `modkit.AuditDetail`), which also takes the project and environment from the route's path.

`requestPath` holds the **route pattern** on every record a route produced (see the one `authn:failure` exception above). Queries on it are exact, which is what makes deriving the human-readable action safe.

`actionClass` is one of `authn`, `authz`, `credential`, `identity`, `deployment`, `config`, `read`, `system`. `severity` runs 1 (info) to 4 (critical); credential and privilege changes are always 4.

### Severity 4 — the events worth alerting on

`authz:root-ou-bypass`, `authz:org-override`, and everything in `actionClass: credential` or `identity` with a mutating verb — notably `authn:login`, `role:*`, `group:*`, `user:*`, `agent-identity:*`, `agent-token:mint`, `workload-credential:*`, `workload-token:exchange`, `event-destination:create` / `:update` / `:delete` / `:rotate-secret`, `ai-gateway:api-keys.create` / `:rotate` / `:revoke`, `ai-gateway:gateway-tokens.rotate` / `:revoke`, `ai-gateway:agents.set-a2a-proxy` / `:remove-a2a-proxy`, `ai-gateway:agent-identities.*`, `ai-gateway:scopes.*`, and `ai-gateway:gateways.set-identity-provider` / `:remove-identity-provider` (changing which issuer a gateway trusts changes who can mint tokens it accepts). `ai-gateway:api-keys.issue-test` is the deliberate exception: console Try-It keys are short-lived, so they rank as a notice.

## What is never recorded

Two properties are **structural** rather than filtered, which is what makes the coverage tier safe to run on every route:

1. **Request and response bodies are never read.** The secrets that flow through this API — sign-in passwords, client secrets, upstream auth values, event-destination signing secrets, user-creation password attributes, and the API keys and tokens returned once on creation — are out of reach of a record, not merely redacted out of one.
2. **`requestPath` is the route pattern for matched requests.** Once a route has matched, path and query-string leakage is eliminated by construction. Edge `authn:failure` records precede route matching and carry the request's `URL.Path` after generic log sanitization, so concrete path segments (org, project, agent names) may remain — but the query string is never read.

On top of that, anything a caller attaches by hand passes an **allow-list keyed by action** (`audit/schema.go`). A key nobody declared is dropped and reported under `_droppedKeys`. This is deliberately not a deny-list: a deny-list fails on the field nobody thought of.

Where a secret must be referenced, `audit.SecretRef` stores a SHA-256 prefix plus the last four characters — enough to correlate "the key that was rotated" with "the key later used", never enough to use.

**URLs are reduced to scheme, host and path.** A URL is the one declared value that can hold a credential in its own syntax — RFC 3986 userinfo (`https://user:pass@idp.example/jwks`) or a token in the query. A detail declared `KindURL` has its userinfo, query and fragment removed before the record is written, and a `[redacted-components]` marker says something was there. This is enforced at redaction, not at the emit site, so it cannot be forgotten by the next caller; a test fails if a URL-valued detail is declared as anything else.

**The free-form attribute summary never records attribute values.** The user-creation API takes a free-form attribute map that is known to carry passwords. For `user:create`, `username` is copied explicitly into `details.username` and used as the resource id and name; the rest of the attribute map is represented only by its key names (`attributeKeys`), a count (`attributeCount`), and a flag when a key looks credential-shaped (`containsSensitiveKey`) — see `audit.AttributeKeySummary`.

**Authentication failures carry no token material** — only a classified reason (see [The authentication gap](#the-authentication-gap)).

Resource **names** are recorded deliberately, including secret names. A name is not a credential, and "which API key was revoked" is where an investigation starts.

## Looking things up

Records are newline-delimited JSON on stdout. These work with `jq` against a collected log file and translate directly to a SIEM query.

Who issued this agent API key?

```bash
jq 'select(.event.action=="ai-gateway:api-keys.create" and .event.details.keyName=="ci-key")
   | {who: .event.actorId, when: .event.occurredAt, agent: .event.details.ownerName, env: .event.environment}'
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

Docker keeps a container's stdout only as long as the container and its log driver allow, which is far shorter than the evidence period most audits require (SOC 2 evidence periods typically run 3–12 months; ISO 27001 A.5.33 requires retention per a defined policy). **Route `log_type=audit` records to a dedicated index or bucket with a retention period that matches your obligations**, ideally write-once storage.

This is the single most important thing to get right. Everything else here is already done for you.

### Tamper evidence

Agent Manager has no write path back into the log store, so the collected copy is one the service itself cannot rewrite. That is a genuine property and it is the basis for treating the pipeline copy as authoritative. It is **not** protection against someone with write access to the log store itself — for that, use write-once storage or forward to a SIEM that platform administrators do not also administer.

### Separation of duties

Anyone who can grant permissions can grant themselves any permission, and that grant is itself audited. No arrangement of in-product RBAC fixes this, because the administrators administer the RBAC. Genuine separation of duties requires shipping these records to a system that Agent Manager administrators do not control. This design enables that; it cannot substitute for it.

## The authentication gap

**With ThunderID as the identity provider, login, logout, failed login, MFA, password change and session revocation are not recorded here, because they do not happen here.** Authentication is performed by Thunder; Agent Manager only ever sees an already-minted JWT.

The exception is file-based login, where this service is the identity provider: `POST /auth/login` checks a username and password against the configured users file and mints the token itself. Every attempt that reaches the login service is recorded as `authn:login` — a success with the `username`, the `roles` the token went out with and its `expiresIn`, written synchronously before the token is returned (a sign-in that cannot be recorded is refused); a refusal with the `username` and a `reason` of `unknown-user` or `bad-password`, written asynchronously. The response never says which of the two it was; only the record does. A request rejected before the login service (a body that is not JSON, or an empty username or password, answered 400) is not recorded.

These are precisely the events SOC 2 CC6.1/CC6.6 and ISO 27001 A.8.15 expect. **With Thunder, a complete trail requires collecting Thunder's own audit events as well.** Any claim of completeness based on Agent Manager alone is wrong.

What this service does record, and which makes the two joinable:

- `actorTokenId` — the token's `jti`, on every record.
- `actorId` — the token subject.
- `authn:failure` — a token rejected at this service's edge, with a classified reason (`expired`, `bad-signature`, `bad-issuer`, `bad-audience`, `unknown-kid`, `malformed`, `missing-header`) and the source IP. Note this is "a bad token reached Agent Manager", which is **not** the same as "a login failed".

**To join:** correlate on `actorId` (subject) plus `actorTokenId` (`jti`) within the token's validity window. Thunder's record of issuing that `jti` to that subject links a login session to every action taken with it.

Authentication-failure records are rate-limited to 10 per source IP per minute, with the suppressed count carried on the next emitted record, so a token flood cannot become a storage-volume problem while still leaving the signal visible.

## Denials

Refusals are recorded on every surface, because a denied attempt is often more interesting than a successful one:

- **REST** — all five deny sites in `middleware/authorization.go`, with the specific missing scope and the caller's scope count, and an `X-Impersonate-Org` request the deployment does not allow (`middleware/apply_org_override.go`, reason `org-override-*`). A module route that refuses a request itself (the AI Gateway's environment tier, a body naming another organization) is recorded on the route's own record: outcome `deny`, with `reason` and, for a scope, `missingScope`.
- **MCP** — insufficient scope, an unknown tool (a probe), and an organization mismatch (a token driving a tool against a different org than its session — the clearest attack signal this surface produces).
- **Internal** — a missing or invalid gateway `api-key`, and a valid key presented for a *different* gateway, which is recorded separately because a valid credential used out of scope is a stronger signal than an invalid one. These are rate-limited per source, since gateways poll continuously and one with a stale key would otherwise emit forever.
- **Edge** — rejected tokens, with a classified reason and no token material.

## Enforcement posture is part of the trail

Authorization is always enforced: every audited route carries the `requiredPermission` that was checked, so a record names the decision that was made rather than the one that might have been.

Two ways past the ordinary check are recorded even when the request succeeds, alongside the route's own record: `authz:root-ou-bypass`, a token admitted through the root-OU bypass whatever scopes it carries, and `authz:org-override`, a request switched to another organization by `X-Impersonate-Org`. The second is filed under the target organization, with the caller's `clientId`, the token's `tokenOuID` and the `targetOuID`; it happens only on routes that allow it (`modkit.AllowOrgOverride`, today the AI Gateway's `GET /m/ai-gateway/orgs/{orgName}/gateways` and `PUT /m/ai-gateway/orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}`) and only when the deployment installs an `OrgOverrideResolver` (`app.Options`). The stock binary installs none, so it never emits one.

`system:startup` is recorded when the service starts, which bounds any gap in the trail to a restart — a reader can tell "nothing happened" apart from "the service was not running".

## Reliability

| Path | Behaviour on failure |
|---|---|
| Ordinary records (`audit.Record`) | Asynchronous, buffered. On overflow, dropped and counted — never blocks a request. A slow sink must not become an outage. |
| Security-critical operations (`audit.Begin`) | Synchronous. If the record cannot be written the operation is **refused**, because an untraceable privileged change is worse than a failed one. |

Drops are counted and logged to the application log, at most once every 30 seconds, with the running total. A trail that silently loses records is worse than one that admits it — which is why emitting `system:audit-dropped` into the trail itself is listed under [Not yet covered](#not-yet-covered).

**The honest limit of "fail-closed" here:** a successful write means the record reached the process's output, not that it reached durable storage. This catches the common failure — the process is running but its sink is broken — but it is not the atomic "change and record commit together" guarantee a same-database write would give. `audit.Begin` therefore writes an *intent* record before an external mutation and an *outcome* record after. A record left at `outcome: "unknown"` means the process died mid-operation; that orphan is deliberate forensic signal, not a defect.

**Where the intent record sits.** `audit.Begin` is placed immediately before the *commit point* — the first change — not at the top of the operation. In `DeleteAgent` and the project delete only read-only checks run before it (the project exists, no module refuses the delete, the project holds no agents), so a refused `Begin` leaves nothing changed and only the coverage-tier envelope records the attempt. On a fail-closed AI Gateway route (`modkit.AuditFailClosed`) the core writes the intent record before the handler runs at all.

This is a deliberate trade, not an oversight. Beginning at the top of an operation would mean every intermediate return path — and there are many, since each validates as it goes — has to resolve the attempt or leave a false `outcome: "unknown"` orphan. Orphans are the signal that the process died mid-operation; manufacturing them on ordinary validation failures would devalue the one thing they mean. The narrow placement keeps the orphan window small: an orphan means the process stopped between `Begin` and the result, so the change may or may not have been made. Its outcome is unknown, which is the condition worth investigating.

The buffer is flushed on shutdown **after both HTTP servers stop**, so in-flight requests finish recording first.

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `AUDIT_ENABLED` | `true` | Turn recording on. Off means no record of privileged operations. |
| `AUDIT_BUFFER_SIZE` | `4096` | Queued events before dropping. |
| `AUDIT_BATCH_SIZE` | `200` | Events written per sink call. |
| `AUDIT_FLUSH_INTERVAL_MS` | `1000` | Maximum time an event waits before being written. |

Docker Compose: the stack (`deployments/standalone/docker-compose.yml`) sets none of these, so the defaults apply. To change one, add it to the `environment` of the `amp-api` service.

## Adding a semantic event

Services and controllers emit through package-level functions that take only a context, so no constructor changes and no new dependencies are needed. Outside a request the calls are no-ops, which keeps unit tests free of audit wiring.

```go
// Ordinary: asynchronous, fail-open.
audit.Record(ctx, audit.ActionSystemAgentIdentityExhausted,
    audit.ResourceNamed(audit.ResourceAgentIdentity, agentName, agentName),
    audit.Project(projName),
    audit.Environment(envName),
    audit.Detail("reason", reason),
)

// Security-critical: refuse the operation if the record cannot be written.
attempt, err := audit.Begin(ctx, audit.ActionProjectDelete,
    audit.Org(ouID),
    audit.ResourceNamed("project", projectName, projectName),
    audit.Detail("projectName", projectName))
if err != nil {
    return err // do NOT perform the operation
}
err = s.platform.DeleteProject(ctx, ouID, projectName)
attempt.Complete(ctx, err)
```

Domain actions are declared in `audit/actions_domain.go`, which registers each one's class, severity and permitted detail keys together so the three cannot drift apart. A test fails if a registered action has no detail schema, so the decision cannot be skipped.

**Use the same action constant the coverage tier derives for the route.** `TestDomainActionsMatchRouteDerivedActions` fails the build if a semantic emit and its route disagree — otherwise "who deleted agent X" returns half the answer depending on which tier recorded it.

`audit.Detail` accepts only scalars and string slices. Refusing structs and maps is what keeps request payloads structurally unable to reach a record.

### Testing a service that emits

Operations that must not happen unrecorded refuse to proceed when no recorder is installed, so a bare `context.Background()` makes them fail by design. Tests exercising those paths use `auditableCtx(t)` (`services/audit_testing_test.go`), which installs a discarding recorder. To assert the refusal itself, pass a bare context and expect `audit.ErrRecorderUnavailable`.

## Semantic events

The operations below emit a record describing what actually changed, not just that the route was called. Everything else is covered by the coverage tier alone.

| Area | Actions | Mode |
|---|---|---|
| API keys | `ai-gateway:api-keys.create` / `:rotate` / `:revoke` (agent, LLM provider, LLM proxy) | Fail-closed |
| API keys | `ai-gateway:api-keys.issue-test` (console Try-It) | Fail-open |
| Gateway tokens | `ai-gateway:gateway-tokens.rotate` / `:revoke` | Fail-closed |
| Sign-in | `authn:login` (file-based login only; a refusal is fail-open, since it issues nothing) | Fail-closed |
| Agent tokens | `agent-token:mint` | Fail-closed |
| Module workloads | `workload-credential:issue`, `workload-token:exchange` | Fail-closed |
| Module workloads | `workload-credential:revoke` | Fail-open |
| Agent OAuth identity | `agent-identity:provision` / `:regenerate-secret` / `:revoke-secret` / `:retry-provisioning` | Fail-closed |
| Agent OAuth identity | `system:agent-identity-provisioned` / `system:agent-identity-exhausted` (reconciler, system actor) | Fail-open |
| Privilege | `role:grant-permission` / `:revoke-permission` / `role:assign` / `:unassign` | Fail-closed |
| Membership | `group:add-member` / `:remove-member` | Fail-closed |
| Users | `user:invite` / `:create` / `:delete` | Fail-closed |
| Agent lifecycle | `agent:delete`, `project:delete` | Fail-closed |
| Event destinations | `event-destination:create` / `:update` / `:delete` / `:rotate-secret` | Fail-closed |
| Event subscriptions | `event-subscription:create` / `:update` / `:delete` (organization, project and agent scope) | Fail-closed |
| Gateways | `ai-gateway:gateways.delete`, `ai-gateway:gateways.set-identity-provider`, `ai-gateway:gateways.remove-identity-provider` | Fail-closed |
| Gateways | `ai-gateway:gateways.create` / `:update` / `:assign-environment` / `:unassign-environment` | Fail-open |
| Internal | `ai-gateway:gateways.push-manifest`, `ai-gateway:api-keys.sync` (bulk sync, coalesced per gateway) | Fail-open |
| Agent config | `ai-gateway:agent-configs.update` / `:delete` | Fail-open |
| A2A card source | `ai-gateway:agents.set-card-source` | Fail-open |
| A2A card source | `ai-gateway:agents.remove-card-source` (it removes the proxy and its stored secret too) | Fail-closed |
| A2A proxy | `ai-gateway:agents.set-a2a-proxy` / `ai-gateway:agents.remove-a2a-proxy` | Fail-closed |
| Per-config API keys | `ai-gateway:api-keys.create` / `:rotate` / `:revoke` (model-config, mcp-config) | Fail-closed |

Three of these carry detail worth calling out:

- **Removing the card source of a proxied environment discards the stored upstream secret too**, so `ai-gateway:agents.remove-card-source` is fail-closed and carries the proxy's `upstreamAuthHeader` and `upstreamAuthChanged`.
- **AI Gateway fail-closed routes are marked on the route** (`modkit.AuditFailClosed`): the core writes the intent record before the handler runs and answers 503 when it cannot; the route's record is the outcome record.
- **`role:grant-permission` records the granted scopes in full.** Scope strings are identifiers, not secrets, and "alice granted SRE deploy-production" is the question the event exists to answer.

## Action reference

Every explicitly registered action this build can record — the class and severity that drive alerting, and the detail keys each is **permitted** to carry beyond the shared ones. A key not listed is dropped before the record is written. A permitted key is not necessarily set: the [sample records](#sample-records) below show what each emit site actually writes. Coverage-tier actions derived from a route's permission and not listed here are classified by `classify` in `audit/actions.go`.

The registries are the source of truth: `audit/actions.go` and `audit/actions_domain.go` for the core (`audit.RegisteredActions()` enumerates them), and the AI Gateway manifest's `gwaudit.Actions` for the module.

Core actions (`audit/actions.go`, `audit/actions_domain.go`, plus the shared keys in `audit/schema.go`):

| Action | Class | Severity | Detail keys |
|---|---|---|---|
| `agent-identity:provision` | credential | 4 critical | `agentName`, `alreadyExisted`, `clientId`, `environment` |
| `agent-identity:regenerate-secret` | credential | 4 critical | `agentName`, `clientId`, `environment` |
| `agent-identity:retry-provisioning` | credential | 4 critical | `agentName`, `environment` |
| `agent-identity:revoke-secret` | credential | 4 critical | `agentName`, `clientId`, `environment` |
| `agent-token:mint` | credential | 4 critical | `agentName`, `expiresIn` |
| `agent:create` | config | 2 notice | `agentName`, `agentType`, `tool` |
| `agent:delete` | config | 3 warning | `agentName` |
| `agent:read` | read | 1 info | — |
| `authn:failure` | authn | 3 warning | `authHeader`, `reason` |
| `authn:login` | credential | 4 critical | `expiresIn`, `reason`, `roles`, `username` |
| `authz:deny` | authz | 3 warning | `grantedScopes`, `missingScope`, `reason`, `requestedOrgId` |
| `authz:org-override` | authz | 4 critical | `clientId`, `targetOuID`, `tokenOuID` |
| `authz:root-ou-bypass` | authz | 4 critical | `rootOUBypass` |
| `environment:read` | read | 1 info | — |
| `event-destination:create` | credential | 4 critical | `availability`, `destination`, `kind`, `projects` |
| `event-destination:delete` | credential | 4 critical | `availability`, `destination`, `kind`, `projects` |
| `event-destination:rotate-secret` | credential | 4 critical | `availability`, `destination`, `kind`, `projects` |
| `event-destination:test` | config | 2 notice | — |
| `event-destination:update` | credential | 4 critical | `availability`, `destination`, `kind`, `projects` |
| `event-subscription:create` | config | 2 notice | `allEvents`, `destinationId`, `enabled`, `environments`, `eventCount`, `scope` |
| `event-subscription:delete` | config | 2 notice | `allEvents`, `destinationId`, `enabled`, `environments`, `eventCount`, `scope` |
| `event-subscription:update` | config | 2 notice | `allEvents`, `destinationId`, `enabled`, `environments`, `eventCount`, `scope` |
| `group:add-member` | identity | 4 critical | `groupName`, `memberCount`, `memberTypes`, `members` |
| `group:remove-member` | identity | 4 critical | `groupName`, `memberCount`, `memberTypes`, `members` |
| `project:create` | config | 2 notice | `projectName`, `tool` |
| `project:delete` | config | 3 warning | `projectName` |
| `project:read` | read | 1 info | — |
| `role:assign` | identity | 4 critical | `assigneeCount`, `assigneeTypes`, `assignees`, `roleName` |
| `role:grant-permission` | identity | 4 critical | `permissionCount`, `permissions`, `resourceServerId`, `roleName` |
| `role:revoke-permission` | identity | 4 critical | `permissionCount`, `permissions`, `resourceServerId`, `roleName` |
| `role:unassign` | identity | 4 critical | `assigneeCount`, `assigneeTypes`, `assignees`, `roleName` |
| `system:agent-identity-exhausted` | credential | 3 warning | `agentName`, `environment`, `reason` |
| `system:agent-identity-provisioned` | credential | 2 notice | `agentName`, `clientId`, `environment` |
| `system:audit-dropped` | system | 4 critical | `droppedTotal`, `sink` |
| `system:startup` | system | 2 notice | `instance`, `sinks`, `version` |
| `user:create` | identity | 4 critical | `attributeCount`, `attributeKeys`, `containsSensitiveKey`, `email`, `groups`, `userType`, `username` |
| `user:delete` | identity | 4 critical | `username` |
| `user:invite` | identity | 4 critical | `attributeCount`, `attributeKeys`, `containsSensitiveKey`, `email`, `groups`, `userType`, `username` |
| `user:update` | identity | 4 critical | `attributeCount`, `attributeKeys`, `containsSensitiveKey`, `email`, `groups`, `userType`, `username` |
| `workload-credential:issue` | credential | 4 critical | `module`, `workload`, `subject` |
| `workload-credential:revoke` | credential | 4 critical | `module` |
| `workload-token:exchange` | credential | 4 critical | `module`, `workload`, `instance`, `credentialId`, `expiresIn`, `reason` |

AI Gateway actions (`modules/aigateway/gwaudit/gwaudit.go`), all 66 the module's manifest declares. A dash means the action permits only the shared keys:

| Action | Class | Severity | Detail keys |
|---|---|---|---|
| `ai-gateway:agent-configs.delete` | config | 2 notice | `agentName`, `configName`, `configType`, `environmentCount` |
| `ai-gateway:agent-configs.update` | config | 1 info | `agentName`, `configName`, `configType`, `updatedFields` |
| `ai-gateway:agent-identities.add-group-member` | credential | 4 critical | — |
| `ai-gateway:agent-identities.assign-role` | credential | 4 critical | — |
| `ai-gateway:agent-identities.create` | credential | 4 critical | — |
| `ai-gateway:agent-identities.delete` | credential | 4 critical | — |
| `ai-gateway:agent-identities.remove-group-member` | credential | 4 critical | — |
| `ai-gateway:agent-identities.unassign-role` | credential | 4 critical | — |
| `ai-gateway:agent-identities.update` | credential | 4 critical | — |
| `ai-gateway:agents.refresh-a2a-proxy` | config | 1 info | — |
| `ai-gateway:agents.refresh-card` | config | 1 info | — |
| `ai-gateway:agents.remove-a2a-proxy` | credential | 4 critical | — |
| `ai-gateway:agents.remove-card-source` | config | 2 notice | `agentName`, `upstreamAuthChanged`, `upstreamAuthHeader` |
| `ai-gateway:agents.set-a2a-proxy` | credential | 4 critical | `agentName`, `upstreamAuthChanged`, `upstreamAuthHeader` |
| `ai-gateway:agents.set-card-source` | config | 1 info | `agentName`, `sourceUrl` |
| `ai-gateway:agents.update-deploy-settings` | config | 1 info | — |
| `ai-gateway:api-keys.create` | credential | 4 critical | `expiresAt`, `gatewayConnected`, `gatewayCount`, `keyName`, `ownerName`, `ownerType` |
| `ai-gateway:api-keys.issue-test` | credential | 2 notice | `expiresAt`, `keyName`, `ownerName`, `ownerType`, `rotated` |
| `ai-gateway:api-keys.list` | read | 2 notice | — |
| `ai-gateway:api-keys.revoke` | credential | 4 critical | `gatewayCount`, `keyName`, `ownerName`, `ownerType` |
| `ai-gateway:api-keys.rotate` | credential | 4 critical | `expiresAt`, `gatewayConnected`, `gatewayCount`, `keyName`, `ownerName`, `ownerType` |
| `ai-gateway:api-keys.sync` | read | 2 notice | `keyCount` |
| `ai-gateway:artifacts.check-exists` | read | 1 info | — |
| `ai-gateway:deployments.read` | read | 1 info | — |
| `ai-gateway:deployments.sync` | read | 2 notice | — |
| `ai-gateway:environments.list-identity-providers` | read | 1 info | — |
| `ai-gateway:gateway-tokens.list` | read | 2 notice | — |
| `ai-gateway:gateway-tokens.revoke` | credential | 4 critical | `gatewayName`, `tokenId` |
| `ai-gateway:gateway-tokens.rotate` | credential | 4 critical | `expiresAt`, `gatewayName`, `requestedOrgId`, `tokenId` |
| `ai-gateway:gateways.assign-environment` | config | 2 notice | `environment`, `gatewayName`, `gatewayType`, `requestedOrgId`, `vhost` |
| `ai-gateway:gateways.create` | config | 2 notice | `environment`, `gatewayName`, `gatewayType`, `requestedOrgId`, `vhost` |
| `ai-gateway:gateways.delete` | config | 3 warning | `environment`, `gatewayName`, `gatewayType`, `requestedOrgId`, `vhost` |
| `ai-gateway:gateways.list-identity-providers` | read | 1 info | — |
| `ai-gateway:gateways.push-manifest` | config | 1 info | `policyCount` |
| `ai-gateway:gateways.read` | read | 1 info | — |
| `ai-gateway:gateways.remove-identity-provider` | credential | 4 critical | `gatewayName`, `identityProviderName`, `issuer`, `jwksUri`, `skipTlsVerify` |
| `ai-gateway:gateways.set-identity-provider` | credential | 4 critical | `gatewayName`, `identityProviderName`, `issuer`, `jwksUri`, `skipTlsVerify` |
| `ai-gateway:gateways.unassign-environment` | config | 2 notice | `environment`, `gatewayName`, `gatewayType`, `requestedOrgId`, `vhost` |
| `ai-gateway:gateways.update` | config | 2 notice | `environment`, `gatewayName`, `gatewayType`, `requestedOrgId`, `vhost` |
| `ai-gateway:llm-provider-templates.create` | config | 1 info | — |
| `ai-gateway:llm-provider-templates.delete` | config | 2 notice | — |
| `ai-gateway:llm-provider-templates.update` | config | 1 info | — |
| `ai-gateway:llm-providers.create` | config | 1 info | — |
| `ai-gateway:llm-providers.delete` | config | 2 notice | — |
| `ai-gateway:llm-providers.read` | read | 1 info | — |
| `ai-gateway:llm-providers.restore` | deployment | 2 notice | — |
| `ai-gateway:llm-providers.update` | config | 1 info | — |
| `ai-gateway:llm-providers.update-catalog` | config | 1 info | — |
| `ai-gateway:llm-proxies.create` | config | 1 info | — |
| `ai-gateway:llm-proxies.delete` | config | 2 notice | — |
| `ai-gateway:llm-proxies.delete-deployment` | config | 1 info | — |
| `ai-gateway:llm-proxies.deploy` | deployment | 2 notice | — |
| `ai-gateway:llm-proxies.restore` | deployment | 2 notice | — |
| `ai-gateway:llm-proxies.undeploy` | deployment | 2 notice | — |
| `ai-gateway:llm-proxies.update` | config | 1 info | — |
| `ai-gateway:mcp-servers.create` | config | 1 info | — |
| `ai-gateway:mcp-servers.delete` | config | 2 notice | — |
| `ai-gateway:mcp-servers.fetch-server-info` | read | 1 info | — |
| `ai-gateway:mcp-servers.read` | read | 1 info | — |
| `ai-gateway:mcp-servers.update` | config | 1 info | — |
| `ai-gateway:scopes.create` | identity | 4 critical | — |
| `ai-gateway:scopes.delete` | identity | 4 critical | — |
| `ai-gateway:scopes.update` | identity | 4 critical | — |

Core read actions (`agent:read`, `project:read`, `environment:read`) are declared so every MCP tool can name what it does; they are classified as reads and not recorded. An AI Gateway read action is recorded when a route names it with `modkit.Audit` — the API-key and gateway-token listings, the identity-provider listings, `gateways.read` on the platform failure summary and the gateway sync routes (`api-keys.sync`, `deployments.sync`, `artifacts.check-exists`). `deployments.read`, `llm-providers.read` and `mcp-servers.read` name MCP read tools and are not recorded.

### Sample records

One record per action, produced by the audit emitter on `main` — `audit.Begin`/`Complete`, `Record`, `RecordSync`, `RecordAncillary`, the coverage envelope and `AuthFailureRecorder` — called with the same options, route pattern and permission as the production emit site named beside it, then serialized by the stdout sink's JSON handler. Each was checked for: class and severity matching the registry, no `_droppedKeys`, and for `Begin` actions an intent record with outcome `unknown` whose `eventId` the outcome record carries as `attemptEventId`. Only the outcome record is shown. Identifiers, names and timestamps are illustrative; every key and the shape of every value is real.

The `ai-gateway:*` examples were captured before the gateway became a module. The records keep their keys, but they are now the route's own record: the handler or service named beside each adds the resource and details with `modkit.AuditResource` and `modkit.AuditDetail`, and for a fail-closed route the core writes the intent record before the handler runs. As the route's record, each also carries `statusCode`, `durationMs` and `details.envelope`, which the samples predate.

Not shown: the three core read actions, which are never recorded; `system:audit-dropped`, which nothing emits yet; and the actions with no captured sample: `authn:login`, `authz:org-override`, `workload-credential:*`, `workload-token:exchange`, `event-destination:*`, `event-subscription:*`, and the AI Gateway actions not listed below. Their permitted detail keys are in the [action reference](#action-reference).

Where two emit sites share an action, the sample shows the first listed; the others differ as noted.

<details>
<summary><code>ai-gateway:agent-configs.update</code> — Route record · `modules/aigateway/gwsvc/agent_configuration_service.go:2785`</summary>

Model configs only; MCP-config updates produce the coverage envelope alone.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:agent-configs.update",
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
    "requestPath": "/m/ai-gateway/agents/{agentName}/model-configs/{configId}",
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
<summary><code>ai-gateway:agent-configs.delete</code> — Route record · `modules/aigateway/gwsvc/agent_configuration_service.go:2957`</summary>

Model configs only; MCP-config deletes produce the coverage envelope alone.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:agent-configs.delete",
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
    "requestPath": "/m/ai-gateway/agents/{agentName}/model-configs/{configId}",
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
<summary><code>agent-identity:provision</code> — Begin + Complete · `controllers/agent_controller.go:592`</summary>

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
    "requestPath": "/agents/{agentName}/identities",
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
<summary><code>agent-identity:regenerate-secret</code> — Begin + Complete · `controllers/agent_controller.go:485`</summary>

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
    "requestPath": "/agents/{agentName}/identities",
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
<summary><code>agent-identity:retry-provisioning</code> — Begin + Complete · `controllers/agent_controller.go:658`</summary>

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
    "requestPath": "/agents/{agentName}/identities/retry",
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
<summary><code>agent-identity:revoke-secret</code> — Begin + Complete · `controllers/agent_controller.go:537`</summary>

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
    "requestPath": "/agents/{agentName}/identities",
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
<summary><code>agent-token:mint</code> — RecordSync · `services/agent_token_manager.go:367`</summary>

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
    "requestPath": "/agents/{agentName}/token",
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
<summary><code>agent:create</code> — Record · `mcp/tools/helpers.go:181`</summary>

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
<summary><code>agent:delete</code> — Begin + Complete · `services/agent_manager.go:513`</summary>

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
    "requestPath": "/agents/{agentName}",
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
<summary><code>ai-gateway:api-keys.create</code> — Fail-closed route record · `modules/aigateway/gwsvc/agent_apikey_service.go:187`</summary>

Agent key. Model-config and MCP-config keys (`modules/aigateway/gwctl/agent_configuration_controller.go`) use `ownerType` `model-config` / `mcp-config` and carry no `gatewayCount`; LLM-provider and LLM-proxy keys carry no project or environment.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:api-keys.create",
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
    "requestPath": "/m/ai-gateway/agents/{agentName}/api-keys",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "api-key",
    "resourceId": "art-71c2",
    "resourceName": "ci-key",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "requiredPermission": "amp:ai-gateway:agent-api-keys.manage",
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
<summary><code>ai-gateway:api-keys.issue-test</code> — Route record · `modules/aigateway/gwsvc/agent_apikey_service.go:349`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:api-keys.issue-test",
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
    "requestPath": "/m/ai-gateway/agents/{agentName}/api-keys/test",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "api-key",
    "resourceId": "art-71c2",
    "resourceName": "test-key-8c1f04b2",
    "projectName": "payments",
    "environment": "development",
    "outcome": "success",
    "requiredPermission": "amp:ai-gateway:agent-api-keys.manage",
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
<summary><code>ai-gateway:api-keys.revoke</code> — Fail-closed route record · `modules/aigateway/gwsvc/agent_apikey_service.go:230`</summary>

Agent key; the other owners differ as for `ai-gateway:api-keys.create`.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:api-keys.revoke",
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
    "requestPath": "/m/ai-gateway/agents/{agentName}/api-keys/{keyName}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "api-key",
    "resourceId": "art-71c2",
    "resourceName": "ci-key",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "requiredPermission": "amp:ai-gateway:agent-api-keys.manage",
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
<summary><code>ai-gateway:api-keys.rotate</code> — Fail-closed route record · `modules/aigateway/gwsvc/agent_apikey_service.go:258`</summary>

Agent key; the other owners differ as for `ai-gateway:api-keys.create`.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:api-keys.rotate",
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
    "requestPath": "/m/ai-gateway/agents/{agentName}/api-keys/{keyName}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "api-key",
    "resourceId": "art-71c2",
    "resourceName": "ci-key",
    "projectName": "payments",
    "environment": "production",
    "outcome": "success",
    "requiredPermission": "amp:ai-gateway:agent-api-keys.manage",
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
<summary><code>ai-gateway:api-keys.sync</code> — coverage envelope · `middleware/audit_route.go:131 (coverage envelope)`</summary>

No semantic emit: the coverage envelope for the gateway bulk-sync routes, coalesced to one record per gateway per window.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "durationMs": 42,
    "action": "ai-gateway:api-keys.sync",
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
<summary><code>authn:failure</code> — Record · `audit/authn.go:112`</summary>

REST edge. The internal gateway server records its own (`modules/aigateway/gwctl/gateway_internal_audit.go`) with surface `internal`, actor `gateway` and auth method `api-key`.

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
<summary><code>authz:deny</code> — Record · `middleware/authorization.go:53`</summary>

REST scope denial. MCP denials (`mcp/tools/authz.go`) carry `tool` and no `grantedScopes`; a refused `X-Impersonate-Org` request carries a reason of `org-override-not-permitted`, `org-override-target-not-found` or `org-override-path-mismatch`. An AI Gateway environment-tier refusal is not an `authz:deny`: it is the route's own record, with outcome `deny`, reason `missing-environment-tier-scope` and the `missingScope`.

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
<summary><code>authz:root-ou-bypass</code> — RecordAncillary · `middleware/authorization.go:179`</summary>

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
    "requestPath": "/orgs/{orgName}/identities/roles/{roleID}/permissions/add",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "outcome": "success",
    "requiredPermission": "amp:role:update",
    "details": {
      "rootOUBypass": true
    }
  }
}
```

</details>

<details>
<summary><code>ai-gateway:gateway-tokens.revoke</code> — Fail-closed route record · `modules/aigateway/gwctl/gateway_controller.go:734`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:gateway-tokens.revoke",
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
    "requestPath": "/m/ai-gateway/orgs/{orgName}/gateways/{gatewayID}/tokens/{tokenID}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "outcome": "success",
    "requiredPermission": "amp:ai-gateway:gateways.token-manage",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "tokenId": "tok-1f8b"
    }
  }
}
```

</details>

<details>
<summary><code>ai-gateway:gateway-tokens.rotate</code> — Fail-closed route record · `modules/aigateway/gwctl/gateway_controller.go:702`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:gateway-tokens.rotate",
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
    "requestPath": "/m/ai-gateway/orgs/{orgName}/gateways/{gatewayID}/tokens",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "outcome": "success",
    "requiredPermission": "amp:ai-gateway:gateways.token-manage",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "tokenId": "tok-4a2d"
    }
  }
}
```

</details>

<details>
<summary><code>ai-gateway:gateways.assign-environment</code> — Route record · `modules/aigateway/gwctl/gateway_controller.go:502`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:gateways.assign-environment",
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
    "requestPath": "/m/ai-gateway/orgs/{orgName}/gateways/{gatewayID}/environments/{envID}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "environment": "e7a1c3d2-4b5f-4e6a-9c8d-1f2e3a4b5c6d",
    "outcome": "success",
    "requiredPermission": "amp:ai-gateway:gateways.update",
    "details": {
      "environment": "e7a1c3d2-4b5f-4e6a-9c8d-1f2e3a4b5c6d"
    }
  }
}
```

</details>

<details>
<summary><code>ai-gateway:gateways.create</code> — Route record · `modules/aigateway/gwctl/gateway_controller.go:286`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:gateways.create",
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
    "requestPath": "/m/ai-gateway/orgs/{orgName}/gateways",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "resourceName": "edge-eu",
    "outcome": "success",
    "requiredPermission": "amp:ai-gateway:gateways.create",
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
<summary><code>ai-gateway:gateways.delete</code> — Fail-closed route record · `modules/aigateway/gwctl/gateway_controller.go:468`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:gateways.delete",
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
    "requestPath": "/m/ai-gateway/orgs/{orgName}/gateways/{gatewayID}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "outcome": "success",
    "requiredPermission": "amp:ai-gateway:gateways.delete",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35"
    }
  }
}
```

</details>

<details>
<summary><code>ai-gateway:gateways.push-manifest</code> — Route record · `modules/aigateway/gwctl/gateway_internal_controller.go:565`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:gateways.push-manifest",
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
<summary><code>ai-gateway:gateways.remove-identity-provider</code> — Fail-closed route record · `modules/aigateway/gwctl/gateway_identity_provider_controller.go:189`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:gateways.remove-identity-provider",
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
    "requestPath": "/m/ai-gateway/orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "resourceName": "gw-3c9e",
    "outcome": "success",
    "requiredPermission": "amp:ai-gateway:gateways.update",
    "details": {
      "attemptEventId": "3f2c9a1e-5b7d-4e8f-a0c6-1d9b2e4f7a35",
      "identityProviderName": "corp-idp"
    }
  }
}
```

</details>

<details>
<summary><code>ai-gateway:gateways.set-identity-provider</code> — Fail-closed route record · `modules/aigateway/gwctl/gateway_identity_provider_controller.go:159`</summary>

`jwksUri` was supplied as `https://svc:s3cret@idp.example.com/oauth2/jwks?key=abc`; userinfo and query are stripped at write time.

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:gateways.set-identity-provider",
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
    "requestPath": "/m/ai-gateway/orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "resourceName": "gw-3c9e",
    "outcome": "success",
    "requiredPermission": "amp:ai-gateway:gateways.update",
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
<summary><code>ai-gateway:gateways.unassign-environment</code> — Route record · `modules/aigateway/gwctl/gateway_controller.go:566`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:gateways.unassign-environment",
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
    "requestPath": "/m/ai-gateway/orgs/{orgName}/gateways/{gatewayID}/environments/{envID}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "environment": "e7a1c3d2-4b5f-4e6a-9c8d-1f2e3a4b5c6d",
    "outcome": "success",
    "requiredPermission": "amp:ai-gateway:gateways.update",
    "details": {
      "environment": "e7a1c3d2-4b5f-4e6a-9c8d-1f2e3a4b5c6d"
    }
  }
}
```

</details>

<details>
<summary><code>ai-gateway:gateways.update</code> — Route record · `modules/aigateway/gwctl/gateway_controller.go:448`</summary>

```json
{
  "log_type": "audit",
  "event": {
    "eventId": "87b3970f-c1e4-4df4-befd-9dc02834dcfc",
    "schemaVersion": 1,
    "occurredAt": "2026-09-25T08:13:37Z",
    "action": "ai-gateway:gateways.update",
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
    "requestPath": "/m/ai-gateway/orgs/{orgName}/gateways/{gatewayID}",
    "ouId": "ou-9f2",
    "orgHandle": "acme",
    "resourceType": "gateway",
    "resourceId": "gw-3c9e",
    "outcome": "success",
    "requiredPermission": "amp:ai-gateway:gateways.update"
  }
}
```

</details>

<details>
<summary><code>group:add-member</code> — Begin + Complete · `controllers/identity_controller.go:717`</summary>

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
<summary><code>group:remove-member</code> — Begin + Complete · `controllers/identity_controller.go:781`</summary>

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
<summary><code>project:create</code> — Record · `mcp/tools/helpers.go:181`</summary>

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
<summary><code>project:delete</code> — Begin + Complete · `services/infra_resource_manager.go:183`</summary>

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
    "requestPath": "/projects/{projName}",
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
<summary><code>role:assign</code> — Begin + Complete · `controllers/identity_controller.go:1292`</summary>

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
<summary><code>role:grant-permission</code> — Begin + Complete · `controllers/identity_controller.go:1177`</summary>

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
        "amp:ai-gateway:deploy.production"
      ],
      "resourceServerId": "rs-amp",
      "roleName": "sre"
    }
  }
}
```

</details>

<details>
<summary><code>role:revoke-permission</code> — Begin + Complete · `controllers/identity_controller.go:1236`</summary>

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
        "amp:ai-gateway:deploy.production"
      ],
      "resourceServerId": "rs-amp",
      "roleName": "sre"
    }
  }
}
```

</details>

<details>
<summary><code>role:unassign</code> — Begin + Complete · `controllers/identity_controller.go:1348`</summary>

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
<summary><code>system:agent-identity-exhausted</code> — Record · `services/agent_thunder_provisioning_service.go:790`</summary>

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
<summary><code>system:agent-identity-provisioned</code> — Record · `services/agent_thunder_provisioning_service.go:733`</summary>

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
<summary><code>system:startup</code> — RecordAncillary · `app/app.go:436`</summary>

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
<summary><code>user:create</code> — Begin + Complete · `controllers/identity_controller.go:199`</summary>

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
<summary><code>user:delete</code> — Begin + Complete · `controllers/identity_controller.go:309`</summary>

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
<summary><code>user:invite</code> — Begin + Complete · `controllers/identity_controller.go:437`</summary>

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
<summary><code>user:update</code> — coverage envelope · `middleware/audit_route.go:131 (coverage envelope)`</summary>

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

Semantic-event coverage is incomplete, not absent, in these places. Each is a real gap in coverage today:

- **Thunder's own events** — with ThunderID as the identity provider, see the authentication gap above. This is the one gap that cannot be closed from this repository.
- **`system:audit-dropped`** — registered, but the recorder only logs drops to the application log; no record reaches the trail.
