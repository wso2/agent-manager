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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { render, screen, fireEvent } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { SnackBarProvider } from "@agent-management-platform/views";
import { vi } from "vitest";
import { ConfigureComponent } from "./index";

// ConfigureComponent (and the always-mounted AddMCPToolConfigPanel drawer
// beneath its MCP tab) call real TanStack Query hooks, which need a
// QueryClientProvider the real app only supplies at the shell level. Stub
// the api-client module boundary instead of wiring up react-query here.
vi.mock("@agent-management-platform/api-client", () => ({
  useListAgentModelConfigs: vi.fn(() => ({ data: { configs: [] }, isLoading: false, error: null })),
  useListAgentMCPConfigs: vi.fn(() => ({ data: { configs: [] }, isLoading: false, error: null })),
  useDeleteAgentModelConfig: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  useDeleteAgentMCPConfig: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
  // AddMCPToolConfigPanel is always mounted (only its Drawer content hides
  // when closed), so its hooks run regardless of which tab is active.
  useGetAgent: vi.fn(() => ({ data: undefined, isLoading: false })),
  useListMCPProxies: vi.fn(() => ({ data: { list: [] }, isLoading: false })),
  useGetMCPProxy: vi.fn(() => ({ data: undefined, isLoading: false })),
  useCreateAgentMCPConfig: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  // Consumed by AddMCPToolConfigPanel via usePipelineEnvironmentsState.
  useListEnvironments: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useGetProject: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
  useListDeploymentPipelines: vi.fn(() => ({ data: undefined, isLoading: false, isError: false })),
}));

import {
  useListAgentModelConfigs,
  useListAgentMCPConfigs,
} from "@agent-management-platform/api-client";

const mockUseListAgentModelConfigs = vi.mocked(useListAgentModelConfigs);
const mockUseListAgentMCPConfigs = vi.mocked(useListAgentMCPConfigs);

const route = "/org/org1/project/proj1/agents/agent1/configure";

function renderWithRouter() {
  return render(
    <SnackBarProvider>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }} initialEntries={[route]} initialIndex={0}>
        <Routes>
          <Route
            path="/org/:orgId/project/:projectId/agents/:agentId/configure"
            element={<ConfigureComponent />}
          />
        </Routes>
      </MemoryRouter>
    </SnackBarProvider>,
  );
}

describe("ConfigureComponent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseListAgentModelConfigs.mockReturnValue({
      data: { configs: [] },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useListAgentModelConfigs>);
    mockUseListAgentMCPConfigs.mockReturnValue({
      data: { configs: [] },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useListAgentMCPConfigs>);
  });

  it("renders the page title and defaults to the LLM Configurations tab", () => {
    renderWithRouter();

    expect(screen.getByText("Configure Agent")).toBeInTheDocument();
    expect(screen.getByText("No LLM configurations added yet")).toBeInTheDocument();
  });

  it("lists LLM configurations by name when present", () => {
    mockUseListAgentModelConfigs.mockReturnValue({
      data: { configs: [{ uuid: "cfg-1", name: "openai-gpt4", createdAt: "2026-01-01T00:00:00Z" }] },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useListAgentModelConfigs>);

    renderWithRouter();

    expect(screen.getByText("openai-gpt4")).toBeInTheDocument();
  });

  it("switches to the Tool Configurations tab and shows its empty state", () => {
    renderWithRouter();

    fireEvent.click(screen.getByRole("tab", { name: "Tool Configurations" }));

    expect(screen.getByText("No tool configurations added yet")).toBeInTheDocument();
  });

  it("lists MCP tool configurations by name once on the Tool Configurations tab", () => {
    mockUseListAgentMCPConfigs.mockReturnValue({
      data: { configs: [{ uuid: "cfg-2", name: "github-mcp", createdAt: "2026-01-01T00:00:00Z" }] },
      isLoading: false,
      error: null,
    } as unknown as ReturnType<typeof useListAgentMCPConfigs>);

    renderWithRouter();
    fireEvent.click(screen.getByRole("tab", { name: "Tool Configurations" }));

    expect(screen.getByText("github-mcp")).toBeInTheDocument();
  });
});
