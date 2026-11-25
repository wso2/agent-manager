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

const hoisted = vi.hoisted(() => ({
  canWrite: true,
  catalog: { data: undefined as unknown, isLoading: false },
}));

vi.mock("@agent-management-platform/api-client", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useMCPPoliciesCatalog: () => hoisted.catalog,
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canRead: true, canWrite: hoisted.canWrite }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));

import { MCPProxyRewriteTab } from "./MCPProxyRewriteTab";

const onUpdate = vi.fn();

const otherPolicy = { name: "rate-limit", version: "v1", params: { rps: 5 } };

const capabilities = {
  tools: [
    {
      name: "search",
      description: "Search repositories",
      inputSchema: { type: "object" },
    },
  ],
  resources: [{ uri: "file:///readme", description: "" }],
  prompts: [{ name: "summarize" }],
};

const config = {
  id: "primary",
  capabilities,
  policies: [otherPolicy],
} as unknown as MCPEndpointConfig;

const configWithRewrite = {
  id: "primary",
  capabilities,
  policies: [
    otherPolicy,
    {
      name: "mcp-rewrite",
      version: "v1",
      params: {
        tools: [{ name: "find_repos", description: "Find repos", target: "search" }],
        resources: [{ uri: "docs://readme", target: "file:///readme" }],
        prompts: [{ name: "tldr", target: "summarize", description: "Short summary" }],
      },
    },
  ],
} as unknown as MCPEndpointConfig;

const renderTab = (tabConfig: MCPEndpointConfig | undefined = config) =>
  render(
    <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <MCPProxyRewriteTab
        config={tabConfig}
        selectedEndpointId="primary"
        orgName="acme"
        onUpdate={onUpdate}
        isUpdating={false}
      />
    </MemoryRouter>,
  );

const toggle = () =>
  document.querySelector('input[type="checkbox"]') as HTMLInputElement;

beforeEach(() => {
  hoisted.canWrite = true;
  hoisted.catalog = {
    data: { data: [{ name: "mcp-rewrite", version: "v1", displayName: "Rewrite" }] },
    isLoading: false,
  };
  onUpdate.mockReset().mockResolvedValue({});
});

describe("MCPProxyRewriteTab", () => {
  it("starts disabled when the endpoint has no rewrite policy", () => {
    renderTab();

    expect(screen.getByText("Disabled")).toBeInTheDocument();
    expect(toggle()).not.toBeChecked();
    expect(screen.queryByLabelText("Name")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("enables rewriting and saves the edited names next to other policies", async () => {
    renderTab();

    fireEvent.click(toggle());
    expect(screen.getByText("Enabled")).toBeInTheDocument();
    // The first tool is selected by default.
    expect(screen.getByLabelText("Name")).toHaveValue("search");
    expect(screen.getByLabelText("Description")).toHaveValue("Search repositories");

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "find_repos" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
    expect(onUpdate).toHaveBeenCalledWith({
      policies: [
        otherPolicy,
        {
          name: "mcp-rewrite",
          version: "v1",
          displayName: "Rewrite",
          params: {
            tools: [
              {
                name: "find_repos",
                description: "Search repositories",
                inputSchema: JSON.stringify({ type: "object" }, null, 2),
                target: "search",
              },
            ],
            // A blank description is left out rather than sent empty.
            resources: [
              { name: "file:///readme", uri: "file:///readme", target: "file:///readme" },
            ],
            prompts: [{ name: "summarize", target: "summarize" }],
          },
        },
      ],
    });
    expect(
      await screen.findByText("Rewrite policy updated successfully."),
    ).toBeInTheDocument();
  });

  it("falls back to required defaults for empty tool fields", async () => {
    renderTab();

    fireEvent.click(toggle());
    fireEvent.change(screen.getByLabelText("Description"), { target: { value: " " } });
    fireEvent.change(screen.getByLabelText("Input Schema"), { target: { value: "" } });
    fireEvent.click(screen.getByText("Advanced"));
    fireEvent.change(screen.getByLabelText("Output Schema"), {
      target: { value: '{"type":"string"}' },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
    const rewrite = onUpdate.mock.calls[0][0].policies[1];
    expect(rewrite.params.tools[0]).toEqual({
      name: "search",
      description: "search",
      inputSchema: '{"type":"object"}',
      outputSchema: '{"type":"string"}',
      target: "search",
    });
  });

  it("loads an existing rewrite and edits resources and prompts", async () => {
    renderTab(configWithRewrite);

    expect(toggle()).toBeChecked();
    expect(screen.getByLabelText("Name")).toHaveValue("find_repos");

    fireEvent.click(screen.getByText("file:///readme"));
    expect(screen.getByLabelText("URI")).toHaveValue("docs://readme");

    fireEvent.click(screen.getByText("summarize"));
    expect(screen.getByLabelText("Name")).toHaveValue("tldr");
    expect(screen.getByLabelText("Description")).toHaveValue("Short summary");
    fireEvent.change(screen.getByLabelText("Description"), {
      target: { value: "One-line summary" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
    const policies = onUpdate.mock.calls[0][0].policies;
    expect(policies).toHaveLength(2);
    expect(policies[1].params.prompts).toEqual([
      { name: "tldr", target: "summarize", description: "One-line summary" },
    ]);
    expect(policies[1].params.resources).toEqual([
      { name: "docs://readme", uri: "docs://readme", target: "file:///readme" },
    ]);
  });

  it("removes the rewrite policy when disabled", async () => {
    renderTab(configWithRewrite);

    fireEvent.click(toggle());
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(onUpdate).toHaveBeenCalledWith({ policies: [otherPolicy] }),
    );
    expect(await screen.findByText("Rewrite policy disabled.")).toBeInTheDocument();
  });

  it("reports a failed save", async () => {
    onUpdate.mockRejectedValue(new Error("boom"));
    renderTab(configWithRewrite);

    fireEvent.click(toggle());
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(await screen.findByText("Failed to update rewrite policy.")).toBeInTheDocument();
  });

  it("discards unsaved edits", () => {
    renderTab(configWithRewrite);

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "other" } });
    fireEvent.click(screen.getByRole("button", { name: "Discard" }));

    expect(screen.getByLabelText("Name")).toHaveValue("find_repos");
  });

  it("blocks saving when the gateway does not offer the rewrite policy", () => {
    hoisted.catalog = { data: { data: [] }, isLoading: false };
    renderTab();

    fireEvent.click(toggle());

    expect(
      screen.getByText(/is not reported as available by the active gateway/),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("explains when there is nothing to rewrite", () => {
    renderTab({ id: "primary", capabilities: {} } as unknown as MCPEndpointConfig);

    expect(screen.getByText("No Capabilities Available")).toBeInTheDocument();
  });

  it("is read-only without write access", () => {
    hoisted.canWrite = false;
    renderTab(configWithRewrite);

    expect(screen.getByLabelText("Name")).toBeDisabled();
    expect(toggle()).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("shows no editor while the catalog loads", () => {
    hoisted.catalog = { data: undefined, isLoading: true };
    renderTab(configWithRewrite);

    expect(screen.queryByText("Rewrite")).not.toBeInTheDocument();
  });
});
