# Instrumentation Matrix Test Suite — Design

This document is the design for the test suite at this directory that
validates AMP's instrumentation contract across a *matrix* of:

- instrumentation providers (Traceloop / OpenLLMetry today; OpenInference, OpenLit,
  vanilla OTel GenAI later),
- provider versions,
- agent frameworks (LangChain, LangGraph, LlamaIndex, CrewAI, …) and their versions,
- Python runtime versions,
- both auto-instrumentation and manual-instrumentation paths,
- both platform-hosted and externally-hosted delivery modes.

It is an upfront design; the implementation plan is a separate artifact.

---

## 1. Why this exists

The thing that motivates the suite is a small set of recurring questions that
AMP currently has no automated way to answer:

1. **Traceloop just released a new version. Will it break anything we ship?**
   Today: we eyeball release notes and pray. Want: a one-click matrix run
   against the new pin that tells us exactly which `(framework × version × python)`
   combos regressed.
2. **We baselined Traceloop *N* months ago. Has anything we depend on drifted
   since?** Today: nothing tells us. Want: a nightly run that catches drift
   in framework-version × Traceloop-version combinations.
3. **Our manual-instrumentation contract — does our published sample still
   match what the observer reads?** Today: docs and code are kept in sync by
   convention. Want: the sample is exercised against the same schema the
   observer uses, so doc drift is a CI failure.
4. **Traceloop claims to support CrewAI, but sometimes it doesn't actually
   work.** Today: discovered in customer reports. Want: matrix cells per
   `(traceloop × crewai-version)` that catch it before customers do.
5. **We want to evaluate OpenInference / OpenLit as alternatives.** Today: no
   way to run a fair comparison. Want: a provider-abstracted suite where
   adding a second provider is "implement one class, drop in one schema."

The suite's success metric is operational: when Traceloop publishes a new
version, the time between "watcher opens the issue" and "we know if it's safe
to onboard" should be **a single CI run**, not a triage epic.

## 2. Goals and non-goals

**Goals**

- Detect regressions in `(provider × provider-version × framework × framework-version × python-version)` combinations.
- Validate emitted spans against a versioned JSON-schema contract that is itself
  generated from `agent-manager-observer`'s parsers (no hand-maintained schema docs).
- Cover both auto- and manual-instrumentation paths using the *same* contract.
- Make adding a new framework, a new framework-version, or a new Traceloop
  version a one-line manifest change.
- Make the heavy tier (full AMP pipeline) cheap enough to run nightly on a
  representative subset of cells.
- Be a pre-onboarding gate for new Traceloop releases — testable before AMP
  commits to baselining a version.
- Alert via the existing Google Chat webhook used by `traceloop-release-watch.yaml`.

**Non-goals**

- Console UI / visual rendering validation. Out of scope.
- Evaluator end-to-end validation. The suite stops at validating
  `AmpAttributes` shape; evaluators consuming that shape are their own concern.
- "Every library Traceloop claims to support." The suite covers a deliberate
  curated set; expansion is a manifest change once the harness is proven.
- Replacing the existing `test/e2e/` Go/Ginkgo suite. That suite tests the
  platform happy path; this one tests the SDK → observer contract. They
  coexist.

## 3. Architecture overview

A Python-based, manifest-driven, two-tier compatibility harness at
`test/instrumentation-matrix/`.

```text
                          matrix.yaml
                  (providers × versions × frameworks
                       × versions × python)
                              │
                              ▼
                   harness.expand_matrix()  ──►  list of Cells
                              │
              ┌───────────────┴───────────────────────┐
              ▼                                        ▼
   ┌──────────────────────┐               ┌────────────────────────────┐
   │  EMISSION TIER        │               │  HEAVY TIER (subset)        │
   │  every relevant PR    │               │  nightly / on-demand        │
   │                       │               │                            │
   │  per-cell venv        │               │  build AMP from source     │
   │  + provider bootstrap │               │  on k3d (make setup)       │
   │  + VCR cassette       │               │  deploy agent → /chat      │
   │  → InMemoryExporter   │               │  → amp-observer    poll    │
   └──────────┬───────────┘               └──────────────┬─────────────┘
              │            captured spans                 │
              └───────────────┬────────────────────────────┘
                              ▼
                  ContractValidator  ◄────  contracts/traceloop/v1/
                  (coverage + shape)        (generated from the observer's
                              │              parsers — see §6)
                              ▼
                  per-cell JSON reports  ──►  summary + triage diffs
                                              (PR step-summary / nightly
                                               issue + Chat alert)
```

Both tiers feed the **same** `ContractValidator` against the **same**
generated schema — emission asks "does the SDK still emit conforming spans?",
heavy asks "do they survive the real pipeline and enrich correctly?"

### 3.1 Two tiers, two questions

| Tier        | Question it answers                                                              | Per-cell cost | Where it runs                                                                            |
| ---         | ---                                                                              | ---           | ---                                                                                      |
| Emission    | Does the instrumentation provider still emit conforming spans for this combo?    | 5–15 s        | In-process, per-cell `venv`, `InMemorySpanExporter`, VCR cassette for LLM HTTP.          |
| Heavy       | Do conforming spans flow through Obs Gateway → Collector → OpenSearch → agent-manager-observer and arrive as well-formed `AmpAttributes`? | 15–20 min     | k3d in CI (AMP built from source via `make setup`), restricted to a representative cell subset. |

### 3.2 What each tier covers and misses

- The **emission tier** exercises the SDK and the contract. It does not
  exercise the prod `sitecustomize.py`, the OTLP exporter, Obs Gateway JWT
  validation, OTel Collector batching, OpenSearch index typing, or
  `agent-manager-observer` enrichment. Emission failures point at the
  provider or the framework.
- The **heavy tier** runs the unmodified prod `sitecustomize.py` against a real
  AMP stack and asserts on the `AmpAttributes` returned by
  `agent-manager-observer`. It does cover everything the emission tier
  misses, at the cost of full-cluster spin-up. Heavy failures distinguish
  between "spans never arrived" (pipeline issue) and "spans arrived but
  enrichment was wrong" (observer regression).

### 3.3 Four moving parts of the suite

1. **Matrix manifest** (`matrix.yaml`) — source of truth for which cells exist.
2. **Provider interface** — pluggable abstraction (`TraceloopProvider` +
   `ManualProvider` today; OpenInference / OpenLit / vanilla-OTel later).
3. **Span contract validator** — versioned JSON-schema bundles per provider.
   Used by both auto and manual cells.
4. **Cell harness** — given a manifest row, builds a venv, runs the agent
   sample, captures spans, validates them against the contract.

### 3.4 Relationship to existing AMP artifacts

