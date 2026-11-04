"""Strict, offline OpenAI chat-completions server for the IT Helpdesk smoke test."""
from __future__ import annotations

import argparse
import json
import re
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

TOOL = "check_system_status"


class ModelServer(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, address):
        super().__init__(address, Handler)
        self.lock = threading.Lock()
        self.completed = set()
        self.errors = []

    def completion(self, body):
        if body.get("stream"):
            raise ValueError("This scenario expects non-streaming chat completions")
        messages = body["messages"]
        user = next(m["content"] for m in reversed(messages) if m["role"] == "user")
        marker = re.search(r"smoke-[a-f0-9]{32}", user).group()
        call_id = "call_" + marker
        tool_results = [m for m in messages if m["role"] == "tool" and m.get("tool_call_id") == call_id]
        if tool_results:
            rows = json.loads(tool_results[-1]["content"])
            if not any(r.get("service") == "VPN Gateway" and r.get("status") == "operational" for r in rows):
                raise ValueError("The real tool did not return the seeded operational VPN Gateway")
            with self.lock:
                self.completed.add(marker)
            message = {"role": "assistant", "content": f"{marker}: VPN Gateway is operational."}
            reason = "stop"
        else:
            names = {t["function"]["name"] for t in body.get("tools", [])}
            if TOOL not in names:
                raise ValueError(f"Agent did not advertise {TOOL}")
            message = {
                "role": "assistant", "content": None,
                "tool_calls": [{"id": call_id, "type": "function", "function": {
                    "name": TOOL, "arguments": json.dumps({"service_query": "vpn"})}}],
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
            self.reply(200, self.server.completion(body))
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
