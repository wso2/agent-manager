# Per-endpoint scope→tool bindings for MCP proxies

Design for [wso2/agent-manager#1346](https://github.com/wso2/agent-manager/issues/1346).

## Problem

An MCP proxy owns a set of scopes. Each scope is an action on the resource server the
proxy is, and its token string is `<proxy-handle>:<action>`. Today one scope row also
owns a single flat list of tools it authorizes, shared across every endpoint of the
proxy:

```
mcp_proxy_scopes(uuid, mcp_proxy_uuid, action, description, tools JSONB)
```

Endpoints are the per-environment variation point. Each endpoint has its own upstream and
its own discovered `capabilities.tools`, and `uq_proxy_env_single` binds at most one
endpoint per environment per proxy. A proxy's endpoints therefore routinely expose
*different* tool sets — but the scope→tool assignment cannot vary with them. Three
consequences:

1. **Validation cannot be strict.** `proxyToolUnion`
   (`agent-manager-service/services/mcp_proxy_scope_service.go:113`) validates a scope's
   tools against the union of tools across *all* endpoints. A tool that exists only on
   the dev endpoint passes validation for a scope that is emitted to prod as well.

2. **Every environment gets identical authz.** `deployMCPProxyEndpoints`
   (`services/mcp_proxy_deployment.go:228`) loads one scope list and passes the same list
   to every `(endpoint, environment)` gateway artifact. `appendMCPIdentityAuthPolicies`
   inverts it into one `mcp-authz` rule per tool, so prod carries rules for tools it does
   not expose and there is no way to grant `write` in dev but not in prod.

3. **The console silently destroys data.** `MCPProxySecurityTab.tsx` builds one row per
   tool of the *selected* endpoint, then `computeScopeReconciliation`
   (`MCPProxySecurityTab.tsx:97`) diffs those rows against the proxy-wide scope list.
   Tools belonging to other endpoints are absent from `desired`, so saving on endpoint
   B's tab strips endpoint A's tools from every scope — and **deletes outright** any scope
   whose tools all belonged to A.

## Approach

Split scope *definition* from scope *assignment*, along the boundary the issue names:

- **Definition stays proxy-level.** `mcp_proxy_scopes` keeps `(action, description)`. The
  token scope string is unchanged, so roles, Thunder resource-server projection, and
  agent token minting are untouched.
- **Assignment moves to the endpoint.** A new table binds `(scope, endpoint) → tools`.

Verified unaffected, because they consume scope *strings* and never tools:
`agentIdentityController.resolveScopeGroups` / `CreateRole`, `EnsureProxyResourceServer`
and the Thunder action projection, `sweepRolePermission`, and
`agentIdentityInjectionService.resolveAgentIdentityScopes`.

### Prerequisite: stable endpoint UUIDs

Proxy `PUT` reconciles endpoints by replacing the set — it deletes every endpoint row and
re-inserts, minting a fresh `uuid.New()` each time (`services/mcp_proxy_service.go:452`
and `persistMCPEndpoints:1305`). A bindings table FK'd to `mcp_proxy_endpoints(uuid)` with
`ON DELETE CASCADE` would therefore be wiped on every proxy update.

Fix it at the source: preserve an endpoint's UUID across the replace when its handle is
unchanged. This mirrors machinery that already exists in the same function —
`existingMCPEndpointIndex` (`mcp_proxy_service.go:1217`) already preserves per-environment
artifact UUIDs and per-endpoint upstream secrets across the same delete/re-insert, keyed
by handle.

- Add `uuidByHandle map[string]uuid.UUID` to `existingMCPEndpointIndex`, populated in
  `indexExistingMCPEndpoints`.
- Add a `uuid` field to `storableMCPEndpoint`; `buildMCPEndpointsForStorage` fills it from
  `uuidByHandle[handle]`, falling back to `uuid.New()` for a handle not previously stored.
- `persistMCPEndpoints` uses that UUID instead of minting one.

Renaming an endpoint handle still mints a new UUID and drops the old row, so its bindings
cascade away. That is the correct reading of a rename, and it matches how upstream-auth
preservation already behaves.

### Data model

```sql
CREATE TABLE mcp_proxy_scope_endpoint_tools (
    scope_uuid    UUID        NOT NULL REFERENCES mcp_proxy_scopes(uuid)    ON DELETE CASCADE,
    endpoint_uuid UUID        NOT NULL REFERENCES mcp_proxy_endpoints(uuid) ON DELETE CASCADE,
    tools         JSONB       NOT NULL DEFAULT '[]',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (scope_uuid, endpoint_uuid)
);
CREATE INDEX idx_mcp_scope_endpoint_tools_endpoint
    ON mcp_proxy_scope_endpoint_tools (endpoint_uuid);
```

Both FKs cascade, so deleting a scope, an endpoint, or the whole proxy needs no
application-side sweep. Absence of a row means "this scope authorizes nothing on this
endpoint" — there is no separate empty state to represent.

`mcp_proxy_scopes.tools` is dropped in the same migration.

### API

Scope CRUD stays proxy-level and becomes definition-only. Assignment gets an endpoint
sub-resource that replaces that endpoint's entire binding set in one call, so a save for
one endpoint cannot touch another by construction.

```
GET    /orgs/{orgName}/mcp-proxies/{proxyId}/endpoints/{endpointId}/scope-bindings
PUT    /orgs/{orgName}/mcp-proxies/{proxyId}/endpoints/{endpointId}/scope-bindings
```

`{endpointId}` is the endpoint **handle** — consistent with `MCPProxyEndpointDTO.ID`,
which the console already passes as `selectedEndpointId` (`mcp_proxy_service.go:922`). The
service resolves handle → UUID for storage.

Request and response body:

```yaml
MCPEndpointScopeBindings:
  type: object
  required: [bindings]
  properties:
    bindings:
      type: array
      items:
        type: object
        required: [action, tools]
        properties:
          action: { type: string }
          tools:
            type: array
            minItems: 1
            items: { type: string }
```

`PUT` is a full replace: an action omitted from `bindings` has its binding for this
endpoint removed. The scope itself survives — only its assignment on this endpoint goes
away.

Scope schemas change shape (clean break; `tools` is removed, not aliased):

```yaml
MCPProxyScopeRequest:        { action, description }          # tools removed
MCPProxyScopeUpdateRequest:  { description }                  # tools removed
MCPProxyScopeResponse:
  action, scope, description, createdAt, updatedAt
  bindings:                                                   # replaces tools
    - endpointId: string
      tools: [string]
```

Endpoints with no tools bound for a scope are omitted from `bindings` rather than
returned with an empty array.

RBAC: `GET` uses `rbac.ScopeRead`, `PUT` uses `rbac.ScopeUpdate` — no new permissions.

### Validation

Per-endpoint and strict: every tool in a binding must appear in *that endpoint's*
`capabilities.tools`. `proxyToolUnion` is replaced by a per-endpoint tool set. The
existing strict-when-known fallback is retained for exactly one case — an endpoint with no
stored capabilities at all validates permissively, since capabilities are not always
discoverable. Unknown tools are a `400`.

Every `action` in a `PUT` must already exist as a scope on the proxy; an unknown action is
a `400`. The current "a scope must authorize at least one tool" rule
(`validateScopeTools`) moves down to the binding level: a *binding* must name at least one
tool, but a *scope* may bind nothing on a given endpoint, or nothing anywhere. That is
what makes "create the scope, then assign it" possible.

### Gateway emission

`deployMCPProxyEndpoints` loads all bindings for the proxy once, grouped by endpoint UUID,
and passes only the current endpoint's bindings down through `deployMCPProxyToGateway` →
`buildMCPProxyDeploymentYAML` → `appendMCPIdentityAuthPolicies`. That last function's
`scopes []models.MCPProxyScope` parameter becomes a prepared per-endpoint slice carrying
the derived scope string and the tools it covers on this endpoint.

The one-rule-per-tool inversion and the any-of `requiredScopes` semantics documented at
`mcp_proxy_deployment.go:563` are unchanged — only the input set narrows. Result: each
environment's gateway carries rules for exactly the tools its endpoint exposes.

The agent-configuration flow needs no change: it no longer deploys its own artifacts
(`buildAgentMCPConfigProxy` only flattens the endpoint bound to the mapping's environment,
and `deployMCPProxyToGateway` has a single caller).

### Environment scope aggregate

`ListEnvironmentScopes` powers the role builder. After resolving the endpoint bound to the
environment (`resolveMCPEndpointForEnv`), filter the proxy's scopes to those with a
binding on that endpoint naming at least one tool. A scope that authorizes nothing in an
environment stops being offered as a grantable permission there. This is a read-only
aggregate, so existing roles are unaffected.

### Migration (037)

1. Create `mcp_proxy_scope_endpoint_tools`.
2. Backfill in Go: for each scope, for each endpoint of its proxy, insert
   `scope.tools ∩ endpoint.capabilities.tools`; when the endpoint has no stored
   capabilities, copy `scope.tools` verbatim. Skip empty intersections — no row means no
   authorization, which is what the gateway already enforced.
3. Drop `mcp_proxy_scopes.tools`.

Intersecting rather than copying wholesale changes no live behavior: an `mcp-authz` rule
naming a tool the endpoint does not expose can never match a request. It also leaves every
backfilled row passing the new strict validation, so the first console save after upgrade
is not a validation failure.

Add `migration037` to `migration_list.go` and bump `latestVersion` to 37.

### Console

`MCPProxySecurityTab.tsx` — its rows are already per-endpoint, so the fix is in the save
path:

- Replace `computeScopeReconciliation` with a function that maps the current rows to this
  endpoint's `bindings` array, and save with a single `PUT scope-bindings`. The
  cross-endpoint clobber disappears with the function that caused it.
- Creating a scope inline still `POST`s the scope first (definition only), then includes
  it in the bindings `PUT`.
- Clearing every tool for a scope no longer deletes it. Scope deletion becomes an explicit
  affordance in the scope list — a behavior change worth calling out in review, since
  today it happens implicitly.

`ViewMCPServer.Component.tsx` — the agent-side tool view builds `scopesByTool` from the
flat `tools` array (`:499`). It already knows the selected environment, so it should read
the bindings of the endpoint bound to that environment.

`libs/types` and `libs/api-client` are hand-written (not generated): update
`api/mcp-proxy-scopes.ts`, `apis/mcp-proxy-scopes.ts`, and `hooks/mcp-proxy-scopes.ts`
following the two-file `apis/` + `hooks/` pattern.

### CLI

`cli/pkg/clients/amsvc/gen/` is generated from the service spec. No CLI command touches
scopes, so `make amctl-gen-client` is the whole change.

### Docs

`documentation/docs/tutorials/authorize-agent-access-to-mcp-tools.mdx` walks through
assigning tools to a scope and needs rewriting for the per-endpoint flow.

## Testing

- **Validation** — a tool on endpoint A rejected for a binding on endpoint B; permissive
  fallback when the endpoint has no stored capabilities; unknown action rejected.
- **Isolation** — replacing endpoint B's bindings leaves endpoint A's rows byte-identical.
  This is the regression test for the console clobber bug.
- **Endpoint UUID stability** — a proxy `PUT` that changes an unrelated field preserves
  endpoint UUIDs and bindings; renaming a handle drops that endpoint's bindings.
- **Emission** — two endpoints with different bindings produce different `mcp-authz` tool
  rules for their respective artifacts, both prefixed with the source proxy handle.
- **Env aggregate** — a scope bound on prod's endpoint but not dev's is absent from dev's
  grantable list.
- **Migration** — backfill intersects against per-endpoint capabilities and copies
  verbatim when capabilities are absent.

Service tests follow the repo's unit tier: no build tag, moq-generated repository mocks,
and the strict CI lint config that also lints test files.

## Out of scope

- **`AMP_AGENTID_SCOPES` stays the proxy-level union.** Narrowing the injected scope list
  to the agent's environment is a real follow-up, but `resolveAgentIdentityScopes`
  (`services/agent_identity_injection_service.go:241`) carries an explicit guard against
  ever writing an empty scope list, because that causes pod-rollout churn. Per-endpoint
  filtering can legitimately produce empty. Over-requesting is harmless today — Thunder
  filters requested scopes down to what the agent's role grants — so this interaction
  deserves its own change.
- **Pruning bindings when a capability refresh removes a tool.** No such pruning exists
  today for `scopes.tools`; the per-endpoint model makes it tractable, but it is a separate
  behavior change.
