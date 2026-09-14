# A2A Agent Support — Milestone 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a new agent subtype `a2a-agent` that provisions with the existing `agent-api` component type but without the `api-configuration` (REST API) trait, and is published to the API gateway as a `kind: Agent` resource carrying both A2A transports and the platform's existing policy chain.

**Architecture:** `a2a-agent` is a *subtype*, so OpenChoreo component provisioning is unchanged — the component is still `proxy/agent-api`. Three things differ: the `api-configuration` trait is not attached (and its `traitEnvironmentConfigs` key is not written), the component endpoint is built from a port with no OpenAPI schema, and agent-manager emits a `kind: Agent` YAML directly to the gateway over the control-plane event channel, exactly as `mcp_proxy_deployment.go` does for `kind: Mcp`. Because `upstream.url` is read from `ReleaseBinding.Status.Endpoints[].ServiceURL` — which is only populated after the binding reconciles — publication cannot happen inside the deploy call. It is instead queued as a durable row and driven by a new background reconciler modelled on `services/agent_thunder_reconciler.go`.

**Tech Stack:** Go 1.x (agent-manager-service), GORM + PostgreSQL, `gopkg.in/yaml.v3`, moq-generated mocks, testify. Go CLI (cobra) in `cli/`. React/TypeScript + MUI console in `console/workspaces/`.

**Spec:** `docs/superpowers/specs/2026-09-09-a2a-agent-support-design.md`

**Cross-repo reference (read-only, do not modify):** `/home/jhivandb/Dev/work/api-platform`, branch `agent-cp-events`.

---

## Global Constraints

Copied verbatim from the spec; every task's requirements implicitly include these.

- **Subtype value:** `a2a-agent`. Agent *type* stays `agent-api`. `getOpenChoreoComponentType` is untouched.
- **Gateway resource:** `apiVersion: gateway.api-platform.wso2.com/v1`, `kind: Agent`.
- **Transports, fixed, in this order:** `{protocolBinding: JSONRPC, pathPrefix: /rpc}`, then `{protocolBinding: HTTP+JSON, pathPrefix: /rest}`. Not user-configurable in M1.
- **`spec.a2a.protocolVersion`:** the string `"1.0"`.
- **No `agentCard` block is written.** The gateway's default is rewritten passthrough, which is what M1 wants.
- **No `spec.resilience` block is written.** The Agent kind disables its route timeout by default for streaming; writing `resilienceTimeoutSeconds` there would sever every stream.
- **`spec.context`:** `/<agentName>` — matches `buildAPIConfigurationTraitParameters` (`clients/openchoreosvc/client/components.go:2486`).
- **`spec.version`:** `v1.0`.
- **Policy chain order:** `CORS` first, then exactly one of `api-key-auth` / `jwt-auth`. Produced by the existing `buildPolicies()` (`services/agent_manager.go:3614`) with no translation layer.
- **`upstream.url`** comes from `ReleaseBinding.Status.Endpoints[].ServiceURL`. Never derived from a naming convention. A resource with an empty upstream must never be emitted.
- **Event names and payloads are fixed by the gateway** (`gateway/gateway-controller/pkg/controlplane/events.go:288-329`):
  - `agent.deployed`, action `CREATE`, payload `{agentId, deploymentId, performedAt}`
  - `agent.deleted`, action `DELETE`, payload `{agentId}`
  - (`agent.undeployed` exists but is deliberately unused in M1, mirroring `mcpproxy.undeployed`.)
  - `agentId` is the **artifact UUID** of the per-environment `models.KindAgent` artifact row.
- **Fetch-back contract:** `GET /api/internal/v1/agents/{agentId}` must return a **ZIP containing the artifact YAML** — the gateway calls `FetchResourceZip("/agents/"+agentID, ...)` then `ExtractYAMLFromZip` (`client.go:3047,3059`).
- **`deployments` row `kind` must be exactly `"Agent"`** — `models.KindAgent` already is.
- **Out of scope, do not implement:** `backend-jwt`, optional/"auth on passthrough" auth, JWKS injection, managed cards, card signing, `rewriteUrls: false`, gRPC, per-operation policies, push-notification management, external (BYO) A2A agents, outbound A2A, catalog/dev-portal surfacing, A2A analytics. See the spec's "Out of scope for Milestone 1" section — anything in it stays out.

### Repo conventions the executor must follow

- **Service unit tests:** package `services`, **no build tag**, moq-generated repository mocks. See `services/mcp_proxy_deployment_test.go`. Lint is strict and lints test files too (`nilnil`, `goheader`, `exhaustruct`, `errorlint`).
- **Integration tests:** `tests/*.go`, **first line `//go:build integration`**, `apitestutils.MakeAppClientWithDeps`. See `tests/create_agent_test.go`.
- **Every new Go file needs the WSO2 Apache-2.0 header** (goheader lint). Copy the header verbatim from `services/mcp_proxy_deployment.go` (year `2026`).
- **Run from `agent-manager-service/`:** `go build ./...`, `go test ./services/... -run <Name>`, `go test -tags=integration ./tests/... -run <Name>`, `make lint`.

---

## File Structure

**agent-manager-service**

| File | Responsibility | Task |
|---|---|---|
| `utils/agent_types.go` | add `AgentSubTypeA2A` constant | 2 |
| `utils/utils.go` | accept the subtype in `validateAgentType` (`:544`); leave `:794` alone | 2 |
| `clients/openchoreosvc/client/components.go` | new `buildEndpoints` branch (`:492`); `isA2AAgent` helper | 2 |
| `clients/openchoreosvc/client/builds.go` | `buildEndpointsFromInputInterface` — verify a2a takes the request's port (`:373`) | 2 |
| `services/agent_manager.go` | narrow the two `isAPIAgent` trait gates (`:474`, `:3229`); gate the `TraitAPIManagement` key in `buildTraitEnvConfigs` (`:3697`); enqueue publication after deploy; enqueue deletion in `deleteAgentAPIArtifact` (`:2786`) | 3, 8, 9 |
| `models/a2a_publication.go` | **new** — `A2APublication` GORM model + status constants | 6 |
| `models/gateway_compat.go` | **new types** — `AgentDeploymentEvent`, `AgentDeletionEvent` | 5 |
| `db_migrations/044_create_a2a_publications.go` | **new** — the `a2a_publications` table | 6 |
| `db_migrations/migration_list.go` | register `migration044` | 6 |
| `repositories/a2a_publication_repository.go` | **new** — upsert / find-due / claim / complete / fail / delete | 6 |
| `services/a2a_agent_deployment.go` | **new** — YAML builder (`buildA2AAgentDeploymentYAML`), artifact-name sanitizer, `deployA2AAgentToGateway`, `broadcastA2AAgentDeletion` | 4, 5 |
| `services/a2a_publication_reconciler.go` | **new** — tick loop, advisory lock, binding-status read, publish-or-retry | 7 |
| `services/gateway_events_service.go` | `BroadcastAgentDeploymentEvent`, `BroadcastAgentDeletionEvent` | 5 |
| `services/gateway_internal_service.go` | `GetActiveAgentDeploymentByGateway` | 10 |
| `utils/api.go` | `CreateAgentYamlZip` | 10 |
| `controllers/gateway_internal_controller.go` | `GetAgent` handler + interface method | 10 |
| `api/gateway_internal_routes.go` | register `GET /agents/{agentId}` | 10 |
| `api/route_authz_invariant_test.go` | add the route to `gatewayInternalRoutes` | 10 |
| `clients/openchoreosvc/client/client.go` + `deployments.go` | `GetReleaseBindingServiceURL` on `OpenChoreoClient` | 7 |
| `wiring/wire.go`, `wiring/params.go`, `app/app.go` | construct + start/stop the reconciler | 7 |
| `docs/api_v1_openapi.yaml` | add `a2a-agent` to the `agentSubType` enum (`:17922`) | 11 |
| `spec/*` | regenerated by `make spec` | 11 |
| `tests/create_agent_a2a_test.go` | **new** integration test | 3 |

**cli**

| File | Responsibility | Task |
|---|---|---|
| `cli/pkg/cmd/agent/create/request.go` | `subTypeA2A` const; interface builder branch; dropped-flag rules | 12 |
| `cli/pkg/cmd/agent/create/validation.go` | accept `a2a-agent` in the subtype switch | 12 |
| `cli/pkg/cmd/agent/create/create.go` | flag help + shell completion | 12 |
| `cli/pkg/cmd/agent/create/template.go` | manifest template comment | 12 |

**console**

| File | Responsibility | Task |
|---|---|---|
| `console/workspaces/libs/types/src/api/agents.ts` | add `'A2A'` to `InputInterfaceType` | 13 |
| `console/workspaces/libs/views/src/utils/provisionTypes.ts` | `displayAgentSubType` case | 13 |
| `console/workspaces/pages/add-new-agent/src/components/InputInterface.tsx` | third card + its collapse panel | 13 |
| `console/workspaces/pages/add-new-agent/src/form/schema.ts` | enum + conditional validation | 13 |
| `console/workspaces/pages/add-new-agent/src/utils/buildAgentPayload.ts` | emit `subType: "a2a-agent"` and the a2a `inputInterface` | 13 |

---

## Task ordering

Task 1 is verification and must run first — five of its findings feed decisions in Tasks 2–5. Tasks 2–3 make the subtype real. Tasks 4–7 build publication. Tasks 8–10 wire the lifecycle and the fetch-back route. Tasks 11–13 are the outer surfaces and are independent of each other.

---

### Task 1: Verify the spec's five open risks

The spec's Risks section lists five items it did not resolve. This task answers each from code before anything is built, because three of them change what later tasks write. **No production code changes here** — the deliverable is a findings document plus one narrow characterization test.

**Files:**
- Create: `docs/superpowers/plans/2026-09-09-a2a-m1-risk-findings.md`
- Create: `agent-manager-service/services/a2a_trait_gating_unit_test.go`
- Read only: `agent-manager-service/controllers/gateway_internal_controller.go`, `deployments/helm-charts/wso2-amp-platform-resources-extension/templates/component-types/agent-api.yaml`, `/home/jhivandb/Dev/work/api-platform/gateway/gateway-controller/pkg/utils/api_utils.go`, `/home/jhivandb/Dev/work/api-platform/gateway/gateway-controller/pkg/controlplane/client.go`, `/home/jhivandb/Dev/work/api-platform/gateway/it/features/agent_streaming.feature`, `/home/jhivandb/Dev/work/api-platform/gateway/it/features/agent_card.feature`

**Interfaces:**
- Consumes: nothing.
- Produces: the findings document. Tasks 3, 4 and 5 read it. If any finding contradicts what this plan assumes, **stop and report** rather than silently diverging.

Below, each risk is stated with the evidence already located, so the step is confirmation rather than a fresh hunt.

- [ ] **Step 1: Risk 1 — API-key identity binding**

Read, in order:
- `agent-manager-service/controllers/gateway_internal_controller.go:242-248` — `GET /apis/api-keys` is served by `getAPIKeysByKind(models.KindAgent)`.
- `/home/jhivandb/Dev/work/api-platform/gateway/gateway-controller/pkg/utils/api_utils.go:329-345` — the gateway's kind→path map. Note `models.KindRestApi` → `/apis/api-keys`, and that **`models.KindAgent` is absent from the map**.
- `/home/jhivandb/Dev/work/api-platform/gateway/gateway-controller/pkg/controlplane/client.go:1112` — the bulk-sync kind loop. `KindAgent` is absent here too.
- `/home/jhivandb/Dev/work/api-platform/gateway/gateway-controller/pkg/controlplane/client.go:3076-3090` — `agentService.Create(ID: deployAgentID)` where `deployAgentID = resolveLocalArtifactID(agentID)` and `agentID` is the event's artifact UUID.

Expected conclusion to record: the binding **does** work, but not for the reason the spec gives. AMS serves its `models.KindAgent` rows at `/apis/api-keys`, which the gateway fetches under its own `KindRestApi` label; the keys are stored keyed by `ArtifactUUID`, and the Agent resource is created with `ID` = that same artifact UUID. The `/agents/{agentName}/api-keys` route in `agent-api-keys.feature` is the gateway's own DP-local key-management API and is unrelated to control-plane sync.

Write this into the findings doc with the four file:line citations. If the reading contradicts it, say so plainly and stop.

- [ ] **Step 2: Risk 2 — does `proxy/agent-api` render without the `api-configuration` trait?**

Read `deployments/helm-charts/wso2-amp-platform-resources-extension/templates/component-types/agent-api.yaml`:
- `:20-28` — `allowedTraits` lists `api-configuration`. **Allowed, not required.**
- `:30-32` — `validations: "${size(workload.endpoints) > 0}"` — "Agent API components must have at least one endpoint."
- `:314-325` — the `Service` resource is rendered by the ComponentType itself, named `${metadata.componentName}` in `${metadata.namespace}`.
- `:327-365` — `httproute-external` is rendered by the ComponentType, with `timeouts.request: 30s` and a comment noting the trait *overwrites* it to `0s`.

Expected conclusion: detaching the trait is safe for rendering, **but the endpoint is mandatory** — an `a2a-agent` with no workload endpoint fails ComponentType validation. This is why Task 2 must add an endpoint branch rather than emitting none. Record both findings.

- [ ] **Step 3: Risk 3 — the dangling `traitEnvironmentConfigs` key**

Read `services/agent_manager.go:3684-3700`. `buildTraitEnvConfigs` unconditionally writes `instanceName(client.TraitAPIManagement)`. The doc comment asserts OpenChoreo resolves these keys against attached trait instances; that assertion is a comment, not observed behaviour.

Record: this is cheap to fix either way, so **the plan gates the key regardless** (Task 3). No live experiment is required. Note in the findings doc that the gating is defensive, not evidence-driven.

- [ ] **Step 4: Risk 4 — omitting `resilience` preserves the streaming-friendly default**

Read `/home/jhivandb/Dev/work/api-platform/gateway/it/features/agent_streaming.feature:19-21`:

> "A2A's streaming operations are the reason the Agent kind disables its route timeout by default and leans on idleTimeout instead."

and note that every scenario in the file sets only `resilience.idleTimeout`, never a request timeout.

Expected conclusion: confirmed by a feature file, not by the stale `schemas.md`. Omitting `spec.resilience` is correct. Record the citation.

- [ ] **Step 5: Risk 5 — `spec.vhost`**

Read `/home/jhivandb/Dev/work/api-platform/gateway/it/features/agent_card.feature:583-632` — the scenario "A configured vhost outranks the address the client dialled" deploys with `vhost: agents.example.com` and asserts the rewritten card advertises `http://agents.example.com/<context>/v1`.

Then read `agent-manager-service/models/gateway.go:35` (`Gateway.Vhost`) and `services/platform_gateway_service.go:210` (where it is set at registration).

Expected conclusion: `spec.vhost` is the host the gateway advertises in a rewritten passthrough card, so the environment's gateway `Vhost` is the right value. Record the citation. Also record the shape decision: emit `vhost` **only when the gateway's `Vhost` is non-empty**, as `MCPProxyDeploymentSpec.Vhost` is a `*string`.

- [ ] **Step 6: Bonus finding — `GET /deployments` reports nothing**

Read `controllers/gateway_internal_controller.go:342-360`. `GetDeployments` returns `{"deployments": []}` unconditionally; its own comment says deployment state "lives entirely in the RestApi/LlmProvider CRs… so there is nothing yet to report here".

Record this: the spec's §2 claim that the `deployments` row backs `syncDeployments` is only half true today. The row is still load-bearing — `GET /agents/{agentId}` reads it via `GetCurrentByGateway` (Task 10) — but bulk sync on gateway reconnect is inert for **every** kind, MCP included, until `GetDeployments` reports rows. **This is out of scope for M1** (it would change behaviour for LLM and MCP artifacts too). Record it as a known gap with a recommendation to file it separately.

- [ ] **Step 7: Write the characterization test for the trait gate**

This test fails now and passes after Task 3. It pins the exact behaviour Risk 3 is about.

Create `agent-manager-service/services/a2a_trait_gating_unit_test.go` (with the standard WSO2 2026 header):

```go
package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
)

// TestBuildTraitEnvConfigsOmitsAPIManagementForA2A pins the rule that an agent
// whose api-configuration trait is not attached must not carry a
// traitEnvironmentConfigs key naming it: OpenChoreo resolves those keys against
// attached trait instances, so a dangling key is config for a trait that is not
// there.
func TestBuildTraitEnvConfigsOmitsAPIManagementForA2A(t *testing.T) {
	apiKey := "my-agent-" + string(client.TraitAPIManagement)

	withTrait := buildTraitEnvConfigs("my-agent", nil, "", 0, false, false, true, "", true)
	require.Contains(t, withTrait, apiKey, "a REST agent still gets its api-configuration key")

	withoutTrait := buildTraitEnvConfigs("my-agent", nil, "", 0, false, false, true, "", false)
	assert.NotContains(t, withoutTrait, apiKey, "an a2a agent must not carry a key for a detached trait")
}
```

- [ ] **Step 8: Run it and watch it fail to compile**

Run: `cd agent-manager-service && go test ./services/... -run TestBuildTraitEnvConfigsOmitsAPIManagementForA2A`
Expected: FAIL — `too many arguments in call to buildTraitEnvConfigs`. That is the correct failure; the ninth parameter does not exist yet.

- [ ] **Step 9: Commit**

```bash
git add docs/superpowers/plans/2026-09-09-a2a-m1-risk-findings.md agent-manager-service/services/a2a_trait_gating_unit_test.go
git commit -m "docs: resolve the five open risks in the A2A M1 design

Adds a failing characterization test for the trait-env-config gate.

Claude-Session: https://claude.ai/code/session_01JXtNk6KuBdVKYBZj4bYeWo"
```

