// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

using OpenTelemetry;
using OpenTelemetry.Exporter;
using OpenTelemetry.Resources;
using OpenTelemetry.Trace;

namespace AmpDotnetAgent;

/// <summary>
/// The .NET analog of the <c>amp-instrumentation</c> package's <c>init_otel()</c>
/// helper. It configures the global OpenTelemetry tracer provider to export spans
/// over OTLP/HTTP to the AMP gateway, using the <c>AMP_OTEL_ENDPOINT</c> and
/// <c>AMP_AGENT_API_KEY</c> environment variables — the same two the Python
/// <c>amp-instrument</c> CLI reads.
///
/// It does <i>no</i> instrumentation itself. The agent's
/// <c>.WithOpenTelemetry(SourceName)</c> / <c>.UseOpenTelemetry(SourceName)</c>
/// (see <see cref="AgentFactory"/>) is what emits the <c>gen_ai.*</c> spans; this
/// class only wires the exporter so those spans reach AMP. Idempotent, like
/// <c>init_otel()</c>.
/// </summary>
public static class AmpTelemetry
{
    /// <summary>
    /// The <see cref="System.Diagnostics.ActivitySource"/> name the agent
    /// instrumentation emits under, and that the tracer provider subscribes to.
    /// Must match the <c>sourceName</c> passed to
    /// <c>WithOpenTelemetry</c>/<c>UseOpenTelemetry</c>. If you drop the explicit
    /// name there, Agent Framework defaults to
    /// <c>"Experimental.Microsoft.Agents.AI"</c> — subscribe to that instead.
    /// </summary>
    public const string SourceName = "amp-dotnet-agent";

    // OTLP/HTTP traces signal path appended to AMP_OTEL_ENDPOINT. When the
    // exporter Endpoint is set in code (rather than via OTEL_EXPORTER_OTLP_ENDPOINT),
    // the SDK does NOT append the signal path, so we add it ourselves — matching
    // amp-instrumentation's otel.py:_traces_endpoint.
    private const string TracesPath = "/v1/traces";

    private static readonly Lock InitLock = new();
    private static TracerProvider? _provider;

    /// <summary>
    /// Whether prompt/response content should be captured in spans. Honors
    /// <c>AMP_TRACE_CONTENT</c> (the variable <c>amp-instrument</c> reads);
    /// content capture is on unless it is explicitly <c>false</c>. Maps to Agent
    /// Framework's <c>EnableSensitiveData</c> option.
    /// </summary>
    public static bool TraceContentEnabled =>
        !string.Equals(
            Environment.GetEnvironmentVariable("AMP_TRACE_CONTENT"),
            "false",
            StringComparison.OrdinalIgnoreCase);

    /// <summary>
    /// Configure the global OpenTelemetry tracer provider to export spans to AMP.
    /// Reads <c>AMP_OTEL_ENDPOINT</c> and <c>AMP_AGENT_API_KEY</c> from the
    /// environment. A second call is a no-op.
    /// </summary>
    /// <exception cref="InvalidOperationException">
    /// If <c>AMP_OTEL_ENDPOINT</c> or <c>AMP_AGENT_API_KEY</c> is unset.
    /// </exception>
    public static void Configure()
    {
        lock (InitLock)
        {
            if (_provider is not null)
            {
                return;
            }

            var endpoint = RequireEnv("AMP_OTEL_ENDPOINT");
            var apiKey = RequireEnv("AMP_AGENT_API_KEY");

            var resource = ResourceBuilder.CreateDefault()
                .AddService(
                    serviceName: Environment.GetEnvironmentVariable("OTEL_SERVICE_NAME")
                        ?? "amp-dotnet-agent");

            var providerBuilder = Sdk.CreateTracerProviderBuilder()
                .SetResourceBuilder(resource)
                // Always record agent spans. Without this, the default
                // ParentBased sampler drops the agent's spans when they are
                // nested under an unsampled parent (e.g. the ASP.NET Core request
                // activity, which this sample does not instrument).
                .SetSampler(new AlwaysOnSampler())
                .AddSource(SourceName)
                .AddOtlpExporter(options =>
                {
                    options.Endpoint = new Uri(TracesEndpoint(endpoint));
                    options.Protocol = OtlpExportProtocol.HttpProtobuf;
                    // URL-encode: the OTLP exporter parses Headers by splitting
                    // on ',' and '=', so a key containing either would break it.
                    options.Headers = $"x-amp-api-key={Uri.EscapeDataString(apiKey)}";
                });

            // Optional: also print spans to the console for local debugging
            // (the .NET analog of amp-instrumentation's console option).
            if (string.Equals(
                    Environment.GetEnvironmentVariable("AMP_OTEL_CONSOLE"),
                    "true",
                    StringComparison.OrdinalIgnoreCase))
            {
                providerBuilder.AddConsoleExporter();
            }

            _provider = providerBuilder.Build();
        }
    }

    /// <summary>
    /// Flush and shut down the tracer provider so the OTLP batch exporter's
    /// buffered spans are exported before the process exits. Wire this to the
    /// application's shutdown event (see <c>Program.cs</c>). A no-op if
    /// <see cref="Configure"/> was never called.
    /// </summary>
    public static void Shutdown()
    {
        lock (InitLock)
        {
            _provider?.Shutdown();
            _provider?.Dispose();
            _provider = null;
        }
    }

    private static string TracesEndpoint(string baseEndpoint)
    {
        var trimmed = baseEndpoint.TrimEnd('/');
        return trimmed.EndsWith(TracesPath, StringComparison.Ordinal)
            ? trimmed
            : trimmed + TracesPath;
    }

    private static string RequireEnv(string name)
    {
        var value = Environment.GetEnvironmentVariable(name)?.Trim();
        if (string.IsNullOrEmpty(value))
        {
            throw new InvalidOperationException(
                $"Environment variable '{name}' is required but not set.");
        }

        return value;
    }
}
