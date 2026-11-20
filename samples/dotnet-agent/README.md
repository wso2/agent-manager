# .NET External Agent Sample

A small .NET agent that sends OpenTelemetry GenAI traces to WSO2 Agent Manager
(AMP) as an external agent. It's the .NET version of the Python
[`manual-instrumentation-agent`](../manual-instrumentation-agent) and uses the
same trace contract, so its traces have the same shape.

> [!NOTE]
> AMP Lite does not store traces and has no traces page. To see the traces
> this sample emits, run your own OpenTelemetry backend and point
> `AMP_OTEL_ENDPOINT` at its OTLP/HTTP endpoint. See the
> [user guide](../../docs/user-guide.md).

## What this shows

Python agents get zero-code tracing from `amp-instrument`, and Ballerina agents
from `ballerinax/amp`. There's no equivalent for .NET yet, so a .NET agent
configures its own OTLP exporter. You run the agent yourself, and it pushes
traces to the OTLP endpoint in `AMP_OTEL_ENDPOINT`. AMP registers the agent and
gives you its API key, which the exporter sends with each batch of spans.

The agent itself is a small weather assistant built with
[Microsoft Agent Framework](https://learn.microsoft.com/en-us/agent-framework/overview/)
(Microsoft's newer agent SDK, built on Microsoft.Extensions.AI). Agent Framework
emits the OpenTelemetry
[GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/)
out of the box, so its spans follow the same contract as an auto-instrumented
Python agent's.

## How it works

There are two parts, which line up with the Python sample's `init_otel()` plus
agent split:

- [`AmpTelemetry.cs`](./AmpTelemetry.cs) is the equivalent of `init_otel()`. It
  reads `AMP_OTEL_ENDPOINT` and `AMP_AGENT_API_KEY`, then sets up the
  OpenTelemetry tracer provider to export spans over OTLP/HTTP to
  `<AMP_OTEL_ENDPOINT>/v1/traces` with the `x-amp-api-key` header. It doesn't do
  any instrumentation on its own.
- [`Agent.cs`](./Agent.cs) is the agent. One `UseOpenTelemetry(sourceName)` call
  on the agent is all you need. Agent Framework wires up the chat and tool spans
  underneath it automatically.

A single `/chat` request produces one trace:

```text
invoke_agent WeatherAgent   (agent, root)
├── chat <model>            (the model decides to call a tool)
│   └── execute_tool GetWeather  (the tool runs)
└── chat <model>            (the model writes the final answer)
```

### What's on the spans

The spans carry the OpenTelemetry GenAI attributes AMP's observer reads:
`gen_ai.operation.name` (`invoke_agent`, `chat`, `execute_tool`),
`gen_ai.agent.name`, `gen_ai.request.model`, `gen_ai.tool.name`,
`gen_ai.usage.input_tokens` / `output_tokens`, and the input and output messages
when content capture is on. The full list is in
[the contract](https://wso2.com/agent-platform/docs/latest/guides/amp-instrumentation/#the-contract).

## Prerequisites

- .NET 10 SDK (see below to install)
- An OpenAI API key (or any OpenAI-compatible endpoint, set via `OPENAI_BASE_URL`)
- Agent Manager running. From the repository root, `make up` starts it (see
  [`deployments/standalone`](../../deployments/standalone/README.md)).

### Installing the .NET SDK

If you don't already have the `dotnet` command, grab the .NET 10 SDK. The
official installers and instructions for every platform are here:
<https://dotnet.microsoft.com/download/dotnet/10.0> (Microsoft's per-OS guide:
<https://learn.microsoft.com/en-us/dotnet/core/install/>).

Quick options:

```bash
# macOS (Homebrew)
brew install dotnet

# Linux / macOS (Microsoft's install script, no root needed)
curl -fsSL https://dot.net/v1/dotnet-install.sh | bash -s -- --channel 10.0
# then add it to PATH for the current shell:
export PATH="$HOME/.dotnet:$PATH"
```

On Windows use the installer from the download page above, or `winget install
Microsoft.DotNet.SDK.10`. Confirm it worked with `dotnet --version` (you should
see a `10.x` version).

## Run it

Register the agent first:

1. Sign in to the console at <http://localhost:9000> (the port is
   `AMP_API_PORT` in `deployments/standalone/.env`) as `admin`; the password
   is `AMP_ADMIN_PASSWORD` in the same file.
2. A new install has no projects, so create one first: click **Add project**
   (**Create project** on the organization overview), enter a **Name** and
   click **Create**.
3. Open the project, click **New agent**, enter the name `Weather Agent` and
   click **Register**.
4. The **Setup Agent** panel opens with an API key already generated. Copy it
   as `AMP_AGENT_API_KEY`; it is shown only once. To change how long it is
   valid, pick another **Token Duration** and click **Regenerate**.

`AMP_OTEL_ENDPOINT` is the OTLP/HTTP base URL of your own OpenTelemetry
collector, without `/v1/traces`.

Then set the environment variables and run it:

```bash
cd samples/dotnet-agent

export AMP_OTEL_ENDPOINT="http://localhost:4318"   # your own OpenTelemetry collector
export AMP_AGENT_API_KEY="<key-from-the-console>"
export OPENAI_API_KEY="<your-openai-key>"

dotnet run
```

The agent listens on `http://localhost:8000`.

## Test it

Send it a request with `curl`:

```bash
curl -X POST http://localhost:8000/chat \
  -H 'Content-Type: application/json' \
  -d '{"session_id": "demo-1", "message": "What is the weather in Tokyo?"}'
```

You get back the agent's answer, and one full trace goes to your collector
behind it.

If you want to watch the spans locally while you test, set `AMP_OTEL_CONSOLE=true`
before running. They print to the console as well as getting exported.

## See the traces

Open them in the backend behind your collector; AMP Lite does not store them.
Each `/chat` call shows up as one trace, with the model, the token counts, and
the input and output.

## Notes

- **Content capture.** Prompt and completion text is captured by default. Set
  `AMP_TRACE_CONTENT=false` to turn it off. The spans and metadata are still
  recorded, you just lose the message text.
- **Experimental conventions.** The .NET GenAI conventions are still
  experimental, and Agent Framework moves fast, so the package versions in
  [`dotnet-agent.csproj`](./dotnet-agent.csproj) are pinned on purpose. Bump them
  deliberately.
- **Infrastructure spans (optional).** If you also want ASP.NET Core, HttpClient,
  and DB spans around the gen_ai spans, add the OpenTelemetry .NET zero-code
  (CoreCLR) auto-instrumentation. There's a commented setup in the
  [`Dockerfile`](./Dockerfile), and more detail in the .NET section of the AMP
  instrumentation docs. The profiler only adds infrastructure spans though; it
  won't produce the gen_ai spans on its own.

## Files

| File | Role |
|---|---|
| `AmpTelemetry.cs` | Configures the OpenTelemetry OTLP exporter to `AMP_OTEL_ENDPOINT`. The `init_otel()` equivalent. |
| `Agent.cs` | The Microsoft Agent Framework agent and its `UseOpenTelemetry` wiring. |
| `Program.cs` | ASP.NET Core minimal API (`POST /chat`, `GET /healthz`). Calls `AmpTelemetry.Configure()`. |
| `dotnet-agent.csproj` | Pinned package references. |
| `Dockerfile` | Container build, with the optional zero-code overlay documented. |
| `.env.example` | Environment variable template. |
