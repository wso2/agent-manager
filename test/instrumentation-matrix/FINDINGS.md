# Instrumentation matrix — findings log

Every time the matrix exposes a gap that requires either an upstream change, an
observer change, or a schema concession, an entry lands here. Each entry has an
ID, the affected combo, the symptom, what was done about it, and what would let
the concession be undone.

The schema rules in `contracts/traceloop/v1/` and `cmd/gen-contract/contract.go`
should never relax silently — every relaxation has an `F-NNN` entry justifying
it. When upstream fixes a finding, drop the concession, regenerate the schemas
via `make gen-instrumentation-contract`, and **remove the entry** — a fully
resolved finding's rationale lives on in its commit message + the code
comments, so this log stays focused on what's still active. (IDs are never
reused, so removed numbers just leave a gap.)

## Conventions

- **ID**: `F-NNN`, monotonically increasing, never reused. Reference from
  commit messages, contract.go, and code comments.
- **Status**: `open` (no fix yet) or `mitigated` (a workaround/concession on
  the AMP side is in force without the upstream being fixed). A genuinely
  resolved finding is removed rather than kept.
- **Combo**: `(provider × version) × (framework × version)` whose emission
  exhibits the gap. If the gap is provider-only, omit the framework half.
- **Symptom**: what the matrix observes — a missing kind, a wrong type,
  spans that don't validate, install failures, etc.
- **Mitigation**: what was changed locally — schema concession, observer fallback,
  classifier rule, skipped cell. Cross-reference the commit SHA.
- **Re-tighten when**: the precise upstream change that would let the
  mitigation be dropped.

---

## F-001 — Traceloop stringifies every `crewai.*` attribute

- **Status**: mitigated
- **Combo**: `traceloop-sdk 0.61.0` × `crewai 1.1.0`
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
- **2026-06-02 (traceloop 0.61.0)**: still stringified — `crewai.agent.max_iter
  = '25'`, `crewai.task.delegations = '0'`, `crewai.task.async_execution =
  'False'`, ids all `str`. Not fixed; concession stays.

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
  into the contract from `agent-manager-observer/opensearch/types.go` comments
  without verifying it had an upstream emitter. Schema fabricated a requirement.
- **Mitigation**: dropped `crewai.task.name` from the crewaitask schema.
  Replaced with required `crewai.task.description` (which IS emitted) so the
  schema still enforces task-identifiability.
- **Re-tighten when**: either (a) the observer is updated to derive
  `CrewAITaskData.Name` from the span name (stripping `.task` suffix) or from
  `crewai.task.description`, or (b) `CrewAITaskData.Name` is removed from the
  observer's data model.

## F-003 — Traceloop's CrewAI does not emit separate tool spans

- **Status**: open / confirmed (see the 2026-06-02 note; F-009 resolved)
- **Combo**: `traceloop-sdk 0.61.0` × `crewai 1.1.0`
- **Discovered**: 2026-05-27
- **Symptom**: CrewAI cell with a tool-using agent never produces a span
  classified as `tool`. Tool execution appears at least as
  `crewai.agent.tools_results` on the agent span.
- **Suspected cause** (provisional): Traceloop's `opentelemetry-instrumentation-crewai`
  doesn't wrap CrewAI's `ToolUsage.execute` / equivalent. The reference
  repo at `nadheesh/2026-AUS-AI-tutorial` notes
  "openinference-instrumentation-crewai" as the source of agent-spans for
  multi-agent traces, suggesting OpenInference may emit separate tool spans
  where Traceloop does not.
- **Why confirmed**: the OTel GenAI semconv allows tool calls to be encoded as
  `gen_ai.tool.call` events on the assistant LLM span rather than as separate
  spans, so an early concern was that the tool signal was simply not being
  captured. The 2026-06-02 revalidation — run after the harness was taught to
  capture span events — settled it: the CrewAI cell emits **zero span events**
  and no separate `tool` span on both 0.60.0 and 0.61.0. The missing tool span
  is a real upstream gap, not a harness blind spot.
