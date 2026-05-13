# Releasing a new AMP instrumentation version

This is the maintainer runbook for cutting a new **AMP instrumentation version** —
the most common reasons being a `traceloop-sdk` (OpenLLMetry) bump or adding/dropping
a supported Python version. It covers both the init-container image
(`python-instrumentation-provider/`, this directory) and the PyPI package
(`libs/amp-instrumentation/`), because they share one version number.

## The model (read this first)

One identifier — the **AMP instrumentation version** (an independent semver, e.g.
`0.2.0`, *decoupled* from the AMP product release) — drives three artifacts:

| Artifact | What it is | Versioned how |
|---|---|---|
| `amp-instrumentation` PyPI package | the externally-hosted auto-instrumentation library + the `init_otel()` helper | the `target_version` you type into the `AMP Instrumentation Release` workflow |
| `ghcr.io/wso2/amp-python-instrumentation-provider:<version>-python<X.Y>` init-container images | the platform-hosted auto-instrumentation, one image per `(AMP-instr version × Python version)` | the `instrumentation_version` field in `.github/release-config.json` |
| `agent-manager-service` platform default | the AMP-instr version a Python agent gets when it hasn't selected one | `OTEL_DEFAULT_INSTRUMENTATION_VERSION` env (default in `config_loader.go`) |

Each AMP-instr version pins **exactly one** `traceloop-sdk` version. Existing agents
stay on the version they were pinned to — bumping the default never moves them.

### Sources of truth — what lives where

| Thing | File / place |
|---|---|
| `traceloop-sdk` pin for the **PyPI package** | `libs/amp-instrumentation/pyproject.toml` → `dependencies` → `"traceloop-sdk==<X>"` |
| **PyPI package version** | the `target_version` input to `.github/workflows/amp_instrumentation_release.yaml` (it `sed`s `pyproject.toml`'s `version`; the repo value is the placeholder `0.0.0-dev`; `__init__.py.__version__` just reads it back from package metadata — don't hand-edit it) |
| **Image build matrix** (which `(AMP-instr version × Python)` images to build, and the `traceloop-sdk` baked into each) | `.github/release-config.json` → `python-instrumentation-provider` → array of `{ "instrumentation_version", "traceloop_version", "python_versions" }` |
| **Image build (primary)** | `.github/workflows/python_instrumentation_image_release.yaml` — standalone `workflow_dispatch` (inputs `branch`, `instrumentation_version`); reads `release-config.json`, filters to the requested version, builds & pushes that matrix. Use this to ship images independently of an AMP product release. |
| **Image build (also runs on every AMP product release)** | `.github/workflows/release.yml` → `build-python-instrumentation-provider-images` job — same `release-config.json` matrix, but rebuilds **every** listed version on each product release (refreshes base-image layers). |
| **Image build args / defaults** | `python-instrumentation-provider/Dockerfile` (`ARG TRACELOOP_VERSION`, `ARG PYTHON_VERSION`) and `python-instrumentation-provider/Makefile` — these defaults are only for local `docker build` / `make build`; CI always passes the real values from `release-config.json` |
| **Platform default AMP-instr version** | `agent-manager-service/config/config_loader.go` → `OTEL_DEFAULT_INSTRUMENTATION_VERSION` (env override) |
| **Customer-facing version → `traceloop-sdk` → supported-Python mapping table** | `documentation/docs/components/amp-instrumentation.mdx` |
| Console: which Python versions an agent can pick | the `languageVersion` field in `console/workspaces/pages/add-new-agent/` (must stay in sync with the `python_versions` we build images for) |

> The init-container image's Python version **must match the agent's runtime Python** —
> the image pre-installs `traceloop-sdk` and its compiled-C-extension deps into
> `packages/`, which the agent's Python loads via `PYTHONPATH`. So we build one image
> per supported Python version, and the set of `python_versions` in `release-config.json`
> must cover what the AMP buildpack supports.

### When do the artifacts actually publish?

