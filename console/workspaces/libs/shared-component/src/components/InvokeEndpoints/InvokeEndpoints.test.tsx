/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { InvokeEndpoints, type InvokeEndpoint } from "./InvokeEndpoints";

const endpoints: InvokeEndpoint[] = [
  { gatewayId: "gw-dev", url: "https://dev/x", displayName: "Dev GW", environment: { id: "dev", displayName: "Development" } },
  { gatewayId: "gw-none", url: "https://none/x" },
  { gatewayId: "gw-prod", url: "https://prod/x", name: "prod-gw", environment: { id: "prod", name: "Prod", isProduction: true } },
  { gatewayId: "gw-prod-2", url: "https://prod2/x", environment: { id: "prod", name: "Prod", isProduction: true } },
  { gatewayId: "gw-alpha", url: "https://alpha/x", environment: { name: "Alpha Environment" } },
];

function setup(props: Partial<React.ComponentProps<typeof InvokeEndpoints>> = {}) {
  const handlers = { onGenerateApiKey: vi.fn(), onClearApiKeyError: vi.fn() };
  render(
    <ThemeProvider theme={createTheme()}>
      <InvokeEndpoints
        endpoints={endpoints}
        isGeneratingApiKey={false}
        apiKeyError={null}
        generatedApiKey={null}
        {...handlers}
        {...props}
      />
    </ThemeProvider>,
  );
  return handlers;
}

describe("InvokeEndpoints", () => {
  const writeText = vi.fn();
  beforeEach(() => {
    writeText.mockReset().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
  });
  afterEach(() => vi.useRealTimers());

  it("shows the empty message when nothing is deployed", () => {
    setup({ endpoints: [], emptyMessage: "Not deployed yet" });
    expect(screen.getByText("Not deployed yet")).toBeInTheDocument();
    expect(screen.queryByText("Generate API Key")).not.toBeInTheDocument();
  });

  it("groups endpoints by environment, production first and unassigned last", () => {
    setup();
    const headings = screen
      .getAllByText(/Environment$|^Unassigned$/)
      .map((el) => el.textContent);
    expect(headings).toEqual([
      "Prod Environment",
      "Alpha Environment",
      "Development Environment",
      "Unassigned",
    ]);
    expect(screen.getByText("Production")).toBeInTheDocument();
    // Label falls back from display name to name to gateway id.
    expect(screen.getByText("Dev GW")).toBeInTheDocument();
    expect(screen.getByText("prod-gw")).toBeInTheDocument();
    expect(screen.getByText("gw-prod-2")).toBeInTheDocument();
    expect(screen.getByDisplayValue("https://prod2/x")).toBeInTheDocument();
  });

  it("copies an invoke URL", async () => {
    setup();
    await act(async () => {
      fireEvent.click(screen.getAllByRole("button", { name: "Copy Invoke URL" })[0]);
    });
    expect(writeText).toHaveBeenCalledWith("https://prod/x");
  });

  it("ignores a denied clipboard", async () => {
    writeText.mockRejectedValue(new Error("denied"));
    setup({ generatedApiKey: "sk-1" });
    await act(async () => {
      fireEvent.click(screen.getAllByRole("button", { name: "Copy Invoke URL" })[0]);
      fireEvent.click(screen.getByRole("button", { name: "Copy API Key" }));
    });
    expect(writeText).toHaveBeenCalledTimes(2);
  });

  it("does not copy an endpoint without a URL", async () => {
    setup({ endpoints: [{ gatewayId: "gw", url: "" }] });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Copy Invoke URL" }));
    });
    expect(writeText).not.toHaveBeenCalled();
  });

  it("generates an API key and shows it once with a copy action", async () => {
    const { onGenerateApiKey } = setup({ generatedApiKey: "sk-generated" });
    fireEvent.click(screen.getByRole("button", { name: "Generate API key" }));
    expect(onGenerateApiKey).toHaveBeenCalled();
    expect(screen.getByText("API Key Generated")).toBeInTheDocument();
    expect(screen.getByDisplayValue("sk-generated")).toBeInTheDocument();
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Copy API Key" }));
    });
    expect(writeText).toHaveBeenCalledWith("sk-generated");
  });

  it("shows a dismissable key error and a busy state", () => {
    const { onClearApiKeyError } = setup({ apiKeyError: "No permission", isGeneratingApiKey: true });
    expect(screen.getByText("No permission")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClearApiKeyError).toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Generating..." })).toBeDisabled();
  });

  it("hides key generation when read-only", () => {
    setup({ readOnly: true });
    expect(screen.queryByText("Generate API Key")).not.toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Copy Invoke URL" }).length).toBe(5);
  });
});
