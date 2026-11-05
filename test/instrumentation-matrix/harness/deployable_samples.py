"""Registry of frameworks that ship a *deployable* agent app under samples/.

The heavy tier deploys a real agent per cell. Only frameworks with a buildpack-
deployable HTTP `/chat` app (an `app.py` + `main.py`, not the emission tier's
in-process `cells/*_sample.py` scripts) can be deployed, so this map is the
single source of truth for:

  * which frameworks the heavy subset runs (harness/heavy_subset.py),
  * which sample each cell deploys (heavy/amp_client.py), and
  * which span kinds the driver asserts for that sample (heavy/driver.py).

`expected_kinds` are the kinds the *deployed sample* emits — for the LangGraph
customer-support-agent that the langchain cell deploys, that's just `llm`; the
crewai agent additionally emits `agent` and `crewaitask`. Tool spans are not
asserted (Traceloop 0.60 may fold them into span events; see FINDINGS F-003).

To make another framework deployable: add a sample under `samples/<name>/`
mirroring this contract, then add an entry here.
"""
from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class DeployableSample:
    app_path: str  # repo-relative path the buildpack builds (leading slash)
    run_command: str  # buildpack run command, e.g. "python main.py"
    expected_kinds: tuple[str, ...]  # span kinds the driver asserts coverage of
    # Non-sensitive env vars set on the deployed workload. Needed when a sample
    # (or the instrumentation provider's sitecustomize, which imports the
    # framework at interpreter startup — before the app runs) requires
    # environment that must exist before the process starts. CrewAI writes
    # under $HOME at import; the buildpack container's HOME=/nonexistent is
    # read-only, so both the CrewAI instrumentor and the app crash without
    # these. Tuple of pairs (not a dict) to keep the dataclass frozen-friendly.
    env: tuple[tuple[str, str], ...] = ()
    # Shared by dependency compatibility checks and heavy deployment selection.
    min_python: str = "3.10"
    max_python_exclusive: str | None = None

    def supports_python(self, version: str) -> bool:
        """Whether an interpreter falls within the sample's supported range."""
        current = tuple(map(int, version.split(".")))
        minimum = tuple(map(int, self.min_python.split(".")))
        maximum = (
            tuple(map(int, self.max_python_exclusive.split(".")))
            if self.max_python_exclusive is not None else None
        )
        return current >= minimum and (maximum is None or current < maximum)


DEPLOYABLE_SAMPLES: dict[str, DeployableSample] = {
    "langchain": DeployableSample(
        app_path="/samples/customer-support-agent",
        run_command="python main.py",
        expected_kinds=("llm",),
    ),
    "crewai": DeployableSample(
        app_path="/samples/crewai-agent",
        run_command="python main.py",
        expected_kinds=("llm", "agent", "crewaitask"),
        min_python="3.11",
        max_python_exclusive="3.14",
        env=(("HOME", "/tmp"), ("CREWAI_STORAGE_DIR", "/tmp/crewai")),
    ),
}
