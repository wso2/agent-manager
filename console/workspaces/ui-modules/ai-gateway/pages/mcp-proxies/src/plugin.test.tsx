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

import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ResourceSummaryState } from "@agent-management-platform/types";

vi.setConfig({ testTimeout: 30000 });

const hoisted = vi.hoisted(() => ({
  canWrite: true,
  access: new Set<string>(),
  list: { data: undefined as unknown, isLoading: false },
}));

vi.mock("@agent-management-platform/api-client", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useListMCPProxies: () => hoisted.list,
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canRead: true, canWrite: hoisted.canWrite }),
  useFeatureManagement: () => ({ hasAccess: (key: string) => hoisted.access.has(key) }),
}));

import { mcpProxiesPlugin } from "./plugin";

const contribution = (type: string) =>
  mcpProxiesPlugin.contributions.find((c) => c.type === type) as unknown as {
    useSummary?: () => ResourceSummaryState;
    useActivation?: () => boolean;
  };

const Summary = () => {
  const summary = contribution("resource-summary").useSummary!();
  return (
    <div>
      {`count=${summary.count} loading=${summary.isLoading} label=${summary.actionLabel} to=${summary.to}`}
    </div>
  );
};

const Activation = () => (
  <div>{`active=${contribution("nav-item").useActivation!()}`}</div>
);

const renderAtOrg = (element: ReactNode) =>
  render(
    <MemoryRouter
      initialEntries={["/org/acme"]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route path="/org/:orgId" element={element} />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  hoisted.canWrite = true;
  hoisted.access = new Set();
  hoisted.list = { data: { list: [], count: 3, pagination: { count: 12 } }, isLoading: false };
});

describe("mcpProxiesPlugin", () => {
  it("summarises the org-wide server count and links writers to register one", () => {
    renderAtOrg(<Summary />);

    expect(
      screen.getByText(
        "count=12 loading=false label=Register an MCP server to=/org/acme/mcp-proxies/add",
      ),
    ).toBeInTheDocument();
  });

  it("links readers to the server list and falls back to the page count", () => {
    hoisted.canWrite = false;
    hoisted.list = { data: { list: [], count: 3 }, isLoading: true };
    renderAtOrg(<Summary />);

    expect(
      screen.getByText(
        "count=3 loading=true label=View MCP servers to=/org/acme/mcp-proxies",
      ),
    ).toBeInTheDocument();
  });

  it("shows the Resources nav when any of its resources is readable", () => {
    const { unmount } = renderAtOrg(<Activation />);
    expect(screen.getByText("active=false")).toBeInTheDocument();
    unmount();

    hoisted.access = new Set(["mcp_server_read"]);
    renderAtOrg(<Activation />);
    expect(screen.getByText("active=true")).toBeInTheDocument();
  });

  it("loads the organization page lazily", async () => {
    const route = mcpProxiesPlugin.contributions.find((c) => c.type === "route") as unknown as {
      path: string;
      load: () => Promise<{ default: unknown }>;
    };

    expect(route.path).toBe("/org/:orgId/mcp-proxies/*");
    const { MCPProxiesOrganization } = await import("./MCPProxies.Organization");
    await expect(route.load()).resolves.toEqual({ default: MCPProxiesOrganization });
  });
});
