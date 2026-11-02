# Local sample smoke tests

This suite covers CrewAI, Customer Support, .NET, Leave Assistant, and Manual
Instrumentation. A2A, Hotel Booking, Insurance Support, and IT Helpdesk are outside
this workflow. Existing nightly and platform workflows are unchanged.

Each check installs the sample's PR-head requirements or builds its project,
starts the real HTTP app, checks readiness, and sends one `/chat` request through
the real model SDK and tool implementation. A localhost model stub requests a
tool and only returns the final response after validating the tool result.

| Sample | Request exercised | Readiness |
| --- | --- | --- |
| CrewAI | Researcher calls `country_capital_lookup`; editor completes the answer | `/openapi.json` |
| Customer Support | `lookup_policy` retrieves refund text from a local FAQ using stub embeddings | `/openapi.json` |
| .NET | `GetWeather` returns the seeded Paris weather | `/healthz` |
| Leave Assistant | `getLeaveBalance` reads employee E-3001 | `/chat` responds 405 to GET |
| Manual Instrumentation | Embeddings, local retrieval, and the real `word_count` tool | `/health` |

Customer Support's database-backed flight/booking tools and Tavily search are
not exercised; the representative request uses the policy tool. The fixture
provides FAQ HTTP, OpenAI embeddings/chat (including streaming), and an OTLP sink.
No paid APIs, model credentials, Agent Manager deployment, or remote trace service
are needed. Synthetic fixture keys are local test values.

## Local commands

Python 3.11+, plus .NET 10 for .NET or the distribution declared in
`samples/leave-assistant-agent/Ballerina.toml` for Ballerina, must be installed and on PATH.
CI installs Ballerina 2201.13.4 with a verified installer checksum.

```bash
python3.11 test/sample-smoke/local/run.py crewai-agent --reports /tmp/crewai-smoke
python3.11 test/sample-smoke/local/run.py customer-support-agent --reports /tmp/customer-smoke
python3.11 test/sample-smoke/local/run.py dotnet-agent --reports /tmp/dotnet-smoke
python3.11 test/sample-smoke/local/run.py leave-assistant-agent --reports /tmp/leave-smoke
python3.11 test/sample-smoke/local/run.py manual-instrumentation-agent --reports /tmp/manual-smoke
python3.11 -m unittest discover -s test/sample-smoke/local -p 'test_*.py' -v
```

.NET and Leave Assistant use their sample's port 8000; run these sequentially
locally and keep that port free. CI isolates every sample in its own job. Python
apps use ephemeral ports. Build/install and request timeouts bound the checks.
The runner copies each sample into a temporary directory, preserves its dependency
files (`bal build --sticky` for Ballerina), and omits local `.env`/`Config.toml`
credentials. Temporary app processes and model servers are cleaned up even on
failure or termination. Reports retain build/install logs, app logs, model
verification evidence, and the HTTP result; failures print bounded log tails.

`POLICY_FAQ_URL` optionally overrides Customer Support's FAQ source, and
`openAiServiceUrl` optionally overrides Leave Assistant's OpenAI endpoint.
Their normal defaults remain unchanged.

## CI selection

Each sample has an independently named `<sample>-local-smoke.yaml` workflow.
Each workflow checks out the exact PR head, including forks and Dependabot.
Changes under that sample select its workflow, including Python requirements,
NuGet project files, Ballerina manifests/lockfiles, and Dockerfiles. Shared harness
changes select all five workflows; each workflow also supports manual dispatch.
Dockerfile changes trigger local source builds/tests; this suite does not build
container images. Ballerina changes are covered even though Dependabot does not
support that ecosystem.
