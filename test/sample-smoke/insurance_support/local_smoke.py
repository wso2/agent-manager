"""Validate the platform's OpenAI streaming stub against the real local agent."""
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import uuid

from support import SAMPLE, check_evidence, invoke, local_model, request, stop_process, wait_ready


def run():
    reports = Path(os.environ["SMOKE_REPORTS"])
    marker = "smoke-" + uuid.uuid4().hex
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    with local_model() as (_, model), (reports / "openai-agent.log").open("w") as log:
        env = {k: os.environ[k] for k in ("PATH", "HOME", "SYSTEMROOT") if k in os.environ}
        env.update(MODEL_PROVIDER="openai", OPENAI_BASE_URL=model + "/v1", OPENAI_API_KEY="smoke-only",
                   OPENAI_MODEL="smoke-model", USE_MCP="false", PORT=str(port), PYTHON_DOTENV_DISABLED="1",
                   OTEL_SDK_DISABLED="true", PYTHONUNBUFFERED="1")
        process = subprocess.Popen([sys.executable, "main.py"], cwd=SAMPLE, env=env,
                                   stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
        try:
            endpoint = f"http://127.0.0.1:{port}"
            assert wait_ready(endpoint + "/health", process)["status"] == "ok"
            invoke(endpoint, marker)
            check_evidence(request(model + "/evidence"), marker)
            print("PASS: real Strands HTTP agent → OpenAI streaming stub → policy tool → response")
        except BaseException:
            print((reports / "openai-agent.log").read_text(), file=sys.stderr)
            raise
        finally:
            stop_process(process)


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(143))
    run()
