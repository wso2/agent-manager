"""HTTP, local model, and process helpers for Insurance Support smoke tests."""
from __future__ import annotations

import contextlib
import json
import os
from pathlib import Path
import signal
import subprocess
import threading
import time
import urllib.error
import urllib.request

from model_stub import ModelServer

ROOT = Path(__file__).resolve().parents[3]
SAMPLE = ROOT / "samples/insurance-support-agent/agent"


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


def invoke(endpoint, marker, headers=None):
    deadline = time.monotonic() + 180
    while True:
        try:
            result = request(endpoint.rstrip("/") + "/chat", {
                "session_id": marker,
                "message": f"List my policies using list_policies. Request ID {marker}",
            }, headers, timeout=60)
            break
        except urllib.error.HTTPError as exc:
            if not headers or exc.code not in (401, 403, 502, 503, 504) or time.monotonic() >= deadline:
                raise
            time.sleep(5)
    if result.get("session_id") != marker or result.get("response", "").strip() != f"{marker}: Policies verified: OZ-AUTO-4417, OZ-HOME-2280, OZ-TRAV-9153.":
        raise AssertionError(f"Unexpected /chat response: {result}")
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