**Neither artifact is published by a PR merge.** All three release workflows are
`workflow_dispatch`-only — they run when someone *manually* dispatches them:

- **PyPI package** — `.github/workflows/amp_instrumentation_release.yaml`. Type the
  `target_version` (e.g. `0.3.0`) and the chosen `branch` (usually `main`); the
  workflow `sed`s `pyproject.toml`'s `version`, builds, publishes to PyPI, and tags
  `amp-instrumentation/v<target_version>`. **Run this when** you've merged the
  `traceloop-sdk` pin update and want to publish a new PyPI version.
- **Init-container images — standalone (primary path)**:
  `.github/workflows/python_instrumentation_image_release.yaml`. Inputs: `branch`
  and `instrumentation_version` (a specific version like `0.3.0`, or `all`). It reads
  `release-config.json`, filters to the requested version, and builds & pushes the
  matching `(instr × python)` matrix as
  `amp-python-instrumentation-provider:<instr_version>-python<X.Y>`. **Use this
  whenever you want to ship instrumentation images independently of an AMP product
  release** (the common case).
- **Init-container images — bundled with the AMP product release**:
  `.github/workflows/release.yml` (the *AMP product* release workflow) also builds
  the *full* `release-config.json` matrix as part of every product release. So even
  if you never trigger the standalone workflow, each product release rebuilds every
  listed instrumentation image with the latest base-image layers.

So when you add a new instrumentation-version entry to `release-config.json` (or a
new Python to an existing entry) and merge the PR — **the images don't appear
immediately**. You publish them by dispatching the standalone workflow (the
preferred path), or wait for the next AMP product release. The entry in
`release-config.json` just tells whichever workflow runs *what* to build.

Avoid pushing from a local `make build` for customer-pullable images — that bypasses
CI and leaves no audit trail.

Every subsequent AMP product release re-runs the same image builds (the matrix in
`release-config.json` doesn't change unless edited), so the same tag gets pushed
again with a refreshed base layer (security patches in `python:X.Y-slim`). That's
expected: the traceloop pin is identical, the tag is logically *"AMP-instr version
X for Python Y"*, and the OS refresh is a feature, not drift.

---

## Scenario A — bump `traceloop-sdk` (new OpenLLMetry version)

Example: `traceloop-sdk` `0.60.0` → `0.65.0`, cutting AMP-instr version `0.3.0`.

1. **Validate** `traceloop-sdk==0.65.0` against the frontier frameworks (existing
   validation process — out of scope here). Only cut a version for releases we've validated.
2. **Pick the new AMP-instr semver.** Minor bump if there's a behaviour change (a new
   OpenLLMetry usually is); patch for trivial fixes. Say `0.3.0`.
3. **PyPI package** (`libs/amp-instrumentation/`):
   - Edit `pyproject.toml` → `dependencies` → `"traceloop-sdk==0.65.0"`. (Leave `version = "0.0.0-dev"` alone.)
   - PR → review → merge to `main`.
   - Run the **`AMP Instrumentation Release`** workflow (`amp_instrumentation_release.yaml`,
     `workflow_dispatch`) with `branch = main`, `target_version = 0.3.0`. It updates
     `pyproject.toml`'s `version`, builds, publishes `amp-instrumentation==0.3.0` to PyPI,
     and tags `amp-instrumentation/v0.3.0`.
