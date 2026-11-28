
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

import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

const idle = { data: undefined, isLoading: false, isError: false };
const agentResult = { data: undefined };

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
  data: {
    list: [
      {
        id: "proj-proxy",
        name: "Project Proxy Display",
        projectName: "p1",
      },
    ],
  },
  isLoading: false,
};

const orgProxiesResult = {
  data: { list: [{ id: "proj-proxy", name: "Org Proxy Display" }] },
  isLoading: false,
};

const pipelineResult = { environments: [], isLoading: false };
const mutationResult = { mutate: vi.fn(), isPending: false };

vi.mock("@agent-management-platform/api-client", () => ({
  useTrack: vi.fn(() => vi.fn()),
  useGetAgent: vi.fn(() => agentResult),
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

// Key management is independent of the connection guide tested here.
vi.mock("./Configure/subComponents/MCPProxyAPIKeysSection", () => ({
  MCPProxyAPIKeysSection: () => null,
}));

vi.mock("./utils/buildInProgress", () => ({
  BUILD_IN_PROGRESS_REASON: "",
  useHasBuildInProgress: () => false,
}));

import {
  useGetAgent,
  useGetAgentMCPConfig,
  useGetMCPProxy,
  useGetProjectMCPProxy,
  useListEnvironments,
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

// Configure an external MCP endpoint with optional API-key security.
function setupMCPEndpoint(headerName?: string, apiKey?: string) {
  const externalAgentResult = {
    data: { provisioning: { type: "external" } },
  };

  const externalConfigResult = {
    data: {
      ...configResult.data,
      envMappings: {
        dev: {
          configuration: {
            proxyName: "proj-proxy",
            url: "https://example.com/mcp",
            ...(headerName && apiKey !== undefined
              ? {
                  authInfo: {
                    name: headerName,
                    value: apiKey,
                  },
                }
              : {}),
          },
        },
      },
    },
    isLoading: false,
    isError: false,
  };

  const environmentResult = {
    data: [
      {
        id: "dev-uuid",
        name: "dev",
        displayName: "Development",
      },
    ],
    isError: false,
  };

  const proxyData = {
    id: "proj-proxy",
    name: "Project Proxy Display",
    version: "v1",
    projectName: "p1",
  };

  const projectProxyResult = {
    data: {
      ...proxyData,
      environments: [
        {
          environmentName: "dev",
          environmentUuid: "dev-uuid",
          security: {
            enabled: true,
            apiKey: {
              enabled: Boolean(headerName),
              key: headerName ?? "",
              in: "header",
            },
            identity: { enabled: false },
          },
        },
      ],
    },
    isLoading: false,
    isError: false,
  };

  vi.mocked(useGetAgent).mockReturnValue(
    externalAgentResult as unknown as ReturnType<typeof useGetAgent>,
  );

  vi.mocked(useGetAgentMCPConfig).mockReturnValue(
    externalConfigResult as unknown as ReturnType<typeof useGetAgentMCPConfig>,
  );

  vi.mocked(useListEnvironments).mockReturnValue(
    environmentResult as unknown as ReturnType<typeof useListEnvironments>,
  );

  vi.mocked(useGetMCPProxy).mockReturnValue(
    {
      data: proxyData,
      isLoading: false,
      isError: false,
    } as unknown as ReturnType<typeof useGetMCPProxy>,
  );

  vi.mocked(useGetProjectMCPProxy).mockReturnValue(
    projectProxyResult as unknown as ReturnType<typeof useGetProjectMCPProxy>,
  );
}

function getCurlCommand(): string {
  fireEvent.click(
    screen.getByRole("button", { name: "Connect to MCP Server" }),
  );

  const curlSample = screen.getByText(
    (_, element) =>
      element?.tagName.toLowerCase() === "code" &&
      /^curl\s/.test(element.textContent?.trim() ?? ""),
  );

  return curlSample.textContent ?? "";
}

function expectInitializeRequest(command: string) {
  expect(command).toContain("https://example.com/mcp");
  expect(command).toContain("POST");
  expect(command).toContain("Content-Type: application/json");
  expect(command).toContain("Accept: application/json, text/event-stream");
  expect(command).toContain('"jsonrpc":"2.0"');
  expect(command).toContain('"method":"initialize"');
  expect(command).toContain('"protocolVersion":"2025-06-18"');
  expect(command).toContain('"capabilities":{}');
  expect(command).toContain('"clientInfo"');
}

describe("ViewMCPServerComponent", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    vi.mocked(useGetAgent).mockReturnValue(
      agentResult as unknown as ReturnType<typeof useGetAgent>,
    );

    vi.mocked(useGetAgentMCPConfig).mockReturnValue(
      configResult as unknown as ReturnType<typeof useGetAgentMCPConfig>,
    );

    vi.mocked(useGetMCPProxy).mockReturnValue(
      idle as unknown as ReturnType<typeof useGetMCPProxy>,
    );

    vi.mocked(useGetProjectMCPProxy).mockReturnValue(
      idle as unknown as ReturnType<typeof useGetProjectMCPProxy>,
    );

    vi.mocked(useListAllMCPProxies).mockReturnValue(
      proxiesResult as unknown as ReturnType<typeof useListAllMCPProxies>,
    );

    vi.mocked(useListEnvironments).mockReturnValue(
      envsResult as unknown as ReturnType<typeof useListEnvironments>,
    );
  });

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
      data: {
        id: "proj-proxy",
        name: "Project Proxy Display",
        version: "v1",
        projectName: "p1",
      },
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

  it("generates an MCP initialize request for an unsecured external endpoint", () => {
    setupMCPEndpoint();
    renderPage();

    const command = getCurlCommand();

    expectInitializeRequest(command);
    expect(command).not.toContain("X-API-Key");
    expect(command).not.toContain("<api-key>");
  });

  it("generates an MCP initialize request with an API key", () => {
    setupMCPEndpoint("X-API-Key", "test-api-key");
    renderPage();

    const command = getCurlCommand();

    expectInitializeRequest(command);
    expect(command).toContain("X-API-Key: test-api-key");
  });

  it("preserves the API key placeholder when the key is unavailable", () => {
    setupMCPEndpoint("X-API-Key");
    renderPage();

    const command = getCurlCommand();

    expectInitializeRequest(command);
    expect(command).toContain("X-API-Key: <api-key>");
  });

  it("preserves a custom API key header in the initialize request", () => {
    setupMCPEndpoint("X-Custom-Key", "test-api-key");
    renderPage();

    const command = getCurlCommand();

    expectInitializeRequest(command);
    expect(command).toContain("X-Custom-Key: test-api-key");
    expect(command).not.toContain("X-API-Key:");
  });
});
