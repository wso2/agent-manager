# Manual Instrumentation Sample Agent

A runnable agent that instruments itself by hand against WSO2 Agent Manager's
**manual instrumentation contract** — emitting OpenTelemetry GenAI spans directly
instead of relying on the Traceloop SDK's auto-instrumentation.

## What this demonstrates

Most AMP agents are auto-instrumented: a platform-injected init container (or the
`amp-instrument` CLI) loads the Traceloop SDK, which monkey-patches known
frameworks. That covers LangChain, LlamaIndex, CrewAI, and the rest.

If your agent runs on a framework Traceloop doesn't cover — or you just want full
control over the spans you emit — you take the **manual path**: turn
auto-instrumentation off and emit your own spans. This sample is a worked example
of that path. It's a small retrieval-augmented agent on a hand-written framework
(plain Python, nothing Traceloop knows about), and it produces spans that render
in the AMP Console exactly like auto-instrumented ones.

It covers **every span kind and every attribute** in the contract, so it doubles
as executable reference. The contract itself is documented in the "Manual
instrumentation" section of the WSO2 Agent Manager Instrumentation docs page.

## How it works

`init_otel()` (from the `amp-instrumentation` package) configures the
OpenTelemetry exporter to AMP. After that, the agent emits its own spans — there
is no Traceloop SDK and no `amp-instrument` CLI involved.

One `/chat` request produces one trace:

```
invoke_agent (agent, root)
└── rag-pipeline (chain)
    ├── embeddings    (embedding) — real OpenAI embeddings call
    ├── vector_search (retriever) — cosine top-k over an in-memory store
    ├── rerank        (rerank)    — simulated reranking
    ├── execute_tool  (tool)      — a real local tool
    └── chat          (llm)       — real OpenAI chat completion
```

The retriever and rerank are simulated so the sample runs with only an OpenAI
key. In a real agent they'd call your vector DB and rerank provider — the span
attributes are identical either way.

### Attribute coverage

Every span-emitting helper lives in [`instrumentation.py`](./instrumentation.py),
one function per kind. The contract is layered: **Layer 1** is the OpenTelemetry
GenAI semantic conventions (`gen_ai.*`, plus `db.*` for retriever); **Layer 2** is
the OpenLLMetry `traceloop.*` extension keys for the gaps OTel hasn't standardized.

| Span kind | Helper | Key attributes emitted |
|---|---|---|
| `agent` | `agent_span` | `gen_ai.operation.name=invoke_agent`, `gen_ai.agent.name/description/tools`, `gen_ai.system`, `gen_ai.request.model`, `gen_ai.system_instructions`, `gen_ai.conversation.id`, `gen_ai.input/output.messages`, `gen_ai.usage.*` |
| `chain` | `chain_span` | `traceloop.span.kind=workflow`, `traceloop.entity.input/output` |
| `embedding` | `embedding_span` | `gen_ai.operation.name=embeddings`, `gen_ai.system`, `gen_ai.request/response.model`, `gen_ai.prompt.{i}.content`, `gen_ai.usage.input_tokens` |
| `retriever` | `retriever_span` | `db.system.name`, `db.collection.name`, `db.vector.query.top_k` |
| `rerank` | `rerank_span` | `traceloop.span.kind=rerank`, `gen_ai.operation.name=rerank`, `rerank.model`, `gen_ai.request.model` |
| `tool` | `tool_span` | `gen_ai.operation.name=execute_tool`, `gen_ai.tool.name/description/call.id`, `traceloop.entity.input/output` |
| `llm` | `llm_span` | `gen_ai.operation.name=chat`, `gen_ai.system`, `gen_ai.request/response.model`, `gen_ai.request.temperature`, `gen_ai.input/output.messages`, `gen_ai.input.tools`, `gen_ai.usage.*` |
| any span | `mark_error` | OTel span `Status=Error` + `error.type` → error badge |
| any span | `evaluation_baggage` | W3C baggage `task_id` / `trial_id` → evaluation-trial correlation |

## Prerequisites

- Python 3.10–3.13.
- An OpenAI API key — the LLM and embedding spans make real OpenAI calls.
- An agent registered in the AMP Console, to get an `AMP_AGENT_API_KEY` and the
  OTLP endpoint.

## Run it — externally-hosted

First register the agent in the AMP Console and generate its API key — follow
[Register an externally-hosted agent](https://wso2.github.io/agent-manager/docs/latest/getting-started/create-your-first-agent/#register-an-externally-hosted-agent).
That gives you the OTLP endpoint and the `AMP_AGENT_API_KEY`.

Then set the two AMP environment variables yourself and run the agent:

```bash
cd samples/manual-instrumentation-agent
python3 -m venv .venv && . .venv/bin/activate
pip install -r requirements.txt

export AMP_OTEL_ENDPOINT="<your-amp-otel-endpoint>"
export AMP_AGENT_API_KEY="<key-from-the-amp-console>"
export OPENAI_API_KEY="<your-openai-key>"

python main.py
```

Note there is no `amp-instrument` wrapper — that's the auto path. On the manual
path you run the app directly; `init_otel()` inside `app.py` wires up the exporter.

## Run it — platform-hosted

Deploy it through AMP like any other agent, with **one difference: turn
auto-instrumentation off**. That makes AMP attach the env-injection trait — which
still supplies `AMP_OTEL_ENDPOINT` and `AMP_AGENT_API_KEY` — instead of the
auto-instrumentation init container. The agent then does its own instrumentation.

Follow [Create a platform-hosted agent](https://wso2.github.io/agent-manager/docs/latest/getting-started/create-your-first-agent/#create-a-platform-hosted-agent)
for the full walkthrough. Use these values in the create-agent form:

| Field | Value |
|---|---|
| Display Name | `Manual Instrumentation Agent` |
| GitHub Repository | `https://github.com/wso2/agent-manager` |
| Branch | `main` |
| App Path | `samples/manual-instrumentation-agent` |
| Language | `Python` |
| Language Version | `3.11` |
| Start Command | `python main.py` |
| Port | `8000` |
| **Enable auto instrumentation** | **Off** |

Add one environment variable — `OPENAI_API_KEY`. Leave `AMP_OTEL_ENDPOINT` and
`AMP_AGENT_API_KEY` unset; the env-injection trait provides them.

## Testing

```bash
curl -X POST http://localhost:8000/chat \
  -H 'Content-Type: application/json' \
  -d '{"session_id": "demo-1", "message": "How does AMP handle observability?"}'
```

The response is the agent's answer. Behind it, one full trace was emitted.

To see an **error badge**, point the agent at a model that doesn't exist (edit
`CHAT_MODEL` in `agent.py`): the LLM call fails, `mark_error` flags the agent
span, and the trace shows an error.

## Observe the traces

In the AMP Console, open the agent → **OBSERVABILITY → Traces**. A `/chat` call
produces one trace whose spans carry the per-kind icons, model and token chips,
input/output, and — on a failed run — an error badge. Because the spans follow
the contract, evaluators run against them too.

## File guide

| File | Role |
|---|---|
| `instrumentation.py` | The manual-instrumentation helpers — one per span kind. The contract, in code. |
| `agent.py` | The hand-written RAG agent that calls those helpers around real OpenAI calls. |
| `app.py` | FastAPI entrypoint; calls `init_otel()`. |
| `main.py` | Local runner (`python main.py`). |
| `.env.example` | Environment variable template. |
