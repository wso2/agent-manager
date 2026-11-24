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

// LLM and tool configuration listings of the Configure page (the A2A tab has
// its own test file).

import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { FeatureManagementProvider } from "@agent-management-platform/feature-management";
import { ConfirmationDialogProvider } from "@agent-management-platform/shared-component";

type ListResult = { data?: unknown; isLoading: boolean; error: unknown };

const state = vi.hoisted(() => ({
  llm: { data: undefined, isLoading: false, error: null } as ListResult,
  mcp: { data: undefined, isLoading: false, error: null } as ListResult,
  deleteLLM: (() => undefined) as unknown as ReturnType<typeof vi.fn>,
  deleteMCP: (() => undefined) as unknown as ReturnType<typeof vi.fn>,
  track: (() => undefined) as unknown as ReturnType<typeof vi.fn>,
  bindings: undefined as unknown,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: { TabSwitch: "tab_switch", ConfirmationCancelled: "cancelled" },
  useTrack: () => ({ track: state.track }),
  useGetAgent: () => ({ data: { agentType: { type: "agent-api", subType: "chat-api" }, provisioning: { type: "internal" } } }),
  useListAgentModelConfigs: () => state.llm,
  useListAgentMCPConfigs: () => state.mcp,
  useDeleteAgentModelConfig: () => ({ mutate: state.deleteLLM, isPending: false }),
  useDeleteAgentMCPConfig: () => ({ mutate: state.deleteMCP, isPending: false }),
  useGetAgentConfigurations: () => ({ data: state.bindings }),
}));
vi.mock("./Configure/subComponents/AddMCPToolConfigPanel", () => ({
  AddMCPToolConfigPanel: ({ open, onClose }: { open: boolean; onClose: () => void }) =>
    open ? <button onClick={onClose}>close add tool panel</button> : null,
}));

import { ConfigureComponent } from "./Configure.Component";

function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location">{`${location.pathname}${location.search}`}</div>;
}

const renderPage = (search = "", canWrite = true) =>
  render(
    <FeatureManagementProvider featureAccess={{ agent_write: canWrite }}>
      <ConfirmationDialogProvider>
        <MemoryRouter
          future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
          initialEntries={[`/org/o/project/p/agents/a/configure${search}`]}
        >
          <Routes>
            <Route
              path="/org/:orgId/project/:projectId/agents/:agentId/configure"
              element={<><ConfigureComponent /><LocationProbe /></>}
            />
            <Route path="*" element={<LocationProbe />} />
          </Routes>
        </MemoryRouter>
      </ConfirmationDialogProvider>
    </FeatureManagementProvider>,
  );

const configs = (names: string[], type = "llm") => ({
  data: {
    configs: names.map((name, i) => ({ uuid: `${type}-${i}`, name, type, description: `${name} desc` })),
  },
  isLoading: false,
  error: null,
});

beforeEach(() => {
  state.llm = configs(["openai-chat", "anthropic-chat"]);
  state.mcp = configs(["weather"], "mcp");
  state.deleteLLM = vi.fn();
  state.deleteMCP = vi.fn();
  state.track = vi.fn();
  state.bindings = undefined;
});

