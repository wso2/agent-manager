"""One-shot scrubber for response-side identifiers in committed cassettes.

VCR's `filter_headers` only applies to request headers. Response headers
leak through unless filtered explicitly via a `before_record_response`
callback. This script walks every committed cassette under cassettes/
and strips response headers that identify the org / project / session
the recording was made from. It's idempotent — running it on a clean
cassette is a no-op.

The same set of response headers is filtered by the record-time
`before_record_response` hook in scripts/record_cassette.py and the
vcr_config fixture in harness/test_cell.py, so freshly-recorded
cassettes don't need scrubbing again.
"""
from __future__ import annotations

import sys
from pathlib import Path

import yaml

HERE = Path(__file__).resolve().parent.parent
CASSETTE_DIR = HERE / "cassettes"

# Lowercased; VCR cassettes use mixed case (e.g. "Cookie", "openai-organization")
# so the check normalises before comparing.
RESPONSE_HEADERS_TO_DROP = {
    "openai-organization",
    "openai-project",
    "anthropic-organization-id",
    "set-cookie",
    "cf-ray",
    "cf-cache-status",
    "x-request-id",
    "request-id",
    "x-openai-proxy-wasm",
    "openai-version",
    "openai-processing-ms",
    "x-ratelimit-limit-requests",
    "x-ratelimit-limit-tokens",
    "x-ratelimit-remaining-requests",
    "x-ratelimit-remaining-tokens",
    "x-ratelimit-reset-requests",
    "x-ratelimit-reset-tokens",
}

REQUEST_HEADERS_TO_DROP = {
    "cookie",                # carries __cf_bm back to the server
    "x-stainless-async",     # OpenAI SDK telemetry headers — identifying noise
    "x-stainless-arch",
    "x-stainless-lang",
    "x-stainless-os",
    "x-stainless-package-version",
    "x-stainless-retry-count",
    "x-stainless-runtime",
    "x-stainless-runtime-version",
    "x-stainless-read-timeout",
    "x-stainless-timeout",
}


def _drop_from(headers: dict, drop: set[str]) -> int:
    n = 0
    for k in list(headers.keys()):
        if k.lower() in drop:
            del headers[k]
            n += 1
    return n


def scrub(cassette_path: Path) -> int:
    """Return the number of header occurrences dropped from this cassette."""
    raw = cassette_path.read_text()
    data = yaml.safe_load(raw)
    if data is None:
        return 0
    dropped = 0
    for interaction in data.get("interactions") or []:
        req = interaction.get("request") or {}
        resp = interaction.get("response") or {}
        dropped += _drop_from(req.get("headers") or {}, REQUEST_HEADERS_TO_DROP)
        dropped += _drop_from(resp.get("headers") or {}, RESPONSE_HEADERS_TO_DROP)
    if dropped:
        cassette_path.write_text(yaml.safe_dump(data, sort_keys=False))
    return dropped


def main() -> int:
    total = 0
    files_changed = 0
    for cassette in sorted(CASSETTE_DIR.rglob("*.yaml")):
        n = scrub(cassette)
        if n:
            print(f"  {cassette.relative_to(HERE)}: dropped {n} header(s)")
            files_changed += 1
            total += n
    if total == 0:
        print("cassettes already clean")
    else:
        print(f"scrubbed {total} header occurrence(s) across {files_changed} cassette(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
