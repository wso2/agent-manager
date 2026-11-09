"""Heavy-tier driver.

Picks the heavy-tier subset, deploys each cell against the live AMP stack
via agent-manager-service's REST API, invokes the agent, polls
agent-manager-observer for the resulting spans, validates against the
contract, and writes a per-cell report. Failures fall into the same
taxonomy the emission tier uses (categorize.FailureCategory).

The heavy CI job brings up AMP from the working tree via the dev
`make setup` chain (build-from-source — so the PR's observer +
instrumentation changes are actually exercised, unlike e2e's released
quick-start), then runs this driver. Service URLs + Thunder IDP creds
default to the values that bring-up exposes; only the LLM keys are real
secrets, forwarded into each deployed agent.

The deploy/invoke/poll bodies are implemented against the Go e2e reference
but have not yet run against a live stack. The heavy job stays
`continue-on-error: true` until a real run validates this end to end;
expect timing constants and the observer `/spans` param mapping to need a
tune on first run.
"""
from __future__ import annotations

import os
import sys
import time
from pathlib import Path

import requests

from harness.cell_filter import CellFilter
from harness.deployable_samples import DEPLOYABLE_SAMPLES
from harness.heavy_subset import filter_shard, select_heavy_run
from harness.manifest import Cell, load_manifest
from harness.reports import CellResult, write_cell_report
from harness.validator import ContractValidator
from heavy import k3d
from heavy.amp_client import AmpClient, DeployedAgent, IdpCredentials
from heavy.diagnostics import classify_no_spans, collect_failure_evidence
from heavy.observer import poll_traces
from providers import PROVIDERS

# Categories that indicate a pipeline-wide (not cell-specific) breakage: once
# one cell hits these, the rest will fail identically, so abort and report.
_SYSTEMIC = {"ingest-rejected", "collector-not-received"}


def _is_systemic(category: str | None) -> bool:
    return category in _SYSTEMIC

HERE = Path(__file__).resolve().parent.parent

# AMP service URLs + Thunder IDP credentials default to the values the dev
# `make setup` bring-up exposes (same defaults the e2e suite's config uses).
# They're env-overridable but never required as secrets — the only real
# secrets are the LLM keys, which are forwarded into the deployed agent.
_DEFAULTS = {
    # The dev `make setup` bring-up port-forwards these to localhost:
    # API 9000 (compose 9000:8080), amp-observer 9098, Thunder IDP 8090.
    # (The quick-start/e2e topology routes Thunder via thunder.amp.localhost
    # :8080 with a Host-header trick — that's NOT what the heavy tier uses.)
    "AMP_API_BASE_URL": "http://localhost:9000",
    "AM_OBSERVER_BASE_URL": "http://localhost:9098",
    "IDP_TOKEN_URL": "http://localhost:8090/oauth2/token",
    "IDP_CLIENT_ID": "amp-api-client",
    "IDP_CLIENT_SECRET": "amp-api-client-secret",
    # GitHub source the in-cluster buildpack clones each deployed sample from.
    # Set both sample source fields when testing a samples fork.
    "AMP_AGENT_REPO_URL": "https://github.com/wso2/agent-manager-samples",
    "AMP_AGENT_REPO_BRANCH": "main",
}

# Secrets the deployed sample agent needs to start and make real calls:
# the LLM provider keys plus TAVILY_API_KEY (the customer-support-agent's
# web-search tool fails construction without it). Forwarded as sensitive env
# vars at create time; absent keys are simply not forwarded.
_AGENT_SECRET_ENV_KEYS = ("OPENAI_API_KEY", "ANTHROPIC_API_KEY", "TAVILY_API_KEY")

def _expected_kinds(cell: Cell) -> list[str]:
    """Span kinds to assert for this cell, taken from the *deployed sample*.

    Heavy deploys the sample matching the cell's framework (see
    harness.deployable_samples), so the asserted kinds are that sample's:
    `llm` for the LangGraph customer-support-agent the langchain cell deploys,
    `llm`+`agent`+`crewaitask` for the crewai agent. Any richer kinds a sample
    also emits are still shape-validated via validate_all and surfaced in the
    coverage "actual" list. Per-framework span shape is the emission tier's job.
    """
    return list(DEPLOYABLE_SAMPLES[cell.framework_name].expected_kinds)


def _env(name: str) -> str:
    return os.environ.get(name, _DEFAULTS[name])


def _log(msg: str) -> None:
    # flush=True so progress streams live to the CI log — stdout is block-
    # buffered off a TTY, which would otherwise dump everything at the end.
    print(msg, flush=True)