---

### Task 2: The `a2a-agent` subtype — constant, validation, endpoint

Makes the subtype a legal value that provisions a component with an endpoint and no OpenAPI schema.

**Files:**
- Modify: `agent-manager-service/utils/agent_types.go:36-40`
- Modify: `agent-manager-service/utils/utils.go:543-549`
- Modify: `agent-manager-service/clients/openchoreosvc/client/components.go:490-524`
- Test: `agent-manager-service/utils/agent_types_a2a_unit_test.go` (create)
- Test: `agent-manager-service/clients/openchoreosvc/client/a2a_endpoints_unit_test.go` (create)

**Interfaces:**
- Consumes: Task 1's finding that the ComponentType requires ≥1 endpoint.
- Produces:
  - `utils.AgentSubTypeA2A utils.AgentSubType = "a2a-agent"`
  - `utils.IsA2AAgentSubType(subType string) bool`
  - `buildEndpoints` emits exactly one endpoint for an `a2a-agent`, with `port` from `req.InputInterface.Port`, `basePath` from `req.InputInterface.BasePath` (defaulting to `/`), and **no** `schemaType` / `schemaFilePath` / `schemaContent` keys.

- [ ] **Step 1: Write the failing tests**

Create `agent-manager-service/utils/agent_types_a2a_unit_test.go` (WSO2 2026 header, package `utils`):

```go
package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func TestIsA2AAgentSubType(t *testing.T) {
	assert.True(t, IsA2AAgentSubType("a2a-agent"))
	assert.False(t, IsA2AAgentSubType("chat-api"))
	assert.False(t, IsA2AAgentSubType("custom-api"))
	assert.False(t, IsA2AAgentSubType(""))
}

func TestValidateAgentTypeAcceptsA2A(t *testing.T) {
	subType := string(AgentSubTypeA2A)
	err := validateAgentType(spec.AgentType{Type: string(AgentTypeAPI), SubType: &subType})
	assert.NoError(t, err)
}

// An a2a-agent serves its own agent card and has no OpenAPI document, so the
// custom-api schema requirement must not reach it.
func TestValidateInputInterfaceA2ANeedsNoSchema(t *testing.T) {
	subType := string(AgentSubTypeA2A)
	port := int32(9099)
	err := validateInputInterface(
		spec.InputInterface{Type: "HTTP", Port: &port},
		spec.AgentType{Type: string(AgentTypeAPI), SubType: &subType},
	)
	assert.NoError(t, err)
}
```

Before writing this file, open `utils/utils.go` and confirm the exact names and signatures of `validateAgentType` and `validateInputInterface` (search for `func validateAgentType` and `func validateInputInterface`) and the exact `spec.InputInterface` / `spec.AgentType` field types. Adjust the call sites in the test to match — do not change the production signatures.

Create `agent-manager-service/clients/openchoreosvc/client/a2a_endpoints_unit_test.go` (WSO2 2026 header, package `client`):

```go
package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// An a2a-agent must still declare exactly one workload endpoint: the agent-api
// ComponentType validates "size(workload.endpoints) > 0" and renders the
// component's Service and HTTPRoutes from it. It carries no OpenAPI schema.
func TestBuildEndpointsForA2AAgent(t *testing.T) {
	basePath := "/"
	port := int32(9099)
	req := CreateComponentRequest{
		Name: "trip-planner",
		AgentType: AgentTypeConfig{
			Type:    string(utils.AgentTypeAPI),
			SubType: string(utils.AgentSubTypeA2A),
		},
		InputInterface: &InputInterfaceConfig{
			Type:     string(utils.InputInterfaceTypeHTTP),
			Port:     port,
			BasePath: basePath,
		},
	}

	endpoints, err := buildEndpoints(req)
	require.NoError(t, err)
	require.Len(t, endpoints, 1)

	ep := endpoints[0]
	assert.Equal(t, "trip-planner-endpoint", ep["name"])
	assert.Equal(t, port, ep["port"])
	assert.Equal(t, "/", ep["basePath"])
	assert.NotContains(t, ep, "schemaType")
	assert.NotContains(t, ep, "schemaFilePath")
	assert.NotContains(t, ep, "schemaContent")
}
```

Before writing this, open `clients/openchoreosvc/client/components.go` and confirm the exact field names and types of `CreateComponentRequest`, `AgentTypeConfig` and `InputInterfaceConfig` (in particular whether `Port` is `int32` or `int`, and whether `InputInterface` is a pointer). Adjust the literal to match.

- [ ] **Step 2: Run both tests to verify they fail**

Run:
```
cd agent-manager-service
go test ./utils/... -run 'TestIsA2AAgentSubType|TestValidateAgentTypeAcceptsA2A|TestValidateInputInterfaceA2ANeedsNoSchema'
go test ./clients/openchoreosvc/client/... -run TestBuildEndpointsForA2AAgent
```
Expected: FAIL — `undefined: IsA2AAgentSubType`, `undefined: AgentSubTypeA2A`, and a zero-length endpoint slice.

- [ ] **Step 3: Add the constant and helper**

In `utils/agent_types.go`, extend the subtype block:

```go
const (
	AgentSubTypeChatAPI   AgentSubType = "chat-api"
	AgentSubTypeCustomAPI AgentSubType = "custom-api"
	// AgentSubTypeA2A is an agent that speaks the A2A protocol. It provisions
	// with the same agent-api component type as the other two but gets no REST
	// API: the api-configuration trait is not attached, and the agent is
	// published to the gateway as a kind: Agent resource instead.
	AgentSubTypeA2A AgentSubType = "a2a-agent"
)

// IsA2AAgentSubType reports whether a subtype string names an A2A agent. It
// exists so the several gates that branch on this subtype read the same and
// cannot drift apart.
func IsA2AAgentSubType(subType string) bool {
	return subType == string(AgentSubTypeA2A)
}
```

- [ ] **Step 4: Accept the subtype in validation**

In `utils/utils.go`, replace the two-value check at `:543-549`:

```go
	// Validate subtype for API agent type
	subType := StrPointerAsStr(agentType.SubType, "")
	if subType != string(AgentSubTypeChatAPI) &&
		subType != string(AgentSubTypeCustomAPI) &&
		subType != string(AgentSubTypeA2A) {
		return NewValidationErrorf(
			"The selected agent subtype is not supported for this agent type",
			"unsupported agent subtype for type %s: %s", agentType.Type, subType,
		)
	}
```

Leave `validateInputInterface` at `:794` untouched: its schema/basePath requirements are already keyed to `custom-api` alone, so `a2a-agent` falls through with only the shared `basePath` and `type` checks applied — which is the wanted behaviour.

- [ ] **Step 5: Add the endpoint branch**

In `clients/openchoreosvc/client/components.go`, after the `custom-api` block (currently ending at `:521`) and before `return endpoints, nil`:

```go
	// An A2A agent declares an endpoint but no OpenAPI document — it serves its
	// own Agent Card, and the platform stores no copy. The endpoint is still
	// mandatory: the agent-api ComponentType validates that at least one exists
	// and renders the component's Service and HTTPRoutes from it.
	if req.AgentType.Type == string(utils.AgentTypeAPI) &&
		utils.IsA2AAgentSubType(req.AgentType.SubType) && req.InputInterface != nil {
		basePath := req.InputInterface.BasePath
		if basePath == "" {
			basePath = "/"
		}
		endpoints = append(endpoints, map[string]any{
			"name":       fmt.Sprintf("%s-endpoint", req.Name),
			"port":       req.InputInterface.Port,
			"type":       req.InputInterface.Type,
			"basePath":   basePath,
			"visibility": DefaultEndpointVisibility,
		})
	}
```

- [ ] **Step 6: Run both tests to verify they pass**

Run:
```
cd agent-manager-service
go test ./utils/... -run 'TestIsA2AAgentSubType|TestValidateAgentTypeAcceptsA2A|TestValidateInputInterfaceA2ANeedsNoSchema'
go test ./clients/openchoreosvc/client/... -run TestBuildEndpointsForA2AAgent
```
Expected: PASS.

- [ ] **Step 7: Check the rebuild path takes the request's port**

Read `clients/openchoreosvc/client/builds.go:373-400`. `buildEndpointsFromInputInterface` uses the chat-API default port only when the subtype is `chat-api`, and otherwise takes `inputInterface.Port` — which is already right for `a2a-agent`. It also attaches `schemaFilePath`/`schemaType` only when `SchemaPath != ""`, which is empty for an a2a agent. **No change needed.** Add a one-line comment there naming `a2a-agent` alongside `custom-api` in the `else` branch so a future reader does not have to re-derive this:

```go
	// Use default port and basePath for chat-api agents, similar to buildEndpoints in components.go.
	// custom-api and a2a-agent both carry their own port and base path.
```

- [ ] **Step 8: Build and lint**

Run: `cd agent-manager-service && go build ./... && make lint`
Expected: clean.

- [ ] **Step 9: Commit**

```bash
git add agent-manager-service/utils agent-manager-service/clients/openchoreosvc/client
git commit -m "feat: add the a2a-agent subtype and its component endpoint

Claude-Session: https://claude.ai/code/session_01JXtNk6KuBdVKYBZj4bYeWo"
```

---

### Task 3: Detach `api-configuration` for A2A agents

Both trait-attach sites and the trait-env-config key are gated on the subtype. This is what makes an `a2a-agent` an agent with no REST API.

**Files:**
- Modify: `agent-manager-service/services/agent_manager.go:374`, `:474`, `:3202`, `:3229`, `:3332`, `:3684-3714`
- Test: `agent-manager-service/services/a2a_trait_gating_unit_test.go` (created in Task 1)
- Test: `agent-manager-service/tests/create_agent_a2a_test.go` (create)

**Interfaces:**
- Consumes: `utils.IsA2AAgentSubType` (Task 2).
- Produces: `buildTraitEnvConfigs(agentName string, policies []map[string]interface{}, artifactID string, resilienceTimeoutSeconds int32, isPythonBuildpack, isBallerinaBuildpack, autoInstrumentation bool, instrumentationImage string, attachAPIManagement bool) map[string]interface{}` — one new trailing `bool` parameter.

- [ ] **Step 1: Write the failing integration test**

Create `agent-manager-service/tests/create_agent_a2a_test.go`. It mirrors the docker-agent case in `tests/create_agent_test.go` (which asserts *two* traits, the second being `TraitAPIManagement`, at `:334-337`) but asserts the api-configuration trait is **absent**.

Start by reading `tests/create_agent_test.go:55-130` and `:300-338` in full, and copy its scaffolding — `apitestutils.CreateMockOpenChoreoClient()`, the `GetComponentFunc` override, `wiring.TestClients`, `apitestutils.MakeAppClientWithDeps`, the request URL and headers. The file must begin with `//go:build integration` followed by the WSO2 header.

```go
//go:build integration

// <WSO2 2026 header — copy verbatim from tests/create_agent_test.go>

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

var testAgentNameA2A = fmt.Sprintf("a2a-agent-%s", uuid.New().String()[:5])

// An a2a-agent has no REST API, so the api-configuration trait must not be
// attached at create. The positive assertion this mirrors is in
// create_agent_test.go, where a docker chat agent gets exactly two traits with
// TraitAPIManagement second.
func TestCreateA2AAgentOmitsAPIConfigurationTrait(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
	openChoreoClient.GetComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
		return &models.AgentResponse{
			UUID:         uuid.New().String(),
			Name:         componentName,
			ProjectName:  projectName,
			Provisioning: models.Provisioning{Type: "internal"},
			CreatedAt:    time.Now(),
		}, nil
	}

	testClients := wiring.TestClients{
		OpenChoreoClient: openChoreoClient,
		SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
	}
	app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

	reqBody := new(bytes.Buffer)
	require.NoError(t, json.NewEncoder(reqBody).Encode(map[string]interface{}{
		"name":        testAgentNameA2A,
		"displayName": "A2A Trip Planner",
		"description": "A2A Agent Description",
		"provisioning": map[string]interface{}{
			"type": "internal",
			"repository": map[string]interface{}{
				"url":     "https://github.com/example/a2a-agent",
				"branch":  "main",
				"appPath": "/",
			},
		},
		"agentType": map[string]interface{}{
			"type":    "agent-api",
			"subType": "a2a-agent",
		},
		"build": map[string]interface{}{
			"type":   "docker",
			"docker": map[string]interface{}{"dockerfilePath": "/Dockerfile"},
		},
		"inputInterface": map[string]interface{}{
			"type": "HTTP",
			"port": 9099,
		},
	}))

	req := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
		reqBody,
	)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)

	require.Equal(t, http.StatusAccepted, rr.Code, "body: %s", rr.Body.String())

	attachTraitsCalls := openChoreoClient.AttachTraitsCalls()
	require.Len(t, attachTraitsCalls, 1)
	for _, tr := range attachTraitsCalls[0].TraitRequests {
		require.NotEqual(t, client.TraitAPIManagement, tr.TraitType,
			"an a2a-agent must not be given the api-configuration trait")
	}
	require.NotEmpty(t, attachTraitsCalls[0].TraitRequests,
		"the instrumentation traits are still attached")
}
```

Read the surrounding test file and match its exact request path, auth header setup and `testOrgName`/`testProjName` usage. If the mock's request URL or body shape differs from the above, follow the file — it is the authority.

- [ ] **Step 2: Run both tests to verify they fail**

Run:
```
cd agent-manager-service
go test -tags=integration ./tests/... -run TestCreateA2AAgentOmitsAPIConfigurationTrait
go test ./services/... -run TestBuildTraitEnvConfigsOmitsAPIManagementForA2A
```
Expected: the integration test FAILs on `an a2a-agent must not be given the api-configuration trait`; the unit test still fails to compile.

- [ ] **Step 3: Gate the create-time attach**

In `services/agent_manager.go`, at `:374`, add the subtype read next to `isAPIAgent`:

```go
	isAPIAgent := req.AgentType != nil && req.AgentType.Type == string(utils.AgentTypeAPI)
	// An A2A agent is an API agent that gets no REST API: it is published to the
	// gateway as a kind: Agent resource instead, so the api-configuration trait
	// (which provisions the RestApi CRD) must not be attached.
	isA2AAgent := req.AgentType != nil && utils.IsA2AAgentSubType(utils.StrPointerAsStr(req.AgentType.SubType, ""))
```

Confirm `req.AgentType.SubType`'s type first — if it is a plain `string` rather than `*string`, drop the `StrPointerAsStr` wrapper.

Then change the gate at `:474`:

```go
	if isAPIAgent && !isA2AAgent {
```

- [ ] **Step 4: Gate the deploy-time attach**

At `:3202`, next to the existing `isAPIAgent`:

```go
	isAPIAgent := agent.Type.Type == string(utils.AgentTypeAPI)
	isA2AAgent := utils.IsA2AAgentSubType(agent.Type.SubType)
```

At `:3229`, change to:

```go
	if isAPIAgent && !isA2AAgent {
```

Leave the `isAPIAgent` gate at `:3323` (component-type env configs / `runtimeClassName`) **unchanged** — an A2A agent still runs in a sandbox and still needs its isolation tier.

- [ ] **Step 5: Gate the trait-env-config key**

In `buildTraitEnvConfigs` (`:3684`), add a trailing parameter and use it. Update the doc comment's first paragraph too:

```go
// attachAPIManagement gates the api-configuration entry. OpenChoreo resolves
// traitEnvironmentConfigs keys against ATTACHED trait instances, so an agent
// whose api-configuration trait was never attached (an a2a-agent) must not
// carry a key naming it — that would leave the release binding holding config
// for a trait that is not there.
func buildTraitEnvConfigs(agentName string, policies []map[string]interface{}, artifactID string, resilienceTimeoutSeconds int32, isPythonBuildpack, isBallerinaBuildpack bool, autoInstrumentation bool, instrumentationImage string, attachAPIManagement bool) map[string]interface{} {
	instanceName := func(traitType client.TraitType) string {
		return agentName + "-" + string(traitType)
	}
	traitEnvConfigs := map[string]interface{}{}
	if attachAPIManagement {
		apiTraitCfg := map[string]interface{}{
			"policies": policies,
		}
		if artifactID != "" {
			apiTraitCfg["artifactId"] = artifactID
		}
		if resilienceTimeoutSeconds > 0 {
			apiTraitCfg["resilienceTimeout"] = client.FormatResilienceTimeout(resilienceTimeoutSeconds)
		}
		traitEnvConfigs[instanceName(client.TraitAPIManagement)] = apiTraitCfg
	}
	if isPythonBuildpack {
```

…leaving the rest of the function body unchanged.

- [ ] **Step 6: Update every caller**

Run `grep -rn 'buildTraitEnvConfigs(' agent-manager-service/ --include='*.go'` and update each call site. The deploy call at `:3222` becomes:

```go
	deployTraitEnvConfigs := buildTraitEnvConfigs(agentName, policies, "", resilienceTimeoutSeconds, isPythonBuildpack, isBallerinaBuildpack, enableAutoInstrumentation, deployInstrumentationImage, !isA2AAgent)
```

Every other caller (promote, and any test helper) passes `true` unless it too has an A2A-aware subtype in scope — check each one and derive the flag the same way where it does. Do not pass `true` blindly at a site that handles arbitrary agents; if a caller does not know the subtype, read it from the agent it already holds.

- [ ] **Step 7: Run both tests to verify they pass**

Run:
```
cd agent-manager-service
go test ./services/... -run TestBuildTraitEnvConfigsOmitsAPIManagementForA2A
go test -tags=integration ./tests/... -run TestCreateA2AAgentOmitsAPIConfigurationTrait
```
Expected: PASS.