| Artifact                                          | Role today                                           | Role in this design                                                                              |
| ---                                               | ---                                                  | ---                                                                                              |
| `.github/release-config.json`                     | Drives image build matrix (which combos ship).       | Unchanged. A CI drift check asserts every baselined version appears in `matrix.yaml`.            |
| `agent-manager-service/instrumentation/baseline.json` | Embedded server-side catalog of known versions.  | Unchanged. Generated from `release-config.json` as today.                                        |
| `python-instrumentation-provider/sitecustomize.py`| Production auto-instrumentation entrypoint.          | Unchanged. The test harness uses a small forked variant; a contract test prevents config drift.  |
| `samples/manual-instrumentation-agent/`           | Published reference for manual contract.             | Becomes the source for the manual cell sample. Drift surfaces as a matrix failure.               |
| `agent-manager-observer/opensearch/process.go`    | Parses raw OTel spans into `AmpAttributes`.          | Schema is *generated from this code* (see §6). The observer and the contract become one unit.    |
| `.github/workflows/traceloop-release-watch.yaml`  | Opens an issue + Chat message on new Traceloop releases. | Unchanged. Becomes the natural trigger for the manual matrix workflow (§10).                 |

## 4. Matrix manifest

**File**: `test/instrumentation-matrix/matrix.yaml`. Hand-curated, reviewed in
PRs, source of truth for what gets tested.

### 4.1 Shape (illustrative)

```yaml
schemaVersion: 1

providers:
  traceloop:
    versions: ["0.60.0", "0.59.0", "0.58.0"]
    # Maps each traceloop version to the init-container image's
    # instrumentation_version (the heavy tier resolves
    # amp-python-instrumentation-provider:<instr>-python<X.Y> from here).
    # Carried separately because release-config.json doesn't archive
    # superseded entries — when 0.2.1 replaced 0.2.0, the catalog dropped
    # 0.2.0 even though the matrix still wants to test it.
    instrumentationVersions:
      "0.60.0": "0.2.1"
      "0.59.0": "0.2.0"
      "0.58.0": "0.1.0"
    contractSchema: "v1"

  manual:
    versions: ["0.2.1", "0.2.0"]          # amp-instrumentation package versions
    # No instrumentationVersions — the manual path doesn't ship an init
    # container, so heavy-tier cells against this provider are skipped.
    contractSchema: "v1"

frameworks:
  - name: langchain
    package: langchain
    versions: ["0.3.27", "0.3.18", "0.2.16"]
    samplePath: cells/langchain_sample.py
    spanKinds: [llm, tool, chain]

  - name: langgraph
    package: langgraph
    versions: ["0.2.74", "0.2.50"]
    samplePath: cells/langgraph_sample.py
    spanKinds: [llm, tool, agent, chain]

  - name: llama-index
    package: llama-index
    versions: ["0.12.0", "0.11.20"]
    samplePath: cells/llamaindex_sample.py
    spanKinds: [llm, embedding, retriever, chain]

  - name: crewai
    package: crewai
    versions: ["0.86.0", "0.80.0"]
    samplePath: cells/crewai_sample.py
    spanKinds: [llm, tool, agent, crewaitask]

  - name: openai-direct
    package: openai
    versions: ["1.55.0", "1.40.0"]
    samplePath: cells/openai_sample.py
    spanKinds: [llm]

  - name: anthropic-direct
    package: anthropic
    versions: ["0.40.0", "0.30.0"]
    samplePath: cells/anthropic_sample.py
    spanKinds: [llm]

  - name: manual-rag
    provider: manual                       # restricts to the manual provider only
    package: amp-instrumentation
    versions: ["n/a"]
    samplePath: cells/manual_rag_sample.py
    spanKinds: [agent, chain, embedding, retriever, rerank, llm, tool]

python:
  versions: ["3.10", "3.11", "3.12", "3.13"]

defaultCell:
  provider: traceloop
  providerVersion: "0.61.0"                # mirrors OTEL_DEFAULT_INSTRUMENTATION_VERSION's traceloop pin
  framework: langchain
  frameworkVersion: "0.3.27"
  python: "3.11"

heavyTier:
  perTraceloopVersion: 1                   # one cell per declared traceloop version
  perFramework: 1                          # one cell per framework family

known-broken:
  - cell: { provider: traceloop, providerVersion: "0.59.0", framework: crewai, frameworkVersion: "0.86.0" }
    reason: "Traceloop 0.59 emits empty gen_ai.system for CrewAI tool spans (issue traceloop/openllmetry#4123, fixed in 0.60.0)."
    until: "2026-07-01"
```

### 4.2 Cell-expansion rules

- Cross-product of `providers × frameworks × python.versions`.
- A `framework` entry with `provider:` restricts that framework to the named
  provider only (used for the manual path). If the named provider isn't
  declared under `providers:`, expansion raises — silently producing zero
  cells from a typo'd restriction is worse than a loud failure.
- A `known-broken` entry skips the cell but still records it in the report as
  `skipped-known-broken`. `until:` makes every skip expire — past that date,
  the cell un-skips and starts running again.
- A cell ID is
  `<provider>-<providerVersion>-<framework>-<frameworkVersion>-py<pythonVersion>`,
  used in CI job names, cassette filenames, the matrix report, and manual
  `workflow_dispatch` inputs.

### 4.3 Why not extend `release-config.json`

`release-config.json` is "what images we build and ship."
`matrix.yaml` is "what combos we run tests against." The latter is a strict
superset — newly-released Traceloop versions we want to validate *before*
committing to a baseline, framework versions that AMP doesn't even know about.
Conflating them blurs both files' responsibility.

A CI check (`make check-matrix-manifest`) verifies that every
`release-config.json` `(traceloop_version, instrumentation_version,
python_version)` triple appears in `matrix.yaml` — the
`providers.traceloop.versions` list, the
`providers.traceloop.instrumentationVersions` map, and `python.versions`
collectively must cover the active baseline. Drift between the two is a build
failure. The reverse direction is *not* checked — the matrix is allowed to
test combos AMP hasn't baselined, and the matrix's
`instrumentationVersions` map is allowed to keep entries
`release-config.json` has dropped (that's the whole point of the separate
mapping).

### 4.4 Onboarding a new Traceloop version (the canary workflow)

The matrix is a shadow / canary for new Traceloop releases, *before* AMP
commits to baselining them.

| Step | What happens                                                                                       | Where the change lands                                          |
| ---  | ---                                                                                                | ---                                                             |
| 1    | `traceloop-release-watch.yaml` opens an issue + Google Chat ping when Traceloop publishes.         | —                                                               |
| 2    | Engineer adds the new version to `matrix.yaml` → `providers.traceloop.versions`. No baseline change.| `matrix.yaml` only.                                             |
| 3    | The emission tier runs the full cross-product against the new version on that PR. All cells against the new version are advisory because it is not the `defaultCell`. | PR CI.                              |
| 4    | The PR-comment matrix report lists exactly which `(framework × framework-version × python)` cells regressed under the new version, with span diffs. | PR comment.                                                   |
| 5a   | All green → follow-up PR bumps `release-config.json`, regenerates `baseline.json`, optionally promotes `defaultCell`. | `release-config.json`, `baseline.json`, `matrix.yaml.defaultCell`. |
| 5b   | Some red, acceptable → onboard with `known-broken` entries documenting each regression, `until:` tracking the upstream fix. | `release-config.json`, `baseline.json`, `matrix.yaml.known-broken`. |
| 5c   | Too red → revert the matrix addition or leave it gated with `known-broken`. Watcher issue stays open. | —                                                             |

