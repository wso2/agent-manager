# Build stage
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS build
WORKDIR /src
COPY dotnet-agent.csproj .
RUN dotnet restore
COPY . .
RUN dotnet publish -c Release -o /app

# Runtime stage
FROM mcr.microsoft.com/dotnet/aspnet:10.0
WORKDIR /app
COPY --from=build /app .

# Run as the non-root user the .NET runtime images provide.
USER $APP_UID

# The agent listens on 8000 (see Program.cs).
EXPOSE 8000

# --- Optional: OpenTelemetry .NET zero-code (CoreCLR) overlay --------------
# The lines below add automatic infrastructure spans (ASP.NET Core, HttpClient,
# DB) *around* the gen_ai.* spans the app already emits. They are NOT required
# for AMP telemetry — the app owns its OTLP exporter and gen_ai spans via
# AmpTelemetry/Agent Framework. Uncomment to enable, and install the auto-
# instrumentation into the image (see the .NET section of the AMP instrumentation
# docs). The profiler alone produces no gen_ai.* spans.
#
# ENV CORECLR_ENABLE_PROFILING=1 \
#     CORECLR_PROFILER={918728DD-259F-4A6A-AC2B-B85E1B658318} \
#     CORECLR_PROFILER_PATH=/otel-dotnet-auto/linux-x64/OpenTelemetry.AutoInstrumentation.Native.so \
#     DOTNET_STARTUP_HOOKS=/otel-dotnet-auto/net/OpenTelemetry.AutoInstrumentation.StartupHook.dll \
#     OTEL_DOTNET_AUTO_HOME=/otel-dotnet-auto \
#     OTEL_DOTNET_AUTO_TRACES_ADDITIONAL_SOURCES=amp-dotnet-agent
# NOTE: use linux-musl-x64 instead of linux-x64 on Alpine base images.

ENTRYPOINT ["dotnet", "dotnet-agent.dll"]
