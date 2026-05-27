"""Direct OpenAI SDK sample for the matrix.

No framework — exercises Traceloop's instrumentation of the openai client
itself. One chat.completions.create call, deterministic via temperature=0.

Cassette: cassettes/openai-direct/chat.yaml.
"""
from __future__ import annotations


def run_scenario() -> str:
    from openai import OpenAI

    client = OpenAI()
    resp = client.chat.completions.create(
        model="gpt-4o-mini",
        temperature=0,
        messages=[
            {"role": "user", "content": "Answer in one word: capital of France?"}
        ],
    )
    return resp.choices[0].message.content or ""