`defaultCell` is a pointer, not a derived value. Promoting a new version to
`defaultCell` is an explicit, reviewed change in the same PR that updates the
baseline. That is the moment the new version goes from "shadow-tested" to
"shipped and PR-required."

## 5. Provider interface

A *provider* is the abstraction that lets the suite swap Traceloop for
OpenInference / OpenLit / vanilla-OTel later without rewriting cells. Today it
ships only `TraceloopProvider` and `ManualProvider`.

### 5.1 The contract

```python
# test/instrumentation-matrix/harness/provider.py
class InstrumentationProvider(Protocol):
    name: str                              # "traceloop" | "manual" | "openinference" | "openlit" | "otel-genai"

    def package_specs(self, version: str) -> list[str]:
        """Pip specs the cell venv needs to install for this provider+version.
        e.g. Traceloop -> ['traceloop-sdk==0.60.0', 'wrapt<2.0.0']
        """

    def bootstrap_module(self) -> Path:
        """Path to the sitecustomize-style module the cell process imports
        to initialize the SDK. For Traceloop this mirrors the production
        sitecustomize.py, pointed at the in-process exporter.
        """

    def contract_schema_id(self) -> str:
        """Which JSON-schema bundle validates spans emitted by this provider.
        e.g. 'traceloop/v1', 'openinference/v0.5', 'otel-genai/v1.30'.
        """

    def normalize_span(self, raw_span: ReadableSpan) -> dict:
        """Optional namespace normalisation hook. Default = identity.
        Exists for providers that need to fold quirks before validation.
        """
```

### 5.2 Registration

```python
# test/instrumentation-matrix/providers/__init__.py
PROVIDERS: dict[str, InstrumentationProvider] = {
    "traceloop": TraceloopProvider(),
    "manual":    ManualProvider(),
    # planned follow-up providers:
    # "openinference": OpenInferenceProvider(),
    # "openlit":       OpenLitProvider(),
    # "otel-genai":    VanillaGenAIProvider(),
}
```

Adding a manifest entry whose `provider:` is not registered fails fast at
suite startup.

### 5.3 What `TraceloopProvider` does

- `package_specs("0.60.0")` → `["traceloop-sdk==0.60.0", "wrapt<2.0.0", "opentelemetry-sdk", ...]`.
  The `wrapt<2` pin is the constraint already baked into
  `python-instrumentation-provider/requirements.txt`; re-exporting it from the
  provider keeps it travelling with the version.
- `bootstrap_module()` → path to `providers/bootstrap/traceloop/sitecustomize.py`
  mirroring `python-instrumentation-provider/sitecustomize.py` but calling
  `Traceloop.init(exporter=InMemorySpanExporter())` instead of OTLP HTTP.
  This is a deliberate fork — a contract test asserts the two files configure
  Traceloop with the same content/metrics flags so they cannot drift apart.
- `contract_schema_id()` → `"traceloop/v1"`.

### 5.4 What `ManualProvider` does

- `package_specs(version)` → `[f"amp-instrumentation=={version}", "opentelemetry-api", "opentelemetry-sdk"]`.
- `bootstrap_module()` → a tiny test bootstrap that calls
  `init_otel(exporter=InMemorySpanExporter())` instead of the prod OTLP exporter.
- `contract_schema_id()` → **`"traceloop/v1"`** — the *same* schema as the
  auto path. The observer reads one shape regardless of source; if the manual
  path validated against a different schema, you could ship a "valid" manual
  span that the observer doesn't parse.

### 5.5 The prod-vs-test bootstrap gap

The test bootstrap intentionally diverges from prod (`InMemorySpanExporter`
instead of the OTLP HTTP exporter). The risk: emission-tier passes but a real
OTLP exporter would fail differently. The heavy tier (§8) closes this gap by
running the unmodified prod `sitecustomize.py` against a real AMP stack on a
representative cell subset.

## 6. Span contract schema and validator

### 6.1 Layout

```
test/instrumentation-matrix/
└── contracts/
    └── traceloop/
        └── v1/
            ├── span.schema.json          # base envelope
            ├── kinds/
            │   ├── llm.schema.json
            │   ├── embedding.schema.json
            │   ├── tool.schema.json
            │   ├── retriever.schema.json
            │   ├── rerank.schema.json
            │   ├── agent.schema.json
            │   ├── chain.schema.json
            │   └── crewaitask.schema.json
            └── resource.schema.json      # service.name + AMP resource attrs
```

### 6.2 Example — `kinds/llm.schema.json`

```jsonc
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "traceloop/v1/kinds/llm.schema.json",
  "title": "Traceloop v1 — LLM span",
  "type": "object",
  "properties": {
    "name":       { "type": "string" },
    "kind":       { "const": "CLIENT" },
    "attributes": {
      "type": "object",
      "required": [
        "gen_ai.system",
        "gen_ai.request.model",
        "gen_ai.usage.input_tokens",
        "gen_ai.usage.output_tokens",
        "traceloop.span.kind"
      ],
      "properties": {
        "gen_ai.system":              { "type": "string", "minLength": 1 },
        "gen_ai.request.model":       { "type": "string", "minLength": 1 },
        "gen_ai.response.model":      { "type": "string" },
        "gen_ai.usage.input_tokens":  { "type": "integer", "minimum": 0 },
        "gen_ai.usage.output_tokens": { "type": "integer", "minimum": 0 },
        "traceloop.span.kind":        { "const": "llm" },
        "gen_ai.prompt.0.role":       { "enum": ["system","user","assistant","tool"] },
        "gen_ai.prompt.0.content":    { "type": "string" }
      },
      "additionalProperties": true
    }
  },
  "required": ["name", "kind", "attributes"]
}
```

### 6.3 Two rules baked into every schema

1. **`additionalProperties: true` everywhere.** Upstream SDKs add attributes
   over time; that must never break us. The schema asserts only the keys AMP's
   observer reads.
2. **`required` is conservative.** A key is `required` only when the observer
   would produce a malformed `AmpAttributes` payload without it.

### 6.4 Validation pipeline

```python
def validate_cell_spans(cell, spans):
    schema_id = cell.provider.contract_schema_id()           # "traceloop/v1"
    validator = ContractValidator.load(schema_id)

    # Pass 1 — coverage: every declared span kind must appear at least once.
    actual_kinds = {classify(s) for s in spans}
    missing = set(cell.spanKinds) - actual_kinds
    assert not missing, f"missing span kinds: {missing}"

    # Pass 2 — shape: every span validates against its kind's schema.
    for span in spans:
        kind = classify(span)
        validator.validate(span, kind=kind)
```

