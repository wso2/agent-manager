# Instrumentation Matrix — operational guide

Day-to-day runbook for the instrumentation-matrix suite: how to extend it,
read its output, and keep it honest. For *why* it's built this way, see the
[design doc](./DESIGN.md). For the running log of
upstream gaps and schema concessions, see [`FINDINGS.md`](./FINDINGS.md).

## 1. What the matrix is (and isn't)

The matrix validates AMP's **instrumentation contract** — "does a given
`(provider × provider-version × framework × framework-version × python)` cell
emit spans the observer can parse" — across the combinations declared in
[`matrix.yaml`](./matrix.yaml). It runs in two tiers:

- **Emission tier** (fast, every relevant PR): runs each cell's sample agent
  in an isolated venv, captures spans via an in-memory exporter with VCR
  cassettes replacing live LLM calls, and validates them against the
  JSON-schema contract.
- **Heavy tier** (nightly / on-demand): deploys a representative subset of
  cells against a real AMP stack on k3d and validates the spans that survive
  the full pipeline. (Heavy-tier deploy/poll is implemented but not yet
  validated against a live stack; see §7.)

It is **not** a correctness test of the agents themselves, a load test, or a
console/UI test — it asserts the *shape* of emitted telemetry, nothing more.

## 2. Add a framework, a framework version, or a Traceloop version

All three are edits to [`matrix.yaml`](./matrix.yaml):

- **New framework version**: add it to that framework's `versions:` list.
  Usually no cassette re-record is needed — versions share the framework's
  cassette when their HTTP is the same. When the HTTP changes (a new prompt
  format, or a switch to native tool calling), the shared cassette can still
  replay but skips the code path the cell is meant to exercise. Record that
  version into `cassettes/<framework>/<version>/`, which the harness prefers
  over the framework-level cassette (CrewAI 1.15 needed its own: it calls
  tools natively where 1.1 used ReAct text).
  The harness only uses that directory once it exists, so create it first,
  then record through the cell itself so the right framework version is
  installed:

  ```bash
  mkdir -p cassettes/crewai/1.15.39
  OPENAI_API_KEY=sk-... VCR_RECORD_MODE=once \
    nox -s emission -- --cell-id=traceloop-0.63.0-crewai-1.15.39-py3.12
  ```
- **New framework**: add a `frameworks:` block (`name`, `package`, `versions`,
  `samplePath`, `spanKinds`, optional `extras`), add a sample under
  `cells/<framework>_sample.py`, and record its cassette (§4 of the design,
  and `scripts/record_cassette.py`).
- **New Traceloop version**: add it to `providers.traceloop.versions` **and**
  add a `providers.traceloop.instrumentationVersions["<ver>"]` entry mapping
  it to the init-container `instrumentation_version` (needed by the heavy
  tier — see §7). `make check-matrix-manifest` enforces that this map covers
  every version `release-config.json` currently ships.

**Example — add a new framework (`haystack`):**

```yaml
# matrix.yaml → frameworks:
  - name: haystack
    provider: traceloop          # restrict to the auto-instrumentation provider
    package: haystack-ai
    versions: ["2.8.0"]
    samplePath: cells/haystack_sample.py
    spanKinds: [llm]             # what the observer should classify out of it
    extras: [haystack-ai, openai]  # extra pip installs the sample needs
```

Then add `cells/haystack_sample.py` (a `run_scenario()` that drives one LLM
call — copy `cells/langchain_sample.py`) and record its cassette:

```bash
OPENAI_API_KEY=sk-... python scripts/record_cassette.py haystack llm
```

**Example — add a Traceloop version (`0.62.0`):**

```yaml
# matrix.yaml → providers.traceloop:
    versions: ["0.61.0", "0.62.0"]          # add the new one
    instrumentationVersions:
      "0.61.0": "0.3.0"
      "0.62.0": "0.4.0"                      # ← the init-container instr version
```

After any edit, sanity-check locally:

```bash
cd test/instrumentation-matrix
nox -s emission -- --cell-id=traceloop-0.62.0-langchain-0.3.27-py3.11   # one cell
nox -s emission -k haystack                                             # by framework
```

## 3. Onboard a new Traceloop release (the canary workflow)

The matrix shadow-tests a new Traceloop release *before* AMP baselines it:

1. `traceloop-release-watch.yaml` opens a `traceloop-release` issue when a new
   `traceloop-sdk` publishes — that's the trigger.
