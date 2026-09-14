# A2A Agent Support — Milestone 1 Design

**Status:** draft, pending review
**Date:** 2026-09-09

## Goal

Let agent-manager provision agents that speak the A2A protocol, and expose them
through the API gateway as first-class `Agent` resources.

Milestone 1 delivers one thing end to end: an agent whose subtype is
`a2a-agent` is provisioned with the existing agent component type, does **not**
get a REST API, and is published to the gateway as a `kind: Agent` resource
carrying both A2A transports, the platform's existing policy chain, and a
rewritten passthrough Agent Card.

## Background

### What the gateway already provides

The gateway (`api-platform`, branch `agent-cp-events` — `a2a` plus one
commit) accepts a `kind: Agent` resource
under `gateway.api-platform.wso2.com/v1`. The parts that matter here:

- `spec.a2a.operationConfigs.transports` — ordered protocol bindings and their
  gateway-facing path prefixes. Only `JSONRPC` and `HTTP+JSON` exist; there is
  no gRPC support.
- `spec.a2a.operationConfigs.policies` — an ordered policy chain applied to
  every A2A operation, with optional per-operation overrides keyed by the
  eleven canonical A2A 1.0 operation names.
- `spec.a2a.agentCard.public` — `mode` is `passthrough` or `managed`.
- `spec.a2a.agentCard.protected` — the authenticated `GetExtendedAgentCard`
  representation. Optional; omitting it reads as `passthrough`, not as
  "unprotected".

Two gateway behaviours drive most of this design and are easy to get wrong,
because the published schema reference lags the implementation on the `a2a`
branch. Both are pinned by the gateway's own integration tests, which are the
authority used here:

**Passthrough cards are URL-rewritten by default.**
`gateway/it/features/agent_card.feature:305` — *"The flag has to be written
out: rewriting is the default, because a proxied card advertises the agent's
own address and would send every client past the gateway."* The scenario at
`:361` deploys an Agent with **no** `agentCard` block and asserts the served
card contains the gateway URL and does **not** contain the upstream's own
address. `rewriteUrls: false` opts out; the validator rejects the flag on
managed cards entirely (`pkg/config/agent_validator.go:644`).

The published `schemas.md` still says the gateway "does not parse, transform,
or sign a proxied card". That statement is stale. Prefer the feature files.

**The public card route sits outside the agent-wide policy scope.**
`agent_card.feature:438` — *"because the public card policies are its own
scope — the Agent-wide auth policy does not apply to it. Discovery has to stay
reachable for a client to learn how to authenticate at all."* So enabling
authentication never hides the public Agent Card.

**The extended card fails closed, unconditionally.** The gateway returns 401
for `GetExtendedAgentCard` unless a policy *in the Agent's own chain*
authenticated the request. This applies in every mode and is not configurable.
An Agent with no auth policy therefore cannot publish an extended card at all.

### What agent-manager already provides

- `utils.AgentSubType` is `chat-api` | `custom-api` (`utils/agent_types.go:38`).
- `TraitAPIManagement` ("api-configuration") provisions the RestApi CRD. It is
  attached at create (`services/agent_manager.go:504`) and re-attached at
  deploy (`:3249`), both gated on `isAPIAgent`.
- MCP proxies are published to the gateway without any trait: build a YAML,
  write a `deployments` row, broadcast an event
  (`services/mcp_proxy_deployment.go`, `services/gateway_events_service.go:137`).
  This is the pattern M1 follows.
- `buildPolicies()` (`services/agent_manager.go:3614`) emits
  `{name, version, params}` maps — structurally identical to the gateway's
  `Policy` schema, with matching policy names (`cors`, `api-key-auth`,
  `jwt-auth`). No translation layer is needed.
- Agent API keys are keyed on `models.KindAgent` artifact rows
  (`services/agent_apikey_service.go:140,299`) and served to the gateway at
  `GET /api/internal/v1/apis/api-keys`
  (`controllers/gateway_internal_controller.go:243`). Those rows are created
  per (project, agent, environment) by `ensureAgentEnvAPIArtifact`
  (`services/agent_api_artifact.go`).

## Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Agent type | New **subtype** `a2a-agent` under the existing `agent-api` type | Component provisioning is unchanged; only the gateway-facing surface differs. |
| REST API | `api-configuration` trait not attached | An A2A agent has no REST API to manage. |
| Gateway publication | agent-manager emits the `Agent` resource directly, over both control-plane channels | Mirrors MCP exactly; avoids an OpenChoreo template change on the critical path. The gateway side already supports both channels. |
| Transports | Both, at fixed platform-chosen prefixes | Covers both client styles with no new API surface in M1. |
| Agent card | Default passthrough with URL rewriting — omit the `agentCard` block | The gateway default is already correct. The agent owns the card's substance; the gateway owns only the addressing. |
| Card content input | None at create time | The agent serves its own card; agent-manager stores no copy. |
| Auth chain | Exactly the policies chat/custom agents get today — on or off, nothing new | Keeps M1 free of unverified gateway dependencies. Optional auth and `backend-jwt` deferred. |
| Upstream | Read from `ReleaseBinding.Status.Endpoints[].ServiceURL` | OpenChoreo already publishes the workload's in-cluster address; deriving it a second time would duplicate a convention this repo doesn't own. |

## Design

### 1. The subtype

`AgentSubTypeA2A AgentSubType = "a2a-agent"` joins the existing two in
`utils/agent_types.go`. It is a subtype, so `AgentType` stays `agent-api` and
`getOpenChoreoComponentType` (`clients/openchoreosvc/client/components.go:301`)
is untouched — the component is still provisioned as `proxy/agent-api`.

Gates that change:

| Site | Today | Change |
|---|---|---|
| `utils/utils.go:544` | rejects any subtype but the two | accept `a2a-agent` |
| `utils/utils.go:794` | `custom-api` ⇒ OpenAPI required | unchanged — `a2a-agent` requires no schema |
| `components.go:495,511` | endpoint built per chat vs custom | new branch: `a2a-agent` takes a port, no schema path |
| `agent_manager.go:473,3228` | `isAPIAgent` ⇒ attach `api-configuration` | narrow to `isAPIAgent && subtype != a2a-agent` |
| `agent_manager.go:3697` | always writes a `TraitAPIManagement` env-config key | gate on the same condition |

That last row is a trap worth stating plainly: `buildTraitEnvConfigs`
unconditionally writes a `traitEnvironmentConfigs` entry keyed by
`instanceName(TraitAPIManagement)`. OpenChoreo resolves those keys against
*attached* trait instances. Detaching the trait without removing the key leaves
the release binding carrying config for a trait that is not there.

Surfaces to extend: the OpenAPI subtype enum and its generated spec models, the
CLI (`cli/pkg/cmd/agent/create` — flags, template, validation, request
building), and the console's agent-creation flow.

### 2. The gateway Agent resource

New `services/a2a_agent_deployment.go`, a structural sibling of
`services/mcp_proxy_deployment.go`: build YAML → write a `deployments` row →
broadcast.

```yaml
apiVersion: gateway.api-platform.wso2.com/v1
kind: Agent
metadata:
  name: <sanitized per-environment artifact name>
spec:
  displayName: <agent display name>
  version: v1.0
  context: /<agentName>
  vhost: <gateway.Vhost for the environment>
  upstream:
    url: <from ReleaseBinding status — see below>
  a2a:
    protocolVersion: "1.0"
    operationConfigs:
      transports:
        - { protocolBinding: JSONRPC,   pathPrefix: /rpc }
        - { protocolBinding: HTTP+JSON, pathPrefix: /rest }
      policies: [ <see §3> ]
```

Notes on specific fields:

- **No `agentCard` block.** The gateway default is rewritten passthrough, which
  is exactly what M1 wants. Writing the block out would only add a way to get
  it wrong.
- **`context: /<agentName>`** matches what the api-configuration trait uses
  today (`buildAPIConfigurationTraitParameters`, `components.go:2486`), so A2A
  and REST agents keep consistent URL shapes.
- **`resilience` is deliberately omitted.** The gateway disables the route
  timeout (`0s`) by default for the JSON-RPC route and for streaming HTTP+JSON
  routes, precisely because A2A streaming operations are long-lived. Writing
  agent-manager's `resilienceTimeoutSeconds` into `spec.resilience` would
  override that default and sever every stream at the timeout. If a timeout is
  ever wanted it belongs at the per-operation level, on non-streaming
  operations only.
