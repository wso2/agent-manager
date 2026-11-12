"""Run a cell's sample agent locally as an external agent.

The heavy tier exercises the real Agent Manager trace pipeline: the sample runs
on this machine under `amp-instrument` from the cell's pinned
`amp-instrumentation` release, on the cell's Python, and exports to the AI
Gateway's /otel endpoint with the agent's Agent Manager token. Spans then go
gateway → collector → trace store → observer, which is what the driver polls.
"""
from __future__ import annotations

import contextlib
import os
from pathlib import Path
import shutil
import signal
import socket
import subprocess
import tempfile
import time

import requests

from harness.deployable_samples import DeployableSample

REPO_ROOT = Path(__file__).resolve().parents[3]
_INSTALL_TIMEOUT_S = 900
_READY_TIMEOUT_S = 180


class CellSkipped(RuntimeError):
    """The cell cannot run here (e.g. its Python interpreter is not installed)."""


def _free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def _stop(process: subprocess.Popen) -> None:
    if process.poll() is None:
        os.killpg(process.pid, signal.SIGTERM)
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=5)


def _wait_ready(url: str, process: subprocess.Popen) -> None:
    deadline = time.monotonic() + _READY_TIMEOUT_S
    last = ""
    while time.monotonic() < deadline:
        if process.poll() is not None:
            raise RuntimeError(f"agent exited before it was ready (exit={process.returncode})")
        try:
            if requests.get(url, timeout=3).status_code < 500:
                return
        except requests.RequestException as e:
            last = str(e)
        time.sleep(1)
    raise TimeoutError(f"agent never became ready at {url} (last: {last})")


@contextlib.contextmanager
def run_local_agent(*, sample: DeployableSample, python_version: str, instrumentation_version: str,
                    token: str, gateway_url: str, agent_env: dict[str, str], log_path: Path):
    """Install and start the sample; yield its base URL. Stops it on exit."""
    python = shutil.which(f"python{python_version}")
    if python is None:
        raise CellSkipped(f"python{python_version} is not installed on this runner")
    app_dir = REPO_ROOT / sample.app_path.lstrip("/")
    with tempfile.TemporaryDirectory(prefix="matrix-heavy-") as work:
        venv = Path(work) / "venv"
        with log_path.open("w") as log:
            subprocess.run([python, "-m", "venv", str(venv)], check=True, stdout=log, stderr=subprocess.STDOUT)
            subprocess.run(
                [str(venv / "bin/python"), "-m", "pip", "install", "--disable-pip-version-check",
                 "-r", str(app_dir / "requirements.txt"), f"amp-instrumentation=={instrumentation_version}"],
                check=True, stdout=log, stderr=subprocess.STDOUT, timeout=_INSTALL_TIMEOUT_S,
            )
            log.write("\n=== agent ===\n")
            log.flush()
            port = _free_port()
            env = {k: os.environ[k] for k in ("PATH", "HOME", "LANG") if k in os.environ}
            env.update(dict(sample.env))
            env.update(agent_env)
            env.update(AMP_OTEL_ENDPOINT=gateway_url.rstrip("/") + "/otel", AMP_AGENT_API_KEY=token,
                       AMP_TRACE_CONTENT="true", OTEL_BSP_SCHEDULE_DELAY="100", PYTHONUNBUFFERED="1")
            process = subprocess.Popen(
                [str(venv / "bin/amp-instrument"), str(venv / "bin/python"), "-m", "uvicorn", sample.asgi_app,
                 "--host", "127.0.0.1", "--port", str(port)],
                cwd=app_dir, env=env, stdout=log, stderr=subprocess.STDOUT, start_new_session=True,
            )
            try:
                endpoint = f"http://127.0.0.1:{port}"
                _wait_ready(endpoint + "/openapi.json", process)
                yield endpoint
            finally:
                _stop(process)
