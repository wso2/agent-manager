/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

const idle = { data: undefined, isLoading: false, isError: false };
// Stable references: fresh objects per render would retrigger the page's effects.
const configResult = {
  data: {
    name: "",
    type: "mcp",
    envMappings: { dev: { configuration: { proxyName: "proj-proxy" } } },
  },
  isLoading: false,
  isError: false,
};
const envsResult = { data: [], isError: false };
const proxiesResult = {
  data: { list: [{ id: "proj-proxy", name: "Project Proxy Display", projectName: "p1" }] },
  isLoading: false,
};
const orgProxiesResult = {
  data: { list: [{ id: "proj-proxy", name: "Org Proxy Display" }] },
  isLoading: false,
};
const pipelineResult = { environments: [], isLoading: false };
const mutationResult = { mutate: vi.fn(), isPending: false };

vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: vi.fn(() => ({ data: undefined })),
  useGetAgentMCPConfig: vi.fn(() => configResult),
  useGetMCPProxy: vi.fn(() => idle),
  useGetProjectMCPProxy: vi.fn(() => idle),
  useListEnvironments: vi.fn(() => envsResult),
  useListAllMCPProxies: vi.fn(() => proxiesResult),
  useListMCPProxyScopes: vi.fn(() => idle),
  useRefreshProjectMCPProxyCapabilities: vi.fn(() => mutationResult),
  useUpdateAgentMCPConfig: vi.fn(() => mutationResult),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  usePipelineEnvironmentsState: () => pipelineResult,
  useAgentIdentityCredentials: () => idle,
  useConfirmIfUnsaved: () => (fn: () => void) => fn(),
  useThunderInstanceForEnv: () => idle,
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));
vi.mock("@agent-management-platform/mcp-proxies", () => ({
  getCapabilityId: () => undefined,
  isToolBlockedByAcl: () => false,
  MCPLogo: () => null,
}));
vi.mock("./utils/buildInProgress", () => ({
  BUILD_IN_PROGRESS_REASON: "",
  useHasBuildInProgress: () => false,
}));

import {
  useGetMCPProxy,
  useGetProjectMCPProxy,
  useListAllMCPProxies,
} from "@agent-management-platform/api-client";
import { ViewMCPServerComponent } from "./ViewMCPServer.Component";

const renderPage = () =>
  render(
    <MemoryRouter initialEntries={["/organizations/o/projects/p1/agent/a1/mcp/cfg"]}>
      <Routes>
        <Route
          path="/organizations/:orgId/projects/:projectId/agent/:agentId/mcp/:proxyId"
          element={<ViewMCPServerComponent />}
        />
      </Routes>
    </MemoryRouter>,
  );

describe("ViewMCPServerComponent", () => {
  it("lists project proxies for the agent's project and resolves their display name", () => {
    renderPage();
    expect(useListAllMCPProxies).toHaveBeenCalledWith(
      { orgName: "o" },
      expect.objectContaining({ project: "p1" }),
    );
    expect(screen.getAllByText(/Project Proxy Display/).length).toBeGreaterThan(0);
  });

  it("does not link a project proxy to the organization's MCP proxy page", () => {
    renderPage();
    expect(screen.getAllByText(/Project Proxy Display/).length).toBeGreaterThan(0);
    expect(
      screen.queryByRole("button", { name: /in the organization/ }),
    ).toBeNull();
  });

  it("links an org proxy to the organization's MCP proxy page", () => {
    vi.mocked(useListAllMCPProxies).mockReturnValue(
      orgProxiesResult as unknown as ReturnType<typeof useListAllMCPProxies>,
    );
    renderPage();
    expect(
      screen.getByRole("button", { name: /Org Proxy Display in the organization/ }),
    ).toBeTruthy();
  });

  it("reads a project proxy's security from its project route", () => {
    const projectProxyResult = {
      data: { id: "proj-proxy", name: "Project Proxy Display", version: "v1", projectName: "p1" },
      isLoading: false,
      isError: false,
    };
    vi.mocked(useGetMCPProxy).mockReturnValue(
      projectProxyResult as unknown as ReturnType<typeof useGetMCPProxy>,
    );
    renderPage();
    expect(useGetProjectMCPProxy).toHaveBeenCalledWith({
      orgName: "o",
      projName: "p1",
      proxyId: "proj-proxy",
    });
  });
});