- [ ] **Step 8: Confirm nothing else regressed**

Run:
```
cd agent-manager-service
go build ./... && go test ./services/... && go test -tags=integration ./tests/... -run 'TestCreateAgent|TestDeployAgent|TestPromoteAgent'
```
Expected: PASS. If an existing test broke, the fix is in the call-site update from Step 6, not in the test.

- [ ] **Step 9: Commit**

```bash
git add agent-manager-service/services agent-manager-service/tests
git commit -m "feat: do not attach api-configuration to a2a agents

Gates both trait-attach sites and the traitEnvironmentConfigs key on the
subtype, so an a2a agent's release binding carries no config for a trait
that is not attached.

Claude-Session: https://claude.ai/code/session_01JXtNk6KuBdVKYBZj4bYeWo"
```

---

### Task 4: The `Agent` YAML builder

The gateway resource itself, as a pure function over its inputs. This is the cross-repo contract, so it gets a golden file.

**Files:**
- Create: `agent-manager-service/services/a2a_agent_deployment.go`
- Create: `agent-manager-service/services/a2a_agent_deployment_test.go`
- Create: `agent-manager-service/services/testdata/a2a_agent_golden.yaml`

**Interfaces:**
- Consumes: Task 1's findings on `resilience`, `vhost` and the card default; the existing `buildPolicies` (`services/agent_manager.go:3614`) and `DeploymentMetadata` (`services/gateway_internal_service.go:66`).
- Produces:
  - `type A2AAgentDeploymentYAML struct { ApiVersion, Kind string; Metadata DeploymentMetadata; Spec A2AAgentDeploymentSpec }`
  - `type A2AAgentDeploymentSpec struct { DisplayName, Version, Context string; Vhost *string; Upstream A2AUpstream; A2A A2AConfig }`
  - `type A2AUpstream struct { URL string }`
  - `type A2AConfig struct { ProtocolVersion string; OperationConfigs A2AOperationConfigs }`
  - `type A2AOperationConfigs struct { Transports []A2ATransport; Policies []map[string]interface{} }`
  - `type A2ATransport struct { ProtocolBinding string; PathPrefix string }`
  - `type A2AAgentDeploymentInput struct { ArtifactName, DisplayName, AgentName, Vhost, UpstreamURL string; Policies []map[string]interface{} }`
  - `func buildA2AAgentDeploymentYAML(in A2AAgentDeploymentInput) (*A2AAgentDeploymentYAML, error)`
  - `func generateA2AAgentDeploymentYAML(in A2AAgentDeploymentInput) (string, error)`
  - `func a2aAgentEnvArtifactName(projectName, agentName, envID string) string`

- [ ] **Step 1: Write the failing tests**

Create `agent-manager-service/services/a2a_agent_deployment_test.go` (WSO2 2026 header, package `services`):

```go
package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func a2aInput() A2AAgentDeploymentInput {
	return A2AAgentDeploymentInput{
		ArtifactName: "checkout-trip-planner-0192f4c19a7d7c3e",
		DisplayName:  "Trip Planner",
		AgentName:    "trip-planner",
		Vhost:        "agents.example.com",
		UpstreamURL:  "http://trip-planner.dp-default:9099",
		Policies: []map[string]interface{}{
			{"name": "cors", "version": "v1", "params": map[string]interface{}{"allowOrigins": []string{"*"}}},
			{"name": "api-key-auth", "version": "v1"},
		},
	}
}

// The emitted resource is a cross-repo contract with the gateway. A golden file
// is what makes a drift in it visible in a diff rather than at runtime.
func TestGenerateA2AAgentDeploymentYAMLMatchesGolden(t *testing.T) {
	got, err := generateA2AAgentDeploymentYAML(a2aInput())
	require.NoError(t, err)

	goldenPath := filepath.Join("testdata", "a2a_agent_golden.yaml")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0o600))
	}
	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	assert.Equal(t, string(want), got)
}

// Both transports, in this order, at these prefixes. M1 fixes them.
func TestBuildA2AAgentDeploymentYAMLTransports(t *testing.T) {
	got, err := buildA2AAgentDeploymentYAML(a2aInput())
	require.NoError(t, err)

	transports := got.Spec.A2A.OperationConfigs.Transports
	require.Len(t, transports, 2)
	assert.Equal(t, "JSONRPC", transports[0].ProtocolBinding)
	assert.Equal(t, "/rpc", transports[0].PathPrefix)
	assert.Equal(t, "HTTP+JSON", transports[1].ProtocolBinding)
	assert.Equal(t, "/rest", transports[1].PathPrefix)
	assert.Equal(t, "1.0", got.Spec.A2A.ProtocolVersion)
}

// The gateway's default for a passthrough card is to rewrite its URLs, and the
// Agent kind disables its route timeout so streaming operations survive. Both
// are what M1 wants, so neither block is written — emitting them is the only
// way to get them wrong.
func TestGenerateA2AAgentDeploymentYAMLOmitsCardAndResilience(t *testing.T) {
	got, err := generateA2AAgentDeploymentYAML(a2aInput())
	require.NoError(t, err)
	assert.NotContains(t, got, "agentCard")
	assert.NotContains(t, got, "resilience")
}

func TestBuildA2AAgentDeploymentYAMLContextIsAgentName(t *testing.T) {
	got, err := buildA2AAgentDeploymentYAML(a2aInput())
	require.NoError(t, err)
	assert.Equal(t, "/trip-planner", got.Spec.Context)
	assert.Equal(t, "v1.0", got.Spec.Version)
	assert.Equal(t, "Trip Planner", got.Spec.DisplayName)
	assert.Equal(t, "checkout-trip-planner-0192f4c19a7d7c3e", got.Metadata.Name)
}

// An empty upstream would produce an Agent that routes nowhere. Refuse rather
// than publish it — the reconciler retries until the binding reports one.
func TestBuildA2AAgentDeploymentYAMLRefusesEmptyUpstream(t *testing.T) {
	in := a2aInput()
	in.UpstreamURL = "   "
	_, err := buildA2AAgentDeploymentYAML(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upstream")
}

// vhost is optional: an environment whose gateway declares none must not emit
// an empty string, which the gateway would read as a host of "".
func TestBuildA2AAgentDeploymentYAMLOmitsEmptyVhost(t *testing.T) {
	in := a2aInput()
	in.Vhost = ""
	got, err := buildA2AAgentDeploymentYAML(in)
	require.NoError(t, err)
	assert.Nil(t, got.Spec.Vhost)

	out, err := generateA2AAgentDeploymentYAML(in)
	require.NoError(t, err)
	assert.NotContains(t, out, "vhost")
}

// The DB handle uses slashes, which are illegal in a Kubernetes name. The
// gateway resource name is derived with dashes and a compacted environment
// UUID, exactly as mcpProxyEnvArtifactHandle does.
func TestA2AAgentEnvArtifactName(t *testing.T) {
	name := a2aAgentEnvArtifactName("checkout", "trip-planner", "0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f")
	assert.Equal(t, "checkout-trip-planner-0192f4c19a7d7c3eb4f21a2b3c4d5e6f", name)
	assert.NotContains(t, name, "/")
}

// The gateway parses these values verbatim; unmarshalling the emitted document
// back is what proves the yaml tags are what the contract needs.
func TestGenerateA2AAgentDeploymentYAMLRoundTrips(t *testing.T) {
	out, err := generateA2AAgentDeploymentYAML(a2aInput())
	require.NoError(t, err)

	var back A2AAgentDeploymentYAML
	require.NoError(t, yaml.Unmarshal([]byte(out), &back))
	assert.Equal(t, apiVersionA2AAgent, back.ApiVersion)
	assert.Equal(t, kindA2AAgent, back.Kind)
	assert.Equal(t, "http://trip-planner.dp-default:9099", back.Spec.Upstream.URL)
	require.Len(t, back.Spec.A2A.OperationConfigs.Policies, 2)
	assert.Equal(t, "cors", back.Spec.A2A.OperationConfigs.Policies[0]["name"])
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd agent-manager-service && go test ./services/... -run 'A2AAgent'`
Expected: FAIL — `undefined: A2AAgentDeploymentInput` and friends.

- [ ] **Step 3: Write the builder**

Create `agent-manager-service/services/a2a_agent_deployment.go` (WSO2 2026 header, package `services`):

```go
package services

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	apiVersionA2AAgent = "gateway.api-platform.wso2.com/v1"
	kindA2AAgent       = "Agent"

	// a2aProtocolVersion is the single A2A version an M1 agent exposes. The
	// gateway performs no version conversion, so this is a platform constant
	// rather than a per-agent setting.
	a2aProtocolVersion = "1.0"

	// a2aAgentVersion mirrors what the api-configuration trait sets for a REST
	// agent, so A2A and REST agents keep consistent artifact versions.
	a2aAgentVersion = "v1.0"

	// The two bindings the gateway supports, at platform-chosen prefixes. There
	// is no gRPC binding. M1 exposes both and makes neither configurable.
	a2aTransportJSONRPC  = "JSONRPC"
	a2aTransportHTTPJSON = "HTTP+JSON"
	a2aPathPrefixJSONRPC = "/rpc"
	a2aPathPrefixHTTPRPC = "/rest"
)

// A2AAgentDeploymentYAML is the kind: Agent resource agent-manager publishes to
// the gateway. It is a structural sibling of MCPProxyDeploymentYAML.
//
// Two blocks are deliberately absent and must stay absent in M1:
//
//   - agentCard. The gateway's default for a proxied card is passthrough WITH
//     URL rewriting, which is exactly what M1 wants — an un-rewritten card
//     advertises the agent's own address and sends every client past the
//     gateway. Writing the block out would only add a way to get it wrong.
//   - resilience. The gateway disables the Agent route's request timeout by
//     default precisely because A2A streaming operations are long-lived.
//     Writing agent-manager's resilienceTimeoutSeconds here would override that
//     and sever every stream at the timeout.
type A2AAgentDeploymentYAML struct {
	ApiVersion string                 `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                 `yaml:"kind" json:"kind"`
	Metadata   DeploymentMetadata     `yaml:"metadata" json:"metadata"`
	Spec       A2AAgentDeploymentSpec `yaml:"spec" json:"spec"`
}

// A2AAgentDeploymentSpec is the spec section of the Agent resource.
type A2AAgentDeploymentSpec struct {
	DisplayName string      `yaml:"displayName" json:"displayName"`
	Version     string      `yaml:"version" json:"version"`
	Context     string      `yaml:"context" json:"context"`
	Vhost       *string     `yaml:"vhost,omitempty" json:"vhost,omitempty"`
	Upstream    A2AUpstream `yaml:"upstream" json:"upstream"`
	A2A         A2AConfig   `yaml:"a2a" json:"a2a"`
}

// A2AUpstream is the address the gateway dials to reach the agent.
type A2AUpstream struct {
	URL string `yaml:"url" json:"url"`
}

// A2AConfig carries the protocol version and the per-operation configuration.
type A2AConfig struct {
	ProtocolVersion  string              `yaml:"protocolVersion" json:"protocolVersion"`
	OperationConfigs A2AOperationConfigs `yaml:"operationConfigs" json:"operationConfigs"`
}

// A2AOperationConfigs holds the transport bindings and the agent-wide policy
// chain. Per-operation policy overrides exist in the gateway schema but are out
// of scope for M1.
type A2AOperationConfigs struct {
	Transports []A2ATransport           `yaml:"transports" json:"transports"`
	Policies   []map[string]interface{} `yaml:"policies" json:"policies"`
}

// A2ATransport binds one A2A protocol to a gateway-facing path prefix.
type A2ATransport struct {
	ProtocolBinding string `yaml:"protocolBinding" json:"protocolBinding"`
	PathPrefix      string `yaml:"pathPrefix" json:"pathPrefix"`
}

// A2AAgentDeploymentInput is everything the builder needs, already resolved.
// Keeping resolution out of the builder is what lets the emitted contract be
// tested as a pure function against a golden file.
type A2AAgentDeploymentInput struct {
	// ArtifactName is the sanitized per-environment resource name; see
	// a2aAgentEnvArtifactName.
	ArtifactName string
	DisplayName  string
	// AgentName is the component name, which becomes the URL context.
	AgentName string
	// Vhost is the environment's gateway vhost. Empty means "omit".
	Vhost string
	// UpstreamURL comes from the release binding's status. Empty is an error.
	UpstreamURL string
	// Policies is the output of buildPolicies — already in the gateway's
	// {name, version, params} shape, so no translation layer is needed.
	Policies []map[string]interface{}
}

// a2aAgentEnvArtifactName builds the per-environment Kubernetes resource name
// for an agent's Agent resource.
//
// It cannot reuse agentEnvAPIArtifactHandle: that builds "project/agent/envID"
// with slashes, which is fine as a database handle and illegal as a Kubernetes
// name. The shape here follows mcpProxyEnvArtifactHandle — dashes, with the
// environment UUID's hyphens stripped — so the two publication paths name
// resources the same way.
func a2aAgentEnvArtifactName(projectName, agentName, envID string) string {
	suffix := strings.ReplaceAll(strings.TrimSpace(envID), "-", "")
	return fmt.Sprintf("%s-%s-%s", strings.TrimSpace(projectName), strings.TrimSpace(agentName), suffix)
}

// generateA2AAgentDeploymentYAML renders the Agent resource as YAML.
func generateA2AAgentDeploymentYAML(in A2AAgentDeploymentInput) (string, error) {
	deployment, err := buildA2AAgentDeploymentYAML(in)
	if err != nil {
		return "", err
	}
	yamlBytes, err := yaml.Marshal(deployment)
	if err != nil {
		return "", fmt.Errorf("failed to marshal A2A agent deployment YAML: %w", err)
	}
	return string(yamlBytes), nil
}

// buildA2AAgentDeploymentYAML assembles the Agent resource.
//
// An empty upstream is refused rather than emitted: the release binding
// publishes its ServiceURL only once it has reconciled, and an Agent published
// before then would route nowhere while looking healthy. The publication
// reconciler retries instead.
func buildA2AAgentDeploymentYAML(in A2AAgentDeploymentInput) (*A2AAgentDeploymentYAML, error) {
	upstreamURL := strings.TrimSpace(in.UpstreamURL)
	if upstreamURL == "" {
		return nil, fmt.Errorf("refusing to publish agent %q: upstream url is not available yet", in.ArtifactName)
	}

	var vhost *string
	if trimmed := strings.TrimSpace(in.Vhost); trimmed != "" {
		vhost = &trimmed
	}

	// Non-nil so "no authentication and no CORS" marshals to an empty array
	// rather than null, matching what buildPolicies guarantees for the trait.
	policies := in.Policies
	if policies == nil {
		policies = []map[string]interface{}{}
	}

	return &A2AAgentDeploymentYAML{
		ApiVersion: apiVersionA2AAgent,
		Kind:       kindA2AAgent,
		Metadata:   DeploymentMetadata{Name: in.ArtifactName},
		Spec: A2AAgentDeploymentSpec{
			DisplayName: in.DisplayName,
			Version:     a2aAgentVersion,
			// Matches what the api-configuration trait sets for a REST agent
			// (buildAPIConfigurationTraitParameters), so A2A and REST agents
			// keep consistent URL shapes.
			Context:  "/" + strings.TrimPrefix(strings.TrimSpace(in.AgentName), "/"),
			Vhost:    vhost,
			Upstream: A2AUpstream{URL: upstreamURL},
			A2A: A2AConfig{
				ProtocolVersion: a2aProtocolVersion,
				OperationConfigs: A2AOperationConfigs{
					Transports: []A2ATransport{
						{ProtocolBinding: a2aTransportJSONRPC, PathPrefix: a2aPathPrefixJSONRPC},
						{ProtocolBinding: a2aTransportHTTPJSON, PathPrefix: a2aPathPrefixHTTPRPC},
					},
					Policies: policies,
				},
			},
		},
	}, nil
}
```

- [ ] **Step 4: Generate the golden file, then read it**

Run:
```
cd agent-manager-service
mkdir -p services/testdata
UPDATE_GOLDEN=1 go test ./services/... -run TestGenerateA2AAgentDeploymentYAMLMatchesGolden
cat services/testdata/a2a_agent_golden.yaml
```

Read the output and check it by eye against the spec's §2 YAML sketch: `apiVersion`, `kind: Agent`, `metadata.name`, `spec.displayName/version/context/vhost`, `spec.upstream.url`, `spec.a2a.protocolVersion: "1.0"`, the two transports in order, the two policies. There must be **no** `agentCard` and **no** `resilience`. If anything is off, fix the builder and regenerate — do not edit the golden file by hand.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd agent-manager-service && go test ./services/... -run 'A2AAgent'`
Expected: PASS, all eight.

- [ ] **Step 6: Lint**

Run: `cd agent-manager-service && make lint`
Expected: clean. `exhaustruct` may require every field of the struct literals to be named — the code above already names all of them.

- [ ] **Step 7: Commit**

```bash
git add agent-manager-service/services
git commit -m "feat: build the kind: Agent gateway resource for a2a agents

Adds a golden file for the emitted YAML: it is a cross-repo contract with
the gateway, and a golden is what makes a drift in it visible.

Claude-Session: https://claude.ai/code/session_01JXtNk6KuBdVKYBZj4bYeWo"
```

---

### Task 5: Broadcast the Agent events

The two control-plane events, with the payload shapes the gateway already parses.

**Files:**
- Modify: `agent-manager-service/models/gateway_compat.go` (append after the MCP event types at `:84-95`)
- Modify: `agent-manager-service/services/gateway_events_service.go` (append after `BroadcastMCPProxyDeletionEvent` at `:141`)
- Create: `agent-manager-service/services/a2a_agent_events_unit_test.go`

