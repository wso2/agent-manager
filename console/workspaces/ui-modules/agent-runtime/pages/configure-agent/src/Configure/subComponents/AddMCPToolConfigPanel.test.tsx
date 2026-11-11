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

import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

vi.mock("@agent-management-platform/api-client", () => ({
  useListMCPProxies: vi.fn(() => ({
    data: {
      list: [{ id: "proj-proxy", name: "Project Proxy", projectName: "p1" }],
    },
    isLoading: false,
  })),
  useGetAgent: vi.fn(() => ({ data: undefined })),
  useListAgentMCPConfigs: vi.fn(() => ({ data: undefined })),
  useCreateAgentMCPConfig: vi.fn(() => ({ mutate: vi.fn(), isError: false, isPending: false })),
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  AGENTID_ENV_VAR_ROWS: [],
  EnvironmentVariablesReference: () => null,
  useMCPProxySecurity: () => ({
    authenticationType: "",
    usesIdentitySecurity: false,
    spec: { editableKeys: [] },
    isLoading: false,
    isResolved: true,
  }),
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));
vi.mock("./MCPServerDisplay", () => ({
  MCPServerDisplay: ({ server }: { server: { name?: string } | null }) => (
    <span>{server?.name}</span>
  ),
}));

import { useListMCPProxies } from "@agent-management-platform/api-client";
import { AddMCPToolConfigPanel } from "./AddMCPToolConfigPanel";

describe("AddMCPToolConfigPanel", () => {
  it("lists project proxies for the agent's project", () => {
    render(
      <MemoryRouter>
        <AddMCPToolConfigPanel open orgId="org" projectId="p1" agentId="a1" onClose={vi.fn()} />
      </MemoryRouter>,
    );
    expect(useListMCPProxies).toHaveBeenCalledWith(
      { orgName: "org" },
      expect.objectContaining({ project: "p1" }),
    );
    expect(screen.getByText("Project Proxy")).toBeTruthy();
  });
});
