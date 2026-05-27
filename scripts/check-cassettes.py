#!/usr/bin/env python3
"""Fail if a committed VCR cassette contains a real API key.

Cassettes under test/instrumentation-matrix/cassettes/ are committed plaintext
so reviewers can read the captured LLM responses. The recording script's
filter_headers / filter_post_data_parameters should strip auth — this is the
belt-and-braces check that nothing got through.

Run from CI on every PR, and as a pre-commit hook if you wire one up.
Exits 0 (clean) or 1 (leak found).
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

# Known live-key prefixes. Add to the list when a new provider's key shape
# becomes a concern. Each pattern is anchored to a prefix that almost never
# appears in benign cassette content.
PATTERNS = [
    # API-key prefixes — credentials, hard fail.
    (re.compile(r"\bsk-(proj-)?[A-Za-z0-9_-]{20,}\b"), "OpenAI API key"),
    (re.compile(r"\bsk-ant-[A-Za-z0-9_-]{20,}\b"), "Anthropic API key"),
    (re.compile(r"\bxai-[A-Za-z0-9_-]{20,}\b"), "xAI API key"),
    (re.compile(r"\bgsk_[A-Za-z0-9_-]{20,}\b"), "Groq API key"),
    # CrewAI's hosted-tracing one-shot access codes. The CrewAI sample sets
    # CREWAI_TRACING_ENABLED=false + CREWAI_DISABLE_TRACING_PROMPT=true to
    # avoid the upload entirely; this regex stays as a backstop in case those
    # env vars stop working or someone removes them.
    (re.compile(r"\bTRACE-[A-Za-z0-9]{8,}\b"), "CrewAI trace access code"),
    # Identifying response-header leftovers. VCR's filter_headers only filters
    # request headers; before_record_response in test_cell.py / record_cassette.py
    # strips these from responses. If they reappear, the hook regressed.
    (re.compile(r"\bproj_[A-Za-z0-9]{20,}\b"), "OpenAI project ID"),
    (re.compile(r"\bwso2-\d+\b"), "OpenAI organization slug"),
    (re.compile(r"\b__cf_bm=[A-Za-z0-9._-]{20,}\b"), "Cloudflare session cookie"),
]

ROOT = Path(__file__).resolve().parent.parent
CASSETTE_DIR = ROOT / "test" / "instrumentation-matrix" / "cassettes"


def main() -> int:
    if not CASSETTE_DIR.is_dir():
        print(f"cassette dir not found: {CASSETTE_DIR}", file=sys.stderr)
        return 0  # nothing to check; not an error

    leaks: list[tuple[Path, str, str]] = []
    for cassette in CASSETTE_DIR.rglob("*.yaml"):
        text = cassette.read_text(errors="ignore")
        for pattern, label in PATTERNS:
            match = pattern.search(text)
            if match:
                leaks.append((cassette.relative_to(ROOT), label, match.group(0)))

    if leaks:
        print("Secret leak(s) found in committed cassette(s):", file=sys.stderr)
        for path, label, matched in leaks:
            # Show only the first 8 chars of the matched secret in the error,
            # so the leak isn't *also* present in CI logs of this check.
            redacted = matched[:8] + "..."
            print(f"  {path}: {label} match {redacted!r}", file=sys.stderr)
        return 1

    print(f"cassettes clean ({CASSETTE_DIR.relative_to(ROOT)})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
