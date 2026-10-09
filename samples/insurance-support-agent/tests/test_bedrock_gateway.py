import json, threading, os
from http.server import BaseHTTPRequestHandler, HTTPServer
from unittest.mock import patch
from config import Config
from agent import build_agent

calls = []


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        calls.append((self.path, dict(self.headers), body))
        if len(calls) == 1:
            content = [
                {
                    "toolUse": {
                        "toolUseId": "lookup-1",
                        "name": "list_policies",
                        "input": {},
                    }
                }
            ]
            reason = "tool_use"
        else:
            assert any(
                "toolResult" in c for m in body["messages"] for c in m["content"]
            )
            content = [
                {
                    "text": "Your policies are OZ-AUTO-4417, OZ-HOME-2280, and OZ-TRAV-9153."
                }
            ]
            reason = "end_turn"
        payload = {
            "output": {"message": {"role": "assistant", "content": content}},
            "stopReason": reason,
            "usage": {"inputTokens": 10, "outputTokens": 10, "totalTokens": 20},
            "metrics": {"latencyMs": 1},
        }
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(payload).encode())


server = HTTPServer(("127.0.0.1", 0), Handler)
threading.Thread(target=server.serve_forever, daemon=True).start()
env = {
    "MODEL_PROVIDER": "bedrock-gateway",
    "LLM_GATEWAY_URL": f"http://127.0.0.1:{server.server_port}/insurance-bedrock",
    "LLM_GATEWAY_API_KEY": "test-gateway-key",
    "BEDROCK_MODEL_ID": "amazon.nova-micro-v1:0",
    "AWS_BEARER_TOKEN_BEDROCK": "ambient-must-not-leak",
}
with patch.dict(os.environ, env, clear=True):
    cfg = Config.from_env()
    assert not cfg.openai_api_key
    result = str(build_agent(cfg)("List my policies."))
    assert "OZ-AUTO-4417" in result and len(calls) == 2
    for path, headers, body in calls:
        headers = {k.lower(): v for k, v in headers.items()}
        assert path == "/insurance-bedrock/model/amazon.nova-micro-v1%3A0/converse", (
            path
        )
        assert headers["api-key"] == "test-gateway-key"
        assert "authorization" not in headers and "x-amz-security-token" not in headers
        assert body["inferenceConfig"]["maxTokens"] == 600
with patch.dict(os.environ, {"OPENAI_API_KEY": "test-openai"}, clear=True):
    assert Config.from_env().model_provider == "openai"
with patch.dict(os.environ, {"MODEL_PROVIDER": "bedrock-gateway"}, clear=True):
    try:
        Config.from_env()
        raise AssertionError("missing gateway accepted")
    except RuntimeError:
        pass
server.shutdown()
print(
    "PASS: real Strands tool round-trip over mock Converse HTTP, gateway auth, ambient credential exclusion, OpenAI default, missing config rejection"
)
