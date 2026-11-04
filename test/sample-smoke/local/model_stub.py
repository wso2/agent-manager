"""Local model, FAQ and OTLP endpoints; fail closed on unexpected tool output."""

import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

FAQ = "# Swiss policies\n## Refunds\nSmoke policy permits refunds within 24 hours.\n## Changes\nContact support for flight changes.\n## Baggage\nOne carry-on bag is permitted."
ANSWER = "smoke-ok: verified local tool result"
TOOLS = {
    "customer-support-agent": (
        "lookup_policy",
        {"query": "refund policy"},
        "Smoke policy permits refunds",
    ),
    "crewai-agent": (
        "country_capital_lookup",
        {"country": "France"},
        "The capital of France is Paris",
    ),
    "dotnet-agent": ("GetWeather", {"city": "Paris"}, "sunny with a high of 24"),
    "leave-assistant-agent": (
        "getLeaveBalance",
        {"employeeId": "E-3001"},
        "Priya Fernando",
    ),
}


class ModelServer(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, sample):
        super().__init__(("127.0.0.1", 0), Handler)
        self.sample = sample
        self.calls = 0
        self.verified = False
        self.embeddings = 0
        self.exports = 0
        self.errors = []
        self.lock = threading.Lock()

    def completion(self, body):
        self.calls += 1
        if self.calls > 8:
            raise ValueError("Model loop exceeded eight calls")
        messages = body["messages"]
        results = [m for m in messages if m["role"] == "tool"]
        if self.sample == "manual-instrumentation-agent":
            assert results, "Manual tool result missing"
            result = json.loads(results[-1]["content"])
            assert result["word_count"] > 0
            self.verified = True
        elif results:
            assert TOOLS[self.sample][2] in json.dumps(results, ensure_ascii=False), (
                results
            )
            self.verified = True
        elif self.sample == "crewai-agent" and self.calls > 1:
            # CrewAI's text/ReAct transport places the observation in a user turn.
            if TOOLS[self.sample][2] in json.dumps(messages):
                self.verified = True
        if self.verified:
            message = {
                "role": "assistant",
                "content": ("Final Answer: " if self.sample == "crewai-agent" else "")
                + ANSWER,
            }
            reason = "stop"
        else:
            name, args, _ = TOOLS[self.sample]
            advertised = {t["function"]["name"] for t in body.get("tools", [])}
            if self.sample == "crewai-agent" and not advertised:
                assert name in json.dumps(messages)
                message = {
                    "role": "assistant",
                    "content": 'Thought: Use the lookup.\nAction: country_capital_lookup\nAction Input: {"country":"France"}',
                }
                reason = "stop"
            else:
                assert name in advertised, advertised
                message = {
                    "role": "assistant",
                    "content": None,
                    "tool_calls": [
                        {
                            "id": "call_smoke",
                            "type": "function",
                            "function": {"name": name, "arguments": json.dumps(args)},
                        }
                    ],
                }
                reason = "tool_calls"
        return {
            "id": "chatcmpl-smoke",
            "object": "chat.completion",
            "created": 1,
            "model": body["model"],
            "choices": [{"index": 0, "message": message, "finish_reason": reason}],
            "usage": {"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20},
        }


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def reply(self, status, body):
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        if self.path == "/faq":
            data = FAQ.encode()
            self.send_response(200)
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
        else:
            self.reply(404, {"error": "Unexpected stub route"})

    def do_POST(self):
        raw = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        if self.path.endswith("/v1/traces"):
            self.server.exports += 1
            self.send_response(200)
            self.send_header("Content-Type", "application/x-protobuf")
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        try:
            body = json.loads(raw)
            if self.path == "/v1/embeddings":
                values = body["input"]
                if isinstance(values, str):
                    values = [values]
                self.server.embeddings += 1
                self.reply(
                    200,
                    {
                        "object": "list",
                        "model": body["model"],
                        "data": [
                            {
                                "object": "embedding",
                                "index": i,
                                "embedding": [1.0, 0.0, 0.25]
                                if "refund" in str(values[i]).lower()
                                else [0.0, 1.0, 0.25],
                            }
                            for i, _ in enumerate(values)
                        ],
                        "usage": {"prompt_tokens": 5, "total_tokens": 5},
                    },
                )
                return
            assert self.path == "/v1/chat/completions", self.path
            with self.server.lock:
                response = self.server.completion(body)
            if not body.get("stream"):
                self.reply(200, response)
                return
            choice = response["choices"][0]
            delta = choice["message"]
            for i, call in enumerate(delta.get("tool_calls", [])):
                call["index"] = i
            chunk = {k: response[k] for k in ("id", "created", "model")}
            chunk.update(
                object="chat.completion.chunk",
                choices=[{"index": 0, "delta": delta, "finish_reason": None}],
            )
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            self.wfile.write(("data: " + json.dumps(chunk) + "\n\n").encode())
            chunk["choices"] = [
                {"index": 0, "delta": {}, "finish_reason": choice["finish_reason"]}
            ]
            chunk["usage"] = response["usage"]
            self.wfile.write(
                ("data: " + json.dumps(chunk) + "\n\ndata: [DONE]\n\n").encode()
            )
            self.wfile.flush()
        except (
            AssertionError,
            ValueError,
            TypeError,
            KeyError,
            IndexError,
            OSError,
        ) as exc:
            self.server.errors.append(f"{type(exc).__name__}: {exc}")
            self.reply(
                400, {"error": {"message": str(exc), "type": "smoke_contract_error"}}
            )
