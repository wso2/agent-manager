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

import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

// These pages render real MUI forms, which can be slow on a busy CI runner.
vi.setConfig({ testTimeout: 30000 });

const hoisted = vi.hoisted(() => ({
  canWrite: true,
  list: {
    data: undefined as unknown,
    isLoading: false,
    error: null as unknown,
  },
  deleteProxy: vi.fn(),
  addConfirmation: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useListMCPProxies: () => hoisted.list,
  useDeleteMCPProxy: () => ({ mutate: hoisted.deleteProxy, isPending: false }),
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canRead: true, canWrite: hoisted.canWrite }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useConfirmationDialog: () => ({ addConfirmation: hoisted.addConfirmation }),
}));
vi.mock("./subComponents/AddMCPProxyForm", () => ({
  AddMCPProxyForm: ({ onCancel }: { onCancel: () => void }) => (
    <button onClick={onCancel}>add form cancel</button>
  ),
}));
vi.mock("./subComponents/ViewMCPProxy", () => ({
  ViewMCPProxy: () => <div>view page</div>,
}));
vi.mock("./subComponents/EditMCPProxy", () => ({
  EditMCPProxy: () => <div>edit page</div>,
}));

import { MCPProxiesOrganization } from "./MCPProxies.Organization";

const Location = () => <div data-testid="location">{useLocation().pathname}</div>;

const renderAt = (path: string) =>
  render(
    <MemoryRouter
      initialEntries={[path]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route
          path="/org/:orgId/mcp-proxies/*"
          element={
            <>
              <MCPProxiesOrganization />
              <Location />
            </>
          }
        />
        <Route path="*" element={<Location />} />
      </Routes>
    </MemoryRouter>,
  );

const proxies = [
  {
    id: "github",
    name: "GitHub MCP",
    version: "1.0.0",
    context: "/default/github",
    description: "Repository tools",
  },
  { id: "jira", name: "Jira MCP", version: "2.1.0", context: "/default/jira" },
];

beforeEach(() => {
  hoisted.canWrite = true;
  hoisted.list = { data: { list: proxies }, isLoading: false, error: null };
  hoisted.deleteProxy.mockReset();
  hoisted.addConfirmation.mockReset();
});

describe("MCPProxiesOrganization list", () => {
  it("lists MCP servers with their versions and a register action", () => {
    renderAt("/org/acme/mcp-proxies");

    expect(screen.getByText("MCP Servers")).toBeInTheDocument();
    expect(screen.getByText("GitHub MCP")).toBeInTheDocument();
    expect(screen.getByText("Jira MCP")).toBeInTheDocument();
    expect(screen.getByText("2.1.0")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Register MCP server" }),
    ).toHaveAttribute("href", "/org/acme/mcp-proxies/add");
  });

  it("filters the list by the search term", () => {
    renderAt("/org/acme/mcp-proxies");

    fireEvent.change(screen.getByPlaceholderText("Search MCP servers"), {
      target: { value: "repository" },
    });

    expect(screen.getByText("GitHub MCP")).toBeInTheDocument();
    expect(screen.queryByText("Jira MCP")).not.toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Search MCP servers"), {
      target: { value: "nothing-matches" },
    });
    expect(
      screen.getByText("No MCP servers match your search"),
    ).toBeInTheDocument();
  });

  it("opens a server's page when its row is clicked", () => {
    renderAt("/org/acme/mcp-proxies");

    fireEvent.click(screen.getByText("Jira MCP"));

    expect(screen.getByText("view page")).toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/org/acme/mcp-proxies/view/jira",
    );
  });

  it("asks for confirmation and deletes the server on confirm", () => {
    renderAt("/org/acme/mcp-proxies");

    fireEvent.click(screen.getAllByRole("button", { name: "Delete MCP Server" })[0]);

    expect(hoisted.addConfirmation).toHaveBeenCalledTimes(1);
    const confirmation = hoisted.addConfirmation.mock.calls[0][0];
    expect(confirmation.title).toBe("Delete MCP Server");
    expect(confirmation.description).toContain("GitHub MCP");
    expect(hoisted.deleteProxy).not.toHaveBeenCalled();

    confirmation.onConfirm();
    expect(hoisted.deleteProxy).toHaveBeenCalledWith({
      orgName: "acme",
      proxyId: "github",
    });
    // The click on the delete cell must not also open the row.
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/org/acme/mcp-proxies",
    );
  });

  it("shows the empty state with the register action when there are no servers", () => {
    hoisted.list = { data: { list: [] }, isLoading: false, error: null };
    renderAt("/org/acme/mcp-proxies");

    expect(screen.getByText("No MCP Servers Yet")).toBeInTheDocument();
    // Only the empty-state CTA, not the header action.
    expect(
      screen.getAllByRole("link", { name: "Register MCP server" }),
    ).toHaveLength(1);
  });

  it("shows the load error", () => {
    hoisted.list = {
      data: undefined,
      isLoading: false,
      error: new Error("backend unavailable"),
    };
    renderAt("/org/acme/mcp-proxies");

    expect(screen.getByText(/backend unavailable/)).toBeInTheDocument();
    expect(screen.queryByText("GitHub MCP")).not.toBeInTheDocument();
  });

  it("hides register and delete actions without write access", () => {
    hoisted.canWrite = false;
    renderAt("/org/acme/mcp-proxies");

    expect(screen.getByText("GitHub MCP")).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Register MCP server" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Delete MCP Server" }),
    ).not.toBeInTheDocument();
  });
});

describe("MCPProxiesOrganization routes", () => {
  it("renders the register page and returns to the list on cancel", () => {
    renderAt("/org/acme/mcp-proxies/add");

    expect(screen.getByText("Register MCP Server")).toBeInTheDocument();
    fireEvent.click(screen.getByText("add form cancel"));

    expect(screen.getByTestId("location")).toHaveTextContent(
      /^\/org\/acme\/mcp-proxies$/,
    );
  });

  it("redirects the register page to the list without write access", () => {
    hoisted.canWrite = false;
    renderAt("/org/acme/mcp-proxies/add");
    expect(screen.queryByText("Register MCP Server")).not.toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent(
      /^\/org\/acme\/mcp-proxies$/,
    );
  });

  it("does not render the edit page without write access", () => {
    hoisted.canWrite = false;
    renderAt("/org/acme/mcp-proxies/view/github/edit");

    expect(screen.queryByText("edit page")).not.toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent(
      /^\/org\/acme\/mcp-proxies$/,
    );
  });

  it("renders the edit page with write access", () => {
    renderAt("/org/acme/mcp-proxies/view/github/edit");

    expect(screen.getByText("edit page")).toBeInTheDocument();
  });

  it("redirects unknown paths to the list", () => {
    renderAt("/org/acme/mcp-proxies/does-not-exist");

    expect(screen.getByTestId("location")).toHaveTextContent(
      /^\/org\/acme\/mcp-proxies$/,
    );
    expect(screen.getByText("GitHub MCP")).toBeInTheDocument();
  });
});
