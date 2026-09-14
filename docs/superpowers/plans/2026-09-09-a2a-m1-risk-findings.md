# A2A M1 — Findings for the spec's five open risks

Verification pass for Task 1 of `2026-09-09-a2a-agent-support-m1.md`. No production
code was changed. Cross-repo citations are against
`/home/jhivandb/Dev/work/api-platform`, branch `agent-cp-events`.

---

## Risk 1 — API-key identity binding: **works, but not for the reason the spec gives**

Confirmed. The binding holds, via the `KindRestApi` path rather than any Agent-specific one.

- `agent-manager-service/controllers/gateway_internal_controller.go:240-243` —
  `GET /api/internal/v1/apis/api-keys` is served by `getAPIKeysByKind(w, r, models.KindAgent)`.
  AMS serves its `KindAgent` rows on the REST-API key path.
- `gateway/gateway-controller/pkg/utils/api_utils.go:329-345` — the gateway's kind→path
  map. `models.KindRestApi` → `/apis/api-keys`; **`models.KindAgent` is absent from the
  map entirely**. So the gateway fetches AMS's Agent keys under its own `KindRestApi` label.
- `gateway/gateway-controller/pkg/controlplane/client.go:1112` — the bulk-sync kind loop
  is `{KindRestApi, KindWebSubApi, KindWebBrokerApi, KindLlmProvider, KindLlmProxy}`.
  `KindAgent` is absent here too, consistent with the above.
- `gateway/gateway-controller/pkg/controlplane/client.go:3076-3090` —
  `agentService.Create(agent.CreateParams{ID: deployAgentID, ...})` where
  `deployAgentID = c.resolveLocalArtifactID(agentID)` and `agentID` is the deployed
  event's artifact UUID.

Keys are stored keyed by `ArtifactUUID`, and the Agent resource is created with `ID` =
that same artifact UUID, so key lookup resolves. The `/agents/{agentName}/api-keys` route
seen in `agent-api-keys.feature` is the gateway's own DP-local key-management API and is
unrelated to control-plane sync.

**Consequence for the plan:** none. `agentId` in the deployed event must be the
per-environment `models.KindAgent` artifact UUID, exactly as the Global Constraints state.

---

## Risk 2 — `proxy/agent-api` renders without `api-configuration`: **yes, but the endpoint is mandatory**

Confirmed, with the caveat the plan already anticipates.

`deployments/helm-charts/wso2-amp-platform-resources-extension/templates/component-types/agent-api.yaml`:

- `:20-28` — `allowedTraits` lists `api-configuration` among four traits. It is
  **allowed, not required**; there is no `requiredTraits` stanza. Detaching it is safe
  for rendering.
- `:30-32` — `validations: - rule: "${size(workload.endpoints) > 0}"`,
  message "Agent API components must have at least one endpoint." An `a2a-agent`
  carrying **no** workload endpoint fails ComponentType validation outright.
- `:314-325` — the `Service` resource is rendered by the ComponentType itself
  (`name: ${metadata.componentName}`, `namespace: ${metadata.namespace}`,
  `ports: ${workload.toServicePorts()}`), not by the trait. Detaching the trait does not
  cost us the Service.
- `:327-365` — `httproute-external` is likewise rendered by the ComponentType, with
  `timeouts.request: 30s` and the comment "Overwritten to 0s by the api-configuration
  trait's HTTPRoute patch".

**Consequence for the plan:** Task 2 must add an endpoint branch that emits a port with
no OpenAPI schema, rather than emitting no endpoint. Also note that without the trait the
in-cluster HTTPRoute keeps its 30s request timeout; this is the *ComponentType's* route,
not the gateway's, and M1's traffic path reaches the agent through the gateway Agent
resource, so it does not sever A2A streams on the M1 path. Flagged for M2 if the
ComponentType route is ever used for A2A directly.

---

## Risk 3 — the dangling `traitEnvironmentConfigs` key: **gated defensively**

`agent-manager-service/services/agent_manager.go:3687-3697` — `buildTraitEnvConfigs`
unconditionally seeds

```go
traitEnvConfigs := map[string]interface{}{
    instanceName(client.TraitAPIManagement): apiTraitCfg,
}
```

The doc comment above it asserts OpenChoreo resolves these keys against attached trait
instances. That assertion is a **comment, not observed behaviour** — nothing in this repo
or the ComponentType template proves what OpenChoreo does with a `traitEnvironmentConfigs`
key naming a trait that is not attached.

No live experiment was run. Gating the key is a two-line change and strictly safer than
leaving it, so **Task 3 gates it regardless**. Recording plainly: this gating is
**defensive, not evidence-driven**.

---

## Risk 4 — omitting `spec.resilience` preserves the streaming-friendly default: **confirmed**

`gateway/it/features/agent_streaming.feature:19-21`:

> "A2A's streaming operations are the reason the Agent kind disables its route timeout by
> default and leans on idleTimeout instead."

Every scenario in that file (`:58-59`, `:135-136`, `:279-280`, `:344-345`) sets only
`resilience.idleTimeout` — never a request timeout. This is a feature file exercised in
CI, a better source than the stale `schemas.md` the spec worried about.

**Consequence for the plan:** omitting `spec.resilience` is correct. Writing
`resilienceTimeoutSeconds` into an Agent would sever every stream. Do not emit the block.

---

## Risk 5 — `spec.vhost`: **the gateway's advertised host, sourced from `Gateway.Vhost`**

`gateway/it/features/agent_card.feature:596-632`, scenario "A configured vhost outranks
the address the client dialled": the Agent is deployed with `vhost: agents.example.com`
while the client dials `localhost:8080`, and the rewritten card asserts
`http://agents.example.com/agent-card-rewrite-vhost/v1` — and explicitly *not*
`localhost:9099` or `localhost:8080`.

So `spec.vhost` is the host the gateway advertises in a rewritten passthrough card. The
right value is the environment's gateway vhost:

- `agent-manager-service/models/gateway.go:35` — `Gateway.Vhost string`.
- `agent-manager-service/services/platform_gateway_service.go:210` — set at gateway
  registration.

**Shape decision:** emit `vhost` **only when the gateway's `Vhost` is non-empty**,
mirroring `MCPProxyDeploymentSpec.Vhost` which is a `*string`.

---

## Bonus finding — `GET /deployments` reports nothing (out of scope for M1)

`agent-manager-service/controllers/gateway_internal_controller.go:346-360` —
`GetDeployments` returns `{"deployments": []}` unconditionally. Its own comment says
deployment state "lives entirely in the RestApi/LlmProvider CRs the gateway-operator
manages, not in agent-manager's own DB, so there is nothing yet to report here".

The spec's §2 claim that the `deployments` row backs `syncDeployments` is therefore only
half true today. The row **is** still load-bearing — `GET /agents/{agentId}` reads it via
`GetCurrentByGateway` (Task 10) — but **bulk sync on gateway reconnect is inert for every
kind**, MCP included, until `GetDeployments` actually reports rows.

**Out of scope for M1**: fixing it would change reconnect behaviour for LLM and MCP
artifacts too. Recommend filing separately.

---

## Verdict

No finding contradicts the plan. Tasks 2–5 proceed as written, with Risk 2's endpoint
requirement and Risk 5's conditional-`vhost` shape carried forward.
