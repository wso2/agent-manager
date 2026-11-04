"""Install/build and exercise one real sample against localhost-only API fixtures."""

import argparse
import json
import os
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

from model_stub import ANSWER, ModelServer

ROOT = Path(__file__).resolve().parents[3]
SAMPLES = (
    "crewai-agent",
    "customer-support-agent",
    "dotnet-agent",
    "leave-assistant-agent",
    "manual-instrumentation-agent",
)


def stop(process):
    if process is not None and process.poll() is None:
        os.killpg(process.pid, signal.SIGTERM)
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=5)


def command(args, cwd, env, log, timeout=600):
    with log.open("w") as output:
        process = subprocess.Popen(
            args,
            cwd=cwd,
            env=env,
            stdout=output,
            stderr=subprocess.STDOUT,
            start_new_session=True,
        )
        try:
            code = process.wait(timeout=timeout)
            if code:
                raise RuntimeError(f"{args[0]} exited {code}; see {log}")
        finally:
            stop(process)


def run(sample, reports):
    reports.mkdir(parents=True, exist_ok=True)
    phase = "installation/build"
    process = None
    model = ModelServer(sample)
    thread = threading.Thread(target=model.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix=sample + "-smoke-") as work:
            work = Path(work)
            source = work / "sample"
            shutil.copytree(
                ROOT / "samples" / sample,
                source,
                ignore=shutil.ignore_patterns(
                    ".env",
                    ".env.*",
                    "Config.toml",
                    "__pycache__",
                    "target",
                    "bin",
                    "obj",
                    ".venv",
                ),
            )
            home = work / "home"
            home.mkdir()
            env = {
                k: v
                for k, v in os.environ.items()
                if k
                in (
                    "PATH",
                    "SYSTEMROOT",
                    "JAVA_HOME",
                    "DOTNET_ROOT",
                    "BALLERINA_HOME",
                    "TMPDIR",
                    "TEMP",
                    "TMP",
                )
            }
            # Reuse language package caches, but never inherit model/provider credentials.
            env["HOME"] = os.environ["HOME"]
            base = f"http://127.0.0.1:{model.server_port}"
            env.update(
                OPENAI_API_KEY="smoke-fixture",
                OPENAI_BASE_URL=base + "/v1",
                OPENAI_API_BASE=base + "/v1",
                TAVILY_API_KEY="smoke-fixture",
                POLICY_FAQ_URL=base + "/faq",
                PYTHON_DOTENV_DISABLED="1",
                AMP_OTEL_ENDPOINT=base + "/otel",
                AMP_AGENT_API_KEY="smoke-fixture",
                AMP_TRACE_CONTENT="true",
                OTEL_BSP_SCHEDULE_DELAY="100",
                CREWAI_TRACING_ENABLED="false",
                CREWAI_DISABLE_TRACING_PROMPT="true",
                CREWAI_TELEMETRY_OPT_OUT="true",
                OTEL_SDK_DISABLED="true" if sample == "crewai-agent" else "false",
                LITELLM_LOCAL_MODEL_COST_MAP="True",
                CREWAI_STORAGE_DIR=str(home / "crewai"),
                DATABASE_URL="postgresql://smoke:smoke@127.0.0.1:1/smoke",
                PYTHONUNBUFFERED="1",
            )
            with socket.socket() as sock:
                sock.bind(("127.0.0.1", 0))
                port = sock.getsockname()[1]
            if sample == "dotnet-agent":
                port = 8000
                command(["dotnet", "restore"], source, env, reports / "restore.log")
                command(
                    ["dotnet", "build", "--no-restore", "-c", "Release"],
                    source,
                    env,
                    reports / "build.log",
                )
                start = ["dotnet", str(source / "bin/Release/net10.0/dotnet-agent.dll")]
                health = "/healthz"
            elif sample == "leave-assistant-agent":
                port = 8000
                (source / "Config.toml").write_text(
                    f'openAiApiKey = "smoke-fixture"\nopenAiServiceUrl = "{base}/v1"\n'
                )
                command(
                    ["bal", "build", "--sticky"],
                    source,
                    env,
                    reports / "build.log",
                    timeout=900,
                )
                start = ["bal", "run", str(source / "target/bin/leave_assistant.jar")]
                health = "/chat"
            else:
                command(
                    [sys.executable, "-m", "venv", str(work / "venv")],
                    source,
                    env,
                    reports / "venv.log",
                )
                python = str(work / "venv/bin/python")
                command(
                    [python, "-m", "pip", "install", "-r", "requirements.txt"],
                    source,
                    env,
                    reports / "install.log",
                    timeout=900,
                )
                command(
                    [python, "-m", "pip", "check"],
                    source,
                    env,
                    reports / "dependency-check.log",
                )
                start = [
                    python,
                    "-m",
                    "uvicorn",
                    "app:app",
                    "--host",
                    "127.0.0.1",
                    "--port",
                    str(port),
                ]
                health = (
                    "/health"
                    if sample == "manual-instrumentation-agent"
                    else "/openapi.json"
                )
            if sample in ("dotnet-agent", "leave-assistant-agent"):
                with socket.socket() as check_port:
                    check_port.bind(("127.0.0.1", port))
            if sample != "leave-assistant-agent":
                env["HOME"] = str(home)
            phase = "startup/health"
            print(f"==> {sample}: {phase}", flush=True)
            with (reports / "app.log").open("w") as output:
                process = subprocess.Popen(
                    start,
                    cwd=source,
                    env=env,
                    stdout=output,
                    stderr=subprocess.STDOUT,
                    start_new_session=True,
                )
                url = f"http://127.0.0.1:{port}"
                deadline = time.monotonic() + 120
                while True:
                    if process.poll() is not None:
                        raise RuntimeError(f"Application exited {process.returncode}")
                    try:
                        with urlopen(url + health, timeout=3):
                            pass
                        break
                    except HTTPError as exc:
                        if sample == "leave-assistant-agent" and exc.code == 405:
                            break
                        raise
                    except (URLError, TimeoutError):
                        if time.monotonic() > deadline:
                            raise TimeoutError("Application readiness timed out")
                        time.sleep(0.5)
                phase = "HTTP model/tool round trip"
                prompt = {
                    "crewai-agent": "What is the capital of France?",
                    "customer-support-agent": "Look up the refund policy.",
                    "dotnet-agent": "What is the weather in Paris?",
                    "leave-assistant-agent": "Check leave balance for E-3001.",
                    "manual-instrumentation-agent": "What is WSO2 Agent Manager?",
                }[sample]
                data = json.dumps(
                    {"session_id": "smoke-session", "message": prompt}
                ).encode()
                with urlopen(
                    Request(
                        url + "/chat",
                        data=data,
                        headers={"Content-Type": "application/json"},
                    ),
                    timeout=120,
                ) as response:
                    result = json.load(response)
                assert ANSWER in result["response"], result
                assert model.verified and not model.errors, model.errors
                if sample in ("customer-support-agent", "manual-instrumentation-agent"):
                    assert model.embeddings >= 2
                (reports / "response.json").write_text(
                    json.dumps(result, indent=2) + "\n"
                )
                print(
                    f"PASS: {sample} install/build → readiness → HTTP → model → real tool → response"
                )
                stop(process)
    except BaseException:
        print(f"FAIL: {sample}: {phase}; reports: {reports}", file=sys.stderr)
        for log in reports.glob("*.log"):
            print(
                f"--- {log.name} ---\n"
                + "\n".join(log.read_text(errors="replace").splitlines()[-45:]),
                file=sys.stderr,
            )
        raise
    finally:
        stop(process)
        model.shutdown()
        model.server_close()
        thread.join(timeout=5)
        (reports / "model-evidence.json").write_text(
            json.dumps(
                {
                    "calls": model.calls,
                    "verified": model.verified,
                    "embeddings": model.embeddings,
                    "exports": model.exports,
                    "errors": model.errors,
                },
                indent=2,
            )
            + "\n"
        )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("sample", choices=SAMPLES)
    parser.add_argument("--reports", type=Path, required=True)
    args = parser.parse_args()
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(143))
    run(args.sample, args.reports.resolve())
