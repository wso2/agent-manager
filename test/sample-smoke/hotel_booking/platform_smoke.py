"""Run the Hotel Booking sample as an external agent and find its trace in Agent Manager."""
from __future__ import annotations

import contextlib
from pathlib import Path
import sys
import tempfile

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from external_agent import (  # noqa: E402
    ExternalSample, base_env, free_port, make_venv, parse_args, run, started, wait_ready,
)
from support import ROOT, check_evidence, invoke  # noqa: E402

SAMPLE = ROOT / "samples/hotel-booking-agent"


@contextlib.contextmanager
def model():
    port = free_port()
    with tempfile.TemporaryDirectory(prefix="hotel-model-") as work, \
            started([sys.executable, str(SAMPLE / "scripts/model_smoke.py"), "--serve", str(port)],
                    SAMPLE, base_env(), Path(work) / "model.log") as process:
        url = f"http://127.0.0.1:{port}"
        wait_ready(url + "/health", process)
        yield url


def hotel_api(stack: contextlib.ExitStack, reports: Path) -> dict[str, str]:
    """Start the sample's hotel API, which the agent's tools call."""
    work = Path(stack.enter_context(tempfile.TemporaryDirectory(prefix="hotel-api-")))
    venv = make_venv(sys.executable, work / "venv", SAMPLE / "services/hotel_api/requirements.txt",
                     reports / "hotel-api-install.log")
    port = free_port()
    process = stack.enter_context(started(
        [str(venv / "bin/python"), "-m", "uvicorn", "service:app", "--app-dir", "services/hotel_api",
         "--host", "127.0.0.1", "--port", str(port)],
        SAMPLE, base_env(), reports / "hotel-api.log"))
    url = f"http://127.0.0.1:{port}"
    wait_ready(url + "/health", process)
    return {"HOTEL_API_BASE_URL": url}


HOTEL_BOOKING = ExternalSample(
    name_prefix="hotel-smoke-",
    app_dir=SAMPLE / "agent",
    requirements=SAMPLE / "agent/requirements.txt",
    command=["-m", "uvicorn", "app:app", "--host", "127.0.0.1", "--port", "{port}"],
    health="/openapi.json",
    model=model,
    env=lambda model_url, _port: {"OPENAI_API_KEY": "smoke-only", "OPENAI_BASE_URL": model_url + "/v1"},
    invoke=lambda endpoint, marker: invoke(endpoint, marker, None),
    check_evidence=check_evidence,
    services=hotel_api,
    marker_prefix="hotel-smoke-",
)

if __name__ == "__main__":
    run(parse_args(__doc__), HOTEL_BOOKING)