2. Add the new version to `matrix.yaml` (`versions` + `instrumentationVersions`).
   No baseline change yet.
3. Open a PR. The emission tier runs the full cross-product against the new
   version; every new-version cell is **advisory** because it isn't the
   `defaultCell`, so a regression doesn't block the PR.
4. Read the matrix summary (§4) to see exactly which
   `(framework × framework-version × python)` cells regressed.
5. Decide:
   - **All green** → follow-up PR bumps `release-config.json`, regenerates
     `baseline.json`, and (optionally) promotes `defaultCell`.
   - **Acceptable reds** → onboard with `known-broken` entries (§6) tracking
     each regression and its `until:` date.
   - **Too red** → leave it gated or revert the matrix addition; the watcher
     issue stays open.

Promoting `defaultCell` is the explicit moment a version goes from
"shadow-tested" to "shipped and PR-required."

## 4. Triage a red cell

A red emission cell is diagnosed from the run, no local repro needed:

1. **Summary table** — on the `publish-matrix-summary` job's page (GitHub
   step summary, not a PR comment — fork PRs can't comment). One row per
   cell with result + a one-line detail (`category: missing <kinds>` or a
   `path` + message for a schema violation).
2. **Per-cell JSON** — `reports/cells/<id>.json`, with the coverage map,
   violations, and gzipped captured spans. In CI each full-matrix shard
   bundles these into a `matrix-reports-py<version>` artifact (the gating
   default cell also uploads `default-cell-report`); locally they're written
   under `reports/cells/`.
3. **Triage diff** — `reports/diffs/<cell-id>.diff.md` lists the schema's
   required keys vs what the cell actually captured. `nox -s report`
   generates these for failing cells.

The `category` tells you whose problem it likely is (design §12.1):
`install-failure` (manifest), `sample-import-failure` (sample/framework),
`no-spans-captured` / `missing-span-kind` (provider), `schema-violation`
(provider or observer), `cassette-miss` (sample or provider),
`pipeline-error` / `infra-error` (heavy-tier infra/observer).

**Example — a red row + its diff.** The summary shows:

```text
| ❌ traceloop-0.61.0-langgraph-0.2.74-py3.11 | fail | missing-span-kind: missing ['tool'] |
```

and `reports/diffs/traceloop-0.61.0-langgraph-0.2.74-py3.11.diff.md` shows which
required keys were present vs missing:

```
## Required attributes vs captured
| Required key            | Status      |
| ---                     | ---         |
| gen_ai.request.model    | present     |
| traceloop.span.kind     | present     |
| (tool-kind span)        | **MISSING** |
```

→ category `missing-span-kind` points at the provider (partial
instrumentation). Reading the diff is usually enough to reach a verdict
without reproducing locally.

If a regression is upstream and you can't fix it now, record it in
[`FINDINGS.md`](./FINDINGS.md) with an `F-NNN` id and gate the cell with
`known-broken` (§6).

## 5. Add a new instrumentation provider

A *provider* is the swap point that lets the matrix test OpenInference /
OpenLit / vanilla-OTel later without rewriting cells. To add one:

1. Implement the `InstrumentationProvider` Protocol in
   `harness/provider.py` — see `providers/traceloop.py` and
   `providers/manual.py` as worked examples. You need:
   - `name`
   - `package_specs(version)` → pip specs the cell venv installs
   - `bootstrap_module()` → a sitecustomize-style module that initialises the
     SDK against an `InMemorySpanExporter` (mirror
     `providers/bootstrap/traceloop/sitecustomize.py`)
   - `contract_schema_id()` → which schema bundle validates its spans
   - `normalize_span(raw)` → optional namespace folding (default identity)
2. Register it in `providers/__init__.py`'s `PROVIDERS` dict.
3. Add a contract schema bundle under `contracts/<provider>/<version>/` if the
   provider emits a different shape (Traceloop and manual share `traceloop/v1`).
4. Reference it from `matrix.yaml` — either as a provider every framework
   cross-products with, or pinned to specific frameworks via a `provider:`
   restriction (as `manual-rag` does for the `manual` provider).

`ManualProvider` is the simplest worked example: it installs only stdlib
OpenTelemetry (`opentelemetry-sdk`/`-api`), and its sitecustomize wires a
`TracerProvider` + `InMemorySpanExporter` directly — deliberately *not*
calling `amp_instrumentation.init_otel()`, since that ships spans over OTLP
and the in-memory harness replaces that path. It validates against the
*same* `traceloop/v1` schema as the auto path — the observer reads one shape
regardless of source.

