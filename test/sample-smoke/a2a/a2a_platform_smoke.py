"""Run the A2A sample as an external agent and find its model trace in Agent Manager."""
from __future__ import annotations

import contextlib
from pathlib import Path
import sys
import threading

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from external_agent import ExternalSample, parse_args, run  # noqa: E402
from a2a_model_stub import ModelServer  # noqa: E402
from a2a_smoke import SAMPLE, check_evidence, invoke  # noqa: E402


@contextlib.contextmanager
def model():
    with ModelServer(("127.0.0.1", 0)) as server:
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            yield f"http://127.0.0.1:{server.server_port}"
        finally:
            server.shutdown()
            thread.join(timeout=5)


A2A = ExternalSample(
    name_prefix="a2a-smoke-",
    app_dir=SAMPLE,
    requirements=SAMPLE / "requirements.txt",
    command=["-m", "uvicorn", "app:app", "--host", "127.0.0.1", "--port", "{port}"],
    health="/health",
    model=model,
    env=lambda model_url, port: {
        "OPENAI_API_KEY": "smoke-only", "OPENAI_BASE_URL": model_url + "/v1",
        "AGENT_PUBLIC_BASE_URL": f"http://127.0.0.1:{port}",
    },
    invoke=lambda endpoint, marker: invoke(endpoint, marker),
    check_evidence=check_evidence,
)

if __name__ == "__main__":
    run(parse_args(__doc__), A2A)
