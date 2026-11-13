# IT Helpdesk sample smoke tests

These checks use the real sample dependencies, HTTP app, LangGraph agent, and
`check_system_status` tool. An HTTP model stub requests a VPN status lookup,
checks the actual tool output, and only then returns a final answer. No OpenAI,
MCP, or other paid-service credentials are needed. This checks integration
compatibility, not the quality or tool-selection behavior of a real model.

## Local HTTP check

From the repository root, with Python 3.11 or newer:

```bash
PYTHON=python3.11 bash samples/it-helpdesk-agent/scripts/smoke_test.sh
# Optional persistent report location:
PYTHON=python3.11 bash samples/it-helpdesk-agent/scripts/smoke_test.sh --reports /tmp/helpdesk-local
```

The runner creates a disposable venv, installs the checked-out requirements,
runs `pip check`, starts the application and model stub on free loopback ports,
checks `/health`, and calls `/chat`. It verifies the response's session/version
and the model's evidence of the real tool round trip. Application processes and
the venv are removed on exit. Install/application logs and JSON evidence remain
in the printed report directory. Package installation needs network access;
inference does not.

## Agent Manager external agent and trace check

`platform_smoke.py` runs the sample as an external agent against a running
Agent Manager and finds its model trace; see [../README.md](../README.md).

## CI

`IT Helpdesk Sample PR Checks` runs on every PR changing this sample, its smoke
harness, or workflow, and can also be dispatched manually. The local job runs
first; the platform job then starts Agent Manager on Docker Compose from the PR
with `amp-dev-stack` and uses the stack's bootstrap test credential, so it needs
no Actions secrets. On failure it keeps the stack's service logs in the
platform smoke artifact.

Run the regression guards independently:

```bash
python3.11 -m unittest discover -s test/sample-smoke/it_helpdesk -p 'test_*.py' -v
```
