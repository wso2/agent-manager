"""A small RAG agent on a hand-written framework.

This agent is deliberately *not* built on LangChain, LlamaIndex, or CrewAI. It's
plain Python. The Traceloop SDK can't auto-instrument it, which is exactly when
you reach for the manual instrumentation path: you emit your own OpenTelemetry
GenAI spans against AMP's contract. All the span emission lives in
``instrumentation.py``; this file is the agent logic that calls those helpers.

One ``run_agent`` call produces one trace covering every span kind AMP supports:

    invoke_agent (agent, root)
    └── rag-pipeline (chain)
        ├── embeddings    (embedding)
        ├── vector_search (retriever)
        ├── rerank        (rerank)
        ├── execute_tool  (tool)
        └── chat          (llm)

The embeddings and chat spans wrap real OpenAI calls. The retriever and rerank
steps are simulated (no external vector DB or rerank service), so the sample
runs with only an OpenAI key. In a real agent they'd call your vector DB and
rerank provider, and the span attributes stay the same.
"""

from __future__ import annotations

import math
import os
import uuid

from openai import OpenAI

import instrumentation as ix

EMBED_MODEL = "text-embedding-3-small"
CHAT_MODEL = "gpt-4o-mini"
RERANK_MODEL = "rerank-english-v3.0"
TOP_K = 3

SYSTEM_PROMPT = (
    "You are a concise assistant that answers questions about the WSO2 Agent "
    "Manager using only the provided context. If the context does not cover the "
    "question, say so."
)

# A tiny in-memory knowledge base. In a real agent this is your vector DB.
KNOWLEDGE_BASE = [
    {"id": "kb-1", "title": "What AMP is",
     "text": "WSO2 Agent Manager is a platform to run, govern, observe, and "
             "evaluate AI agents at scale."},
    {"id": "kb-2", "title": "Observability",
     "text": "AMP captures every agent interaction (LLM calls, tool calls, "
             "retrievals) as OpenTelemetry traces stored for analysis."},
    {"id": "kb-3", "title": "Auto-instrumentation",
     "text": "Platform-hosted Python agents are auto-instrumented by an injected "
             "init container; externally-hosted agents use the amp-instrument CLI."},
    {"id": "kb-4", "title": "Manual instrumentation",
     "text": "Agents on a framework the Traceloop SDK does not cover emit their "
             "own OpenTelemetry GenAI spans against AMP's published contract."},
    {"id": "kb-5", "title": "Evaluation",
     "text": "AMP runs evaluators over agent traces; LLM-as-judge evaluators need "
             "the span input and output to be populated."},
]

_client: OpenAI | None = None
_doc_vectors: list[list[float]] | None = None


def _openai() -> OpenAI:
    global _client
    if _client is None:
        if not os.getenv("OPENAI_API_KEY"):
            raise RuntimeError("OPENAI_API_KEY is required to run this sample.")
        _client = OpenAI()
    return _client


def _ensure_index() -> list[list[float]]:
    """Embed the knowledge base once (real OpenAI call) and cache the vectors.
    This is index-building, not request handling, so it isn't traced.
    """
    global _doc_vectors
    if _doc_vectors is None:
        resp = _openai().embeddings.create(
            model=EMBED_MODEL, input=[d["text"] for d in KNOWLEDGE_BASE]
        )
        _doc_vectors = [item.embedding for item in resp.data]
    return _doc_vectors


def _cosine(a: list[float], b: list[float]) -> float:
    dot = sum(x * y for x, y in zip(a, b))
    na = math.sqrt(sum(x * x for x in a))
    nb = math.sqrt(sum(y * y for y in b))
    return dot / (na * nb) if na and nb else 0.0


def _word_count(text: str) -> int:
    """A trivial local tool: the kind of function a custom agent calls."""
    return len(text.split())


