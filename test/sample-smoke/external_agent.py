"""Run a sample as an external agent against a running Agent Manager and find its trace.

The sample runs on this machine from the checkout, under `amp-instrument` from
libs/amp-instrumentation (so the PR's instrumentation is what gets exercised),
against a local model stub. Agent Manager registers it as an external agent and
issues the token its spans are sent with to the AI Gateway's /otel endpoint;
the run passes once this invocation's model span is readable from the observer.

Each sample's platform_smoke.py describes how to start it (an ExternalSample)
and calls run().
"""
from __future__ import annotations

import argparse
import contextlib
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import tempfile
import time
from typing import Callable, ContextManager
import urllib.error
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "test/instrumentation-matrix"))
from heavy.amp_client import AmpClient, DeployedAgent, IdpCredentials  # noqa: E402
from harness.classify import classify_span  # noqa: E402

INSTRUMENTATION = ROOT / "libs/amp-instrumentation"


@dataclass
class ExternalSample:
    """How to install, start and drive one sample on this machine."""

    # Project and agent name prefix; with the 12-character suffix it must stay
    # within Agent Manager's 25-character project name limit.
    name_prefix: str
    app_dir: Path
    requirements: Path
    # Arguments after the venv's python; "{port}" is replaced with the app port.
    command: list[str]
    # GET path that returns JSON once the app is ready.
    health: str
    # Context manager yielding the model stub's base URL (no /v1).
    model: Callable[[], ContextManager[str]]
    # Sample environment for a given model URL and app port.
    env: Callable[[str, int], dict[str, str]]
    invoke: Callable[[str, str], object]
    check_evidence: Callable[[dict, str], None]
    # Optional extra services; given an ExitStack and the reports dir, returns
    # environment entries the app needs to reach them.
    services: Callable[[contextlib.ExitStack, Path], dict[str, str]] | None = None
    extra_env: dict[str, str] = field(default_factory=dict)
    # Prefix of the per-invocation marker the model stub looks for.
    marker_prefix: str = "smoke-"


def request(url, body=None, headers=None, timeout=30):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json", **(headers or {})})
    with urllib.request.urlopen(req, timeout=timeout) as response:
        return json.load(response)


def wait_ready(url, process=None, timeout=120):
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        if process is not None and process.poll() is not None:
            raise RuntimeError(f"Process exited before readiness (exit={process.returncode})")
        try:
            return request(url, timeout=3)
        except urllib.error.HTTPError as exc:
            if exc.code not in (404, 502, 503, 504):
                raise
            last = str(exc)
        except (OSError, ValueError) as exc:
            last = str(exc)
        time.sleep(1)
    raise TimeoutError(f"Readiness timed out for {url}: {last}")


def wait_until(description, poll, timeout=180):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        result = poll()
        if result:
            return result
        time.sleep(5)
    raise TimeoutError(f"Timed out waiting for {description}")


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def stop_process(process):
    if process.poll() is None:
        os.killpg(process.pid, signal.SIGTERM)
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=5)


