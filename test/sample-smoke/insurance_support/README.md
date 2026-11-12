# Insurance Support platform smoke

The PR workflow runs the local integration/HTTP tests and web build first, then
starts Agent Manager on Docker Compose from the PR checkout. No repository
secrets or paid APIs are required; the IDP credential in CI belongs to that
ephemeral stack.

`platform_smoke.py` runs the sample as an external agent (see
[../README.md](../README.md)). A local OpenAI-compatible model service supports
Strands streaming completions. It requests the real `list_policies` tool and
validates all three policy IDs before responding. The test invokes `/chat` and
requires an observer model span containing this invocation's unique marker.
Bedrock and MCP behavior remains covered by the existing offline integration
tests; the platform scenario uses the default OpenAI path and local policy tools.

Run the regression guards independently:

```bash
python3.11 -m unittest discover -s test/sample-smoke/insurance_support -p 'test_*.py' -v
```
