# Observer attribute-read map

Per-kind catalog of every attribute key the observer reads in
[`opensearch/process.go`](../../opensearch/process.go) and
[`opensearch/crewai_process.go`](../../opensearch/crewai_process.go), grouped
by the `populate*Attributes` function for that kind.

The `gen-contract` tool turns this into JSON Schema bundles under
`test/instrumentation-matrix/contracts/traceloop/v1/`. Edit `contract.go`,
not this file, when the observer's reads change — this file is the rationale
record; `contract.go` is the source of truth that codegen reads.

## Read semantics

The observer is **lenient**: every attribute read uses the
`if v, ok := attrs[key].(string); ok` pattern and leaves the corresponding
`AmpAttributes` field empty when absent. So nothing in the schema is "required
or the observer crashes." `required` in the generated schemas means one of:

- **Classification trigger** — a key the `has<Kind>Attributes` discriminator
  needs to set, otherwise this span gets classified as `unknown` and the wrong
  parser runs (or none does).
- **Downstream-consumer essential** — an attribute that evaluators, the
  Console renderer, or token-cost aggregation will be visibly broken without.
  Token usage on LLM spans is the canonical example.

Everything else is `properties` with a `type:` but not `required`.

## LLM (`populateLLMAttributes`, lines 159–193)

| Key | Read for | Required? |
| --- | --- | --- |
| `gen_ai.operation.name` | classification (`hasLLMAttributes` checks `{chat, text_completion, generate_content}`) | **yes** |
| `gen_ai.request.model` | fallback for `LLMData.Model` when response model absent | **yes** (a model name is needed downstream) |
| `gen_ai.response.model` | preferred `LLMData.Model` | no |
| `gen_ai.system` | `LLMData.Vendor` | **yes** (cost aggregation needs vendor) |
| `gen_ai.request.temperature` | `LLMData.Temperature` | no |
| `gen_ai.usage.input_tokens` (+ legacy `prompt_tokens`) | `LLMTokenUsage.InputTokens` | **yes** (cost) |
| `gen_ai.usage.output_tokens` (+ legacy `completion_tokens`) | `LLMTokenUsage.OutputTokens` | **yes** (cost) |
| `gen_ai.usage.cache_read_input_tokens` | `LLMTokenUsage.CacheReadInputTokens` | no |

**Gap surfaced by Phase 1 matrix run:** Traceloop 0.60 emits
`gen_ai.provider.name` (current OTel GenAI semconv) instead of the legacy
`gen_ai.system`. The observer only reads `gen_ai.system`, so vendor stays empty.
Schema marks `gen_ai.system` required → matrix cells against Traceloop 0.60 go
red until the observer learns to fall back to `gen_ai.provider.name`. That is
exactly the regression the matrix exists to catch.

## Embedding (`populateEmbeddingAttributes`, lines 211–235)

| Key | Read for | Required? |
| --- | --- | --- |
| `gen_ai.operation.name` | classification (`{embeddings, embedding}`) | **yes** |
| `gen_ai.request.model` / `gen_ai.response.model` | `EmbeddingData.Model` | **yes** (one of the two) |
| `gen_ai.system` | `EmbeddingData.Vendor` | **yes** |
| `gen_ai.usage.*` | `EmbeddingData.TokenUsage` | no |
| `gen_ai.embedding.dimension` | classification fallback only | no |

## Tool (`populateToolAttributes`, lines 195–209; `ExtractToolExecutionDetails`, line 1671)

| Key | Read for | Required? |
| --- | --- | --- |
| `traceloop.entity.name` / `function.name` / `gen_ai.tool.name` | `ToolData.Name` (one of) | **yes** (one of) |
| `traceloop.entity.input` / `function.arguments` / `gen_ai.tool.arguments` / `gen_ai.input.messages` | tool input | no |
| `traceloop.entity.output` / `function.result` / `gen_ai.tool.output` / `gen_ai.output.messages` | tool output | no |
| `gen_ai.tool.status` | `SpanStatus.Error` | no |

Schema's `required` lists none of the three name keys directly (JSON Schema
`required` is AND-only without `anyOf`); the codegen tool emits the name keys as
an `anyOf` block.

## Retriever (`populateRetrieverAttributes`, lines 237–262)

| Key | Read for | Required? |
| --- | --- | --- |
| `db.system.name` / `db.system` | `RetrieverData.VectorDB` (one of) | **yes** (one of) |
| `db.collection.name` | `RetrieverData.Collection` | no |
| `db.vector.query.top_k` | `RetrieverData.TopK` | no |

## Rerank

The observer does not have a dedicated `populateRerankAttributes` function.
Rerank spans are classified by `traceloop.span.kind == "rerank"` and otherwise
fall through to `populateLLMAttributes` (when LLM-ish) or remain unenriched.
For now the schema is permissive — no required attributes beyond the envelope.

## Agent (`populateAgentAttributes`, lines 264–320)

| Key | Read for | Required? |
| --- | --- | --- |
| `gen_ai.input.messages` | `AmpAttributes.Input` (preferred) | no |
| `gen_ai.output.messages` | `AmpAttributes.Output` (preferred) | no |
| `traceloop.entity.input` / `.output` | input/output fallback | no |
| `gen_ai.agent.name` | `AgentData.Name` | **yes** |
| `gen_ai.agent.tools` | `AgentData.Tools` (JSON-string) | no |
| `gen_ai.request.model` | `AgentData.Model` | no |
| `gen_ai.system` | `AgentData.Framework` | no |
| `crewai.agent.max_iter` | `AgentData.MaxIter` | no |
| `gen_ai.conversation.id` | `AgentData.ConversationID` | no |

## Chain (`populateChainAttributes`, lines 322–333)

Read-only of input/output via `extractSpanInputOutput`; no chain-specific
required attributes. Schema is permissive beyond the envelope.

## CrewAI task (`populateCrewAITaskAttributes`, lines 335–404 + `crewai_process.go`)

| Key | Read for | Required? |
| --- | --- | --- |
| `crewai.task.name` | `CrewAITaskData.Name` | **yes** |
| `crewai.task.description` | `CrewAITaskData.Description` | no |
| `crewai.task.tools` | `CrewAITaskData.Tools` (JSON-string) | no |
| `traceloop.entity.output` | task output | no |

## Resource attributes (process-level, not span-attribute)

| Key | Required? | Note |
| --- | --- | --- |
| `service.name` | **yes** | Used by every downstream renderer; AMP injects it automatically. |