**Interfaces:**
- Consumes: `GatewayEventsService.broadcastEvent` (`services/gateway_events_service.go:71`).
- Produces:
  - `models.AgentDeploymentEvent{AgentID, DeploymentID string; PerformedAt time.Time}` with json tags `agentId`, `deploymentId`, `performedAt`
  - `models.AgentDeletionEvent{AgentID string}` with json tag `agentId`
  - `(*GatewayEventsService).BroadcastAgentDeploymentEvent(gatewayID string, event *models.AgentDeploymentEvent) error`
  - `(*GatewayEventsService).BroadcastAgentDeletionEvent(gatewayID string, event *models.AgentDeletionEvent) error`

- [ ] **Step 1: Confirm the payload shapes against the gateway**

Read `/home/jhivandb/Dev/work/api-platform/gateway/gateway-controller/pkg/controlplane/events.go:288-329` and confirm, field by field, that `AgentDeployedEventPayload` is `{agentId string, deploymentId string, performedAt time.Time}` and `AgentDeletedEventPayload` is `{agentId string}`. Then read `client.go:1481-1486` and confirm the three event-name strings are exactly `agent.deployed`, `agent.undeployed`, `agent.deleted`.

If any of this differs from the plan, **stop and report** — the wire format is not negotiable from this side.

- [ ] **Step 2: Write the failing test**

Create `agent-manager-service/services/a2a_agent_events_unit_test.go` (WSO2 2026 header, package `services`):

```go
package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// recordingEventHub captures what would be published, so the event's wire shape
// can be asserted without a live hub.
type recordingEventHub struct {
	published []eventhub.Event
}

func (h *recordingEventHub) PublishEvent(gatewayID string, evt eventhub.Event) error {
	h.published = append(h.published, evt)
	return nil
}

// The gateway parses these names and payloads verbatim
// (pkg/controlplane/events.go and client.go's dispatch switch); they are fixed
// by that code, not chosen here.
func TestBroadcastAgentDeploymentEvent(t *testing.T) {
	hub := &recordingEventHub{}
	svc := NewGatewayEventsService(hub)

	performedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	err := svc.BroadcastAgentDeploymentEvent("gw-1", &models.AgentDeploymentEvent{
		AgentID:      "0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f",
		DeploymentID: "dep-1",
		PerformedAt:  performedAt,
	})
	require.NoError(t, err)
	require.Len(t, hub.published, 1)

	evt := hub.published[0]
	assert.Equal(t, eventhub.EventType("agent.deployed"), evt.EventType)
	assert.Equal(t, "CREATE", evt.Action)
	assert.Equal(t, "0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f", evt.EntityID)

	var envelope struct {
		Type    string `json:"type"`
		Payload struct {
			AgentID      string    `json:"agentId"`
			DeploymentID string    `json:"deploymentId"`
			PerformedAt  time.Time `json:"performedAt"`
		} `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(evt.EventData), &envelope))
	assert.Equal(t, "agent.deployed", envelope.Type)
	assert.Equal(t, "0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f", envelope.Payload.AgentID)
	assert.Equal(t, "dep-1", envelope.Payload.DeploymentID)
	assert.True(t, performedAt.Equal(envelope.Payload.PerformedAt))
}

