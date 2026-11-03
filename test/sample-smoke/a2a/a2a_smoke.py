"""Exercise the real A2A server and both skills against a local OpenAI stub."""
import argparse
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import uuid
from urllib.parse import urlsplit

from a2a_model_stub import ModelServer

ROOT = Path(__file__).resolve().parents[3]
from smoke import request, wait_ready, stop_process

SAMPLE = ROOT / 'samples/a2a-agent'


def check_task(response, marker, artifact):
    if 'error' in response:
        raise AssertionError(f'A2A protocol error: {response}')
    result = response.get('result', response)
    task = result.get('task', result)
    if task.get('status', {}).get('state') != 'TASK_STATE_COMPLETED':
        raise AssertionError(f'A2A task did not complete: {response}')
    artifacts = task.get('artifacts', [])
    found = next((a for a in artifacts if a.get('name') == artifact), None)
    if found is None or marker not in json.dumps(found):
        raise AssertionError(f'Missing request-specific {artifact}: {response}')
    parts = found.get('parts', [])
    if artifact == 'summary.txt':
        if ''.join(p.get('text', '') for p in parts) != marker + ': Priya will cut the release branch Thursday.':
            raise AssertionError(f'Wrong summary artifact: {found}')
    else:
        expected = {'action_items': [{'owner': 'Priya', 'task': marker + ': cut the release branch', 'due': 'Thursday'}]}
        if len(parts) != 1 or parts[0].get('data') != expected:
            raise AssertionError(f'Wrong action-items artifact: {found}')
    return task


def check_evidence(evidence, marker):
    skills = {c['skill'] for c in evidence['calls'] if c['marker'] == marker}
    if skills != {'summary', 'actions'}:
        raise AssertionError(f'Both real model calls required: {evidence}')


def invoke(endpoint, marker, headers=None, version=None):
    headers = {'A2A-Version': '1.0', **(headers or {})}
    card = wait_ready(endpoint.rstrip('/') + '/.well-known/agent-card.json', headers=headers)
    if {s['id'] for s in card['skills']} != {'summarize-notes', 'extract-action-items'}:
        raise AssertionError(f'Unexpected skills: {card}')
    interfaces = {i['protocolBinding']: i['url'] for i in card['supportedInterfaces']}
    if set(interfaces) != {'JSONRPC', 'HTTP+JSON'}:
        raise AssertionError(f'Unexpected interfaces: {card}')
    # Resolve through the returned card and ensure the gateway rewrites its URLs.
    for url in interfaces.values():
        if urlsplit(url).netloc != urlsplit(endpoint).netloc or not url.startswith(endpoint.rstrip('/') + '/'):
            raise AssertionError(f'Card advertises a different server: {url}')
    notes = f'{marker}: Priya will cut the release branch Thursday.'
    def message(skill):
        return {'messageId': uuid.uuid4().hex, 'role': 'ROLE_USER',
                'metadata': {'skill': skill}, 'parts': [{'text': notes}]}
    summary = request(interfaces['JSONRPC'], {'jsonrpc': '2.0', 'id': 1, 'method': 'SendMessage', 'params': {'message': message('summarize-notes')}}, headers)
    task = check_task(summary, marker, 'summary.txt')
    actions = request(interfaces['HTTP+JSON'].rstrip('/') + '/message:send', {'message': message('extract-action-items')}, headers)
    check_task(actions, marker, 'action-items.json')
    fetched = request(interfaces['HTTP+JSON'].rstrip('/') + '/tasks/' + task['id'], headers=headers)
    check_task(fetched, marker, 'summary.txt')
    if fetched.get('id') != task['id']:
        raise AssertionError(f'Task cannot be read across transports: {fetched}')
    return {'card': card, 'summary': summary, 'actions': actions}


def local_smoke(python, reports):
    reports.mkdir(parents=True, exist_ok=True)
    phase = 'dependency installation'
    try:
        with tempfile.TemporaryDirectory(prefix='a2a-smoke-') as work:
            venv = Path(work) / 'venv'
            subprocess.run([python, '-m', 'venv', str(venv)], check=True)
            executable = str(venv / 'bin/python')
            with (reports / 'install.log').open('w') as log:
                subprocess.run([executable, '-m', 'pip', 'install', '-r', str(SAMPLE / 'requirements.txt')], stdout=log, stderr=subprocess.STDOUT, check=True)
                subprocess.run([executable, '-m', 'pip', 'check'], stdout=log, stderr=subprocess.STDOUT, check=True)
            phase = 'startup'
            with socket.socket() as sock:
                sock.bind(('127.0.0.1', 0)); port = sock.getsockname()[1]
            with ModelServer(('127.0.0.1', 0)) as model, (reports / 'application.log').open('w') as log:
                thread = threading.Thread(target=model.serve_forever, daemon=True); thread.start()
                endpoint = f'http://127.0.0.1:{port}'
                model_url = f'http://127.0.0.1:{model.server_port}'
                env = {k: v for k, v in os.environ.items() if not k.startswith(('OPENAI_', 'OTEL_', 'AMP_', 'LANGSMITH_', 'LANGCHAIN_', 'PYTHONPATH', 'PYTHONHOME', 'AGENT_PUBLIC_BASE_URL'))}
                env.update(OPENAI_API_KEY='smoke-only', OPENAI_BASE_URL=model_url + '/v1', AGENT_PUBLIC_BASE_URL=endpoint)
                proc = None
                try:
                    proc = subprocess.Popen([executable, '-m', 'uvicorn', 'app:app', '--host', '127.0.0.1', '--port', str(port)], cwd=SAMPLE, env=env, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
                    assert wait_ready(endpoint + '/health', proc)['status'] == 'ok'
                    phase = 'A2A discovery and both skill round trips'
                    marker = 'smoke-' + uuid.uuid4().hex
                    result = invoke(endpoint, marker)
                    evidence = request(model_url + '/evidence'); check_evidence(evidence, marker)
                    (reports / 'result.json').write_text(json.dumps({'response': result, 'model': evidence}, indent=2))
                finally:
                    if proc is not None: stop_process(proc)
                    (reports / 'model-evidence.json').write_text(json.dumps(request(model_url + '/evidence'), indent=2))
                    model.shutdown(); thread.join(timeout=5)
    except BaseException:
        print(f'FAIL: {phase}; reports: {reports}', file=sys.stderr)
        for filename in ('install.log', 'application.log'):
            path = reports / filename
            if path.exists(): print('\n'.join(path.read_text().splitlines()[-60:]), file=sys.stderr)
        raise
    print(f'PASS: install, startup, health, discovery, JSON-RPC summary, REST actions, cross-transport task read; {reports}')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--python', default=sys.executable)
    parser.add_argument('--reports', type=Path, default=Path(tempfile.mkdtemp(prefix='a2a-reports-')))
    args = parser.parse_args()
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(143))
    local_smoke(args.python, args.reports.resolve())
