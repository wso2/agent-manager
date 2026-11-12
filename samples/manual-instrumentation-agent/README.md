# Manual Instrumentation Sample Agent

A runnable agent that instruments itself by hand. Instead of relying on the
Traceloop SDK's auto-instrumentation, it emits OpenTelemetry GenAI spans directly,
against WSO2 Agent Manager's **manual instrumentation contract**.

## What this demonstrates

Most Python agents are auto-instrumented: the `amp-instrument` CLI loads the
Traceloop SDK, which monkey-patches known frameworks like LangChain, LlamaIndex,
and CrewAI.

But Traceloop doesn't cover every framework. If yours isn't covered, or you just
want full control over the spans you emit, you take the **manual path**: run
the agent without `amp-instrument` and emit your own spans. This sample is a worked example
of that path. It's a small retrieval-augmented agent on a hand-written framework
(plain Python, nothing Traceloop knows about), and its spans render in the AMP
Console exactly like auto-instrumented ones.

It emits a span for every kind in the contract, with every attribute, so it
doubles as an executable reference. For the full supported schema, see
[the contract](https://wso2.com/agent-platform/docs/latest/guides/amp-instrumentation/#the-contract)
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
whether it's required, is [the contract](https://wso2.com/agent-platform/docs/latest/guides/amp-instrumentation/#the-contract).

## Prerequisites

- Python 3.10 to 3.13.
- An OpenAI API key. The LLM and embedding spans make real OpenAI calls.
- Agent Manager running. From the repository root, `make up` starts it (see
  [`deployments/standalone`](../../deployments/standalone/README.md)).

## Register it in Agent Manager

You run the agent yourself; Agent Manager registers it and issues the token its
traces are sent with.

1. Sign in to the console at <http://localhost:3000> as `admin` (the password is
   `THUNDER_ADMIN_PASSWORD` in `deployments/standalone/.env`).
2. Open the **Default** project, click **Add Agent**, and register an agent
   named `Manual Instrumentation Agent`.
3. In the **Set up agent** panel that opens, pick a token duration and click
   **Generate**. Copy the `AMP_OTEL_ENDPOINT` and `AMP_AGENT_API_KEY` values it
   shows.

## Run it

Set the two AMP environment variables yourself and run the agent:

```bash
cd samples/manual-instrumentation-agent
python3 -m venv .venv && . .venv/bin/activate
pip install -r requirements.txt

export AMP_OTEL_ENDPOINT="http://default-default.gateway.localhost:19080/otel"
export AMP_AGENT_API_KEY="<key-from-the-console>"
export OPENAI_API_KEY="<your-openai-key>"

python main.py
```

There's no `amp-instrument` wrapper here; that's the auto path. On the manual
path you run the app directly, and `init_otel()` inside `app.py` wires up the
exporter.

## Testing

The agent is listening on `localhost:8000`, so `curl` it directly:

```bash
curl -X POST http://localhost:8000/chat \
  -H 'Content-Type: application/json' \
  -d '{"session_id": "demo-1", "message": "How does AMP handle observability?"}'
```

The response is the agent's answer, and behind it one full trace was
emitted.

The bundled knowledge base is a handful of facts about WSO2 Agent Manager, so ask
it questions it can actually answer:

- `What is WSO2 Agent Manager?`
- `How does AMP handle observability?`
- `How are Python agents auto-instrumented?`
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