def _outcome(r: CellResult) -> str:
    if r.result == "pass":
        return f"✅ pass ({','.join(r.coverage.get('actual', []))})"
    if r.result == "skipped":
        return f"⊘ skipped — {r.skip_reason}"
    detail = ""
    if r.violations:
        detail = ": " + (r.violations[0].get("message") or "").strip()
    return f"❌ fail ({r.category}){detail}"


def main() -> int:
    m = load_manifest(HERE / "matrix.yaml")
    # Optional narrowing, all empty/unset by default (= the whole subset):
    #   FILTER_* — the manual workflow's version/framework/python filters;
    #   HEAVY_CELL_ID — one heavy-subset cell, for cheap iteration;
    #   HEAVY_PROVIDER_VERSION — the CI shard (one job per Traceloop version).
    only = os.environ.get("HEAVY_CELL_ID", "").strip()
    cells = select_heavy_run(m, CellFilter.from_env(), only)
    shard = os.environ.get("HEAVY_PROVIDER_VERSION", "").strip()
    if shard:
        cells = filter_shard(cells, shard)
    if not cells:
        print(
            f"no heavy-subset cell matched (HEAVY_CELL_ID={only!r}, "
            f"HEAVY_PROVIDER_VERSION={shard!r}, FILTER_* env)",
            file=sys.stderr,
        )
        return 1

    client = AmpClient(
        base_url=_env("AMP_API_BASE_URL"),
        idp=IdpCredentials(
            token_url=_env("IDP_TOKEN_URL"),
            client_id=_env("IDP_CLIENT_ID"),
            client_secret=_env("IDP_CLIENT_SECRET"),
        ),
    )
    observer_base_url = _env("AM_OBSERVER_BASE_URL")
    agent_env = {k: os.environ[k] for k in _AGENT_SECRET_ENV_KEYS if os.environ.get(k)}

    reports_dir = HERE / "reports" / "heavy"
    total = len(cells)
    _log(f"heavy tier: running {total} cell(s)")
    passed = failed = skipped = 0
    for idx, cell in enumerate(cells, 1):
        _log(f"[{idx}/{total}] {cell.id}")
        try:
            result = _run_cell(cell, client, observer_base_url, agent_env)
        except Exception as e:  # noqa: BLE001 - one cell's failure must not abort the rest
            # deploy/invoke/poll raise (AmpError, TimeoutError, transport) —
            # record the cell as a pipeline failure and keep going so the
            # remaining cells still run and report.
            result = CellResult(
                cell_id=cell.id,
                result="fail",
                category="pipeline-error",
                skip_reason=None,
                durations={},
                coverage={
                    "expected": _expected_kinds(cell),
                    "actual": [],
                    "missing": _expected_kinds(cell),
                },
                violations=[{"spanName": "", "kind": "", "rule": "driver",
                             "path": "", "message": f"{type(e).__name__}: {e}"}],
                captured_spans=[],
            )
        write_cell_report(result, reports_dir=reports_dir)
        _log("      " + _outcome(result))
        if result.result == "fail":
            failed += 1
        elif result.result == "skipped":
            skipped += 1
        else:
            passed += 1
        if result.result == "fail" and _is_systemic(result.category):
            reason = f"aborted: pipeline failure ({result.category}) in {cell.id}"
            _log(f"      ⏭  {reason} — skipping remaining cells")
            for rest in cells[idx:]:
                skipped += 1
                write_cell_report(
                    CellResult(
                        cell_id=rest.id, result="skipped", category=None,
                        skip_reason=reason,
                        coverage={"expected": _expected_kinds(rest), "actual": [], "missing": []},
                    ),
                    reports_dir=reports_dir,
                )
            break
    _log(f"heavy summary: {passed} passed, {failed} failed, {skipped} skipped")
    return 1 if failed else 0


