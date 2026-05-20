# Manual Instrumentation Sample Agent

A runnable agent that instruments itself by hand. Instead of relying on the
Traceloop SDK's auto-instrumentation, it emits OpenTelemetry GenAI spans directly,
against WSO2 Agent Manager's **manual instrumentation contract**.

## What this demonstrates

Most AMP agents are auto-instrumented. A platform-injected init container, or the
`amp-instrument` CLI, loads the Traceloop SDK, which monkey-patches known
frameworks like LangChain, LlamaIndex, and CrewAI.

But Traceloop doesn't cover every framework. If yours isn't covered, or you just
want full control over the spans you emit, you take the **manual path**: turn
auto-instrumentation off and emit your own spans. This sample is a worked example
of that path. It's a small retrieval-augmented agent on a hand-written framework
(plain Python, nothing Traceloop knows about), and its spans render in the AMP
Console exactly like auto-instrumented ones.

It emits a span for every kind in the contract, with every attribute, so it
doubles as an executable reference. For the full supported schema, see
[the contract](https://wso2.github.io/agent-manager/docs/latest/components/amp-instrumentation/#the-contract)
in the WSO2 Agent Manager documentation.

## How it works

`init_otel()`, from the `amp-instrumentation` package, configures the
OpenTelemetry exporter to AMP. After that the agent emits its own spans. There's
no Traceloop SDK and no `amp-instrument` CLI involved.

One `/chat` request produces one trace:

```text
invoke_agent  (agent, root)
└── rag-pipeline  (chain)
    ├── embeddings     (embedding)   real OpenAI call
    ├── vector_search  (retriever)   simulated
    ├── rerank         (rerank)      simulated
    ├── chat           (llm)         simulated: decides to call a tool
    ├── execute_tool   (tool)        real local call
    └── chat           (llm)         real OpenAI call: final answer
```

The `embeddings` span and the final `chat` span wrap real OpenAI calls. The
`vector_search`, `rerank`, and the first `chat` span (where the model decides to
call a tool) are simulated, so the trace is deterministic and the sample runs
with only an OpenAI key. In a real agent those would call your vector DB, rerank
provider, and the model; the span attributes are the same either way.

### Attribute coverage

Every span-emitting helper lives in [`instrumentation.py`](./instrumentation.py),
one function per kind. The contract is layered. Layer 1 is the OpenTelemetry
GenAI semantic conventions (`gen_ai.*`, plus `db.*` for retriever spans). Layer 2
is the OpenLLMetry `traceloop.*` extension keys, used for the gaps OTel hasn't
standardized yet.

| Span kind | Helper | Key attributes emitted |
|---|---|---|
| `agent` | `agent_span` | `gen_ai.operation.name=invoke_agent`, `gen_ai.agent.name/description/tools`, `gen_ai.system`, `gen_ai.request.model`, `gen_ai.system_instructions`, `gen_ai.conversation.id`, `gen_ai.input/output.messages`, `gen_ai.usage.*` |
| `chain` | `chain_span` | `traceloop.span.kind=workflow`, `traceloop.entity.input/output` |
| `embedding` | `embedding_span` | `gen_ai.operation.name=embeddings`, `gen_ai.system`, `gen_ai.request/response.model`, `gen_ai.prompt.{i}.content`, `gen_ai.usage.input_tokens` |
| `retriever` | `retriever_span` | `db.system.name`, `db.collection.name`, `db.vector.query.top_k` |
| `rerank` | `rerank_span` | `traceloop.span.kind=rerank`, `gen_ai.operation.name=rerank`, `rerank.model`, `gen_ai.request.model` |
| `tool` | `tool_span` | `gen_ai.operation.name=execute_tool`, `gen_ai.tool.name/description/call.id`, `traceloop.entity.input/output` |
| `llm` | `llm_span` | `gen_ai.operation.name=chat`, `gen_ai.system`, `gen_ai.request/response.model`, `gen_ai.request.temperature`, `gen_ai.input/output.messages`, `gen_ai.input.tools`, `gen_ai.usage.*` |
| any span | `mark_error` | OTel span `Status=Error` plus `error.type`, which the Console shows as an error badge |
| any span | `evaluation_baggage` | W3C baggage `task_id` / `trial_id`, which correlates the trace to an evaluation trial |

This table is a quick map. The authoritative reference, with every attribute and
whether it's required, is [the contract](https://wso2.github.io/agent-manager/docs/latest/components/amp-instrumentation/#the-contract).

## Prerequisites

- Python 3.10 to 3.13.
- An OpenAI API key. The LLM and embedding spans make real OpenAI calls.
- An agent registered in the AMP Console, which gives you an `AMP_AGENT_API_KEY`
  and the OTLP endpoint.

## Run it externally-hosted

First register the agent in the AMP Console and generate its API key. Follow
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

There's no `amp-instrument` wrapper here; that's the auto path. On the manual
path you run the app directly, and `init_otel()` inside `app.py` wires up the
exporter.

## Run it platform-hosted

Deploy it through AMP like any other agent, with one difference: **turn
auto-instrumentation off**. With it off, AMP attaches the env-injection trait
instead of the auto-instrumentation init container. The trait still supplies
`AMP_OTEL_ENDPOINT` and `AMP_AGENT_API_KEY`, so the agent has what it needs to
export. It just does the instrumentation itself.

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

Add one environment variable, `OPENAI_API_KEY`. Leave `AMP_OTEL_ENDPOINT` and
`AMP_AGENT_API_KEY` unset; the env-injection trait provides them.

## Testing

If you ran it externally-hosted, the agent is listening on `localhost:8000`, so
`curl` it directly:

```bash
curl -X POST http://localhost:8000/chat \
  -H 'Content-Type: application/json' \
  -d '{"session_id": "demo-1", "message": "How does AMP handle observability?"}'
```

If you deployed it platform-hosted, the agent runs inside the AMP cluster, not on
your machine. Test it from the agent's **Chat** interface in the AMP Console
instead.

Either way, the response is the agent's answer, and behind it one full trace was
emitted.

The bundled knowledge base is a handful of facts about WSO2 Agent Manager, so ask
it questions it can actually answer:

- `What is WSO2 Agent Manager?`
- `How does AMP handle observability?`
- `How are platform-hosted Python agents instrumented?`
- `When should I use manual instrumentation instead of auto-instrumentation?`
- `What do LLM-as-judge evaluators need from a trace?`

Ask something the knowledge base doesn't cover and the agent says so. That's a
valid trace too, and a good one to inspect.

To see an error badge, point the agent at a model that doesn't exist (edit
`CHAT_MODEL` in `agent.py`). The LLM call fails, `mark_error` flags the agent
span, and the trace shows an error.

## Observe the traces

In the AMP Console, open the agent and go to **OBSERVABILITY → Traces**. A
`/chat` call produces one trace. Its spans carry the per-kind icons, the model
and token chips, the input and output, and an error badge on a failed run.
Because the spans follow the contract, evaluators run against them too.

## File guide

| File | Role |
|---|---|
| `instrumentation.py` | The manual-instrumentation helpers, one per span kind. The contract, in code. |
| `agent.py` | The hand-written RAG agent that calls those helpers around real OpenAI calls. |
| `app.py` | FastAPI entrypoint; calls `init_otel()`. |
| `main.py` | Local runner (`python main.py`). |
| `.env.example` | Environment variable template. |
