"""One-shot cassette recorder. Requires OPENAI_API_KEY (and/or ANTHROPIC_API_KEY).

Usage:
    python scripts/record_cassette.py langchain llm_chat_completion
"""
from __future__ import annotations

import importlib.util
import os
import sys
from pathlib import Path

import vcr

HERE = Path(__file__).resolve().parent.parent


def main():
    if len(sys.argv) != 3:
        sys.exit("usage: record_cassette.py <framework> <scenario>")
    framework, scenario = sys.argv[1], sys.argv[2]

    if not os.getenv("OPENAI_API_KEY") and not os.getenv("ANTHROPIC_API_KEY"):
        sys.exit("OPENAI_API_KEY or ANTHROPIC_API_KEY required to record cassettes")

    cassette = HERE / "cassettes" / framework / f"{scenario}.yaml"
    cassette.parent.mkdir(parents=True, exist_ok=True)

    sample_path = HERE / "cells" / f"{framework.replace('-', '_')}_sample.py"
    if not sample_path.exists():
        sys.exit(f"sample not found: {sample_path}")

    # Register the module under its own name in sys.modules so get_type_hints()
    # can find the module's globals when LangGraph (and friends) lazily resolve
    # `from __future__ import annotations` forward references.
    module_name = f"{framework.replace('-', '_')}_sample"
    spec = importlib.util.spec_from_file_location(module_name, sample_path)
    module = importlib.util.module_from_spec(spec)
    assert spec is not None and spec.loader is not None
    sys.modules[module_name] = module
    spec.loader.exec_module(module)

    cfg = {
        "record_mode": "once",
        "filter_headers": [
            ("authorization", "REDACTED"),
            ("x-api-key", "REDACTED"),
            ("openai-organization", "REDACTED"),
        ],
        "filter_post_data_parameters": [("api_key", "REDACTED")],
        "decode_compressed_response": True,
    }

    with vcr.VCR(**cfg).use_cassette(str(cassette)):
        print("cassette:", cassette)
        out = module.run_scenario()
        print("scenario output:", out)


if __name__ == "__main__":
    main()
