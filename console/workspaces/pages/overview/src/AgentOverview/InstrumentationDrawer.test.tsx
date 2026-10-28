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
import { describe, expect, it, vi } from "vitest";

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
}));

vi.mock("./TokenGenerationStep", () => ({
  TokenGenerationStep: ({ stepNumber }: { stepNumber: number }) => (
    <div data-testid="token-step">{stepNumber}</div>
  ),
}));

import { InstrumentationDrawer } from "./InstrumentationDrawer";

const ENDPOINT = "http://default-default.gateway.localhost:19080/otel";

const renderDrawer = () =>
  render(
    <InstrumentationDrawer
      open
      onClose={() => {}}
      agentId="agent"
      orgName="org"
      projName="proj"
      agentName="agent"
      instrumentationUrl={ENDPOINT}
      apiKey="test-key"
    />,
  );

describe("InstrumentationDrawer", () => {
  it("shows generic OTLP exporter settings when Other is selected", () => {
    renderDrawer();

    fireEvent.mouseDown(screen.getByRole("combobox"));
    fireEvent.click(within(screen.getByRole("listbox")).getByText("Other"));

    expect(screen.getByText("OpenTelemetry Export Guide")).toBeInTheDocument();
    expect(screen.getByTestId("token-step")).toHaveTextContent("1");
    const text = document.body.textContent ?? "";
    expect(text).toContain(`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="${ENDPOINT}/v1/traces"`);
    expect(text).toContain('OTEL_EXPORTER_OTLP_TRACES_PROTOCOL="http/protobuf"');
    expect(text).toContain('OTEL_EXPORTER_OTLP_TRACES_HEADERS="x-amp-api-key=test-key"');
    expect(text).not.toContain("amp-instrument");
  });
});