- **`upstream.url` comes from the release binding's status, not from a
  convention.** `gen.EndpointURLStatus.ServiceURL`
  (`clients/openchoreosvc/gen/types.gen.go:1884`) carries the workload's
  in-cluster address as `{Host, Port, Scheme, Path}` — e.g.
  `it-helpdesk.dp-default:8080`, the same `name.namespace` form the platform
  accepts for a gateway's own cluster-local `runtimeUrl`
  (`platform_gateway_service.go:1241`). agent-manager already lists release
  bindings by component and environment in two places
  (`GetComponentEndpoints`, `components.go:2645`; `extractEndpointsFromBinding`,
  `deployments.go:1571`); both take `ExternalURLs` and ignore the `ServiceURL`
  sibling. The platform already treats this field as authoritative for the
  running workload: `probedPorts` (`deployments.go:1368`) reads its `Port` to
  aim the TCP startup probe.

  Two details the implementation must settle. **Ordering:** status is populated
  once the binding reconciles, so an `Agent` published at deploy time may read
  an empty `ServiceURL` and route nowhere. Publication should hang off the
  existing readiness machinery (`bootDeadlineExceeded`, `agentStartupBudget`)
  rather than off the deploy call, and must refuse to emit a resource with an
  empty upstream. **Selection:** an agent component should expose exactly one
  endpoint, but the rule for picking one needs stating, and nothing in this
  repo reads `Scheme` today — `http` may have to be assumed.
- **`metadata.name` needs sanitizing.** `agentEnvAPIArtifactHandle` builds
  `project/agent/envID` with slashes (`agent_api_artifact.go:31`) — fine as a
  DB handle, illegal as a Kubernetes name. `mcpProxyEnvArtifactHandle` already
  solves the same problem with dashes plus a stripped environment UUID; follow
  it.

**Artifact reuse.** The resource reuses the existing per-environment
`models.KindAgent` artifact row from `ensureAgentEnvAPIArtifact`. Because agent
API keys are already keyed on those rows and already served to the gateway, the
entire API-key path is inherited with no new code — which is what "the same
capabilities as chat/custom agents" requires.

**Publication** mirrors MCP one-for-one, across both of the gateway's
control-plane channels.

*Incremental (an actual deploy).* agent-manager broadcasts; the gateway's
control-plane client dispatches on the event name
(`gateway/gateway-controller/pkg/controlplane/client.go:1422-1478`), fetches the
artifact back from agent-manager, stores it, and publishes an internal
`eventhub.EventTypeAgent` so its sibling replicas converge. That last hop —
`agent_processor.go`, which re-reads the artifact from the gateway's own
database rather than taking it from the event — already exists on branch `a2a`.

The gateway accepts three event names, and their payloads are fixed by
`pkg/controlplane/events.go:288-329`:

| Event type | Action | Payload |
|---|---|---|
| `agent.deployed` | `CREATE` | `{agentId, deploymentId, performedAt}` |
| `agent.undeployed` | `DELETE` | `{agentId, deploymentId, performedAt}` |
| `agent.deleted` | `DELETE` | `{agentId}` |

`agentId` is the artifact UUID, which is how the gateway ties the resource back
to its API keys. M1 needs `agent.deployed` and `agent.deleted`; agent-manager
broadcasts no `mcpproxy.undeployed` today, so `agent.undeployed` follows suit
and is left unused. Undeploy is worth revisiting — the gateway's
`AgentService.Undeploy` guards against a mismatched deployment ID and against a
stale timestamp, which delete does not.

*Bulk (gateway reconnect).* `syncDeployments` polls `GET /deployments`, diffs
against local state, and batch-fetches what it is missing
(`pkg/controlplane/sync.go:55`). This is why the resource needs a `deployments`
row and not just a broadcast: without one, an Agent survives only until the
gateway restarts.

**The one new internal surface: `GET /agents/{agentId}`.** On receiving
`agent.deployed` the gateway calls
`FetchResourceZip("/agents/"+agentID, ...)` then `ExtractYAMLFromZip`
(`client.go:3047,3059`), so the response must be a **zip containing the
artifact YAML** — the same contract `GetMCPProxy` already satisfies
(`api/gateway_internal_routes.go:57`). agent-manager has no such route today;
adding it is the only new internal endpoint M1 needs.

**Gateway-side support is in place** on `api-platform` branch
`agent-cp-events` (`73f1a35d0 add agent event handling`, one commit on top of
`a2a`). Verified for both channels:

- Event dispatch for all three names (`client.go:1481-1486`) into real handlers
  (`:2999`, `:3124`, `:3214`), acking under `resourceType: "agent"`.
- The deployed handler resolves secret refs from the YAML, then applies via
  `agentService.Create` with `Origin: models.OriginControlPlane`.
- Bulk sync: `models.KindAgent` in `processSyncFetches` (`sync.go:239`, applied
  at `:349-368`), in the deletion path (`:504`, `:609`), and in `syncEventType`
  (`:969` → `eventhub.EventTypeAgent`, so replica convergence uses the Agent
  lane rather than falling through to the API lane).