4. **Init-container images** (`.github/release-config.json`): **add a new entry** to the
   `python-instrumentation-provider` array (keep the old ones — see "Retention" below):
   ```json
   { "instrumentation_version": "0.3.0", "traceloop_version": "0.65.0", "python_versions": ["3.10", "3.11", "3.12", "3.13"] }
   ```
   No Dockerfile change needed. Merge the PR, then publish the images by dispatching the
   **`AMP Python Instrumentation Image Release`** workflow
   (`python_instrumentation_image_release.yaml`) with `branch = main`,
   `instrumentation_version = 0.3.0`. It builds & pushes the `(0.3.0 × supported python)`
   matrix to `amp-python-instrumentation-provider:0.3.0-python{X.Y}`. (You don't have to
   wait for an AMP product release — that workflow is independent. The next product
   release will also rebuild this matrix, refreshing the base layers — that's fine.)
5. **Make it the platform default** (when you want *new* agents to get `0.3.0`):
   set `OTEL_DEFAULT_INSTRUMENTATION_VERSION=0.3.0` on the `agent-manager-service`
   deployment (or change the default in `config_loader.go`). Existing agents are unaffected.
6. **Docs / mapping table**: add a `0.3.0 → traceloop-sdk 0.65.0 → python 3.10–3.13` row
   to `documentation/docs/components/amp-instrumentation.mdx`.
7. **Console** (if a version selector exists): add `0.3.0` to its options.

## Scenario B — add (or drop) a supported Python version

Example: AMP buildpack starts supporting Python `3.14`.

1. **Confirm the buildpack supports it** — an agent can only run on a Python version the
   buildpack supports; that's what makes the image worth building.
2. **Init-container images** (`.github/release-config.json`): add `"3.14"` to the
   `python_versions` array of the AMP-instr version(s) you want it for (typically at least
   the current one). To *drop* a Python (e.g. EOL `3.10`), remove it — but only once no live
   agent runs on it; the image stays pullable for whatever versions remain listed in each entry.
   No Dockerfile change (`ARG PYTHON_VERSION` already parameterizes it).
3. **Console** (B9): add `"3.14"` to the `languageVersion` dropdown options. Keep this list
   exactly aligned with the `python_versions` we build images for — if a user picks a Python
   we have no image for, the init container `ImagePullBackOff`s.
4. **No PyPI change** — `amp-instrumentation` isn't Python-version-specific (the per-Python
   pre-install only matters for the init-container image; on the externally-hosted path the
   user's own environment provides the Python).
5. **Docs**: update the supported-Python list / mapping table in `amp-instrumentation.mdx`.

## Retention

Keep **every published `instrumentation_version` entry** in `release-config.json` — the
images are small, and agents pinned to an old version need their image to stay pullable
(the release workflow simply rebuilds whatever's listed, picking up base-image patches).
Only prune a very old entry after confirming no agent pins it.

## Verifying a release

- **PyPI:** `pip install amp-instrumentation==0.3.0 && pip show traceloop-sdk` (expect the pinned version) and `python -c "import amp_instrumentation; print(amp_instrumentation.__version__)"` (expect `0.3.0`).
- **Image:** `docker run --rm ghcr.io/wso2/amp-python-instrumentation-provider:0.3.0-python3.11 sh -c 'cat /instrumentations/otel-tracing/traceloop_sdk-*.dist-info/METADATA | grep ^Version'` (or just `ls /instrumentations/otel-tracing/`).
- **agent-manager-service:** deploy a Python agent with auto-instrumentation on; confirm the init container in the pod is `…:<expected version>-python<agent's Python>`.

## Quick reference — what changes where

| Change | `libs/amp-instrumentation/pyproject.toml` | `amp_instrumentation_release.yaml` run | `.github/release-config.json` | `python_instrumentation_image_release.yaml` run | `agent-manager-service` env | `amp-instrumentation.mdx` | Console `languageVersion` |
|---|---|---|---|---|---|---|---|
| Bump `traceloop-sdk` (new AMP-instr version) | `traceloop-sdk==<new>` | `target_version=<new AMP-instr version>` | add `{instrumentation_version, traceloop_version, python_versions}` entry | `instrumentation_version=<new>` to publish the images | bump `OTEL_DEFAULT_INSTRUMENTATION_VERSION` when promoting to default | add a row | add the version (if listed) |
| Add a supported Python | — | — | add `"3.X"` to `python_versions` | re-run (`all` or the affected version) to publish the new-Python images | — | update the Python list | add `"3.X"` |
