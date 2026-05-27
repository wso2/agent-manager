"""Direct Anthropic SDK sample for the matrix.

No framework — exercises Traceloop's instrumentation of the anthropic client
itself. One messages.create call, deterministic via temperature=0.

Cassette: cassettes/anthropic-direct/messages.yaml.
"""
from __future__ import annotations


def run_scenario() -> str:
    from anthropic import Anthropic

    client = Anthropic()
    resp = client.messages.create(
        model="claude-haiku-4-5",
        max_tokens=64,
        temperature=0,
        messages=[
            {"role": "user", "content": "Answer in one word: capital of France?"}
        ],
    )
    return resp.content[0].text if resp.content else ""