`classify()` reuses the same logic the observer uses, factored into a shared
helper, to keep "what kind is this span" consistent between matrix and
production.

### 6.5 Schema source of truth

Schemas are **generated from `agent-manager-observer`'s parsing code**, not
hand-written.

```
observer parsers  ──gen──>  JSON schemas  ──gen──>  docs MDX tables
                              │
                              └──validates──>  manual-rag sample's spans
                              │
                              └──validates──>  auto-path samples' spans
```

A new `make gen-instrumentation-contract` target walks the observer's per-kind
extractors (e.g., the LLM parser reads `gen_ai.system`, `gen_ai.request.model`, …)
and emits the schemas under `contracts/traceloop/v1/`. If the observer adds a
new required field, regenerating produces a schema diff in the same PR. The
reverse direction is also enforced: a schema-only change is rejected unless the
observer parser is updated in the same PR. Drift in any direction is a build
failure.

### 6.6 Schema versioning

- `traceloop/v1` is the schema-of-record today.
- When AMP's observer adds a new required attribute in a non-backwards-compatible
  way, we cut `traceloop/v2`. Old Traceloop versions can stay pinned to `v1` via
  `providers.traceloop.contractSchema` so historical cells keep validating against
  the schema in force at the time.
- `manifest.providers.<name>.contractSchema` selects which schema version
  validates each provider entry.

## 7. Cassette management (VCR)

The matrix can't make real LLM API calls every run — too slow, too flaky, too
expensive. VCR cassettes record live HTTP interactions once, then replay them
deterministically.

### 7.1 Tooling

`pytest-recording` (wraps `vcrpy`). Supports `httpx` and `requests`, which
covers OpenAI, Anthropic, Cohere, Bedrock under the hood.

### 7.2 Cassette layout

```
test/instrumentation-matrix/
└── cassettes/
    ├── langchain/
    │   ├── llm_chat_completion.yaml
    │   ├── tool_call.yaml
    │   └── chain_with_memory.yaml
    ├── langgraph/...
    ├── llama-index/...
    ├── crewai/...
    ├── openai-direct/...
    ├── anthropic-direct/...
    └── manual-rag/
        ├── embedding.yaml
        └── chat.yaml
```

Cassettes are keyed by `framework + scenario`, **not** by
`(framework × framework-version × python)`. Reason: a cassette captures what
the agent sent to OpenAI, not what the SDK monkey-patched around it. Different
framework versions produce different *spans* but identical *HTTP requests* (the
prompt and model are fixed by the test sample). One cassette per framework ×
scenario, reused across every cell using that framework.

### 7.3 Cassette modes

| Mode            | When                                       | Behavior                                                |
| ---             | ---                                        | ---                                                     |
| `none`          | Default in CI                              | Replay only. Unrecorded request fails the cell loudly.  |
| `once`          | Local first-time record                    | Record if no cassette exists; replay otherwise.         |
| `new_episodes`  | Local re-record after sample changes       | Replay existing; record new interactions.               |
| `rewrite`       | Quarterly cassette refresh                 | Always re-record. Requires `RECORD_LLM_API_KEYS=1`.     |

### 7.4 Recording workflow

```bash
export OPENAI_API_KEY=sk-...
export ANTHROPIC_API_KEY=sk-ant-...
nox -s record -- --framework=langchain --scenario=tool_call
# Produces cassettes/langchain/tool_call.yaml — review the diff, commit it.

nox -s record-all     # quarterly full refresh
```

Cassettes are committed plaintext YAML so LLM responses are reviewable in PRs.

### 7.5 Secret filtering

```python
# conftest.py
@pytest.fixture(scope="session")
def vcr_config():
    return {
        "filter_headers": [
            ("authorization", "REDACTED"),
            ("x-api-key", "REDACTED"),
            ("openai-organization", "REDACTED"),
        ],
        "filter_post_data_parameters": [
            ("api_key", "REDACTED"),
        ],
        "before_record_response": _strip_response_headers,
        "decode_compressed_response": True,
        "record_mode": os.getenv("VCR_RECORD_MODE", "none"),
    }
```

A pre-commit hook (`scripts/check-cassettes.py`) greps committed cassettes for
known key prefixes (`sk-`, `sk-ant-`, `xai-`, …) as belt-and-braces.

### 7.6 What cassettes catch and don't

- **Catch**: SDK-side regressions — Traceloop monkey-patches a method that no
  longer exists; a framework version changes its internal method signature and
  Traceloop stops capturing; a new attribute AMP requires stops being set. The
  SDK still receives the same HTTP response; only the spans differ.
- **Don't catch**: real model behavior shifts, or a Traceloop change in *what
  gets sent* to the LLM. The latter surfaces as a loud `cassette-miss` failure,
  which is the intended signal.

### 7.7 Refresh policy

- Bumping `framework.versions` does **not** require re-recording.
- Bumping `traceloop.versions` does **not** require re-recording.
- Changing a `cells/<sample>.py` agent's prompt, model, or tool definition
  **does** require re-recording.
- Quarterly refresh exists to keep cassettes current with provider-side schema
  evolution.

## 8. Cell harness

How a single emission-tier cell executes. This is the inner loop the matrix
runs ~100 times per CI run.

### 8.1 Driver

`nox` is the entry point. One `nox` session per cell, parameterised from the
manifest. `nox` is preferred over `tox` because the cell list is *computed*
from `matrix.yaml`; `nox`'s Python config supports that natively.

### 8.2 `noxfile.py` (sketch)

```python
# test/instrumentation-matrix/noxfile.py
import nox
from harness.manifest import expand_matrix, Cell

CELLS: list[Cell] = expand_matrix("matrix.yaml")

@nox.session(python=False)
@nox.parametrize("cell", CELLS, ids=[c.id for c in CELLS])
def emission(session, cell):
    """One emission-tier cell."""
    session.run("python", f"{cell.python}", "-m", "venv", session.cache_dir / cell.id, external=True)
    venv = session.cache_dir / cell.id
    pip  = venv / "bin" / "pip"

    session.run(str(pip), "install", "--quiet",
                *cell.provider.package_specs(cell.providerVersion),
                f"{cell.framework.package}=={cell.frameworkVersion}",
                "pytest", "pytest-recording", "jsonschema",
                external=True)

    session.run(str(venv / "bin" / "pytest"),
                "harness/test_cell.py",
                f"--cell-id={cell.id}",
                "-q", "--no-header",
                external=True,
                env={"PYTHONPATH": str(cell.provider.bootstrap_module().parent),
                     "VCR_RECORD_MODE": "none",
                     "CELL_MANIFEST": cell.serialize()})
```

### 8.3 Per-cell test body

