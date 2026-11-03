# A2A sample smoke tests

These tests run the real sample and installed dependencies against a deterministic
local OpenAI HTTP/SSE server. They need no paid model account or Actions secrets.
The application is not mocked. Existing nightly and platform tests are preserved.

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

## Agent Manager deployment and trace check

Use a running local dev stack (`make setup`) and its kubectl context. Install
`requests` in a test environment, then run:

```bash
python3.11 -m venv /tmp/a2a-platform-env
/tmp/a2a-platform-env/bin/pip install requests
export IDP_CLIENT_SECRET='<local dev-stack bootstrap credential>'
/tmp/a2a-platform-env/bin/python test/sample-smoke/a2a/a2a_platform_smoke.py \
  --repo https://github.com/wso2/agent-manager \
  --branch '<published PR head branch>' --sha '<full published PR head SHA>' \
  --reports /tmp/a2a-platform
```

For fork PRs use the fork repository. Unpublished sample changes cannot be built
remotely; local smoke tests the working tree. The shared platform harness creates
an isolated project, agent and model-stub namespace, waits for the initial branch
build to finish, then explicitly builds `commitId=<PR SHA>`. It verifies the full
WorkflowRun commit, repository, application path, and active image before testing.
The agent uses A2A subtype, port 9099, and auto instrumentation. A scoped temporary
NetworkPolicy permits only this agent to reach the stub. The test probes workload
health, calls the gateway's card and both transports with an API key, and polls
Observer for an LLM span containing this request's marker. A stale trace cannot
pass. It cleans up agent/project, policy, stub namespace and port forwards even
on failure. Cleanup errors fail the job. No existing agents or traces are deleted.

The A2A workflow checks out the exact PR head. Both jobs run for any change under
`samples/a2a-agent/`, including dependency files, and for changes to this harness
or its shared dependencies. The platform job builds Agent Manager from that
checkout using `amp-dev-stack`; it verifies the PR-built Observer image and health
before testing. Docker/k3s/Kubernetes diagnostics are uploaded on failure.
The A2A harness is self-contained and reuses the existing platform API client.

## Regression guards

```bash
/tmp/a2a-platform-env/bin/python -m unittest discover \
  -s test/sample-smoke/a2a -p 'test_*.py' -v
```

These reject failed/protocol-error tasks, missing/wrong artifacts, stale or partial
model evidence, wrong sample paths and wrong build commits. 

Project cleanup requires the DELETE API to acknowledge removal and then waits
for the exact project CR to disappear from Kubernetes. The current project GET
endpoint incorrectly returns 500 for a deleted project; the test does not treat
that error as success. A terminating CR, failed DELETE or failed Kubernetes query
still fails cleanup. Agent deletion continues to require API confirmation.
