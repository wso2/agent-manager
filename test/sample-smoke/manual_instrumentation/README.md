# Manual Instrumentation external agent and trace smoke

The PR workflow runs the sample's local smoke test first, then starts Agent
Manager on Docker Compose from the PR checkout with `amp-dev-stack`.

`platform_smoke.py` runs the sample as an external agent (see
[../README.md](../README.md)), but without `amp-instrument`: the sample exports
its own spans through `init_otel()`. It reuses the local smoke's model stub,
which only answers once the sample's real `word_count` tool result comes back.
The run passes once the observer returns a model span containing this
invocation's marker.

## Local run

Start the stack (`make up` from the repository root), then:

```bash
python3.11 -m venv /tmp/manual-smoke-env && /tmp/manual-smoke-env/bin/pip install requests
IDP_CLIENT_SECRET=amp-api-client-secret \
  /tmp/manual-smoke-env/bin/python test/sample-smoke/manual_instrumentation/platform_smoke.py \
  --python python3.11 --reports /tmp/manual-instrumentation-platform
python3.11 -m unittest discover -s test/sample-smoke/manual_instrumentation -p 'test_*.py' -v
```