Two consequences for this side. The `deployments` row's `kind` must be exactly
`"Agent"` for the bulk path to match — `models.KindAgent` already is. And the
gateway's DP→CP push stays off (`ControlPlanePushSupported = false`), which is
the opposite direction and does not affect M1.

### 3. The auth chain

Two modes, and both already exist. agent-manager attaches exactly the policy
chain it attaches for chat and custom agents today; the gateway's own rules
supply the A2A-specific behaviour on top.

```
CORS  →  [ api-key-auth | jwt-auth ]
```

Ordering is fixed and already correct in `buildPolicies`: CORS first *"so
preflight OPTIONS requests are handled before any auth policy runs"*
(`agent_manager.go:3616`).

**Mode A — auth off.** No auth policy. All eleven operations are open and the
agent applies whatever authorization it wants to. `GetExtendedAgentCard`
returns 401 unconditionally — the gateway's guard is not configurable — so an
agent in this mode cannot publish an extended card. The public card stays
reachable either way.

**Mode B — auth required.** The auth policy is attached unconditionally. Every
operation needs a credential, and `GetExtendedAgentCard` works. This is exactly
the behaviour chat/custom agents have today.

The consequence to state plainly: in M1 an A2A agent either authenticates
everything or nothing. An agent that wants unauthenticated capabilities *and*
an extended card is not expressible — that needs optional auth, which is
deferred (see Out of scope). Nothing in M1's shape blocks adding it later: it
is a third value of the same setting, attaching the same policy with an
`executionCondition`.

The agent also receives no gateway-minted identity in M1. `backend-jwt` is
deferred with optional auth, so an agent that needs to know who the caller is
must read whatever the gateway forwards, as chat/custom agents do today.

### 4. Lifecycle

| Event | Hook | Mirrors |
|---|---|---|
| Deploy / redeploy | after `UpdateComponentDeploymentConfig` (`agent_manager.go:~3250`) | `deployMCPProxyToGateway` |
| Delete | `deleteAgentAPIArtifact` (`agent_manager.go:2786`) | `broadcastMCPProxyDeletion` |
| Gateway reconnect | none in this repo | the `deployments` row backs `syncDeployments`, which handles `KindAgent` |

Redeploy writes a fresh `deployments` row and re-broadcasts `CREATE`, as MCP
does; `CreateWithLimitEnforcement` already caps history. Deletion broadcasts to
every active gateway in the org, best-effort with warnings.

## Risks and open dependencies

Ranked by likelihood of biting.

1. **API-key identity binding is assumed.** §2 claims the API-key path is
   inherited because the gateway keys Agents to the `KindAgent` artifact UUID.
   But `agent-api-keys.feature` addresses keys at `/agents/{agentName}/api-keys`
   — by **name**. Plausible, unverified, and the URL shape hints otherwise.

2. **`proxy/agent-api` with no `api-configuration` trait is untested.** If the
   ComponentType template assumes the trait, detaching it may break rendering.

3. **The dangling trait-env-config key** (§1, `agent_manager.go:3697`) — the
   claim that OpenChoreo rejects a `traitEnvironmentConfigs` key with no
   attached trait rests on a code comment, not observed behaviour. Cheap to fix
   either way.

4. **Omitting `resilience` preserves the streaming-friendly `0s` default** —
   sourced from `schemas.md` prose, the same document that proved stale on
   `rewriteUrls`. Worth a feature-file check, because streams break silently if
   it is wrong.

5. **`spec.vhost`** — that `models.Gateway.Vhost` is the right value is
   inferred; agent-manager sets no vhost for RestApi today.

No blocking external dependency remains. The gateway's control-plane support
for Agents — previously the one architectural unknown here — landed on
`agent-cp-events` and is described in §2; the event payload shapes this design
assumed match it exactly. Everything above is answerable in this repo.

`upstream.url` was previously listed here as a blocking risk on the grounds
that the `backendHost`/`backendPort` convention moved into the OpenChoreo trait
template and had no owner in this repo. It is resolved: §2 reads the address
out of the binding's status instead of re-deriving it, so the value follows
OpenChoreo automatically rather than drifting against it.

## Testing

Following the repo's existing tiers:

- **Service-layer unit tests** (moq'd repositories, per the
  `add-service-unit-test` conventions) for the YAML builder and for the policy
  chain in both auth modes.
