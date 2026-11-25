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
import { beforeEach, describe, expect, it, vi } from "vitest";

// These pages render real MUI forms, which can be slow on a busy CI runner.
vi.setConfig({ testTimeout: 30000 });
import type { MCPEndpointConfig } from "@agent-management-platform/types";

type PanelProps = {
  items: { key: string; method: string; path: string; summary?: string }[];
  mode: "allow" | "deny";
  onModeChange: (mode: "allow" | "deny") => void;
  exceptionKeys: string[];
  onExceptionKeysChange: (keys: string[]) => void;
  isDirty: boolean;
  readOnly: boolean;
  onSave: () => void;
  onDiscard: () => void;
  status: { message: string } | null;
};

const hoisted = vi.hoisted(() => ({
  canWrite: true,
  catalog: { data: undefined as unknown, isLoading: false },
  panel: null as PanelProps | null,
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
  // A thin stand-in that shows what the tab hands the shared panel and lets the
  // test drive it like a user would.
  AccessControlPanel: (props: PanelProps) => {
    hoisted.panel = props;
    return (
      <div>
        <div>mode: {props.mode}</div>
        <div>exceptions: {props.exceptionKeys.join(",")}</div>
        <div>dirty: {String(props.isDirty)}</div>
        <div>read only: {String(props.readOnly)}</div>
        <ul>
          {props.items.map((item) => (
            <li key={item.key}>
              {item.method} {item.path} {item.summary ?? ""}
            </li>
          ))}
        </ul>
        <button onClick={() => props.onModeChange("deny")}>deny all</button>
        <button
          onClick={() => props.onExceptionKeysChange([...props.exceptionKeys, "tool::search"])}
        >
          except search
        </button>
        <button onClick={props.onSave}>save</button>
        <button onClick={props.onDiscard}>discard</button>
        {props.status && <div>{props.status.message}</div>}
      </div>
    );
  },
}));

import { MCPProxyManageToolsTab } from "./MCPProxyManageToolsTab";

const onUpdate = vi.fn();

const otherPolicy = { name: "rate-limit", version: "v1", params: {} };

const config = {
  id: "primary",
  capabilities: {
    tools: [
      { name: "search", description: "Search repositories" },
      { name: "delete_repo", title: "Delete a repo" },
    ],
    resources: [{ uri: "file:///readme" }],
    prompts: [],
  },
  policies: [otherPolicy],
} as unknown as MCPEndpointConfig;

const renderTab = (tabConfig: MCPEndpointConfig | undefined = config) =>
  render(
    <MCPProxyManageToolsTab
      config={tabConfig}
      selectedEndpointId="primary"
      orgName="acme"
      onUpdate={onUpdate}
      isUpdating={false}
    />,
  );

beforeEach(() => {
  hoisted.canWrite = true;
  hoisted.panel = null;
  hoisted.catalog = {
    data: { data: [{ name: "mcp-acl-list", version: "v2", displayName: "ACL" }] },
    isLoading: false,
  };
  onUpdate.mockReset().mockResolvedValue({});
});

describe("MCPProxyManageToolsTab", () => {
  it("lists every capability with its description and allows all by default", () => {
    renderTab();

    expect(screen.getByText("Tool search Search repositories")).toBeInTheDocument();
    expect(screen.getByText("Tool delete_repo Delete a repo")).toBeInTheDocument();
    expect(screen.getByText("Resource file:///readme")).toBeInTheDocument();
    expect(screen.getByText("mode: allow")).toBeInTheDocument();
    expect(screen.getByText("dirty: false")).toBeInTheDocument();
  });

  it("reads the mode and exceptions from an existing ACL policy", () => {
    renderTab({
      ...config,
      policies: [
        {
          name: "mcp-acl-list",
          version: "v1",
          params: {
            tools: { mode: "DENY", exceptions: ["search"] },
            resources: { mode: "deny", exceptions: ["file:///readme"] },
          },
        },
      ],
    } as unknown as MCPEndpointConfig);

    expect(screen.getByText("mode: deny")).toBeInTheDocument();
    expect(
      screen.getByText("exceptions: tool::search,resource::file:///readme"),
    ).toBeInTheDocument();
  });

  it("saves a deny-list policy for the kinds the endpoint has", async () => {
    renderTab();

    fireEvent.click(screen.getByText("deny all"));
    fireEvent.click(screen.getByText("except search"));
    expect(screen.getByText("dirty: true")).toBeInTheDocument();
    fireEvent.click(screen.getByText("save"));

    await waitFor(() =>
      expect(onUpdate).toHaveBeenCalledWith({
        policies: [
          otherPolicy,
          {
            name: "mcp-acl-list",
            version: "v2",
            displayName: "ACL",
            params: {
              tools: { mode: "deny", exceptions: ["search"] },
              resources: { mode: "deny", exceptions: [] },
            },
          },
        ],
      }),
    );
    expect(
      await screen.findByText("Access control updated successfully."),
    ).toBeInTheDocument();
    expect(screen.getByText("dirty: false")).toBeInTheDocument();
  });

  it("replaces an existing ACL policy in place", async () => {
    renderTab({
      ...config,
      policies: [
        { name: "mcp-acl-list", version: "v1", params: { tools: { mode: "allow" } } },
        otherPolicy,
      ],
    } as unknown as MCPEndpointConfig);

    fireEvent.click(screen.getByText("deny all"));
    fireEvent.click(screen.getByText("save"));

    await waitFor(() => expect(onUpdate).toHaveBeenCalledTimes(1));
    const policies = onUpdate.mock.calls[0][0].policies;
    expect(policies.map((p: { name: string }) => p.name)).toEqual([
      "mcp-acl-list",
      "rate-limit",
    ]);
  });

  it("reports a failed save", async () => {
    onUpdate.mockRejectedValue(new Error("boom"));
    renderTab();

    fireEvent.click(screen.getByText("deny all"));
    fireEvent.click(screen.getByText("save"));

    expect(await screen.findByText("Failed to update access control.")).toBeInTheDocument();
  });

  it("discards back to the saved rules", () => {
    renderTab();

    fireEvent.click(screen.getByText("deny all"));
    fireEvent.click(screen.getByText("discard"));

    expect(screen.getByText("mode: allow")).toBeInTheDocument();
    expect(screen.getByText("dirty: false")).toBeInTheDocument();
  });

  it("warns and refuses to save when the gateway lacks the ACL policy", () => {
    hoisted.catalog = { data: { data: [] }, isLoading: false };
    renderTab();

    expect(
      screen.getByText(/is not reported as available by the active gateway/),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByText("deny all"));
    expect(screen.getByText("dirty: false")).toBeInTheDocument();
    fireEvent.click(screen.getByText("save"));

    expect(
      screen.getByText("Access control policy is not available on the active gateway."),
    ).toBeInTheDocument();
    expect(onUpdate).not.toHaveBeenCalled();
  });

  it("explains when there are no capabilities", () => {
    renderTab({ id: "primary", capabilities: { tools: [] } } as unknown as MCPEndpointConfig);

    expect(screen.getByText("No Capabilities Available")).toBeInTheDocument();
  });

  it("passes read-only to the panel without write access", () => {
    hoisted.canWrite = false;
    renderTab();

    expect(screen.getByText("read only: true")).toBeInTheDocument();
  });
});
