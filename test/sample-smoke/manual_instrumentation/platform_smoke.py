"""Run the manual instrumentation sample as an external agent and find its trace in Agent Manager.

The sample exports its own spans through init_otel(), so it runs without the
amp-instrument wrapper.
"""
from __future__ import annotations

import contextlib
import json
from pathlib import Path
import sys
import threading

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE.parent))
sys.path.insert(0, str(HERE.parent / "local"))
from external_agent import ROOT, ExternalSample, parse_args, request, run  # noqa: E402
from model_stub import ANSWER, Handler, ModelServer  # noqa: E402

SAMPLE = ROOT / "samples/manual-instrumentation-agent"


class EvidenceServer(ModelServer):
    """The local smoke's model stub, also recording requests for /evidence."""

    def __init__(self):
        super().__init__("manual-instrumentation-agent")
        self.RequestHandlerClass = EvidenceHandler
        self.requests = []

    def completion(self, body):
        self.requests.append(json.dumps(body))
        return super().completion(body)


class EvidenceHandler(Handler):
    def do_GET(self):
        if self.path != "/evidence":
            return super().do_GET()
        server = self.server
        with server.lock:
            self.reply(200, {"calls": server.calls, "verified": server.verified, "embeddings": server.embeddings,
                             "errors": server.errors, "requests": server.requests})


@contextlib.contextmanager
def model():
    server = EvidenceServer()
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield f"http://127.0.0.1:{server.server_port}"
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


def invoke(endpoint, marker):
    result = request(endpoint + "/chat", {"session_id": marker, "message": f"What is WSO2 Agent Manager? ({marker})"},
                     timeout=120)
    assert ANSWER in result["response"], result
    return result


def check_evidence(evidence, marker):
    assert not evidence["errors"], evidence["errors"]
    assert evidence["verified"], "the model never saw the word_count tool result"
    assert evidence["embeddings"] >= 1, "the knowledge base was not embedded through the stub"
    assert any(marker in body for body in evidence["requests"]), "no model request carried this invocation's marker"


MANUAL = ExternalSample(
    name_prefix="manual-smoke-",
    app_dir=SAMPLE,
    requirements=SAMPLE / "requirements.txt",
    command=["-m", "uvicorn", "app:app", "--host", "127.0.0.1", "--port", "{port}"],
    health="/health",
    model=model,
    env=lambda model_url, _port: {"OPENAI_API_KEY": "smoke-only", "OPENAI_BASE_URL": model_url + "/v1"},
    invoke=invoke,
    check_evidence=check_evidence,
    auto_instrument=False,
)

if __name__ == "__main__":
    run(parse_args(__doc__), MANUAL)
