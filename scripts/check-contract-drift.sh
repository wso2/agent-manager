#!/usr/bin/env bash
# Fails (exit 1) if the generated matrix contract differs from what is
# committed. Invoked from CI; can also be run locally.
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

(cd traces-observer-service && GOFLAGS=-mod=mod go run ./cmd/gen-contract "$TMP")

if ! diff -rq "$TMP" test/instrumentation-matrix/contracts/traceloop/v1 >/dev/null; then
    echo "contract drift detected — run 'make gen-instrumentation-contract' and commit the result:"
    diff -ru "$TMP" test/instrumentation-matrix/contracts/traceloop/v1 || true
    exit 1
fi
echo "contract is up-to-date"