describe("ConfigureComponent", () => {
  it("lists the agent's LLM configurations by default", () => {
    renderPage();
    expect(screen.getByRole("tab", { name: "LLM Configurations" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("openai-chat")).toBeInTheDocument();
    expect(screen.getByText("anthropic-chat")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add LLM configuration" })).toHaveAttribute(
      "href",
      "/org/o/project/p/agents/a/configure/llm-providers/add",
    );
  });

  it("shows a loading skeleton instead of the table", () => {
    state.llm = { data: undefined, isLoading: true, error: null };
    renderPage();
    expect(screen.queryByRole("columnheader", { name: "Name" })).not.toBeInTheDocument();
    expect(screen.queryByText("No LLM configurations added yet")).not.toBeInTheDocument();
  });

  it("shows an empty state with an add action", () => {
    state.llm = configs([]);
    renderPage();
    expect(screen.getByText("No LLM configurations added yet")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Add LLM configuration" })).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search LLM configurations")).not.toBeInTheDocument();
  });

  it("shows the load error", () => {
    state.llm = { data: undefined, isLoading: false, error: new Error("backend down") };
    renderPage();
    expect(screen.getByText("Failed to load LLM configurations")).toBeInTheDocument();
    expect(screen.getByText("backend down")).toBeInTheDocument();
  });

  it("filters configurations by search", () => {
    renderPage();
    const search = screen.getByPlaceholderText("Search LLM configurations");
    fireEvent.change(search, { target: { value: "anthropic" } });
    expect(screen.queryByText("openai-chat")).not.toBeInTheDocument();
    expect(screen.getByText("anthropic-chat")).toBeInTheDocument();
    fireEvent.change(search, { target: { value: "gemini" } });
    expect(screen.getByText("No LLM configurations match your search")).toBeInTheDocument();
  });

  it("opens a configuration when its row is clicked", () => {
    renderPage();
    fireEvent.click(screen.getByText("openai-chat"));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/org/o/project/p/agents/a/configure/llm-providers/view/llm-0",
    );
  });

  it("removes an LLM configuration after confirmation", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Remove configuration openai-chat" }));
    expect(screen.getByText("Remove LLM Configuration")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    expect(state.deleteLLM).toHaveBeenCalledWith({
      orgName: "o",
      projName: "p",
      agentName: "a",
      configId: "llm-0",
    });
  });

  it("does not remove when the confirmation is cancelled", () => {
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: "Remove configuration openai-chat" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(state.deleteLLM).not.toHaveBeenCalled();
  });

  it("paginates long lists", () => {
    state.llm = configs(Array.from({ length: 12 }, (_, i) => `cfg-${i + 1}`));
    renderPage();
    expect(screen.getByText("cfg-10")).toBeInTheDocument();
    expect(screen.queryByText("cfg-11")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    expect(screen.getByText("cfg-11")).toBeInTheDocument();
    expect(screen.queryByText("cfg-1")).not.toBeInTheDocument();
  });

  it("switches to the tools tab, records it and keeps other params", () => {
    renderPage("?env=prod");
    fireEvent.click(screen.getByRole("tab", { name: "Tool Configurations" }));
    expect(state.track).toHaveBeenCalledWith("tab_switch", { page: "configure-agent", tab: "tools" });
    expect(screen.getAllByTestId("location")[0]).toHaveTextContent("env=prod");
    expect(screen.getAllByTestId("location")[0]).toHaveTextContent("tab=tools");
    expect(screen.getByText("weather")).toBeInTheDocument();
  });

  it("adds tool configurations through the side panel and removes them", () => {
    renderPage("?tab=tools");
    fireEvent.click(screen.getByRole("button", { name: "Add tool configuration" }));
    fireEvent.click(screen.getByRole("button", { name: "close add tool panel" }));
    expect(screen.queryByRole("button", { name: "close add tool panel" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Remove weather" }));
    expect(screen.getByText('Are you sure you want to remove "weather" from this agent?')).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    expect(state.deleteMCP).toHaveBeenCalledWith({
      orgName: "o",
      projName: "p",
      agentName: "a",
      configId: "mcp-0",
    });
  });

  it("opens a tool configuration with its id encoded", () => {
    state.mcp = { data: { configs: [{ uuid: "a/b", name: "slashy", type: "mcp" }] }, isLoading: false, error: null };
    renderPage("?tab=tools");
    fireEvent.click(screen.getByText("slashy"));
    expect(screen.getByTestId("location")).toHaveTextContent("a%2Fb");
  });

  it("says which bindings are still being set up", () => {
    state.bindings = {
      bindings: [
        { kind: "llm", target: "openai", state: "pending" },
        { kind: "mcp", target: "weather", state: "pending" },
        { kind: "llm", target: "done", state: "ready" },
      ],
    };
    renderPage();
    expect(
      screen.getByText("Setting up LLM provider openai, MCP proxy weather. It appears here once ready."),
    ).toBeInTheDocument();
  });

  it("surfaces bindings that failed to provision", () => {
    state.bindings = {
      bindings: [
        { kind: "mcp", target: "weather", state: "failed", lastError: "gateway unreachable" },
        { kind: "llm", target: "openai", state: "pending" },
      ],
    };
    renderPage();
    expect(
      screen.getByText("Setting up MCP proxy weather failed and is retried: gateway unreachable"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/It appears here once ready/)).not.toBeInTheDocument();
  });

  it("hides add and remove actions without write access", () => {
    renderPage("", false);
    expect(screen.getByText("openai-chat")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Add LLM configuration" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Remove configuration/ })).not.toBeInTheDocument();
  });
});