```python
def test_emission_cell(cell):
    # (a) The cell's bootstrap was already on PYTHONPATH; sitecustomize
    #     initialized the provider with an InMemorySpanExporter accessible
    #     via a process-global registry.
    exporter: InMemorySpanExporter = harness.exporter_handle()

    # (b) Load + run the sample. The sample imports the framework, builds a
    #     small agent, calls .invoke(). VCR replays the LLM HTTP.
    sample = importlib.import_module(cell.framework.samplePath_to_module())
    sample.run_scenario()

    # (c) Flush + grab spans.
    trace.get_tracer_provider().force_flush(timeout_millis=5000)
    spans = exporter.get_finished_spans()
    exporter.clear()

    # (d) Coverage + shape validation against the contract.
    validator = ContractValidator.load(cell.provider.contract_schema_id())
    coverage_result = validator.assert_coverage(spans, expected_kinds=cell.spanKinds)
    shape_results   = validator.validate_all(spans)

    # (e) Emit a per-cell JSON report (consumed by the aggregator).
    write_cell_report(cell, spans, coverage_result, shape_results)

    assert coverage_result.ok, coverage_result.missing
    assert all(r.ok for r in shape_results), shape_results
```

### 8.4 Per-cell isolation guarantees

1. **Fresh venv per cell.** Different cells install different SDK and framework
   versions; they cannot share an interpreter without ABI conflicts. The venv
   is cached under `.nox/<cell-id>/` so subsequent runs skip the install when
   nothing changed.
2. **Subprocess per cell.** Even with separate venvs, the provider monkey-patches
   global state at import time; running cells in-process risks one cell's
   patches leaking into another. `nox` already runs each session in a subprocess.
3. **Cassette isolation.** Each cell sample declares its cassette explicitly;
   no implicit shared state.

### 8.5 Per-cell report

`reports/cells/<cell-id>.json`:

```jsonc
{
  "cellId": "traceloop-0.60.0-langchain-0.3.27-py3.11",
  "result": "pass",
  "category": null,
  "skipReason": null,
  "durations": { "install": 4.2, "scenario": 1.1, "validate": 0.3 },
  "coverage":  { "expected": ["llm","tool","chain"], "missing": [] },
  "violations": [
    {
      "spanName": "openai.chat",
      "kind": "llm",
      "rule": "required",
      "path": "/attributes/gen_ai.usage.input_tokens",
      "message": "is required"
    }
  ],
  "capturedSpans": "<gzip+base64 of N spans, kept for diffing>"
}
```

### 8.6 Local developer loop

```bash
# Run one cell
nox -s emission -- --cell-id=traceloop-0.60.0-langchain-0.3.27-py3.11

# Run all langchain cells
nox -s emission -k langchain

# Run the full matrix locally — ~5–10 min once cache is warm
nox -s emission
```

### 8.7 Edge-case categorisation

- **Provider install failure** (`traceloop-sdk==X.Y.Z` not on PyPI) →
  `skipped: install-failure` in the report.
- **Framework install failure** → same.
- **Sample import failure** (framework moved a class) → `fail` with traceback
  under category `sample-import-failure`.
- **No spans captured** → `fail` with category `no-spans-captured` rather than
  the less-actionable "missing span kinds".

## 9. Heavy tier (full pipeline)

### 9.1 What it exercises

- Production `sitecustomize.py` (the real one shipped in the init-container image).
- Obs Gateway JWT validation + tenant tagging.
- OTel Collector batching / serialization at OTLP wire level.
- OpenSearch index mapping (a span that's valid JSON but has a field type the
  index doesn't accept gets silently dropped — emission tier never sees this).
- `agent-manager-observer` enrichment — the `AmpAttributes` shape AMP's
  consumers actually read.

### 9.2 Cell subset

Heavy is a **pipeline test with a per-framework axis for frameworks that ship
a deployable sample**. It deploys the sample matching each cell's framework —
`samples/customer-support-agent` (a LangChain/LangGraph app) for the default
framework, `samples/crewai-agent` for crewai — and verifies its spans survive
the full deployed path and validate against the contract. Three axes change
what gets deployed, so the subset (`harness/heavy_subset.py`) crosses them:

1. **instrumentation/Traceloop version** — the init-container image.
2. **python version** — the buildpack interpreter the agent is built and
   instrumented on.
3. **framework** — but only frameworks with a *deployable* sample, listed in
   `harness/deployable_samples.py`.

So the subset is one cell per (Traceloop version × python) for each deployable
framework, pinned to that framework's representative version. With one
Traceloop version, four pythons, and two deployable frameworks (langchain,
crewai) that's eight cells; it grows as Traceloop versions, pythons, or
deployable samples are added.

The driver deploys the framework-matching sample and asserts that framework's
span kinds (`harness/deployable_samples.py:DEPLOYABLE_SAMPLES[...].expected_kinds`
— `llm` for the LangGraph sample the langchain cell deploys, `llm`+`agent`+
`crewaitask` for crewai), plus shape-validation of every captured span.
Frameworks without a deployable sample (`langgraph`, `llama-index`,
`openai-direct`, `anthropic-direct`) stay emission-only — their per-framework
span *shape* is the emission tier's job (§5). Bringing one into the heavy axis
means adding a deployable sample under `samples/` and an entry to
`DEPLOYABLE_SAMPLES`.

### 9.3 Infra

k3d on a GHA-hosted runner (default) with a self-hosted-runner escape hatch.

```
┌─────────────────────────────── GHA job ────────────────────────────┐
│ ubuntu-latest                                                       │
│                                                                      │
│ ┌─ k3d cluster (built from source via `make setup` / amp-dev-stack)┐│
│ │  • agent-manager-service        • Thunder IDP                    ││
│ │  • agent-manager-observer       • Obs Gateway                    ││
│ │  • OTel Collector               • OpenSearch                     ││
│ │  • <agent pods under test, deployed per cell via REST API>       ││
│ └────────────────────────────────────────────────────────────────────┘│
│                                                                      │
│ Test driver (Python, in heavy/driver.py):                            │
│   token = thunder_oauth2_client_credentials(...)                     │
│   for cell in heavy_cells:                                           │
│       reset_opensearch_indices()                                     │
│       deployed = amp_client.deploy_agent(cell, instr_version)        │
│       try:                                                           │
│           invoke_agent(deployed)                                     │
│           traces = observer.poll_traces(deployed, timeout=120s)      │
│       finally:                                                       │
│           amp_client.teardown_agent(deployed)                        │
│       validate_against("traceloop/v1")                               │
└─────────────────────────────────────────────────────────────────────┘
```

Agents are deployed through `agent-manager-service`'s REST API (the same
flow `test/e2e/framework/shared_agent.go` uses), not raw Kubernetes
Workload manifests. Creating the agent auto-triggers build + deploy (the
driver does not POST a separate deployment — a redundant deploy re-renders
the workload without the secret env and breaks the binding). The request
pins the cell's `instrumentation_version` (selecting the init-container
image) and `python_version` (the buildpack language version); the framework
rides only as an informational `AMP_MATRIX_FRAMEWORK` env var, since the
deployed sample's own `requirements.txt` defines its framework. The driver
records the deployed agent's `(org, project, agent, environment)` on a
`DeployedAgent` record so the subsequent observer poll has the keys
`GET /api/v1/traces` requires.