func TestBroadcastAgentDeletionEvent(t *testing.T) {
	hub := &recordingEventHub{}
	svc := NewGatewayEventsService(hub)

	err := svc.BroadcastAgentDeletionEvent("gw-1", &models.AgentDeletionEvent{
		AgentID: "0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f",
	})
	require.NoError(t, err)
	require.Len(t, hub.published, 1)

	evt := hub.published[0]
	assert.Equal(t, eventhub.EventType("agent.deleted"), evt.EventType)
	assert.Equal(t, "DELETE", evt.Action)

	var envelope struct {
		Type    string `json:"type"`
		Payload struct {
			AgentID string `json:"agentId"`
		} `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(evt.EventData), &envelope))
	assert.Equal(t, "agent.deleted", envelope.Type)
	assert.Equal(t, "0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f", envelope.Payload.AgentID)
}
```

Open `agent-manager-service/eventhub/` first and confirm the `EventHub` interface's full method set. If it has methods beyond `PublishEvent`, `recordingEventHub` must implement all of them — add no-op implementations returning zero values, with a comment saying the test only exercises publication.

- [ ] **Step 3: Run to verify it fails**

Run: `cd agent-manager-service && go test ./services/... -run 'TestBroadcastAgent'`
Expected: FAIL — `undefined: models.AgentDeploymentEvent`.

- [ ] **Step 4: Add the event models**

Append to `agent-manager-service/models/gateway_compat.go`:

```go
// AgentDeploymentEvent represents an A2A Agent deployment event.
//
// The field names and JSON tags are fixed by the gateway's
// AgentDeployedEventPayload (gateway-controller/pkg/controlplane/events.go);
// they are matched here, not chosen.
type AgentDeploymentEvent struct {
	AgentID      string    `json:"agentId"`
	DeploymentID string    `json:"deploymentId"`
	PerformedAt  time.Time `json:"performedAt"`
}

// AgentDeletionEvent represents an A2A Agent deletion event. Matches the
// gateway's AgentDeletedEventPayload, which carries only the agent ID.
type AgentDeletionEvent struct {
	AgentID string `json:"agentId"`
}
```

Confirm `time` is already imported in that file; add it if not.

- [ ] **Step 5: Add the broadcast methods**

Append to `agent-manager-service/services/gateway_events_service.go`, after `BroadcastMCPProxyDeletionEvent`:

```go
// BroadcastAgentDeploymentEvent tells a gateway that an A2A Agent has been
// deployed. The gateway responds by fetching /agents/{agentId} back from this
// service, so agentId must be the artifact UUID that route is keyed on.
func (s *GatewayEventsService) BroadcastAgentDeploymentEvent(gatewayID string, event *models.AgentDeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "agent.deployed", "CREATE", event.AgentID, event)
}

// BroadcastAgentDeletionEvent tells a gateway to drop an A2A Agent.
//
// There is deliberately no BroadcastAgentUndeploymentEvent. The gateway accepts
// agent.undeployed and guards it against a mismatched deployment ID and a stale
// timestamp — guards delete does not have — but agent-manager broadcasts no
// mcpproxy.undeployed today either, and M1 follows that precedent rather than
// introducing a second teardown path. Worth revisiting.
func (s *GatewayEventsService) BroadcastAgentDeletionEvent(gatewayID string, event *models.AgentDeletionEvent) error {
	return s.broadcastEvent(gatewayID, "agent.deleted", "DELETE", event.AgentID, event)
}
```

- [ ] **Step 6: Run to verify it passes**

Run: `cd agent-manager-service && go test ./services/... -run 'TestBroadcastAgent'`
Expected: PASS.

- [ ] **Step 7: Build and lint, then commit**

Run: `cd agent-manager-service && go build ./... && make lint`

```bash
git add agent-manager-service/models agent-manager-service/services
git commit -m "feat: broadcast agent.deployed and agent.deleted to gateways

Payload shapes are fixed by the gateway's controlplane/events.go and are
matched here rather than invented.

Claude-Session: https://claude.ai/code/session_01JXtNk6KuBdVKYBZj4bYeWo"
```

---

### Task 6: The publication queue — model, migration, repository

`upstream.url` is only readable after the release binding reconciles, so publication cannot happen inside the deploy call. This task adds the durable queue that lets a deploy record its intent and a reconciler act on it later. It is the concrete answer to the ordering problem the spec raises.

**Files:**
- Create: `agent-manager-service/models/a2a_publication.go`
- Create: `agent-manager-service/db_migrations/044_create_a2a_publications.go`
- Modify: `agent-manager-service/db_migrations/migration_list.go`
- Create: `agent-manager-service/repositories/a2a_publication_repository.go`
- Create: `agent-manager-service/repositories/a2a_publication_repository_unit_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `models.A2APublicationStatus` with values `A2APublicationStatusPending`, `A2APublicationStatusPublished`, `A2APublicationStatusFailed`
  - `models.A2APublication{ID uuid.UUID; OUID, ProjectName, AgentName, EnvironmentName string; EnvironmentUUID, ArtifactUUID uuid.UUID; Status A2APublicationStatus; AttemptCount int; LastError string; NextAttemptAt *time.Time; CreatedAt, UpdatedAt time.Time}` on table `a2a_publications`
  - `repositories.A2APublicationRepository` interface:
    - `Enqueue(ctx context.Context, pub *models.A2APublication) error` — upsert on `(ou_id, project_name, agent_name, environment_name)`, resetting to pending with attempt 0
    - `FindDue(ctx context.Context, now time.Time, limit int) ([]models.A2APublication, error)`
    - `MarkPublished(ctx context.Context, id uuid.UUID) error`
    - `MarkAttemptFailed(ctx context.Context, id uuid.UUID, lastErr string, nextAttemptAt time.Time) error`
    - `MarkFailed(ctx context.Context, id uuid.UUID, lastErr string) error`
    - `DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error`
  - `repositories.NewA2APublicationRepository(db *gorm.DB) A2APublicationRepository`
  - A `//go:generate moq` directive on the interface, matching the neighbouring repositories.

- [ ] **Step 1: Read the shape this mirrors**

Read `agent-manager-service/models/agent_thunder_client.go:51-80` and `agent-manager-service/db_migrations/042_create_env_thunder_urls.go` in full. The model below follows the first; the migration follows the second's structure (`var migrationNNN = migration{ID: NN, Migrate: func(db *gorm.DB) error { return db.Transaction(...) }}` with `runSQL`).

Also open one existing repository — `repositories/agent_thunder_client_repository.go` — and copy its interface/struct/constructor layout and its `//go:generate moq` directive line verbatim in shape.

- [ ] **Step 2: Write the model**

Create `agent-manager-service/models/a2a_publication.go` (WSO2 2026 header):

```go
package models

import (
	"time"

	"github.com/google/uuid"
)

// A2APublicationStatus is where one agent-environment pair stands in the
// publish-to-gateway cycle.
type A2APublicationStatus string

const (
	// A2APublicationStatusPending means the deploy has been recorded but the
	// Agent resource has not been emitted yet — normally because the release
	// binding has not published its ServiceURL.
	A2APublicationStatusPending A2APublicationStatus = "pending"
	// A2APublicationStatusPublished means the Agent resource was written to a
	// deployments row and broadcast.
	A2APublicationStatusPublished A2APublicationStatus = "published"
	// A2APublicationStatusFailed means the attempt budget was exhausted. The row
	// is kept so an operator can see which agent never reached its gateway.
	A2APublicationStatusFailed A2APublicationStatus = "failed"
)

// A2APublication is one agent-environment pair's outstanding gateway
// publication.
//
// This table exists because upstream.url is read from the release binding's
// status, which is only populated once the binding reconciles — several minutes
// after the deploy call returns. Publishing inline would emit an Agent with an
// empty upstream that routes nowhere, and holding the deploy request open until
// the binding is ready would make every deploy slow and still lose the work on
// a process restart. Recording the intent durably is what lets a background
// reconciler finish the job.
type A2APublication struct {
	ID              uuid.UUID            `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID            string               `gorm:"column:ou_id;not null"`
	ProjectName     string               `gorm:"column:project_name;not null"`
	AgentName       string               `gorm:"column:agent_name;not null"`
	EnvironmentName string               `gorm:"column:environment_name;not null"`
	EnvironmentUUID uuid.UUID            `gorm:"column:environment_uuid;type:uuid;not null"`
	// ArtifactUUID is the per-environment models.KindAgent artifact row. It is
	// the agentId the gateway is told about and the key its API keys are bound
	// to, so it is captured at enqueue time rather than re-derived later.
	ArtifactUUID  uuid.UUID            `gorm:"column:artifact_uuid;type:uuid;not null"`
	Status        A2APublicationStatus `gorm:"column:status;not null;default:'pending'"`
	AttemptCount  int                  `gorm:"column:attempt_count;not null;default:0"`
	LastError     string               `gorm:"column:last_error;not null;default:''"`
	NextAttemptAt *time.Time           `gorm:"column:next_attempt_at"`
	CreatedAt     time.Time            `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt     time.Time            `gorm:"column:updated_at;not null;default:NOW()"`
}

func (A2APublication) TableName() string { return "a2a_publications" }
```

- [ ] **Step 3: Write the migration**

Create `agent-manager-service/db_migrations/044_create_a2a_publications.go` (WSO2 2026 header):

```go
package dbmigrations

import (
	"gorm.io/gorm"
)

// a2a_publications is the queue of A2A agents whose gateway Agent resource is
// still to be emitted.
//
// One row per (org, project, agent, environment): a redeploy resets the
// existing row to pending rather than adding a second, because publication is
// idempotent — it always emits the agent's current state, so two outstanding
// rows for the same pair would only do the same work twice.
//
// The (status, next_attempt_at) index is what the reconciler's due-scan reads;
// without it the scan degrades to a full table scan every tick.
var migration044 = migration{
	ID: 44,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			createTable := `
			CREATE TABLE IF NOT EXISTS a2a_publications (
				id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				ou_id            VARCHAR(255) NOT NULL,
				project_name     VARCHAR(255) NOT NULL,
				agent_name       VARCHAR(255) NOT NULL,
				environment_name VARCHAR(255) NOT NULL,
				environment_uuid UUID NOT NULL,
				artifact_uuid    UUID NOT NULL,
				status           VARCHAR(32) NOT NULL DEFAULT 'pending',
				attempt_count    INTEGER NOT NULL DEFAULT 0,
				last_error       TEXT NOT NULL DEFAULT '',
				next_attempt_at  TIMESTAMPTZ,
				created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

				CONSTRAINT uq_a2a_publications_agent_env
					UNIQUE (ou_id, project_name, agent_name, environment_name)
			)`
			if err := runSQL(tx, createTable); err != nil {
				return err
			}

			createIndex := `
			CREATE INDEX IF NOT EXISTS idx_a2a_publications_due
				ON a2a_publications (status, next_attempt_at)`
			return runSQL(tx, createIndex)
		})
	},
}
```

Then append `migration044,` to the slice in `db_migrations/migration_list.go`.

- [ ] **Step 4: Write the repository**

Create `agent-manager-service/repositories/a2a_publication_repository.go` (WSO2 2026 header). Match the `//go:generate moq` directive line format used by the neighbouring repository file you read in Step 1.

```go
package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

//go:generate moq -out a2a_publication_repository_moq.go . A2APublicationRepository

// A2APublicationRepository is the queue of outstanding A2A gateway publications.
type A2APublicationRepository interface {
	// Enqueue records that an agent-environment pair needs publishing, resetting
	// any existing row for that pair to pending with a fresh attempt budget. A
	// redeploy must re-publish, and a pair that previously exhausted its budget
	// must get another chance.
	Enqueue(ctx context.Context, pub *models.A2APublication) error
	// FindDue returns pending rows whose next attempt time has arrived, oldest
	// first, capped at limit.
	FindDue(ctx context.Context, now time.Time, limit int) ([]models.A2APublication, error)
	MarkPublished(ctx context.Context, id uuid.UUID) error
	// MarkAttemptFailed records a retryable failure and schedules the next try.
	MarkAttemptFailed(ctx context.Context, id uuid.UUID, lastErr string, nextAttemptAt time.Time) error
	// MarkFailed ends the retry cycle. The row is kept as the record of an agent
	// that never reached its gateway.
	MarkFailed(ctx context.Context, id uuid.UUID, lastErr string) error
	// DeleteForAgent removes every environment's row for a deleted agent.
	DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error
}

type a2aPublicationRepository struct {
	db *gorm.DB
}

// NewA2APublicationRepository creates an A2APublicationRepository.
func NewA2APublicationRepository(db *gorm.DB) A2APublicationRepository {
	return &a2aPublicationRepository{db: db}
}

func (r *a2aPublicationRepository) Enqueue(ctx context.Context, pub *models.A2APublication) error {
	now := time.Now()
	pub.Status = models.A2APublicationStatusPending
	pub.AttemptCount = 0
	pub.LastError = ""
	pub.NextAttemptAt = &now
	pub.UpdatedAt = now

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "ou_id"}, {Name: "project_name"},
			{Name: "agent_name"}, {Name: "environment_name"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"environment_uuid", "artifact_uuid", "status",
			"attempt_count", "last_error", "next_attempt_at", "updated_at",
		}),
	}).Create(pub).Error
}

func (r *a2aPublicationRepository) FindDue(ctx context.Context, now time.Time, limit int) ([]models.A2APublication, error) {
	var due []models.A2APublication
	err := r.db.WithContext(ctx).
		Where("status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)",
			models.A2APublicationStatusPending, now).
		Order("next_attempt_at ASC, created_at ASC").
		Limit(limit).
		Find(&due).Error
	if err != nil {
		return nil, err
	}
	return due, nil
}

func (r *a2aPublicationRepository) MarkPublished(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&models.A2APublication{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":          models.A2APublicationStatusPublished,
			"last_error":      "",
			"next_attempt_at": nil,
			"updated_at":      time.Now(),
		}).Error
}

func (r *a2aPublicationRepository) MarkAttemptFailed(ctx context.Context, id uuid.UUID, lastErr string, nextAttemptAt time.Time) error {
	return r.db.WithContext(ctx).Model(&models.A2APublication{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"attempt_count":   gorm.Expr("attempt_count + 1"),
			"last_error":      lastErr,
			"next_attempt_at": nextAttemptAt,
			"updated_at":      time.Now(),
		}).Error
}

func (r *a2aPublicationRepository) MarkFailed(ctx context.Context, id uuid.UUID, lastErr string) error {
	return r.db.WithContext(ctx).Model(&models.A2APublication{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":          models.A2APublicationStatusFailed,
			"attempt_count":   gorm.Expr("attempt_count + 1"),
			"last_error":      lastErr,
			"next_attempt_at": nil,
			"updated_at":      time.Now(),
		}).Error
}

func (r *a2aPublicationRepository) DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error {
	return r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ?", ouID, projectName, agentName).
		Delete(&models.A2APublication{}).Error
}
```

- [ ] **Step 5: Generate the mock**

Run: `cd agent-manager-service && make mocks PKG=./repositories/...`
Expected: `repositories/a2a_publication_repository_moq.go` appears. If `moq` is not installed, install the pinned version named in `agent-manager-service/README.md` (`go install github.com/matryer/moq@v0.5.3`) and re-run.

- [ ] **Step 6: Write the repository test**

Create `agent-manager-service/repositories/a2a_publication_repository_unit_test.go` (WSO2 2026 header, package `repositories`). First check how the neighbouring repository unit tests get a database — search `repositories/*_test.go` for `sqlmock`, `sqlite`, or a `testDB` helper, and follow whichever pattern is already there.

If no in-package DB harness exists, write the test against the exported behaviour that needs no DB — the status constants and the model's `TableName` — and put the queue's real behaviour under the reconciler's test in Task 7 using the generated moq. Say so in a comment. Do not invent a new test-database mechanism for this repo.

```go
package repositories

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// The reconciler's due-scan filters on this exact string; a rename that missed
// the query would silently stop every publication.
func TestA2APublicationTableAndStatuses(t *testing.T) {
	assert.Equal(t, "a2a_publications", models.A2APublication{}.TableName())
	assert.Equal(t, models.A2APublicationStatus("pending"), models.A2APublicationStatusPending)
	assert.Equal(t, models.A2APublicationStatus("published"), models.A2APublicationStatusPublished)
	assert.Equal(t, models.A2APublicationStatus("failed"), models.A2APublicationStatusFailed)
}
```

- [ ] **Step 7: Build, migrate and test**

Run:
```
cd agent-manager-service
go build ./...
go test ./repositories/... -run TestA2APublication
make lint
```
Expected: PASS, clean.

- [ ] **Step 8: Commit**

```bash
git add agent-manager-service/models agent-manager-service/db_migrations agent-manager-service/repositories
git commit -m "feat: add the a2a publication queue

upstream.url is only readable once the release binding reconciles, so a
deploy records its publication intent durably and a reconciler finishes it.

Claude-Session: https://claude.ai/code/session_01JXtNk6KuBdVKYBZj4bYeWo"
```

---

### Task 7: The publication reconciler

Drains the queue: read the binding's `ServiceURL`, and once it is there, build the YAML, write the `deployments` row and broadcast. This is where the ordering problem is actually solved.

**Files:**
- Modify: `agent-manager-service/clients/openchoreosvc/client/client.go` (add to the `OpenChoreoClient` interface at `:51`)
- Modify: `agent-manager-service/clients/openchoreosvc/client/deployments.go` (implement it)
- Create: `agent-manager-service/services/a2a_publication_reconciler.go`
- Create: `agent-manager-service/services/a2a_publication_reconciler_test.go`
- Modify: `agent-manager-service/wiring/wire.go`, `agent-manager-service/wiring/params.go`, `agent-manager-service/app/app.go`

**Interfaces:**
- Consumes: `buildA2AAgentDeploymentYAML` / `a2aAgentEnvArtifactName` (Task 4); `BroadcastAgentDeploymentEvent` (Task 5); `A2APublicationRepository` (Task 6); `buildPolicies` and `resolveAPIConfig` (`services/agent_manager.go`); `resolveEgressGatewayForEnvironment` (`services/gateway_roles.go:70`); `DeploymentRepository.CreateWithLimitEnforcement`; `models.Gateway.Vhost`.
- Produces:
  - `client.OpenChoreoClient.GetReleaseBindingServiceURL(ctx context.Context, ouID, componentName, environment string) (string, error)` — returns `""` with a nil error when the binding exists but has published no ServiceURL yet.
  - `services.A2APublicationReconcilerService` interface with `Start(ctx context.Context) error` and `Stop() error`
  - `services.NewA2APublicationReconcilerService(...) A2APublicationReconcilerService`

- [ ] **Step 1: Add the ServiceURL reader to the OpenChoreo client**

Add to the `OpenChoreoClient` interface in `clients/openchoreosvc/client/client.go`, next to `GetComponentEndpoints` (`:91`):

```go
	// GetReleaseBindingServiceURL returns the in-cluster address of the
	// component's endpoint in one environment, as the release binding's status
	// reports it. Empty string with a nil error means the binding has not
	// published one yet.
	GetReleaseBindingServiceURL(ctx context.Context, ouID, componentName, environment string) (string, error)
```

Implement it in `clients/openchoreosvc/client/deployments.go`, modelled on `GetComponentEndpoints` (`components.go:2625-2666`) but reading `ServiceURL` rather than `ExternalURLs`:

```go
// GetReleaseBindingServiceURL reads the workload's in-cluster address for one
// environment out of the release binding's status.
//
// The address is READ rather than derived. OpenChoreo already publishes it —
// the agent-api ComponentType renders a Service named after the component in
// the component's namespace, and the binding reports it as {Host, Port, Scheme,
// Path} — so deriving it a second time from a naming convention would duplicate
// something this repo does not own, and drift the moment OpenChoreo changed it.
// The platform already treats this field as authoritative: probedPorts aims the
// TCP startup probe with its Port.
//
// An empty string with a nil error is the normal state immediately after a
// deploy: status is populated only once the binding reconciles. Callers must
// distinguish it from an error and retry rather than publish.
//
// Selection: an agent component declares exactly one endpoint (the agent-api
// ComponentType requires at least one, and buildEndpoints emits exactly one),
// so the first endpoint carrying a ServiceURL is taken. Scheme is defaulted to
// http when absent — nothing in this repo reads it today, and the in-cluster
// hop to an agent's container is plain HTTP.
func (c *openChoreoClient) GetReleaseBindingServiceURL(ctx context.Context, ouID, componentName, environment string) (string, error) {
	namespaceName := c.NamespaceFor(ouID)
	resp, err := c.ocClient.ListReleaseBindingsWithResponse(ctx, namespaceName, &gen.ListReleaseBindingsParams{
		Component: &componentName,
		Limit:     &defaultListLimit,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list release bindings for %s: %w", componentName, err)
	}
	if resp.StatusCode() != http.StatusOK {
		return "", handleErrorResponse(resp.StatusCode(), ErrorResponses{
			JSON401: resp.JSON401,
			JSON403: resp.JSON403,
			JSON404: resp.JSON404,
			JSON500: resp.JSON500,
		})
	}
	if resp.JSON200 == nil {
		return "", nil
	}

	for _, binding := range resp.JSON200.Items {
		if binding.Spec == nil || binding.Spec.Environment != environment {
			continue
		}
		if binding.Status == nil || binding.Status.Endpoints == nil {
			return "", nil
		}
		for _, ep := range *binding.Status.Endpoints {
			if ep.ServiceURL == nil || strings.TrimSpace(ep.ServiceURL.Host) == "" {
				continue
			}
			svc := *ep.ServiceURL
			if svc.Scheme == nil || strings.TrimSpace(*svc.Scheme) == "" {
				scheme := "http"
				svc.Scheme = &scheme
			}
			return buildEndpointURLString(&svc), nil
		}
		return "", nil
	}
	return "", nil
}
```

Check the imports `deployments.go` already has (`fmt`, `net/http`, `strings`, the `gen` package) and add whatever is missing.

- [ ] **Step 2: Regenerate the OpenChoreo client mock**

Run: `cd agent-manager-service && make mocks PKG=./clients/...`
Expected: the mock gains `GetReleaseBindingServiceURLFunc`. If the mocks live under `clients/clientmocks`, run `make mocks` without `PKG` and check `git status` to confirm the mock was regenerated.

- [ ] **Step 3: Write the failing reconciler tests**

Create `agent-manager-service/services/a2a_publication_reconciler_test.go` (WSO2 2026 header, package `services`).

Before writing, open `services/agent_thunder_reconciler_test.go` and copy how it constructs its service under test with moq'd dependencies. Match the moq field-naming convention exactly (`<Method>Func`, `<Method>Calls()`).

```go
package services

import (
	"context"
	"errors"
	"log/slog"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func pendingPublication() models.A2APublication {
	return models.A2APublication{
		ID:              uuid.New(),
		OUID:            "org-1",
		ProjectName:     "checkout",
		AgentName:       "trip-planner",
		EnvironmentName: "dev",
		EnvironmentUUID: uuid.New(),
		ArtifactUUID:    uuid.New(),
		Status:          models.A2APublicationStatusPending,
	}
}

// The whole reason this reconciler exists: status is populated only after the
// binding reconciles, so an early attempt must retry rather than emit an Agent
// that routes nowhere.
func TestReconcilerRetriesWhenServiceURLIsNotReadyYet(t *testing.T) {
	// Build the service with:
	//   - an OpenChoreoClient mock whose GetReleaseBindingServiceURLFunc returns ("", nil)
	//   - an A2APublicationRepository mock whose FindDueFunc returns one pendingPublication()
	//   - a GatewayEventsService over a recordingEventHub
	//   - a DeploymentRepository mock
	// then call the single-row entry point (publishOne) directly.
	//
	// Assert: no deployment row was created, no event was published, and
	// MarkAttemptFailedCalls() has exactly one entry whose nextAttemptAt is in
	// the future.
	t.Skip("fill in once the service constructor exists — see Step 5")
}

// Past the startup budget an agent that never became ready is called failed
// rather than retried forever.
func TestReconcilerGivesUpPastTheStartupBudget(t *testing.T) {
	t.Skip("fill in once the service constructor exists — see Step 5")
}

// The happy path: a ready binding produces a deployments row carrying the Agent
// YAML and an agent.deployed broadcast keyed on the artifact UUID.
func TestReconcilerPublishesOnceServiceURLIsAvailable(t *testing.T) {
	t.Skip("fill in once the service constructor exists — see Step 5")
}

var _ = errors.New
var _ = context.Background
var _ = time.Now
var _ = assert.True
var _ = require.NoError
```

The three `t.Skip` bodies are placeholders **only for this step** — Step 5 replaces every one of them with a real test. Do not commit a skipped test.

- [ ] **Step 4: Write the reconciler**

Create `agent-manager-service/services/a2a_publication_reconciler.go` (WSO2 2026 header, package `services`):

```go
package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

const (
	a2aReconcilerTickInterval = 30 * time.Second
	// a2aReconcilerLockID is this loop's own PostgreSQL advisory lock ID,
	// distinct from schedulerLockID and reconcilerLockID so the three background
	// loops never block each other.
	a2aReconcilerLockID  = int64(739281458)
	a2aReconcilerBatch   = 50
	a2aReconcilerRetryIn = 30 * time.Second

	// a2aPublicationAttemptBudget bounds how long an agent may fail to publish
	// an upstream before the row is called failed.
	//
	// The number is agentStartupBudget (10 minutes, the point past which the
	// agent-api startup probe has already given up at least once, so nothing is
	// still starting) divided by the tick interval, with slack. Past it the
	// binding is not going to report a ServiceURL, and retrying forever would
	// only hide that from whoever has to fix it.
	a2aPublicationAttemptBudget = 30
)

// A2APublicationReconcilerService drains the a2a_publications queue.
type A2APublicationReconcilerService interface {
	Start(ctx context.Context) error
	Stop() error
}

type a2aPublicationReconcilerService struct {
	pubRepo        repositories.A2APublicationRepository
	deploymentRepo repositories.DeploymentRepository
	gatewayRepo    repositories.GatewayRepository
	agentConfigRepo repositories.AgentConfigRepository
	ocClient       client.OpenChoreoClient
	events         *GatewayEventsService
	logger         *slog.Logger
	stopCh         chan struct{}
	stopOnce       sync.Once
}

// NewA2APublicationReconcilerService creates an A2APublicationReconcilerService.
func NewA2APublicationReconcilerService(
	pubRepo repositories.A2APublicationRepository,
	deploymentRepo repositories.DeploymentRepository,
	gatewayRepo repositories.GatewayRepository,
	agentConfigRepo repositories.AgentConfigRepository,
	ocClient client.OpenChoreoClient,
	events *GatewayEventsService,
	logger *slog.Logger,
) A2APublicationReconcilerService {
	return &a2aPublicationReconcilerService{
		pubRepo:         pubRepo,
		deploymentRepo:  deploymentRepo,
		gatewayRepo:     gatewayRepo,
		agentConfigRepo: agentConfigRepo,
		ocClient:        ocClient,
		events:          events,
		logger:          logger,
		stopCh:          make(chan struct{}),
	}
}

func (s *a2aPublicationReconcilerService) Start(ctx context.Context) error {
	go s.runLoop(ctx)
	s.logger.Info("A2A publication reconciler started")
	return nil
}

func (s *a2aPublicationReconcilerService) Stop() error {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.logger.Info("A2A publication reconciler stopped")
	})
	return nil
}

func (s *a2aPublicationReconcilerService) runLoop(ctx context.Context) {
	ticker := time.NewTicker(a2aReconcilerTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runCycle(ctx)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// runCycle claims the due batch under an advisory lock so only one replica
// scans at a time, then releases it before the slow OpenChoreo and event-hub
// calls — mirroring agentThunderReconcilerService.runCycle.
func (s *a2aPublicationReconcilerService) runCycle(ctx context.Context) {
	tx := db.GetDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		s.logger.Error("Failed to begin transaction for A2A publication advisory lock", "error", tx.Error)
		return
	}

	var locked bool
	if err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", a2aReconcilerLockID).Scan(&locked).Error; err != nil {
		s.logger.Error("Failed to try A2A publication advisory lock", "error", err)
		tx.Rollback()
		return
	}
	if !locked {
		tx.Rollback()
		return
	}

	due, err := s.pubRepo.FindDue(ctx, time.Now(), a2aReconcilerBatch)
	if err != nil {
		s.logger.Error("Failed to query due A2A publications", "error", err)
		tx.Rollback()
		return
	}
	if err := tx.Commit().Error; err != nil {
		s.logger.Error("Failed to commit A2A publication advisory lock transaction", "error", err)
		return
	}

	for _, pub := range due {
		s.publishOne(ctx, pub)
	}
}

// publishOne emits one agent-environment pair's Agent resource, or schedules a
// retry when it cannot yet.
func (s *a2aPublicationReconcilerService) publishOne(ctx context.Context, pub models.A2APublication) {
	if err := s.attemptPublish(ctx, pub); err != nil {
		s.recordAttemptFailure(ctx, pub, err)
		return
	}
	if err := s.pubRepo.MarkPublished(ctx, pub.ID); err != nil {
		s.logger.Error("Published A2A agent but failed to mark the queue row",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName, "error", err)
	}
}

// recordAttemptFailure retries within the budget and gives up past it.
func (s *a2aPublicationReconcilerService) recordAttemptFailure(ctx context.Context, pub models.A2APublication, cause error) {
	if pub.AttemptCount+1 >= a2aPublicationAttemptBudget {
		s.logger.Error("A2A agent never reached its gateway within the attempt budget",
			"agentName", pub.AgentName, "environment", pub.EnvironmentName,
			"attempts", pub.AttemptCount+1, "error", cause)
		if err := s.pubRepo.MarkFailed(ctx, pub.ID, cause.Error()); err != nil {
			s.logger.Error("Failed to mark A2A publication failed", "error", err)
		}
		return
	}
	s.logger.Debug("A2A agent not publishable yet, will retry",
		"agentName", pub.AgentName, "environment", pub.EnvironmentName,
		"attempt", pub.AttemptCount+1, "reason", cause)
	if err := s.pubRepo.MarkAttemptFailed(ctx, pub.ID, cause.Error(), time.Now().Add(a2aReconcilerRetryIn)); err != nil {
		s.logger.Error("Failed to schedule A2A publication retry", "error", err)
	}
}

// errUpstreamNotReady means the binding has not published a ServiceURL yet. It
// is the expected condition for the first few attempts after a deploy, not a
// misconfiguration.
var errUpstreamNotReady = errors.New("release binding has not published a service URL yet")

func (s *a2aPublicationReconcilerService) attemptPublish(ctx context.Context, pub models.A2APublication) error {
	upstreamURL, err := s.ocClient.GetReleaseBindingServiceURL(ctx, pub.OUID, pub.AgentName, pub.EnvironmentName)
	if err != nil {
		return fmt.Errorf("failed to read release binding service URL: %w", err)
	}
	if upstreamURL == "" {
		return errUpstreamNotReady
	}

	gateway, err := s.resolveGateway(pub)
	if err != nil {
		return err
	}

	// The policy chain is exactly what a chat/custom agent gets: the persisted
	// per-environment config, run through the same buildPolicies. No A2A-specific
	// policy exists in M1 — the gateway's own Agent rules supply the rest.
	cfg, err := s.agentConfigRepo.Get(ctx, pub.OUID, pub.ProjectName, pub.AgentName, pub.EnvironmentName)
	if err != nil {
		return fmt.Errorf("failed to load agent config: %w", err)
	}
	policies := buildPolicies(resolveAPIConfig(cfg, nil, nil, nil, nil, false))

	yamlStr, err := generateA2AAgentDeploymentYAML(A2AAgentDeploymentInput{
		ArtifactName: a2aAgentEnvArtifactName(pub.ProjectName, pub.AgentName, pub.EnvironmentUUID.String()),
		DisplayName:  pub.AgentName,
		AgentName:    pub.AgentName,
		Vhost:        gateway.Vhost,
		UpstreamURL:  upstreamURL,
		Policies:     policies,
	})
	if err != nil {
		return err
	}

	deploymentID := uuid.New()
	deployed := models.DeploymentStatusDeployed
	deployment := &models.Deployment{
		DeploymentID: deploymentID,
		Name:         fmt.Sprintf("%s-deployment", pub.AgentName),
		ArtifactUUID: pub.ArtifactUUID,
		OUID:         pub.OUID,
		GatewayUUID:  gateway.UUID,
		Content:      []byte(yamlStr),
		Status:       &deployed,
	}
	// The deployments row is not optional bookkeeping: GET /agents/{agentId}
	// serves the Agent YAML back out of it when the gateway fetches after the
	// event, so an event without a row is an event the gateway cannot act on.
	if err := s.deploymentRepo.CreateWithLimitEnforcement(deployment, maxDeploymentsPerAPI+deploymentLimitBuffer); err != nil {
		return fmt.Errorf("failed to create A2A agent deployment row: %w", err)
	}

	event := &models.AgentDeploymentEvent{
		AgentID:      pub.ArtifactUUID.String(),
		DeploymentID: deploymentID.String(),
		PerformedAt:  time.Now().Truncate(time.Millisecond),
	}
	if err := s.events.BroadcastAgentDeploymentEvent(gateway.UUID.String(), event); err != nil {
		return fmt.Errorf("failed to broadcast agent deployment event: %w", err)
	}

	s.logger.Info("Published A2A agent to gateway",
		"agentName", pub.AgentName, "environment", pub.EnvironmentName,
		"artifactID", pub.ArtifactUUID, "gateway", gateway.Name, "upstream", upstreamURL)
	return nil
}

// resolveGateway picks the environment's gateway. An A2A agent is inbound
// traffic, so it belongs on the environment's INGRESS gateway — the same slot a
// REST agent's api-configuration trait targets via the apiGatewayName
// convention — not on an egress gateway, which hosts outbound LLM/MCP artifacts.
func (s *a2aPublicationReconcilerService) resolveGateway(pub models.A2APublication) (*models.Gateway, error) {
	envID := pub.EnvironmentUUID.String()
	gateways, err := s.gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID: pub.OUID,
		EnvironmentID:  &envID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list gateways for environment %s: %w", envID, err)
	}
	for _, gw := range gateways {
		if gw != nil && gw.IsIngressCapable() {
			return gw, nil
		}
	}
	// An environment has at most one ingress gateway, and it is registered by the
	// bootstrap job, so absence is a timing condition rather than a
	// misconfiguration — retry.
	return nil, fmt.Errorf("no ingress gateway is mapped to environment %s yet", envID)
}
```

Two signatures above are asserted, not verified. **Before compiling, check both and adjust:**
- `s.agentConfigRepo.Get(...)` — open `repositories/agent_config_repository.go` and use its real getter name and signature (it may be `GetByAgentAndEnvironment` or take no context).
- `resolveAPIConfig(cfg, nil, nil, nil, nil, false)` — open `services/agent_manager.go:3445` and match the real parameter list.

Also confirm `models.Deployment`'s field names against `services/mcp_proxy_deployment.go:104-113`, and `repositories.GatewayFilterOptions`'s fields against `services/gateway_roles.go:75-79`.

- [ ] **Step 5: Replace the three skipped tests with real ones**

Rewrite `services/a2a_publication_reconciler_test.go`, removing every `t.Skip`. Each test constructs `&a2aPublicationReconcilerService{...}` directly with moq'd fields (not through the constructor — direct struct construction keeps the test independent of constructor-argument churn) and calls `publishOne`.

- `TestReconcilerRetriesWhenServiceURLIsNotReadyYet` — `GetReleaseBindingServiceURLFunc` returns `("", nil)`. Assert `len(deploymentRepo.CreateWithLimitEnforcementCalls()) == 0`, `len(hub.published) == 0`, `len(pubRepo.MarkAttemptFailedCalls()) == 1`, and that the recorded `nextAttemptAt` is after `time.Now()`.
- `TestReconcilerGivesUpPastTheStartupBudget` — same, but the publication row has `AttemptCount: a2aPublicationAttemptBudget - 1`. Assert `MarkFailedCalls()` has one entry and `MarkAttemptFailedCalls()` is empty.
- `TestReconcilerPublishesOnceServiceURLIsAvailable` — `GetReleaseBindingServiceURLFunc` returns `("http://trip-planner.dp-default:9099", nil)`; the gateway repo returns one ingress-capable gateway with a `Vhost`; the agent-config repo returns a config with API-key security on. Assert exactly one `CreateWithLimitEnforcement` call whose `Content` unmarshals into an `A2AAgentDeploymentYAML` with that upstream and `kind: Agent`; exactly one published event of type `agent.deployed` whose `EntityID` is the publication's `ArtifactUUID`; and one `MarkPublished` call.

Reuse `recordingEventHub` from `services/a2a_agent_events_unit_test.go` (Task 5) — same package, so it is already in scope. Delete the `var _ =` placeholder block.

- [ ] **Step 6: Run the tests**

Run: `cd agent-manager-service && go test ./services/... -run TestReconciler -v`
Expected: three PASS, no SKIP.

- [ ] **Step 7: Wire it up**

In `wiring/params.go`, add next to `AgentThunderReconciler` (`:79`):

```go
	A2APublicationReconciler services.A2APublicationReconcilerService
```

In `wiring/wire.go`, add `services.NewA2APublicationReconcilerService` and `repositories.NewA2APublicationRepository` to the provider sets, next to their neighbours (`services.NewAgentThunderReconcilerService` at `:83`, and the repository set wherever `NewDeploymentRepo` is registered).

Regenerate: `cd agent-manager-service && make codegen` (or `go generate -tags=wireinject ./...`, per the Makefile's `codegen` target). Confirm `wiring/wire_gen.go` picked up both.

In `app/app.go`, after the thunder reconciler block (`:192-199`):

```go
	// Start the A2A publication reconciler. It runs unconditionally: it is a
	// no-op for an org with no A2A agents, and gating it on a feature flag would
	// leave a deploy's queued publication stranded if the flag were ever off.
	a2aReconcilerCtx, a2aReconcilerCancel := context.WithCancel(backgroundCtx)
	if err := dependencies.A2APublicationReconciler.Start(a2aReconcilerCtx); err != nil {
		slog.Error("failed to start A2A publication reconciler", "error", err)
		os.Exit(1)
	}
```

and a matching `Stop()` + `a2aReconcilerCancel()` in the shutdown block alongside the thunder reconciler's (`:252`). Follow the existing block's exact ordering and error handling.

- [ ] **Step 8: Build, test and lint**

Run:
```
cd agent-manager-service
go build ./... && go test ./services/... && go test ./clients/... && make lint
```
Expected: PASS, clean.

- [ ] **Step 9: Commit**

```bash
git add agent-manager-service
git commit -m "feat: reconcile a2a agent publication off the release binding status

Publication waits for ReleaseBinding.Status.Endpoints[].ServiceURL rather
than deriving an address, and refuses to emit an Agent with an empty
upstream. A background loop retries until the binding reports one, and
gives up past the agent startup budget.

Claude-Session: https://claude.ai/code/session_01JXtNk6KuBdVKYBZj4bYeWo"
```

---

### Task 8: Enqueue publication on deploy

**Files:**
- Modify: `agent-manager-service/services/agent_manager.go:82-140` (struct + constructor), `:3229-3260` (deploy)
- Modify: `agent-manager-service/wiring/wire_gen.go` (regenerated)
- Test: `agent-manager-service/services/a2a_deploy_enqueue_unit_test.go` (create)

**Interfaces:**
- Consumes: `repositories.A2APublicationRepository` (Task 6); `ensureAgentEnvAPIArtifact` (`services/agent_api_artifact.go:34`); `isA2AAgent` (Task 3).
- Produces: `agentManagerService` gains an `a2aPublicationRepo repositories.A2APublicationRepository` field and a matching constructor parameter, appended **after** `identityClient` and before `logger`.

- [ ] **Step 1: Write the failing test**

Create `agent-manager-service/services/a2a_deploy_enqueue_unit_test.go` (WSO2 2026 header, package `services`):

```go
package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// A deploy records the intent to publish rather than publishing inline: the
// upstream address is not readable until the release binding reconciles.
func TestEnqueueA2APublicationRecordsTheArtifactUUID(t *testing.T) {
	var enqueued []models.A2APublication
	repo := &repositories.A2APublicationRepositoryMock{
		EnqueueFunc: func(ctx context.Context, pub *models.A2APublication) error {
			enqueued = append(enqueued, *pub)
			return nil
		},
	}
	svc := &agentManagerService{a2aPublicationRepo: repo, logger: testLogger()}

	envUUID := uuid.New()
	artifactUUID := uuid.New()
	svc.enqueueA2APublication(context.Background(), "org-1", "checkout", "trip-planner", "dev", envUUID, artifactUUID)

	require.Len(t, enqueued, 1)
	assert.Equal(t, "trip-planner", enqueued[0].AgentName)
	assert.Equal(t, "dev", enqueued[0].EnvironmentName)
	assert.Equal(t, envUUID, enqueued[0].EnvironmentUUID)
	assert.Equal(t, artifactUUID, enqueued[0].ArtifactUUID)
}

// A queue failure must not fail a deploy that has otherwise succeeded — the
// reconciler is not the deploy's critical path, and the agent is running.
func TestEnqueueA2APublicationSurvivesRepoFailure(t *testing.T) {
	repo := &repositories.A2APublicationRepositoryMock{
		EnqueueFunc: func(ctx context.Context, pub *models.A2APublication) error {
			return assert.AnError
		},
	}
	svc := &agentManagerService{a2aPublicationRepo: repo, logger: testLogger()}
	assert.NotPanics(t, func() {
		svc.enqueueA2APublication(context.Background(), "org-1", "checkout", "trip-planner", "dev", uuid.New(), uuid.New())
	})
}
```

Check the generated moq's type name (`A2APublicationRepositoryMock` vs `A2APublicationRepositoryMoq`) in `repositories/a2a_publication_repository_moq.go` and use the real one.

- [ ] **Step 2: Run to verify it fails**

Run: `cd agent-manager-service && go test ./services/... -run TestEnqueueA2APublication`
Expected: FAIL — `a2aPublicationRepo` and `enqueueA2APublication` are undefined.

- [ ] **Step 3: Add the dependency**

In `services/agent_manager.go`, add to the struct (`:82`), after `identityClient`:

```go
	a2aPublicationRepo        repositories.A2APublicationRepository
```

Add the matching parameter to `NewAgentManagerService` — **after `identityClient thundersvc.IdentityClient` and before `logger *slog.Logger`** — and assign it in the returned literal. Then find every caller (`grep -rn 'NewAgentManagerService(' agent-manager-service/ --include='*.go'`) and add the argument; test helpers may pass `nil`.

- [ ] **Step 4: Add the enqueue helper**

Append to `services/agent_manager.go`:

```go
// enqueueA2APublication records that an A2A agent's gateway resource is due to
// be emitted for one environment.
//
// The publication cannot happen here. upstream.url is read from the release
// binding's status, which OpenChoreo populates only once the binding
// reconciles — after this call returns — so an inline publish would emit an
// Agent with an empty upstream that routes nowhere while looking healthy. The
// A2A publication reconciler picks the row up and finishes the job.
//
// Best effort: the agent is deployed and running by this point, and failing the
// deploy over a queue write would report a false failure for work that
// succeeded. A missed row surfaces as an agent that never appears on its
// gateway, which the next redeploy re-queues.
func (s *agentManagerService) enqueueA2APublication(
	ctx context.Context,
	ouID, projectName, agentName, environmentName string,
	environmentUUID, artifactUUID uuid.UUID,
) {
	if s.a2aPublicationRepo == nil {
		return
	}
	pub := &models.A2APublication{
		OUID:            ouID,
		ProjectName:     projectName,
		AgentName:       agentName,
		EnvironmentName: environmentName,
		EnvironmentUUID: environmentUUID,
		ArtifactUUID:    artifactUUID,
	}
	if err := s.a2aPublicationRepo.Enqueue(ctx, pub); err != nil {
		s.logger.Error("Failed to queue A2A agent gateway publication; the agent is deployed but will not reach its gateway until the next redeploy",
			"agentName", agentName, "environment", environmentName, "error", err)
	}
}
```

Confirm `uuid` is imported in that file.

- [ ] **Step 5: Call it from deploy**

In `DeployAgent`, inside the `if isAPIAgent && !isA2AAgent { ... }` block's sibling position — that is, add a new block right after it, still holding `targetEnv` and `lowestEnv` in scope:

```go
	if isA2AAgent {
		// The artifact row is created for every API agent kind, A2A included: it
		// is what the agent's API keys are already bound to and what the gateway
		// is told about, so the entire API-key path is inherited with no new code.
		apiArtifact, artifactErr := ensureAgentEnvAPIArtifact(s.db, s.artifactRepo, ouID, projectName, agentName, targetEnv.UUID)
		if artifactErr != nil {
			return "", fmt.Errorf("cannot deploy A2A agent without environment API artifact record: %w", artifactErr)
		}
		envUUID, parseErr := uuid.Parse(targetEnv.UUID)
		if parseErr != nil {
			return "", fmt.Errorf("environment %q has an unparseable UUID %q: %w", lowestEnv, targetEnv.UUID, parseErr)
		}
		s.enqueueA2APublication(ctx, ouID, projectName, agentName, lowestEnv, envUUID, apiArtifact.UUID)
	}
```

Place this call **after** `applyEnvScopedWorkloadConfig` and the `UpdateReleaseBindingTraitConfigs` write, so the row is only queued once the binding this deploy needs actually exists. Confirm `targetEnv.UUID`'s type first (it may already be a `uuid.UUID`, in which case drop the parse).

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd agent-manager-service && go test ./services/... -run TestEnqueueA2APublication`
Expected: PASS.

- [ ] **Step 7: Rewire and run everything**

Run:
```
cd agent-manager-service
make codegen
go build ./... && go test ./services/... && go test -tags=integration ./tests/... && make lint
```
Expected: PASS, clean.

- [ ] **Step 8: Commit**

```bash
git add agent-manager-service
git commit -m "feat: queue an a2a agent's gateway publication on deploy

Claude-Session: https://claude.ai/code/session_01JXtNk6KuBdVKYBZj4bYeWo"
```

---

### Task 9: Broadcast deletion

**Files:**
- Modify: `agent-manager-service/services/agent_manager.go:2786-2808` (`deleteAgentAPIArtifact`)
- Modify: `agent-manager-service/services/a2a_agent_deployment.go` (add the deletion helper)
- Test: `agent-manager-service/services/a2a_agent_deletion_unit_test.go` (create)

**Interfaces:**
- Consumes: `BroadcastAgentDeletionEvent` (Task 5); `DeploymentRepository.GetDeployedGatewaysByProvider`; `GatewayRepository.ListWithFilters`; `A2APublicationRepository.DeleteForAgent` (Task 6).
- Produces: `func broadcastA2AAgentDeletion(ctx context.Context, events *GatewayEventsService, deploymentRepo repositories.DeploymentRepository, gatewayRepo repositories.GatewayRepository, artifactUUID uuid.UUID, ouID string, logger *slog.Logger)`

- [ ] **Step 1: Write the failing test**

Create `agent-manager-service/services/a2a_agent_deletion_unit_test.go` (WSO2 2026 header, package `services`):

```go
package services

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
)

// Deletion goes to every gateway the artifact was deployed to AND every active
// gateway in the org, best effort — the same union broadcastMCPProxyDeletion
// uses, because a gateway that holds a stale copy is worse than a redundant
// event.
func TestBroadcastA2AAgentDeletionReachesEveryCandidateGateway(t *testing.T) {
	artifactUUID := uuid.New()
	hub := &recordingEventHub{}
	events := NewGatewayEventsService(hub)

	// deploymentRepo returns one gateway ID; gatewayRepo returns a second,
	// different active gateway. Both must receive the event, and neither twice.
	// Build the two moqs, call broadcastA2AAgentDeletion, then assert.
	_ = events
	_ = artifactUUID

	require.Len(t, hub.published, 2)
	seen := map[string]bool{}
	for _, evt := range hub.published {
		assert.Equal(t, eventhub.EventType("agent.deleted"), evt.EventType)
		var envelope struct {
			Payload struct {
				AgentID string `json:"agentId"`
			} `json:"payload"`
		}
		require.NoError(t, json.Unmarshal([]byte(evt.EventData), &envelope))
		assert.Equal(t, artifactUUID.String(), envelope.Payload.AgentID)
		assert.False(t, seen[evt.GatewayID], "no gateway is told twice")
		seen[evt.GatewayID] = true
	}
}
```

Fill in the two moq constructions before running — read `services/mcp_proxy_deployment.go:317-355` (`gatewayIDsForDeletion`) for the exact repository calls and their return shapes, and mirror them.

- [ ] **Step 2: Run to verify it fails**

Run: `cd agent-manager-service && go test ./services/... -run TestBroadcastA2AAgentDeletion`
Expected: FAIL — `undefined: broadcastA2AAgentDeletion`.

- [ ] **Step 3: Write the deletion helper**

Append to `services/a2a_agent_deployment.go` (adding the needed imports):

```go
// broadcastA2AAgentDeletion tells every gateway that could be holding this
// Agent to drop it.
//
// The recipient set is the union of the gateways the artifact has deployment
// rows for and every active gateway in the org — the same union
// gatewayIDsForDeletion builds for MCP proxies, and for the same reason: a
// gateway left holding a deleted agent keeps routing to a workload that is
// gone, so a redundant delete is much cheaper than a missed one.
//
// Best effort. Deletion of the agent itself has already happened by the time
// this runs; failing it here would leave the caller unable to complete a delete
// it cannot undo.
func broadcastA2AAgentDeletion(
	ctx context.Context,
	events *GatewayEventsService,
	deploymentRepo repositories.DeploymentRepository,
	gatewayRepo repositories.GatewayRepository,
	artifactUUID uuid.UUID,
	ouID string,
	logger *slog.Logger,
) {
	_ = ctx
	if events == nil || artifactUUID == uuid.Nil {
		return
	}

	gatewayIDs := map[string]struct{}{}
	if deploymentRepo != nil {
		deployed, err := deploymentRepo.GetDeployedGatewaysByProvider(artifactUUID, ouID)
		if err != nil {
			logger.Warn("Failed to list deployed gateways for A2A agent deletion",
				"artifactID", artifactUUID, "error", err)
		}
		for _, id := range deployed {
			if strings.TrimSpace(id) != "" {
				gatewayIDs[id] = struct{}{}
			}
		}
	}
	if gatewayRepo != nil {
		active := true
		gateways, err := gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
			OrganizationID: ouID,
			Status:         &active,
		})
		if err != nil {
			logger.Warn("Failed to list active gateways for A2A agent deletion",
				"artifactID", artifactUUID, "error", err)
		}
		for _, gw := range gateways {
			if gw != nil {
				gatewayIDs[gw.UUID.String()] = struct{}{}
			}
		}
	}

	event := &models.AgentDeletionEvent{AgentID: artifactUUID.String()}
	for gatewayID := range gatewayIDs {
		if err := events.BroadcastAgentDeletionEvent(gatewayID, event); err != nil {
			logger.Warn("Failed to broadcast A2A agent deletion event",
				"artifactID", artifactUUID, "gatewayID", gatewayID, "error", err)
		}
	}
}
```

- [ ] **Step 4: Call it from the delete path**

`deleteAgentAPIArtifact` (`services/agent_manager.go:2786`) already resolves the environment and looks the artifact up by handle. Broadcast **before** deleting the artifact row, so the artifact UUID is still readable:

```go
	artifact, err := s.artifactRepo.GetByHandle(agentEnvAPIArtifactHandle(projectName, agentName, environment.UUID), ouID)
	if err != nil {
		return
	}
	// Before the row goes: the gateway keys its Agent on this UUID, and once the
	// row is gone there is nothing left to name in the event.
	broadcastA2AAgentDeletion(ctx, s.gatewayEventsService, s.deploymentRepo, s.gatewayRepo, artifact.UUID, ouID, s.logger)
	if s.a2aPublicationRepo != nil {
		if pubErr := s.a2aPublicationRepo.DeleteForAgent(ctx, ouID, projectName, agentName); pubErr != nil {
			s.logger.Warn("Failed to clear A2A publication queue rows for deleted agent",
				"agentName", agentName, "error", pubErr)
		}
	}
	if delErr := s.artifactRepo.Delete(s.db, artifact.UUID.String()); delErr != nil {
```

`agentManagerService` currently holds neither `gatewayEventsService` nor `deploymentRepo`. Add both as struct fields and constructor parameters (after `a2aPublicationRepo`, before `logger`), update every caller and the wire providers, and regenerate. If `GatewayEventsService` is not already a wire provider reachable here, check how `MCPProxyService` obtains it (`grep -n 'gatewayEventsService' agent-manager-service/services/mcp_proxy_service.go`) and follow that.

The broadcast is unconditional rather than gated on the subtype: deleting an agent that was never an A2A agent sends a delete for an artifact no gateway holds, which every gateway ignores — and the alternative, reading the subtype off a component that may already be gone, is the fragile half of the trade.

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd agent-manager-service && go test ./services/... -run TestBroadcastA2AAgentDeletion`
Expected: PASS.

- [ ] **Step 6: Full build and test**

Run:
```
cd agent-manager-service
make codegen
go build ./... && go test ./services/... && go test -tags=integration ./tests/... -run 'TestDeleteAgent|TestCreateAgent' && make lint
```
Expected: PASS, clean.

- [ ] **Step 7: Commit**

```bash
git add agent-manager-service
git commit -m "feat: broadcast agent.deleted when an agent is deleted

Claude-Session: https://claude.ai/code/session_01JXtNk6KuBdVKYBZj4bYeWo"
```

---

### Task 10: `GET /api/internal/v1/agents/{agentId}`

The one new internal surface M1 needs. The gateway calls it after `agent.deployed` and expects a ZIP containing the artifact YAML.

**Files:**
- Modify: `agent-manager-service/services/gateway_internal_service.go` (after `GetActiveMCPProxyDeploymentByGateway`, `:108`)
- Modify: `agent-manager-service/utils/api.go` (after `CreateMCPProxyYamlZip`, `:153`)
- Modify: `agent-manager-service/controllers/gateway_internal_controller.go` (interface `:37-48`; handler after `GetMCPProxy`, `:230`)
- Modify: `agent-manager-service/api/gateway_internal_routes.go`
- Modify: `agent-manager-service/api/route_authz_invariant_test.go:104-115`
- Modify: `agent-manager-service/utils/errors.go` (or wherever the `Err*NotFound` sentinels live) — add `ErrAgentArtifactNotFound`
- Test: `agent-manager-service/utils/a2a_zip_unit_test.go` (create)

**Interfaces:**
- Consumes: `DeploymentRepository.GetCurrentByGateway`; `resolveAllSecretsInYAML` (`services/gateway_internal_service.go`).
- Produces:
  - `utils.CreateAgentYamlZip(agentYamlMap map[string]string) ([]byte, error)` — one entry per agent, named `agent-<id>.yaml`
  - `(*GatewayInternalAPIService).GetActiveAgentDeploymentByGateway(ctx context.Context, agentID, ouID, gatewayID string) (map[string]string, error)`
  - `GatewayInternalController.GetAgent(w http.ResponseWriter, r *http.Request)`

- [ ] **Step 1: Confirm the fetch contract**

Read `/home/jhivandb/Dev/work/api-platform/gateway/gateway-controller/pkg/controlplane/client.go:3045-3070` and confirm the path is `"/agents/"+agentID` and that the response goes through `ExtractYAMLFromZip`. Then read `ExtractYAMLFromZip`'s implementation and confirm what it expects — in particular whether it takes the first `.yaml` entry or requires exactly one. Match it.

- [ ] **Step 2: Write the failing test**

Create `agent-manager-service/utils/a2a_zip_unit_test.go` (WSO2 2026 header, package `utils`):

```go
package utils

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The gateway fetches the Agent definition as a ZIP and pulls the YAML out of
// it (FetchResourceZip then ExtractYAMLFromZip), so the response shape is fixed
// by that code.
func TestCreateAgentYamlZip(t *testing.T) {
	yamlBody := "apiVersion: gateway.api-platform.wso2.com/v1\nkind: Agent\n"
	data, err := CreateAgentYamlZip(map[string]string{"agent-uuid-1": yamlBody})
	require.NoError(t, err)

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	require.Len(t, reader.File, 1)
	assert.Equal(t, "agent-agent-uuid-1.yaml", reader.File[0].Name)

	rc, err := reader.File[0].Open()
	require.NoError(t, err)
	defer rc.Close()
	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, yamlBody, string(content))
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `cd agent-manager-service && go test ./utils/... -run TestCreateAgentYamlZip`
Expected: FAIL — `undefined: CreateAgentYamlZip`.

- [ ] **Step 4: Write the zip helper**

Append to `agent-manager-service/utils/api.go`, copying `CreateMCPProxyYamlZip` (`:123-153`) exactly and changing only the file-name prefix:

```go
// CreateAgentYamlZip creates a ZIP file containing A2A Agent YAML files.
//
// The gateway fetches an Agent definition with FetchResourceZip and then
// ExtractYAMLFromZip, so the response has to be a zip and not the YAML itself —
// the same contract CreateMCPProxyYamlZip satisfies for MCP proxies.
func CreateAgentYamlZip(agentYamlMap map[string]string) ([]byte, error) {
	// ... body identical to CreateMCPProxyYamlZip, with:
	//     fileName := fmt.Sprintf("agent-%s.yaml", agentID)
}
```

Write the body out in full rather than calling into the MCP helper — the two are separate contracts that happen to coincide today, and coupling them would make a future change to one silently change the other.

- [ ] **Step 5: Add the service lookup**

Append to `services/gateway_internal_service.go`, after `GetActiveMCPProxyDeploymentByGateway`:

```go
// GetActiveAgentDeploymentByGateway retrieves the currently deployed A2A Agent
// artifact for the given UUID on this gateway.
//
// The UUID is the per-environment models.KindAgent artifact row, which is also
// what the agent's API keys are bound to — so the gateway resolves the Agent
// and its keys through the same identity.
func (s *GatewayInternalAPIService) GetActiveAgentDeploymentByGateway(ctx context.Context, agentID, ouID, gatewayID string) (map[string]string, error) {
	deployment, err := s.deploymentRepo.GetCurrentByGateway(agentID, gatewayID, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, utils.ErrAgentArtifactNotFound
	}

	resolvedYaml, err := s.resolveAllSecretsInYAML(ctx, string(deployment.Content))
	if err != nil {
		slog.Error("GatewayInternalAPIService: failed to resolve secrets in Agent YAML",
			"agentID", agentID, "error", err)
		return nil, fmt.Errorf("failed to resolve secrets: %w", err)
	}

	return map[string]string{agentID: resolvedYaml}, nil
}
```

Add the sentinel next to the other `Err*NotFound` values (find them with `grep -rn 'ErrMCPProxyNotFound =' agent-manager-service/utils/`):

```go
	// ErrAgentArtifactNotFound means no active Agent deployment exists for this
	// artifact on this gateway.
	ErrAgentArtifactNotFound = errors.New("agent artifact not found")
```

- [ ] **Step 6: Add the handler**

Add `GetAgent(w http.ResponseWriter, r *http.Request)` to the `GatewayInternalController` interface, and append the handler after `GetMCPProxy`, copying its structure exactly:

```go
// GetAgent handles GET /api/internal/v1/agents/:agentId
//
// The gateway calls this after an agent.deployed event, so the response must be
// a ZIP containing the Agent YAML — see CreateAgentYamlZip.
func (c *gatewayInternalController) GetAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	identity, ok := c.authenticateGateway(w, r, "internal-read")
	if !ok {
		return
	}

	ouID := identity.OUID
	gatewayID := identity.ID
	agentID := r.PathValue("agentId")
	if agentID == "" {
		http.Error(w, "Agent ID is required", http.StatusBadRequest)
		return
	}
	if _, err := uuid.Parse(agentID); err != nil {
		http.Error(w, "Agent ID must be an artifact UUID", http.StatusBadRequest)
		return
	}

	agent, err := c.gatewayInternalService.GetActiveAgentDeploymentByGateway(ctx, agentID, ouID, gatewayID)
	if err != nil {
		if errors.Is(err, utils.ErrAgentArtifactNotFound) || errors.Is(err, utils.ErrDeploymentNotActive) {
			http.Error(w, "No active deployment found for this agent on this gateway", http.StatusNotFound)
			return
		}
		log.Error("Failed to get agent", "error", err)
		http.Error(w, "Failed to get agent", http.StatusInternalServerError)
		return
	}

	zipData, err := utils.CreateAgentYamlZip(agent)
	if err != nil {
		log.Error("Failed to create ZIP file for agent", "agentID", agentID, "error", err)
		http.Error(w, "Failed to create agent package", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"agent-%s.zip\"", agentID))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipData)))

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(zipData); err != nil {
		log.Error("Failed to write ZIP response", "agentID", agentID, "error", err)
	}
}
```

- [ ] **Step 7: Register the route and the allowlist entry**

In `api/gateway_internal_routes.go`, after the MCP line:

```go
	// A2A Agent endpoints. The gateway fetches this after an agent.deployed
	// event and expects a ZIP containing the Agent YAML.
	rr.HandleFuncWithValidation("GET /agents/{agentId}", ctrl.GetAgent)
