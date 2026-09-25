"""Minimal Strands agent sample for the matrix.

A model calls a local tool, then produces a final answer. Covers agent,
chain, llm, and tool spans using the shared cassette-replay harness.

Cassette: cassettes/strands/test_emission_cell.yaml.
"""
from __future__ import annotations


def run_scenario() -> str:
    from strands import Agent, tool
    from strands.models.openai import OpenAIModel

    @tool
    def double(x: int) -> int:
        """Return twice the integer x."""
        return x * 2

    agent = Agent(
        model=OpenAIModel(
            model_id="gpt-4o-mini", stream=False, params={"temperature": 0}
        ),
        tools=[double],
        callback_handler=None,
    )
    return str(agent("What is double of 21? Use the tool."))
