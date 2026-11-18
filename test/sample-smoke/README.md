# Sample smoke tests

Each sample directory has a local smoke test (the sample against a local model
stub, no Agent Manager) and a platform smoke test, `platform_smoke.py`, that
runs the same sample as an **external agent** against a running Agent Manager.

The platform smoke uses `external_agent.py`:

1. installs the sample's requirements and `libs/amp-instrumentation` from this
   checkout into a fresh virtualenv,
2. registers a project and an external agent and mints the agent's API key,
3. starts the model stub (and any services the sample needs) and runs the sample
   under `amp-instrument`, with `AMP_AGENT_API_KEY` set to the key and
   `AMP_OTEL_ENDPOINT` pointing at a local sink that counts the exports carrying
   that key in `x-amp-api-key` and drops the spans (Agent Manager stores no
   traces),
4. drives the sample with a unique marker and checks the model stub's evidence
   for that invocation, then deletes the agent and project. Most samples must
   complete a tool round trip; the A2A sample instead sends its two skill
   requests and checks the returned artifacts and both model calls. After the
   sample stops, at least one span export must have carried the agent's key.

The smoke signs in with client credentials against ThunderID, so the stack
must run in idp mode. Set both lines in `deployments/standalone/.env` before
`make up` or `make dev-up`; `AUTH_MODE` alone leaves the API validating its
own keys and refusing ThunderID's tokens:

```bash
AUTH_MODE=idp
KEY_MANAGER_JWKS_URL=http://thunder:${THUNDER_PORT}/oauth2/jwks
```

Then run it:

```bash
python3.11 -m venv /tmp/smoke-env && /tmp/smoke-env/bin/pip install requests
# The stack's bootstrap credential; never use production credentials.
IDP_CLIENT_SECRET=amp-api-client-secret \
  /tmp/smoke-env/bin/python test/sample-smoke/it_helpdesk/platform_smoke.py \
  --python python3.11 --reports /tmp/it-helpdesk-platform
```

Defaults match the stack's `.env.example`; override with `--api` and
`--token-url` (or `AMP_API_BASE_URL` and `IDP_TOKEN_URL`). Reports hold the
install and application logs, the model evidence and the response, with the
number of span exports the sink received, with and without the key.

`manual_instrumentation/` has only the platform smoke; its local smoke is in
`local/`, and the sample runs without `amp-instrument` because it exports its
own spans (`ExternalSample.auto_instrument=False`).

In CI each sample's `*-sample-pr-checks.yaml` (Manual Instrumentation:
`manual-instrumentation-agent-local-smoke.yaml`) runs the local smoke, then the
platform smoke on a stack built from the PR by `.github/actions/amp-dev-stack`.
