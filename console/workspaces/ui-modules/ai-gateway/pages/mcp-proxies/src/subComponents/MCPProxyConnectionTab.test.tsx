/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

// These pages render real MUI forms, which can be slow on a busy CI runner.
vi.setConfig({ testTimeout: 30000 });
import type { MCPEndpointConfig } from "@agent-management-platform/types";

const hoisted = vi.hoisted(() => ({ canWrite: true }));

vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canRead: true, canWrite: hoisted.canWrite }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));

import { MCPProxyConnectionTab } from "./MCPProxyConnectionTab";

const onUpdate = vi.fn();

const config = {
  id: "primary",
  upstream: {
    main: {
      url: "https://mcp.example.com/mcp",
      auth: { type: "api-key", header: "Authorization" },
    },
  },
  resilience: { timeout: "10s" },
} as unknown as MCPEndpointConfig;

const renderTab = (
  tabConfig: MCPEndpointConfig | undefined,
  isLoading = false,
) =>
  render(
    <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <MCPProxyConnectionTab
        config={tabConfig}
        selectedEndpointId="primary"
        isLoading={isLoading}
        onUpdate={onUpdate}
        isUpdating={false}
      />
    </MemoryRouter>,
  );

const urlField = () => screen.getByLabelText("MCP Server Endpoint URL");

beforeEach(() => {
  hoisted.canWrite = true;
  onUpdate.mockReset().mockResolvedValue({});
});

describe("MCPProxyConnectionTab", () => {
  it("shows the saved connection with the credential masked", () => {
    renderTab(config);

    expect(urlField()).toHaveValue("https://mcp.example.com/mcp");
    expect(screen.getByPlaceholderText("Key")).toHaveValue("Authorization");
    expect(screen.getByPlaceholderText("Value")).toHaveValue("••••••••••••");
    expect(screen.getByLabelText("Request Timeout")).toHaveValue("10s");
    expect(screen.getByText(/The stored value is hidden/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Discard" })).toBeDisabled();
  });

  it("saves a new URL and keeps the stored credential", async () => {
    renderTab(config);

    fireEvent.change(urlField(), { target: { value: " https://mcp.example.com/v2 " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(onUpdate).toHaveBeenCalledWith({
        upstream: {
          main: {
            url: "https://mcp.example.com/v2",
            auth: { type: "api-key", header: "Authorization" },
          },
        },
        resilience: { timeout: "10s", idleTimeout: undefined },
      }),
    );
    expect(await screen.findByText("Connection updated successfully.")).toBeInTheDocument();
  });

  it("replaces the credential once a new value is typed", async () => {
    renderTab(config);

    const valueField = screen.getByPlaceholderText("Value");
    fireEvent.focus(valueField);
    expect(valueField).toHaveValue("");
    fireEvent.change(valueField, { target: { value: "Bearer new" } });
    fireEvent.click(screen.getByRole("button", { name: "Show header value" }));
    expect(valueField).toHaveAttribute("type", "text");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
    expect(onUpdate.mock.calls[0][0].upstream.main.auth).toEqual({
      type: "api-key",
      header: "Authorization",
      value: "Bearer new",
    });
    // The saved credential is masked again.
    await waitFor(() => expect(valueField).toHaveValue("••••••••••••"));
  });

  it("drops the auth header when it is unchecked", async () => {
    renderTab(config);

    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.change(screen.getByLabelText("Idle Timeout"), { target: { value: "30s" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
    expect(onUpdate.mock.calls[0][0]).toEqual({
      upstream: { main: { url: "https://mcp.example.com/mcp", auth: undefined } },
      resilience: { timeout: "10s", idleTimeout: "30s" },
    });
  });

  it("validates the URL and timeouts", () => {
    renderTab(config);

    fireEvent.change(urlField(), { target: { value: "" } });
    fireEvent.blur(urlField());
    expect(screen.getByText("MCP Server Endpoint URL is required")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();

    fireEvent.change(urlField(), { target: { value: "https://ok.example.com" } });
    expect(screen.queryByText("MCP Server Endpoint URL is required")).not.toBeInTheDocument();

    const timeout = screen.getByLabelText("Request Timeout");
    fireEvent.change(timeout, { target: { value: "ten" } });
    fireEvent.blur(timeout);
    expect(screen.getByText("Enter a duration like 5s, 500ms, or 1m")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("reports a failed save", async () => {
    onUpdate.mockRejectedValue(new Error("boom"));
    renderTab(config);

    fireEvent.change(urlField(), { target: { value: "https://mcp.example.com/v2" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Failed to update connection.")).toBeInTheDocument();
  });

  it("discards edits", () => {
    renderTab(config);

    fireEvent.change(urlField(), { target: { value: "https://other.example.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Discard" }));

    expect(urlField()).toHaveValue("https://mcp.example.com/mcp");
  });

  it("is read-only without write access", () => {
    hoisted.canWrite = false;
    renderTab(config);

    expect(urlField()).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("renders no form while loading or without a config", () => {
    const { unmount } = renderTab(config, true);
    expect(screen.queryByLabelText("MCP Server Endpoint URL")).not.toBeInTheDocument();
    unmount();

    renderTab(undefined);
    expect(screen.queryByLabelText("MCP Server Endpoint URL")).not.toBeInTheDocument();
  });
});