Auth is OAuth2 `client_credentials` against Thunder IDP. The service URLs
(`AMP_API_BASE_URL`, `AM_OBSERVER_BASE_URL`) and IDP creds
(`IDP_TOKEN_URL`, `IDP_CLIENT_ID`, `IDP_CLIENT_SECRET`) **default** to the
values the dev bring-up exposes (the same defaults the e2e config uses) —
they're overridable but never required. The only real secrets are the LLM
keys, which the driver forwards into each deployed agent so it can make
real provider calls.

### 9.4 One cluster, many cells

- Cluster spin-up amortises across all heavy cells in the job (~5 min once,
  not per cell).
- Between cells the driver creates and tears down agents via the REST API.
  No direct image-swapping happens at the pod spec level — the
  `instrumentation_version` field on the build request controls which
  init-container image lands in the rendered pod.
- OpenSearch state is reset between cells via `spans-*` index deletion in
  the `openchoreo-observability-plane` namespace.

### 9.5 Build-from-source bring-up

The heavy CI job stands up AMP from the **working tree** via the dev
`make setup` chain, wrapped in the `.github/actions/amp-dev-stack`
composite action: `setup-k3d` → `setup-openchoreo` (which builds the
amp-observer + python-instrumentation-provider images from source and
`k3d image import`s them) → `setup-platform` (agent-manager-service via
docker-compose) → migrate → port-forward → gateway.

This is deliberately **not** `deployments/quick-start/install.sh`, which
deploys *released* images at a pinned `VERSION` (that's what the e2e suite
uses). The matrix's purpose is to catch regressions in the PR's observer +
instrumentation code, so it must run the PR's images — an earlier design
that pre-baked a released-image snapshot was dropped for this reason.

The bring-up runs per heavy job (nightly / on-demand), ~15–20 min; that's
acceptable for a non-PR tier and avoids the cross-workflow snapshot-artifact
machinery the earlier design needed.

### 9.6 Heavy-tier failure categories

| Failure                                                         | Treatment                                            |
| ---                                                             | ---                                                  |
| Cell fails at *emission* stage (spans never arrive)             | Counts as emission-tier failure.                     |
| Cell fails at *pipeline* stage (spans arrive, AmpAttributes malformed) | Observer regression suspected; tagged distinctly.    |
| Cell fails at *infra* stage (k3d, helm, OpenSearch readiness)   | Tagged `infra-error`, auto-retried once before failing the job. The repo memory entries `agent-deploy-503-transient` and `k3d host.k3d.internal DNS` document why this is real. |

### 9.7 Self-hosted runner escape hatch

A `runs-on` matrix input lets the heavy tier swap to a self-hosted runner with
a persistent k3d cluster. Not done yet, but the workflow is shaped so adding
the swap later is a one-line change.

### 9.8 Out of scope for heavy tier

- Console rendering / visual validation.
- Evaluator end-to-end runs. The evaluation-job consumes `AmpAttributes`;
  validating `AmpAttributes` shape is the contract the evaluator depends on.

## 10. Manual-instrumentation cells

The manual path is a different mode (no SDK monkey-patching; the agent emits its
own OTel GenAI spans), but the matrix design reuses the same harness with a
different *sample* and a different *provider*.

### 10.1 What `ManualProvider` does — see §5.4.

### 10.2 What the manual sample exercises

`cells/manual_rag_sample.py` is a lightly-trimmed adaptation of
`samples/manual-instrumentation-agent/` — the existing sample that covers every
span kind AMP supports:

```
invoke_agent (agent, root)
└── rag-pipeline (chain)
    ├── embeddings    (embedding)  — real OpenAI call (cassette-recorded)
    ├── vector_search (retriever)  — simulated
    ├── rerank        (rerank)     — simulated
    ├── chat          (llm)        — simulated tool-decision
    ├── execute_tool  (tool)       — real local call
    └── chat          (llm)        — real OpenAI call: final answer (cassette-recorded)
```

One sample, full kind coverage, deterministic via VCR for the OpenAI calls.

### 10.3 Regressions this catches that nothing else would

1. **Sample drift.** If `samples/manual-instrumentation-agent` falls behind a
   schema update (a new `required` field added to `agent` spans), the
   `manual-rag` cell goes red. Without this, the sample is documentation that
   nothing exercises.
2. **`amp-instrumentation` package regressions.** The library's `init_otel()`
   helper is tested across Python versions — without this, the package is only
   tested via "does it ship to PyPI" + ad-hoc usage.

### 10.4 Heavy-tier coverage for the manual path

`heavyTier.perFramework: 1` automatically includes the manual sample as one
heavy cell — same deploy / invoke / validate flow as auto cells. Validates that
a manually-emitted span survives the full pipeline and produces a sensible
`AmpAttributes`. This is the most important manual-path test of all because
it's the one that proves "the contract docs match what AMP actually consumes."

### 10.5 Where schema and docs converge

`make gen-instrumentation-contract` and `scripts/check-contract-drift.sh` cover
the JSON schemas under `contracts/traceloop/v1/` and nothing else: the generator
writes `span.schema.json`, `resource.schema.json` and `kinds/*.schema.json`, and
the drift check diffs exactly those. Neither emits nor validates documentation.

