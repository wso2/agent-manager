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

using System.Diagnostics;
using System.Text.Json.Serialization;
using AmpDotnetAgent;

// Wire the OpenTelemetry exporter to AMP before creating the agent, exactly like
// the manual-instrumentation Python sample calls init_otel() in app.py.
AmpTelemetry.Configure();

var agent = AgentFactory.Create();

var builder = WebApplication.CreateBuilder(args);
var app = builder.Build();

// Flush buffered spans to AMP before the process exits.
app.Lifetime.ApplicationStopping.Register(AmpTelemetry.Shutdown);

app.MapGet("/healthz", () => Results.Ok("ok"));

// One /chat request -> one trace (invoke_agent -> chat -> execute_tool).
app.MapPost("/chat", async (ChatRequest req) =>
{
    // Detach from ASP.NET Core's request activity. It isn't exported, so leaving
    // it as the parent would make invoke_agent an orphan and AMP would find no
    // root span for the trace. With no current activity, invoke_agent is the root.
    Activity.Current = null;

    var response = await agent.RunAsync(req.Message);
    return Results.Ok(new ChatResponse(req.SessionId, response.Text));
});

app.Run("http://0.0.0.0:8000");

public record ChatRequest(
    [property: JsonPropertyName("session_id")] string? SessionId,
    [property: JsonPropertyName("message")] string Message);

public record ChatResponse(
    [property: JsonPropertyName("session_id")] string? SessionId,
    [property: JsonPropertyName("response")] string Response);