def run_agent(
    question: str,
    conversation_id: str | None = None,
    task_id: str | None = None,
    trial_id: str | None = None,
) -> str:
    """Run one agent turn and emit one fully-instrumented trace."""
    conversation_id = conversation_id or str(uuid.uuid4())
    client = _openai()
    doc_vectors = _ensure_index()

    tool_defs = [{
        "name": "word_count",
        "description": "Counts the words in a piece of text.",
    }]

    # W3C baggage joins this trace to an evaluation trial, when one drove it.
    with ix.evaluation_baggage(task_id, trial_id):
        with ix.agent_span(
            name="amp-knowledge-agent",
            description="Answers questions about WSO2 Agent Manager from a knowledge base.",
            framework="custom-rag-framework",
            model=CHAT_MODEL,
            system_instructions=SYSTEM_PROMPT,
            conversation_id=conversation_id,
            input_messages=[{"role": "user", "content": question}],
            tools=tool_defs,
        ) as (agent, agent_result):
            try:
                answer, usage = _rag_pipeline(client, doc_vectors, question, tool_defs)
            except Exception as exc:  # noqa: BLE001 - surface any failure as an error span
                ix.mark_error(agent, f"agent run failed: {exc}")
                raise

            agent_result.output_messages = [{"role": "assistant", "content": answer}]
            agent_result.input_tokens = usage[0]
            agent_result.output_tokens = usage[1]
            return answer


def _rag_pipeline(client, doc_vectors, question, tool_defs):
    with ix.chain_span(name="rag-pipeline", workflow_input=question) as (chain, chain_result):
        # 1. Embedding: embed the user's question (real OpenAI call).
        with ix.embedding_span(
            system="openai", request_model=EMBED_MODEL, texts=[question],
        ) as (_embed_span, embed_result):
            resp = client.embeddings.create(model=EMBED_MODEL, input=[question])
            query_vector = resp.data[0].embedding
            embed_result.response_model = resp.model
            embed_result.input_tokens = resp.usage.prompt_tokens

        # 2. Retriever: cosine top-k over the in-memory store.
        with ix.retriever_span(
            vector_db="chroma", collection="amp-knowledge-base", top_k=TOP_K,
        ):
            ranked = sorted(
                zip(KNOWLEDGE_BASE, (_cosine(query_vector, v) for v in doc_vectors)),
                key=lambda pair: pair[1],
                reverse=True,
            )
            hits = [doc for doc, _score in ranked[:TOP_K]]

        # 3. Rerank: reorder the hits (simulated rerank service).
        with ix.rerank_span(
            model=RERANK_MODEL, query=question, candidate_count=len(hits),
        ):
            q_words = {w.lower() for w in question.split()}
            hits = sorted(
                hits,
                key=lambda d: len(q_words & {w.lower() for w in d["text"].split()}),
                reverse=True,
            )

        context_text = "\n".join(f"- {d['title']}: {d['text']}" for d in hits)

        # 4. Tool: a real local tool call.
        with ix.tool_span(
            name="word_count",
            description="Counts the words in a piece of text.",
            call_id=str(uuid.uuid4()),
            arguments={"text": context_text},
        ) as (_tool_span, tool_result):
            tool_result["output"] = {"word_count": _word_count(context_text)}

        # 5. LLM: generate the answer with the retrieved context (real call).
        input_messages = [
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user",
             "content": f"Context:\n{context_text}\n\nQuestion: {question}"},
        ]
        with ix.llm_span(
            system="openai",
            request_model=CHAT_MODEL,
            input_messages=input_messages,
            temperature=0.3,
            tools=tool_defs,
        ) as (_llm_span, llm_result):
            resp = client.chat.completions.create(
                model=CHAT_MODEL, messages=input_messages, temperature=0.3,
            )
            answer = resp.choices[0].message.content or ""
            llm_result.response_model = resp.model
            llm_result.output_messages = [{"role": "assistant", "content": answer}]
            llm_result.input_tokens = resp.usage.prompt_tokens
            llm_result.output_tokens = resp.usage.completion_tokens

        chain_result["output"] = answer
        return answer, (llm_result.input_tokens, llm_result.output_tokens)
