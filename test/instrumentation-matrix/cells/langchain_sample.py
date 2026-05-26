"""Minimal LangChain agent sample for the matrix.

Single-turn LLM call via langchain_openai.ChatOpenAI. Deterministic content
(temperature=0, fixed prompt). Recorded in cassettes/langchain/llm_chat_completion.yaml.
"""
from __future__ import annotations


def run_scenario() -> str:
    from langchain_openai import ChatOpenAI

    llm = ChatOpenAI(model="gpt-4o-mini", temperature=0)
    reply = llm.invoke("Answer in one word: capital of France?")
    return getattr(reply, "content", str(reply))
