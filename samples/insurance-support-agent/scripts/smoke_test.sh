#!/usr/bin/env bash
set -euo pipefail
sample="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
export SMOKE_REPORTS="${SMOKE_REPORTS:-$(mktemp -d -t insurance-smoke-reports)}"
mkdir -p "$SMOKE_REPORTS"
trap 'rm -rf "$work"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
exec > >(tee "$SMOKE_REPORTS/smoke.log") 2>&1
echo "Reports: $SMOKE_REPORTS"
"${PYTHON:-python3.11}" -m venv "$work/venv"
python="$work/venv/bin/python"
"$python" -m pip install -r "$sample/agent/requirements.txt" -r "$sample/mcp-server/requirements.txt"
"$python" -m pip check
export PYTHONPATH="$sample/agent"
export PYTHON_DOTENV_DISABLED=1
"$python" "$sample/tests/test_bedrock_gateway.py"
"$python" "$sample/tests/test_mcp.py" -v
"$python" "$sample/tests/smoke_http.py"
"$python" "$sample/../../test/sample-smoke/insurance_support/local_smoke.py"
