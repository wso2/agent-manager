# Manual Instrumentation Sample Agent

A runnable agent that instruments itself by hand. Instead of relying on the
Traceloop SDK's auto-instrumentation, it emits OpenTelemetry GenAI spans directly,
against WSO2 Agent Manager's **manual instrumentation contract**.

> [!NOTE]
> AMP Lite does not store traces and has no traces page. To see the traces
> this sample emits, run your own OpenTelemetry backend and point
> `AMP_OTEL_ENDPOINT` at its OTLP/HTTP endpoint. See the
> [user guide](../../docs/user-guide.md).

## What this demonstrates

Most Python agents are auto-instrumented: the `amp-instrument` CLI loads the
Traceloop SDK, which monkey-patches known frameworks like LangChain, LlamaIndex,
and CrewAI.

But Traceloop doesn't cover every framework. If yours isn't covered, or you just
want full control over the spans you emit, you take the **manual path**: run
the agent without `amp-instrument` and emit your own spans. This sample is a worked example
of that path. It's a small retrieval-augmented agent on a hand-written framework
(plain Python, nothing Traceloop knows about), and its spans have exactly the
same shape as auto-instrumented ones.

It emits a span for every kind in the contract, with every attribute, so it
doubles as an executable reference. For the full supported schema, see
[the contract](https://wso2.com/agent-platform/docs/latest/guides/amp-instrumentation/#the-contract)
in the WSO2 Agent Manager documentation.

## How it works

`init_otel()`, from the `amp-instrumentation` package, configures the
OpenTelemetry exporter to `AMP_OTEL_ENDPOINT`. After that the agent emits its own spans. There's
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
| any span | `mark_error` | OTel span `Status=Error` plus `error.type`, which trace viewers show as an error |
| any span | `evaluation_baggage` | W3C baggage `task_id` / `trial_id`, which correlates the trace to an evaluation trial |

This table is a quick map. The authoritative reference, with every attribute and
whether it's required, is [the contract](https://wso2.com/agent-platform/docs/latest/guides/amp-instrumentation/#the-contract).

## Prerequisites

- Python 3.10 to 3.13.
- An OpenAI API key. The LLM and embedding spans make real OpenAI calls.
- Agent Manager running. From the repository root, `make up` starts it (see
  [`deployments/standalone`](../../deployments/standalone/README.md)).

## Register it in Agent Manager

You run the agent yourself; Agent Manager registers it and issues its API key.

1. Sign in to the console at <http://localhost:9000> (the port is
   `AMP_API_PORT` in `deployments/standalone/.env`) as `admin`; the password
   is `AMP_ADMIN_PASSWORD` in the same file.
2. A new install has no projects, so create one first: click **Add project**
   (**Create project** on the organization overview), enter a **Name** and
   click **Create**.
3. Open the project, click **New agent**, enter the name
   `Manual Instrumentation Agent` and click **Register**.
4. The **Setup Agent** panel opens with an API key already generated. Copy it
   as `AMP_AGENT_API_KEY`; it is shown only once. To change how long it is
   valid, pick another **Token Duration** and click **Regenerate**.

## Run it

Set the two AMP environment variables yourself and run the agent.
`AMP_OTEL_ENDPOINT` is the OTLP/HTTP base URL of your own OpenTelemetry
collector, without `/v1/traces`:

```bash
cd samples/manual-instrumentation-agent
python3 -m venv .venv && . .venv/bin/activate
pip install -r requirements.txt

export AMP_OTEL_ENDPOINT="http://localhost:4318"   # your own OpenTelemetry collector
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

To see an error, point the agent at a model that doesn't exist (edit
`CHAT_MODEL` in `agent.py`). The LLM call fails, `mark_error` flags the agent
span, and the trace shows an error.

## Observe the traces

Open them in the backend behind your collector; AMP Lite does not store them. A
`/chat` call produces one trace. Its spans carry the span kind, the model and
token usage, the input and output, and an error status on a failed run.

## File guide

| File | Role |
|---|---|
| `instrumentation.py` | The manual-instrumentation helpers, one per span kind. The contract, in code. |
| `agent.py` | The hand-written RAG agent that calls those helpers around real OpenAI calls. |
| `app.py` | FastAPI entrypoint; calls `init_otel()`. |
| `main.py` | Local runner (`python main.py`). |
| `.env.example` | Environment variable template. |
