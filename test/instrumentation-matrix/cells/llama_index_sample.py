"""Minimal LlamaIndex RAG sample for the matrix.

A tiny in-memory document store + OpenAI embeddings + OpenAI chat. The
embedding call exercises the embedding span kind; the chat call exercises
the LLM span kind. Deterministic via temperature=0.

Cassette: cassettes/llama-index/rag_pipeline.yaml.
"""
from __future__ import annotations


def run_scenario() -> str:
    from llama_index.core import Document, Settings, VectorStoreIndex
    from llama_index.embeddings.openai import OpenAIEmbedding
    from llama_index.llms.openai import OpenAI

    Settings.llm = OpenAI(model="gpt-4o-mini", temperature=0)
    Settings.embed_model = OpenAIEmbedding(model="text-embedding-3-small")

    docs = [
        Document(text="WSO2 Agent Manager is an open platform to run AI agents."),
        Document(text="The capital of France is Paris."),
    ]
    index = VectorStoreIndex.from_documents(docs)
    engine = index.as_query_engine()
    response = engine.query("What is the capital of France?")
    return str(response)