```

In `api/route_authz_invariant_test.go`, add `"GET /agents/{agentId}",` to the `gatewayInternalRoutes` slice (`:104-115`).

**No audit action is needed.** `shouldAudit` (`audit/policy.go:124`) audits a GET only when it is in `sensitiveReadPaths`; `GET /mcp-proxies/{proxyId}` is not there either, and this route serves an Agent definition rather than key material.

- [ ] **Step 8: Run the tests to verify they pass**

Run:
```
cd agent-manager-service
go test ./utils/... -run TestCreateAgentYamlZip
go test ./api/... -run 'TestSecurityInvariant|TestAudit'
go build ./... && make lint
```
Expected: PASS, clean. If `TestSecurityInvariant…` fails naming the new route, the allowlist entry in Step 7 was missed or its string does not match the registered pattern character for character.

- [ ] **Step 9: Commit**

```bash
git add agent-manager-service
git commit -m "feat: serve the Agent definition at GET /api/internal/v1/agents/{agentId}

The gateway fetches this after agent.deployed and extracts the YAML from a
zip, so the response matches the contract GetMCPProxy already satisfies.

Claude-Session: https://claude.ai/code/session_01JXtNk6KuBdVKYBZj4bYeWo"
```

---

### Task 11: OpenAPI subtype enum and generated models

**Files:**
- Modify: `agent-manager-service/docs/api_v1_openapi.yaml:17920-17925`
- Modify: `agent-manager-service/spec/*` (regenerated)

**Interfaces:**
- Consumes: nothing.
- Produces: `a2a-agent` accepted by the `agentSubType` enum on the kind-version response.

- [ ] **Step 1: Find every subtype enum**

Run: `cd agent-manager-service && grep -n 'chat-api' docs/api_v1_openapi.yaml`

There is exactly one enum today, at `:17922-17925` on the agent-kind-version schema's `agentSubType`. The `AgentType.subType` property (`:11476-11478`) is a free-form string with no enum — leave it alone. If the grep shows more occurrences than these, update every enum it finds and none of the free-form strings.

- [ ] **Step 2: Extend the enum**

At `:17920-17925`:

```yaml
        agentSubType:
          type: string
          description: Agent sub-type (chat-api, custom-api or a2a-agent)
          enum:
            - chat-api
            - custom-api
            - a2a-agent
```

- [ ] **Step 3: Regenerate**

Run: `cd agent-manager-service && make spec`
Expected: `spec/` changes. Review with `git diff --stat agent-manager-service/spec` — the diff should be small and confined to the subtype enum's generated validation. If it is large, the generator version drifted; check `scripts/gen_client.sh` and stop rather than committing an unrelated regeneration.

- [ ] **Step 4: Build and test**

Run: `cd agent-manager-service && go build ./... && go test ./... && make lint`
Expected: PASS, clean.

- [ ] **Step 5: Commit**

```bash
git add agent-manager-service/docs/api_v1_openapi.yaml agent-manager-service/spec
git commit -m "feat: accept a2a-agent in the agent subtype enum

Claude-Session: https://claude.ai/code/session_01JXtNk6KuBdVKYBZj4bYeWo"
```

---

### Task 12: CLI `agent create --subtype a2a-agent`

**Files:**
- Modify: `cli/pkg/cmd/agent/create/request.go:35-36`, `:176-194`, `:255-270`
- Modify: `cli/pkg/cmd/agent/create/validation.go:184-210`
- Modify: `cli/pkg/cmd/agent/create/create.go:148`, `:162`, `:180`
- Modify: `cli/pkg/cmd/agent/create/template.go:33`, `:57`
- Test: `cli/pkg/cmd/agent/create/a2a_test.go` (create)

**Interfaces:**
- Consumes: the service's `a2a-agent` subtype (Task 2).
- Produces: `subTypeA2A = "a2a-agent"` in package `create`; `--subtype a2a-agent` accepted, requiring `--port` and rejecting `--openapi-spec`.

- [ ] **Step 1: Write the failing tests**

Read `cli/pkg/cmd/agent/create/validation_test.go:270-310` and `:440-480` first, and match their helper usage (`opts`, `assertContains`, `details`) exactly.

Create `cli/pkg/cmd/agent/create/a2a_test.go`:

```go
package create

import (
	"testing"
)

// An a2a agent serves its own agent card and has no OpenAPI document, so the
// spec flag is not merely optional — passing it is a mistake worth naming.
func TestA2ASubtypeRejectsOpenAPISpec(t *testing.T) {
	opts := validInternalOptions()
	opts.SubType = subTypeA2A
	opts.OpenAPISpec = "/openapi.yaml"

	details := droppedInternalFlags(opts)
	assertContains(t, details, "--openapi-spec is not allowed for subtype a2a-agent")
}

// The port is where the agent's A2A server listens; the gateway's upstream is
// derived from the endpoint it declares, so there is no default to fall back on.
func TestA2ASubtypeRequiresPort(t *testing.T) {
	req := validInternalRequest()
	subType := subTypeA2A
	req.AgentType.SubType = &subType
	req.InputInterface.Port = nil

	v := violations(req)
	assertContains(t, v, "spec.inputInterface.port is required for subtype a2a-agent")
}

func TestA2ASubtypeIsAValidSubtype(t *testing.T) {
	req := validInternalRequest()
	subType := subTypeA2A
	req.AgentType.SubType = &subType
	port := int32(9099)
	req.InputInterface.Port = &port

	for _, v := range violations(req) {
		if v == `spec.agentType.subType must be "chat-api", "custom-api" or "a2a-agent", got "a2a-agent"` {
			t.Fatalf("a2a-agent must be accepted as a subtype")
		}
	}
}
```

Replace `validInternalOptions()`, `validInternalRequest()` and `violations()` with whatever the existing tests actually use — read `validation_test.go:35-60` for the real helper names and adapt. Do not add new helpers if equivalents exist.

- [ ] **Step 2: Run to verify they fail**

Run: `cd cli && go test ./pkg/cmd/agent/create/... -run A2A`
Expected: FAIL — `undefined: subTypeA2A`.

- [ ] **Step 3: Add the constant and validation**

In `request.go:35-36`:

```go
	subTypeChatAPI   = "chat-api"
	subTypeCustomAPI = "custom-api"
	// subTypeA2A is an agent that speaks the A2A protocol. It declares a port
	// like a custom-api agent but carries no OpenAPI document: it serves its own
	// agent card, and the platform stores no copy.
	subTypeA2A = "a2a-agent"
```

In `validation.go`, add a case to the subtype switch (`:189`) and update the two message strings:

```go
	switch subType {
	case subTypeChatAPI:
	case subTypeCustomAPI:
		// ... unchanged ...
	case subTypeA2A:
		if req.InputInterface == nil || req.InputInterface.Port == nil {
			v = append(v, "spec.inputInterface.port is required for subtype a2a-agent")
		}
	case "":
		if hasRepo {
			v = append(v, "spec.agentType.subType is required for internal provisioning (chat-api, custom-api or a2a-agent)")
		}
	default:
		v = append(v, fmt.Sprintf("spec.agentType.subType must be %q, %q or %q, got %q",
			subTypeChatAPI, subTypeCustomAPI, subTypeA2A, subType))
	}
```

Search `validation_test.go` for the old `must be %q or %q` assertion string (`:472`) and update it to the new three-value wording.

- [ ] **Step 4: Reject the spec flag**

In `request.go`, inside `droppedInternalFlags`, after the chat-api block:

```go
	if opts.SubType == subTypeA2A && opts.OpenAPISpec != "" {
		v = append(v, "--openapi-spec is not allowed for subtype a2a-agent")
	}
```

`buildInterface` already does the right thing for `a2a-agent`: it returns early only for `subTypeChatAPI`, so an a2a agent keeps the caller's `--port` and picks up `--base-path` when given, and never sets `Schema` because `--openapi-spec` is now refused. Add a comment above the `if opts.SubType == subTypeChatAPI` early return (`:182`) recording that:

```go
	// chat-api's port and paths are fixed by the platform. custom-api and
	// a2a-agent both carry their own; only custom-api adds a schema.
```

- [ ] **Step 5: Update flag help, completion and the template**

`create.go:148`:
```go
	cmd.Flags().StringVar(&opts.SubType, "subtype", "", "Agent sub-type: chat-api, custom-api or a2a-agent")
```

`create.go:162`:
```go
	cmd.Flags().IntVar(&opts.Port, "port", 8000, "Service port (1..65535) (custom-api and a2a-agent only; chat-api uses a fixed port)")
```

`create.go:180`:
```go
		return []string{subTypeChatAPI, subTypeCustomAPI, subTypeA2A}, cobra.ShellCompDirectiveNoFileComp
```

`template.go:33`:
```yaml
    subType: chat-api           # or: custom-api, a2a-agent (see inputInterface below)
```

and after the `custom-api only` block at `:57`, add:

```yaml
    # --- a2a-agent only ---
    # An A2A agent needs only a port: it serves its own agent card, so there is
    # no OpenAPI document to point at.
```

Read the surrounding template text and match its comment style and indentation exactly.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd cli && go test ./pkg/cmd/agent/create/...`
Expected: PASS — including the pre-existing tests, one of whose assertion strings you updated in Step 3.

- [ ] **Step 7: Build and lint**

Run: `cd cli && go build ./... && go vet ./...`
Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add cli
git commit -m "feat(cli): accept --subtype a2a-agent in agent create

Claude-Session: https://claude.ai/code/session_01JXtNk6KuBdVKYBZj4bYeWo"
```

---

### Task 13: Console agent-creation flow

**Files:**
- Modify: `console/workspaces/libs/types/src/api/agents.ts:68`
- Modify: `console/workspaces/libs/views/src/utils/provisionTypes.ts:30-37`
- Modify: `console/workspaces/pages/add-new-agent/src/components/InputInterface.tsx:48-67`, `:194-215`
- Modify: `console/workspaces/pages/add-new-agent/src/form/schema.ts:168`, `:236-265`
- Modify: `console/workspaces/pages/add-new-agent/src/utils/buildAgentPayload.ts:200-262`
- Test: `console/workspaces/pages/add-new-agent/src/utils/buildAgentPayload.a2a.test.ts` (create)

**Interfaces:**
- Consumes: the API's `a2a-agent` subtype (Tasks 2, 11).
- Produces: `InputInterfaceType` becomes `'DEFAULT' | 'CUSTOM' | 'A2A'`; selecting the A2A card produces `agentType.subType === "a2a-agent"` with an `inputInterface` carrying `type` and `port` only.

- [ ] **Step 1: Find the test runner**

Run: `cd console && cat package.json | head -40 && ls workspaces/pages/add-new-agent/src/**/*.test.ts* 2>/dev/null | head`

Identify the test command (likely `pnpm test` or `npx vitest`) and an existing test file to copy the import style from. If `add-new-agent` has **no** test files at all, do not introduce a test framework — skip Steps 2 and 5, and verify by building and by reading the diff instead. Say so in the commit message.

- [ ] **Step 2: Write the failing test**

Create `console/workspaces/pages/add-new-agent/src/utils/buildAgentPayload.a2a.test.ts`, copying the imports and form-fixture construction from whichever existing test you found:

```ts
import { describe, expect, it } from 'vitest';
import { buildAgentPayload } from './buildAgentPayload';

