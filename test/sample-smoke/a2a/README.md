# A2A sample smoke tests

These tests run the real sample and installed dependencies against a deterministic
local OpenAI HTTP/SSE server. They need no paid model account or Actions secrets.
The application is not mocked.

## Local smoke

From the repository root, with Python 3.11 installed:

```bash
bash samples/a2a-agent/scripts/smoke_test.sh --reports /tmp/a2a-smoke
```

A temporary virtual environment installs the sample's current `requirements.txt`
and runs `pip check`. The test starts uvicorn and checks `/health`, agent-card
discovery, a JSON-RPC `SendMessage` summary (using streaming OpenAI output), and
HTTP+JSON action extraction (using a nonstreaming JSON completion). It validates
completed task states, exact text/data artifacts, and reads the JSON-RPC task back
through REST. The card must advertise the server being called. Both model calls
must carry this invocation's unique marker. The app process and stub are stopped
and the temporary environment removed on success, failure, and cancellation.
Install/app logs, model evidence and response JSON remain in the reports directory.

## Agent Manager external agent and trace check

`a2a_platform_smoke.py` runs the sample as an external agent against a running
Agent Manager and finds its model trace; see [../README.md](../README.md).

## Regression guards

```bash
python3.11 -m unittest discover -s test/sample-smoke/a2a -p 'test_*.py' -v
```

These reject failed/protocol-error tasks, missing/wrong artifacts, stale or
partial model evidence, and traces without this invocation's model span.
