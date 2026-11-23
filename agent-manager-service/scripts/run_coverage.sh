#!/bin/bash
#
# Run both test tiers (unit + integration), merge their coverage profiles,
# and emit a combined HTML report plus a function-level summary.
#
# Requires the same isolated test database as run_tests_isolated.sh for the
# integration tier. Set SKIP_INTEGRATION=1 to produce a unit-only report
# (useful locally when no database is available).

set -euo pipefail

PROJECT_ROOT=$(pwd)
mkdir -p localdata
UNIT_OUT="$PROJECT_ROOT/localdata/coverage-unit.out"
INTEGRATION_OUT="$PROJECT_ROOT/localdata/coverage-integration.out"
MERGED_OUT="$PROJECT_ROOT/localdata/coverage.out"
HTML_OUT="$PROJECT_ROOT/localdata/coverage.html"

echo "==> Running unit tier"
COVERAGE_OUT="$UNIT_OUT" bash scripts/run_unit_tests.sh

PROFILES=("$UNIT_OUT")

if [ "${SKIP_INTEGRATION:-0}" = "1" ]; then
    echo "==> SKIP_INTEGRATION=1 set; skipping integration tier"
    # A profile left by an earlier full run would count stale hits.
    rm -f "$INTEGRATION_OUT"
else
    echo "==> Running integration tier"
    COVERAGE_OUT="$INTEGRATION_OUT" bash scripts/run_tests_isolated.sh
    PROFILES+=("$INTEGRATION_OUT")
fi

echo "==> Merging coverage profiles"
# Each line after the `mode:` header looks like:
#   <file>:<startLine>.<startCol>,<endLine>.<endCol> <numStmts> <count>
# A block is identified by everything except the trailing <count>. Merging two
# profiles means emitting one line per block with the counts summed (atomic
# mode). Naive concatenation leaves duplicate blocks, which `go tool cover`
# mis-totals — so we de-duplicate and sum here.
awk '
    FNR == 1 { next }                       # skip the per-file "mode:" header
    {
        count = $NF                          # last field is the hit count
        key = $0
        sub(/[ \t][^ \t]+$/, "", key)        # key = line without the count
        if (!(key in seen)) { order[++n] = key; seen[key] = 1 }
        sum[key] += count
    }
    END {
        print "mode: atomic"
        for (i = 1; i <= n; i++) print order[i], sum[order[i]]
    }
' "${PROFILES[@]}" > "$MERGED_OUT"

echo "==> Generating HTML report"
go tool cover -html="$MERGED_OUT" -o "$HTML_OUT"

echo ""
echo "========================================"
echo "Combined coverage"
echo "========================================"
go tool cover -func="$MERGED_OUT" | tail -1
echo ""
echo "Profiles merged: ${PROFILES[*]}"
echo "Merged profile:  $MERGED_OUT"
echo "HTML report:     $HTML_OUT"
