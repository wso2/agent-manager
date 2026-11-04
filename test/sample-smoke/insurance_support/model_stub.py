"""Strict, offline OpenAI chat-completions server for the Insurance Support smoke test."""
from __future__ import annotations

import argparse
import json
import re
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

TOOL = "list_policies"
EXPECTED = {"OZ-AUTO-4417", "OZ-HOME-2280", "OZ-TRAV-9153"}


class ModelServer(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, address):
        super().__init__(address, Handler)
        self.lock = threading.Lock()
        self.completed = set()
        self.errors = []

    def completion(self, body):
        messages = body["messages"]
        user = next(m["content"] for m in reversed(messages) if m["role"] == "user")
        if isinstance(user, list):
            user = " ".join(c.get("text", "") for c in user)
        marker = re.search(r"smoke-[a-f0-9]{32}", user).group()
        call_id = "call_" + marker
        tool_results = [m for m in messages if m["role"] == "tool" and m.get("tool_call_id") == call_id]
        if tool_results:
            content = tool_results[-1]["content"]
            if isinstance(content, list):
                content = "".join(c.get("text", "") for c in content)
            policies = json.loads(content)["policies"]
            if {p["policy_number"] for p in policies} != EXPECTED:
                raise ValueError("The real policy tool did not return the expected policy fixtures")
            with self.lock:
                self.completed.add(marker)
            message = {"role": "assistant", "content": f"{marker}: Policies verified: OZ-AUTO-4417, OZ-HOME-2280, OZ-TRAV-9153."}
            reason = "stop"
        else:
            names = {t["function"]["name"] for t in body.get("tools", [])}
            if TOOL not in names:
                raise ValueError(f"Agent did not advertise {TOOL}")
            message = {
                "role": "assistant", "content": None,
                "tool_calls": [{"id": call_id, "type": "function", "function": {
                    "name": TOOL, "arguments": "{}"}}],
            }
            reason = "tool_calls"
        return {
            "id": "chatcmpl-" + marker, "object": "chat.completion", "created": 0,
            "model": body["model"],
            "choices": [{"index": 0, "message": message, "finish_reason": reason}],
            "usage": {"prompt_tokens": 20, "completion_tokens": 10, "total_tokens": 30},
        }


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def reply(self, status, body):
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        if self.path == "/health":
            self.reply(200, {"status": "ok"})
        elif self.path == "/evidence":
            with self.server.lock:
                self.reply(200, {"completed": sorted(self.server.completed), "errors": list(self.server.errors)})
        else:
            self.reply(404, {"error": "Unknown stub route"})

    def do_POST(self):
        if self.path != "/v1/chat/completions":
            self.reply(404, {"error": "Unknown model API"})
            return
        try:
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            completion = self.server.completion(body)
            if not body.get("stream"):
                self.reply(200, completion)
                return
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            choice = completion["choices"][0]
            delta = choice["message"]
            for index, call in enumerate(delta.get("tool_calls", [])):
                call["index"] = index
            chunk = {k: completion[k] for k in ("id", "created", "model")}
            chunk["object"] = "chat.completion.chunk"
            chunk["choices"] = [{"index": 0, "delta": delta, "finish_reason": None}]
            self.wfile.write(("data: " + json.dumps(chunk) + "\n\n").encode())
            chunk["choices"] = [{"index": 0, "delta": {}, "finish_reason": choice["finish_reason"]}]
            chunk["usage"] = completion["usage"]
            self.wfile.write(("data: " + json.dumps(chunk) + "\n\ndata: [DONE]\n\n").encode())
            self.wfile.flush()
        except (ValueError, KeyError, StopIteration, AttributeError, TypeError) as exc:
            error = f"{type(exc).__name__}: {exc}"
            with self.server.lock:
                self.server.errors.append(error)
            self.reply(400, {"error": {"message": error, "type": "smoke_contract_error"}})


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=8099)
    args = parser.parse_args()
    with ModelServer(("0.0.0.0", args.port)) as server:
        server.serve_forever()
