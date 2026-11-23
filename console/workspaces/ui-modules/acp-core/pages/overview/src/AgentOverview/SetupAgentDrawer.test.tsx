/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const manifest = vi.hoisted(() => ({ modules: [] as { name: string }[] }));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  moduleBase: (module: string) => `/m/${module}`,
  useTrack: () => ({ track: vi.fn() }),
  useUIManifest: () => ({ data: { modules: manifest.modules, pages: [] } }),
  useGenerateAgentToken: () => ({
    data: undefined,
    error: null,
    isFetching: false,
    refetch: vi.fn(),
  }),
}));

vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureAccess: () => true,
}));

import { SetupAgentDrawer, traceEndpoint } from "./SetupAgentDrawer";

const renderDrawer = () =>
  render(
    <SetupAgentDrawer open onClose={() => {}} orgName="org" projName="proj" agentName="agent" />,
  );

describe("SetupAgentDrawer", () => {
  beforeEach(() => {
    manifest.modules = [{ name: "core" }];
  });

  it("offers only the agent's API key without the observability module", () => {
    renderDrawer();

    expect(screen.getByText("Generate API Key")).toBeInTheDocument();
    expect(screen.getByText("Generate an API key for this agent.")).toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: /language/i })).not.toBeInTheDocument();
    const text = document.body.textContent ?? "";
    expect(text).not.toMatch(/trace|OTEL|amp-instrument/i);
  });

  it("shows the Python guide with the module's endpoint while it is active", () => {
    manifest.modules = [{ name: "core" }, { name: "observability" }];
    renderDrawer();

    expect(screen.getByText("Zero-code Instrumentation Guide")).toBeInTheDocument();
    const text = document.body.textContent ?? "";
    expect(text).toMatch(/export AMP_OTEL_ENDPOINT="[^"]*\/m\/observability\/otel"/);
    expect(text).toContain('export AMP_AGENT_API_KEY="<your-api-key>"');
    expect(text).toContain("amp-instrument <run_command>");
  });

  it("shows generic OTLP exporter settings when Other is selected", () => {
    manifest.modules = [{ name: "observability" }];
    renderDrawer();

    fireEvent.mouseDown(screen.getByRole("combobox", { name: /language/i }));
    fireEvent.click(within(screen.getByRole("listbox")).getByText("Other"));

    expect(screen.getByText("OpenTelemetry Export Guide")).toBeInTheDocument();
    const text = document.body.textContent ?? "";
    const traces = /OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="[^"]*\/m\/observability\/otel\/v1\/traces"/;
    expect(text).toMatch(traces);
    expect(text).toContain('OTEL_EXPORTER_OTLP_TRACES_HEADERS="x-amp-api-key=<your-api-key>"');
    expect(text).not.toContain("amp-instrument");
  });
});

describe("traceEndpoint", () => {
  it("is the observability module beside the API", () => {
    expect(traceEndpoint("http://localhost:9000", "http://console"))
      .toBe("http://localhost:9000/m/observability/otel");
    expect(traceEndpoint("https://amp.example.com/", "http://console"))
      .toBe("https://amp.example.com/m/observability/otel");
  });

  it("uses the console's origin when the API is on it", () => {
    expect(traceEndpoint("", "http://localhost:9000"))
      .toBe("http://localhost:9000/m/observability/otel");
    expect(traceEndpoint("/amp", "https://host")).toBe("https://host/amp/m/observability/otel");
  });
});
