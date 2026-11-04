"""Run the real HTTP agent against a local Bedrock Converse tool-loop stub."""
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.request import Request, urlopen

ROOT = Path(__file__).resolve().parents[1]
EXPECTED = {"OZ-AUTO-4417", "OZ-HOME-2280", "OZ-TRAV-9153"}


def run():
    evidence = {"calls": 0, "tool_verified": False, "errors": []}

    class Model(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def do_POST(self):
            try:
                assert self.path.endswith("/model/smoke-model/converse"), self.path
                assert self.headers.get("api-key") == "smoke-fixture"
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                evidence["calls"] += 1
                if evidence["calls"] == 1:
                    assert any(t.get("toolSpec", {}).get("name") == "list_policies" for t in body["toolConfig"]["tools"])
                    content = [{"toolUse": {"toolUseId": "smoke-policy", "name": "list_policies", "input": {}}}]
                    reason = "tool_use"
                else:
                    assert evidence["calls"] == 2, "Unexpected extra model call"
                    results = [c["toolResult"] for m in body["messages"] for c in m["content"] if "toolResult" in c]
                    result = next(r for r in results if r["toolUseId"] == "smoke-policy")
                    assert result.get("status", "success") == "success"
                    value = result["content"][0]
                    data = value.get("json") or json.loads(value["text"])
                    assert {p["policy_number"] for p in data["policies"]} == EXPECTED
                    evidence["tool_verified"] = True
                    content = [{"text": "Verified policies: " + ", ".join(sorted(EXPECTED))}]
                    reason = "end_turn"
                payload = {"output": {"message": {"role": "assistant", "content": content}}, "stopReason": reason,
                           "usage": {"inputTokens": 10, "outputTokens": 10, "totalTokens": 20}, "metrics": {"latencyMs": 1}}
                self.send_response(200)
            except Exception as exc:
                evidence["errors"].append(str(exc))
                payload = {"message": str(exc)}
                self.send_response(500)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(payload).encode())

    def request(url, body=None):
        data = json.dumps(body).encode() if body is not None else None
        with urlopen(Request(url, data=data, headers={"Content-Type": "application/json"}), timeout=30) as response:
            return json.load(response)

    reports = Path(os.environ["SMOKE_REPORTS"])
    reports.mkdir(parents=True, exist_ok=True)
    model = ThreadingHTTPServer(("127.0.0.1", 0), Model)
    thread = threading.Thread(target=model.serve_forever, daemon=True)
    thread.start()
    with socket.socket() as port_socket:
        port_socket.bind(("127.0.0.1", 0))
        port = port_socket.getsockname()[1]
    # Only explicit local configuration reaches the app; dotenv is disabled too.
    env = {k: os.environ[k] for k in ("PATH", "HOME", "SYSTEMROOT") if k in os.environ}
    env.update({"PYTHON_DOTENV_DISABLED": "1", "MODEL_PROVIDER": "bedrock-gateway", "USE_MCP": "false",
                "LLM_GATEWAY_URL": f"http://127.0.0.1:{model.server_port}", "LLM_GATEWAY_API_KEY": "smoke-fixture",
                "BEDROCK_MODEL_ID": "smoke-model", "PORT": str(port), "PYTHONUNBUFFERED": "1", "OTEL_SDK_DISABLED": "true"})
    process = None
    try:
        with (reports / "agent.log").open("w") as log:
            process = subprocess.Popen([sys.executable, "main.py"], cwd=ROOT / "agent", env=env,
                                       stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
            url = f"http://127.0.0.1:{port}"
            deadline = time.monotonic() + 60
            while True:
                if process.poll() is not None:
                    raise RuntimeError(f"Agent exited with {process.returncode}")
                try:
                    health = request(url + "/health")
                    break
                except OSError:
                    if time.monotonic() >= deadline:
                        raise TimeoutError("Agent did not become healthy within 60 seconds")
                    time.sleep(0.2)
            assert health["status"] == "ok", health
            result = request(url + "/chat", {"message": "List my policies", "session_id": "smoke-http"})
            assert result["session_id"] == "smoke-http", result
            assert all(policy in result["response"] for policy in EXPECTED), result
            assert evidence["calls"] == 2 and evidence["tool_verified"] and not evidence["errors"], evidence
            (reports / "result.json").write_text(json.dumps(result, indent=2) + "\n")
            print("PASS: startup → health → HTTP chat → real policy tool → model response")
    except BaseException:
        print((reports / "agent.log").read_text(), file=sys.stderr)
        raise
    finally:
        if process is not None and process.poll() is None:
            os.killpg(process.pid, signal.SIGTERM)
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait(timeout=5)
        model.shutdown()
        model.server_close()
        thread.join(timeout=5)
        (reports / "model-evidence.json").write_text(json.dumps(evidence, indent=2) + "\n")


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(143))
    run()