describe('buildAgentPayload for an A2A agent', () => {
  const params = { orgName: 'acme', projName: 'checkout' };

  const formValues = {
    // ...copy a complete valid CreateAgentFormValues fixture from the existing
    // test or from InternalAgentFlow.tsx's initial state, then override:
    deploymentType: 'new',
    interfaceType: 'A2A',
    port: 9099,
  } as never;

  it('sends the a2a-agent subtype', () => {
    const { body } = buildAgentPayload(formValues, params);
    expect(body.agentType).toEqual({ type: 'agent-api', subType: 'a2a-agent' });
  });

  // An A2A agent serves its own agent card, so there is no OpenAPI path to send.
  it('sends a port and no schema', () => {
    const { body } = buildAgentPayload(formValues, params);
    expect(body.inputInterface).toEqual({ type: 'HTTP', port: 9099 });
  });
});
```

Check `buildAgentPayload`'s real exported name and argument list at `console/workspaces/pages/add-new-agent/src/utils/buildAgentPayload.ts` and match it.

- [ ] **Step 3: Run to verify it fails**

Run: `cd console && pnpm test buildAgentPayload.a2a` (or the equivalent command from Step 1)
Expected: FAIL — the payload carries `subType: 'chat-api'`, because the current ternary maps anything that is not `CUSTOM` to chat.

- [ ] **Step 4: Make the changes**

`libs/types/src/api/agents.ts:68`:
```ts
export type InputInterfaceType = 'DEFAULT' | 'CUSTOM' | 'A2A';
```

`libs/views/src/utils/provisionTypes.ts`:
```ts
export function displayAgentSubType(subType?: string) {
  switch (subType) {
    case "custom-api":
      return "Custom API";
    case "chat-api":
      return "Chat";
    case "a2a-agent":
      return "A2A";
  }
}
```

`pages/add-new-agent/src/components/InputInterface.tsx`, add a third card to `inputInterfaces` (`:48-67`):
```ts
  {
    label: "A2A Agent",
    description:
      "Speaks the A2A protocol. The agent serves its own agent card; the gateway exposes both A2A transports.",
    default: false,
    value: "A2A",
  },