The customer-facing manual-instrumentation contract is the "Manual
instrumentation" section of `docs/guides/amp-instrumentation.mdx` in
[wso2/docs-agent-platform](https://github.com/wso2/docs-agent-platform). It is
written by hand and lives in a different repository, so adding a required
attribute here does not update it and no check catches the two drifting apart.
Changing the contract means a second pull request against that repo.

## 11. CI workflows and triggers

Three workflows, one per trigger tier. All under `.github/workflows/`.

### 11.1 File map

```
.github/workflows/
├── instrumentation-matrix-pr.yaml          # tier 1 — PR / push, path-filtered
├── instrumentation-matrix-nightly.yaml     # tier 2 — nightly cron, full + heavy
└── instrumentation-matrix-manual.yaml      # tier 3 — workflow_dispatch, targeted
```

### 11.2 Tier 1 — PR workflow

```yaml
on:
  pull_request:
    paths:
      - 'python-instrumentation-provider/**'
      - 'libs/amp-instrumentation/**'
      - 'samples/manual-instrumentation-agent/**'
      - '.github/release-config.json'
      - 'test/instrumentation-matrix/**'
      - 'agent-manager-observer/**'
  push:
    branches: [main]
    paths: <same as above>
```

Jobs:

1. `verify-contract-and-manifest` — `make check-contract-drift` (generated
   schemas vs observer parsers) + `make check-matrix-manifest` (matrix ⊇
   release-config). Gates the rest.
2. `default-cell-required` — runs only the `defaultCell`. The required status
   check on the PR.
3. `full-emission-matrix` — runs every cell, sharded into one parallel job
   per Python version (`nox -s emission -- --python-version=<v>`); advisory,
   `continue-on-error`.
4. `publish-matrix-summary` — needs `full-emission-matrix` +
   `default-cell-required`; merges the shard artifacts, aggregates
   `reports/cells/*.json` and renders the summary table to the job's GitHub
   step-summary page (not a PR comment — fork PRs get a read-only token that
   can't comment).
5. `scan-cassettes-for-secrets` — greps committed cassettes for leaked keys.

A required check on `full-emission-matrix` overall would let a single advisory
cell block the PR. Keeping `default-cell-required` separate keeps the required
check stable.

### 11.3 Tier 2 — Nightly workflow

```yaml
on:
  schedule:
    - cron: '30 3 * * *'          # 03:30 UTC / 09:00 IST — matches traceloop-release-watch
  workflow_dispatch:
```

Jobs:

1. `full-emission-matrix` — same as tier 1 but ungated by file paths.
2. `heavy-tier` — needs `full-emission-matrix`; one shard per Traceloop
   version (listed by `resolve-heavy-shards`, passed to the driver as
   `HEAVY_PROVIDER_VERSION`). Each shard builds AMP from source on its own
   k3d stack (amp-dev-stack), iterates its heavy cells, and uploads its
   reports.
3. `revalidate-known-broken` — re-runs every `known-broken` cell; opens an
   issue if any previously-broken cell now passes ("drop the exemption").
4. `publish-matrix-summary` — aggregates emission + heavy reports, renders
   the summary, and exposes counts / likely-cause as job outputs.
5. `open-issue-on-failure` — if anything red, opens (or updates) an
   `instrumentation-matrix-failure` GitHub issue with the matrix report.
6. `notify-google-chat` — posts to the team Google Chat space via the same
   webhook used by `traceloop-release-watch.yaml`. One message per nightly,
   listing failed cells with a link to the issue. Quiet on success.

### 11.4 Tier 3 — Manual workflow

```yaml
on:
  workflow_dispatch:
    inputs:
      traceloop_versions:   { type: string, description: 'comma-separated; default = all' }
      frameworks:           { type: string, description: 'comma-separated; default = all' }
      framework_versions:   { type: string, description: 'comma-separated; default = all' }
      python_versions:      { type: string, description: 'comma-separated; default = all' }
      include_heavy_tier:   { type: boolean, default: false }
      include_record_mode:  { type: boolean, default: false }   # re-record cassettes
      heavy_only:           { type: boolean, default: false }   # skip emission
      heavy_cell_id:        { type: string,  default: '' }      # one heavy cell
```

The four filters narrow the heavy subset as well as the emission matrix
(`harness/cell_filter.py`). The heavy tier is sharded per Traceloop version
like the nightly's, so `traceloop_versions=<new>` runs one ~50-minute shard
rather than the whole subset.

`include_record_mode` runs with `VCR_RECORD_MODE=once`, uses an
OPENAI/ANTHROPIC key from a CI secret, and opens a follow-up PR with the
re-recorded cassettes for review.

### 11.5 Summary format (rendered to the run's step-summary page)

```
## Instrumentation matrix — emission tier

| Cell | Result | Detail |
|---|---|---|
| ✅ traceloop-0.60.0-langchain-0.3.27-py3.11   | pass    | (default cell, required)             |
| ✅ traceloop-0.60.0-langchain-0.3.27-py3.12   | pass    |                                      |
| ❌ traceloop-0.60.0-crewai-0.86.0-py3.11      | fail    | required attr `gen_ai.system` missing on tool span (view diff) |
| ⚠️ traceloop-0.60.0-llama-index-0.12.0-py3.13| skipped | install-failure: no py3.13 wheel yet  |
| ✅ manual-0.2.1-manual-rag-py3.11             | pass    |                                      |

Total: 96 pass · 1 fail · 3 skipped · 0 advisory failing required check
⚠️ 1 advisory cell red. Default cell green. Required check passing.
```

### 11.6 Required-status table

| Workflow         | Required check              | What blocks the PR / triggers alerts          |
| ---              | ---                         | ---                                           |
| Tier 1 (PR)      | `default-cell-required`     | Default cell broken.                          |
| Tier 1 (PR)      | `publish-matrix-summary`    | Informational only.                           |
| Tier 2 (nightly) | none — runs on main         | Failures open an issue + Chat message.        |
| Tier 3 (manual)  | none — operator-driven      | Operator decides next step from the report.   |

### 11.7 Concurrency

All three workflows share `concurrency: instrumentation-matrix-${{ github.ref }}`
with `cancel-in-progress: true` on tier 1 only. Tier 2 / 3 never cancel — a
force-push shouldn't be able to interrupt a nightly or on-demand run.

### 11.8 Cost ceiling

- Tier 1, full matrix: ~10 min on one `ubuntu-latest` runner per PR push.
- Tier 2, nightly: ~10 min emission + ~1 h heavy. ~2 runner-hours/day.
- Tier 3: ad-hoc.

Under 2 % of a typical team's monthly GHA minutes.

## 12. Reporting, triage, failure handling

### 12.1 Failure taxonomy

Every cell stamps a single `category`. The aggregator counts by category, so
triage starts with "what kind of failure am I looking at," not "which cell."

| Category                | What it means                                                                    | Likely owner                                              |
| ---                     | ---                                                                              | ---                                                       |
| `install-failure`       | Pip can't install the requested `(provider, framework, python)` combo.            | Manifest maintainer — `known-broken` until upstream ships. |
| `sample-import-failure` | Sample file fails to import because the framework moved/renamed an API.          | Sample maintainer — leading indicator that Traceloop needs an update. |
| `no-spans-captured`     | Sample ran, SDK didn't see anything.                                              | Provider (Traceloop) — monkey-patch broke.                |
| `missing-span-kind`     | Some spans captured, but a declared kind is absent.                              | Provider — partial instrumentation broke.                 |
| `schema-violation`      | All kinds present, but one or more attributes fail JSON-schema validation.       | Provider OR observer — see §12.2.                         |
| `cassette-miss`         | A recorded interaction no longer matches.                                        | Sample or provider — upstream changed what's sent.        |
| `pipeline-error` (heavy) | Spans emitted, pipeline returned 5xx, malformed `AmpAttributes`, or timed out.   | `agent-manager-observer` or infra.                        |
| `infra-error` (heavy)   | k3d / helm / OpenSearch didn't come up.                                          | Suite infra — auto-retried once.                          |

### 12.2 Likely-cause heuristics

The aggregator suggests which side of the fence a failure is on:

- `schema-violation` on every cell of `traceloop@X.Y.Z`, regardless of framework
  → Traceloop regression in X.Y.Z.
- `schema-violation` on cells across *all* Traceloop versions sharing the same
  attribute → schema/observer regression (the schema added a `required` no
  provider emits yet).
- `schema-violation` confined to one `(traceloop × framework × framework-version)`
  triplet → upstream framework regression Traceloop hasn't caught up to.

Each diagnosis is a one-line annotation linking to raw cell reports.

### 12.3 The `known-broken` workflow

1. Engineer adds entries under `matrix.yaml.known-broken` with `reason`, `until`.
2. Matrix run still expands and reports the cell, but as `skipped-known-broken`.
3. Nightly: `revalidate-known-broken` job re-runs every known-broken cell
   unconditionally. A passing cell auto-opens an issue suggesting the exemption
   be dropped.
4. Past `until:`, the cell un-skips automatically. Forces a deliberate
   re-extend or fix.

### 12.4 Google Chat alert (mirrors `traceloop-release-watch.yaml`)

```
🔴 Instrumentation matrix — nightly run failed
Run: <link>
Issue: <link to auto-opened issue>

Summary
  • 4 failed / 92 passed / 4 skipped
  • Likely cause: Traceloop 0.61.0 regression — all langchain cells red
  • Heavy tier: 1 pipeline-error on traceloop-0.60.0-crewai

Top failing categories
  • schema-violation: 3
  • pipeline-error: 1
```

Quiet on success. Posts only on failure or on "previously-broken cell now
passes."

### 12.5 Triage page

Each run produces a triage directory under
`test/instrumentation-matrix/reports/<run-id>/`:

```
├── summary.md
├── cells/
│   └── <cell-id>.json
├── diffs/
│   └── <cell-id>.diff.md      # span-by-span JSON diff against the schema
└── spans/
    └── <cell-id>.ndjson       # raw captured spans for deep dives
```

The `.diff.md` files are the workhorse — for `schema-violation`, they show the
expected schema slice, the captured span attribute map, and a unified diff
highlighting missing / wrong-typed keys. Most red cells are diagnosed from the
diff alone (~30 s of reading) without reproducing locally. **Design target: red
cell → verdict in under a minute.**

## 13. File layout

```
test/instrumentation-matrix/
├── README.md                              # quickstart + doc index
├── DESIGN.md                              # this document (architecture)
├── RUNBOOK.md                             # operational how-to (incl. heavy-tier deploy contract)
├── FINDINGS.md                            # upstream gaps + schema concessions log (F-NNN ids)
├── matrix.yaml                            # the manifest (§4)
├── noxfile.py                             # cell driver (§8) + report + heavy sessions
├── pyproject.toml                         # suite-level deps (nox, jsonschema, pytest-recording)
├── harness/
│   ├── manifest.py                        # parses matrix.yaml, expands cells
│   ├── provider.py                        # Protocol (§5)
│   ├── validator.py                       # ContractValidator (§6.4)
│   ├── classify.py                        # span-kind classifier (shared with observer)
│   ├── exporter_handle.py                 # InMemorySpanExporter registry
│   ├── reports.py                         # writes + loads per-cell JSON reports
│   ├── aggregator.py                      # builds the per-tier Markdown summary
│   ├── categorize.py                      # FailureCategory taxonomy (§12.1)
│   ├── triage.py                          # per-cell .diff.md generator (§12.5)
│   ├── notify.py                          # Google Chat alert message builder
│   ├── heavy_subset.py                    # representative-cell selector for the heavy tier (§9.2)
│   ├── deployable_samples.py              # framework → deployable sample app + asserted kinds (§9.2)
│   ├── revalidate.py                      # known-broken re-expansion helper (§12.3)
│   └── test_cell.py                       # per-cell pytest body (§8.3)
├── providers/
│   ├── __init__.py                        # PROVIDERS registry
│   ├── traceloop.py
│   ├── manual.py
│   └── bootstrap/                         # forked sitecustomize per provider (InMem capture)
│       ├── traceloop/sitecustomize.py
│       └── manual/sitecustomize.py
├── cells/
│   ├── langchain_sample.py
│   ├── langgraph_sample.py
│   ├── llama_index_sample.py
│   ├── crewai_sample.py
│   ├── openai_direct_sample.py
│   ├── anthropic_direct_sample.py
│   └── manual_rag_sample.py
├── contracts/
│   └── traceloop/v1/...                   # §6.1
├── cassettes/                             # §7.2
├── heavy/                                 # heavy-tier driver (§9; deploy contract in RUNBOOK.md §7)
│   ├── driver.py                          # orchestrator
│   ├── amp_client.py                      # agent-manager-service REST client
│   ├── observer.py                        # agent-manager-observer query
│   └── k3d.py                             # per-cell OpenSearch index reset
├── scripts/
│   ├── record_cassette.py                 # one-shot cassette recorder (§7.4)
│   ├── scrub_cassettes.py                 # strip identifying headers post-record
│   ├── expand_filter.py                   # cell-id filter for the manual workflow
│   └── resolve_heavy_shards.py            # per-Traceloop heavy shards (nightly + manual)
└── reports/                               # per-run output (gitignored)

.github/
├── actions/amp-dev-stack/                 # build-from-source AMP bring-up (§9.5)
└── workflows/
    ├── instrumentation-matrix-pr.yaml
    ├── instrumentation-matrix-nightly.yaml
    └── instrumentation-matrix-manual.yaml

scripts/
├── check-cassettes.py                     # cassette secret-leak guard
├── check-contract-drift.sh                # observer parser ↔ schema drift
└── check-matrix-manifest.py               # matrix ⊇ release-config invariant
```

## 14. Open questions / future work

These are deliberately deferred to a later milestone:

1. **OpenInference / OpenLit providers.** The suite ships the abstraction,
   not these implementations. Wiring them up is a contained follow-up: one
   provider class, one contract schema bundle.
2. **Vector-DB and reranker cells.** Chroma, Pinecone, Cohere rerank, etc.
   Deferred until the core suite is solid; manifest already accommodates them.
3. **A self-hosted runner for the heavy tier.** GHA-hosted is fine for
   current volumes; the workflow is shaped to swap in a persistent runner
   later.
4. **Console rendering tests.** Out of scope for this design.
5. **Migrating off Traceloop entirely.** Out of scope; the provider
   abstraction is what makes that conversation tractable when it arrives.
6. **Heavy-tier first live run.** `heavy/amp_client.py` and
   `heavy/observer.py` are implemented against the e2e Go reference (with
   mocked-HTTP unit tests) but have never run against a live AMP stack. The
   first real run needs the LLM-key secrets + a green `amp-dev-stack`
   build-from-source bring-up, and will likely require tuning that bring-up,
   the timing constants, and the observer `/spans` param mapping. Until then
   the heavy jobs are `continue-on-error: true`.
7. **Per-kind triage diffs.** Today's `.diff.md` pages list the union of
   `attributes.required` across the cell's expected kinds. A richer
   per-kind diff (expected schema slice vs captured span attribute map)
   would shave more seconds off the "red cell → verdict" target.
8. **Span events in the captured-span shape.** `harness/test_cell.py::
   _to_dict` flattens spans without their events. Some providers may
   encode logical sub-operations (a tool call inside an LLM span) as
   events rather than separate spans; F-009 in `FINDINGS.md` tracks the
   harness fix.

---