- **`tests/create_agent_a2a_test.go`** asserting the `api-configuration` trait
  is *absent* from `TraitsToAttach` while the others remain — mirroring the
  positive assertion at `tests/create_agent_test.go:337` — and that no
  `TraitAPIManagement` key appears in `traitEnvironmentConfigs`.
- **Golden-YAML coverage** of the emitted `Agent` resource. This is a
  cross-repo contract; a golden file is what makes a drift visible.
- **Lifecycle tests** for deploy/redeploy/delete broadcast behaviour, following
  the MCP deployment tests.

## Out of scope for Milestone 1

Everything below is deliberately excluded. The list is deliberately broader than
what came up during design — it includes capabilities other A2A platforms
commonly ship — so that milestone triage is a decision rather than an omission.

### Agent Card

- **Managed cards** — gateway-authored card content, requiring agent-manager to
  fetch and store the agent's card.
- **Card signing (JWS)** — the gateway currently rejects `signing.enabled`
  outright; its Section 15 is deferred, so this is blocked upstream regardless.
- **Explicit `agentCard.protected` configuration** — M1 relies on the default
  (passthrough and guarded).
- **`rewriteUrls: false` opt-out** — for agents that must publish their own
  exact bytes, signatures included.
- **Custom card path** — M1 uses the well-known default.
- **Card ETag / caching control** — the gateway provides this for managed cards
  only.

### Protocol and transport

- **gRPC** — not supported by the gateway.
- **User-configurable transports and path prefixes** — M1 fixes `/rpc` and
  `/rest`.
- **Multiple protocol versions and version negotiation** — an agent exposes
  exactly one version and the gateway performs no conversion.
- **Streaming verification** — `SendStreamingMessage` and `SubscribeToTask`
  should work by default (M1 omits `spec.resilience` precisely to preserve the
  disabled route timeout), but M1 does not test streaming behaviour end to end.

### Task management

- **Push notification configuration** — the four
  `*TaskPushNotificationConfig` operations route like any other, but platform
  management of webhook registration, verification and delivery is not
  addressed.
- **Task history, persistence and listing surfaces.**

### Security and governance

- **Optional ("auth on passthrough") mode** — attach the auth policy with an
  `executionCondition` so a credential is checked when present but never
  required. This is what lets an A2A agent expose unauthenticated capabilities
  *and* an extended agent card, and it is the most likely candidate to promote
  into Milestone 2. Deferred because the mechanism is unverified: no `optional`
  flag exists on the gateway's auth policies, and it is not established that
  the `executionCondition` expression language can test for the presence of a
  credential header.
- **`backend-jwt` minting** — a gateway-signed token carrying the caller's
  identity to the agent, so the agent can authorize per capability without
  seeing the client's original credential. Deferred with optional auth, and
  independently blocked: the policy-hub API returns the policy's description
  but 404s on every definition path, so its `params` block cannot be written
  from evidence, and it is unconfirmed that the policy is installed in the
  gateway image and valid on an Agent's chain.
- **JWKS delivery to the agent** — required by `backend-jwt` and deferred with
  it. The intended home is the existing env-injection trait, which already
  derives the gateway runtime address from the `api-platform-<org>-<env>`
  convention for `otelEndpoint` (`components.go:2563-2566`) and could derive a
  JWKS URL from a boolean parameter, at the cost of one OpenChoreo template
  change. The fallback is `agent_identity_injection_service.go`, which already
  injects per-environment system variables from agent-manager alone. The
  endpoint's actual address is unconfirmed.
- **Per-operation policies** — rate limits and quotas keyed by canonical
  operation name. The gateway supports this; M1 attaches an agent-wide chain
  only.
- **Per-skill or per-operation scope authorization** — mapping A2A operations
  or skills to required scopes at the gateway.
- **Guardrails on A2A message payloads** — the platform already ships LLM
  guardrails; the analogous content controls for A2A messages are not wired.
- **Upstream authentication to the agent** — mTLS or credential injection on
  the gateway→agent hop (`UpstreamAuth` exists in the schema).
- **Agent-to-agent delegation and trust chains** — on-behalf-of token exchange
  between chained A2A agents.

### Discovery and operations

- **Catalog and dev-portal surfacing** of A2A agents, their skills and their
  capabilities. Requires card content the platform does not store in M1.
- **A2A-specific analytics and metrics** — the gateway has agent analytics
  plumbing; agent-manager does not consume it.
- **Tracing of A2A operations** through the existing observability stack.

### Platform

- **External (BYO-hosted) A2A agents** — the `external-agent-api` type.
- **Outbound A2A** — an agent-manager agent consuming other A2A agents as
  dependencies.
