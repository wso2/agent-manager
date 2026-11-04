# Insurance Support platform smoke

The PR workflow runs the local integration/HTTP tests and web build first, then
starts a fresh Agent Manager stack from the PR checkout on Ubuntu 24.04. No
preflight image probe is used. No repository secrets or paid APIs are required;
the IDP credential in CI belongs to the ephemeral development stack.

The test creates a unique project, agent, and model-stub namespace. It waits for
the automatic branch build to finish, requests `POST .../builds?commitId=<SHA>`,
and polls that returned build name. It verifies repository, branch, application
path, abbreviated build SHA, full WorkflowRun commit, and active deployment image.
A branch build or unrelated image cannot satisfy the check.

A local OpenAI-compatible model service supports Strands streaming completions.
It requests the real `list_policies` tool and validates all three policy IDs
before responding. A narrowly scoped NetworkPolicy permits this agent to reach
only that stub. The test probes the actual workload's `/health` over a temporary
port-forward, invokes the published `/chat` endpoint with an agent API key, and
requires an Observer model span containing this invocation's unique marker.
Bedrock and MCP behavior remains covered by the existing offline integration tests;
the platform scenario uses the default OpenAI path and local policy tools.

## Run locally

Use an existing development AMP stack with its API, Observer, and Thunder IDP
reachable at their default forwarded ports (9000, 9098, 8090). Do not use a
production stack. The script does not install or rebuild a local stack.

```bash
python3.11 -m venv /tmp/insurance-platform-env
/tmp/insurance-platform-env/bin/pip install requests
export IDP_CLIENT_SECRET='<local-development-client-secret>'
/tmp/insurance-platform-env/bin/python test/sample-smoke/insurance_support/platform_smoke.py \
  --repo https://github.com/<fork-owner>/agent-manager \
  --branch <PR-head-branch> \
  --sha <full-published-PR-head-SHA> \
  --reports /tmp/insurance-platform-reports
```

Use `--api`, `--observer`, and `--token-url` for non-default forwarded addresses.
The source commit must be published. Uncommitted agent application changes are
rejected because a remote build cannot test them.

Regression guards run without an AMP stack:

```bash
/tmp/insurance-platform-env/bin/python -m unittest discover \
  -s test/sample-smoke/insurance_support -p 'test_*.py' -v
```

The sample's `scripts/smoke_test.sh` also exercises the platform model stub through
the real local HTTP app, in addition to the existing Bedrock/MCP checks.

## Diagnostics and cleanup

Reports include source provenance, the chat response, stub evidence, and the
matching trace. Failures capture pod and build logs before cleanup. The workflow
also retains Docker/k3s diagnostics if stack setup fails. Full workload specs and
identity credentials are not dumped.

Success, failure, and termination unwind the port-forwards, temporary egress
policy, agent, project, and stub namespace. Agent and project deletion is polled;
cleanup errors fail the job. Project absence is verified against the actual
OpenChoreo Project CR because the current API can return 500 for a missing
project. Unexpected Kubernetes/API failures are not treated as successful cleanup.