## 6. The `known-broken` workflow

When a cell exposes a regression you can't fix immediately:

1. Add an entry under `matrix.yaml.known-broken` with the `cell` match
   pattern (a subset of `provider`/`providerVersion`/`framework`/
   `frameworkVersion`/`python` — missing keys widen the match), a `reason`,
   and an `until:` ISO date.

   ```yaml
   # matrix.yaml → known-broken:
     # gate one specific cell:
     - cell:
         provider: traceloop
         providerVersion: "0.61.0"
         framework: crewai
         frameworkVersion: "1.1.0"
       reason: "F-012: traceloop 0.61 drops crewai tool spans"
       until: "2026-09-01"
     # …or widen by omitting keys — this gates ALL crewai cells on 0.61:
     - cell: { provider: traceloop, providerVersion: "0.61.0", framework: crewai }
       reason: "F-012"
       until: "2026-09-01"
   ```

2. The matrix still expands and reports the cell, but as
   `skipped-known-broken`, so it doesn't block the PR.
3. Nightly, the `revalidate-known-broken` job re-runs every known-broken cell
   unconditionally. If a previously-broken cell now **passes**, it opens an
   issue (titled "known-broken cells now passing — drop exemptions", labeled
   `instrumentation-matrix-revalidate`) suggesting you drop the exemption.
4. Past `until:`, the cell un-skips automatically — forcing a deliberate
   re-extend or fix rather than letting an exemption rot silently.

Pair every `known-broken` entry with an `F-NNN` entry in `FINDINGS.md` so the
"why" survives.

## 7. Heavy tier (deploy contract + ops)

The heavy tier (`nox -s heavy`, `heavy/driver.py`) is a **pipeline test**: it
deploys a real agent against a real AMP stack on k3d, invokes it, polls
`agent-manager-observer`, and checks that the spans survive the full path
(auto-instrumentation → gateway → collector → OpenSearch → observer) and
validate against the `traceloop/v1` contract.

It has a **per-framework axis for frameworks that ship a deployable sample**.
`harness/deployable_samples.py` maps each such framework to its sample app
path, run command, and the span kinds the driver asserts:

- `langchain` → `samples/customer-support-agent` (LangChain/LangGraph), asserts `llm`
- `crewai` → `samples/crewai-agent`, asserts `llm` + `agent` + `crewaitask`

The driver deploys the sample matching each cell's framework and asserts that
framework's kinds (plus shape-validation of every captured span). The other two
axes that change the deployed agent are the **instrumentation/Traceloop
version** (the init-container image) and the **python version** (the buildpack
interpreter). So the subset (`harness/heavy_subset.py`) is one cell per
(Traceloop version × python) for each deployable framework, pinned to that
framework's representative version and restricted to that sample's supported
Python versions from `harness/deployable_samples.py`. CrewAI requires Python
3.11+; the customer-support sample retains Python 3.10 coverage. With one
Traceloop version and Python 3.10–3.13, that's seven cells (four customer-support
and three CrewAI). The dependency compatibility check uses the same runtime
constraints.

The nightly and manual workflows run one heavy job per Traceloop version, each on its own stack
(seven cells). A new Traceloop release in `matrix.yaml` adds a
parallel shard automatically; it does not lengthen the existing ones.

Frameworks without a deployable sample (`langgraph`, `llama-index`,
`openai-direct`, `anthropic-direct`) stay emission-only — per-framework span
*shape* for them is the emission tier's job. Adding a deployable sample under
`samples/` plus a `DEPLOYABLE_SAMPLES` entry is what brings a framework into the
heavy axis.

### Bring-up: build-from-source

The CI heavy job stands up AMP from the **working tree** via the dev
`make setup` chain, wrapped in the `.github/actions/amp-dev-stack` composite:
`setup-k3d` → `setup-openchoreo` (builds + `k3d image import`s the
amp-observer + python-instrumentation-provider images from source) →
`setup-platform` (agent-manager-service via docker-compose) → migrate →
port-forward → gateway.

