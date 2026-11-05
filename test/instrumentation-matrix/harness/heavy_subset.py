"""Pick a representative subset of cells for the heavy tier.

The heavy tier is expensive — every cell deploys a real agent against a k3d
stack, hits the API, polls the observer. So we run a sample, not the full
matrix.

**Heavy has a per-framework axis for frameworks that ship a deployable sample.**
`harness/deployable_samples.py` lists them (langchain -> customer-support-agent,
crewai -> crewai-agent today). For each, the driver deploys the matching sample
and asserts that framework's span kinds. Frameworks without a deployable sample
(`langgraph`, `llama-index`, `openai-direct`, `anthropic-direct`) stay
emission-only — their per-framework span *shape* is the emission tier's job (it
runs each `cells/<framework>_sample.py` in-process).

Three axes change what gets deployed, so the subset crosses all three:

1. **instrumentation/Traceloop version** — pins the init-container image.
2. **python version** — the buildpack builds (and instrumentation runs) the
   agent on that interpreter, so a py-version-specific break surfaces through
   the deployed pipeline. Only sample-supported interpreters are selected.
3. **framework** — but only frameworks in DEPLOYABLE_SAMPLES.

So the subset is one cell per (Traceloop version x python) for each deployable
framework, pinned to that framework's representative version: the default-cell
version for the default framework, the first declared version otherwise.
"""
from __future__ import annotations

from harness.cell_filter import CellFilter
from harness.deployable_samples import DEPLOYABLE_SAMPLES
from harness.manifest import Cell, Manifest, expand_matrix


def select_heavy_subset(cells: list[Cell], manifest: Manifest) -> list[Cell]:
    """Select representative deployments on each sample's supported runtimes."""
    default = manifest.default_cell

    # Pin one framework_version per deployable framework. The framework axis is
    # representative, not exhaustive: one version stands in for the framework,
    # just as the Traceloop axis stands in for span shape.
    pinned_version: dict[str, str] = {}
    for fw in manifest.frameworks:
        if fw.name in DEPLOYABLE_SAMPLES:
            pinned_version[fw.name] = (
                default.framework_version
                if fw.name == default.framework
                else fw.versions[0]
            )

    out: list[Cell] = []
    seen: set[str] = set()
    for c in cells:
        # Heavy only covers the default (init-container-shipping) provider;
        # manual cells are emission-only.
        if c.provider_name != default.provider:
            continue
        if c.framework_name not in pinned_version:
            continue
        if not DEPLOYABLE_SAMPLES[c.framework_name].supports_python(c.python):
            continue
        if c.framework_version != pinned_version[c.framework_name]:
            continue
        if c.id in seen:
            continue
        seen.add(c.id)
        out.append(c)
    return out


def select_heavy_run(
    manifest: Manifest, cell_filter: CellFilter = CellFilter(), cell_id: str = ""
) -> list[Cell]:
    """The heavy cells one run covers: the subset, narrowed by the manual
    workflow's filters, or just `cell_id` when one is given (filters are
    ignored then — it names the exact cell to run). The driver and the CI
    shard resolver both call this, so they always agree."""
    cells = select_heavy_subset(expand_matrix(manifest), manifest)
    if cell_id:
        return [c for c in cells if c.id == cell_id]
    return cell_filter.apply(cells)


def heavy_shards(cells: list[Cell]) -> list[str]:
    """The provider versions the heavy subset spans, one CI shard each.

    The nightly runs one heavy job per Traceloop version, each on its own
    stack, so a new release adds a parallel shard instead of lengthening a
    single serial run.
    """
    return sorted({c.provider_version for c in cells}, key=_version_key)


def filter_shard(cells: list[Cell], provider_version: str) -> list[Cell]:
    return [c for c in cells if c.provider_version == provider_version]


def _version_key(v: str) -> tuple:
    return tuple(int(p) if p.isdigit() else p for p in v.split("."))
