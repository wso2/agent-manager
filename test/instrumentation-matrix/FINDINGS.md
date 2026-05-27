# Instrumentation matrix — findings log

Every time the matrix exposes a gap that requires either an upstream change, an
observer change, or a schema concession, an entry lands here. Each entry has an
ID, the affected combo, the symptom, what we did about it, and what would let
us undo the concession.

The schema rules in `contracts/traceloop/v1/` and `cmd/gen-contract/contract.go`
should never relax silently — every relaxation has an `F-NNN` entry justifying
it. When upstream fixes a finding, drop the concession, regenerate the schemas
via `make gen-instrumentation-contract`, and mark the finding `resolved` here.

## Conventions

- **ID**: `F-NNN`, monotonically increasing. Reference from commit messages,
  contract.go, and code comments.
- **Status**: `open` / `mitigated` / `resolved`. A finding can be `mitigated`
  by a workaround on our side without the upstream being fixed.
- **Combo**: `(provider × version) × (framework × version)` whose emission
  exhibits the gap. If the gap is provider-only, omit the framework half.
- **Symptom**: what the matrix observes — a missing kind, a wrong type,
  spans that don't validate, install failures, etc.
- **Mitigation**: what we changed locally — schema concession, observer fallback,
  classifier rule, skipped cell. Cross-reference the commit SHA.
- **Re-tighten when**: the precise upstream change that would let us drop the
  mitigation.

---

## F-001 — Traceloop stringifies every `crewai.*` attribute

- **Status**: mitigated
- **Combo**: `traceloop-sdk 0.60.0` × `crewai 1.1.0`
- **Discovered**: 2026-05-27 (CrewAI cell first record + replay)
- **Symptom**: every attribute under the `crewai.*` namespace arrives on the
  captured span as a Python `str`, even values that are logically `int` /
  `bool` / structured (`crewai.agent.max_iter`, `crewai.task.delegations`,
  `crewai.task.async_execution`, `crewai.task.id`, …). The original v1
  schema declared `crewai.agent.max_iter` as `integer`; emission failed
  validation with `'25' is not of type 'integer'`.
- **Suspected cause**: Traceloop's `opentelemetry-instrumentation-crewai`
  applies `str(...)` (or similar serialization) to every CrewAI attribute
  before calling `span.set_attribute(...)`. Other Traceloop instrumentations
  (OpenAI, LangChain) preserve native types — so this is specific to the
  CrewAI module.
- **Mitigation**: declare every `crewai.*` attribute type as `string` in
  `cmd/gen-contract/contract.go`. The schema reflects reality; if Traceloop
  later tightens, validation tightens with it.
- **Re-tighten when**: Traceloop's CrewAI instrumentation emits native types.
  Check by inspecting a fresh CrewAI cell's `capturedSpans` and grepping for
  any `[int]`/`[bool]` typed `crewai.*` value.

## F-002 — Observer's `CrewAITaskData.Name` has no upstream source