This is deliberately **not** `deployments/quick-start/install.sh`, which
deploys *released* images at a pinned `VERSION` (that's what e2e uses). The
matrix's job is to catch regressions in the PR's observer + instrumentation
code, so it must run the PR's images.

### Environment

The only real **secrets** are the LLM keys; everything else defaults to the
values the dev bring-up exposes (overridable, never required):

| Variable | Kind | Default / source |
|---|---|---|
| `OPENAI_API_KEY` | **secret** | forwarded into each deployed agent so it can make real calls |
| `ANTHROPIC_API_KEY` | **secret** | for the anthropic-direct cell |
| `AMP_API_BASE_URL` | default | `http://localhost:9000` |
| `AM_OBSERVER_BASE_URL` | default | `http://localhost:9098` |
| `IDP_TOKEN_URL` | default | `http://thunder.amp.localhost:8080/oauth2/token` |
| `IDP_CLIENT_ID` | default | `amp-api-client` |
| `IDP_CLIENT_SECRET` | default | `amp-api-client-secret` |
| `HEAVY_PROVIDER_VERSION` | filter | run only this Traceloop version's cells; CI sets it per shard. Empty = every version |
| `FILTER_TRACELOOP_VERSIONS`, `FILTER_FRAMEWORKS`, `FILTER_FRAMEWORK_VERSIONS`, `FILTER_PYTHON_VERSIONS` | filter | comma-separated or `all`; the manual workflow passes its filter inputs here so they narrow the heavy subset as well as the emission matrix. Empty = all |
| `HEAVY_CELL_ID` | filter | run only this one heavy-subset cell, ignoring the `FILTER_*` vars. Empty = the whole (filtered) subset |

Auth is OAuth2 `client_credentials` against Thunder IDP (no static admin
token); the client fetches + refreshes a bearer token, mirroring
`test/e2e/framework/auth.go`. Agents are created through
`agent-manager-service`'s REST API — `test/e2e/framework/shared_agent.go` is
the canonical reference — not raw Workload manifests.

### Per-cell flow

```
reset OpenSearch indices
deploy_agent(cell)          # create project → agent (pins instrumentationVersion +
                            #   forwards LLM keys) → poll build → deploy → mint API key
invoke_agent(deployed)      # POST /chat (X-API-Key), retry through warm-up
spans = poll_traces(...)    # list traces → span summaries → per-span detail
teardown_agent(deployed)    # always (finally)
validate(spans) + write report
```

Span fetch reuses the emission validator: the observer's span-detail endpoint
(`GET /api/v1/traces/{traceId}/spans/{spanId}`) returns a raw `attributes`
map identical in shape to what the emission tier validates, so there's no
separate heavy contract.

### Status (not yet run against a live stack)

The deploy / invoke / poll bodies are **implemented** against the Go e2e
reference with mocked-HTTP unit tests (`tests/test_heavy_client.py`), but
haven't run against a live AMP stack. The heavy jobs stay
`continue-on-error: true` until a real run is green. Three things are
first-run-tunable:

1. **The `amp-dev-stack` bring-up** — the `make setup` chain hasn't run on a
   CI runner; watch `setup-platform.sh`'s Node check, the (unneeded) console
   in docker-compose, and the port-forwards exposing the API (`:9000`),
   amp-observer (`:9098`), and Thunder IDP (`:8090`).
2. **Endpoints the driver assumes** — IDP token URL (`localhost:8090`), the
   agent's `/chat` route + `{session_id, message}` body, and reading the
   endpoint URL from the deployments response (the e2e suite reads it from
   `/endpoints`). Timing constants (build 600s, deploy 300s) too.
3. **The observer `/spans` param names** — `poll_traces` sends both
   `organization`/`project`/`agent` and best-effort `namespace`/`component`;
   confirm the right mapping on a live observer.

## 8. Where the schemas come from

The JSON-schema contract under `contracts/traceloop/v1/` is **generated from
`agent-manager-observer`'s parsers**, not hand-written:

```bash
make gen-instrumentation-contract     # regenerate schemas from the observer
make check-contract-drift             # fail if generated output != committed
```

`check-contract-drift` (run by the `verify-contract-and-manifest` CI job)
enforces both directions: if the observer adds a required attribute without
regenerating, or a schema is hand-edited without a matching observer change,
the build fails. Every deliberate concession (a relaxed `required`, a
stringified type) is justified by an `F-NNN` entry in `FINDINGS.md`.

`check-matrix-manifest` (same CI job) separately enforces that `matrix.yaml`
covers every `(traceloop_version, instrumentation_version, python_version)`
that `release-config.json` currently ships — the matrix may be a superset,
but never a subset, of what AMP baselines.