def _run_cell(
    cell: Cell, client: AmpClient, observer_base_url: str, agent_env: dict[str, str]
) -> CellResult:
    expected = _expected_kinds(cell)
    if cell.instrumentation_version is None:
        # Heavy tier only covers init-container-shipping providers today.
        # Manual cells are emission-only.
        return CellResult(
            cell_id=cell.id,
            result="skipped",
            category=None,
            skip_reason="no instrumentation_version (manual provider)",
            durations={},
            coverage={"expected": expected, "actual": [], "missing": []},
            violations=[],
            captured_spans=[],
        )

    _log(f"      deploying (instr {cell.instrumentation_version}, py{cell.python})…")
    k3d.reset_opensearch_indices()
    deployed = client.deploy_agent(
        cell_id=cell.id,
        instrumentation_version=cell.instrumentation_version,
        framework_name=cell.framework_name,
        framework_package=cell.framework_package,
        framework_version=cell.framework_version,
        python_version=cell.python,
        agent_env=agent_env,
        repo_url=_env("AMP_AGENT_REPO_URL"),
        repo_branch=_env("AMP_AGENT_REPO_BRANCH"),
    )
    _log(f"      deployed {deployed.agent_name}; invoking /chat…")
    try:
        _invoke_agent(deployed)
        _log("      invoked; polling observer for spans…")
        spans = poll_traces(client, deployed, observer_base_url)
        # Gather boundary evidence while the agent pod still exists (teardown
        # in the finally below removes it), so a 0-span failure can be localized.
        evidence = collect_failure_evidence(deployed) if not spans else None
    finally:
        client.teardown_agent(deployed)
    _log(f"      captured {len(spans)} span(s)")

    if not spans:
        category = classify_no_spans(evidence).value
        return CellResult(
            cell_id=cell.id,
            result="fail",
            category=category,
            skip_reason=None,
            durations={},
            coverage={
                "expected": expected,
                "actual": [],
                "missing": expected,
            },
            violations=[],
            captured_spans=[],
            evidence={
                "summary": f"agent_init={evidence.agent_init} "
                           f"export_status={evidence.agent_export_status} "
                           f"collector_received={evidence.collector_received}",
                **(evidence.raw or {}),
            },
        )

    provider = PROVIDERS[cell.provider_name]
    validator = ContractValidator.load(provider.contract_schema_id())
    coverage = validator.assert_coverage(spans, expected_kinds=expected)
    # Heavy spans round-trip through the observer/OpenSearch, which returns
    # numeric attributes as strings; coerce the schema-declared numeric
    # attributes back before shape-validating. The emission tier validates
    # native-typed cassette spans and stays strict (coerce_numeric=False).
    shape_results = validator.validate_all(spans, coerce_numeric=True)
    violations = [
        {
            "spanName": r.span_name,
            "kind": r.kind,
            "rule": "schema",
            "path": r.path,
            "message": r.message,
        }
        for r in shape_results
        if not r.ok
    ]

    base_coverage = {
        "expected": expected,
        "actual": sorted(coverage.actual),
        "missing": sorted(coverage.missing),
    }
    if not coverage.ok:
        return CellResult(
            cell_id=cell.id,
            result="fail",
            category="missing-span-kind",
            skip_reason=None,
            durations={},
            coverage=base_coverage,
            violations=violations,
            captured_spans=spans,
        )
    if violations:
        return CellResult(
            cell_id=cell.id,
            result="fail",
            # See FailureCategory.PIPELINE_ERROR docstring for why heavy-tier
            # schema violations map here rather than to SCHEMA_VIOLATION.
            category="pipeline-error",
            skip_reason=None,
            durations={},
            coverage=base_coverage,
            violations=violations,
            captured_spans=spans,
        )
    return CellResult(
        cell_id=cell.id,
        result="pass",
        category=None,
        skip_reason=None,
        durations={},
        coverage=base_coverage,
        violations=[],
        captured_spans=spans,
    )


def _invoke_agent(deployed: DeployedAgent) -> None:
    """POST a fixed prompt to the deployed agent's `/chat` endpoint to drive
    trace emission. Auth is the `X-API-Key` header. Retries through the
    post-deploy warm-up window where the endpoint can briefly 502/503/401
    (API-key propagation to the gateway lags the mint call) or 404 (the
    gateway route lags its deployment ack) — mirrors
    test/e2e/operations/agent/invoke_agent.go.

    Body + path match the deployed sample (customer-support-agent):
    `POST <endpoint>/chat {"session_id", "message"}`. A single-turn prompt is
    enough — every cell sample bottoms out in one LLM call, which is the span
    we need.
    """
    url = deployed.endpoint_url.rstrip("/") + "/chat"
    body = {"session_id": f"matrix-{deployed.agent_name}",
            "message": "Answer in one word: capital of France?"}
    deadline = time.monotonic() + 180
    last = ""
    while time.monotonic() < deadline:
        try:
            resp = requests.post(
                url,
                json=body,
                headers={"X-API-Key": deployed.api_key},
                timeout=60,
            )
        except requests.RequestException as e:  # endpoint not reachable yet
            last = str(e)
            time.sleep(5)
            continue
        if resp.status_code in (401, 404, 502, 503):  # warming up / key or route not propagated
            last = f"{resp.status_code}"
            time.sleep(5)
            continue
        if resp.status_code != 200:
            raise RuntimeError(
                f"agent invocation returned {resp.status_code}: {resp.text[:300]}"
            )
        return
    raise TimeoutError(f"agent endpoint never became ready (last: {last})")


if __name__ == "__main__":
    raise SystemExit(main())
