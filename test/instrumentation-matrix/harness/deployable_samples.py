"""Registry of frameworks that ship a runnable agent app under samples/.

The heavy tier runs a real agent per cell as an external agent (see
heavy/local_agent.py). Only frameworks with an HTTP `/chat` ASGI app (an
`app.py`, not the emission tier's in-process `cells/*_sample.py` scripts) can
run there, so this map is the single source of truth for:

  * which frameworks the heavy subset runs (harness/heavy_subset.py),
  * which sample each cell runs (heavy/local_agent.py), and
  * which span kinds the driver asserts for that sample (heavy/driver.py).

`expected_kinds` are the kinds the *running sample* emits — for the LangGraph
customer-support-agent that the langchain cell runs, that's just `llm`; the
crewai agent additionally emits `agent` and `crewaitask`. Tool spans are not
asserted (Traceloop 0.60 may fold them into span events; see FINDINGS F-003).

To make another framework runnable: add a sample under `samples/<name>/`
mirroring this contract, then add an entry here.
"""
from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class DeployableSample:
    app_path: str  # repo-relative sample directory (leading slash)
    asgi_app: str  # uvicorn target inside app_path, e.g. "app:app"
    expected_kinds: tuple[str, ...]  # span kinds the driver asserts coverage of
    # Non-sensitive env vars set on the running agent. Needed when a sample
    # (or amp-instrument, which imports the framework at interpreter startup —
    # before the app runs) requires environment that must exist before the
    # process starts. CrewAI writes under $HOME at import, so it crashes under a
    # read-only HOME without these. Tuple of pairs (not a dict) to keep the
    # dataclass frozen-friendly.
    env: tuple[tuple[str, str], ...] = ()
    # Shared by dependency compatibility checks and heavy cell selection.
    min_python: str = "3.10"

    def supports_python(self, version: str) -> bool:
        """Whether an interpreter meets the sample's minimum Python version."""
        current = tuple(map(int, version.split(".")))
        minimum = tuple(map(int, self.min_python.split(".")))
        return current >= minimum


DEPLOYABLE_SAMPLES: dict[str, DeployableSample] = {
    "langchain": DeployableSample(
        app_path="/samples/customer-support-agent",
        asgi_app="app:app",
        expected_kinds=("llm",),
    ),
    "crewai": DeployableSample(
        app_path="/samples/crewai-agent",
        asgi_app="app:app",
        expected_kinds=("llm", "agent", "crewaitask"),
        min_python="3.11",
        env=(("HOME", "/tmp"), ("CREWAI_STORAGE_DIR", "/tmp/crewai")),
    ),
}
