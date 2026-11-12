# .NET External Agent Sample

A small .NET agent that sends OpenTelemetry GenAI traces to WSO2 Agent Manager
(AMP) as an external agent. It's the .NET version of the Python
[`manual-instrumentation-agent`](../manual-instrumentation-agent) and uses the
same trace contract, so it shows up in the AMP Console the same way.

## What this shows

Python agents get zero-code tracing from `amp-instrument`, and Ballerina agents
from `ballerinax/amp`. There's no equivalent for .NET yet, so a .NET agent
configures its own OTLP exporter. You run the agent yourself, and it pushes
traces to AMP's OTLP endpoint. AMP registers the agent and gives you an API key
to authenticate the traces you send.

The agent itself is a small weather assistant built with
[Microsoft Agent Framework](https://learn.microsoft.com/en-us/agent-framework/overview/)
(Microsoft's newer agent SDK, built on Microsoft.Extensions.AI). Agent Framework
emits the OpenTelemetry
[GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/)
out of the box, so its spans render in the Console and feed evaluators just like
an auto-instrumented Python agent does.

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

1. Sign in to the console at <http://localhost:3000> as `admin` (the password is
   `THUNDER_ADMIN_PASSWORD` in `deployments/standalone/.env`).
2. Open the **Default** project, click **Add Agent**, and register an agent
   named `Weather Agent`.
3. In the **Set up agent** panel that opens, pick a token duration and click
   **Generate**. Copy the API key it shows. `AMP_OTEL_ENDPOINT` is the AI
   Gateway's `/otel` path, without `/v1/traces`.

Then set the environment variables and run it:

```bash
cd samples/dotnet-agent

export AMP_OTEL_ENDPOINT="http://default-default.gateway.localhost:19080/otel"
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

You get back the agent's answer, and one full trace goes to AMP behind it.

If you want to watch the spans locally while you test, set `AMP_OTEL_CONSOLE=true`
before running. They print to the console as well as getting exported.

## See the traces

Open the agent in the AMP Console and go to **OBSERVABILITY → Traces**. Each
`/chat` call shows up as one trace, with the per-kind icons, the model and token
chips, and the input and output. The spans follow the contract, so evaluators
run against them too.

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
| `AmpTelemetry.cs` | Configures the OpenTelemetry OTLP exporter to AMP. The `init_otel()` equivalent. |
| `Agent.cs` | The Microsoft Agent Framework agent and its `UseOpenTelemetry` wiring. |
| `Program.cs` | ASP.NET Core minimal API (`POST /chat`, `GET /healthz`). Calls `AmpTelemetry.Configure()`. |
| `dotnet-agent.csproj` | Pinned package references. |
| `Dockerfile` | Container build, with the optional zero-code overlay documented. |
| `.env.example` | Environment variable template. |