```

and a collapse panel after the `CUSTOM` one (`:205`), containing only the port field. Copy the `CUSTOM` panel's port `<Form.ElementWrapper>` / `<TextField>` markup verbatim and drop the OpenAPI-path and base-path fields:

```tsx
        <Collapse in={formData.interfaceType === "A2A"}>
          <Form.Stack spacing={2}>
            <Alert severity="info">
              The gateway exposes both A2A transports — JSON-RPC at{" "}
              <strong>/rpc</strong> and HTTP+JSON at <strong>/rest</strong> — and
              serves the agent's own card at the well-known path.
            </Alert>
            {/* port field, copied from the CUSTOM panel */}
          </Form.Stack>
        </Collapse>
```

Also extend `handleSelect` (`:94-133`): the branch that clears `port`/`openApiPath`/`basePath` errors currently fires for anything that is not `CUSTOM`. For `A2A` it must validate `port` and clear only `openApiPath` and `basePath`. Read the function in full and restructure it to a three-way switch rather than bolting on another condition.

`pages/add-new-agent/src/form/schema.ts:168`:
```ts
  interfaceType: z.enum(['DEFAULT', 'CUSTOM', 'A2A']),
```

and in the refinements (`:236-265`): the port checks at `:236` and `:244` must fire for `A2A` as well as `CUSTOM`; the `basePath` (`:253`) and `openApiPath` (`:261`) checks must stay `CUSTOM`-only. Change the first two conditions to `(data.interfaceType === 'CUSTOM' || data.interfaceType === 'A2A')` and leave the last two alone.

`pages/add-new-agent/src/utils/buildAgentPayload.ts:200-262` — replace the two-way ternaries with an explicit mapping. Add above `agentType`:

```ts
        // Three subtypes now, so a two-way ternary would silently map A2A to
        // chat. An A2A agent needs a port and no schema: it serves its own
        // agent card and the platform stores no copy.
        const subTypeByInterface = {
          CUSTOM: "custom-api",
          A2A: "a2a-agent",
          DEFAULT: "chat-api",
        } as const;
```

then:

```ts
        agentType: {
          type: "agent-api",
          subType: subTypeByInterface[data.interfaceType],
        },
```

and:

```ts
        inputInterface: {
          type: "HTTP",
          ...(data.interfaceType === "CUSTOM"
            ? {
              port: Number(data.port),
              basePath: data.basePath || "/",
              schema: { path: data.openApiPath ?? "" },
            }
            : data.interfaceType === "A2A"
            ? { port: Number(data.port) }
            : {}),
        },
```

Leave the external-agent branch at `:281` (`subType: "custom-api"` under `external-agent-api`) unchanged — external A2A agents are explicitly out of scope for M1.

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd console && pnpm test buildAgentPayload.a2a`
Expected: PASS.

- [ ] **Step 6: Typecheck and build**

Run: `cd console && pnpm build` (or the repo's typecheck script — check `console/package.json`)
Expected: clean. TypeScript will flag any `switch`/lookup over `InputInterfaceType` that did not gain an `A2A` arm; fix each one it names.

- [ ] **Step 7: Commit**

```bash
git add console
git commit -m "feat(console): offer A2A as an agent interface type

Claude-Session: https://claude.ai/code/session_01JXtNk6KuBdVKYBZj4bYeWo"
```

---

### Task 14: End-to-end lifecycle test

The last gate: create → deploy → publish → delete, asserted through the service layer with mocked infrastructure. Everything before this tested one seam; this tests that they join.

**Files:**
- Create: `agent-manager-service/tests/a2a_agent_lifecycle_test.go`

**Interfaces:**
- Consumes: everything from Tasks 2–10.
- Produces: nothing.

- [ ] **Step 1: Read the model**

Read `agent-manager-service/tests/deploy_agent_test.go` in full. It is the closest existing shape: it drives `DeployAgent` through the HTTP surface with a mocked OpenChoreo client. Copy its scaffolding.

- [ ] **Step 2: Write the test**

Create `agent-manager-service/tests/a2a_agent_lifecycle_test.go`, first line `//go:build integration`, then the WSO2 2026 header, package `tests`.

Structure it as one function with three subtests sharing an app:

```go
func TestA2AAgentLifecycle(t *testing.T) {
	// Scaffolding: an app built with apitestutils.MakeAppClientWithDeps, whose
	// OpenChoreo mock's GetReleaseBindingServiceURLFunc is switchable so the
	// not-ready and ready cases can both be driven.

	t.Run("deploy queues a publication rather than publishing inline", func(t *testing.T) {
		// Create an a2a-agent, then deploy it.
		// Assert: an a2a_publications row exists for (org, project, agent, env)
		//   with status pending;
		// assert: no deployments row was written for the artifact yet — the
		//   binding has published no ServiceURL, and an Agent with an empty
		//   upstream must never be emitted.
	})

	t.Run("the reconciler publishes once the binding reports a service URL", func(t *testing.T) {
		// Point GetReleaseBindingServiceURLFunc at a real address, then drive one
		// reconciler cycle directly (construct the reconciler over the same
		// repositories the app uses and call its exported Start/Stop, or call the
		// unexported publishOne — whichever the wiring in this package allows).
		// Assert: exactly one deployments row exists whose Kind is Agent and
		//   whose Content unmarshals to an A2AAgentDeploymentYAML with that
		//   upstream, both transports, and no agentCard or resilience block;
		// assert: the publication row is now published.
	})

	t.Run("redeploy re-queues and re-publishes", func(t *testing.T) {
		// Deploy again. Assert the publication row is pending with attempt_count
		// reset, and after another cycle a SECOND deployments row exists — a
		// redeploy writes a fresh row and re-broadcasts CREATE, as MCP does.
	})

	t.Run("delete broadcasts agent.deleted and clears the queue", func(t *testing.T) {
		// Delete the agent. Assert an agent.deleted event was published carrying
		// the artifact UUID, and that no a2a_publications row survives.
	})
}
```

Fill each subtest in against the real helpers. Where a subtest cannot be driven through the HTTP surface — the reconciler cycle in particular — construct the service directly over the same repositories the app was built with, and say so in a comment.

If `apitestutils` gives no way to reach the app's repositories or its event hub, add the narrowest accessor needed to `tests/apitestutils/` rather than reaching into service internals from the test, and comment why it exists.

- [ ] **Step 3: Run it**

Run: `cd agent-manager-service && go test -tags=integration ./tests/... -run TestA2AAgentLifecycle -v`
Expected: PASS, all four subtests.

- [ ] **Step 4: Run everything, in both tiers**

Run:
```
cd agent-manager-service && go build ./... && go test ./... && go test -tags=integration ./tests/... && make lint
cd ../cli && go build ./... && go test ./...
cd ../console && pnpm build
```
Expected: PASS, clean. Report any failure with its output rather than working around it.

- [ ] **Step 5: Commit**

```bash
git add agent-manager-service/tests
git commit -m "test: cover the a2a agent create-deploy-publish-delete lifecycle

Claude-Session: https://claude.ai/code/session_01JXtNk6KuBdVKYBZj4bYeWo"
```

---

## Coverage against the spec

| Spec section | Tasks |
|---|---|
| §1 The subtype — constant, `utils.go:544`, `components.go:495/511` | 2 |
| §1 — `agent_manager.go:473/3228` trait gates | 3 |
| §1 — `agent_manager.go:3697` trait-env-config key | 1 (test), 3 (fix) |
| §1 — OpenAPI enum, CLI, console | 11, 12, 13 |
| §2 The gateway Agent resource — YAML shape, transports, no card, no resilience, context, vhost | 4 |
| §2 — `upstream.url` from the binding status; the ordering problem; the selection rule | 6, 7 |
| §2 — `metadata.name` sanitizing | 4 |
| §2 — artifact reuse and the inherited API-key path | 1 (verify), 8 |
| §2 — publication: `deployments` row + broadcast | 5, 7 |
| §2 — `GET /agents/{agentId}` zip contract | 10 |
| §3 The auth chain — CORS then one auth policy, both modes | 4 (builder), 7 (wiring), 14 |
| §4 Lifecycle — deploy/redeploy | 8, 14 |
| §4 Lifecycle — delete | 9, 14 |
| §4 Lifecycle — gateway reconnect | 1 Step 6 (the gap is recorded; the `deployments` row is written in 7) |
| Risks 1–5 | 1 |
| Testing — service unit tests | 4, 5, 7, 8, 9 |
| Testing — `tests/create_agent_a2a_test.go` | 3 |
| Testing — golden YAML | 4 |
| Testing — lifecycle tests | 14 |

## Deviations from the spec, and why

Three, all flagged rather than silent:

1. **The publication reconciler is a new table plus a new background loop.** The spec says publication "should hang off the existing readiness machinery (`bootDeadlineExceeded`, `agentStartupBudget`)". That machinery lives in the OpenChoreo client's deployment-status *read* path (`clients/openchoreosvc/client/deployments.go:1388`) and runs only when a caller lists deployments — hanging a publish off it would put a side effect inside a GET and would never fire for an agent nobody is watching. Task 7 keeps the spec's *intent* (publication is driven by readiness, not by the deploy call, and never emits an empty upstream) and borrows `agentStartupBudget`'s reasoning for its own attempt budget, but drives it from a durable queue.

2. **`GET /deployments` still returns an empty list.** The spec's §2 says the `deployments` row is what makes an Agent survive a gateway restart. Task 1 Step 6 establishes that the endpoint reports nothing for any kind today, so bulk sync is inert for MCP and LLM artifacts too. Fixing it would change behaviour for those, so it stays out of M1 and is recorded as a gap to file separately. The row is still written, because the incremental fetch-back path reads it.

3. **The gateway is resolved as the environment's *ingress* gateway**, not through `resolveEgressGatewayForArtifact`. The spec says publication "mirrors MCP exactly", and MCP uses egress resolution — but egress is for outbound LLM/MCP artifacts, and an A2A agent is inbound traffic, the same slot a REST agent's `api-configuration` trait targets. Task 7's `resolveGateway` documents this.