- **Status**: mitigated
- **Combo**: any `crewai *` (CrewAI's Task class lacks a `name` field)
- **Discovered**: 2026-05-27
- **Symptom**: schemas required `crewai.task.name`; cells failed validation
  because the attribute is never emitted. Observer's
  `populateCrewAITaskAttributes` reads `attrs["crewai.task.name"]` and writes
  it to `CrewAITaskData.Name` — leaving `Name` empty in practice.
- **Suspected cause**: CrewAI Tasks are identified by `description` (the
  natural-language prompt), not by a `name` field. The `name` key was carried
  into the contract from `traces-observer-service/opensearch/types.go` comments
  without verifying it had an upstream emitter. Schema fabricated a requirement.
- **Mitigation**: dropped `crewai.task.name` from the crewaitask schema.
  Replaced with required `crewai.task.description` (which IS emitted) so the
  schema still enforces task-identifiability.
- **Re-tighten when**: either (a) the observer is updated to derive
  `CrewAITaskData.Name` from the span name (stripping `.task` suffix) or from
  `crewai.task.description`, or (b) `CrewAITaskData.Name` is removed from the
  observer's data model.

## F-003 — Traceloop's CrewAI 0.60 does not emit separate tool spans

- **Status**: mitigated
- **Combo**: `traceloop-sdk 0.60.0` × `crewai 1.1.0`
- **Discovered**: 2026-05-27
- **Symptom**: CrewAI cell with a tool-using agent never produces a span
  classified as `tool`. Tool execution appears only as
  `crewai.agent.tools_results` on the agent span, not as a child span.
- **Suspected cause**: Traceloop's `opentelemetry-instrumentation-crewai`
  doesn't wrap CrewAI's `ToolUsage.execute` / equivalent. The reference
  repo at `nadheesh/2026-AUS-AI-tutorial` notes "openinference-instrumentation-crewai"
  as the source of agent-spans for multi-agent traces, suggesting OpenInference
  may emit separate tool spans where Traceloop does not.
- **Mitigation**: `matrix.yaml.frameworks[crewai].spanKinds` set to
  `[llm, agent, crewaitask]` (omits `tool`).
- **Re-tighten when**: Traceloop ships a release that wraps CrewAI tool
  execution as separate spans, OR we add OpenInference as a second
  instrumentation provider in v2 of the matrix and the OpenInference CrewAI
  cell asserts `tool` kind.

## F-004 — `crewai 1.14.x` × `traceloop-sdk 0.60` is unresolvable

- **Status**: mitigated
- **Combo**: `traceloop-sdk 0.60.0` × `crewai 1.14.5`
- **Discovered**: 2026-05-27
- **Symptom**: `pip install` returns `ResolutionImpossible`. `traceloop-sdk 0.60.0`
  requires `opentelemetry-api >=1.38, <2`; `crewai 1.14.5` requires
  `opentelemetry-api ~=1.34`.
- **Suspected cause**: crewai 1.x tightened its OTel pin between 1.1 and 1.14,
  out of step with traceloop's release cadence.
- **Mitigation**: matrix pins `crewai 1.1.0` — the most recent 1.x version
  whose OTel deps are still compatible with traceloop 0.60.
- **Re-tighten when**: Traceloop ships a 0.61+ release with looser
  `opentelemetry-api` requirements; or CrewAI relaxes its `opentelemetry-api`
  pin. At that point we can bump the matrix pin and re-record.

## F-005 — Traceloop 0.60 vendor key is `gen_ai.provider.name`, observer only read `gen_ai.system`

- **Status**: resolved (commit `42f4067d`)
- **Combo**: `traceloop-sdk 0.60.0` × any framework
- **Discovered**: 2026-05-26 (LangChain Phase 1 default cell)
- **Symptom**: every Traceloop 0.60+ span produced empty
  `LLMData.Vendor` / `EmbeddingData.Vendor` / `AgentData.Framework`.
- **Cause**: OTel GenAI semconv renamed `gen_ai.system` to `gen_ai.provider.name`
  mid-2025; Traceloop 0.60 emits the new key, observer code only read the old.
- **Fix**: added `extractVendor` helper in `opensearch/process.go` that prefers
  the legacy key and falls back to the current one. Schema's `VendorAnyOf`
  accepts either.

## F-006 — Traceloop's LlamaIndex `OpenAIEmbedding` instrumentation omits vendor

- **Status**: mitigated
- **Combo**: `traceloop-sdk 0.60.0` × `llama-index 0.12.0`
- **Discovered**: 2026-05-26
- **Symptom**: embedding-kind spans from LlamaIndex have
  `gen_ai.operation.name=embeddings` and `gen_ai.request.model=text-embedding-3-small`
  but no vendor attribute at all (neither `gen_ai.system` nor `gen_ai.provider.name`).
- **Mitigation**: `embedding` kind dropped from `VendorAnyOf` in the codegen.
  LLM kind still requires vendor.
- **Re-tighten when**: Traceloop's LlamaIndex instrumentation emits
  `gen_ai.provider.name` on embedding spans.

## F-008 — Traceloop's LangGraph 0.60 doesn't emit separate tool spans

- **Status**: mitigated
- **Combo**: `traceloop-sdk 0.60.0` × `langgraph 0.2.74` (+ `langchain-core` `@tool`)
- **Discovered**: 2026-05-27
- **Symptom**: a LangGraph agent that uses a `@tool` function invoked via
  `ToolNode` produces no span carrying any tool signal (no
  `traceloop.span.kind=tool`, no `gen_ai.tool.name`, no
  `gen_ai.operation.name=execute_tool`). The graph correctly chooses to call
  the tool — the cassette captures a `finish_reason: tool_calls` response —
  but the tool execution itself goes unspanned.
- **Suspected cause**: Traceloop's `opentelemetry-instrumentation-langchain`
  doesn't wrap LangChain's `@tool` decorator or LangGraph's `ToolNode`. The
  tool function runs in-process without an instrumentation layer.
- **Mitigation**: `matrix.yaml.frameworks[langgraph].spanKinds = [llm]`.
- **Re-tighten when**: Traceloop adds tool-call wrapping for LangChain/LangGraph
  tools, OR we add OpenInference as a second provider in v2 of the matrix.

## F-009 — Harness doesn't capture span events

- **Status**: open (harness bug)
- **Combo**: any
- **Discovered**: 2026-05-27 while investigating F-008
- **Symptom**: `harness/test_cell.py:_to_dict` flattens a `ReadableSpan` into
  a dict with `name`, `kind`, `attributes`, `resource`, `traceId`, `spanId`,
  `parentSpanId` — but not `events`. If a provider encodes a logical
  sub-operation (a tool call inside an LLM span, say) as a span event rather
  than a child span, the matrix sees nothing and misclassifies coverage as
  "missing-span-kind" when the data is actually present.
- **Suspected cause**: my own code.
- **Mitigation**: none yet — the missing tool-spans are being treated as
  upstream gaps (F-003, F-008) but could be in-event in some cases. Need to
  verify before tightening the upstream stories.
- **Fix when**: add `events` to the `_to_dict` output, augment the classifier
  to recognize a tool-call event on an LLM span as evidence of a tool-kind
  sub-operation, and add a matching JSON-schema validation slot for span
  events. Reconfirm F-003 and F-008 with the new harness before closing.

## F-007 — Traceloop emits `workflow` / `task` for generic wrapper spans

- **Status**: resolved (commit `62e0a698`)
- **Combo**: `traceloop-sdk 0.60.0` × any framework (LlamaIndex, CrewAI most prominently)
- **Discovered**: 2026-05-26
- **Symptom**: many wrapper spans carry `traceloop.span.kind=workflow` or `task`;
  AMP's observer has only a `chain` kind for "generic workflows".
- **Fix**: classifier (`harness/classify.py`) maps `workflow` and `task` to
  `chain`, but only after the `gen_ai.operation.name` discriminator runs so a
  real embedding span wrapped in a Traceloop `task` still classifies as embedding.
