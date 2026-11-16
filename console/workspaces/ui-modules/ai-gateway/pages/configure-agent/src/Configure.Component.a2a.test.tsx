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

import type { ReactNode } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mutation = { mutate: vi.fn(), isPending: false };
const emptyList = { data: { configs: [] }, isLoading: false, error: null };

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: { TabSwitch: "tab_switch" },
  useTrack: () => ({ track: vi.fn() }),
  useGetAgent: vi.fn(),
  useGetAllAgentBuilds: () => ({ data: { builds: [] }, isLoading: false }),
  useGetAgentBuilds: () => ({ data: undefined, isLoading: false }),
  useListAgentModelConfigs: () => emptyList,
  useListAgentMCPConfigs: () => emptyList,
  useDeleteAgentModelConfig: () => mutation,
  useDeleteAgentMCPConfig: () => mutation,
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  usePipelineEnvironmentsState: () => ({
    environments: [{ name: "dev", displayName: "Development" }, { name: "prod", displayName: "Production" }],
    isLoading: false,
    isError: false,
  }),
}));
vi.mock("@agent-management-platform/views", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@agent-management-platform/views")>()),
  PageLayout: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
vi.mock("./Configure/subComponents/AgentConfigTableSection", () => ({
  AgentConfigTableSection: () => <div>config table</div>,
}));
vi.mock("./Configure/subComponents/AddMCPToolConfigPanel", () => ({
  AddMCPToolConfigPanel: () => null,
}));
vi.mock("./Configure/subComponents/A2AAgentCardPanel", () => ({
  A2AAgentCardPanel: ({ envId, external }: { envId: string; external: boolean }) => (
    <div>{`card panel ${envId} ${external ? "external" : "platform"}`}</div>
  ),
}));

import { useGetAgent } from "@agent-management-platform/api-client";
import { ConfigureComponent } from "./Configure.Component";

const mockAgent = (subType: string, provisioning = "internal") =>
  vi.mocked(useGetAgent).mockReturnValue({
    data: {
      agentType: { type: provisioning === "external" ? "external-agent-api" : "agent-api", subType },
      provisioning: { type: provisioning },
    },
  } as unknown as ReturnType<typeof useGetAgent>);

function SearchProbe() {
  return <div data-testid="search">{useLocation().search}</div>;
}

const renderPage = (search: string) =>
  render(
    <MemoryRouter initialEntries={[`/org/o/project/p/agents/a/configure${search}`]}>
      <Routes>
        <Route
          path="/org/:orgId/project/:projectId/agents/:agentId/configure"
          element={<><ConfigureComponent /><SearchProbe /></>}
        />
      </Routes>
    </MemoryRouter>,
  );

beforeEach(() => {
  vi.clearAllMocks();
});

describe("Configure page A2A tab", () => {
  it("is hidden for a non-A2A agent, which falls back to the first tab", () => {
    mockAgent("chat-api");
    renderPage("?tab=a2a");
    expect(screen.queryByRole("tab", { name: "A2A" })).not.toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "LLM Configurations" })).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByText(/card panel/)).not.toBeInTheDocument();
  });

  it("is shown for every external agent, which registers its card URL there", () => {
    vi.mocked(useGetAgent).mockReturnValue({
      data: { agentType: { type: "external-agent-api", subType: "custom-api" }, provisioning: { type: "external" } },
    } as unknown as ReturnType<typeof useGetAgent>);
    renderPage("?tab=a2a");
    expect(screen.getByRole("tab", { name: "A2A" })).toHaveAttribute("aria-selected", "true");
  });

  it("opens on the environment named in the URL", () => {
    mockAgent("a2a-agent", "external");
    renderPage("?tab=a2a&env=prod");
    expect(screen.getByRole("tab", { name: "A2A" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Production" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("card panel prod external")).toBeInTheDocument();
  });

  it("defaults to the first environment and records a switch in the URL", () => {
    mockAgent("a2a-agent");
    renderPage("?tab=a2a");
    expect(screen.getByText("card panel dev platform")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "Production" }));
    expect(screen.getByText("card panel prod platform")).toBeInTheDocument();
    expect(screen.getByTestId("search")).toHaveTextContent("env=prod");
  });
});
