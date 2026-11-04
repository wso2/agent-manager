"""Deterministic OpenAI HTTP/SSE fixture for the A2A sample; stdlib only."""
import json
import re
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class ModelServer(ThreadingHTTPServer):
    def __init__(self, address):
        super().__init__(address, Handler)
        self.calls = []
        self.lock = threading.Lock()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        if self.path not in ('/health', '/evidence'):
            self.send_error(404)
            return
        with self.server.lock:
            data = {'calls': list(self.server.calls)} if self.path == '/evidence' else {'status': 'ok'}
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        self.wfile.write(json.dumps(data).encode())

    def do_POST(self):
        if self.path != '/v1/chat/completions':
            self.send_error(404)
            return
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        notes = body['messages'][-1]['content']
        match = re.search(r'smoke-[0-9a-f]{32}', notes)
        if not match:
            self.send_error(400, 'Missing invocation marker')
            return
        marker = match[0]
        streaming = body.get('stream', False)
        skill = 'summary' if streaming else 'actions'
        text = marker + ': Priya will cut the release branch Thursday.' if streaming else json.dumps({
            'action_items': [{'owner': 'Priya', 'task': marker + ': cut the release branch', 'due': 'Thursday'}]})
        with self.server.lock:
            self.server.calls.append({'marker': marker, 'skill': skill})
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream' if streaming else 'application/json')
        self.end_headers()
        common = {'id': 'chatcmpl-' + marker, 'model': body['model'], 'created': 1}
        if streaming:
            for delta, finish in [({'role': 'assistant', 'content': text[:30]}, None), ({'content': text[30:]}, None), ({}, 'stop')]:
                chunk = {**common, 'object': 'chat.completion.chunk', 'choices': [{'index': 0, 'delta': delta, 'finish_reason': finish}]}
                self.wfile.write(('data: ' + json.dumps(chunk) + '\n\n').encode())
            self.wfile.write(b'data: [DONE]\n\n')
        else:
            self.wfile.write(json.dumps({**common, 'object': 'chat.completion', 'choices': [{'index': 0, 'message': {'role': 'assistant', 'content': text}, 'finish_reason': 'stop'}], 'usage': {'prompt_tokens': 10, 'completion_tokens': 10, 'total_tokens': 20}}).encode())


if __name__ == '__main__':
    ModelServer(('0.0.0.0', 8099)).serve_forever()
