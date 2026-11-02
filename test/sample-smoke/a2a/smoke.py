"""HTTP readiness and process cleanup helpers for A2A smoke tests."""
import json
import os
import signal
import subprocess
import time
import urllib.error
import urllib.request

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


def stop_process(process):
    if process.poll() is None:
        os.killpg(process.pid, signal.SIGTERM)
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=5)
