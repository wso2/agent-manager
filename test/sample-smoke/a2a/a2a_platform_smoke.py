"""Build the PR A2A sample, deploy through Agent Manager and find its model trace."""
import argparse
import os
from pathlib import Path
import signal
import sys

from a2a_smoke import invoke, check_evidence
from platform_smoke import run

SAMPLE = {
    'path': 'samples/a2a-agent', 'prefix': 'a2a-smoke-', 'port': 9099,
    'agent_type': {'type': 'agent-api', 'subType': 'a2a-agent'},
    'stub': Path(__file__).with_name('a2a_model_stub.py'),
    'invoke': invoke, 'check_evidence': check_evidence,
    'env': lambda url, sha: {'OPENAI_API_KEY': 'smoke-only', 'OPENAI_BASE_URL': url,
                            'AMP_TRACE_CONTENT': 'true'},
}

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('repo', 'branch', 'sha'):
        parser.add_argument('--' + name, required=True)
    parser.add_argument('--reports', type=Path, default=Path('/tmp/a2a-platform-reports'))
    for name, env, default in [('api', 'AMP_API_BASE_URL', 'http://localhost:9000'),
                               ('observer', 'AM_OBSERVER_BASE_URL', 'http://localhost:9098'),
                               ('token-url', 'IDP_TOKEN_URL', 'http://localhost:8090/oauth2/token'),
                               ('client-id', 'IDP_CLIENT_ID', 'amp-api-client'),
                               ('org', 'DEFAULT_ORG', 'default'), ('environment', 'DEFAULT_ENV', 'default')]:
        parser.add_argument('--' + name, default=os.getenv(env, default))
    parser.add_argument('--pipeline', default='default')
    args = parser.parse_args()
    args.client_secret = os.environ.get('IDP_CLIENT_SECRET', '')
    if not args.client_secret: parser.error('Set IDP_CLIENT_SECRET to the local dev stack credential')
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(143))
    run(args, SAMPLE)
