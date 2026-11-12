# Sample smoke tests

Each sample directory has a local smoke test (the sample against a local model
stub, no Agent Manager) and a platform smoke test, `platform_smoke.py`, that
runs the same sample as an **external agent** against a running Agent Manager.

The platform smoke uses `external_agent.py`:

1. installs the sample's requirements and `libs/amp-instrumentation` from this
   checkout into a fresh virtualenv,
2. registers a project and an external agent and mints the agent's trace token,
3. starts the model stub (and any services the sample needs) and runs the sample
   under `amp-instrument`, with `AMP_OTEL_ENDPOINT` pointing at the AI Gateway's
   `/otel` endpoint and `AMP_AGENT_API_KEY` set to the token,
4. sends one request carrying a unique marker and checks the model stub saw the
   full tool round trip,
5. polls the observer until a trace for this agent contains a model span with
   that marker, then deletes the agent and project.

Run it against the Docker Compose stack (`make up` or `make dev-up`):

```bash
python3.11 -m venv /tmp/smoke-env && /tmp/smoke-env/bin/pip install requests
# The stack's bootstrap credential; never use production credentials.
IDP_CLIENT_SECRET=amp-api-client-secret \
  /tmp/smoke-env/bin/python test/sample-smoke/it_helpdesk/platform_smoke.py \
  --python python3.11 --reports /tmp/it-helpdesk-platform
```

Defaults match the stack's `.env.example`; override with `--api`, `--observer`,
`--gateway`, `--token-url` (or `AMP_API_BASE_URL`, `AM_OBSERVER_BASE_URL`,
`AMP_GATEWAY_URL`, `IDP_TOKEN_URL`). Reports hold the install and application
logs, the model evidence, the trace listing and the matching spans.

In CI each sample's `*-sample-pr-checks.yaml` runs the local smoke, then the
platform smoke on a stack built from the PR by `.github/actions/amp-dev-stack`.
