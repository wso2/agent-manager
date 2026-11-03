"""Install and exercise the real IT Helpdesk HTTP application without paid APIs."""
from __future__ import annotations

import argparse
import contextlib
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.request
import uuid

from model_stub import ModelServer

ROOT = Path(__file__).resolve().parents[3]
SAMPLE = ROOT / "samples/it-helpdesk-agent"


def request(url, body=None, headers=None, timeout=30):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json", **(headers or {})})
    with urllib.request.urlopen(req, timeout=timeout) as response:
        return json.load(response)


def wait_ready(url, process=None, timeout=90, headers=None):
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        if process is not None and process.poll() is not None:
            raise RuntimeError(f"Application exited before readiness (exit={process.returncode})")
        try:
            return request(url, headers=headers, timeout=3)
        except urllib.error.HTTPError as exc:
            # Minted gateway keys can take a few seconds to propagate.
            if exc.code not in (404, 502, 503, 504) and not (headers and exc.code in (401, 403)):
                raise
            last = str(exc)
        except (OSError, ValueError) as exc:
            last = str(exc)
        time.sleep(1)
    raise TimeoutError(f"Readiness timed out for {url}: {last}")


def invoke(endpoint, marker, headers=None, version=None):
    deadline = time.monotonic() + 180
    while True:
        try:
            result = request(endpoint.rstrip("/") + "/chat", {
                "session_id": marker,
                "message": f"Check VPN status using check_system_status. Request ID {marker}",
            }, headers, timeout=60)
            break
        except urllib.error.HTTPError as exc:
            if not headers or exc.code not in (401, 403, 502, 503, 504) or time.monotonic() >= deadline:
                raise
            time.sleep(5)
    if result.get("session_id") != marker or result.get("response") != f"{marker}: VPN Gateway is operational.":
        raise AssertionError(f"Unexpected /chat response: {result}")
    if version is not None and result.get("agent_version") != version:
        raise AssertionError(f"Wrong deployed version: {result.get('agent_version')}")
    return result


def check_evidence(evidence, marker):
    if evidence["errors"] or marker not in evidence["completed"]:
        raise AssertionError(f"Model/tool round trip was not verified: {evidence}")


@contextlib.contextmanager
def local_model():
    with ModelServer(("127.0.0.1", 0)) as server:
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            yield server, f"http://127.0.0.1:{server.server_port}"
        finally:
            server.shutdown()
            thread.join(timeout=5)


def stop_process(process):
    if process.poll() is None:
        os.killpg(process.pid, signal.SIGTERM)
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=5)


def local_smoke(python, reports):
    reports.mkdir(parents=True, exist_ok=True)
    phase = "dependency installation"
    print(f"==> {phase}", flush=True)
    try:
        with tempfile.TemporaryDirectory(prefix="it-helpdesk-smoke-") as work:
            venv = Path(work) / "venv"
            subprocess.run([python, "-m", "venv", str(venv)], check=True)
            executable = str(venv / "bin/python")
            with (reports / "install.log").open("w") as log:
                subprocess.run([executable, "-m", "pip", "install", "--disable-pip-version-check", "-r", str(SAMPLE / "requirements.txt")], stdout=log, stderr=subprocess.STDOUT, check=True)
                subprocess.run([executable, "-m", "pip", "check"], stdout=log, stderr=subprocess.STDOUT, check=True)
            phase = "application startup"
            print(f"==> {phase}", flush=True)
            with socket.socket() as sock:
                sock.bind(("127.0.0.1", 0))
                port = sock.getsockname()[1]
            marker = "smoke-" + uuid.uuid4().hex
            with local_model() as (_, model_url), (reports / "application.log").open("w") as log:
                env = {k: v for k, v in os.environ.items() if not k.startswith(("OPENAI_", "AMP_", "OTEL_", "LANGCHAIN_", "LANGSMITH_", "GITHUB_MCP_"))}
                env.pop("PYTHONPATH", None)
                env.pop("PYTHONHOME", None)
                env.update(LANGCHAIN_TRACING_V2="false", LANGSMITH_TRACING="false", USE_LLM_PROVIDER="true", LLM_PROVIDER_URL=model_url + "/v1", LLM_PROVIDER_KEY="smoke-only", USE_MCP="false", AGENT_VERSION="local-smoke")
                process = subprocess.Popen([executable, "-m", "uvicorn", "app:app", "--host", "127.0.0.1", "--port", str(port)], cwd=SAMPLE, env=env, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
                try:
                    endpoint = f"http://127.0.0.1:{port}"
                    health = wait_ready(endpoint + "/health", process)
                    if health.get("status") != "ok" or health.get("mcp_enabled") is not False:
                        raise AssertionError(f"Unexpected health: {health}")
                    phase = "HTTP model/tool round trip"
                    print(f"==> {phase}", flush=True)
                    result = invoke(endpoint, marker, version="local-smoke")
                    evidence = request(model_url + "/evidence")
                    check_evidence(evidence, marker)
                    (reports / "result.json").write_text(json.dumps({"health": health, "response": result, "model": evidence}, indent=2) + "\n")
                finally:
                    try:
                        (reports / "model-evidence.json").write_text(json.dumps(request(model_url + "/evidence"), indent=2) + "\n")
                    finally:
                        stop_process(process)
    except BaseException:
        print(f"FAIL: {phase}; logs: {reports}", file=sys.stderr)
        for name in ("install.log", "application.log"):
            path = reports / name
            if path.exists():
                print(f"--- {name} (last 60 lines) ---\n" + "\n".join(path.read_text().splitlines()[-60:]), file=sys.stderr)
        raise
    print(f"PASS: install, startup, health, HTTP → model → real tool → model → response; reports: {reports}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--python", default=sys.executable)
    parser.add_argument("--reports", type=Path, default=Path(tempfile.mkdtemp(prefix="it-helpdesk-reports-")))
    args = parser.parse_args()
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(143))
    local_smoke(args.python, args.reports.resolve())
