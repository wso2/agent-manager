"""Local OpenAI-compatible fixture and HTTP chat smoke assertion (stdlib only)."""
import argparse
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
import re
from urllib.request import Request, urlopen
import uuid


def completion(body):
    messages = body['messages']
    marker = re.search(r'hotel-smoke-[0-9a-f]{32}', json.dumps(messages))
    if not marker:
        raise ValueError('Missing invocation marker')
    marker = marker[0]
    tool_results = [m for m in messages if m.get('role') == 'tool']
    if tool_results:
        result = tool_results[-1]
        if result.get('tool_call_id') != 'call_' + marker:
            raise ValueError('Unexpected tool call ID')
        hotels = json.loads(result['content']).get('hotels', [])
        if not any(h.get('hotel_id') == 'mock-002-tokyo' for h in hotels):
            raise ValueError('Real Tokyo hotel search result was not returned')
        message = {'role': 'assistant', 'content': marker + ' Tokyo hotel search verified'}
        finish = 'stop'
    else:
        if not any(t.get('function', {}).get('name') == 'search_hotels_tool' for t in body.get('tools', [])):
            raise ValueError('Hotel search tool was not bound to the model')
        message = {'role': 'assistant', 'content': None, 'tool_calls': [{
            'id': 'call_' + marker, 'type': 'function', 'function': {
                'name': 'search_hotels_tool', 'arguments': json.dumps({'destination': 'Tokyo'})}}]}
        finish = 'tool_calls'
    return {'id': 'chatcmpl-smoke', 'object': 'chat.completion', 'created': 0,
            'model': body['model'], 'choices': [{'index': 0, 'message': message, 'finish_reason': finish}],
            'usage': {'prompt_tokens': 1, 'completion_tokens': 1, 'total_tokens': 2}}, marker, finish


class Handler(BaseHTTPRequestHandler):
    completed = []
    errors = []

    def reply(self, status, body):
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        self.reply(200, {'completed': self.completed, 'errors': self.errors})

    def do_POST(self):
        try:
            if self.path != '/v1/chat/completions':
                raise ValueError('Unexpected model endpoint: ' + self.path)
            body, marker, finish = completion(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
            if finish == 'stop':
                self.completed.append(marker)
            self.reply(200, body)
        except Exception as exc:
            self.errors.append(str(exc))
            self.reply(400, {'error': {'message': str(exc), 'type': 'invalid_request_error'}})


def read(url, body=None):
    data = json.dumps(body).encode() if body is not None else None
    with urlopen(Request(url, data=data, headers={'Content-Type': 'application/json'}), timeout=45) as response:
        return json.load(response)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--serve', type=int)
    parser.add_argument('--host', default='127.0.0.1')
    parser.add_argument('--agent')
    parser.add_argument('--model')
    args = parser.parse_args()
    if args.serve:
        HTTPServer((args.host, args.serve), Handler).serve_forever()
    else:
        marker = 'hotel-smoke-' + uuid.uuid4().hex
        result = read(args.agent + '/chat', {'message': 'Search hotels in Tokyo. ' + marker, 'session_id': marker})
        assert result.get('response') == marker + ' Tokyo hotel search verified', result
        evidence = read(args.model + '/evidence')
        assert marker in evidence['completed'] and not evidence['errors'], evidence
        print('PASS: /chat → local model → hotel search tool → real Hotel API → model → response')