@contextlib.contextmanager
def started(command, cwd, env, log_path):
    """Start a process in its own session and stop it on exit."""
    with log_path.open("w") as log:
        process = subprocess.Popen(command, cwd=cwd, env=env, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
        try:
            yield process
        finally:
            stop_process(process)


def make_venv(python, work, requirements, log_path, *extra):
    venv = Path(work)
    subprocess.run([python, "-m", "venv", str(venv)], check=True)
    executable = str(venv / "bin/python")
    with log_path.open("a") as log:
        subprocess.run([executable, "-m", "pip", "install", "--disable-pip-version-check", "-r", str(requirements), *extra],
                       stdout=log, stderr=subprocess.STDOUT, check=True)
        subprocess.run([executable, "-m", "pip", "check"], stdout=log, stderr=subprocess.STDOUT, check=True)
    return venv


def base_env():
    # Only what a process needs to run; never the caller's model or telemetry settings.
    return {k: os.environ[k] for k in ("PATH", "HOME", "LANG", "SYSTEMROOT") if k in os.environ}


def trace_matches(spans, marker):
    # A populated trace list is insufficient: require this invocation's model span.
    return any(classify_span(span) == "llm" and marker in json.dumps(span) for span in spans)


def poll_trace(client, deployed, observer, marker, since, reports):
    params = {"organization": deployed.org, "project": deployed.project_name,
              "agent": deployed.agent_name, "environment": deployed.environment,
              "startTime": (since - timedelta(seconds=5)).isoformat(),
              "endTime": (since + timedelta(minutes=10)).isoformat(), "limit": 100}

    def get(path, query=None):
        response = client._session.get(observer.rstrip("/") + path, params=query,
                                       headers={"Authorization": f"Bearer {client.access_token()}"}, timeout=30)
        response.raise_for_status()
        return response.json()

    def poll():
        listing = get("/api/v1/traces", params)
        (reports / "trace-list.json").write_text(json.dumps(listing, indent=2) + "\n")
        for trace in listing.get("traces", []):
            tid = trace["traceId"]
            summaries = get(f"/api/v1/traces/{tid}/spans", params)
            spans = [get(f"/api/v1/traces/{tid}/spans/{span['spanId']}") for span in summaries.get("spans", [])]
            if trace_matches(spans, marker):
                (reports / "trace-spans.json").write_text(json.dumps(spans, indent=2) + "\n")
                return tid
        return None

    return wait_until("this invocation's model span in the observer", poll)


def run(args, sample: ExternalSample):
    reports = args.reports.resolve()
    reports.mkdir(parents=True, exist_ok=True)
    name = sample.name_prefix + uuid.uuid4().hex[:12]
    if len(name) > 25:
        raise ValueError(f"name prefix {sample.name_prefix!r} leaves {name!r} over 25 characters")
    marker = sample.marker_prefix + uuid.uuid4().hex
    client = AmpClient(args.api, IdpCredentials(args.token_url, args.client_id, args.client_secret),
                       org=args.org, environment=args.environment)
    deployed = DeployedAgent(args.org, name, name, args.environment, "", "")
    registered = False
    phase = "dependency installation"
    try:
        with tempfile.TemporaryDirectory(prefix=sample.name_prefix) as work, contextlib.ExitStack() as stack:
            print(f"==> {phase}", flush=True)
            venv = make_venv(args.python, Path(work) / "venv", sample.requirements, reports / "install.log",
                             str(INSTRUMENTATION))

            phase = "external agent registration"
            print(f"==> {phase}", flush=True)
            registered = True
            token = client.register_external_agent(args.org, name)

            phase = "model stub and services"
            print(f"==> {phase}", flush=True)
            model_url = stack.enter_context(sample.model())
            service_env = sample.services(stack, reports) if sample.services else {}

            phase = "application startup under amp-instrument"
            print(f"==> {phase}", flush=True)
            port = free_port()
            env = base_env()
            env.update(sample.extra_env)
            env.update(service_env)
            env.update(sample.env(model_url, port))
            env.update(AMP_OTEL_ENDPOINT=args.gateway.rstrip("/") + "/otel", AMP_AGENT_API_KEY=token,
                       AMP_TRACE_CONTENT="true", OTEL_BSP_SCHEDULE_DELAY="100", PYTHONUNBUFFERED="1")
            command = [str(venv / "bin/amp-instrument"), str(venv / "bin/python"),
                       *[part.replace("{port}", str(port)) for part in sample.command]]
            app = stack.enter_context(started(command, sample.app_dir, env, reports / "application.log"))
            endpoint = f"http://127.0.0.1:{port}"
            wait_ready(endpoint + sample.health, app)

            phase = "model round trip"
            print(f"==> {phase}", flush=True)
            since = datetime.now(timezone.utc)
            result = sample.invoke(endpoint, marker)
            evidence = request(model_url + "/evidence")
            (reports / "model-evidence.json").write_text(json.dumps(evidence, indent=2) + "\n")
            sample.check_evidence(evidence, marker)

            phase = "trace ingestion"
            print(f"==> {phase}", flush=True)
            trace_id = poll_trace(client, deployed, args.observer, marker, since, reports)
            (reports / "result.json").write_text(json.dumps({"traceId": trace_id, "response": result}, indent=2, default=str) + "\n")
            print(f"PASS: local app under amp-instrument → model round trip → AI Gateway → trace {trace_id}")
    except BaseException:
        print(f"FAIL: {phase}; reports: {reports}", file=sys.stderr)
        for log in ("install.log", "application.log"):
            path = reports / log
            if path.exists():
                print(f"--- {log} (last 60 lines) ---\n" + "\n".join(path.read_text().splitlines()[-60:]), file=sys.stderr)
        raise
    finally:
        if registered:
            client.teardown_agent(deployed)


def parse_args(description):
    parser = argparse.ArgumentParser(description=description)
    parser.add_argument("--python", default=sys.executable)
    parser.add_argument("--reports", type=Path, default=Path(tempfile.mkdtemp(prefix="external-smoke-")))
    for name, env, default in [("api", "AMP_API_BASE_URL", "http://localhost:9000"),
                               ("observer", "AM_OBSERVER_BASE_URL", "http://localhost:9098"),
                               ("gateway", "AMP_GATEWAY_URL", "http://default-default.gateway.localhost:19080"),
                               ("token-url", "IDP_TOKEN_URL", "http://thunder.amp.localhost:8080/oauth2/token"),
                               ("client-id", "IDP_CLIENT_ID", "amp-api-client"),
                               ("org", "DEFAULT_ORG", "default"), ("environment", "DEFAULT_ENV", "default")]:
        parser.add_argument("--" + name, default=os.getenv(env, default))
    args = parser.parse_args()
    args.client_secret = os.environ.get("IDP_CLIENT_SECRET", "")
    if not args.client_secret:
        parser.error("Set IDP_CLIENT_SECRET to the Agent Manager API client secret")
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(143))
    return args