- **Mitigation**: `matrix.yaml.frameworks[crewai].spanKinds` is set to
  `[llm, agent, crewaitask]` (omits `tool`) so the cell passes.
- **Re-tighten when**: Traceloop ships a release that wraps CrewAI tool
  execution as separate spans (or as tool-call events the classifier counts),
  OR OpenInference is added as a second instrumentation provider and its CrewAI
  cell asserts the `tool` kind.

## F-004 — `crewai 1.14.x` × `traceloop-sdk 0.61` is unresolvable

- **Status**: mitigated
- **Combo**: `traceloop-sdk 0.61.0` × `crewai 1.14.5`
- **Discovered**: 2026-05-27
- **Symptom**: `pip install` returns `ResolutionImpossible`. `traceloop-sdk 0.61.0`
  requires `opentelemetry-api >=1.38, <2`; `crewai 1.14.5` requires
  `opentelemetry-api ~=1.34`.
- **Suspected cause**: crewai 1.x tightened its OTel pin between 1.1 and 1.14,
  out of step with traceloop's release cadence.
- **Mitigation**: matrix pins `crewai 1.1.0` — the most recent 1.x version
  whose OTel deps are still compatible with traceloop 0.60.
- **Re-tighten when**: Traceloop ships a 0.61+ release with looser
  `opentelemetry-api` requirements; or CrewAI relaxes its `opentelemetry-api`
  pin. At that point the matrix pin can be bumped and the cassette re-recorded.
- **2026-06-02 (traceloop 0.61.0)**: still unresolvable — `traceloop-sdk
  0.61.0` requires `opentelemetry-api>=1.38,<2`, `crewai 1.14.5` requires
  `>=1.34,<1.35`. The 0.61 release did **not** loosen the pin; crewai stays at
  1.1.0.

## F-006 — Traceloop's LlamaIndex `OpenAIEmbedding` instrumentation omits vendor

- **Status**: mitigated
- **Combo**: `traceloop-sdk 0.61.0` × `llama-index 0.12.0`
- **Discovered**: 2026-05-26
- **Symptom**: embedding-kind spans from LlamaIndex have
  `gen_ai.operation.name=embeddings` and `gen_ai.request.model=text-embedding-3-small`
  but no vendor attribute at all (neither `gen_ai.system` nor `gen_ai.provider.name`).
- **Mitigation**: `embedding` kind dropped from `VendorAnyOf` in the codegen.
  LLM kind still requires vendor.
- **Re-tighten when**: Traceloop's LlamaIndex instrumentation emits
  `gen_ai.provider.name` on embedding spans.
- **2026-06-02 (traceloop 0.61.0)**: embedding spans still carry no vendor
  (neither `gen_ai.system` nor `gen_ai.provider.name`). Not fixed; concession
  stays.

## F-010 — Old SDK pins blow up against the httpx the matrix resolves

- **Status**: mitigated
- **Combo**: `openai 1.55.0` × `httpx >= 0.28`; `anthropic 0.40.0` × `httpx >= 0.28`
- **Discovered**: 2026-05-27
- **Symptom**: `TypeError: Client.__init__() got an unexpected keyword argument 'proxies'`
  at SDK client construction. httpx 0.28 removed the `proxies` kwarg; older
  versions of the OpenAI / Anthropic SDKs still pass it.
- **Cause**: Traceloop 0.60 doesn't pin httpx tight; the resolver picks 0.28+
  (current). Old SDK pins predate that breaking change.
- **Mitigation**: matrix pins `openai 2.38.0` and `anthropic 0.45.0` — both
  support httpx 0.28+. The original `1.55.0` / `0.40.0` numbers in the plan
  were speculative, not validated against current httpx; the matrix caught
  that.
- **Re-tighten when**: not applicable — these are forward pins to currently-
  compatible versions; if either SDK regresses or Traceloop pins httpx tight
  enough to require an older SDK, the matrix will surface it.
