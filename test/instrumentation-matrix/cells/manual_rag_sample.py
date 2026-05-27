"""Manual-instrumentation cell sample.

Thin shim around the published AMP manual-instrumentation sample at
samples/manual-instrumentation-agent/. The matrix exercises THAT sample so
any drift between it and the contract surfaces here.

The published sample's `run_agent` emits one trace covering every span kind
AMP supports: agent → chain → (embedding, retriever, rerank, llm, tool, llm).
Two OpenAI calls happen — the initial embedding and the final chat — both
cassette-recorded.
"""
from __future__ import annotations

import sys
from pathlib import Path

# Add the published-sample directory to sys.path so we can import its modules
# directly. The cell ends up testing exactly what customers see.
_REPO_ROOT = Path(__file__).resolve().parents[3]
_SAMPLE_DIR = _REPO_ROOT / "samples" / "manual-instrumentation-agent"
if str(_SAMPLE_DIR) not in sys.path:
    sys.path.insert(0, str(_SAMPLE_DIR))


def run_scenario() -> str:
    # Imported here (not at module top) so the sys.path edit above happens
    # before resolution and our cell-bootstrap initializes OTel first.
    from agent import run_agent

    return run_agent("What is AMP's approach to observability?")
